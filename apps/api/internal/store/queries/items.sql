-- name: EnsureUser :exec
INSERT INTO users (id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: CreateItem :one
INSERT INTO items (user_id, url, body) VALUES ($1, $2, $3) RETURNING *;

-- name: GetItem :one
SELECT * FROM items WHERE user_id = $1 AND id = $2;

-- name: ListItems :many
-- Keyset (not OFFSET) pagination: the seek is anchored to a row, so a capture
-- landing at the head between two requests cannot shift a window and re-serve
-- a row the client already holds. id breaks created_at ties.
SELECT * FROM items
WHERE user_id = $1 AND (feed_id IS NULL OR kept_at IS NOT NULL)
  AND (
    sqlc.narg(cursor_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListItemsAll :many
-- Same as ListItems but without the Mind predicate: serves the types-only
-- Lens rule path, which deliberately spans the feed river so Lenses (and
-- Kindle Lens digests) include unkept feed items, matching the text/colour
-- rule path (which runs through search and already sees everything).
SELECT * FROM items
WHERE user_id = $1
ORDER BY created_at DESC LIMIT $2;

-- name: CreateFeedItem :one
INSERT INTO items (user_id, url, feed_id) VALUES ($1, $2, $3) RETURNING *;

-- name: ListFeedItems :many
SELECT * FROM items
WHERE user_id = $1 AND feed_id IS NOT NULL
  AND (sqlc.narg(filter_feed_id)::uuid IS NULL OR feed_id = sqlc.narg(filter_feed_id))
  AND (
    sqlc.narg(cursor_created_at)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: SetItemKept :execrows
UPDATE items SET kept_at = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: GetItemByURL :one
SELECT * FROM items WHERE user_id = $1 AND url = $2 LIMIT 1;

-- name: DeleteItem :execrows
DELETE FROM items WHERE user_id = $1 AND id = $2;

-- name: ListItemsForExport :many
SELECT * FROM items WHERE user_id = $1 ORDER BY created_at ASC;

-- name: ListItemURLs :many
SELECT url FROM items WHERE user_id = $1 AND url <> '';

-- name: AdoptFeedItems :execrows
UPDATE items SET feed_id = $3 WHERE user_id = $1 AND id = ANY($2::uuid[]) AND feed_id IS NULL;

-- name: UpdateItemExtraction :exec
UPDATE items SET title = $3, body = $4, lead_image_url = $5, card_type = $6, tagged_location = $7, updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: UpdateItemEnrichment :exec
UPDATE items SET summary = $3, tags = $4, updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: SetUserTags :execrows
UPDATE items SET user_tags = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: SetItemPinned :execrows
UPDATE items SET pinned_at = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: ListPinned :many
SELECT * FROM items WHERE user_id = $1 AND pinned_at IS NOT NULL ORDER BY pinned_at DESC;

-- name: ListDriftCandidates :many
SELECT * FROM items
WHERE user_id = $1 AND status = 'enriched' AND pinned_at IS NULL
  AND (last_drifted_at IS NULL OR last_drifted_at < now() - interval '30 days')
  AND (feed_id IS NULL OR kept_at IS NOT NULL)
ORDER BY last_drifted_at NULLS FIRST, created_at ASC
LIMIT $2;

-- name: CountDriftCandidates :one
SELECT count(*) FROM items
WHERE user_id = $1 AND status = 'enriched' AND pinned_at IS NULL
  AND (last_drifted_at IS NULL OR last_drifted_at < now() - interval '30 days')
  AND (feed_id IS NULL OR kept_at IS NOT NULL);

-- name: DriftAction :execrows
UPDATE items
SET last_drifted_at = now(),
    pinned_at = CASE WHEN sqlc.arg(keep)::boolean THEN now() ELSE pinned_at END,
    updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: SetItemStatus :exec
UPDATE items SET status = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: SetItemPalette :exec
UPDATE items SET palette = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: UpsertEmbedding :exec
INSERT INTO item_embeddings (item_id, user_id, embedding) VALUES ($1, $2, $3)
ON CONFLICT (item_id) DO UPDATE SET embedding = EXCLUDED.embedding;

-- name: SetItemURL :exec
UPDATE items SET url = $3 WHERE user_id = $1 AND id = $2;

-- name: SetItemPageCount :exec
UPDATE items SET page_count = $3 WHERE user_id = $1 AND id = $2;

-- name: CreateQuoteItem :one
INSERT INTO items (user_id, body, card_type, url) VALUES ($1, $2, 'quote', '') RETURNING *;
