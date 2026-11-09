-- Phase 3: why a session ended, and a partial index to find open sessions.
-- end_reason is free TEXT validated in internal/domain (ADR 0012): exited | agent-shutdown | recovered.

-- +goose Up
ALTER TABLE play_session ADD COLUMN end_reason TEXT;
CREATE INDEX play_session_open_idx ON play_session (ended_at) WHERE ended_at IS NULL;

-- +goose Down
DROP INDEX play_session_open_idx;
ALTER TABLE play_session DROP COLUMN end_reason;
