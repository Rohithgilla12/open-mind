-- name: DeleteItemPlaces :exec
DELETE FROM item_places WHERE user_id = $1 AND item_id = $2;

-- name: DeleteItemPlace :execrows
DELETE FROM item_places WHERE user_id = $1 AND item_id = $2 AND id = $3;

-- name: InsertItemPlace :exec
INSERT INTO item_places (user_id, item_id, name, hint, address, lat, lng, source)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListItemPlaces :many
SELECT * FROM item_places WHERE user_id = $1 AND item_id = $2 ORDER BY created_at, name;

-- name: ListPlaces :many
SELECT p.id, p.name, p.hint, p.address, p.lat, p.lng, p.source,
       i.id AS item_id, i.title AS item_title, i.card_type AS item_card_type
FROM item_places p
JOIN items i ON i.id = p.item_id AND i.user_id = p.user_id
WHERE p.user_id = $1
ORDER BY i.created_at DESC, p.name;
