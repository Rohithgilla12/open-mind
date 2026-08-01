package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/ai"
)

func TestNoopProvider(t *testing.T) {
	p := ai.NewNoop()
	ctx := context.Background()
	if s, err := p.Summarise(ctx, "T", "B"); err != nil || s != "" {
		t.Errorf("Summarise = (%q, %v), want empty, nil", s, err)
	}
	if tags, err := p.Tag(ctx, "T", "B"); err != nil || len(tags) != 0 {
		t.Errorf("Tag = (%v, %v), want empty, nil", tags, err)
	}
	if _, err := p.Embed(ctx, "x"); !errors.Is(err, ai.ErrNotSupported) {
		t.Errorf("Embed err = %v, want ErrNotSupported", err)
	}
	if q, err := p.ParseQuery(ctx, "red poster"); err != nil || q.Text != "red poster" || q.Color != "" || len(q.Types) != 0 || len(q.Domains) != 0 {
		t.Errorf("ParseQuery = (%+v, %v), want text-only passthrough", q, err)
	}
	if _, err := p.ExtractPlaces(ctx, "t", "c"); !errors.Is(err, ai.ErrNotSupported) {
		t.Errorf("ExtractPlaces err = %v, want ErrNotSupported", err)
	}
	if _, err := p.ExtractPlacesVision(ctx, "t", "c", []byte{1}); !errors.Is(err, ai.ErrNotSupported) {
		t.Errorf("ExtractPlacesVision err = %v, want ErrNotSupported", err)
	}
}
