package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// driftBatchSize is the fixed number of items surfaced per Drift session — a
// finite, calm daily batch rather than an endless feed.
const driftBatchSize = 5

// GetDrift returns today's Drift batch: up to driftBatchSize resurfacing
// candidates (enriched, unpinned, not drifted in the last 30 days) ordered
// never-drifted-first then oldest-saved, plus the total candidate count for the
// "n of total" line. It always returns an array (never null).
func (s *Server) GetDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := userID(ctx)

	items, err := s.store.Queries.ListDriftCandidates(ctx, db.ListDriftCandidatesParams{UserID: uid, Limit: driftBatchSize})
	if err != nil {
		slog.Error("listing drift candidates", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list drift")
		return
	}
	total, err := s.store.Queries.CountDriftCandidates(ctx, uid)
	if err != nil {
		slog.Error("counting drift candidates", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list drift")
		return
	}

	out := DriftResponse{Items: make([]Item, 0, len(items)), Total: int(total)}
	for _, it := range items {
		out.Items = append(out.Items, toAPIItem(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// DriftItem acts on a drifted item: it always marks the item drifted now (so it
// won't resurface for 30 days) and, when keep is true, pins it to the Desk. An
// action affecting no rows (unknown id or another user's item) returns 404.
func (s *Server) DriftItem(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req DriftActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx := r.Context()
	rows, err := s.store.Queries.DriftAction(ctx, db.DriftActionParams{UserID: userID(ctx), ID: id, Keep: req.Keep})
	if err != nil {
		slog.Error("drift action", "err", err)
		writeError(w, http.StatusInternalServerError, "could not act on drift")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
