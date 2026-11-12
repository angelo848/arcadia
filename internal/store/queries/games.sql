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

-- name: GetGameForUpdate :one
SELECT id, platform_id, title, description, release_date, status, favorite, completed_at
FROM game
WHERE id = ?;

-- name: UpdateGameFields :execrows
UPDATE game
SET title = sqlc.arg(title),
    description = sqlc.arg(description),
    platform_id = sqlc.arg(platform_id),
    status = sqlc.arg(status),
    favorite = sqlc.arg(favorite),
    completed_at = sqlc.arg(completed_at),
    release_date = sqlc.arg(release_date),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: PlatformExists :one
SELECT COUNT(*) FROM platform
WHERE id = ?;

-- name: HasOpenSession :one
SELECT COUNT(*) FROM play_session
WHERE game_id = ? AND ended_at IS NULL;

-- name: DeleteGameRow :execrows
DELETE FROM game
WHERE id = ?;
