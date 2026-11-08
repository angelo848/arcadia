-- Initial schema: docs/ARCHITECTURE.md §5.
-- Every instant column (*_at) is INTEGER holding Unix milliseconds in UTC (ADR 0009);
-- release_date is TEXT in YYYY-MM-DD.
-- Enum columns are free TEXT; their values are validated in internal/domain (ADR 0012).

-- +goose Up
CREATE TABLE emulator (
    id            INTEGER PRIMARY KEY,
    name          TEXT    NOT NULL,
    exe_path      TEXT    NOT NULL,
    args_template TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE platform (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL UNIQUE,
    category    TEXT    NOT NULL,
    icon        TEXT,
    emulator_id INTEGER REFERENCES emulator (id) ON DELETE SET NULL
);

CREATE TABLE game (
    id                  INTEGER PRIMARY KEY,
    platform_id         INTEGER NOT NULL REFERENCES platform (id) ON DELETE RESTRICT,
    title               TEXT    NOT NULL,
    sort_title          TEXT    NOT NULL DEFAULT '',
    release_date        TEXT,
    description         TEXT    NOT NULL DEFAULT '',
    install_dir         TEXT    NOT NULL DEFAULT '',
    launch_type         TEXT    NOT NULL,
    launch_target       TEXT    NOT NULL DEFAULT '',
    launch_args         TEXT    NOT NULL DEFAULT '',
    working_dir         TEXT    NOT NULL DEFAULT '',
    rating              REAL,
    favorite            INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
    hidden              INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
    status              TEXT    NOT NULL DEFAULT 'not_played',
    first_played_at     INTEGER,
    completed_at        INTEGER,
    total_play_seconds  INTEGER NOT NULL DEFAULT 0,
    last_played_at      INTEGER,
    play_count          INTEGER NOT NULL DEFAULT 0,
    hltb_main_hours     REAL,
    hltb_extra_hours    REAL,
    hltb_complete_hours REAL,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);

CREATE INDEX game_platform_id_idx ON game (platform_id);

CREATE TABLE company (
    id   INTEGER PRIMARY KEY,
    name TEXT    NOT NULL UNIQUE
);

CREATE TABLE game_developer (
    game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    company_id INTEGER NOT NULL REFERENCES company (id) ON DELETE CASCADE,
    PRIMARY KEY (game_id, company_id)
);

CREATE TABLE game_publisher (
    game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    company_id INTEGER NOT NULL REFERENCES company (id) ON DELETE CASCADE,
    PRIMARY KEY (game_id, company_id)
);

CREATE TABLE genre (
    id   INTEGER PRIMARY KEY,
    name TEXT    NOT NULL UNIQUE
);

CREATE TABLE game_genre (
    game_id  INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    genre_id INTEGER NOT NULL REFERENCES genre (id) ON DELETE CASCADE,
    PRIMARY KEY (game_id, genre_id)
);

CREATE TABLE play_mode (
    id   INTEGER PRIMARY KEY,
    name TEXT    NOT NULL UNIQUE
);

INSERT INTO play_mode (name) VALUES
    ('single'),
    ('multi_local'),
    ('multi_online'),
    ('coop_local'),
    ('coop_online');

CREATE TABLE game_play_mode (
    game_id      INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    play_mode_id INTEGER NOT NULL REFERENCES play_mode (id) ON DELETE CASCADE,
    PRIMARY KEY (game_id, play_mode_id)
);

CREATE TABLE media_asset (
    id         INTEGER PRIMARY KEY,
    game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    thumb_path TEXT,
    source     TEXT    NOT NULL DEFAULT '',
    width      INTEGER,
    height     INTEGER,
    region     TEXT,
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1))
);

CREATE INDEX media_asset_game_id_idx ON media_asset (game_id, kind);

CREATE TABLE additional_app (
    id         INTEGER PRIMARY KEY,
    game_id    INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    target     TEXT    NOT NULL,
    args       TEXT    NOT NULL DEFAULT '',
    track_time INTEGER NOT NULL DEFAULT 0 CHECK (track_time IN (0, 1))
);

CREATE TABLE tracking_rule (
    id      INTEGER PRIMARY KEY,
    game_id INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    kind    TEXT    NOT NULL,
    value   TEXT    NOT NULL,
    learned INTEGER NOT NULL DEFAULT 0 CHECK (learned IN (0, 1))
);

CREATE INDEX tracking_rule_game_id_idx ON tracking_rule (game_id);

CREATE TABLE play_session (
    id             INTEGER PRIMARY KEY,
    game_id        INTEGER NOT NULL REFERENCES game (id) ON DELETE CASCADE,
    started_at     INTEGER NOT NULL,
    ended_at       INTEGER,
    last_seen_at   INTEGER NOT NULL,
    active_seconds INTEGER NOT NULL DEFAULT 0,
    state          TEXT    NOT NULL,
    source         TEXT    NOT NULL
);

CREATE INDEX play_session_game_id_started_at_idx ON play_session (game_id, started_at);

CREATE TABLE pause_interval (
    id         INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES play_session (id) ON DELETE CASCADE,
    paused_at  INTEGER NOT NULL,
    resumed_at INTEGER,
    reason     TEXT    NOT NULL
);

CREATE INDEX pause_interval_session_id_idx ON pause_interval (session_id);

CREATE TABLE playlist (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    filter_json TEXT
);

-- rowid of game_fts is game.id; kept in sync by the application layer.
CREATE VIRTUAL TABLE game_fts USING fts5 (title, description, developer, publisher);

-- +goose Down
DROP TABLE game_fts;
DROP TABLE playlist;
DROP TABLE pause_interval;
DROP TABLE play_session;
DROP TABLE tracking_rule;
DROP TABLE additional_app;
DROP TABLE media_asset;
DROP TABLE game_play_mode;
DROP TABLE play_mode;
DROP TABLE game_genre;
DROP TABLE genre;
DROP TABLE game_publisher;
DROP TABLE game_developer;
DROP TABLE company;
DROP TABLE game;
DROP TABLE platform;
DROP TABLE emulator;
