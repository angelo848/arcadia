-- +goose Up
ALTER TABLE play_session ADD COLUMN edited_at INTEGER;
CREATE INDEX play_session_started_at_idx ON play_session (started_at);
-- +goose Down
DROP INDEX play_session_started_at_idx;
ALTER TABLE play_session DROP COLUMN edited_at;
