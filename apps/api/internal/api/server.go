package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Server implements the generated ServerInterface backed by the store and an
// insert-only River client. Capture is sacred: CreateItem returns as soon as
// the row is persisted; enrichment is queued and runs asynchronously.
type Server struct {
	store       *store.Store
	riverClient *river.Client[pgx.Tx]
	provider    ai.Provider
}

// NewServer wires the HTTP handler: dev-user middleware + generated routing.
func NewServer(s *store.Store, riverClient *river.Client[pgx.Tx], provider ai.Provider) http.Handler {
	srv := &Server{store: s, riverClient: riverClient, provider: provider}
	r := chi.NewRouter()
	r.Use(devUser)
	return HandlerFromMux(srv, r)
}

// CreateItem persists a saved item and queues enrichment, then returns 201.
func (s *Server) CreateItem(w http.ResponseWriter, r *http.Request) {
	var req CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Url == nil || !validURL(*req.Url) {
		writeError(w, http.StatusBadRequest, "url must be a valid http(s) URL")
		return
	}

	ctx := r.Context()
	uid := userID(ctx)
	item, err := s.store.Queries.CreateItem(ctx, db.CreateItemParams{UserID: uid, Url: *req.Url, Body: ""})
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
	if params.Q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	ctx := r.Context()
	results, err := search.Hybrid(ctx, s.store, s.provider, userID(ctx), params.Q, defaultListLimit)
	if err != nil {
		slog.Error("hybrid search", "err", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	out := make([]SearchResult, 0, len(results))
	for _, res := range results {
		out = append(out, SearchResult{Item: toAPIItem(res.Item), Score: float32(res.Score)})
	}
	writeJSON(w, http.StatusOK, out)
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
	if it.CardType != "" {
		ct := ItemCardType(it.CardType)
		out.CardType = &ct
	}
	if len(it.Tags) > 0 {
		tags := it.Tags
		out.Tags = &tags
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
