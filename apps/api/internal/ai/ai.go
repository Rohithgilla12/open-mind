// Package ai defines the pluggable AI provider adapter used by the
// enrichment pipeline and search query parsing.
package ai

import (
	"context"
	"errors"
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
// Unknown or unimplemented values (including "gemini", pending a future
// task) fall back to the noop provider with a warning logged.
func FromEnv() Provider {
	switch provider := os.Getenv("AI_PROVIDER"); provider {
	case "", "noop":
		return NewNoop()
	default:
		slog.Warn("ai: unknown or unimplemented AI_PROVIDER, falling back to noop", "provider", provider)
		return NewNoop()
	}
}
