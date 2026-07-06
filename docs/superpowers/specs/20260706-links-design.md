# Bidirectional Linking — Design

Date: 2026-07-06 · Status: Designed autonomously (user asleep; standing "milestones over milestones" directive) · Milestone 4, PRD §9 `links(from_item, to_item)` + §10

## Goal

Connect two saved items ("this article pairs with that note") and see the connection from **both** sides — the commonplace-book habit of weaving threads between captures. Manual links only in v1; AI-suggested links (embedding similarity) are a later layer.

## Data model

Migration `0010_links.sql`:

```sql
CREATE TABLE links (
    user_id    uuid NOT NULL REFERENCES users(id),
    a_item     uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    b_item     uuid NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, a_item, b_item),
    CHECK (a_item < b_item)
);
CREATE INDEX links_b_idx ON links (user_id, b_item);
```

A link is **undirected**: the pair is canonicalised on write (`a_item < b_item` by uuid ordering), so `A→B` and `B→A` are the same row — no duplicate-direction bookkeeping, and the CHECK also forbids self-links. `ON DELETE CASCADE` keeps links consistent when an item is deleted. Both lookup directions are indexed (PK covers `a_item`, secondary covers `b_item`).

## Contract (openapi.yaml — REST, so it IS in the spec, unlike /mcp)

- `GET /items/{id}/links` → `200 [Item]` — the items linked to `{id}` (both directions), newest-linked first. 404 if `{id}` isn't the caller's.
- `POST /items/{id}/links` body `{"toId": "<uuid>"}` → `201 [Item]` (the updated linked list) — canonicalises the pair; linking twice is idempotent (`ON CONFLICT DO NOTHING` → still 201). 400 self-link/bad uuid; 404 if either item isn't the caller's (cross-tenant probes look identical to missing).
- `DELETE /items/{id}/links/{toId}` → `204`; 404 when no such link.
- Regenerate Go server + TS client (`task generate` / `make generate`).

## Store (sqlc, all user-scoped)

`links.sql`: `CreateLink` (insert canonical pair, `ON CONFLICT DO NOTHING`), `DeleteLink` (canonical pair, returns rows), `ListLinkedItems` (join `items` on `a_item`/`b_item` union, ordered by link `created_at DESC`), plus reuse `GetItem` for ownership checks of both endpoints before writing.

## Handlers (`internal/api/links.go`)

- Ownership: resolve both ids via user-scoped `GetItem` before create/delete → cross-tenant/missing = 404 (matches every other endpoint).
- Canonicalise `(min, max)` by `uuid.Compare` (bytes) at the handler layer so the store only ever sees ordered pairs.
- Self-link (`id == toId`) → 400.

## Web (detail page)

The item detail rail (`/item/[id]`) gains a **Linked** section under tags:
- Linked items as compact rows (title-or-host + card-type caption), click → navigate to that item; an × unlinks (DELETE, optimistic remove).
- **+ Link** opens an inline picker: a small search input querying `GET /api/items?limit=20` (filtered client-side by the typed text against title/url) — deliberately simple; no new search infra. Picking an item POSTs the link and refreshes the list. Current item and already-linked items are excluded from picks.
- New cookie-proxy routes: `apps/web/app/api/items/[id]/links/route.ts` (GET, POST) and `.../links/[toId]/route.ts` (DELETE) — same `apiFetch` pass-through pattern as the rest.
- Warm tokens throughout; mono captions; no new dependencies.

## Tests

- Go: DB-backed table-driven tests — create/list from both sides (bidirectionality is THE property: link A→B, assert it appears on B's list too), idempotent re-link, self-link 400, cross-tenant 404, delete + 404-on-absent, cascade on item delete.
- Web: `tsc`/build; e2e via local compose (create two items, link, verify both detail pages show each other, unlink).

## Out of scope

AI-suggested links, link notes/labels, a graph view, MCP link tools, backlink counts on grid cards.

## Execution

Subagent-driven, 3 tasks: (1) migration + contract + store + handlers + Go tests; (2) web rail section + picker + proxy routes; (3) compose e2e + TODO (deploy batched until the VPS recovers — it went unreachable 2026-07-06 late evening).
