package models

type WebPage struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`

	Headings      map[string][]string `json:"headings"`
	Paragraphs    []string            `json:"paragraphs"`
	BodyText      string              `json:"body_text"`
	InternalLinks []string            `json:"internal_links"`
	ExternalLinks []string            `json:"external_links"`
	ErrorString   string              `json:"error_string"`
}
