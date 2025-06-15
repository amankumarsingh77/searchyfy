package config

import (
	"fmt"
	"github.com/spf13/viper"
)

func LoadCrawlerConfig(filename string) (*CrawlerConfig, error) {
	viper.SetConfigName(filename)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	var config CrawlerConfig
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("cannot read the file %w", err)
	}
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error reading the config file %w", err)
	}
	return &config, nil
}

func GetDefaultConfig() *CrawlerConfig {
	return &CrawlerConfig{
		MaxDepth: 1,
		Workers:  3,
		Redis: RedisConfig{
			Host:     "localhost:6379",
			Port:     6379,
			Local:    true,
			SSL:      false,
			URLQueue: "url_queue",
		},
		DB: PostgresConfig{
			Host:     "localhost",
			Port:     5433,
			User:     "admin",
			Password: "secret",
			DBName:   "inverted_index_db",
			SSL:      false,
			Local:    true,
		},
	}
}
