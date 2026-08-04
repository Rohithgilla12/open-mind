# Infinite scrolling on web and mobile — design

**Date:** 2026-08-04
**Status:** approved, not yet implemented

## Problem

Every reverse-chron list in the product stops at 50 items with no way to reach the 51st.
`GET /items` and `GET /feed` take a `limit` (default 50, ceiling 200) and return a bare JSON array;
there is no cursor, no offset, and no total. A library larger than 50 saves is silently truncated on
the web Mind, the web Feed river, the mobile Library tab, and the mobile Feed tab. Nothing in the UI
says so.

Raising `limit` is not a fix: it moves the cliff and makes first paint worse.

## Decision

Cursor (keyset) pagination on `/items` and `/feed`, surfaced as infinite scroll on the four
reverse-chron surfaces. The response body becomes an envelope carrying an opaque `nextCursor`.

### Scope

Paginated:

| Surface | File |
| --- | --- |
| Web Mind (unsearched only) | `apps/web/app/page.tsx` |
| Web Feed river | `apps/web/components/FeedRiver.tsx` |
| Mobile Library (unsearched, unfiltered only) | `apps/mobile/app/(tabs)/index.tsx` |
| Mobile Feed | `apps/mobile/app/(tabs)/feed.tsx` |

Deliberately **not** paginated:

- **Search** (`/search`) and **Lens detail**, which share the same path. RRF fuses two independent
  rankings and hard-caps at 50 (`internal/search/search.go:194`, `ruleResultLimit = 50`), so page 2
  is not "the next 50 by score" — the fused ranking would have to be re-materialised or cached per
  query. That is its own spec. Fifty best matches is also a defensible answer to a search.
- **`/desk`**, which has neither a `limit` nor a cursor today and returns every pin. Pins are
  hand-curated and unlikely to pass a few hundred rows.
- **`/places`**, a map rather than a list.

## The contract

`openapi.yaml` first, then `task generate`, then handlers — the usual order.

A new schema, shared by both endpoints:

```yaml
ItemPage:
  type: object
  required: [items]
  properties:
    items: { type: array, items: { $ref: "#/components/schemas/Item" } }
    nextCursor:
      type: string
      description: "Opaque. Pass back as ?cursor= for the next page. Absent when there are no more items."
```

`GET /items` gains `cursor`; `GET /feed` gains `cursor` alongside its existing `limit` and `feedId`.
Both 200 bodies become `ItemPage`. `limit` keeps the current 50 default and 200 ceiling
(`internal/api/server.go:35`).

### Why an envelope and not a header

`/search` and `/drift` already return envelopes, so `ItemPage` is the house shape rather than a new
idea. A cursor in an `X-Next-Cursor` header would have been non-breaking, but it is invisible in the
generated client and forces every proxy route to forward it explicitly.

The cost is accepted knowingly: the body shape changes, and two in-repo clients parse the old shape
in a way that fails silently. Both are fixed in this change (see *Other clients*). Neither the
extension nor the dock is published or distributed yet, so there is no build in the wild that this
strands — and mobile already tolerates both shapes.

### Knock-on

`apps/web/lib/types.ts:3` derives `Item` from the `/items` 200 body:

```ts
export type Item = paths["/items"]["get"]["responses"]["200"]["content"]["application/json"][number];
//                                                                                        ^ becomes ["items"][number]
```

This fails `tsc` after regeneration rather than degrading quietly, which is the desired failure mode.

## Keyset, not offset

`items.created_at` is not unique, so the sort key becomes `(created_at DESC, id DESC)` and the cursor
is a row-value seek. `ListItems` and `ListFeedItems` change in place — callers that do not paginate
(`internal/api/mcp.go:73`, tests) pass a nil cursor — rather than growing parallel queries.

```sql
-- name: ListItems :many
SELECT * FROM items
WHERE user_id = $1
  AND (feed_id IS NULL OR kept_at IS NOT NULL)
  AND (
    sqlc.narg(cursor_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);
```

**Why keyset and not `OFFSET`.** Captures land at the head of this list constantly. A save arriving
between page 1 and page 2 shifts every offset by one, so `OFFSET 50` re-serves an item the client
already has. A row-value seek is anchored to a row rather than to a count, so it is immune. A delete
between pages can skip one row, which is harmless and self-correcting on refresh.

### Cursor encoding

Opaque to clients: base64url of `<RFC3339Nano created_at>|<uuid>`, built from the last row actually
returned so the value round-trips exactly (Postgres stores microseconds; formatting what was read
back avoids any precision mismatch).

A cursor that fails to decode is a **400 `invalid cursor`**, never a silent fall back to page 1 — a
client sending a stale or corrupt cursor should learn about it, not receive the top of the list and
believe it paged.

Helpers live in a new `internal/api/cursor.go` with table tests for round-trip, malformed input, and
the empty case.

### Precise `nextCursor`

Handlers fetch `limit + 1` rows and trim to `limit`. `nextCursor` is emitted only when the extra row
existed, so the client never makes a final request that returns nothing.

### Index

New migration:

```sql
CREATE INDEX items_user_created_id_idx ON items (user_id, created_at DESC, id DESC);
DROP INDEX items_user_created_idx;
```

The dropped index (`0001_init.sql:28`, `(user_id, created_at DESC)`) is a strict prefix of the new
one, so every query it served — including `ListItemsAll` and search — is still served. Keeping both
would pay write cost twice for nothing.

## Web

### The Mind (`/`)

Page 1 continues to render on the server, so first paint is unchanged. It passes `initialItems` and
`initialCursor` into a new client component `ItemRiver`, which holds `pages: Item[][]` and renders
**one `<Grid>` per page**.

While searching, the existing `<Grid>` path renders directly with no pagination — search is not
paginated, so there is no cursor to follow.

#### Why one block per page

`.mind-col` is a CSS `column-count` masonry (`apps/web/app/globals.css:68`). Appending into a single
container makes the browser rebalance every column, so cards the reader has already passed move.
Measured on a 12-card page at 1240px: **8 of 12 already-visible cards changed position.**

Rendering each page as its own multi-column block seals page 1 the moment page 2 arrives — measured
**0 of 12 moved**. It needs no CSS change and no change to `Grid`, so `/lens` is untouched.

The cost is a seam: column bottoms do not line up across the page boundary, by usually about one
card's height, because each block balances internally. On a page called the Mind, in a product whose
identity is print, that reads as a spread break rather than as breakage.

A JS-distributed fixed-column masonry also measures 0 of 12 and has no seam, but it flips reading
order from column-major to row-major, moves the column count out of CSS media queries into a JS
resize listener (two sources of truth), makes uneven column bottoms permanent rather than per-page,
and rewrites the Mind's core layout. Rejected as disproportionate; revisit only if the seam proves
annoying against a real 50-card page.

### The masthead count

`Topbar.tsx:57` renders `{count} gatherings · organised by the machine`, where `count` is however many
rows page 1 returned. Today that is silently capped at 50; under pagination it would either stay
frozen at 50 while the river grows beneath it, or tick upward as the reader scrolls. Both are worse
than the status quo, because both assert a library size that is not one.

So when a `nextCursor` exists, the subline renders `50+ gatherings`. It stays server-rendered and does
not track client-side appends — an unknown-but-larger total stated honestly, which is the same call
made for mobile's grouped headers below. A true total would need `total` on `ItemPage` (a `COUNT(*)`
per request) or a count endpoint; that stays a follow-up.

### Load control

A real `<button>` ("Load more saves") is rendered whenever a cursor exists. An IntersectionObserver
on a sentinel calls the same loader. Auto-load is the enhancement; the button is the control —
infinite scroll whose only trigger is a scroll event is unreachable by keyboard. Appends are
announced via `aria-live="polite"`.

Load failures follow the house pattern already used by `FeedRiver` and `RelatedRail`: an inline
retry, with the already-loaded pages left on screen.

### The Feed river

Same treatment; `FeedRiver` already owns client state. Two specifics: pages reset when
`activeFeedId` changes, and `setKept` maps across pages rather than one flat array.

Neither proxy route changes — `apps/web/app/api/items/route.ts` and `.../feed/route.ts` already
forward the query string verbatim, so `?cursor=` arrives without help.

## Mobile

`listItems` and `listFeedItems` (`apps/mobile/lib/api.ts`) return
`{ ok, status, items, nextCursor }`. They parse the envelope **and keep the existing bare-array
tolerance**: against a self-hosted instance predating this change there is no `nextCursor`, so
pagination stops after page 1 and the app stays useful. That graceful degradation is the reason the
tolerance is kept rather than cleaned up.

Library and Feed move to `useInfiniteQuery` with
`getNextPageParam: (last) => last.nextCursor ?? undefined`, `onEndReached`, and a footer
`ActivityIndicator` while `isFetchingNextPage`. Items become `data.pages.flatMap((p) => p.items)`.

Search and colour-filter branches return a single cursor-less page, so they stay unpaginated through
the same hook — no conditional hook, no second code path.

Mobile's Feed has no per-feed filter (that is web-only), so there is no filter-change reset to handle
on this side.

### Refresh must not fan out

TanStack Query is v5.101.2, where `refetchPage` no longer exists: `refetch()` on an infinite query
re-requests every loaded page sequentially. Ten loaded pages would mean ten requests on one
pull-to-refresh.

Pull-to-refresh and `useSoftFocusRefetch` therefore trim the cache to the first page, then refetch:

```ts
queryClient.setQueryData(key, (d) =>
  d && { pages: d.pages.slice(0, 1), pageParams: d.pageParams.slice(0, 1) },
);
```

One request, no spinner flash (data is never cleared), and scrolling back down re-pages naturally.

### Mutation caches must learn the new shape

`apps/mobile/lib/mutations.ts` currently assumes both list caches are flat:

```ts
qc.setQueriesData<Item[]>({ queryKey: ["feed"] }, apply);              // :19  expects Item[]
qc.setQueriesData<LibraryData>({ queryKey: ["items"] }, (prev) => …);  // :28  expects { items }
```

Under `useInfiniteQuery` both caches become `{ pages, pageParams }`. Left alone, the feed patch calls
`.map` on an object and **throws on every pin, keep, and delete**, while the items patch writes
`{ ...prev, items: undefined }` and corrupts the cache silently. So `patchItemInCaches` gains a helper
that maps across `pages` for these two keys.

`["search"]` and `["desk"]` are untouched: search and the colour filter stay single-page `useQuery`,
and the Desk is out of scope. The existing key split (`["items", limit]` and `["search", q]` are
separate roots) is what keeps this containable.

`useInvalidateLists` (`:40`) has the same fan-out problem as refresh — `invalidateQueries` refetches
every loaded page of an active infinite query, so one pin with ten pages loaded means ten requests. It
applies the same trim-to-first-page step before invalidating the two infinite keys, which makes the
rule uniform across pull-to-refresh, focus refetch, and mutation invalidation: **trim, then refetch.**
The optimistic patch above is what keeps the visible list correct in the meantime.

### Group-by-kind counts

Library's grouped mode renders headers like `Articles · 12` (`typeLabelPlural`). With pages loaded
that number describes what has been fetched, not what exists — a false claim about the library. While
`hasNextPage` is true the header shows the label alone and drops the count.

## Other clients

Two consumers parse the old shape in a way that turns an unrecognised body into a healthy-looking
empty list:

- `apps/extension/lib/save.ts:118`
- `apps/dock/src/lib/api.ts:255`

Both do `Array.isArray(data) ? data : []` while returning `ok: true`. Both learn the envelope, and
both start returning `ok: false` for a body they cannot recognise — a pre-existing silent failure
that the envelope would otherwise detonate. Neither needs pagination: the extension shows a handful
of recents, the dock shows 8.

`internal/api/mcp.go:73` passes a nil cursor; its behaviour is unchanged.

## Tests

Go, against real Postgres per the repo rule:

- Page through a full set: **no duplicates, no gaps**, every row seen exactly once.
- `nextCursor` absent on the final page, and no trailing empty request.
- **Two items sharing a `created_at` straddling a page boundary** — the `id` tiebreaker.
- **A save inserted at the head between page 1 and page 2 does not re-serve a row** — the property
  keyset exists to buy.
- A malformed cursor returns 400.
- Cross-tenant scoping still holds with a cursor supplied.
- Cursor encode/decode round-trip, malformed, and empty.

Web vitest runs node-only over `lib/`, so the page-merge logic is extracted as a pure reducer and
tested there rather than by mounting `ItemRiver`.

Mobile jest covers envelope parsing, the bare-array fallback, and — importantly — `patchItemInCaches`
against a **multi-page** cache: patching an item that lives on page 3 must reach it, and must not
throw or blank the cache. That is the regression the current flat-shape assumption would have shipped.

## Documentation

`/architecture` needs no update — no new pipeline stage, client, or core dependency — so its
`LAST_UPDATED` stays put. `TODO.md` gains a Done entry plus the follow-ups below.

## Follow-ups (for TODO.md Later)

- Search and Lens pagination, deliberately out of scope here; needs a fused-ranking cursor.
- `/desk` pagination if a Desk ever grows past a few hundred pins.
- Revisit the Mind's masonry (JS fixed columns) only if the per-page seam proves annoying against a
  real 50-card page on live data.
- Grouped Library headers lose their counts while more pages remain; a real total would need a count
  endpoint or a `total` on `ItemPage`.
