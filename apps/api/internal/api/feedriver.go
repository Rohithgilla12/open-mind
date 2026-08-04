package api

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// GetFeedItems returns one page of the caller's feed-sourced items, newest
// first, whether or not they've been kept into the library. An optional
// feedId narrows to a single subscription. Pagination is keyset: nextCursor
// encodes the last row's (created_at, id) and is absent on the last page.
func (s *Server) GetFeedItems(w http.ResponseWriter, r *http.Request, params GetFeedItemsParams) {
	limit := listLimit(params.Limit)
	cur, err := decodeCursor(params.Cursor)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}

	var filterFeedID pgtype.UUID
	if params.FeedId != nil {
		filterFeedID = pgtype.UUID{Bytes: *params.FeedId, Valid: true}
	}

	ctx := r.Context()
	rows, err := s.store.Queries.ListFeedItems(ctx, db.ListFeedItemsParams{
		UserID:          userID(ctx),
		FilterFeedID:    filterFeedID,
		CursorCreatedAt: cursorTimestamp(cur),
		CursorID:        cursorUUID(cur),
		LimitCount:      int32(limit + 1),
	})
	if err != nil {
		slog.Error("listing feed items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list feed items")
		return
	}
	writeJSON(w, http.StatusOK, toItemPage(rows, limit))
}
