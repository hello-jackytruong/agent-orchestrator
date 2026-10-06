package provideraccounts

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

func TestClaudeDefaultScopePreservesOtherAccountsAndProcessEnvironment(t *testing.T) {
	for _, move := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-only", true: "move-existing"}[move], func(t *testing.T) {
			h := setupAccounts(t)
			a := h.login(t, "claude", "a@example.test")
			b := h.login(t, "claude", "b@example.test")
			c := h.login(t, "claude", "c@example.test")
			codex := h.login(t, "codex", "codex@example.test")
			for _, session := range []string{"busy", "idle", "explicit-a"} {
				h.assign(t, session, domain.HarnessClaudeCode, a)
			}
			h.assign(t, "explicit-c", domain.HarnessClaudeCode, c)
			h.assign(t, "codex", domain.HarnessCodex, codex)
			h.guard.busy["busy"] = true
			env, err := h.svc.LaunchAccountEnv(h.ctx, "busy")
			if err != nil {
				t.Fatal(err)
			}
			before, err := h.svc.State(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err = h.svc.SetPrimaryWithOptions(h.ctx, b, move); err != nil {
				t.Fatal(err)
			}
			want := a
			if move {
				want = b
			}
			for _, session := range []string{"busy", "idle", "explicit-a"} {
				h.route(t, session, want)
			}
			h.route(t, "explicit-c", c)
			h.route(t, "codex", codex)
			chosen, managed, err := h.svc.ResolveAccount(h.ctx, domain.HarnessClaudeCode, "")
			if err != nil || !managed || chosen != b {
				t.Fatalf("new session=%s managed=%v err=%v", chosen, managed, err)
			}
			nextEnv, err := h.svc.LaunchAccountEnv(h.ctx, "busy")
			if err != nil || !reflect.DeepEqual(env, nextEnv) {
				t.Fatalf("process config changed: %v", err)
			}
			after, err := h.svc.State(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if after.ClaudePrimaryGeneration != before.ClaudePrimaryGeneration+1 || after.CodexPrimaryGeneration != before.CodexPrimaryGeneration {
				t.Fatalf("generations before=%+v after=%+v", before.Primaries, after.Primaries)
			}
			if len(h.guard.acquired) != 0 || len(h.proxy.deleted) != 0 {
				t.Fatal("live rebind interrupted work or deleted credentials")
			}
		})
	}
}

func TestClaudeQuotaEventsAreProviderAndGenerationScoped(t *testing.T) {
	for _, tc := range []struct {
		name, provider, auth                       string
		generation                                 int64
		expired, disabled, wantSwitch, wantPending bool
	}{
		{name: "current", provider: "claude", auth: "a@example.test-auth", generation: 1, wantSwitch: true},
		{name: "other-account", provider: "claude", auth: "b@example.test-auth", generation: 1},
		{name: "old-generation", provider: "claude", auth: "a@example.test-auth", generation: 9},
		{name: "expired", provider: "claude", auth: "a@example.test-auth", generation: 1, expired: true},
		{name: "disabled", provider: "claude", auth: "a@example.test-auth", generation: 1, disabled: true, wantPending: true},
		{name: "missing-auth", provider: "claude", generation: 1},
		{name: "unknown-provider", provider: "other", auth: "a@example.test-auth", generation: 1},
		{name: "legacy-is-codex", auth: "a@example.test-auth", generation: 1},
		{name: "wrong-provider", provider: "codex", auth: "a@example.test-auth", generation: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := setupAccounts(t)
			a := h.login(t, "claude", "a@example.test")
			b := h.login(t, "claude", "b@example.test")
			codex := h.login(t, "codex", "codex-a@example.test")
			h.login(t, "codex", "codex-b@example.test")
			h.assign(t, "claude-session", domain.HarnessClaudeCode, a)
			h.assign(t, "codex-session", domain.HarnessCodex, codex)
			if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, !tc.disabled); err != nil {
				t.Fatal(err)
			}
			if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
				t.Fatal(err)
			}
			reset := time.Now().Add(time.Hour).Unix()
			if tc.expired {
				reset = time.Now().Add(-time.Hour).Unix()
			}
			h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: tc.name, Provider: tc.provider, AuthID: tc.auth, Generation: tc.generation, ResetAt: reset}}
			if err := h.svc.ProcessQuotaEvents(h.ctx); err != nil {
				t.Fatal(err)
			}
			want := a
			if tc.wantSwitch {
				want = b
			}
			h.route(t, "claude-session", want)
			h.route(t, "codex-session", codex)
			state, err := h.svc.State(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got, _ := primary(state, "claude"); got != want {
				t.Fatalf("default=%s want=%s", got, want)
			}
			if got, _ := primary(state, "codex"); got != codex {
				t.Fatal("Claude event changed Codex default")
			}
			if (len(h.proxy.quotaEvents) == 1) != tc.wantPending {
				t.Fatalf("pending=%+v", h.proxy.quotaEvents)
			}
		})
	}
}

func TestClaudeQuotaPreferenceRequiresSameProviderAndDisablesAfterSignOut(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "claude", "a@example.test")
	h.login(t, "codex", "codex-a@example.test")
	h.login(t, "codex", "codex-b@example.test")
	if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, true); !errors.Is(err, ports.ErrProviderQuotaSwitchRequiresReplacement) {
		t.Fatalf("mixed-provider replacement accepted: %v", err)
	}
	b := h.login(t, "claude", "b@example.test")
	if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.SetCodexQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Remove(h.ctx, b, "", true); err != nil {
		t.Fatal(err)
	}
	enabled, err := h.svc.ClaudeQuotaAutoSwitch(h.ctx)
	if err != nil || enabled {
		t.Fatalf("Claude enabled=%v err=%v", enabled, err)
	}
	codexEnabled, err := h.svc.CodexQuotaAutoSwitch(h.ctx)
	if err != nil || !codexEnabled {
		t.Fatalf("Codex preference changed: %v", err)
	}
	h.login(t, "claude", "c@example.test")
	enabled, err = h.svc.ClaudeQuotaAutoSwitch(h.ctx)
	if err != nil || enabled {
		t.Fatal("adding an account silently re-enabled quota switching")
	}
	if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	state, err := h.svc.State(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := primary(state, "claude"); got != a {
		t.Fatal("preference changed default")
	}
}

func TestClaudeQuotaGenerationRejectsDelayedEventAfterDefaultReturns(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "claude", "a@example.test")
	b := h.login(t, "claude", "b@example.test")
	if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	h.assign(t, "session", domain.HarnessClaudeCode, a)
	before, _ := h.svc.State(h.ctx)
	if err := h.svc.SetPrimaryWithOptions(h.ctx, b, true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.SetPrimaryWithOptions(h.ctx, a, true); err != nil {
		t.Fatal(err)
	}
	h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: "old-a", Provider: "claude", AuthID: "a@example.test-auth", Generation: before.ClaudePrimaryGeneration}}
	if err := h.svc.ProcessQuotaEvents(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.route(t, "session", a)
	if len(h.proxy.quotaEvents) != 0 {
		t.Fatal("stale event not acknowledged")
	}
}

func TestLegacyProviderGenerationAdvancesOnFirstDefaultChange(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			h := setupAccounts(t)
			h.login(t, provider, "a@example.test")
			b := h.login(t, provider, "b@example.test")
			// Saved account state predates provider generation tracking.
			h.store.state.CodexPrimaryGeneration = 0
			h.store.state.ClaudePrimaryGeneration = 0
			h.proxy.snapshot = snapshot(h.store.state)
			if err := h.svc.SetPrimary(h.ctx, b); err != nil {
				t.Fatal(err)
			}
			state, err := h.svc.State(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := primaryGeneration(state, provider); got != 2 {
				t.Fatalf("generation=%d want=2", got)
			}
		})
	}
}

func TestClaudeQuotaRecoveryAcrossMutationFailures(t *testing.T) {
	for _, failure := range []string{"save", "apply", "commit", "finish"} {
		t.Run(failure, func(t *testing.T) {
			h := setupAccounts(t)
			a := h.login(t, "claude", "a@example.test")
			b := h.login(t, "claude", "b@example.test")
			h.assign(t, "working", domain.HarnessClaudeCode, a)
			h.guard.busy["working"] = true
			if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, true); err != nil {
				t.Fatal(err)
			}
			before, _ := h.svc.State(h.ctx)
			h.proxy.quotaEvents = []ports.ProviderQuotaEvent{{ID: "quota-a", Provider: "claude", AuthID: "a@example.test-auth", Generation: before.ClaudePrimaryGeneration}}
			if failure == "apply" {
				h.proxy.fail = failure
			} else {
				h.store.fail = failure
			}
			if err := h.svc.ProcessQuotaEvents(h.ctx); err != nil {
				t.Fatal(err)
			}
			if len(h.proxy.quotaEvents) != 1 {
				t.Fatal("failed operation lost its event")
			}
			if len(h.proxy.deleted) != 0 {
				t.Fatal("quota switch deleted credentials")
			}
			h.store.fail = ""
			h.proxy.fail = ""
			h.svc = New(h.store, h.proxy, h.guard, []byte(strings.Repeat("k", 32)), "http://127.0.0.1:1234", h.svc.newID)
			if err := h.svc.Recover(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := h.svc.ProcessQuotaEvents(h.ctx); err != nil {
				t.Fatal(err)
			}
			h.route(t, "working", b)
			state, err := h.svc.State(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if state.ClaudePrimaryGeneration != before.ClaudePrimaryGeneration+1 {
				t.Fatalf("replayed switch twice: %d", state.ClaudePrimaryGeneration)
			}
			if len(h.proxy.quotaEvents) != 0 || len(h.proxy.ackedQuota) != 1 {
				t.Fatal("recovery did not acknowledge event once")
			}
			if len(h.guard.acquired) != 0 {
				t.Fatal("recovery interrupted active session")
			}
		})
	}
}

func TestClaudeQuotaPreferenceRecheckedAtMutationBoundary(t *testing.T) {
	h := setupAccounts(t)
	a := h.login(t, "claude", "a@example.test")
	h.login(t, "claude", "b@example.test")
	h.assign(t, "session", domain.HarnessClaudeCode, a)
	if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, true); err != nil {
		t.Fatal(err)
	}
	state, _ := h.svc.State(h.ctx)
	event := ports.ProviderQuotaEvent{ID: "quota", Provider: "claude", AuthID: "a@example.test-auth", Generation: state.ClaudePrimaryGeneration}
	// Simulate the user disabling recovery after the poll read the preference.
	if err := h.svc.SetClaudeQuotaAutoSwitch(h.ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.switchPrimaryForQuota(h.ctx, "claude", event); !errors.Is(err, errQuotaReplacementUnavailable) {
		t.Fatalf("disabled preference bypassed: %v", err)
	}
	h.route(t, "session", a)
}
