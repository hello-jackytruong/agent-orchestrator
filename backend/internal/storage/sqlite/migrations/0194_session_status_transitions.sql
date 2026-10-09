-- +goose Up
CREATE TABLE session_status_transitions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status TEXT NOT NULL,
    trigger_source TEXT NOT NULL,
    reason TEXT,
    metadata TEXT NOT NULL DEFAULT '{}',
    started_at TIMESTAMP NOT NULL,
    ended_at TIMESTAMP,
    duration_ms INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_session_status_transitions_session_started
ON session_status_transitions(session_id, started_at ASC);

-- +goose StatementBegin
CREATE TRIGGER session_status_transitions_cdc_insert
AFTER INSERT ON session_status_transitions
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT s.project_id, s.id, 'session_updated',
           json_object('id', s.id, 'sessionId', s.id,
                       'timelineTransitionId', NEW.id,
                       'activity', s.activity_state,
                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),
           NEW.created_at
    FROM sessions s WHERE s.id = NEW.session_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER session_status_transitions_cdc_update
AFTER UPDATE OF ended_at, duration_ms ON session_status_transitions
WHEN OLD.ended_at IS NOT NEW.ended_at
    OR OLD.duration_ms IS NOT NEW.duration_ms
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT s.project_id, s.id, 'session_updated',
           json_object('id', s.id, 'sessionId', s.id,
                       'timelineTransitionId', NEW.id,
                       'activity', s.activity_state,
                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),
           COALESCE(NEW.ended_at, NEW.created_at)
    FROM sessions s WHERE s.id = NEW.session_id;
END;
-- +goose StatementEnd

-- Baseline backfill for pre-existing sessions
INSERT INTO session_status_transitions (
    id, session_id, from_status, to_status, trigger_source, reason, started_at, created_at
)
SELECT 
    lower(hex(randomblob(16))),
    id,
    NULL,
    COALESCE(activity_state, 'idle'),
    'system',
    'Baseline status backfill',
    COALESCE(created_at, CURRENT_TIMESTAMP),
    CURRENT_TIMESTAMP
FROM sessions;

-- +goose Down
DROP TRIGGER IF EXISTS session_status_transitions_cdc_update;
DROP TRIGGER IF EXISTS session_status_transitions_cdc_insert;
DROP TABLE session_status_transitions;
