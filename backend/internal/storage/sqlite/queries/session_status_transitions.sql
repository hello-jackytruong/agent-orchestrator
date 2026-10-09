-- name: InsertSessionStatusTransition :one
INSERT INTO session_status_transitions (
    id, session_id, from_status, to_status, trigger_source,
    reason, metadata, started_at, ended_at, duration_ms, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, session_id, from_status, to_status, trigger_source,
          reason, metadata, started_at, ended_at, duration_ms, created_at;

-- name: CloseSessionStatusTransition :execrows
UPDATE session_status_transitions
SET ended_at = ?, duration_ms = ?
WHERE id = ? AND ended_at IS NULL;

-- name: GetLatestSessionStatusTransition :one
SELECT id, session_id, from_status, to_status, trigger_source,
       reason, metadata, started_at, ended_at, duration_ms, created_at
FROM session_status_transitions
WHERE session_id = ?
ORDER BY started_at DESC, rowid DESC
LIMIT 1;

-- name: ListSessionStatusTransitions :many
SELECT id, session_id, from_status, to_status, trigger_source,
       reason, metadata, started_at, ended_at, duration_ms, created_at
FROM session_status_transitions
WHERE session_id = ?
ORDER BY started_at DESC, rowid DESC
LIMIT ? OFFSET ?;

-- name: CountSessionStatusTransitions :one
SELECT COUNT(*)
FROM session_status_transitions
WHERE session_id = ?;
