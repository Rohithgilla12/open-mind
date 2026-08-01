# Feed river (separate feed items from The Mind) — design

Date: 2026-07-16. Feed-originated items currently mix into the home grid;
the user wants them separate. Approved shape: "option 1 + promotion".

## Semantics (user-confirmed)

- **The Mind (home grid) excludes unkept feed items**: it shows
  `feed_id IS NULL OR kept_at IS NOT NULL`.
- **New "Feed" nav page**: feed items only, reverse-chronological, a calm
  list (not masonry), each row showing its source feed's title; per-feed
  filter chips (client-side from the feeds list).
- **Search includes everything** — feed items get a subtle badge in results.
- **Drift excludes unkept feed items** (same predicate as The Mind): the
  resurfacing ritual covers your saves + kept posts, never the unread
  firehose.
- **Keep (promotion)**: one action on a feed card sets `kept_at = now()`;
  the item then also appears in The Mind (and Drift) while remaining in the
  Feed river with provenance intact. Unkeep clears it. Pin/tags/links/
  Lenses/Kindle keep working on feed items unchanged.
- **Lenses (and Kindle Lens digests) include unkept feed items on all rule
  paths** (user decision) — the digest is the curated slice of the
  firehose. This holds for both rule shapes: text/colour rules already
  see everything because they run through `search.Run`; the types-only
  rule now runs through `ListItemsAll` (no Mind predicate) instead of
  `ListItems`, so it matches.
  > **Superseded 2026-08-01** by search-query-redesign: Lens runs default to
  > `scope: library` (Mind only); pass `scope: all` to include the feed river.

## Schema (migration, next free number)

```sql
ALTER TABLE items ADD COLUMN feed_id uuid REFERENCES feeds(id) ON DELETE SET NULL;
ALTER TABLE items ADD COLUMN kept_at timestamptz;
CREATE INDEX items_feed_idx ON items (user_id, feed_id) WHERE feed_id IS NOT NULL;
```

- `ON DELETE SET NULL` keeps referential integrity when a feed row goes
  away, but a nulled `feed_id` would silently dump that feed's unread items
  into The Mind. So **unsubscribing keeps the feed's items in your
  library deliberately**: the DELETE /feeds handler first runs
  `UPDATE items SET kept_at = now() WHERE user_id = $1 AND feed_id = $2
  AND kept_at IS NULL`, then deletes the feed. Rationale: you subscribed
  because the content was worth keeping; silently hiding it on
  unsubscribe would lose data. Documented in self-hosting.

## Pipeline changes

- `internal/feeds` `saveEntries` passes the feed's id → new
  `CreateFeedItem` query (`INSERT ... (user_id, url, feed_id)`); `Add`'s
  backfill and the poller both use it. File imports, extension, API saves
  are untouched (`feed_id NULL`).
- `ListItems` (home) becomes `WHERE user_id=$1 AND (feed_id IS NULL OR
  kept_at IS NOT NULL)`; new `ListFeedItems` (feed page, reverse-chron,
  optional feed filter param); `ListDriftCandidates` gains the same
  Mind predicate. Export includes everything. `ListItemURLs` (dedup)
  unchanged — a URL saved via feed still dedups a manual save (the manual
  save of an existing feed URL instead sets `kept_at` — capture-is-sacred
  twist: POST /items with a URL that exists as an unkept feed item marks it
  kept and returns it, rather than 200-duplicating or erroring).

## Contract

- `Item`/`ItemDetail` gain nullable `feedId` + `keptAt`.
- `PATCH /items/{id}` gains optional `kept bool` (mirrors `pinned`).
- `GET /feed?limit&cursor?` — no cursor v1: `GET /feed?limit=` (default 50,
  max 200) + optional `feedId` filter param.
- MCP: `list_recent` unchanged (recency is recency); `search_items`
  unchanged; no new tools v1.

## Web

- Sidebar nav gains **Feed** between The Mind and Desk; page lists feed
  items (row: feed title mono kicker, item title, summary line, relative
  time), chips to filter by feed, **Keep** button per row (kept state
  shown); empty state for no subscriptions pointing at /feeds.
- Home grid/search cards: small mono badge (feed title or "via feed") on
  feed-originated items in search results.
- Detail page: kept/feed provenance line in the rail; Keep/Unkeep toggle
  where applicable.

## Prod migration note (operational, not code)

Existing backfilled items (~200, created 2026-07-16 before this ships) have
no provenance. Cleanup at deploy: delete items created on/after the first
feed-subscription timestamp whose URL host matches a subscribed feed's site
host AND that are untouched (no tags/pins/links/highlights), then let the
next poll re-backfill them with `feed_id` set (URL dedup prevents dupes
only for surviving rows, so deletion must precede the poll). Run as a
reviewed SQL script during deploy; the design doc's operator step, not a
migration.

The shipped script (`scripts/one-off/20260716-feedriver-adopt.sql`) instead
**adopts these rows in place** (`UPDATE items SET feed_id = f.id ...`)
rather than delete-and-repoll: it's safer, since it can't lose an item to a
missed re-poll or a feed that's since gone quiet, and it preserves any
tags/pins/links a user already added. Host matching is anchored (host
equality, not substring) to avoid false positives; preview the SELECT
before running the UPDATE.

## Testing

- Store/DB: CreateFeedItem provenance; home query excludes unkept + shows
  kept; drift predicate; unsubscribe keeps items (kept_at stamped);
  save-existing-feed-URL promotes instead of duplicating.
- Feed endpoint: ordering, feedId filter, scoping, caps.
- PATCH kept true/false round-trip; combinable with pinned/userTags.
- Web build; compose e2e: subscribe fixture feed → items land in /feed not
  home → keep one → appears in home → unkeep → gone; drift excludes unkept.
