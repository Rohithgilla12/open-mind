package jobs_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/reelmedia"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

var driftScoreTestUser = uuid.MustParse("00000000-0000-0000-0000-0000000000d4")

func newDriftScoreTestStore(t *testing.T) *store.Store {
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
	if _, err := pool.Exec(ctx, `TRUNCATE items, item_embeddings, jev_decisions, user_settings, river_job CASCADE`); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	s := store.New(pool)
	if err := s.Queries.EnsureUser(ctx, driftScoreTestUser); err != nil {
		t.Fatalf("ensure user: %v", err)
	}
	return s
}

func enableDriftAI(t *testing.T, s *store.Store) {
	t.Helper()
	if err := s.Queries.UpsertUserSetting(context.Background(), db.UpsertUserSettingParams{
		UserID: driftScoreTestUser,
		Key:    jev.SettingKeyAIAssisted,
		Value:  "true",
	}); err != nil {
		t.Fatalf("setting: %v", err)
	}
}

func seedDriftCandidate(t *testing.T, s *store.Store, title, body string, created time.Time) db.Item {
	t.Helper()
	ctx := context.Background()
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{
		UserID: driftScoreTestUser,
		Url:    "https://example.com/" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `
		UPDATE items SET title=$2, body=$3, status='enriched', card_type='note',
		       created_at=$4, pinned_at=NULL, last_drifted_at=NULL
		WHERE id=$1`, item.ID, title, body, created); err != nil {
		t.Fatalf("ready: %v", err)
	}
	item, err = s.Queries.GetItem(ctx, db.GetItemParams{UserID: driftScoreTestUser, ID: item.ID})
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return item
}

func TestScanDriftScores_NoopWithoutJev(t *testing.T) {
	s := newDriftScoreTestStore(t)
	enableDriftAI(t, s)
	rc, err := jobs.NewRiverClient(s.Pool, &enrich.Pipeline{Store: s}, nil, jobs.KindleDeps{}, jobs.NotifyDeps{}, nil, reelmedia.ModeThumbnail, nil, false)
	if err != nil {
		t.Fatalf("river: %v", err)
	}
	w := &jobs.ScanDriftScoresWorker{Store: s, Jev: nil, River: rc}
	if err := w.Work(context.Background(), &river.Job[jobs.ScanDriftScoresArgs]{}); err != nil {
		t.Fatalf("work: %v", err)
	}
	var n int
	if err := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM river_job WHERE kind=$1`, jobs.ScoreDriftArgs{}.Kind()).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("enqueued %d jobs with nil Jev", n)
	}
}

func TestScanDriftScores_EnqueuesOptedInUsers(t *testing.T) {
	s := newDriftScoreTestStore(t)
	enableDriftAI(t, s)
	rc, err := jobs.NewRiverClient(s.Pool, &enrich.Pipeline{Store: s}, nil, jobs.KindleDeps{}, jobs.NotifyDeps{}, nil, reelmedia.ModeThumbnail, nil, false)
	if err != nil {
		t.Fatalf("river: %v", err)
	}
	w := &jobs.ScanDriftScoresWorker{Store: s, Jev: fakeJev{}, River: rc}
	if err := w.Work(context.Background(), &river.Job[jobs.ScanDriftScoresArgs]{}); err != nil {
		t.Fatalf("work: %v", err)
	}
	var n int
	var raw []byte
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*), (SELECT args FROM river_job WHERE kind=$1 LIMIT 1) FROM river_job WHERE kind=$1`,
		jobs.ScoreDriftArgs{}.Kind()).Scan(&n, &raw); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("enqueued %d, want 1", n)
	}
	var args jobs.ScoreDriftArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if args.UserID != driftScoreTestUser {
		t.Fatalf("user = %s", args.UserID)
	}
}

func TestScoreDrift_HttptestReorderAndSkip(t *testing.T) {
	s := newDriftScoreTestStore(t)
	enableDriftAI(t, s)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	low := seedDriftCandidate(t, s, "old-low", "Unrelated stale note.", now.Add(-200*24*time.Hour))
	high := seedDriftCandidate(t, s, "old-high", "Still actionable postgres guide.", now.Add(-100*24*time.Hour))

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			State struct {
				Item struct {
					Title   string `json:"title"`
					Excerpt string `json:"excerpt"`
				} `json:"item"`
				RecentActivity string `json:"recent_activity"`
			} `json:"state"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, "bad", 400)
			return
		}
		if req.State.RecentActivity == "" {
			t.Error("empty recent_activity")
		}
		if len([]rune(req.State.Item.Excerpt)) > jev.ExcerptMaxChars {
			t.Errorf("excerpt too long")
		}
		worth, action := 0.1, 0.1
		if req.State.Item.Title == "old-high" {
			worth, action = 0.95, 0.9
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-1.13.0",
			"answers": map[string]any{
				"worth_resurfacing": map[string]any{"type": "noul", "noul": worth},
				"still_actionable":  map[string]any{"type": "noul", "noul": action},
			},
			"usage": map[string]any{"input_tokens": 120, "output_tokens": 10},
		})
	}))
	defer srv.Close()

	client := jev.New("test", jev.WithBaseURL(srv.URL), jev.WithHTTPClient(srv.Client()))
	w := &jobs.ScoreDriftWorker{Store: s, Jev: client, Now: func() time.Time { return now }}
	if err := w.Work(context.Background(), &river.Job[jobs.ScoreDriftArgs]{
		Args: jobs.ScoreDriftArgs{UserID: driftScoreTestUser},
	}); err != nil {
		t.Fatalf("work: %v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("eval calls = %d, want ≥2", calls.Load())
	}

	highReloaded, err := s.Queries.GetItem(context.Background(), db.GetItemParams{UserID: driftScoreTestUser, ID: high.ID})
	if err != nil {
		t.Fatalf("get high: %v", err)
	}
	lowReloaded, err := s.Queries.GetItem(context.Background(), db.GetItemParams{UserID: driftScoreTestUser, ID: low.ID})
	if err != nil {
		t.Fatalf("get low: %v", err)
	}
	if !highReloaded.DriftScore.Valid || !lowReloaded.DriftScore.Valid {
		t.Fatalf("scores missing: high=%v low=%v", highReloaded.DriftScore, lowReloaded.DriftScore)
	}
	if highReloaded.DriftScore.Float32 <= lowReloaded.DriftScore.Float32 {
		t.Fatalf("high score %v should beat low %v", highReloaded.DriftScore.Float32, lowReloaded.DriftScore.Float32)
	}

	var applied int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM jev_decisions WHERE user_id=$1 AND surface='drift' AND action='applied'`,
		driftScoreTestUser).Scan(&applied); err != nil {
		t.Fatalf("decisions: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied decisions = %d, want 2", applied)
	}

	// Idempotent re-run: still succeeds.
	if err := w.Work(context.Background(), &river.Job[jobs.ScoreDriftArgs]{
		Args: jobs.ScoreDriftArgs{UserID: driftScoreTestUser},
	}); err != nil {
		t.Fatalf("rerun: %v", err)
	}

	// Skip path: server errors → no score on a fresh item.
	skipItem := seedDriftCandidate(t, s, "skip-me", "Will fail evaluate.", now.Add(-50*24*time.Hour))
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failSrv.Close()
	failClient := jev.New("test", jev.WithBaseURL(failSrv.URL), jev.WithHTTPClient(failSrv.Client()))
	wFail := &jobs.ScoreDriftWorker{Store: s, Jev: failClient, Now: func() time.Time { return now }}
	if err := wFail.Work(context.Background(), &river.Job[jobs.ScoreDriftArgs]{
		Args: jobs.ScoreDriftArgs{UserID: driftScoreTestUser},
	}); err != nil {
		t.Fatalf("fail work: %v", err)
	}
	skipReloaded, err := s.Queries.GetItem(context.Background(), db.GetItemParams{UserID: driftScoreTestUser, ID: skipItem.ID})
	if err != nil {
		t.Fatalf("get skip: %v", err)
	}
	if skipReloaded.DriftScore.Valid {
		t.Fatalf("skip should leave score unset, got %v", skipReloaded.DriftScore.Float32)
	}
	var skipped int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM jev_decisions WHERE user_id=$1 AND item_id=$2 AND surface='drift' AND action='skipped'`,
		driftScoreTestUser, skipItem.ID).Scan(&skipped); err != nil {
		t.Fatalf("skipped count: %v", err)
	}
	if skipped == 0 {
		t.Fatal("expected skipped decision row")
	}
}

func TestScoreDrift_NoopWithoutSetting(t *testing.T) {
	s := newDriftScoreTestStore(t)
	now := time.Now().UTC()
	seedDriftCandidate(t, s, "alone", "body", now.Add(-40*24*time.Hour))
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{},"usage":{}}`))
	}))
	defer srv.Close()
	client := jev.New("test", jev.WithBaseURL(srv.URL), jev.WithHTTPClient(srv.Client()))
	w := &jobs.ScoreDriftWorker{Store: s, Jev: client}
	if err := w.Work(context.Background(), &river.Job[jobs.ScoreDriftArgs]{
		Args: jobs.ScoreDriftArgs{UserID: driftScoreTestUser},
	}); err != nil {
		t.Fatalf("work: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("Evaluate called %d times without opt-in", calls.Load())
	}
}

type fakeJev struct{}

func (fakeJev) Evaluate(context.Context, any, map[string]jev.Question) (*jev.Result, error) {
	return nil, jev.ErrDisabled
}
