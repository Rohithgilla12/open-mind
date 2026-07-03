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
```

## Configuration

All configuration is via environment variables (see `.env.example`):

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://openmind:openmind@localhost:5433/openmind` | Postgres connection string. Inside compose the API uses the `db` service host automatically. |
| `TEST_DATABASE_URL` | `postgres://openmind:openmind@localhost:5433/openmind_test` | Connection string used by the Go test suite only. |
| `AI_PROVIDER` | `noop` | Enrichment provider: `noop` or `gemini`. **`noop` is the default** — the app is fully functional (title extraction + FTS search) with no AI key. |
| `GEMINI_API_KEY` | _(empty)_ | Required only when `AI_PROVIDER=gemini`. Enables AI summaries, tags, and semantic (vector) search. |
| `PORT` | `8080` | HTTP listen port. |

### AI is optional

With `AI_PROVIDER=noop` (default), saves are extracted and made searchable via Postgres FTS — no external calls, no API key. Set `AI_PROVIDER=gemini` and provide `GEMINI_API_KEY` to add AI summaries, auto-tags, and vector search:

```bash
AI_PROVIDER=gemini GEMINI_API_KEY=<your-key> docker compose up -d
```

Only budget model tiers are used in the enrichment pipeline; a flagship model is never wired in.

> This is the Milestone 0 quickstart. Expanded operational docs (backups, upgrades, reverse proxy, auth) land in Milestone 1.
