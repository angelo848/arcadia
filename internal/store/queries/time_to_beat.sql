-- name: GetTimeToBeat :one
SELECT * FROM time_to_beat WHERE game_id = ?;

-- name: UpsertTimeToBeat :exec
INSERT INTO time_to_beat (
  game_id, source, state, external_id, external_title, main_s, main_extra_s, completionist_s,
  title_at_fetch, term, candidates, confirmed, fetched_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (game_id) DO UPDATE SET
  source          = excluded.source,
  state           = excluded.state,
  external_id     = excluded.external_id,
  external_title  = excluded.external_title,
  main_s          = excluded.main_s,
  main_extra_s    = excluded.main_extra_s,
  completionist_s = excluded.completionist_s,
  title_at_fetch  = excluded.title_at_fetch,
  term            = excluded.term,
  candidates      = excluded.candidates,
  confirmed       = excluded.confirmed,
  fetched_at      = excluded.fetched_at,
  expires_at      = excluded.expires_at;
