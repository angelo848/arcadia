-- name: InsertPauseOpen :exec
INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
VALUES (?, ?, NULL, ?);

-- name: GetOpenPause :one
SELECT id, session_id, paused_at, resumed_at, reason FROM pause_interval
WHERE session_id = ? AND resumed_at IS NULL;

-- name: ClosePause :exec
UPDATE pause_interval
SET resumed_at = ?
WHERE id = ?;

-- name: DeletePause :exec
DELETE FROM pause_interval
WHERE id = ?;

-- name: ClosePauseAt :execrows
UPDATE pause_interval
SET resumed_at = sqlc.arg(ended_at)
WHERE session_id = sqlc.arg(session_id) AND resumed_at IS NULL AND paused_at < sqlc.arg(ended_at);

-- name: DeleteOpenPause :exec
DELETE FROM pause_interval
WHERE session_id = ? AND resumed_at IS NULL;

-- name: ListPauses :many
SELECT id, session_id, paused_at, resumed_at, reason FROM pause_interval
WHERE session_id = ?
ORDER BY paused_at, id;
