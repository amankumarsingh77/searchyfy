package query

import (
	"context"
	"fmt"
	"math"
	"sync"
)

func (e *QueryEngine) resolveTermIDsBatch(ctx context.Context, plan *QueryPlan) error {
	if len(plan.terms) == 0 {
		return nil
	}

	termMap := make(map[string]int64, len(plan.terms))
	var missingTerms []string

	for _, term := range plan.terms {
		if val, ok := e.termCache.Get(term); ok {
			if id, ok := val.(int64); ok {
				termMap[term] = id
				continue
			}
		}
		missingTerms = append(missingTerms, term)
	}

	if len(missingTerms) > 0 {
		rows, err := e.pool.Query(ctx, getTermsBatch, missingTerms)
		if err != nil {
			return fmt.Errorf("failed to fetch terms: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var term string
			var id int64
			if err := rows.Scan(&term, &id); err != nil {
				continue
			}
			termMap[term] = id
			e.termCache.Put(term, id)
		}
	}

	plan.termIDs = make([]int64, 0, len(plan.terms))
	for _, term := range plan.terms {
		if id, ok := termMap[term]; ok {
			plan.termIDs = append(plan.termIDs, id)
		}
	}

	return nil
}

func (e *QueryEngine) getPostingsBatch(ctx context.Context, termIDs []int64) (map[int64][]Posting, error) {
	result := make(map[int64][]Posting, len(termIDs))
	var missingTermIDs []int64

	for _, termID := range termIDs {
		if val, ok := e.postingCache.Get(termID); ok {
			if postings, ok := val.([]Posting); ok {
				result[termID] = postings
				continue
			}
		}
		missingTermIDs = append(missingTermIDs, termID)
	}

	if len(missingTermIDs) > 0 {
		rows, err := e.pool.Query(ctx, getPostingsByTermIDBatch, missingTermIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch postings: %w", err)
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
			result[termID] = postings
			e.postingCache.Put(termID, postings)
		}
	}

	return result, nil
}

func (e *QueryEngine) booleanSearchOptimized(ctx context.Context, plan *QueryPlan) ([]int64, error) {
	if len(plan.termIDs) == 0 {
		return nil, nil
	}

	var siteFilteredDocs map[int64]struct{}
	if site, ok := plan.filters["site"]; ok {
		rows, err := e.pool.Query(ctx, getSiteFilteredDocs, site)
		if err != nil {
			return nil, fmt.Errorf("site filter failed: %w", err)
		}
		defer rows.Close()

		siteFilteredDocs = make(map[int64]struct{})
		for rows.Next() {
			var docID int64
			if err := rows.Scan(&docID); err != nil {
				continue
			}
			siteFilteredDocs[docID] = struct{}{}
		}

		if len(siteFilteredDocs) == 0 {
			return nil, nil
		}
	}

	var docIDs []int64
	var err error

	if plan.operator == "AND" {
		docIDs, err = e.performIntersectionSearch(ctx, plan.termIDs, siteFilteredDocs)
	} else {
		docIDs, err = e.performUnionSearch(ctx, plan.termIDs, siteFilteredDocs)
	}

	if err != nil {
		return nil, fmt.Errorf("boolean search failed: %w", err)
	}

	return docIDs, nil
}

func (e *QueryEngine) performIntersectionSearch(ctx context.Context, termIDs []int64, siteFilter map[int64]struct{}) ([]int64, error) {
	rows, err := e.pool.Query(ctx, getBooleanIntersection, termIDs, len(termIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docIDs := GetDocIDSlice()
	defer PutDocIDSlice(docIDs)

	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			continue
		}

		if siteFilter != nil {
			if _, ok := siteFilter[docID]; !ok {
				continue
			}
		}

		docIDs = append(docIDs, docID)
	}

	result := make([]int64, len(docIDs))
	copy(result, docIDs)
	return result, nil
}

func (e *QueryEngine) performUnionSearch(ctx context.Context, termIDs []int64, siteFilter map[int64]struct{}) ([]int64, error) {
	rows, err := e.pool.Query(ctx, getBooleanUnion, termIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docIDs := GetDocIDSlice()
	defer PutDocIDSlice(docIDs)

	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			continue
		}

		if siteFilter != nil {
			if _, ok := siteFilter[docID]; !ok {
				continue
			}
		}

		docIDs = append(docIDs, docID)
	}

	result := make([]int64, len(docIDs))
	copy(result, docIDs)
	return result, nil
}

func (e *QueryEngine) phraseSearchOptimized(ctx context.Context, plan *QueryPlan) ([]int64, error) {
	if len(plan.termIDs) < 2 {
		return e.booleanSearchOptimized(ctx, plan)
	}

	postingsByTerm, err := e.getPostingsBatch(ctx, plan.termIDs)
	if err != nil {
		return nil, err
	}

	for _, termID := range plan.termIDs {
		if len(postingsByTerm[termID]) == 0 {
			return nil, nil
		}
	}

	commonDocs := e.findCommonDocuments(postingsByTerm, plan.termIDs)
	if len(commonDocs) == 0 {
		return nil, nil
	}

	docIDs := GetDocIDSlice()
	defer PutDocIDSlice(docIDs)

	var mu sync.Mutex
	var wg sync.WaitGroup

	docChan := make(chan int64, len(commonDocs))

	numWorkers := e.maxWorkers
	if numWorkers > len(commonDocs) {
		numWorkers = len(commonDocs)
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for docID := range docChan {
				if e.checkPhraseMatch(docID, plan.termIDs, postingsByTerm) {
					mu.Lock()
					docIDs = append(docIDs, docID)
					mu.Unlock()
				}
			}
		}()
	}

	go func() {
		for docID := range commonDocs {
			docChan <- docID
		}
		close(docChan)
	}()

	wg.Wait()

	result := make([]int64, len(docIDs))
	copy(result, docIDs)
	return result, nil
}

func (e *QueryEngine) findCommonDocuments(postingsByTerm map[int64][]Posting, termIDs []int64) map[int64]struct{} {
	if len(termIDs) == 0 {
		return nil
	}

	commonDocs := make(map[int64]struct{})
	for _, posting := range postingsByTerm[termIDs[0]] {
		commonDocs[posting.DocID] = struct{}{}
	}

	for i := 1; i < len(termIDs); i++ {
		termDocs := make(map[int64]struct{})
		for _, posting := range postingsByTerm[termIDs[i]] {
			termDocs[posting.DocID] = struct{}{}
		}

		newCommonDocs := make(map[int64]struct{})
		for docID := range commonDocs {
			if _, exists := termDocs[docID]; exists {
				newCommonDocs[docID] = struct{}{}
			}
		}
		commonDocs = newCommonDocs

		if len(commonDocs) == 0 {
			break
		}
	}

	return commonDocs
}

func (e *QueryEngine) checkPhraseMatch(docID int64, termIDs []int64, postingsByTerm map[int64][]Posting) bool {

	termPositions := make([][]int32, len(termIDs))

	for i, termID := range termIDs {
		for _, posting := range postingsByTerm[termID] {
			if posting.DocID == docID {
				termPositions[i] = posting.Positions
				break
			}
		}

		if len(termPositions[i]) == 0 {
			return false
		}
	}

	return e.hasConsecutivePositions(termPositions)
}

func (e *QueryEngine) hasConsecutivePositions(termPositions [][]int32) bool {
	if len(termPositions) == 0 {
		return false
	}

	for _, firstPos := range termPositions[0] {
		if e.checkConsecutiveFromPosition(firstPos, termPositions) {
			return true
		}
	}

	return false
}

func (e *QueryEngine) checkConsecutiveFromPosition(startPos int32, termPositions [][]int32) bool {

	for i := 1; i < len(termPositions); i++ {
		expectedPos := startPos + int32(i)
		found := false

		for _, pos := range termPositions[i] {
			if pos == expectedPos {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func (e *QueryEngine) binarySearchPosition(positions []int32, target int32) bool {
	left, right := 0, len(positions)-1

	for left <= right {
		mid := left + (right-left)/2

		if positions[mid] == target {
			return true
		} else if positions[mid] < target {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}

	return false
}

func (e *QueryEngine) checkConsecutiveFromPositionOptimized(startPos int32, termPositions [][]int32) bool {
	for i := 1; i < len(termPositions); i++ {
		expectedPos := startPos + int32(i)
		if !e.binarySearchPosition(termPositions[i], expectedPos) {
			return false
		}
	}
	return true
}

func (e *QueryEngine) getIDFBatch(ctx context.Context, termIDs []int64) (map[int64]float64, error) {
	result := make(map[int64]float64, len(termIDs))
	var missingTermIDs []int64
	for _, termID := range termIDs {
		if val, ok := e.idfCache.Get(termID); ok {
			if idf, ok := val.(float64); ok {
				result[termID] = idf
				continue
			}
		}
		missingTermIDs = append(missingTermIDs, termID)
	}
	if len(missingTermIDs) > 0 {
		totalDocs := float64(e.getTotalDocs())

		rows, err := e.pool.Query(ctx, getDocFrequencyBatch, missingTermIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch document frequencies: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var termID int64
			var docFreq int64

			if err := rows.Scan(&termID, &docFreq); err != nil {
				continue
			}
			idf := math.Log(totalDocs / float64(docFreq+1))
			result[termID] = idf
			e.idfCache.Put(termID, idf)
		}
	}

	return result, nil
}
