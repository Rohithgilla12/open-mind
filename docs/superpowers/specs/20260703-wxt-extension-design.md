# WXT Capture Extension — Design

Date: 2026-07-03 · Status: Approved design, pre-implementation · Follows expose-ready-web (spec `20260703-expose-ready-web-design.md`)

## Goal

Capture from anywhere in the browser: save the current page, a text selection, or an image to Openmind via the toolbar and context menus. The extension is a thin client (CLAUDE.md: capture + display only) talking to the deployed web app's `/api/*` routes with a Bearer token — the Go API stays private.

## Section 1 — Web routes accept Bearer

- `apps/web/lib/api.ts`: token resolution becomes cookie **or** incoming `Authorization: Bearer` header. Route handlers pass their `Request` through so the header can be read; server components keep cookie-only.
- New `GET /api/auth/check`: forwards a cheap probe (`GET /items?limit=1`) with the resolved token; 200 on success, 401 invalid, 429 passed through, 502 unreachable. This is the extension's validate endpoint.
- `apps/web/middleware.ts`: matcher (or in-function check) excludes all `/api/` paths from the login redirect — API-shaped routes return JSON 401 rather than a 307 to an HTML page (fixes the previously logged matcher trap).
- No change to the Go API: rate limiting and bearer auth already apply behind the proxy.

## Section 2 — Backend: image-URL saves

`POST /items {url}` where the URL is a direct image must not go through text extraction (binary → trafilatura error → `failed`).

- `internal/enrich/pipeline.go`: before extraction, detect image URLs — path extension in {png,jpg,jpeg,gif,webp,avif} (query-string tolerant), OR a HEAD/GET `Content-Type: image/*` sniff on the existing safe client. On match: skip extraction, set `leadImageUrl = url`, `cardType = "image"`, title = filename stem, then the shared `enrichText` path over the title. Idempotent; covered by a pipeline test with an httptest server serving `image/png`.

## Section 3 — Extension (`apps/extension`, WXT + React, MV3)

- **Storage/settings**: `browser.storage.local` holds `{ instanceUrl, token }`. Options page: two inputs (instance URL defaulting to `https://openmind.gilla.fun`, token), a Validate button calling `GET {instanceUrl}/api/auth/check` with Bearer, inline success/error. Styling from `packages/ui` tokens.
- **Popup**: current tab favicon/title/URL; "Save page" button → `POST {instanceUrl}/api/items {url}` with Bearer; states: saving → saved (badge flash `✓`) / error (message + link to options when 401/unset).
- **Context menus** (background service worker):
  - "Save selection to Openmind" (context: selection) → note body = selection text + `\n\n— <page URL>` → `POST {note}`.
  - "Save image to Openmind" (context: image) → `POST {url: srcUrl}`.
  - Both surface result via badge flash (and basic `notifications` permission on failure).
- **Permissions**: `storage`, `activeTab`, `contextMenus`, `notifications`; host permissions limited to the configured instance origin (`optional_host_permissions` with request-on-save if feasible in WXT; otherwise `<all_urls>` justified in the README — decide at implementation, prefer narrow).
- **Builds**: `pnpm --filter extension build` (Chrome MV3) + `build:firefox`. Extension joins the pnpm workspace + turbo (build/lint scripts, strict TS). No test framework; `tsc --noEmit` as lint plus manual e2e against the deployed instance recorded in the task report.

## Testing

- Go: pipeline image-URL test (skips extractor, cardType image, leadImageUrl set, idempotent re-run).
- Web: build + `tsc`; a Bearer-path check is exercised via the extension e2e (curl equivalent recorded).
- Extension: `tsc --noEmit`, WXT build both targets, manual e2e: validate token, save page, save selection, save image against the live deployment.

## Out of scope

Display/browse inside the extension, offline queueing, Safari build, keyboard shortcuts, quote card type (selections save as notes), multi-account.

## Execution model

Subagent-driven as before; final whole-branch review before merge; deploy = web app redeploy (Section 1/2 changes) + local extension load instructions for the user (no store publishing this slice).
