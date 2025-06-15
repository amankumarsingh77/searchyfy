# Web Crawler with TF-IDF - Command Line Interface

This directory contains the main executable for the web crawler with TF-IDF scoring functionality.

## Prerequisites

Before running the crawler, ensure you have:

1. **PostgreSQL Database** running on `localhost:5433`
   - Database: `inverted_index_db`
   - User: `admin`
   - Password: `secret`

2. **Redis Server** running on `localhost:6379`
   - Used for URL frontier and Bloom filter

3. **Go 1.19+** installed

## Building

```bash
# From the project root directory
go build -o crawler ./cmd/
```

## Usage

The crawler supports multiple modes of operation:

### 1. Crawl Mode (Default)
Crawls websites and stores page data in the database:

```bash
./crawler -mode=crawl -workers=3 -seeds=seed_urls.csv
```

### 2. TF-IDF Calculation Mode
Calculates TF-IDF scores for crawled documents:

```bash
./crawler -mode=tfidf
```

### 3. Search Mode
Searches the indexed documents using TF-IDF scores:

```bash
./crawler -mode=search -query="bollywood movies"
```

### 4. Full Pipeline Mode
Runs the complete pipeline: crawl → TF-IDF → search:

```bash
./crawler -mode=full -workers=5 -query="bollywood news"
```

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-mode` | Operation mode: `crawl`, `tfidf`, `search`, or `full` | `crawl` |
| `-workers` | Number of worker goroutines | `3` |
| `-query` | Search query (required for search mode) | `""` |
| `-seeds` | Path to seed URLs file | `seed_urls.csv` |
| `-config` | Path to configuration file | `crawler.yaml` |

## Configuration

The crawler looks for a `crawler.yaml` configuration file. If not found, it uses default settings:

```yaml
MaxDepth: 1
Workers: 3

Redis:
  Host: localhost:6379
  Port: 6379
  Local: true
  SSL: false
  URLQueue: url_queue

DB:
  Host: localhost
  Port: 5433
  User: admin
  Password: secret
  DBName: inverted_index_db
  SSL: false
  Local: true
```

## Seed URLs

Create a `seed_urls.csv` file with starting URLs, or the crawler will use default Bollywood-related websites.

## Examples

### Basic Crawling
```bash
# Crawl with 5 workers
./crawler -mode=crawl -workers=5

# Crawl specific sites
echo "https://example.com" > my_seeds.csv
./crawler -mode=crawl -seeds=my_seeds.csv
```

### Search After Crawling
```bash
# First crawl
./crawler -mode=crawl -workers=3

# Then calculate TF-IDF
./crawler -mode=tfidf

# Finally search
./crawler -mode=search -query="movie reviews"
```

### Full Pipeline
```bash
# Do everything in one command
./crawler -mode=full -workers=5 -query="bollywood actors"
```

## Monitoring

The crawler provides detailed logging:
- Worker status and progress
- Pages crawled successfully/with errors
- TF-IDF calculation progress
- Search results

## Graceful Shutdown

The crawler handles `SIGINT` (Ctrl+C) and `SIGTERM` signals gracefully:
- Stops accepting new URLs
- Completes current crawling tasks
- Saves progress to database
- Closes connections properly

## Troubleshooting

### Database Connection Issues
- Ensure PostgreSQL is running on the correct port
- Check database credentials in configuration
- Verify database exists and is accessible

### Redis Connection Issues
- Ensure Redis server is running
- Check Redis configuration
- Verify Bloom filter functionality

### Crawling Issues
- Check network connectivity
- Verify seed URLs are accessible
- Monitor rate limiting and delays

## Architecture

The main.go orchestrates:
1. **Configuration Loading**: From YAML file or defaults
2. **Database Connection**: PostgreSQL for storing pages and TF-IDF scores
3. **Redis Setup**: For URL frontier and deduplication
4. **Worker Management**: Concurrent crawling with configurable workers
5. **TF-IDF Processing**: Mathematical scoring of document relevance
6. **Search Interface**: Query processing and result ranking

## Next Steps

After running the crawler:
1. Monitor database for crawled pages
2. Verify TF-IDF scores are calculated
3. Test search functionality
4. Tune configuration for your use case
5. Add custom seed URLs for your domain
