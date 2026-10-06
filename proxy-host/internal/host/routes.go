// Package host embeds CLIProxyAPI behind AO's exact-session routing boundary.
package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrBusy = errors.New("session has an in-flight model request")
var ErrRevision = errors.New("routing revision conflict")

// Route carries a hashed capability, never a provider credential.
type Route struct {
	SessionID  string `json:"session_id"`
	TicketHash string `json:"ticket_hash"`
	Provider   string `json:"provider"`
	AuthID     string `json:"auth_id"`
}
type Snapshot struct {
	Revision                uint64   `json:"revision"`
	Routes                  []Route  `json:"routes"`
	AuthIDs                 []string `json:"auth_ids,omitempty"`
	RequestBoundary         bool     `json:"request_boundary,omitempty"`
	CodexPrimaryGeneration  int64    `json:"codex_primary_generation,omitempty"`
	ClaudePrimaryGeneration int64    `json:"claude_primary_generation,omitempty"`
}

type QuotaEvent struct {
	ID         string `json:"id"`
	Provider   string `json:"provider,omitempty"`
	AuthID     string `json:"auth_id"`
	ResetAt    int64  `json:"reset_at,omitempty"`
	Generation int64  `json:"generation,omitempty"`
}

// Routes serializes durable snapshots with request admission. In-flight requests
// retain their selected account; provider rebinds may apply at the next request.
type Routes struct {
	mu         sync.Mutex
	path       string
	quotaPath  string
	snapshot   Snapshot
	quota      []QuotaEvent
	byTicket   map[string]Route
	active     map[string]int
	activeAuth map[string]int
}

func OpenRoutes(path string) (*Routes, error) {
	r := &Routes{snapshot: Snapshot{Routes: []Route{}}, path: path, quotaPath: path + ".quota", byTicket: make(map[string]Route), active: make(map[string]int), activeAuth: make(map[string]int)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		// A quota sidecar can only be useful after a routing snapshot exists, but
		// loading it here makes recovery tolerant of an interrupted first write.
		if quotaData, quotaErr := os.ReadFile(r.quotaPath); quotaErr == nil {
			if err := json.Unmarshal(quotaData, &r.quota); err != nil {
				return nil, fmt.Errorf("read quota events: %w", err)
			}
		}
		return r, nil
	}
	if err = json.Unmarshal(data, &r.snapshot); err != nil {
		return nil, fmt.Errorf("read routing snapshot: %w", err)
	}
	index, err := validateSnapshot(r.snapshot)
	if err != nil {
		return nil, err
	}
	r.byTicket = index
	if data, quotaErr := os.ReadFile(r.quotaPath); quotaErr == nil {
		if err := json.Unmarshal(data, &r.quota); err != nil {
			return nil, fmt.Errorf("read quota events: %w", err)
		}
	} else if !errors.Is(quotaErr, os.ErrNotExist) {
		return nil, quotaErr
	}
	return r, nil
}

func (r *Routes) RecordQuota(authID string, resetAt time.Time) {
	r.RecordProviderQuota("codex", authID, resetAt)
}

func (r *Routes) RecordProviderQuota(provider, authID string, resetAt time.Time) {
	if authID == "" {
		return
	}
	if provider == "" {
		provider = "codex"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.quota {
		eventProvider := event.Provider
		if eventProvider == "" {
			eventProvider = "codex"
		}
		if eventProvider == provider && event.AuthID == authID {
			return
		}
	}
	generation := r.snapshot.CodexPrimaryGeneration
	if provider == "claude" {
		generation = r.snapshot.ClaudePrimaryGeneration
	}
	if generation == 0 {
		generation = 1
	}
	event := QuotaEvent{ID: TicketHash("quota:" + provider + ":" + authID), Provider: provider, AuthID: authID, Generation: generation}
	if provider == "codex" {
		event.ID = TicketHash("quota:" + authID) // Preserve IDs held by older helpers.
	}
	if !resetAt.IsZero() {
		event.ResetAt = resetAt.Unix()
	}
	r.quota = append(r.quota, event)
	data, _ := json.Marshal(r.quota)
	_ = writePrivate(r.quotaPath, data)
}

func (r *Routes) QuotaEvents() []QuotaEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]QuotaEvent(nil), r.quota...)
}

func (r *Routes) AckQuotaEvents(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	kept := r.quota[:0]
	for _, event := range r.quota {
		if _, ok := seen[event.ID]; !ok {
			kept = append(kept, event)
		}
	}
	r.quota = kept
	data, err := json.Marshal(r.quota)
	if err != nil {
		return err
	}
	return writePrivate(r.quotaPath, data)
}
func TicketHash(ticket string) string {
	sum := sha256.Sum256([]byte(ticket))
	return hex.EncodeToString(sum[:])
}
func validateSnapshot(s Snapshot) (map[string]Route, error) {
	index := make(map[string]Route, len(s.Routes))
	sessions := make(map[string]bool, len(s.Routes))
	for _, route := range s.Routes {
		if s.AuthIDs != nil && route.AuthID != "" && !slices.Contains(s.AuthIDs, route.AuthID) {
			return nil, errors.New("route account absent from signed-in inventory")
		}
		hash, err := hex.DecodeString(route.TicketHash)
		if err != nil || len(hash) != 32 || strings.ToLower(route.TicketHash) != route.TicketHash || route.SessionID == "" || (route.Provider != "codex" && route.Provider != "claude") {
			return nil, errors.New("invalid session route")
		}
		if _, ok := index[route.TicketHash]; ok || sessions[route.SessionID] {
			return nil, errors.New("duplicate session route")
		}
		index[route.TicketHash] = route
		sessions[route.SessionID] = true
	}
	return index, nil
}
func (r *Routes) Apply(s Snapshot) error {
	requestBoundary := s.RequestBoundary
	s.RequestBoundary = false // Admission instruction, not an effective routing fact.
	index, err := validateSnapshot(s)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Exact replay is safe after a lost acknowledgement; mismatched replay is not.
	old, _ := json.Marshal(r.snapshot)
	next, _ := json.Marshal(s)
	if s.Revision == r.snapshot.Revision && string(old) == string(next) {
		return nil
	}
	// Upgrade legacy metadata without changing routes, existing generations, or revision.
	legacy := r.snapshot
	if legacy.AuthIDs == nil {
		legacy.AuthIDs = s.AuthIDs
	}
	if legacy.ClaudePrimaryGeneration == 0 {
		legacy.ClaudePrimaryGeneration = s.ClaudePrimaryGeneration
	}
	upgraded, _ := json.Marshal(legacy)
	// A snapshot that differs only by metadata introduced after the helper
	// wrote its file is a safe replay, even when the caller is at a request
	// boundary. There is no route or credential change to admit in this case.
	bootstrap := string(upgraded) == string(next)
	if s.Revision != r.snapshot.Revision+1 && !bootstrap {
		return ErrRevision
	}
	for _, authID := range r.snapshot.AuthIDs {
		if !slices.Contains(s.AuthIDs, authID) && r.activeAuth[authID] > 0 {
			return ErrBusy
		}
	}
	for hash, route := range r.byTicket {
		if index[hash] != route && r.active[route.SessionID] > 0 {
			next := index[hash]
			if requestBoundary && slices.Contains(s.AuthIDs, route.AuthID) && slices.Contains(s.AuthIDs, next.AuthID) && (route.Provider == "codex" || route.Provider == "claude") && next.Provider == route.Provider && next.SessionID == route.SessionID && next.TicketHash == route.TicketHash {
				continue
			}
			return ErrBusy
		}
	}
	if err = writePrivate(r.path, next); err != nil {
		return err
	}
	r.snapshot = s
	r.snapshot.Routes = slices.Clone(s.Routes)
	r.snapshot.AuthIDs = slices.Clone(s.AuthIDs)
	r.snapshot.CodexPrimaryGeneration = s.CodexPrimaryGeneration
	r.snapshot.ClaudePrimaryGeneration = s.ClaudePrimaryGeneration
	r.byTicket = index
	return nil
}
func (r *Routes) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := r.snapshot
	result.Routes = slices.Clone(result.Routes)
	result.AuthIDs = slices.Clone(result.AuthIDs)
	return result
}
func (r *Routes) Acquire(ticket string) (Route, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	route, ok := r.byTicket[TicketHash(ticket)]
	if !ok || ticket == "" {
		return Route{}, nil, errors.New("unknown session ticket")
	}
	if route.AuthID == "" {
		return Route{}, nil, errors.New("login required")
	}
	r.active[route.SessionID]++
	r.activeAuth[route.AuthID]++
	var once sync.Once
	return route, func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.active[route.SessionID]--
			r.activeAuth[route.AuthID]--
			if r.activeAuth[route.AuthID] == 0 {
				delete(r.activeAuth, route.AuthID)
			}
			if r.active[route.SessionID] == 0 {
				delete(r.active, route.SessionID)
			}
		})
	}, nil
}
func writePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".routes-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
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
