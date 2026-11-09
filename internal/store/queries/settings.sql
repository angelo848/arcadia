-- name: GetSetting :one
SELECT * FROM app_setting
WHERE key = ?;

-- name: UpsertSetting :exec
INSERT INTO app_setting (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
    value      = excluded.value,
    updated_at = excluded.updated_at;
