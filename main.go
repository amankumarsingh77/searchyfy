package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type webPage struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Canonical   string   `json:"canonical"`
	Content     string   `json:"content"`
	Language    string   `json:"language"`

	Headings   map[string][]string `json:"headings"`
	Paragraphs []string            `json:"paragraphs"`
	BodyText   string              `json:"body_text"`

	InternalLinks []string `json:"internal_links"`
	ExternalLinks []string `json:"external_links"`

	Images  []string `json:"images"`
	Videos  []string `json:"videos"`
	Iframes []string `json:"iframes"`

	OpenGraph   map[string]string `json:"open_graph"`
	TwitterData map[string]string `json:"twitter_data"`
	JSONLD      map[string]string `json:"json_ld"`

	RobotMeta    string `json:"robot_meta"`
	LastModified string `json:"last_modified"`
	PublishedAt  string `json:"published_at"`
	UpdatedAt    string `json:"updated_at"`

	ResponseHeaders *http.Header `json:"response_headers"`
}

var gzipWriter *gzip.Writer
var fileHandle *os.File

func initGzipWriter(filename string) {
	var err error
	fileHandle, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open gzip file: %v", err)
	}

	gzipWriter = gzip.NewWriter(fileHandle)
}

func closeGzipWriter() {
	if err := gzipWriter.Close(); err != nil {
		log.Fatal("Gzip close error:", err)
	}
	if err := fileHandle.Close(); err != nil {
		log.Fatal("File close error:", err)
	}
}

func main() {
	c := colly.NewCollector(
		colly.Async(true),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36"),
		colly.IgnoreRobotsTxt(),
		colly.MaxDepth(1),
	)

	initGzipWriter("pages.ndjson.gz")
	defer closeGzipWriter()

	err := c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 10,
		Delay:       500 * time.Millisecond,
	})

	if err != nil {
		log.Fatal(err)
	}

	c.OnHTML("html", func(e *colly.HTMLElement) {
		doc := e.DOM
		page := webPage{
			URL:             e.Request.URL.String(),
			Title:           doc.Find("title").Text(),
			Description:     doc.Find("meta[name='description']").AttrOr("content", ""),
			Keywords:        strings.Split(doc.Find("meta[name='keywords']").AttrOr("content", ""), ","),
			Canonical:       doc.Find("link[rel='canonical']").AttrOr("href", ""),
			Language:        doc.Find("html").AttrOr("lang", ""),
			Content:         doc.Find("body").Text(),
			LastModified:    doc.Find("meta[name='last-modified']").AttrOr("content", ""),
			RobotMeta:       doc.Find("meta[name='robots']").AttrOr("content", ""),
			ResponseHeaders: e.Response.Headers,
		}

		page.Headings = make(map[string][]string)
		for i := 1; i <= 6; i++ {
			hTag := "h" + strconv.Itoa(i)
			doc.Find(hTag).Each(func(_ int, s *goquery.Selection) {
				page.Headings[hTag] = append(page.Headings[hTag], strings.TrimSpace(s.Text()))
			})
		}

		doc.Find("p").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if len(text) > 0 {
				page.Paragraphs = append(page.Paragraphs, text)
				page.BodyText += text + "\n"
			}
		})

		doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
			link := e.Request.AbsoluteURL(s.AttrOr("href", ""))
			if len(link) == 0 {
				return
			}
			if strings.HasPrefix(link, e.Request.URL.Scheme+"://"+e.Request.URL.Host) {
				page.InternalLinks = append(page.InternalLinks, link)
			} else {
				page.ExternalLinks = append(page.ExternalLinks, link)
			}
		})

		doc.Find("img").Each(func(_ int, s *goquery.Selection) {
			page.Images = append(page.Images, e.Request.AbsoluteURL(s.AttrOr("src", "")))
		})

		doc.Find("video").Each(func(_ int, s *goquery.Selection) {
			page.Videos = append(page.Videos, e.Request.AbsoluteURL(s.AttrOr("src", "")))
		})

		page.OpenGraph = extractMetaTags(doc, "property", "og:")
		page.TwitterData = extractMetaTags(doc, "name", "twitter:")

		savePage(page)

	})
	c.OnRequest(func(r *colly.Request) {
		//if isVisited(r.URL.String()) {
		//	log.Println("Already visited", r.URL.String())
		//	return
		//}
		log.Println("Visiting", r.URL.String())
	})

	c.OnResponse(func(r *colly.Response) {
		log.Println("Response received for", r.Request.URL.String())
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Println("Error:", r.Request.URL.String(), err)
	})

	c.OnScraped(func(r *colly.Response) {
		log.Println("Finished", r.Request.URL.String())
	})

	urls := []string{"https://en.wikipedia.org/wiki/India", "https://en.wikipedia.org/wiki/India"}
	for _, url := range urls {
		c.Visit(url)
	}
	c.Wait()
	dir, _ := os.Getwd()
	readPagesFromGzip(filepath.Join(dir, "pages.ndjson.gz"))
	log.Println(filepath.Join(dir, "pages.ndjson.gz"))
}

func extractMetaTags(doc *goquery.Selection, attr, prefix string) map[string]string {
	result := make(map[string]string)
	doc.Find("meta").Each(func(_ int, s *goquery.Selection) {
		name := s.AttrOr(attr, "")
		if strings.HasPrefix(name, prefix) {
			content := s.AttrOr("content", "")
			result[name] = content
		}
	})
	return result
}

func savePage(p webPage) {
	data, err := json.Marshal(p)
	if err != nil {
		log.Printf("JSON marshal failed for %s: %v", p.URL, err)
		return
	}

	_, err = gzipWriter.Write(append(data, '\n'))
	if err != nil {
		log.Printf("Write failed: %v", err)
	}
}

func hashURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

var vis = make(map[string]bool)

func isVisited(url string) bool {
	hash := hashURL(url)
	if vis[hash] {
		return true
	}
	vis[hash] = true
	return false
}

func readPagesFromGzip(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal("Error opening file:", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		log.Fatal("Error creating gzip reader:", err)
	}
	defer gr.Close()

	scanner := bufio.NewScanner(gr)

	// Optional: Increase buffer size in case lines are large
	const maxCapacity = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	count := 0
	for scanner.Scan() {
		var p webPage
		line := scanner.Bytes()

		err := json.Unmarshal(line, &p)
		if err != nil {
			log.Printf("Skipping bad line: %v\nLine: %s\n", err, string(line))
			continue
		}
		fmt.Println("Read:", p.URL)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("Scanner error:", err)
	}

	fmt.Printf("Total pages read: %d\n", count)
}
