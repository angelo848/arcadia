-- Media (phase 7, MID-004, MID-005). A media row is one image kept for a game; the hash of
-- its content is unique per game, so the same file is never stored twice. A media_job is one
-- download waiting in the queue, kept across restarts (MID-005). Deleting a game deletes both.

-- +goose Up
CREATE TABLE media (
  id         INTEGER PRIMARY KEY,
  game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('cover', 'screenshot', 'title')),
  provider   TEXT    NOT NULL,
  source_url TEXT    NOT NULL DEFAULT '',
  hash       TEXT    NOT NULL,
  path       TEXT    NOT NULL,
  thumb_path TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (game_id, hash)
);

CREATE TABLE media_job (
  id         INTEGER PRIMARY KEY,
  game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('cover', 'screenshot', 'title')),
  provider   TEXT    NOT NULL,
  source_url TEXT    NOT NULL,
  state      TEXT    NOT NULL CHECK (state IN ('pending', 'running', 'done', 'failed')),
  attempts   INTEGER NOT NULL DEFAULT 0,
  last_error TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE media_job;
DROP TABLE media;
