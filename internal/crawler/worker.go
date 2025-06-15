package crawler

import (
	"context"
	"github.com/amankumarsingh77/search_engine/internal/common/database"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/amankumarsingh77/search_engine/models"
)

type Worker struct {
	ID       string
	frontier URLFrontier
	crawler  WebCrawler
	wg       *sync.WaitGroup
	stopChan chan struct{}
	outChan  chan models.WebPage
	maxDepth int64
	db       *database.MongoClient
	logger   *log.Logger
}

const batchSize = 50

func NewWorker(id string, frontier URLFrontier, outChan chan models.WebPage, logger *log.Logger, webCrawler WebCrawler, db *database.MongoClient, maxDepth int64) *Worker {
	var wg *sync.WaitGroup
	return &Worker{
		ID:       id,
		frontier: frontier,
		crawler:  webCrawler,
		outChan:  outChan,
		stopChan: make(chan struct{}),
		logger:   logger,
		wg:       wg,
		db:       db,
		maxDepth: maxDepth,
	}
}

func (w *Worker) Start(ctx context.Context) {
	defer w.wg.Done()
	w.logger.Printf("Worker %s: Starting", w.ID)

	const maxConcurrentCrawls = 5
	sem := make(chan struct{}, maxConcurrentCrawls)

	for {
		select {
		case <-ctx.Done():
			w.logger.Printf("Worker %s: Context cancelled, shutting down", w.ID)
			return
		case <-w.stopChan:
			w.logger.Printf("Worker %s: Stop signal received, shutting down", w.ID)
			return
		default:
			batchItems, err := w.frontier.NextBatch(ctx, w.ID, batchSize)
			if err != nil {
				if strings.Contains(err.Error(), "frontier is empty") || strings.Contains(err.Error(), "timeout reached") {
					w.logger.Printf("Worker %s: Frontier empty or timeout, sleeping", w.ID)
					select {
					case <-ctx.Done():
						return
					case <-w.stopChan:
						return
					case <-time.After(5 * time.Second):
					}
					continue
				}
				w.logger.Printf("Worker %s: Error getting next URLs: %v. Shutting down.", w.ID, err)
				return
			}

			var batchWg sync.WaitGroup
			var pagesData []*models.WebPage
			for _, item := range batchItems {
				urlToCrawl := item.Url
				if urlToCrawl == "" {
					w.logger.Printf("Worker %s: Frontier returned empty URL, skipping", w.ID)
					continue
				}

				sem <- struct{}{}
				batchWg.Add(1)

				go func(url string) {
					defer batchWg.Done()
					defer func() { <-sem }()

					w.logger.Printf("Worker %s: Processing URL: %s", w.ID, url)
					pageData, err := w.crawler.CrawlPage(url)
					if err != nil {
						w.logger.Printf("Worker %s: Failed to process %s: %v", w.ID, url, err)
						if pageData == nil {
							pageData = &models.WebPage{
								URL:         url,
								ErrorString: err.Error(),
							}
						} else {
							pageData.ErrorString = err.Error()
						}
						if err = w.frontier.Fail(ctx, item, w.ID, err.Error()); err != nil {
							w.logger.Printf("Worker %s: CRITICAL - Failed to report crawl failure for %s: %v", w.ID, url, err)
						}
					} else {
						if err = w.frontier.Done(ctx, item, w.ID); err != nil {
							w.logger.Printf("Worker %s: CRITICAL - Failed to report crawl success for %s: %v", w.ID, url, err)
						}
						//for _, link := range pageData.InternalLinks {
						//	select {
						//	case <-ctx.Done():
						//		w.logger.Printf("Worker %s: Context cancelled while adding links", w.ID)
						//		return
						//	case <-w.stopChan:
						//		w.logger.Printf("Worker %s: Stop signal received while adding links", w.ID)
						//		return
						//	default:
						//		if item.Depth == w.maxDepth {
						//			w.logger.Println("Max depth reached : skipping")
						//			return
						//		}
						//		if err = w.frontier.Seed(ctx, link, item.Depth+1); err != nil {
						//			w.logger.Printf("Worker %s: Could not add link %s to frontier: %v", w.ID, link, err)
						//		}
						//	}
						//}
						pagesData = append(pagesData, pageData)
						w.logger.Printf("Worker %s: Added %d links from %s to frontier", w.ID, len(pageData.InternalLinks), url)
					}

					//if w.outChan != nil {
					//	select {
					//	case w.outChan <- *pageData:
					//	case <-ctx.Done():
					//		w.logger.Printf("Worker %s: Context cancelled while sending to output channel", w.ID)
					//		return
					//	case <-w.stopChan:
					//		w.logger.Printf("Worker %s: Stop signal received while sending to output channel", w.ID)
					//		return
					//	case <-time.After(2 * time.Second):
					//		w.logger.Printf("Worker %s: Timeout sending page data for %s to output channel", w.ID, url)
					//	}
					//}

					delay := time.Duration(500+rand.Intn(500)) * time.Millisecond
					w.logger.Printf("Worker %s: Sleeping for %v before next request", w.ID, delay)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
					case <-w.stopChan:
					}

				}(urlToCrawl)
			}
			batchWg.Wait()
			if err = w.db.AddBatchWebPage(pagesData); err != nil {
				w.logger.Printf("failed to add batch pages to db : %v", err)
			}
		}
	}
}

func (w *Worker) Stop() {
	w.logger.Printf("Worker %s: Sending stop signal", w.ID)
	select {
	case <-w.stopChan:
	default:
		close(w.stopChan)
	}
}
