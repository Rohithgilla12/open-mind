# User Tags & Tag Provenance — Design

Date: 2026-07-06 · Status: Designed autonomously (user: "next, let's go"; auth deferred) · Closes the flagged "Import tag preservation needs a tag source distinction" blocker + adds manual curation

## Goal

Let a person tag their own saves and have those tags **survive re-enrichment**, and preserve tags carried in imported bookmark/CSV files. Turns the library from an AI-only tag dump into something curatable. No new dependency, no user decision.

## Model — provenance via a second column (not a join table)

Keep `items.tags text[]` = **AI tags** (overwritten each enrichment, as today). Add `items.user_tags text[] NOT NULL DEFAULT '{}'` = **user/imported tags** (never touched by enrichment). This is the minimal provenance split the PRD calls for; a normalized `item_tags(source)` table is heavier than warranted at this scale and would ripple through search/cards/import. The displayed/searchable tag set is the **deduped union** of the two (user tags win on case/display).

- Migration `0006_user_tags.sql`: add `user_tags`; **rebuild the `search_tsv` generated column** to also index `user_tags` (drop + re-add the STORED generated column with `… || setweight(array_to_tsvector(user_tags), 'B')`, and drop + recreate the `items_search_tsv_idx` gin index). Additive, no data loss.
- Enrichment (`UpdateItemEnrichment`) writes only `tags` — already true, so user tags are inherently preserved across re-runs. Verify no path clobbers `user_tags`.

## API

- `Item`/`ItemDetail` schema gains `userTags: []string` (alongside `tags`). Regenerate.
- `PATCH /items/{id}` with body `{ userTags: []string }` — sets the full user-tags list (canonicalised: trim, drop empties, lowercase, dedupe, cap 30, each ≤50 chars). Returns the updated `ItemDetail`. User-scoped (404 cross-tenant/missing). This is a general item-edit endpoint (userTags-only for now; title/other edits can extend it later). Bearer + rate-limited.
- sqlc `SetUserTags(user_id, id, user_tags) :execrows` (0 rows → 404).

## Import tag preservation

Extend `internal/importer` to capture tags where the format carries them, and the `/import` handler to set them as `user_tags` on each created item:
- **Netscape bookmark HTML**: `<A HREF=… TAGS="a,b,c">` (Pocket/Raindrop/Firefox use `TAGS`) → split on comma.
- **CSV**: a `tags` column (Pocket uses `tags`, Raindrop `tags` space-or-comma separated) → detect by header name, split.
- Plain URL lists: no tags.
- `ImportResult` unchanged; the parser's per-entry shape gains optional `tags []string`; the handler passes them to a `CreateItem`-then-`SetUserTags` (or a create variant) for new items. Imported tags are canonicalised like manual tags.

## Web

- **Detail page** (`/item/[id]`): a tags section showing AI tags (existing style) and **editable user tags** — each user tag a chip with an × to remove; an "+ add tag" input. Add/remove → `PATCH /api/items/[id]` `{userTags}` (client) → refresh. AI vs user tags visually distinguished (e.g. user tags get a subtle cobalt/user affordance; AI tags plain). Tokens-only.
- **Cards**: the tags row renders the union (deduped) — no visual change needed beyond including user tags.
- `/api/items/[id]` gains a PATCH proxy (cookie→bearer via apiFetch).

## Testing

- Go: migration applies (user_tags present, search_tsv rebuilt, gin index exists); `SetUserTags` canonicalises + is user-scoped (cross-tenant → 404); a full enrichment run does NOT clear pre-set user_tags (provenance test — set user_tags, run pipeline, assert user_tags intact while ai tags updated); PATCH handler (canonicalisation, 404, cross-tenant); importer parses TAGS from bookmark HTML + a CSV tags column (unit); import handler sets user_tags on created items (DB test). Search: an item findable by a user_tag term (search_tsv includes user_tags).
- Web: build + lint; e2e on box (add/remove a tag on a detail page → persists; import a tagged bookmark file → item carries the tag).

## Out of scope

Editing title/summary/cardType (endpoint is extensible but this slice is tags-only); tag autocomplete/suggestions; tag rename across items; a tags browse/index page; per-tag Lens shortcuts (Lenses already cover saved tag queries).

## Execution

Subagent-driven. Reuse `internal/importer` + the item/detail web patterns. The `search_tsv` rebuild migration is the one delicate step — test it applies cleanly on a populated DB. Deploy api+web after whole-branch review; restart cloudflared (web recreate → new IP).
