# Web Design Pass — Design

Date: 2026-07-04 · Status: Approved (user: design pass first, docs/design palette canonical) · Reference authority: `docs/design/README.md` + `docs/design/Openmind.dc.html` (pixel-close), `.impeccable.md` (context)

## Goal

Make the web app look like the `docs/design` handoff — a warm, editorial "commonplace book kept by a machine" — instead of the current plain functional grid. Same MVP feature set (grid + capture + search + detail + export); this is a visual/craft overhaul, not new product surface.

## Canonical palette decision (locked)

Adopt the **`docs/design/README.md` warm palette** as canonical; it supersedes CLAUDE.md's cooler set. Update `packages/ui/src/tokens.ts` and the CLAUDE.md design-tokens line to match. Values: canvas `#E4DDCD`, paper `#F4F0E6` (+ header `#F1ECE1`), panel `#EBE5D7`, card `#FCFBF6`, ink `#1C1A16`, ink-muted `#57534A`/`#6B655A`, ink-faint `#A39C8B`/`#8A8578`, hairline `rgba(28,26,22,.10–.13)`, cobalt `#1B3FD1` (+ deep `#17206b`), terracotta `#C24A2E`, gold `#E0B23A`, green `#2E7D5B`/`#1f5a41`, note surface `#FBF4D8` (border `rgba(140,120,40,.25)`, text `#3A3320`, meta `#9A8A3A`). Fonts: **Newsreader** (serif titles/quotes, italics), **Instrument Sans** (UI/body), **JetBrains Mono** (all metadata/tags, uppercase, letter-spaced) — via `next/font/google`.

## In scope

1. **Design foundation** — tokens swap; global CSS helpers from the mockup (`.serif`, `.meta`, `.tag`, `.dot`, `.card` w/ shadow + `translateY(-2px)` hover, `.chip`, `.savebtn`); warm paper background `#F4F0E6` with the faint horizontal-rule texture overlay; three Google fonts loaded via next/font.
2. **App shell** — left **230px sidebar** (`#EBE5D7`): wordmark (cobalt 3-line logo mark + "Openmind" Newsreader 600) · nav with **The Mind** active; **Desk / Drift / Lenses** rendered but visibly muted with a "soon" affordance (M2 — no dead clicks) · account row (avatar initial, "signed in") · storage meter (green bar). Main column = fluid.
3. **Topbar** — 2px terracotta hairline; "The Mind" title (Newsreader 27) + mono subline with the **real** item count; search pill (focuses/routes to search) with ⌘K chip; **Save something** button (focuses the quick-add); Export JSON link. (Grid/Ledger toggle: render Grid only; Ledger deferred to M2.)
4. **Filter strip** — type chips `All / Articles / Images / Quotes / Products / Video / Notes / Books` filtering the grid by `cardType` (client-side or `?type=`); active chip = ink fill.
5. **Type-aware cards** — replace the one-size white card with the mockup's bespoke treatments per `cardType`: **article** (lead-image hero, or cobalt→deep gradient fallback when no image, serif title, summary, tags, palette dots, "Article · domain"), **quote** (dark `#1C1A16` card, gold `"`, italic serif, attribution), **image** (the actual image field + palette + source), **product** (image + green price + specs), **note** (`#FBF4D8` surface, serif body, no hero), **video** (dark thumb + play glyph, or lead-image thumb, + title), **tweet** (avatar + handle + text), **book** (spine mock + title + tags), **recipe** (hero + mono lines). All carry the mono meta line. Domain derived from `url` (guard empty for notes/uploads). **Broken/missing images fall back to the type's gradient/placeholder — never a broken-image glyph.**
6. **Palette dots (signature — do not drop)** — server extracts 3–5 dominant colours from a lead image at enrichment (jpeg/png/gif via stdlib `image` decode; store `palette []string` on the item; add to openapi/schema). Cards render dots from `palette`; cards with no extractable palette (text notes, undecodable/external images) render a small **derived** palette from a deterministic hash of title+tags (documented placeholder until fuller extraction). Never render zero dots.
7. **Card detail** — restyle `/item/[id]` to the reader look (or an overlay): mono meta line, large Newsreader title, serif summary lead, tag chips, palette swatches, "Open original ↗" (cobalt), delete (danger). Broken-image fallback here too.
8. **Capture / search / upload** — restyle the quick-add bar, image drop zone, and search box to the paper/card system (cobalt-focus rings, Newsreader placeholder on search). Pending items show an "enriching…" state in cobalt.

## Out of scope (M2 / later)

Ledger view, Drift, Desk, Lenses (functional), search overlay with understood-as chips + colour swatches (keep the current inline search, just restyled), ⌘K palette, SSE live-enrich updates, the paper-texture toggle, avatar uploads.

## Testing

- Web build + lint green; visual verification on the deploy box (screenshot the grid, a detail page, capture, and each present card type) — the bar is "reads as the docs/design handoff, not a generic grid."
- Go: palette-extraction unit test (a known small image → deterministic non-empty palette; undecodable input → empty, no error); pipeline stays idempotent; existing suite green.
- Broken-image fallback verified (a card whose leadImageUrl 404s shows the gradient, not a broken glyph).

## Execution

Subagent-driven; frontend work implemented against the mockup file (`docs/design/Openmind.dc.html`) for exact CSS/structure. Deploy web (+ api for the palette field) after the whole-branch review.
