// Package mcp exposes Openmind's capture and search as Model Context Protocol
// tools over Streamable HTTP. It depends only on the store/search data types
// (never on internal/api), so the HTTP layer imports it, not the reverse.
package mcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// ErrNotFound signals an unknown or cross-tenant id; tools map it to a
// "not found" tool error rather than a transport-level failure.
var ErrNotFound = errors.New("not found")

// SearchOutcome is what Backend.Search returns: ranked results plus an optional
// human-readable echo of how a parsed query was understood.
type SearchOutcome struct {
	Results    []search.Result
	Understood string
}

// Backend is the capability surface the tools need. Implemented by *api.Server.
type Backend interface {
	Save(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error)
	Search(ctx context.Context, uid uuid.UUID, q, color string, parse bool) (SearchOutcome, error)
	ListRecent(ctx context.Context, uid uuid.UUID, limit int) ([]db.Item, error)
	GetItem(ctx context.Context, uid uuid.UUID, id uuid.UUID) (db.Item, error)
	ListLenses(ctx context.Context, uid uuid.UUID) ([]db.Lense, error)
	RunLens(ctx context.Context, uid uuid.UUID, id uuid.UUID) ([]search.Result, error)
}

// ItemSummary is the compact item shape returned by list/search tools.
type ItemSummary struct {
	ID        string   `json:"id"`
	URL       string   `json:"url,omitempty"`
	Title     string   `json:"title,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	CardType  string   `json:"cardType,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags"`
	UserTags  []string `json:"userTags"`
	CreatedAt string   `json:"createdAt"`
}

// SavedItem is the save_item result.
type SavedItem struct {
	ID        string `json:"id"`
	URL       string `json:"url,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

// ItemDetail extends ItemSummary with the archived body (get_item).
type ItemDetail struct {
	ItemSummary
	Body string `json:"body,omitempty"`
}

// SearchHit pairs an item with its ranking score.
type SearchHit struct {
	Item  ItemSummary `json:"item"`
	Score float64     `json:"score"`
}

// LensInfo is a saved query as exposed to the agent.
type LensInfo struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Rule LensRule `json:"rule"`
}

// LensRule mirrors the stored rule signals (all optional).
type LensRule struct {
	Q     string   `json:"q,omitempty"`
	Color string   `json:"color,omitempty"`
	Types []string `json:"types,omitempty"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toSummary(it db.Item) ItemSummary {
	return ItemSummary{
		ID:        it.ID.String(),
		URL:       it.Url,
		Title:     it.Title,
		Summary:   it.Summary,
		CardType:  it.CardType,
		Status:    it.Status,
		Tags:      nonNil(it.Tags),
		UserTags:  nonNil(it.UserTags),
		CreatedAt: it.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toHits(rs []search.Result) []SearchHit {
	out := make([]SearchHit, 0, len(rs))
	for _, r := range rs {
		out = append(out, SearchHit{Item: toSummary(r.Item), Score: r.Score})
	}
	return out
}

// NewHandler builds the MCP server, registers the six tools, and returns the
// Streamable HTTP handler ready to mount. uidFor resolves the caller from the
// per-request context (single-user: always the dev user).
func NewHandler(b Backend, uidFor func(context.Context) uuid.UUID) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "openmind", Version: "0.1.0"}, nil)
	registerTools(server, b, uidFor)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}
