package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// PatchItem edits an item owned by the caller. Two optional fields are
// supported: userTags (the supplied list fully replaces the item's user tags —
// an empty array clears them, tags are canonicalised before storage) and pinned
// (true pins to the Desk with pinned_at = now, false unpins by clearing it). A
// body carrying neither field has nothing to update and is rejected as a bad
// request. Unknown or cross-tenant ids resolve to 404 because the updates are
// user-scoped. Returns the updated ItemDetail on success.
func (s *Server) PatchItem(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.UserTags == nil && req.Pinned == nil {
		writeError(w, http.StatusBadRequest, "no updatable fields provided")
		return
	}

	ctx := r.Context()
	uid := userID(ctx)

	if req.Pinned != nil {
		pinnedAt := pgtype.Timestamptz{Valid: false}
		if *req.Pinned {
			pinnedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		}
		rows, err := s.store.Queries.SetItemPinned(ctx, db.SetItemPinnedParams{
			UserID:   uid,
			ID:       id,
			PinnedAt: pinnedAt,
		})
		if err != nil {
			slog.Error("setting item pinned", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update item")
			return
		}
		if rows == 0 {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
	}

	if req.UserTags != nil {
		rows, err := s.store.Queries.SetUserTags(ctx, db.SetUserTagsParams{
			UserID:   uid,
			ID:       id,
			UserTags: canonicalTags(*req.UserTags),
		})
		if err != nil {
			slog.Error("setting user tags", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update item")
			return
		}
		if rows == 0 {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
	}

	item, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		slog.Error("getting item after update", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch item")
		return
	}
	writeJSON(w, http.StatusOK, toAPIItemDetail(item))
}
