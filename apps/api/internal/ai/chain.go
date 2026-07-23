package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// limiterWaitTimeout bounds how long a chain will block on a saturated rate
// limiter before treating the provider as unavailable and failing over. It is
// a var (not const) so tests can shrink it. See spec "Configuration".
var limiterWaitTimeout = 2 * time.Second

// ChainEntry describes one provider in a fallback chain, with an optional
// client-side requests-per-minute cap (RPM <= 0 means no proactive limit).
type ChainEntry struct {
	Name     string
	Provider Provider
	RPM      int
}

type chainEntry struct {
	name    string
	p       Provider
	limiter *rate.Limiter
}

// Chain is a Provider that tries an ordered list of providers per operation,
// failing over on retryable errors, unsupported operations, and rate-limiter
// saturation. See the design spec's "Chain semantics" section.
type Chain struct {
	entries []chainEntry
	name    string
}

// NewChain builds a fallback Chain from the given ordered entries. Entries with
// a positive RPM get a token-bucket limiter (rate RPM/60 per second, burst
// max(1, RPM/10)).
func NewChain(entries ...ChainEntry) *Chain {
	ce := make([]chainEntry, 0, len(entries))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		var lim *rate.Limiter
		if e.RPM > 0 {
			burst := e.RPM / 10
			if burst < 1 {
				burst = 1
			}
			lim = rate.NewLimiter(rate.Limit(float64(e.RPM)/60.0), burst)
		}
		ce = append(ce, chainEntry{name: e.Name, p: e.Provider, limiter: lim})
		names = append(names, e.Name)
	}
	return &Chain{entries: ce, name: fmt.Sprintf("chain(%s)", strings.Join(names, ","))}
}

// Name returns the composed chain name, e.g. chain(gemini,openai,noop).
func (c *Chain) Name() string { return c.name }

// runChain applies op to each provider in order until one succeeds, sharing the
// fallover logic across all Provider operations (DRY). ErrNotSupported is
// skipped silently; retryable/other errors are logged and cause fail-over; a
// saturated limiter is treated as a retryable fallover. When every provider
// reported ErrNotSupported the chain returns ErrNotSupported; otherwise it
// returns the last non-skip error.
func runChain[T any](ctx context.Context, c *Chain, op string, fn func(Provider) (T, error)) (T, error) {
	var zero T
	var lastErr error
	allNotSupported := true

	for _, e := range c.entries {
		if e.limiter != nil {
			wctx, cancel := context.WithTimeout(ctx, limiterWaitTimeout)
			err := e.limiter.Wait(wctx)
			cancel()
			if err != nil {
				allNotSupported = false
				lastErr = &RetryableError{Status: 429, Err: fmt.Errorf("rate limiter saturated: %w", err)}
				slog.Warn("ai chain: rate limiter saturated, failing over",
					"provider", e.name, "op", op, "err", err)
				continue
			}
		}

		res, err := fn(e.p)
		if err == nil {
			return res, nil
		}
		if errors.Is(err, ErrNotSupported) {
			continue
		}

		allNotSupported = false
		lastErr = err
		if Retryable(err) {
			slog.Warn("ai chain: retryable failure, failing over",
				"provider", e.name, "op", op, "err", err)
		} else {
			slog.Error("ai chain: provider error, failing over",
				"provider", e.name, "op", op, "err", err)
		}
	}

	if allNotSupported {
		return zero, ErrNotSupported
	}
	if lastErr == nil {
		lastErr = ErrNotSupported
	}
	return zero, lastErr
}

// Summarise tries each provider in order until one returns a summary.
func (c *Chain) Summarise(ctx context.Context, title, body string) (string, error) {
	return runChain(ctx, c, "summarise", func(p Provider) (string, error) {
		return p.Summarise(ctx, title, body)
	})
}

// Tag tries each provider in order until one returns tags.
func (c *Chain) Tag(ctx context.Context, title, body string) ([]string, error) {
	return runChain(ctx, c, "tag", func(p Provider) ([]string, error) {
		return p.Tag(ctx, title, body)
	})
}

// Embed tries each provider in order until one returns an embedding.
func (c *Chain) Embed(ctx context.Context, text string) ([]float32, error) {
	return runChain(ctx, c, "embed", func(p Provider) ([]float32, error) {
		return p.Embed(ctx, text)
	})
}

// ParseQuery tries each provider in order until one returns a parsed query.
func (c *Chain) ParseQuery(ctx context.Context, q string) (ParsedQuery, error) {
	return runChain(ctx, c, "parsequery", func(p Provider) (ParsedQuery, error) {
		return p.ParseQuery(ctx, q)
	})
}

// ExtractPlaces tries each provider in order until one returns places.
func (c *Chain) ExtractPlaces(ctx context.Context, title, caption string) ([]Place, error) {
	return runChain(ctx, c, "extractplaces", func(p Provider) ([]Place, error) {
		return p.ExtractPlaces(ctx, title, caption)
	})
}

// ExtractPlacesVision tries each provider in order until one returns places
// from a thumbnail (Gemini implements; text-only providers skip).
func (c *Chain) ExtractPlacesVision(ctx context.Context, title, caption string, image []byte) ([]Place, error) {
	return runChain(ctx, c, "extractplacesvision", func(p Provider) ([]Place, error) {
		return p.ExtractPlacesVision(ctx, title, caption, image)
	})
}

// ExtractPlacesVisionFrames tries each provider in order until one returns
// places from a batch of sampled video frames (Gemini implements; text-only
// providers skip).
func (c *Chain) ExtractPlacesVisionFrames(ctx context.Context, title, caption string, frames [][]byte) ([]Place, error) {
	return runChain(ctx, c, "extractplacesvisionframes", func(p Provider) ([]Place, error) {
		return p.ExtractPlacesVisionFrames(ctx, title, caption, frames)
	})
}
