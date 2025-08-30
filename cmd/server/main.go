//go:generate swag init -g codespace_server.go -o ../../gen/api --parseDependency --parseInternal -ot json,yaml

// cmd/server/main.go

// @title Codespace Operator API
// @version 2.0.0
// @description REST API for managing Codespace sessions with RBAC and multi-provider authentication
// @termsOfService https://github.com/codespace-operator/codespace-operator/blob/main/TERMS.md

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
// @description Bearer token authentication (JWT)

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name codespace_session
// @description Session cookie authentication

// @tag.name authentication
// @tag.description Authentication and authorization operations

// @tag.name sessions
// @tag.description Session management operations

// @tag.name admin
// @tag.description Administrative operations (requires admin role)

// @tag.name health
// @tag.description Health and readiness checks

// @tag.name user
// @tag.description User information and permissions

package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codespace-operator/codespace-operator/internal/common"
	server "github.com/codespace-operator/codespace-operator/internal/server"
)

var logger = common.GetLogger()

const DEFAULT_APP_NAME = "codespace-server"

func main() {
	var rootCmd = &cobra.Command{
		Use:   "codespace-server",
		Short: "Codespace Operator Web Server",
		Long:  `A web server that provides a REST API and UI for managing Codespace sessions.`,
		Run:   func(cmd *cobra.Command, args []string) { runServer(loadConfigWithOverrides(cmd), args) },
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
	rootCmd.Flags().String("oidc-issuer-url", "", "OIDC issuer URL (e.g., https://dev-xxxx.okta.com)")
	rootCmd.Flags().String("oidc-client-id", "", "OIDC client ID")
	rootCmd.Flags().String("oidc-client-secret", "", "OIDC client secret")
	rootCmd.Flags().String("oidc-redirect-url", "", "OIDC redirect URL (https://host/auth/callback)")
	rootCmd.Flags().StringSlice("oidc-scopes", []string{}, "OIDC scopes (default: openid profile email)")
	rootCmd.Flags().Bool("oidc-insecure-skip-verify", false, "Skip OIDC server certificate verification")

	rootCmd.Flags().Bool("enable-local-login", false, "Enable local users login)")
	rootCmd.Flags().Bool("enable-bootstrap-login", false, "Enable bootstrap login (dev only)")
	rootCmd.Flags().Bool("allow-token-param", false, "Allow ?access_token=... on URLs (NOT recommended)")
	rootCmd.Flags().Int("session-ttl-minutes", 60, "Session cookie TTL in minutes")
	rootCmd.Flags().String("session-cookie-name", "", "Override session cookie name")

	// RBAC flags
	rootCmd.Flags().String("rbac-model-path", "", "Path to Casbin model.conf (overrides default/env)")
	rootCmd.Flags().String("rbac-policy-path", "", "Path to Casbin policy.csv (overrides default/env)")
	rootCmd.Flags().String("app-name", DEFAULT_APP_NAME, "Override application name")

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", "err", err)
	}

}

// Wrapper to run server package
func runServer(cfg *server.ServerConfig, args []string) {
	server.RunServer(cfg, args)
}

// loadConfigWithOverrides loads configuration with CLI flag overrides
func loadConfigWithOverrides(cmd *cobra.Command) *server.ServerConfig {
	// Set config path if provided
	if cmd.Flags().Changed("config") {
		if p, _ := cmd.Flags().GetString("config"); strings.TrimSpace(p) != "" {
			os.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", p)
		}
	}

	// Load base configuration
	cfg, err := server.LoadServerConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "err", err)
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
	overrideString(&cfg.AppName, "app-name")

	if cmd.Flags().Changed("kube-qps") {
		cfg.KubeQPS, _ = cmd.Flags().GetFloat32("kube-qps")
	}
	if cmd.Flags().Changed("kube-burst") {
		cfg.KubeBurst, _ = cmd.Flags().GetInt("kube-burst")
	}

	// Authentication overrides
	overrideString(&cfg.OIDCIssuerURL, "oidc-issuer-url")
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
	overrideBool(&cfg.BootstrapLoginAllowed, "enable-bootstrap-login")
	// RBAC overrides
	overrideString(&cfg.RBACModelPath, "rbac-model-path")
	overrideString(&cfg.RBACPolicyPath, "rbac-policy-path")

	return cfg
}
