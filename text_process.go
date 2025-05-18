package main

import (
	"encoding/json"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"log"
	"os"
)

//type RawTFIndex map[string]map[string]int
//
//type WebPage struct {
//	URL           string              `json:"url"`
//	Title         string              `json:"title"`
//	Description   string              `json:"description"`
//	Keywords      []string            `json:"keywords"`
//	Headings      map[string][]string `json:"headings"`
//	Paragraphs    []string            `json:"paragraphs"`
//	BodyText      string              `json:"body_text"`
//	InternalLinks []string            `json:"internal_links"`
//	ExternalLinks []string            `json:"external_links"`
//}
//
//var stopWords = map[string]bool{
//	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
//	"so": true, "if": true, "because": true, "in": true, "on": true, "at": true,
//	"by": true, "for": true, "with": true, "about": true, "against": true,
//	"between": true, "into": true, "through": true, "during": true, "before": true,
//	"after": true, "above": true, "below": true, "to": true, "from": true,
//	"up": true, "down": true, "off": true, "over": true, "under": true,
//	"i": true, "me": true, "my": true, "myself": true, "we": true, "our": true,
//	"you": true, "your": true, "he": true, "him": true, "his": true, "she": true,
//	"her": true, "it": true, "its": true, "they": true, "them": true,
//	"is": true, "am": true, "are": true, "was": true, "were": true, "be": true,
//	"been": true, "have": true, "has": true, "had": true, "do": true, "does": true,
//	"did": true, "will": true, "would": true, "shall": true, "should": true,
//	"can": true, "could": true, "may": true, "might": true, "must": true,
//	"not": true, "no": true, "yes": true, "there": true, "here": true,
//	"just": true, "now": true, "then": true, "also": true, "too": true, "very": true,
//	"that": true, "this": true, "these": true, "those": true, "as": true,
//	"such": true, "than": true, "same": true, "own": true, "again": true,
//	"once": true, "ever": true, "always": true,
//}
//
//func normalize(text string) string {
//	text = strings.ToLower(text)
//
//	// Remove citation-like patterns [12], [citation needed]
//	citation := regexp.MustCompile(`\[\d+[a-zA-Z]*\]`)
//	text = citation.ReplaceAllString(text, "")
//
//	// Remove markdown links [text](url)
//	markdownLink := regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
//	text = markdownLink.ReplaceAllString(text, "$1")
//
//	// Remove all non-alphabetic characters (including numbers)
//	nonAlpha := regexp.MustCompile(`[^a-z\s]`)
//	text = nonAlpha.ReplaceAllString(text, " ")
//
//	// Replace multiple spaces with single space
//	space := regexp.MustCompile(`\s+`)
//	text = space.ReplaceAllString(text, " ")
//
//	return norm.NFC.String(strings.TrimSpace(text))
//}
//
//func tokenizeAndFilter(text string) []string {
//	tokens := strings.Fields(text)
//	var filtered []string
//	for _, token := range tokens {
//		if !stopWords[token] {
//			filtered = append(filtered, token)
//		}
//	}
//	return filtered
//}
//
//func stemTokens(tokens []string) []string {
//	var res []string
//	for _, token := range tokens {
//		stemmed := porterstemmer.StemString(token)
//		res = append(res, stemmed)
//	}
//	return res
//}
//
//func preprocess(text string) []string {
//	text = normalize(text)
//	tokens := tokenizeAndFilter(text)
//	tokens = stemTokens(tokens)
//	return tokens
//}
//
//func addToIndex(index RawTFIndex, tokens []string, url string) {
//	for _, token := range tokens {
//		if _, exists := index[token]; !exists {
//			index[token] = make(map[string]int)
//		}
//		index[token][url]++ // deduplicated set-like behavior
//	}
//}
//
//func saveInvertedIndex(db *sqlx.DB, index RawTFIndex) error {
//	tx, err := db.Beginx()
//	if err != nil {
//		return err
//	}
//	defer tx.Rollback()
//
//	insertToken := `INSERT INTO tokens (token) VALUES ($1) ON CONFLICT (token) DO NOTHING`
//	selectTokenID := `SELECT id FROM tokens WHERE token = $1`
//	insertTokenDoc := `INSERT INTO token_documents (token_id, url, tf) VALUES ($1, $2, $3) ON CONFLICT (token_id, url) DO UPDATE SET tf = EXCLUDED.tf`
//
//	for token, urlMap := range index {
//		_, err := tx.Exec(insertToken, token)
//		if err != nil {
//			return err
//		}
//
//		var tokenID int
//		err = tx.Get(&tokenID, selectTokenID, token)
//		if err != nil {
//			return err
//		}
//
//		for url, tfValue := range urlMap {
//			// Here tfValue is struct{} in your type, so you should change RawTFIndex to hold counts (int)
//			// Assuming tfValue is int, change your map accordingly.
//			_, err := tx.Exec(insertTokenDoc, tokenID, url, tfValue)
//			if err != nil {
//				return err
//			}
//		}
//	}
//
//	return tx.Commit()
//}

func main() {
	// Open the JSON file
	file, err := os.Open("page.json")
	if err != nil {
		log.Fatalf("failed to open the file: %v", err)
	}
	defer file.Close()

	var page WebPage
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&page); err != nil {
		log.Fatalf("failed to decode JSON: %v", err)
	}

	// Connect to PostgreSQL using sqlx
	dsn := "postgres://admin:secret@localhost:5433/inverted_index_db?sslmode=disable"
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping DB: %v", err)
	}

	// Create inverted index
	index := make(RawTFIndex)
	for _, para := range page.Paragraphs {
		tokens := preprocess(para)
		addToIndex(index, tokens, page.URL)
	}

	// Save to Postgres
	if err := saveInvertedIndex(db, index); err != nil {
		log.Fatalf("failed to save index: %v", err)
	}

	log.Println("✅ Inverted index saved to Postgres successfully.")
}
