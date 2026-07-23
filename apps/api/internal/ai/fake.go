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

// ExtractPlaces returns a stable two-place list so job tests can assert
// idempotency, and an empty list for an empty caption so no-signal paths are
// testable too. Confidences are intentionally below the Fake vision fixture
// for "Fake Cafe" so merge tests can assert vision wins on overlap.
func (*Fake) ExtractPlaces(_ context.Context, _, caption string) ([]Place, error) {
	if caption == "" {
		return nil, nil
	}
	return []Place{
		{Name: "Fake Cafe", Hint: "Faketown", Confidence: 0.6},
		{Name: "Fake Museum", Hint: "", Confidence: 0.7},
	}, nil
}

// ExtractPlacesVision returns a deterministic vision fixture when image bytes
// are present: an overlapping cafe (higher confidence than caption) plus a
// vision-only landmark. Empty image → no places (mirrors "no thumbnail").
func (*Fake) ExtractPlacesVision(_ context.Context, _, _ string, image []byte) ([]Place, error) {
	if len(image) == 0 {
		return nil, nil
	}
	return []Place{
		{Name: "Fake Cafe", Hint: "Faketown", Confidence: 0.95},
		{Name: "Vision Landmark", Hint: "Faketown", Confidence: 0.8},
	}, nil
}

// ExtractPlacesVisionFrames returns a deterministic fixture when at least one
// frame is present, mirroring ExtractPlacesVision's shape for the batched,
// multi-frame call. No frames → no places (mirrors "no usable frames").
func (*Fake) ExtractPlacesVisionFrames(_ context.Context, _, _ string, frames [][]byte) ([]Place, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	return []Place{
		{Name: "Frame Diner", Hint: "Faketown", Confidence: 0.8},
		{Name: "Vision Landmark", Hint: "Faketown", Confidence: 0.7},
	}, nil
}
