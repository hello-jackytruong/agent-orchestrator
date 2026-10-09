package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestSessionStatusTransitionsStore(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// Seed project and session
	seedProject(t, st, "proj-1")
	rec := sampleRecord("proj-1")
	rec.ID = "sess-1"
	sess, err := st.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)

	// 1. Insert first transition
	t1, err := st.InsertSessionStatusTransition(ctx, domain.SessionStatusTransition{
		ID:            "trans-1",
		SessionID:     sess.ID,
		FromStatus:    nil,
		ToStatus:      "provisioning",
		TriggerSource: "system",
		StartedAt:     now,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("insert transition 1: %v", err)
	}
	if t1.ID != "trans-1" || t1.ToStatus != "provisioning" || t1.FromStatus != nil {
		t.Fatalf("unexpected t1: %+v", t1)
	}

	// 2. Close first transition
	t1End := now.Add(5 * time.Second)
	duration := int64(5000)
	if err := st.CloseSessionStatusTransition(ctx, "trans-1", t1End, duration); err != nil {
		t.Fatalf("close transition 1: %v", err)
	}

	// 3. Insert second transition
	fromStatus := "provisioning"
	reason := "Agent ready"
	t2, err := st.InsertSessionStatusTransition(ctx, domain.SessionStatusTransition{
		ID:            "trans-2",
		SessionID:     sess.ID,
		FromStatus:    &fromStatus,
		ToStatus:      "active",
		TriggerSource: "agent",
		Reason:        &reason,
		StartedAt:     t1End,
		CreatedAt:     t1End,
	})
	if err != nil {
		t.Fatalf("insert transition 2: %v", err)
	}
	if t2.ID != "trans-2" || t2.ToStatus != "active" || t2.FromStatus == nil || *t2.FromStatus != "provisioning" {
		t.Fatalf("unexpected t2: %+v", t2)
	}

	// 4. GetLatestSessionStatusTransition
	latest, ok, err := st.GetLatestSessionStatusTransition(ctx, sess.ID)
	if err != nil || !ok {
		t.Fatalf("get latest: ok=%v, err=%v", ok, err)
	}
	if latest.ID != "trans-2" || latest.ToStatus != "active" {
		t.Fatalf("latest = %+v, want trans-2", latest)
	}

	// 5. ListSessionStatusTransitions (note: session creation also seeded 1 baseline transition via backfill in production, but in fresh create it's only what we inserted or what create inserted)
	items, total, err := st.ListSessionStatusTransitions(ctx, sess.ID, 10, 0)
	if err != nil {
		t.Fatalf("list transitions: %v", err)
	}
	if total < 2 || len(items) < 2 {
		t.Fatalf("expected at least 2 transitions, got len=%d, total=%d", len(items), total)
	}
}
