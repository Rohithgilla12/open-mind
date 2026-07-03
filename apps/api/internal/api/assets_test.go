package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/store"
)

// newSrvWithAssets builds a Server plus the asset dir it writes to, so tests can
// assert the blob landed on disk.
func newSrvWithAssets(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], maxBytes int64) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	as, err := assets.NewFSStore(dir)
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	return api.NewServer(s, rc, ai.NewNoop(), "", as, maxBytes), dir
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// multipartUpload builds a multipart/form-data body with a single "file" field.
func multipartUpload(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &body, mw.FormDataContentType()
}

func postUpload(t *testing.T, url, filename string, content []byte) *http.Response {
	t.Helper()
	body, ctype := multipartUpload(t, filename, content)
	resp, err := http.Post(url, ctype, body)
	if err != nil {
		t.Fatalf("post upload: %v", err)
	}
	return resp
}

func TestCreateAssetHappyPath(t *testing.T) {
	s, rc, pool := testDeps(t)
	h, dir := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := postUpload(t, srv.URL+"/assets", "my-photo.png", pngBytes(t))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var item map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["cardType"] != "image" {
		t.Errorf("cardType = %v, want image", item["cardType"])
	}
	if item["title"] != "my-photo" {
		t.Errorf("title = %v, want my-photo", item["title"])
	}
	lead, _ := item["leadImageUrl"].(string)
	if !strings.HasPrefix(lead, "/assets/") {
		t.Errorf("leadImageUrl = %q, want /assets/ prefix", lead)
	}
	assetID := strings.TrimPrefix(lead, "/assets/")

	// Asset row exists, scoped to the dev user, with png content-type.
	var (
		ct   string
		size int64
	)
	if err := pool.QueryRow(context.Background(),
		`SELECT content_type, byte_size FROM assets WHERE id = $1 AND user_id = $2`,
		assetID, api.DevUserID).Scan(&ct, &size); err != nil {
		t.Fatalf("asset row: %v", err)
	}
	if ct != "image/png" {
		t.Errorf("content_type = %q, want image/png", ct)
	}
	if size == 0 {
		t.Errorf("byte_size = 0, want > 0")
	}

	// Blob is on disk under the asset UUID.
	if _, err := os.Stat(filepath.Join(dir, assetID)); err != nil {
		t.Errorf("blob not on disk: %v", err)
	}

	// Enrichment enqueued.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&count); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("enrich_item jobs = %d, want 1", count)
	}
}

func TestCreateAssetOversize413(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	big := make([]byte, 11<<20)
	// Give it a valid PNG header so it isn't rejected as a type first.
	copy(big, pngBytes(t))
	resp := postUpload(t, srv.URL+"/assets", "big.png", big)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
}

func TestCreateAssetRejectsNonImage(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cases := map[string][]byte{
		"text/plain": []byte("just some plain text, definitely not an image at all"),
		"svg":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			resp := postUpload(t, srv.URL+"/assets", name+".dat", content)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnsupportedMediaType {
				t.Errorf("status = %d, want 415", resp.StatusCode)
			}
		})
	}
}

// jpegWithExif encodes a 2x2 JPEG and injects an APP1 EXIF segment right after
// the SOI marker, simulating a photo carrying EXIF.
func jpegWithExif(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	base := buf.Bytes()
	payload := append([]byte("Exif\x00\x00"), 0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00)
	l := len(payload) + 2
	seg := append([]byte{0xFF, 0xE1, byte(l >> 8), byte(l)}, payload...)
	out := make([]byte, 0, len(base)+len(seg))
	out = append(out, base[:2]...)
	out = append(out, seg...)
	out = append(out, base[2:]...)
	return out
}

func TestCreateAssetRejectsAVIF(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Minimal ISOBMFF ftyp box with an avif brand.
	avif := []byte{0, 0, 0, 0x14, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f', 0, 0, 0, 0, 'm', 'i', 'f', '1'}
	resp := postUpload(t, srv.URL+"/assets", "photo.avif", avif)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", resp.StatusCode)
	}
}

func TestCreateAssetStripsExif(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	up := postUpload(t, srv.URL+"/assets", "photo.jpg", jpegWithExif(t))
	if up.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d, want 201", up.StatusCode)
	}
	var item map[string]any
	if err := json.NewDecoder(up.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	up.Body.Close()
	lead := item["leadImageUrl"].(string)

	got, err := http.Get(srv.URL + lead)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer got.Body.Close()
	data, _ := io.ReadAll(got.Body)
	if bytes.Contains(data, []byte("Exif\x00\x00")) {
		t.Errorf("served bytes still contain EXIF marker")
	}
	if bytes.Contains(data, []byte{0xFF, 0xE1}) {
		t.Errorf("served bytes still contain APP1 marker")
	}
}

func TestGetAssetStreamsWithHeaders(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	want := pngBytes(t)
	up := postUpload(t, srv.URL+"/assets", "pic.png", want)
	var item map[string]any
	if err := json.NewDecoder(up.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	up.Body.Close()
	lead := item["leadImageUrl"].(string)

	got, err := http.Get(srv.URL + lead)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.StatusCode)
	}
	if ct := got.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if nosniff := got.Header.Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", nosniff)
	}
	data, _ := io.ReadAll(got.Body)
	if !bytes.Equal(data, want) {
		t.Errorf("served bytes differ from uploaded (%d vs %d)", len(data), len(want))
	}
}

func TestGetAssetCrossTenant404(t *testing.T) {
	s, rc, pl := testDeps(t)
	h, dir := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// Seed an asset owned by another user, with a real file on disk.
	other := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	ctx := context.Background()
	if err := s.Queries.EnsureUser(ctx, other); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	var assetID uuid.UUID
	if err := pl.QueryRow(ctx,
		`INSERT INTO assets (user_id, content_type, byte_size) VALUES ($1, 'image/png', 3) RETURNING id`,
		other).Scan(&assetID); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, assetID.String()), []byte("png"), 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	resp, err := http.Get(srv.URL + "/assets/" + assetID.String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", resp.StatusCode)
	}
}

func TestGetAssetBadUUID(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/assets/not-a-uuid")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		t.Errorf("bad-uuid status = %d, want 400 or 404", resp.StatusCode)
	}

	// A well-formed but unknown UUID → 404.
	resp2, err := http.Get(srv.URL + "/assets/" + uuid.New().String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("unknown uuid status = %d, want 404", resp2.StatusCode)
	}
}
