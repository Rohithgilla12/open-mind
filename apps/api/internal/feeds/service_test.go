package feeds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
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
	if _, err := pool.Exec(ctx, `TRUNCATE feeds, items, item_embeddings, notifications, river_job CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	st := store.New(pool)
	river, err := jobs.NewRiverClient(pool, nil, nil, jobs.KindleDeps{}, jobs.NotifyDeps{}, nil, reelmedia.ModeThumbnail, nil, false)
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

	// ListItems only surfaces items with no feed (or explicitly kept feed items),
	// so once the two backfilled items are adopted onto the feed they drop out
	// of this view — only the seeded, feed-less item remains.
	items, err := s.Store.Queries.ListItems(ctx, db.ListItemsParams{UserID: uid, LimitCount: 100})
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("items = %d, want 1 (seeded item; backfilled items adopted onto feed)", len(items))
	}
	if got := countEnrichJobs(t, s); got != 2 {
		t.Errorf("enrich jobs = %d, want 2", got)
	}

	// The two backfilled items should have been adopted onto the feed once it
	// was persisted; the pre-existing seeded item never gets provenance.
	feedItems, err := s.Store.Queries.ListFeedItems(ctx, db.ListFeedItemsParams{
		UserID: uid, FilterFeedID: pgtype.UUID{Bytes: feed.ID, Valid: true}, LimitCount: 100,
	})
	if err != nil {
		t.Fatalf("list feed items: %v", err)
	}
	if len(feedItems) != 2 {
		t.Errorf("feed items = %d, want 2 (backfilled items adopted onto feed)", len(feedItems))
	}
}

// TestFeedServiceAddBackfillDoesNotNotify asserts that the pre-persist
// backfill in Add — feedID is nil for this path — never enqueues a
// feed_river notification, even though it creates items. Notifying the
// instant someone subscribes (rather than only on later polls finding
// something new) would be the single most annoying thing this feature could
// do.
func TestFeedServiceAddBackfillDoesNotNotify(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	if _, added, err := s.Add(ctx, uid, srv.URL); err != nil || added != 2 {
		t.Fatalf("add: added=%d err=%v, want 2/nil", added, err)
	}

	due, err := s.Store.Queries.ListDueNotifications(ctx, uid)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("pending notifications = %d, want 0 (backfill must stay silent)", len(due))
	}
}

// TestFeedRefreshEnqueuesFeedRiverNotification asserts that Refresh — unlike
// Add's backfill — enqueues exactly one feed_river notification when it finds
// new entries, keyed by feed id and the current UTC hour, carrying feed_id
// and count in its data payload for Coalesce to sum.
func TestFeedRefreshEnqueuesFeedRiverNotification(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	// Add's backfill must not have left a pending notification behind.
	if due, err := s.Store.Queries.ListDueNotifications(ctx, uid); err != nil || len(due) != 0 {
		t.Fatalf("pending after add = %d err=%v, want 0/nil", len(due), err)
	}

	entries.Store(&[]string{"https://example.com/a", "https://example.com/b", "https://example.com/c"})
	added, err := s.Refresh(ctx, feed)
	if err != nil || added != 2 {
		t.Fatalf("refresh: added=%d err=%v, want 2/nil", added, err)
	}

	due, err := s.Store.Queries.ListDueNotifications(ctx, uid)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("pending notifications = %d, want 1", len(due))
	}
	if due[0].Category != "feed_river" {
		t.Errorf("category = %q, want feed_river", due[0].Category)
	}
	wantKey := fmt.Sprintf("feed_river:%s:%s", feed.ID, time.Now().UTC().Format("2006-01-02T15"))
	if due[0].DedupeKey != wantKey {
		t.Errorf("dedupe_key = %q, want %q", due[0].DedupeKey, wantKey)
	}
	var data map[string]any
	if err := json.Unmarshal(due[0].Data, &data); err != nil {
		t.Fatalf("decoding data: %v", err)
	}
	if data["feed_id"] != feed.ID.String() {
		t.Errorf("data.feed_id = %v, want %v", data["feed_id"], feed.ID)
	}
	if count, ok := data["count"].(float64); !ok || int(count) != 2 {
		t.Errorf("data.count = %v, want 2", data["count"])
	}
}

// TestFeedRefreshDedupesFeedRiverNotificationWithinHour runs two refreshes
// that each find one new entry within the same UTC hour: the outbox's
// partial unique index must collapse them into a single pending row rather
// than duplicating it, so a caller can safely retry a poll.
func TestFeedRefreshDedupesFeedRiverNotificationWithinHour(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	entries.Store(&[]string{"https://example.com/a", "https://example.com/b"})
	if _, err := s.Refresh(ctx, feed); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b", "https://example.com/c"})
	if _, err := s.Refresh(ctx, feed); err != nil {
		t.Fatalf("second refresh: %v", err)
	}

	due, err := s.Store.Queries.ListDueNotifications(ctx, uid)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("pending notifications = %d, want 1 (deduped within the hour)", len(due))
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

	// The newly polled item should carry the feed's id directly (CreateFeedItem),
	// not via a later adoption step.
	newItem, err := s.Store.Queries.GetItemByURL(ctx, db.GetItemByURLParams{UserID: uid, Url: "https://example.com/c"})
	if err != nil {
		t.Fatalf("get new item: %v", err)
	}
	if newItem.FeedID.Bytes != feed.ID || !newItem.FeedID.Valid {
		t.Errorf("new item feed_id = %+v, want %v", newItem.FeedID, feed.ID)
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

// conditionalServer serves an RSS document with ETag/Last-Modified validators
// and honours If-None-Match/If-Modified-Since with a 304 whose body is
// deliberately garbage — parsing it would fail, which is how the tests prove a
// 304 short-circuits before the parser.
func conditionalServer(t *testing.T, etag, lastModified string, entries *atomic.Pointer[[]string], gotConditional *atomic.Bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag && etag != "" {
			gotConditional.Store(true)
			w.WriteHeader(http.StatusNotModified)
			_, _ = w.Write([]byte("<<<garbage — must never be parsed>>>"))
			return
		}
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		if lastModified != "" {
			w.Header().Set("Last-Modified", lastModified)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel>`)
		b.WriteString(`<title>Cond Feed</title><link>https://example.com</link>`)
		for i, u := range *entries.Load() {
			fmt.Fprintf(&b, `<item><title>E%d</title><link>%s</link></item>`, i, u)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFeedAddStoresValidators(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/v1"})
	var got atomic.Bool
	srv := conditionalServer(t, `"v1"`, "Mon, 13 Jul 2026 00:00:00 GMT", &entries, &got)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if feed.Etag != `"v1"` || feed.LastModified != "Mon, 13 Jul 2026 00:00:00 GMT" {
		t.Errorf("validators = %q / %q, want stored from response", feed.Etag, feed.LastModified)
	}
}

func TestFeedRefresh304SkipsParseAndKeepsValidators(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/c1"})
	var got atomic.Bool
	srv := conditionalServer(t, `"same"`, "", &entries, &got)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	itemsBefore := countEnrichJobs(t, s)

	added, err := s.Refresh(ctx, feed)
	if err != nil || added != 0 {
		t.Fatalf("refresh: added=%d err=%v, want 0/nil", added, err)
	}
	if !got.Load() {
		t.Fatal("second poll did not send If-None-Match")
	}
	refetched, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if refetched.LastStatus != "ok" {
		t.Errorf("status = %q, want ok (304 is healthy)", refetched.LastStatus)
	}
	if !refetched.LastPolledAt.Valid || !refetched.LastPolledAt.Time.After(feed.LastPolledAt.Time) {
		t.Error("last_polled_at not stamped on 304")
	}
	if refetched.Etag != `"same"` {
		t.Errorf("etag = %q, want preserved", refetched.Etag)
	}
	if countEnrichJobs(t, s) != itemsBefore {
		t.Error("304 refresh must create no items/jobs")
	}
}

func TestFeedRefreshChangedContentUpdatesValidators(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/n1"})
	// Server never matches If-None-Match (etag arg differs per request cycle):
	// simulate by using a server whose etag is "v2" while the stored one is
	// different — the conditional never matches, so every poll is a 200.
	var got atomic.Bool
	srv := conditionalServer(t, `"v2"`, "", &entries, &got)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	// Force a stale stored validator, then refresh: 200 path must overwrite it.
	// The schedule columns are preserved as-is; this test isn't exercising them.
	if err := s.Store.Queries.SetFeedPolled(ctx, db.SetFeedPolledParams{
		UserID: uid, ID: feed.ID, LastPolledAt: feed.LastPolledAt, LastStatus: "ok",
		Etag: `"stale"`, LastModified: "",
		NextPollAt: feed.NextPollAt, PollIntervalMinutes: feed.PollIntervalMinutes,
	}); err != nil {
		t.Fatalf("stale validators: %v", err)
	}
	feed.Etag = `"stale"`

	entries.Store(&[]string{"https://example.com/n1", "https://example.com/n2"})
	added, err := s.Refresh(ctx, feed)
	if err != nil || added != 1 {
		t.Fatalf("refresh: added=%d err=%v, want 1/nil", added, err)
	}
	refetched, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if refetched.Etag != `"v2"` {
		t.Errorf("etag = %q, want refreshed to v2", refetched.Etag)
	}
}

func TestFeedNoValidatorServerStillWorks(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/p1"})
	srv := rssServer(t, &entries) // plain server: no ETag/Last-Modified, no 304
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if feed.Etag != "" || feed.LastModified != "" {
		t.Errorf("validators = %q/%q, want empty", feed.Etag, feed.LastModified)
	}
	if added, err := s.Refresh(ctx, feed); err != nil || added != 0 {
		t.Fatalf("refresh: %d/%v, want 0/nil", added, err)
	}
}

func TestFeedFetchErrorPreservesValidators(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/e1"})
	var got atomic.Bool
	srv := conditionalServer(t, `"keep"`, "", &entries, &got)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	srv.Close() // next poll fails at transport level

	if added, err := s.Refresh(ctx, feed); err != nil || added != 0 {
		t.Fatalf("refresh after close: %d/%v, want 0/nil (error recorded, not returned)", added, err)
	}
	refetched, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if !strings.HasPrefix(refetched.LastStatus, "error:") {
		t.Errorf("status = %q, want error:…", refetched.LastStatus)
	}
	if refetched.Etag != `"keep"` {
		t.Errorf("etag = %q, want preserved through error", refetched.Etag)
	}
}

// assertNextPollAt checks that a feed's next_poll_at is within a few seconds
// of baseline+want, tolerating the small gap between computing the baseline
// and the service's own time.Now() call inside recordStatus.
func assertNextPollAt(t *testing.T, got pgtype.Timestamptz, baseline time.Time, want time.Duration) {
	t.Helper()
	if !got.Valid {
		t.Fatal("next_poll_at not set")
	}
	wantAt := baseline.Add(want)
	if diff := got.Time.Sub(wantAt); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("next_poll_at = %v, want ~%v (diff %v)", got.Time, wantAt, diff)
	}
}

func TestFeedAddSetsInitialSchedule(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	before := time.Now()
	feed, added, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}

	got, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if got.PollIntervalMinutes != 30 {
		t.Errorf("poll_interval_minutes = %d, want 30", got.PollIntervalMinutes)
	}
	assertNextPollAt(t, got.NextPollAt, before, 30*time.Minute)
}

func TestFeedAddZeroBackfillStartsAtFloor(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	// Feed publishes no entries at all: backfill adds nothing.
	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	before := time.Now()
	feed, added, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added != 0 {
		t.Fatalf("added = %d, want 0", added)
	}

	got, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	// Subscribing is a user signal of interest, so even a zero-entry backfill
	// must start at the 30-minute floor, not double to 60 (added==0 is only
	// meaningful for Refresh's steady-state backoff).
	if got.PollIntervalMinutes != 30 {
		t.Errorf("poll_interval_minutes = %d, want 30 (subscribe always resets to the floor)", got.PollIntervalMinutes)
	}
	assertNextPollAt(t, got.NextPollAt, before, 30*time.Minute)
}

func TestFeedRefreshDoublesIntervalWhenUnchanged(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	before := time.Now()
	if added, err := s.Refresh(ctx, feed); err != nil || added != 0 {
		t.Fatalf("refresh: added=%d err=%v, want 0/nil", added, err)
	}

	got, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if got.PollIntervalMinutes != 60 {
		t.Errorf("poll_interval_minutes = %d, want 60 (doubled from 30)", got.PollIntervalMinutes)
	}
	assertNextPollAt(t, got.NextPollAt, before, 60*time.Minute)
}

func TestFeedRefreshResetsIntervalOnNewItems(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := rssServer(t, &entries)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	// Unchanged refresh doubles the interval to 60m first.
	if _, err := s.Refresh(ctx, feed); err != nil {
		t.Fatalf("refresh (unchanged): %v", err)
	}
	doubled, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if doubled.PollIntervalMinutes != 60 {
		t.Fatalf("poll_interval_minutes = %d, want 60 before the reset case", doubled.PollIntervalMinutes)
	}

	// A new entry must reset the interval back to the floor.
	entries.Store(&[]string{"https://example.com/a", "https://example.com/b"})
	before := time.Now()
	if added, err := s.Refresh(ctx, doubled); err != nil || added != 1 {
		t.Fatalf("refresh (changed): added=%d err=%v, want 1/nil", added, err)
	}

	got, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if got.PollIntervalMinutes != 30 {
		t.Errorf("poll_interval_minutes = %d, want 30 (reset on new items)", got.PollIntervalMinutes)
	}
	assertNextPollAt(t, got.NextPollAt, before, 30*time.Minute)
}

// cacheControlServer serves a plain RSS document that always returns 200 with
// the given Cache-Control header, so the test can assert the header floors
// the adaptive interval even when the poll itself is otherwise unchanged.
func cacheControlServer(t *testing.T, cacheControl string, entries *atomic.Pointer[[]string]) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Cache-Control", cacheControl)
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel>`)
		b.WriteString(`<title>Cache Feed</title><link>https://example.com</link>`)
		for i, u := range *entries.Load() {
			fmt.Fprintf(&b, `<item><title>Entry %d</title><link>%s</link></item>`, i, u)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFeedRefreshCacheControlFloorsInterval(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	uid := newUser(t, s)

	var entries atomic.Pointer[[]string]
	entries.Store(&[]string{"https://example.com/a"})
	srv := cacheControlServer(t, "max-age=7200", &entries)
	s.HTTPClient = srv.Client()

	feed, _, err := s.Add(ctx, uid, srv.URL)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	// Unchanged poll would normally double 30m -> 60m, but the origin's
	// 7200s (120m) max-age is a hard floor that beats the doubling.
	before := time.Now()
	if added, err := s.Refresh(ctx, feed); err != nil || added != 0 {
		t.Fatalf("refresh: added=%d err=%v, want 0/nil", added, err)
	}

	got, err := s.Store.Queries.GetFeed(ctx, db.GetFeedParams{UserID: uid, ID: feed.ID})
	if err != nil {
		t.Fatalf("get feed: %v", err)
	}
	if got.PollIntervalMinutes < 120 {
		t.Errorf("poll_interval_minutes = %d, want >= 120 (cache floor beats doubling)", got.PollIntervalMinutes)
	}
	assertNextPollAt(t, got.NextPollAt, before, 120*time.Minute)
}
