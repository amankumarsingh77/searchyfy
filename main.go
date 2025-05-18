package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"log"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq" // PostgreSQL driver
	_ "github.com/lib/pq"
	"github.com/reiver/go-porterstemmer"
	"golang.org/x/text/unicode/norm"
)

// SearchResult represents a search result item
type SearchResult struct {
	DocID       int64   `db:"doc_id"` // Switched to doc_id
	URL         string  `db:"url"`
	Title       string  `db:"title"`
	Description string  `db:"description"`
	Keywords    string  `db:"keywords"`
	Score       float64 `db:"score"` // Score is now float64 for TF-IDF
}

// webPage represents the crawled data from a webpage
type webPage struct {
	DocID       int64    `json:"doc_id,omitempty"` // Added DocID
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`

	Headings      map[string][]string `json:"headings"`
	Paragraphs    []string            `json:"paragraphs"`
	BodyText      string              `json:"body_text"` // Combined text for processing
	InternalLinks []string            `json:"internal_links"`
	ExternalLinks []string            `json:"external_links"`
}

// RawTFData stores raw term frequencies: map[token_text]map[doc_id]raw_tf_count
type RawTFData map[string]map[int64]int

// DocLengths stores the total number of terms for each document: map[doc_id]length
type DocLengths map[int64]int

// IDFStore stores IDF for each token_id: map[token_id]idf_score
type IDFStore map[int]float64

// Global data structures for collecting TF and doc lengths during crawl
// These need to be managed carefully for concurrent access if Colly is highly parallel.
var (
	globalRawTFData  = make(RawTFData)
	globalDocLengths = make(DocLengths)
	globalDataMutex  = &sync.Mutex{} // To protect shared globalRawTFData and globalDocLengths
)

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true, "is": true, "are": true, "in": true, "on": true, "it": true, "this": true, "that": true, "to": true, "for": true, "of": true, "with": true,
	// Add more comprehensive stop words
}

func normalize(text string) string {
	text = strings.ToLower(text)
	citation := regexp.MustCompile(`\[\d+[a-zA-Z]*\]`)
	text = citation.ReplaceAllString(text, "")
	markdownLink := regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	text = markdownLink.ReplaceAllString(text, "$1")
	nonAlpha := regexp.MustCompile(`[^a-z\s]`)
	text = nonAlpha.ReplaceAllString(text, " ")
	space := regexp.MustCompile(`\s+`)
	text = space.ReplaceAllString(text, " ")
	return norm.NFC.String(strings.TrimSpace(text))
}

func tokenizeAndFilter(text string) []string {
	tokens := strings.Fields(text)
	var filtered []string
	for _, token := range tokens {
		if !stopWords[token] && len(token) > 1 { // Optionally filter very short tokens
			filtered = append(filtered, token)
		}
	}
	return filtered
}

func stemTokens(tokens []string) []string {
	var res []string
	for _, token := range tokens {
		// Each token here is already > 1 character due to tokenizeAndFilter
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Log the error and the token that caused the panic
					log.Printf("WARNING: Recovered from panic while stemming token '%s': %v", token, r)
					// Decide how to handle the token:
					// Option 1: Add the original, unstemmed token.
					res = append(res, token)
					// Option 2: Skip this token (current behavior if res is not appended here).
					// Option 3: Add a placeholder or empty string if appropriate for your indexing.
				}
			}()
			stemmed := porterstemmer.StemString(token)
			res = append(res, stemmed)
		}()
	}
	return res
}

func preprocess(text string) []string {
	text = normalize(text)
	tokens := tokenizeAndFilter(text)
	tokens = stemTokens(tokens)
	return tokens
}

// addToIndexAndDocLength populates the raw TF index and tracks document lengths.
// It now uses docID (int64) instead of url (string).
func addToIndexAndDocLength(rawTFs RawTFData, docLengths DocLengths, tokens []string, docID int64) {
	globalDataMutex.Lock() // Protect shared maps
	defer globalDataMutex.Unlock()

	if _, exists := docLengths[docID]; !exists {
		docLengths[docID] = 0
	}
	docLengths[docID] += len(tokens)

	for _, token := range tokens {
		if _, exists := rawTFs[token]; !exists {
			rawTFs[token] = make(map[int64]int)
		}
		rawTFs[token][docID]++
	}
}

// calculateIDF calculates the Inverse Document Frequency for each token_id.
func calculateIDF(tokenIDToDocFreq map[int]int, totalDocs int) IDFStore {
	idfStore := make(IDFStore)
	if totalDocs == 0 {
		return idfStore
	}
	for tokenID, numDocsContainingToken := range tokenIDToDocFreq {
		if numDocsContainingToken > 0 {
			// Using a common smoothed IDF formula: log(1 + (N - df + 0.5) / (df + 0.5))
			// This variant is often used in systems like Lucene (BM25 IDF component)
			// and helps ensure positive values and handles terms appearing in many documents.
			idfScore := math.Log(1 + (float64(totalDocs-numDocsContainingToken)+0.5)/(float64(numDocsContainingToken)+0.5))
			idfStore[tokenID] = idfScore
		}
	}
	return idfStore
}

// saveWebpageAndGetID saves webpage info to DB and returns its doc_id.
func saveWebpageAndGetID(db *sqlx.DB, page webPage) (int64, error) {
	keywords := strings.Join(page.Keywords, ",")
	var docID int64

	// Try to get existing doc_id for the URL first to handle potential race conditions
	// or if the page was already saved by a slightly different process.
	err := db.Get(&docID, "SELECT doc_id FROM webpages WHERE url = $1", page.URL)
	if err == nil {
		// URL exists, update its content
		updateSQL := `
            UPDATE webpages SET title = $1, description = $2, keywords = $3, updated_at = CURRENT_TIMESTAMP
            WHERE doc_id = $4`
		_, err = db.Exec(updateSQL, page.Title, page.Description, keywords, docID)
		if err != nil {
			return 0, fmt.Errorf("failed to update webpage (doc_id: %d): %w", docID, err)
		}
		return docID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to check existing webpage by URL %s: %w", page.URL, err)
	}

	// URL does not exist, insert new record
	insertSQL := `
        INSERT INTO webpages (url, title, description, keywords)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (url) DO UPDATE SET
            title = EXCLUDED.title,
            description = EXCLUDED.description,
            keywords = EXCLUDED.keywords,
            updated_at = CURRENT_TIMESTAMP
        RETURNING doc_id
    `
	err = db.QueryRowx(insertSQL, page.URL, page.Title, page.Description, keywords).Scan(&docID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert/update webpage %s: %w", page.URL, err)
	}
	return docID, nil
}

// getOrInsertTokenID retrieves a token's ID from the DB, inserting it if it doesn't exist.
func getOrInsertTokenID(db *sqlx.Tx, tokenText string) (int, error) {
	var tokenID int
	err := db.Get(&tokenID, "SELECT id FROM tokens WHERE token_text = $1", tokenText)
	if err == sql.ErrNoRows {
		err = db.QueryRowx("INSERT INTO tokens (token_text) VALUES ($1) RETURNING id", tokenText).Scan(&tokenID)
		if err != nil {
			return 0, fmt.Errorf("failed to insert token '%s': %w", tokenText, err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("failed to query token '%s': %w", tokenText, err)
	}
	return tokenID, nil
}

// processAndStoreTFIDF calculates and stores TF-IDF scores in the database.
func processAndStoreTFIDF(db *sqlx.DB, rawTFs RawTFData, docLengths DocLengths) error {
	log.Println("Starting TF-IDF calculation and storage...")
	totalNumberOfDocuments := len(docLengths)
	if totalNumberOfDocuments == 0 {
		log.Println("No documents processed, skipping TF-IDF calculation.")
		return nil
	}

	tokenTextToIDMap := make(map[string]int)
	tokenIDToDocFreq := make(map[int]int) // Stores document frequency for each token_id

	log.Println("Step 1: Populating tokens table and collecting document frequencies...")
	// First pass: ensure all tokens are in DB and get their IDs, and count document frequencies
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for token processing: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	for tokenText, docMap := range rawTFs {
		tokenID, err := getOrInsertTokenID(tx, tokenText) // Use transaction for token insertion
		if err != nil {
			return fmt.Errorf("error getting/inserting token_id for '%s': %w", tokenText, err)
		}
		tokenTextToIDMap[tokenText] = tokenID
		tokenIDToDocFreq[tokenID] = len(docMap)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction for token processing: %w", err)
	}
	log.Printf("Processed %d unique tokens.", len(tokenTextToIDMap))

	log.Println("Step 2: Calculating IDF scores...")
	idfScores := calculateIDF(tokenIDToDocFreq, totalNumberOfDocuments)

	log.Println("Step 3: Calculating TF-IDF and saving to token_documents...")
	insertTFIDFSQL := `
        INSERT INTO token_documents (token_id, doc_id, tfidf_score)
        VALUES ($1, $2, $3)
        ON CONFLICT (token_id, doc_id) DO UPDATE SET tfidf_score = EXCLUDED.tfidf_score
    `
	// Begin a new transaction for batch inserting TF-IDF scores
	tfidfTx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for TF-IDF storage: %w", err)
	}
	defer tfidfTx.Rollback()

	stmt, err := tfidfTx.Preparex(insertTFIDFSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare TF-IDF insert statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for tokenText, docMap := range rawTFs {
		tokenID, ok := tokenTextToIDMap[tokenText]
		if !ok {
			log.Printf("Warning: Token text '%s' not found in tokenTextToIDMap. Skipping.", tokenText)
			continue
		}

		idf := idfScores[tokenID]
		// if idf == 0 { // Terms with IDF 0 (e.g., in all docs with some formulas) might be skipped
		//  continue
		// }

		for docID, rawTFCount := range docMap {
			docLen := docLengths[docID]
			var normalizedTF float64
			if docLen > 0 {
				normalizedTF = float64(rawTFCount) / float64(docLen)
			}

			tfidfScore := normalizedTF * idf
			if tfidfScore > 0 { // Only store if TF-IDF is meaningful
				_, err := stmt.Exec(tokenID, docID, tfidfScore)
				if err != nil {
					// Consider collecting errors and continuing, or failing fast
					return fmt.Errorf("failed to insert/update TF-IDF for token_id %d, doc_id %d: %w", tokenID, docID, err)
				}
				count++
				if count%1000 == 0 { // Log progress
					log.Printf("Inserted/Updated %d TF-IDF scores...", count)
				}
			}
		}
	}

	if err := tfidfTx.Commit(); err != nil {
		return fmt.Errorf("failed to commit TF-IDF scores: %w", err)
	}

	log.Printf("Successfully calculated and stored %d TF-IDF scores.", count)
	return nil
}

// searchIndex performs a search based on TF-IDF scores.
func searchIndex(db *sqlx.DB, query string) ([]SearchResult, error) {
	processedQueryTokens := preprocess(query)
	if len(processedQueryTokens) == 0 {
		return nil, fmt.Errorf("no valid tokens in query after preprocessing")
	}

	// Get token_ids for the processed query tokens
	var queryTokenIDs []int
	for _, tokenText := range processedQueryTokens {
		var tokenID int
		err := db.Get(&tokenID, "SELECT id FROM tokens WHERE token_text = $1", tokenText)
		if err == sql.ErrNoRows {
			// Token not in our vocabulary, skip it for search
			log.Printf("Query token '%s' not found in vocabulary.", tokenText)
			continue
		} else if err != nil {
			return nil, fmt.Errorf("error fetching token_id for '%s': %w", tokenText, err)
		}
		queryTokenIDs = append(queryTokenIDs, tokenID)
	}

	if len(queryTokenIDs) == 0 {
		return nil, fmt.Errorf("no query tokens found in vocabulary")
	}

	// SQL to retrieve relevant documents and sum their TF-IDF scores for matching tokens
	sqlQuery := `
        SELECT
            td.doc_id,
            w.url,
            w.title,
            w.description,
            w.keywords,
            SUM(td.tfidf_score) as score
        FROM token_documents td
        JOIN webpages w ON td.doc_id = w.doc_id
        WHERE td.token_id = ANY($1)
        GROUP BY td.doc_id, w.url, w.title, w.description, w.keywords
        ORDER BY score DESC
        LIMIT 20 -- Add a limit for practical search results
    `

	var results []SearchResult
	err := db.Select(&results, sqlQuery, pq.Array(queryTokenIDs))
	if err != nil {
		return nil, fmt.Errorf("search query failed: %w", err)
	}
	return results, nil
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://admin:secret@localhost:5433/inverted_index_db?sslmode=disable"
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping DB: %v", err)
	}
	log.Println("Successfully connected to the database.")

	// --- Crawling Phase ---
	c := colly.NewCollector(
		colly.Async(true),
		colly.UserAgent("GoTFIDFSearchCrawler/1.0 (+http://example.com/bot)"), // Be a good bot
		colly.IgnoreRobotsTxt(), // Be careful with this in production
		colly.MaxDepth(1),       // Limit depth for testing
		// colly.AllowedDomains("example.com", "www.example.com"), // Restrict to specific domains
	)

	err = c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 5, // Adjust parallelism
		Delay:       1 * time.Second,
		// RandomDelay: 1 * time.Second,
	})
	if err != nil {
		log.Fatal("Failed to set limit rule:", err)
	}

	c.OnHTML("html", func(e *colly.HTMLElement) {
		doc := e.DOM
		pageURL := e.Request.URL.String()

		// Extract content
		title := strings.TrimSpace(doc.Find("title").Text())
		description := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
		keywordsRaw := strings.TrimSpace(doc.Find("meta[name='keywords']").AttrOr("content", ""))
		var keywords []string
		if keywordsRaw != "" {
			keywords = strings.Split(keywordsRaw, ",")
			for i, k := range keywords {
				keywords[i] = strings.TrimSpace(k)
			}
		}

		var bodyTextBuilder strings.Builder
		doc.Find("h1, h2, h3, h4, h5, h6, p, span, div").Each(func(_ int, s *goquery.Selection) {
			// Avoid script/style content if they are inside these tags
			if s.Is("script") || s.Is("style") {
				return
			}
			bodyTextBuilder.WriteString(strings.TrimSpace(s.Text()))
			bodyTextBuilder.WriteString("\n")
		})

		pageData := webPage{
			URL:         pageURL,
			Title:       title,
			Description: description,
			Keywords:    keywords,
			BodyText:    bodyTextBuilder.String(), // Use this for preprocessing
		}

		// Save webpage to DB and get its doc_id
		docID, err := saveWebpageAndGetID(db, pageData)
		if err != nil {
			log.Printf("Error saving webpage %s: %v", pageURL, err)
			return // Skip indexing if page saving fails
		}

		// Preprocess and add to in-memory TF/DocLength stores
		tokens := preprocess(pageData.Title + " " + pageData.Description + " " + pageData.BodyText) // Combine relevant text fields
		if len(tokens) > 0 {
			addToIndexAndDocLength(globalRawTFData, globalDocLengths, tokens, docID)
		}

		// Optionally, find and visit more links
		doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			link := e.Request.AbsoluteURL(s.AttrOr("href", ""))
			// Add basic filtering for links if needed
			if link != "" {
				// e.Request.Visit(link) // Be careful with recursion and scope
			}
		})
	})

	c.OnRequest(func(r *colly.Request) {
		log.Println("Visiting", r.URL.String())
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Printf("Error crawling %s: %v (Status: %d)", r.Request.URL.String(), err, r.StatusCode)
	})

	c.OnScraped(func(r *colly.Response) {
		log.Println("Finished scraping", r.Request.URL.String())
	})

	startURLs := []string{
		"https://www.imdb.com/india/top-rated-indian-movies/",
		"https://www.bollywoodhungama.com/movie-release-dates/",
		"https://www.filmfare.com/",
		"https://www.pinkvilla.com/entertainment/bollywood",
		"https://www.koimoi.com/",
		"https://www.bollywoodlife.com/",
		"https://timesofindia.indiatimes.com/entertainment/hindi/bollywood/news",
		"https://www.india.com/entertainment/bollywood/",
		"https://www.indiatoday.in/movies/bollywood",
		"https://www.hindustantimes.com/entertainment/bollywood",
		"https://www.zoomtventertainment.com/bollywood",
		"https://www.ibtimes.co.in/bollywood",
		"https://www.cineblitz.in/",
		"https://www.bollywoodbubble.com/",
		"https://www.spotboye.com/",
		"https://www.bollywoodmdb.com/",
		"https://www.desimartini.com/bollywood-news/",
		"https://www.bollywoodmantra.com/",
		"https://www.bollywoodtadka.in/",
		"https://www.masala.com/bollywood",
		"https://www.mid-day.com/entertainment/bollywood-news",
		"https://www.rediff.com/movies/",
		"https://www.indiaglitz.com/bollywood",
		"https://www.deccanchronicle.com/entertainment/bollywood",
		"https://www.thequint.com/entertainment/bollywood",
		"https://www.filmibeat.com/bollywood.html",
		"https://www.thenewsminute.com/entertainment/bollywood",
		"https://www.bollywoodshaadis.com/",
		"https://www.bollywoodcat.com/",
		"https://www.movietalkies.com/",
		"https://www.bollywooddhamaka.in/",
		"https://www.bollywoodpapa.com/",
		"https://www.bollywoodgaram.com/",
		"https://www.bollywoodchakkar.com/",
		"https://www.tellychakkar.com/movie",
		"https://www.bollyworm.com/",
		"https://www.bollywoodherald.com/",
		"https://www.indicine.com/",
		"https://www.bollyarena.net/",
		"https://www.boxofficeindia.com/",
	}

	for _, url := range startURLs {
		err := c.Visit(url)
		if err != nil {
			log.Printf("Error initiating visit to %s: %v", url, err)
		}
	}

	log.Println("Crawler started. Waiting for tasks to complete...")
	c.Wait() // Wait for all crawling tasks to finish
	log.Println("Crawling finished.")

	// --- TF-IDF Calculation and Storage Phase ---
	// This happens AFTER crawling is complete
	err = processAndStoreTFIDF(db, globalRawTFData, globalDocLengths)
	if err != nil {
		log.Fatalf("Failed to process and store TF-IDF: %v", err)
	}
	log.Println("TF-IDF processing complete.")

	// --- Search Phase ---
	searchQuery := "zoom tv"
	log.Printf("Searching for: '%s'", searchQuery)
	startTime := time.Now()
	searchResults, err := searchIndex(db, searchQuery)
	if err != nil {
		log.Fatalf("Search error: %v", err)
	}
	searchDuration := time.Since(startTime)

	fmt.Println("\n--- Search Results ---")
	if len(searchResults) == 0 {
		fmt.Println("No results found.")
	}
	for i, res := range searchResults {
		fmt.Printf("%d. URL: %s\n", i+1, res.URL)
		fmt.Printf("   Title: %s\n", res.Title)
		// fmt.Printf("   Description: %s\n", res.Description)
		fmt.Printf("   Score: %f\n", res.Score)
		fmt.Println("---")
	}
	fmt.Printf("Search took %s\n", searchDuration)
}

// savePageToJson is a utility function (not used in main flow anymore, but kept for reference)
func savePageToJson(p webPage) {
	fileName := strings.ReplaceAll(p.URL, "http://", "")
	fileName = strings.ReplaceAll(fileName, "https://", "")
	fileName = strings.ReplaceAll(fileName, "/", "_") + ".json"
	file, err := os.Create(fileName)
	if err != nil {
		log.Printf("Failed to create JSON file %s: %v", fileName, err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(p)
	if err != nil {
		log.Printf("Could not encode data for %s: %v", p.URL, err)
	}
}
