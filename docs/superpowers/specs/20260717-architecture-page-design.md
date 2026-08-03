# Architecture & tech-stack page (`/architecture`) — design

Date: 2026-07-17
Status: approved

## Problem

The project's architecture and stack are documented across `docs/PRD.md`,
`CLAUDE.md`, and ~40 specs under `docs/superpowers/`, but there's nothing a
visitor or prospective contributor can read *in the website*. With OSS launch
approaching, we want a single, public, extensive page that explains how
Openmind is built and why — a contributor-facing deep dive that doubles as a
launch showcase.

## Goals

- One public page in `apps/web` at `/architecture`, styled with the warm design
  tokens, matching the existing `/privacy`, `/terms`, `/welcome` pattern.
- Deep contributor-level content: monorepo layout, the OpenAPI contract
  workflow, the sacred save path, the enrichment pipeline, the pluggable AI
  chain, hybrid search, data/tenancy, the thin clients, and self-hosting.
- Easy to keep updated as the architecture evolves.

## Non-goals

- No blog engine, CMS, or multi-post structure — this is a single page.
- No new runtime dependency (no MDX/markdown renderer) just for one page.
- Not auth-gated; it is public by design.

## Approach

**Placement.** New route `apps/web/app/architecture/page.tsx`. Public: add
`/architecture` to `isPublicRoute` in `apps/web/middleware.ts` (clerk mode) and
to the legacy-mode public-prefix check. Linked from the `/welcome` footer
(alongside Privacy · Terms) and from the repo `README.md`.

**Authoring — hand-built structured TSX, no new deps.** Every existing public
page is plain TSX with inline styles plus the `.serif`/`.meta` helper classes
and `@openmind/ui` tokens. There is no markdown renderer in the web app, and
the project's dependency discipline argues against adding one for a single
page. To stay maintainable, page content lives as **structured data** (typed
arrays for the stack table, the pipeline stages, the principles, the client
apps) declared at the top of the file and rendered by small local components.
Updating the page means editing readable data, not hunting through JSX.

**Diagrams — CSS/flexbox box-and-arrow, no Mermaid.** Theme-consistent with the
palette. Three diagrams:
1. Capture → enrichment flow (save returns instantly; River job runs stages async).
2. The contract workflow (`openapi.yaml` → `task generate` → Go server + TS client).
3. Search fusion (FTS + pgvector → rank fusion → optional rerank).

**"Keep it updated".** A visible `Last updated` date line at the top (mono
`.meta`, like `/privacy`), plus one line added to `CLAUDE.md`'s *Session
workflow* so future sessions refresh this page when the architecture changes.
This makes upkeep a durable convention rather than a one-off promise.

## Content outline (in order)

1. **Intro** — what Openmind is; the six non-negotiable principles as the spine.
2. **The shape** — monorepo layout (`apps/api`, `web`, `extension`, `mobile`,
   `dock`, `packages/*`); one Go binary (`cmd/openmind: serve|work|all|migrate`).
3. **The contract** — `openapi.yaml` as source of truth → `task generate`
   (oapi-codegen Go server + TS client, sqlc); never hand-edit generated code.
4. **Capture is sacred** — the instant save path; enrichment is always async.
5. **The enrichment pipeline** — extract → classify → summarise → embed; River
   jobs, priority lanes, idempotent + retryable. Real stages: readability /
   trafilatura / domdistiller extraction, PDF (go-pdfium + wazero WASM), colour
   palette, image URL, social video. Jobs: enrich, digest, kindle, places,
   fetch-lead-image, poll.
6. **Pluggable AI** — adapter interface (`Summarise`, `Tag`, `Embed`,
   `ParseQuery`); ordered fallback chain; 429 → failover; cheap-models-only;
   `noop` keeps the app whole (manual tags, FTS-only search). Providers:
   Gemini, OpenAI-compatible, `noop`, `fake` (tests).
7. **Hybrid search** — Postgres FTS + pgvector fusion, rank fusion, colour
   search, optional rerank.
8. **Data & tenancy** — sqlc + pgx, every table `user_id`-scoped, migrations.
9. **The clients** — web (Next.js 15 / React 19), extension (WXT), mobile
   (Expo, share-sheet-first), dock (Tauri); all thin — capture + display, no
   business logic.
10. **Self-hosting** — `docker compose up` = Postgres + one binary; no required
    Redis or sidecars; AI optional behind config.
11. **Tech-stack table** — layer → choice → why.

## Accurate stack facts (verified 2026-07-17)

- API: Go 1.25; chi/v5 router; River v0.40 jobs (riverpgxv5); pgx/v5;
  pgvector-go; sqlc; oapi-codegen; modelcontextprotocol/go-sdk (MCP);
  go-readability + go-trafilatura + go-domdistiller (extraction); klippa
  go-pdfium + tetratelabs/wazero (PDF via WASM); google.golang.org/genai
  (Gemini). AI providers under `internal/ai`: gemini, openai, noop, fake.
- Web: Next 15, React 19, Clerk (optional; token mode has zero Clerk runtime),
  maplibre-gl, `@openmind/api-client` (generated), `@openmind/ui` (tokens).
- Tasks: Taskfile (`dev`, `generate`, `generate:api|sqlc|ts`, `test`, `lint`,
  `migrate`).

## Testing

- Vitest render smoke test asserting the page renders its main headings.
- `pnpm --filter web lint` (tsc `--noEmit`) + `pnpm --filter web test` pass.

## Files touched

- `apps/web/app/architecture/page.tsx` (new)
- `apps/web/app/architecture/page.test.tsx` (new)
- `apps/web/middleware.ts` (exempt `/architecture` in both modes)
- `apps/web/app/welcome/page.tsx` (footer link)
- `README.md` (link)
- `CLAUDE.md` (keep-updated convention line)
