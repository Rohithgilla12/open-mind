package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// postImport uploads a file to /import as multipart/form-data and returns the
// decoded ImportResult plus the HTTP status.
func postImport(t *testing.T, url, filename, content string) (map[string]int, int) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	mw.Close()
	resp, err := http.Post(url+"/import", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]int
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return out, resp.StatusCode
}

func TestImportBookmarksIsIdempotent(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
  <DT><A HREF="https://example.com/a">A</A>
  <DT><A HREF="https://example.com/b">B</A>
  <DT><A HREF="https://example.com/a">A again (dup in file)</A>
  <DT><A HREF="not-a-url">bad</A>
</DL>`

	// First import: two unique valid URLs created, one in-file dup skipped, one bad failed.
	res, status := postImport(t, srv.URL, "bookmarks.html", html)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if res["total"] != 4 || res["imported"] != 2 || res["skipped"] != 1 || res["failed"] != 1 {
		t.Fatalf("first import = %+v, want total 4 / imported 2 / skipped 1 / failed 1", res)
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM items WHERE url LIKE 'https://example.com/%'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("items created = %d, want 2", count)
	}

	// Enrichment must be queued for the created items.
	var jobCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 2 {
		t.Errorf("enrich jobs = %d, want 2", jobCount)
	}

	// Re-import the same file: everything already saved → all skipped, nothing new.
	res, _ = postImport(t, srv.URL, "bookmarks.html", html)
	if res["imported"] != 0 || res["skipped"] != 3 || res["failed"] != 1 {
		t.Fatalf("re-import = %+v, want imported 0 / skipped 3 / failed 1", res)
	}
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM items WHERE url LIKE 'https://example.com/%'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("items after re-import = %d, want 2 (idempotent)", count)
	}
}

func TestImportPreservesTags(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
  <DT><A HREF="https://tagged.example.com/a" TAGS="Go, Rust, Go">Tagged</A>
  <DT><A HREF="https://tagged.example.com/b">Untagged</A>
</DL>`

	res, status := postImport(t, srv.URL, "tagged.html", html)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if res["imported"] != 2 {
		t.Fatalf("imported = %d, want 2", res["imported"])
	}

	// Tagged item carries canonicalised (lowercased, deduped) user tags.
	var tags []string
	if err := pool.QueryRow(context.Background(),
		`SELECT user_tags FROM items WHERE url = 'https://tagged.example.com/a'`).Scan(&tags); err != nil {
		t.Fatalf("query tagged: %v", err)
	}
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "rust" {
		t.Errorf("user_tags = %v, want [go rust]", tags)
	}

	// Untagged item has no user tags.
	if err := pool.QueryRow(context.Background(),
		`SELECT user_tags FROM items WHERE url = 'https://tagged.example.com/b'`).Scan(&tags); err != nil {
		t.Fatalf("query untagged: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("untagged user_tags = %v, want empty", tags)
	}
}

func TestImportRejectsEmptyFile(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	_, status := postImport(t, srv.URL, "notes.txt", "just some prose\nno links at all\n")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}
