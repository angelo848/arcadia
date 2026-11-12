-- name: InsertImportedGame :one
INSERT INTO game (platform_id, title, launch_type, launch_target, install_dir, description, release_date, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: FindGameByLaunchTarget :one
SELECT id FROM game
WHERE launch_type = sqlc.arg(launch_type) AND lower(launch_target) = lower(sqlc.arg(launch_target))
LIMIT 1;

-- name: FindGameByTitleAndPlatform :one
SELECT id FROM game
WHERE lower(title) = lower(sqlc.arg(title)) AND platform_id = sqlc.arg(platform_id)
LIMIT 1;

-- name: InsertTrackingRule :exec
INSERT INTO tracking_rule (game_id, kind, value, learned)
VALUES (?, ?, ?, 0);

-- name: InsertImportedSession :one
INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
VALUES (?, ?, ?, ?, ?, 'ended', 'import')
RETURNING id;
