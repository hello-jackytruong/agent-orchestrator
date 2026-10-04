package ports

import (
	"context"
	"errors"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// ErrProviderAccountConflict reports an incompatible concurrent or duplicate account operation.
var ErrProviderAccountConflict = errors.New("account operation conflicts with current state")

// ErrProviderLoginUnknown reports a login attempt that is no longer known.
var ErrProviderLoginUnknown = errors.New("login attempt not found; sign in again")

// ErrProviderLoginCallbackBusy reports another login occupying the provider callback port.
var ErrProviderLoginCallbackBusy = errors.New("login callback port is in use; finish the other login and retry")

// ErrProviderLoginRequired reports an account without usable saved credentials.
var ErrProviderLoginRequired = errors.New("provider account login required")

// ErrProviderAccountBusy refuses routing changes while affected sessions are working.
var ErrProviderAccountBusy = errors.New("sessions are using this account; wait until they are idle")

// ErrProviderPrimaryRequired requires a replacement before removing a usable primary.
var ErrProviderPrimaryRequired = errors.New("choose a replacement primary account first")

// ErrProviderAccountUnknown reports an account absent from the catalogue.
var ErrProviderAccountUnknown = errors.New("provider account not found")

// ErrProviderAccountIncompatible reports an account or operation for a different provider.
var ErrProviderAccountIncompatible = errors.New("account does not match the session provider")

// ErrProviderAccountRecovery reports a pending or inconsistent routing operation.
var ErrProviderAccountRecovery = errors.New("provider account operation requires recovery")

// ProviderAccountStore persists routing facts and the recoverable mutation journal.
type ProviderAccountStore interface {
	LoadProviderAccountState(context.Context) (domain.ProviderAccountState, *domain.ProviderAccountIntent, error)
	SaveProviderAccountIntent(context.Context, int64, domain.ProviderAccountIntent) error
	CommitProviderAccountIntent(context.Context, int64) error
	FinishProviderAccountIntent(context.Context, int64) error
}

// ProviderRouteSnapshot is the complete revisioned helper routing table.
type ProviderRouteSnapshot struct {
	Revision int64           `json:"revision"`
	Routes   []ProviderRoute `json:"routes"`
}

// ProviderRoute contains a ticket hash and an exact upstream account identity.
type ProviderRoute struct {
	SessionID  domain.SessionID `json:"session_id"`
	TicketHash string           `json:"ticket_hash"`
	Provider   string           `json:"provider"`
	AuthID     string           `json:"auth_id"`
}

// ProviderAccountProxy acknowledges routing changes and deletes saved upstream credentials.
type ProviderAccountProxy interface {
	ApplyRoutes(context.Context, ProviderRouteSnapshot) error
	DeleteCredential(context.Context, string) error
}

// ProviderAccountSessionGuard fences affected sessions while their account mappings change.
type ProviderAccountSessionGuard interface {
	AcquireAccountMutation(context.Context, []domain.SessionID) (func(), error)
}

// ProviderAccountRouting resolves defaults and prepares managed session launches.
type ProviderAccountRouting interface {
	ResolveAccount(context.Context, domain.AgentHarness, string) (string, bool, error)
	AssignAccount(context.Context, domain.SessionID, domain.AgentHarness, string) error
	SessionAccount(context.Context, domain.SessionID) (domain.ProviderSessionRoute, bool, error)
	LaunchAccountEnv(context.Context, domain.SessionID) (map[string]string, error)
}

// ProviderLogin tracks one upstream login attempt, including private OAuth state.
type ProviderLogin struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	State     string `json:"state"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	AccountID string `json:"account_id"`
}

// VerifiedProviderLogin contains identity obtained from the successful upstream credential.
type VerifiedProviderLogin struct {
	Provider      string `json:"provider"`
	Email         string `json:"email"`
	CredentialRef string `json:"credential_ref"`
	AuthID        string `json:"auth_id"`
}

// ProviderAccountLoginProxy starts, verifies, and cancels upstream logins.
type ProviderAccountLoginProxy interface {
	StartAccountLogin(context.Context, string, string) (ProviderLogin, error)
	AccountLoginStatus(context.Context, ProviderLogin) (string, error)
	CancelAccountLogin(context.Context, ProviderLogin) error
	VerifiedAccountLogin(context.Context, string) (VerifiedProviderLogin, error)
}
