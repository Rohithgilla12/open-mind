package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

const maxLensNameRunes = 120

// weeklyScheduleRe matches "weekly:0".."weekly:6" (Sunday=0..Saturday=6).
var weeklyScheduleRe = regexp.MustCompile(`^weekly:[0-6]$`)

// validDigestSchedule reports whether v is a recognised digest schedule:
// empty (disabled), "daily", or "weekly:0".."weekly:6".
func validDigestSchedule(v string) bool {
	switch v {
	case "", "daily":
		return true
	default:
		return weeklyScheduleRe.MatchString(v)
	}
}

// validCardType reports whether t is one of the card types the schema allows,
// used to reject unknown type filters in a Lens rule.
func validCardType(t string) bool {
	switch t {
	case "article", "product", "book", "recipe", "video", "tweet", "image", "note", "quote", "repo":
		return true
	default:
		return false
	}
}

// normalisedRule is the validated, canonical form of a LensRule: trimmed text,
// trimmed colour, deduped/known card types, normalised domains, and optional
// scope. It is what gets persisted so a stored rule is always directly usable
// by search.RunLensRule. Empty scope means library at run time.
type normalisedRule struct {
	q, color       string
	types, domains []string
	scope          search.Scope // "" means library at run time
}

// parseRule validates an incoming LensRule and returns its canonical form. A
// rule must carry at least one signal (q, colour, types, or domains); an
// unknown colour, card type, domain, or scope is rejected. The returned error
// message is safe to surface.
func parseRule(rule LensRule) (normalisedRule, error) {
	var out normalisedRule
	if rule.Q != nil {
		out.q = strings.TrimSpace(*rule.Q)
	}
	if rule.Color != nil {
		out.color = strings.TrimSpace(*rule.Color)
	}
	if out.color != "" && !search.ValidColor(out.color) {
		return out, errors.New("rule.color is not a recognised colour")
	}
	if rule.Types != nil {
		seen := map[string]bool{}
		for _, t := range *rule.Types {
			ts := strings.TrimSpace(string(t))
			if !validCardType(ts) {
				return out, errors.New("rule.types contains an unknown card type")
			}
			if !seen[ts] {
				seen[ts] = true
				out.types = append(out.types, ts)
			}
		}
	}
	if rule.Domains != nil {
		var nonEmpty []string
		for _, raw := range *rule.Domains {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			if _, ok := search.NormalizeDomain(trimmed); !ok {
				return out, errors.New("rule.domains contains an invalid host")
			}
			nonEmpty = append(nonEmpty, trimmed)
		}
		out.domains = search.NormalizeDomains(nonEmpty)
	}
	if rule.Scope != nil {
		switch *rule.Scope {
		case LensRuleScopeLibrary, LensRuleScopeAll:
			out.scope = search.Scope(*rule.Scope)
		default:
			return out, errors.New("rule.scope must be library or all")
		}
	}
	if out.q == "" && out.color == "" && len(out.types) == 0 && len(out.domains) == 0 {
		return out, errors.New("rule must set at least one of q, color, types, or domains")
	}
	return out, nil
}

// marshalRule encodes a canonical rule as the jsonb payload stored on the row.
// Empty scope is omitted so the stored form stays compact (library is the
// run-time default). Empty domains are omitted likewise.
func marshalRule(n normalisedRule) ([]byte, error) {
	r := LensRule{}
	if n.q != "" {
		r.Q = &n.q
	}
	if n.color != "" {
		r.Color = &n.color
	}
	if len(n.types) > 0 {
		ts := make([]LensRuleTypes, 0, len(n.types))
		for _, t := range n.types {
			ts = append(ts, LensRuleTypes(t))
		}
		r.Types = &ts
	}
	if len(n.domains) > 0 {
		ds := append([]string(nil), n.domains...)
		r.Domains = &ds
	}
	if n.scope != "" {
		sc := LensRuleScope(n.scope)
		r.Scope = &sc
	}
	return json.Marshal(r)
}

// decodeLensRequest reads and validates a create/update body (both share the
// CreateLensRequest shape). It returns the trimmed name, canonical rule, and
// the raw digestSchedule (nil if the field was omitted).
func decodeLensRequest(w http.ResponseWriter, r *http.Request) (string, normalisedRule, *string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req CreateLensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return "", normalisedRule{}, nil, false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return "", normalisedRule{}, nil, false
	}
	if utf8.RuneCountInString(name) > maxLensNameRunes {
		writeError(w, http.StatusBadRequest, "name too long (max 120 chars)")
		return "", normalisedRule{}, nil, false
	}
	rule, err := parseRule(req.Rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", normalisedRule{}, nil, false
	}
	if req.DigestSchedule != nil && !validDigestSchedule(*req.DigestSchedule) {
		writeError(w, http.StatusBadRequest, "invalid digest schedule")
		return "", normalisedRule{}, nil, false
	}
	return name, rule, req.DigestSchedule, true
}

// ListLenses returns the caller's saved Lenses, newest first. Always an array.
func (s *Server) ListLenses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lenses, err := s.store.Queries.ListLenses(ctx, userID(ctx))
	if err != nil {
		slog.Error("listing lenses", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list lenses")
		return
	}
	out := make([]Lens, 0, len(lenses))
	for _, l := range lenses {
		out = append(out, toAPILens(l))
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateLens persists a new Lens and returns 201.
func (s *Server) CreateLens(w http.ResponseWriter, r *http.Request) {
	name, rule, digestSchedule, ok := decodeLensRequest(w, r)
	if !ok {
		return
	}
	raw, err := marshalRule(rule)
	if err != nil {
		slog.Error("marshalling lens rule", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save lens")
		return
	}
	ctx := r.Context()
	uid := userID(ctx)
	l, err := s.store.Queries.CreateLens(ctx, db.CreateLensParams{UserID: uid, Name: name, Rule: raw})
	if err != nil {
		slog.Error("creating lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save lens")
		return
	}
	if digestSchedule != nil && *digestSchedule != "" {
		l, err = s.store.Queries.UpdateLensDigestSchedule(ctx, db.UpdateLensDigestScheduleParams{UserID: uid, ID: l.ID, DigestSchedule: *digestSchedule})
		if err != nil {
			slog.Error("setting lens digest schedule", "err", err)
			writeError(w, http.StatusInternalServerError, "could not save lens")
			return
		}
	}
	writeJSON(w, http.StatusCreated, toAPILens(l))
}

// GetLens returns a single Lens owned by the caller; unknown/foreign id → 404.
func (s *Server) GetLens(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	l, err := s.store.Queries.GetLens(ctx, db.GetLensParams{UserID: userID(ctx), ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "lens not found")
			return
		}
		slog.Error("getting lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch lens")
		return
	}
	writeJSON(w, http.StatusOK, toAPILens(l))
}

// UpdateLens renames a Lens and/or replaces its rule; unknown id → 404.
func (s *Server) UpdateLens(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	name, rule, digestSchedule, ok := decodeLensRequest(w, r)
	if !ok {
		return
	}
	raw, err := marshalRule(rule)
	if err != nil {
		slog.Error("marshalling lens rule", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update lens")
		return
	}
	ctx := r.Context()
	uid := userID(ctx)
	l, err := s.store.Queries.UpdateLens(ctx, db.UpdateLensParams{UserID: uid, ID: id, Name: name, Rule: raw})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "lens not found")
			return
		}
		slog.Error("updating lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update lens")
		return
	}
	if digestSchedule != nil {
		l, err = s.store.Queries.UpdateLensDigestSchedule(ctx, db.UpdateLensDigestScheduleParams{UserID: uid, ID: id, DigestSchedule: *digestSchedule})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "lens not found")
				return
			}
			slog.Error("updating lens digest schedule", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update lens")
			return
		}
	}
	writeJSON(w, http.StatusOK, toAPILens(l))
}

// DeleteLens removes a Lens owned by the caller; a no-op delete → 404.
func (s *Server) DeleteLens(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	rows, err := s.store.Queries.DeleteLens(ctx, db.DeleteLensParams{UserID: userID(ctx), ID: id})
	if err != nil {
		slog.Error("deleting lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete lens")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "lens not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetLensItems runs a Lens's saved rule and returns the items it currently
// matches — a live view, so new saves surface here without manual filing.
func (s *Server) GetLensItems(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)
	l, err := s.store.Queries.GetLens(ctx, db.GetLensParams{UserID: uid, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "lens not found")
			return
		}
		slog.Error("getting lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch lens")
		return
	}

	rule := decodeStoredRule(l.Rule)
	results, err := s.runLensRule(ctx, uid, rule)
	if err != nil {
		if errors.Is(err, search.ErrBadColor) {
			// A stored colour became invalid — treat as an empty view rather than 500.
			slog.Warn("lens has an invalid stored colour", "lens_id", id)
			writeJSON(w, http.StatusOK, SearchResponse{Results: []SearchResult{}})
			return
		}
		slog.Error("running lens rule", "err", err)
		writeError(w, http.StatusInternalServerError, "could not run lens")
		return
	}
	out := SearchResponse{Results: make([]SearchResult, 0, len(results))}
	for _, res := range results {
		out.Results = append(out.Results, SearchResult{Item: toAPIItem(res.Item), Score: float32(res.Score)})
	}
	writeJSON(w, http.StatusOK, out)
}

// runLensRule executes a canonical rule via the shared search.RunLensRule
// seam (also used by the send-to-Kindle Lens digest job), so both paths see
// identical matches. Empty scope defaults to library (Mind only).
func (s *Server) runLensRule(ctx context.Context, uid uuid.UUID, rule normalisedRule) ([]search.Result, error) {
	q := search.Query{
		Text:    rule.q,
		Color:   rule.color,
		Types:   rule.types,
		Domains: rule.domains,
		Scope:   rule.scope,
	}
	if q.Scope == "" {
		q.Scope = search.ScopeLibrary
	}
	return search.RunLensRule(ctx, s.store, s.provider, uid, q)
}

// decodeStoredRule reads a persisted jsonb rule into its canonical form. Stored
// rules were validated on write, so a decode error degrades to an empty rule
// rather than failing the request.
func decodeStoredRule(raw []byte) normalisedRule {
	var lr LensRule
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &lr); err != nil {
			slog.Warn("decoding stored lens rule", "err", err)
			return normalisedRule{}
		}
	}
	n, _ := parseRule(lr)
	return n
}

// toAPILens maps a stored lens row to the API model, decoding its jsonb rule.
func toAPILens(l db.Lense) Lens {
	var rule LensRule
	if len(l.Rule) > 0 {
		if err := json.Unmarshal(l.Rule, &rule); err != nil {
			slog.Warn("decoding lens rule for response", "lens_id", l.ID, "err", err)
		}
	}
	out := Lens{
		Id:             openapi_types.UUID(l.ID),
		Name:           l.Name,
		Rule:           rule,
		CreatedAt:      l.CreatedAt.Time,
		DigestSchedule: l.DigestSchedule,
	}
	if l.LastDigestAt.Valid {
		lastDigestAt := l.LastDigestAt.Time
		out.LastDigestAt = &lastDigestAt
	}
	return out
}
