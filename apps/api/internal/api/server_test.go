package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store"
)

func testDeps(t *testing.T) (*store.Store, *river.Client[pgx.Tx], *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://openmind:openmind@localhost:5433/openmind_test"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := store.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE items, item_embeddings, river_job CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, api.DevUserID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	p := &enrich.Pipeline{Store: s, AI: ai.NewNoop(), Extractor: enrich.NewTrafilatura(nil)}
	rc, err := jobs.NewRiverClient(pool, p, false)
	if err != nil {
		t.Fatalf("river client: %v", err)
	}
	return s, rc, pool
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func TestCreateItemIsInstantAndPending(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	start := time.Now()
	resp := postJSON(t, srv.URL+"/items", `{"url":"https://example.com/a"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("save took %v; capture must be instant", elapsed)
	}
	var item map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["status"] != "pending" {
		t.Errorf("status = %v, want pending", item["status"])
	}
	if item["url"] != "https://example.com/a" {
		t.Errorf("url = %v", item["url"])
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&count); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("enrich_item job rows = %d, want 1", count)
	}
}

func TestCreateItemRejectsBadURL(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	for _, body := range []string{`{"url":"not a url"}`, `{"url":"ftp://example.com"}`, `{"url":""}`} {
		resp := postJSON(t, srv.URL+"/items", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestCreateItemFromNote(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/items", `{"note":"remember the milk"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var item map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["status"] != "pending" {
		t.Errorf("status = %v, want pending", item["status"])
	}

	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = 'enrich_item'`).Scan(&count); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	if count != 1 {
		t.Errorf("enrich_item job rows = %d, want 1", count)
	}
}

func TestCreateItemRejectsBadURLOrNoteCombos(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	for _, body := range []string{
		`{"url":"https://example.com","note":"both"}`,
		`{}`,
		`{"note":"   "}`,
	} {
		resp := postJSON(t, srv.URL+"/items", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestListItems(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/items", `{"url":"https://example.com/first"}`).Body.Close()
	time.Sleep(10 * time.Millisecond)
	postJSON(t, srv.URL+"/items", `{"url":"https://example.com/second"}`).Body.Close()

	resp, err := http.Get(srv.URL + "/items")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0]["url"] != "https://example.com/second" {
		t.Errorf("newest-first ordering wrong: got %v first", items[0]["url"])
	}
}

func TestSearchItemsReturnsEmptyArray(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?q=anything")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := make([]byte, 2)
	resp.Body.Read(body)
	if string(body) != "[]" {
		t.Errorf("search body = %q, want []", string(body))
	}
}
