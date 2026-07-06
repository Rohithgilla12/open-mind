package api

import (
	"log/slog"
	"net/http"
)

// GetDesk returns the caller's Desk: pinned items ordered newest-pinned first.
// It always returns an array (never null) so clients can rely on the shape.
func (s *Server) GetDesk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.store.Queries.ListPinned(ctx, userID(ctx))
	if err != nil {
		slog.Error("listing pinned items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list desk")
		return
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		out = append(out, toAPIItem(it))
	}
	writeJSON(w, http.StatusOK, out)
}
