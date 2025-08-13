package indexer

import (
	"log"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/reiver/go-porterstemmer"
	"golang.org/x/text/unicode/norm"
)

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true, "is": true, "are": true, "in": true,
	"on": true, "it": true, "this": true, "that": true, "to": true, "for": true, "of": true, "with": true,
}

func removeInvalidUTF8(s string) string {
	valid := make([]rune, 0, len(s))
	for i, r := range s {
		if r == utf8.RuneError {
			_, size := utf8.DecodeRuneInString(s[i:])
			if size == 1 {
				continue
			}
		}
		valid = append(valid, r)
	}
	return string(valid)
}

func normalize(text string) string {
	text = removeInvalidUTF8(text)
	text = strings.ToLower(text)

	replacements := []*regexp.Regexp{
		regexp.MustCompile(`<[^>]*>`),
		regexp.MustCompile(`[a-z\-]+:\s*[^;]+;`),
		regexp.MustCompile(`"[^"]+"\s*:\s*"?[^",}{\[\]]*"?`),
		regexp.MustCompile(`[a-f0-9]{32,}\.(jpg|jpeg|png|svg|webp)`),
		regexp.MustCompile(`https?://[^\s"]+`),
		regexp.MustCompile(`[\d\-_\.]{6,}`),
		regexp.MustCompile(`[a-z_]+\([^\)]*\)`),
		regexp.MustCompile(`\[\d+[a-z]*]`),
		regexp.MustCompile(`\[(.*?)\]\((.*?)\)`),
	}

	for _, r := range replacements {
		text = r.ReplaceAllString(text, " ")
	}

	text = strings.ReplaceAll(text, "-", " ")
	text = strings.ReplaceAll(text, "/", " ")

	text = regexp.MustCompile(`[^a-z\s]`).ReplaceAllString(text, " ")

	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return norm.NFC.String(strings.TrimSpace(text))
}

func tokenizeAndFilter(text string) []string {
	tokens := strings.Fields(text)
	var filtered []string

	for _, token := range tokens {
		if len(token) <= 1 || stopWords[token] {
			continue
		}

		vowelCount := 0
		for _, c := range token {
			if strings.ContainsRune("aeiou", c) {
				vowelCount++
			}
		}
		if vowelCount == 0 || hasRepeatedChars(token, 3) {
			continue
		}

		filtered = append(filtered, token)
	}
	return filtered
}

func hasRepeatedChars(token string, n int) bool {
	if len(token) < n {
		return false
	}
	count := 1
	prev := rune(token[0])
	for _, c := range token[1:] {
		if c == prev {
			count++
			if count >= n {
				return true
			}
		} else {
			count = 1
			prev = c
		}
	}
	return false
}

func stemTokens(tokens []string) []string {
	var res []string
	for _, token := range tokens {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("WARNING: Recovered from panic while stemming token '%s': %v", token, r)
				}
			}()
			stemmed := porterstemmer.StemString(token)

			if len(stemmed) <= 1 || !regexp.MustCompile(`[aeiou]`).MatchString(stemmed) {
				return
			}
			res = append(res, stemmed)
		}()
	}
	return res
}

func normalizePageContent(text string) []string {
	clean := normalize(text)
	tokens := tokenizeAndFilter(clean)
	tokens = stemTokens(tokens)
	return tokens
}
