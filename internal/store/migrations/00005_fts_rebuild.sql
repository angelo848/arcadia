-- Rebuilds the game_fts index for every existing game (STO-024). Data only: no schema
-- change. The expression is the same one reindexGame uses in internal/store/fts.go.
-- Runs inside goose's transaction, like the earlier migrations.

-- +goose Up
DELETE FROM game_fts;
INSERT INTO game_fts (rowid, title, description, developer, publisher)
SELECT g.id, g.title, g.description,
       COALESCE((SELECT group_concat(c.name, ' ') FROM game_developer gd
                 JOIN company c ON c.id = gd.company_id WHERE gd.game_id = g.id), ''),
       COALESCE((SELECT group_concat(c.name, ' ') FROM game_publisher gp
                 JOIN company c ON c.id = gp.company_id WHERE gp.game_id = g.id), '')
FROM game g;

-- +goose Down
-- Only the index rows go; the version 4 code does not read game_fts.
DELETE FROM game_fts;
