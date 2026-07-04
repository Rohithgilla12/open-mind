package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI is a Provider backed by any OpenAI-compatible chat/completions and
// embeddings API (OpenAI, DeepSeek, Groq, Cerebras, Together, Ollama's /v1
// endpoint). It uses the standard library HTTP client only — no SDK.
type OpenAI struct {
	baseURL    string
	apiKey     string
	model      string
	embedModel string
	client     *http.Client
}

// OpenAIOption configures an OpenAI provider.
type OpenAIOption func(*OpenAI)

// WithHTTPClient overrides the HTTP client, primarily for tests.
func WithHTTPClient(c *http.Client) OpenAIOption {
	return func(o *OpenAI) { o.client = c }
}

// NewOpenAI creates an OpenAI-compatible provider. baseURL is the API root
// (e.g. https://api.openai.com/v1); model is used for chat/completions;
// embedModel is used for embeddings — an empty embedModel makes Embed return
// ErrNotSupported.
func NewOpenAI(baseURL, apiKey, model, embedModel string, opts ...OpenAIOption) *OpenAI {
	o := &OpenAI{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		embedModel: embedModel,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Name returns the provider name.
func (*OpenAI) Name() string { return "openai" }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Summarise generates a short summary via chat/completions.
func (o *OpenAI) Summarise(ctx context.Context, title, body string) (string, error) {
	req := chatRequest{
		Model:       o.model,
		Temperature: 0.3,
		Messages: []chatMessage{
			{Role: "system", Content: "You summarise saved web pages in 2-3 sentences for a personal knowledge library. Reply with only the summary prose."},
			{Role: "user", Content: fmt.Sprintf("Title: %s\n\n%s", title, truncate(body, 12000))},
		},
	}
	var resp chatResponse
	if err := o.doJSON(ctx, "/chat/completions", req, &resp); err != nil {
		return "", fmt.Errorf("openai summarise: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai summarise: empty response")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// Tag generates 3-6 lowercase topic tags via chat/completions JSON mode.
func (o *OpenAI) Tag(ctx context.Context, title, body string) ([]string, error) {
	req := chatRequest{
		Model:          o.model,
		Temperature:    0.3,
		ResponseFormat: &responseFormat{Type: "json_object"},
		Messages: []chatMessage{
			{Role: "system", Content: `You generate 3-6 short lowercase topic tags for a saved web page. Respond with a JSON object of the form {"tags": ["tag1", "tag2"]} and nothing else.`},
			{Role: "user", Content: fmt.Sprintf("Title: %s\n\n%s", title, truncate(body, 12000))},
		},
	}
	var resp chatResponse
	if err := o.doJSON(ctx, "/chat/completions", req, &resp); err != nil {
		return nil, fmt.Errorf("openai tag: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai tag: empty response")
	}
	var parsed struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &parsed); err != nil {
		return nil, fmt.Errorf("parsing openai tags: %w", err)
	}
	tags := make([]string, 0, len(parsed.Tags))
	for _, t := range parsed.Tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

type embedRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions"`
}

type embedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns a 768-dimensional embedding via the embeddings endpoint.
// Returns ErrNotSupported when no embedding model is configured.
func (o *OpenAI) Embed(ctx context.Context, text string) ([]float32, error) {
	if o.embedModel == "" {
		return nil, ErrNotSupported
	}
	req := embedRequest{Model: o.embedModel, Input: truncate(text, 8000), Dimensions: EmbedDims}
	var resp embedResponse
	if err := o.doJSON(ctx, "/embeddings", req, &resp); err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai embed: empty response")
	}
	vec := resp.Data[0].Embedding
	if len(vec) != EmbedDims {
		return nil, fmt.Errorf("openai embed: expected %d dimensions, got %d", EmbedDims, len(vec))
	}
	return vec, nil
}

// ParseQuery interprets a natural-language query, splitting it into a text
// portion, an optional colour, and card-type filters via chat/completions JSON
// mode.
func (o *OpenAI) ParseQuery(ctx context.Context, q string) (ParsedQuery, error) {
	req := chatRequest{
		Model:          o.model,
		Temperature:    0,
		ResponseFormat: &responseFormat{Type: "json_object"},
		Messages: []chatMessage{
			{Role: "system", Content: parseQueryInstruction},
			{Role: "user", Content: truncate(q, 2000)},
		},
	}
	var resp chatResponse
	if err := o.doJSON(ctx, "/chat/completions", req, &resp); err != nil {
		return ParsedQuery{}, fmt.Errorf("openai parsequery: %w", err)
	}
	if len(resp.Choices) == 0 {
		return ParsedQuery{}, fmt.Errorf("openai parsequery: empty response")
	}
	var parsed struct {
		Text  string   `json:"text"`
		Color string   `json:"color"`
		Types []string `json:"types"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &parsed); err != nil {
		return ParsedQuery{}, fmt.Errorf("parsing openai query: %w", err)
	}
	return ParsedQuery{
		Text:  strings.TrimSpace(parsed.Text),
		Color: strings.TrimSpace(parsed.Color),
		Types: sanitiseTypes(parsed.Types),
	}, nil
}

// doJSON performs a POST with a JSON body and decodes a JSON response,
// classifying HTTP errors as RetryableError so the chain can fail over.
func (o *OpenAI) doJSON(ctx context.Context, path string, reqBody, out any) error {
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return &RetryableError{Status: resp.StatusCode, Err: fmt.Errorf("openai api: %s", strings.TrimSpace(string(body)))}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
