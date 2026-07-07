-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, name, key_hash, prefix) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListAPIKeys :many
-- Includes revoked keys (revoked_at set) so callers can render key history.
SELECT * FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: RevokeAPIKey :execrows
UPDATE api_keys SET revoked_at = now() WHERE user_id = $1 AND id = $2 AND revoked_at IS NULL;

-- name: GetAPIKeyByHash :one
SELECT
    api_keys.id AS api_key_id,
    api_keys.user_id,
    api_keys.name,
    api_keys.prefix,
    api_keys.last_used_at,
    users.email,
    users.clerk_user_id
FROM api_keys
JOIN users ON users.id = api_keys.user_id
WHERE api_keys.key_hash = $1 AND api_keys.revoked_at IS NULL;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: CreateDeviceLink :one
INSERT INTO device_links (code_hash, user_id, device_hint, expires_at) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ClaimDeviceLink :one
-- Atomic single-use claim: the row transitions from unclaimed to claimed in
-- one statement, so concurrent claims of the same code can only succeed once.
UPDATE device_links SET claimed_at = now()
WHERE code_hash = $1 AND claimed_at IS NULL AND expires_at > now()
RETURNING user_id;

-- name: GetUserByClerkID :one
SELECT * FROM users WHERE clerk_user_id = $1;

-- name: CreateUserFromClerk :one
INSERT INTO users (clerk_user_id, email) VALUES ($1, $2) RETURNING *;
