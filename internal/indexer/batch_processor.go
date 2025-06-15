package indexer

import (
	"context"
	"fmt"
	"github.com/amankumarsingh77/search_engine/models"
	"strings"
)

type BatchProcessor struct {
	adapter *Storage
}

type Batch struct {
	docs    []*models.WebPage
	termMap map[string]map[int][]int
}

func NewBatchProcessor(adapter *Storage) *BatchProcessor {
	return &BatchProcessor{
		adapter: adapter,
	}
}

func (p *BatchProcessor) ProcessBatch(ctx context.Context, batch *Batch) error {
	docIDs, err := p.adapter.InsertDocuments(ctx, batch.docs)
	if err != nil {
		return fmt.Errorf("failed to insert documents: %w", err)
	}

	terms := make([]string, 0, len(batch.termMap))
	for term := range batch.termMap {
		terms = append(terms, term)
	}

	termMap, err := p.adapter.UpsertTerms(ctx, terms)
	if err != nil {
		return fmt.Errorf("failed to upsert terms: %w", err)
	}

	if err = p.adapter.InsertPosting(ctx, termMap, docIDs, batch.termMap); err != nil {
		return fmt.Errorf("failed to insert postings: %w", err)
	}

	return nil
}

func (p *BatchProcessor) CreateBatch(docs []*models.WebPage) *Batch {
	docBatch := &Batch{
		docs:    docs,
		termMap: make(map[string]map[int][]int),
	}
	for docIdx, doc := range docs {
		tokens := normalizePageContent(doc.Title + " " + doc.Description + " " + doc.BodyText + " " + strings.Join(doc.Paragraphs, " "))
		docs[docIdx].TokenCount = len(tokens)
		for pos, token := range tokens {
			if docBatch.termMap[token] == nil {
				docBatch.termMap[token] = make(map[int][]int)
			}
			docBatch.termMap[token][docIdx] = append(docBatch.termMap[token][docIdx], pos)
		}
	}
	return docBatch
}
