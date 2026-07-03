# TODO

> Solo tracker. Now = this week's evenings. Claude Code: read this at session start, update at session end. Graduates to GitHub Issues at OSS launch.

## Milestone 0 — Week-0 spike (gate: commit or kill)

### Now
- [ ] Scaffold monorepo: Taskfile, go.work, pnpm workspaces, turbo.json, docker-compose (postgres + pgvector)
- [ ] Spike pipeline end-to-end in one binary: URL → go-readability extract → cheap-model summary/tags (Cerebras or Gemini Flash-Lite) → pgvector embed → hybrid search returns it
- [ ] Extraction bake-off on 20 real URLs from my saves: go-readability vs go-trafilatura vs Jina Reader — pick primary on data

### Next
- [ ] Virtualised masonry grid spike: 500 mixed cards at 60fps (port docs/design/openmind-mockup.html)
- [ ] Karakeep repo deep-dive: what to learn, what to avoid (notes → docs/research.md)
- [ ] Decide: Next.js vs TanStack Start
- [ ] Decide: name + domain check

## Milestone 1 — "Save & find"

### Later
- [ ] openapi.yaml v0: auth, items CRUD, search; oapi-codegen + TS client generation wired into `task generate`
- [ ] Schema + migrations: items, item_embeddings, tags, assets (user_id everywhere)
- [ ] River queue: enrichment job, priority lanes, idempotency test (run twice, same result)
- [ ] AI adapter: OpenAI-compatible client + fallback chain + noop provider
- [ ] Capture: web quick-add (URL / note / image)
- [ ] WXT extension: save page / selection / image
- [ ] Grid + card detail view (type-aware renderers)
- [ ] Hybrid search: FTS + pgvector + rank fusion
- [ ] JSON export
- [ ] Self-host quickstart: docker compose up + docs/self-hosting.md

## Done
- [x] PRD v1 (docs/PRD.md)
- [x] Grid mockup (docs/design/openmind-mockup.html)
- [x] CLAUDE.md
