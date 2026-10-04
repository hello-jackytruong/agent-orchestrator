package proxyhost

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func privateClient(t *testing.T, handler transportFunc) *Client {
	t.Helper()
	return &Client{root: t.TempDir(), state: manifest{Port: 12345, ControlKey: strings.Repeat("c", 64), InferenceKey: strings.Repeat("b", 64), TicketKey: strings.Repeat("a", 64)}, http: &http.Client{Transport: handler, Timeout: time.Second}}
}
func writeIdentity(t *testing.T, root string, m manifest) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "run"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "run", "host.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestProviderHostLoadsStablePrivateIdentity(t *testing.T) {
	root := t.TempDir()
	m := manifest{Port: 32456, PID: 123, ControlKey: strings.Repeat("c", 64), InferenceKey: strings.Repeat("b", 64), TicketKey: strings.Repeat("a", 64)}
	writeIdentity(t, root, m)
	c, err := New(root, "/tmp/ao-proxy-host")
	if err != nil {
		t.Fatal(err)
	}
	if c.state != m || c.Endpoint() != "http://127.0.0.1:32456" {
		t.Fatalf("state=%+v endpoint=%s", c.state, c.Endpoint())
	}
	key, err := c.TicketKey()
	want, _ := hex.DecodeString(m.TicketKey)
	if err != nil || !reflect.DeepEqual(key, want) {
		t.Fatalf("ticket bytes=%x err=%v", key, err)
	}
	again, err := New(root, "different-binary")
	if err != nil || again.state != m {
		t.Fatalf("restart changed identity=%+v err=%v", again, err)
	}
	stat, err := os.Stat(filepath.Join(root, "run", "host.json"))
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("identity mode=%v err=%v", stat, err)
	}
}
func TestProviderHostRejectsCorruptOrUnsafeIdentity(t *testing.T) {
	valid := manifest{Port: 32456, ControlKey: strings.Repeat("c", 64), InferenceKey: strings.Repeat("b", 64), TicketKey: strings.Repeat("a", 64)}
	cases := []struct {
		name   string
		change func(*manifest)
	}{
		{"zero-port", func(m *manifest) { m.Port = 0 }},
		{"negative-port", func(m *manifest) { m.Port = -1 }},
		{"overflow-port", func(m *manifest) { m.Port = 65536 }},
		{"short-control-key", func(m *manifest) { m.ControlKey = "c" }},
		{"short-inference-key", func(m *manifest) { m.InferenceKey = "i" }},
		{"short-ticket-key", func(m *manifest) { m.TicketKey = "t" }},
		{"non-hex-control-key", func(m *manifest) { m.ControlKey = strings.Repeat("z", 64) }},
		{"non-hex-inference-key", func(m *manifest) { m.InferenceKey = strings.Repeat("z", 64) }},
		{"non-hex-ticket-key", func(m *manifest) { m.TicketKey = strings.Repeat("z", 64) }},
		{"same-api-keys", func(m *manifest) { m.InferenceKey = m.ControlKey }},
		{"ticket-is-control", func(m *manifest) { m.TicketKey = m.ControlKey }},
		{"ticket-is-inference", func(m *manifest) { m.TicketKey = m.InferenceKey }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			m := valid
			tc.change(&m)
			writeIdentity(t, root, m)
			if _, err := New(root, ""); err == nil {
				t.Fatal("unsafe saved identity accepted")
			}
		})
	}
	t.Run("invalid-json", func(t *testing.T) {
		root := t.TempDir()
		writeIdentity(t, root, valid)
		if err := os.WriteFile(filepath.Join(root, "run", "host.json"), []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := New(root, ""); err == nil {
			t.Fatal("corrupt identity accepted")
		}
	})
	t.Run("relative-data-directory", func(t *testing.T) {
		if _, err := New("relative", ""); err == nil {
			t.Fatal("relative identity accepted")
		}
	})
}
func TestProviderHostHealthyReuseDoesNotSpawnOrChangePID(t *testing.T) {
	calls := 0
	c := privateClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/ao/status" || r.URL.Host != "127.0.0.1:12345" {
			t.Fatalf("unsafe probe=%s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("c", 64) {
			t.Fatal("probe omitted private control key")
		}
		return fakeResponse(200, `{"protocol_version":1,"revision":9,"routes":[]}`), nil
	})
	c.state.PID = 6789
	for i := 0; i < 3; i++ {
		if err := c.Ensure(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 || c.state.PID != 6789 {
		t.Fatalf("reuse calls=%d pid=%d", calls, c.state.PID)
	}
}
func TestProviderHostIncompatibleProtocolNeverStartsReplacement(t *testing.T) {
	for _, body := range []string{`{"protocol_version":0}`, `{"protocol_version":2}`, `{"revision":0,"routes":[]}`} {
		t.Run(body, func(t *testing.T) {
			c := privateClient(t, func(*http.Request) (*http.Response, error) { return fakeResponse(200, body), nil })
			c.binary = "/binary-that-must-never-run"
			if err := c.Ensure(context.Background()); !errors.Is(err, errProtocol) {
				t.Fatalf("err=%v", err)
			}
			if c.state.PID != 0 {
				t.Fatal("replacement launched for incompatible running helper")
			}
		})
	}
}
func TestProviderHostUnavailableBuildFailsWithoutAmbientFallback(t *testing.T) {
	c := privateClient(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	if err := c.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "not packaged") {
		t.Fatalf("err=%v", err)
	}
	if c.state.PID != 0 {
		t.Fatal("unavailable build invented a helper pid")
	}
}
func TestProviderHostUnverifiedLiveOwnerIsNeverKilledOrReplaced(t *testing.T) {
	c := privateClient(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout") })
	c.state.PID = os.Getpid()
	c.binary = "/must-never-run"
	if err := c.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "ownership is unverified") {
		t.Fatalf("err=%v", err)
	}
	if c.state.PID != os.Getpid() {
		t.Fatal("unverified live process was replaced")
	}
}
func TestProviderHostRouteAcknowledgementMustBeExact(t *testing.T) {
	request := ports.ProviderRouteSnapshot{Revision: 8, Routes: []ports.ProviderRoute{{SessionID: domain.SessionID("s"), Provider: "codex", TicketHash: "hash", AuthID: "exact"}}}
	cases := []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"exact", `{"revision":8,"routes":[{"session_id":"s","provider":"codex","ticket_hash":"hash","auth_id":"exact"}]}`, 200, false},
		{"wrong-revision", `{"revision":7,"routes":[{"session_id":"s","provider":"codex","ticket_hash":"hash","auth_id":"exact"}]}`, 200, true},
		{"different-account", `{"revision":8,"routes":[{"session_id":"s","provider":"codex","ticket_hash":"hash","auth_id":"other"}]}`, 200, true},
		{"missing-route", `{"revision":8,"routes":[]}`, 200, true},
		{"invalid-json", `{`, 200, true},
		{"busy", `{"error":"busy"}`, 409, true},
		{"disk-error", `{"error":"disk"}`, 500, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			puts := 0
			c := privateClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/ao/status" {
					return fakeResponse(200, `{"protocol_version":1}`), nil
				}
				puts++
				if r.Method != http.MethodPut || r.URL.Path != "/ao/routes" {
					t.Fatalf("unexpected mutation=%s %s", r.Method, r.URL)
				}
				var sent ports.ProviderRouteSnapshot
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(sent, request) {
					t.Fatalf("route changed in transport=%+v", sent)
				}
				return fakeResponse(tc.status, tc.body), nil
			})
			err := c.ApplyRoutes(context.Background(), request)
			if (err != nil) != tc.wantErr || puts != 1 {
				t.Fatalf("puts=%d err=%v wantErr=%v", puts, err, tc.wantErr)
			}
		})
	}
}
func TestProviderHostDeletingCredentialEscapesExactlyOneName(t *testing.T) {
	name := "alice+work?all=true&name=other.json"
	deletes := 0
	c := privateClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/ao/status" {
			return fakeResponse(200, `{"protocol_version":1}`), nil
		}
		deletes++
		if r.Method != http.MethodDelete || r.URL.Query().Get("name") != name || len(r.URL.Query()) != 1 {
			t.Fatalf("credential query injection=%s", r.URL)
		}
		return fakeResponse(200, `{"status":"ok"}`), nil
	})
	if err := c.DeleteCredential(context.Background(), name); err != nil || deletes != 1 {
		t.Fatalf("delete count=%d err=%v", deletes, err)
	}
}
func TestProviderHostDeleteReplayRequiresVerifiedAbsence(t *testing.T) {
	cases := []struct {
		name, listing string
		listStatus    int
		wantErr       bool
	}{
		{"already-absent", `{"files":[]}`, 200, false},
		{"still-present", `{"files":[{"name":"a.json"}]}`, 200, true},
		{"other-credential", `{"files":[{"name":"b.json"}]}`, 200, false},
		{"missing-files-field", `{}`, 200, true},
		{"malformed-list", `{`, 200, true},
		{"unsupported-list", `{}`, 404, true},
		{"unavailable-list", `{}`, 503, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := privateClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/ao/status" {
					return fakeResponse(200, `{"protocol_version":1}`), nil
				}
				if r.Method == http.MethodDelete {
					return fakeResponse(404, `{"error":"not found"}`), nil
				}
				if r.Method != http.MethodGet || r.URL.Query().Get("name") != "a.json" {
					t.Fatalf("verification=%s %s", r.Method, r.URL)
				}
				return fakeResponse(tc.listStatus, tc.listing), nil
			})
			err := c.DeleteCredential(context.Background(), "a.json")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
func TestProviderHostRawErrorNeverExposesProviderResponse(t *testing.T) {
	c := privateClient(t, func(*http.Request) (*http.Response, error) {
		return fakeResponse(500, `{"refresh_token":"PRIVATE-REFRESH","access_token":"PRIVATE-ACCESS"}`), nil
	})
	err := c.call(context.Background(), http.MethodGet, "/ao/status", nil, nil, nil)
	if err == nil || strings.Contains(err.Error(), "PRIVATE") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("unsafe error=%v", err)
	}
}

func TestProviderHostMissingIdentityDoesNotReplaceEstablishedHelper(t *testing.T) {
	for _, name := range []string{"run/routes.json", "config.yaml", "auth/account.json"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			original := []byte("existing private helper state")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			client, err := New(root, "/binary-that-must-never-run")
			if err == nil || client != nil {
				t.Fatal("missing established identity generated a new endpoint and ticket key")
			}
			if !strings.Contains(err.Error(), "identity") {
				t.Fatalf("missing identity was not explained: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "run", "host.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("replacement identity was persisted")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(original) {
				t.Fatal("existing helper state was modified")
			}
		})
	}
}
