package crawler

import (
	"context"
	"fmt"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/internal/common/database"
	"github.com/amankumarsingh77/search_engine/models"
	"github.com/amankumarsingh77/search_engine/pkg"
	"github.com/redis/go-redis/v9"
	"log"
	"os"
	"sync"
)

type Spider struct {
	cfg         *config.CrawlerConfig
	httpClient  *HttpClient
	frontier    URLFrontier
	db          *database.MongoClient
	redisClient *redis.Client
	bfClient    *BloomFilter
	cleanUp     func()
	log         *log.Logger
	wg          sync.WaitGroup
}

func NewWebCrawler(ctx context.Context, cfg *config.CrawlerConfig) (*Spider, error) {
	mongoClient, err := database.NewMongoClient(ctx, &cfg.Mongo)
	if err != nil {
		return nil, fmt.Errorf("failed to load mongo client : %v", err)
	}
	redisClient, err := NewRedisClient(ctx, &cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to load redis client : %v", err)
	}
	bfClient, err := NewRedisBloomFilter(&cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to load bloom filter client : %v", err)
	}
	frontier := NewURLFrontier(redisClient, bfClient)
	cleanup := func() {
		fmt.Println("Cleaning up frontier and redis resources")
		redisClient.Close()
		frontier.Close()
	}
	httpClient := NewHttpClient(cfg)
	return &Spider{
		cfg:         cfg,
		httpClient:  httpClient,
		frontier:    frontier,
		db:          mongoClient,
		redisClient: redisClient,
		bfClient:    bfClient,
		log:         log.New(os.Stdout, "[Spider]: ", log.LstdFlags|log.Lshortfile),
		cleanUp:     cleanup,
	}, nil
}

func (c *Spider) RunCrawler(ctx context.Context) {
	crawlCtx, cancel := context.WithCancel(ctx)
	defer c.cleanUp()
	defer cancel()
	log.Println("Starting crawler...")
	webProcessor := NewHttpCrawler(c.httpClient, c.frontier, c.db)
	pageChan := make(chan models.WebPage, 10000)
	workers := make([]*Worker, c.cfg.Workers)

	for i := 0; i < c.cfg.Workers; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		logger := log.New(os.Stdout, fmt.Sprintf("[%s]", workerID), log.LstdFlags|log.Lshortfile)
		workers[i] = NewWorker(workerID, c.frontier, pageChan, logger, webProcessor, c.db, c.cfg.MaxDepth)
		c.wg.Add(1)
		go workers[i].Start(crawlCtx)
	}
	log.Printf("Started %d workers. Crawling in progress", c.cfg.Workers)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(pageChan)
		close(done)
	}()

	select {
	case <-crawlCtx.Done():
		log.Println("Crawling cancelled")
	case <-done:
		log.Println("All workers finished")
	}
	for _, worker := range workers {
		worker.Stop()
	}
}

func (c *Spider) SeedUrls(filename string) {
	seedUrls, err := pkg.LoadSeedURLs(filename)
	if err != nil {
		c.log.Fatal(err)
	}
	ctx := context.Background()
	c.log.Println("Seeding urls to the queue")
	for _, url := range seedUrls {
		if err = c.frontier.Seed(ctx, url, 0); err != nil {
			c.log.Printf("skipping url %s : error : %v", url, err)
		}
	}
}
