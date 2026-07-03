# Milestone 0 Spike — Design

Date: 2026-07-03 · Status: Approved design, pre-implementation

## Goal

Complete Milestone 0 of Openmind: prove the entire risky path — save → queue → extract → AI enrich → embed → hybrid search — in one Go binary, inside the real monorepo skeleton, and settle the extraction library choice with evidence.

**Decisions locked in this design:**

- Spike proves the full slice with a real AI provider (Gemini), not plumbing-only.
- Scaffold is the real repo skeleton per CLAUDE.md; the spike lives in `apps/api` and nothing is thrown away.
- Web stack: **Next.js** (closes the Next.js vs TanStack Start open question, PRD §14).
- AI providers for the spike: **Gemini + noop** only. Fallback-chain mechanics are Milestone 1.
- Sequencing: scaffold → spike → bake-off.

## Phase 1 — Monorepo scaffold

Real skeleton, minimal contents:

- **Root**: `Taskfile.yml` (`dev`, `generate`, `test`, `lint`, `migrate`; `sources:`/`generates:` checksums on codegen), `docker-compose.yml` (Postgres 17 with pgvector + one `openmind` service built from `apps/api`), `go.work`, `pnpm-workspace.yaml`, `turbo.json`, `.env.example`, `git init` + initial commit.
- **`openapi.yaml` v0 stub**: `POST /items`, `GET /items`, `GET /search` only — just enough surface for the spike. All API types codegen'd (oapi-codegen for Go, TS client into `packages/api-client`); never hand-written.
- **`apps/api`**: Go module. `cmd/openmind` single binary with `serve | work | all | migrate` subcommands. Packages: `internal/api` (chi, generated server interface), `internal/store` (sqlc + pgx), `internal/jobs` (River), `internal/enrich`, `internal/ai`, `internal/search`.
- **Initial migration**: `users`, `items` (id, user_id, url, title, body, summary, tags, card_type, status, timestamps), `item_embeddings` (item_id, user_id, `vector` column). Every table has `user_id`; every store method takes `ctx` and is user-scoped.
- **Auth for the spike**: auto-provisioned single dev user injected by middleware — multi-tenant schema from day one, no real auth yet.
- **`apps/web`**: bare Next.js stub consuming `packages/api-client`. No real UI this milestone.
- **`packages/api-client`**: generated only. **`packages/ui`**: design tokens from `docs/design/README.md` only.
- **`apps/extension`, `apps/mobile`, `apps/dock`**: placeholder dir + README each.

## Phase 2 — Pipeline spike

**Save path (capture is sacred):** `POST /items {url}` → insert row with `status=pending` → enqueue River `EnrichItem` job (payload: item ID only) → `201` in <100 ms. No AI or network fetch in the request path.

**Enrichment job** — staged, each stage persisted and idempotent (re-running a stage on already-enriched state yields identical results):

1. **Extract** — go-trafilatura as day-one default (interface-backed; swapped after the bake-off). Fetch URL; store title, body text, lead image URL.
2. **Classify** — heuristic only for the spike (URL patterns / og:type → article, video, tweet, image). No AI call.
3. **Summarise + tag** — one Gemini Flash-Lite call via the `ai.Provider` adapter interface: `Summarise`, `Tag`, `Embed`, `ParseQuery` (interface complete from day one; spike implements/uses the first three). Provider selected by `AI_PROVIDER` env var; `noop` implemented alongside and keeps the app fully functional. 429/errors → River retry; a failed job never blocks or corrupts the save.
4. **Embed** — `gemini-embedding-001` → pgvector; item flips to `status=enriched`.

**Search:** `GET /search?q=` runs Postgres FTS and pgvector cosine similarity, fused with naive RRF. Deliberately crude — the point is both indexes answering one query. Under `noop`, FTS-only search still works.

**Definition of done:**

1. Save a real URL; watch it enrich; find it via a term that appears only in the AI-generated summary.
2. Re-run the enrichment job on the same item → identical final state (idempotency test).
3. Repeat the save with `AI_PROVIDER=noop` → item still saved and findable via FTS.

**Tests:** table-driven Go tests per stage using a fake provider; store tests against the compose Postgres (not mocks); the idempotency test above.

## Phase 3 — Extraction bake-off

- **Harness:** `apps/api/cmd/bakeoff` CLI reusing the spike's extractor interface. Candidates: go-trafilatura, go-readability, Jina Reader (HTTP; API key optional).
- **Corpus:** ~18 URLs in `apps/api/testdata/bakeoff.json` spanning card types: long-form articles, recipes, product pages, tweets/X, YouTube, image-heavy posts, one paywalled page, one JS-heavy SPA.
- **Scoring:** per URL × extractor — title correctness, body completeness/noise, lead image found, latency, hard failure. 0–2 per dimension, manual review (first pass by Claude, sanity-checked by Rohith). Results written to `docs/bakeoff-results.md`.
- **Outcome:** winner becomes the default extractor; Jina Reader remains an optional config-gated fallback on extraction failure.

## Wrap-up

- Update `TODO.md`: Milestone 0 → Done; promote Milestone 1 items to Now.
- Out of scope for this design (stay as separate TODO items): masonry grid perf spike, Karakeep deep-dive, naming/domain decision, AI fallback chain, real auth, any Milestone 2+ feature.

## Execution model

Orchestrated implementation: scaffold and mechanical stages delegated to Sonnet subagents; Go pipeline core, search fusion, and all review done by the orchestrator (Fable) or Opus subagents.
