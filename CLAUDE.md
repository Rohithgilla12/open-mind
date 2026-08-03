# CLAUDE.md

Openmind (working title) — open-source, self-hostable "commonplace book": save anything (links, notes, images, quotes), AI enriches and organises it, find it by fragments (colour, keyword, vibe). Full spec in `docs/PRD.md`; design references and mockups in `docs/design/`.

## Session workflow

- **`TODO.md` at the repo root is the task tracker.** Start each session by reading it; when finishing work, update it (move items to Done, add discovered follow-ups under Next/Later). Keep it structured as Now / Next / Later / Done per milestone — prune Done periodically.
- Do not start Milestone 2+ items while Milestone 1 has open Now/Next tasks.
- At OSS launch, tasks graduate to GitHub Issues; TODO.md then tracks only the maintainer's private planning.
- **Keep the public `/architecture` page current.** When the architecture changes (a new pipeline stage, a new client, a swapped core dependency), update the structured data in `apps/web/app/architecture/page.tsx` and bump its `LAST_UPDATED` date.

## Non-negotiable principles

1. **Capture is sacred.** Save paths must return instantly; enrichment is always async. Never make a save wait on an AI call.
2. **Pipeline is idempotent and retryable.** Enrichment failures never block or corrupt a save. Every job is safe to re-run.
3. **Single-binary self-hosting.** `docker compose up` (Postgres + one Go binary) is the whole deployment. Never add a required service (no Redis, no Python sidecars). Optional integrations go behind config.
4. **Multi-tenant from day one.** Every table has `user_id`; every query is scoped in the store layer. Self-hosted single-user mode is just an auto-provisioned account.
5. **AI is pluggable, never assumed.** All AI goes through the adapter interface (`Summarise`, `Tag`, `Embed`, `ParseQuery`). The `noop` provider must always keep the app fully functional (manual tags, FTS-only search).
6. **Cheap models only in the pipeline.** Default to budget tiers (Gemini Flash-Lite, DeepSeek, Cerebras/Groq free, Ollama). Never wire a flagship model into enrichment.

## Repo layout

```
apps/api/        Go — API + River workers, one binary (cmd/openmind: serve|work|all|migrate)
  internal/api/      HTTP handlers, middleware, auth
  internal/enrich/   pipeline stages: extract → classify → summarise → embed
  internal/ai/       provider adapters + ordered fallback chain
  internal/search/   hybrid: Postgres FTS + pgvector, rank fusion, optional rerank
  internal/store/    sqlc queries, migrations
  internal/jobs/     River job definitions, priority lanes
apps/web/        Next.js
apps/extension/  WXT (React)
apps/mobile/     Expo — share-sheet-first capture
apps/dock/       Tauri floating dock (P2)
packages/api-client/  TS client GENERATED from openapi.yaml — never edit by hand
packages/ui/          shared React components + design tokens
openapi.yaml     THE contract — single source of truth
```

## The contract workflow (most important convention)

`openapi.yaml` is the spine. To change or add an endpoint:
1. Edit `openapi.yaml`
2. Regenerate: `make generate` (runs `oapi-codegen` for Go + TS client into `packages/api-client`)
3. Implement the Go handler; update consumers against the regenerated client
Never hand-write API types in TS. Never add a Go route that isn't in the spec.

## Commands (Taskfile — https://taskfile.dev, not Make)

```
task dev          # postgres (docker) + Go api live reload + pnpm dev via turbo
task generate     # openapi codegen (Go server + TS client) + sqlc — uses sources/generates checksums, skips if unchanged
task test         # go test ./... + turbo run test
task lint         # golangci-lint + turbo run lint
task migrate      # apply migrations locally
task --list       # discover everything else
docker compose up # full self-host simulation
```

Task definitions live in `Taskfile.yml` at the root. Use `deps:` for parallelism and `sources:`/`generates:` on codegen tasks so they no-op when inputs are unchanged. JS-only tasks: `pnpm turbo run <task> --filter=<app>`. Go work happens with plain `go` from `apps/api`.

## Go conventions

- Standard library first; justify every new dependency. Current approved core: chi (router), River (jobs), sqlc + pgx, oapi-codegen, modelcontextprotocol/go-sdk (MCP server).
- Errors: wrap with `fmt.Errorf("doing x: %w", err)`; typed sentinel errors in `internal/` packages where callers branch on them.
- All queries through sqlc in `internal/store` — no inline SQL in handlers or jobs.
- Every store method takes `ctx` and is scoped by `user_id`. A query without a `user_id` predicate is a bug unless explicitly justified (migrations, admin).
- AI provider calls: always through the adapter chain; respect per-provider rate-limit config; treat 429 as fallover-to-next, not failure.
- Jobs: define in `internal/jobs`, keep payloads minimal (IDs, not blobs), fetch fresh state inside the job.

## TypeScript conventions

- pnpm workspaces + Turborepo. Strict TS everywhere.
- Data fetching only through `packages/api-client`.
- Design tokens live in `packages/ui` — warm palette: canvas `#E4DDCD`, paper `#F4F0E6`, panel `#EBE5D7`, card surface `#FCFBF6`, ink `#1C1A16`, ink-muted `#57534A`, ink-faint `#A39C8B`, hairline `rgba(28,26,22,.11)`; accents cobalt `#1B3FD1`, terracotta `#C24A2E`, gold `#E0B23A`, green `#2E7D5B`; note surface `#FBF4D8`; Instrument Sans (UI/body), JetBrains Mono (metadata), Newsreader italic for titles/quotes. Canonical palette lives in `docs/design/README.md` + `packages/ui`. Don't hardcode colours in apps.
- Extension (WXT) and mobile (Expo) are thin clients: capture + display, no business logic. Enrichment logic lives server-side only.

## Product vocabulary (use these names, never mymind's)

- **Lens** — saved query/rule collection (not "Smart Space")
- **Drift** — resurfacing mode (not "Serendipity")
- **Desk** — pinned board (not "Top of Mind")
- Card types: article, product, book, recipe, video, tweet, image, note, quote, repo

## Testing expectations

- Go: table-driven tests; pipeline stages tested with the `noop`/fake AI provider; store tests run against real Postgres (dockertest or the compose db), not mocks.
- Every enrichment job needs an idempotency test (run twice, same result).
- Search: golden tests over a small fixture corpus for FTS + vector fusion.

## Do not

- Do not add required infrastructure beyond Postgres.
- Do not put AI calls in the request path of a save.
- Do not hand-edit generated code (`packages/api-client`, sqlc output, oapi-codegen output).
- Do not use mymind's feature names, taglines, or copy their UI verbatim.
- Do not wire secrets into code or commit `.env`; config via env vars documented in `docs/self-hosting.md`.
- Do not start Milestone 2+ features (Lenses, Drift, reranking, Kindle, dock) while Milestone 1 is incomplete — check `docs/PRD.md` §10 and `TODO.md`.
