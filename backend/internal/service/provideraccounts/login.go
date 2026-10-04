package provideraccounts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// LoginCoordinator serializes login attempts and records only verified account identities.
type LoginCoordinator struct {
	mu       sync.Mutex
	accounts *Service
	proxy    ports.ProviderAccountLoginProxy
	attempts map[string]ports.ProviderLogin
	newID    func() string
	expires  map[string]time.Time
	now      func() time.Time
}

// NewLoginCoordinator creates the daemon-owned login attempt coordinator.
func NewLoginCoordinator(accounts *Service, proxy ports.ProviderAccountLoginProxy, newID func() string) *LoginCoordinator {
	return &LoginCoordinator{accounts: accounts, proxy: proxy, attempts: make(map[string]ports.ProviderLogin), newID: newID, expires: make(map[string]time.Time), now: time.Now}
}

// Start starts or resumes a provider login attempt.
func (l *LoginCoordinator) Start(ctx context.Context, provider, accountID string) (ports.ProviderLogin, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if provider != "codex" && provider != "claude" {
		return ports.ProviderLogin{}, ports.ErrProviderAccountIncompatible
	}
	for _, login := range l.attempts {
		if login.Provider == provider && login.Status == "waiting" {
			current, err := l.status(ctx, login.ID)
			if err != nil {
				return current, err
			}
			if current.Status == "waiting" {
				if current.AccountID == accountID {
					return current, nil
				}
				return ports.ProviderLogin{}, fmt.Errorf("a login is already in progress for this provider: %w", ports.ErrProviderAccountConflict)
			}
		}
	}
	if accountID != "" {
		state, err := l.accounts.State(ctx)
		if err != nil {
			return ports.ProviderLogin{}, err
		}
		a, ok := account(state, accountID)
		if !ok {
			return ports.ProviderLogin{}, ports.ErrProviderAccountUnknown
		}
		if a.Provider != provider {
			return ports.ProviderLogin{}, ports.ErrProviderAccountIncompatible
		}
		if a.CredentialRef != "" {
			return ports.ProviderLogin{}, fmt.Errorf("account is already signed in: %w", ports.ErrProviderAccountConflict)
		}
	}
	login, err := l.proxy.StartAccountLogin(ctx, provider, l.newID())
	if err != nil {
		return login, err
	}
	login.AccountID = accountID
	l.attempts[login.ID] = login
	l.expires[login.ID] = l.now().Add(6 * time.Minute)
	return login, nil
}

// Status polls a login and commits its verified account before reporting completion.
func (l *LoginCoordinator) Status(ctx context.Context, id string) (ports.ProviderLogin, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status(ctx, id)
}
func (l *LoginCoordinator) status(ctx context.Context, id string) (ports.ProviderLogin, error) {
	login, ok := l.attempts[id]
	if !ok {
		return login, ports.ErrProviderLoginUnknown
	}
	if login.Status != "waiting" {
		return login, nil
	}
	status, err := l.proxy.AccountLoginStatus(ctx, login)
	if err != nil {
		return login, err
	}
	if status == "complete" {
		verified, err := l.proxy.VerifiedAccountLogin(ctx, id)
		if err != nil {
			return login, err
		}
		if verified.Provider != login.Provider {
			login.Status = "failed"
			l.attempts[id] = login
			return login, errors.Join(ports.ErrProviderAccountIncompatible, l.accounts.discardLogin(ctx, verified))
		}
		accountID, err := l.accounts.RecordLogin(ctx, verified.Provider, verified.Email, verified.CredentialRef, verified.AuthID, login.AccountID)
		if err != nil {
			if errors.Is(err, ports.ErrProviderAccountIncompatible) || errors.Is(err, ports.ErrProviderAccountConflict) || errors.Is(err, ports.ErrProviderAccountUnknown) {
				login.Status = "failed"
				l.attempts[id] = login
				err = errors.Join(err, l.accounts.discardLogin(ctx, verified))
			}
			return login, err
		}
		login.AccountID = accountID
	}
	if status == "waiting" && !l.now().Before(l.expires[id]) {
		if err := l.proxy.CancelAccountLogin(ctx, login); err != nil {
			return login, err
		}
		status = "failed"
	}
	login.Status = status
	l.attempts[id] = login
	return login, nil
}

// Cancel cancels a pending attempt without removing existing accounts.
func (l *LoginCoordinator) Cancel(ctx context.Context, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	login, ok := l.attempts[id]
	if !ok {
		return nil
	}
	if login.Status != "waiting" {
		return nil
	}
	if err := l.proxy.CancelAccountLogin(ctx, login); err != nil {
		return err
	}
	login.Status = "cancelled"
	l.attempts[id] = login
	return nil
}
