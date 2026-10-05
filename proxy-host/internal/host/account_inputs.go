package host

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

const deviceClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const devicePage = "https://auth.openai.com/codex/device"
const deviceRedirect = "https://auth.openai.com/deviceauth/callback"

type loginOperation struct {
	ID        string            `json:"id"`
	Provider  string            `json:"provider"`
	Mode      string            `json:"mode"`
	Status    string            `json:"status"`
	URL       string            `json:"url,omitempty"`
	Code      string            `json:"code,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
	Result    map[string]string `json:"-"`
	cancel    context.CancelFunc
}

// LoginInputs extends the SDK's browser flow with private credential inputs.
// The config file is the source of truth; SDK watchers own runtime loading.
// No API keys or token JSON are returned by these operations.
type LoginInputs struct {
	mu                  sync.Mutex
	configMu            sync.Mutex
	configPath, authDir string
	auth                *coreauth.Manager
	ops                 map[string]*loginOperation
	client              *http.Client
	deviceOrigin        string
}

func NewLoginInputs(configPath, authDir string) *LoginInputs {
	return &LoginInputs{configPath: configPath, authDir: authDir, ops: make(map[string]*loginOperation), client: &http.Client{Timeout: 15 * time.Second}, deviceOrigin: "https://auth.openai.com"}
}

func validLoginID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\.") && strings.TrimSpace(id) == id
}
func (l *LoginInputs) operation(id string) (loginOperation, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	op, ok := l.ops[id]
	if !ok {
		return loginOperation{}, false
	}
	return *op, true
}
func (l *LoginInputs) begin(id, provider, mode string, timeout time.Duration) (context.Context, *loginOperation, error) {
	if !validLoginID(id) || (provider != "codex" && provider != "claude") {
		return nil, nil, errors.New("invalid login request")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.ops[id]; exists {
		return nil, nil, errors.New("login already exists")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	op := &loginOperation{ID: id, Provider: provider, Mode: mode, Status: "waiting", ExpiresIn: int(timeout.Seconds()), cancel: cancel}
	l.ops[id] = op
	return ctx, op, nil
}
func (l *LoginInputs) finish(id string, result map[string]string, err error) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	op := l.ops[id]
	if op == nil || op.Status != "waiting" {
		return false
	}
	op.cancel()
	op.Status = "failed"
	if err == nil {
		op.Status = "complete"
		op.Result = result
	}
	return err == nil
}
func (l *LoginInputs) cancel(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if op := l.ops[id]; op != nil && op.Status == "waiting" {
		op.Status = "cancelled"
		op.cancel()
	}
}

func (l *LoginInputs) startDevice(ctx context.Context, id string) (loginOperation, error) {
	if !validLoginID(id) {
		return loginOperation{}, errors.New("invalid login request")
	}
	l.mu.Lock()
	_, alreadyRunning := l.ops[id]
	l.mu.Unlock()
	if alreadyRunning {
		return loginOperation{}, errors.New("login already exists")
	}
	if l.client == nil {
		return loginOperation{}, errors.New("sign-in client is unavailable")
	}
	var code struct {
		DeviceID  string          `json:"device_auth_id"`
		Code      string          `json:"user_code"`
		Alternate string          `json:"usercode"`
		Interval  json.RawMessage `json:"interval"`
	}
	if err := l.request(ctx, l.deviceOrigin+"/api/accounts/deviceauth/usercode", "application/json", mustJSON(map[string]string{"client_id": deviceClientID}), &code); err != nil {
		return loginOperation{}, err
	}
	if code.Code == "" {
		code.Code = code.Alternate
	}
	if code.DeviceID == "" || code.Code == "" {
		return loginOperation{}, errors.New("device login did not return a code")
	}
	worker, op, err := l.begin(id, "codex", "device", 15*time.Minute)
	if err != nil {
		return loginOperation{}, err
	}
	l.mu.Lock()
	op.URL = devicePage
	op.Code = code.Code
	view := *op
	l.mu.Unlock()
	interval := 5
	raw := strings.Trim(string(code.Interval), "\"")
	if n, e := strconv.Atoi(raw); e == nil && n > 0 && n <= 60 {
		interval = n
	}
	go l.runDevice(worker, id, code.DeviceID, code.Code, time.Duration(interval)*time.Second)
	return view, nil
}

func (l *LoginInputs) runDevice(ctx context.Context, id, deviceID, code string, interval time.Duration) {
	var authorization struct {
		Code      string `json:"authorization_code"`
		Verifier  string `json:"code_verifier"`
		Challenge string `json:"code_challenge"`
	}
	for {
		err := l.request(ctx, l.deviceOrigin+"/api/accounts/deviceauth/token", "application/json", mustJSON(map[string]string{"device_auth_id": deviceID, "user_code": code}), &authorization)
		if err == nil {
			break
		}
		var status inputHTTPError
		if !errors.As(err, &status) || (status != 403 && status != 404) {
			l.finish(id, nil, err)
			return
		}
		select {
		case <-ctx.Done():
			l.finish(id, nil, ctx.Err())
			return
		case <-time.After(interval):
		}
	}
	if authorization.Code == "" || authorization.Verifier == "" || authorization.Challenge == "" {
		l.finish(id, nil, errors.New("incomplete device authorization"))
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {deviceClientID}, "code": {authorization.Code}, "redirect_uri": {deviceRedirect}, "code_verifier": {authorization.Verifier}}
	var token struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		ID      string `json:"id_token"`
		Expires int    `json:"expires_in"`
	}
	if err := l.request(ctx, l.deviceOrigin+"/oauth/token", "application/x-www-form-urlencoded", []byte(form.Encode()), &token); err != nil {
		l.finish(id, nil, err)
		return
	}
	claims := jwtClaims(token.ID)
	email := stringValue(claims, "email")
	if token.Access == "" || token.Refresh == "" || email == "" {
		l.finish(id, nil, errors.New("incomplete device credentials"))
		return
	}
	scope, _ := claims["https://api.openai.com/auth"].(map[string]any)
	value := map[string]any{"type": "codex", "access_token": token.Access, "refresh_token": token.Refresh, "id_token": token.ID, "email": email, "account_id": stringValue(scope, "chatgpt_account_id"), "plan_type": stringValue(scope, "chatgpt_plan_type"), "last_refresh": time.Now().UTC().Format(time.RFC3339), "expired": time.Now().UTC().Add(time.Duration(token.Expires) * time.Second).Format(time.RFC3339)}
	l.installFile(ctx, id, "codex", "oauth", email, mustJSON(value))
}

func (l *LoginInputs) importJSON(id, provider, raw string) (loginOperation, error) {
	if provider != "codex" && provider != "claude" {
		return loginOperation{}, errors.New("unsupported credential provider")
	}
	if len(raw) > 1<<20 {
		return loginOperation{}, errors.New("credential JSON exceeds 1 MiB")
	}
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil || value == nil {
		return loginOperation{}, errors.New("credential JSON is invalid")
	}
	if stringValue(value, "type") != provider {
		return loginOperation{}, errors.New("credential JSON provider does not match")
	}
	email := stringValue(value, "email")
	if email == "" {
		email = stringValue(jwtClaims(stringValue(value, "id_token")), "email")
		value["email"] = email
	}
	hasSessionKey := provider == "claude" && stringValue(value, "session_key") != ""
	if email == "" || (stringValue(value, "access_token") == "" && stringValue(value, "refresh_token") == "" && !hasSessionKey) {
		return loginOperation{}, errors.New("credential JSON requires an email and access or refresh token")
	}
	// Import only credential fields. Routing/network policy must stay AO-owned.
	allowed := map[string]any{"type": provider, "email": email}
	for _, key := range []string{"access_token", "refresh_token", "id_token", "session_key", "account_id", "expired", "plan_type", "last_refresh"} {
		if v, ok := value[key]; ok {
			if _, valid := v.(string); !valid {
				return loginOperation{}, errors.New("credential fields must be strings")
			}
			allowed[key] = v
		}
	}
	ctx, op, err := l.begin(id, provider, "import", time.Minute)
	if err != nil {
		return loginOperation{}, err
	}
	view := *op
	go l.installFile(ctx, id, provider, "imported", email, mustJSON(allowed))
	return view, nil
}

func (l *LoginInputs) installFile(ctx context.Context, id, provider, kind, email string, data []byte) {
	name := "ao-" + id + ".json"
	if err := ctx.Err(); err != nil {
		l.finish(id, nil, err)
		return
	}
	if err := l.writeAuthFile(name, data); err != nil {
		l.finish(id, nil, err)
		return
	}
	auth, err := l.waitAuth(ctx, func(a *coreauth.Auth) bool { return a.Provider == provider && filepath.Base(a.FileName) == name })
	result := map[string]string{"provider": provider, "kind": kind, "email": email, "credential_ref": name}
	if auth != nil {
		result["auth_id"] = auth.ID
	}
	if !l.finish(id, result, err) {
		_ = os.Remove(filepath.Join(l.authDir, name))
	}
}

func (l *LoginInputs) writeAuthFile(name string, data []byte) error {
	if !json.Valid(data) {
		return errors.New("invalid auth JSON")
	}
	if filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return errors.New("invalid auth filename")
	}
	return writePrivate(filepath.Join(l.authDir, name), data)
}

func (l *LoginInputs) updateConfig(change func(*config.Config) error) error {
	l.configMu.Lock()
	defer l.configMu.Unlock()
	cfg, err := config.LoadConfig(l.configPath)
	if err != nil {
		return err
	}
	if err = change(cfg); err != nil {
		return err
	}
	return config.SaveConfigPreserveComments(l.configPath, cfg)
}

func (l *LoginInputs) addAPIKey(id, provider, key, base, label string) (loginOperation, error) {
	if provider != "codex" && provider != "claude" {
		return loginOperation{}, errors.New("unsupported API-key provider")
	}
	key, base, label = strings.TrimSpace(key), strings.TrimRight(strings.TrimSpace(base), "/"), strings.TrimSpace(label)
	u, err := url.Parse(base)
	if key == "" || err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return loginOperation{}, errors.New("API key and a valid HTTP(S) base URL are required")
	}
	ctx, op, err := l.begin(id, provider, "api_key", time.Minute)
	if err != nil {
		return loginOperation{}, err
	}
	err = l.updateConfig(func(cfg *config.Config) error {
		if provider == "codex" {
			for _, entry := range cfg.CodexKey {
				if entry.APIKey == key && entry.BaseURL == base {
					return errors.New("API key already exists")
				}
			}
			cfg.CodexKey = append(cfg.CodexKey, config.CodexKey{APIKey: key, BaseURL: base})
		} else {
			for _, entry := range cfg.ClaudeKey {
				if entry.APIKey == key && entry.BaseURL == base {
					return errors.New("API key already exists")
				}
			}
			cfg.ClaudeKey = append(cfg.ClaudeKey, config.ClaudeKey{APIKey: key, BaseURL: base})
		}
		return nil
	})
	if err != nil {
		l.finish(id, nil, err)
		return loginOperation{}, err
	}
	view := *op
	go func() {
		auth, err := l.waitAuth(ctx, func(a *coreauth.Auth) bool {
			return a.Provider == provider && a.Attributes["api_key"] == key && a.Attributes["base_url"] == base
		})
		if label == "" && auth != nil {
			label = strings.Title(provider) + " API key " + auth.ID
		}
		result := map[string]string{"provider": provider, "kind": "api_key", "email": label}
		if auth != nil {
			result["credential_ref"] = "config:" + auth.ID
			result["auth_id"] = auth.ID
		}
		if !l.finish(id, result, err) {
			_ = l.removeAPIKey(provider, key, base)
		}
	}()
	return view, nil
}

func (l *LoginInputs) removeAPIKey(provider, key, base string) error {
	return l.updateConfig(func(cfg *config.Config) error {
		if provider == "codex" {
			next := cfg.CodexKey[:0]
			for _, e := range cfg.CodexKey {
				if e.APIKey != key || e.BaseURL != base {
					next = append(next, e)
				}
			}
			cfg.CodexKey = next
		} else {
			next := cfg.ClaudeKey[:0]
			for _, e := range cfg.ClaudeKey {
				if e.APIKey != key || e.BaseURL != base {
					next = append(next, e)
				}
			}
			cfg.ClaudeKey = next
		}
		return nil
	})
}
func (l *LoginInputs) deleteAPIKey(ctx context.Context, ref string) error {
	if !strings.HasPrefix(ref, "config:") || len(ref) == len("config:") || strings.ContainsAny(ref, "/\\") {
		return errors.New("invalid API-key reference")
	}
	if l.auth == nil {
		return errors.New("CLIProxy auth manager is unavailable")
	}
	id := strings.TrimPrefix(ref, "config:")
	auth, ok := l.auth.GetByID(id)
	if !ok {
		return nil
	}
	if auth.Attributes["api_key"] == "" {
		return errors.New("invalid API-key reference")
	}
	if err := l.removeAPIKey(auth.Provider, auth.Attributes["api_key"], auth.Attributes["base_url"]); err != nil {
		return err
	}
	l.auth.Remove(ctx, id)
	return nil
}
func (l *LoginInputs) waitAuth(ctx context.Context, match func(*coreauth.Auth) bool) (*coreauth.Auth, error) {
	if l.auth == nil {
		return nil, errors.New("CLIProxy auth manager is unavailable")
	}
	for {
		for _, a := range l.auth.List() {
			if match(a) {
				return a, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type inputHTTPError int

func (e inputHTTPError) Error() string { return fmt.Sprintf("sign-in provider returned HTTP %d", e) }
func (l *LoginInputs) request(ctx context.Context, endpoint, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return errors.New("sign-in provider is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return inputHTTPError(resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return errors.New("invalid sign-in provider response")
	}
	return nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func stringValue(v map[string]any, key string) string {
	s, _ := v[key].(string)
	return strings.TrimSpace(s)
}
func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	_ = json.Unmarshal(b, &claims)
	return claims
}
