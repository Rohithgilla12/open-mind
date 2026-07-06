# Desk — Pinboard — Design

Date: 2026-07-06 · Status: Designed autonomously (user: "do next milestones") · Builds the designed-but-unbuilt **Desk** screen (docs/design/README.md §6); pairs with Drift next (Drift "keep" → Desk)

## Goal

Let a person pin items to a **Desk** — "what you're working with right now" — a small curated board separate from the full library. Pin/unpin from any card or the detail view; the Desk screen shows pinned items newest-first. Signature commonplace-book feature, fully designed, self-contained, no new dependency.

## Data model

Migration `0007_desk.sql`: `ALTER TABLE items ADD COLUMN pinned_at timestamptz;` (null = not on desk). Index `CREATE INDEX items_user_pinned_idx ON items (user_id, pinned_at DESC) WHERE pinned_at IS NOT NULL;` (partial — Desk queries only pinned rows). Additive, no rewrite of the generated column (unlike 0006).

## API

- Extend the existing `PATCH /items/{id}` (currently `{userTags}`) with an optional `pinned: bool`: `true` → `pinned_at = now()` (idempotent — re-pin keeps the original? no, set to now() so re-pin re-orders; fine), `false` → `pinned_at = NULL`. Both `userTags` and `pinned` optional; at least one required (else 400, as today). Returns updated `ItemDetail`.
- `Item`/`ItemDetail` schema gains `pinnedAt: string|null` (date-time) so cards/detail can show pin state.
- `GET /desk` → `Item[]` — the user's pinned items ordered `pinned_at DESC`. Bearer + rate-limited (guarded).
- sqlc: `SetItemPinned(user_id, id, pinned_at) :execrows` (nullable timestamptz param), `ListPinned(user_id) :many`.

## Web

- **`/desk` page**: gold 2px top hairline (per design), "Your desk" (Newsreader) + mono subline (e.g. "N pinned · what you're working with"), the pinned items in the same masonry Grid + type-aware cards. Empty state teaches ("Pin anything to keep it close — it'll wait here while the rest of your mind stays out of the way."). 
- **Pin affordance**: a small pin toggle on the detail page (and optionally a hover pin on cards — keep to detail + a card corner button to limit scope). Pinning → PATCH `{pinned:true}` → refresh; unpin → `{pinned:false}`. On the Desk page, the toggle reads "unpin". Client component, cookie→bearer via the existing `/api/items/[id]` PATCH proxy.
- **Sidebar**: the "Desk" nav item (currently muted "soon") becomes a live link to `/desk`, active-state aware. Drift stays "soon" (built next).
- Cards may show a subtle pin indicator when `pinnedAt` is set (small gold dot/pin glyph) — optional, keep tasteful.

## Testing

- Go: migration applies (pinned_at column + partial index); `SetItemPinned` sets/clears + user-scoped (cross-tenant → 0 rows → 404); `ListPinned` returns only pinned, newest-first, user-scoped; PATCH `{pinned:true/false}` handler (200, updates pinnedAt; combined with userTags in one PATCH works; neither field → 400); `GET /desk` handler (only pinned, user-scoped, empty → `[]`). toAPIItem/Detail emit `pinnedAt`.
- Web: build + lint; e2e on box (pin an item → appears on /desk; unpin → gone; Desk nav works).

## Out of scope

Drag-to-reorder the desk (order is pin-time), desk sections/columns beyond masonry, desk sharing, a pin limit. Drift integration (Drift's "keep" calling pin) lands with the Drift milestone.

## Execution

Subagent-driven. Reuse the PATCH /items infra (from user-tags), the Grid/ItemCard, and the feeds/lenses page patterns. Deploy api+web after whole-branch review; restart cloudflared (web recreate).
