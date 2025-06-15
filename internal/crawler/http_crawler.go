package crawler

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/amankumarsingh77/search_engine/internal/common/database"
	"github.com/amankumarsingh77/search_engine/models"
	httpUrl "net/url"
	"strings"
)

type httpCrawler struct {
	collector *HttpClient
	frontier  URLFrontier
	db        database.MongoClient
}

type WebCrawler interface {
	CrawlPage(url string) (*models.WebPage, error)
}

func NewHttpCrawler(collector *HttpClient, frontier URLFrontier, db *database.MongoClient) WebCrawler {
	return &httpCrawler{
		collector: collector,
		frontier:  frontier,
		db:        *db,
	}
}

func (c *httpCrawler) CrawlPage(url string) (*models.WebPage, error) {
	var pageData *models.WebPage

	respBody, err := c.collector.Visit(url)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to read response : %v", err)
	}
	title := strings.TrimSpace(doc.Find("title").Text())
	description := strings.TrimSpace(doc.Find("meta[name='description']").AttrOr("content", ""))
	keywordsRaw := strings.TrimSpace(doc.Find("meta[name='keywords']").AttrOr("content", ""))
	var keywords []string
	if keywordsRaw != "" {
		keywords = strings.Split(keywordsRaw, ",")
		for i, word := range keywords {
			keywords[i] = strings.TrimSpace(word)
		}
	}

	var bodyTextBuilder strings.Builder
	allowedTags := map[string]bool{
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"p": true, "span": true, "div": true,
	}
	doc.Find("h1, h2, h3, h4, h5, h6, p, span, div").Each(func(_ int, s *goquery.Selection) {
		tag := goquery.NodeName(s)
		if !allowedTags[tag] {
			return
		}
		text := s.Text()
		text = strings.Join(strings.Fields(text), " ")
		if text != "" {
			bodyTextBuilder.WriteString(text)
			bodyTextBuilder.WriteString(" ")
		}
	})

	var internalLinks, externalLinks []string
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		seen := make(map[string]bool)
		parsedUrl, err := httpUrl.Parse(strings.TrimSpace(s.AttrOr("href", "")))
		if err != nil {
			return
		}
		baseUrl, _ := httpUrl.Parse(url)
		absUrl := baseUrl.ResolveReference(parsedUrl).String()

		if seen[absUrl] {
			return
		}
		if strings.Contains(absUrl, baseUrl.Host) {
			internalLinks = append(internalLinks, absUrl)
		} else {
			externalLinks = append(externalLinks, absUrl)
		}
	})

	var paras []string
	doc.Find("p").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}
		normalizedText := normalize(text)
		if normalizedText != "" {
			paras = append(paras, normalizedText)
		}
	})

	pageData = &models.WebPage{
		URL:           url,
		Title:         title,
		Description:   description,
		Paragraphs:    paras,
		Keywords:      keywords,
		BodyText:      bodyTextBuilder.String(),
		InternalLinks: internalLinks,
		ExternalLinks: externalLinks,
	}

	//docID, err := c.db.AddWebPage(pageData)
	//if err != nil {
	//	log.Printf("Error saving webpage %s: %v", url, err)
	//	return nil, fmt.Errorf("failed to save webpage: %w", err)
	//}
	//pageData.ID = docID
	return pageData, nil
}
