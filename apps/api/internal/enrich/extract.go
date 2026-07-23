package enrich

import (
	"context"
)

// maxResponseBytes caps how much of a fetched response body extractors will
// read, bounding memory use against hostile or runaway responses.
const maxResponseBytes = 10 << 20 // 10 MB

// Extraction holds the article content pulled from a saved URL.
type Extraction struct {
	Title          string
	Body           string
	LeadImageURL   string
	TaggedLocation string
}

// Extractor fetches and extracts the main content of a URL.
type Extractor interface {
	Name() string
	Extract(ctx context.Context, url string) (Extraction, error)
}
