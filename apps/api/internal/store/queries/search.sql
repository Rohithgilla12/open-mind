-- name: SearchFTS :many
SELECT *, ts_rank(search_tsv, websearch_to_tsquery('english', $2))::float8 AS rank
FROM items
WHERE user_id = $1 AND search_tsv @@ websearch_to_tsquery('english', $2)
ORDER BY rank DESC LIMIT $3;

-- name: SearchVector :many
SELECT i.*, (1 - (e.embedding <=> $2))::float8 AS similarity
FROM item_embeddings e JOIN items i ON i.id = e.item_id
WHERE e.user_id = $1
ORDER BY e.embedding <=> $2 LIMIT $3;

-- name: ListItemsWithPalette :many
SELECT * FROM items WHERE user_id = $1 AND cardinality(palette) > 0;
