package indexer

import (
	"context"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/models"
	"log"
	"sync"
	"time"
)

type Indexer struct {
	adapter      *Storage
	processor    *BatchProcessor
	batchSize    int
	workers      int
	documentChan chan models.WebPage
}

func NewIndexer(cfg *config.IndexerConfig, adapter *Storage, batchProcessor *BatchProcessor) *Indexer {
	indexer := &Indexer{
		adapter:      adapter,
		processor:    batchProcessor,
		batchSize:    cfg.BatchSize,
		workers:      cfg.Workers,
		documentChan: make(chan models.WebPage, cfg.BatchSize*2),
	}
	return indexer
}
func (i *Indexer) Start() {
	var wg sync.WaitGroup
	wg.Add(i.workers)

	for w := 0; w < i.workers; w++ {
		go i.worker(&wg)
	}

	wg.Wait()
}

func (i *Indexer) worker(wg *sync.WaitGroup) {
	defer wg.Done()
	var docs []*models.WebPage
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case doc, ok := <-i.documentChan:
			if !ok {
				if len(docs) > 0 {
					i.processDocuments(docs)
				}
				return
			}

			docs = append(docs, &doc)
			if len(docs) >= i.batchSize {
				i.processDocuments(docs)
				docs = nil
			}

		case <-ticker.C:
			if len(docs) > 0 {
				i.processDocuments(docs)
				docs = nil
			}
		}
	}
}

func (i *Indexer) processDocuments(docs []*models.WebPage) {
	//ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	//defer cancel()

	batch := i.processor.CreateBatch(docs)
	if err := i.processor.ProcessBatch(context.Background(), batch); err != nil {
		log.Fatalf("failed to process the batch %v", err)
	}
}

func (i *Indexer) AddDocument(doc models.WebPage) {
	i.documentChan <- doc
}

func (i *Indexer) Close() {
	close(i.documentChan)
}
