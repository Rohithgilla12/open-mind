# TODO

> Solo tracker. Now = this week's evenings. Claude Code: read this at session start, update at session end. Graduates to GitHub Issues at OSS launch.

## Milestone 1 — "Save & find"

### Now
- [ ] Verify Gemini provider live (needs `GEMINI_API_KEY`) — during first deploy: DoD run (real article → status enriched, summary + tags populated, `/search?q=<summary-only word>` returns it)
- [ ] openapi.yaml v0: auth, items CRUD, search; oapi-codegen + TS client generation wired into `task generate`
- [ ] Schema + migrations: items, item_embeddings, tags, assets (user_id everywhere)
- [ ] AI adapter: OpenAI-compatible client + fallback chain + noop provider

### Next
- [ ] River queue: enrichment job, priority lanes, idempotency test (run twice, same result)
- [ ] Hybrid search: FTS + pgvector + rank fusion
- [ ] Capture: web quick-add (URL / note / image)
- [ ] Grid + card detail view (type-aware renderers)
- [ ] WXT extension: save page / selection / image
- [ ] JSON export
- [ ] SSRF hardening for extractor fetches (private-IP dialer guard, redirect re-check) + basic auth/rate limiting before public exposure
- [ ] Self-host quickstart: expand docs/self-hosting.md (Milestone 1 polish)
- [ ] Virtualised masonry grid spike: 500 mixed cards at 60fps (port docs/design/openmind-mockup.html)
- [ ] Karakeep repo deep-dive: what to learn, what to avoid (notes → docs/research.md)
- [ ] Decide: name + domain check

## Done

### Milestone 0 — Week-0 spike (committed, not killed)
- [x] Scaffold monorepo: Taskfile, go.work, pnpm workspaces, turbo.json, docker-compose (postgres + pgvector)
- [x] Pipeline end-to-end in one binary: URL → extract → summary/tags → pgvector embed → hybrid search returns it (verified e2e with noop provider 2026-07-03)
- [x] Extraction bake-off on 18 real URLs (docs/bakeoff-results.md): **winner = go-trafilatura** (best body extraction, acceptable latency); kept as default. Jina Reader kept as a config-gated fallback for later. go-readability close second.
- [x] Decide: Next.js vs TanStack Start → **Next.js locked in** (App Router, React 19)
- [x] E2e verification + wrap-up: `docker compose build` + noop parity run green; `task test` + `task lint` green (real-Gemini leg deferred to first deploy)

### Pre-Milestone 0
- [x] PRD v1 (docs/PRD.md)
- [x] Grid mockup (docs/design/openmind-mockup.html)
- [x] CLAUDE.md
