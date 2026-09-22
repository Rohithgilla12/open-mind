-- name: InsertJevDecision :one
INSERT INTO jev_decisions (
    user_id, item_id, surface, model, questions_v, answers, action,
    latency_ms, input_tokens, applied_tags, suggested_tags, dismissed_tags
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING id, user_id, item_id, surface, model, questions_v, answers, action,
          user_verdict, latency_ms, input_tokens, created_at,
          applied_tags, suggested_tags, dismissed_tags;

-- name: GetJevCaptureDecision :one
SELECT id, user_id, item_id, surface, model, questions_v, answers, action,
       user_verdict, latency_ms, input_tokens, created_at,
       applied_tags, suggested_tags, dismissed_tags
FROM jev_decisions
WHERE user_id = $1 AND item_id = $2 AND surface = 'capture'
LIMIT 1;

-- name: SetJevUserVerdict :execrows
UPDATE jev_decisions
SET user_verdict = sqlc.arg(user_verdict)
WHERE user_id = sqlc.arg(user_id) AND id = sqlc.arg(id);

-- name: AddJevDismissedTag :execrows
-- Append a dismissed suggestion tag (idempotent via DISTINCT).
UPDATE jev_decisions
SET dismissed_tags = (
    SELECT COALESCE(array_agg(DISTINCT t), '{}')
    FROM unnest(dismissed_tags || ARRAY[sqlc.arg(tag)::text]) AS t
)
WHERE user_id = sqlc.arg(user_id) AND id = sqlc.arg(id);

-- name: ListUserTagVocabulary :many
-- Distinct user_tags across the caller's library, ordered for stable question ids.
SELECT DISTINCT unnest(user_tags)::text AS tag
FROM items
WHERE user_id = $1 AND cardinality(user_tags) > 0
ORDER BY tag;
