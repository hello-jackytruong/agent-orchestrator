package provideraccounts

import (
	"errors"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestCodexQuotaAutoSwitchRequiresTwoSignedInAccounts(t *testing.T) {
	h := setupAccounts(t)
	h.login(t, "codex", "a@test.example")
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); !errors.Is(err, ports.ErrProviderQuotaSwitchRequiresReplacement) {
		t.Fatalf("enable with one account error=%v", err)
	}
	state, err := h.svc.State(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.CodexQuotaAutoSwitch {
		t.Fatal("quota auto-switch enabled without a replacement account")
	}
}

func TestCodexQuotaAutoSwitchMovesPreviousPrimaryRoutesAndLeavesOtherAccounts(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "codex", "a@test.example")
	b := h.login(t, "codex", "b@test.example")
	c := h.login(t, "codex", "c@test.example")
	h.assign(t, "default", domain.HarnessCodex, a)
	h.assign(t, "explicit-a", domain.HarnessCodex, a)
	h.assign(t, "other", domain.HarnessCodex, c)
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: "quota-a", AuthID: "a@test.example-auth", ResetAt: time.Now().Add(time.Hour).Unix()}}
	if err := h.svc.ProcessCodexQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"default", "explicit-a"} {
		h.route(t, session, b)
	}
	h.route(t, "other", c)
	state, err := h.svc.State(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := primary(state, "codex"); got != b {
		t.Fatalf("primary=%q, want %q", got, b)
	}
	if len(h.proxy.quotaEvents) != 0 || len(h.proxy.ackedQuota) != 1 {
		t.Fatalf("quota events=%+v acknowledgements=%v", h.proxy.quotaEvents, h.proxy.ackedQuota)
	}
}

func TestCodexQuotaAutoSwitchIgnoresStaleAndUnavailableEvents(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "codex", "a@test.example")
	h.login(t, "codex", "b@test.example")
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ProcessCodexQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: "stale", AuthID: "unknown"}, {ID: "expired", AuthID: "a@test.example-auth", ResetAt: time.Now().Add(-time.Minute).Unix()}}
	if err := h.svc.ProcessCodexQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := h.svc.State(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := primary(state, "codex"); got != a {
		t.Fatalf("stale event changed primary to %q", got)
	}
	if len(h.proxy.quotaEvents) != 0 {
		t.Fatalf("stale events were not acknowledged: %+v", h.proxy.quotaEvents)
	}
}

func TestCodexQuotaAutoSwitchDisabledLeavesPendingEventAndPrimaryUnchanged(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "codex", "a@test.example")
	h.login(t, "codex", "b@test.example")
	h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: "quota-a", AuthID: "a@test.example-auth"}}
	if err := h.svc.ProcessCodexQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := h.svc.State(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := primary(state, "codex"); got != a || len(h.proxy.quotaEvents) != 1 {
		t.Fatalf("disabled recovery changed primary=%q events=%+v", got, h.proxy.quotaEvents)
	}
}

func TestCodexQuotaAutoSwitchDoesNotFilterReplacementByPendingQuota(t *testing.T) {
	h := setupAccounts(t)
	h.login(t, "codex", "a@test.example")
	b := h.login(t, "codex", "b@test.example")
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	h.proxy.quotaEvents = []ports.ProviderQuotaEvent{
		{ID: "quota-a", AuthID: "a@test.example-auth"},
		{ID: "quota-b", AuthID: "b@test.example-auth"},
	}
	if err := h.svc.ProcessCodexQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := h.svc.State(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := primary(state, "codex"); got != b {
		t.Fatalf("primary=%q, want replacement %q", got, b)
	}
	if len(h.proxy.quotaEvents) != 1 || h.proxy.quotaEvents[0].ID != "quota-b" {
		t.Fatalf("second quota event should remain for the next poll: %+v", h.proxy.quotaEvents)
	}
	if len(h.proxy.ackedQuota) != 1 || h.proxy.ackedQuota[0] != "quota-a" {
		t.Fatalf("acknowledged=%v, want only quota-a", h.proxy.ackedQuota)
	}
}

func TestCodexQuotaAutoSwitchDisablesWhenSecondAccountIsSignedOut(t *testing.T) {
	h := setupAccounts(t)
	h.login(t, "codex", "a@test.example")
	b := h.login(t, "codex", "b@test.example")
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Remove(h.ctx, b, "", true); err != nil {
		t.Fatal(err)
	}
	enabled, err := h.svc.CodexQuotaAutoSwitch(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("quota auto-switch stayed enabled with one signed-in account")
	}
}

func TestCodexQuotaAutoSwitchRejectsDelayedEventAfterPrimaryCycles(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "codex", "a@test.example")
	b := h.login(t, "codex", "b@test.example")
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	state, _ := h.svc.State(h.ctx)
	generation := state.CodexPrimaryGeneration
	if err := h.svc.SetPrimary(h.ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.SetPrimary(h.ctx, a); err != nil {
		t.Fatal(err)
	}
	h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: "delayed", AuthID: "a@test.example-auth", Generation: generation}}
	if err := h.svc.ProcessCodexQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	state, _ = h.svc.State(h.ctx)
	if got, _ := primary(state, "codex"); got != a {
		t.Fatalf("delayed event changed primary to %q", got)
	}
	if len(h.proxy.quotaEvents) != 0 {
		t.Fatal("delayed event was not acknowledged")
	}
}
