package controllers_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/httpd/controllers"
)

func TestGetSessionTimeline_Success(t *testing.T) {
	svc := newFakeSessionService()
	sessID := domain.SessionID("ao-1")
	now := time.Now().UTC().Truncate(time.Second)
	dur := int64(15000)
	reason := "Agent completed step"

	svc.transitions[sessID] = []domain.SessionStatusTransition{
		{
			ID:            "trans-1",
			SessionID:     sessID,
			FromStatus:    nil,
			ToStatus:      "idle",
			TriggerSource: "system",
			StartedAt:     now.Add(-20 * time.Second),
			EndedAt:       &now,
			DurationMs:    &dur,
			CreatedAt:     now.Add(-20 * time.Second),
		},
		{
			ID:            "trans-2",
			SessionID:     sessID,
			FromStatus:    ptr("idle"),
			ToStatus:      "active",
			TriggerSource: "agent",
			Reason:        &reason,
			StartedAt:     now,
			CreatedAt:     now,
		},
	}

	srv := newSessionTestServer(t, svc)

	body, status, _ := doRequest(t, srv, "GET", "/api/v1/sessions/ao-1/timeline", "")
	if status != http.StatusOK {
		t.Fatalf("GET timeline status = %d, want 200; body=%s", status, body)
	}

	var resp controllers.SessionTimelineResponse
	mustJSON(t, body, &resp)

	if resp.Total != 2 {
		t.Errorf("resp.Total = %d, want 2", resp.Total)
	}
	if resp.Limit != 100 {
		t.Errorf("resp.Limit = %d, want 100", resp.Limit)
	}
	if resp.Offset != 0 {
		t.Errorf("resp.Offset = %d, want 0", resp.Offset)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("len(resp.Items) = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].ID != "trans-1" || resp.Items[0].ToStatus != "idle" || resp.Items[0].TriggerSource != "system" {
		t.Errorf("unexpected item 0: %+v", resp.Items[0])
	}
	if resp.Items[0].DurationMs == nil || *resp.Items[0].DurationMs != 15000 {
		t.Errorf("unexpected duration item 0: %+v", resp.Items[0].DurationMs)
	}
	if resp.Items[1].ID != "trans-2" || resp.Items[1].ToStatus != "active" || resp.Items[1].TriggerSource != "agent" {
		t.Errorf("unexpected item 1: %+v", resp.Items[1])
	}
	if resp.Items[1].FromStatus == nil || *resp.Items[1].FromStatus != "idle" {
		t.Errorf("unexpected fromStatus item 1: %+v", resp.Items[1].FromStatus)
	}
}

func TestGetSessionTimeline_Pagination(t *testing.T) {
	svc := newFakeSessionService()
	sessID := domain.SessionID("ao-1")
	now := time.Now().UTC().Truncate(time.Second)

	for i := 1; i <= 5; i++ {
		svc.transitions[sessID] = append(svc.transitions[sessID], domain.SessionStatusTransition{
			ID:            fmt.Sprintf("trans-%d", i),
			SessionID:     sessID,
			ToStatus:      "active",
			TriggerSource: "agent",
			StartedAt:     now.Add(time.Duration(i) * time.Second),
			CreatedAt:     now.Add(time.Duration(i) * time.Second),
		})
	}

	srv := newSessionTestServer(t, svc)

	body, status, _ := doRequest(t, srv, "GET", "/api/v1/sessions/ao-1/timeline?limit=2&offset=1", "")
	if status != http.StatusOK {
		t.Fatalf("GET timeline status = %d, want 200; body=%s", status, body)
	}

	var resp controllers.SessionTimelineResponse
	mustJSON(t, body, &resp)

	if resp.Total != 5 {
		t.Errorf("resp.Total = %d, want 5", resp.Total)
	}
	if resp.Limit != 2 {
		t.Errorf("resp.Limit = %d, want 2", resp.Limit)
	}
	if resp.Offset != 1 {
		t.Errorf("resp.Offset = %d, want 1", resp.Offset)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("len(resp.Items) = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].ID != "trans-2" || resp.Items[1].ID != "trans-3" {
		t.Errorf("unexpected paginated items: 0=%s, 1=%s", resp.Items[0].ID, resp.Items[1].ID)
	}
}

func TestGetSessionTimeline_ClampsLimit(t *testing.T) {
	svc := newFakeSessionService()
	srv := newSessionTestServer(t, svc)

	body, status, _ := doRequest(t, srv, "GET", "/api/v1/sessions/ao-1/timeline?limit=999", "")
	if status != http.StatusOK {
		t.Fatalf("GET timeline status = %d, want 200; body=%s", status, body)
	}

	var resp controllers.SessionTimelineResponse
	mustJSON(t, body, &resp)
	if resp.Limit != 500 || svc.lastTransitionLimit != 500 {
		t.Fatalf("limit = response %d service %d, want 500", resp.Limit, svc.lastTransitionLimit)
	}
}

func TestGetSessionTimeline_InvalidPagination(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		code string
	}{
		{name: "non-numeric limit", url: "/api/v1/sessions/ao-1/timeline?limit=nope", code: "INVALID_LIMIT"},
		{name: "zero limit", url: "/api/v1/sessions/ao-1/timeline?limit=0", code: "INVALID_LIMIT"},
		{name: "negative offset", url: "/api/v1/sessions/ao-1/timeline?offset=-1", code: "INVALID_OFFSET"},
		{name: "non-numeric offset", url: "/api/v1/sessions/ao-1/timeline?offset=nope", code: "INVALID_OFFSET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFakeSessionService()
			srv := newSessionTestServer(t, svc)

			body, status, _ := doRequest(t, srv, "GET", tc.url, "")
			if status != http.StatusBadRequest {
				t.Fatalf("GET timeline status = %d, want 400; body=%s", status, body)
			}

			var errResp map[string]any
			mustJSON(t, body, &errResp)
			if errResp["code"] != tc.code {
				t.Fatalf("error code = %v, want %s; body=%s", errResp["code"], tc.code, body)
			}
			if svc.lastTransitionLimit != 0 || svc.lastTransitionOffset != 0 {
				t.Fatalf("service should not be called, got limit=%d offset=%d", svc.lastTransitionLimit, svc.lastTransitionOffset)
			}
		})
	}
}

func TestGetSessionTimeline_NotFound(t *testing.T) {
	svc := newFakeSessionService()
	srv := newSessionTestServer(t, svc)

	body, status, _ := doRequest(t, srv, "GET", "/api/v1/sessions/nonexistent/timeline", "")
	if status != http.StatusNotFound {
		t.Fatalf("GET timeline status = %d, want 404; body=%s", status, body)
	}

	var errResp map[string]any
	mustJSON(t, body, &errResp)
	if errResp["code"] != "SESSION_NOT_FOUND" {
		t.Errorf("expected SESSION_NOT_FOUND error code, got %v", errResp["code"])
	}
}

func ptr(s string) *string {
	return &s
}
