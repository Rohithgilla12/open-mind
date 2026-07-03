# PRD — "Openmind" (working title)

**An open-source, self-hostable extension for your mind.**

| | |
|---|---|
| Author | Rohith Gilla |
| Date | 2026-07-03 |
| Status | Draft v0.1 |
| Licence | AGPL-3.0 (proposed — see §12) |

---

## 1. Vision

A private, beautiful, AI-enriched place to save everything you care about — notes, links, images, quotes, articles — with **zero manual organisation**. You save; the system understands, enriches, and resurfaces.

The open-source twist: **you own the data and the AI**. Self-host it on a VPS, point it at your own LLM provider (Anthropic, OpenAI, Ollama local models), and never worry about the company shutting down or changing its pricing.

**One-liner:** *Remember everything. Organise nothing. Own all of it.*

## 2. Why this should exist

- mymind is closed-source, subscription-only (~$6.99–12.99/mo), no export-first mindset, no API, no self-hosting.
- Existing OSS alternatives each miss the mark:
  - **Linkwarden / Shiori / Wallabag** — bookmark/read-later focused, folder-and-tag heavy, no visual-first UI, weak AI.
  - **Memos / Joplin** — notes-first, not capture-first.
  - **Hoarder (Karakeep)** — closest competitor: OSS, AI tagging, self-hosted. But UI is utilitarian, no serendipity/resurfacing, weak associative search, no "vibe" search.
- The gap: **design-quality + AI-native enrichment + privacy/self-hosting** in one product. Nobody combines mymind's taste with OSS ownership.

## 3. Target users

1. **Design-minded developers & indie hackers** — want a moodboard + reference hub, comfortable with Docker, allergic to subscriptions for their own data.
2. **Privacy-conscious knowledge workers** — researchers, writers who won't put their reading history in a US SaaS.
3. **Homelab / self-host community** (r/selfhosted, awesome-selfhosted) — a proven, passionate distribution channel.

## 4. Product principles

1. **Capture is sacred** — saving must take < 1 second and never interrupt flow.
2. **The machine organises** — no required folders, tags, or filing. Structure is emergent (AI) not imposed (user).
3. **Search over navigation** — you find things by fragments: a colour, a word, a brand, "that article about Raft".
4. **Private by default** — no social layer, no sharing (v1), no analytics phoning home.
5. **Your models, your keys** — pluggable AI provider; degrade gracefully with no AI at all.
6. **Beautiful or nothing** — the visual grid IS the product. If it looks like a database admin panel, we've failed.

## 4.1 Identity & differentiation — inspired, not a clone

mymind and Anybox inform the *problem space*; the product must have its own identity. Rules:

**Own metaphor.** mymind's metaphor is "your mind." Ours is the **commonplace book** — the centuries-old practice (Da Vinci, Locke, Woolf) of keeping a personal volume of quotes, clippings, sketches and ideas. Same job, deeper lineage, and it gives us a naming/copy vocabulary ("pages", "gatherings", "marginalia") that is genuinely ours. It also speaks directly to our readers/researchers/engineers audience.

**Own feature names.** Never reuse mymind's names. Renames now canonical throughout this PRD:
- ~~Smart Spaces~~ → **Lenses** (a Lens is a saved query you look at your collection through — arguably a clearer name for the mechanic)
- ~~Serendipity~~ → **Drift** (calm resurfacing of forgotten saves)
- ~~Top of Mind~~ → **Desk** (what's on your desk right now)

**Own copy.** Never echo their taglines ("extension for your mind", "remember everything, organize nothing"). Our line comes from our metaphor and our differentiators, e.g.: *"Your commonplace book, kept by a machine — private, self-hosted, yours."* (Iterate later; direction is what matters.)

**Own visual language.** Already divergent by design: mymind is soft, cream, image-forward with hidden metadata; we are gallery paper + ink + cobalt, mono-type metadata *visible* (AI tags, reading time), and the signature **extracted-palette dots on every card**. Keep widening this gap, never close it.

**Own capabilities as the story.** Lead marketing with what they structurally can't do: self-hosting and data ownership, MCP (talk to your collection from Claude), bring-your-own-model with a $0 tier, rerank-quality search, cross-platform dock. The pitch is ownership, not imitation.

**Line we don't cross:** no copying of their UI layouts pixel-for-pixel, illustrations, onboarding flows, or manifesto text. Feature parity is legitimate; expression is theirs.

## 5. Core features

### 5.1 Capture (P0)
- **Browser extension** (Chrome/Edge/Firefox via WXT): one-click save of page, selection (highlight → quote card), or image (right-click).
- **Web app quick-add**: paste a URL, drop an image, or type a note. Cmd+K everywhere.
- **Mobile (P1)**: Expo app whose primary surface is the **share sheet**, not the app itself. From any app (browser, Twitter, YouTube): Share → Openmind → saved → dismissed, in under 2 seconds.
  - Android: share intent target for URLs/text/images (straightforward).
  - iOS: native Share Extension via `expo-share-intent` / `expo-share-extension` config plugins + custom dev client (not Expo Go).
  - Save is optimistic and fire-and-forget: instant "Saved ✓" toast, offline queue for bad networks, enrichment happens server-side. Opening the full app is optional.
- **API-first**: every capture path hits the same public REST endpoint — enables n8n, Raycast, CLI, MCP server integrations for free.

### 5.2 Ingestion & enrichment pipeline (P0)
On save, an async pipeline (Go workers, queue-backed):
1. **Content extraction** — in-process Go readability extraction (go-readability / go-trafilatura) as primary; **Jina Reader** (`r.jina.ai`) as fallback for stubborn/JS-heavy pages; OG metadata always; full-page screenshot as last resort.
2. **Type detection** — article / product / book / recipe / video / tweet / image / note / quote. Each type gets a bespoke card renderer.
3. **AI enrichment** — summary (2–3 sentences), auto-tags, extracted entities (brand, author, price), dominant colours for images.
4. **Embedding generation** — text + image embeddings stored in pgvector for semantic search.
5. **Archival** — full-content snapshot stored (S3-compatible / local disk), so links never rot.

Pipeline must be **idempotent and retryable**; enrichment failures never block the save.

**Queue & batching (River, Postgres-backed):**
- Every save enqueues an enrichment job; capture returns instantly, the card appears immediately and enriches in place.
- Jobs **batch** where providers support it — group pending summaries/tags into one batched API call (50% discount on Anthropic/OpenAI batch tiers), and batch embeddings natively (embedding APIs accept arrays).
- **Rate-limit aware per provider**: token buckets per provider config, exponential backoff on 429, automatic failover down the provider chain. This is what makes free tiers (Cerebras/Groq/Gemini) viable for bulk imports — a 3,000-item Pocket import just queues and grinds through overnight with visible progress, instead of failing.
- Priority lanes: interactive saves > re-enrichment > bulk imports.
- Dead-letter queue with a visible "needs attention" state in the UI; one-click retry.

### 5.3 The Mind (grid view) (P0)
- Masonry grid of visual cards, type-aware rendering (product cards show price/image; recipes show ingredients; quotes are typographic).
- Infinite scroll, keyboard navigable, blazing fast (virtualised).
- Card detail view: original link, clean reader mode, AI summary, edit notes.

### 5.4 Search (P0 — the moat)
- **Hybrid search**: full-text (Postgres FTS) + semantic (pgvector) + structured filters, fused with reciprocal rank.
- Search by: keyword, colour (`color:teal`), type (`type:recipe`), domain, date, "similar to this" (embedding neighbours).
- **Natural-language queries**: "that Go article about durable workflows I saved last month" → LLM parses to filters + semantic query.
- < 200 ms p95 on 50k items.

### 5.5 Lenses (P1)
- A Lens = a saved query/rule ("anything about running shoes", "design inspiration with warm colours"). New saves auto-appear through it. No manual filing.
- Manual pin/unpin to override.

### 5.6 Drift & Desk (P1)
- **Drift**: a calm, full-screen mode resurfacing old saves one at a time — keep or let go. Spaced-resurfacing algorithm (older + never-revisited items weighted up).
- **Desk**: pinboard shown on open — what you're working with right now.

### 5.7 Reader mode (P1)
- Distraction-free article reading of archived content, highlights saved back as quote cards.

### 5.8 Import/Export (P0 for trust)
- Import: Pocket, Raindrop, Hoarder, browser bookmarks HTML, mymind export.
- Export: full JSON + media dump at any time. This is a headline feature for an OSS project.

### 5.9 MCP server (P1 — the AI-native differentiator)
- First-class MCP server shipped with the app: `save_item`, `search_mind`, `get_item`, `list_lenses`, `add_note`.
- Claude (or any MCP client) becomes an interface to your mind: "save this article", "what did I save about Raft last month", "summarise everything through my design-inspiration lens".
- Reuses the same REST API; auth via API token. Build guided by the mcp-builder patterns already in use for kindle-mcp-server.
- This is something mymind, Anybox, and Karakeep all lack — lead marketing with it.

### 5.10 Send to Kindle (P3 — Kindling absorbed)
- Any saved article → "Send to Kindle" action: clean extracted content → EPUB → delivered via Send-to-Kindle email.
- Batch mode: send a whole Lens as a compiled digest ("this week's saves as one EPUB").
- Reuses Kindling's proven pipeline (extraction → EPUB → email); Openmind becomes its superset and Kindling retires or becomes a thin frontend.
- Resolves Open Question #4.

### 5.11 Floating dock — desktop companion (P2, inspired by Anybox's Anydock)
- Small **Tauri** desktop app (Loadout experience reuses directly):
  - **Floating dock / menu-bar icon**: pinned favourite links + Desk items, one click to open.
  - **Quick Save** (global hotkey): grabs frontmost browser tab's URL + title, saves instantly.
  - **Quick Find** (global hotkey): Spotlight-style search over your mind, Enter to open.
- Thin client over the REST API; cross-platform (macOS/Windows/Linux) — beats Anybox's Apple-only limitation.
- Unlimited pins (Anybox caps at 12 free / 30 pro — an easy "OSS is better" talking point).

## 6. Explicit non-goals (v1)
- Collaboration, sharing, public profiles, social anything.
- Real-time multiplayer editing.
- Full document editor (notes are markdown, simple).
- Full-featured native desktop app (the Tauri companion is a thin dock/search utility, not a full client — PWA covers the main app).

## 7. Architecture

```
┌─────────────┐  ┌──────────────┐  ┌─────────────┐
│ WXT ext.    │  │ Next.js web  │  │ Expo mobile │
└──────┬──────┘  └──────┬───────┘  └──────┬──────┘
       └────────────────┼─────────────────┘
                 REST + SSE (Go API)
                        │
        ┌───────────────┼────────────────┐
        │               │                │
   PostgreSQL      Worker pool      Object store
   (+ pgvector,    (Go, River      (S3-compatible
    FTS)           queue)           or local disk)
                        │
              AI provider adapter
        (Anthropic / OpenAI / Ollama / none)
```

**Stack rationale (plays to existing strengths):**
- **Backend**: Go — API + workers in one binary. River (Postgres-backed job queue) avoids a Redis dependency; single `docker compose up` with just Postgres + app.
- **DB**: Postgres 16 + pgvector + FTS. One database for everything.
- **Web**: Next.js (or TanStack Start — decide in spike; Kindling learnings apply).
- **Extension**: React-based. **WXT** as default (actively maintained, Kindling scaffolding reuse); Plasmo acceptable alternative — both are thin React wrappers over the manifest, swap cost is low.
- **Extraction**: in-process Go library first (no extra service, keeps the single-binary story), Jina Reader as configurable API fallback.
- **Mobile**: Expo, share-sheet-first (see §5.1), thin client over the REST API (P1, post-launch). Backend stays Go — decided.
- **AI adapter**: interface with `Summarise`, `Tag`, `Embed`, `ParseQuery`; implementations per provider; a `noop` provider means the app fully works without AI (manual tags, FTS-only search).

### 7.1 AI model strategy — cheap by design

Enrichment tasks (tagging, summaries, type detection, query parsing) are simple, high-volume classification/extraction — **never use flagship models**. Default to budget tiers; the adapter makes the model a config value, not a code change.

**Supported providers (v1):** Anthropic, **Google Gemini**, OpenAI, and any **OpenAI-compatible endpoint** (DeepSeek, Cerebras, Groq, OpenRouter, NVIDIA NIM, LiteLLM, Ollama) via configurable base URL.

**Fallback chain (built into the Go adapter, not a proxy dependency):** providers are configured as an ordered list; on 429/5xx the worker retries the next provider. This natively replicates the LiteLLM pattern without adding a Python sidecar — and anyone who prefers LiteLLM/OpenRouter as a gateway just points `base_url` at it.

**The $0 self-hosting path (headline README feature):**
- Inference: **Cerebras free tier** (huge quota, extremely fast) → **Groq free** → **OpenRouter free models** → or **Gemini free tier** — all zero cost with just API keys.
- Fully offline: **Ollama** (Qwen3 4B–8B, Gemma 3) — no keys, no network.
- Caveat handled in-product: free tiers are rate-limited, so bulk imports queue with backoff and clearly show progress rather than failing.

**Recommended paid defaults (as of mid-2026 pricing, per 1M tokens in/out):**

| Tier | Model | Price | Use |
|---|---|---|---|
| Cheapest API | Gemini 2.5 Flash-Lite | ~$0.10 / $0.40 | Default for tags, type detection, summaries |
| Cheapest OSS API | DeepSeek V3.2 | ~$0.14 / $0.28 | Alternative default; OpenAI-compatible, 90% cache discount |
| Balanced | Gemini 2.5 Flash / Claude Haiku 4.5 | ~$0.15–1 / $0.60–5 | NL query parsing (needs better instruction following) |
| Free / local | Ollama: Qwen3 4B–8B, Gemma 3 4B, Llama small | $0 (your hardware) | Fully private self-hosting; runs on a modest VPS/homelab |
| Never | Opus / GPT-5 / flagships | — | Not needed for any pipeline task |

**Gemini free tier** is a headline onboarding path: generous free quota means a new self-hoster can run the full AI pipeline at **$0** with just a Google API key.

**Embeddings:**
- Default API: **Gemini embedding** (top-tier quality, free tier) or OpenAI `text-embedding-3-small`.
- **Jina embeddings** — first-class adapter: multilingual + **multimodal** (text and images in one vector space), which directly powers "similar vibe" visual search.
- Free: **NVIDIA NIM** (open embedding models, free API keys).
- Local: `nomic-embed-text` or `bge-m3` via Ollama — free, fast on CPU.

**Reranking (optional search stage, quality differentiator):** hybrid search retrieves top ~50 (FTS + vector), then an optional reranker (**Jina reranker** or NVIDIA NIM reranker models) reorders before display. Config-gated, off by default, big precision win for power users. None of the competitors do this.

**Extraction:** Go-native readability library (go-readability / go-trafilatura) as primary — in-process, zero dependencies; **Jina Reader** (`r.jina.ai`, free tier) as the fallback for stubborn/JS-heavy pages. For text-in-images (screenshots, posters), **nemotron-ocr-v2 via NVIDIA NIM's free endpoints** gives us OCR-powered image search at $0 — a feature mymind gates behind its paid tier.

**Cost envelope target:** enriching a saved item ≈ 2–4k input tokens + ~300 output tokens. At Flash-Lite rates that's **< $0.001 per save** — 1,000 saves/month ≈ $0.50–1. Publish this math in the README; "costs less than a coffee per year" is the pitch.

**Cost controls built in:** batch enrichment where the provider supports it (50% off), prompt caching for the shared system prompt, per-task model overrides in config (`enrich_model`, `query_model`, `embed_model`).

**Deployment targets:** Docker Compose (primary), single VPS behind Cloudflare Tunnel, optional hosted cloud later (see §12).

## 8. Repository & code structure

**Single monorepo** — five surfaces, one API contract, one PR can change the API and every client together.

```
openmind/
├── apps/
│   ├── api/                # Go — API server + River workers (one binary)
│   │   ├── cmd/openmind/   # main.go (serve | work | migrate | all-in-one)
│   │   ├── internal/
│   │   │   ├── api/        # HTTP handlers, middleware, auth
│   │   │   ├── enrich/     # pipeline stages: extract, classify, summarise, embed
│   │   │   ├── ai/         # provider adapters + fallback chain (openai-compat, anthropic, gemini, jina, ollama, noop)
│   │   │   ├── search/     # hybrid search: FTS + pgvector + fusion (+ optional rerank)
│   │   │   ├── store/      # sqlc-generated queries, migrations
│   │   │   └── jobs/       # River job definitions, priority lanes
│   │   └── migrations/
│   ├── web/                # Next.js / TanStack Start
│   ├── extension/          # WXT (React)
│   ├── mobile/             # Expo (share-sheet-first)
│   └── dock/               # Tauri (P2)
├── packages/
│   ├── api-client/         # TS client GENERATED from OpenAPI — never hand-written
│   ├── ui/                 # shared React components + design tokens (paper/ink/cobalt)
│   └── config/             # shared tsconfig, eslint, prettier
├── openapi.yaml            # THE contract — single source of truth
├── docker-compose.yml      # postgres + api; `docker compose up` = running product
├── turbo.json              # JS task orchestration
├── go.work
└── .github/workflows/      # ci.yml, release.yml
```

**Key decisions:**
- **The API contract is the spine.** `openapi.yaml` at the root; Go handlers generated/validated with `oapi-codegen`, TS client generated into `packages/api-client`. Web, extension, mobile, and dock all consume the same generated client — API drift becomes a compile error, not a runtime bug.
- **Go stays idiomatic, not shoehorned.** `go.work` at root, standard `internal/` layout, `sqlc` for typed queries. Turborepo orchestrates JS tasks only; Go builds via Make/Go directly (CI calls both).
- **pnpm workspaces + Turborepo** for the four JS/TS apps and shared packages; remote caching later if CI slows.
- **One binary ships everything server-side**: `openmind serve` (API), `openmind work` (workers), `openmind all` (both, the self-host default), `openmind migrate`. The web app is statically built and embedded via Go `embed` in release builds — self-hosters deploy literally one container.
- **CI (GitHub Actions):** lint+test per workspace (Turbo filters + `go test ./...`), extension zip artifact, Docker image on tag via GoReleaser + ko or a plain Dockerfile, EAS build for mobile on release branches only.
- **Versioning:** single version for the platform (api+web+image), extension and mobile released on their own store cadence but pinned to a minimum API version.

**Repo hygiene for OSS launch (data-peek playbook):** CONTRIBUTING.md, architecture doc with the pipeline diagram, `docker compose up` quickstart in README above the fold, good-first-issue labels seeded before the HN post.

## 9. Data model (sketch)

```
items(id, user_id, type, url, title, summary, content_md,
      metadata jsonb, colors text[], created_at, archived_at)
item_embeddings(item_id, embedding vector)
tags(id, name, user_id) / item_tags(item_id, tag_id, source: ai|user)
lenses(id, name, rule jsonb) 
highlights(id, item_id, text, note)
links(from_item, to_item)          -- bidirectional linking (P2)
assets(id, item_id, kind: screenshot|archive|image, storage_key)
```

## 10. MVP cut (what ships first)

**Milestone 1 — "Save & find" (4–6 weeks of evenings):**
- Go API + Postgres + worker pipeline
- URL/note/image capture via web app + Chrome extension
- Type detection + AI summary/tags (Anthropic adapter + noop)
- Masonry grid + hybrid search
- JSON export

**Milestone 2 — "It feels magic":** colour search, NL query parsing, Lenses, reader mode, imports.

**Milestone 3 — "It's alive":** Drift, Desk, mobile share sheet, **MCP server**.

**Milestone 4 — "Everywhere":** Tauri floating dock (Quick Save / Quick Find / pinned bar), bidirectional linking, **Send to Kindle** (single item + Lens digest).

## 11. Success metrics
- 1,000 GitHub stars in 3 months (data-peek playbook: HN launch, r/selfhosted, awesome-selfhosted PR).
- 200 active self-hosted instances (opt-in anonymous ping) by month 6.
- Capture-to-enriched latency p95 < 30 s.
- Contributor health: 10+ external contributors merged in 6 months.

## 12. Risks

| Risk | Mitigation |
|---|---|
| Hoarder/Karakeep already owns the niche | Differentiate on design quality, associative/colour search, Drift resurfacing — the "taste" features they lack. |
| AI cost scares self-hosters | Ollama/local-first path + full no-AI mode from day one. |
| Extraction quality (paywalls, SPAs) | Tiered pipeline: Go lib → Jina Reader → screenshot always; community-maintained site rules later. |
| Scope creep into "Notion clone" | Non-goals enforced; notes stay simple markdown. |
| Solo-maintainer burnout | Milestone 1 is genuinely small; heavy reuse of Kindling/data-peek/Loadout assets. |

## 13. Open-source & sustainability

### 13.1 Licence
- **AGPL-3.0** — protects against closed-source SaaS forks while keeping self-hosting free. Critically, it does NOT prevent *you* (the copyright holder) from running the hosted cloud version — and requires competitors who host it to open-source their modifications.
- All contributors sign a lightweight CLA (or use DCO) so you retain the right to offer the cloud service and dual-license later if ever needed. Decide this **before** the first external PR — retrofitting is painful.

### 13.2 Cloud offering (the sustainability engine)
Same codebase, hosted and managed — the proven Plausible / Cal.com / Ghost model. Open-source is the distribution channel; cloud is the revenue.

**Positioning:** "Don't want to run a server? We'll run it for you." Target the 90% of interested users who will never touch Docker.

**Pricing sketch (validate later):**

| Plan | Price | Includes |
|---|---|---|
| Free | $0 | 200 items, community support — enough to feel the magic |
| Pro | ~$4–5/mo or ~$40/yr | Unlimited items, AI enrichment included (no API key needed), full archive storage |
| Founder Lifetime | ~$79, capped at first 200 | Runstamp playbook — early revenue + evangelists |

Undercuts mymind (~$7–13/mo) meaningfully while AI costs stay negligible (§7.1 math: even a heavy user costs pennies/month in enrichment).

**Unit economics guardrails:**
- AI enrichment on cloud runs on Flash-Lite/DeepSeek tier → < $0.05/user/month even for power users.
- Storage is the real cost driver (article archives, screenshots, images) → per-plan storage caps, image optimisation, cold storage (R2/B2) for old archives.
- Infra: start on the existing VPS + Cloudflare (near-zero fixed cost); move to managed Postgres + object storage only when revenue justifies it.

**Architecture implications (this changes v1 decisions):**
- **Multi-tenant from day one** — `user_id` on every table, row-level scoping enforced in the API layer. Retrofitting tenancy is a rewrite; building it now is cheap. (Resolves Open Question #2: multi-user, day one.)
- Auth: email magic link + OAuth (Google/GitHub). Self-hosted single-user mode = auto-provisioned default account, zero-config.
- Billing: Stripe (existing expertise), feature-flag gating by plan.
- Usage metering on AI calls + storage per user from the start — needed for both cost control and plan limits.
- **Trust boundary:** cloud users' API keys are never required (we supply AI); self-hosters bring their own. Clear privacy policy: no training on user data, export always available, deletion is real.

**Sequencing:** ship OSS first, build stars + community (months 0–4); launch cloud beta once ~50 people have asked for hosting (the demand signal); Founder Lifetime deal at cloud launch.

### 13.3 Secondary revenue
- GitHub Sponsors (data-peek playbook already proven).
- Optional paid support/priority for self-hosting orgs (later).

## 14. Open questions
1. Next.js vs TanStack Start for the web app?
2. ~~Single-user vs multi-user?~~ **Resolved: multi-tenant from day one** (§13.2) — self-hosted single-user mode is just an auto-provisioned account.
3. Image embeddings: CLIP via local ONNX vs provider API?
4. ~~Does this merge with or supersede Kindling?~~ **Resolved: Openmind absorbs Kindling** — Send to Kindle ships as §5.10 in Milestone 4.
5. Name — must work as both an OSS repo and a cloud brand with an available domain. Candidates: `mindkeep`, `remnant`, `keepsake`, `stash`, or "-peek" suite fit.
6. CLA vs DCO for contributions (must decide before first external PR — see §13.1).

## 15. Week-0 spike (before committing)
- [ ] Prove pipeline: URL → Go readability extract (Jina Reader fallback) → cheap-model summary/tags → pgvector embed → hybrid search returns it. One evening, one binary.
- [ ] Extraction bake-off on 20 real URLs from your saves (blogs, docs, SPAs, paywalled): go-readability vs go-trafilatura vs Jina Reader — pick the primary on data.
- [ ] Render 500 mixed cards in a virtualised masonry grid at 60 fps.
- [ ] Deep-dive Karakeep's repo: what to learn, what to avoid.
