-- name: CreateLens :one
INSERT INTO lenses (user_id, name, rule) VALUES ($1, $2, $3) RETURNING *;

-- name: ListLenses :many
SELECT * FROM lenses WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetLens :one
SELECT * FROM lenses WHERE user_id = $1 AND id = $2;

-- name: UpdateLens :one
UPDATE lenses SET name = $3, rule = $4, updated_at = now()
WHERE user_id = $1 AND id = $2 RETURNING *;

-- name: DeleteLens :execrows
DELETE FROM lenses WHERE user_id = $1 AND id = $2;
