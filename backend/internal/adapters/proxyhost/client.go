// Package proxyhost talks to the one detached AO-owned CLIProxyAPI helper.
package proxyhost

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

var errResponse = errors.New("invalid helper response")

var errProtocol = errors.New("account helper protocol is incompatible; update AO after stopping its sessions")

type statusError struct {
	status int
	code   string
}

func (e statusError) Error() string { return fmt.Sprintf("proxy operation failed (HTTP %d)", e.status) }

type manifest struct {
	Port         int    `json:"port"`
	PID          int    `json:"pid"`
	ControlKey   string `json:"control_key"`
	InferenceKey string `json:"inference_key"`
	TicketKey    string `json:"ticket_key"`
}

// Client communicates with the detached helper using its private control identity.
type Client struct {
	mu           sync.Mutex
	root, binary string
	state        manifest
	http         *http.Client
}

// New opens or creates the private helper identity.
func New(root, binary string) (*Client, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("proxy data directory must be absolute")
	}
	path := filepath.Join(root, "run", "host.json")
	c := &Client{root: root, binary: binary, http: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(data, &c.state); err != nil {
			return nil, fmt.Errorf("read proxy identity: %w", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// Losing an established identity is not a first launch: replacing its
		// keys/port would strand existing sessions and could start a second host.
		for _, artifact := range []string{"run/routes.json", "config.yaml", "auth"} {
			if _, statErr := os.Stat(filepath.Join(root, artifact)); !errors.Is(statErr, os.ErrNotExist) {
				return nil, errors.New("proxy identity is missing while helper state exists; restore the original identity before continuing")
			}
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			_ = listener.Close()
			return nil, errors.New("invalid loopback listener address")
		}
		c.state.Port = address.Port
		_ = listener.Close()
		for _, destination := range []*string{&c.state.ControlKey, &c.state.InferenceKey, &c.state.TicketKey} {
			var key [32]byte
			if _, err = rand.Read(key[:]); err != nil {
				return nil, err
			}
			*destination = hex.EncodeToString(key[:])
		}
		if err := c.save(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if c.state.Port < 1 || c.state.Port > 65535 || len(c.state.ControlKey) != 64 || len(c.state.InferenceKey) != 64 || len(c.state.TicketKey) != 64 {
		return nil, errors.New("invalid proxy identity")
	}
	for _, value := range []string{c.state.ControlKey, c.state.InferenceKey, c.state.TicketKey} {
		if _, err := hex.DecodeString(value); err != nil {
			return nil, errors.New("invalid proxy identity key")
		}
	}
	if c.state.ControlKey == c.state.InferenceKey || c.state.ControlKey == c.state.TicketKey || c.state.InferenceKey == c.state.TicketKey {
		return nil, errors.New("proxy identity keys must be distinct")
	}
	return c, nil
}

// Endpoint returns the stable loopback inference address.
func (c *Client) Endpoint() string { return "http://127.0.0.1:" + strconv.Itoa(c.state.Port) }

// TicketKey returns the private key used to derive session tickets.
func (c *Client) TicketKey() ([]byte, error) { return hex.DecodeString(c.state.TicketKey) }
func (c *Client) save() error {
	path := filepath.Join(c.root, "run", "host.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(c.state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".host-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func (c *Client) call(ctx context.Context, method, path string, body, output any, headers map[string]string) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Endpoint()+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.state.ControlKey)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("proxy helper unavailable: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var detail struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&detail)
		if response.StatusCode == http.StatusConflict && detail.Code == "SESSION_BUSY" {
			return ports.ErrProviderAccountBusy
		}
		return statusError{status: response.StatusCode, code: detail.Code}
	}
	if output != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
			return errors.Join(errResponse, err)
		}
	}
	return nil
}
func (c *Client) probe(ctx context.Context) error {
	var status struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err := c.call(ctx, http.MethodGet, "/ao/status", nil, &status, nil); err != nil {
		return err
	}
	if status.ProtocolVersion != 2 {
		return errProtocol
	}
	return nil
}

// Ensure reuses the owned helper or starts it after confirming its previous process is gone.
func (c *Client) Ensure(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	probeCtx, done := context.WithTimeout(ctx, time.Second)
	err := c.probe(probeCtx)
	done()
	if err == nil {
		return nil
	}
	var responseError statusError
	if errors.Is(err, errProtocol) || errors.Is(err, errResponse) || errors.As(err, &responseError) {
		return err
	}
	if c.state.PID > 0 {
		dead, err := processDead(c.state.PID)
		if err != nil || !dead {
			return errors.New("proxy helper ownership is unverified; active sessions were left untouched")
		}
	}
	if c.binary == "" {
		return errors.New("proxy helper is not packaged with this AO build")
	}
	if err := os.MkdirAll(filepath.Join(c.root, "logs"), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(c.root, "logs", "host.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	cmd := exec.Command(c.binary, "--data-dir", c.root, "--port", strconv.Itoa(c.state.Port))
	cmd.Dir = c.root
	cmd.Env = append(os.Environ(), "AO_PROXY_CONTROL_KEY="+c.state.ControlKey, "AO_PROXY_INFERENCE_KEY="+c.state.InferenceKey, "WRITABLE_PATH="+c.root, "MANAGEMENT_PASSWORD=")
	cmd.Stdout = log
	cmd.Stderr = log
	detach(cmd)
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("start proxy helper: %w", err)
	}
	c.state.PID = cmd.Process.Pid
	saveErr := c.save()
	go func() { _ = cmd.Wait() }()
	if saveErr != nil {
		return saveErr
	}
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return errors.New("proxy helper did not become ready")
		case <-ticker.C:
			probeCtx, done := context.WithTimeout(ctx, time.Second)
			err = c.probe(probeCtx)
			done()
			if err == nil {
				return nil
			}
		}
	}
}

// ApplyRoutes requires acknowledgement of the complete routing snapshot.
func (c *Client) ApplyRoutes(ctx context.Context, snapshot ports.ProviderRouteSnapshot) error {
	if err := c.Ensure(ctx); err != nil {
		return err
	}
	var acknowledged ports.ProviderRouteSnapshot
	if err := c.call(ctx, http.MethodPut, "/ao/routes", snapshot, &acknowledged, nil); err != nil {
		return err
	}
	if acknowledged.Revision != snapshot.Revision || !slices.Equal(acknowledged.Routes, snapshot.Routes) || !slices.Equal(acknowledged.AuthIDs, snapshot.AuthIDs) || acknowledged.CodexPrimaryGeneration != snapshot.CodexPrimaryGeneration {
		return errors.New("proxy routing acknowledgement does not match the requested snapshot")
	}
	return nil
}

// QuotaEvents returns provider-confirmed Codex usage-limit events waiting for
// AO to decide whether the primary should move.
func (c *Client) QuotaEvents(ctx context.Context) ([]ports.ProviderQuotaEvent, error) {
	if err := c.Ensure(ctx); err != nil {
		return nil, err
	}
	var response struct {
		Events []ports.ProviderQuotaEvent `json:"events"`
	}
	if err := c.call(ctx, http.MethodGet, "/ao/quota-events", nil, &response, nil); err != nil {
		return nil, err
	}
	return response.Events, nil
}

// AckQuotaEvents removes events that AO has handled or determined to be stale.
func (c *Client) AckQuotaEvents(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := c.Ensure(ctx); err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, "/ao/quota-events/ack", struct {
		IDs []string `json:"ids"`
	}{IDs: ids}, nil, nil)
}

type codexUsageWindow struct {
	UsedPercent   *float64 `json:"used_percent"`
	WindowMinutes int64    `json:"window_minutes"`
	ResetAfter    int64    `json:"reset_after_seconds"`
	ResetAt       int64    `json:"reset_at"`
}

type codexUsageLimits struct {
	Primary         *codexUsageWindow `json:"primary"`
	Secondary       *codexUsageWindow `json:"secondary"`
	PrimaryWindow   *codexUsageWindow `json:"primary_window"`
	SecondaryWindow *codexUsageWindow `json:"secondary_window"`
}

type codexAdditionalLimit struct {
	Name      string            `json:"limit_name"`
	RateLimit *codexUsageLimits `json:"rate_limit"`
	Primary   *codexUsageWindow `json:"primary"`
}

type codexUsageResponse struct {
	PlanType            string                 `json:"plan_type"`
	RateLimits          *codexUsageLimits      `json:"rate_limits"`
	RateLimit           *codexUsageLimits      `json:"rate_limit"`
	AdditionalRateLimit []codexAdditionalLimit `json:"additional_rate_limits"`
}

type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetAt     string  `json:"resets_at"`
}

type claudeUsageResponse struct {
	FiveHour *claudeUsageWindow `json:"five_hour"`
	SevenDay *claudeUsageWindow `json:"seven_day"`
}

// FetchAccountUsage asks the helper to use CLIProxy's native authenticated
// provider usage request. AO never handles tokens or credential indexes.
func (c *Client) FetchAccountUsage(ctx context.Context, provider, authID, credentialRef string) (domain.ProviderAccountUsage, error) {
	if strings.TrimSpace(authID) == "" || strings.TrimSpace(credentialRef) == "" || (provider != "codex" && provider != "claude") {
		return domain.ProviderAccountUsage{}, errors.New("account is not signed in")
	}
	if err := c.Ensure(ctx); err != nil {
		return domain.ProviderAccountUsage{}, err
	}
	var raw json.RawMessage
	if err := c.call(ctx, http.MethodPost, "/ao/account-usage", struct {
		AuthID   string `json:"auth_id"`
		Provider string `json:"provider"`
	}{AuthID: authID, Provider: provider}, &raw, nil); err != nil {
		return domain.ProviderAccountUsage{}, err
	}
	if provider == "claude" {
		var quota claudeUsageResponse
		if err := json.Unmarshal(raw, &quota); err != nil {
			return domain.ProviderAccountUsage{}, err
		}
		usage := domain.ProviderAccountUsage{Status: "available", CheckedAt: time.Now().UTC()}
		appendClaude := func(name string, window *claudeUsageWindow) {
			if window == nil || window.Utilization < 0 || window.Utilization > 100 {
				return
			}
			usage.Windows = append(usage.Windows, domain.ProviderAccountUsageWindow{Name: name, RemainingFraction: 1 - window.Utilization/100, ResetTime: window.ResetAt})
		}
		appendClaude("5 hour", quota.FiveHour)
		appendClaude("7 day", quota.SevenDay)
		if len(usage.Windows) == 0 {
			return domain.ProviderAccountUsage{}, errors.New("Claude usage response contained no quota data")
		}
		return usage, nil
	}
	var quota codexUsageResponse
	if err := json.Unmarshal(raw, &quota); err != nil {
		return domain.ProviderAccountUsage{}, err
	}
	usage := domain.ProviderAccountUsage{Status: "available", CheckedAt: time.Now().UTC()}
	usage.Plan = strings.TrimSpace(quota.PlanType)
	appendWindow := func(name string, window *codexUsageWindow) {
		if window == nil || window.UsedPercent == nil || *window.UsedPercent < 0 || *window.UsedPercent > 100 {
			return
		}
		remaining := 1 - (*window.UsedPercent / 100)
		reset := ""
		if window.ResetAt > 0 {
			reset = time.Unix(window.ResetAt, 0).UTC().Format(time.RFC3339)
		} else if window.ResetAfter >= 0 {
			reset = time.Now().UTC().Add(time.Duration(window.ResetAfter) * time.Second).Format(time.RFC3339)
		}
		usage.Windows = append(usage.Windows, domain.ProviderAccountUsageWindow{Name: name, RemainingFraction: remaining, ResetTime: reset})
	}
	appendLimits := func(limits *codexUsageLimits) {
		if limits == nil {
			return
		}
		primary, secondary := limits.Primary, limits.Secondary
		if primary == nil {
			primary = limits.PrimaryWindow
		}
		if secondary == nil {
			secondary = limits.SecondaryWindow
		}
		appendWindow("primary", primary)
		appendWindow("secondary", secondary)
	}
	appendLimits(quota.RateLimits)
	appendLimits(quota.RateLimit)
	for _, limit := range quota.AdditionalRateLimit {
		rates := limit.RateLimit
		if rates != nil {
			appendWindow(strings.TrimSpace(limit.Name)+" primary", rates.Primary)
			appendWindow(strings.TrimSpace(limit.Name)+" secondary", rates.Secondary)
		} else {
			appendWindow(strings.TrimSpace(limit.Name)+" primary", limit.Primary)
		}
	}
	if usage.Plan == "" && len(usage.Windows) == 0 {
		return domain.ProviderAccountUsage{}, errors.New("Codex usage response contained no quota data")
	}
	return usage, nil
}

// DeleteCredential removes an upstream credential and verifies an already missing file.
func (c *Client) DeleteCredential(ctx context.Context, name string) error {
	if strings.HasPrefix(name, "config:") && len(name) > len("config:") && !strings.ContainsAny(name, "/\\") {
		if err := c.Ensure(ctx); err != nil {
			return err
		}
		return c.call(ctx, http.MethodDelete, "/ao/api-key?ref="+queryEscape(name), nil, nil, nil)
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
		return errors.New("invalid credential reference")
	}
	if err := c.Ensure(ctx); err != nil {
		return err
	}
	err := c.call(ctx, http.MethodDelete, "/v8/management/credentials?name="+queryEscape(name), nil, nil, nil)
	var code statusError
	if errors.As(err, &code) && code.status == http.StatusNotFound {
		var listing struct {
			Files []struct {
				Name string `json:"name"`
			} `json:"files"`
		}
		if listErr := c.call(ctx, http.MethodGet, "/v8/management/credentials?name="+queryEscape(name), nil, &listing, nil); listErr != nil {
			return listErr
		}
		if listing.Files == nil {
			return errors.New("credential deletion could not be verified")
		}
		for _, file := range listing.Files {
			if file.Name == name {
				return err
			}
		}
		return nil
	}
	return err
}

// Management is private to AO's login coordinator; raw responses never reach UI.
func (c *Client) Management(ctx context.Context, method, path string, body, output any, loginID string) error {
	if err := c.Ensure(ctx); err != nil {
		return err
	}
	headers := map[string]string{}
	if loginID != "" {
		headers["X-AO-Login-ID"] = loginID
	}
	return c.call(ctx, method, "/v8/management/"+path, body, output, headers)
}

// LoginResult reads the identity tagged by one successful login.
func (c *Client) LoginResult(ctx context.Context, id string, output any) error {
	return c.call(ctx, http.MethodGet, "/ao/login-result/"+queryEscape(id), nil, output, nil)
}
