package crawler

import (
	"fmt"
	redisbloom "github.com/RedisBloom/redisbloom-go"
	"github.com/amankumarsingh77/search_engine/config"
	"log"
	"strings"
)

const (
	approxItems     = 1_000_000
	errorRate       = 0.1
	bloomFilterName = "visited_url"
)

type BloomFilter struct {
	client *redisbloom.Client
}

func NewRedisBloomFilter(cfg *config.RedisConfig) (*BloomFilter, error) {
	client := redisbloom.NewClient(
		cfg.Host,
		"",
		nil,
	)
	if err := client.Reserve(bloomFilterName, errorRate, approxItems); err != nil {
		if strings.Contains(err.Error(), "item exists") {
			log.Println("Skipping : Bloom filter already reserved")
		} else {
			return nil, fmt.Errorf("could not reserve bloom filter :%w", err)
		}
	}
	return &BloomFilter{
		client,
	}, nil
}

func (r *BloomFilter) Add(url string) error {
	_, err := r.client.Add(bloomFilterName, url)
	return err
}

func (r *BloomFilter) Exists(url string) (bool, error) {
	exists, err := r.client.Exists(bloomFilterName, url)
	if err != nil {
		return false, fmt.Errorf("failed to check bloom filter : %w", err)
	}
	return exists, nil
}
