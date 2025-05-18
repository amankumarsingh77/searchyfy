package crawler

import (
	"context"
	"github.com/amankumarsingh77/search_engine/models"
	"github.com/gocolly/colly"
	"log"
	"strings"
	"sync"
	"time"
)

type Worker struct {
	ID       string
	frontier URLFrontier
	crawler  WebCrawler
	wg       *sync.WaitGroup
	stopChan chan struct{}
	outChan  chan models.WebPage
	logger   *log.Logger
}

func NewWorker(id string, frontier URLFrontier, outChan chan models.WebPage, logger *log.Logger, webCrawler WebCrawler) *Worker {
	c := colly.NewCollector(
		colly.Async(true),
		colly.UserAgent("GoTFIDFSearchCrawler/1.0 (+http://example.com/bot)"),
		colly.IgnoreRobotsTxt(),
		colly.MaxDepth(1),
	)
	c.SetRequestTimeout(10 * time.Second)

	err := c.Limit(
		&colly.LimitRule{
			DomainGlob:  "*",
			Parallelism: 5,
			Delay:       1 * time.Second,
			RandomDelay: 1 * time.Second,
		},
	)
	if err != nil {
		log.Fatalf("Failed to set limit rule: %v", err)
	}
	return &Worker{
		ID:       id,
		frontier: frontier,
		crawler:  webCrawler,
		outChan:  outChan,
		stopChan: make(chan struct{}),
		logger:   logger,
	}
}

func (w *Worker) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	w.logger.Printf("Worker %s: Starting", w.ID)
	for {
		select {
		case <-ctx.Done():
			w.logger.Printf("Worker %s: Context cancelled, shutting down", w.ID)
			return
		case <-w.stopChan:
			w.logger.Printf("Worker %s: Stop signal received, shutting down", w.ID)
			return
		default:
			urlToCrawl, err := w.frontier.Next(ctx, w.ID)
			if err != nil {
				if strings.Contains(err.Error(), "frontier is empty") || strings.Contains(err.Error(), "timeout reached") {
					w.logger.Printf("Worker %s: Frontier is empty or timeout, sleeping for a bit.", w.ID)
					time.Sleep(5 * time.Second)
					continue
				}
				w.logger.Printf("Worker %s: Error getting next URL from frontier: %v. Shutting down.", w.ID, err)
				return
			}
			if urlToCrawl == "" {
				w.logger.Printf("Worker %s: Frontier returned empty URL, sleeping.", w.ID)
				time.Sleep(1 * time.Second)
				continue
			}
			w.logger.Printf("Worker %s: Processing URL: %s", w.ID, urlToCrawl)
			pageData, err := w.crawler.Process(urlToCrawl)
			if err != nil {
				w.logger.Printf("Worker %s: Failed to process %s: %v", w.ID, urlToCrawl, err)
				pageData.ErrorString = err.Error()
				if err = w.frontier.Fail(ctx, urlToCrawl, w.ID, err.Error()); err != nil {
					w.logger.Printf("Worker %s: CRITICAL - Failed to report crawl success for %s to frontier: %v", w.ID, urlToCrawl, err)
				}
			}
			if w.outChan != nil {
				select {
				case w.outChan <- *pageData:
				case <-ctx.Done():
					w.logger.Printf("Worker %s: Context cancelled while sending to output channel.", w.ID)
					return
				case <-time.After(2 * time.Second):
					w.logger.Printf("Worker %s: Timeout sending page data for %s to output channel.", w.ID, urlToCrawl)
				}
			}
			var allNewLinks []string
			allNewLinks = append(allNewLinks, pageData.InternalLinks...)
			if len(allNewLinks) > 0 {
				for _, link := range allNewLinks {
					err = w.frontier.Add(ctx, link)
					if err != nil {
						w.logger.Printf("Worker %s: Could not requeue the link %s : Skipping", w.ID, link)
					}
				}
				w.logger.Printf("Worker %s: Discovered %d links from %s.", w.ID, len(allNewLinks), urlToCrawl)
			}
		}

	}
}

func (w *Worker) Stop() {
	w.logger.Printf("Worker %s: Sending stop signal", w.ID)
	close(w.stopChan)
	close(w.outChan)
}
