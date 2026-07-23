package jobs_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

var digestScanTestUser = uuid.MustParse("00000000-0000-0000-0000-0000000000bb")

// newDigestScanTestStore connects to the test Postgres, migrates, truncates
// the tables this suite touches (including river_job, so job counts start at
// zero), and provisions the test user plus an insert-only River client the
// scan worker can enqueue send_kindle jobs through.
func newDigestScanTestStore(t *testing.T) (*store.Store, *river.Client[pgx.Tx]) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, item_embeddings, lenses, river_job CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, digestScanTestUser); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	rc, err := jobs.NewRiverClient(pool, nil, nil, jobs.KindleDeps{}, nil, reelmedia.ModeThumbnail, nil, false)
	if err != nil {
		t.Fatalf("river client: %v", err)
	}
	return s, rc
}

func newDigestScanItem(t *testing.T, s *store.Store, title, body string) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: digestScanTestUser, Url: "https://example.com/" + title})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := s.Queries.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: digestScanTestUser, ID: item.ID, Title: title, Body: body, CardType: "note",
	}); err != nil {
		t.Fatalf("update extraction: %v", err)
	}
	item, err = s.Queries.GetItem(ctx, db.GetItemParams{UserID: digestScanTestUser, ID: item.ID})
	if err != nil {
		t.Fatalf("reload item: %v", err)
	}
	return item
}

// sendKindleJobArgs is the subset of send_kindle args this suite inspects.
type sendKindleJobArgs struct {
	ItemIDs []uuid.UUID `json:"item_ids"`
}

func listSendKindleJobs(t *testing.T, s *store.Store) []sendKindleJobArgs {
	t.Helper()
	rows, err := s.Pool.Query(context.Background(), `SELECT args FROM river_job WHERE kind = $1`, jobs.SendKindleArgs{}.Kind())
	if err != nil {
		t.Fatalf("querying send_kindle jobs: %v", err)
	}
	defer rows.Close()
	var out []sendKindleJobArgs
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		var a sendKindleJobArgs
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatalf("decoding args: %v", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating rows: %v", err)
	}
	return out
}

func createDigestLens(t *testing.T, s *store.Store, name, schedule string) db.Lense {
	t.Helper()
	ctx := context.Background()
	lens, err := s.Queries.CreateLens(ctx, db.CreateLensParams{UserID: digestScanTestUser, Name: name, Rule: []byte(`{"types":["note"]}`)})
	if err != nil {
		t.Fatalf("create lens: %v", err)
	}
	if _, err := s.Queries.UpdateLensDigestSchedule(ctx, db.UpdateLensDigestScheduleParams{UserID: digestScanTestUser, ID: lens.ID, DigestSchedule: schedule}); err != nil {
		t.Fatalf("set schedule: %v", err)
	}
	return lens
}

// digestScanDeps is the KindleDeps used by runDigestScan's default,
// deliverable configuration: a configured transport with a server-wide
// fallback recipient, so existing tests that don't care about delivery keep
// passing unchanged. Tests exercising the delivery gate call
// runDigestScanWithDeps directly with a different KindleDeps.
var digestScanDeps = jobs.KindleDeps{To: "fallback@kindle.com", Configured: true}

func runDigestScan(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx]) {
	t.Helper()
	runDigestScanWithDeps(t, s, rc, digestScanDeps)
}

func runDigestScanWithDeps(t *testing.T, s *store.Store, rc *river.Client[pgx.Tx], deps jobs.KindleDeps) {
	t.Helper()
	w := &jobs.ScanDigestsWorker{Store: s, Provider: ai.NewNoop(), River: rc, Deps: deps}
	if err := w.Work(context.Background(), &river.Job[jobs.ScanDigestsArgs]{Args: jobs.ScanDigestsArgs{}}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// TestScanDigestsEnqueuesNewItemsOnly seeds a daily lens whose last digest
// was 2 days ago, with one item created well before that stamp (2h earlier,
// outside the 1h enrichment-grace window), one created 30min before the
// stamp (inside the grace window — a late-enriched item), and one created
// after. The grace-window and post-stamp items should end up in the enqueued
// send_kindle job, the old one must not, and the stamp should advance.
func TestScanDigestsEnqueuesNewItemsOnly(t *testing.T) {
	s, rc := newDigestScanTestStore(t)
	ctx := context.Background()

	pre := newDigestScanItem(t, s, "pre-stamp", "old content")
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = now() - interval '2 days 2 hours' WHERE id = $1`, pre.ID); err != nil {
		t.Fatalf("backdating pre-stamp item: %v", err)
	}
	grace := newDigestScanItem(t, s, "grace-window", "late-enriched content")
	if _, err := s.Pool.Exec(ctx, `UPDATE items SET created_at = now() - interval '2 days' - interval '30 minutes' WHERE id = $1`, grace.ID); err != nil {
		t.Fatalf("backdating grace item: %v", err)
	}
	lens := createDigestLens(t, s, "Daily notes", "daily")
	if _, err := s.Pool.Exec(ctx, `UPDATE lenses SET last_digest_at = now() - interval '2 days' WHERE id = $1`, lens.ID); err != nil {
		t.Fatalf("backdating stamp: %v", err)
	}
	post := newDigestScanItem(t, s, "post-stamp", "new content")

	runDigestScan(t, s, rc)

	found := listSendKindleJobs(t, s)
	if len(found) != 1 {
		t.Fatalf("send_kindle jobs = %d, want 1", len(found))
	}
	got := map[uuid.UUID]bool{}
	for _, id := range found[0].ItemIDs {
		got[id] = true
	}
	if len(got) != 2 || !got[post.ID] || !got[grace.ID] {
		t.Errorf("item_ids = %v, want exactly {post %v, grace %v} (pre %v excluded)", found[0].ItemIDs, post.ID, grace.ID, pre.ID)
	}

	updated, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: lens.ID})
	if err != nil {
		t.Fatalf("reload lens: %v", err)
	}
	if !updated.LastDigestAt.Valid || time.Since(updated.LastDigestAt.Time) > time.Minute {
		t.Errorf("last_digest_at not advanced to ~now: %+v", updated.LastDigestAt)
	}
}

// TestScanDigestsSkipsEmptyWithoutStamp asserts a due lens with no matching
// items produces no job and leaves last_digest_at untouched, so a quiet lens
// doesn't drift its own due-ness.
func TestScanDigestsSkipsEmptyWithoutStamp(t *testing.T) {
	s, rc := newDigestScanTestStore(t)
	ctx := context.Background()

	lens := createDigestLens(t, s, "Empty notes", "daily")

	runDigestScan(t, s, rc)

	if found := listSendKindleJobs(t, s); len(found) != 0 {
		t.Fatalf("send_kindle jobs = %d, want 0", len(found))
	}
	updated, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: lens.ID})
	if err != nil {
		t.Fatalf("reload lens: %v", err)
	}
	if updated.LastDigestAt.Valid {
		t.Errorf("last_digest_at = %+v, want unset (never stamped)", updated.LastDigestAt)
	}
}

// TestScanDigestsIdempotentWithinHour runs the scan twice in a row: the
// first run enqueues the one matching item and stamps the lens, so the
// second run — with no new items since that stamp — must not enqueue again.
func TestScanDigestsIdempotentWithinHour(t *testing.T) {
	s, rc := newDigestScanTestStore(t)

	newDigestScanItem(t, s, "one", "content")
	createDigestLens(t, s, "Notes", "daily")

	runDigestScan(t, s, rc)
	runDigestScan(t, s, rc)

	if found := listSendKindleJobs(t, s); len(found) != 1 {
		t.Fatalf("send_kindle jobs = %d, want 1 (second run sees nothing new)", len(found))
	}
}

// TestScanDigestsSkipsWhenTransportUnconfigured asserts that a due lens with
// matching items is skipped entirely (no send_kindle job, no stamp) when
// KindleDeps.Configured is false — the transport being down must never
// permanently exclude that window's items from a future digest.
func TestScanDigestsSkipsWhenTransportUnconfigured(t *testing.T) {
	s, rc := newDigestScanTestStore(t)
	ctx := context.Background()

	newDigestScanItem(t, s, "one", "content")
	lens := createDigestLens(t, s, "Notes", "daily")

	runDigestScanWithDeps(t, s, rc, jobs.KindleDeps{Configured: false})

	if found := listSendKindleJobs(t, s); len(found) != 0 {
		t.Fatalf("send_kindle jobs = %d, want 0 (transport unconfigured)", len(found))
	}
	updated, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: lens.ID})
	if err != nil {
		t.Fatalf("reload lens: %v", err)
	}
	if updated.LastDigestAt.Valid {
		t.Errorf("last_digest_at = %+v, want unset (never stamped when transport unconfigured)", updated.LastDigestAt)
	}
}

// TestScanDigestsSkipsWhenNoRecipient asserts that a due lens with matching
// items is skipped (no send_kindle job, no stamp) when the transport is
// configured but the lens's user has no resolvable recipient — neither a
// kindle_email user setting nor a server-wide fallback (Deps.To).
func TestScanDigestsSkipsWhenNoRecipient(t *testing.T) {
	s, rc := newDigestScanTestStore(t)
	ctx := context.Background()

	newDigestScanItem(t, s, "one", "content")
	lens := createDigestLens(t, s, "Notes", "daily")

	runDigestScanWithDeps(t, s, rc, jobs.KindleDeps{Configured: true})

	if found := listSendKindleJobs(t, s); len(found) != 0 {
		t.Fatalf("send_kindle jobs = %d, want 0 (no deliverable recipient)", len(found))
	}
	updated, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: lens.ID})
	if err != nil {
		t.Fatalf("reload lens: %v", err)
	}
	if updated.LastDigestAt.Valid {
		t.Errorf("last_digest_at = %+v, want unset (never stamped without a recipient)", updated.LastDigestAt)
	}
}

// TestScanDigestsIsolatesBrokenLens seeds a lens with a corrupt stored rule
// (invalid JSON) alongside a normal, valid lens for the same scan. The
// broken lens must fail and be skipped without aborting the run — the valid
// lens still gets its item enqueued and its stamp advanced, proving Work's
// per-lens error isolation (a bad rule for one lens must never block
// digests for every other lens).
func TestScanDigestsIsolatesBrokenLens(t *testing.T) {
	s, rc := newDigestScanTestStore(t)
	ctx := context.Background()

	broken := createDigestLens(t, s, "Broken", "daily")
	// Valid JSON (jsonb rejects anything else at insert time) but a type
	// mismatch against kindleLensRule.Q (string), so json.Unmarshal fails.
	if _, err := s.Pool.Exec(ctx, `UPDATE lenses SET rule = '{"q": 123}' WHERE id = $1`, broken.ID); err != nil {
		t.Fatalf("corrupting rule: %v", err)
	}

	newDigestScanItem(t, s, "one", "content")
	valid := createDigestLens(t, s, "Valid", "daily")

	runDigestScan(t, s, rc)

	if found := listSendKindleJobs(t, s); len(found) != 1 {
		t.Fatalf("send_kindle jobs = %d, want 1 (only the valid lens)", len(found))
	}

	updatedValid, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: valid.ID})
	if err != nil {
		t.Fatalf("reload valid lens: %v", err)
	}
	if !updatedValid.LastDigestAt.Valid {
		t.Errorf("valid lens last_digest_at not stamped despite broken sibling lens")
	}

	updatedBroken, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: broken.ID})
	if err != nil {
		t.Fatalf("reload broken lens: %v", err)
	}
	if updatedBroken.LastDigestAt.Valid {
		t.Errorf("broken lens last_digest_at = %+v, want unset (never processed successfully)", updatedBroken.LastDigestAt)
	}
}

// TestScanDigestsProceedsWithUserKindleEmail asserts that a due lens for a
// user with a kindle_email setting proceeds to enqueue+stamp even with no
// server-wide fallback configured — the per-user setting alone is enough to
// make the lens deliverable.
func TestScanDigestsProceedsWithUserKindleEmail(t *testing.T) {
	s, rc := newDigestScanTestStore(t)
	ctx := context.Background()

	newDigestScanItem(t, s, "one", "content")
	lens := createDigestLens(t, s, "Notes", "daily")
	if err := s.Queries.UpsertUserSetting(ctx, db.UpsertUserSettingParams{
		UserID: digestScanTestUser, Key: "kindle_email", Value: "personal@kindle.com",
	}); err != nil {
		t.Fatalf("set kindle_email: %v", err)
	}

	runDigestScanWithDeps(t, s, rc, jobs.KindleDeps{Configured: true})

	if found := listSendKindleJobs(t, s); len(found) != 1 {
		t.Fatalf("send_kindle jobs = %d, want 1", len(found))
	}
	updated, err := s.Queries.GetLens(ctx, db.GetLensParams{UserID: digestScanTestUser, ID: lens.ID})
	if err != nil {
		t.Fatalf("reload lens: %v", err)
	}
	if !updated.LastDigestAt.Valid {
		t.Errorf("last_digest_at not stamped, want stamped after successful enqueue")
	}
}
