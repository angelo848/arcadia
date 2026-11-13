-- Statistics (phase 8, EST-001..EST-005). Sessions count with their source: tracked by the agent or
-- imported from LaunchBox (IMP-007). since is the start instant in ms; 0 means all time.

-- name: StatsByGame :many
SELECT g.id AS game_id, g.title, p.name AS platform,
       CAST(COUNT(s.id) AS INTEGER) AS sessions,
       CAST(COALESCE(SUM(s.active_seconds), 0) AS INTEGER) AS active_seconds,
       CAST(COALESCE(SUM(CASE WHEN s.source = 'import' THEN s.active_seconds ELSE 0 END), 0) AS INTEGER) AS imported_seconds,
       CAST(MAX(s.started_at) AS INTEGER) AS last_started_at
FROM play_session s
JOIN game g ON g.id = s.game_id
JOIN platform p ON p.id = g.platform_id
WHERE s.started_at >= sqlc.arg(since)
GROUP BY g.id, g.title, p.name
ORDER BY active_seconds DESC, g.title COLLATE NOCASE;

-- name: StatsByPlatform :many
SELECT p.id AS platform_id, p.name,
       CAST(COALESCE(SUM(s.active_seconds), 0) AS INTEGER) AS active_seconds,
       CAST(COALESCE(SUM(CASE WHEN s.source = 'import' THEN s.active_seconds ELSE 0 END), 0) AS INTEGER) AS imported_seconds
FROM play_session s
JOIN game g ON g.id = s.game_id
JOIN platform p ON p.id = g.platform_id
WHERE s.started_at >= sqlc.arg(since)
GROUP BY p.id, p.name
HAVING SUM(s.active_seconds) > 0
ORDER BY active_seconds DESC, p.name COLLATE NOCASE;

-- name: SessionTimesSince :many
SELECT started_at, active_seconds, source
FROM play_session
WHERE started_at >= sqlc.arg(since)
ORDER BY started_at;
