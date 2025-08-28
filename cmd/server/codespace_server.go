//go:generate swag init -g codespace_server.go -o ../../gen/api --parseDependency --parseInternal -ot json,yaml

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
	"fmt"
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

type ErrorResponse struct {
	Error string `json:"error" example:"Invalid request"`
}

const InstanceIDLabel = "codespace.dev/instance-id"
const cmPrefixName = "codespace-server-instance"

var (
	gvr = schema.GroupVersionResource{
		Group:    codespacev1.GroupVersion.Group,
		Version:  codespacev1.GroupVersion.Version,
		Resource: "sessions",
	}
)

type serverDeps struct {
	client     client.Client
	dyn        dynamic.Interface
	scheme     *runtime.Scheme
	config     *config.ServerConfig
	rbac       *RBAC
	localUsers *localUsers
	instanceID string
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "codespace-server",
		Short: "Codespace Operator Web Server",
		Long:  `A web server that provides a REST API and UI for managing Codespace sessions.`,
		Run:   runServer,
	}

	// Basic server flags
	rootCmd.Flags().String("config", "", "Path to config file or directory (highest precedence)")
	rootCmd.Flags().IntP("port", "p", 8080, "Server port")
	rootCmd.Flags().String("host", "", "Server host (empty for all interfaces)")
	rootCmd.Flags().String("allow-origin", "", "CORS allow origin")
	rootCmd.Flags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().Float32("kube-qps", 50.0, "Kubernetes client QPS limit")
	rootCmd.Flags().Int("kube-burst", 100, "Kubernetes client burst limit")
	rootCmd.Flags().Bool("cluster-scope", false, "Enable cluster-scoped mode")

	// Authentication flags
	rootCmd.Flags().Bool("oidc-insecure-skip-verify", false, "Skip OIDC server certificate verification")
	rootCmd.Flags().String("oidc-issuer", "", "OIDC issuer URL (e.g., https://dev-xxxx.okta.com)")
	rootCmd.Flags().String("oidc-client-id", "", "OIDC client ID")
	rootCmd.Flags().String("oidc-client-secret", "", "OIDC client secret")
	rootCmd.Flags().String("oidc-redirect-url", "", "OIDC redirect URL (https://host/auth/callback)")
	rootCmd.Flags().StringSlice("oidc-scopes", []string{}, "OIDC scopes (default: openid profile email)")
	rootCmd.Flags().Bool("enable-local-login", false, "Enable legacy bootstrap login (dev only)")
	rootCmd.Flags().Bool("allow-token-param", false, "Allow ?access_token=... on URLs (NOT recommended)")
	rootCmd.Flags().Int("session-ttl-minutes", 60, "Session cookie TTL in minutes")
	rootCmd.Flags().String("session-cookie-name", "", "Override session cookie name")

	// RBAC flags
	rootCmd.Flags().String("rbac-model-path", "", "Path to Casbin model.conf (overrides default/env)")
	rootCmd.Flags().String("rbac-policy-path", "", "Path to Casbin policy.csv (overrides default/env)")

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Command execution failed", "err", err)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	// Load configuration with CLI overrides
	cfg := loadConfigWithOverrides(cmd)
	configureLogger(cfg.LogLevel)

	if cfg.LogLevel == "debug" {
		logger.Info("Configuration loaded", "config", cfg)
	}

	// Setup Kubernetes clients
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
		logger.Fatal("Add codespace scheme", "err", err)
	}

	client, err := client.New(k8sCfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Fatal("client", "err", err)
	}
	dyn, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		logger.Fatal("Dynamic client", "err", err)
	}

	// Test Kubernetes connectivity
	if err := helpers.TestKubernetesConnection(client); err != nil {
		logger.Fatal("Kubernetes connection test failed", "err", err)
	}

	// Setup RBAC
	rbac, err := setupRBAC(cfg)
	if err != nil {
		logger.Fatal("RBAC init failed", "err", err)
	}

	// Setup local users
	users, err := loadLocalUsers(cfg.LocalUsersPath)
	if err != nil {
		logger.Fatal("Local users load failed", "err", err)
	}
	instanceID, err := ensureInstallationID(context.Background(), client)
	logger.Info(fmt.Sprintf("Ensured server installation ID: %s", instanceID), "instanceID", instanceID)
	if err != nil {
		logger.Error("failed to ensure server id", "err", err)
	}

	// Create server dependencies
	deps := &serverDeps{
		client:     client,
		dyn:        dyn,
		scheme:     scheme,
		config:     cfg,
		rbac:       rbac,
		localUsers: users,
		instanceID: instanceID,
	}

	// Setup HTTP handlers
	mux := setupHandlers(deps)

	// Build middleware chain: security → CORS → logging → auth → handlers
	var handler http.Handler = mux
	handler = corsMiddleware(cfg.AllowOrigin)(handler)
	handler = logRequests(handler)
	handler = authGate(&authConfigLike{
		JWTSecret:         cfg.JWTSecret,
		SessionCookieName: cfg.SessionCookieName,
		AllowTokenParam:   cfg.AllowTokenParam,
	}, handler)

	logger.Printf("🚀 Codespace Server starting on %s", cfg.GetAddr())
	if swagDocAvailable() {
		logger.Printf("📚 API Documentation available at http://%s/api/docs/", cfg.GetAddr())
	}

	// Report if running cluster-scoped
	if cfg.ClusterScope {
		logger.Info(" ----------------- Running in cluster-scoped mode ----------------- ")
	} else {
		logger.Info(" --------------- Running in instance-id scoped mode --------------- ")
	}

	if err := http.ListenAndServe(cfg.GetAddr(), handler); err != nil {
		logger.Fatal("ListenAndServe", "err", err)
	}
}

// loadConfigWithOverrides loads configuration with CLI flag overrides
func loadConfigWithOverrides(cmd *cobra.Command) *config.ServerConfig {
	// Set config path if provided
	if cmd.Flags().Changed("config") {
		if p, _ := cmd.Flags().GetString("config"); strings.TrimSpace(p) != "" {
			os.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", p)
		}
	}

	// Load base configuration
	cfg, err := config.LoadServerConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", "err", err)
	}

	// Apply CLI overrides
	overrideString := func(target *string, flag string) {
		if cmd.Flags().Changed(flag) {
			if v, _ := cmd.Flags().GetString(flag); v != "" {
				*target = v
			}
		}
	}

	overrideBool := func(target *bool, flag string) {
		if cmd.Flags().Changed(flag) {
			*target, _ = cmd.Flags().GetBool(flag)
		}
	}

	overrideInt := func(target *int, flag string) {
		if cmd.Flags().Changed(flag) {
			*target, _ = cmd.Flags().GetInt(flag)
		}
	}

	// Apply overrides
	overrideBool(&cfg.ClusterScope, "cluster-scope")
	overrideInt(&cfg.Port, "port")
	overrideString(&cfg.Host, "host")
	overrideString(&cfg.AllowOrigin, "allow-origin")
	overrideString(&cfg.LogLevel, "log-level")

	if cmd.Flags().Changed("kube-qps") {
		cfg.KubeQPS, _ = cmd.Flags().GetFloat32("kube-qps")
	}
	if cmd.Flags().Changed("kube-burst") {
		cfg.KubeBurst, _ = cmd.Flags().GetInt("kube-burst")
	}

	// Authentication overrides
	overrideString(&cfg.OIDCIssuerURL, "oidc-issuer")
	overrideString(&cfg.OIDCClientID, "oidc-client-id")
	overrideString(&cfg.OIDCClientSecret, "oidc-client-secret")
	overrideString(&cfg.OIDCRedirectURL, "oidc-redirect-url")
	if cmd.Flags().Changed("oidc-scopes") {
		cfg.OIDCScopes, _ = cmd.Flags().GetStringSlice("oidc-scopes")
	}
	overrideBool(&cfg.OIDCInsecureSkipVerify, "oidc-insecure-skip-verify")
	overrideBool(&cfg.EnableLocalLogin, "enable-local-login")
	overrideBool(&cfg.AllowTokenParam, "allow-token-param")
	overrideInt(&cfg.SessionTTLMinutes, "session-ttl-minutes")
	overrideString(&cfg.SessionCookieName, "session-cookie-name")

	// RBAC overrides
	overrideString(&cfg.RBACModelPath, "rbac-model-path")
	overrideString(&cfg.RBACPolicyPath, "rbac-policy-path")

	return cfg
}

// setupRBAC initializes the RBAC system
func setupRBAC(cfg *config.ServerConfig) (*RBAC, error) {
	// Push explicit RBAC paths into environment if provided
	if cfg.RBACModelPath != "" {
		os.Setenv(envModelPath, cfg.RBACModelPath)
	}
	if cfg.RBACPolicyPath != "" {
		os.Setenv(envPolicyPath, cfg.RBACPolicyPath)
	}

	rbac, err := NewRBACFromEnv(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RBAC: %w", err)
	}

	logger.Info("RBAC system initialized",
		"modelPath", rbac.modelPath,
		"policyPath", rbac.policyPath,
	)

	return rbac, nil
}
