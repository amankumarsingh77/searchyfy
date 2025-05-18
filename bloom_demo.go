package main

import (
	"context"
	"github.com/amankumarsingh77/search_engine/config"
	"github.com/amankumarsingh77/search_engine/crawler"
	"log"
)

func main() {
	cfg, err := config.LoadCrawlerConfig()
	ctx := context.Background()
	if err != nil {
		log.Println("could not config file : %w", err)
	}
	bloom_client, err := crawler.NewRedisBloomFilter(ctx, &cfg.Redis)
	if err != nil {
		log.Println("could not create a bloom client : %w", err)
	}
	if err = bloom_client.Add("https://google.com"); err != nil {
		log.Println(err)
	}
	exists, err := bloom_client.Exists("https://google.com")
	if err != nil {
		log.Println(err)
	}
	log.Println(exists)
}
