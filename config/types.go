package config

import "time"

type CrawlerConfig struct {
	ProxyUrl     string
	ProxyEnabled bool
	MaxDepth     int64
	Workers      int
	APIADDR      int
	Redis        RedisConfig
	DB           PostgresConfig
	Mongo        MongoConfig
	Index        IndexerConfig
	Query        QueryEngineConfig
	Search       SearchAPIConfig
}

type IndexerConfig struct {
	DBURL     string
	PoolSize  int
	Workers   int
	BatchSize int
}

type SearchAPIConfig struct {
	WarmCache bool
	HTTPAddr  string
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

type QueryEngineConfig struct {
	TermCacheSize     int
	PostingCacheSize  int
	StemmerLang       string
	MaxWorkers        int
	BatchSize         int
	CacheRefreshTime  time.Duration
	DocumentCacheSize int
}

type MongoConfig struct {
	URI         string
	Local       bool
	DBName      string
	CrawlerColl string
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
