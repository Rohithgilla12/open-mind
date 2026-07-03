package enrich

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	readability "github.com/go-shiori/go-readability"
)

// Readability extracts article content using go-shiori/go-readability.
type Readability struct{ client *http.Client }

// NewReadability returns an Extractor backed by go-readability. A nil client
// falls back to an SSRF-safe client with a 30s timeout, which refuses to
// dial loopback, private, link-local, and other internal addresses.
func NewReadability(client *http.Client) *Readability {
	if client == nil {
		client = SafeHTTPClient(30 * time.Second)
	}
	return &Readability{client: client}
}

func (*Readability) Name() string { return "readability" }

func (r *Readability) Extract(ctx context.Context, rawURL string) (Extraction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Extraction{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "openmind/0.1 (+https://github.com/rohithgilla12/open-mind)")
	resp, err := r.client.Do(req)
	if err != nil {
		return Extraction{}, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Extraction{}, fmt.Errorf("fetching %s: status %d", rawURL, resp.StatusCode)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Extraction{}, fmt.Errorf("parsing url %s: %w", rawURL, err)
	}
	body := io.LimitReader(resp.Body, maxResponseBytes)
	article, err := readability.FromReader(body, parsed)
	if err != nil {
		return Extraction{}, fmt.Errorf("extracting %s: %w", rawURL, err)
	}
	return Extraction{
		Title:        article.Title,
		Body:         article.TextContent,
		LeadImageURL: article.Image,
	}, nil
}
