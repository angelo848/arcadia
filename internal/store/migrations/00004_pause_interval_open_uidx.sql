-- +goose Up
CREATE UNIQUE INDEX pause_interval_open_uidx ON pause_interval (session_id) WHERE resumed_at IS NULL;
-- +goose Down
DROP INDEX pause_interval_open_uidx;
