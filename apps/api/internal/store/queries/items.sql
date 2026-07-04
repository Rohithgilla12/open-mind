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

-- name: UpdateItemExtraction :exec
UPDATE items SET title = $3, body = $4, lead_image_url = $5, card_type = $6, updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: UpdateItemEnrichment :exec
UPDATE items SET summary = $3, tags = $4, updated_at = now()
WHERE user_id = $1 AND id = $2;

-- name: SetItemStatus :exec
UPDATE items SET status = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: SetItemPalette :exec
UPDATE items SET palette = $3, updated_at = now() WHERE user_id = $1 AND id = $2;

-- name: UpsertEmbedding :exec
INSERT INTO item_embeddings (item_id, user_id, embedding) VALUES ($1, $2, $3)
ON CONFLICT (item_id) DO UPDATE SET embedding = EXCLUDED.embedding;
