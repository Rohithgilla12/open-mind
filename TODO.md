# TODO

> Solo tracker. Now = this week's evenings. Claude Code: read this at session start, update at session end. Graduates to GitHub Issues at OSS launch.

## Milestone 2 — "It feels magic"

> Opened 2026-07-04. M1 "Now" was empty; remaining M1 "Next" items are either user decisions (name/domain), deferred (multi-user auth — parked until laptop access), or triage candidates now folded into this backlog. M2 scope per PRD §10: colour search, NL query parsing, Lenses, reader mode, imports.

### Now
- [ ] NL query parsing — wire provider `ParseQuery` into `/search` so natural-language queries ("blue book about bread") split into text + colour + filters

### Next
- [ ] Lenses (saved query/rule collections) — schema + API + web
- [ ] Reader mode polish (M1 shipped a type-aware detail view; M2 = distraction-free reading)
- [ ] Imports: Pocket/Omnivore, RSS feeds, PDF capture (from Karakeep research, docs/research.md)
- [ ] Web UI for colour search — search overlay with colour swatches + understood-as chips (design already specced, deferred in M1 design pass)

### Later
- [ ] Lossless AVIF metadata stripping / re-allow AVIF uploads (M1 carry-over — AVIF currently 415s at upload pending a metadata-strip implementation)

## Milestone 1 — "Save & find" (deferred tail)

- [ ] Decide: name + domain check (user decision)
- [ ] Real auth (multi-user) — replaces OPENMIND_TOKEN single-user mode. **Deferred to laptop session (2026-07-04)**; schema is already multi-tenant, so this is login/accounts work, not a data-model change.

## Done

### Milestone 2 — colour search
- [x] Colour search backend (2026-07-04) — `/search` gains an optional `color` param (hex `#RRGGBB`/shorthand or named colour incl. Openmind accents cobalt/terracotta/gold/green); `q` now optional, ≥1 of q/color required. New `search/color.go`: hex+name parse → sRGB→CIELAB, ΔE*76 nearest-palette-colour ranking over the stored `palette text[]`, fused into the existing RRF alongside FTS+vector so text+colour queries combine. New tenant-scoped `ListItemsWithPalette` query. Contract regenerated (Go + TS client). Unit tests (parse/ΔE ordering/ranking) green; DB-backed tests (`TestColorSearch*`) written — run in CI/laptop (no Docker daemon in web session). Web UI + NL colour parsing still to come.

### Milestone 1 — web design pass
- [x] Warm-editorial reskin of the web app (`docs/superpowers/plans/20260704-design-pass.md`) — tokens/fonts/shell (Task 1), type-aware cards + image fallback (Task 2), topbar/filter strip/capture/search (Task 3), palette dots server-extraction + render (Task 4), **card detail reader restyle (Task 5)**, visual verification (Task 6). Detail page (`/item/[id]`) rebuilt to the reader look: bright reader panel on canvas, mono meta line, large Newsreader title, serif summary lead, cobalt "Open original ↗", right-hand rail (palette swatches + tags + "archived locally"), type-aware bodies (article/product/book/recipe/video/tweet hero + summary + body, quote = gold-glyph italic serif, note = serif body, image = large image). Verified 2026-07-04: `pnpm --filter web build`+`lint` green; drove the built page with headless Chromium across article / broken-image / quote / note / image fixtures — all 200, **zero `<img>` tags** (images painted as background-over-gradient, so a 404 hero reveals the type gradient, never a broken-image glyph), reader shell + palette rail present. Remaining M2 design screens (out of scope): Ledger view, Drift, Desk, functional Lenses, search overlay with understood-as chips + colour swatches, ⌘K quick-capture palette.

### Milestone 1 — image upload
- [x] Strip EXIF/GPS/XMP/IPTC metadata from uploaded images (privacy) — e2e verified 2026-07-04: crafted a JPEG with an injected APP1 `Exif\0\0`+fake GPS/TIFF segment, `POST /api/assets` → 201, `GET /api/assets/<id>` → downloaded bytes contain zero `Exif\0\0` occurrences, file still starts `FFD8`/ends `FFD9`, and `jpeg.Decode` succeeds (bounds unchanged, 2x2). AVIF removed from the upload allowlist (fake `ftyp`-branded `avif` file → `POST /api/assets` → `415 unsupported image type`) pending lossless AVIF metadata stripping. Docs updated (`docs/self-hosting.md`, image-upload spec).
- [x] Assets table + image upload capture — e2e verified 2026-07-04: `POST /api/assets` (multipart, cookie auth) → 201 image card with `leadImageUrl=/assets/<id>`; `GET /api/assets/<id>` → 200 `image/png` bytes with `X-Content-Type-Options: nosniff`; unauth `GET` → 401; item enriched within ~8s (`GET /items` status `enriched`). Fixed a blocking bug found during e2e: distroless `nonroot` API image couldn't write to a freshly-initialised `assetsdata` named volume (root:root ownership) — `apps/api/Dockerfile` now pre-creates `/data/assets` chowned to uid/gid 65532 so Docker seeds the volume with writable ownership. Oversize (`413`) covered by existing unit tests, not re-verified manually.

### Milestone 1 — AI adapter
- [x] AI adapter: OpenAI-compatible client + ordered fallback chain (gemini → openai-compatible → noop), per-provider rate-limit config, 429 = fall over not fail — e2e verified 2026-07-03: openai (unreachable `.invalid` base URL) → noop fallover, item enriched with empty summary, chain log confirmed (`ai chain: provider error, failing over provider=openai op=summarise`).

### Research
- [x] Karakeep repo deep-dive (docs/research.md, 2026-07-03): multi-container TS stack (app + Meilisearch + Chrome + workers) vs our single binary; no vector search in core; AGPL — interop fine, never copy code. Learn from: capture breadth, importers, feeds. Avoid: headless-Chrome archival weight, unvirtualised unbounded views.
- [x] openapi.yaml contract + codegen and items/embeddings schema — delivered across Milestone 0 and later slices (superseded the original Now items)

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
