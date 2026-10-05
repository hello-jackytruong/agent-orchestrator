package proxyhost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// Relays exist only for the current OAuth attempt. They never bind LAN addresses
// or put callback codes into the renderer or logs.
var relayOwners sync.Map

// StartAccountLogin reserves the local callback and requests an upstream login link.
func (c *Client) StartAccountLogin(ctx context.Context, provider, id string) (ports.ProviderLogin, error) {
	var port int
	var path string
	switch provider {
	case "codex":
		port = 1455
		path = "/auth/callback"
	case "claude":
		port = 54545
		path = "/callback"
	default:
		return ports.ProviderLogin{}, errors.New("unsupported account provider")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return ports.ProviderLogin{}, ports.ErrProviderLoginCallbackBusy
	}
	var login ports.ProviderLogin
	if err = c.Management(ctx, http.MethodGet, "oauth/auth-url?provider="+provider, nil, &login, id); err != nil {
		_ = listener.Close()
		return login, err
	}
	login.ID = id
	login.Provider = provider
	login.Mode = "browser"
	login.Status = "waiting"
	if login.State == "" || login.URL == "" {
		_ = listener.Close()
		return login, errors.New("provider did not return a login link")
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != path || r.URL.Query().Get("state") != login.State {
			http.Error(w, "Unknown login attempt", http.StatusBadRequest)
			return
		}
		callbackCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		body := map[string]string{"provider": provider, "state": login.State, "code": r.URL.Query().Get("code"), "error": r.URL.Query().Get("error")}
		if err := c.Management(callbackCtx, http.MethodPost, "oauth/callback", body, nil, ""); err != nil {
			http.Error(w, "Login could not be completed. Return to AO and retry.", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Login received. You can return to AO."))
	})}
	relayOwners.Store(c.root+"/"+id, server)
	go func() { _ = server.Serve(listener) }()
	time.AfterFunc(6*time.Minute, func() { closeRelay(c.root, id) })
	return login, nil
}
func closeRelay(root, id string) {
	if server, ok := relayOwners.LoadAndDelete(root + "/" + id); ok {
		if callback, valid := server.(*http.Server); valid {
			_ = callback.Close()
		}
	}
}

// AccountLoginStatus polls upstream completion and releases terminal callback listeners.
func (c *Client) AccountLoginStatus(ctx context.Context, login ports.ProviderLogin) (string, error) {
	if login.Mode != "" && login.Mode != "browser" {
		var result ports.ProviderLogin
		if err := c.call(ctx, http.MethodGet, "/ao/login/status?id="+queryEscape(login.ID), nil, &result, nil); err != nil {
			return "", err
		}
		return result.Status, nil
	}
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := c.Management(ctx, http.MethodGet, "oauth/status?state="+queryEscape(login.State), nil, &result, ""); err != nil {
		return "", err
	}
	switch result.Status {
	case "wait":
		return "waiting", nil
	case "ok":
		closeRelay(c.root, login.ID)
		return "complete", nil
	case "error":
		closeRelay(c.root, login.ID)
		return "failed", nil
	}
	return "", errors.New("unknown login status")
}

// CancelAccountLogin cancels upstream login and closes its callback listener.
func (c *Client) CancelAccountLogin(ctx context.Context, login ports.ProviderLogin) error {
	if login.Mode != "" && login.Mode != "browser" {
		return c.call(ctx, http.MethodDelete, "/ao/login/status?id="+queryEscape(login.ID), nil, nil, nil)
	}
	err := c.Management(ctx, http.MethodDelete, "oauth/session?state="+queryEscape(login.State), nil, nil, "")
	closeRelay(c.root, login.ID)
	return err
}

// VerifiedAccountLogin reads the verified identity for the exact login attempt.
func (c *Client) VerifiedAccountLogin(ctx context.Context, id string) (ports.VerifiedProviderLogin, error) {
	var result ports.VerifiedProviderLogin
	err := c.LoginResult(ctx, id, &result)
	return result, err
}

// StartAccountLoginMode forwards credentials only over the private helper channel.
func (c *Client) StartAccountLoginMode(ctx context.Context, provider, id, mode string, input ports.ProviderLoginInput) (ports.ProviderLogin, error) {
	path := ""
	body := map[string]string{"id": id, "provider": provider}
	switch mode {
	case "device":
		if provider != "codex" {
			return ports.ProviderLogin{}, ports.ErrProviderAccountIncompatible
		}
		path = "/ao/login/device/start"
	case "import":
		path = "/ao/login/import"
		body["credential_json"] = input.CredentialJSON
	case "api_key":
		path = "/ao/api-key"
		body["api_key"] = input.APIKey
		body["base_url"] = input.BaseURL
		body["label"] = input.Label
	default:
		return ports.ProviderLogin{}, ports.ErrProviderAccountIncompatible
	}
	if err := c.Ensure(ctx); err != nil {
		return ports.ProviderLogin{}, err
	}
	var login ports.ProviderLogin
	err := c.call(ctx, http.MethodPost, path, body, &login, nil)
	return login, err
}
