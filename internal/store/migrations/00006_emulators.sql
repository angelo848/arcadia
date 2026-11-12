-- Emulators (phase 6b, EMU-001, EMU-002). A game may name its own emulator; the
-- platform's default (platform.emulator_id, migration 1) is used otherwise. Names are
-- unique without regard to case. Deleting an emulator clears the references (SET NULL)
-- and deletes no game.

-- +goose Up
ALTER TABLE game ADD COLUMN emulator_id INTEGER REFERENCES emulator (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX emulator_name_uidx ON emulator (name COLLATE NOCASE);

-- +goose Down
DROP INDEX emulator_name_uidx;
ALTER TABLE game DROP COLUMN emulator_id;
