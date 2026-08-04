package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/feeds"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// rssFeedServer serves an RSS 2.0 document whose entry links are controlled by
// the atomic pointer. When status is non-2xx it serves that error instead, so a
// test can simulate an unreachable/broken feed.
func rssFeedServer(t *testing.T, entries *atomic.Pointer[[]string], status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != 0 && (status < 200 || status >= 300) {
			http.Error(w, "boom", status)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel>`)
		b.WriteString(`<title>Test Feed</title><link>https://example.com</link>`)
		for i, u := range *entries.Load() {
			fmt.Fprintf(&b, `<item><title>Entry %d</title><link>%s</link></item>`, i, u)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newFeedSrv builds a Server whose feed service fetches through feedClient (an
// httptest server's client bypasses the SSRF guard), so POST /feeds can reach a
// local test feed. Pass a nil client when the feed is never fetched (bad-url).
func newFeedSrv(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], feedClient *http.Client) http.Handler {
	t.Helper()
	as, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	fs := feeds.NewService(s)
	if feedClient != nil {
		fs.HTTPClient = feedClient
	}
	fs.River = rc
	return api.NewServer(s, rc, ai.NewNoop(), api.AuthConfig{Mode: api.AuthModeToken}, as, 10<<20, fs, api.KindleConfig{}, nil)
}

func TestCreateFeedBackfillsAndReturns201(t *testing.T) {
	s, rc, pool := testDeps(t)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b"})
	feedSrv := rssFeedServer(t, &entries, 0)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, feedSrv.Client()))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/feeds", fmt.Sprintf(`{"url":%q}`, feedSrv.URL))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var feed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if feed["title"] != "Test Feed" {
		t.Errorf("title = %v, want Test Feed", feed["title"])
	}
	if feed["url"] != feedSrv.URL {
		t.Errorf("url = %v, want %v", feed["url"], feedSrv.URL)
	}
	if feed["lastStatus"] != "ok" {
		t.Errorf("lastStatus = %v, want ok", feed["lastStatus"])
	}

	// Feed-river semantics: backfilled items route to GET /feed (with feedId set),
	// not to GET /items (home list). Each item enqueues enrichment.
	feedResp, err := http.Get(srv.URL + "/feed")
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	defer feedResp.Body.Close()
	var feedPage struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(feedResp.Body).Decode(&feedPage); err != nil {
		t.Fatalf("decode feed items: %v", err)
	}
	feedItems := feedPage.Items
	if len(feedItems) != 2 {
		t.Errorf("feed items = %d, want 2", len(feedItems))
	}
	for i, item := range feedItems {
		if item["feedId"] == nil {
			t.Errorf("feed item %d has no feedId", i)
		}
	}

	// Backfilled items must NOT appear in GET /items (home list).
	homeResp, err := http.Get(srv.URL + "/items")
	if err != nil {
		t.Fatalf("get items: %v", err)
	}
	defer homeResp.Body.Close()
	var homePage struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(homeResp.Body).Decode(&homePage); err != nil {
		t.Fatalf("decode home items: %v", err)
	}
	if len(homePage.Items) != 0 {
		t.Errorf("home items = %d, want 0 (backfilled items in feed only)", len(homePage.Items))
	}

	var jobs int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&jobs); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if jobs != 2 {
		t.Errorf("enrich jobs = %d, want 2", jobs)
	}
}

func TestCreateFeedDuplicateConflict(t *testing.T) {
	s, rc, _ := testDeps(t)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	feedSrv := rssFeedServer(t, &entries, 0)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, feedSrv.Client()))
	t.Cleanup(srv.Close)

	body := fmt.Sprintf(`{"url":%q}`, feedSrv.URL)
	first := postJSON(t, srv.URL+"/feeds", body)
	first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", first.StatusCode)
	}
	second := postJSON(t, srv.URL+"/feeds", body)
	second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Errorf("second status = %d, want 409", second.StatusCode)
	}
}

func TestCreateFeedBadURL(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, nil))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/feeds", `{"url":"notaurl"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateFeedUnreachableReturns502(t *testing.T) {
	s, rc, _ := testDeps(t)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{})
	feedSrv := rssFeedServer(t, &entries, http.StatusInternalServerError)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, feedSrv.Client()))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/feeds", fmt.Sprintf(`{"url":%q}`, feedSrv.URL))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}

	// A broken feed must not be persisted.
	persisted, err := s.Queries.ListFeeds(context.Background(), api.DevUserID)
	if err != nil {
		t.Fatalf("list feeds: %v", err)
	}
	if len(persisted) != 0 {
		t.Errorf("feeds = %d, want 0 (broken feed must not persist)", len(persisted))
	}
}

func TestListFeeds(t *testing.T) {
	s, rc, _ := testDeps(t)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	feedSrv := rssFeedServer(t, &entries, 0)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, feedSrv.Client()))
	t.Cleanup(srv.Close)

	add := postJSON(t, srv.URL+"/feeds", fmt.Sprintf(`{"url":%q}`, feedSrv.URL))
	add.Body.Close()

	resp, err := http.Get(srv.URL + "/feeds")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("feeds = %d, want 1", len(list))
	}
	if list[0]["url"] != feedSrv.URL {
		t.Errorf("url = %v, want %v", list[0]["url"], feedSrv.URL)
	}
}

func TestDeleteFeed(t *testing.T) {
	s, rc, _ := testDeps(t)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	feedSrv := rssFeedServer(t, &entries, 0)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, feedSrv.Client()))
	t.Cleanup(srv.Close)

	add := postJSON(t, srv.URL+"/feeds", fmt.Sprintf(`{"url":%q}`, feedSrv.URL))
	var created map[string]any
	if err := json.NewDecoder(add.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	add.Body.Close()
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("created feed has no id")
	}

	del := doReq(t, http.MethodDelete, srv.URL+"/feeds/"+id, "")
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", del.StatusCode)
	}

	// Deleting again is a 404 (the row is gone).
	again := doReq(t, http.MethodDelete, srv.URL+"/feeds/"+id, "")
	again.Body.Close()
	if again.StatusCode != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", again.StatusCode)
	}
}

func TestDeleteFeedCrossTenant(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newFeedSrv(t, s, rc, nil))
	t.Cleanup(srv.Close)

	// A feed owned by another user must not be deletable by the dev user.
	other := uuid.New()
	if err := s.Queries.EnsureUser(context.Background(), other); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	feed, err := s.Queries.CreateFeed(context.Background(), db.CreateFeedParams{UserID: other, Url: "https://other.example.com/feed"})
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}

	del := doReq(t, http.MethodDelete, srv.URL+"/feeds/"+feed.ID.String(), "")
	del.Body.Close()
	if del.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant delete status = %d, want 404", del.StatusCode)
	}
}

// doReq issues an arbitrary-method request and returns the response.
func doReq(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var r *http.Request
	var err error
	if body == "" {
		r, err = http.NewRequest(method, url, nil)
	} else {
		r, err = http.NewRequest(method, url, strings.NewReader(body))
	}
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}
