package crawler

import (
	"fmt"
	"github.com/amankumarsingh77/search_engine/config"
	"io"
	"net/http"
	"net/url"
	"time"
)

type HttpClient struct {
	client  *http.Client
	headers http.Header
}

func NewHttpClient(cfg *config.CrawlerConfig) *HttpClient {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
	}
	if cfg.ProxyEnabled {
		proxyUrl, err := url.Parse(cfg.ProxyUrl)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyUrl)
		} else {
			fmt.Printf("failed to load the proxy : %s. Please check the config file", cfg.ProxyUrl)
		}
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	headers := http.Header{
		"User-Agent":      []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"},
		"Accept":          []string{"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"Accept-Language": []string{"en-US,en;q=0.5"},
		"Connection":      []string{"keep-alive"},
	}
	return &HttpClient{
		client:  client,
		headers: headers,
	}
}

func (h *HttpClient) Visit(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for key, vals := range h.headers {
		for _, val := range vals {
			req.Header.Add(key, val)
		}
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("bad response status: %s", resp.Status)
	}
	return resp.Body, nil
}
