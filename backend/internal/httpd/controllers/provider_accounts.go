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

type providerPrimaryOptions interface {
	SetPrimaryWithOptions(context.Context, string, bool) error
}

type providerAccountUsageReader interface {
	AccountUsages(context.Context, []domain.ProviderAccount) map[string]domain.ProviderAccountUsage
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
	r.Patch("/provider-accounts/quota-auto-switch", c.setQuotaAutoSwitch)
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
	case errors.Is(err, ports.ErrProviderQuotaSwitchRequiresReplacement):
		return apierr.Conflict("QUOTA_AUTO_SWITCH_REQUIRES_SECOND_ACCOUNT", ports.ErrProviderQuotaSwitchRequiresReplacement.Error(), nil)
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
	if mode, ok := c.Svc.(interface {
		CodexQuotaAutoSwitch(context.Context) (bool, error)
	}); ok {
		result.CodexQuotaAutoSwitch, err = mode.CodexQuotaAutoSwitch(r.Context())
		if err != nil {
			envelope.WriteError(w, r, accountAPIError(err))
			return
		}
	}
	if mode, ok := c.Svc.(interface {
		ClaudeQuotaAutoSwitch(context.Context) (bool, error)
	}); ok {
		result.ClaudeQuotaAutoSwitch, err = mode.ClaudeQuotaAutoSwitch(r.Context())
		if err != nil {
			envelope.WriteError(w, r, accountAPIError(err))
			return
		}
	}
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
	usageByAccount := map[string]domain.ProviderAccountUsage{}
	if reader, ok := c.Svc.(providerAccountUsageReader); ok {
		usageByAccount = reader.AccountUsages(r.Context(), state.Accounts)
	}
	for _, provider := range []string{"codex", "claude"} {
		result.Defaults = append(result.Defaults, ProviderPrimaryView{Provider: provider, PrimaryID: primaries[provider], Managed: true})
	}
	for _, a := range state.Accounts {
		view := ProviderAccountView{ID: a.ID, Provider: a.Provider, Email: a.Email, Kind: a.Kind, SignedIn: a.CredentialRef != "", Primary: primaries[a.Provider] == a.ID, Sessions: []string{}}
		if usage, ok := usageByAccount[a.ID]; ok {
			view.Usage = providerAccountUsageView(usage)
		}
		for _, route := range state.Routes {
			if route.AccountID == a.ID {
				view.Sessions = append(view.Sessions, string(route.SessionID))
			}
		}
		result.Accounts = append(result.Accounts, view)
	}
	envelope.WriteJSON(w, 200, result)
}

func providerAccountUsageView(usage domain.ProviderAccountUsage) *ProviderAccountUsageView {
	view := &ProviderAccountUsageView{Status: usage.Status, Plan: usage.Plan, CheckedAt: usage.CheckedAt, Message: usage.Message}
	if usage.Windows != nil {
		view.Windows = make([]ProviderAccountUsageWindowView, 0, len(usage.Windows))
		for _, window := range usage.Windows {
			view.Windows = append(view.Windows, ProviderAccountUsageWindowView{Name: window.Name, RemainingFraction: window.RemainingFraction, ResetTime: window.ResetTime})
		}
	}
	return view
}

func (c *ProviderAccountsController) setQuotaAutoSwitch(w http.ResponseWriter, r *http.Request) {
	if !c.ready(w, r) {
		return
	}
	var input UpdateCodexQuotaAutoSwitchRequest
	if err := decodeAccountJSON(r, &input); err != nil || input.Enabled == nil {
		envelope.WriteError(w, r, apierr.Invalid("QUOTA_AUTO_SWITCH_INVALID", "enabled must be true or false", nil))
		return
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "codex"
	}
	var setter func(context.Context, bool) error
	switch provider {
	case "codex":
		if service, ok := c.Svc.(interface {
			SetCodexQuotaAutoSwitch(context.Context, bool) error
		}); ok {
			setter = service.SetCodexQuotaAutoSwitch
		}
	case "claude":
		if service, ok := c.Svc.(interface {
			SetClaudeQuotaAutoSwitch(context.Context, bool) error
		}); ok {
			setter = service.SetClaudeQuotaAutoSwitch
		}
	default:
		envelope.WriteError(w, r, apierr.Invalid("PROVIDER_REQUIRED", "Choose Codex or Claude", nil))
		return
	}
	if setter == nil {
		envelope.WriteAPIError(w, r, http.StatusNotImplemented, "service_unavailable", "PROVIDER_ACCOUNTS_UNAVAILABLE", "Quota switching is unavailable in this build", nil)
		return
	}
	if err := setter(r.Context(), *input.Enabled); err != nil {
		envelope.WriteError(w, r, accountAPIError(err))
		return
	}
	c.list(w, r)
}
func (c *ProviderAccountsController) setPrimary(w http.ResponseWriter, r *http.Request) {
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
	var err error
	if input.MoveExisting != nil {
		if setter, ok := c.Svc.(providerPrimaryOptions); ok {
			err = setter.SetPrimaryWithOptions(r.Context(), chi.URLParam(r, "accountId"), *input.MoveExisting)
		} else {
			err = apierr.Invalid("PROVIDER_ACCOUNTS_UNAVAILABLE", "Account default options are unavailable in this build", nil)
		}
	} else {
		err = c.Svc.SetPrimary(r.Context(), chi.URLParam(r, "accountId"))
	}
	if err != nil {
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
	return ProviderLoginResponse{ID: login.ID, Provider: login.Provider, Mode: login.Mode, URL: login.URL, Code: login.Code, ExpiresIn: login.ExpiresIn, Status: login.Status, AccountID: login.AccountID}
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
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "browser"
	}
	switch mode {
	case "browser":
	case "device":
		if input.Provider != "codex" {
			envelope.WriteError(w, r, apierr.Invalid("LOGIN_MODE_UNSUPPORTED", "Device login is available for Codex only", nil))
			return
		}
	case "import":
		if strings.TrimSpace(input.CredentialJSON) == "" {
			envelope.WriteError(w, r, apierr.Invalid("CREDENTIAL_JSON_REQUIRED", "Paste or choose a credential JSON file", nil))
			return
		}
	case "api_key":
		if strings.TrimSpace(input.APIKey) == "" || strings.TrimSpace(input.BaseURL) == "" {
			envelope.WriteError(w, r, apierr.Invalid("API_KEY_FIELDS_REQUIRED", "API key and base URL are required", nil))
			return
		}
	default:
		envelope.WriteError(w, r, apierr.Invalid("LOGIN_MODE_UNSUPPORTED", "Choose browser, device, API key, or JSON import", nil))
		return
	}
	var login ports.ProviderLogin
	var err error
	if mode == "browser" {
		login, err = c.Login.Start(r.Context(), input.Provider, input.AccountID)
	} else if modes, ok := c.Login.(interface {
		StartRequest(context.Context, string, string, string, ports.ProviderLoginInput) (ports.ProviderLogin, error)
	}); ok {
		login, err = modes.StartRequest(r.Context(), input.Provider, input.AccountID, mode, ports.ProviderLoginInput{APIKey: input.APIKey, BaseURL: input.BaseURL, Label: input.Label, CredentialJSON: input.CredentialJSON})
	} else {
		envelope.WriteError(w, r, apierr.Invalid("LOGIN_MODE_UNSUPPORTED", "This login method is unavailable in this build", nil))
		return
	}
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
