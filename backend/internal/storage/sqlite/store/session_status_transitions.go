package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/gen"
)

// InsertSessionStatusTransition inserts a new status transition record.
func (s *Store) InsertSessionStatusTransition(ctx context.Context, t domain.SessionStatusTransition) (domain.SessionStatusTransition, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var fromStatus sql.NullString
	if t.FromStatus != nil {
		fromStatus = sql.NullString{String: *t.FromStatus, Valid: true}
	}
	var reason sql.NullString
	if t.Reason != nil {
		reason = sql.NullString{String: *t.Reason, Valid: true}
	}
	var endedAt sql.NullTime
	if t.EndedAt != nil {
		endedAt = sql.NullTime{Time: t.EndedAt.UTC(), Valid: true}
	}
	var durationMs sql.NullInt64
	if t.DurationMs != nil {
		durationMs = sql.NullInt64{Int64: *t.DurationMs, Valid: true}
	}
	metadata := t.Metadata
	if len(metadata) == 0 {
		metadata = []byte("{}")
	}

	row, err := s.qw.InsertSessionStatusTransition(ctx, gen.InsertSessionStatusTransitionParams{
		ID:            t.ID,
		SessionID:     string(t.SessionID),
		FromStatus:    fromStatus,
		ToStatus:      t.ToStatus,
		TriggerSource: t.TriggerSource,
		Reason:        reason,
		Metadata:      string(metadata),
		StartedAt:     t.StartedAt.UTC(),
		EndedAt:       endedAt,
		DurationMs:    durationMs,
		CreatedAt:     t.CreatedAt.UTC(),
	})
	if err != nil {
		return domain.SessionStatusTransition{}, fmt.Errorf("insert session transition: %w", err)
	}
	return transitionRowToDomain(row), nil
}

// CloseSessionStatusTransition marks an open transition as ended with its duration in milliseconds.
func (s *Store) CloseSessionStatusTransition(ctx context.Context, id string, endedAt time.Time, durationMs int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	rows, err := s.qw.CloseSessionStatusTransition(ctx, gen.CloseSessionStatusTransitionParams{
		ID:         id,
		EndedAt:    sql.NullTime{Time: endedAt.UTC(), Valid: true},
		DurationMs: sql.NullInt64{Int64: durationMs, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("close session transition %s: %w", id, err)
	}
	if rows != 1 {
		return fmt.Errorf("close session transition %s: expected 1 row, got %d", id, rows)
	}
	return nil
}

// GetLatestSessionStatusTransition returns the most recent status transition for a session.
func (s *Store) GetLatestSessionStatusTransition(ctx context.Context, sessionID domain.SessionID) (domain.SessionStatusTransition, bool, error) {
	row, err := s.qr.GetLatestSessionStatusTransition(ctx, string(sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SessionStatusTransition{}, false, nil
	}
	if err != nil {
		return domain.SessionStatusTransition{}, false, fmt.Errorf("get latest session transition %s: %w", sessionID, err)
	}
	return transitionRowToDomain(row), true, nil
}

// ListSessionStatusTransitions returns a paginated list of transitions ordered newest first.
func (s *Store) ListSessionStatusTransitions(ctx context.Context, sessionID domain.SessionID, limit, offset int64) ([]domain.SessionStatusTransition, int64, error) {
	total, err := s.qr.CountSessionStatusTransitions(ctx, string(sessionID))
	if err != nil {
		return nil, 0, fmt.Errorf("count session transitions %s: %w", sessionID, err)
	}

	rows, err := s.qr.ListSessionStatusTransitions(ctx, gen.ListSessionStatusTransitionsParams{
		SessionID: string(sessionID),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list session transitions %s: %w", sessionID, err)
	}

	items := make([]domain.SessionStatusTransition, len(rows))
	for i, r := range rows {
		items[i] = transitionRowToDomain(r)
	}
	return items, total, nil
}

func transitionRowToDomain(r gen.SessionStatusTransition) domain.SessionStatusTransition {
	var fromStatus, reason *string
	if r.FromStatus.Valid {
		fromStatus = &r.FromStatus.String
	}
	if r.Reason.Valid {
		reason = &r.Reason.String
	}
	var endedAt *time.Time
	if r.EndedAt.Valid {
		endedAt = &r.EndedAt.Time
	}
	var durationMs *int64
	if r.DurationMs.Valid {
		durationMs = &r.DurationMs.Int64
	}

	return domain.SessionStatusTransition{
		ID:            r.ID,
		SessionID:     domain.SessionID(r.SessionID),
		FromStatus:    fromStatus,
		ToStatus:      r.ToStatus,
		TriggerSource: r.TriggerSource,
		Reason:        reason,
		Metadata:      json.RawMessage(r.Metadata),
		StartedAt:     r.StartedAt,
		EndedAt:       endedAt,
		DurationMs:    durationMs,
		CreatedAt:     r.CreatedAt,
	}
}
