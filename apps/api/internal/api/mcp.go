package api

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

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
	var types []string
	var understood string
	if parse && q != "" {
		if parsed, err := s.provider.ParseQuery(ctx, q); err == nil {
			text = parsed.Text
			types = parsed.Types
			if color == "" && parsed.Color != "" && search.ValidColor(parsed.Color) {
				color = parsed.Color
			}
			if text == "" && color == "" {
				text = q
			}
			understood = understoodString(text, color, types)
		}
	}
	results, err := search.Run(ctx, s.store, s.provider, uid, text, color, types, defaultListLimit)
	if err != nil {
		return appmcp.SearchOutcome{}, err
	}
	return appmcp.SearchOutcome{Results: results, Understood: understood}, nil
}

func (b mcpBackend) ListRecent(ctx context.Context, uid uuid.UUID, limit int) ([]db.Item, error) {
	return b.s.store.Queries.ListItems(ctx, db.ListItemsParams{UserID: uid, Limit: int32(limit)})
}

func (b mcpBackend) GetItem(ctx context.Context, uid uuid.UUID, id uuid.UUID) (db.Item, error) {
	it, err := b.s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Item{}, appmcp.ErrNotFound
	}
	return it, err
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
		return nil, err
	}
	rule := decodeStoredRule(l.Rule)
	results, err := s.runLensRule(ctx, uid, rule)
	if errors.Is(err, search.ErrBadColor) {
		return []search.Result{}, nil // stored colour went bad → empty view, mirror GetLensItems
	}
	return results, err
}

// understoodString renders the parsed-query echo as a short human line for the
// MCP tool result (the REST layer uses buildUnderstood for its JSON shape).
func understoodString(text, color string, types []string) string {
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
	return strings.Join(parts, " · ")
}
