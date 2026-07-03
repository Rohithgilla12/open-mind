# TODO

> Solo tracker. Now = this week's evenings. Claude Code: read this at session start, update at session end. Graduates to GitHub Issues at OSS launch.

## Milestone 1 — "Save & find"

### Now

- [ ] openapi.yaml v0: auth, items CRUD, search; oapi-codegen + TS client generation wired into `task generate`
- [ ] Schema + migrations: items, item_embeddings, tags, assets (user_id everywhere)
- [ ] AI adapter: OpenAI-compatible client + fallback chain + noop provider

### Next
- [ ] Karakeep repo deep-dive: what to learn, what to avoid (notes → docs/research.md)
- [ ] Decide: name + domain check

## Done

### Milestone 0 / M1 — masonry grid perf spike
- [x] Masonry grid perf spike (docs/research.md, 2026-07-03): CSS-columns masonry holds 60fps easily — median 120fps (120Hz-capped), 0 dropped frames, 8.3ms avg frame at both 500 and 1000 cards. Verdict: **keep CSS columns for M1, do not virtualise**; add `content-visibility: auto` as the cheap first lever only if a future unbounded view shows first-paint/memory pressure.

### Milestone 1 — card detail + export
- [x] Card detail + reader view (type-aware renderers) — e2e verified 2026-07-03: `GET /item/<id>` with logged-in cookie returns 200 HTML containing the item's title.
- [x] JSON export — e2e verified 2026-07-03: `GET /api/export` with Bearer token returns a JSON array with non-empty `body` per item; item removed from the array after `DELETE /api/items/<id>` (204).

### Milestone 1 — extension
- [x] WXT extension: save page / selection / image, options page (instance URL + token, validate/save), e2e verified 2026-07-03: browser extension capture (url + note) via Bearer token auth against a fresh local build (`pnpm --filter extension build`); server-side confirmed with `GET /api/auth/check` (200 correct token / 401 wrong token) and `POST /api/items` for both `{url}` and `{note}` payloads (201).

### Milestone 1 — expose-ready web
- [x] SSRF hardening for extractor fetches (private-IP dialer guard, redirect re-check) + bearer auth + per-IP rate limiting before public exposure
- [x] Web container: `output: standalone` + monorepo Dockerfile; compose `web` service on `127.0.0.1:3000` (`API_URL=http://api:8080`, shared `OPENMIND_TOKEN`, bearer-cookie login validated against the API). docs/self-hosting.md expanded (web service, token setup, map domain to web:3000 only). Verified e2e 2026-07-03: login 200 / wrong-token 401, add URL + note, home + `?q=` search render enriched cards.

### Milestone 0 — Week-0 spike (committed, not killed)
- [x] Gemini provider verified live on deploy box 2026-07-03: real article → enriched with summary + 5 tags; /search?q=entrepreneurship (AI-tag-only term) returns it
- [x] Scaffold monorepo: Taskfile, go.work, pnpm workspaces, turbo.json, docker-compose (postgres + pgvector)
- [x] Pipeline end-to-end in one binary: URL → extract → summary/tags → pgvector embed → hybrid search returns it (verified e2e with noop provider 2026-07-03)
- [x] Extraction bake-off on 18 real URLs (docs/bakeoff-results.md): **winner = go-trafilatura** (best body extraction, acceptable latency); kept as default. Jina Reader kept as a config-gated fallback for later. go-readability close second.
- [x] Decide: Next.js vs TanStack Start → **Next.js locked in** (App Router, React 19)
- [x] E2e verification + wrap-up: `docker compose build` + noop parity run green; `task test` + `task lint` green (real-Gemini leg deferred to first deploy)

### Pre-Milestone 0
- [x] PRD v1 (docs/PRD.md)
- [x] Grid mockup (docs/design/openmind-mockup.html)
- [x] CLAUDE.md
