# Expose-Ready API + Web Capture/Grid — Design

Date: 2026-07-03 · Status: Approved design, pre-implementation · Follows Milestone 0 (spec `20260703-milestone-0-spike-design.md`)

## Goal

Make the deployed instance safe to put behind a public hostname and give it a usable web UI: SSRF-hardened extraction, single-user token auth, basic rate limiting, note capture, and a Next.js app (login → masonry grid → quick-add → search) that fronts the API server-side. Outcome: `openmind.gilla.fun → web:3000`; the API container never gets public exposure.

## Section 1 — API expose-readiness

**SSRF guard** (`internal/enrich`):
- `SafeHTTPClient(timeout)` shared by trafilatura/readability/jina extractors.
- `net.Dialer.Control` hook rejects connections to loopback, RFC1918 private, link-local (169.254/16, fe80::/10), unique-local (fc00::/7), unspecified, and multicast addresses — checked at dial time so DNS rebinding and every redirect hop are covered.
- Redirect cap (max 5) via `CheckRedirect`.
- Unit tests: dial refused for `http://127.0.0.1`, `http://169.254.169.254`, `http://10.0.0.1`; public addresses pass the guard function (pure-function tests on the IP checker; no live network).

**Auth** (`internal/api`):
- `OPENMIND_TOKEN` env var. When non-empty: middleware requires `Authorization: Bearer <token>` (constant-time compare) on all routes except `GET /healthz`. 401 otherwise.
- When empty: middleware disabled; `slog.Warn` at startup ("API is unauthenticated — set OPENMIND_TOKEN before exposing it").
- Compose passes `OPENMIND_TOKEN: ${OPENMIND_TOKEN:-}` to api and web services; documented in `.env.example` and `docs/self-hosting.md`.

**Rate limiting** (`internal/api`):
- Per-client-IP token bucket via `golang.org/x/time/rate` (justified dep: golang.org/x sub-repo), applied to `POST /items` and `GET /search`: 60 req/min, burst 10. In-memory map with periodic pruning. 429 on limit.
- Client IP: `X-Forwarded-For` first hop if present (we are always behind the web proxy/tunnel), else RemoteAddr.

**Contract changes** (`openapi.yaml` → `task generate`):
- `securitySchemes: bearerAuth` (http bearer), applied globally; `/healthz` documented with empty security.
- `GET /healthz` → 200 `{"status":"ok"}`.
- `CreateItemRequest`: `url` becomes optional, add optional `note` (string, 1–10000 chars). Exactly one of `url`/`note` must be present (validated in handler; 400 otherwise). Note saves get `cardType: note`.

**Pipeline: note items** (`internal/enrich`):
- `Pipeline.Run` on an item with empty URL and non-empty body-note: skip extraction, classify as `note`, run summarise/tag/embed on the note text. With noop provider the note is stored verbatim and FTS-searchable. Store change: `CreateItem` gains a `body` param (notes store their text in `items.body` at save time; URL items still start empty). Idempotency test extended to a note item.

## Section 2 — Web app (`apps/web`)

**Auth flow**:
- `/login` page: single token input. Posts to a route handler that verifies the token against the API (`GET /items?limit=1` with Bearer) and, on success, sets an httpOnly, Secure, SameSite=Lax cookie holding the token. Logout clears it.
- Next.js middleware redirects unauthenticated requests (no cookie) to `/login`.
- All data access goes through server-side route handlers / server components that read the cookie and call the API (`API_URL` env, default `http://localhost:8080`; in compose, `http://api:8080`) with the Bearer header. The token never reaches client JS.

**UI** (tokens from `packages/ui`, look per `docs/design/README.md`):
- Grid: CSS-columns masonry (no virtualisation yet — that's the separate perf-spike TODO). Type-aware cards: article (lead image, title, summary, domain), tweet (quote-styled text, author line), video (thumbnail + title), image (full-bleed image), note (text on paper card, Newsreader italic per tokens). Pending items render with a subtle "enriching…" state.
- Quick-add bar at top: one input; if it parses as an http(s) URL → save as URL, else save as note. POST via server route handler; refresh list on success.
- Search box: queries `/search` (server-side), renders results in the same grid; clearing returns to recents. Simple `useTransition` + router refresh; no client state library (YAGNI).

**Deployment**:
- `apps/web` gets `output: "standalone"`, a Dockerfile (node:22-alpine build → standalone runner), and a `web` service in compose (`127.0.0.1:3000:3000`, env `API_URL=http://api:8080`, `OPENMIND_TOKEN` shared). Server override file gains the web port if 3000 is taken on the deploy box.
- Mapping when ready: `openmind.gilla.fun → http://localhost:3000` (Cloudflare dashboard). API stays localhost-only.

## Testing

- Go: table-driven tests for the IP guard; auth middleware tests (401/200/healthz-exempt, constant-time compare); rate-limit test (burst then 429); note-item pipeline + idempotency tests; handler test for url/note validation.
- Web: `tsc --noEmit` + build; manual e2e on the deploy box (login, add URL, add note, search) recorded in the task report. No JS test framework this slice (YAGNI).

## Out of scope

Real multi-user auth, WXT extension, card detail/reader view, virtualised grid, JSON export, Jina fallback wiring, AI fallback chain.

## Execution model

Same as Milestone 0: subagent-driven (fresh implementer + reviewer per task), Sonnet for mechanical tasks, Opus for integration-heavy ones, final whole-branch review before merge and redeploy.
