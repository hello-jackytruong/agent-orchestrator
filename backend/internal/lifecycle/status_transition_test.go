package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type transitionFakeStore struct {
	*fakeStore
	transitions map[domain.SessionID][]domain.SessionStatusTransition
}

func newTransitionFakeStore() *transitionFakeStore {
	return &transitionFakeStore{
		fakeStore:   newFakeStore(),
		transitions: make(map[domain.SessionID][]domain.SessionStatusTransition),
	}
}

func (f *transitionFakeStore) InsertSessionStatusTransition(_ context.Context, t domain.SessionStatusTransition) (domain.SessionStatusTransition, error) {
	f.transitions[t.SessionID] = append(f.transitions[t.SessionID], t)
	return t, nil
}

func (f *transitionFakeStore) CloseSessionStatusTransition(_ context.Context, id string, endedAt time.Time, durationMs int64) error {
	for sessID, list := range f.transitions {
		for i, tr := range list {
			if tr.ID == id {
				tr.EndedAt = &endedAt
				tr.DurationMs = &durationMs
				f.transitions[sessID][i] = tr
				return nil
			}
		}
	}
	return nil
}

func (f *transitionFakeStore) GetLatestSessionStatusTransition(_ context.Context, sessionID domain.SessionID) (domain.SessionStatusTransition, bool, error) {
	list := f.transitions[sessionID]
	if len(list) == 0 {
		return domain.SessionStatusTransition{}, false, nil
	}
	return list[len(list)-1], true, nil
}

func TestRecordTransition_LifecycleFlow(t *testing.T) {
	store := newTransitionFakeStore()
	clk := time.Now().UTC().Truncate(time.Second)
	m := New(store, nil)
	m.clock = func() time.Time { return clk }

	sessID := domain.SessionID("sess-trans-1")
	store.sessions[sessID] = domain.SessionRecord{
		ID:        sessID,
		ProjectID: "proj-1",
		Harness:   domain.HarnessCodex,
		Activity: domain.Activity{
			State: domain.ActivityIdle,
		},
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1",
		},
	}

	// 1. Initial MarkSpawned
	if err := m.MarkSpawned(context.Background(), sessID, domain.SessionMetadata{RuntimeLaunchID: "launch-1"}); err != nil {
		t.Fatalf("MarkSpawned: %v", err)
	}

	trs := store.transitions[sessID]
	if len(trs) != 1 {
		t.Fatalf("expected 1 initial transition, got %d", len(trs))
	}
	if trs[0].ToStatus != "idle" || trs[0].TriggerSource != "system" {
		t.Fatalf("unexpected initial transition: %+v", trs[0])
	}

	// 2. Transition to active
	clk = clk.Add(10 * time.Second)
	signal := ports.ActivitySignal{
		LaunchID: "launch-1",
		Valid:    true,
		State:    domain.ActivityActive,
	}
	if err := m.ApplyActivitySignal(context.Background(), sessID, signal); err != nil {
		t.Fatalf("ApplyActivitySignal: %v", err)
	}

	trs = store.transitions[sessID]
	if len(trs) != 2 {
		t.Fatalf("expected 2 transitions after active signal, got %d", len(trs))
	}
	// Initial transition should be closed with ~10000ms duration
	if trs[0].EndedAt == nil || trs[0].DurationMs == nil || *trs[0].DurationMs != 10000 {
		t.Fatalf("expected initial transition closed with 10000ms, got: %+v", trs[0])
	}
	// New transition should be active
	if trs[1].ToStatus != "active" || trs[1].FromStatus == nil || *trs[1].FromStatus != "idle" {
		t.Fatalf("unexpected active transition: %+v", trs[1])
	}

	// 3. Heartbeat / duplicate signal should be deduplicated
	clk = clk.Add(5 * time.Second)
	if err := m.ApplyActivitySignal(context.Background(), sessID, signal); err != nil {
		t.Fatalf("ApplyActivitySignal duplicate: %v", err)
	}
	trs = store.transitions[sessID]
	if len(trs) != 2 {
		t.Fatalf("duplicate signal should not insert new transition, got %d", len(trs))
	}

	// 4. MarkTerminated
	clk = clk.Add(20 * time.Second)
	if err := m.MarkTerminated(context.Background(), sessID); err != nil {
		t.Fatalf("MarkTerminated: %v", err)
	}

	trs = store.transitions[sessID]
	if len(trs) != 3 {
		t.Fatalf("expected 3 transitions after termination, got %d", len(trs))
	}
	// Previous active transition should be closed with ~25000ms duration
	if trs[1].EndedAt == nil || trs[1].DurationMs == nil || *trs[1].DurationMs != 25000 {
		t.Fatalf("expected active transition closed with 25000ms, got: %+v", trs[1])
	}
	if trs[2].ToStatus != "terminated" {
		t.Fatalf("unexpected terminal transition: %+v", trs[2])
	}
}

func TestRuntimeObservation_WorkloadExitRecordsTransition(t *testing.T) {
	store := newTransitionFakeStore()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	m := New(store, nil)
	m.clock = func() time.Time { return now }

	sessID := domain.SessionID("sess-trans-workload-exit")
	store.sessions[sessID] = domain.SessionRecord{
		ID:       sessID,
		Activity: domain.Activity{State: domain.ActivityActive, LastActivityAt: now.Add(-2 * time.Minute)},
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1",
		},
	}
	store.transitions[sessID] = []domain.SessionStatusTransition{{
		ID:            "tr-active",
		SessionID:     sessID,
		ToStatus:      string(domain.ActivityActive),
		TriggerSource: "agent",
		StartedAt:     now.Add(-30 * time.Second),
		CreatedAt:     now.Add(-30 * time.Second),
	}}

	observedAt := now.Add(5 * time.Second)
	if err := m.ApplyRuntimeObservation(context.Background(), sessID, ports.RuntimeFacts{
		Runtime:    ports.ProbeAlive,
		Workload:   ports.ProbeDead,
		LaunchID:   "launch-1",
		ObservedAt: observedAt,
	}); err != nil {
		t.Fatalf("ApplyRuntimeObservation: %v", err)
	}

	trs := store.transitions[sessID]
	if len(trs) != 2 {
		t.Fatalf("expected workload exit transition, got %d transitions: %+v", len(trs), trs)
	}
	if trs[0].EndedAt == nil || !trs[0].EndedAt.Equal(observedAt) {
		t.Fatalf("expected active transition closed at observation time, got %+v", trs[0])
	}
	if trs[1].ToStatus != string(domain.ActivityExited) || trs[1].TriggerSource != "system" {
		t.Fatalf("unexpected workload exit transition: %+v", trs[1])
	}
	if trs[1].FromStatus == nil || *trs[1].FromStatus != string(domain.ActivityActive) {
		t.Fatalf("workload exit transition from status = %v, want active", trs[1].FromStatus)
	}
	if !trs[1].StartedAt.Equal(observedAt) {
		t.Fatalf("workload exit transition started_at = %s, want %s", trs[1].StartedAt, observedAt)
	}
}

func TestRuntimeObservation_RuntimeDeathRecordsTransition(t *testing.T) {
	store := newTransitionFakeStore()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	m := New(store, nil)
	m.clock = func() time.Time { return now }

	sessID := domain.SessionID("sess-trans-runtime-death")
	store.sessions[sessID] = domain.SessionRecord{
		ID:       sessID,
		Activity: domain.Activity{State: domain.ActivityActive, LastActivityAt: now.Add(-2 * time.Minute)},
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1",
		},
	}
	store.transitions[sessID] = []domain.SessionStatusTransition{{
		ID:            "tr-active",
		SessionID:     sessID,
		ToStatus:      string(domain.ActivityActive),
		TriggerSource: "agent",
		StartedAt:     now.Add(-45 * time.Second),
		CreatedAt:     now.Add(-45 * time.Second),
	}}

	observedAt := now.Add(10 * time.Second)
	if err := m.ApplyRuntimeObservation(context.Background(), sessID, ports.RuntimeFacts{
		Runtime:    ports.ProbeDead,
		Workload:   ports.ProbeFailed,
		LaunchID:   "launch-1",
		ObservedAt: observedAt,
	}); err != nil {
		t.Fatalf("ApplyRuntimeObservation: %v", err)
	}

	trs := store.transitions[sessID]
	if len(trs) != 2 {
		t.Fatalf("expected runtime death transition, got %d transitions: %+v", len(trs), trs)
	}
	if trs[0].EndedAt == nil || !trs[0].EndedAt.Equal(observedAt) {
		t.Fatalf("expected active transition closed at observation time, got %+v", trs[0])
	}
	if trs[1].ToStatus != "terminated" || trs[1].TriggerSource != "system" {
		t.Fatalf("unexpected runtime death transition: %+v", trs[1])
	}
	if trs[1].FromStatus == nil || *trs[1].FromStatus != string(domain.ActivityActive) {
		t.Fatalf("runtime death transition from status = %v, want active", trs[1].FromStatus)
	}
	if !trs[1].StartedAt.Equal(observedAt) {
		t.Fatalf("runtime death transition started_at = %s, want %s", trs[1].StartedAt, observedAt)
	}
}
