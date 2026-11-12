-- name: EnsurePCPlatform :exec
INSERT INTO platform (name, category)
VALUES ('PC', 'pc')
ON CONFLICT (name) DO NOTHING;

-- name: GetPlatformIDByName :one
SELECT id FROM platform
WHERE name = ?;

-- name: InsertGame :one
INSERT INTO game (platform_id, title, launch_type, launch_target, launch_args, working_dir, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListGames :many
SELECT g.id, g.platform_id, p.name AS platform_name, g.title, g.release_date,
       g.description, g.launch_type, g.launch_target, g.launch_args, g.working_dir,
       g.favorite, g.status, g.first_played_at, g.last_played_at, g.completed_at,
       g.total_play_seconds, g.play_count
FROM game g
JOIN platform p ON p.id = g.platform_id
ORDER BY g.title COLLATE NOCASE, g.id;

-- name: GetGame :one
SELECT * FROM game
WHERE id = ?;
