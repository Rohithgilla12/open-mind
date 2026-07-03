# Self-hosting Openmind

Openmind self-hosts as **Postgres + one Go binary**. No Redis, no Python sidecars — `docker compose up` is the whole deployment.

## Quickstart

```bash
git clone <repo> && cd open-mind
cp .env.example .env      # optional: defaults work out of the box
docker compose up -d      # starts Postgres (pgvector) + the API/worker binary
```

The API listens on `http://localhost:8080`. Migrations run automatically on start.

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

> This is the Milestone 0 quickstart. Expanded operational docs (backups, upgrades, reverse proxy, auth) land in Milestone 1.
