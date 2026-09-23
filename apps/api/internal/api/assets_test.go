package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
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
	"github.com/rohithgilla12/openmind/api/internal/feeds"
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
	feedSvc := feeds.NewService(s)
	feedSvc.River = rc
	return api.NewServer(s, rc, ai.NewNoop(), api.AuthConfig{Mode: api.AuthModeToken}, as, maxBytes, feedSvc, api.KindleConfig{}, nil, nil), dir
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

// Minimal AVIF fixture builder for these handler tests.
//
// internal/assets/avif_test.go already has a general-purpose buildAVIF, but
// it lives in a _test.go file so it isn't importable from this package
// (api_test), and exporting a testing.T-taking helper from a non-test file
// in internal/assets would leak a test-only concern into production code.
// The least-invasive option is to duplicate the (small) box-assembly logic
// needed for exactly the two fixtures this test needs: a valid AVIF with one
// av01 item + one Exif item, and a corrupt one. See avif-task-3-report.md
// for the fuller rationale.

func avifBE16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

func avifBE32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func avifMkBox(typ string, body []byte) []byte {
	out := make([]byte, 0, 8+len(body))
	out = append(out, avifBE32(uint32(8+len(body)))...)
	out = append(out, []byte(typ)...)
	out = append(out, body...)
	return out
}

func avifFullBoxBody(version byte, rest []byte) []byte {
	body := make([]byte, 0, 4+len(rest))
	body = append(body, version, 0, 0, 0)
	body = append(body, rest...)
	return body
}

func avifInfe(id uint16, itemType string) []byte {
	rest := make([]byte, 0, 2+2+4+1)
	rest = append(rest, avifBE16(id)...) // item_ID (version 2: 16-bit)
	rest = append(rest, avifBE16(0)...)  // item_protection_index
	rest = append(rest, []byte(itemType)...)
	rest = append(rest, 0x00) // item_name, empty and null-terminated
	return avifMkBox("infe", avifFullBoxBody(2, rest))
}

// buildTestAVIF assembles a minimal but spec-valid AVIF (ftyp brand "avif",
// meta with hdlr/pitm/iinf/iloc, trailing mdat) with exactly two items: an
// av01 item (id 1, primary) and an Exif item (id 2), mirroring the layout
// internal/assets/avif_test.go's buildAVIF produces for the same case.
func buildTestAVIF(t *testing.T, av01Data, exifData []byte) []byte {
	t.Helper()

	ftypBody := make([]byte, 0, 4+4+8)
	ftypBody = append(ftypBody, []byte("avif")...) // major_brand
	ftypBody = append(ftypBody, avifBE32(0)...)    // minor_version
	ftypBody = append(ftypBody, []byte("avif")...) // compatible_brands...
	ftypBody = append(ftypBody, []byte("mif1")...)
	ftyp := avifMkBox("ftyp", ftypBody)

	hdlrRest := make([]byte, 0, 4+4+12+1)
	hdlrRest = append(hdlrRest, 0, 0, 0, 0)
	hdlrRest = append(hdlrRest, []byte("pict")...)
	hdlrRest = append(hdlrRest, make([]byte, 12)...)
	hdlrRest = append(hdlrRest, 0x00)
	hdlr := avifMkBox("hdlr", avifFullBoxBody(0, hdlrRest))

	pitm := avifMkBox("pitm", avifFullBoxBody(0, avifBE16(1))) // primary item = av01 (id 1)

	iinfRest := avifBE16(2) // entry_count
	iinfRest = append(iinfRest, avifInfe(1, "av01")...)
	iinfRest = append(iinfRest, avifInfe(2, "Exif")...)
	iinf := avifMkBox("iinf", avifFullBoxBody(0, iinfRest))

	// iloc: offset_size=4, length_size=4, base_offset_size=0, index_size=0;
	// one extent per item, offsets patched below once mdat's position is known.
	itemIDs := []uint16{1, 2}
	lengths := []int{len(av01Data), len(exifData)}
	ilocRest := []byte{0x44, 0x00}
	ilocRest = append(ilocRest, avifBE16(uint16(len(itemIDs)))...) // item_count
	patchPos := make([]int, len(itemIDs))
	for i, id := range itemIDs {
		ilocRest = append(ilocRest, avifBE16(id)...) // item_ID
		ilocRest = append(ilocRest, avifBE16(0)...)  // data_reference_index
		ilocRest = append(ilocRest, avifBE16(1)...)  // extent_count
		patchPos[i] = len(ilocRest)
		ilocRest = append(ilocRest, avifBE32(0)...)                  // extent_offset (placeholder)
		ilocRest = append(ilocRest, avifBE32(uint32(lengths[i]))...) // extent_length
	}
	iloc := avifMkBox("iloc", avifFullBoxBody(0, ilocRest))

	var metaChildren []byte
	metaChildren = append(metaChildren, hdlr...)
	metaChildren = append(metaChildren, pitm...)
	metaChildren = append(metaChildren, iinf...)
	metaChildren = append(metaChildren, iloc...)
	meta := avifMkBox("meta", avifFullBoxBody(0, metaChildren))

	prefix := make([]byte, 0, len(ftyp)+len(meta))
	prefix = append(prefix, ftyp...)
	prefix = append(prefix, meta...)

	// Absolute position (within prefix) of iloc's own body, so patchPos
	// (relative to ilocRest) can be translated into absolute prefix offsets.
	ilocRestBase := len(ftyp) + 8 /* meta header */ + 4 /* meta version/flags */ +
		len(hdlr) + len(pitm) + len(iinf) + 8 /* iloc header */ + 4 /* iloc version/flags */

	mdatPayload := make([]byte, 0, len(av01Data)+len(exifData))
	itemOffsetInMdat := make([]int, len(itemIDs))
	itemOffsetInMdat[0] = 0
	mdatPayload = append(mdatPayload, av01Data...)
	itemOffsetInMdat[1] = len(mdatPayload)
	mdatPayload = append(mdatPayload, exifData...)

	const mdatHeaderLen = 8
	for i := range itemIDs {
		abs := len(prefix) + mdatHeaderLen + itemOffsetInMdat[i]
		pos := ilocRestBase + patchPos[i]
		binary.BigEndian.PutUint32(prefix[pos:pos+4], uint32(abs))
	}

	out := make([]byte, 0, len(prefix)+mdatHeaderLen+len(mdatPayload))
	out = append(out, prefix...)
	out = append(out, avifMkBox("mdat", mdatPayload)...)
	return out
}

// buildMalformedAVIF returns a byte slice that still sniffs as "image/avif"
// (its ftyp box is intact and complete) but whose meta box is truncated mid
// way through iloc's entries: the meta/iloc box headers still declare their
// original, now-too-large sizes, so parsing it fails rather than silently
// producing a wrong result — exercising the StripMetadata error path (400),
// as opposed to the allowlist path (415).
func buildMalformedAVIF(t *testing.T) []byte {
	t.Helper()
	full := buildTestAVIF(t, []byte("fake-av1-bitstream-payload"), []byte{0x4D, 0x4D, 0x00, 0x2A})
	idx := bytes.Index(full, []byte("iloc"))
	if idx < 0 {
		t.Fatalf("fixture has no iloc box")
	}
	return append([]byte(nil), full[:idx+12]...)
}

func TestCreateAssetAVIFAccepted(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, dir := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	avif := buildTestAVIF(t, []byte("fake-av1-bitstream-payload"), []byte{0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x08})
	resp := postUpload(t, srv.URL+"/assets", "photo.avif", avif)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (AVIF should now be accepted)", resp.StatusCode)
	}

	var item map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	lead, _ := item["leadImageUrl"].(string)
	if !strings.HasPrefix(lead, "/assets/") {
		t.Fatalf("leadImageUrl = %q, want /assets/ prefix", lead)
	}
	assetID := strings.TrimPrefix(lead, "/assets/")

	stored, err := os.ReadFile(filepath.Join(dir, assetID))
	if err != nil {
		t.Fatalf("reading stored blob: %v", err)
	}

	// The Exif item must be gone from the stored bytes: re-stripping should be
	// a no-op (byte-identical output), proving the upload path already
	// stripped it rather than storing it verbatim.
	reStripped, err := assets.StripMetadata("image/avif", stored)
	if err != nil {
		t.Fatalf("StripMetadata(stored): %v", err)
	}
	if !bytes.Equal(reStripped, stored) {
		t.Errorf("stored AVIF bytes are not already stripped: re-stripping changed them (stored %d bytes, re-stripped %d bytes)", len(stored), len(reStripped))
	}
}

func TestCreateAssetMalformedAVIF400(t *testing.T) {
	s, rc, pool := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	var itemsBefore int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM items`).Scan(&itemsBefore); err != nil {
		t.Fatalf("counting items before: %v", err)
	}

	resp := postUpload(t, srv.URL+"/assets", "corrupt.avif", buildMalformedAVIF(t))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "could not process image" {
		t.Errorf("error = %v, want %q", body["error"], "could not process image")
	}

	var itemsAfter int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM items`).Scan(&itemsAfter); err != nil {
		t.Fatalf("counting items after: %v", err)
	}
	if itemsAfter != itemsBefore {
		t.Errorf("items count = %d, want unchanged %d (no orphan row on 400)", itemsAfter, itemsBefore)
	}

	var assetsCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM assets`).Scan(&assetsCount); err != nil {
		t.Fatalf("counting assets: %v", err)
	}
	if assetsCount != 0 {
		t.Errorf("assets count = %d, want 0 (no asset row on 400)", assetsCount)
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
	if csp := got.Header.Get("Content-Security-Policy"); csp != "sandbox" {
		t.Errorf("Content-Security-Policy = %q, want sandbox", csp)
	}
	data, _ := io.ReadAll(got.Body)
	if !bytes.Equal(data, want) {
		t.Errorf("served bytes differ from uploaded (%d vs %d)", len(data), len(want))
	}
}

func TestCreateAssetPDF(t *testing.T) {
	s, rc, pool := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 10<<20)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	pdfBytes := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF")
	resp := postUpload(t, srv.URL+"/assets", "paper.pdf", pdfBytes)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var item map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["cardType"] != "article" {
		t.Errorf("cardType = %v, want article", item["cardType"])
	}
	if item["title"] != "paper" {
		t.Errorf("title = %v, want paper", item["title"])
	}
	if _, ok := item["leadImageUrl"]; ok && item["leadImageUrl"] != "" && item["leadImageUrl"] != nil {
		t.Errorf("leadImageUrl = %v, want empty/absent", item["leadImageUrl"])
	}
	itemURL, _ := item["url"].(string)
	if !strings.HasPrefix(itemURL, "/assets/") {
		t.Fatalf("url = %q, want /assets/ prefix", itemURL)
	}
	assetID := strings.TrimPrefix(itemURL, "/assets/")

	var ct string
	if err := pool.QueryRow(context.Background(),
		`SELECT content_type FROM assets WHERE id = $1 AND user_id = $2`,
		assetID, api.DevUserID).Scan(&ct); err != nil {
		t.Fatalf("asset row: %v", err)
	}
	if ct != "application/pdf" {
		t.Errorf("content_type = %q, want application/pdf", ct)
	}

	got, err := http.Get(srv.URL + itemURL)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.StatusCode)
	}
	if ctHdr := got.Header.Get("Content-Type"); ctHdr != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ctHdr)
	}
	if nosniff := got.Header.Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", nosniff)
	}
	if csp := got.Header.Get("Content-Security-Policy"); csp != "sandbox" {
		t.Errorf("Content-Security-Policy = %q, want sandbox", csp)
	}
	data, _ := io.ReadAll(got.Body)
	if !bytes.Equal(data, pdfBytes) {
		t.Errorf("served bytes differ from uploaded (%d vs %d)", len(data), len(pdfBytes))
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&count); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("enrich_item jobs = %d, want 1", count)
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
