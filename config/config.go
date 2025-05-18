package config

import (
	"fmt"
	"github.com/spf13/viper"
)

type CrawlerConfig struct {
	ProxyUrl     string
	ProxyEnabled bool
	MaxDepth     int
	Workers      int
	Redis        RedisConfig
	DB           PostgresConfig
}

type RedisConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Local    bool
	SSL      bool
	URLQueue string
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	SSL      bool
	Local    bool
	DBName   string
}

func LoadCrawlerConfig() (*CrawlerConfig, error) {
	viper.SetConfigName("crawler")
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
