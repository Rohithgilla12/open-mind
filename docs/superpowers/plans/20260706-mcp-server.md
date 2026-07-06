# Openmind MCP Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose Openmind's capture + search to AI agents via a Model Context Protocol server served over Streamable HTTP at `/mcp` in the existing API binary.

**Architecture:** A new isolated `internal/mcp` package defines a narrow `Backend` interface, compact tool-output DTOs, and six typed tool handlers, and returns an `http.Handler` via the official Go MCP SDK. `*api.Server` implements `Backend` (delegating to the existing store, `search.Run`, `runLensRule`, and a newly-extracted shared `capture` helper), and `api.NewServer` mounts the handler on the chi router so it inherits the existing Bearer-auth + rate-limit middleware. No REST contract change — `/mcp` is JSON-RPC and deliberately absent from `openapi.yaml`.

**Tech Stack:** Go, chi, pgx/sqlc, River, `github.com/modelcontextprotocol/go-sdk` v1.x (new dep), `github.com/google/uuid`.

Spec: `docs/superpowers/specs/20260706-mcp-server-design.md`.

## Global Constraints

- **Single-binary self-host:** no new process/service. `/mcp` is served by the existing API binary. (CLAUDE.md principle 3.)
- **Capture is sacred:** `save_item` returns as soon as the row is persisted; enrichment enqueue is best-effort and never fails the save. (Principle 1.)
- **User-scoped store access:** every tool resolves a user id and passes it to user-scoped queries; single-user mode uses `api.DevUserID`. (Principle 4.)
- **AI pluggable:** `search_items` must work under the noop provider (parse degrades to raw FTS). (Principle 5.)
- **No contract change:** do NOT edit `openapi.yaml`; do NOT run codegen for this feature; do NOT hand-edit generated files (`internal/api/gen.go`, sqlc output).
- **Go conventions:** stdlib-first; wrap errors `fmt.Errorf("doing x: %w", err)`; all DB access through existing sqlc queries in `internal/store`; no banner comments (no `// ==== …` dividers).
- **SDK is external and new:** the code blocks below use the SDK API as documented at context7 `/modelcontextprotocol/go-sdk/v1.2.0`. Exact identifier/field names (e.g. `CallToolResultFor`, `ServerRequest`, `StreamableClientTransport`, `StreamableHTTPOptions`) are authoritative in the SDK's godoc — if the compiler disagrees with a symbol here, query context7 `/modelcontextprotocol/go-sdk/v1.2.0` and follow the SDK, keeping the same behaviour.
- Work happens in `apps/api` with plain `go` (module `github.com/rohithgilla12/openmind/api`). Run tests with `go test ./...` from `apps/api`.

---

### Task 1: `internal/mcp` package — Backend interface, DTOs, tools, handler + unit tests

**Files:**
- Modify: `apps/api/go.mod`, `apps/api/go.sum` (add the SDK dep)
- Create: `apps/api/internal/mcp/mcp.go` (Backend interface, `SearchOutcome`, DTOs, `NewHandler`, mapping helpers)
- Create: `apps/api/internal/mcp/tools.go` (the six tool registrations + handlers)
- Create: `apps/api/internal/mcp/mcp_test.go` (fake Backend + in-process HTTP tests)
- Modify: `CLAUDE.md` (add the SDK to the approved-core dependency list)

**Interfaces:**
- Consumes: `github.com/rohithgilla12/openmind/api/internal/store/db` (`db.Item`, `db.Lense`), `github.com/rohithgilla12/openmind/api/internal/search` (`search.Result`), `github.com/google/uuid`.
- Produces (Task 2 relies on these exact names/types):
  - `type Backend interface` with methods:
    - `Save(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error)`
    - `Search(ctx context.Context, uid uuid.UUID, q, color string, parse bool) (SearchOutcome, error)`
    - `ListRecent(ctx context.Context, uid uuid.UUID, limit int) ([]db.Item, error)`
    - `GetItem(ctx context.Context, uid uuid.UUID, id uuid.UUID) (db.Item, error)`
    - `ListLenses(ctx context.Context, uid uuid.UUID) ([]db.Lense, error)`
    - `RunLens(ctx context.Context, uid uuid.UUID, id uuid.UUID) ([]search.Result, error)`
  - `type SearchOutcome struct { Results []search.Result; Understood string }`
  - `func NewHandler(b Backend, uidFor func(context.Context) uuid.UUID) http.Handler`
  - Sentinel: `var ErrNotFound = errors.New("not found")` — Backend implementations return this (wrapped is fine) for unknown/foreign ids so tools emit a clean "not found" tool error.

- [ ] **Step 1: Add the SDK dependency**

Run (from `apps/api`):
```bash
go get github.com/modelcontextprotocol/go-sdk@latest
```
Expected: `go.mod` gains `github.com/modelcontextprotocol/go-sdk vX.Y.Z` (v1.x). If `@latest` resolves below v1.0.0, pin explicitly: `go get github.com/modelcontextprotocol/go-sdk@v1.2.0`.

- [ ] **Step 2: Write `mcp.go` — interface, DTOs, mapping, `NewHandler` skeleton**

Create `apps/api/internal/mcp/mcp.go`:
```go
// Package mcp exposes Openmind's capture and search as Model Context Protocol
// tools over Streamable HTTP. It depends only on the store/search data types
// (never on internal/api), so the HTTP layer imports it, not the reverse.
package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Rule  LensRule  `json:"rule"`
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
```
Note: verify `it.CreatedAt` is a `pgtype.Timestamptz` with a `.Time` field (it is, per `toAPIItem` in `internal/api/items.go`). `search.Result` has fields `Item db.Item` and `Score float64` (per `internal/api/lenses.go` usage `res.Item`, `res.Score`). `db.Lense` has `ID`, `Name`, `Rule []byte`, `CreatedAt`.

- [ ] **Step 3: Write `tools.go` — the six tool handlers**

Create `apps/api/internal/mcp/tools.go`:
```go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolErr returns a tool-execution error result the model can read and act on
// (as opposed to a transport error). Out is the tool's declared output type.
func toolErr[Out any](msg string) (*mcp.CallToolResultFor[Out], error) {
	return &mcp.CallToolResultFor[Out]{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, nil
}

func ok[Out any](out Out) (*mcp.CallToolResultFor[Out], error) {
	return &mcp.CallToolResultFor[Out]{StructuredContent: out}, nil
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
	Results   []SearchHit `json:"results"`
	Understood string     `json:"understood,omitempty"`
}
type lensListOut struct {
	Lenses []LensInfo `json:"lenses"`
}

func registerTools(s *mcp.Server, b Backend, uidFor func(context.Context) uuid.UUID) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "save_item",
		Description: "Save a URL or a text note to the Openmind library. Provide exactly one of url or note. Returns immediately; AI enrichment (summary, tags, type) runs asynchronously.",
	}, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[saveInput]]) (*mcp.CallToolResultFor[SavedItem], error) {
		in := req.Params.Arguments
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
	}, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[searchInput]]) (*mcp.CallToolResultFor[searchOut], error) {
		in := req.Params.Arguments
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
	}, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[recentInput]]) (*mcp.CallToolResultFor[itemListOut], error) {
		limit := req.Params.Arguments.Limit
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
	}, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[idInput]]) (*mcp.CallToolResultFor[ItemDetail], error) {
		id, err := uuid.Parse(strings.TrimSpace(req.Params.Arguments.ID))
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
	}, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[struct{}]]) (*mcp.CallToolResultFor[lensListOut], error) {
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
	}, func(ctx context.Context, req *mcp.ServerRequest[*mcp.CallToolParamsFor[idInput]]) (*mcp.CallToolResultFor[searchOut], error) {
		id, err := uuid.Parse(strings.TrimSpace(req.Params.Arguments.ID))
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
```
Add `"errors"` to this file's imports (used by `isNotFound`). The `fmt` import is only needed if you use it — remove if unused to satisfy the compiler.

- [ ] **Step 4: Write the failing unit test with a fake Backend**

Create `apps/api/internal/mcp/mcp_test.go`:
```go
package mcp_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

type fakeBackend struct {
	saved     []db.Item
	items     []db.Item
	lenses    []db.Lense
	failLens  bool
	notFound  bool
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func (f *fakeBackend) Save(_ context.Context, _ uuid.UUID, url, note string) (db.Item, error) {
	it := db.Item{ID: uuid.New(), Url: url, Body: note, Status: "pending", CreatedAt: ts(time.Unix(0, 0))}
	f.saved = append(f.saved, it)
	return it, nil
}
func (f *fakeBackend) Search(_ context.Context, _ uuid.UUID, q, color string, parse bool) (appmcp.SearchOutcome, error) {
	rs := make([]search.Result, 0, len(f.items))
	for _, it := range f.items {
		rs = append(rs, search.Result{Item: it, Score: 1})
	}
	u := ""
	if parse {
		u = "text " + q
	}
	return appmcp.SearchOutcome{Results: rs, Understood: u}, nil
}
func (f *fakeBackend) ListRecent(_ context.Context, _ uuid.UUID, limit int) ([]db.Item, error) {
	if limit < len(f.items) {
		return f.items[:limit], nil
	}
	return f.items, nil
}
func (f *fakeBackend) GetItem(_ context.Context, _ uuid.UUID, id uuid.UUID) (db.Item, error) {
	if f.notFound {
		return db.Item{}, appmcp.ErrNotFound
	}
	return db.Item{ID: id, Url: "https://x", Status: "enriched", Body: "hello body", CreatedAt: ts(time.Unix(0, 0))}, nil
}
func (f *fakeBackend) ListLenses(_ context.Context, _ uuid.UUID) ([]db.Lense, error) {
	return f.lenses, nil
}
func (f *fakeBackend) RunLens(_ context.Context, _ uuid.UUID, id uuid.UUID) ([]search.Result, error) {
	if f.failLens {
		return nil, appmcp.ErrNotFound
	}
	return []search.Result{{Item: db.Item{ID: id, Url: "https://l", Status: "enriched", CreatedAt: ts(time.Unix(0, 0))}, Score: 2}}, nil
}

// connect spins the MCP handler on an httptest server and returns a connected
// client session that talks the real Streamable HTTP transport.
func connect(t *testing.T, b appmcp.Backend) *sdk.ClientSession {
	t.Helper()
	h := appmcp.NewHandler(b, func(context.Context) uuid.UUID { return uuid.New() })
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	sess, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: srv.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func call(t *testing.T, sess *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func TestToolsListHasSix(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	res, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != 6 {
		t.Fatalf("want 6 tools, got %d", len(res.Tools))
	}
}

func TestSaveItemRequiresExactlyOne(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	// neither
	if r := call(t, sess, "save_item", map[string]any{}); !r.IsError {
		t.Fatal("expected error when neither url nor note given")
	}
	// both
	if r := call(t, sess, "save_item", map[string]any{"url": "https://x", "note": "n"}); !r.IsError {
		t.Fatal("expected error when both url and note given")
	}
	// exactly one
	if r := call(t, sess, "save_item", map[string]any{"url": "https://x"}); r.IsError {
		t.Fatal("unexpected error saving a url")
	}
}

func TestGetItemNotFound(t *testing.T) {
	sess := connect(t, &fakeBackend{notFound: true})
	r := call(t, sess, "get_item", map[string]any{"id": uuid.New().String()})
	if !r.IsError {
		t.Fatal("expected not-found tool error")
	}
}

func TestGetItemBadUUID(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	r := call(t, sess, "get_item", map[string]any{"id": "not-a-uuid"})
	if !r.IsError {
		t.Fatal("expected bad-uuid tool error")
	}
}

func TestSearchEmpty(t *testing.T) {
	sess := connect(t, &fakeBackend{})
	r := call(t, sess, "search_items", map[string]any{})
	if !r.IsError {
		t.Fatal("expected error when no query or color")
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail (compile first)**

Run (from `apps/api`):
```bash
go test ./internal/mcp/... 2>&1 | head -40
```
Expected: compiles and tests run. If a SDK symbol name is wrong (e.g. `StreamableClientTransport`, `CallToolParams`, `ClientSession`, `ListTools`), the compile error names it — query context7 `/modelcontextprotocol/go-sdk/v1.2.0` for the correct symbol and adjust `mcp.go`/`tools.go`/`mcp_test.go` accordingly (behaviour unchanged). Iterate until the package compiles and the five tests pass. (They should pass once compiling — the fake Backend needs no DB.)

- [ ] **Step 6: Verify tests pass**

Run: `go test ./internal/mcp/...`
Expected: `ok  github.com/rohithgilla12/openmind/api/internal/mcp`

- [ ] **Step 7: Update CLAUDE.md approved-deps line**

In `CLAUDE.md`, find the Go conventions line listing approved core deps ("Current approved core: chi (router), River (jobs), sqlc + pgx, oapi-codegen.") and append the SDK:
```
- Standard library first; justify every new dependency. Current approved core: chi (router), River (jobs), sqlc + pgx, oapi-codegen, modelcontextprotocol/go-sdk (MCP server).
```

- [ ] **Step 8: Commit**

```bash
cd apps/api && go mod tidy
cd .. && git add apps/api/go.mod apps/api/go.sum apps/api/internal/mcp/ CLAUDE.md
git commit -m "feat(mcp): internal/mcp package — six tools over Streamable HTTP + unit tests"
```

---

### Task 2: Wire MCP into the API server — shared `capture` helper, Backend adapter, mount `/mcp`

**Files:**
- Modify: `apps/api/internal/api/items.go` (extract `capture` from `CreateItem`)
- Create: `apps/api/internal/api/mcp.go` (`Backend` method implementations on `*Server`)
- Modify: `apps/api/internal/api/server.go` (`NewServer` mounts `/mcp`)
- Modify: `apps/api/internal/api/server_test.go` (assert `/mcp` is reachable + auth-guarded) — or create `apps/api/internal/api/mcp_test.go`

**Interfaces:**
- Consumes (from Task 1): `internal/mcp` `Backend`, `SearchOutcome`, `NewHandler`, `ErrNotFound`.
- Consumes (existing): `db.CreateItemParams`, `s.store.Queries.CreateItem/GetItem/ListItems/ListLenses/GetLens`, `s.riverClient.Insert`, `jobs.EnrichArgs`, `search.Run`, `s.provider.ParseQuery`, `s.runLensRule`, `parseRule`, `decodeStoredRule`, `buildUnderstood`, `userID`, `DevUserID`.
- Produces: `*Server` satisfies `mcp.Backend`.

- [ ] **Step 1: Extract the shared `capture` helper**

In `apps/api/internal/api/items.go`, add (near `CreateItem`):
```go
// capture persists a saved item (exactly one of url/note) and best-effort
// enqueues enrichment, returning the stored row. Shared by the REST CreateItem
// handler and the MCP save_item tool so the two save paths never diverge.
// Capture is sacred: a failed enrichment enqueue is logged, never returned.
func (s *Server) capture(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error) {
	url = strings.TrimSpace(url)
	note = strings.TrimSpace(note)
	if (url == "") == (note == "") {
		return db.Item{}, fmt.Errorf("provide exactly one of url or note")
	}
	params := db.CreateItemParams{UserID: uid}
	if url != "" {
		if !validURL(url) {
			return db.Item{}, fmt.Errorf("url must be a valid http(s) URL")
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
	if _, err := s.riverClient.Insert(ctx, jobs.EnrichArgs{UserID: uid, ItemID: item.ID}, nil); err != nil {
		slog.Error("enqueueing enrichment job", "item_id", item.ID, "err", err)
	}
	return item, nil
}
```
Add imports to `items.go` as needed: `"fmt"`, `"github.com/google/uuid"` (check what's already imported — `strings`, `unicode/utf8`, `slog`, `db`, `jobs` are already used in this file per `CreateItem`).

- [ ] **Step 2: Refactor `CreateItem` to use `capture` (behaviour identical)**

Replace the body of `CreateItem` after JSON decode + the `hasURL == hasNote` guard with a call to `capture`. The handler keeps its own request decode + the exact same error strings so REST responses are unchanged:
```go
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
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toAPIItem(item))
}
```
Note: the previous `CreateItem` returned 400 "provide exactly one of url or note" for both/neither and 400 for a bad URL / long note; `capture`'s error messages match those, so behaviour is preserved. The old 500 path for a DB error now surfaces as 400 with a wrapped message — acceptable, but to keep the 500 for infra failures, optionally branch: if the error wraps `creating item:` return 500. Keep it simple: a `strings.HasPrefix(err.Error(), "creating item")` → 500, else 400. Implement that branch to preserve the original status codes.

- [ ] **Step 3: Run existing item tests to confirm no regression**

Run: `go test ./internal/api/... -run TestCreateItem -v` (and the broader `-run Item`)
Expected: PASS (same statuses as before). If a DB-backed test needs Postgres and none is running, note it and rely on CI; the compile must succeed.

- [ ] **Step 4: Implement the Backend adapter on `*Server`**

Create `apps/api/internal/api/mcp.go`:
```go
package api

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"
	"github.com/rohithgilla12/openmind/api/internal/search"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// The MCP tools call these; they mirror the REST handlers' logic but return
// data + errors instead of writing HTTP. *Server satisfies appmcp.Backend.

func (s *Server) Save(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error) {
	return s.capture(ctx, uid, url, note)
}

func (s *Server) Search(ctx context.Context, uid uuid.UUID, q, color string, parse bool) (appmcp.SearchOutcome, error) {
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

func (s *Server) ListRecent(ctx context.Context, uid uuid.UUID, limit int) ([]db.Item, error) {
	return s.store.Queries.ListItems(ctx, db.ListItemsParams{UserID: uid, Limit: int32(limit)})
}

func (s *Server) GetItem(ctx context.Context, uid uuid.UUID, id uuid.UUID) (db.Item, error) {
	it, err := s.store.Queries.GetItem(ctx, db.GetItemParams{UserID: uid, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Item{}, appmcp.ErrNotFound
	}
	return it, err
}

func (s *Server) ListLenses(ctx context.Context, uid uuid.UUID) ([]db.Lense, error) {
	return s.store.Queries.ListLenses(ctx, uid)
}

func (s *Server) RunLens(ctx context.Context, uid uuid.UUID, id uuid.UUID) ([]search.Result, error) {
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
		parts = append(parts, "types "+joinComma(types))
	}
	return joinSep(parts, " · ")
}
```
Add small local helpers `joinComma`/`joinSep` OR just use `strings.Join(types, ",")` and `strings.Join(parts, " · ")` with a `"strings"` import — prefer the stdlib and delete the helper stubs.

- [ ] **Step 5: Mount `/mcp` in `NewServer`**

In `apps/api/internal/api/server.go`, inside `NewServer`, after the middleware are registered and before `return HandlerFromMux(srv, r)`, mount the handler:
```go
	mcpHandler := appmcp.NewHandler(srv, func(ctx context.Context) uuid.UUID { return userID(ctx) })
	r.Handle("/mcp", mcpHandler)
	r.Handle("/mcp/*", mcpHandler)
	return HandlerFromMux(srv, r)
```
Add imports to `server.go`: `"context"`, `"github.com/google/uuid"`, `appmcp "github.com/rohithgilla12/openmind/api/internal/mcp"`. `userID` already exists in this package (used throughout) and falls back to `DevUserID` when the context has no user (verify in `middleware.go`; the `devUser` middleware sets it on every request including `/mcp`).

- [ ] **Step 6: Write the wiring test — `/mcp` is mounted and auth-guarded**

Create `apps/api/internal/api/mcp_test.go`:
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer builds a Server with nil store/river is unsafe to call tools,
// but /mcp initialize + auth guard don't touch the backend, so a minimal
// handler suffices for the wiring assertions here.
func TestMCPMountedAndGuarded(t *testing.T) {
	h := NewServer(nil, nil, nil, "secret", nil, 0, nil)
	// initialize is a POST of JSON-RPC; without a token the bearer guard rejects.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer secret")
	req2.Header.Set("Accept", "application/json, text/event-stream")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("with token: want 200 initialize, got %d (body: %s)", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), `"serverInfo"`) {
		t.Fatalf("initialize response missing serverInfo: %s", rec2.Body.String())
	}
}
```
Note: `NewServer(nil, …)` builds the handler without touching the store; `initialize`/auth don't call the backend. If the rate limiter or `devUser` middleware panics on a nil field, adjust to pass minimal non-nil stubs — but they don't (rate limiter is per-IP, devUser only sets context). If `initialize` requires specific headers the SDK enforces, the test's Accept header covers the SSE/JSON negotiation; consult context7 if the code differs.

- [ ] **Step 7: Run the wiring test + full package build**

Run:
```bash
go build ./... && go vet ./... && go test ./internal/api/... -run TestMCP -v
```
Expected: build + vet clean; `TestMCPMountedAndGuarded` PASS (401 without token, 200 + serverInfo with token).

- [ ] **Step 8: Commit**

```bash
git add apps/api/internal/api/items.go apps/api/internal/api/mcp.go apps/api/internal/api/server.go apps/api/internal/api/mcp_test.go
git commit -m "feat(mcp): mount /mcp in the API server; shared capture helper + Backend adapter"
```

---

### Task 3: DB-backed e2e, deploy, docs

**Files:**
- Create: `apps/api/scripts/mcp-e2e.sh` (or a `*_test.go` guarded by a DB env) — a raw JSON-RPC drive against the compose stack
- Modify: `docs/self-hosting.md` (new MCP section)
- Modify: `TODO.md` (MCP → Done)

**Interfaces:** none produced; this task verifies and documents.

- [ ] **Step 1: Bring up the local stack**

Run (from repo root):
```bash
OPENMIND_TOKEN=devtoken AI_PROVIDER=noop docker compose up -d --build api web
```
Wait for health: `curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8787/healthz` returns `200` (self-host maps API to host `8787` per the deploy memory; if running the raw compose it may be `8080` — check `docker compose port api 8080`). Use the resolved base URL as `$API` below.

- [ ] **Step 2: Drive the MCP endpoint over raw JSON-RPC**

Streamable HTTP expects `Accept: application/json, text/event-stream` and a session. The simplest robust check uses `initialize` then `tools/list` then `tools/call`. Save `apps/api/scripts/mcp-e2e.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
API="${API:-http://localhost:8787}"
TOK="${OPENMIND_TOKEN:-devtoken}"
hdr=(-H "Authorization: Bearer $TOK" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream")

echo "== initialize =="
curl -sS "${hdr[@]}" -X POST "$API/mcp" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"0"}}}' | tee /tmp/mcp-init.txt

echo "== tools/list =="
curl -sS "${hdr[@]}" -X POST "$API/mcp" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | tee /tmp/mcp-tools.txt

echo "== tools/call save_item =="
curl -sS "${hdr[@]}" -X POST "$API/mcp" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_item","arguments":{"url":"https://danluu.com/why-benchmark/"}}}' | tee /tmp/mcp-save.txt

echo "== tools/call list_recent =="
curl -sS "${hdr[@]}" -X POST "$API/mcp" \
  -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_recent","arguments":{"limit":5}}}' | tee /tmp/mcp-recent.txt

echo "== tools/call search_items =="
curl -sS "${hdr[@]}" -X POST "$API/mcp" \
  -d '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_items","arguments":{"query":"benchmark"}}}' | tee /tmp/mcp-search.txt
```
Run: `chmod +x apps/api/scripts/mcp-e2e.sh && API=<resolved> OPENMIND_TOKEN=devtoken apps/api/scripts/mcp-e2e.sh`

Expected: `initialize` returns `serverInfo.name=openmind`; `tools/list` lists the six tool names; `save_item` returns a result with an `id`; `list_recent` includes that item; `search_items` returns results once enrichment has run (noop still indexes title/url — allow a few seconds, or assert the call succeeds with a results array). Note: if the server issues an `Mcp-Session-Id` response header on initialize, echo it back on subsequent calls (`-H "Mcp-Session-Id: <id>"`); the script may need a second pass to capture and reuse it — adjust and record. If SSE framing makes parsing awkward, add `-H "Accept: application/json"` only (the SDK serves JSON when SSE isn't required) — record whichever works.

- [ ] **Step 3: Tear down local stack**

Run: `docker compose down`

- [ ] **Step 4: Deploy the API to the VPS**

Per the deploy runbook (memory `openmind-deploy-target.md`):
```bash
ssh ubuntu@150.230.131.66 'cd ~/open-mind && git pull && docker compose up -d --build api'
```
(Only `api` changed. If the api container's IP moved and the tunnel points at it, `docker restart cloudflared`.) Verify:
```bash
curl -sS -H "Authorization: Bearer <REAL_TOKEN>" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \
  -X POST https://openmind.gilla.fun/mcp \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'
```
Expected: 200 with `serverInfo.name=openmind`. Fetch the real token from the server `.env` via ssh (never print it into chat).

- [ ] **Step 5: Document in `docs/self-hosting.md`**

Add an `## MCP server` section covering: what it exposes (the six tools), that it lives at `<instance>/mcp` and uses the **same `OPENMIND_TOKEN`** as a Bearer header, and copy-paste client config:
````markdown
## MCP server

Openmind speaks the Model Context Protocol at `<instance>/mcp` (Streamable HTTP),
authenticated with the same `OPENMIND_TOKEN` you set for the API. It exposes six
tools: `save_item`, `search_items`, `list_recent`, `get_item`, `list_lenses`,
`run_lens`.

**Claude Code:**
```bash
claude mcp add --transport http openmind https://openmind.example.com/mcp \
  --header "Authorization: Bearer $OPENMIND_TOKEN"
```

**Claude Desktop** (`claude_desktop_config.json`) — via `mcp-remote` for the
auth header:
```json
{
  "mcpServers": {
    "openmind": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://openmind.example.com/mcp",
               "--header", "Authorization: Bearer YOUR_TOKEN"]
    }
  }
}
```
````
Verify the exact `claude mcp add` flag names against the current Claude Code docs (via context7 or `claude mcp add --help`) before finalising; correct if they differ.

- [ ] **Step 6: Update `TODO.md`**

Under `## Done`, add a Milestone 3 MCP entry (dated 2026-07-06, plan reference, evidence: unit tests, local e2e outputs, live smoke against `openmind.gilla.fun/mcp`, the six tools, the docs section). Note Milestone 3 "It's alive" is now complete (Drift ✅, Desk ✅, mobile share sheet ✅, MCP ✅). Add fast-follows to `### Later` or `### Next`: stdio transport, write-tools (tagging/pin/create-lens/delete), Desk/Drift tools, `/mcp` rate-limit tuning.

- [ ] **Step 7: Commit**

```bash
git add apps/api/scripts/mcp-e2e.sh docs/self-hosting.md TODO.md
git commit -m "feat(mcp): e2e drive script, self-hosting docs, TODO — MCP server verified live"
```
No further deploy (already deployed in Step 4).

---

## Self-Review notes

- **Spec coverage:** transport (Task 2 Step 5), dependency (Task 1 Step 1 + 7), package layout/Backend (Task 1), capture helper (Task 2 Step 1), six tools (Task 1 Step 3), error handling incl. not-found + bad input (Task 1 tests, Task 2 GetItem/RunLens), contract exception (Global Constraints — no openapi edit), testing unit + e2e + live (Tasks 1 & 3), deployment + docs (Task 3). All covered.
- **Type consistency:** `Backend` method signatures identical in Task 1 (definition) and Task 2 (implementation); `SearchOutcome{Results, Understood}`, `ErrNotFound`, `NewHandler(b, uidFor)` used consistently; `search.Result{Item, Score}` and `db.Item`/`db.Lense` field names cross-checked against `internal/api/items.go` and `lenses.go`.
- **SDK caveat is explicit** in Global Constraints and each risky step, so a wrong SDK symbol becomes a lookup, not a guess.
