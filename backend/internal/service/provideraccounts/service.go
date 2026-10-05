// Package provideraccounts owns managed account defaults and session assignments.
package provideraccounts

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// Service owns managed provider defaults, session assignments, and mutation recovery.
type Service struct {
	store                 ports.ProviderAccountStore
	proxy                 ports.ProviderAccountProxy
	guard                 ports.ProviderAccountSessionGuard
	codexRequestSwitching bool
	gate                  chan struct{}
	ticketKey             []byte
	endpoint              string
	newID                 func() string
	usageMu               sync.Mutex
	usageCache            map[string]cachedUsage
}

type cachedUsage struct {
	value     domain.ProviderAccountUsage
	expiresAt time.Time
}

const providerUsageCacheTTL = 2 * time.Minute

// EnableCodexRequestSwitching is a startup-only opt-in pending live continuity verification.
func (s *Service) EnableCodexRequestSwitching() { s.codexRequestSwitching = true }
func (s *Service) CodexRequestSwitching() bool  { return s.codexRequestSwitching }

// CodexQuotaAutoSwitch reports whether confirmed Codex quota exhaustion may
// move the current primary to another signed-in account.
func (s *Service) CodexQuotaAutoSwitch(ctx context.Context) (bool, error) {
	state, err := s.State(ctx)
	return state.CodexQuotaAutoSwitch && signedInCount(state, "codex") >= 2, err
}

// SetCodexQuotaAutoSwitch persists the user's quota recovery preference.
func (s *Service) SetCodexQuotaAutoSwitch(ctx context.Context, enabled bool) error {
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		if enabled && signedInCount(*state, "codex") < 2 {
			return "", ports.ErrProviderQuotaSwitchRequiresReplacement
		}
		state.CodexQuotaAutoSwitch = enabled
		return "", nil
	})
}

var errQuotaEventStale = errors.New("quota event is stale")
var errQuotaReplacementUnavailable = errors.New("no signed-in replacement account is available")

// ProcessCodexQuotaEvents applies confirmed quota events to the current
// primary. It deliberately changes only the primary and routes that currently
// use it; sessions on other accounts are untouched.
func (s *Service) ProcessCodexQuotaEvents(ctx context.Context) error {
	quotaProxy, ok := s.proxy.(ports.ProviderQuotaEvents)
	if !ok {
		return nil
	}
	events, err := quotaProxy.QuotaEvents(ctx)
	if err != nil {
		return err
	}
	state, err := s.State(ctx)
	if err != nil || !state.CodexQuotaAutoSwitch || signedInCount(state, "codex") < 2 {
		return err
	}
	var acknowledged []string
	for _, event := range events {
		if event.AuthID == "" {
			acknowledged = append(acknowledged, event.ID)
			continue
		}
		if event.ResetAt > 0 && event.ResetAt <= time.Now().Unix() {
			acknowledged = append(acknowledged, event.ID)
			continue
		}
		if err := s.switchPrimaryForQuota(ctx, event); err != nil {
			if errors.Is(err, errQuotaEventStale) {
				acknowledged = append(acknowledged, event.ID)
			}
			continue
		}
		acknowledged = append(acknowledged, event.ID)
		// Apply at most one switch per poll. Other events were observed against
		// the previous primary generation and must be re-evaluated next time.
		break
	}
	return quotaProxy.AckQuotaEvents(ctx, acknowledged)
}

func (s *Service) switchPrimaryForQuota(ctx context.Context, event ports.ProviderQuotaEvent) error {
	return s.mutateWithBoundary(ctx, true, func(state *domain.ProviderAccountState) (string, error) {
		currentGeneration := state.CodexPrimaryGeneration
		if currentGeneration == 0 {
			if current, exists := primary(*state, "codex"); exists && current != "" {
				currentGeneration = 1
			}
		}
		if event.Generation > 0 && currentGeneration != event.Generation {
			return "", errQuotaEventStale
		}
		current, ok := primary(*state, "codex")
		if !ok || current == "" {
			return "", errQuotaEventStale
		}
		accountUsingEvent, ok := account(*state, current)
		if !ok || accountUsingEvent.AuthID != event.AuthID {
			return "", errQuotaEventStale
		}
		replacement := ""
		for _, candidate := range state.Accounts {
			if candidate.Provider != "codex" || candidate.ID == current || candidate.CredentialRef == "" || candidate.AuthID == "" {
				continue
			}
			replacement = candidate.ID
			break
		}
		if replacement == "" {
			return "", errQuotaReplacementUnavailable
		}
		for i, route := range state.Routes {
			if route.Provider == "codex" && route.AccountID == current {
				state.Routes[i].AccountID = replacement
			}
		}
		setPrimary(state, "codex", replacement)
		return "", nil
	})
}

// New creates the account service with a stable private ticket identity.
func New(store ports.ProviderAccountStore, proxy ports.ProviderAccountProxy, guard ports.ProviderAccountSessionGuard, key []byte, endpoint string, newID func() string) *Service {
	return &Service{store: store, proxy: proxy, guard: guard, gate: make(chan struct{}, 1), ticketKey: append([]byte(nil), key...), endpoint: endpoint, newID: newID, usageCache: make(map[string]cachedUsage)}
}

// AccountUsages returns safe, best-effort quota summaries for signed-in
// accounts. Account inventory remains usable when a provider cannot report
// usage, so each failed lookup becomes an explicit unavailable state.
func (s *Service) AccountUsages(ctx context.Context, accounts []domain.ProviderAccount) map[string]domain.ProviderAccountUsage {
	result := make(map[string]domain.ProviderAccountUsage)
	proxy, ok := s.proxy.(ports.ProviderAccountUsageProxy)
	if !ok {
		for _, account := range accounts {
			if account.CredentialRef != "" && account.AuthID != "" {
				result[account.ID] = domain.ProviderAccountUsage{Status: "unavailable", Message: "Usage unavailable", CheckedAt: time.Now().UTC()}
			}
		}
		return result
	}
	now := time.Now()
	for _, account := range accounts {
		if account.CredentialRef == "" || account.AuthID == "" {
			continue
		}
		key := account.ID + "\x00" + account.AuthID
		s.usageMu.Lock()
		cached, found := s.usageCache[key]
		s.usageMu.Unlock()
		if found && now.Before(cached.expiresAt) {
			result[account.ID] = cached.value
			continue
		}
		usage, err := proxy.FetchAccountUsage(ctx, account.Provider, account.AuthID, account.CredentialRef)
		if err != nil {
			usage = domain.ProviderAccountUsage{Status: "unavailable", Message: "Usage unavailable", CheckedAt: now.UTC()}
		}
		if usage.Status == "" {
			usage.Status = "available"
		}
		if usage.CheckedAt.IsZero() {
			usage.CheckedAt = now.UTC()
		}
		s.usageMu.Lock()
		s.usageCache[key] = cachedUsage{value: usage, expiresAt: now.Add(providerUsageCacheTTL)}
		s.usageMu.Unlock()
		result[account.ID] = usage
	}
	return result
}

// Provider maps supported harnesses to their account provider.
func Provider(h domain.AgentHarness) string {
	switch h {
	case domain.HarnessCodex:
		return "codex"
	case domain.HarnessClaudeCode:
		return "claude"
	}
	return ""
}
func (s *Service) lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.gate <- struct{}{}:
		return func() { <-s.gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// State reads durable account facts.
func (s *Service) State(ctx context.Context) (domain.ProviderAccountState, error) {
	state, _, err := s.store.LoadProviderAccountState(ctx)
	return state, err
}
func account(state domain.ProviderAccountState, id string) (domain.ProviderAccount, bool) {
	for _, a := range state.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	return domain.ProviderAccount{}, false
}
func signedInCount(state domain.ProviderAccountState, provider string) int {
	count := 0
	for _, a := range state.Accounts {
		if a.Provider == provider && a.CredentialRef != "" && a.AuthID != "" {
			count++
		}
	}
	return count
}
func primary(state domain.ProviderAccountState, provider string) (string, bool) {
	for _, p := range state.Primaries {
		if p.Provider == provider {
			return p.PrimaryID, true
		}
	}
	return "", false
}
func setPrimary(state *domain.ProviderAccountState, provider, id string) {
	previous, _ := primary(*state, provider)
	if provider == "codex" && previous != id {
		state.CodexPrimaryGeneration++
	}
	for i, p := range state.Primaries {
		if p.Provider == provider {
			state.Primaries[i].PrimaryID = id
			return
		}
	}
	state.Primaries = append(state.Primaries, domain.ProviderPrimary{Provider: provider, PrimaryID: id})
}
func eligible(state domain.ProviderAccountState, id, provider string) error {
	a, found := account(state, id)
	if !found {
		return ports.ErrProviderAccountUnknown
	}
	if a.Provider != provider {
		return ports.ErrProviderAccountIncompatible
	}
	if a.CredentialRef == "" || a.AuthID == "" {
		return ports.ErrProviderLoginRequired
	}
	return nil
}

// ResolveAccount chooses an explicit account or the matching primary for a new session.
func (s *Service) ResolveAccount(ctx context.Context, harness domain.AgentHarness, explicit string) (string, bool, error) {
	state, pending, err := s.store.LoadProviderAccountState(ctx)
	if err != nil {
		return "", false, err
	}
	if pending != nil {
		return "", false, ports.ErrProviderAccountRecovery
	}
	provider := Provider(harness)
	if provider == "" {
		if explicit != "" {
			return "", false, ports.ErrProviderAccountIncompatible
		}
		return "", false, nil
	}
	id, managed := primary(state, provider)
	if explicit != "" {
		id = explicit
		managed = true
	}
	if !managed {
		return "", false, nil
	}
	if id == "" {
		return "", true, ports.ErrProviderLoginRequired
	}
	if err := eligible(state, id, provider); err != nil {
		return "", true, err
	}
	return id, true, nil
}

// SessionAccount reads the saved assignment without changing native sessions.
func (s *Service) SessionAccount(ctx context.Context, id domain.SessionID) (domain.ProviderSessionRoute, bool, error) {
	state, err := s.State(ctx)
	if err != nil {
		return domain.ProviderSessionRoute{}, false, err
	}
	for _, r := range state.Routes {
		if r.SessionID == id {
			return r, true, nil
		}
	}
	return domain.ProviderSessionRoute{}, false, nil
}
func (s *Service) ticket(id domain.SessionID) string {
	mac := hmac.New(sha256.New, s.ticketKey)
	mac.Write([]byte("ao-provider-session\x00" + string(id)))
	return hex.EncodeToString(mac.Sum(nil))
}

// LaunchAccountEnv provides the stable private endpoint and ticket for a managed session.
func (s *Service) LaunchAccountEnv(ctx context.Context, id domain.SessionID) (map[string]string, error) {
	route, managed, err := s.SessionAccount(ctx, id)
	if err != nil || !managed {
		return nil, err
	}
	if len(s.ticketKey) < 32 || s.endpoint == "" {
		return nil, errors.New("proxy launch configuration unavailable")
	}
	ticket := s.ticket(id)
	// Even waiting sessions retain their endpoint/ticket and cannot fall back to
	// ambient credentials. Re-login changes the mapping, not their environment.
	if route.Provider == "codex" {
		return map[string]string{"AO_PROXY_ENDPOINT": s.endpoint, "AO_PROXY_TICKET": ticket}, nil
	}
	return map[string]string{"AO_PROXY_ENDPOINT": "", "AO_PROXY_TICKET": "", "ANTHROPIC_BASE_URL": s.endpoint, "ANTHROPIC_AUTH_TOKEN": ticket, "ANTHROPIC_API_KEY": "", "CLAUDE_CODE_OAUTH_TOKEN": "", "CLAUDE_CODE_USE_BEDROCK": "", "CLAUDE_CODE_USE_VERTEX": "", "CLAUDE_CODE_USE_FOUNDRY": ""}, nil
}
func snapshot(state domain.ProviderAccountState) ports.ProviderRouteSnapshot {
	generation := state.CodexPrimaryGeneration
	if generation == 0 {
		if current, exists := primary(state, "codex"); exists && current != "" {
			generation = 1
		}
	}
	result := ports.ProviderRouteSnapshot{Revision: state.Revision, CodexPrimaryGeneration: generation, Routes: make([]ports.ProviderRoute, 0, len(state.Routes))}
	for _, a := range state.Accounts {
		if a.CredentialRef != "" && a.AuthID != "" {
			result.AuthIDs = append(result.AuthIDs, a.AuthID)
		}
	}
	for _, r := range state.Routes {
		authID := ""
		if a, ok := account(state, r.AccountID); ok {
			authID = a.AuthID
		}
		result.Routes = append(result.Routes, ports.ProviderRoute{SessionID: r.SessionID, TicketHash: r.TicketHash, Provider: r.Provider, AuthID: authID})
	}
	return result
}
func (s *Service) reconcile(ctx context.Context, guarded bool) error {
	state, pending, err := s.store.LoadProviderAccountState(ctx)
	if err != nil || pending == nil {
		return err
	}
	if pending.Next.Revision != state.Revision && pending.Next.Revision != state.Revision+1 {
		return ports.ErrProviderAccountRecovery
	}
	if !guarded && state.Revision != pending.Next.Revision {
		affected := mutationSessions(snapshot(state), snapshot(pending.Next), pending.RequestBoundary)
		if len(affected) > 0 {
			if s.guard == nil {
				return ports.ErrProviderAccountBusy
			}
			done, guardErr := s.guard.AcquireAccountMutation(ctx, affected)
			if guardErr != nil {
				return guardErr
			}
			defer done()
		}
	}
	next := snapshot(pending.Next)
	next.RequestBoundary = pending.RequestBoundary
	if err = s.proxy.ApplyRoutes(ctx, next); err != nil {
		// A helper admission refusal proves that it changed no routes. Clear
		// this uncommitted intent so an idle retry requires a new user action.
		// Failed cleanup leaves the operation unresolved, not safely cancelled.
		if errors.Is(err, ports.ErrProviderAccountBusy) && state.Revision != pending.Next.Revision {
			if cleanupErr := s.store.FinishProviderAccountIntent(ctx, state.Revision); cleanupErr != nil {
				return fmt.Errorf("abort account routing: %w", errors.Join(ports.ErrProviderAccountRecovery, cleanupErr))
			}
		}
		return fmt.Errorf("apply account routing: %w", err)
	}
	if state.Revision != pending.Next.Revision {
		if err = s.store.CommitProviderAccountIntent(ctx, state.Revision); err != nil {
			return fmt.Errorf("commit account routing: %w", err)
		}
	}
	if pending.DeleteCredential != "" {
		if err = s.proxy.DeleteCredential(ctx, pending.DeleteCredential); err != nil {
			return fmt.Errorf("remove saved credential: %w", err)
		}
	}
	return s.store.FinishProviderAccountIntent(ctx, pending.Next.Revision)
}

// Recover resolves a durably admitted operation whose outcome is still pending.
func (s *Service) Recover(ctx context.Context) error {
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return s.reconcile(ctx, false)
}

// RestoreHost recovers an admitted operation and verifies the helper's effective
// routes, restarting only a confirmed dead owned helper through the proxy port.
func (s *Service) RestoreHost(ctx context.Context) error {
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.reconcile(ctx, false); err != nil {
		return err
	}
	state, _, err := s.store.LoadProviderAccountState(ctx)
	if err != nil || len(state.Primaries) == 0 {
		return err
	}
	return s.proxy.ApplyRoutes(ctx, snapshot(state))
}

func (s *Service) mutate(ctx context.Context, change func(*domain.ProviderAccountState) (string, error)) error {
	return s.mutateWithBoundary(ctx, false, change)
}

func (s *Service) mutateWithBoundary(ctx context.Context, requestBoundary bool, change func(*domain.ProviderAccountState) (string, error)) error {
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.reconcile(ctx, false); err != nil {
		return err
	}
	state, _, err := s.store.LoadProviderAccountState(ctx)
	if err != nil {
		return err
	}
	oldRevision := state.Revision
	before := snapshot(state)
	deletion, err := change(&state)
	if err != nil {
		return err
	}
	after := snapshot(state)
	requestBoundary = requestBoundary || (s.codexRequestSwitching && deletion == "")
	affected := mutationSessions(before, after, requestBoundary)
	if len(affected) > 0 {
		if s.guard == nil {
			return ports.ErrProviderAccountBusy
		}
		done, err := s.guard.AcquireAccountMutation(ctx, affected)
		if err != nil {
			return err
		}
		defer done()
	}
	state.Revision++
	if err := s.store.SaveProviderAccountIntent(ctx, oldRevision, domain.ProviderAccountIntent{Next: state, DeleteCredential: deletion, RequestBoundary: requestBoundary}); err != nil {
		return err
	}
	return s.reconcile(ctx, true)
}

// RecordLogin accepts only an inventory-verified successful upstream login.
func (s *Service) RecordLogin(ctx context.Context, provider, email, credentialRef, authID, reloginID string) (string, error) {
	return s.RecordCredential(ctx, ports.VerifiedProviderLogin{Provider: provider, Email: email, CredentialRef: credentialRef, AuthID: authID}, reloginID)
}

// RecordCredential accepts an inventory-verified OAuth, imported, or API-key
// credential. API-key references are opaque helper-owned values and therefore
// are allowed to use the config: namespace.
func (s *Service) RecordCredential(ctx context.Context, verified ports.VerifiedProviderLogin, reloginID string) (string, error) {
	provider, email, credentialRef, authID := verified.Provider, verified.Email, verified.CredentialRef, verified.AuthID
	validRef := filepath.Base(credentialRef) == credentialRef && !strings.ContainsAny(credentialRef, "/\\") && credentialRef != "." && credentialRef != ".."
	if strings.HasPrefix(credentialRef, "config:") {
		validRef = len(credentialRef) > len("config:") && !strings.ContainsAny(credentialRef, "/\\")
	}
	if (provider != "codex" && provider != "claude") || strings.TrimSpace(email) == "" || credentialRef == "" || authID == "" || !validRef {
		return "", fmt.Errorf("invalid verified provider account: %w", ports.ErrProviderAccountIncompatible)
	}
	kind := strings.TrimSpace(verified.Kind)
	if kind == "" {
		kind = "oauth"
	}
	id := reloginID
	if id == "" {
		id = s.newID()
	}
	err := s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		if old, ok := account(*state, id); ok {
			if old.Provider != provider || !strings.EqualFold(old.Email, email) {
				return "", ports.ErrProviderAccountIncompatible
			}
			if old.CredentialRef != "" {
				if old.CredentialRef == credentialRef && old.AuthID == authID {
					return "", nil
				}
				return "", fmt.Errorf("account is already signed in: %w", ports.ErrProviderAccountConflict)
			}
			for i, a := range state.Accounts {
				if a.ID == id {
					state.Accounts[i] = domain.ProviderAccount{ID: id, Provider: provider, Email: email, Kind: kind, CredentialRef: credentialRef, AuthID: authID}
				}
			}
		} else {
			if reloginID != "" {
				return "", ports.ErrProviderAccountUnknown
			}
			for _, a := range state.Accounts {
				if a.Provider == provider && strings.EqualFold(a.Email, email) {
					if a.CredentialRef == credentialRef && a.AuthID == authID {
						id = a.ID
						return "", nil
					}
					return "", fmt.Errorf("account already exists; sign in to its existing entry: %w", ports.ErrProviderAccountConflict)
				}
			}
			state.Accounts = append(state.Accounts, domain.ProviderAccount{ID: id, Provider: provider, Email: email, Kind: kind, CredentialRef: credentialRef, AuthID: authID})
		}
		current, _ := primary(*state, provider)
		if current == "" {
			setPrimary(state, provider, id)
			for i, r := range state.Routes {
				if r.Provider == provider && r.AccountID == "" {
					state.Routes[i].AccountID = id
				}
			}
		}
		return "", nil
	})
	return id, err
}

// SetPrimary changes the default and optionally rebinds previous-primary Codex routes.
func (s *Service) SetPrimary(ctx context.Context, id string) error {
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		a, ok := account(*state, id)
		if !ok {
			return "", ports.ErrProviderAccountUnknown
		}
		if err := eligible(*state, id, a.Provider); err != nil {
			return "", err
		}
		if s.codexRequestSwitching && a.Provider == "codex" {
			previous, _ := primary(*state, a.Provider)
			for i, r := range state.Routes {
				if r.Provider == a.Provider && r.AccountID == previous {
					state.Routes[i].AccountID = id
				}
			}
		}
		setPrimary(state, a.Provider, id)
		return "", nil
	})
}

// AssignAccount creates a session ticket bound to an eligible account.
func (s *Service) AssignAccount(ctx context.Context, id domain.SessionID, harness domain.AgentHarness, accountID string) error {
	provider := Provider(harness)
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		if provider == "" {
			return "", ports.ErrProviderAccountIncompatible
		}
		if err := eligible(*state, accountID, provider); err != nil {
			return "", err
		}
		for _, r := range state.Routes {
			if r.SessionID == id {
				return "", fmt.Errorf("session already has an account route: %w", ports.ErrProviderAccountConflict)
			}
		}
		sum := sha256.Sum256([]byte(s.ticket(id)))
		state.Routes = append(state.Routes, domain.ProviderSessionRoute{SessionID: id, Provider: provider, AccountID: accountID, TicketHash: hex.EncodeToString(sum[:])})
		return "", nil
	})
}

// Switch retains the ticket; opt-in Codex rebinds apply at the next request.
func (s *Service) Switch(ctx context.Context, id domain.SessionID, target string) error {
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		for i, r := range state.Routes {
			if r.SessionID == id {
				if err := eligible(*state, target, r.Provider); err != nil {
					return "", err
				}
				state.Routes[i].AccountID = target
				return "", nil
			}
		}
		return "", fmt.Errorf("older native sessions cannot change managed accounts: %w", ports.ErrProviderAccountIncompatible)
	})
}

// Remove signs out or removes an account and reassigns its affected sessions.
func (s *Service) Remove(ctx context.Context, id, replacement string, signOut bool) error {
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		a, ok := account(*state, id)
		if !ok {
			return "", ports.ErrProviderAccountUnknown
		}
		current, _ := primary(*state, a.Provider)
		if current == id {
			hasOther := false
			for _, other := range state.Accounts {
				if other.ID != id && other.Provider == a.Provider && other.CredentialRef != "" {
					hasOther = true
				}
			}
			if hasOther {
				if replacement == "" || replacement == id {
					return "", ports.ErrProviderPrimaryRequired
				}
				if err := eligible(*state, replacement, a.Provider); err != nil {
					return "", err
				}
				current = replacement
			} else {
				current = ""
			}
			setPrimary(state, a.Provider, current)
		}
		for i, r := range state.Routes {
			if r.AccountID == id {
				state.Routes[i].AccountID = current
			}
		}
		accounts := state.Accounts[:0]
		for _, entry := range state.Accounts {
			if entry.ID == id {
				if !signOut {
					continue
				}
				entry.CredentialRef = ""
				entry.AuthID = ""
			}
			accounts = append(accounts, entry)
		}
		state.Accounts = accounts
		if a.Provider == "codex" && signedInCount(*state, "codex") < 2 {
			state.CodexQuotaAutoSwitch = false
		}
		return a.CredentialRef, nil
	})
}

func changedSessions(before, after ports.ProviderRouteSnapshot) []domain.SessionID {
	var affected []domain.SessionID
	for _, old := range before.Routes {
		found := false
		for _, next := range after.Routes {
			if old.SessionID == next.SessionID {
				found = true
				if old.AuthID != next.AuthID {
					affected = append(affected, old.SessionID)
				}
				break
			}
		}
		if !found {
			affected = append(affected, old.SessionID)
		}
	}
	return affected
}

// Only a non-revoking Codex rebind may skip native idle admission. Destructive
// mutations still fence workers/reviewers; the helper separately drains auth leases.
func mutationSessions(before, after ports.ProviderRouteSnapshot, requestBoundary bool) []domain.SessionID {
	if requestBoundary {
		for i, old := range before.Routes {
			for _, next := range after.Routes {
				if old.SessionID == next.SessionID && old.Provider == "codex" && next.Provider == old.Provider && next.TicketHash == old.TicketHash && next.AuthID != "" {
					before.Routes[i].AuthID = next.AuthID
				}
			}
		}
	}
	return changedSessions(before, after)
}

// RecoveryRequired reports whether a durable account operation remains unfinished.
func (s *Service) RecoveryRequired(ctx context.Context) (bool, error) {
	_, pending, err := s.store.LoadProviderAccountState(ctx)
	return pending != nil, err
}

// ForgetAccount revokes a deleted seed session's ticket. Archived sessions keep
// their assignment because they may be restored with native history.
func (s *Service) ForgetAccount(ctx context.Context, id domain.SessionID) error {
	_, managed, err := s.SessionAccount(ctx, id)
	if err != nil || !managed {
		return err
	}
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		routes := state.Routes[:0]
		for _, r := range state.Routes {
			if r.SessionID != id {
				routes = append(routes, r)
			}
		}
		state.Routes = routes
		return "", nil
	})
}

// discardLogin deletes a rejected login only when no effective or pending account uses its credential.
func (s *Service) discardLogin(ctx context.Context, login ports.VerifiedProviderLogin) error {
	if login.CredentialRef == "" {
		return nil
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	state, pending, err := s.store.LoadProviderAccountState(ctx)
	if err != nil {
		return err
	}
	entries := state.Accounts
	if pending != nil {
		entries = append(entries, pending.Next.Accounts...)
	}
	for _, a := range entries {
		if a.CredentialRef == login.CredentialRef {
			return nil
		}
	}
	return s.proxy.DeleteCredential(ctx, login.CredentialRef)
}
