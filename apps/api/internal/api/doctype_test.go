package api

import (
	"archive/zip"
	"bytes"
	"testing"
)

// zipFixture builds an archive from name→body pairs. Names listed in stored
// are written uncompressed, which ODF and EPUB require of their mimetype
// member.
func zipFixture(t *testing.T, entries [][2]string, stored ...string) []byte {
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

func TestDetectDocType(t *testing.T) {
	tests := []struct {
		name string
		data func(*testing.T) []byte
		want string
	}{
		{
			name: "docx by its main document part",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{
					{"[Content_Types].xml", "<Types/>"},
					{"word/document.xml", "<w:document/>"},
				})
			},
			want: docxContentType,
		},
		{
			name: "odt by its stored mimetype member",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{
					{"mimetype", odtContentType},
					{"content.xml", "<office/>"},
				}, "mimetype")
			},
			want: odtContentType,
		},
		{
			name: "epub by its stored mimetype member",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{
					{"mimetype", epubContentType},
					{"META-INF/container.xml", "<container/>"},
				}, "mimetype")
			},
			want: epubContentType,
		},
		{
			name: "rtf by magic",
			data: func(*testing.T) []byte { return []byte(`{\rtf1\ansi hello}`) },
			want: rtfContentType,
		},
		{
			name: "xlsx is rejected",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{
					{"[Content_Types].xml", "<Types/>"},
					{"xl/workbook.xml", "<workbook/>"},
				})
			},
			want: "",
		},
		{
			name: "pptx is rejected",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{
					{"[Content_Types].xml", "<Types/>"},
					{"ppt/presentation.xml", "<presentation/>"},
				})
			},
			want: "",
		},
		{
			name: "ods is rejected despite a valid mimetype member",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{
					{"mimetype", "application/vnd.oasis.opendocument.spreadsheet"},
					{"content.xml", "<office/>"},
				}, "mimetype")
			},
			want: "",
		},
		{
			name: "plain zip is rejected",
			data: func(t *testing.T) []byte {
				return zipFixture(t, [][2]string{{"readme.txt", "hello"}})
			},
			want: "",
		},
		{
			name: "a deflated mimetype member is not trusted",
			data: func(t *testing.T) []byte {
				// Compressed rather than stored: not what the ODF spec
				// mandates, so it must not be read as an identity claim.
				return zipFixture(t, [][2]string{
					{"mimetype", odtContentType},
					{"content.xml", "<office/>"},
				})
			},
			want: "",
		},
		{
			name: "truncated zip is rejected",
			data: func(t *testing.T) []byte {
				full := zipFixture(t, [][2]string{{"word/document.xml", "<w:document/>"}})
				return full[:len(full)/2]
			},
			want: "",
		},
		{
			name: "empty input",
			data: func(*testing.T) []byte { return nil },
			want: "",
		},
		{
			name: "png is not a document",
			data: func(*testing.T) []byte { return []byte("\x89PNG\r\n\x1a\n") },
			want: "",
		},
		{
			name: "pdf is not a document",
			data: func(*testing.T) []byte { return []byte("%PDF-1.7\n") },
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectDocType(tt.data(t)); got != tt.want {
				t.Errorf("detectDocType = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsDocument(t *testing.T) {
	for _, ct := range []string{docxContentType, odtContentType, rtfContentType, epubContentType} {
		if !isDocument(ct) {
			t.Errorf("isDocument(%q) = false, want true", ct)
		}
	}
	for _, ct := range []string{"application/pdf", "image/png", "application/zip", ""} {
		if isDocument(ct) {
			t.Errorf("isDocument(%q) = true, want false", ct)
		}
	}
}

func TestDocCardTypeFor(t *testing.T) {
	tests := []struct {
		contentType string
		want        string
	}{
		{contentType: epubContentType, want: "book"},
		{contentType: docxContentType, want: "article"},
		{contentType: odtContentType, want: "article"},
		{contentType: rtfContentType, want: "article"},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := docCardTypeFor(tt.contentType); got != tt.want {
				t.Errorf("docCardTypeFor(%q) = %q, want %q", tt.contentType, got, tt.want)
			}
		})
	}
}
