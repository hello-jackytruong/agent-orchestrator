package domain

import (
	"encoding/json"
	"time"
)

// SessionStatusTransition records one granular status change in a session's lifecycle.
type SessionStatusTransition struct {
	ID            string          `json:"id"`
	SessionID     SessionID       `json:"sessionId"`
	FromStatus    *string         `json:"fromStatus,omitempty"`
	ToStatus      string          `json:"toStatus"`
	TriggerSource string          `json:"triggerSource"` // 'agent', 'user', 'scm_ci', 'scm_review', 'system'
	Reason        *string         `json:"reason,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	StartedAt     time.Time       `json:"startedAt"`
	EndedAt       *time.Time      `json:"endedAt,omitempty"`
	DurationMs    *int64          `json:"durationMs,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}
