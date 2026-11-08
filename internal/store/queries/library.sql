-- name: CountGames :one
SELECT COUNT(*) FROM game;

-- name: ListPlatforms :many
SELECT * FROM platform
ORDER BY name;

-- name: InsertPlatform :one
INSERT INTO platform (name, category)
VALUES (?, ?)
RETURNING id;
