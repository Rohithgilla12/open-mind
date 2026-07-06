package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// canonicalPair orders two item ids so the stored pair always satisfies the
// links table's a_item < b_item invariant regardless of link direction.
func canonicalPair(x, y uuid.UUID) (uuid.UUID, uuid.UUID) {
	if bytes.Compare(x[:], y[:]) < 0 {
		return x, y
	}
	return y, x
}

// ownsItem resolves an item under the caller's user id; false covers both
// missing and cross-tenant (they must be indistinguishable).
func (s *Server) ownsItem(ctx context.Context, uid, id uuid.UUID) (bool, error) {
	_, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// listLinkedAPIItems fetches the items linked to id and maps them to the API
// model, always returning a non-nil slice.
func (s *Server) listLinkedAPIItems(ctx context.Context, uid, id uuid.UUID) ([]Item, error) {
	items, err := s.store.Queries.ListLinkedItems(ctx, db.ListLinkedItemsParams{UserID: uid, AItem: id})
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		out = append(out, toAPIItem(it))
	}
	return out, nil
}

// ListItemLinks returns the items linked to id in either direction, newest
// link first. Unknown or cross-tenant ids resolve to 404.
func (s *Server) ListItemLinks(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)

	owned, err := s.ownsItem(ctx, uid, id)
	if err != nil {
		slog.Error("checking item ownership", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list links")
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	out, err := s.listLinkedAPIItems(ctx, uid, id)
	if err != nil {
		slog.Error("listing linked items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list links")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateItemLink links id to another item owned by the caller (undirected,
// idempotent) and returns the updated linked list. Self-links are rejected;
// unknown or cross-tenant ids (either side) resolve to 404.
func (s *Server) CreateItemLink(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req CreateItemLinkJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	toID := uuid.UUID(req.ToId)
	if toID == id {
		writeError(w, http.StatusBadRequest, "cannot link an item to itself")
		return
	}

	ctx := r.Context()
	uid := userID(ctx)

	ownedFrom, err := s.ownsItem(ctx, uid, id)
	if err != nil {
		slog.Error("checking item ownership", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}
	if !ownedFrom {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	ownedTo, err := s.ownsItem(ctx, uid, toID)
	if err != nil {
		slog.Error("checking item ownership", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}
	if !ownedTo {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	a, b := canonicalPair(id, toID)
	if _, err := s.store.Queries.CreateLink(ctx, db.CreateLinkParams{UserID: uid, AItem: a, BItem: b}); err != nil {
		slog.Error("creating link", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}

	out, err := s.listLinkedAPIItems(ctx, uid, id)
	if err != nil {
		slog.Error("listing linked items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// DeleteItemLink removes the link between id and toId. A delete that affects
// no rows (unknown link, unknown id, or cross-tenant) returns 404.
func (s *Server) DeleteItemLink(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, toId openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)

	owned, err := s.ownsItem(ctx, uid, id)
	if err != nil {
		slog.Error("checking item ownership", "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete link")
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	a, b := canonicalPair(id, uuid.UUID(toId))
	rows, err := s.store.Queries.DeleteLink(ctx, db.DeleteLinkParams{UserID: uid, AItem: a, BItem: b})
	if err != nil {
		slog.Error("deleting link", "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete link")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
