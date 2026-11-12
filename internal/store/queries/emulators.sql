-- name: ListEmulators :many
SELECT id, name, exe_path, args_template FROM emulator
ORDER BY name COLLATE NOCASE, id;

-- name: GetEmulator :one
SELECT id, name, exe_path, args_template FROM emulator
WHERE id = ?;

-- name: EmulatorNameTaken :one
SELECT COUNT(*) FROM emulator
WHERE name = sqlc.arg(name) COLLATE NOCASE AND id <> sqlc.arg(except_id);

-- name: InsertEmulator :one
INSERT INTO emulator (name, exe_path, args_template)
VALUES (?, ?, ?)
RETURNING id;

-- name: UpdateEmulator :execrows
UPDATE emulator
SET name = ?, exe_path = ?, args_template = ?
WHERE id = ?;

-- name: DeleteEmulator :execrows
DELETE FROM emulator WHERE id = ?;

-- name: SetPlatformEmulator :execrows
UPDATE platform SET emulator_id = sqlc.narg(emulator_id) WHERE id = sqlc.arg(id);

-- name: PlatformEmulatorID :one
SELECT emulator_id FROM platform WHERE id = ?;

-- name: GameEmulatorRef :one
SELECT g.emulator_id AS game_emulator_id, p.emulator_id AS platform_emulator_id
FROM game g JOIN platform p ON p.id = g.platform_id
WHERE g.id = ?;

-- name: SetGameEmulator :execrows
UPDATE game SET emulator_id = sqlc.narg(emulator_id), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);
