package query

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

type DocumentDetail struct {
	ID          int64
	URL         string
	Title       string
	Description string
	TokenCount  int
}

func (e *QueryEngine) fetchDocumentDetailsBatch(ctx context.Context, scoredDocs []ScoredDoc, queryTerms []string) ([]SearchResult, error) {
	if len(scoredDocs) == 0 {
		return nil, nil
	}

	docIDs := make([]int64, len(scoredDocs))
	for i, sd := range scoredDocs {
		docIDs[i] = sd.DocID
	}

	docDetails, err := e.getDocumentDetailsBatch(ctx, docIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document details: %w", err)
	}

	results := make([]SearchResult, 0, len(scoredDocs))

	for _, sd := range scoredDocs {
		doc, exists := docDetails[sd.DocID]
		if !exists {
			continue
		}

		snippet := e.generateEnhancedSnippet(doc.Description, queryTerms, 150)

		results = append(results, SearchResult{
			DocID:       sd.DocID,
			URL:         doc.URL,
			Title:       e.highlightTerms(doc.Title, queryTerms),
			Description: doc.Description,
			Score:       sd.Score,
			Snippet:     snippet,
		})
	}

	return results, nil
}

func (e *QueryEngine) getDocumentDetailsBatch(ctx context.Context, docIDs []int64) (map[int64]DocumentDetail, error) {
	result := make(map[int64]DocumentDetail, len(docIDs))
	var missingDocIDs []int64

	for _, docID := range docIDs {
		cacheKey := fmt.Sprintf("doc_detail_%d", docID)
		if val, ok := e.docCache.Get(cacheKey); ok {
			if detail, ok := val.(DocumentDetail); ok {
				result[docID] = detail
				continue
			}
		}
		missingDocIDs = append(missingDocIDs, docID)
	}

	if len(missingDocIDs) > 0 {
		rows, err := e.pool.Query(ctx, getDocumentsBatch, missingDocIDs, missingDocIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch documents: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var detail DocumentDetail
			if err := rows.Scan(&detail.ID, &detail.URL, &detail.Title, &detail.Description, &detail.TokenCount); err != nil {
				continue
			}

			result[detail.ID] = detail

			cacheKey := fmt.Sprintf("doc_detail_%d", detail.ID)
			e.docCache.Put(cacheKey, detail)
		}
	}

	return result, nil
}

func (e *QueryEngine) generateEnhancedSnippet(text string, queryTerms []string, maxLength int) string {
	if len(text) == 0 {
		return ""
	}

	cleanText := html.UnescapeString(text)
	cleanText = strings.ReplaceAll(cleanText, "\n", " ")
	cleanText = strings.ReplaceAll(cleanText, "\t", " ")
	cleanText = regexp.MustCompile(`\s+`).ReplaceAllString(cleanText, " ")
	cleanText = strings.TrimSpace(cleanText)

	if len(cleanText) <= maxLength {
		return e.highlightTerms(cleanText, queryTerms)
	}

	bestPos := e.findBestSnippetPosition(cleanText, queryTerms, maxLength)

	start := bestPos
	end := bestPos + maxLength

	if start > 0 {
		wordStart := strings.LastIndex(cleanText[:start+20], " ")
		if wordStart > 0 && wordStart < start+20 {
			start = wordStart + 1
		}
	}

	if end < len(cleanText) {
		wordEnd := strings.Index(cleanText[end-20:], " ")
		if wordEnd > 0 {
			end = end - 20 + wordEnd
		}
	}

	if start < 0 {
		start = 0
	}
	if end > len(cleanText) {
		end = len(cleanText)
	}

	snippet := cleanText[start:end]

	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(cleanText) {
		snippet = snippet + "..."
	}

	return e.highlightTerms(snippet, queryTerms)
}

func (e *QueryEngine) findBestSnippetPosition(text string, queryTerms []string, windowSize int) int {
	if len(text) <= windowSize {
		return 0
	}

	bestPos := 0
	bestScore := 0
	textLower := strings.ToLower(text)

	for i := 0; i <= len(text)-windowSize; i += windowSize / 4 {
		windowEnd := i + windowSize
		if windowEnd > len(text) {
			windowEnd = len(text)
		}

		window := textLower[i:windowEnd]
		score := 0

		for _, term := range queryTerms {
			termLower := strings.ToLower(term)
			if len(termLower) > 2 {
				score += strings.Count(window, termLower) * len(termLower)
			}
		}

		if score > bestScore {
			bestScore = score
			bestPos = i
		}
	}

	return bestPos
}

func (e *QueryEngine) highlightTerms(text string, queryTerms []string) string {
	if len(queryTerms) == 0 {
		return text
	}

	result := text

	terms := make([]string, len(queryTerms))
	copy(terms, queryTerms)

	for i := 0; i < len(terms); i++ {
		for j := i + 1; j < len(terms); j++ {
			if len(terms[i]) < len(terms[j]) {
				terms[i], terms[j] = terms[j], terms[i]
			}
		}
	}

	for _, term := range terms {
		if len(term) < 2 {
			continue
		}

		pattern := `(?i)\b` + regexp.QuoteMeta(term) + `\b`
		regex, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}

		result = regex.ReplaceAllStringFunc(result, func(match string) string {

			if strings.Contains(match, "<mark>") {
				return match
			}
			return "<mark>" + match + "</mark>"
		})
	}

	return result
}

func (e *QueryEngine) rankResults(results []SearchResult, queryTerms []string) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	for i := range results {
		results[i].Score = e.calculateEnhancedScore(results[i], queryTerms)
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

func (e *QueryEngine) calculateEnhancedScore(result SearchResult, queryTerms []string) float64 {
	score := result.Score

	titleLower := strings.ToLower(result.Title)
	titleBoost := 0.0
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		if strings.Contains(titleLower, termLower) {
			titleBoost += 0.2 * float64(len(term))
		}
	}

	urlBoost := 0.0
	if len(result.URL) < 50 {
		urlBoost = 0.1
	} else if len(result.URL) > 200 {
		urlBoost = -0.1
	}

	descBoost := 0.0
	descLower := strings.ToLower(result.Description)
	for _, term := range queryTerms {
		termLower := strings.ToLower(term)
		count := strings.Count(descLower, termLower)
		descBoost += float64(count) * 0.05
	}

	return score + titleBoost + urlBoost + descBoost
}

func truncateByWords(text string, maxWords int) string {
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return text
	}

	return strings.Join(words[:maxWords], " ") + "..."
}

func (e *QueryEngine) validateResults(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	validResults := make([]SearchResult, 0, len(results))

	for _, result := range results {

		if result.URL == "" || result.Title == "" {
			continue
		}

		if !utf8.ValidString(result.Title) {
			result.Title = strings.ToValidUTF8(result.Title, "")
		}
		if !utf8.ValidString(result.Description) {
			result.Description = strings.ToValidUTF8(result.Description, "")
		}
		if !utf8.ValidString(result.Snippet) {
			result.Snippet = strings.ToValidUTF8(result.Snippet, "")
		}

		if len(result.Title) > 200 {
			result.Title = result.Title[:200] + "..."
		}
		if len(result.Description) > 500 {
			result.Description = result.Description[:500] + "..."
		}

		validResults = append(validResults, result)
	}

	return validResults
}

func (e *QueryEngine) deduplicateResults(results []SearchResult) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	seen := make(map[string]bool)
	deduped := make([]SearchResult, 0, len(results))

	for _, result := range results {

		normalizedURL := strings.ToLower(strings.TrimRight(result.URL, "/"))

		if !seen[normalizedURL] {
			seen[normalizedURL] = true
			deduped = append(deduped, result)
		}
	}

	return deduped
}
