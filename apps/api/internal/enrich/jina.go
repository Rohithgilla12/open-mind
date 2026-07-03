package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultJinaBaseURL is the public Jina Reader endpoint. The target URL is
// appended: GET https://r.jina.ai/<url>.
const defaultJinaBaseURL = "https://r.jina.ai"

// Jina extracts article content via the Jina Reader API (r.jina.ai). It works
// unauthenticated (rate-limited); an API key raises the limits via Bearer auth.
type Jina struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

// JinaOption configures a Jina extractor.
type JinaOption func(*Jina)

// WithJinaBaseURL overrides the Reader endpoint, primarily for testing.
func WithJinaBaseURL(baseURL string) JinaOption {
	return func(j *Jina) { j.baseURL = strings.TrimRight(baseURL, "/") }
}

// NewJina returns an Extractor backed by the Jina Reader API. A nil client
// falls back to an SSRF-safe client with a 30s timeout, which refuses to
// dial loopback, private, link-local, and other internal addresses. An
// empty apiKey omits auth.
func NewJina(client *http.Client, apiKey string, opts ...JinaOption) *Jina {
	if client == nil {
		client = SafeHTTPClient(30 * time.Second)
	}
	j := &Jina{client: client, apiKey: apiKey, baseURL: defaultJinaBaseURL}
	for _, opt := range opts {
		opt(j)
	}
	return j
}

func (*Jina) Name() string { return "jina" }

type jinaResponse struct {
	Data struct {
		Title   string            `json:"title"`
		Content string            `json:"content"`
		Images  map[string]string `json:"images"`
	} `json:"data"`
}

func (j *Jina) Extract(ctx context.Context, rawURL string) (Extraction, error) {
	endpoint := j.baseURL + "/" + rawURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Extraction{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if j.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.apiKey)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return Extraction{}, fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Extraction{}, fmt.Errorf("fetching %s: status %d", rawURL, resp.StatusCode)
	}
	var parsed jinaResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&parsed); err != nil {
		return Extraction{}, fmt.Errorf("decoding jina response for %s: %w", rawURL, err)
	}
	var lead string
	for _, img := range parsed.Data.Images {
		lead = img
		break
	}
	return Extraction{
		Title:        parsed.Data.Title,
		Body:         parsed.Data.Content,
		LeadImageURL: lead,
	}, nil
}
