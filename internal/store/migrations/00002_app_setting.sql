-- +goose Up
CREATE TABLE app_setting (
    key        TEXT    PRIMARY KEY,
    value      TEXT    NOT NULL,
    updated_at INTEGER NOT NULL   -- Unix ms UTC (ADR 0009)
);

-- +goose Down
DROP TABLE app_setting;
