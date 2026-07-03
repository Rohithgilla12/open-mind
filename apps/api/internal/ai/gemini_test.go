package ai_test

import (
	"context"
	"os"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/ai"
)

func TestGeminiProvider(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	ctx := context.Background()
	p, err := ai.NewGemini(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := p.Summarise(ctx, "Go", "Go is a statically typed compiled language designed at Google.")
	if err != nil || sum == "" {
		t.Errorf("Summarise = (%q, %v)", sum, err)
	}
	tags, err := p.Tag(ctx, "Go", "Go is a statically typed compiled language designed at Google.")
	if err != nil || len(tags) == 0 {
		t.Errorf("Tag = (%v, %v)", tags, err)
	}
	vec, err := p.Embed(ctx, "hello world")
	if err != nil || len(vec) != 768 {
		t.Errorf("Embed len = %d, err = %v; want 768", len(vec), err)
	}
}
