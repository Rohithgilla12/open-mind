# Web Design Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]` tracking.

**Goal:** Reskin the web app to the `docs/design` warm-editorial handoff — foundation, app shell, type-aware cards, palette dots, restyled detail/capture/search. Spec: `docs/superpowers/specs/20260704-design-pass-design.md`. Pixel authority: `docs/design/Openmind.dc.html` + `docs/design/README.md`.

**Architecture:** Almost all in `apps/web` (App Router, strict TS, inline-style-object components using `@openmind/ui` tokens). One backend task adds a `palette` field (Go pipeline extracts dominant colours from lead images). Fonts via `next/font/google`.

## Global Constraints

- Canonical palette = `docs/design/README.md` warm set (see spec); update `packages/ui/src/tokens.ts` + CLAUDE.md tokens line. **No hardcoded colours in apps** — everything from `@openmind/ui`. After the token swap, grep apps/web for stray hex and route through tokens.
- Match the mockup's CSS closely (`.serif/.meta/.tag/.dot/.card/.chip/.savebtn`, card shadow + `translateY(-2px)` hover, warm paper + rule texture). Read `docs/design/Openmind.dc.html` for exact values — don't approximate.
- **Broken/missing lead images must fall back to the type's gradient/placeholder — never a broken-image glyph.**
- Data only via `apiFetch`/proxies; token stays server-side; `assetSrc` rewrite for `/assets/` still applies. Strict TS; `pnpm turbo run build lint --filter=web` green. Go: `go test -p 1 ./... && golangci-lint run ./...` green. No banner comments; Go errors `%w`. Commit per task; no dead nav (M2 items rendered muted "soon").
- Fonts: Newsreader / Instrument Sans / JetBrains Mono via `next/font/google` (self-hosted by Next — no external CSS link, CSP-friendly).

---

### Task 1: Design foundation — tokens, fonts, global CSS, shell

**Files:** `packages/ui/src/tokens.ts`, `CLAUDE.md` (tokens line), `apps/web/app/layout.tsx`, `apps/web/app/globals.css`, `apps/web/lib/fonts.ts` (next/font), `apps/web/components/Shell.tsx`, `apps/web/app/page.tsx` (wrap in Shell)

**Interfaces:** Produces the `tokens` warm palette (add `canvas`, `panel`, `cardSurface`, `inkMuted`, `inkFaint`, `terracotta`, `gold`, `green`, `noteSurface`, `hairline`); `Shell` (server component) = sidebar + main column, renders `children` in the content area; global `.serif/.meta/.tag/.dot/.card/.chip` classes in globals.css mirroring the mockup.

- [ ] Step 1: rewrite `tokens.ts` to the spec's warm values (keep `surface`→`cardSurface #FCFBF6`, keep `danger`). Update the CLAUDE.md design-tokens line to match + add a note "canonical palette lives in docs/design/README.md + packages/ui".
- [ ] Step 2: `lib/fonts.ts` — `next/font/google` for Newsreader (ital + weights 400/500/600), Instrument_Sans (400/500/600), JetBrains_Mono (400/500/600); export CSS variables; apply on `<body>` in layout.tsx. Body bg `#E4DDCD`, base font Instrument Sans, `::selection` cobalt.
- [ ] Step 3: `globals.css` — port the mockup's helper classes verbatim (`.serif`, `.meta`, `.tag`, `.dot`, `.card` + `.card:hover`, `.chip`, `.savebtn`, `.mind-col>*`) using the font variables; add the paper texture utility (`repeating-linear-gradient(#00000000 0 31px, rgba(28,26,22,.028) 31px 32px)`).
- [ ] Step 4: `Shell.tsx` — 230px sidebar (`#EBE5D7`): logo mark + "Openmind" (Newsreader 600); nav: **The Mind** active (cobalt-tint), **Desk/Drift** + **Lenses** section rendered muted with a small "soon" tag and `cursor:default` (no links); account row (avatar initial "R", "Owner · signed in"); storage meter (green bar, static "local · self-hosted"). Main column renders children. `display:flex;height:100vh`.
- [ ] Step 5: wrap `page.tsx` content in `<Shell>`; `pnpm turbo run build lint --filter=web` green; commit `feat(web): warm design foundation — tokens, fonts, css helpers, app shell`.

---

### Task 2: Type-aware cards + image fallback

**Files:** `apps/web/components/ItemCard.tsx` (rewrite), `apps/web/components/Grid.tsx` (column-count 3, gap 16), `apps/web/lib/cards.ts` (helpers: `domainOf`, `assetSrc` already exists — move/keep; `cardKind` normalisation)

**Interfaces:** Consumes item `{cardType,title,summary,tags,leadImageUrl,url,status,body}`. Produces per-type card renderers matching the mockup treatments. Broken-image handling: an `<img>` with `onError` swapping to the type gradient (client island only where needed) OR render server-side with a gradient underlay behind the img so a failed load reveals the gradient (prefer the underlay — keeps cards server components).

- [ ] Step 1: study the 8 card blocks in `docs/design/Openmind.dc.html` (lines ~117–200) for exact structure/spacing per type.
- [ ] Step 2: implement `ItemCard` switching on `cardType`: article, quote, image, product, note, video, tweet, book, recipe — each per mockup, mapping real fields with graceful fallbacks (missing summary/tags/price/etc. omit cleanly). Domain from `url` (guard empty). Lead image via `assetSrc`; wrap images in a container with the type's gradient as background so a broken/missing image shows the gradient, not a glyph (`img{position:relative;width:100%}` over a gradient div; `alt` differentiated per spec). Pending → cobalt "enriching…" caption.
- [ ] Step 3: `Grid` → `.mind-col` column-count 3 (responsive: 4 at wide, 2 at mid, 1 mobile via container/media). Empty state: italic Newsreader "Nothing gathered yet — drop a link or a thought above."
- [ ] Step 4: build+lint green; commit `feat(web): type-aware editorial cards with graceful image fallback`.

---

### Task 3: Topbar, filter strip, capture/search restyle

**Files:** `apps/web/app/page.tsx`, `apps/web/components/Topbar.tsx`, `apps/web/components/FilterStrip.tsx`, `apps/web/components/QuickAdd.tsx`, `apps/web/components/SearchBox.tsx`, `apps/web/components/ImageDrop.tsx`

**Interfaces:** Topbar (terracotta hairline, title + real count subline, search pill, Save button focusing quick-add, Export link). FilterStrip filters grid by `cardType` via `?type=` (page reads it, filters server-side). Restyled capture/search/upload to the card/paper system.

- [ ] Step 1: `Topbar` per mockup lines ~76–93 (drop Grid/Ledger toggle — Grid only for now; keep Export JSON as a mono link). Subline uses the real item count passed from the page.
- [ ] Step 2: `FilterStrip` chips (All/Articles/Images/Quotes/Products/Video/Notes/Books) → `?type=`; page filters items by cardType (all = no filter); active chip ink-filled.
- [ ] Step 3: restyle QuickAdd (paper input, cobalt Save, cobalt focus ring), SearchBox (Newsreader placeholder "Search a colour, a word, a vibe…", cobalt-outlined), ImageDrop (dashed hairline zone, cobalt on dragover) — all tokens-only.
- [ ] Step 4: build+lint green; commit `feat(web): editorial topbar, type filter, restyled capture & search`.

---

### Task 4: Palette dots (server extraction + render)

**Files (Go):** `openapi.yaml` (Item.palette `[]string`), `apps/api/internal/store/migrations/0003_palette.sql` (`items.palette text[] not null default '{}'`), queries + regen, `apps/api/internal/enrich/palette.go` (+ test), `apps/api/internal/enrich/pipeline.go` (extract when a lead image exists), `apps/api/internal/api/server.go` (map palette in toAPIItem/Detail). **Files (web):** `apps/web/components/Palette.tsx`, use in `ItemCard` + detail; `apps/web/lib/palette.ts` (derived-hash fallback).

**Interfaces:** `enrich.DominantColors(data []byte, n int) ([]string, error)` — decode (jpeg/png/gif via stdlib `image`), downsample, bucket to n hex colours; undecodable → `(nil, nil)`. Pipeline: after an item has a lead image whose bytes are available (uploaded asset on disk, or fetched image-URL), extract up to 5 and store `palette`. Web `Palette` renders `.dot` per hex; when `palette` empty, `derivedPalette(title, tags)` returns 2–3 deterministic hex from a hash (documented placeholder).

- [ ] Step 1 (Go TDD): `palette_test.go` — a solid-red 4×4 png → palette contains a red-ish hex; a 2-colour image → ≥2 distinct; undecodable bytes → nil,nil no error. Implement `DominantColors` (simple: decode, sample pixels, quantise by rounding to a coarse grid, take top-N by count → hex). Migration + sqlc + openapi regen; map palette through handlers. Pipeline: for uploaded images read the stored blob via the asset store; for image-URL cards reuse the fetched bytes if available (else skip). Keep idempotent. Suite+lint green; commit `feat(api): extract dominant colour palette from lead images`.
- [ ] Step 2 (web): `Palette.tsx` + `derivedPalette` fallback; render in cards (the dots row) + detail (swatches). build+lint green; commit `feat(web): palette dots on cards and detail`.

---

### Task 5: Card detail reader restyle

**Files:** `apps/web/app/item/[id]/page.tsx`, `apps/web/app/item/[id]/DeleteButton.tsx`

- [ ] Step 1: restyle the detail page to the reader look (README §7): paper panel, mono meta line, large Newsreader title, serif summary lead, tag chips, Palette swatches, lead image with gradient fallback, "Open original ↗" (cobalt) + Delete (danger, keep confirm). "← library" back link. Pending/failed states styled.
- [ ] Step 2: build+lint green; commit `feat(web): editorial reader detail view`.

---

### Task 6: Visual e2e + docs + wrap-up

- [ ] Step 1: local `docker compose up -d --build api web`; log in; screenshot (via the playwright MCP or note manual) the grid, a couple of card types, capture, and a detail page; confirm it reads as the handoff and no broken-image glyphs. Purge the black 2×2 test upload items (delete via API) so the demo is clean. Stop api/web.
- [ ] Step 2: `docs/self-hosting.md` unaffected; update `TODO.md` (design pass → Done, dated; note remaining M2 design screens: Ledger/Drift/Desk/Lenses/search-overlay). Update `docs/design/README.md`? no — it's the reference. 
- [ ] Step 3: commit `feat(web): design-pass visual verification + todo`. Controller merges, redeploys api+web, re-verifies on the box, tells the user.
