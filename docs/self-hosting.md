# Self-hosting Openmind

Openmind self-hosts as **Postgres + one Go binary**. No Redis, no Python sidecars — `docker compose up` is the whole deployment.

## Quickstart

```bash
git clone <repo> && cd open-mind
cp .env.example .env      # optional: defaults work out of the box
docker compose up -d      # starts Postgres (pgvector) + the API/worker binary
```

This starts three services:

| Service | Bound to | Purpose |
|---|---|---|
| `db` | `127.0.0.1:5433` | Postgres + pgvector (persistent volume) |
| `api` | `127.0.0.1:8080` | Go API + River enrichment worker (one binary) |
| `web` | `127.0.0.1:3000` | Next.js web UI |

The API listens on `http://localhost:8080`; the web UI on `http://localhost:3000`. Migrations run automatically on api start.

Smoke test:

```bash
# Save a URL (returns instantly with status "pending")
curl -s -XPOST localhost:8080/items \
  -d '{"url":"https://paulgraham.com/greatwork.html"}' \
  -H 'content-type: application/json'

# After a few seconds, enrichment finishes (status "enriched")
curl -s localhost:8080/items | python3 -m json.tool

# Find it via full-text search
curl -s 'localhost:8080/search?q=great' | python3 -m json.tool

# Save a plain note instead of a URL (exactly one of url or note per save)
curl -s -XPOST localhost:8080/items \
  -d '{"note":"remember the milk"}' \
  -H 'content-type: application/json'
```

## Authentication

By default the API is **unauthenticated** — convenient for single-user local use, but the binary logs a warning on startup and you must not expose it to a network as-is.

Set `OPENMIND_TOKEN` to a strong secret to require a bearer token on every request (`/healthz` stays exempt for load-balancer probes):

```bash
OPENMIND_TOKEN=$(openssl rand -hex 32) docker compose up -d

curl -s localhost:8080/items -H "Authorization: Bearer $OPENMIND_TOKEN"
```

Requests with a missing or wrong token get `401`. The write and search endpoints (`POST /items`, `GET /search`) are additionally rate-limited per client IP (60 requests/minute, burst 10); over-limit requests get `429`.

### Web UI

The `web` service reaches the API in-network via `API_URL=http://api:8080` and shares the same `OPENMIND_TOKEN` as the api. Log in at `http://localhost:3000/login` with the token; the web app stores it in an httpOnly cookie and injects the bearer header server-side, so the token is never exposed to the browser.

When `OPENMIND_TOKEN` is set, the login page validates the token against the API before accepting it (wrong token → `401`). With no token set, any value is accepted (single-user localhost mode).

### Exposing to a network

Both `api` and `web` bind to `127.0.0.1` only by default. **Map your public domain / reverse proxy to the `web` service (port 3000) only** — the browser never talks to the API directly, and the API does not need to be publicly reachable. Terminate TLS at your proxy (the login cookie is flagged `Secure` in production, so the UI must be served over HTTPS). Always set a strong `OPENMIND_TOKEN` before exposing anything.

## Configuration

All configuration is via environment variables (see `.env.example`):

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://openmind:openmind@localhost:5433/openmind` | Postgres connection string. Inside compose the API uses the `db` service host automatically. |
| `TEST_DATABASE_URL` | `postgres://openmind:openmind@localhost:5433/openmind_test` | Connection string used by the Go test suite only. |
| `AI_PROVIDER` | `noop` | Enrichment provider: `noop` or `gemini`. **`noop` is the default** — the app is fully functional (title extraction + FTS search) with no AI key. |
| `GEMINI_API_KEY` | _(empty)_ | Required only when `AI_PROVIDER=gemini`. Enables AI summaries, tags, and semantic (vector) search. |
| `PORT` | `8080` | HTTP listen port. |
| `OPENMIND_TOKEN` | _(empty)_ | Bearer token guarding the API. Empty = unauthenticated (fine for single-user localhost). Set a strong secret before exposing the API on a network. |

### AI is optional

With `AI_PROVIDER=noop` (default), saves are extracted and made searchable via Postgres FTS — no external calls, no API key. Set `AI_PROVIDER=gemini` and provide `GEMINI_API_KEY` to add AI summaries, auto-tags, and vector search:

```bash
AI_PROVIDER=gemini GEMINI_API_KEY=<your-key> docker compose up -d
```

Only budget model tiers are used in the enrichment pipeline; a flagship model is never wired in.

## Exporting your library

A link to a full JSON export is on the web UI's home page (top-level nav). It calls `GET /api/export` (bearer-token or logged-in-cookie authenticated, scoped to your account) and returns every saved item as a JSON array, including each item's extracted text (`body`), title, tags, and metadata — so you always have a portable, lock-in-free copy of everything you've saved.

## Browser extension

The WXT + React browser extension (`apps/extension`) is a thin capture client — it saves the active tab's URL, a selection as a note, or an image, and talks to your instance over the same bearer-token auth as the web UI. Enrichment stays server-side.

Build:

```bash
pnpm --filter extension build           # → apps/extension/.output/chrome-mv3
pnpm --filter extension build:firefox   # → apps/extension/.output/firefox-mv2
```

Load unpacked:

- **Chrome / Edge / Brave**: open `chrome://extensions`, enable **Developer mode**, click **Load unpacked**, select `apps/extension/.output/chrome-mv3`.
- **Firefox**: open `about:debugging#/runtime/this-firefox`, click **Load Temporary Add-on…**, select any file inside `apps/extension/.output/firefox-mv2` (e.g. `manifest.json`). Temporary add-ons are removed on browser restart.

Options page: open the extension's **Settings** (Chrome: right-click the toolbar icon → *Options*), set the **Instance URL** (e.g. `http://localhost:3000` for local testing) and paste the same value you set for `OPENMIND_TOKEN` server-side as the **access token**, then **Validate** and **Save settings**.

For the full manual verification checklist (popup save, context-menu save-selection/save-image, error states), see `apps/extension/README.md` — not duplicated here.

> This is the Milestone 0 quickstart. Expanded operational docs (backups, upgrades, reverse proxy, auth) land in Milestone 1.
