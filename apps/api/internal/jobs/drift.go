package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// driftScanInterval is how often the periodic scan_drift_scores job runs. It
// is hourly so a daily scoring cadence (see driftScoreDue) is checked often
// enough without missing a day if one tick slips.
const driftScanInterval = time.Hour

// driftScoreMaxAttempts caps River retries for per-user score_drift jobs.
const driftScoreMaxAttempts = 3

// JevClient is the subset of *jev.Client the Drift scoring job needs.
type JevClient interface {
	Evaluate(ctx context.Context, state any, questions map[string]jev.Question) (*jev.Result, error)
}

// ScanDriftScoresArgs is the River periodic job that fans out ScoreDriftArgs
// for every user with AI-assisted organisation enabled. Payload is empty;
// fresh state is loaded inside Work.
type ScanDriftScoresArgs struct{}

// Kind identifies the job type in River.
func (ScanDriftScoresArgs) Kind() string { return "scan_drift_scores" }

// ScanDriftScoresWorker lists opted-in users and enqueues one score_drift job
// per user. A missing Jev client is a no-op (feature off). Individual enqueue
// failures are logged and skipped so one bad user never blocks the scan.
type ScanDriftScoresWorker struct {
	river.WorkerDefaults[ScanDriftScoresArgs]
	Store *store.Store
	Jev   JevClient
	River *river.Client[pgx.Tx]
}

// Work enqueues per-user Drift scoring jobs when Jev is configured.
func (w *ScanDriftScoresWorker) Work(ctx context.Context, _ *river.Job[ScanDriftScoresArgs]) error {
	if w.Jev == nil {
		return nil
	}
	users, err := w.Store.Queries.ListUsersWithAIAssisted(ctx)
	if err != nil {
		return fmt.Errorf("scan_drift_scores: listing users: %w", err)
	}
	for _, uid := range users {
		if _, err := w.River.Insert(ctx, ScoreDriftArgs{UserID: uid}, &river.InsertOpts{
			MaxAttempts: driftScoreMaxAttempts,
		}); err != nil {
			slog.Error("scan_drift_scores: enqueueing score_drift", "user_id", uid, "err", err)
			continue
		}
	}
	return nil
}

// ScoreDriftArgs scores one user's Drift candidates. Payload is the user id
// only — candidates and recent activity are fetched fresh inside the job.
type ScoreDriftArgs struct {
	UserID uuid.UUID `json:"user_id"`
}

// Kind identifies the job type in River.
func (ScoreDriftArgs) Kind() string { return "score_drift" }

// ScoreDriftWorker evaluates DriftQuestions for up to DriftScorePool
// candidates, stores blend scores on the items, and logs jev_decisions with
// surface=drift. Skip / API errors leave existing scores (and thus GET /drift
// heuristic order) untouched for that item.
type ScoreDriftWorker struct {
	river.WorkerDefaults[ScoreDriftArgs]
	Store *store.Store
	Jev   JevClient
	// Now is optional; tests inject a fixed clock. Production uses time.Now.
	Now func() time.Time
}

// Work scores Drift candidates for args.UserID. Idempotent: re-running
// overwrites drift_score / inserts another decision row. Safe to retry.
func (w *ScoreDriftWorker) Work(ctx context.Context, job *river.Job[ScoreDriftArgs]) error {
	if w.Jev == nil {
		return nil
	}
	uid := job.Args.UserID
	if uid == uuid.Nil {
		return fmt.Errorf("score_drift: missing user_id")
	}

	enabled, err := w.aiAssistedEnabled(ctx, uid)
	if err != nil {
		return fmt.Errorf("score_drift: reading setting: %w", err)
	}
	if !enabled {
		return nil
	}

	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}

	candidates, err := w.Store.Queries.ListDriftCandidates(ctx, db.ListDriftCandidatesParams{
		UserID: uid,
		Limit:  int32(jev.DriftScorePool),
	})
	if err != nil {
		return fmt.Errorf("score_drift: listing candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}

	activity, err := w.recentActivity(ctx, uid, now)
	if err != nil {
		return fmt.Errorf("score_drift: building activity: %w", err)
	}

	questions := jev.DriftQuestions()
	for _, item := range candidates {
		if err := w.scoreOne(ctx, uid, item, activity, questions, now); err != nil {
			// Per-item failures must not fail the whole user job: log and continue
			// so one bad Evaluate does not block the rest of the pool.
			slog.Warn("score_drift: scoring item", "user_id", uid, "item_id", item.ID, "err", err)
		}
	}
	return nil
}

func (w *ScoreDriftWorker) scoreOne(
	ctx context.Context,
	uid uuid.UUID,
	item db.Item,
	activity string,
	questions map[string]jev.Question,
	now time.Time,
) error {
	excerpt := strings.TrimSpace(item.Summary)
	if excerpt == "" {
		excerpt = strings.TrimSpace(item.Body)
	}
	title := strings.TrimSpace(item.Title)
	itemURL := strings.TrimSpace(item.Url)
	if title == "" && excerpt == "" && itemURL == "" {
		return nil
	}

	state := jev.BuildDriftState(jev.DriftItem{
		Title:   title,
		URL:     itemURL,
		Excerpt: excerpt,
	}, activity)

	evalCtx, cancel := context.WithTimeout(ctx, jev.DriftTimeout)
	defer cancel()
	started := time.Now()
	result, evalErr := w.Jev.Evaluate(evalCtx, state, questions)
	latency := int(time.Since(started).Milliseconds())

	if evalErr != nil {
		if jev.Skip(evalErr) {
			slog.Info("score_drift: skipped", "item_id", item.ID, "err", evalErr, "latency_ms", latency)
		} else {
			slog.Warn("score_drift: evaluate failed", "item_id", item.ID, "err", evalErr, "latency_ms", latency)
		}
		w.insertDecision(ctx, uid, item.ID, jev.DefaultModel, []byte("{}"), string(jev.ActionSkipped), latency, 0)
		// Leave drift_score unchanged so GET /drift keeps heuristic order.
		return nil
	}

	answersRaw, err := json.Marshal(result.Answers)
	if err != nil {
		w.insertDecision(ctx, uid, item.ID, result.Model, []byte("{}"), string(jev.ActionSkipped), latency, result.Usage.InputTokens)
		return fmt.Errorf("encoding answers: %w", err)
	}

	created := now
	if item.CreatedAt.Valid {
		created = item.CreatedAt.Time
	}
	score := jev.ScoreDriftCandidate(result.Answers, created, now)
	if err := w.Store.Queries.SetDriftScore(ctx, db.SetDriftScoreParams{
		UserID:     uid,
		ID:         item.ID,
		DriftScore: pgtype.Float4{Float32: float32(score), Valid: true},
	}); err != nil {
		return fmt.Errorf("setting drift score: %w", err)
	}
	w.insertDecision(ctx, uid, item.ID, result.Model, answersRaw, string(jev.ActionApplied), latency, result.Usage.InputTokens)
	return nil
}

func (w *ScoreDriftWorker) recentActivity(ctx context.Context, uid uuid.UUID, now time.Time) (string, error) {
	rows, err := w.Store.Queries.ListRecentSavesForActivity(ctx, db.ListRecentSavesForActivityParams{
		UserID: uid,
		Limit:  int32(jev.ActivitySummaryMaxSaves),
	})
	if err != nil {
		return "", err
	}
	saves := make([]jev.RecentSave, 0, len(rows))
	for _, r := range rows {
		site := ""
		if r.UrlHost.Valid {
			site = r.UrlHost.String
		}
		if site == "" {
			site = hostFromURL(r.Url)
		}
		created := time.Time{}
		if r.CreatedAt.Valid {
			created = r.CreatedAt.Time
		}
		saves = append(saves, jev.RecentSave{
			Title:    r.Title,
			Site:     site,
			CardType: r.CardType,
			Created:  created,
		})
	}
	return jev.FormatRecentActivity(saves, now), nil
}

func (w *ScoreDriftWorker) aiAssistedEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	val, err := w.Store.Queries.GetUserSetting(ctx, db.GetUserSettingParams{
		UserID: userID,
		Key:    jev.SettingKeyAIAssisted,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(val), "true"), nil
}

func (w *ScoreDriftWorker) insertDecision(
	ctx context.Context,
	userID, itemID uuid.UUID,
	model string,
	answers []byte,
	action string,
	latencyMs, inputTokens int,
) {
	if answers == nil {
		answers = []byte("{}")
	}
	if model == "" {
		model = jev.DefaultModel
	}
	params := db.InsertJevDecisionParams{
		UserID:        userID,
		ItemID:        pgtype.UUID{Bytes: itemID, Valid: true},
		Surface:       jev.SurfaceDrift,
		Model:         model,
		QuestionsV:    jev.QuestionsVersion,
		Answers:       answers,
		Action:        action,
		AppliedTags:   []string{},
		SuggestedTags: []string{},
		DismissedTags: []string{},
	}
	if latencyMs > 0 {
		params.LatencyMs = pgtype.Int4{Int32: int32(latencyMs), Valid: true}
	}
	if inputTokens > 0 {
		params.InputTokens = pgtype.Int4{Int32: int32(inputTokens), Valid: true}
	}
	if _, err := w.Store.Queries.InsertJevDecision(ctx, params); err != nil {
		slog.Warn("score_drift: inserting decision", "item_id", itemID, "action", action, "err", err)
	}
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
