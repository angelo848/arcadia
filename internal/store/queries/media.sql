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

-- Video jobs (phase 13, VID-002..VID-008).

-- name: VideoGameInfoRow :one
SELECT g.title, g.release_date, p.seed_key
FROM game g
JOIN platform p ON p.id = g.platform_id
WHERE g.id = ?;

-- name: VideoCandidateByGame :one
SELECT g.id, p.seed_key,
       (EXISTS (SELECT 1 FROM media m WHERE m.game_id = g.id AND m.kind = 'video')
        OR EXISTS (SELECT 1 FROM media_job j WHERE j.game_id = g.id AND j.kind = 'video'
                   AND j.state IN ('pending','running'))) AS has_video
FROM game g
JOIN platform p ON p.id = g.platform_id
WHERE g.id = ?;

-- name: VideoCandidatesAll :many
SELECT g.id, p.seed_key,
       (EXISTS (SELECT 1 FROM media m WHERE m.game_id = g.id AND m.kind = 'video')
        OR EXISTS (SELECT 1 FROM media_job j WHERE j.game_id = g.id AND j.kind = 'video'
                   AND j.state IN ('pending','running'))) AS has_video
FROM game g
JOIN platform p ON p.id = g.platform_id
ORDER BY g.id;

-- name: VideoCandidatesByPlatform :many
SELECT g.id, p.seed_key,
       (EXISTS (SELECT 1 FROM media m WHERE m.game_id = g.id AND m.kind = 'video')
        OR EXISTS (SELECT 1 FROM media_job j WHERE j.game_id = g.id AND j.kind = 'video'
                   AND j.state IN ('pending','running'))) AS has_video
FROM game g
JOIN platform p ON p.id = g.platform_id
WHERE g.platform_id = ?
ORDER BY g.id;

-- name: ListRunningVideoJobIDs :many
SELECT id FROM media_job WHERE kind = 'video' AND state = 'running' ORDER BY id;

-- name: CancelPendingVideoJobs :execrows
UPDATE media_job SET state = 'failed', last_error = 'cancelado'
WHERE kind = 'video' AND state = 'pending';

-- name: CancelRunningVideoJobs :execrows
UPDATE media_job SET state = 'failed', last_error = 'cancelado'
WHERE kind = 'video' AND state = 'running';

-- name: VideoJobCounts :one
SELECT
  CAST(COALESCE(SUM(state = 'pending'), 0) AS INTEGER) AS pending,
  CAST(COALESCE(SUM(state = 'running'), 0) AS INTEGER) AS running,
  CAST(COALESCE(SUM(state = 'failed' AND last_error <> 'cancelado'), 0) AS INTEGER) AS failed
FROM media_job
WHERE kind = 'video';

-- name: GetMedia :one
SELECT id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at
FROM media
WHERE id = ?;
