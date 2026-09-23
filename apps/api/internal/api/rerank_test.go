package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/feeds"
	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func newSrvWithJev(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], jevClient api.JevClient) http.Handler {
	t.Helper()
	as, err := assets.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatalf("asset store: %v", err)
	}
	feedSvc := feeds.NewService(s)
	feedSvc.River = rc
	return api.NewServer(s, rc, ai.NewNoop(), api.AuthConfig{Mode: api.AuthModeToken}, as, 10<<20, feedSvc, api.KindleConfig{}, nil, jevClient)
}

func enableAIAssisted(t *testing.T, s *store.Store) {
	t.Helper()
	if err := s.Queries.UpsertUserSetting(context.Background(), db.UpsertUserSettingParams{
		UserID: api.DevUserID,
		Key:    jev.SettingKeyAIAssisted,
		Value:  "true",
	}); err != nil {
		t.Fatalf("setting: %v", err)
	}
}

func seedRerankItems(t *testing.T, s *store.Store) (weakID, strongID string) {
	t.Helper()
	ctx := context.Background()
	weak, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: api.DevUserID,
		Body:   "Shares the word bananas but is about shipping crates.",
	})
	if err != nil {
		t.Fatalf("create weak: %v", err)
	}
	strong, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: api.DevUserID,
		Body:   "How to bake banana bread with ripe bananas and walnuts.",
	})
	if err != nil {
		t.Fatalf("create strong: %v", err)
	}
	for _, row := range []struct {
		id    interface{}
		title string
		body  string
	}{
		{weak.ID, "Keyword overlap only", "Shares the word bananas but is about shipping crates."},
		{strong.ID, "Banana bread recipe", "How to bake banana bread with ripe bananas and walnuts."},
	} {
		if _, err := s.Pool.Exec(ctx, `
			UPDATE items SET title = $2, body = $3, status = 'enriched', card_type = 'note'
			WHERE id = $1`, row.id, row.title, row.body); err != nil {
			t.Fatalf("ready: %v", err)
		}
	}
	return weak.ID.String(), strong.ID.String()
}

func TestLensRerank_ReorderAndSkip(t *testing.T) {
	s, rc, _ := testDeps(t)
	enableAIAssisted(t, s)
	weakID, strongID := seedRerankItems(t, s)

	var evalCalls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evalCalls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			State struct {
				Query      string `json:"query"`
				Candidates []struct {
					ID      string `json:"id"`
					Snippet string `json:"snippet"`
				} `json:"candidates"`
			} `json:"state"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, "bad", 400)
			return
		}
		if req.State.Query != "bananas" {
			t.Errorf("query = %q", req.State.Query)
		}
		if len(req.State.Candidates) == 0 {
			t.Error("no candidates")
		}
		for _, c := range req.State.Candidates {
			if len([]rune(c.Snippet)) > jev.SnippetMaxChars {
				t.Errorf("snippet too long: %d", len([]rune(c.Snippet)))
			}
		}
		answers := map[string]any{}
		for i, c := range req.State.Candidates {
			p := 0.1
			if c.ID == strongID {
				p = 0.99
			}
			if c.ID == weakID {
				p = 0.05
			}
			answers["candidate_"+itoaTest(i)] = map[string]any{"type": "noul", "noul": p}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-1.13.0",
			"answers": answers,
			"usage":   map[string]int{"input_tokens": 120, "output_tokens": 10},
		})
	}))
	defer jevSrv.Close()

	client := jev.New("test-key", jev.WithBaseURL(jevSrv.URL), jev.WithHTTPClient(jevSrv.Client()))
	srv := httptest.NewServer(newSrvWithJev(t, s, rc, client))
	defer srv.Close()

	created := postJSON(t, srv.URL+"/lenses", `{"name":"Bananas","rule":{"q":"bananas"}}`)
	var lens struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&lens); err != nil {
		t.Fatalf("lens: %v", err)
	}
	created.Body.Close()

	base := doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items", "")
	var baseOut api.SearchResponse
	if err := json.NewDecoder(base.Body).Decode(&baseOut); err != nil {
		t.Fatalf("base decode: %v", err)
	}
	base.Body.Close()
	if len(baseOut.Results) < 2 {
		t.Fatalf("want ≥2 results, got %d", len(baseOut.Results))
	}
	if evalCalls.Load() != 0 {
		t.Fatalf("jev called without rerank flag: %d", evalCalls.Load())
	}

	reranked := doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items?rerank=true", "")
	var out api.SearchResponse
	if err := json.NewDecoder(reranked.Body).Decode(&out); err != nil {
		t.Fatalf("rerank decode: %v", err)
	}
	reranked.Body.Close()
	if evalCalls.Load() != 1 {
		t.Fatalf("eval calls = %d", evalCalls.Load())
	}
	if len(out.Results) < 2 {
		t.Fatalf("rerank results = %d", len(out.Results))
	}
	if out.Results[0].Item.Id.String() != strongID {
		t.Fatalf("first = %s, want strong %s (order %+v)", out.Results[0].Item.Id, strongID, itemIDs(out))
	}

	var n int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM jev_decisions WHERE user_id=$1 AND surface='rerank' AND item_id IS NULL AND action='applied'`,
		api.DevUserID,
	).Scan(&n); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if n != 1 {
		t.Fatalf("rerank decisions = %d, want 1", n)
	}

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		evalCalls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failSrv.Close()
	failClient := jev.New("test-key", jev.WithBaseURL(failSrv.URL), jev.WithHTTPClient(failSrv.Client()))
	failHTTP := httptest.NewServer(newSrvWithJev(t, s, rc, failClient))
	defer failHTTP.Close()

	before := itemIDs(baseOut)
	skipResp := doJSON(t, http.MethodGet, failHTTP.URL+"/lenses/"+lens.ID+"/items?rerank=true", "")
	var skipOut api.SearchResponse
	if err := json.NewDecoder(skipResp.Body).Decode(&skipOut); err != nil {
		t.Fatalf("skip decode: %v", err)
	}
	skipResp.Body.Close()
	after := itemIDs(skipOut)
	if len(after) != len(before) {
		t.Fatalf("skip len %d vs %d", len(after), len(before))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Fatalf("Skip must preserve order: got %v want %v", after, before)
		}
	}
	var skipped int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM jev_decisions WHERE user_id=$1 AND surface='rerank' AND action='skipped'`,
		api.DevUserID,
	).Scan(&skipped); err != nil {
		t.Fatalf("skipped count: %v", err)
	}
	if skipped < 1 {
		t.Fatalf("want ≥1 skipped decision, got %d", skipped)
	}
}

func TestLensRerank_RequiresSetting(t *testing.T) {
	s, rc, _ := testDeps(t)
	_, _ = seedRerankItems(t, s)
	var calls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{},"usage":{}}`))
	}))
	defer jevSrv.Close()
	client := jev.New("test-key", jev.WithBaseURL(jevSrv.URL), jev.WithHTTPClient(jevSrv.Client()))
	srv := httptest.NewServer(newSrvWithJev(t, s, rc, client))
	defer srv.Close()

	created := postJSON(t, srv.URL+"/lenses", `{"name":"Bananas","rule":{"q":"bananas"}}`)
	var lens struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(created.Body).Decode(&lens)
	created.Body.Close()

	resp := doJSON(t, http.MethodGet, srv.URL+"/lenses/"+lens.ID+"/items?rerank=true", "")
	resp.Body.Close()
	if calls.Load() != 0 {
		t.Fatalf("jev called without aiAssistedOrganisation: %d", calls.Load())
	}
}

func TestSearchRerank_SkippedOnTypeaheadPathWithoutFlag(t *testing.T) {
	s, rc, _ := testDeps(t)
	enableAIAssisted(t, s)
	_, _ = seedRerankItems(t, s)
	var calls atomic.Int32
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{},"usage":{}}`))
	}))
	defer jevSrv.Close()
	client := jev.New("test-key", jev.WithBaseURL(jevSrv.URL), jev.WithHTTPClient(jevSrv.Client()))
	srv := httptest.NewServer(newSrvWithJev(t, s, rc, client))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/search?q=bananas")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls.Load() != 0 {
		t.Fatalf("search without rerank must not call jev (typeahead-safe): %d", calls.Load())
	}
}

func itemIDs(out api.SearchResponse) []string {
	ids := make([]string, len(out.Results))
	for i, r := range out.Results {
		ids[i] = r.Item.Id.String()
	}
	return ids
}

func itoaTest(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
