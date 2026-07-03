// Package assets implements a filesystem blob store for uploaded images.
// Blobs live on a mounted volume; metadata lives in the assets table. One file
// per asset UUID, so the store can never be coerced into path traversal.
package assets

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// ErrTooLarge is returned by Put when the reader exceeds the byte cap.
var ErrTooLarge = errors.New("asset exceeds size limit")

// FSStore stores blobs on disk under a single directory, one file per asset
// UUID. Filenames are derived solely from the parsed asset UUID — never from
// user input — so a malicious filename cannot escape the directory.
type FSStore struct {
	dir string
}

// NewFSStore creates the store directory (mkdir -p) and returns the store.
func NewFSStore(dir string) (*FSStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating assets dir %q: %w", dir, err)
	}
	return &FSStore{dir: dir}, nil
}

// path builds the on-disk path for an asset. id is a parsed UUID whose String()
// form is only hex and hyphens — no separators — so filepath.Join cannot be
// tricked into leaving dir.
func (s *FSStore) path(id uuid.UUID) string {
	return filepath.Join(s.dir, id.String())
}

// Put writes r to dir/<id>, enforcing a hard byte cap. It returns the number of
// bytes written. If the stream exceeds max the partial file is removed and
// ErrTooLarge is returned. This cap is defence in depth: the HTTP handler also
// wraps the body in http.MaxBytesReader.
func (s *FSStore) Put(id uuid.UUID, r io.Reader, max int64) (int64, error) {
	p := s.path(id)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, fmt.Errorf("creating asset file: %w", err)
	}
	// Read one byte past the cap so an exactly-at-cap file passes but any
	// overflow is detected.
	n, copyErr := io.Copy(f, io.LimitReader(r, max+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(p)
		return 0, fmt.Errorf("writing asset: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(p)
		return 0, fmt.Errorf("closing asset: %w", closeErr)
	}
	if n > max {
		_ = os.Remove(p)
		return 0, ErrTooLarge
	}
	return n, nil
}

// Open returns a reader for the stored asset. The caller must Close it.
func (s *FSStore) Open(id uuid.UUID) (io.ReadCloser, error) {
	f, err := os.Open(s.path(id))
	if err != nil {
		return nil, fmt.Errorf("opening asset: %w", err)
	}
	return f, nil
}
