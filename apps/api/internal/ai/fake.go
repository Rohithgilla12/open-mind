package ai

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// Fake is a deterministic Provider for tests. It performs no network calls
// and produces stable output for a given input, which lets pipeline tests
// assert idempotency.
type Fake struct{}

// NewFake returns a deterministic test Provider.
func NewFake() *Fake { return &Fake{} }

func (*Fake) Name() string { return "fake" }

func (*Fake) Summarise(_ context.Context, title, _ string) (string, error) {
	return "summary of " + title, nil
}

func (*Fake) Tag(context.Context, string, string) ([]string, error) {
	return []string{"fake", "tags"}, nil
}

// Embed returns a deterministic 768-dimension vector derived from a SHA-256
// hash of the input text, so the same text always yields the same vector.
func (*Fake) Embed(_ context.Context, text string) ([]float32, error) {
	sum := sha256.Sum256([]byte(text))
	vec := make([]float32, 768)
	for i := range vec {
		// Reuse the digest cyclically; combine byte and index for spread.
		b := sum[i%len(sum)]
		vec[i] = float32(binary.BigEndian.Uint16([]byte{b, sum[(i+1)%len(sum)]})) / 65535.0
	}
	return vec, nil
}

func (*Fake) ParseQuery(_ context.Context, q string) (ParsedQuery, error) {
	return ParsedQuery{Text: q}, nil
}
