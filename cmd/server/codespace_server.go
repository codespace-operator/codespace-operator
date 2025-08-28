//go:generate swag init -g codespace_server.go -o ../../docs --parseDependency --parseInternal

// @title Codespace Operator API
// @version 1.0.0
// @description REST API for managing Codespace sessions in Kubernetes
// @termsOfService https://github.com/codespace-operator/codespace-operator

// @contact.name Codespace Operator Support
// @contact.url https://github.com/codespace-operator/codespace-operator
// @contact.email support@codespace.dev

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name codespace_session

package main

import (
	"context"
	"embed"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

//go:embed all:static
var staticFS embed.FS

var (
	gvr = schema.GroupVersionResource{
		Group:    codespacev1.GroupVersion.Group,
		Version:  codespacev1.GroupVersion.Version,
		Resource: "sessions",
	}
)

type serverDeps struct {
	typed      client.Client
	dyn        dynamic.Interface
	scheme     *runtime.Scheme
	config     *config.ServerConfig
	rbac       *RBAC
	localUsers *localUsers
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "codespace-server",
		Short: "Codespace Operator Web Server",
		Long:  `A web server that provides a REST API and UI for managing Codespace sessions.`,
		Run:   runServer,
	}

	rootCmd.Flags().String("config", "", "Path to config file or directory (highest precedence)")
	rootCmd.Flags().IntP("port", "p", 8080, "Server port")
	rootCmd.Flags().String("host", "", "Server host (empty for all interfaces)")
	rootCmd.Flags().String("allow-origin", "", "CORS allow origin")
	rootCmd.Flags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().Float32("kube-qps", 50.0, "Kubernetes client QPS limit")
	rootCmd.Flags().Int("kube-burst", 100, "Kubernetes client burst limit")

	// OIDC + feature flags
	rootCmd.Flags().Bool("oidc-insecure-skip-verify", false, "Skip OIDC server certificate verification")
	rootCmd.Flags().String("oidc-issuer", "", "OIDC issuer URL (e.g., https://dev-xxxx.okta.com)")
	rootCmd.Flags().String("oidc-client-id", "", "OIDC client ID")
	rootCmd.Flags().String("oidc-client-secret", "", "OIDC client secret")
	rootCmd.Flags().String("oidc-redirect-url", "", "OIDC redirect URL (https://host/auth/callback)")
	rootCmd.Flags().StringSlice("oidc-scopes", []string{}, "OIDC scopes (default: openid profile email)")
	rootCmd.Flags().Bool("enable-bootstrap-login", false, "Enable legacy bootstrap login (dev only)")
	rootCmd.Flags().Bool("allow-token-param", false, "Allow ?access_token=... on URLs (NOT recommended)")
	rootCmd.Flags().Int("session-ttl-minutes", 60, "Session cookie TTL in minutes")
	rootCmd.Flags().String("session-cookie-name", "", "Override session cookie name")
	rootCmd.Flags().String("rbac-model-path", "", "Path to Casbin model.conf (overrides default/env)")
	rootCmd.Flags().String("rbac-policy-path", "", "Path to Casbin policy.csv (overrides default/env)")

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Command execution failed", "err", err)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	// Highest precedence: --config (file or dir)
	if cmd.Flags().Changed("config") {
		p, _ := cmd.Flags().GetString("config")
		if strings.TrimSpace(p) != "" {
			// make it visible to config.LoadServerConfig/setupViper
			_ = os.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", p)
		}
	}

	cfg, err := config.LoadServerConfig()
	configureLogger(cfg.LogLevel)
	if err != nil {
		logger.Fatal("Failed to load configuration", "err", err)
	}
	// CLI overrides
	setString := func(name *string, flag string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetString(flag)
			if v != "" {
				*name = v
			}
		}
	}
	setBool := func(name *bool, flag string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetBool(flag)
			*name = v
		}
	}
	setInt := func(name *int, flag string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetInt(flag)
			*name = v
		}
	}

	setString(&cfg.AllowOrigin, "allow-origin")
	if cmd.Flags().Changed("kube-qps") {
		cfg.KubeQPS, _ = cmd.Flags().GetFloat32("kube-qps")
	}
	if cmd.Flags().Changed("kube-burst") {
		cfg.KubeBurst, _ = cmd.Flags().GetInt("kube-burst")
	}
	setString(&cfg.OIDCIssuerURL, "oidc-issuer")
	setString(&cfg.OIDCClientID, "oidc-client-id")
	setString(&cfg.OIDCClientSecret, "oidc-client-secret")
	setString(&cfg.OIDCRedirectURL, "oidc-redirect-url")
	if cmd.Flags().Changed("oidc-scopes") {
		cfg.OIDCScopes, _ = cmd.Flags().GetStringSlice("oidc-scopes")
	}
	setBool(&cfg.OIDCInsecureSkipVerify, "oidc-insecure-skip-verify")
	setBool(&cfg.EnableLocalLogin, "enable-local-login")
	setBool(&cfg.AllowTokenParam, "allow-token-param")
	setInt(&cfg.SessionTTLMinutes, "session-ttl-minutes")
	setString(&cfg.SessionCookieName, "session-cookie-name")
	setString(&cfg.RBACModelPath, "rbac-model-path")
	setString(&cfg.RBACPolicyPath, "rbac-policy-path")
	if cfg.LogLevel == "debug" {
		logger.Printf("Configuration: %+v", cfg)
	}

	k8sCfg, err := helpers.BuildKubeConfig()
	if err != nil {
		logger.Fatal("Kubernetes config", "err", err)
	}
	k8sCfg.Timeout = 30 * time.Second
	k8sCfg.QPS = cfg.KubeQPS
	k8sCfg.Burst = cfg.KubeBurst

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Fatal("Add corev1 scheme", "err", err)
	}
	if err := codespacev1.AddToScheme(scheme); err != nil {
		logger.Fatal("Add scheme", "err", err)
	}

	typed, err := client.New(k8sCfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Fatal("Typed client", "err", err)
	}
	dyn, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		logger.Fatal("Dynamic client", "err", err)
	}

	// If explicit RBAC paths are provided, push them into env so NewRBACFromEnv picks them up.
	if cfg.RBACModelPath != "" {
		_ = os.Setenv(envModelPath, cfg.RBACModelPath)
	}
	if cfg.RBACPolicyPath != "" {
		_ = os.Setenv(envPolicyPath, cfg.RBACPolicyPath)
	}
	rbac, err := NewRBACFromEnv(context.Background())
	if err != nil {
		logger.Fatal("RBAC init failed", "err", err)
	}

	users, err := loadLocalUsers(cfg.LocalUsersPath)
	if err != nil {
		logger.Fatal("local users load failed", "err", err)
	}

	deps := &serverDeps{
		typed:      typed,
		dyn:        dyn,
		scheme:     scheme,
		config:     cfg,
		rbac:       rbac,
		localUsers: users,
	}

	mux := setupHandlersWithDocs(deps)
	registerAuthHandlers(mux, deps)

	// build middleware chain:
	//   authGate( logRequests( withCORS( mux )))
	var handler http.Handler = mux
	handler = withCORS(cfg.AllowOrigin, handler) // CORS first so preflights are handled
	handler = logRequests(handler)               // wrap CORS so OPTIONS are logged too
	handler = authGate(&configLike{
		JWTSecret:         cfg.JWTSecret,
		SessionCookieName: cfg.SessionCookieName,
		AllowTokenParam:   cfg.AllowTokenParam,
	}, handler)

	logger.Printf("Listening on %s", cfg.GetAddr())
	if err := http.ListenAndServe(cfg.GetAddr(), handler); err != nil {
		logger.Fatal("ListenAndServe", "err", err)
	}

}
func authGate(cfg *configLike, next http.Handler) http.Handler {
	// only guard /api/*; allow health, static, and /auth/*
	authed := requireAPIToken(cfg, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/healthz" || p == "/readyz" ||
			strings.HasPrefix(p, "/auth/") ||
			p == "/" || strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(p, "/api/") {
			authed.ServeHTTP(w, r)
			return
		}
		// default public
		next.ServeHTTP(w, r)
	})
}
