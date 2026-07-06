# Openmind MCP Server — Design

Date: 2026-07-06 · Status: Approved (user: core-four + Lenses, HTTP-only) · Milestone 3's last item ("It's alive": Drift ✅, Desk ✅, mobile share sheet ✅, **MCP server** ← this)

## Goal

Expose the Openmind commonplace book to AI agents (Claude Desktop, Claude Code, any MCP client) so an agent can **save into** and **search** your library — turning Openmind into a memory an agent can read from and write to. Reuses the existing store, search, capture path, and Bearer auth; adds no new required service.

## Non-negotiables respected

- **Single-binary self-host** (principle 3): the MCP server is served by the existing API binary at `/mcp`. No new process, no new required infra. `docker compose up` still deploys everything.
- **Capture is sacred** (principle 1): `save_item` returns as soon as the row is persisted; enrichment stays async (same River enqueue as REST `CreateItem`).
- **Multi-tenant store scoping** (principle 4): every tool runs user-scoped. In today's single-user mode that's the auto-provisioned dev user (`api.DevUserID`), exactly like the rest of the app; when real auth lands, the user id threads through unchanged.
- **AI pluggable** (principle 5): `search_items` with `parse=true` degrades to plain FTS under the noop provider, so the tool works with no AI configured.

## Architecture

### Transport — Streamable HTTP, mounted in the API binary

Use the SDK's `mcp.NewStreamableHTTPHandler(...)` (returns a plain `http.Handler`) and mount it on the existing chi router in `api.NewServer`:

```
r.Handle("/mcp", mcpHandler)
r.Handle("/mcp/*", mcpHandler)   // tolerate trailing-path clients
```

Because it's mounted **after** the existing middleware, `/mcp` automatically inherits:
- `requireBearer(token)` — the same `OPENMIND_TOKEN`; MCP clients send `Authorization: Bearer <token>`. No new auth code.
- the per-IP rate limiter (`rate.Limit(1)`, burst 10) — shared with the REST API. Adequate for an interactive agent; noted as a known ceiling (see Error handling).
- `devUser` — sets the user id in the request context.

Remote access works through the existing Cloudflare tunnel: `https://openmind.gilla.fun/mcp`. stdio is intentionally **out of scope** for this slice (see Follow-ups).

### Contract exception (deliberate)

`/mcp` is a JSON-RPC protocol endpoint, **not** a REST resource, so it is **not** added to `openapi.yaml` and is **not** part of the oapi-codegen surface. This is a conscious, documented exception to "never add a Go route that isn't in the spec": the MCP protocol negotiates its own schema over JSON-RPC. No codegen change; `make generate`/`task generate` output is unaffected.

### Package layout — isolated + testable

New package **`internal/mcp`** owns the MCP concern behind a narrow interface, so it is unit-testable against a fake and has no dependency on the HTTP/api layer (avoids an import cycle — `api` imports `mcp`, never the reverse):

```go
// internal/mcp/mcp.go
package mcp

// Backend is the capability surface the tools need. Implemented by *api.Server.
type Backend interface {
    Save(ctx context.Context, uid uuid.UUID, url, note string) (db.Item, error)
    Search(ctx context.Context, uid uuid.UUID, q, color string, parse bool) (SearchOutcome, error)
    ListRecent(ctx context.Context, uid uuid.UUID, limit int) ([]db.Item, error)
    GetItem(ctx context.Context, uid uuid.UUID, id uuid.UUID) (db.Item, error)
    ListLenses(ctx context.Context, uid uuid.UUID) ([]db.Lense, error)
    RunLens(ctx context.Context, uid uuid.UUID, id uuid.UUID) ([]search.Result, error)
}

// NewHandler builds the MCP server, registers the six tools, and returns the
// Streamable HTTP handler ready to mount. userID resolves the caller from the
// request context (today: always api.DevUserID via a small func passed in).
func NewHandler(b Backend, uidFor func(context.Context) uuid.UUID) http.Handler
```

`internal/mcp` imports only `store/db`, `search`, `github.com/google/uuid`, and the MCP SDK — no `api` import. It maps `db.Item`/`db.Lense`/`search.Result` to its own compact tool-output DTOs (below), so tool schemas are clean and don't leak the full OpenAPI models.

`internal/api` provides the adapter: `*api.Server` gains methods satisfying `Backend` (mostly one-line delegations to `s.store`, `search.Run`, `s.runLensRule`, and a new shared `capture` helper). `NewServer` builds the handler and mounts it. No change to `NewServer`'s signature or `main.go`.

### Shared capture helper (prevents drift)

Extract the save body of `CreateItem` into `func (s *Server) capture(ctx, uid, url, note string) (db.Item, error)` — validates exactly-one-of url/note, `CreateItem`s the row, enqueues `jobs.EnrichArgs` (best-effort, never fails the save). Both REST `CreateItem` and `Backend.Save` call it, so the MCP save path can't diverge from the REST one.

## Dependency

`github.com/modelcontextprotocol/go-sdk` **v1.x** (official SDK, stable 1.0.0/1.2.0; High source reputation). Justified new dependency: implementing MCP's JSON-RPC framing, capability negotiation, and JSON-Schema tool declaration by hand is infeasible and error-prone; this is the canonical implementation. Server-side only (Go `go.mod`); no TS/web impact. Approved-core list in `CLAUDE.md` gains this entry.

SDK shape used (v1.2.0):
- `mcp.NewServer(&mcp.Implementation{Name:"openmind", Version:…}, nil)`
- `mcp.AddTool[In,Out](server, &mcp.Tool{Name, Description}, handler)` — input/output JSON Schemas inferred from the `In`/`Out` structs.
- handler: `func(ctx, *mcp.ServerRequest[*mcp.CallToolParamsFor[In]]) (*mcp.CallToolResultFor[Out], error)` — returns `&mcp.CallToolResultFor[Out]{StructuredContent: out}`.
- `mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{})` — one shared server instance (single-user); request context flows into handlers so the user id resolves per request.

## Tool surface (6 tools)

All inputs/outputs are typed Go structs; the SDK generates the JSON Schema. Descriptions are written for an LLM caller.

| Tool | Input | Output | Backend call |
|------|-------|--------|--------------|
| `save_item` | `{ url?: string, note?: string }` (exactly one) | `SavedItem { id, url, status, createdAt }` | `Save` → `capture` |
| `search_items` | `{ query?: string, color?: string, parse?: bool }` (≥1 of query/color) | `SearchResults { results: SearchHit[] }` | `Search` → `search.Run` (+ optional `ParseQuery`) |
| `list_recent` | `{ limit?: int (default 20, max 200) }` | `ItemList { items: ItemSummary[] }` | `ListRecent` → `ListItems` |
| `get_item` | `{ id: string (uuid) }` | `ItemDetail { …summary fields…, body }` | `GetItem` → `GetItem` |
| `list_lenses` | `{}` | `LensList { lenses: LensInfo[] }` | `ListLenses` → `ListLenses` |
| `run_lens` | `{ id: string (uuid) }` | `SearchResults { results: SearchHit[] }` | `RunLens` → `runLensRule` |

Tool-output DTOs (defined in `internal/mcp`, minimal on purpose):
- `ItemSummary { id, url, title?, summary?, cardType?, status, tags[], userTags[], createdAt }`
- `SavedItem { id, url, status, createdAt }`
- `SearchHit { item: ItemSummary, score }`
- `ItemDetail = ItemSummary + { body }`
- `LensInfo { id, name, rule: { q?, color?, types? } }`

`SearchOutcome` (Backend → mcp) carries `[]search.Result` plus the optional understood-query echo, which `search_items` folds into a short human-readable note in the tool result text (e.g. `understood: "cabins" · color green · types image,article`). `parse` defaults to **true** for `search_items` (an agent phrasing a fuzzy query benefits from NL parsing; degrades to raw FTS under noop).

## Error handling

- Invalid tool input (both/neither of url/note; empty search; bad uuid; unknown colour) → return a tool error (`isError` result with a clear message), never a panic; the agent can correct and retry.
- `save_item` never fails on enrichment-enqueue error (capture is sacred) — mirrors REST.
- `get_item`/`run_lens` on unknown/foreign id → tool error "not found" (maps `pgx.ErrNoRows`), consistent with REST 404 + user scoping.
- `run_lens` with a stored colour that no longer parses → empty results (not an error), mirroring `GetLensItems`.
- **Rate limit:** `/mcp` shares the per-IP limiter (burst 10, 1/s refill). A burst of tool calls can hit 429; the SDK surfaces transport errors to the client. Documented as a known ceiling; raising or exempting `/mcp` is a follow-up if it bites. (No change to the limiter in this slice.)
- Auth: a missing/wrong Bearer token → 401 from `requireBearer` before MCP sees the request (same as REST).

## Testing / verification

- **Unit (`internal/mcp`):** a fake `Backend` + the SDK's in-memory client/transport (or direct handler invocation) covering each tool's happy path and error path (both-fields save, empty search, bad uuid, not-found). No DB, no network.
- **DB-backed e2e (api or a script):** against the compose stack (`docker compose up -d --build api web`, noop provider) with the Bearer token, drive raw JSON-RPC over HTTP: `initialize` → `tools/list` (asserts the 6 tools) → `tools/call save_item {url}` (201-equivalent, id returned) → `tools/call search_items {query}` (finds it once enriched) → `tools/call list_recent` / `get_item` / `list_lenses` / `run_lens`. Record outputs.
- **Live smoke:** after deploy, `curl https://openmind.gilla.fun/mcp` `initialize` with the real token returns the server info; note the exact `claude mcp add` command in docs.
- `go test ./...`, `go vet`, `golangci-lint` green; `task generate` unchanged (no contract change).

## Deployment

- Redeploy the API container on the VPS (existing runbook: rebuild `api`, `docker restart cloudflared` after web changes — here only `api` changes, but restart cloudflared if its origin IP moved). No migration (no schema change). No new env var (reuses `OPENMIND_TOKEN`).
- `docs/self-hosting.md` gains an **MCP** section: the `/mcp` URL, that it uses the same token, and copy-paste client config:
  - Claude Code: `claude mcp add --transport http openmind https://<instance>/mcp --header "Authorization: Bearer <token>"`
  - Claude Desktop: the `mcpServers` JSON (via `mcp-remote` if a native HTTP entry isn't available), noting the header.

## Out of scope / follow-ups

- **stdio transport** (`openmind mcp`) for co-located local self-host — add later if wanted.
- **Write tools beyond save**: tagging (`set_user_tags`), pinning to Desk, creating Lenses, delete — deliberately omitted from the first slice (read + capture is the core loop; mutating tools widen blast radius and want more thought on agent safety).
- **Desk / Drift tools** — easy fast-follow once the pattern is proven.
- **MCP resources / prompts** — only tools in v1; resources (e.g. exposing an item as a resource) can follow.
- **Per-endpoint rate-limit tuning** for `/mcp`.

## Execution

Subagent-driven (superpowers:subagent-driven-development). Rough task split: (1) dependency + `internal/mcp` package (Backend interface, DTOs, six tool handlers, `NewHandler`) + unit tests with a fake backend; (2) `internal/api` adapter (`capture` helper + `Backend` methods on `*Server`) + mount `/mcp` in `NewServer`; (3) DB-backed e2e + deploy + `docs/self-hosting.md` + `TODO.md`. Per-task review; whole-branch review before merge.
