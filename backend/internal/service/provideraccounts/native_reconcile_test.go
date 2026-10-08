package provideraccounts

import (
	"context"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type nativeSourceFake struct {
	inputs   map[string]ports.NativeProviderCredential
	verified map[string]ports.VerifiedProviderLogin
	calls    int
}

func (f *nativeSourceFake) ReadNativeAccount(_ context.Context, provider string) (ports.NativeProviderCredential, error) {
	f.calls++
	return f.inputs[provider], nil
}
func (f *nativeSourceFake) ImportNativeAccount(_ context.Context, provider string, _ ports.NativeProviderCredential) (ports.VerifiedProviderLogin, error) {
	return f.verified[provider], nil
}

func TestRefreshNativeAccountsImportsOnceAndPreservesExplicitRemoval(t *testing.T) {
	h := setupAccounts(t)
	source := &nativeSourceFake{inputs: map[string]ports.NativeProviderCredential{"codex": {Fingerprint: "native-one"}}, verified: map[string]ports.VerifiedProviderLogin{"codex": {Provider: "codex", Email: "native@example.test", Kind: "oauth", CredentialRef: "native.json", AuthID: "native-auth"}}}
	h.svc.SetNativeAccountSource(source)
	if err := h.svc.RefreshNativeAccounts(h.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := h.svc.State(h.ctx)
	if err != nil || len(state.Accounts) != 1 || !state.Accounts[0].Global {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	id := state.Accounts[0].ID
	if err := h.svc.Remove(h.ctx, id, "", true); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.RefreshNativeAccounts(h.ctx); err != nil {
		t.Fatal(err)
	}
	state, err = h.svc.State(h.ctx)
	if err != nil || len(state.Accounts) != 1 || state.Accounts[0].CredentialRef != "" {
		t.Fatalf("native refresh undid sign-out: state=%+v err=%v", state, err)
	}
	if source.calls != 4 {
		t.Fatalf("expected both providers on each refresh, calls=%d", source.calls)
	}
}
