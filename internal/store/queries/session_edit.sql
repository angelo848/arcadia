-- name: SumCountedSeconds :one
SELECT CAST(COALESCE(SUM(active_seconds), 0) AS INTEGER) AS total
FROM play_session
WHERE game_id = ? AND state = 'ended';

-- name: InsertManualSession :one
INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
VALUES (sqlc.arg(game_id), sqlc.arg(started_at), sqlc.arg(ended_at), sqlc.arg(ended_at), sqlc.arg(active_seconds), 'ended', 'manual')
RETURNING id;

-- name: FindOverlappingSession :one
SELECT s.id, s.game_id, g.title, s.started_at, s.ended_at
FROM play_session s
JOIN game g ON g.id = s.game_id
WHERE s.id <> sqlc.arg(self)
  AND (
    (s.state = 'ended' AND s.source IN ('tracked', 'manual')
       AND s.started_at < sqlc.arg(range_end) AND s.ended_at > sqlc.arg(range_start))
    OR (s.ended_at IS NULL AND s.started_at < sqlc.arg(range_end))
  )
ORDER BY s.started_at, s.id
LIMIT 1;

-- name: RecomputeGamePlayFacts :execrows
UPDATE game
SET first_played_at = (SELECT MIN(ps.started_at) FROM play_session ps
                       WHERE ps.game_id = sqlc.arg(id) AND ps.state = 'ended'),
    last_played_at = (SELECT MAX(ps.ended_at) FROM play_session ps
                      WHERE ps.game_id = sqlc.arg(id) AND ps.state = 'ended'),
    play_count = (SELECT COUNT(*) FROM play_session ps
                  WHERE ps.game_id = sqlc.arg(id) AND ps.state = 'ended'),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: GetSessionForEdit :one
SELECT s.id, s.game_id, g.title, s.state, s.source, s.started_at, s.ended_at,
       s.last_seen_at, s.active_seconds, s.end_reason, s.edited_at
FROM play_session s
JOIN game g ON g.id = s.game_id
WHERE s.id = ?;

-- name: UpdateSessionTimes :execrows
UPDATE play_session
SET started_at = sqlc.arg(started_at),
    ended_at = sqlc.arg(ended_at),
    last_seen_at = sqlc.arg(last_seen_at),
    active_seconds = sqlc.arg(active_seconds),
    edited_at = sqlc.arg(edited_at)
WHERE id = sqlc.arg(id) AND ended_at IS NOT NULL;

-- name: UpdatePauseBounds :exec
UPDATE pause_interval
SET paused_at = sqlc.arg(paused_at), resumed_at = sqlc.arg(resumed_at)
WHERE id = sqlc.arg(id);

-- name: DeleteClosedSession :execrows
DELETE FROM play_session WHERE id = ? AND ended_at IS NOT NULL;

-- name: GetGameUpdatedAt :one
SELECT updated_at FROM game WHERE id = ?;

-- name: SetGameUpdatedAt :execrows
UPDATE game SET updated_at = ? WHERE id = ?;

-- name: AddToTotal :execrows
UPDATE game
SET total_play_seconds = MIN(MAX(total_play_seconds + sqlc.arg(delta), 0), sqlc.arg(max_total)),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);
