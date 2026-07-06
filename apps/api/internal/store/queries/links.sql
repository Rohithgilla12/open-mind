-- name: CreateLink :execrows
INSERT INTO links (user_id, a_item, b_item)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: DeleteLink :execrows
DELETE FROM links WHERE user_id = $1 AND a_item = $2 AND b_item = $3;

-- name: ListLinkedItems :many
SELECT i.* FROM links l
JOIN items i ON i.id = CASE WHEN l.a_item = $2 THEN l.b_item ELSE l.a_item END
WHERE l.user_id = $1 AND ($2 IN (l.a_item, l.b_item))
ORDER BY l.created_at DESC;
