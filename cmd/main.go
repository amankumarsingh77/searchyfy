package main

import (
	"context"
	"flag"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/internal/common/database"
	"github.com/amankumarsingh77/search_engine/internal/crawler"
	"github.com/amankumarsingh77/search_engine/internal/indexer"
	"github.com/amankumarsingh77/search_engine/internal/query"
	"github.com/amankumarsingh77/search_engine/pkg/search"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var (
		configFile = flag.String("config", "crawler.yaml", "Path to configuration file")
		mode       = flag.String("mode", "crawl", "Mode: crawl, tfidf, search, indexer or seed")
		workers    = flag.Int("workers", 3, "Number of worker goroutines")
		seedFile   = flag.String("seedfile", "seed_urls.csv", "Path to seed URLs file")
	)
	flag.Parse()

	cfg, err := config.LoadCrawlerConfig(*configFile)
	if err != nil {
		log.Printf("Failed to load configuration from %s: %v", *configFile, err)
		log.Println("Using default configuration...")
		cfg = config.GetDefaultConfig()
	}

	if *workers > 0 {
		cfg.Workers = *workers
	} else {
		log.Fatalf("Number of workers must be a natural number")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal, gracefully shutting down...")
		cancel()
	}()

	switch *mode {
	case "crawl":
		webCrawler, err := crawler.NewWebCrawler(ctx, cfg)
		if err != nil {
			log.Fatalf("Failed to initialize the crawler: %v", err)
		}
		webCrawler.RunCrawler(ctx)

	case "seed":
		webCrawler, err := crawler.NewWebCrawler(ctx, cfg)
		if err != nil {
			log.Fatalf("Failed to initialize the crawler: %v", err)
		}
		webCrawler.SeedUrls(*seedFile)

	case "indexer":
		adapter, err := indexer.NewPostgresClient(&cfg.Index)
		if err != nil {
			log.Fatal(err)
		}
		defer adapter.Close()

		batchProcessor := indexer.NewBatchProcessor(adapter)
		idx := indexer.NewIndexer(&cfg.Index, adapter, batchProcessor)
		defer idx.Close()

		mongoClient, err := database.NewMongoClient(ctx, &cfg.Mongo)
		if err != nil {
			log.Fatal(err)
		}
		redisClient, err := crawler.NewRedisClient(ctx, &cfg.Redis)
		if err != nil {
			log.Fatal(err)
		}
		var lastID *primitive.ObjectID
		ID, err := redisClient.Get(ctx, "last_indexed_object_id").Result()
		if err == nil && ID != "" {
			oid, err := primitive.ObjectIDFromHex(ID)
			if err == nil {
				lastID = &oid
				log.Printf("Received last process id : %s", lastID.Hex())
			}
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered in indexer goroutine: %v", r)
					//cancel()
				}
				if lastID != nil {
					log.Println("Saving last indexed ID")
					err = redisClient.Set(ctx, "last_indexed_object_id", lastID.Hex(), 0).Err()
					if err != nil {
						log.Printf("Failed to save lastID to Redis: %v", err)
					}
				}
			}()
			idx.Start()
		}()
		for {
			select {
			case <-ctx.Done():
				log.Println("Context cancelled, stopping indexer ....")
				if lastID != nil {
					log.Println("Saving last indexed ID")
					redisCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					err := redisClient.Set(redisCtx, "last_indexed_object_id", lastID.Hex(), 0).Err()
					if err != nil {
						log.Printf("Failed to save lastID to Redis: %v", err)
					} else {
						log.Println("Successfully saved lastID to Redis.")
					}
				}
				return
			default:
				docs, lastProcessedID, err := mongoClient.GetBatchWebPage(cfg.Index.BatchSize, lastID, true)
				if err != nil {
					log.Printf("Failed to get batch: %v", err)
					time.Sleep(2 * time.Second)
					continue
				}
				if len(docs) == 0 {
					log.Println("No more documents to process.")
					//time.Sleep(5 * time.Second)
					//continue
					return
				}
				for _, doc := range docs {
					idx.AddDocument(doc)
				}
				lastID = lastProcessedID
			}

		}

	case "tfidf":
		log.Println("TF-IDF mode selected (not yet implemented).")

	case "search":
		if cfg.Index.DBURL == "" {
			log.Fatal("DBURL is empty in config")
		}

		pgConfig, err := pgxpool.ParseConfig(cfg.Index.DBURL)
		if err != nil {
			log.Fatalf("failed to parse PostgreSQL config: %w", err)
		}

		pgConfig.MaxConns = int32(cfg.Workers * 2)
		pgConfig.MinConns = 1

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		dbPool, err := pgxpool.NewWithConfig(ctx, pgConfig)
		if err != nil {
			log.Fatalf("failed to create PostgreSQL connection pool: %w", err)
		}
		defer dbPool.Close()
		log.Println("Search mode selected (not yet implemented).")
		searchAPI := search.NewSearchAPI(dbPool, &cfg.Query)
		queryEngine := query.NewQueryEngine(dbPool, &cfg.Query)

		// Initialize Fiber app
		app := fiber.New(fiber.Config{
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		})

		// Register routes
		searchAPI.RegisterRoutes(app) // Make sure this is adapted for *fiber.App

		// Warm up cache
		if cfg.Search.WarmCache {
			log.Println("Warming up query cache...")
			if err := queryEngine.WarmCache(context.Background(), 1000); err != nil {
				log.Printf("Cache warm-up failed: %v", err)
			}
		}

		// Run server in goroutine
		go func() {
			log.Printf("Starting search API on %s", cfg.Search.HTTPAddr)
			if err := app.Listen(cfg.Search.HTTPAddr); err != nil {
				log.Fatalf("Fiber app failed: %v", err)
			}
		}()

		// Graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down server...")

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := app.Shutdown(); err != nil {
			log.Printf("Fiber shutdown failed: %v", err)
		}

		log.Println("Server exited properly")
	default:
		log.Fatalf("Unknown mode: %s. Use crawl, tfidf, search, indexer, or seed.", *mode)
	}
}
