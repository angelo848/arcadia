-- Media and the download queue (phase 7, MID-004, MID-005). Times are INTEGER ms (ADR 0009).

-- name: ListMediaForGame :many
SELECT id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at
FROM media
WHERE game_id = ?
ORDER BY kind, id;

-- name: InsertMedia :one
INSERT INTO media (game_id, kind, provider, source_url, hash, path, thumb_path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: DeleteMedia :execrows
DELETE FROM media WHERE id = ?;

-- name: InsertMediaJob :one
INSERT INTO media_job (game_id, kind, provider, source_url, state, created_at)
VALUES (?, ?, ?, ?, 'pending', ?)
RETURNING id;

-- name: ListMediaJobsByState :many
SELECT id, game_id, kind, provider, source_url, state, attempts, last_error, created_at
FROM media_job
WHERE state = ?
ORDER BY id;

-- name: SetMediaJobState :execrows
UPDATE media_job
SET state = ?, attempts = ?, last_error = ?
WHERE id = ?;

-- name: DeleteMediaJob :execrows
DELETE FROM media_job WHERE id = ?;

-- name: InsertMediaIgnore :execrows
INSERT INTO media (game_id, kind, provider, source_url, hash, path, thumb_path, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (game_id, hash) DO NOTHING;

-- name: ResetRunningMediaJobs :execrows
UPDATE media_job SET state = 'pending' WHERE state = 'running';

-- name: GetMediaJob :one
SELECT id, game_id, kind, provider, source_url, state, attempts, last_error, created_at
FROM media_job
WHERE id = ?;

-- name: CountActiveMediaJobsByKind :many
SELECT kind, COUNT(*) AS n FROM media_job WHERE game_id = ? AND state IN ('pending','running') GROUP BY kind;
