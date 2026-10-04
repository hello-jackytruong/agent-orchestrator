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

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// Service owns managed provider defaults, session assignments, and mutation recovery.
type Service struct {
	store     ports.ProviderAccountStore
	proxy     ports.ProviderAccountProxy
	guard     ports.ProviderAccountSessionGuard
	gate      chan struct{}
	ticketKey []byte
	endpoint  string
	newID     func() string
}

// New creates the account service with a stable private ticket identity.
func New(store ports.ProviderAccountStore, proxy ports.ProviderAccountProxy, guard ports.ProviderAccountSessionGuard, key []byte, endpoint string, newID func() string) *Service {
	return &Service{store: store, proxy: proxy, guard: guard, gate: make(chan struct{}, 1), ticketKey: append([]byte(nil), key...), endpoint: endpoint, newID: newID}
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
func primary(state domain.ProviderAccountState, provider string) (string, bool) {
	for _, p := range state.Primaries {
		if p.Provider == provider {
			return p.PrimaryID, true
		}
	}
	return "", false
}
func setPrimary(state *domain.ProviderAccountState, provider, id string) {
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
	result := ports.ProviderRouteSnapshot{Revision: state.Revision, Routes: make([]ports.ProviderRoute, 0, len(state.Routes))}
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
		affected := changedSessions(snapshot(state), snapshot(pending.Next))
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
	if err = s.proxy.ApplyRoutes(ctx, snapshot(pending.Next)); err != nil {
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
	affected := changedSessions(before, after)
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
	if err := s.store.SaveProviderAccountIntent(ctx, oldRevision, domain.ProviderAccountIntent{Next: state, DeleteCredential: deletion}); err != nil {
		return err
	}
	return s.reconcile(ctx, true)
}

// RecordLogin accepts only an inventory-verified successful upstream login.
func (s *Service) RecordLogin(ctx context.Context, provider, email, credentialRef, authID, reloginID string) (string, error) {
	if (provider != "codex" && provider != "claude") || strings.TrimSpace(email) == "" || credentialRef == "" || authID == "" || filepath.Base(credentialRef) != credentialRef || strings.ContainsAny(credentialRef, "/\\") || credentialRef == "." || credentialRef == ".." {
		return "", fmt.Errorf("invalid verified provider account: %w", ports.ErrProviderAccountIncompatible)
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
					state.Accounts[i] = domain.ProviderAccount{ID: id, Provider: provider, Email: email, CredentialRef: credentialRef, AuthID: authID}
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
			state.Accounts = append(state.Accounts, domain.ProviderAccount{ID: id, Provider: provider, Email: email, CredentialRef: credentialRef, AuthID: authID})
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

// SetPrimary changes the default for future sessions of an account provider.
func (s *Service) SetPrimary(ctx context.Context, id string) error {
	return s.mutate(ctx, func(state *domain.ProviderAccountState) (string, error) {
		a, ok := account(*state, id)
		if !ok {
			return "", ports.ErrProviderAccountUnknown
		}
		if err := eligible(*state, id, a.Provider); err != nil {
			return "", err
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

// Switch changes an idle managed session account while retaining its ticket.
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
