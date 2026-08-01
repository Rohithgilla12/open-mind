package api

import (
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// toAPIPlace maps a db.ItemPlace row to the API shape. Lat/Lng are only set
// when the row was geocoded (NULL columns stay absent in JSON).
func toAPIPlace(row db.ItemPlace) Place {
	p := Place{
		Id:      row.ID,
		Name:    row.Name,
		Hint:    row.Hint,
		Address: row.Address,
		Source:  row.Source,
	}
	if row.Lat.Valid {
		p.Lat = &row.Lat.Float64
	}
	if row.Lng.Valid {
		p.Lng = &row.Lng.Float64
	}
	return p
}

// GetItemPlaces returns the places extracted from one item, oldest first.
// Empty until the extract_places job has run (or when it found nothing) —
// never an error, so clients can simply hide the section.
func (s *Server) GetItemPlaces(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)

	owned, err := s.ownsItem(ctx, uid, id)
	if err != nil {
		slog.Error("checking item ownership", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load places")
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	rows, err := s.store.Queries.ListItemPlaces(ctx, db.ListItemPlacesParams{UserID: uid, ItemID: id})
	if err != nil {
		slog.Error("querying item places", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load places")
		return
	}
	out := make([]Place, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPIPlace(row))
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteItemPlace drops one extracted place from an item — the user's escape
// hatch when the model invents a venue or reads a brand name off a reel as
// somewhere you can visit. An unknown or cross-tenant item 404s on the
// ownership check; a delete that then affects no rows (unknown place, or a
// place hanging off a different item of yours) 404s too. The delete stays
// scoped by user_id regardless, so the ownership check is defence in depth
// rather than the only guard.
func (s *Server) DeleteItemPlace(w http.ResponseWriter, r *http.Request, id openapi_types.UUID, placeId openapi_types.UUID) {
	ctx := r.Context()
	uid := userID(ctx)

	owned, err := s.ownsItem(ctx, uid, id)
	if err != nil {
		slog.Error("checking item ownership", "item_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete place")
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	rows, err := s.store.Queries.DeleteItemPlace(ctx, db.DeleteItemPlaceParams{UserID: uid, ItemID: id, ID: placeId})
	if err != nil {
		slog.Error("deleting item place", "item_id", id, "place_id", placeId, "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete place")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "place not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
