package crawler

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/amankumarsingh77/search_engine/models"
	"github.com/gocolly/colly"
	"log"
	"strings"
)

type crawler struct {
	collector *colly.Collector
	frontier  URLFrontier
	db        DB
}

type WebCrawler interface {
	Process(url string) (*models.WebPage, error)
}

func NewCrawler(collector *colly.Collector, frontier URLFrontier) WebCrawler {
	return &crawler{
		collector: collector,
		frontier:  frontier,
	}
}

func (c *crawler) Process(url string) (*models.WebPage, error) {
	c.collector.OnHTML("html", func(e *colly.HTMLElement) {
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

		pageData := models.WebPage{
			URL:         pageURL,
			Title:       title,
			Description: description,
			Keywords:    keywords,
			BodyText:    bodyTextBuilder.String(), // Use this for preprocessing
		}

		// Save webpage to DB and get its doc_id
		docID, err := c.db.AddWebpage(pageData)
		if err != nil {
			log.Printf("Error saving webpage %s: %v", pageURL, err)
			return // Skip indexing if page saving fails
		}

		// Preprocess and add to in-memory TF/DocLength stores
		tokens := normalizePageContent(pageData.Title + " " + pageData.Description + " " + pageData.BodyText) // Combine relevant text fields
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

	c.collector.OnRequest(func(r *colly.Request) {
		log.Println("Visiting", r.URL.String())
	})

	c.collector.OnError(func(r *colly.Response, err error) {
		log.Printf("Error crawling %s: %v (Status: %d)", r.Request.URL.String(), err, r.StatusCode)
	})

	c.collector.OnScraped(func(r *colly.Response) {
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
}
