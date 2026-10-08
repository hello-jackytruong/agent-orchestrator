package provideraccounts

import (
	"context"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// SetNativeAccountSource is called once during daemon wiring, before serving.
func (s *Service) SetNativeAccountSource(source ports.ProviderNativeAccountSource) { s.native = source }

// RefreshNativeAccounts discovers new native logins without treating local
// logout as revocation. The durable receipt prevents repeated imports, token
// rollback, and a settings refresh undoing the user's local sign-out/removal.
func (s *Service) RefreshNativeAccounts(ctx context.Context) error {
	if s.native == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err = s.reconcile(ctx, false); err != nil {
		return err
	}
	for _, provider := range []string{"codex", "claude"} {
		native, err := s.native.ReadNativeAccount(ctx, provider)
		if err != nil || native.Fingerprint == "" {
			continue
		} // Locked/missing sources leave managed credentials untouched.
		state, err := s.State(ctx)
		if err != nil {
			return err
		}
		if state.NativeImports[provider].Fingerprint == native.Fingerprint {
			continue
		}
		verified, err := s.native.ImportNativeAccount(ctx, provider, native)
		if err != nil || verified.Provider != provider || strings.TrimSpace(verified.Email) == "" || verified.AuthID == "" || verified.CredentialRef == "" {
			continue
		}
		err = s.mutateLocked(ctx, true, func(state *domain.ProviderAccountState) (string, error) {
			id, deletion := "", ""
			for i := range state.Accounts {
				a := &state.Accounts[i]
				if a.Provider != provider {
					continue
				}
				a.Global = strings.EqualFold(a.Email, verified.Email) && a.Kind != "api_key"
				if !a.Global {
					continue
				}
				id = a.ID
				if a.CredentialRef == "" {
					a.CredentialRef, a.AuthID = verified.CredentialRef, verified.AuthID
				} else if a.CredentialRef != verified.CredentialRef {
					// Keep the managed credential: it may already have a newer refresh token.
					deletion = verified.CredentialRef
				}
			}
			if id == "" {
				// The catalogue's identity is provider + email, including API-key labels.
				for _, a := range state.Accounts {
					if a.Provider == provider && strings.EqualFold(a.Email, verified.Email) {
						return "", ports.ErrProviderAccountConflict
					}
				}
				id = s.newID()
				state.Accounts = append(state.Accounts, domain.ProviderAccount{ID: id, Provider: provider, Email: verified.Email, Kind: "oauth", Global: true, CredentialRef: verified.CredentialRef, AuthID: verified.AuthID})
				fillMissingDisplayNames(state)
			}
			if current, _ := primary(*state, provider); current == "" {
				setPrimary(state, provider, id)
				for i := range state.Routes {
					if state.Routes[i].Provider == provider && state.Routes[i].AccountID == "" {
						state.Routes[i].AccountID = id
					}
				}
			}
			if state.NativeImports == nil {
				state.NativeImports = make(map[string]domain.NativeProviderImport)
			}
			state.NativeImports[provider] = domain.NativeProviderImport{Fingerprint: native.Fingerprint, AccountID: id}
			return deletion, nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
