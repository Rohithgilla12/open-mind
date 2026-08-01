package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rohithgilla12/openmind/api/internal/api"
)

// fakeRaindrop serves the Raindrop.io export endpoint: a wrong token gets 401,
// the right one gets the canned CSV. It fails the test on any unexpected path.
func fakeRaindrop(t *testing.T, token, csv string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/raindrops/0/export.csv" {
			t.Errorf("unexpected raindrop path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(csv))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// postRaindrop calls POST /import/raindrop with the given token and returns
// the decoded summary plus the HTTP status.
func postRaindrop(t *testing.T, url, token string) (map[string]int, int) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"token": token})
	resp, err := http.Post(url+"/import/raindrop", "application/json", bytes.NewReader(body))
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

func TestImportRaindrop(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	csv := "id,title,note,excerpt,url,folder,tags,created,cover,highlights,favorite\n" +
		"1,Go blog,,,https://raindrop.example.com/go,Dev,\"go, reading\",2024-01-01T00:00:00Z,,,false\n" +
		"2,Pie,,,https://raindrop.example.com/pie,Unsorted,,2024-01-02T00:00:00Z,,,false\n" +
		"3,Broken,,,not-a-url,Dev,,2024-01-03T00:00:00Z,,,false\n"
	rd := fakeRaindrop(t, "good-token", csv)
	t.Cleanup(api.SetRaindropBaseForTest(rd.URL))

	res, status := postRaindrop(t, srv.URL, "good-token")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if res["total"] != 3 || res["imported"] != 2 || res["failed"] != 1 {
		t.Fatalf("result = %+v, want total 3 / imported 2 / failed 1", res)
	}

	// Raindrop tags and the collection both land as canonicalised user tags.
	var tags []string
	if err := pool.QueryRow(context.Background(),
		`SELECT user_tags FROM items WHERE url = 'https://raindrop.example.com/go'`).Scan(&tags); err != nil {
		t.Fatalf("query tags: %v", err)
	}
	if len(tags) != 3 || tags[0] != "go" || tags[1] != "reading" || tags[2] != "dev" {
		t.Errorf("user_tags = %v, want [go reading dev]", tags)
	}

	// The Unsorted default bucket must not become a tag.
	if err := pool.QueryRow(context.Background(),
		`SELECT user_tags FROM items WHERE url = 'https://raindrop.example.com/pie'`).Scan(&tags); err != nil {
		t.Fatalf("query pie tags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("pie user_tags = %v, want empty", tags)
	}

	// Enrichment queued for both created items.
	var jobCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 2 {
		t.Errorf("enrich jobs = %d, want 2", jobCount)
	}

	// Re-running the import is idempotent: everything already saved is skipped.
	res, _ = postRaindrop(t, srv.URL, "good-token")
	if res["imported"] != 0 || res["skipped"] != 2 || res["failed"] != 1 {
		t.Fatalf("re-import = %+v, want imported 0 / skipped 2 / failed 1", res)
	}
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM items WHERE url LIKE 'https://raindrop.example.com/%'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("items after re-import = %d, want 2 (idempotent)", count)
	}
}

func TestImportRaindropRejectedToken(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	rd := fakeRaindrop(t, "good-token", "id,url\n")
	t.Cleanup(api.SetRaindropBaseForTest(rd.URL))

	if _, status := postRaindrop(t, srv.URL, "wrong-token"); status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if _, status := postRaindrop(t, srv.URL, "   "); status != http.StatusBadRequest {
		t.Errorf("blank token status = %d, want 400", status)
	}
}

func TestImportRaindropEmptyAccount(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	// Header-only CSV — a Raindrop account with nothing in it is a valid,
	// zero-item import, not an error.
	rd := fakeRaindrop(t, "good-token", "id,title,url,folder,tags\n")
	t.Cleanup(api.SetRaindropBaseForTest(rd.URL))

	res, status := postRaindrop(t, srv.URL, "good-token")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if res["total"] != 0 || res["imported"] != 0 {
		t.Errorf("result = %+v, want all zeros", res)
	}
}

func TestImportRaindropUpstreamError(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	rd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(rd.Close)
	t.Cleanup(api.SetRaindropBaseForTest(rd.URL))

	if _, status := postRaindrop(t, srv.URL, "any-token"); status != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", status)
	}
}
