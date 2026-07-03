package enrich_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func newTestStore(t *testing.T) *store.Store {
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
	pool.Exec(ctx, `TRUNCATE items, item_embeddings CASCADE`)
	return store.New(pool)
}

func serveFixture(t *testing.T, path string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", path, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPipelineRunIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	srv := serveFixture(t, "testdata/article.html")
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: srv.URL})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	p := &enrich.Pipeline{Store: s, AI: ai.NewFake(), Extractor: enrich.NewTrafilatura(srv.Client())}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if first.Status != "enriched" || first.Summary == "" || len(first.Tags) == 0 {
		t.Fatalf("not enriched: %+v", first)
	}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if second.Summary != first.Summary || second.Title != first.Title || second.Status != first.Status {
		t.Errorf("second run changed state:\nfirst  %+v\nsecond %+v", first, second)
	}
}

func TestPipelineNoopProviderStillCompletes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := uuid.New()
	if err := s.Queries.EnsureUser(ctx, userID); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	srv := serveFixture(t, "testdata/article.html")
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: srv.URL})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	p := &enrich.Pipeline{Store: s, AI: ai.NewNoop(), Extractor: enrich.NewTrafilatura(srv.Client())}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if got.Status != "enriched" {
		t.Errorf("status = %q, want enriched", got.Status)
	}
	if got.Summary != "" {
		t.Errorf("summary = %q, want empty for noop", got.Summary)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM item_embeddings WHERE item_id = $1`, item.ID).Scan(&count); err != nil {
		t.Fatalf("counting embeddings: %v", err)
	}
	if count != 0 {
		t.Errorf("embedding rows = %d, want 0 for noop", count)
	}
}
