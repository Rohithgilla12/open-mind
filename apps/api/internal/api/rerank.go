package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/jev"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// JevClient is the subset of *jev.Client the retrieval path needs.
type JevClient interface {
	Evaluate(ctx context.Context, state any, questions map[string]jev.Question) (*jev.Result, error)
}

// maybeRerank optionally reorders hybrid results with TypeSafe Jev.
//
// Gates (all required): wantRerank query flag, a configured Jev client, the
// user's AI-assisted organisation setting, and a non-empty text query.
// On Skip / any API error / timeout the original order is returned unchanged.
// One jev_decisions row is written per Evaluate (item_id null, surface=rerank).
func (s *Server) maybeRerank(ctx context.Context, uid uuid.UUID, query string, want bool, results []search.Result) []search.Result {
	if !want || len(results) == 0 {
		return results
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return results
	}
	if s.jev == nil {
		return results
	}

	enabled, err := s.aiAssistedOrganisation(ctx, uid)
	if err != nil {
		slog.Warn("jev rerank: reading setting", "err", err)
		return results
	}
	if !enabled {
		return results
	}

	k := len(results)
	if k > jev.RerankTopK {
		k = jev.RerankTopK
	}
	window := results[:k]
	tail := results[k:]

	scores := make([]float64, k)
	cands := make([]jev.RerankCandidate, k)
	hits := make([]jev.RankedHit, k)
	for i, res := range window {
		scores[i] = res.Score
		snippet := itemRerankSnippet(res.Item)
		title := strings.TrimSpace(res.Item.Title)
		cands[i] = jev.RerankCandidate{
			ID:      res.Item.ID.String(),
			Title:   title,
			Snippet: snippet,
		}
		hits[i] = jev.RankedHit{
			ID:        res.Item.ID.String(),
			Title:     title,
			Snippet:   snippet,
			OrigIndex: i,
		}
	}
	sims := jev.NormalizeVectorSims(scores)
	for i := range hits {
		hits[i].VectorSim = sims[i]
	}

	state := jev.BuildRerankState(query, cands)
	questions := jev.RerankQuestions(cands)
	if len(questions) == 0 {
		return results
	}

	evalCtx, cancel := context.WithTimeout(ctx, jev.RerankTimeout)
	defer cancel()
	started := time.Now()
	result, evalErr := s.jev.Evaluate(evalCtx, state, questions)
	latency := int(time.Since(started).Milliseconds())

	if evalErr != nil {
		if jev.Skip(evalErr) {
			slog.Info("jev rerank: skipped", "err", evalErr, "latency_ms", latency)
		} else {
			slog.Warn("jev rerank: evaluate failed", "err", evalErr, "latency_ms", latency)
		}
		s.insertRerankDecision(ctx, uid, jev.DefaultModel, []byte("{}"), string(jev.ActionSkipped), latency, 0)
		return results
	}

	answersRaw, err := json.Marshal(result.Answers)
	if err != nil {
		slog.Warn("jev rerank: encoding answers", "err", err)
		s.insertRerankDecision(ctx, uid, result.Model, []byte("{}"), string(jev.ActionSkipped), latency, result.Usage.InputTokens)
		return results
	}

	ordered := jev.ReorderByBlend(hits, result.Answers)
	out := make([]search.Result, 0, len(results))
	for _, h := range ordered {
		r := window[h.OrigIndex]
		r.Score = jev.BlendRerank(h.VectorSim, jev.CandidateNoul(result.Answers, h.OrigIndex))
		out = append(out, r)
	}
	out = append(out, tail...)

	s.insertRerankDecision(ctx, uid, result.Model, answersRaw, string(jev.ActionApplied), latency, result.Usage.InputTokens)
	if latency > int(jev.RerankTimeout/time.Millisecond) {
		slog.Info("jev rerank: over ideal budget", "latency_ms", latency, "budget_ms", int(jev.RerankTimeout/time.Millisecond))
	}
	return out
}

func (s *Server) aiAssistedOrganisation(ctx context.Context, userID uuid.UUID) (bool, error) {
	val, err := s.store.Queries.GetUserSetting(ctx, db.GetUserSettingParams{
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

func (s *Server) insertRerankDecision(
	ctx context.Context,
	userID uuid.UUID,
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
		ItemID:        pgtype.UUID{}, // null — one row per Evaluate covering all candidates
		Surface:       jev.SurfaceRerank,
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
	if _, err := s.store.Queries.InsertJevDecision(ctx, params); err != nil {
		slog.Warn("jev rerank: inserting decision", "action", action, "err", err)
	}
}

// itemRerankSnippet prefers the short summary, else body — never a full
// document past SnippetMaxChars (clipped again in BuildRerankState).
func itemRerankSnippet(it db.Item) string {
	if s := strings.TrimSpace(it.Summary); s != "" {
		return s
	}
	return strings.TrimSpace(it.Body)
}
