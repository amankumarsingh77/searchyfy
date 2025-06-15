package crawler

import (
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/reiver/go-porterstemmer"
	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
)

func normalizeUrl(rawUrl string) (string, error) {
	rawUrl = strings.TrimSpace(rawUrl)

	if !strings.Contains(rawUrl, "://") {
		rawUrl = "https://" + rawUrl
	}
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", fmt.Errorf("error parsing URL: %w", err)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")

	p := idna.New(idna.ValidateForRegistration())
	asciiHost, err := p.ToASCII(host)
	if err != nil {
		return "", fmt.Errorf("could not convert host to ASCII: %w", err)
	}
	u.Host = asciiHost

	if u.Host != "" && u.Path == "" {
		u.Path = "/"
	}
	u.Fragment = ""

	return u.String(), nil
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true, "is": true, "are": true, "in": true, "on": true, "it": true, "this": true, "that": true, "to": true, "for": true, "of": true, "with": true,
}

func normalize(text string) string {
	text = strings.ToLower(text)
	citation := regexp.MustCompile(`\[\d+[a-zA-Z]*]`)
	text = citation.ReplaceAllString(text, "")
	markdownLink := regexp.MustCompile(`\[(.*?)]\((.*?)\)`)
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
		if !stopWords[token] && len(token) > 1 {
			filtered = append(filtered, token)
		}
	}
	return filtered
}

func stemTokens(tokens []string) []string {
	var res []string
	for _, token := range tokens {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("WARNING: Recovered from panic while stemming token '%s': %v", token, r)
					res = append(res, token)
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
