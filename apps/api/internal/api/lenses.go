package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

const maxLensNameRunes = 120

// validCardType reports whether t is one of the card types the schema allows,
// used to reject unknown type filters in a Lens rule.
func validCardType(t string) bool {
	switch t {
	case "article", "product", "book", "recipe", "video", "tweet", "image", "note", "quote":
		return true
	default:
		return false
	}
}

// normalisedRule is the validated, canonical form of a LensRule: trimmed text,
// trimmed colour, and deduped/known card types. It is what gets persisted so a
// stored rule is always directly usable by search.Run.
type normalisedRule struct {
	q     string
	color string
	types []string
}

// parseRule validates an incoming LensRule and returns its canonical form. A
// rule must carry at least one signal (q, colour, or types); an unknown colour
// or card type is rejected. The returned error message is safe to surface.
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
	if out.q == "" && out.color == "" && len(out.types) == 0 {
		return out, errors.New("rule must set at least one of q, color, or types")
	}
	return out, nil
}

// marshalRule encodes a canonical rule as the jsonb payload stored on the row.
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
	return json.Marshal(r)
}

// decodeLensRequest reads and validates a create/update body (both share the
// CreateLensRequest shape). It returns the trimmed name and canonical rule.
func decodeLensRequest(w http.ResponseWriter, r *http.Request) (string, normalisedRule, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req CreateLensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return "", normalisedRule{}, false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return "", normalisedRule{}, false
	}
	if utf8.RuneCountInString(name) > maxLensNameRunes {
		writeError(w, http.StatusBadRequest, "name too long (max 120 chars)")
		return "", normalisedRule{}, false
	}
	rule, err := parseRule(req.Rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", normalisedRule{}, false
	}
	return name, rule, true
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
	name, rule, ok := decodeLensRequest(w, r)
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
	l, err := s.store.Queries.CreateLens(ctx, db.CreateLensParams{UserID: userID(ctx), Name: name, Rule: raw})
	if err != nil {
		slog.Error("creating lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save lens")
		return
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
	name, rule, ok := decodeLensRequest(w, r)
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
	l, err := s.store.Queries.UpdateLens(ctx, db.UpdateLensParams{UserID: userID(ctx), ID: id, Name: name, Rule: raw})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "lens not found")
			return
		}
		slog.Error("updating lens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not update lens")
		return
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
// identical matches.
func (s *Server) runLensRule(ctx context.Context, uid uuid.UUID, rule normalisedRule) ([]search.Result, error) {
	return search.RunLensRule(ctx, s.store, s.provider, uid, rule.q, rule.color, rule.types)
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
	return Lens{
		Id:        openapi_types.UUID(l.ID),
		Name:      l.Name,
		Rule:      rule,
		CreatedAt: l.CreatedAt.Time,
	}
}
