package enrich

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/markusmobius/go-trafilatura"
)

// Trafilatura extracts article content using the go-trafilatura library.
type Trafilatura struct{ client *http.Client }

// NewTrafilatura returns an Extractor backed by go-trafilatura. A nil client
// falls back to an SSRF-safe client with a 30s timeout, which refuses to
// dial loopback, private, link-local, and other internal addresses.
func NewTrafilatura(client *http.Client) *Trafilatura {
	if client == nil {
		client = SafeHTTPClient(30 * time.Second)
	}
	return &Trafilatura{client: client}
}

func (*Trafilatura) Name() string { return "trafilatura" }

func (t *Trafilatura) Extract(ctx context.Context, rawURL string) (Extraction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Extraction{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "openmind/0.1 (+https://github.com/rohithgilla12/open-mind)")
	resp, err := t.client.Do(req)
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
	result, err := trafilatura.Extract(body, trafilatura.Options{OriginalURL: parsed, IncludeImages: true})
	if err != nil {
		return Extraction{}, fmt.Errorf("extracting %s: %w", rawURL, err)
	}
	return Extraction{
		Title:        result.Metadata.Title,
		Body:         result.ContentText,
		LeadImageURL: result.Metadata.Image,
	}, nil
}
