package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/lifecycle"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
	sessionsvc "github.com/aoagents/agent-orchestrator/backend/internal/service/session"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/sqlitetest"
)

func TestTimeline_EndToEndIntegration(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)

	// 1. Create a session in real SQLite storage
	created, err := store.CreateSession(ctx, domain.SessionRecord{
		Kind:    domain.KindWorker,
		Harness: domain.HarnessCodex,
		Mode:    domain.SessionModeTUI,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	sessionID := created.ID

	// 2. Initialize lifecycle manager with the store
	lm := lifecycle.New(store, nil)

	// 3. Mark spawned
	if err := lm.MarkSpawned(ctx, sessionID, domain.SessionMetadata{}); err != nil {
		t.Fatalf("MarkSpawned: %v", err)
	}

	// 4. Send activity signal -> active (Working)
	err = lm.ApplyActivitySignal(ctx, sessionID, ports.ActivitySignal{
		Valid:     true,
		State:     domain.ActivityActive,
		Timestamp: time.Now().Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyActivitySignal active: %v", err)
	}

	// 5. Send activity signal -> waiting_input (Needs input)
	err = lm.ApplyActivitySignal(ctx, sessionID, ports.ActivitySignal{
		Valid:     true,
		State:     domain.ActivityWaitingInput,
		Timestamp: time.Now().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ApplyActivitySignal waiting_input: %v", err)
	}

	// 6. Terminate session
	if err := lm.MarkTerminated(ctx, sessionID); err != nil {
		t.Fatalf("MarkTerminated: %v", err)
	}

	// 7. Query transitions via session service backed by the real store
	svc := sessionsvc.New(nil, store)
	transitions, total, err := svc.ListTransitions(ctx, sessionID, 100, 0)
	if err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}

	t.Logf("Total transitions recorded: %d", total)
	for i, tr := range transitions {
		t.Logf("Transition %d: %v -> %s (trigger: %s, duration: %v, endedAt: %v)",
			i, tr.FromStatus, tr.ToStatus, tr.TriggerSource, tr.DurationMs, tr.EndedAt)
	}

	if total < 4 {
		t.Fatalf("expected at least 4 transitions, got %d", total)
	}

	// Final transition must be terminated
	last := transitions[len(transitions)-1]
	if last.ToStatus != "terminated" {
		t.Errorf("expected final status to be terminated, got %s", last.ToStatus)
	}
	if last.TriggerSource != "system" {
		t.Errorf("expected final trigger source to be system, got %s", last.TriggerSource)
	}
}
