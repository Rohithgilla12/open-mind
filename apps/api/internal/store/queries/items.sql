-- name: EnsureUser :exec
INSERT INTO users (id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: CreateItem :one
INSERT INTO items (user_id, url, body) VALUES ($1, $2, $3) RETURNING *;

-- name: GetItem :one
SELECT * FROM items WHERE user_id = $1 AND id = $2;

-- name: ListItems :many
SELECT * FROM items WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: DeleteItem :execrows
DELETE FROM items WHERE user_id = $1 AND id = $2;

-- name: ListItemsForExport :many
SELECT * FROM items WHERE user_id = $1 ORDER BY created_at ASC;

-- name: ListItemURLs :many
SELECT url FROM items WHERE user_id = $1 AND url <> '';

-- name: UpdateItemExtraction :exec
UPDATE items SET title = $3, body = $4, lead_image_url = $5, card_type = $6, updated_at = now()
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
ORDER BY last_drifted_at NULLS FIRST, created_at ASC
LIMIT $2;

-- name: CountDriftCandidates :one
SELECT count(*) FROM items
WHERE user_id = $1 AND status = 'enriched' AND pinned_at IS NULL
  AND (last_drifted_at IS NULL OR last_drifted_at < now() - interval '30 days');

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
