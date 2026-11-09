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
SELECT * FROM game
ORDER BY title COLLATE NOCASE, id;

-- name: GetGame :one
SELECT * FROM game
WHERE id = ?;
