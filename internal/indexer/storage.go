package indexer

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Storage struct {
	pool      *pgxpool.Pool
	termCache sync.Map
}

func NewPostgresClient(cfg *config.IndexerConfig) (*Storage, error) {
	if cfg.DBURL == "" {
		return nil, fmt.Errorf("DBURL is empty in config")
	}

	pgConfig, err := pgxpool.ParseConfig(cfg.DBURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PostgreSQL config: %w", err)
	}

	pgConfig.MaxConns = int32(cfg.Workers * 2)
	pgConfig.MinConns = 1

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL connection pool: %w", err)
	}

	return &Storage{
		pool: pool,
	}, nil
}

func (s *Storage) InsertDocuments(ctx context.Context, docs []*models.WebPage) ([]int64, error) {
	ids := make([]int64, len(docs))
	batch := &pgx.Batch{}

	for _, doc := range docs {
		url := removeInvalidUTF8(doc.URL)
		title := removeInvalidUTF8(doc.Title)
		desc := removeInvalidUTF8(doc.Description)
		batch.Queue(insertDocuments, url, title, desc, doc.TokenCount)
	}

	res := s.pool.SendBatch(ctx, batch)
	defer res.Close()

	for i := range docs {
		if err := res.QueryRow().Scan(&ids[i]); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *Storage) UpsertTerms(ctx context.Context, terms []string) (map[string]int64, error) {
	termMap := make(map[string]int64)
	var missingTerms []string
	for _, term := range terms {
		if id, ok := s.termCache.Load(term); ok {
			termMap[term] = id.(int64)
		} else {
			missingTerms = append(missingTerms, term)
		}
	}
	if len(missingTerms) == 0 {
		return termMap, nil
	}
	sort.Strings(missingTerms)
	if _, err := s.pool.Exec(ctx, insertMissingTerms, missingTerms); err != nil {
		return nil, fmt.Errorf("error inserting terms: %w ", err)
	}
	rows, err := s.pool.Query(ctx, getIDsByTerms, missingTerms)
	if err != nil {
		return nil, fmt.Errorf("error fetching term IDs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var term string
		if err = rows.Scan(&id, &term); err != nil {
			return nil, err
		}
		termMap[term] = id
		s.termCache.Store(term, id)
	}
	return termMap, nil
}

func (s *Storage) InsertPosting(
	ctx context.Context,
	termMap map[string]int64,
	docIDs []int64,
	positions map[string]map[int][]int,
) error {
	const maxBatchSize = 1000
	type posting struct {
		termID    int64
		docID     int64
		positions []int32
	}

	var allPostings []posting
	for term, docPositions := range positions {
		termID, ok := termMap[term]
		if !ok {
			continue
		}

		for docIdx, pos := range docPositions {
			if docIdx < 0 || docIdx >= len(docIDs) {
				log.Printf("WARNING: Invalid doc index %d for term %s", docIdx, term)
				continue
			}

			docID := docIDs[docIdx]
			int32Positions := make([]int32, len(pos))
			for i, p := range pos {
				int32Positions[i] = int32(p)
			}

			allPostings = append(allPostings, posting{
				termID:    termID,
				docID:     docID,
				positions: int32Positions,
			})
		}
	}

	sort.Slice(allPostings, func(i, j int) bool {
		if allPostings[i].termID == allPostings[j].termID {
			return allPostings[i].docID < allPostings[j].docID
		}
		return allPostings[i].termID < allPostings[j].termID
	})

	for i := 0; i < len(allPostings); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(allPostings) {
			end = len(allPostings)
		}

		batch := &pgx.Batch{}
		for _, p := range allPostings[i:end] {
			batch.Queue(insertPostings, p.termID, p.docID, p.positions)
		}

		results := s.pool.SendBatch(ctx, batch)
		for j := 0; j < batch.Len(); j++ {
			if _, err := results.Exec(); err != nil {
				results.Close()
				return fmt.Errorf("error inserting posting in batch [%d-%d]: %w", i, end, err)
			}
		}
		if err := results.Close(); err != nil {
			return fmt.Errorf("error closing batch: %w", err)
		}
	}

	return nil
}

func (s *Storage) Close() {
	s.pool.Close()
}
