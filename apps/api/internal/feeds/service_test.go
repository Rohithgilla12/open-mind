package feeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// testService builds a Service against the test Postgres, wired with a real
// insert-only River client so enqueued enrichment jobs land in river_job and
// can be counted. It truncates feed/item/job state so each test starts clean.
func testService(t *testing.T) *Service {
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
	if _, err := pool.Exec(ctx, `TRUNCATE feeds, items, item_embeddings, river_job CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	st := store.New(pool)
	river, err := jobs.NewRiverClient(pool, nil, nil, jobs.KindleDeps{}, false)
	if err != nil {
		t.Fatalf("river client: %v", err)
	}
	return &Service{Store: st, River: river}
}

// rssServer serves an RSS 2.0 document whose entry links are controlled by the
// atomic pointer, so a test can change what a feed publishes between refreshes.
func rssServer(t *testing.T, entries *atomic.Pointer[[]string]) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

func newUser(t *testing.T, s *Service) uuid.UUID {
	t.Helper()
	uid := uuid.New()
	if err := s.Store.Queries.EnsureUser(context.Background(), uid); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	return uid
}

func countEnrichJobs(t *testing.T, s *Service) int {
	t.Helper()
	var n int
	if err := s.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = $1`, jobs.EnrichArgs{}.Kind()).Scan(&n); err != nil {
		t.Fatalf("counting jobs: %v", err)
	}
	return n
}

func TestFeedServiceAddBackfillsAndDedups(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b", "https://example.com/c"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	// One entry is already saved, so only the two new ones should be backfilled.
	if _, err := s.Store.Queries.CreateItem(ctx, db.CreateItemParams{UserID: uid, Url: "https://example.com/b"}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	feed, added, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}
	if feed.Title != "Test Feed" {
		t.Errorf("title = %q, want %q", feed.Title, "Test Feed")
	}
	if feed.LastStatus != "ok" || !feed.LastPolledAt.Valid {
		t.Errorf("status = %q polled = %v, want ok/valid", feed.LastStatus, feed.LastPolledAt.Valid)
	}

	items, err := s.Store.Queries.ListItems(ctx, db.ListItemsParams{UserID: uid, Limit: 100})
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("items = %d, want 3 (1 seeded + 2 new)", len(items))
	}
	if got := countEnrichJobs(t, s); got != 2 {
		t.Errorf("enrich jobs = %d, want 2", got)
	}
}

func TestFeedServiceAddDuplicate(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	if _, _, err := s.Add(ctx, uid, srv.URL); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, _, err := s.Add(ctx, uid, srv.URL); err != ErrAlreadySubscribed {
		t.Errorf("second add err = %v, want ErrAlreadySubscribed", err)
	}
}

func TestFeedRefreshOnlyNewEntries(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	feed, added, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added != 2 {
		t.Fatalf("initial added = %d, want 2", added)
	}

	// Nothing changed: refresh saves nothing.
	if n, err := s.Refresh(ctx, feed); err != nil || n != 0 {
		t.Fatalf("refresh (unchanged) = (%d, %v), want (0, nil)", n, err)
	}

	// Feed publishes one more entry: refresh saves exactly that one.
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b", "https://example.com/c"})
	if n, err := s.Refresh(ctx, feed); err != nil || n != 1 {
		t.Fatalf("refresh (one new) = (%d, %v), want (1, nil)", n, err)
	}
}

func TestFeedRefreshRecordsErrorStatus(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	s.HTTPClient = srv.Client()

	// Persist a feed row directly (Add would reject a 500), then refresh it.
	feed, err := s.Store.Queries.CreateFeed(ctx, db.CreateFeedParams{UserID: uid, Url: srv.URL})
	if err != nil {
		t.Fatalf("create feed: %v", err)
	}

	n, err := s.Refresh(ctx, feed)
	if err != nil {
		t.Fatalf("refresh returned fatal error: %v", err)
	}
	if n != 0 {
		t.Errorf("added = %d, want 0", n)
	}

	got, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if !strings.HasPrefix(got.LastStatus, "error") {
		t.Errorf("last_status = %q, want error prefix", got.LastStatus)
	}
	if !got.LastPolledAt.Valid {
		t.Error("last_polled_at not set after refresh")
	}
}
