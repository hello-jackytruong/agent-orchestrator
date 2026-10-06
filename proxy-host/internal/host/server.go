package host

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

// Boundary is installed before SDK routes, auth and logging. Only the daemon can
// access the management allowlist; agents can access inference with a ticket.
type Boundary struct {
	Routes                   *Routes
	ControlKey, InferenceKey string
	LoginInputs              *LoginInputs
}

func (b Boundary) Middleware(c *gin.Context) {
	c.Request.Header.Del(accountHeader)
	c.Request.Header.Del(providerHeader)
	if strings.HasPrefix(c.Request.URL.Path, "/ao/") || strings.HasPrefix(c.Request.URL.Path, "/v8/management/") || strings.HasPrefix(c.Request.URL.Path, "/v0/management/") {
		if !equalKey(c.GetHeader("Authorization"), "Bearer "+b.ControlKey) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if strings.Contains(c.Request.URL.Path, "/management/") && !allowedManagement(c.Request) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		// The upstream OAuth forwarder binds all interfaces; AO owns loopback relays.
		query := c.Request.URL.Query()
		query.Del("is_webui")
		c.Request.URL.RawQuery = query.Encode()
		c.Next()
		return
	}
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		c.AbortWithStatus(http.StatusNotImplemented)
		return
	}
	switch c.Request.URL.Path {
	case "/v1/responses", "/v1/responses/compact", "/v1/messages", "/v1/messages/count_tokens", "/v1/models":
	default:
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	ticket := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if ticket == "" {
		ticket = c.GetHeader("X-Api-Key")
	}
	route, release, err := b.Routes.Acquire(ticket)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "Session account unavailable. Please sign in.", "type": "authentication_error"}})
		return
	}
	defer release()
	if (strings.Contains(c.Request.URL.Path, "messages") && route.Provider != "claude") || (strings.Contains(c.Request.URL.Path, "responses") && route.Provider != "codex") {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.Request.Header.Set(accountHeader, route.AuthID)
	c.Request.Header.Set(providerHeader, route.Provider)
	c.Request.Header.Set("Authorization", "Bearer "+b.InferenceKey)
	c.Request.Header.Del("X-Api-Key")
	c.Next()
}
func equalKey(actual, expected string) bool {
	return len(expected) > 7 && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
func allowedManagement(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/v0/management/") {
		if r.Method == http.MethodPost && r.URL.Path == "/v0/management/quota/fetch" {
			return true
		}
		return (r.Method == http.MethodGet || r.Method == http.MethodPut || r.Method == http.MethodDelete) &&
			(r.URL.Path == "/v0/management/codex-api-key" || r.URL.Path == "/v0/management/claude-api-key")
	}
	key := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/v8/management/")
	switch key {
	case "GET credentials", "POST credentials", "DELETE credentials", "GET oauth/auth-url", "GET oauth/status", "POST oauth/callback", "DELETE oauth/session":
		return true
	}
	return false
}
func (b Boundary) Configure(engine *gin.Engine, h *handlers.BaseAPIHandler, _ *config.Config) {
	h.AuthManager.SetSelector(exactSelector{})
	if b.LoginInputs != nil {
		b.LoginInputs.auth = h.AuthManager
	}
	engine.GET("/ao/status", func(c *gin.Context) {
		snapshot := b.Routes.Snapshot()
		c.JSON(http.StatusOK, gin.H{"protocol_version": 2, "revision": snapshot.Revision, "routes": snapshot.Routes, "auth_ids": snapshot.AuthIDs, "quota_events": b.Routes.QuotaEvents()})
	})
	engine.GET("/ao/quota-events", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"events": b.Routes.QuotaEvents()})
	})
	engine.POST("/ao/account-usage", func(c *gin.Context) {
		var body struct {
			AuthID   string `json:"auth_id"`
			Provider string `json:"provider"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.AuthID) == "" || (body.Provider != "codex" && body.Provider != "claude") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account usage request"})
			return
		}
		auth, ok := h.AuthManager.GetByID(strings.TrimSpace(body.AuthID))
		if !ok || auth.Provider != body.Provider {
			c.JSON(http.StatusNotFound, gin.H{"error": "provider account not found"})
			return
		}
		usageURL := "https://chatgpt.com/backend-api/wham/usage"
		if body.Provider == "claude" {
			usageURL = "https://api.anthropic.com/api/oauth/usage"
		}
		req, err := h.AuthManager.NewHttpRequest(c.Request.Context(), auth, http.MethodGet, usageURL, nil, http.Header{"Accept": []string{"application/json"}})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "account usage request could not be prepared"})
			return
		}
		resp, err := h.AuthManager.HttpRequest(c.Request.Context(), auth, req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "account usage request failed"})
			return
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "account usage response could not be read"})
			return
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			c.JSON(http.StatusBadGateway, gin.H{"error": "account usage provider returned an error"})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})
	engine.POST("/ao/quota-events/ack", func(c *gin.Context) {
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) > 256 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quota event acknowledgement"})
			return
		}
		if err := b.Routes.AckQuotaEvents(body.IDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"events": b.Routes.QuotaEvents()})
	})
	engine.GET("/ao/login-result/:id", func(c *gin.Context) {
		if b.LoginInputs != nil {
			if op, ok := b.LoginInputs.operation(c.Param("id")); ok && op.Status == "complete" && op.Result != nil {
				c.JSON(http.StatusOK, op.Result)
				return
			}
		}
		for _, a := range h.AuthManager.List() {
			loginID := c.Param("id")
			if a.Metadata["ao_login_id"] == loginID || a.FileName == "ao-"+loginID+".json" {
				email, _ := a.Metadata["email"].(string)
				ref := a.FileName
				if a.Attributes["api_key"] != "" {
					ref = "config-index:" + a.Provider + ":" + a.EnsureIndex()
				}
				c.JSON(http.StatusOK, gin.H{"provider": a.Provider, "email": email, "credential_ref": ref, "auth_id": a.ID})
				return
			}
		}
		c.AbortWithStatus(http.StatusNotFound)
	})
	engine.POST("/ao/login/device/start", func(c *gin.Context) {
		var body struct {
			ID string `json:"id"`
		}
		if b.LoginInputs == nil || c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.ID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid device login"})
			return
		}
		op, err := b.LoginInputs.startDevice(c.Request.Context(), body.ID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, op)
	})
	engine.GET("/ao/login/status", func(c *gin.Context) {
		if b.LoginInputs == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "unknown login"})
			return
		}
		op, ok := b.LoginInputs.operation(c.Query("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "unknown login"})
			return
		}
		c.JSON(http.StatusOK, op)
	})
	engine.DELETE("/ao/login/status", func(c *gin.Context) {
		if b.LoginInputs != nil {
			b.LoginInputs.cancel(c.Query("id"))
		}
		c.Status(http.StatusNoContent)
	})
	engine.POST("/ao/tag-api-key", func(c *gin.Context) {
		var body struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
			APIKey   string `json:"api_key"`
			BaseURL  string `json:"base_url"`
			Label    string `json:"label"`
		}
		if c.ShouldBindJSON(&body) != nil || !validLoginID(body.ID) || (body.Provider != "codex" && body.Provider != "claude") || body.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid API-key tag"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		a, err := waitAuth(ctx, h.AuthManager, func(a *coreauth.Auth) bool {
			return a.Provider == body.Provider && a.Attributes["api_key"] == body.APIKey && strings.TrimRight(a.Attributes["base_url"], "/") == strings.TrimRight(body.BaseURL, "/")
		})
		if err != nil {
			if c.Request.Context().Err() != nil {
				c.AbortWithStatus(http.StatusRequestTimeout)
			} else {
				c.AbortWithStatus(http.StatusNotFound)
			}
			return
		}
		if a.Metadata == nil {
			a.Metadata = map[string]any{}
		}
		a.Metadata["ao_login_id"] = body.ID
		label := strings.TrimSpace(body.Label)
		if label == "" {
			label = body.Provider + " API key"
		}
		a.Metadata["email"] = label
		updated, err := h.AuthManager.Update(c.Request.Context(), a)
		if err != nil || updated == nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"auth_id": updated.ID})
	})
	engine.DELETE("/ao/api-key", func(c *gin.Context) {
		if b.LoginInputs == nil {
			c.Status(http.StatusNotFound)
			return
		}
		if err := b.LoginInputs.deleteAPIKey(c.Request.Context(), c.Query("ref")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "API key removal failed"})
			return
		}
		c.Status(http.StatusNoContent)
	})
	engine.PUT("/ao/routes", func(c *gin.Context) {
		var s Snapshot
		if err := c.ShouldBindJSON(&s); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid routing snapshot"})
			return
		}
		if err := b.Routes.Apply(s); err != nil {
			code := http.StatusInternalServerError
			if errors.Is(err, ErrBusy) || errors.Is(err, ErrRevision) {
				code = http.StatusConflict
			}
			reason := "ROUTING_FAILED"
			if errors.Is(err, ErrBusy) {
				reason = "SESSION_BUSY"
			} else if errors.Is(err, ErrRevision) {
				reason = "ROUTE_REVISION_CONFLICT"
			}
			c.JSON(code, gin.H{"error": err.Error(), "code": reason})
			return
		}
		c.JSON(http.StatusOK, b.Routes.Snapshot())
	})
}
