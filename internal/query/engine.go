package query

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/amankumarsingh77/search_engine/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type QueryEngine struct {
	pool         *pgxpool.Pool
	termCache    *LRUCache
	postingCache *LRUCache
	idfCache     *LRUCache
	docCache     *LRUCache

	totalDocs       atomic.Int64
	avgTokenCount   atomic.Uint64
	statsLastUpdate atomic.Int64

	maxWorkers       int
	batchSize        int
	cacheRefreshTime time.Duration

	//stmtGetTerms    *pgx.PreparedStatement
	//stmtGetPostings *pgx.PreparedStatement
	//stmtGetDocs     *pgx.PreparedStatement
	//stmtGetDocFreq  *pgx.PreparedStatement
}

func NewQueryEngine(pool *pgxpool.Pool, cfg *config.QueryEngineConfig) *QueryEngine {
	numWorkers := runtime.NumCPU() * 2
	if cfg.MaxWorkers > 0 {
		numWorkers = cfg.MaxWorkers
	}

	batchSize := 100
	if cfg.BatchSize > 0 {
		batchSize = cfg.BatchSize
	}

	cacheRefreshTime := 5 * time.Minute
	if cfg.CacheRefreshTime > 0 {
		cacheRefreshTime = cfg.CacheRefreshTime
	}

	engine := &QueryEngine{
		pool:             pool,
		termCache:        NewLRUCache(cfg.TermCacheSize, 30*time.Minute),
		postingCache:     NewLRUCache(cfg.PostingCacheSize, 15*time.Minute),
		idfCache:         NewLRUCache(10000, time.Hour),
		docCache:         NewLRUCache(cfg.DocumentCacheSize, 20*time.Minute),
		maxWorkers:       numWorkers,
		batchSize:        batchSize,
		cacheRefreshTime: cacheRefreshTime,
	}

	go engine.refreshGlobalStats()

	go engine.periodicCacheRefresh()

	return engine
}

func (e *QueryEngine) Search(ctx context.Context, rawQuery string, page, pageSize int) ([]SearchResult, int, float64, error) {
	start := time.Now()

	plan := Parse(rawQuery, page, pageSize)
	if len(plan.terms) == 0 {
		return []SearchResult{}, 0, 0.0, nil
	}

	if err := e.resolveTermIDsBatch(ctx, plan); err != nil {
		return nil, 0, 0.0, fmt.Errorf("term resolution failed: %w", err)
	}

	if len(plan.termIDs) == 0 {
		return []SearchResult{}, 0, 0.0, nil
	}

	var docIDs []int64
	var err error

	switch plan.operator {
	case "PHRASE":
		docIDs, err = e.phraseSearchOptimized(ctx, plan)
	default:
		docIDs, err = e.booleanSearchOptimized(ctx, plan)
	}

	if err != nil {
		return nil, 0, 0.0, fmt.Errorf("search failed: %w", err)
	}

	if len(docIDs) == 0 {
		return []SearchResult{}, 0, 0.0, nil
	}

	scoredDocs, err := e.rankResultsOptimized(ctx, docIDs, plan)
	if err != nil {
		return nil, 0, 0.0, fmt.Errorf("ranking failed: %w", err)
	}

	total := len(scoredDocs)
	startIdx := (plan.page - 1) * plan.pageSize
	endIdx := startIdx + plan.pageSize

	if startIdx >= total {
		return []SearchResult{}, total, time.Since(start).Seconds(), nil
	}
	if endIdx > total {
		endIdx = total
	}

	pagedDocs := scoredDocs[startIdx:endIdx]

	results, err := e.fetchDocumentDetailsBatch(ctx, pagedDocs, plan.terms)
	if err != nil {
		return nil, 0, 0.0, fmt.Errorf("fetch details failed: %w", err)
	}

	return results, total, time.Since(start).Seconds(), nil
}

func (e *QueryEngine) refreshGlobalStats() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var totalDocs int64
	if err := e.pool.QueryRow(ctx, getTotalNoDocs).Scan(&totalDocs); err == nil {
		e.totalDocs.Store(totalDocs)
	}

	var avgTokenCount float64
	if err := e.pool.QueryRow(ctx, getAvgTokenCount).Scan(&avgTokenCount); err == nil {
		e.avgTokenCount.Store(uint64(avgTokenCount))
	}

	e.statsLastUpdate.Store(time.Now().Unix())
}

func (e *QueryEngine) periodicCacheRefresh() {
	ticker := time.NewTicker(e.cacheRefreshTime)
	defer ticker.Stop()

	for range ticker.C {
		e.refreshGlobalStats()
	}
}

func (e *QueryEngine) getTotalDocs() int64 {
	if time.Since(time.Unix(e.statsLastUpdate.Load(), 0)) > e.cacheRefreshTime {
		go e.refreshGlobalStats()
	}
	return e.totalDocs.Load()
}

func (e *QueryEngine) getAvgTokenCount() float64 {
	return float64(e.avgTokenCount.Load())
}

func (e *QueryEngine) WarmCache(ctx context.Context, topN int) error {
	rows, err := e.pool.Query(ctx, getTopNQuery, topN)
	if err != nil {
		return fmt.Errorf("failed to get top terms: %w", err)
	}
	defer rows.Close()

	var termIDs []int64
	for rows.Next() {
		var termID int64
		if err := rows.Scan(&termID); err != nil {
			return err
		}
		termIDs = append(termIDs, termID)
	}

	batchSize := e.batchSize
	for i := 0; i < len(termIDs); i += batchSize {
		end := i + batchSize
		if end > len(termIDs) {
			end = len(termIDs)
		}

		batch := termIDs[i:end]
		if err := e.warmPostingCacheBatch(ctx, batch); err != nil {
			return fmt.Errorf("failed to warm cache for batch: %w", err)
		}
	}

	return nil
}

func (e *QueryEngine) warmPostingCacheBatch(ctx context.Context, termIDs []int64) error {
	rows, err := e.pool.Query(ctx, getPostingsByTermIDBatch, termIDs)
	if err != nil {
		return err
	}
	defer rows.Close()

	postingsByTerm := make(map[int64][]Posting)

	for rows.Next() {
		var termID, docID int64
		var positions []int32

		if err := rows.Scan(&termID, &docID, &positions); err != nil {
			continue
		}

		postingsByTerm[termID] = append(postingsByTerm[termID], Posting{
			DocID:     docID,
			Positions: positions,
		})
	}

	for termID, postings := range postingsByTerm {
		e.postingCache.Put(termID, postings)
	}

	return nil
}

func (e *QueryEngine) InitializeDatabase(ctx context.Context) error {
	if _, err := e.pool.Exec(ctx, createOptimizedIndexes); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	if _, err := e.pool.Exec(ctx, createTermFrequencyView); err != nil {
		return fmt.Errorf("failed to create term frequency view: %w", err)
	}

	return nil
}

func (e *QueryEngine) RefreshMaterializedViews(ctx context.Context) error {
	_, err := e.pool.Exec(ctx, refreshTermFrequencies)
	return err
}
