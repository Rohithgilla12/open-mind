package api_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/api"
)

const (
	docxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	odtType  = "application/vnd.oasis.opendocument.text"
	rtfType  = "application/rtf"
	epubType = "application/epub+zip"
)

// zipDoc builds an archive from name→body pairs; names in stored are written
// uncompressed, as ODF and EPUB require of their mimetype member.
func zipDoc(t *testing.T, entries [][2]string, stored ...string) []byte {
	t.Helper()
	isStored := make(map[string]bool, len(stored))
	for _, n := range stored {
		isStored[n] = true
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		method := zip.Deflate
		if isStored[e[0]] {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e[0], Method: method})
		if err != nil {
			t.Fatalf("creating entry %s: %v", e[0], err)
		}
		if _, err := w.Write([]byte(e[1])); err != nil {
			t.Fatalf("writing entry %s: %v", e[0], err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

func docxUpload(t *testing.T) []byte {
	t.Helper()
	return zipDoc(t, [][2]string{
		{"[Content_Types].xml", "<Types/>"},
		{"word/document.xml", "<w:document/>"},
	})
}

func TestCreateAssetDocuments(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		data         func(*testing.T) []byte
		wantType     string
		wantCardType string
		wantTitle    string
	}{
		{
			name:     "docx",
			filename: "report.docx",
			data:     docxUpload,
			wantType: docxType, wantCardType: "article", wantTitle: "report",
		},
		{
			name:     "odt",
			filename: "notes.odt",
			data: func(t *testing.T) []byte {
				return zipDoc(t, [][2]string{
					{"mimetype", odtType},
					{"content.xml", "<office/>"},
				}, "mimetype")
			},
			wantType: odtType, wantCardType: "article", wantTitle: "notes",
		},
		{
			name:     "epub becomes a book card",
			filename: "novel.epub",
			data: func(t *testing.T) []byte {
				return zipDoc(t, [][2]string{
					{"mimetype", epubType},
					{"META-INF/container.xml", "<container/>"},
				}, "mimetype")
			},
			wantType: epubType, wantCardType: "book", wantTitle: "novel",
		},
		{
			name:     "rtf",
			filename: "memo.rtf",
			data:     func(*testing.T) []byte { return []byte(`{\rtf1\ansi hello world}`) },
			wantType: rtfType, wantCardType: "article", wantTitle: "memo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, rc, pool := testDeps(t)
			h, _ := newSrvWithAssets(t, s, rc, 10<<20)
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			data := tt.data(t)
			resp := postUpload(t, srv.URL+"/assets", tt.filename, data)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d, want 201", resp.StatusCode)
			}
			var item map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if item["cardType"] != tt.wantCardType {
				t.Errorf("cardType = %v, want %v", item["cardType"], tt.wantCardType)
			}
			if item["title"] != tt.wantTitle {
				t.Errorf("title = %v, want %v", item["title"], tt.wantTitle)
			}

			// The asset path must land in the item's URL: that is what routes
			// the job to the document path rather than the note path.
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
			if ct != tt.wantType {
				t.Errorf("content_type = %q, want %q", ct, tt.wantType)
			}

			// Documents are stored verbatim — no metadata stripping.
			got, err := http.Get(srv.URL + itemURL)
			if err != nil {
				t.Fatalf("get asset: %v", err)
			}
			defer got.Body.Close()
			served, _ := io.ReadAll(got.Body)
			if !bytes.Equal(served, data) {
				t.Errorf("served bytes differ from uploaded (%d vs %d)", len(served), len(data))
			}

			var count int
			if err := pool.QueryRow(context.Background(),
				`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&count); err != nil {
				t.Fatalf("counting jobs: %v", err)
			}
			if count != 1 {
				t.Errorf("enrich_item jobs = %d, want 1", count)
			}
		})
	}
}

// Formats we deliberately do not accept must be refused at upload, not
// discovered later by a failing job.
func TestCreateAssetRejectsUnsupportedDocuments(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		data     func(*testing.T) []byte
	}{
		{
			name:     "xlsx",
			filename: "sheet.xlsx",
			data: func(t *testing.T) []byte {
				return zipDoc(t, [][2]string{
					{"[Content_Types].xml", "<Types/>"},
					{"xl/workbook.xml", "<workbook/>"},
				})
			},
		},
		{
			name:     "pptx",
			filename: "deck.pptx",
			data: func(t *testing.T) []byte {
				return zipDoc(t, [][2]string{
					{"[Content_Types].xml", "<Types/>"},
					{"ppt/presentation.xml", "<presentation/>"},
				})
			},
		},
		{
			name:     "plain zip",
			filename: "archive.zip",
			data: func(t *testing.T) []byte {
				return zipDoc(t, [][2]string{{"readme.txt", "hello"}})
			},
		},
		{
			name:     "docx extension on a non-document",
			filename: "fake.docx",
			data:     func(*testing.T) []byte { return []byte("just some plain text") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, rc, _ := testDeps(t)
			h, _ := newSrvWithAssets(t, s, rc, 10<<20)
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			resp := postUpload(t, srv.URL+"/assets", tt.filename, tt.data(t))
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnsupportedMediaType {
				t.Errorf("status = %d, want 415", resp.StatusCode)
			}
		})
	}
}

// The size cap applies to documents exactly as it does to images.
func TestCreateAssetDocumentOversize413(t *testing.T) {
	s, rc, _ := testDeps(t)
	h, _ := newSrvWithAssets(t, s, rc, 64)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := postUpload(t, srv.URL+"/assets", "report.docx", docxUpload(t))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
}
