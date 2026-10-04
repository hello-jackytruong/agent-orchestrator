package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/apierr"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/envelope"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// ProviderAccountService defines the account operations available to HTTP handlers.
type ProviderAccountService interface {
	State(context.Context) (domain.ProviderAccountState, error)
	SetPrimary(context.Context, string) error
	Remove(context.Context, string, string, bool) error
	Switch(context.Context, domain.SessionID, string) error
	SessionAccount(context.Context, domain.SessionID) (domain.ProviderSessionRoute, bool, error)
	RecoveryRequired(context.Context) (bool, error)
}

// ProviderLoginService defines login operations available to HTTP handlers.
type ProviderLoginService interface {
	Start(context.Context, string, string) (ports.ProviderLogin, error)
	Status(context.Context, string) (ports.ProviderLogin, error)
	Cancel(context.Context, string) error
}

// ProviderAccountsController exposes safe account and session routing operations.
type ProviderAccountsController struct {
	Svc   ProviderAccountService
	Login ProviderLoginService
}

// Register registers account management and session assignment routes.
func (c *ProviderAccountsController) Register(r chi.Router) {
	r.Get("/provider-accounts", c.list)
	r.Post("/provider-accounts/login", c.startLogin)
	r.Get("/provider-accounts/login/{loginId}", c.loginStatus)
	r.Delete("/provider-accounts/login/{loginId}", c.cancelLogin)
	r.Put("/provider-accounts/{accountId}/primary", c.setPrimary)
	r.Post("/provider-accounts/{accountId}/sign-out", c.signOut)
	r.Delete("/provider-accounts/{accountId}", c.remove)
	r.Get("/sessions/{sessionId}/provider-account", c.sessionAccount)
	r.Put("/sessions/{sessionId}/provider-account", c.switchAccount)
}
func accountAPIError(err error) error {
	switch {
	case errors.Is(err, ports.ErrProviderLoginCallbackBusy):
		return apierr.Conflict("PROVIDER_LOGIN_CALLBACK_BUSY", ports.ErrProviderLoginCallbackBusy.Error(), nil)
	case errors.Is(err, ports.ErrProviderLoginUnknown):
		return apierr.NotFound("PROVIDER_LOGIN_NOT_FOUND", ports.ErrProviderLoginUnknown.Error())
	case errors.Is(err, ports.ErrProviderAccountConflict):
		return apierr.Conflict("PROVIDER_ACCOUNT_CONFLICT", ports.ErrProviderAccountConflict.Error(), nil)
	case errors.Is(err, ports.ErrProviderAccountBusy):
		return apierr.Conflict("PROVIDER_ACCOUNT_IN_USE", ports.ErrProviderAccountBusy.Error(), nil)
	case errors.Is(err, ports.ErrProviderPrimaryRequired):
		return apierr.Conflict("PROVIDER_PRIMARY_REQUIRED", ports.ErrProviderPrimaryRequired.Error(), nil)
	case errors.Is(err, ports.ErrProviderAccountUnknown):
		return apierr.NotFound("PROVIDER_ACCOUNT_NOT_FOUND", ports.ErrProviderAccountUnknown.Error())
	case errors.Is(err, ports.ErrProviderLoginRequired):
		return apierr.Conflict("PROVIDER_LOGIN_REQUIRED", ports.ErrProviderLoginRequired.Error(), nil)
	case errors.Is(err, ports.ErrProviderAccountIncompatible):
		return apierr.Invalid("PROVIDER_ACCOUNT_INCOMPATIBLE", ports.ErrProviderAccountIncompatible.Error(), nil)
	case errors.Is(err, ports.ErrProviderAccountRecovery):
		return apierr.Conflict("PROVIDER_ACCOUNT_RECOVERY_REQUIRED", ports.ErrProviderAccountRecovery.Error(), nil)
	}
	return err
}
func (c *ProviderAccountsController) ready(w http.ResponseWriter, r *http.Request) bool {
	if c.Svc == nil {
		envelope.WriteAPIError(w, r, 503, "service_unavailable", "PROVIDER_ACCOUNTS_UNAVAILABLE", "Account manager is unavailable in this build", nil)
		return false
	}
	return true
}
func (c *ProviderAccountsController) list(w http.ResponseWriter, r *http.Request) {
	if !c.ready(w, r) {
		return
	}
	state, err := c.Svc.State(r.Context())
	if err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	result := ProviderAccountsResponse{Accounts: []ProviderAccountView{}, Defaults: []ProviderPrimaryView{}}
	pending, err := c.Svc.RecoveryRequired(r.Context())
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	result.RecoveryRequired = pending
	primaries := map[string]string{}
	for _, p := range state.Primaries {
		primaries[p.Provider] = p.PrimaryID
	}
	for _, provider := range []string{"codex", "claude"} {
		id, managed := primaries[provider]
		result.Defaults = append(result.Defaults, ProviderPrimaryView{Provider: provider, PrimaryID: id, Managed: managed})
	}
	for _, a := range state.Accounts {
		view := ProviderAccountView{ID: a.ID, Provider: a.Provider, Email: a.Email, SignedIn: a.CredentialRef != "", Primary: primaries[a.Provider] == a.ID, Sessions: []string{}}
		for _, route := range state.Routes {
			if route.AccountID == a.ID {
				view.Sessions = append(view.Sessions, string(route.SessionID))
			}
		}
		result.Accounts = append(result.Accounts, view)
	}
	envelope.WriteJSON(w, 200, result)
}
func (c *ProviderAccountsController) setPrimary(w http.ResponseWriter, r *http.Request) {
	if !c.ready(w, r) {
		return
	}
	if err := c.Svc.SetPrimary(r.Context(), chi.URLParam(r, "accountId")); err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	c.list(w, r)
}
func (c *ProviderAccountsController) remove(w http.ResponseWriter, r *http.Request) {
	c.removeAccount(w, r, false)
}
func (c *ProviderAccountsController) signOut(w http.ResponseWriter, r *http.Request) {
	c.removeAccount(w, r, true)
}
func (c *ProviderAccountsController) removeAccount(w http.ResponseWriter, r *http.Request, signOut bool) {
	if !c.ready(w, r) {
		return
	}
	var input ProviderAccountChangeRequest
	if r.ContentLength != 0 {
		if err := decodeAccountJSON(r, &input); err != nil {
			envelope.WriteError(w, r, apierr.Invalid("INVALID_JSON", "Invalid account change request", nil))
			return
		}
	}
	if err := c.Svc.Remove(r.Context(), chi.URLParam(r, "accountId"), input.ReplacementPrimaryID, signOut); err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	c.list(w, r)
}
func (c *ProviderAccountsController) sessionAccount(w http.ResponseWriter, r *http.Request) {
	if !c.ready(w, r) {
		return
	}
	route, managed, err := c.Svc.SessionAccount(r.Context(), domain.SessionID(chi.URLParam(r, "sessionId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, 200, SessionProviderAccountResponse{Managed: managed, Provider: route.Provider, AccountID: route.AccountID, LoginRequired: managed && route.AccountID == ""})
}
func (c *ProviderAccountsController) switchAccount(w http.ResponseWriter, r *http.Request) {
	if !c.ready(w, r) {
		return
	}
	var input ProviderAccountChangeRequest
	if err := decodeAccountJSON(r, &input); err != nil || strings.TrimSpace(input.AccountID) == "" {
		envelope.WriteError(w, r, apierr.Invalid("ACCOUNT_REQUIRED", "Choose an account", nil))
		return
	}
	if err := c.Svc.Switch(r.Context(), domain.SessionID(chi.URLParam(r, "sessionId")), input.AccountID); err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	c.sessionAccount(w, r)
}
func loginView(login ports.ProviderLogin) ProviderLoginResponse {
	return ProviderLoginResponse{ID: login.ID, Provider: login.Provider, URL: login.URL, Status: login.Status, AccountID: login.AccountID}
}
func (c *ProviderAccountsController) loginReady(w http.ResponseWriter, r *http.Request) bool {
	if !c.ready(w, r) {
		return false
	}
	if c.Login == nil {
		envelope.WriteAPIError(w, r, 503, "service_unavailable", "PROVIDER_LOGIN_UNAVAILABLE", "Account sign-in is unavailable in this build", nil)
		return false
	}
	return true
}
func (c *ProviderAccountsController) startLogin(w http.ResponseWriter, r *http.Request) {
	if !c.loginReady(w, r) {
		return
	}
	var input ProviderLoginRequest
	if err := decodeAccountJSON(r, &input); err != nil || (input.Provider != "codex" && input.Provider != "claude") {
		envelope.WriteError(w, r, apierr.Invalid("PROVIDER_REQUIRED", "Choose Codex or Claude", nil))
		return
	}
	login, err := c.Login.Start(r.Context(), input.Provider, input.AccountID)
	if err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	envelope.WriteJSON(w, 200, loginView(login))
}
func (c *ProviderAccountsController) loginStatus(w http.ResponseWriter, r *http.Request) {
	if !c.loginReady(w, r) {
		return
	}
	login, err := c.Login.Status(r.Context(), chi.URLParam(r, "loginId"))
	if err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	envelope.WriteJSON(w, 200, loginView(login))
}
func (c *ProviderAccountsController) cancelLogin(w http.ResponseWriter, r *http.Request) {
	if !c.loginReady(w, r) {
		return
	}
	if err := c.Login.Cancel(r.Context(), chi.URLParam(r, "loginId")); err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeAccountJSON(r *http.Request, output any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || !strings.HasPrefix(strings.TrimSpace(string(body)), "{") {
		return errors.New("invalid account request body")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON object")
	}
	return nil
}
