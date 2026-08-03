# Handoff: Openmind — "The Mind" app UI

## Overview
Openmind is a private, self-hostable "commonplace book, kept by a machine": you save anything (links, images, quotes, notes, products, books, videos), and the system enriches, organises, and resurfaces it. This handoff covers the **core web app UI**: the main collection view (**The Mind**), its **Ledger** alternate view, **Search**, **Drift** (resurfacing), **Desk** (pinboard), plus the **card detail/reader** and **quick-capture** overlays.

## About the Design Files
The files in this bundle are **design references authored in HTML/JS** (a single self-contained prototype). They demonstrate intended look, layout, and behavior — they are **not production code to copy**. The task is to **recreate these designs in Openmind's real environment**: per the PRD that's a **Go API + Postgres/pgvector backend** with a **Next.js (or TanStack Start) web frontend**. Rebuild the UI with the frontend's own component patterns, routing, and data layer; wire it to the real REST/SSE API instead of the prototype's in-memory arrays.

The prototype is built as a "Design Component" (a small runtime wrapper around React). Ignore the wrapper mechanics — read the markup for structure/styling and the logic class for state/behavior.

## Fidelity
**High-fidelity.** Colors, typography, spacing, radii, and interactions are final and intended to be matched closely. Exact values are in **Design Tokens** below. Recreate pixel-closely, then swap the prototype's mock data for real API data.

---

## Design Tokens

### Colour
| Token | Hex | Use |
|---|---|---|
| Canvas (app behind panels) | `#E4DDCD` | outermost background |
| Paper (main surfaces / header) | `#F4F0E6` / `#F1ECE1` | content backgrounds |
| Panel / sidebar | `#EFEADE` / `#EBE5D7` | sidebar, rails, desk bg |
| Card surface | `#FCFBF6` | default card |
| Card surface (bright reader) | `#FBFAF5` | reader/overlay panels |
| Ink (primary text) | `#1C1A16` | headings, body |
| Ink muted | `#57534A` / `#6B655A` | secondary text |
| Ink faint (metadata) | `#A39C8B` / `#8A8578` | mono metadata |
| Hairline / border | `rgba(28,26,22,.10–.13)` | card & panel borders |
| **Cobalt (primary accent)** | `#1B3FD1` | buttons, active states, links |
| Cobalt deep | `#17206b` | gradients, article palette |
| **Terracotta (margin accent)** | `#C24A2E` | ledger margin rule, red hairline |
| Gold (Drift accent) | `#E0B23A` | Drift "keep", gold hairline |
| Green | `#2E7D5B` / `#1f5a41` | storage meter, product palette |
| Note surface | `#FBF4D8` (border `rgba(140,120,40,.25)`, text `#3A3320`, meta `#9A8A3A`) | note cards |

### Typography (Google Fonts)
- **Newsreader** (serif) — titles, quotes, editorial voice. Weights 400/500/600, italics used heavily. `letter-spacing:-.01em to -.02em` on large sizes.
- **Instrument Sans** — UI text, buttons, body. Weights 400/500/600.
- **JetBrains Mono** — ALL metadata, tags, chrome labels. Small (9.5–12px), `letter-spacing:.02–.09em`, often `text-transform:uppercase`.

Type scale (px): screen titles 26–27 (Newsreader 600); card titles 15–17 (Newsreader 600); body/summary 12–13.5 (Instrument Sans); metadata 9.5–10 (JetBrains Mono); tags 10 (JetBrains Mono).

### Radius / Shadow / Spacing
- Radius: cards `11px`, buttons/pills `9–11px`, chips/tags `20px` (pill), overlays `14–16px`, palette dots `50%`.
- Card shadow: `0 1px 2px rgba(28,26,22,.05), 0 10px 26px -20px rgba(28,26,22,.45)`; hover lifts `translateY(-2px)` + deeper shadow, `transition:.18s ease`.
- Overlay shadow: `0 40px 90px -20px rgba(0,0,0,.6)`; backdrop `rgba(20,18,14,.5)` + `backdrop-filter: blur(3px)`.
- Grid: CSS **columns masonry** — `column-count:3` (comfortable) / `4` (dense), `column-gap:16px`, each card `break-inside:avoid; margin-bottom:16px`.
- Content padding: screens `~22–30px` vertical, `26–36px` horizontal.

### Signature detail — the extracted palette
Every card renders a row of small **palette dots** (`9px`, `border-radius:50%`, subtle inset ring) = the item's 3–5 dominant colours, extracted by the enrichment pipeline. Reused as swatch filters in Search and as the visual fingerprint across the app. Do not drop this — it's core to the brand.

---

## Screens / Views

### 1. App shell
- **Layout**: `display:flex; height:100vh`. Fixed **left sidebar 230px** + fluid **main column**.
- **Sidebar** (top→bottom): wordmark ("Openmind", Newsreader 600, 20px, with a 3-line cobalt logo mark) · nav items **Desk / The Mind / Drift** · divider · **LENSES** label + 4 lens rows (coloured dot + name + count) + "New lens" · spacer · **account row** (avatar with initial, "Rohith Gilla / Owner · signed in", chevron) · **storage meter** ("Local · self-hosted", green progress bar at 34%, "3.1 GB / 9 GB archived").
- Nav item: `padding:7px 10px; radius:8px; 13px Instrument Sans 500; color:#57534A`. Active (`.on`): `background:rgba(27,63,209,.1); color:#1B3FD1`. Hover: `background:rgba(28,26,22,.05)`.
- Active nav is driven by current screen (`mind` | `desk`); Drift is an overlay, not a persistent screen.

### 2. The Mind (default screen, Grid view)
- **Purpose**: browse the whole collection.
- **Top**: 2px red hairline (`linear-gradient(90deg,#C24A2E,#C24A2E 40%,transparent)`), then a **topbar** (`flex-wrap:wrap; gap:12px 16px`): title block ("The Mind" 27px Newsreader + mono subline "1,284 gatherings · organised by the machine") · **search pill** (flex 1, min 130 / max 400px, cobalt-focus look, "⌘K" chip) · **Grid/Ledger segmented toggle** · **"Save something"** cobalt button.
- **Filter strip**: pill chips `All / Articles / Images / Quotes / Products / Video / Notes / Books` + right-aligned "Sorted · recently saved ▾". Active chip = ink fill (`#1C1A16` bg, `#F4F0E6` text).
- **Grid**: masonry of **type-aware cards** over a paper background with a faint horizontal-rule texture overlay (`repeating-linear-gradient(... 31px, rgba(28,26,22,.028) ...)`, toggleable).
- **Card types** (each rendered bespoke): **article** (gradient hero + serif title + summary + tags + palette + "Article · 8 min · domain"), **quote** (dark `#1C1A16` card, gold `"` glyph, italic serif, attribution), **image** (tall gradient/photo field + palette + source), **product** (image + price top-right in green + specs), **note** (`#FBF4D8` surface, no hero, serif body), **video** (dark thumb + play glyph + duration), **tweet/post** (avatar + handle + text), **book** (spine mock + title + tags), **recipe** (hero + mono ingredient list), **repo** (article layout — gradient hero + serif title + summary + tags + palette + code-host domain). All carry the palette-dots + mono meta line.

### 3. Ledger view (alternate of The Mind)
- Toggled via the Grid/Ledger control. Same data, chronological "commonplace book" layout.
- **Layout**: centered column `max-width:760px`. A **terracotta vertical margin rule** at `left:104px` (`rgba(194,74,46,.4)`). Italic subtitle "a commonplace book, kept by a machine" (`#C24A2E`).
- **Entries**: `grid-template-columns:64px 1fr; gap:16px`, bottom hairline. Left = stacked date ("03 / JUL", mono, right-aligned). Right = serif title, optional italic excerpt / thumbnail / quote block, palette dots + mono meta line. Quote entries get a gold left border.

### 4. Search (overlay)
- **Trigger**: click search pill **or ⌘K**.
- **Layout**: dim backdrop; centered panel `820px`, `max-height:82vh`, top-anchored (`padding-top:8vh`), `#F4F0E6`, radius 16.
- **Input row**: cobalt-outlined field with focus ring `0 0 0 3px rgba(27,63,209,.08)`; ⌕ icon; **Newsreader input** (placeholder = the NL example query); "esc" chip.
- **Understood-as row** (only when query non-empty): "understood as" + cobalt-tinted chips derived from the query (`type: …`, `colour ≈ …`, `after: …`, `topic ≈ …`), right side "fts + vector · reranked".
- **Body**: left rail 132px (Colour swatch grid + "Try" hints `color:teal / type:recipe / similar:this`); right = "N results" + **2-column masonry of compact result cards** (small hero, serif title, palette dots + meta). Empty state: italic "No gatherings match — try a colour, a type, or a looser word."
- In production, replace the client-side keyword filter with the real **hybrid search** (Postgres FTS + pgvector, reciprocal-rank fusion, optional reranker) and an LLM **query parser** that emits the chips + structured filters.

### 5. Drift (full-screen overlay)
- **Purpose**: calm, finite resurfacing of forgotten saves.
- **Layout**: fixed full-screen, radial dark bg (`#242019→#161410`), centered column. Top: mono "Drift · {n} of {total} today" + progress dots (gold = seen/current, faint = todo). Center: a single **floating card** (`animation: floaty 5.5s ease-in-out infinite`, `@keyframes floaty { 0%,100%{translateY(0)} 50%{translateY(-8px)} }`) with hero, serif title, optional summary, palette + meta. Below: italic "Saved 8 months ago · never revisited". Buttons: **Let go** (outline, light) + **Keep on my desk** (gold fill). Footer hint + ✕ close (top-right).
- **Completion state**: gold ❍ glyph, italic "That's your drift for today.", tally "kept X · let Y go", "Back to The Mind" button.
- Spaced-resurfacing weighting (older + never-revisited up) is a backend concern.

### 6. Desk (screen)
- **Purpose**: "what you're working with right now."
- **Layout**: gold 2px hairline; `#EFEADE` bg; "Your desk" 27px Newsreader + mono subline. **3-column masonry** (`max-width:1000px`) of pinned cards (article/note/quote/image variants; note = yellow surface, quote = dark surface). Footer tip about keeping/clearing.
- **Connection to Drift**: keeping an item in Drift **prepends it to Desk** (dedup by title).

### 7. Card detail / Reader (overlay)
- **Trigger**: click any card (grid or ledger).
- **Layout**: dim backdrop; centered panel `920px × 86vh`, `display:flex`. **Left reader** (scroll, `padding:40px 48px`): mono meta line + ✕; large serif title; serif summary lead; for articles a cobalt pull-quote + body paragraph; action buttons **Open original ↗** (cobalt) / **Send to Kindle** / **Add highlight** (outline). **Right rail 266px** (`#EFEADE`): **Palette** (large swatches), **Tags** (from item + dashed "+ add"), **Similar in your mind** (3 thumbs), "Archived locally · link can't rot · esc to close".
- Content is populated from the clicked item.

### 8. Quick capture (overlay)
- **Trigger**: "Save something" button.
- **Layout**: dim backdrop, top-anchored sheet `480px`, radius 14. Header: ＋ icon + URL (ellipsized) + blinking caret + detected "Article ✓" chip. Body: 66px thumb + serif title + domain + tags with "enriching…" state. Footer: "saves instantly · enriches in place" + "↵ Save".
- Represents the async pipeline: capture returns instantly, card appears, enriches in place.

---

## Interactions & Behavior
- **Grid ⇄ Ledger**: segmented toggle swaps the view; same underlying items.
- **Type filter chips**: filter cards live in both Grid and Ledger (prototype shows/hides by `data-type`; in production filter the query/list).
- **Card click → detail overlay**; **search pill / ⌘K → search overlay**; **Save → capture overlay**; **sidebar Drift → Drift overlay**; **Desk / The Mind → switch screen**.
- **Esc** closes any overlay (detail / capture / search / drift). Backdrop click closes; inner panel click is stopped from propagating.
- **Search**: typing updates results + understood-as chips live; input is **controlled** (must reflect state or keystrokes are lost); empty query shows all, no-match shows empty state.
- **Drift**: Keep/Let go advance the index and update the tally; Keep also pushes to Desk; finishing shows the completion state.
- **Hover**: cards lift (`translateY(-2px)` + deeper shadow, `.18s`); nav items tint.
- Animation: `floaty` keyframe for the Drift card and the search caret.

## State Management
Prototype state (map to your store/route + server data):
- `screen`: `'mind' | 'desk'` — top-level route.
- `view`: `'grid' | 'ledger'` — Mind sub-view.
- `filter`: type filter (`'all' | 'article' | 'image' | …`).
- `open`: currently-open item for the detail overlay (or null).
- `capture`: boolean — capture overlay.
- `search`: boolean — search overlay; `query`: string (controlled input).
- `drift`: boolean; `driftIndex`, `driftKept`, `driftLet`: Drift progress; `driftItems`: the queue.
- `desk`: pinned items (seed + Drift-kept, dedup by title).
- Tweakable props: `defaultView` (grid/ledger), `density` (comfortable/dense → column-count), `warmPaper` (paper texture on/off).

Data fetching (production): collection list (paginated/virtualised), item detail, hybrid search endpoint (+ NL parse), Drift queue (spaced-resurfacing), Desk pins, capture POST → enrichment job. Use SSE for "enriches in place" updates.

## Assets
No external image assets — all imagery is CSS gradients/placeholders standing in for real thumbnails, hero images, extracted colours, book covers, and avatars. Replace with real archived screenshots/images and pipeline-extracted palettes. Icons are Unicode glyphs (◵ ◧ ❍ ⌕ ＋ ✕ ☰); swap for your icon set. Fonts: Newsreader, Instrument Sans, JetBrains Mono (Google Fonts).

## Files
- `Openmind.dc.html` — the committed app prototype (all screens + overlays + logic). Primary reference.
- `Openmind — The Mind.dc.html` — earlier exploration canvas: 3 card/grid directions (1a editorial, 1b gallery-dense, 1c commonplace ledger) + static versions of all seven screens. Useful for alternate treatments.
