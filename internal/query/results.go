package query

import (
	"context"
	"strings"
)

func (e *QueryEngine) fetchDocumentDetails(ctx context.Context, scoredDocs []ScoredDOC) ([]SearchResult, error) {
	if len(scoredDocs) == 0 {
		return nil, nil
	}

	// Prepare document IDs
	docIDs := make([]int64, len(scoredDocs))
	for i, sd := range scoredDocs {
		docIDs[i] = sd.DocID
	}

	// Fetch documents
	rows, err := e.pool.Query(ctx, getMultipleDocs, docIDs, docIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Map documents by ID
	docMap := make(map[int64]struct {
		URL         string
		Title       string
		Description string
	})
	for rows.Next() {
		var id int64
		var url, title, description string
		if err := rows.Scan(&id, &url, &title, &description); err != nil {
			return nil, err
		}
		docMap[id] = struct {
			URL         string
			Title       string
			Description string
		}{URL: url, Title: title, Description: description}
	}

	// Prepare results with snippets - ONLY FOR DOCUMENTS THAT EXIST
	results := make([]SearchResult, 0, len(scoredDocs))
	for _, sd := range scoredDocs {
		doc, exists := docMap[sd.DocID]
		if !exists {
			// Skip documents that weren't found in the database
			continue
		}

		results = append(results, SearchResult{
			DocID:   sd.DocID,
			URL:     doc.URL,
			Title:   doc.Title,
			Score:   sd.Score,
			Snippet: generateSnippet(doc.Description, 150),
		})
	}

	return results, nil
}

func generateSnippet(content string, length int) string {
	if len(content) <= length {
		return content
	}

	pos := strings.Index(content[length:], " ")
	if pos == -1 {
		return content[:length] + "..."
	}

	return content[:length+pos] + "..."
}
