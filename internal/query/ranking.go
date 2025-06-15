package query

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
)

func (e *QueryEngine) rankResults(ctx context.Context, docIDs []int64, plan *QueryPlan) ([]ScoredDOC, error) {
	if len(docIDs) == 0 {
		return nil, fmt.Errorf("no docIDs found to rank")
	}

	var totalDocs int
	err := e.pool.QueryRow(ctx, getTotalNoDocs).Scan(&totalDocs)
	if err != nil {
		return nil, err
	}
	//log.Println()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		scored = make([]ScoredDOC, len(docIDs))
		errCh  = make(chan error, 1)
		sem    = make(chan struct{}, 20)
	)

	for i, docID := range docIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, docId int64) {
			defer wg.Done()
			defer func() { <-sem }()
			score, err := e.calculateBM25(ctx, docID, plan.termIDs, totalDocs)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			mu.Lock()
			scored[i] = ScoredDOC{DocID: docID, Score: score}
			mu.Unlock()
		}(i, docID)
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return nil, err
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored, nil
}

func (e *QueryEngine) calculateBM25(
	ctx context.Context,
	docID int64,
	termIDs []int64,
	totalDocs int,
) (float64, error) {

	score := 0.0
	k1 := 1.2
	b := 0.75

	var avgTokenCount float64
	err := e.pool.QueryRow(ctx, getAvgTokenCount).Scan(&avgTokenCount)
	if err != nil {
		return 0, err
	}
	var docTokenCount int
	err = e.pool.QueryRow(ctx, getDocumentTokenCount, docID).Scan(&docTokenCount)
	if err != nil {
		return 0, err
	}

	docLengthNorm := float64(docTokenCount) / avgTokenCount

	for _, termID := range termIDs {
		var tf int
		err := e.pool.QueryRow(ctx, getPostingOfTermLength, docID, termID).Scan(&tf)

		if err != nil || tf == 0 {
			continue
		}

		idf, err := e.getIDF(ctx, termID, totalDocs)
		if err != nil {
			return 0, err
		}

		tfComp := float64(tf) * (k1 + 1)
		denominator := float64(tf) + k1*(1-b+b*docLengthNorm)
		score += idf * (tfComp / denominator)
	}

	return score, nil
}

func (e *QueryEngine) getIDF(ctx context.Context, termID int64, totalDocs int) (float64, error) {
	if val, ok := e.idfCache.Load(termID); ok {
		return val.(float64), nil
	}

	var df int
	err := e.pool.QueryRow(ctx, getDocFrequency, termID).Scan(&df)
	if err != nil {
		return 0, err
	}

	idf := math.Log(float64(totalDocs) / float64(df+1))
	e.idfCache.Store(termID, idf)
	return idf, nil
}
