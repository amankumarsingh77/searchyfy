# Searchyfy

A high-performance, distributed web search engine built in Go featuring intelligent crawling, advanced indexing with TF-IDF scoring, and lightning-fast search capabilities.

## Overview

Searchyfy is a production-ready search engine that combines web crawling, document indexing, and search functionality into a single, scalable system. Built with modern Go practices, it supports distributed crawling, sophisticated text processing, and multiple search algorithms including BM25 and TF-IDF ranking.

## Key Features

- **Distributed Web Crawling**: Multi-threaded crawler with Redis-based URL frontier and Bloom filter deduplication
- **Advanced Text Processing**: Smart tokenization, stemming, and stop-word filtering with UTF-8 normalization
- **Inverted Index**: PostgreSQL-backed inverted index with efficient posting lists and term frequency storage
- **Multiple Ranking Algorithms**: BM25, TF-IDF, and hybrid scoring with customizable parameters
- **Real-time Search API**: RESTful search API with pagination, caching, and sub-second response times
- **Web Interface**: Clean, responsive search interface with Google-like user experience
- **Horizontal Scalability**: Supports multiple crawler workers and indexer processes
- **Graceful Operations**: Comprehensive error handling, graceful shutdown, and recovery mechanisms

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Crawler   │───▶│     Indexer     │───▶│ Search Engine   │
│                 │    │                 │    │                 │
│ • HTTP Client   │    │ • Text Proc.    │    │ • Query Parser  │
│ • URL Frontier  │    │ • Inverted Idx  │    │ • Ranking       │
│ • Bloom Filter  │    │ • Batch Proc.   │    │ • Result Cache  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ MongoDB (Raw)   │    │ PostgreSQL (Idx)│    │   Redis Cache   │
│ • Web Pages     │    │ • Terms         │    │ • Query Cache   │
│ • Metadata      │    │ • Documents     │    │ • Session Data  │
│ • Link Graph    │    │ • Postings      │    │ • Statistics    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Technology Stack

- **Language**: Go 1.24+
- **Databases**: PostgreSQL (inverted index), MongoDB (raw data), Redis (caching, queuing)
- **Web Framework**: Fiber v2 (high-performance HTTP framework)
- **Text Processing**: Porter Stemmer, Unicode normalization
- **Search Algorithms**: BM25, TF-IDF, Cosine Similarity
- **Containerization**: Docker & Docker Compose
- **Dependencies**: See [go.mod](go.mod) for complete list

## Quick Start

### Prerequisites

- Go 1.24 or higher
- Docker and Docker Compose
- Git

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/amankumarsingh77/searchyfy.git
   cd searchyfy
   ```

2. **Start infrastructure services**
   ```bash
   docker-compose up -d postgres redis
   ```

3. **Install dependencies**
   ```bash
   go mod download
   ```

4. **Build the application**
   ```bash
   go build -o searchyfy ./cmd/
   ```

### Basic Usage

1. **Start crawling websites**
   ```bash
   ./searchyfy -mode=crawl -workers=5
   ```

2. **Index the crawled content**
   ```bash
   ./searchyfy -mode=indexer
   ```

3. **Start the search API**
   ```bash
   ./searchyfy -mode=search
   ```

4. **Access the web interface**
   Open http://localhost:8080 in your browser

## Detailed Usage

### Command Line Options

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `-mode` | Operation mode | `crawl` | `-mode=search` |
| `-workers` | Number of worker goroutines | `3` | `-workers=10` |
| `-config` | Configuration file path | `crawler.yaml` | `-config=prod.yaml` |
| `-seedfile` | Seed URLs file path | `seed_urls.csv` | `-seedfile=urls.csv` |

### Available Modes

#### 1. Crawl Mode
Discovers and downloads web pages, storing raw content in MongoDB.

```bash
./searchyfy -mode=crawl

./searchyfy -mode=crawl -workers=10 # Be gentle with then number here

./searchyfy -mode=seed -seedfile=custom_urls.csv
```

#### 2. Indexer Mode
Processes raw content into searchable inverted index in PostgreSQL.

```bash
./searchyfy -mode=indexer

```

#### 3. Search Mode
Runs the search API server with web interface.

```bash
./searchyfy -mode=search
```

### Configuration

Configuration is managed through `crawler.yaml`:

```yaml
ProxyUrl: https://demo-proxy.workers.dev?destination=
ProxyEnabled: true
MaxDepth: 5
Workers: 1

Redis:
  Host: localhost:6379
  Port: 6379
  User: ""
  Password: ""
  Local: true
  SSL: false
  URLQueue: url_queue

Search:
  WarmCache: false
  HTTPAddr: ":8080"

DB:
  Host: localhost
  Port: 5433
  User: admin
  Password: secret
  DBName: inverted_index_db
  SSL: false
  Local: true

Mongo:
  URI: mongodb://localhost:27017
  Local: true
  DBName: searchyfy
  CrawlerColl: rawdata

Query:
  TermCacheSize: 10000
  PostingCacheSize: 5000
  StemmerLang: eng

Index:
  DBURL: postgresql://<username>:<passwod>@localhost:5433/inverted_index_db
  PoolSize: 100
  Workers: 3
  BatchSize: 500
```

## API Documentation

### Search Endpoint

**GET** `/search`

Search for documents using the indexed content.

#### Parameters
- `q` (required): Search query string
- `page` (optional): Page number (default: 1)
- `page_size` (optional): Results per page (default: 10, max: 100)

#### Example Request
```bash
curl "http://localhost:8080/search?q=machine+learning&page=1&page_size=10"
```

#### Example Response
```json
{
  "query": "machine learning",
  "page": 1,
  "page_size": 10,
  "total": 156,
  "total_pages": 16,
  "response_time": 0.045,
  "results": [
    {
      "title": "Introduction to Machine Learning",
      "url": "https://example.com/ml-intro",
      "snippet": "Machine learning is a subset of artificial intelligence...",
      "score": 8.42
    }
  ]
}
```

### Health Check Endpoint

**GET** `/`

Returns the search interface homepage.

## Advanced Features

### Ranking Algorithms

Searchyfy supports multiple ranking algorithms:

1. **BM25** (default): Probabilistic ranking with tunable parameters
2. **TF-IDF**: Classic term frequency-inverse document frequency
3. **Cosine Similarity**: Vector space model similarity
4. **Hybrid Scoring**: Combines multiple signals for enhanced relevance

### Performance Optimizations

- **Parallel Processing**: Multi-threaded crawling, indexing, and search
- **Efficient Caching**: Redis-based query and term caching
- **Batch Operations**: Bulk database operations for improved throughput
- **Connection Pooling**: Optimized database connection management
- **Bloom Filters**: Memory-efficient duplicate URL detection

### Text Processing Pipeline

1. **HTML Parsing**: Extract title, description, headings, and body text
2. **Normalization**: Unicode normalization and character cleaning
3. **Tokenization**: Split text into meaningful tokens
4. **Filtering**: Remove stop words and invalid tokens
5. **Stemming**: Reduce words to their root forms
6. **Indexing**: Build inverted index with position information

## Deployment

### Using Docker Compose

```bash
docker-compose up -d
docker-compose logs -f search_engine
docker-compose up -d --scale crawler=3
```

### Production Deployment

For production deployment:

1. **Use environment variables** for configuration
2. **Enable SSL/TLS** for API endpoints
3. **Set up monitoring** and health checks
4. **Configure log aggregation**
5. **Implement backup strategies** for databases

### Environment Variables

```bash
export SEARCH_ENGINE_REDIS_HOST=redis.production.com
export SEARCH_ENGINE_DB_HOST=postgres.production.com
export SEARCH_ENGINE_MONGO_URI=mongodb://mongo.production.com:27017
```

## Monitoring and Maintenance

### Key Metrics to Monitor

- Crawl rate (pages per minute)
- Index size and growth
- Search latency (p95, p99)
- Error rates by component
- Database performance
- Memory and CPU usage

### Maintenance Tasks

- **Database Optimization**: Regular VACUUM and ANALYZE on PostgreSQL
- **Cache Warming**: Preload frequently accessed terms
- **Index Rebuilding**: Periodic full reindexing for optimization
- **Log Rotation**: Manage log file sizes
- **Backup Verification**: Regular backup testing

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Write comprehensive tests for new features
- Follow Go best practices and idiomatic code
- Update documentation for API changes
- Ensure backward compatibility
- Add appropriate logging and error handling

## Performance Benchmarks

On a standard development machine (8-core CPU, 16GB RAM):

- **Crawling**: ~50-100 pages/minute per worker
- **Indexing**: ~1000 documents/second
- **Search**: <50ms average response time
- **Memory**: ~100MB base usage + ~1MB per 10K documents


## Acknowledgments

- Built with Go and the amazing Go ecosystem
- Inspired by academic research in information retrieval
- Thanks to the open-source community for excellent libraries and tools

---

**Note**: This is a research and educational project. For production use, ensure proper security auditing, performance testing, and compliance with relevant regulations.
