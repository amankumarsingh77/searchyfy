package query

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
)

const (
	BM25_K1 = 1.2
	BM25_B  = 0.75
)

type DocumentLength struct {
	DocID      int64
	TokenCount int
	Normalized float64
}

func (e *QueryEngine) rankResultsOptimized(ctx context.Context, docIDs []int64, plan *QueryPlan) ([]ScoredDoc, error) {
	if len(docIDs) == 0 {
		return nil, nil
	}

	docLengths, err := e.getDocumentLengthsBatch(ctx, docIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get document lengths: %w", err)
	}

	idfValues, err := e.getIDFBatch(ctx, plan.termIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get IDF values: %w", err)
	}

	termFreqs, err := e.getTermFrequenciesBatch(ctx, docIDs, plan.termIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get term frequencies: %w", err)
	}

	scoredDocs := make([]ScoredDoc, len(docIDs))
	var wg sync.WaitGroup

	numWorkers := e.maxWorkers
	if numWorkers > len(docIDs) {
		numWorkers = len(docIDs)
	}

	jobChan := make(chan int, len(docIDs))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobChan {
				docID := docIDs[idx]
				score := e.calculateBM25Score(docID, plan.termIDs, docLengths[docID], idfValues, termFreqs)
				scoredDocs[idx] = ScoredDoc{DocID: docID, Score: score}
			}
		}()
	}

	go func() {
		for i := range docIDs {
			jobChan <- i
		}
		close(jobChan)
	}()

	wg.Wait()

	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].Score > scoredDocs[j].Score
	})

	return scoredDocs, nil
}

func (e *QueryEngine) getDocumentLengthsBatch(ctx context.Context, docIDs []int64) (map[int64]DocumentLength, error) {
	result := make(map[int64]DocumentLength, len(docIDs))
	var missingDocIDs []int64

	for _, docID := range docIDs {
		cacheKey := fmt.Sprintf("doc_len_%d", docID)
		if val, ok := e.docCache.Get(cacheKey); ok {
			if docLen, ok := val.(DocumentLength); ok {
				result[docID] = docLen
				continue
			}
		}
		missingDocIDs = append(missingDocIDs, docID)
	}

	if len(missingDocIDs) > 0 {
		avgTokenCount := e.getAvgTokenCount()

		rows, err := e.pool.Query(ctx, "SELECT id, token_count FROM documents WHERE id = ANY($1)", missingDocIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch document lengths: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var docID int64
			var tokenCount int

			if err := rows.Scan(&docID, &tokenCount); err != nil {
				continue
			}

			docLen := DocumentLength{
				DocID:      docID,
				TokenCount: tokenCount,
				Normalized: float64(tokenCount) / avgTokenCount,
			}

			result[docID] = docLen

			cacheKey := fmt.Sprintf("doc_len_%d", docID)
			e.docCache.Put(cacheKey, docLen)
		}
	}

	return result, nil
}

func (e *QueryEngine) getTermFrequenciesBatch(ctx context.Context, docIDs []int64, termIDs []int64) (map[string]int, error) {
	result := make(map[string]int)

	query := `
		SELECT doc_id, term_id, array_length(positions, 1) as tf
		FROM postings 
		WHERE doc_id = ANY($1) AND term_id = ANY($2)
	`

	rows, err := e.pool.Query(ctx, query, docIDs, termIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch term frequencies: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var docID, termID int64
		var tf int

		if err := rows.Scan(&docID, &termID, &tf); err != nil {
			continue
		}

		key := fmt.Sprintf("%d_%d", docID, termID)
		result[key] = tf
	}

	return result, nil
}

func (e *QueryEngine) calculateBM25Score(
	docID int64,
	termIDs []int64,
	docLength DocumentLength,
	idfValues map[int64]float64,
	termFreqs map[string]int,
) float64 {
	score := 0.0

	for _, termID := range termIDs {
		key := fmt.Sprintf("%d_%d", docID, termID)
		tf := termFreqs[key]

		if tf == 0 {
			continue
		}

		idf, exists := idfValues[termID]
		if !exists {
			continue
		}

		// BM25 formula: IDF * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * (|d| / avgdl)))
		tfComponent := float64(tf) * (BM25_K1 + 1)
		denominator := float64(tf) + BM25_K1*(1-BM25_B+BM25_B*docLength.Normalized)

		score += idf * (tfComponent / denominator)
	}

	return score
}

// Alternative scoring methods for experimentation

func (e *QueryEngine) calculateTFIDFScore(
	docID int64,
	termIDs []int64,
	docLength DocumentLength,
	idfValues map[int64]float64,
	termFreqs map[string]int,
) float64 {
	score := 0.0

	for _, termID := range termIDs {
		key := fmt.Sprintf("%d_%d", docID, termID)
		tf := termFreqs[key]

		if tf == 0 {
			continue
		}

		idf, exists := idfValues[termID]
		if !exists {
			continue
		}

		// TF-IDF: (1 + log(tf)) * IDF
		tfScore := 1.0 + math.Log(float64(tf))
		score += tfScore * idf
	}

	return score
}

func (e *QueryEngine) calculateCosineSimilarity(
	docID int64,
	termIDs []int64,
	docLength DocumentLength,
	idfValues map[int64]float64,
	termFreqs map[string]int,
) float64 {
	var dotProduct, docNorm, queryNorm float64

	for _, termID := range termIDs {
		key := fmt.Sprintf("%d_%d", docID, termID)
		tf := termFreqs[key]

		idf, exists := idfValues[termID]
		if !exists {
			continue
		}

		docWeight := float64(tf) * idf
		queryWeight := 1.0 * idf
		dotProduct += docWeight * queryWeight
		docNorm += docWeight * docWeight
		queryNorm += queryWeight * queryWeight
	}

	if docNorm == 0 || queryNorm == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(docNorm) * math.Sqrt(queryNorm))
}

func (e *QueryEngine) calculateHybridScore(
	docID int64,
	termIDs []int64,
	docLength DocumentLength,
	idfValues map[int64]float64,
	termFreqs map[string]int,
	plan *QueryPlan,
) float64 {
	bm25Score := e.calculateBM25Score(docID, termIDs, docLength, idfValues, termFreqs)

	score := bm25Score

	if plan.operator == "PHRASE" {
		score *= 1.2
	}

	if docLength.TokenCount < 500 {
		score *= 1.1
	}

	termCount := 0
	for _, termID := range termIDs {
		key := fmt.Sprintf("%d_%d", docID, termID)
		if termFreqs[key] > 0 {
			termCount++
		}
	}

	if termCount > 1 {
		score *= 1.0 + (0.1 * float64(termCount-1))
	}

	return score
}

func (e *QueryEngine) batchScoreDocuments(ctx context.Context, docIDs []int64, plan *QueryPlan, batchSize int) ([]ScoredDoc, error) {
	var allScoredDocs []ScoredDoc

	for i := 0; i < len(docIDs); i += batchSize {
		end := i + batchSize
		if end > len(docIDs) {
			end = len(docIDs)
		}

		batch := docIDs[i:end]
		scoredBatch, err := e.rankResultsOptimized(ctx, batch, plan)
		if err != nil {
			return nil, err
		}

		allScoredDocs = append(allScoredDocs, scoredBatch...)
	}

	sort.Slice(allScoredDocs, func(i, j int) bool {
		return allScoredDocs[i].Score > allScoredDocs[j].Score
	})

	return allScoredDocs, nil
}
