package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"
	"golang.org/x/time/rate"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	"github.com/rohithgilla12/openmind/api/internal/assets"
	"github.com/rohithgilla12/openmind/api/internal/feeds"
	"github.com/rohithgilla12/openmind/api/internal/jobs"
	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
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
// KindleConfig carries the two independent facts the kindle handlers gate
// on: whether the server's SMTP transport is usable at all, and whether it
// also has a server-wide fallback recipient (KINDLE_EMAIL). A user's own
// kindle_email setting can only ever supply a recipient — it can never
// substitute for a missing SMTP transport.
type KindleConfig struct {
	SMTPConfigured bool
	EnvRecipient   bool
}

type Server struct {
	store        *store.Store
	riverClient  *river.Client[pgx.Tx]
	provider     ai.Provider
	assetStore   *assets.FSStore
	assetMaxByte int64
	feeds        *feeds.Service
	kindle       KindleConfig
}

// NewServer wires the HTTP handler: per-IP rate limiting, credential
// resolution, and generated routing. In token mode with an empty
// LegacyToken, auth is disabled (single-user self-host) — the caller is
// warned at startup. assetStore backs the image upload/serve endpoints and
// maxBytes caps upload size. kindleCfg carries whether Send-to-Kindle's SMTP
// transport (SMTP_HOST + SMTP_FROM) is configured, and whether the server
// also has a KINDLE_EMAIL fallback recipient; the kindle handlers 409 when
// SMTP isn't configured, or when it is but neither a fallback recipient nor
// a per-user kindle_email setting is available. trusted is the set of
// reverse-proxy networks allowed to supply X-Forwarded-For for client-IP
// resolution in the rate limiters; nil trusts no proxy and limits by the
// direct connection IP.
func NewServer(s *store.Store, riverClient *river.Client[pgx.Tx], provider ai.Provider, authCfg AuthConfig, assetStore *assets.FSStore, maxBytes int64, feedSvc *feeds.Service, kindleCfg KindleConfig, trusted []*net.IPNet) http.Handler {
	srv := &Server{store: s, riverClient: riverClient, provider: provider, assetStore: assetStore, assetMaxByte: maxBytes, feeds: feedSvc, kindle: kindleCfg}
	r := chi.NewRouter()
	// Rate limiting runs before credential resolution so failed guesses consume
	// limiter tokens by construction — brute-force attempts are throttled to
	// 429 rather than getting unlimited 401 probes. The device-link claim
	// endpoint layers its own stricter, dedicated bucket on top since its code
	// is the only credential on that route.
	r.Use(rateLimit(rate.Limit(1), 10, trusted))
	r.Use(claimRateLimit(trusted))
	r.Use(mcpIPRateLimit(trusted))
	r.Use(mcpRateLimit(trusted))
	r.Use(authenticate(s, authCfg))
	mcpHandler := appmcp.NewHandler(mcpBackend{srv}, func(ctx context.Context) uuid.UUID { return userID(ctx) })
	r.Handle("/mcp", mcpHandler)
	r.Handle("/mcp/*", mcpHandler)
	return HandlerFromMux(srv, r)
}

// capture persists a saved item (exactly one of url/note) and best-effort
// enqueues enrichment, returning the stored row. Shared by the REST CreateItem
// handler and the MCP save_item tool so the two save paths never diverge.
// Capture is sacred: a failed enrichment enqueue is logged, never returned.
func (s *Server) capture(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error) {
	// url is intentionally left untrimmed: the original CreateItem validated and
	// stored the raw URL, so a whitespace-padded URL fails validURL and returns
	// 400 rather than silently succeeding. Callers that want trimming (e.g. the
	// MCP save_item tool) trim before calling capture.
	note = strings.TrimSpace(note)
	if (url == "") == (note == "") {
		return db.Item{}, fmt.Errorf("provide exactly one of url or note")
	}
	params := db.CreateItemParams{UserID: uid}
	if url != "" {
		if !validURL(url) {
			return db.Item{}, fmt.Errorf("url must be a valid http(s) URL")
		}
		// Promote-on-save: if the URL already exists as an unkept feed item, keep
		// it rather than inserting a duplicate row. This is the path by which a
		// feed-sourced item a user actively re-saves graduates into the library
		// proper; it's already been (or will be) enriched via the feed's own
		// pipeline run, so no fresh enrichment job is enqueued here. A URL that's
		// already kept, already non-feed, or belongs to another feed item falls
		// through to today's plain insert.
		if existing, err := s.store.Queries.GetItemByURL(ctx, db.GetItemByURLParams{UserID: uid, Url: url}); err == nil {
			if existing.FeedID.Valid && !existing.KeptAt.Valid {
				rows, err := s.store.Queries.SetItemKept(ctx, db.SetItemKeptParams{
					UserID: uid,
					ID:     existing.ID,
					KeptAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				})
				if err != nil {
					return db.Item{}, fmt.Errorf("creating item: %w", err)
				}
				if rows > 0 {
					item, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: existing.ID})
					if err != nil {
						return db.Item{}, fmt.Errorf("creating item: %w", err)
					}
					return item, nil
				}
				// rows == 0 means someone else's concurrent keep won the race between
				// our SetItemKept call and theirs: the item is already kept by the
				// time we looked, so re-fetch and return it rather than falling
				// through to a duplicate insert. Only a genuine not-found (the item
				// vanished entirely) falls through to the plain insert path below.
				item, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: existing.ID})
				if err == nil {
					slog.Warn("promote race: item already kept", "user_id", uid, "item_id", existing.ID)
					return item, nil
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return db.Item{}, fmt.Errorf("creating item: %w", err)
				}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.Item{}, fmt.Errorf("creating item: %w", err)
		}
		params.Url = url
	} else {
		if utf8.RuneCountInString(note) > maxNoteRunes {
			return db.Item{}, fmt.Errorf("note too long (max %d chars)", maxNoteRunes)
		}
		params.Body = note
	}
	item, err := s.store.Queries.CreateItem(ctx, params)
	if err != nil {
		return db.Item{}, fmt.Errorf("creating item: %w", err)
	}
	// Enrichment is best-effort to enqueue: a failed insert must never fail the
	// save (capture is sacred). River jobs can be re-queued later.
	if _, err := s.riverClient.Insert(ctx, jobs.EnrichArgs{UserID: uid, ItemID: item.ID}, nil); err != nil {
		slog.Error("enqueueing enrichment job", "item_id", item.ID, "err", err)
	}
	return item, nil
}

// CreateItem persists a saved item and queues enrichment, then returns 201.
func (s *Server) CreateItem(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var url, note string
	if req.Url != nil {
		url = *req.Url
	}
	if req.Note != nil {
		note = *req.Note
	}
	item, err := s.capture(r.Context(), userID(r.Context()), url, note)
	if err != nil {
		// capture wraps DB insert failures with a "creating item:" prefix; every
		// other error is a client input problem. Preserve the original REST
		// status codes: 500 for infra failures, 400 for bad input.
		if strings.HasPrefix(err.Error(), "creating item") {
			slog.Error("creating item", "err", err)
			writeError(w, http.StatusInternalServerError, "could not save item")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
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
// caller and returns ranked results: library items (direct saves and kept feed
// items) first, unkept feed-river matches trailing, each group by descending
// fused score. It always returns an array (never null) so clients can rely on
// the shape.
func (s *Server) SearchItems(w http.ResponseWriter, r *http.Request, params SearchItemsParams) {
	var q, color string
	if params.Q != nil {
		q = strings.TrimSpace(*params.Q)
	}
	if params.Color != nil {
		color = strings.TrimSpace(*params.Color)
	}

	var types []string
	if params.Types != nil {
		for _, t := range *params.Types {
			ts := strings.TrimSpace(string(t))
			if !validCardType(ts) {
				writeError(w, http.StatusBadRequest, "unknown card type")
				return
			}
			types = append(types, ts)
		}
	}

	var domains []string
	if params.Domains != nil {
		domains = search.NormalizeDomains(*params.Domains)
	}

	scope := search.ScopeAll
	if params.Scope != nil && *params.Scope == SearchItemsParamsScopeLibrary {
		scope = search.ScopeLibrary
	}

	ctx := r.Context()

	// text/color/types/domains are the signals actually searched. Without
	// parsing they are the explicit params; with parse=true the AI provider
	// fills gaps (explicit always wins).
	text := q
	var understood *UnderstoodQuery
	if params.Parse != nil && *params.Parse && q != "" {
		parsed, err := s.provider.ParseQuery(ctx, q)
		if err != nil {
			// A parse failure must never fail the search: fall back to the raw
			// query and report nothing understood.
			slog.Warn("query parse failed; searching raw query", "err", err)
		} else {
			text = strings.TrimSpace(parsed.Text)
			if color == "" && parsed.Color != "" && search.ValidColor(parsed.Color) {
				color = parsed.Color
			}
			if len(types) == 0 {
				types = parsed.Types
			}
			if len(domains) == 0 {
				domains = parsed.Domains
			}
			// Never search nothing: if parsing stripped everything, fall back to
			// the raw query as free text.
			if text == "" && color == "" && len(types) == 0 && len(domains) == 0 {
				text = q
			}
			understood = buildUnderstood(text, color, types, domains)
		}
	}

	query := search.Query{
		Text: text, Color: color, Types: types, Domains: domains, Scope: scope,
	}
	if !query.HasMatchSignal() {
		writeError(w, http.StatusBadRequest, "q, color, types, or domains is required")
		return
	}

	results, err := search.RunQuery(ctx, s.store, s.provider, userID(ctx), query, defaultListLimit)
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
func buildUnderstood(text, color string, types, domains []string) *UnderstoodQuery {
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
	if len(domains) > 0 {
		ds := make([]string, len(domains))
		copy(ds, domains)
		u.Domains = &ds
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
	// pageCount is null unless the item's original is a stored PDF.
	if it.PageCount.Valid {
		pageCount := int(it.PageCount.Int32)
		out.PageCount = &pageCount
	}
	// feedId is null unless the item originated from a feed.
	if it.FeedID.Valid {
		feedID := openapi_types.UUID(it.FeedID.Bytes)
		out.FeedId = &feedID
	}
	// keptAt is null unless the item has been explicitly kept from its feed.
	if it.KeptAt.Valid {
		keptAt := it.KeptAt.Time
		out.KeptAt = &keptAt
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
		PageCount:    base.PageCount,
		FeedId:       base.FeedId,
		KeptAt:       base.KeptAt,
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
