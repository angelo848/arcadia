-- name: InsertSession :one
INSERT INTO play_session (game_id, started_at, last_seen_at, active_seconds, state, source)
VALUES (?, ?, ?, 0, 'running', 'tracked')
RETURNING id;

-- name: UpdateOpenSession :execrows
UPDATE play_session
SET state = ?, last_seen_at = ?, active_seconds = ?
WHERE id = ? AND ended_at IS NULL;

-- name: GetOpenSession :one
SELECT id, game_id, started_at, last_seen_at, active_seconds FROM play_session
WHERE id = ? AND ended_at IS NULL;

-- name: CloseSession :execrows
UPDATE play_session
SET ended_at = sqlc.arg(ended_at), last_seen_at = sqlc.arg(ended_at), active_seconds = sqlc.arg(active_seconds),
    state = sqlc.arg(state), end_reason = sqlc.arg(end_reason)
WHERE id = sqlc.arg(id) AND ended_at IS NULL;

-- name: AddPlayToGame :execrows
UPDATE game
SET total_play_seconds = total_play_seconds + sqlc.arg(seconds),
    play_count = play_count + 1,
    last_played_at = sqlc.arg(last_played_at),
    first_played_at = COALESCE(first_played_at, sqlc.arg(first_played_at)),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteSession :exec
DELETE FROM play_session
WHERE id = ?;

-- name: ListOpenSessions :many
SELECT id, game_id, started_at, last_seen_at, active_seconds FROM play_session
WHERE ended_at IS NULL
ORDER BY id;

-- name: CountSessionsByGame :one
SELECT COUNT(*) FROM play_session
WHERE game_id = ?;

-- name: ListSessionsByGame :many
SELECT id, game_id, state, started_at, ended_at, active_seconds, end_reason FROM play_session
WHERE game_id = sqlc.arg(game_id)
ORDER BY started_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
