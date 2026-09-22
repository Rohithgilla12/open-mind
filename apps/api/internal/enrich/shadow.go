package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// JevClient is the subset of *jev.Client the capture path needs. Tests inject
// an httptest-backed client; production uses jev.FromEnv().
type JevClient interface {
	Evaluate(ctx context.Context, state any, questions map[string]jev.Question) (*jev.Result, error)
}

// liveCapture runs Phase 2 Jev evaluation after the item is already saved.
// On success it applies high-confidence tags, stores mid-confidence suggestions
// on the decision row, and may override card_type when the mapping is clean.
// Failures and skips are logged and swallowed so enrichment continues.
//
// Gates (both required): a non-nil Jev client with an API key, and the user's
// "AI-assisted organisation" setting. Without both, no call and no row.
func (p *Pipeline) liveCapture(ctx context.Context, userID, itemID uuid.UUID, itemURL, title, body string) {
	if p.Jev == nil {
		return
	}
	q := p.Store.Queries

	// Idempotent: one capture decision per item. A prior row means a re-run
	// of enrich must not call Jev again, re-apply tags, or insert another log.
	if _, err := q.GetJevCaptureDecision(ctx, db.GetJevCaptureDecisionParams{
		UserID: userID, ItemID: uuidToPgtype(itemID),
	}); err == nil {
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("jev capture: checking existing decision", "item_id", itemID, "err", err)
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
		slog.Warn("jev capture: reading setting", "item_id", itemID, "err", err)
		return
	}
	if !enabled {
		return
	}

	tags, err := q.ListUserTagVocabulary(ctx, userID)
	if err != nil {
		slog.Warn("jev capture: loading tag vocabulary", "item_id", itemID, "err", err)
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
			slog.Info("jev capture: skipped", "item_id", itemID, "err", evalErr)
		} else {
			slog.Warn("jev capture: evaluate failed", "item_id", itemID, "err", evalErr)
		}
		p.insertJevDecision(ctx, userID, itemID, jev.DefaultModel, []byte("{}"), string(jev.ActionSkipped), latency, 0, nil, nil)
		return
	}

	answersRaw, err := json.Marshal(result.Answers)
	if err != nil {
		slog.Warn("jev capture: encoding answers", "item_id", itemID, "err", err)
		p.insertJevDecision(ctx, userID, itemID, result.Model, []byte("{}"), string(jev.ActionSkipped), latency, result.Usage.InputTokens, nil, nil)
		return
	}

	plan := jev.PlanCapture(result.Answers)
	applied := canonUserTags(plan.AppliedTags)
	suggested := canonUserTags(plan.SuggestedTags)
	p.applyCapturePlan(ctx, userID, itemID, jev.CapturePlan{
		AppliedTags:   applied,
		SuggestedTags: suggested,
		CardType:      plan.CardType,
		Action:        plan.Action,
	})
	p.insertJevDecision(ctx, userID, itemID, result.Model, answersRaw, string(plan.Action), latency, result.Usage.InputTokens, applied, suggested)
}

// applyCapturePlan mutates the item for auto-applied tags and an optional
// card_type override. Mid-confidence suggestions stay on the decision row only.
// Tags in plan are expected to already be canonical.
func (p *Pipeline) applyCapturePlan(ctx context.Context, userID, itemID uuid.UUID, plan jev.CapturePlan) {
	q := p.Store.Queries
	if len(plan.AppliedTags) == 0 && plan.CardType == "" {
		return
	}

	item, err := q.GetItem(ctx, db.GetItemParams{UserID: userID, ID: itemID})
	if err != nil {
		slog.Warn("jev capture: loading item to apply", "item_id", itemID, "err", err)
		return
	}

	if len(plan.AppliedTags) > 0 {
		merged := jev.MergeTags(item.UserTags, plan.AppliedTags)
		if _, err := q.SetUserTags(ctx, db.SetUserTagsParams{
			UserID: userID, ID: itemID, UserTags: merged,
		}); err != nil {
			slog.Warn("jev capture: applying tags", "item_id", itemID, "err", err)
		}
	}

	// Only override the generic "article" default — never stomp note/image/
	// video/repo/quote heuristics that Classify already set.
	if plan.CardType != "" && item.CardType == "article" && plan.CardType != "article" {
		if _, err := q.SetItemCardType(ctx, db.SetItemCardTypeParams{
			UserID: userID, ID: itemID, CardType: plan.CardType,
		}); err != nil {
			slog.Warn("jev capture: setting card type", "item_id", itemID, "err", err)
		}
	}
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

func (p *Pipeline) insertJevDecision(
	ctx context.Context,
	userID, itemID uuid.UUID,
	model string,
	answers []byte,
	action string,
	latencyMs, inputTokens int,
	applied, suggested []string,
) {
	if answers == nil {
		answers = []byte("{}")
	}
	if model == "" {
		model = jev.DefaultModel
	}
	if applied == nil {
		applied = []string{}
	}
	if suggested == nil {
		suggested = []string{}
	}
	params := db.InsertJevDecisionParams{
		UserID:        userID,
		ItemID:        uuidToPgtype(itemID),
		Surface:       jev.SurfaceCapture,
		Model:         model,
		QuestionsV:    jev.QuestionsVersion,
		Answers:       answers,
		Action:        action,
		AppliedTags:   applied,
		SuggestedTags: suggested,
		DismissedTags: []string{},
	}
	if latencyMs > 0 {
		params.LatencyMs = pgtype.Int4{Int32: int32(latencyMs), Valid: true}
	}
	if inputTokens > 0 {
		params.InputTokens = pgtype.Int4{Int32: int32(inputTokens), Valid: true}
	}
	if _, err := p.Store.Queries.InsertJevDecision(ctx, params); err != nil {
		slog.Warn("jev capture: inserting decision", "item_id", itemID, "action", action, "err", err)
	}
}

// canonUserTags mirrors api.canonicalTags (trim, lower, dedupe, rune/count caps)
// so the enrich package does not import api.
func canonUserTags(in []string) []string {
	const maxTags, maxRunes = 30, 50
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxRunes {
			tag = string([]rune(tag)[:maxRunes])
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
		if len(out) == maxTags {
			break
		}
	}
	return out
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
