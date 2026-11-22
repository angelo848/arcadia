-- Time to beat (phase 15, TTB-001, TTB-002). New table only; nothing else changes.
-- Runs in one goose transaction.

-- +goose Up
CREATE TABLE time_to_beat (
  game_id         INTEGER PRIMARY KEY REFERENCES game (id) ON DELETE CASCADE,
  source          TEXT    NOT NULL CHECK (source IN ('hltb', 'igdb')),          -- igdb: fallback (TTB-033)
  state           TEXT    NOT NULL CHECK (state IN ('found', 'doubt', 'not-found')),
  external_id     TEXT    NOT NULL DEFAULT '',
  external_title  TEXT    NOT NULL DEFAULT '',
  main_s          INTEGER NULL CHECK (main_s IS NULL OR main_s > 0),
  main_extra_s    INTEGER NULL CHECK (main_extra_s IS NULL OR main_extra_s > 0),
  completionist_s INTEGER NULL CHECK (completionist_s IS NULL OR completionist_s > 0),
  title_at_fetch  TEXT    NOT NULL,
  term            TEXT    NOT NULL,
  candidates      TEXT    NOT NULL DEFAULT '[]',   -- JSON (rules §5), only for state = 'doubt'
  confirmed       INTEGER NOT NULL DEFAULT 0 CHECK (confirmed IN (0, 1)),
  fetched_at      INTEGER NOT NULL,                -- Unix ms UTC (ADR 0009)
  expires_at      INTEGER NOT NULL,
  CHECK (source = 'hltb' OR (state = 'found' AND confirmed = 0))                 -- igdb only as automatic found (TTB-002, TTB-040)
);

-- +goose Down
DROP TABLE time_to_beat;
