package host

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

// Boundary is installed before SDK routes, auth and logging. Only the daemon can
// access the management allowlist; agents can access inference with a ticket.
type Boundary struct {
	Routes                   *Routes
	ControlKey, InferenceKey string
}

func (b Boundary) Middleware(c *gin.Context) {
	c.Request.Header.Del(accountHeader)
	c.Request.Header.Del(providerHeader)
	if strings.HasPrefix(c.Request.URL.Path, "/ao/") || strings.HasPrefix(c.Request.URL.Path, "/v8/management/") {
		if !equalKey(c.GetHeader("Authorization"), "Bearer "+b.ControlKey) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/v8/management/") && !allowedManagement(c.Request) {
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
	key := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/v8/management/")
	switch key {
	case "GET credentials", "DELETE credentials", "GET oauth/auth-url", "GET oauth/status", "POST oauth/callback", "DELETE oauth/session":
		return true
	}
	return false
}
func (b Boundary) Configure(engine *gin.Engine, h *handlers.BaseAPIHandler, _ *config.Config) {
	h.AuthManager.SetSelector(exactSelector{})
	engine.GET("/ao/status", func(c *gin.Context) {
		snapshot := b.Routes.Snapshot()
		c.JSON(http.StatusOK, gin.H{"protocol_version": 1, "revision": snapshot.Revision, "routes": snapshot.Routes})
	})
	engine.GET("/ao/login-result/:id", func(c *gin.Context) {
		for _, a := range h.AuthManager.List() {
			if a.Metadata["ao_login_id"] == c.Param("id") {
				email, _ := a.Metadata["email"].(string)
				c.JSON(http.StatusOK, gin.H{"provider": a.Provider, "email": email, "credential_ref": a.FileName, "auth_id": a.ID})
				return
			}
		}
		c.AbortWithStatus(http.StatusNotFound)
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
