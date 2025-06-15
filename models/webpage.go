package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type WebPage struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	URL         string             `bson:"url" json:"url"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	Keywords    []string           `bson:"keywords" json:"keywords"`
	TokenCount  int                `json:"token_count"`

	Headings      map[string][]string `bson:"headings" json:"headings"`
	Paragraphs    []string            `bson:"paragraphs" json:"paragraphs"`
	BodyText      string              `bson:"body_text" json:"body_text"`
	InternalLinks []string            `bson:"internal_links" json:"internal_links"`
	ExternalLinks []string            `bson:"external_links" json:"external_links"`
	ErrorString   string              `bson:"error_string,omitempty" json:"error_string,omitempty"`
	CreatedAt     primitive.DateTime  `bson:"created_at" json:"created_at"`
	UpdatedAt     primitive.DateTime  `bson:"updated_at" json:"updated_at"`
}
