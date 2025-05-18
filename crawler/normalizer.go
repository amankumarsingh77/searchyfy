package crawler

import (
	"fmt"
	"github.com/reiver/go-porterstemmer"
	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
	"log"
	"net/url"
	"regexp"
	"strings"
)

func normalizeUrl(rawUrl string) (string, error) {
	rawUrl = strings.TrimSpace(rawUrl)
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", fmt.Errorf("error in parsing the raw url :%w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	p := idna.New(idna.ValidateForRegistration())
	asciiHost, err := p.ToASCII(host)
	if err == nil {
		host = asciiHost
	} else {
		log.Fatal("could not convert the ")
	}
	u.Host = host
	if u.Host != "" && u.Path == "" {
		u.Path = "/"
	}
	u.Fragment = ""
	return u.String(), nil
}

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

func normalizePageContent(text string) []string {
	text = normalize(text)
	tokens := tokenizeAndFilter(text)
	tokens = stemTokens(tokens)
	return tokens
}
