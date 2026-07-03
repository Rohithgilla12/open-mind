# Card Detail + Reader View + Export + Delete — Design

Date: 2026-07-03 · Status: Designed autonomously (user authorised overnight run); review async · Follows wxt-extension spec

## Goal

Clicking a card opens the saved item in a readable, type-aware detail view; items can be deleted; the whole library exports as JSON. Closes the "Card detail + reader view" and "JSON export" backlog items.

## Contract additions (`openapi.yaml` → regenerate)

- `GET /items/{id}` → `ItemDetail` (= `Item` + required `body: string`); 404 JSON when not found (user-scoped).
- `DELETE /items/{id}` → 204; 404 when not found. Deleting cascades embeddings (FK already ON DELETE CASCADE).
- `GET /export` → streamed JSON array of `ItemDetail` for the authenticated user, `Content-Disposition: attachment; filename="openmind-export-YYYYMMDD.json"` set by the WEB route (API returns plain JSON array).
- Bearer auth + rate limiting apply as-is (limiter guard extends to `GET /export` — it's a heavy endpoint).

## API implementation

- sqlc: `DeleteItem(user_id, id) :execrows`, `ListItemsForExport(user_id) :many` (no limit, ordered created_at ASC). `GetItem` exists.
- Handlers: `GetItem` (404 on pgx.ErrNoRows), `DeleteItem` (404 when 0 rows), `ExportItems` (encode array; buffered encode acceptable at this scale — no streaming complexity, YAGNI).
- Tests: handler tests for 404 cross-tenant (other user's item id → 404), delete-then-get 404, export contains body.

## Web

- Cards link to `/item/[id]` (whole card wrapped in `next/link`; QuickAdd/SearchBox unaffected).
- `/item/[id]` server component via `apiFetch`: type-aware detail —
  - article/product/recipe/book: reader view — lead image (when present), title, domain + "open original" link, summary block (tokens.cobalt accent), tag chips, body text in a readable column (~65ch, `tokens.font.sans`, generous line-height). Body paragraphs split on blank lines.
  - note: full note text, Newsreader italic, quote-card styling.
  - image: large image, title, source link.
  - tweet/video: body/summary as quote + prominent source link (video: lead image thumb above).
  - pending: "still enriching" banner; failed: muted error note + original URL link.
- Delete: client component button (top-right, danger token) → `DELETE /api/items/[id]` proxy route → `router.push("/")`. Confirm via native `confirm()` (YAGNI).
- Export: link in a small header/footer bar on `/` → `/api/export` proxy route setting the attachment filename (YYYYMMDD per date conventions).
- New web proxy routes pass `req` through (Bearer OR cookie) — extension/API-token callers get them too.

## Out of scope

Editing items, bulk operations, import, keyboard nav, Drift/Lens/Desk (M2), reader typography settings.

## Testing

Go handler tests as above (real Postgres); web build + lint; deploy-box e2e: open detail of an enriched article (curl HTML 200 contains title), delete an item (204 → list shrinks), export downloads valid JSON with bodies.
