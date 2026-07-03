# Research

# Karakeep deep-dive

**Date:** 2026-07-03
**Author:** research pass for Openmind
**Subject:** Karakeep (formerly Hoarder) — github.com/karakeep-app/karakeep — closest OSS competitor to Openmind.

Legend: **[V]** = verified against a cited source; **[I]** = inference / analysis by the researcher.

---

## 1. Architecture

### Stack [V]
- **Language/runtime:** TypeScript throughout, Node.js (version pinned via `.nvmrc`).
- **Web app:** Next.js (app router) — serves the web UI, a tRPC server, and a Hono-based REST API wrapper (`@karakeep/api`).
- **Client↔server:** tRPC over HTTP with `superjson` serialisation.
- **Monorepo:** pnpm + Turbo. Apps: `apps/web`, `apps/workers`, `apps/mobile` (React Native), `apps/browser-extension`. Shared packages: `@karakeep/db`, `@karakeep/shared` (config, logger, inference client), `@karakeep/shared-server` (queue defs), `@karakeep/trpc` (business logic), `@karakeep/api`, `@karakeep/shared-react`.
- **ORM/migrations:** Drizzle ORM (`@karakeep/db`).
- **Auth:** NextAuth.
- **Browser automation:** Puppeteer / headless Chrome (Playwright referenced in dev). Monolith for full-page archival; yt-dlp for video archiving.

### Database [V, with a caveat]
- Karakeep's application state is **SQLite** via `better-sqlite3`, stored at `DATA_DIR`, with optional WAL mode (`DB_WAL_MODE`) — this is confirmed by the DeepWiki architecture writeup. One general web summary described a "PostgreSQL/database backend" for self-hosting, but that appears to be a model conflation; **the authoritative architecture source says SQLite** [V for SQLite; the Postgres mention is treated as unreliable]. This is the single most important architectural contrast with Openmind (Postgres 16 + pgvector).

### Services required for self-hosting [V]
Karakeep self-hosting is **multi-container** — three services minimum:
1. **Main web app** (Next.js, port 3000) — UI + API + tRPC.
2. **Headless Chrome / Puppeteer** (port 9222 via `BROWSER_WEB_URL`) — screenshots, PDF/full-page archival. This is the heaviest component (see §3).
3. **Meilisearch** (port 7700 via `MEILI_ADDR`) — full-text search index.

Plus a **workers** process (`apps/workers`) for background jobs (may run in the same or a separate container). The Docker dev stack also includes a "prep" service for dependency install and migrations. So a realistic production deployment is **3–4 containers + persistent volumes** for app data and the Meili index (bind mounts / named volumes).

### Job queue [V]
- SQLite-backed queue via the **`liteque`** package. Workers poll for tasks that are enqueued by tRPC procedures. No Redis. (Philosophically similar to Openmind's "no extra infra" goal — but they achieve it on SQLite, Openmind on Postgres/River.)
- Workers are selectively toggled with `WORKERS_ENABLED_WORKERS` / `WORKERS_DISABLED_WORKERS`.
- **Worker pipeline** [V]: `crawler` (metadata, screenshots, PDFs on new bookmark) → `inference` (AI tagging, summarisation, OCR post-crawl) → `search` (Meilisearch indexing on data change) → `assetPreprocessing` (image optimisation) → `webhook` (HTTP dispatch on events) → `feed` (cron RSS ingestion).

### Search implementation [V + I]
- Primary: **Meilisearch** for full-text indexing across titles, page content, OCR-extracted image text, and PDF text [V].
- **Semantic/vector search is NOT a first-class core feature.** There is a **community sidecar** (`jamesbrooksco/karakeep-semantic-search`) that adds embeddings via **Qdrant** + OpenAI/Ollama and exposes a REST API — but that is a separate project, not shipped in core [V].
- Meilisearch itself now supports hybrid/vector search with pluggable embedders (Gemini, Mistral, Cohere, etc.), so Karakeep *could* layer semantic search on Meili, but the core product's search is essentially keyword/FTS [I]. **This is Openmind's stated moat (§5.4 PRD): hybrid FTS + pgvector + rank fusion + optional rerank is a genuine differentiator.**

### AI integration model [V]
- Abstraction: `InferenceClientFactory` (`packages/shared/inference.ts`) constructs either an `OpenAIInferenceClient` or an `OllamaInferenceClient` based on env vars (`OPENAI_API_KEY` or `OLLAMA_BASE_URL`).
- Configurable models: text model, image model, embeddings model (`packages/shared/config.ts`).
- **Only two provider families** are first-class: OpenAI(-compatible) and Ollama. There is no built-in ordered fallback chain, no per-provider rate-limit/token-bucket handling, no batch-tier cost optimisation [I — absence, based on the two-client factory design].
- Enrichment steps: automatic **tagging**, **summarisation**, and **OCR** (OCR results cached via `OCR_CACHE_DIR`). AI is optional; the app functions without it (manual tagging).
- Positioned as "LLM Agents friendly" with a CLI and official "skills" [V].

---

## 2. What they do well (Openmind should learn from)

- **Breadth of capture surfaces** [V]: web app, Chrome + Firefox + Safari extensions, native **iOS** (App Store) and **Android** (Google Play) apps, REST API, CLI. Openmind's PRD matches most of this (WXT extension, Expo mobile, REST) — Karakeep proves the multi-surface bet is right and that a Safari extension matters to the self-host crowd.
- **Rich import paths** [V]: Chrome bookmarks, Pocket, Linkwarden, Omnivore, Tab Session Manager, plus **Floccus** browser-bookmark sync. Openmind PRD §5.8 should explicitly add Omnivore and Linkwarden importers (Omnivore's shutdown created a large migration audience) and consider a Karakeep importer for switchers.
- **Feeds / RSS auto-hoarding** [V]: a cron `feed` worker ingests RSS automatically. Openmind has no feed ingestion in the PRD — worth considering as a post-M1 capture surface.
- **Full-content archival** [V]: Monolith (single-file HTML archive) + yt-dlp video archiving + PDF handling. Openmind PRD covers snapshot archival (§5.2) but does NOT mention video archiving — a gap vs Karakeep.
- **OCR built in** [V]: makes screenshots/images searchable. Openmind plans this via NVIDIA NIM nemotron-ocr (§7.1) — parity, good.
- **Rule-based management engine + bulk actions + webhooks** [V]: automation for power users. Openmind's "Lenses" (§5.5) overlap conceptually; webhooks are a cheap, high-value addition not in Openmind's PRD.
- **Community infrastructure** [V]: Discord, active GitHub discussions/releases, DeepWiki docs, managed cloud (cloud.karakeep.app). Confirms the OSS-plus-hosted-cloud model in Openmind PRD §13 is proven in this exact niche.
- **Clean layered API** [V]: tRPC internally + a Hono REST wrapper for external/agent use. Openmind's OpenAPI-contract-first approach (single generated client) is arguably cleaner for multi-language clients, but Karakeep validates shipping a documented public REST surface for automation/agents.

---

## 3. What to avoid (friction, complaints, principle conflicts)

- **Heavy multi-container footprint** [V]: requires Meilisearch + headless Chrome + app. Community writeups note it "uses more resources than lightweight alternatives like Shiori." **Directly validates Openmind's single-binary + Postgres-only principle** as a differentiator — do not regress into a required search daemon or mandatory browser container.
- **Headless Chrome memory/crashes** [V]: the Chrome container is the dominant CPU/RAM consumer (app idles ~200 MB; Chrome renders pages). It is a known OOM-kill culprit; guides recommend adding RAM/swap. **Lesson:** keep Openmind's in-process Go readability extraction as primary and make any headless-browser/screenshot path strictly optional and off by default.
- **Frontend does not virtualise the grid** [V]: Issue #1978 — the dashboard renders every fetched bookmark at once, each card holding its own React Query subscription, producing millions of DOM nodes and multi-GB browser memory (tabs reported at ~6 GB, crashing the browser). **Direct lesson for Openmind:** the masonry grid MUST be virtualised from day one (PRD §5.3 already mandates this — treat it as non-negotiable, and it's a live spike item in §15).
- **Dev-mode in production image** [V]: an earlier Docker image ran via `tsx` (dev mode), causing high CPU. A Node/TS packaging footgun; Openmind's compiled Go binary sidesteps this class of problem entirely.
- **No native semantic search in core** [V/I]: search is keyword-first; semantic requires a community Qdrant sidecar — added infra and complexity. Openmind's pgvector-in-Postgres approach is both more capable and lower-friction.
- **Two-provider AI model** [I]: OpenAI-or-Ollama only, no fallback chain, no rate-limit-aware batching. Bulk imports on free tiers would be fragile. Openmind's ordered fallback chain + token buckets + batch tiers (§7.1) is a concrete advantage — especially for the "$0 self-hosting" pitch.

---

## 4. Feature matrix — Karakeep vs Openmind (current + PRD roadmap)

| Capability | Karakeep [V unless noted] | Openmind (PRD status) |
|---|---|---|
| Save links / notes / images / PDFs | Yes | Links/notes/images P0; PDF not explicit in PRD (gap) |
| Lists / collections | Yes (with collaboration) | "Lenses" = saved queries (P1); no manual collections by design (capture-is-sacred, machine organises) |
| Manual + AI tagging | Yes | Yes (AI tags P0; manual tags via noop provider) |
| Full-text search | Meilisearch | Postgres FTS (P0) |
| Semantic / vector search | Community Qdrant sidecar only | pgvector hybrid + rank fusion + optional rerank (P0/§7.1) — **Openmind advantage** |
| Natural-language query | No | Yes, LLM-parsed to filters (M2) — **Openmind advantage** |
| Colour / "vibe" search | No | Yes, palette dots + colour search (M2) — **Openmind advantage** |
| OCR | Yes | Yes (NVIDIA NIM, §7.1) |
| Full-page archival | Monolith | Snapshot archival (S3/local) P0 |
| Video archiving | Yes (yt-dlp) | **Not in PRD — gap** |
| RSS / feeds | Yes (cron worker) | **Not in PRD — gap** |
| Rules engine / automation | Yes | Lenses (overlap); no rules engine |
| Webhooks | Yes | **Not in PRD — gap** |
| Bulk actions | Yes | Not specified |
| Browser extension | Chrome/Firefox/Safari | WXT (Chrome/Edge/Firefox) P0; Safari not mentioned |
| Mobile apps | Native iOS + Android (stores) | Expo, share-sheet-first (M3/P1) |
| Import | Chrome, Pocket, Linkwarden, Omnivore, Tab Session Mgr, Floccus | Pocket, Raindrop, Hoarder/Karakeep, browser HTML, mymind (P0) — should add Omnivore, Linkwarden |
| Export | Yes (REST/data) | Full JSON + media dump (P0) — headline feature |
| MCP server | No (has CLI + agent "skills") | First-class MCP (M3) — **Openmind advantage** |
| Send to Kindle | No | Yes (M4) — **Openmind advantage** |
| Desktop dock | No | Tauri dock (M4/P2) — **Openmind advantage** |
| Drift / resurfacing | No | Yes (M3) — **Openmind advantage** |
| Design-forward visual grid | Utilitarian | Core differentiator (paper/ink/cobalt, palette dots) |
| Self-host footprint | 3–4 containers (app+Meili+Chrome+workers) | 1 container + Postgres — **Openmind advantage** |
| Database | SQLite | Postgres 16 + pgvector |
| Job queue | liteque (SQLite) | River (Postgres) |
| Multi-language UI | Yes | Not specified |
| Dark mode | Yes | Not specified (design-led) |

**Net read [I]:** Karakeep wins today on breadth-of-capture maturity (native mobile apps in stores, Safari ext, feeds, video archiving, webhooks, shipped and stable). Openmind's differentiation is design quality, hybrid/semantic/colour/NL search, resurfacing (Drift), MCP, Send-to-Kindle, and a materially lighter single-binary+Postgres deployment.

---

## 5. Licence & code-reuse considerations

- **Licence: AGPL-3.0** [V], copyright **Localhost Labs Ltd**.
- Openmind's proposed licence is also **AGPL-3.0** (PRD §13.1) — **compatible in principle**, but:
  - AGPL is copyleft with the network-use clause. You may **read** Karakeep's code for ideas/architecture, but **copying substantial code verbatim into Openmind would pull in AGPL obligations and, critically, Localhost Labs' copyright** — you cannot relicense it, and it would complicate any future dual-license/cloud arrangement you retain via your own CLA/DCO (PRD §13.1).
  - **Do not** copy source files, schema, or non-trivial code blocks. Re-implement independently. Feature parity is fine; expression/copyright is theirs.
  - Their stack is TypeScript/Node — Openmind is Go server-side, so accidental copy-paste is unlikely, but the browser-extension and web layers are the risk zone (both TS/React). Keep a clean-room boundary there.
  - Interop is safe and encouraged: writing a **Karakeep importer** (reading their export format) creates no licence entanglement and is a good switcher on-ramp.

---

## Sources

- [Karakeep GitHub repo](https://github.com/karakeep-app/karakeep)
- [Karakeep Architecture — DeepWiki](https://deepwiki.com/karakeep-app/karakeep/2-architecture)
- [Karakeep Docs — Introduction](https://docs.karakeep.app/)
- [Issue #1978 — Browser memory just grows](https://github.com/karakeep-app/karakeep/issues/1978)
- [Self-Host Setup — Karakeep AI bookmark manager](https://selfhostsetup.com/posts/karakeep-ai-bookmark-manager/)
- [Deployn — Install Karakeep with Docker](https://deployn.de/en/blog/karakeep-server/)
- [karakeep-semantic-search (community Qdrant sidecar)](https://github.com/jamesbrooksco/karakeep-semantic-search)
- [Meilisearch — vector/hybrid search](https://www.meilisearch.com/blog/what-is-vector-search)

## Research gaps / caveats

- SQLite vs Postgres: one general blog implied Postgres; the authoritative DeepWiki architecture doc says SQLite (`better-sqlite3`). Treated SQLite as correct — worth a direct code check (`packages/db`) before quoting publicly.
- Did not open the full Karakeep issue tracker beyond the memory issue; a deeper pass on import-quality and enrichment-failure complaints would strengthen §3.
- Exact list of shipped importers may have grown since the cited docs; verify against docs.karakeep.app before publishing an Openmind comparison table externally.

# Masonry grid perf spike (2026-07-03)

**Question:** Does the current CSS-columns masonry (`.grid { columns: 4 280px }` in `apps/web/app/globals.css`) hold 60fps while scrolling 500 mixed cards, or do we need to virtualise the grid before OSS launch?

## Setup

- Spike page: `apps/web/app/spike/grid/page.tsx` — renders N deterministic synthetic items (mulberry32 seed `0x5eed`, no API) through the **real** `ItemCard` component and the **real** `.grid` CSS, so the measurement reflects production rendering. Card types are mixed across all nine (`article`, `note`, `tweet`, `image`, `video`, `product`, `book`, `quote`, `recipe`); ~40–100% carry a `picsum.photos` lead image at `?w=400` with a height varied 200–600px so columns actually stagger; text length varies per card; ~15% render the `enriching…` pending state. Item count via `?n=` (default 500).
- `/spike` added to the `middleware.ts` bypass (same pattern as `/api/`) — synthetic data only, no auth.
- Build: `pnpm turbo run build --filter=web` green. Served with `next start -p 3947` (production build, not dev — no HMR/Fast Refresh overhead).
- Measurement: Playwright MCP → `browser_evaluate` running a ~6s programmatic smooth-scroll loop (`window.scrollTo` stepped via `requestAnimationFrame`), counting rAF frame deltas: frame count, average FPS, dropped frames (delta > 25ms), longest frame, average frame time. **3 runs, median reported.** Run on a 120Hz display (rAF is therefore capped at ~120fps — the load-bearing metrics are dropped-frame count and average frame time against the 16.67ms/60fps budget, not the FPS ceiling).

## Numbers

### 500 cards (`/spike/grid`) — scrollHeight ~48,100px, 500 `<article>` nodes

| Run | Avg FPS | Dropped (>25ms) | Longest frame | Avg frame |
|-----|---------|-----------------|---------------|-----------|
| 1   | 121.3   | 0               | 16.6 ms       | 8.25 ms   |
| 2   | 120.0   | 0               | 16.7 ms       | 8.34 ms   |
| 3   | 120.1   | 0               | 10.4 ms       | 8.33 ms   |
| **Median** | **120.1** | **0** | **10.4 ms** | **8.33 ms** |

### 1000 cards (`/spike/grid?n=1000`) — scrollHeight ~95,400px, 1000 `<article>` nodes (headroom)

| Run | Avg FPS | Dropped (>25ms) | Longest frame | Avg frame |
|-----|---------|-----------------|---------------|-----------|
| 1   | 120.1   | 0               | 10.4 ms       | 8.32 ms   |
| 2   | 120.1   | 0               | 18.9 ms       | 8.33 ms   |
| 3   | 120.1   | 0               | 10.4 ms       | 8.33 ms   |
| **Median** | **120.1** | **0** | **18.9 ms** | **8.33 ms** |

LCP was not captured via `getEntriesByType('largest-contentful-paint')` in the eval context (returned empty); render was visually immediate. All cards are present in the DOM at first paint (no client-side pagination).

## Verdict

**PASS — comfortably clears the 60fps/500-cards bar.** Median 120.1fps (display-capped) with **zero dropped frames** and an **8.33ms average frame time — half the 16.67ms budget for 60fps**. At 1000 cards the numbers are unchanged: still 0 dropped frames, same average frame. Scroll is GPU-composited; CSS multi-column layout is computed once and does not re-run per frame, so scrolling stays cheap regardless of item count.

## Recommendation

**Keep CSS columns for Milestone 1 — do not virtualise the scroll now.** Scroll performance is not the bottleneck the earlier Karakeep note (§3, issue #1978) warned about; that failure mode was **memory / DOM-node growth and per-card React Query subscriptions**, not paint jank. Our cards are cheap static markup with no per-card subscription, so 500–1000 nodes scroll flawlessly.

Caveats and follow-ups (not blockers for M1):

- The real risk at scale is **memory and initial-render cost**, not scroll FPS. This spike deliberately measured scroll jank; it did not profile heap or first-paint at 5000+ cards. Openmind's default listing is bounded (recents / search results), so unbounded lists are not a near-term concern.
- **`content-visibility: auto; contain-intrinsic-size` was not needed** and was therefore not benchmarked — FPS was already at the display ceiling with 0 dropped frames, so there was no jank delta to recover. It remains the recommended **cheap first lever** if a future unbounded "all items" view ever shows first-paint or memory pressure: add it to the `.grid > *` wrapper to let the browser skip layout/paint of off-screen cards with near-zero code change and no new dependency (aligns with the single-binary / minimal-deps principle).
- Only if a genuinely unbounded, thousands-of-items view is introduced **and** `content-visibility` proves insufficient should true windowing be considered. Preferred approach then: **`@tanstack/react-virtual`** (headless, unopinionated, works with custom layouts) over a masonry-specific lib — but masonry + virtualisation is genuinely hard (variable heights across columns), so exhaust `content-visibility` first.

