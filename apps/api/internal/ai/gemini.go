package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

const (
	geminiGenModel   = "gemini-flash-lite-latest"
	geminiEmbedModel = "gemini-embedding-001"
)

// EmbedDims is the fixed dimensionality of embedding vectors produced across
// the app. The pgvector column and the query guard both depend on this value.
const EmbedDims = 768

// Gemini is a Provider backed by Google's Gemini API.
type Gemini struct{ client *genai.Client }

// NewGemini creates a Gemini provider using the given API key.
func NewGemini(ctx context.Context, apiKey string) (*Gemini, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}
	return &Gemini{client: client}, nil
}

// Name returns the provider name.
func (*Gemini) Name() string { return "gemini" }

// Summarise generates a short summary of the given title and body.
func (g *Gemini) Summarise(ctx context.Context, title, body string) (string, error) {
	prompt := fmt.Sprintf("Summarise this saved web page in 2-3 sentences for a personal knowledge library. Title: %s\n\n%s", title, truncate(body, 12000))
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, genai.Text(prompt), nil)
	if err != nil {
		return "", fmt.Errorf("gemini summarise: %w", classifyGeminiErr(err))
	}
	return resp.Text(), nil
}

// Tag generates topic tags for the given title and body.
func (g *Gemini) Tag(ctx context.Context, title, body string) ([]string, error) {
	prompt := fmt.Sprintf("Generate 3-6 short lowercase topic tags for this saved page. Title: %s\n\n%s", title, truncate(body, 12000))
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
	}
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, genai.Text(prompt), cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini tag: %w", classifyGeminiErr(err))
	}
	var tags []string
	if err := json.Unmarshal([]byte(resp.Text()), &tags); err != nil {
		return nil, fmt.Errorf("parsing gemini tags: %w", err)
	}
	return tags, nil
}

// Embed returns a 768-dimensional embedding for the given text.
func (g *Gemini) Embed(ctx context.Context, text string) ([]float32, error) {
	dims := int32(EmbedDims)
	resp, err := g.client.Models.EmbedContent(ctx, geminiEmbedModel, genai.Text(truncate(text, 8000)), &genai.EmbedContentConfig{OutputDimensionality: &dims})
	if err != nil {
		return nil, fmt.Errorf("gemini embed: %w", classifyGeminiErr(err))
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("gemini embed: empty response")
	}
	return resp.Embeddings[0].Values, nil
}

// ParseQuery is a passthrough; Gemini does not currently rewrite queries.
func (g *Gemini) ParseQuery(_ context.Context, q string) (string, error) { return q, nil }

// classifyGeminiErr inspects a genai SDK error and wraps it as a
// RetryableError when it represents a transient failure (429 or 5xx), so the
// fallback chain knows to fail over to the next provider. genai.APIError
// carries the real HTTP status in its Code field; when that type isn't
// present in the error chain, fall back to inspecting the error string for
// quota/rate-limit hints.
func classifyGeminiErr(err error) error {
	if err == nil {
		return err
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code == 429 || apiErr.Code >= 500 {
			return &RetryableError{Status: apiErr.Code, Err: err}
		}
		return err
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "429") || strings.Contains(msg, "quota") || strings.Contains(msg, "rate") {
		return &RetryableError{Status: 429, Err: err}
	}

	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
