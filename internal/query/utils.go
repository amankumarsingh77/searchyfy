package query

import (
	"context"
	"fmt"
)

func (e *QueryEngine) resolveTermIDs(ctx context.Context, plan *QueryPlan) error {
	termMap := make(map[string]int64)
	var missingTerms []string

	// Check cache first
	for _, term := range plan.terms {
		if val, ok := e.termCache.Get(term); ok {
			if id, ok := val.(int64); ok {
				termMap[term] = id
			} else {
				missingTerms = append(missingTerms, term)
			}
		} else {
			missingTerms = append(missingTerms, term)
		}
	}

	if len(missingTerms) > 0 {
		rows, err := e.pool.Query(ctx, getTerms, missingTerms)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var term string
			var id int64
			if err := rows.Scan(&term, &id); err != nil {
				return err
			}
			termMap[term] = id
			e.termCache.Put(term, id)
		}
	}

	for _, term := range plan.terms {
		if id, ok := termMap[term]; ok {
			plan.termIDs = append(plan.termIDs, id)
		}
	}

	return nil
}

func (e *QueryEngine) getPostings(ctx context.Context, termID int64) []Posting {
	if val, ok := e.postingCache.Get(termID); ok {
		// Type assert to []Posting
		if postings, ok := val.([]Posting); ok {
			return postings
		}
	}

	rows, err := e.pool.Query(ctx, getPostingsByTermID, termID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var postings []Posting
	for rows.Next() {
		var docID int64
		var positions []int32
		if err := rows.Scan(&docID, &positions); err != nil {
			continue
		}
		postings = append(postings, Posting{DocID: docID, Positions: positions})
	}

	// Update cache
	e.postingCache.Put(termID, postings)
	return postings
}

func (e *QueryEngine) booleanSearch(ctx context.Context, plan *QueryPlan) ([]int64, error) {
	args := []interface{}{plan.termIDs}

	query := getDocIDsByTermID
	if plan.operator == "AND" {
		query += ` HAVING COUNT(DISTINCT term_id) = $2`
		args = append(args, len(plan.termIDs))
	}

	if site, ok := plan.filters["site"]; ok {
		query = getDocByFilters
		args = append(args, site)

		if plan.operator == "AND" {
			query += ` HAVING COUNT(DISTINCT p.term_id) = $3`
			args = append(args, len(plan.termIDs))
		}
	}

	rows, err := e.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docIDs []int64
	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			return nil, err
		}
		docIDs = append(docIDs, docID)
	}

	return docIDs, nil
}

func (e *QueryEngine) phraseSearch(ctx context.Context, plan *QueryPlan) ([]int64, error) {
	if len(plan.termIDs) < 2 {
		return e.booleanSearch(ctx, plan)
	}

	query := getDocByPhrase

	for i := 2; i < len(plan.termIDs); i++ {
		query += fmt.Sprintf(`
			JOIN postings p%d ON p1.doc_id = p%d.doc_id
			AND p%d.term_id = $%d
			AND EXISTS (
				SELECT 1
				FROM unnest(p%d.positions) pos%d
				JOIN unnest(p%d.positions) pos%d ON pos%d = pos%d + %d
			)
		`, i+1, i+1, i+1, i+1, i, i, i+1, i+1, i+1, i, i)
	}

	args := make([]interface{}, len(plan.termIDs))
	for i, id := range plan.termIDs {
		args[i] = id
	}

	rows, err := e.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docIDs []int64
	for rows.Next() {
		var docID int64
		if err := rows.Scan(&docID); err != nil {
			return nil, err
		}
		docIDs = append(docIDs, docID)
	}

	return docIDs, nil
}
