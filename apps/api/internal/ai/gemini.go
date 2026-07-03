package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"
)

const (
	geminiGenModel   = "gemini-flash-lite-latest"
	geminiEmbedModel = "gemini-embedding-001"
	embedDims        = int32(768)
)

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
		return "", fmt.Errorf("gemini summarise: %w", err)
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
		return nil, fmt.Errorf("gemini tag: %w", err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(resp.Text()), &tags); err != nil {
		return nil, fmt.Errorf("parsing gemini tags: %w", err)
	}
	return tags, nil
}

// Embed returns a 768-dimensional embedding for the given text.
func (g *Gemini) Embed(ctx context.Context, text string) ([]float32, error) {
	dims := embedDims
	resp, err := g.client.Models.EmbedContent(ctx, geminiEmbedModel, genai.Text(truncate(text, 8000)), &genai.EmbedContentConfig{OutputDimensionality: &dims})
	if err != nil {
		return nil, fmt.Errorf("gemini embed: %w", err)
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("gemini embed: empty response")
	}
	return resp.Embeddings[0].Values, nil
}

// ParseQuery is a passthrough; Gemini does not currently rewrite queries.
func (g *Gemini) ParseQuery(_ context.Context, q string) (string, error) { return q, nil }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
