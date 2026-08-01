package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

// httpDetectImageMIME sniffs a content type from image bytes for multimodal
// parts. Falls back to image/jpeg when the sniffer returns a non-image type
// (Gemini rejects unknown MIME types; jpeg is the common reel thumbnail).
func httpDetectImageMIME(data []byte) string {
	mime := http.DetectContentType(data)
	mime = strings.SplitN(mime, ";", 2)[0]
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return mime
	default:
		return "image/jpeg"
	}
}

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

// ParseQuery interprets a natural-language query, splitting it into a text
// portion, an optional colour, card-type filters, and domain filters via a
// JSON-mode call.
func (g *Gemini) ParseQuery(ctx context.Context, q string) (ParsedQuery, error) {
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"text":    {Type: genai.TypeString},
				"color":   {Type: genai.TypeString},
				"types":   {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
				"domains": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
			},
		},
	}
	prompt := fmt.Sprintf("%s\n\nQuery: %s", parseQueryInstruction, truncate(q, 2000))
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, genai.Text(prompt), cfg)
	if err != nil {
		return ParsedQuery{}, fmt.Errorf("gemini parsequery: %w", classifyGeminiErr(err))
	}
	var parsed struct {
		Text    string   `json:"text"`
		Color   string   `json:"color"`
		Types   []string `json:"types"`
		Domains []string `json:"domains"`
	}
	if err := json.Unmarshal([]byte(resp.Text()), &parsed); err != nil {
		return ParsedQuery{}, fmt.Errorf("parsing gemini query: %w", err)
	}
	return ParsedQuery{
		Text:    strings.TrimSpace(parsed.Text),
		Color:   strings.TrimSpace(parsed.Color),
		Types:   sanitiseTypes(parsed.Types),
		Domains: sanitiseDomains(parsed.Domains),
	}, nil
}

// placesResponseSchema is the shared JSON schema for caption and vision place
// extraction so both calls answer in the same shape.
func placesResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"places": {Type: genai.TypeArray, Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name":       {Type: genai.TypeString},
					"hint":       {Type: genai.TypeString},
					"confidence": {Type: genai.TypeNumber},
				},
			}},
		},
	}
}

// ExtractPlaces pulls the visitable places named in a video caption via a
// JSON-mode call with a response schema, so the model can only answer in the
// expected shape.
func (g *Gemini) ExtractPlaces(ctx context.Context, title, caption string) ([]Place, error) {
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   placesResponseSchema(),
	}
	prompt := fmt.Sprintf("%s\n\nTitle: %s\nCaption: %s", extractPlacesInstruction, truncate(title, 500), truncate(caption, 8000))
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, genai.Text(prompt), cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini extractplaces: %w", classifyGeminiErr(err))
	}
	var parsed placesJSON
	if err := json.Unmarshal([]byte(resp.Text()), &parsed); err != nil {
		return nil, fmt.Errorf("parsing gemini places: %w", err)
	}
	return placesFromJSON(parsed), nil
}

// ExtractPlacesVision reads place names from a video thumbnail via a
// multimodal JSON-mode call (image bytes + optional caption context).
func (g *Gemini) ExtractPlacesVision(ctx context.Context, title, caption string, image []byte) ([]Place, error) {
	if len(image) == 0 {
		return nil, nil
	}
	mime := httpDetectImageMIME(image)
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   placesResponseSchema(),
	}
	prompt := fmt.Sprintf("%s\n\nTitle: %s\nCaption: %s", extractPlacesVisionInstruction, truncate(title, 500), truncate(caption, 4000))
	contents := []*genai.Content{
		genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromText(prompt),
			genai.NewPartFromBytes(image, mime),
		}, genai.RoleUser),
	}
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini extractplacesvision: %w", classifyGeminiErr(err))
	}
	var parsed placesJSON
	if err := json.Unmarshal([]byte(resp.Text()), &parsed); err != nil {
		return nil, fmt.Errorf("parsing gemini vision places: %w", err)
	}
	return placesFromJSON(parsed), nil
}

// ExtractPlacesVisionFrames reads place names from several sampled video
// frames via a single batched multimodal JSON-mode call (one image part per
// frame, plus optional caption context).
func (g *Gemini) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	prompt := fmt.Sprintf("%s\n\nTitle: %s\nCaption: %s", extractPlacesFramesInstruction, truncate(title, 500), truncate(caption, 4000))
	parts := []*genai.Part{genai.NewPartFromText(prompt)}
	for _, f := range frames {
		if len(f) == 0 {
			continue
		}
		parts = append(parts, genai.NewPartFromBytes(f, httpDetectImageMIME(f)))
	}
	if len(parts) == 1 { // no usable frames
		return nil, nil
	}
	cfg := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   placesResponseSchema(),
	}
	contents := []*genai.Content{genai.NewContentFromParts(parts, genai.RoleUser)}
	resp, err := g.client.Models.GenerateContent(ctx, geminiGenModel, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini extractplacesvisionframes: %w", classifyGeminiErr(err))
	}
	var parsed placesJSON
	if err := json.Unmarshal([]byte(resp.Text()), &parsed); err != nil {
		return nil, fmt.Errorf("parsing gemini frames places: %w", err)
	}
	return placesFromJSON(parsed), nil
}

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
	if strings.Contains(msg, "429") || strings.Contains(msg, "quota") || strings.Contains(msg, "rate limit") {
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
