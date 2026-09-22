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

// PatchItem edits an item owned by the caller. Three optional fields are
// supported: userTags (the supplied list fully replaces the item's user tags —
// an empty array clears them, tags are canonicalised before storage), pinned
// (true pins to the Desk with pinned_at = now, false unpins by clearing it),
// and kept (true keeps the item in the library independent of its feed with
// kept_at = now, false clears it). A body carrying none of these fields has
// nothing to update and is rejected as a bad request. Unknown or cross-tenant
// ids resolve to 404 because the updates are user-scoped. Returns the updated
// ItemDetail on success.
func (s *Server) PatchItem(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.UserTags == nil && req.Pinned == nil && req.Kept == nil {
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

	if req.Kept != nil {
		keptAt := pgtype.Timestamptz{Valid: false}
		if *req.Kept {
			keptAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		}
		rows, err := s.store.Queries.SetItemKept(ctx, db.SetItemKeptParams{
			UserID: uid,
			ID:     id,
			KeptAt: keptAt,
		})
		if err != nil {
			slog.Error("setting item kept", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update item")
			return
		}
		if rows == 0 {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
	}

	if req.UserTags != nil {
		cur, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "item not found")
				return
			}
			slog.Error("getting item before tag update", "err", err)
			writeError(w, http.StatusInternalServerError, "could not update item")
			return
		}
		before := cur.UserTags
		canon := canonicalTags(*req.UserTags)
		rows, err := s.store.Queries.SetUserTags(ctx, db.SetUserTagsParams{
			UserID:   uid,
			ID:       id,
			UserTags: canon,
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
		s.maybeBackfillJevTagEdit(ctx, uid, id, before, canon)
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
	writeJSON(w, http.StatusOK, s.itemDetailWithJev(ctx, uid, item))
}
