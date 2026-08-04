package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/rohithgilla12/openmind/api/internal/ai"
	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// NewMCPBackend builds the MCP Backend adapter without the HTTP router, for
// transports that never serve HTTP (the stdio command). It wraps a minimal
// *Server carrying only the dependencies the MCP tools reach: the store,
// an insert-only River client (save_item enqueues enrichment), and the AI
// provider (search query parsing). Asset/feed/kindle handlers are HTTP-only
// and never reached through the Backend interface.
func NewMCPBackend(s *store.Store, riverClient *river.Client[pgx.Tx], provider ai.Provider) appmcp.Backend {
	return mcpBackend{&Server{store: s, riverClient: riverClient, provider: provider}}
}

// mcpBackend adapts *Server to appmcp.Backend. It cannot live directly on
// *Server: the generated ServerInterface already has GetItem(w, r, id) and
// ListLenses(w, r) as REST handler methods, and Go does not allow a second
// method of the same name with a different signature on the same type. The
// six Backend methods mirror the REST handlers' logic but return data +
// errors instead of writing HTTP.
type mcpBackend struct{ s *Server }

func (b mcpBackend) Save(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error) {
	return b.s.capture(ctx, uid, url, note)
}

func (b mcpBackend) Search(ctx context.Context, uid uuid.UUID, q, color string, parse bool) (appmcp.SearchOutcome, error) {
	s := b.s
	text := q
	var types, domains []string
	var understood string
	if parse && q != "" {
		if parsed, err := s.provider.ParseQuery(ctx, q); err == nil {
			text = parsed.Text
			types = parsed.Types
			domains = parsed.Domains
			if color == "" && parsed.Color != "" && search.ValidColor(parsed.Color) {
				color = parsed.Color
			}
			if text == "" && color == "" && len(types) == 0 && len(domains) == 0 {
				text = q
			}
			understood = understoodString(text, color, types, domains)
		}
	}
	results, err := search.RunQuery(ctx, s.store, s.provider, uid, search.Query{
		Text: text, Color: color, Types: types, Domains: domains, Scope: search.ScopeAll,
	}, defaultListLimit)
	if err != nil {
		return appmcp.SearchOutcome{}, err
	}
	return appmcp.SearchOutcome{Results: results, Understood: understood}, nil
}

func (b mcpBackend) ListRecent(ctx context.Context, uid uuid.UUID, limit int) ([]db.Item, error) {
	return b.s.store.Queries.ListItems(ctx, db.ListItemsParams{UserID: uid, LimitCount: int32(limit)})
}

func (b mcpBackend) GetItem(ctx context.Context, uid uuid.UUID, id uuid.UUID) (db.Item, error) {
	it, err := b.s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Item{}, appmcp.ErrNotFound
	}
	if err != nil {
		return db.Item{}, fmt.Errorf("fetching item: %w", err)
	}
	return it, nil
}

func (b mcpBackend) ListLenses(ctx context.Context, uid uuid.UUID) ([]db.Lense, error) {
	return b.s.store.Queries.ListLenses(ctx, uid)
}

func (b mcpBackend) RunLens(ctx context.Context, uid uuid.UUID, id uuid.UUID) ([]search.Result, error) {
	s := b.s
	l, err := s.store.Queries.GetLens(ctx, db.GetLensParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appmcp.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fetching lens: %w", err)
	}
	rule := decodeStoredRule(l.Rule)
	results, err := s.runLensRule(ctx, uid, rule)
	if errors.Is(err, search.ErrBadColor) {
		return []search.Result{}, nil // stored colour went bad → empty view, mirror GetLensItems
	}
	return results, err
}

func (b mcpBackend) SetUserTags(ctx context.Context, uid, id uuid.UUID, tags []string) (db.Item, error) {
	canon := canonicalTags(tags)
	if canon == nil {
		canon = []string{}
	}
	rows, err := b.s.store.Queries.SetUserTags(ctx, db.SetUserTagsParams{UserID: uid, ID: id, UserTags: canon})
	if err != nil {
		return db.Item{}, fmt.Errorf("setting user tags: %w", err)
	}
	if rows == 0 {
		return db.Item{}, appmcp.ErrNotFound
	}
	return b.GetItem(ctx, uid, id)
}

func (b mcpBackend) SetPinned(ctx context.Context, uid, id uuid.UUID, pinned bool) (db.Item, error) {
	var at pgtype.Timestamptz
	if pinned {
		at = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	rows, err := b.s.store.Queries.SetItemPinned(ctx, db.SetItemPinnedParams{UserID: uid, ID: id, PinnedAt: at})
	if err != nil {
		return db.Item{}, fmt.Errorf("setting pinned: %w", err)
	}
	if rows == 0 {
		return db.Item{}, appmcp.ErrNotFound
	}
	return b.GetItem(ctx, uid, id)
}

func (b mcpBackend) DeleteItem(ctx context.Context, uid, id uuid.UUID) (db.Item, error) {
	it, err := b.GetItem(ctx, uid, id) // capture the echo before the row disappears
	if err != nil {
		return db.Item{}, err
	}
	rows, err := b.s.store.Queries.DeleteItem(ctx, db.DeleteItemParams{UserID: uid, ID: id})
	if err != nil {
		return db.Item{}, fmt.Errorf("deleting item: %w", err)
	}
	if rows == 0 {
		return db.Item{}, appmcp.ErrNotFound
	}
	return it, nil
}

func (b mcpBackend) CreateLens(ctx context.Context, uid uuid.UUID, name string, rule appmcp.LensRule) (db.Lense, error) {
	apiRule := LensRule{}
	if rule.Q != "" {
		apiRule.Q = &rule.Q
	}
	if rule.Color != "" {
		apiRule.Color = &rule.Color
	}
	if len(rule.Types) > 0 {
		types := make([]LensRuleTypes, 0, len(rule.Types))
		for _, t := range rule.Types {
			types = append(types, LensRuleTypes(t))
		}
		apiRule.Types = &types
	}
	norm, err := parseRule(apiRule)
	if err != nil {
		return db.Lense{}, fmt.Errorf("invalid rule: %w", err)
	}
	raw, err := marshalRule(norm)
	if err != nil {
		return db.Lense{}, fmt.Errorf("encoding rule: %w", err)
	}
	l, err := b.s.store.Queries.CreateLens(ctx, db.CreateLensParams{UserID: uid, Name: name, Rule: raw})
	if err != nil {
		return db.Lense{}, fmt.Errorf("creating lens: %w", err)
	}
	return l, nil
}

func (b mcpBackend) DeleteLens(ctx context.Context, uid, id uuid.UUID) (db.Lense, error) {
	l, err := b.s.store.Queries.GetLens(ctx, db.GetLensParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Lense{}, appmcp.ErrNotFound
	}
	if err != nil {
		return db.Lense{}, fmt.Errorf("fetching lens: %w", err)
	}
	if _, err := b.s.store.Queries.DeleteLens(ctx, db.DeleteLensParams{UserID: uid, ID: id}); err != nil {
		return db.Lense{}, fmt.Errorf("deleting lens: %w", err)
	}
	return l, nil
}

func (b mcpBackend) GetDesk(ctx context.Context, uid uuid.UUID) ([]db.Item, error) {
	items, err := b.s.store.Queries.ListPinned(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("listing desk: %w", err)
	}
	return items, nil
}

func (b mcpBackend) GetDrift(ctx context.Context, uid uuid.UUID) ([]db.Item, int, error) {
	items, err := b.s.store.Queries.ListDriftCandidates(ctx, db.ListDriftCandidatesParams{UserID: uid, Limit: driftBatchSize})
	if err != nil {
		return nil, 0, fmt.Errorf("listing drift candidates: %w", err)
	}
	total, err := b.s.store.Queries.CountDriftCandidates(ctx, uid)
	if err != nil {
		return nil, 0, fmt.Errorf("counting drift candidates: %w", err)
	}
	return items, int(total), nil
}

func (b mcpBackend) Related(ctx context.Context, uid, id uuid.UUID) ([]appmcp.RelatedResult, error) {
	if _, err := b.GetItem(ctx, uid, id); err != nil {
		return nil, err
	}
	rows, err := b.s.store.Queries.RelatedByEmbedding(ctx, db.RelatedByEmbeddingParams{
		UserID: uid, ItemID: id, MaxDistance: relatedMaxDistance, LimitCount: relatedLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("querying related items: %w", err)
	}
	out := make([]appmcp.RelatedResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, appmcp.RelatedResult{Item: relatedRowToDBItem(row), Distance: row.Distance})
	}
	return out, nil
}

// relatedRowToDBItem maps the RelatedByEmbedding row's item columns back into
// a db.Item, mirroring relatedRowToAPIItem's field set but without the
// REST-shape conversion — the MCP adapter deals in db.Item throughout.
func relatedRowToDBItem(row db.RelatedByEmbeddingRow) db.Item {
	return db.Item{
		ID:            row.ID,
		UserID:        row.UserID,
		Url:           row.Url,
		Title:         row.Title,
		Body:          row.Body,
		LeadImageUrl:  row.LeadImageUrl,
		Summary:       row.Summary,
		Tags:          row.Tags,
		CardType:      row.CardType,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		Palette:       row.Palette,
		UserTags:      row.UserTags,
		PinnedAt:      row.PinnedAt,
		LastDriftedAt: row.LastDriftedAt,
		SearchTsv:     row.SearchTsv,
		PageCount:     row.PageCount,
	}
}

// understoodString renders the parsed-query echo as a short human line for the
// MCP tool result (the REST layer uses buildUnderstood for its JSON shape).
func understoodString(text, color string, types, domains []string) string {
	var parts []string
	if text != "" {
		parts = append(parts, "text "+text)
	}
	if color != "" {
		parts = append(parts, "color "+color)
	}
	if len(types) > 0 {
		parts = append(parts, "types "+strings.Join(types, ","))
	}
	if len(domains) > 0 {
		parts = append(parts, "domains "+strings.Join(domains, ","))
	}
	return strings.Join(parts, " · ")
}
