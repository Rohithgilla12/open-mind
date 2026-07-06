-- name: CreateFeed :one
INSERT INTO feeds (user_id, url, title, site_url) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListFeeds :many
SELECT * FROM feeds WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetFeed :one
SELECT * FROM feeds WHERE user_id = $1 AND id = $2;

-- name: DeleteFeed :execrows
DELETE FROM feeds WHERE user_id = $1 AND id = $2;

-- name: SetFeedPolled :exec
UPDATE feeds SET last_polled_at = $3, last_status = $4 WHERE user_id = $1 AND id = $2;

-- name: ListFeedsDue :many
-- Cross-user by design: the periodic poller runs system-wide, refreshing every
-- feed whose last poll is null or older than the cutoff. Items it saves are
-- still scoped to each feed's own user_id.
SELECT * FROM feeds WHERE last_polled_at IS NULL OR last_polled_at < $1 ORDER BY last_polled_at ASC NULLS FIRST;
