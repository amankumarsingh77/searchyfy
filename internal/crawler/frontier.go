package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/redis/go-redis/v9"
	"log"
)

func NewRedisClient(ctx context.Context, cfg *config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Host,
		Password: "",
		DB:       0,
	})
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("error pinging the redis : %w", err)
	}
	return client, nil
}

type URLFrontier interface {
	Visit(ctx context.Context, url string) error
	NextBatch(ctx context.Context, workerID string, count int) ([]*crawlItem, error)
	Done(ctx context.Context, item *crawlItem, workerID string) error
	Fail(ctx context.Context, crawlData *crawlItem, workerID, reason string) error
	Size(ctx context.Context) (int64, error)
	UpdateLastIndexedItem(ctx context.Context, id string) error
	GetLastIndexedItem(ctx context.Context) (string, error)
	Seed(ctx context.Context, url string, depth int64) error
	Close() error
}

const (
	pendingQueue    = "pending"
	failedQueue     = "failed"
	processingQueue = "processing:"
)

type urlFrontier struct {
	redisClient      *redis.Client
	redisBloomClient *BloomFilter
}

type crawlItem struct {
	Url   string `json:"url"`
	Depth int64  `json:"depth"`
}

func NewURLFrontier(redisClient *redis.Client, redisBloomClient *BloomFilter) URLFrontier {
	return &urlFrontier{
		redisClient:      redisClient,
		redisBloomClient: redisBloomClient,
	}
}

func (f *urlFrontier) Seed(ctx context.Context, url string, depth int64) error {
	normalizedUrl, err := normalizeUrl(url)
	if err != nil {
		return err
	}
	exists, err := f.redisBloomClient.Exists(normalizedUrl)
	if err != nil {
		return fmt.Errorf("failed to check the bloom filter: %w", err)
	}
	if exists {
		//log.Printf("%s url already exists: skipping", normalizedUrl)
		return nil
	}
	item := crawlItem{
		normalizedUrl,
		depth,
	}
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal crawl item: %w", err)
	}
	if err = f.redisBloomClient.Add(normalizedUrl); err != nil {
		return fmt.Errorf("failed to add url to bloom filter: %w", err)
	}

	if err = f.redisClient.LPush(ctx, pendingQueue, data).Err(); err != nil {
		return fmt.Errorf("failed to push seed URL to pending queue: %w", err)
	}

	return nil
}

func (f *urlFrontier) UpdateLastIndexedItem(ctx context.Context, id string) error {
	return f.redisClient.Set(ctx, "last_indexed_object_id", id, 0).Err()
}

func (f *urlFrontier) GetLastIndexedItem(ctx context.Context) (string, error) {
	val, err := f.redisClient.Get(ctx, "last_indexed_object_id").Result()
	if err != nil {
		return "", err
	}
	return val, nil
}

func (f *urlFrontier) Visit(ctx context.Context, url string) error {
	normalizedUrl, err := normalizeUrl(url)
	if err != nil {
		return fmt.Errorf("failed to normalize url: %w", err)
	}

	exists, err := f.redisBloomClient.Exists(normalizedUrl)
	if err != nil {
		return fmt.Errorf("failed to check bloom filter: %w", err)
	}
	if exists {
		//log.Printf("%s url already exists: skipping", normalizedUrl)
		return nil
	}

	if err = f.redisBloomClient.Add(normalizedUrl); err != nil {
		return fmt.Errorf("failed to add url to bloom filter: %w", err)
	}

	return nil
}

func (f *urlFrontier) NextBatch(ctx context.Context, workerID string, count int) ([]*crawlItem, error) {
	if count <= 0 {
		return nil, errors.New("invalid batch size")
	}
	processingKey := processingQueue + workerID
	processingItems, err := f.redisClient.LRange(ctx, processingKey, 0, int64(count-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read processing queue: %w", err)
	}
	var crawlItems []*crawlItem

	if len(processingItems) > 0 {
		log.Printf("found %d urls in processing", len(processingItems))
		for _, itemStr := range processingItems {
			var item crawlItem
			if err := json.Unmarshal([]byte(itemStr), &item); err != nil {
				fmt.Printf("failed to unmarshal item from processing queue: %s : skipping\n", itemStr)
				continue
			}
			crawlItems = append(crawlItems, &item)
		}
		return crawlItems, nil
	}
	resp, err := f.redisClient.LRange(ctx, pendingQueue, int64(-count), -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read pending queue: %w", err)
	}

	for _, itemStr := range resp {
		var item crawlItem
		if err := json.Unmarshal([]byte(itemStr), &item); err != nil {
			fmt.Printf("failed to unmarshal item %s : skipping\n", itemStr)
			continue
		}
		crawlItems = append(crawlItems, &item)
	}
	if len(crawlItems) == 0 {
		return nil, errors.New("frontier is empty")
	}

	pipe := f.redisClient.TxPipeline()
	pipe.LTrim(ctx, pendingQueue, 0, int64(-(count + 1)))
	for _, item := range crawlItems {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal crawl item: %w", err)
		}
		pipe.LPush(ctx, processingQueue+workerID, data)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("redis transaction failed: %w", err)
	}

	return crawlItems, nil
}

func (f *urlFrontier) Done(ctx context.Context, item *crawlItem, workerID string) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal the data : %v", err)
	}
	err = f.redisBloomClient.Add(item.Url)
	if err != nil {
		log.Printf("failed to add url to bloom filter : %v", err)
	}
	return f.redisClient.LRem(ctx, processingQueue+workerID, 0, data).Err()
}

func (f *urlFrontier) Fail(ctx context.Context, crawlData *crawlItem, workerID, reason string) error {
	item := struct {
		Item   crawlItem `json:"item"`
		Reason string    `json:"reason"`
	}{Item: *crawlData, Reason: reason}

	jsonData, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal failed item: %w", err)
	}

	dataToRemove, err := json.Marshal(crawlData)
	if err != nil {
		return fmt.Errorf("failed to marshal crawlData for removal: %w", err)
	}

	pipe := f.redisClient.TxPipeline()
	pipe.LRem(ctx, processingQueue+workerID, 0, dataToRemove)
	pipe.LPush(ctx, failedQueue, jsonData)
	_, err = pipe.Exec(ctx)

	return err
}

func (f *urlFrontier) Size(ctx context.Context) (int64, error) {
	return f.redisClient.LLen(ctx, pendingQueue).Result()
}

func (f *urlFrontier) Close() error {
	if err := f.redisClient.Close(); err != nil {
		return fmt.Errorf("failed to close redis client: %w", err)
	}
	return nil
}
