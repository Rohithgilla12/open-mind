# Expose-Ready API + Web Capture/Grid Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SSRF-hardened extraction, bearer-token auth + rate limiting, note capture, and a Next.js login/grid/quick-add/search UI, deployable as `web` in compose so only the web app is ever exposed publicly.

**Architecture:** API changes live in `internal/enrich` (safe HTTP client, note pipeline), `internal/api` (auth + rate-limit middleware), and the contract (`openapi.yaml` → regenerate). The web app authenticates once via token → httpOnly cookie; all API calls happen server-side (route handlers / server components) with the Bearer header, so the API container stays localhost-only. Spec: `docs/superpowers/specs/20260703-expose-ready-web-design.md`.

**Tech Stack:** Existing Go stack + `golang.org/x/time/rate`; Next.js 15 App Router (no new JS deps), compose `web` service (node:22-alpine standalone).

## Global Constraints

- Capture stays sacred: `POST /items` never calls AI or fetches URLs; note or URL, still <100ms.
- Exactly one of `url`/`note` in CreateItemRequest; note items get `cardType: note`, text stored in `items.body` at save; notes skip extraction, are FTS-searchable under noop.
- SSRF guard rejects loopback / RFC1918 / link-local (169.254/16, fe80::/10) / unique-local (fc00::/7) / unspecified / multicast at **dial time** (covers DNS rebinding + every redirect hop); redirect cap 5. All three extractors use it.
- `OPENMIND_TOKEN` set → Bearer required (constant-time compare) on all routes except `GET /healthz`; unset → disabled + `slog.Warn` at startup.
- Rate limit: 60 req/min burst 10 per client IP on `POST /items` and `GET /search`; 429 over limit; X-Forwarded-For first hop else RemoteAddr.
- Token never reaches browser JS: httpOnly Secure SameSite=Lax cookie, server-side proxying only. `API_URL` env (compose: `http://api:8080`).
- No new required infra; no hand-editing generated code; no banner comments; errors `fmt.Errorf("doing x: %w", err)`; every query user-scoped.
- Design tokens only from `packages/ui` — no hardcoded colours in apps. Card types vocabulary: article, product, book, recipe, video, tweet, image, note, quote.
- Store tests / integration tests: real Postgres `postgres://openmind:openmind@localhost:5433/openmind_test` (compose db), no t.Parallel, suite runs via `go test -p 1 ./...`.
- If a library API doesn't compile as written, consult context7 rather than guessing.
- Commit after every task; never amend/force-push.

---

### Task 1: SSRF-safe HTTP client

**Files:**
- Create: `apps/api/internal/enrich/safehttp.go`
- Test: `apps/api/internal/enrich/safehttp_test.go`
- Modify: `apps/api/internal/enrich/trafilatura.go` (nil-client default), `readability.go`, `jina.go` (same pattern)

**Interfaces:**
- Produces: `enrich.SafeHTTPClient(timeout time.Duration) *http.Client`; `enrich.BlockedIP(ip net.IP) bool` (exported for tests). All extractors' nil-client fallback becomes `SafeHTTPClient(30 * time.Second)`.

- [ ] **Step 1: Write failing tests**

```go
package enrich_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/rohithgilla12/openmind/api/internal/enrich"
)

func TestBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"::1", true},
		{"fe80::1", true},
		{"fd00::1", true},
		{"0.0.0.0", true},
		{"224.0.0.1", true},
		{"8.8.8.8", false},
		{"142.250.72.14", false},
		{"2607:f8b0::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := enrich.BlockedIP(net.ParseIP(tt.ip)); got != tt.blocked {
				t.Errorf("BlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestSafeClientRefusesLoopback(t *testing.T) {
	client := enrich.SafeHTTPClient(5 * time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://127.0.0.1:1/", nil)
	_, err := client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected blocked-address error, got %v", err)
	}
}
```
(add `net/http` import)

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/api && go test ./internal/enrich/ -run 'TestBlockedIP|TestSafeClient' -v` → FAIL (undefined).

- [ ] **Step 3: Implement `safehttp.go`**

```go
package enrich

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// BlockedIP reports whether the address must never be fetched: loopback,
// private, link-local, unique-local, unspecified, or multicast ranges.
func BlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() ||
		(ip.To4() == nil && len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc)
}

// SafeHTTPClient returns a client for fetching user-supplied URLs. The dial
// Control hook runs after DNS resolution, so rebinding and redirects to
// internal addresses are rejected at every hop.
func SafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("parsing dial address %s: %w", address, err)
			}
			if BlockedIP(net.ParseIP(host)) {
				return fmt.Errorf("blocked address %s", host)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects fetching %s", via[0].URL)
			}
			return nil
		},
	}
}
```

Note: `ip.IsPrivate()` covers RFC1918 + fc00::/7 in modern Go — verify with the test; the explicit fc00 check is belt-and-braces, drop it if `IsPrivate` already passes the `fd00::1` case.

- [ ] **Step 4: Swap nil-client defaults in all three extractors**

In `trafilatura.go`, `readability.go`, `jina.go`, replace the nil-client fallback (`&http.Client{Timeout: 30 * time.Second}` or equivalent) with `SafeHTTPClient(30 * time.Second)` and update the doc comments. Existing extractor tests pass httptest clients, so they still work.

- [ ] **Step 5: Run full package, commit**

Run: `go test -p 1 ./... && go vet ./...` → PASS.
```bash
git add apps/api && git commit -m "feat(enrich): ssrf-safe http client for extractor fetches"
```

---

### Task 2: Contract — bearerAuth, /healthz, note capture

**Files:**
- Modify: `openapi.yaml`
- Generated: `apps/api/internal/api/gen.go`, `packages/api-client/src/schema.d.ts`

**Interfaces:**
- Produces: `api.CreateItemRequest{Url *string, Note *string}` (both now optional pointers — Task 4 revalidates), `GetHealthz` on ServerInterface, bearer scheme in both generated artifacts. Item schema unchanged (notes carry `url: ""`).

- [ ] **Step 1: Edit `openapi.yaml`**

Add under `components`:
```yaml
  securitySchemes:
    bearerAuth: { type: http, scheme: bearer }
```
Top level after `info`:
```yaml
security:
  - bearerAuth: []
```
New path:
```yaml
  /healthz:
    get:
      operationId: getHealthz
      security: []
      responses:
        "200":
          description: liveness
          content:
            application/json:
              schema:
                type: object
                required: [status]
                properties:
                  status: { type: string }
```
Change CreateItemRequest:
```yaml
    CreateItemRequest:
      type: object
      properties:
        url: { type: string, format: uri }
        note: { type: string, minLength: 1, maxLength: 10000 }
```
(drop `required: [url]` — exactly-one enforced in the handler; add `description: "Exactly one of url or note must be provided."`)

- [ ] **Step 2: Regenerate + fix compile**

Run: `task generate` (regenerates gen.go, sqlc unchanged, TS schema). `apps/api` now fails to compile: `CreateItem` handler reads `req.Url` as string — update in Task 4; for THIS task, adjust the handler minimally to keep compiling: `if req.Url == nil || !validURL(*req.Url) { 400 }` and use `*req.Url` (note handling lands in Task 4). Implement `GetHealthz` on the server returning `{"status":"ok"}` with no auth dependency.

- [ ] **Step 3: Verify + commit**

Run: `go test -p 1 ./... && go vet ./...` → PASS (existing handler tests unchanged: they send `url`).
Run: `pnpm --filter @openmind/api-client generate && pnpm turbo run build --filter=web` → PASS.
```bash
git add openapi.yaml apps/api packages/api-client && git commit -m "feat(contract): bearer auth scheme, healthz, note capture field"
```

---

### Task 3: Note items — store + pipeline

**Files:**
- Modify: `apps/api/internal/store/queries/items.sql` (CreateItem gains body), regenerate sqlc
- Modify: `apps/api/internal/enrich/pipeline.go`, `apps/api/internal/enrich/classify.go`
- Modify callers of `CreateItemParams` (api server, tests)
- Test: extend `apps/api/internal/enrich/pipeline_test.go`

**Interfaces:**
- Consumes: `db.CreateItemParams` (gains `Body string`), `ai.Provider`.
- Produces: `Pipeline.Run` handles note items (URL == ""): no extraction, `cardType note`, title = first line of body truncated to 80 runes, summarise/tag/embed over the note text. `db.CreateItemParams{UserID, Url, Body}` — Task 4's handler passes note text as Body with Url "".

- [ ] **Step 1: sqlc change**

```sql
-- name: CreateItem :one
INSERT INTO items (user_id, url, body) VALUES ($1, $2, $3) RETURNING *;
```
Run `task generate:sqlc`; update existing callers (`server.go`, `store_test.go`, `pipeline_test.go`, `search_test.go`) to add `Body: ""` (or note text where relevant).

- [ ] **Step 2: Failing pipeline test**

```go
func TestPipelineNoteItemIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := uuid.New()
	s.Queries.EnsureUser(ctx, userID)
	note := "Grocery run ideas\nBuy sourdough starter and rye flour for the weekend bake."
	item, err := s.Queries.CreateItem(ctx, db.CreateItemParams{UserID: userID, Url: "", Body: note})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	p := &enrich.Pipeline{Store: s, AI: ai.NewFake(), Extractor: failingExtractor{}}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("first run: %v", err)
	}
	got, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if got.CardType != "note" || got.Status != "enriched" {
		t.Fatalf("cardType=%q status=%q", got.CardType, got.Status)
	}
	if got.Title != "Grocery run ideas" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Body != note {
		t.Errorf("body was rewritten")
	}
	if err := p.Run(ctx, userID, item.ID); err != nil {
		t.Fatalf("second run: %v", err)
	}
	again, _ := s.Queries.GetItem(ctx, db.GetItemParams{UserID: userID, ID: item.ID})
	if again.Title != got.Title || again.Summary != got.Summary || again.Body != got.Body {
		t.Errorf("note pipeline not idempotent")
	}
}

// failingExtractor proves notes never touch the extractor.
type failingExtractor struct{}

func (failingExtractor) Name() string { return "failing" }
func (failingExtractor) Extract(context.Context, string) (enrich.Extraction, error) {
	return enrich.Extraction{}, fmt.Errorf("extractor must not be called for notes")
}
```

- [ ] **Step 3: Run → FAIL. Step 4: Implement**

In `pipeline.go`, at the top of `Run` after loading the item:
```go
	if item.Url == "" {
		return p.runNote(ctx, userID, item)
	}
```
```go
func (p *Pipeline) runNote(ctx context.Context, userID uuid.UUID, item db.Item) error {
	q := p.Store.Queries
	title := noteTitle(item.Body)
	if err := q.UpdateItemExtraction(ctx, db.UpdateItemExtractionParams{
		UserID: userID, ID: item.ID,
		Title: title, Body: item.Body, LeadImageUrl: "", CardType: "note",
	}); err != nil {
		return fmt.Errorf("saving note metadata: %w", err)
	}
	return p.enrichText(ctx, userID, item.ID, title, item.Body)
}
```
Refactor the existing URL path's summarise/tag/embed/status block into `enrichText(ctx, userID, itemID, title, body string) error` used by both paths (keeps DRY, preserves the ErrNotSupported/dimension-guard behaviour). `noteTitle`: first line, `strings.TrimSpace`, truncate to 80 runes.

- [ ] **Step 5: Run full suite → PASS. Commit**

```bash
git add apps/api && git commit -m "feat(enrich): note items skip extraction, shared text-enrichment path"
```

---

### Task 4: Auth + rate limit middleware, note handler, cmd wiring

**Files:**
- Create: `apps/api/internal/api/auth.go`, `apps/api/internal/api/ratelimit.go`
- Modify: `apps/api/internal/api/server.go` (CreateItem url/note validation; middleware wiring), `apps/api/cmd/openmind/main.go` (startup warn), `docker-compose.yml`, `.env.example`, `docs/self-hosting.md`
- Test: `apps/api/internal/api/auth_test.go`, `ratelimit_test.go`, extend `server_test.go`

**Interfaces:**
- Consumes: generated `CreateItemRequest{Url, Note *string}`, `GetHealthz` (Task 2), `db.CreateItemParams{UserID, Url, Body}` (Task 3).
- Produces: `api.NewServer(store, riverClient, provider, token string) http.Handler` — note the new `token` param (cmd passes `os.Getenv("OPENMIND_TOKEN")`); `requireBearer(token string) func(http.Handler) http.Handler`; `rateLimit(rps rate.Limit, burst int) func(http.Handler) http.Handler`.

- [ ] **Step 1: Failing auth tests**

```go
func TestBearerAuth(t *testing.T) {
	srv := httptest.NewServer(api.NewServer(s, riverClient, ai.NewNoop(), "sekret"))
	defer srv.Close()
	tests := []struct {
		name, path, header string
		want               int
	}{
		{"no token", "/items", "", 401},
		{"wrong token", "/items", "Bearer nope", 401},
		{"right token", "/items", "Bearer sekret", 200},
		{"healthz exempt", "/healthz", "", 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Errorf("%s = %d, want %d", tt.path, resp.StatusCode, tt.want)
			}
		})
	}
}

func TestAuthDisabledWhenTokenEmpty(t *testing.T) {
	srv := httptest.NewServer(api.NewServer(s, riverClient, ai.NewNoop(), ""))
	defer srv.Close()
	// GET /items with no header → 200
}
```

- [ ] **Step 2: Failing rate-limit test**

```go
func TestRateLimit429(t *testing.T) {
	h := api.NewServer(s, riverClient, ai.NewNoop(), "")
	srv := httptest.NewServer(h)
	defer srv.Close()
	var last int
	for i := 0; i < 12; i++ {
		resp, _ := http.Get(srv.URL + "/search?q=x")
		last = resp.StatusCode
		resp.Body.Close()
	}
	if last != 429 {
		t.Errorf("12th rapid request = %d, want 429", last)
	}
}
```
(burst 10 → requests 11+ within the same second must 429; keep the loop tight, no sleeps)

- [ ] **Step 3: Implement**

`auth.go`:
```go
package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func requireBearer(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

`ratelimit.go`: `map[string]*rate.Limiter` guarded by `sync.Mutex`, `clientIP(r)` = first comma-separated hop of `X-Forwarded-For` else host of RemoteAddr; limiter created `rate.NewLimiter(rate.Limit(1), 10)` (60/min = 1/sec); prune entries idle >10min on each 100th lookup. Middleware applies only when `(r.Method == POST && r.URL.Path == "/items") || (r.Method == GET && r.URL.Path == "/search")`; over-limit → 429 JSON. `go get golang.org/x/time`.

`server.go`: `NewServer(..., token string)`; wiring order `r.Use(devUser)`, `if token != "" { r.Use(requireBearer(token)) }`, `r.Use(rateLimit(1, 10))`. CreateItem validation:
```go
	hasURL := req.Url != nil && *req.Url != ""
	hasNote := req.Note != nil && strings.TrimSpace(*req.Note) != ""
	if hasURL == hasNote { // both or neither
		http.Error(w, `{"error":"provide exactly one of url or note"}`, http.StatusBadRequest)
		return
	}
```
URL branch unchanged (`validURL`); note branch: `CreateItem(ctx, db.CreateItemParams{UserID: uid, Url: "", Body: strings.TrimSpace(*req.Note)})`, same enqueue.

`cmd/openmind/main.go`: read `OPENMIND_TOKEN`, pass to NewServer, and when empty log `slog.Warn("API is unauthenticated — set OPENMIND_TOKEN before exposing it")`.

`docker-compose.yml` api env: `OPENMIND_TOKEN: ${OPENMIND_TOKEN:-}`. `.env.example`: `OPENMIND_TOKEN=` with comment. `docs/self-hosting.md`: document token + note capture.

- [ ] **Step 4: Add note-save handler test** (`POST /items {"note":"remember the milk"}` → 201, cardType note after enrich isn't needed here — assert 201 + pending + job row; and `{"url":..,"note":..}` → 400, `{}` → 400).

- [ ] **Step 5: Full suite + commit**

Run: `go test -p 1 ./... && go vet ./...` → PASS.
```bash
git add -A && git commit -m "feat(api): bearer auth, per-ip rate limiting, note capture endpoint"
```

---

### Task 5: Web auth — login, cookie, server-side API helper

**Files:**
- Create: `apps/web/middleware.ts`, `apps/web/app/login/page.tsx`, `apps/web/app/login/LoginForm.tsx`, `apps/web/app/api/auth/route.ts`, `apps/web/lib/api.ts`
- Modify: `apps/web/app/page.tsx` (use lib/api), `apps/web/package.json` if needed

**Interfaces:**
- Consumes: API `GET /items?limit=1` for token validation; `@openmind/api-client` types.
- Produces: `lib/api.ts` exports `apiFetch(path: string, init?: RequestInit): Promise<Response>` (server-only: reads cookie `om_token`, adds Bearer, base `process.env.API_URL ?? "http://localhost:8080"`). Cookie name `om_token`. Task 6 consumes `apiFetch`.

- [ ] **Step 1: `app/api/auth/route.ts`**

```ts
import { cookies } from "next/headers";
import { NextResponse } from "next/server";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

export async function POST(req: Request) {
  const { token } = (await req.json()) as { token?: string };
  if (!token) return NextResponse.json({ error: "token required" }, { status: 400 });
  const probe = await fetch(`${API_URL}/items?limit=1`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!probe.ok) return NextResponse.json({ error: "invalid token" }, { status: 401 });
  (await cookies()).set("om_token", token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 90,
  });
  return NextResponse.json({ ok: true });
}

export async function DELETE() {
  (await cookies()).delete("om_token");
  return NextResponse.json({ ok: true });
}
```

- [ ] **Step 2: `middleware.ts`**

```ts
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function middleware(req: NextRequest) {
  const hasToken = req.cookies.has("om_token");
  const isLogin = req.nextUrl.pathname.startsWith("/login");
  if (!hasToken && !isLogin) {
    return NextResponse.redirect(new URL("/login", req.url));
  }
  if (hasToken && isLogin) {
    return NextResponse.redirect(new URL("/", req.url));
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next|api/auth|favicon.ico).*)"],
};
```

- [ ] **Step 3: `lib/api.ts`**

```ts
import "server-only";
import { cookies } from "next/headers";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const token = (await cookies()).get("om_token")?.value;
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init?.body) headers.set("content-type", "application/json");
  return fetch(`${API_URL}${path}`, { ...init, headers, cache: "no-store" });
}
```
(`pnpm --filter web add server-only`)

- [ ] **Step 4: Login page** — server component rendering `LoginForm` (client component: token input styled with ui tokens, posts to `/api/auth`, on 200 `router.push("/")`, on 401 inline error). Update `app/page.tsx` to use `apiFetch("/items")`.

- [ ] **Step 5: Verify + commit**

Run: `pnpm turbo run build --filter=web && pnpm turbo run lint` → PASS.
```bash
git add apps/web && git commit -m "feat(web): token login, httpOnly cookie, server-side api proxy"
```

---

### Task 6: Web UI — grid, cards, quick-add, search

**Files:**
- Create: `apps/web/components/Grid.tsx`, `apps/web/components/ItemCard.tsx`, `apps/web/components/QuickAdd.tsx`, `apps/web/components/SearchBox.tsx`, `apps/web/app/api/items/route.ts`, `apps/web/app/globals.css`
- Modify: `apps/web/app/page.tsx`, `apps/web/app/layout.tsx`

**Interfaces:**
- Consumes: `apiFetch` (Task 5); API `GET /items`, `GET /search?q=`, `POST /items {url|note}`; `tokens` from `@openmind/ui`; Item type derived from `@openmind/api-client` paths.
- Produces: `/` renders grid of recents, `/?q=term` renders search results; QuickAdd posts then `router.refresh()`.

- [ ] **Step 1: `app/api/items/route.ts`** (proxy for the client-side QuickAdd)

```ts
import { NextResponse } from "next/server";
import { apiFetch } from "../../../lib/api";

export async function POST(req: Request) {
  const body = await req.text();
  const res = await apiFetch("/items", { method: "POST", body });
  return new NextResponse(await res.text(), {
    status: res.status,
    headers: { "content-type": "application/json" },
  });
}
```

- [ ] **Step 2: `page.tsx`** — server component: `searchParams: Promise<{ q?: string }>`; when `q` present fetch `/search?q=` and map `r.item` + score, else `/items`; render `<QuickAdd />`, `<SearchBox initial={q} />`, `<Grid items={items} />`. Empty state: "Nothing here yet — drop a link or a thought above." Pending items pass `status` through.

- [ ] **Step 3: Components**

`Grid.tsx` (server): `<div className="grid">` using CSS columns (`globals.css`: `.grid { columns: 4 280px; column-gap: 16px; } .grid > * { break-inside: avoid; margin-bottom: 16px; }` with paper background/typography base from tokens applied in `layout.tsx`).

`ItemCard.tsx` (server): switch on `cardType`:
- `note`: body text (clamp 8 lines), Newsreader italic per `tokens.font.quote`.
- `tweet`: quote-styled body/summary + domain line.
- `image`: `leadImageUrl` full-bleed `<img>` (plain img, `max-width:100%`).
- `video`: leadImageUrl thumb + title.
- default (article/product/recipe/book/quote): leadImageUrl if any, title, summary (clamp 4), domain from url via `new URL(item.url).hostname` guarded for empty url.
- `status === "pending"`: subtle "enriching…" caption in `tokens.color.cobalt`.
Card chrome: white surface, `1px solid tokens.color.line`, radius 10px, padding 14px — via a shared inline style object in the file.

`QuickAdd.tsx` (client): single input + button; on submit, `const isURL = /^https?:\/\//i.test(v.trim())` → body `{url}` or `{note}`; `fetch("/api/items", {method:"POST", body: JSON.stringify(...)})`; on 201 clear input + `router.refresh()`; on error inline message. Disabled while submitting via `useTransition`.

`SearchBox.tsx` (client): input; on submit `router.push(q ? `/?q=${encodeURIComponent(q)}` : "/")`.

- [ ] **Step 4: Verify + commit**

Run: `pnpm turbo run build --filter=web && pnpm turbo run lint` → PASS. Manual: `task db` running, start api with `OPENMIND_TOKEN=devtoken`, `pnpm --filter web dev` **only if no dev server already runs — check with the controller/user first per workspace rules; otherwise verify via `next build` + the deploy-box e2e in Task 7**.
```bash
git add apps/web packages && git commit -m "feat(web): masonry grid, type-aware cards, quick-add, search"
```

---

### Task 7: Web container + deploy + wrap-up

**Files:**
- Create: `apps/web/Dockerfile`, modify `apps/web/next.config.ts` (`output: "standalone"`)
- Modify: `docker-compose.yml` (web service), `TODO.md`, `docs/self-hosting.md`

**Interfaces:**
- Consumes: everything prior.
- Produces: compose `web` service on `127.0.0.1:3000`, env `API_URL=http://api:8080`, `OPENMIND_TOKEN` shared with api.

- [ ] **Step 1: Dockerfile**

```dockerfile
FROM node:22-alpine AS deps
WORKDIR /repo
RUN corepack enable
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/api-client/package.json packages/api-client/
COPY packages/ui/package.json packages/ui/
RUN pnpm install --frozen-lockfile

FROM deps AS build
COPY openapi.yaml ./
COPY packages ./packages
COPY apps/web ./apps/web
RUN pnpm --filter web build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /repo/apps/web/.next/standalone ./
COPY --from=build /repo/apps/web/.next/static ./apps/web/.next/static
EXPOSE 3000
CMD ["node", "apps/web/server.js"]
```
(standalone output path may nest under `apps/web` — adjust COPY/CMD to what `next build` actually emits; verify by listing `.next/standalone`.)

- [ ] **Step 2: compose**

```yaml
  web:
    build:
      context: .
      dockerfile: apps/web/Dockerfile
    environment:
      API_URL: http://api:8080
      OPENMIND_TOKEN: ${OPENMIND_TOKEN:-}
    ports:
      - "127.0.0.1:3000:3000"
    depends_on:
      - api
```

- [ ] **Step 3: Local verify**

`OPENMIND_TOKEN=devtoken docker compose up -d --build` → login at `http://localhost:3000/login` with `devtoken`, add a URL and a note, search — record evidence. `docker compose stop web api` after.

- [ ] **Step 4: Docs + TODO**

`docs/self-hosting.md`: web service, token setup, "map your domain to web:3000 only". `TODO.md`: move this slice to Done (auth+SSRF item included), keep remaining Next items.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat(deploy): web container + compose service, self-hosting docs"
```

- [ ] **Step 6: Deploy (controller step)** — merge to main, push, rsync to `ubuntu@150.230.131.66:~/open-mind/`, set `OPENMIND_TOKEN` + optional `GEMINI_API_KEY` in server `.env`, extend the server-only `docker-compose.override.yml` for the web port if 3000 is taken, `docker compose up -d --build`, e2e via ssh curl, then tell the user the hostname → port mapping.
