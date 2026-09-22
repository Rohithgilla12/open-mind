package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// JevClient is the subset of *jev.Client the shadow path needs. Tests inject
// an httptest-backed client; production uses jev.FromEnv().
type JevClient interface {
	Evaluate(ctx context.Context, state any, questions map[string]jev.Question) (*jev.Result, error)
}

// shadowCapture runs Phase 1 Jev evaluation after the item is already saved.
// It never mutates the item: answers are logged to jev_decisions only.
// Failures and skips are logged and swallowed so enrichment continues.
//
// Gates (both required): a non-nil Jev client with an API key, and the user's
// "AI-assisted organisation" setting. Without both, no call and no row.
func (p *Pipeline) shadowCapture(ctx context.Context, userID, itemID uuid.UUID, itemURL, title, body string) {
	if p.Jev == nil {
		return
	}
	q := p.Store.Queries

	// Idempotent: one capture decision per item. A prior row means a re-run
	// of enrich must not call Jev again or insert another log line.
	if _, err := q.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: userID, ItemID: uuidToPgtype(itemID),
	}); err == nil {
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("jev shadow: checking existing decision", "item_id", itemID, "err", err)
		return
	}

	excerpt := strings.TrimSpace(body)
	title = strings.TrimSpace(title)
	itemURL = strings.TrimSpace(itemURL)
	if title == "" && excerpt == "" && itemURL == "" {
		return
	}

	enabled, err := p.aiAssistedEnabled(ctx, userID)
	if err != nil {
		slog.Warn("jev shadow: reading setting", "item_id", itemID, "err", err)
		return
	}
	if !enabled {
		return
	}

	tags, err := q.ListUserTagVocabulary(ctx, userID)
	if err != nil {
		slog.Warn("jev shadow: loading tag vocabulary", "item_id", itemID, "err", err)
		tags = nil
	}

	state := jev.BuildCaptureState(itemURL, title, siteFromURL(itemURL), excerpt)
	questions := jev.CaptureQuestions(tags)

	evalCtx, cancel := context.WithTimeout(ctx, jev.CaptureTimeout)
	defer cancel()
	started := time.Now()
	result, evalErr := p.Jev.Evaluate(evalCtx, state, questions)
	latency := int(time.Since(started).Milliseconds())

	if evalErr != nil {
		if jev.Skip(evalErr) {
			slog.Info("jev shadow: skipped", "item_id", itemID, "err", evalErr)
			p.insertJevDecision(ctx, userID, itemID, jev.DefaultModel, []byte("{}"), string(jev.ActionSkipped), latency, 0)
			return
		}
		slog.Warn("jev shadow: evaluate failed", "item_id", itemID, "err", evalErr)
		p.insertJevDecision(ctx, userID, itemID, jev.DefaultModel, []byte("{}"), string(jev.ActionSkipped), latency, 0)
		return
	}

	answersRaw, err := json.Marshal(result.Answers)
	if err != nil {
		slog.Warn("jev shadow: encoding answers", "item_id", itemID, "err", err)
		p.insertJevDecision(ctx, userID, itemID, result.Model, []byte("{}"), string(jev.ActionSkipped), latency, result.Usage.InputTokens)
		return
	}

	p.insertJevDecision(ctx, userID, itemID, result.Model, answersRaw, string(jev.ActionShadow), latency, result.Usage.InputTokens)
}

func (p *Pipeline) aiAssistedEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	val, err := p.Store.Queries.GetUserSetting(ctx, db.GetUserSettingParams{
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

func (p *Pipeline) insertJevDecision(ctx context.Context, userID, itemID uuid.UUID, model string, answers []byte, action string, latencyMs, inputTokens int) {
	if answers == nil {
		answers = []byte("{}")
	}
	if model == "" {
		model = jev.DefaultModel
	}
	params := db.InsertJevDecisionParams{
		UserID:     userID,
		ItemID:     uuidToPgtype(itemID),
		Surface:    jev.SurfaceCapture,
		Model:      model,
		QuestionsV: jev.QuestionsVersion,
		Answers:    answers,
		Action:     action,
	}
	if latencyMs > 0 {
		params.LatencyMs = pgtype.Int4{Int32: int32(latencyMs), Valid: true}
	}
	if inputTokens > 0 {
		params.InputTokens = pgtype.Int4{Int32: int32(inputTokens), Valid: true}
	}
	if _, err := p.Store.Queries.InsertJevDecision(ctx, params); err != nil {
		slog.Warn("jev shadow: inserting decision", "item_id", itemID, "action", action, "err", err)
	}
}

func siteFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
