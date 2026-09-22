-- name: InsertJevDecision :one
INSERT INTO jev_decisions (
    user_id, item_id, surface, model, questions_v, answers, action, latency_ms, input_tokens
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id, user_id, item_id, surface, model, questions_v, answers, action, user_verdict, latency_ms, input_tokens, created_at;

-- name: GetJevCaptureDecision :one
SELECT id, user_id, item_id, surface, model, questions_v, answers, action, user_verdict, latency_ms, input_tokens, created_at
FROM jev_decisions
WHERE user_id = $1 AND item_id = $2 AND surface = 'capture'
LIMIT 1;

-- name: ListUserTagVocabulary :many
-- Distinct user_tags across the caller's library, ordered for stable question ids.
SELECT DISTINCT unnest(user_tags)::text AS tag
FROM items
WHERE user_id = $1 AND cardinality(user_tags) > 0
ORDER BY tag;
