package sqlite

import (
	"strings"
	"testing"
	"time"
)

func TestMigrateSessionStatusTransitions(t *testing.T) {
	// Start with database at version 193
	db := openMigratedDatabaseCopy(t, 193)

	// Seed a test project and sessions to test baseline backfill
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
INSERT INTO projects (id, path, registered_at, config) VALUES ('proj-1', '/repo/test', ?, '{}');
INSERT INTO sessions (id, project_id, num, harness, activity_state, activity_last_at, workspace_path, branch, created_at, updated_at)
VALUES 
    ('sess-1', 'proj-1', 1, 'codex', 'active', ?, '/tmp/sess-1', 'main', ?, ?),
    ('sess-2', 'proj-1', 2, 'claude-code', 'idle', ?, '/tmp/sess-2', 'main', ?, ?);
`, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("failed to seed test sessions: %v", err)
	}

	body, err := migrationsFS.ReadFile("migrations/0194_session_status_transitions.sql")
	if err != nil {
		t.Fatalf("read 0192 migration file: %v", err)
	}

	parts := strings.Split(string(body), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatalf("expected up and down sections in migration 0192")
	}
	upSQL := strings.TrimPrefix(parts[0], "-- +goose Up\n")
	downSQL := strings.TrimSpace(parts[1])

	// Apply Up migration
	if _, err := db.Exec(upSQL); err != nil {
		t.Fatalf("apply 0192 up migration: %v", err)
	}

	// Verify table columns
	expectedColumns := []string{
		"id", "session_id", "from_status", "to_status", "trigger_source",
		"reason", "metadata", "started_at", "ended_at", "duration_ms", "created_at",
	}
	cols := tableColumns(t, db, "session_status_transitions")
	for _, expected := range expectedColumns {
		found := false
		for _, c := range cols {
			if c == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected column %s in session_status_transitions; found: %v", expected, cols)
		}
	}

	// Verify backfill rows were created for both seeded sessions
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM session_status_transitions`).Scan(&count); err != nil {
		t.Fatalf("count transitions: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 backfilled transitions, got %d", count)
	}

	// Verify sess-1 transition
	var toStatus, triggerSource string
	if err := db.QueryRow(`SELECT to_status, trigger_source FROM session_status_transitions WHERE session_id = 'sess-1'`).Scan(&toStatus, &triggerSource); err != nil {
		t.Fatalf("query sess-1 transition: %v", err)
	}
	if toStatus != "active" || triggerSource != "system" {
		t.Fatalf("sess-1 transition = (%q, %q), want ('active', 'system')", toStatus, triggerSource)
	}

	// Verify timeline writes emit the existing session_updated CDC event.
	var transitionID string
	if err := db.QueryRow(`SELECT id FROM session_status_transitions WHERE session_id = 'sess-1'`).Scan(&transitionID); err != nil {
		t.Fatalf("query sess-1 transition id: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM change_log WHERE session_id = 'sess-1'`); err != nil {
		t.Fatalf("clear change_log for sess-1 timeline CDC check: %v", err)
	}
	if _, err := db.Exec(`UPDATE session_status_transitions SET ended_at = ?, duration_ms = 1200 WHERE id = ?`, now, transitionID); err != nil {
		t.Fatalf("update transition for CDC check: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM change_log WHERE session_id = 'sess-1' AND event_type = 'session_updated' AND json_extract(payload, '$.timelineTransitionId') = ?`, transitionID).Scan(&count); err != nil {
		t.Fatalf("count transition CDC events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 transition CDC event, got %d", count)
	}

	// Verify cascade deletion
	if _, err := db.Exec(`DELETE FROM change_log WHERE session_id = 'sess-1'`); err != nil {
		t.Fatalf("clear change_log for sess-1: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM sessions WHERE id = 'sess-1'`); err != nil {
		t.Fatalf("delete sess-1: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM session_status_transitions WHERE session_id = 'sess-1'`).Scan(&count); err != nil {
		t.Fatalf("check cascade deletion: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 transitions for sess-1 after cascade delete, got %d", count)
	}

	// Apply Down migration
	if _, err := db.Exec(downSQL); err != nil {
		t.Fatalf("apply 0192 down migration: %v", err)
	}

	// Verify table is dropped
	var tableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'session_status_transitions'`).Scan(&tableCount); err != nil || tableCount != 0 {
		t.Fatalf("expected session_status_transitions to be dropped, count = %d, err = %v", tableCount, err)
	}
}
