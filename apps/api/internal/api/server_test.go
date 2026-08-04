package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/feeds"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, item_embeddings, lenses, feeds, river_job, api_keys, device_links, user_settings CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, api.DevUserID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	p := &enrich.Pipeline{Store: s, AI: ai.NewNoop(), Extractor: enrich.NewTrafilatura(nil)}
	rc, err := jobs.NewRiverClient(pool, p, nil, jobs.KindleDeps{}, jobs.NotifyDeps{}, nil, reelmedia.ModeThumbnail, nil, false)
	if err != nil {
		t.Fatalf("river client: %v", err)
	}
	return s, rc, pool
}

// newSrv builds a Server handler backed by a throwaway on-disk asset store and
// the standard 10 MiB upload cap, with Send-to-Kindle unconfigured. Tests that
// need to inspect the asset dir use newSrvWithAssets instead; kindle tests use
// newSrvWithKindle.
func newSrv(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], token string) http.Handler {
	t.Helper()
	return newSrvWithProvider(t, s, rc, token, ai.NewNoop())
}

// newSrvWithProvider builds a Server backed by a specific AI provider, so tests
// can exercise natural-language query parsing with a scripted interpretation.
func newSrvWithProvider(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], token string, p ai.Provider) http.Handler {
	t.Helper()
	return newSrvWithKindle(t, s, rc, token, p, api.KindleConfig{})
}

// newSrvWithKindle builds a Server with Send-to-Kindle's config set
// explicitly, so kindle handler tests can exercise the 409-unconfigured,
// SMTP-without-recipient, and happy paths without touching real SMTP.
func newSrvWithKindle(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], token string, p ai.Provider, kindleCfg api.KindleConfig) http.Handler {
	t.Helper()
	as, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	feedSvc := feeds.NewService(s)
	feedSvc.River = rc
	return api.NewServer(s, rc, p, api.AuthConfig{Mode: api.AuthModeToken, LegacyToken: token}, as, 10<<20, feedSvc, kindleCfg, nil)
}

// newSrvWithAuthConfig builds a Server with an explicit AuthConfig, for tests
// exercising credential resolution beyond the legacy-token path (API keys,
// Clerk JWTs).
func newSrvWithAuthConfig(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], authCfg api.AuthConfig) http.Handler {
	t.Helper()
	as, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	feedSvc := feeds.NewService(s)
	feedSvc.River = rc
	return api.NewServer(s, rc, ai.NewNoop(), authCfg, as, 10<<20, feedSvc, api.KindleConfig{}, nil)
}

// parseProvider is a noop provider whose ParseQuery returns a scripted result,
// simulating an AI backend that interprets a natural-language query.
type parseProvider struct {
	*ai.Noop
	parsed ai.ParsedQuery
}

func (p parseProvider) ParseQuery(context.Context, string) (ai.ParsedQuery, error) {
	return p.parsed, nil
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	for _, body := range []string{`{"url":"not a url"}`, `{"url":"ftp://example.com"}`, `{"url":""}`} {
		resp := postJSON(t, srv.URL+"/items", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// TestCreateItemRejectsWhitespacePaddedURL guards against a regression where
// the capture helper trimmed the URL before validating/storing it: a
// whitespace-padded URL must still fail validURL and return 400, exactly as
// the pre-refactor CreateItem did (only the note is trimmed). This never
// reaches the store, so — like TestMCPMountedAndGuarded — a Server built with
// a nil store/river/provider/assets is safe here; no Postgres required.
func TestCreateItemRejectsWhitespacePaddedURL(t *testing.T) {
	srv := httptest.NewServer(api.NewServer(nil, nil, nil, api.AuthConfig{Mode: api.AuthModeToken}, nil, 0, nil, api.KindleConfig{}, nil))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/items", `{"url":"http://x.com "}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateItemFromNote(t *testing.T) {
	s, rc, pool := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	var page struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(page.Items))
	}
	if page.NextCursor != nil {
		t.Errorf("nextCursor = %q with only 2 items and a limit of 50, want absent", *page.NextCursor)
	}
	if page.Items[0]["url"] != "https://example.com/second" {
		t.Errorf("newest-first ordering wrong: got %v first", page.Items[0]["url"])
	}
}

func TestGetItemDetail(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	otherID := seedOtherUserItem(t, s, "someone else")
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
	otherID := seedOtherUserItem(t, s, "protected")
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
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
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
func seedOtherUserItem(t *testing.T, s *store.Store, body string) string {
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
	return item.ID.String()
}

func TestSearchItemsReturnsEmptyArray(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?q=anything")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out api.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Results == nil {
		t.Errorf("results = nil, want non-null empty array")
	}
	if len(out.Results) != 0 {
		t.Errorf("results = %v, want empty", out.Results)
	}
	if out.Understood != nil {
		t.Errorf("understood = %+v, want nil without parse", out.Understood)
	}
}

// seedEnriched inserts an enriched item owned by the dev user with the given
// card type and palette, so search/parse tests have real rows to match.
func seedEnriched(t *testing.T, s *store.Store, title, body, cardType string, palette []string) db.Item {
	t.Helper()
	return seedEnrichedURL(t, s, "https://example.com/"+title, title, body, cardType, palette)
}

// seedEnrichedURL is seedEnriched with an explicit URL (for domain-filter tests).
func seedEnrichedURL(t *testing.T, s *store.Store, rawURL, title, body, cardType string, palette []string) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: api.DevUserID, Url: rawURL, Body: ""})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: api.DevUserID, ID: item.ID, Title: title, Body: body, CardType: cardType,
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}
	if err := s.Queries.UpdateItemEnrichment(ctx, db.UpdateItemEnrichmentParams{
		UserID: api.DevUserID, ID: item.ID, Summary: body, Tags: []string{title},
	}); err != nil {
		t.Fatalf("update enrichment: %v", err)
	}
	if len(palette) > 0 {
		if err := s.Queries.SetItemPalette(ctx, db.SetItemPaletteParams{UserID: api.DevUserID, ID: item.ID, Palette: palette}); err != nil {
			t.Fatalf("set palette: %v", err)
		}
	}
	return item
}

// TestSearchItemsParseSplitsQuery drives the full parse=true path: the provider
// splits "blue book about bread" into text+colour+type, the response echoes what
// was understood, and the type filter narrows results to the matching card.
func TestSearchItemsParseSplitsQuery(t *testing.T) {
	s, rc, _ := testDeps(t)
	book := seedEnriched(t, s, "bread book", "a book about baking bread", "book", []string{"#1B3FD1"})
	seedEnriched(t, s, "bread article", "an article about baking bread", "article", []string{"#D1291B"})

	prov := parseProvider{Noop: ai.NewNoop(), parsed: ai.ParsedQuery{Text: "bread", Color: "blue", Types: []string{"book"}}}
	srv := httptest.NewServer(newSrvWithProvider(t, s, rc, "", prov))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?q=" + url.QueryEscape("blue book about bread") + "&parse=true")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out api.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if out.Understood == nil {
		t.Fatal("understood = nil, want populated with parse=true")
	}
	if out.Understood.Text == nil || *out.Understood.Text != "bread" {
		t.Errorf("understood.text = %v, want bread", out.Understood.Text)
	}
	if out.Understood.Color == nil || *out.Understood.Color != "blue" {
		t.Errorf("understood.color = %v, want blue", out.Understood.Color)
	}
	if out.Understood.Types == nil || len(*out.Understood.Types) != 1 || (*out.Understood.Types)[0] != "book" {
		t.Errorf("understood.types = %v, want [book]", out.Understood.Types)
	}
	if out.Understood.Domains != nil {
		t.Errorf("understood.domains = %v, want nil", out.Understood.Domains)
	}

	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1 (type filter drops the article)", len(out.Results))
	}
	if out.Results[0].Item.Id.String() != book.ID.String() {
		t.Errorf("result = %v, want book %v", out.Results[0].Item.Id, book.ID)
	}
}

func TestSearchItemsExplicitTypes(t *testing.T) {
	s, rc, _ := testDeps(t)
	book := seedEnriched(t, s, "bread book", "a book about baking bread", "book", nil)
	seedEnriched(t, s, "bread article", "an article about baking bread", "article", nil)

	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?types=book&q=" + url.QueryEscape("bread"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out api.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	if out.Results[0].Item.Id.String() != book.ID.String() {
		t.Errorf("result = %v, want book %v", out.Results[0].Item.Id, book.ID)
	}
	if out.Understood != nil {
		t.Errorf("understood = %+v, want nil without parse", out.Understood)
	}
}

func TestSearchItemsExplicitDomains(t *testing.T) {
	s, rc, _ := testDeps(t)
	xcom := seedEnrichedURL(t, s, "https://x.com/shoes", "x shoes", "shoes on x", "tweet", nil)
	seedEnrichedURL(t, s, "https://example.com/shoes", "ex shoes", "shoes elsewhere", "article", nil)

	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?domains=x.com")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out api.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	if out.Results[0].Item.Id.String() != xcom.ID.String() {
		t.Errorf("result = %v, want x.com item %v", out.Results[0].Item.Id, xcom.ID)
	}
}

func TestSearchItemsExplicitWinsOverParse(t *testing.T) {
	s, rc, _ := testDeps(t)
	seedEnriched(t, s, "bread book", "a book about baking bread", "book", nil)
	article := seedEnriched(t, s, "bread article", "an article about baking bread", "article", nil)

	// Parse would filter to book; explicit types=article must win.
	prov := parseProvider{Noop: ai.NewNoop(), parsed: ai.ParsedQuery{Text: "bread", Types: []string{"book"}}}
	srv := httptest.NewServer(newSrvWithProvider(t, s, rc, "", prov))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?q=" + url.QueryEscape("bread book") + "&types=article&parse=true")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out api.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Understood == nil || out.Understood.Types == nil || len(*out.Understood.Types) != 1 || (*out.Understood.Types)[0] != "article" {
		t.Errorf("understood.types = %v, want [article]", out.Understood)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	if out.Results[0].Item.Id.String() != article.ID.String() {
		t.Errorf("result = %v, want article %v", out.Results[0].Item.Id, article.ID)
	}
}

func TestSearchItemsParseDomains(t *testing.T) {
	s, rc, _ := testDeps(t)
	seedEnrichedURL(t, s, "https://x.com/a", "x post", "a post", "tweet", nil)

	prov := parseProvider{Noop: ai.NewNoop(), parsed: ai.ParsedQuery{Text: "shoes", Domains: []string{"x.com"}}}
	srv := httptest.NewServer(newSrvWithProvider(t, s, rc, "", prov))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search?q=" + url.QueryEscape("posts from x.com about shoes") + "&parse=true")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out api.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Understood == nil {
		t.Fatal("understood = nil, want populated")
	}
	if out.Understood.Domains == nil || len(*out.Understood.Domains) != 1 || (*out.Understood.Domains)[0] != "x.com" {
		t.Errorf("understood.domains = %v, want [x.com]", out.Understood.Domains)
	}
	if out.Understood.Text == nil || *out.Understood.Text != "shoes" {
		t.Errorf("understood.text = %v, want shoes", out.Understood.Text)
	}
}

func TestSearchItemsRequiresMatchSignal(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/search")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "q, color, types, or domains is required" {
		t.Errorf("error = %q, want match-signal message", body.Error)
	}
}
