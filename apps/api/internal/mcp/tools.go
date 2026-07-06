package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolErr returns a tool-execution error the model can read and act on (as
// opposed to a transport error): the SDK packs it into CallToolResult with
// IsError set, rather than failing the RPC call itself. Out is the tool's
// declared output type.
func toolErr[Out any](msg string) (*mcp.CallToolResult, Out, error) {
	var zero Out
	return nil, zero, errors.New(msg)
}

func ok[Out any](out Out) (*mcp.CallToolResult, Out, error) {
	return nil, out, nil
}

type saveInput struct {
	URL  string `json:"url,omitempty"`
	Note string `json:"note,omitempty"`
}
type searchInput struct {
	Query string `json:"query,omitempty"`
	Color string `json:"color,omitempty"`
	Parse *bool  `json:"parse,omitempty"`
}
type recentInput struct {
	Limit int `json:"limit,omitempty"`
}
type idInput struct {
	ID string `json:"id"`
}

type itemListOut struct {
	Items []ItemSummary `json:"items"`
}
type searchOut struct {
	Results    []SearchHit `json:"results"`
	Understood string      `json:"understood,omitempty"`
}
type lensListOut struct {
	Lenses []LensInfo `json:"lenses"`
}

func registerTools(s *mcp.Server, b Backend, uidFor func(context.Context) uuid.UUID) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "save_item",
		Description: "Save a URL or a text note to the Openmind library. Provide exactly one of url or note. Returns immediately; AI enrichment (summary, tags, type) runs asynchronously.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in saveInput) (*mcp.CallToolResult, SavedItem, error) {
		url := strings.TrimSpace(in.URL)
		note := strings.TrimSpace(in.Note)
		if (url == "") == (note == "") {
			return toolErr[SavedItem]("provide exactly one of url or note")
		}
		it, err := b.Save(ctx, uidFor(ctx), url, note)
		if err != nil {
			return toolErr[SavedItem]("could not save: " + err.Error())
		}
		return ok(SavedItem{
			ID:        it.ID.String(),
			URL:       it.Url,
			Status:    it.Status,
			CreatedAt: it.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_items",
		Description: "Search the Openmind library with hybrid full-text + semantic search. Provide a natural-language query and/or a colour (name or hex). By default the query is parsed into text/colour/type filters (falls back to plain search when no AI is configured).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOut, error) {
		q := strings.TrimSpace(in.Query)
		color := strings.TrimSpace(in.Color)
		if q == "" && color == "" {
			return toolErr[searchOut]("provide a query or a color")
		}
		parse := true
		if in.Parse != nil {
			parse = *in.Parse
		}
		res, err := b.Search(ctx, uidFor(ctx), q, color, parse)
		if err != nil {
			return toolErr[searchOut]("search failed: " + err.Error())
		}
		return ok(searchOut{Results: toHits(res.Results), Understood: res.Understood})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_recent",
		Description: "List the most recently saved items, newest first. limit defaults to 20 (max 200).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in recentInput) (*mcp.CallToolResult, itemListOut, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 200 {
			limit = 200
		}
		items, err := b.ListRecent(ctx, uidFor(ctx), limit)
		if err != nil {
			return toolErr[itemListOut]("could not list items: " + err.Error())
		}
		out := itemListOut{Items: make([]ItemSummary, 0, len(items))}
		for _, it := range items {
			out.Items = append(out.Items, toSummary(it))
		}
		return ok(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_item",
		Description: "Fetch the full detail (including the archived body text) of a single saved item by its id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, ItemDetail, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[ItemDetail]("id must be a valid uuid")
		}
		it, err := b.GetItem(ctx, uidFor(ctx), id)
		if err != nil {
			if isNotFound(err) {
				return toolErr[ItemDetail]("item not found")
			}
			return toolErr[ItemDetail]("could not fetch item: " + err.Error())
		}
		return ok(ItemDetail{ItemSummary: toSummary(it), Body: it.Body})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_lenses",
		Description: "List the user's saved Lenses (named saved searches). Use run_lens to fetch the items a Lens currently matches.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, lensListOut, error) {
		lenses, err := b.ListLenses(ctx, uidFor(ctx))
		if err != nil {
			return toolErr[lensListOut]("could not list lenses: " + err.Error())
		}
		out := lensListOut{Lenses: make([]LensInfo, 0, len(lenses))}
		for _, l := range lenses {
			info := LensInfo{ID: l.ID.String(), Name: l.Name}
			var lr LensRule
			if len(l.Rule) > 0 {
				_ = json.Unmarshal(l.Rule, &lr)
			}
			info.Rule = lr
			out.Lenses = append(out.Lenses, info)
		}
		return ok(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "run_lens",
		Description: "Run a saved Lens by its id and return the items it currently matches (a live view).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, searchOut, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[searchOut]("id must be a valid uuid")
		}
		res, err := b.RunLens(ctx, uidFor(ctx), id)
		if err != nil {
			if isNotFound(err) {
				return toolErr[searchOut]("lens not found")
			}
			return toolErr[searchOut]("could not run lens: " + err.Error())
		}
		return ok(searchOut{Results: toHits(res)})
	})
}

func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
