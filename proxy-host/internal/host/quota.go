package host

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// quotaResultPolicy turns only the provider's explicit Codex usage-limit
// signal into a durable event. Rate limits, outages, auth failures, and other
// errors remain ordinary request failures and never move AO's primary.
type quotaResultPolicy struct{ routes *Routes }

func (p quotaResultPolicy) ApplyResultPolicy(_ context.Context, result coreauth.Result) coreauth.Result {
	if p.routes == nil || result.Provider != "codex" || result.Success || result.Error == nil {
		return result
	}
	if !isCodexUsageLimitResult(result.Error) {
		return result
	}
	resetAt := time.Time{}
	if result.RetryAfter != nil && *result.RetryAfter > 0 {
		resetAt = time.Now().Add(*result.RetryAfter)
	}
	p.routes.RecordQuota(result.AuthID, resetAt)
	return result
}

func isCodexUsageLimitResult(err *coreauth.Error) bool {
	if err == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(err.Code), "usage_limit_reached") {
		return true
	}
	var value any
	if json.Unmarshal([]byte(err.Message), &value) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(node any) bool {
		switch typed := node.(type) {
		case map[string]any:
			for key, child := range typed {
				if (strings.EqualFold(key, "type") || strings.EqualFold(key, "code")) && strings.EqualFold(strings.TrimSpace(anyString(child)), "usage_limit_reached") {
					return true
				}
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func anyString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
