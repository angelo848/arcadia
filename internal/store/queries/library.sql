-- name: CountGames :one
SELECT COUNT(*) FROM game;

-- name: ListPlatforms :many
SELECT * FROM platform
ORDER BY name;

-- name: InsertPlatform :one
INSERT INTO platform (name, category)
VALUES (?, ?)
RETURNING id;

-- name: GetGameRow :one
SELECT g.id, g.platform_id, p.name AS platform_name, g.title, g.release_date,
       g.description, g.launch_type, g.launch_target, g.launch_args, g.working_dir,
       g.favorite, g.status, g.first_played_at, g.last_played_at, g.completed_at,
       g.total_play_seconds, g.play_count
FROM game g
JOIN platform p ON p.id = g.platform_id
WHERE g.id = ?;

-- name: GameGenreNames :many
SELECT c.name FROM game_genre gg
JOIN genre c ON c.id = gg.genre_id
WHERE gg.game_id = ?
ORDER BY c.name COLLATE NOCASE, c.id;

-- name: GameDeveloperNames :many
SELECT c.name FROM game_developer gd
JOIN company c ON c.id = gd.company_id
WHERE gd.game_id = ?
ORDER BY c.name COLLATE NOCASE, c.id;

-- name: GamePublisherNames :many
SELECT c.name FROM game_publisher gp
JOIN company c ON c.id = gp.company_id
WHERE gp.game_id = ?
ORDER BY c.name COLLATE NOCASE, c.id;

-- name: GamePlayModeNames :many
SELECT m.name FROM game_play_mode gm
JOIN play_mode m ON m.id = gm.play_mode_id
WHERE gm.game_id = ?
ORDER BY m.name COLLATE NOCASE, m.id;

-- name: GetGenreByName :one
SELECT id FROM genre
WHERE name = ? COLLATE NOCASE
ORDER BY id
LIMIT 1;

-- name: InsertGenre :one
INSERT INTO genre (name)
VALUES (?)
RETURNING id;

-- name: GetCompanyByName :one
SELECT id FROM company
WHERE name = ? COLLATE NOCASE
ORDER BY id
LIMIT 1;

-- name: InsertCompany :one
INSERT INTO company (name)
VALUES (?)
RETURNING id;

-- name: PlayModeIDByName :one
SELECT id FROM play_mode
WHERE name = ?;

-- name: GameDeveloperIDs :many
SELECT company_id FROM game_developer
WHERE game_id = ?
ORDER BY company_id;

-- name: GamePublisherIDs :many
SELECT company_id FROM game_publisher
WHERE game_id = ?
ORDER BY company_id;

-- name: GameGenreIDs :many
SELECT genre_id FROM game_genre
WHERE game_id = ?
ORDER BY genre_id;

-- name: GamePlayModeIDs :many
SELECT play_mode_id FROM game_play_mode
WHERE game_id = ?
ORDER BY play_mode_id;

-- name: DeleteGameDevelopers :exec
DELETE FROM game_developer
WHERE game_id = ?;

-- name: DeleteGamePublishers :exec
DELETE FROM game_publisher
WHERE game_id = ?;

-- name: DeleteGameGenres :exec
DELETE FROM game_genre
WHERE game_id = ?;

-- name: DeleteGamePlayModes :exec
DELETE FROM game_play_mode
WHERE game_id = ?;

-- name: InsertGameDeveloper :exec
INSERT INTO game_developer (game_id, company_id)
VALUES (?, ?);

-- name: InsertGamePublisher :exec
INSERT INTO game_publisher (game_id, company_id)
VALUES (?, ?);

-- name: InsertGameGenre :exec
INSERT INTO game_genre (game_id, genre_id)
VALUES (?, ?);

-- name: InsertGamePlayMode :exec
INSERT INTO game_play_mode (game_id, play_mode_id)
VALUES (?, ?);

-- name: ListGenreNames :many
SELECT id, name FROM genre
ORDER BY name COLLATE NOCASE, id;

-- name: ListCompanyNames :many
SELECT id, name FROM company
ORDER BY name COLLATE NOCASE, id;

-- name: ListPlatformsByName :many
SELECT * FROM platform
ORDER BY name COLLATE NOCASE, id;

-- name: GetPlatformByNameNoCase :one
SELECT * FROM platform
WHERE name = ? COLLATE NOCASE
ORDER BY id
LIMIT 1;
