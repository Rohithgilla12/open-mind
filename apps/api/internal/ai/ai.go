// Package ai defines the pluggable AI provider adapter used by the
// enrichment pipeline and search query parsing.
package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// Enrichment holds the AI-generated summary and tags for a saved item.
type Enrichment struct {
	Summary string
	Tags    []string
}

// Provider is the adapter interface every AI backend must implement.
// The noop provider keeps the app fully functional without any AI backend
// configured.
type Provider interface {
	Name() string
	Summarise(ctx context.Context, title, body string) (string, error)
	Tag(ctx context.Context, title, body string) ([]string, error)
	Embed(ctx context.Context, text string) ([]float32, error)
	ParseQuery(ctx context.Context, q string) (string, error)
}

// ErrNotSupported is returned by providers that don't implement a given
// operation, e.g. Embed on the noop provider.
var ErrNotSupported = errors.New("ai: operation not supported by provider")

// FromEnv builds a Provider based on the AI_PROVIDER environment variable.
// "gemini" requires GEMINI_API_KEY and returns an error if it is unset.
// Unknown values fall back to the noop provider with a warning logged.
// It takes a context because building some providers (e.g. gemini) makes
// a network round-trip during client construction.
func FromEnv(ctx context.Context) (Provider, error) {
	switch provider := os.Getenv("AI_PROVIDER"); provider {
	case "", "noop":
		return NewNoop(), nil
	case "gemini":
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ai: AI_PROVIDER=gemini requires GEMINI_API_KEY")
		}
		p, err := NewGemini(ctx, apiKey)
		if err != nil {
			return nil, fmt.Errorf("building gemini provider: %w", err)
		}
		return p, nil
	default:
		slog.Warn("ai: unknown AI_PROVIDER, falling back to noop", "provider", provider)
		return NewNoop(), nil
	}
}
