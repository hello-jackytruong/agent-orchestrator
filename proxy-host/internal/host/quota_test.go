package host

import (
	"context"
	"testing"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

func TestQuotaResultPolicyPersistsOnlyCodexUsageLimit(t *testing.T) {
	path := t.TempDir() + "/routes.json"
	routes, err := OpenRoutes(path)
	if err != nil {
		t.Fatal(err)
	}
	policy := quotaResultPolicy{routes: routes}
	policy.ApplyResultPolicy(context.Background(), coreauth.Result{Provider: "codex", AuthID: "a", Error: &coreauth.Error{Code: "rate_limit_exceeded"}})
	policy.ApplyResultPolicy(context.Background(), coreauth.Result{Provider: "claude", AuthID: "b", Error: &coreauth.Error{Code: "usage_limit_reached"}})
	retry := time.Hour
	policy.ApplyResultPolicy(context.Background(), coreauth.Result{Provider: "codex", AuthID: "a", RetryAfter: &retry, Error: &coreauth.Error{Message: `{"error":{"type":"usage_limit_reached"}}`}})
	policy.ApplyResultPolicy(context.Background(), coreauth.Result{Provider: "codex", AuthID: "a", Error: &coreauth.Error{Code: "usage_limit_reached"}})
	events := routes.QuotaEvents()
	if len(events) != 1 || events[0].AuthID != "a" {
		t.Fatalf("quota events=%+v", events)
	}
	reopened, err := OpenRoutes(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.QuotaEvents(); len(got) != 1 || got[0].AuthID != "a" {
		t.Fatalf("reopened quota events=%+v", got)
	}
	if err := reopened.AckQuotaEvents([]string{events[0].ID}); err != nil {
		t.Fatal(err)
	}
	if len(reopened.QuotaEvents()) != 0 {
		t.Fatal("acknowledged quota event remains")
	}
}
