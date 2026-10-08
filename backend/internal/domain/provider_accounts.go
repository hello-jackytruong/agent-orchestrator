package domain

import (
	"hash/fnv"
	"strings"
	"time"
)

// ProviderAccount is AO's safe identity plus a private upstream credential
// reference. Empty CredentialRef is the durable signed-out fact.
type ProviderAccount struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email"`
	Kind        string `json:"kind,omitempty"`
	// Global marks the account currently discovered from the provider's native
	// login. It is informational; routing still follows the saved primary.
	Global        bool   `json:"global,omitempty"`
	CredentialRef string `json:"credential_ref"`
	AuthID        string `json:"auth_id"`
}

// GeneratedProviderAccountName gives older accounts a stable friendly label
// without depending on provider profile APIs.
func GeneratedProviderAccountName(provider, id string) string {
	words := []string{"Cedar", "Maple", "Willow", "River", "Summit", "Harbor", "Meadow", "Pine", "Juniper", "Clover", "Ember", "Atlas"}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(provider + ":" + id)))
	word := words[int(h.Sum32())%len(words)]
	name := "Account"
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		name = "Codex"
	case "claude":
		name = "Claude"
	}
	return word + " " + name
}

// ProviderAccountUsage is a safe, short-lived view of upstream quota data.
// It deliberately contains no credential or provider-private response data.
type ProviderAccountUsage struct {
	Status    string                       `json:"status"`
	Plan      string                       `json:"plan,omitempty"`
	Windows   []ProviderAccountUsageWindow `json:"windows,omitempty"`
	CheckedAt time.Time                    `json:"checked_at,omitempty"`
	Message   string                       `json:"message,omitempty"`
}

// ProviderAccountUsageWindow is one normalized quota window returned by the
// proxy's management API.
type ProviderAccountUsageWindow struct {
	Name              string  `json:"name,omitempty"`
	RemainingFraction float64 `json:"remaining_fraction"`
	ResetTime         string  `json:"reset_time,omitempty"`
}

// ProviderPrimary records a provider default, including an empty default after its last logout.
type ProviderPrimary struct {
	Provider  string `json:"provider"`
	PrimaryID string `json:"primary_id"`
}

// ProviderSessionRoute binds one session ticket to a provider and account.
type ProviderSessionRoute struct {
	SessionID  SessionID `json:"session_id"`
	Provider   string    `json:"provider"`
	AccountID  string    `json:"account_id"`
	TicketHash string    `json:"ticket_hash"`
}

// ProviderAccountState contains only durable routing facts. A primary entry,
// even when empty, records deliberate adoption of managed routing.
// NativeProviderImport remembers an observed source even after local sign-out
// or removal, so refresh cannot undo an explicit account-manager action.
type NativeProviderImport struct {
	Fingerprint string `json:"fingerprint"`
	AccountID   string `json:"account_id"`
}

type ProviderAccountState struct {
	NativeImports           map[string]NativeProviderImport `json:"native_imports,omitempty"`
	Revision                int64                           `json:"revision"`
	Accounts                []ProviderAccount               `json:"accounts"`
	Primaries               []ProviderPrimary               `json:"primaries"`
	Routes                  []ProviderSessionRoute          `json:"routes"`
	CodexQuotaAutoSwitch    bool                            `json:"codex_quota_auto_switch,omitempty"`
	CodexPrimaryGeneration  int64                           `json:"codex_primary_generation,omitempty"`
	ClaudeQuotaAutoSwitch   bool                            `json:"claude_quota_auto_switch,omitempty"`
	ClaudePrimaryGeneration int64                           `json:"claude_primary_generation,omitempty"`
}

// ProviderAccountIntent survives a lost helper acknowledgement or daemon exit.
// Effective facts commit after acknowledgement; credential cleanup completes
// before the intent is cleared and the operation reports success.
type ProviderAccountIntent struct {
	Next             ProviderAccountState `json:"next"`
	DeleteCredential string               `json:"delete_credential"`
	RequestBoundary  bool                 `json:"request_boundary,omitempty"`
}
