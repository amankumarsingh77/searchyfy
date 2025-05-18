package crawler

import (
	"context"
	"fmt"
	redisbloom "github.com/RedisBloom/redisbloom-go"
	"github.com/amankumarsingh77/search_engine/config"
	"log"
)

const (
	approx_items      = 1_000_000
	error_rate        = 0.1
	bloom_filter_name = "visited_url"
)

type BloomFilter struct {
	ctx    context.Context
	client *redisbloom.Client
}

func NewRedisBloomFilter(ctx context.Context, cfg *config.RedisConfig) (*BloomFilter, error) {
	log.Println(cfg)
	client := redisbloom.NewClient(
		cfg.Host,
		"",
		nil,
	)
	if err := client.Reserve(bloom_filter_name, error_rate, approx_items); err != nil {
		return nil, fmt.Errorf("could not reserver bloom filter :%w", err)
	}
	return &BloomFilter{
		ctx,
		client,
	}, nil
}

func (r *BloomFilter) Add(url string) error {
	_, err := r.client.Add(bloom_filter_name, url)
	return err
}

func (r *BloomFilter) Exists(url string) (bool, error) {
	exists, err := r.client.Exists(bloom_filter_name, url)
	if err != nil {
		return false, fmt.Errorf("failed to check bloom filter : %w", err)
	}
	return exists, nil
}
