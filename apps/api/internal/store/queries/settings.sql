-- name: GetUserSetting :one
SELECT value FROM user_settings WHERE user_id = $1 AND key = $2;

-- name: UpsertUserSetting :exec
INSERT INTO user_settings (user_id, key, value) VALUES ($1, $2, $3)
ON CONFLICT (user_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: DeleteUserSetting :execrows
DELETE FROM user_settings WHERE user_id = $1 AND key = $2;

-- name: ListUserSettings :many
SELECT key, value FROM user_settings WHERE user_id = $1;

-- name: ListUsersWithAIAssisted :many
-- Users who opted into AI-assisted organisation (Jev capture / rerank / Drift).
SELECT user_id FROM user_settings
WHERE key = 'ai_assisted_organisation' AND lower(trim(value)) = 'true';
