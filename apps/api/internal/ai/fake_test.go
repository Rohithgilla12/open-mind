package ai

import (
	"context"
	"testing"
)

func TestFakeExtractPlacesVisionFrames(t *testing.T) {
	f := NewFake()
	if got, err := f.ExtractPlacesVisionFrames(context.Background(), "t", "c", nil); err != nil || got != nil {
		t.Fatalf("empty frames: got (%v, %v), want (nil, nil)", got, err)
	}
	got, err := f.ExtractPlacesVisionFrames(context.Background(), "t", "c", [][]byte{{0x1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want fixture places for non-empty frames, got none")
	}
}
