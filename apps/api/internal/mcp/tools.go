package mcp

import (
	"context"
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
type tagsInput struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"`
}
type pinInput struct {
	ID     string `json:"id"`
	Pinned bool   `json:"pinned"`
}
type deleteInput struct {
	ID      string `json:"id"`
	Confirm bool   `json:"confirm,omitempty"`
}
type createLensInput struct {
	Name string   `json:"name"`
	Rule LensRule `json:"rule"`
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
type driftOut struct {
	Items []ItemSummary `json:"items"`
	Total int           `json:"total"`
}
type relatedOut struct {
	Results []RelatedHit `json:"results"`
}
type RelatedHit struct {
	Item     ItemSummary `json:"item"`
	Distance float64     `json:"distance"`
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
		Description: "Search the Openmind library with hybrid full-text + semantic search. Provide a natural-language query and/or a colour (name or hex). By default the query is parsed into text/colour/type/domain filters (falls back to plain search when no AI is configured).",
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
			out.Lenses = append(out.Lenses, toLensInfo(l))
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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_user_tags",
		Description: "Replace the user's own tags on an item (full replace; AI tags are separate and unaffected). Tags are trimmed, lowercased, deduped; an empty list clears them.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in tagsInput) (*mcp.CallToolResult, ItemSummary, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[ItemSummary]("id must be a valid uuid")
		}
		it, err := b.SetUserTags(ctx, uidFor(ctx), id, in.Tags)
		if err != nil {
			if isNotFound(err) {
				return toolErr[ItemSummary]("item not found")
			}
			return toolErr[ItemSummary]("could not set tags: " + err.Error())
		}
		return ok(toSummary(it))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "pin_item",
		Description: "Pin an item to the Desk (pinned:true) or unpin it (pinned:false). Pinning is also how you keep a Drift candidate on the user's Desk.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pinInput) (*mcp.CallToolResult, ItemSummary, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[ItemSummary]("id must be a valid uuid")
		}
		it, err := b.SetPinned(ctx, uidFor(ctx), id, in.Pinned)
		if err != nil {
			if isNotFound(err) {
				return toolErr[ItemSummary]("item not found")
			}
			return toolErr[ItemSummary]("could not update pin: " + err.Error())
		}
		return ok(toSummary(it))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_item",
		Description: "Permanently delete a saved item, including its archived body — this cannot be undone. Requires confirm:true; a call without it returns the item so you can check with the user first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteInput) (*mcp.CallToolResult, DeletedItem, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[DeletedItem]("id must be a valid uuid")
		}
		if !in.Confirm {
			it, err := b.GetItem(ctx, uidFor(ctx), id)
			if err != nil {
				if isNotFound(err) {
					return toolErr[DeletedItem]("item not found")
				}
				return toolErr[DeletedItem]("could not fetch item: " + err.Error())
			}
			label := it.Title
			if label == "" {
				label = it.Url
			}
			return toolErr[DeletedItem](`refusing to delete "` + label + `" — re-call with confirm:true after checking with the user`)
		}
		it, err := b.DeleteItem(ctx, uidFor(ctx), id)
		if err != nil {
			if isNotFound(err) {
				return toolErr[DeletedItem]("item not found")
			}
			return toolErr[DeletedItem]("could not delete: " + err.Error())
		}
		return ok(DeletedItem{Deleted: true, ID: it.ID.String(), Title: it.Title, URL: it.Url})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_lens",
		Description: "Create a Lens (a named saved search). rule needs at least one of q (text query), color (name or hex), types (card types), or domains (URL hosts). Optional scope is library (default, Mind only) or all (include unkept feed).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createLensInput) (*mcp.CallToolResult, LensInfo, error) {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return toolErr[LensInfo]("name is required")
		}
		l, err := b.CreateLens(ctx, uidFor(ctx), name, in.Rule)
		if err != nil {
			return toolErr[LensInfo]("could not create lens: " + err.Error())
		}
		return ok(toLensInfo(l))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_lens",
		Description: "Delete a Lens by id. The response echoes the deleted lens (name + rule) so it can be recreated with create_lens if this was a mistake. Items are never affected.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, DeletedLens, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[DeletedLens]("id must be a valid uuid")
		}
		l, err := b.DeleteLens(ctx, uidFor(ctx), id)
		if err != nil {
			if isNotFound(err) {
				return toolErr[DeletedLens]("lens not found")
			}
			return toolErr[DeletedLens]("could not delete lens: " + err.Error())
		}
		return ok(DeletedLens{Deleted: true, Lens: toLensInfo(l)})
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_desk",
		Description: "List the items pinned to the user's Desk, newest-pinned first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, itemListOut, error) {
		items, err := b.GetDesk(ctx, uidFor(ctx))
		if err != nil {
			return toolErr[itemListOut]("could not list desk: " + err.Error())
		}
		out := itemListOut{Items: make([]ItemSummary, 0, len(items))}
		for _, it := range items {
			out.Items = append(out.Items, toSummary(it))
		}
		return ok(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_drift",
		Description: "Read today's Drift resurfacing candidates (forgotten items worth revisiting) plus the total candidate count. Read-only: it never consumes the user's Drift — to keep an item, use pin_item.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, driftOut, error) {
		items, total, err := b.GetDrift(ctx, uidFor(ctx))
		if err != nil {
			return toolErr[driftOut]("could not read drift: " + err.Error())
		}
		out := driftOut{Items: make([]ItemSummary, 0, len(items)), Total: total}
		for _, it := range items {
			out.Items = append(out.Items, toSummary(it))
		}
		return ok(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "related_items",
		Description: "Find items similar to the given item by embedding distance (nearest first, max 5). Returns an empty list until the item has been embedded; suggestions exclude items already linked.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idInput) (*mcp.CallToolResult, relatedOut, error) {
		id, err := uuid.Parse(strings.TrimSpace(in.ID))
		if err != nil {
			return toolErr[relatedOut]("id must be a valid uuid")
		}
		results, err := b.Related(ctx, uidFor(ctx), id)
		if err != nil {
			if isNotFound(err) {
				return toolErr[relatedOut]("item not found")
			}
			return toolErr[relatedOut]("could not fetch related items: " + err.Error())
		}
		out := relatedOut{Results: make([]RelatedHit, 0, len(results))}
		for _, r := range results {
			out.Results = append(out.Results, RelatedHit{Item: toSummary(r.Item), Distance: r.Distance})
		}
		return ok(out)
	})
}

func isNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
