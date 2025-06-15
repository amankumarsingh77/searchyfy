package query

import (
	"context"
	"fmt"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"time"
)

type QueryEngine struct {
	pool         *pgxpool.Pool
	termCache    *LRUCache
	postingCache *LRUCache
	idfCache     *sync.Map
}

func NewQueryEngine(pool *pgxpool.Pool, cfg *config.QueryEngineConfig) *QueryEngine {
	return &QueryEngine{
		pool:         pool,
		termCache:    NewLRUCache(cfg.TermCacheSize),
		postingCache: NewLRUCache(cfg.PostingCacheSize),
		idfCache:     &sync.Map{},
	}
}

func (e *QueryEngine) Search(ctx context.Context, rawQuery string, page, pageSize int) ([]SearchResult, int, float64, error) {
	start := time.Now()
	plan := Parse(rawQuery, page, pageSize)

	if err := e.resolveTermIDs(ctx, plan); err != nil {
		return nil, 0, 0.0, fmt.Errorf("term resolution failed: %w", err)
	}

	var docsIds []int64
	var err error

	switch plan.operator {
	case "PHRASE":
		docsIds, err = e.phraseSearch(ctx, plan)
	default:
		docsIds, err = e.booleanSearch(ctx, plan)
	}
	if err != nil {
		return nil, 0, 0.0, fmt.Errorf("search failed: %w", err)
	}

	scoredDocs, err := e.rankResults(ctx, docsIds, plan)
	if err != nil {
		return nil, 0, 0.0, fmt.Errorf("ranking failed: %w", err)
	}

	total := len(scoredDocs)
	startIdx := (plan.page - 1) * plan.pageSize
	endIdx := startIdx + plan.pageSize

	if startIdx > total {
		return []SearchResult{}, total, 0.0, nil
	}
	if endIdx > total {
		endIdx = total
	}

	pagedDocs := scoredDocs[startIdx:endIdx]
	results, err := e.fetchDocumentDetails(ctx, pagedDocs)
	if err != nil {
		return nil, 0, 0.0, fmt.Errorf("fetch details failed: %w", err)
	}

	endTime := time.Since(start)
	return results, total, endTime.Seconds(), nil
}

func (e *QueryEngine) WarmCache(ctx context.Context, topN int) error {
	rows, err := e.pool.Query(ctx, getTopNQuery, topN)
	if err != nil {
		return err
	}
	var termIds []int64
	for rows.Next() {
		var termID int64
		if err = rows.Scan(&termID); err != nil {
			return err
		}
		termIds = append(termIds, termID)
	}
	var wg sync.WaitGroup
	for _, termID := range termIds {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			e.getPostings(ctx, termID)
		}(termID)
	}
	wg.Wait()
	return nil
}
