package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"
	"golang.org/x/time/rate"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/feeds"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
	maxNoteRunes     = 10000
	maxBodyBytes     = 64 << 10
)

// Server implements the generated ServerInterface backed by the store and an
// insert-only River client. Capture is sacred: CreateItem returns as soon as
// the row is persisted; enrichment is queued and runs asynchronously.
type Server struct {
	store        *store.Store
	riverClient  *river.Client[pgx.Tx]
	provider     ai.Provider
	assetStore   *assets.FSStore
	assetMaxByte int64
	feeds        *feeds.Service
}

// NewServer wires the HTTP handler: dev-user middleware, optional bearer auth,
// per-IP rate limiting, and generated routing. When token is empty, auth is
// disabled (single-user self-host) — the caller is warned at startup.
// assetStore backs the image upload/serve endpoints and maxBytes caps upload size.
func NewServer(s *store.Store, riverClient *river.Client[pgx.Tx], provider ai.Provider, token string, assetStore *assets.FSStore, maxBytes int64, feedSvc *feeds.Service) http.Handler {
	srv := &Server{store: s, riverClient: riverClient, provider: provider, assetStore: assetStore, assetMaxByte: maxBytes, feeds: feedSvc}
	r := chi.NewRouter()
	r.Use(devUser)
	// Rate limiting runs before bearer auth so failed token guesses consume
	// limiter tokens by construction — brute-force attempts are throttled to
	// 429 rather than getting unlimited 401 probes.
	r.Use(rateLimit(rate.Limit(1), 10))
	if token != "" {
		r.Use(requireBearer(token))
	}
	return HandlerFromMux(srv, r)
}

// CreateItem persists a saved item and queues enrichment, then returns 201.
func (s *Server) CreateItem(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	hasURL := req.Url != nil && *req.Url != ""
	hasNote := req.Note != nil && strings.TrimSpace(*req.Note) != ""
	if hasURL == hasNote { // both or neither
		writeError(w, http.StatusBadRequest, "provide exactly one of url or note")
		return
	}

	ctx := r.Context()
	uid := userID(ctx)

	params := db.CreateItemParams{UserID: uid}
	if hasURL {
		if !validURL(*req.Url) {
			writeError(w, http.StatusBadRequest, "url must be a valid http(s) URL")
			return
		}
		params.Url = *req.Url
	} else {
		note := strings.TrimSpace(*req.Note)
		if utf8.RuneCountInString(note) > maxNoteRunes {
			writeError(w, http.StatusBadRequest, "note too long (max 10000 chars)")
			return
		}
		params.Body = note
	}

	item, err := s.store.Queries.CreateItem(ctx, params)
	if err != nil {
		slog.Error("creating item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save item")
		return
	}

	// Enrichment is best-effort to enqueue: a failed insert must never fail the
	// save (capture is sacred). River jobs can be re-queued later.
	if _, err := s.riverClient.Insert(ctx, jobs.EnrichArgs{UserID: uid, ItemID: item.ID}, nil); err != nil {
		slog.Error("enqueueing enrichment job", "item_id", item.ID, "err", err)
	}

	writeJSON(w, http.StatusCreated, toAPIItem(item))
}

// GetItem returns the full detail (including body) for a single item owned by
// the caller. Cross-tenant access and unknown ids both resolve to 404 because
// the query is user-scoped and returns ErrNoRows.
func (s *Server) GetItem(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	item, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: userID(ctx), ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		slog.Error("getting item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not fetch item")
		return
	}
	writeJSON(w, http.StatusOK, toAPIItemDetail(item))
}

// DeleteItem removes an item owned by the caller and returns 204. A delete that
// affects no rows (unknown id or another user's item) returns 404.
func (s *Server) DeleteItem(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	rows, err := s.store.Queries.DeleteItem(ctx, db.DeleteItemParams{UserID: userID(ctx), ID: id})
	if err != nil {
		slog.Error("deleting item", "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete item")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ExportItems returns the caller's entire library as full item details in
// created_at ASC order. It always returns an array (never null).
func (s *Server) ExportItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.store.Queries.ListItemsForExport(ctx, userID(ctx))
	if err != nil {
		slog.Error("exporting items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not export items")
		return
	}
	out := make([]ItemDetail, 0, len(items))
	for _, it := range items {
		out = append(out, toAPIItemDetail(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetHealthz reports liveness with no auth dependency.
func (s *Server) GetHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListItems returns the caller's items, newest first.
func (s *Server) ListItems(w http.ResponseWriter, r *http.Request, params ListItemsParams) {
	limit := defaultListLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	ctx := r.Context()
	items, err := s.store.Queries.ListItems(ctx, db.ListItemsParams{UserID: userID(ctx), Limit: int32(limit)})
	if err != nil {
		slog.Error("listing items", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list items")
		return
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		out = append(out, toAPIItem(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// SearchItems runs hybrid search (FTS + pgvector, RRF fusion) scoped to the
// caller and returns ranked results, newest-ranked first. It always returns an
// array (never null) so clients can rely on the shape.
func (s *Server) SearchItems(w http.ResponseWriter, r *http.Request, params SearchItemsParams) {
	var q, color string
	if params.Q != nil {
		q = strings.TrimSpace(*params.Q)
	}
	if params.Color != nil {
		color = strings.TrimSpace(*params.Color)
	}
	if q == "" && color == "" {
		writeError(w, http.StatusBadRequest, "q or color is required")
		return
	}
	ctx := r.Context()

	// text/color/types are the signals actually searched. Without parsing they
	// are the raw params; with parse=true the AI provider splits q into them.
	text := q
	var types []string
	var understood *UnderstoodQuery
	if params.Parse != nil && *params.Parse && q != "" {
		parsed, err := s.provider.ParseQuery(ctx, q)
		if err != nil {
			// A parse failure must never fail the search: fall back to the raw
			// query and report nothing understood.
			slog.Warn("query parse failed; searching raw query", "err", err)
		} else {
			text = strings.TrimSpace(parsed.Text)
			types = parsed.Types
			// A parsed colour applies only when the caller gave none explicitly,
			// and only if it's a colour search can actually use.
			if color == "" && parsed.Color != "" && search.ValidColor(parsed.Color) {
				color = parsed.Color
			}
			// Never search nothing: if parsing stripped everything, fall back to
			// the raw query as free text.
			if text == "" && color == "" {
				text = q
			}
			understood = buildUnderstood(text, color, types)
		}
	}

	results, err := search.Run(ctx, s.store, s.provider, userID(ctx), text, color, types, defaultListLimit)
	if errors.Is(err, search.ErrBadColor) {
		writeError(w, http.StatusBadRequest, "invalid color")
		return
	}
	if err != nil {
		slog.Error("hybrid search", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	out := SearchResponse{Results: make([]SearchResult, 0, len(results)), Understood: understood}
	for _, res := range results {
		out.Results = append(out.Results, SearchResult{Item: toAPIItem(res.Item), Score: float32(res.Score)})
	}
	writeJSON(w, http.StatusOK, out)
}

// buildUnderstood assembles the UnderstoodQuery echoed back to clients, omitting
// empty fields so the response only carries what was actually searched.
func buildUnderstood(text, color string, types []string) *UnderstoodQuery {
	u := &UnderstoodQuery{}
	if text != "" {
		u.Text = &text
	}
	if color != "" {
		u.Color = &color
	}
	if len(types) > 0 {
		ts := make([]UnderstoodQueryTypes, 0, len(types))
		for _, t := range types {
			ts = append(ts, UnderstoodQueryTypes(t))
		}
		u.Types = &ts
	}
	return u
}

// validURL accepts only absolute http/https URLs.
func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// toAPIItem maps a stored item to the API model, converting empty columns to
// omitted optional fields.
func toAPIItem(it db.Item) Item {
	out := Item{
		Id:        openapi_types.UUID(it.ID),
		Url:       it.Url,
		Status:    ItemStatus(it.Status),
		CreatedAt: it.CreatedAt.Time,
	}
	if it.Title != "" {
		out.Title = &it.Title
	}
	if it.Summary != "" {
		out.Summary = &it.Summary
	}
	if it.LeadImageUrl != "" {
		out.LeadImageUrl = &it.LeadImageUrl
	}
	if it.CardType != "" {
		ct := ItemCardType(it.CardType)
		out.CardType = &ct
	}
	if len(it.Tags) > 0 {
		tags := it.Tags
		out.Tags = &tags
	}
	// user tags are always emitted (empty array, never null) so clients can
	// render/edit the set without a nil check.
	userTags := it.UserTags
	if userTags == nil {
		userTags = []string{}
	}
	out.UserTags = &userTags
	if len(it.Palette) > 0 {
		palette := it.Palette
		out.Palette = &palette
	}
	// pinnedAt is null unless the item is on the Desk.
	if it.PinnedAt.Valid {
		pinnedAt := it.PinnedAt.Time
		out.PinnedAt = &pinnedAt
	}
	return out
}

// toAPIItemDetail maps a stored item to the detail API model: the shared Item
// fields plus the full body.
// toAPIItemDetail mirrors toAPIItem plus Body; keep field copies in sync when Item gains fields.
func toAPIItemDetail(it db.Item) ItemDetail {
	base := toAPIItem(it)
	out := ItemDetail{
		Id:           base.Id,
		Url:          base.Url,
		Status:       ItemDetailStatus(base.Status),
		CreatedAt:    base.CreatedAt,
		Title:        base.Title,
		Summary:      base.Summary,
		LeadImageUrl: base.LeadImageUrl,
		Tags:         base.Tags,
		UserTags:     base.UserTags,
		Palette:      base.Palette,
		PinnedAt:     base.PinnedAt,
		Body:         it.Body,
	}
	if base.CardType != nil {
		ct := ItemDetailCardType(*base.CardType)
		out.CardType = &ct
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
