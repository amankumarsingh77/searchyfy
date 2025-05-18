package crawler

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
)

type URLFrontier interface {
	Add(ctx context.Context, url string) error
	Next(ctx context.Context, workerID string) (string, error)
	Done(ctx context.Context, url string, workerID string) error
	Fail(ctx context.Context, url string, workerID string, reason string) error
	Size(ctx context.Context) (int64, error)
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

func NewURLFrontier(redisClient *redis.Client, redisBloomClient *BloomFilter) URLFrontier {
	return &urlFrontier{
		redisClient:      redisClient,
		redisBloomClient: redisBloomClient,
	}
}

func (f *urlFrontier) Add(ctx context.Context, url string) error {
	// 1) normalize the url
	normalized_url, err := normalizeUrl(url)
	if err != nil {
		return fmt.Errorf("failed to normalize the url : %s err: %w ", url, err)
	}
	// 2) Add the url to  if not visited.
	exists, err := f.redisBloomClient.Exists(normalized_url)
	if err != nil {
		return fmt.Errorf("failed to check the bloom filter : %w", err)
	}
	if exists {
		log.Printf("%s url already exists : skipping ", normalized_url)
	} else {
		err = f.redisBloomClient.Add(url)
		if err != nil {
			return fmt.Errorf("failed to add the url to bloom filter : %w", err)
		}
		f.redisClient.LPush(ctx, pendingQueue, normalized_url)
	}
	return nil
}

func (f *urlFrontier) Next(ctx context.Context, workerID string) (string, error) {
	return f.redisClient.RPopLPush(ctx, pendingQueue, processingQueue+workerID).Result()
}

func (f *urlFrontier) Done(ctx context.Context, url string, workerID string) error {
	return f.redisClient.LRem(ctx, processingQueue+workerID, 0, url).Err()
}

func (f *urlFrontier) Fail(ctx context.Context, url, workerID string, reason string) error {
	type failedItem struct {
		Url    string `json:"url"`
		Reason string `json:"reason"`
	}
	pipe := f.redisClient.TxPipeline()
	pipe.LRem(ctx, processingQueue+workerID, 0, url)
	// can requeue in pending ,but it might create an infinite loop if retries are not handled so will just push it to different queue
	pipe.LPush(ctx, failedQueue, failedItem{url, reason})
	//pipe.LPush(f.ctx, pending_queue, url)
	_, err := pipe.Exec(ctx)
	return err
}

func (f *urlFrontier) Size(ctx context.Context) (int64, error) {
	return f.redisClient.LLen(ctx, pendingQueue).Result()
}

func (f *urlFrontier) Close() error {
	if err := f.redisClient.Close(); err != nil {
		return fmt.Errorf("failed to cloe redis client : %w", err)
	}
	return nil
}
