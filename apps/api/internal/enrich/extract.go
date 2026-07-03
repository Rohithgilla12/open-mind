package enrich

import (
	"context"
)

// Extraction holds the article content pulled from a saved URL.
type Extraction struct {
	Title        string
	Body         string
	LeadImageURL string
}

// Extractor fetches and extracts the main content of a URL.
type Extractor interface {
	Name() string
	Extract(ctx context.Context, url string) (Extraction, error)
}
