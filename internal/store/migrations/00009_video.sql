-- Video kind (phase 13, VID-001, VID-002, ADR 0037). Only widens the CHECK on the media kind
-- of media and media_job (rebuild of both tables; no new column). No table references them,
-- so foreign_keys does not need to be turned off. Runs in one goose transaction.

-- +goose Up
CREATE TABLE media_new (
  id         INTEGER PRIMARY KEY,
  game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('cover', 'screenshot', 'title', 'banner', 'background', 'select', 'video')),
  provider   TEXT    NOT NULL,
  source_url TEXT    NOT NULL DEFAULT '',
  hash       TEXT    NOT NULL,
  path       TEXT    NOT NULL,
  thumb_path TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (game_id, hash)
);
INSERT INTO media_new (id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at)
  SELECT id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at FROM media;
DROP TABLE media;
ALTER TABLE media_new RENAME TO media;

CREATE TABLE media_job_new (
  id         INTEGER PRIMARY KEY,
  game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('cover', 'screenshot', 'title', 'banner', 'background', 'select', 'video')),
  provider   TEXT    NOT NULL,
  source_url TEXT    NOT NULL,
  state      TEXT    NOT NULL CHECK (state IN ('pending', 'running', 'done', 'failed')),
  attempts   INTEGER NOT NULL DEFAULT 0,
  last_error TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
INSERT INTO media_job_new (id, game_id, kind, provider, source_url, state, attempts, last_error, created_at)
  SELECT id, game_id, kind, provider, source_url, state, attempts, last_error, created_at FROM media_job;
DROP TABLE media_job;
ALTER TABLE media_job_new RENAME TO media_job;

-- +goose Down
DELETE FROM media WHERE kind = 'video';
DELETE FROM media_job WHERE kind = 'video';

CREATE TABLE media_old (
  id         INTEGER PRIMARY KEY,
  game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('cover', 'screenshot', 'title', 'banner', 'background', 'select')),
  provider   TEXT    NOT NULL,
  source_url TEXT    NOT NULL DEFAULT '',
  hash       TEXT    NOT NULL,
  path       TEXT    NOT NULL,
  thumb_path TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (game_id, hash)
);
INSERT INTO media_old (id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at)
  SELECT id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at FROM media;
DROP TABLE media;
ALTER TABLE media_old RENAME TO media;

CREATE TABLE media_job_old (
  id         INTEGER PRIMARY KEY,
  game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL CHECK (kind IN ('cover', 'screenshot', 'title', 'banner', 'background', 'select')),
  provider   TEXT    NOT NULL,
  source_url TEXT    NOT NULL,
  state      TEXT    NOT NULL CHECK (state IN ('pending', 'running', 'done', 'failed')),
  attempts   INTEGER NOT NULL DEFAULT 0,
  last_error TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
INSERT INTO media_job_old (id, game_id, kind, provider, source_url, state, attempts, last_error, created_at)
  SELECT id, game_id, kind, provider, source_url, state, attempts, last_error, created_at FROM media_job;
DROP TABLE media_job;
ALTER TABLE media_job_old RENAME TO media_job;
