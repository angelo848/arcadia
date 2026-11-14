-- Platforms of the seed (phase 9, PLT-001..PLT-008; contracts/seed-platforms.md section 3).
-- Runs in one goose transaction (the file has no non-transactional directive): either every step applies
-- or none does.
-- The seed values below must match domain.SeedPlatforms (a store test compares them).
-- "Same name" means trim(name) equal ignoring ASCII case (COLLATE NOCASE).
-- game.platform_id is ON DELETE RESTRICT: every row deleted here has had its games moved first.
-- game.updated_at is never touched by the moves.
-- The Down section only drops the indexes and the seed_key column. It does NOT undo data:
-- renamed, merged or inserted platforms stay as they are (data migration without a way back).

-- +goose Up

-- Step 1.
ALTER TABLE platform ADD COLUMN seed_key TEXT;

-- Highest platform id before any row is deleted: the seed rows inserted in step 6 get ids above it,
-- so the id of a platform merged away here is not reused by an unrelated seed platform.
CREATE TEMP TABLE platform_id_mark AS SELECT COALESCE(MAX(id), 0) AS mark FROM platform;

-- Step 2 (PLT-005): rows sharing a name collapse into the lowest id.
-- The survivor with no emulator inherits the one of the first other row (by id) that has one.
UPDATE platform
SET emulator_id = (
    SELECT o.emulator_id FROM platform o
    WHERE trim(o.name) = trim(platform.name) COLLATE NOCASE
      AND o.id > platform.id
      AND o.emulator_id IS NOT NULL
    ORDER BY o.id
    LIMIT 1)
WHERE emulator_id IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM platform l
    WHERE trim(l.name) = trim(platform.name) COLLATE NOCASE AND l.id < platform.id);

UPDATE game
SET platform_id = (
    SELECT MIN(k.id) FROM platform k
    WHERE trim(k.name) = (SELECT trim(p.name) FROM platform p WHERE p.id = game.platform_id) COLLATE NOCASE)
WHERE platform_id IN (
    SELECT p.id FROM platform p
    WHERE EXISTS (
        SELECT 1 FROM platform l
        WHERE trim(l.name) = trim(p.name) COLLATE NOCASE AND l.id < p.id));

DELETE FROM platform
WHERE EXISTS (
    SELECT 1 FROM platform l
    WHERE trim(l.name) = trim(platform.name) COLLATE NOCASE AND l.id < platform.id);

-- Step 3 (PLT-003): "PC" and "Windows" both exist: games go to Windows, which inherits the emulator.
UPDATE platform
SET emulator_id = (SELECT pc.emulator_id FROM platform pc WHERE trim(pc.name) = 'PC' COLLATE NOCASE)
WHERE trim(name) = 'Windows' COLLATE NOCASE
  AND emulator_id IS NULL
  AND EXISTS (SELECT 1 FROM platform pc WHERE trim(pc.name) = 'PC' COLLATE NOCASE);

UPDATE game
SET platform_id = (SELECT w.id FROM platform w WHERE trim(w.name) = 'Windows' COLLATE NOCASE)
WHERE EXISTS (SELECT 1 FROM platform w WHERE trim(w.name) = 'Windows' COLLATE NOCASE)
  AND platform_id IN (SELECT pc.id FROM platform pc WHERE trim(pc.name) = 'PC' COLLATE NOCASE);

DELETE FROM platform
WHERE trim(name) = 'PC' COLLATE NOCASE
  AND EXISTS (SELECT 1 FROM platform w WHERE trim(w.name) = 'Windows' COLLATE NOCASE);

-- Step 4 (PLT-002): "PC" with no "Windows" is renamed in place (id, games and emulator kept).
UPDATE platform SET name = 'Windows' WHERE trim(name) = 'PC' COLLATE NOCASE;

-- Step 5 (PLT-004): a row with a canonical name adopts the key, canonical name and category of the seed.
-- Id, games, emulator_id and icon are kept.
WITH seed(key, name, category) AS (
    VALUES
        ('windows',  'Windows',                             'pc'),
        ('3ds',      'Nintendo 3DS',                        'handheld'),
        ('n64',      'Nintendo 64',                         'console'),
        ('nes',      'Nintendo Entertainment System',       'console'),
        ('gba',      'Nintendo Game Boy Advance',           'handheld'),
        ('gamecube', 'Nintendo GameCube',                   'console'),
        ('switch',   'Nintendo Switch',                     'console'),
        ('switch2',  'Nintendo Switch 2',                   'console'),
        ('wii',      'Nintendo Wii',                        'console'),
        ('ps1',      'Sony Playstation',                    'console'),
        ('ps2',      'Sony Playstation 2',                  'console'),
        ('ps3',      'Sony Playstation 3',                  'console'),
        ('snes',     'Super Nintendo Entertainment System', 'console'))
UPDATE platform
SET seed_key = (SELECT s.key FROM seed s WHERE s.name = trim(platform.name) COLLATE NOCASE),
    name     = (SELECT s.name FROM seed s WHERE s.name = trim(platform.name) COLLATE NOCASE),
    category = (SELECT s.category FROM seed s WHERE s.name = trim(platform.name) COLLATE NOCASE)
WHERE EXISTS (SELECT 1 FROM seed s WHERE s.name = trim(platform.name) COLLATE NOCASE);

-- Step 6 (PLT-001): canonical names with no row are inserted (no icon, no emulator).
WITH seed(n, key, name, category) AS (
    VALUES
        (1,  'windows',  'Windows',                             'pc'),
        (2,  '3ds',      'Nintendo 3DS',                        'handheld'),
        (3,  'n64',      'Nintendo 64',                         'console'),
        (4,  'nes',      'Nintendo Entertainment System',       'console'),
        (5,  'gba',      'Nintendo Game Boy Advance',           'handheld'),
        (6,  'gamecube', 'Nintendo GameCube',                   'console'),
        (7,  'switch',   'Nintendo Switch',                     'console'),
        (8,  'switch2',  'Nintendo Switch 2',                   'console'),
        (9,  'wii',      'Nintendo Wii',                        'console'),
        (10, 'ps1',      'Sony Playstation',                    'console'),
        (11, 'ps2',      'Sony Playstation 2',                  'console'),
        (12, 'ps3',      'Sony Playstation 3',                  'console'),
        (13, 'snes',     'Super Nintendo Entertainment System', 'console'))
INSERT INTO platform (id, name, category, seed_key)
SELECT (SELECT mark FROM platform_id_mark) + s.n, s.name, s.category, s.key FROM seed s
WHERE NOT EXISTS (SELECT 1 FROM platform p WHERE p.seed_key = s.key);

DROP TABLE platform_id_mark;

-- Step 7 (PLT-006).
CREATE UNIQUE INDEX platform_name_nocase_uidx ON platform (name COLLATE NOCASE);

-- Step 8.
CREATE UNIQUE INDEX platform_seed_key_uidx ON platform (seed_key) WHERE seed_key IS NOT NULL;

-- +goose Down
-- Does not undo data (renames, merges and inserted platforms stay).
DROP INDEX platform_seed_key_uidx;
DROP INDEX platform_name_nocase_uidx;
ALTER TABLE platform DROP COLUMN seed_key;
