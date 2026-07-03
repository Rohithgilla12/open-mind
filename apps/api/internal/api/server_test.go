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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
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

func TestCreateItemRejectsOversizeInput(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		body string
	}{
		{"note over rune cap", `{"note":"` + strings.Repeat("a", 10001) + `"}`},
		{"body over byte cap", `{"note":"` + strings.Repeat("a", 70000) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+"/items", tt.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
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

func TestGetItemDetail(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/items", `{"note":"detail body here"}`)
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	id := created["id"].(string)

	// Owner fetch → 200 with body field.
	got, err := http.Get(srv.URL + "/items/" + id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", got.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(got.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail["body"] != "detail body here" {
		t.Errorf("body = %v, want %q", detail["body"], "detail body here")
	}

	// Another user's item → 404.
	otherID := seedOtherUserItem(t, pool, s, "someone else")
	resp2, err := http.Get(srv.URL + "/items/" + otherID)
	if err != nil {
		t.Fatalf("get other: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", resp2.StatusCode)
	}

	// Random uuid → 404.
	resp3, err := http.Get(srv.URL + "/items/11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("get random: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("random uuid status = %d, want 404", resp3.StatusCode)
	}
}

func TestDeleteItem(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/items", `{"note":"delete me"}`)
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	id := created["id"].(string)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/items/"+id, nil)
	del, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", del.StatusCode)
	}

	// Subsequent GET → 404.
	got, err := http.Get(srv.URL + "/items/" + id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got.Body.Close()
	if got.StatusCode != http.StatusNotFound {
		t.Errorf("after delete status = %d, want 404", got.StatusCode)
	}

	// Deleting another user's item → 404 and the row survives.
	otherID := seedOtherUserItem(t, pool, s, "protected")
	req2, _ := http.NewRequest(http.MethodDelete, srv.URL+"/items/"+otherID, nil)
	del2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("delete other: %v", err)
	}
	del2.Body.Close()
	if del2.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant delete status = %d, want 404", del2.StatusCode)
	}
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM items WHERE id = $1`, otherID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("other user's row count = %d, want 1 (must survive)", count)
	}
}

func TestExportItems(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(api.NewServer(s, rc, ai.NewNoop(), ""))
	t.Cleanup(srv.Close)

	postJSON(t, srv.URL+"/items", `{"note":"first note"}`).Body.Close()
	time.Sleep(10 * time.Millisecond)
	postJSON(t, srv.URL+"/items", `{"note":"second note"}`).Body.Close()

	resp, err := http.Get(srv.URL + "/export")
	if err != nil {
		t.Fatalf("get export: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// ASC order by created_at.
	if items[0]["body"] != "first note" || items[1]["body"] != "second note" {
		t.Errorf("export order wrong: got %v then %v", items[0]["body"], items[1]["body"])
	}
	for i, it := range items {
		if _, ok := it["body"]; !ok {
			t.Errorf("item %d missing body field", i)
		}
	}
}

// seedOtherUserItem inserts a note item owned by a distinct user and returns its id.
func seedOtherUserItem(t *testing.T, pool *pgxpool.Pool, s *store.Store, body string) string {
	t.Helper()
	other := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	ctx := context.Background()
	if err := s.Queries.EnsureUser(ctx, other); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: other, Body: body})
	if err != nil {
		t.Fatalf("create other item: %v", err)
	}
	_ = pool
	return item.ID.String()
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
