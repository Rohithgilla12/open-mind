package api

import (
	"log/slog"
	"net/http"
)

// GetAccount returns read-only facts about the calling account — its identity
// and how much it holds — for the web app's sidebar.
//
// Deliberately separate from GET /settings: that endpoint is mutable
// preferences with a PATCH counterpart, whereas nothing here is settable.
//
// Email is omitted rather than faked when the account has none. Token mode has
// no identity provider, so the single auto-provisioned user's email is empty;
// clients must render a neutral label in that case. Returning a placeholder
// here would put a fabricated identity in every self-hoster's UI, which is the
// bug this endpoint exists to remove.
func (s *Server) GetAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	row, err := s.store.Queries.GetAccount(ctx, userID(ctx))
	if err != nil {
		slog.Error("fetching account", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch account")
		return
	}
	out := Account{
		ItemCount:  row.ItemCount,
		AssetBytes: row.AssetBytes,
	}
	if row.Email != "" {
		email := row.Email
		out.Email = &email
	}
	writeJSON(w, http.StatusOK, out)
}
