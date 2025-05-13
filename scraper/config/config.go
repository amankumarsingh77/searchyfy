package config

type CrawlerConfig struct {
	ProxyUrl     string
	ProxyEnabled bool
	MaxDepth     int
	Workers      int
}

func LoadCrawlerConfig() *CrawlerConfig {
	viper
}
