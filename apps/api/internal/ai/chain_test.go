package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stub is a scripted Provider for chain tests. Each op returns the configured
// value/error and counts its invocations.
type stub struct {
	name    string
	summary string
	tags    []string
	vec     []float32
	query   ParsedQuery
	places  []Place
	err     error
	calls   int
}

func (s *stub) Name() string { return s.name }

func (s *stub) Summarise(context.Context, string, string) (string, error) {
	s.calls++
	return s.summary, s.err
}

func (s *stub) Tag(context.Context, string, string) ([]string, error) {
	s.calls++
	return s.tags, s.err
}

func (s *stub) Embed(context.Context, string) ([]float32, error) {
	s.calls++
	return s.vec, s.err
}

func (s *stub) ParseQuery(context.Context, string) (ParsedQuery, error) {
	s.calls++
	return s.query, s.err
}

func (s *stub) ExtractPlaces(context.Context, string, string) ([]Place, error) {
	s.calls++
	return s.places, s.err
}

func (s *stub) ExtractPlacesVision(context.Context, string, string, []byte) ([]Place, error) {
	s.calls++
	return s.places, s.err
}

func (s *stub) ExtractPlacesVisionFrames(context.Context, string, string, [][]byte) ([]Place, error) {
	s.calls++
	return s.places, s.err
}

func TestChainFalloverOnRetryable(t *testing.T) {
	p1 := &stub{name: "p1", err: &RetryableError{Status: 429}}
	p2 := &stub{name: "p2", summary: "ok"}
	c := NewChain(
		ChainEntry{Name: "p1", Provider: p1},
		ChainEntry{Name: "p2", Provider: p2},
	)

	got, err := c.Summarise(context.Background(), "t", "b")
	if err != nil {
		t.Fatalf("Summarise err = %v, want nil", err)
	}
	if got != "ok" {
		t.Fatalf("Summarise = %q, want %q", got, "ok")
	}
	if p1.calls != 1 || p2.calls != 1 {
		t.Fatalf("calls: p1=%d p2=%d, want 1,1", p1.calls, p2.calls)
	}
}

func TestChainSkipOnNotSupported(t *testing.T) {
	p1 := &stub{name: "p1", err: ErrNotSupported}
	p2 := &stub{name: "p2", vec: make([]float32, EmbedDims)}
	c := NewChain(
		ChainEntry{Name: "p1", Provider: p1},
		ChainEntry{Name: "p2", Provider: p2},
	)

	vec, err := c.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed err = %v, want nil", err)
	}
	if len(vec) != EmbedDims {
		t.Fatalf("Embed len = %d, want %d", len(vec), EmbedDims)
	}
}

func TestChainFirstSuccessWins(t *testing.T) {
	p1 := &stub{name: "p1", summary: "first"}
	p2 := &stub{name: "p2", summary: "second"}
	c := NewChain(
		ChainEntry{Name: "p1", Provider: p1},
		ChainEntry{Name: "p2", Provider: p2},
	)

	got, err := c.Summarise(context.Background(), "t", "b")
	if err != nil {
		t.Fatalf("Summarise err = %v", err)
	}
	if got != "first" {
		t.Fatalf("Summarise = %q, want %q", got, "first")
	}
	if p2.calls != 0 {
		t.Fatalf("p2.calls = %d, want 0 (should not be reached)", p2.calls)
	}
}

func TestChainExhaustionReturnsLastError(t *testing.T) {
	sentinel := &RetryableError{Status: 503}
	p1 := &stub{name: "p1", err: &RetryableError{Status: 429}}
	p2 := &stub{name: "p2", err: sentinel}
	c := NewChain(
		ChainEntry{Name: "p1", Provider: p1},
		ChainEntry{Name: "p2", Provider: p2},
	)

	_, err := c.Summarise(context.Background(), "t", "b")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Summarise err = %v, want last error %v", err, sentinel)
	}
}

func TestChainAllNotSupportedReturnsNotSupported(t *testing.T) {
	p1 := &stub{name: "p1", err: ErrNotSupported}
	p2 := &stub{name: "p2", err: ErrNotSupported}
	c := NewChain(
		ChainEntry{Name: "p1", Provider: p1},
		ChainEntry{Name: "p2", Provider: p2},
	)

	_, err := c.Embed(context.Background(), "text")
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("Embed err = %v, want ErrNotSupported", err)
	}
}

func TestChainLimiterSaturationFallsOver(t *testing.T) {
	orig := limiterWaitTimeout
	limiterWaitTimeout = 50 * time.Millisecond
	t.Cleanup(func() { limiterWaitTimeout = orig })

	p1 := &stub{name: "p1", summary: "primary"}
	p2 := &stub{name: "p2", summary: "backup"}
	// RPM=1 → burst 1: first call consumes the token, second saturates.
	c := NewChain(
		ChainEntry{Name: "p1", Provider: p1, RPM: 1},
		ChainEntry{Name: "p2", Provider: p2},
	)

	got1, err := c.Summarise(context.Background(), "t", "b")
	if err != nil || got1 != "primary" {
		t.Fatalf("first call = %q, %v; want primary, nil", got1, err)
	}

	got2, err := c.Summarise(context.Background(), "t", "b")
	if err != nil {
		t.Fatalf("second call err = %v, want nil (should fall over)", err)
	}
	if got2 != "backup" {
		t.Fatalf("second call = %q, want backup (limiter should fall over)", got2)
	}
	if p1.calls != 1 {
		t.Fatalf("p1.calls = %d, want 1 (second call must skip saturated provider)", p1.calls)
	}
}

func TestChainExtractPlacesVisionFramesFallsOverToFake(t *testing.T) {
	p1 := NewNoop()
	p2 := NewFake()
	c := NewChain(
		ChainEntry{Name: "noop", Provider: p1},
		ChainEntry{Name: "fake", Provider: p2},
	)

	got, err := c.ExtractPlacesVisionFrames(context.Background(), "t", "c", [][]byte{{0x1}})
	if err != nil {
		t.Fatalf("ExtractPlacesVisionFrames err = %v, want nil", err)
	}
	if len(got) == 0 {
		t.Fatal("want fixture places from fake after noop skips, got none")
	}
}

func TestChainName(t *testing.T) {
	c := NewChain(
		ChainEntry{Name: "gemini", Provider: &stub{name: "gemini"}},
		ChainEntry{Name: "openai", Provider: &stub{name: "openai"}},
		ChainEntry{Name: "noop", Provider: &stub{name: "noop"}},
	)
	if got := c.Name(); got != "chain(gemini,openai,noop)" {
		t.Fatalf("Name() = %q, want chain(gemini,openai,noop)", got)
	}
}
