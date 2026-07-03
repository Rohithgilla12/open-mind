package assets_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rohithgilla12/openmind/api/internal/assets"
)

func TestPutOpenRoundTrip(t *testing.T) {
	s, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := uuid.New()
	want := []byte("the quick brown fox")
	n, err := s.Put(id, bytes.NewReader(want), 1<<20)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if n != int64(len(want)) {
		t.Errorf("wrote %d bytes, want %d", n, len(want))
	}

	rc, err := s.Open(id)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round-trip = %q, want %q", got, want)
	}
}

func TestOpenMissing(t *testing.T) {
	s, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := s.Open(uuid.New()); err == nil {
		t.Error("Open of missing asset succeeded; want error")
	}
}

func TestPutRespectsCap(t *testing.T) {
	s, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := uuid.New()
	// 100-byte stream against a 10-byte cap.
	_, err = s.Put(id, strings.NewReader(strings.Repeat("a", 100)), 10)
	if !errors.Is(err, assets.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
	// Oversize put must not leave a file behind.
	if _, err := s.Open(id); err == nil {
		t.Error("oversize Put left a file behind")
	}
}

func TestPutAtCap(t *testing.T) {
	s, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	id := uuid.New()
	if _, err := s.Put(id, strings.NewReader(strings.Repeat("a", 10)), 10); err != nil {
		t.Errorf("exactly-at-cap Put failed: %v", err)
	}
}
