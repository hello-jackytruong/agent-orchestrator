package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	proxyapi "github.com/router-for-me/CLIProxyAPI/v8/sdk/api"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

func Build(root string, port int, controlKey, inferenceKey string, routes *Routes) (*cliproxy.Service, error) {
	if !filepath.IsAbs(root) || port < 1 || port > 65535 || len(controlKey) < 32 || len(inferenceKey) < 32 || controlKey == inferenceKey {
		return nil, errors.New("invalid proxy host configuration")
	}
	cfg := &config.Config{Host: "127.0.0.1", Port: port, AuthDir: filepath.Join(root, "auth"), CommercialMode: true, MaxRetryCredentials: 1, WebsocketAuth: true}
	if err := os.MkdirAll(cfg.AuthDir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(cfg.AuthDir, 0700); err != nil {
		return nil, err
	}
	cfg.APIKeys = []string{inferenceKey}
	cfg.Routing.Strategy = "fill-first"
	cfg.RemoteManagement.DisableControlPanel = true
	cfg.RemoteManagement.DisableAutoUpdatePanel = true
	// The SDK checks for a configured secret before accepting its local password.
	secret, err := bcrypt.GenerateFromPassword([]byte(controlKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	cfg.RemoteManagement.SecretKey = string(secret)
	path := filepath.Join(root, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err = writePrivate(path, data); err != nil {
		return nil, err
	}
	boundary := Boundary{Routes: routes, ControlKey: controlKey, InferenceKey: inferenceKey}
	return cliproxy.NewBuilder().WithConfig(cfg).WithConfigPath(path).WithLocalManagementPassword(controlKey).
		WithPostAuthHook(tagLogin).WithServerOptions(proxyapi.WithEngineConfigurator(func(e *gin.Engine) { e.Use(boundary.Middleware) }), proxyapi.WithRouterConfigurator(boundary.Configure)).Build()
}
func Run(ctx context.Context, root string, port int, controlKey, inferenceKey string) error {
	routes, err := OpenRoutes(filepath.Join(root, "run", "routes.json"))
	if err != nil {
		return err
	}
	service, err := Build(root, port, controlKey, inferenceKey, routes)
	if err != nil {
		return err
	}
	return service.Run(ctx)
}

func tagLogin(ctx context.Context, a *coreauth.Auth) error {
	info := coreauth.GetRequestInfo(ctx)
	if info == nil || a == nil {
		return nil
	}
	id := info.Headers.Get("X-AO-Login-ID")
	if len(id) == 0 || len(id) > 128 {
		return nil
	}
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	a.Metadata["ao_login_id"] = id
	return nil
}
