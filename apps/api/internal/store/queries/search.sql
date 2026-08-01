-- library_only: when true, Mind predicate
-- types: NULL or '{}' means no type filter; else card_type = ANY(types)
-- domains: NULL or '{}' means no domain filter; else host equals or is subdomain

-- name: SearchFTS :many
SELECT *, ts_rank(search_tsv, websearch_to_tsquery('english', $2))::float8 AS rank
FROM items
WHERE user_id = $1
  AND search_tsv @@ websearch_to_tsquery('english', $2)
  AND (
    NOT sqlc.arg(library_only)::bool
    OR feed_id IS NULL
    OR kept_at IS NOT NULL
  )
  AND (
    sqlc.narg(filter_types)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_types)::text[]) = 0
    OR card_type = ANY (sqlc.narg(filter_types)::text[])
  )
  AND (
    sqlc.narg(filter_domains)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_domains)::text[]) = 0
    OR EXISTS (
      SELECT 1
      FROM unnest(sqlc.narg(filter_domains)::text[]) AS d(domain)
      WHERE url_host = d.domain
         OR url_host LIKE '%.' || d.domain
    )
  )
ORDER BY rank DESC
LIMIT $3;

-- name: SearchVector :many
SELECT i.*, (1 - (e.embedding <=> $2))::float8 AS similarity
FROM item_embeddings e JOIN items i ON i.id = e.item_id
WHERE e.user_id = $1
  AND (
    NOT sqlc.arg(library_only)::bool
    OR i.feed_id IS NULL
    OR i.kept_at IS NOT NULL
  )
  AND (
    sqlc.narg(filter_types)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_types)::text[]) = 0
    OR i.card_type = ANY (sqlc.narg(filter_types)::text[])
  )
  AND (
    sqlc.narg(filter_domains)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_domains)::text[]) = 0
    OR EXISTS (
      SELECT 1
      FROM unnest(sqlc.narg(filter_domains)::text[]) AS d(domain)
      WHERE i.url_host = d.domain
         OR i.url_host LIKE '%.' || d.domain
    )
  )
ORDER BY e.embedding <=> $2 LIMIT $3;

-- name: ListItemsWithPalette :many
SELECT * FROM items
WHERE user_id = $1
  AND cardinality(palette) > 0
  AND (
    NOT sqlc.arg(library_only)::bool
    OR feed_id IS NULL
    OR kept_at IS NOT NULL
  )
  AND (
    sqlc.narg(filter_types)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_types)::text[]) = 0
    OR card_type = ANY (sqlc.narg(filter_types)::text[])
  )
  AND (
    sqlc.narg(filter_domains)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_domains)::text[]) = 0
    OR EXISTS (
      SELECT 1
      FROM unnest(sqlc.narg(filter_domains)::text[]) AS d(domain)
      WHERE url_host = d.domain
         OR url_host LIKE '%.' || d.domain
    )
  );

-- name: ListItemsMatching :many
-- Filter-only listing (types and/or domains; optional library scope).
-- Newest first. Used when a Query has no text/colour rank signal.
SELECT *
FROM items
WHERE user_id = sqlc.arg(user_id)
  AND (
    NOT sqlc.arg(library_only)::bool
    OR feed_id IS NULL
    OR kept_at IS NOT NULL
  )
  AND (
    sqlc.narg(filter_types)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_types)::text[]) = 0
    OR card_type = ANY (sqlc.narg(filter_types)::text[])
  )
  AND (
    sqlc.narg(filter_domains)::text[] IS NULL
    OR cardinality(sqlc.narg(filter_domains)::text[]) = 0
    OR EXISTS (
      SELECT 1
      FROM unnest(sqlc.narg(filter_domains)::text[]) AS d(domain)
      WHERE url_host = d.domain
         OR url_host LIKE '%.' || d.domain
    )
  )
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count);

-- name: RelatedByEmbedding :many
-- Nearest unlinked items to the given item's embedding, same user only.
-- The links table stores one canonicalised row per pair (a_item < b_item),
-- so the exclusion checks the pair in canonical order.
SELECT i.*, (e.embedding <=> src.embedding)::float8 AS distance
FROM item_embeddings src
JOIN item_embeddings e ON e.user_id = src.user_id AND e.item_id <> src.item_id
JOIN items i ON i.id = e.item_id
WHERE src.user_id = sqlc.arg(user_id) AND src.item_id = sqlc.arg(item_id)
  AND (e.embedding <=> src.embedding) <= sqlc.arg(max_distance)::float8
  AND NOT EXISTS (
    SELECT 1 FROM links l
    WHERE l.user_id = src.user_id
      AND l.a_item = LEAST(src.item_id, e.item_id)
      AND l.b_item = GREATEST(src.item_id, e.item_id)
  )
ORDER BY e.embedding <=> src.embedding
LIMIT sqlc.arg(limit_count);
