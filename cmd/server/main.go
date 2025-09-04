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

	"github.com/codespace-operator/common/common/pkg/common"
	"github.com/spf13/cobra"

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
	rootCmd.Flags().String("app-name", DEFAULT_APP_NAME, "Override application name")
	rootCmd.Flags().Bool("developer-mode", false, "Enable developer mode (less secure, for testing only)")
	// Authentication flags
	rootCmd.Flags().String("auth-path", "", "Authentication endpoint path")
	rootCmd.Flags().String("auth-logout-path", "", "Authentication logout endpoint path")

	// OIDC flags
	rootCmd.Flags().String("oidc-issuer-url", "", "OIDC issuer URL (e.g., https://dev-xxxx.okta.com)")
	rootCmd.Flags().String("oidc-client-id", "", "OIDC client ID")
	rootCmd.Flags().String("oidc-client-secret", "", "OIDC client secret")
	rootCmd.Flags().String("oidc-redirect-url", "", "OIDC redirect URL (https://host/auth/callback)")
	rootCmd.Flags().StringSlice("oidc-scopes", []string{}, "OIDC scopes (default: openid profile email)")
	rootCmd.Flags().Bool("oidc-insecure-skip-verify", false, "Skip OIDC server certificate verification")

	// Local login flags
	rootCmd.Flags().Bool("enable-local-login", false, "Enable local users login)")
	rootCmd.Flags().Bool("local-users-path", false, "Path to local users file")
	rootCmd.Flags().Bool("enable-bootstrap-login", false, "Enable bootstrap login (dev only)")
	rootCmd.Flags().Bool("allow-token-param", false, "Allow ?access_token=... on URLs (NOT recommended)")
	rootCmd.Flags().Int("session-ttl-minutes", 60, "Session cookie TTL in minutes")
	rootCmd.Flags().String("session-cookie-name", "", "Override session cookie name")

	// LDAP flags
	rootCmd.Flags().String("ldap-url", "", "LDAP server URL (e.g., ldaps://ldap.example.com:636 or ldap://host:389)")
	rootCmd.Flags().Bool("ldap-start-tls", false, "Use StartTLS (if URL is ldap://)")
	rootCmd.Flags().Bool("ldap-insecure-skip-verify", false, "Skip LDAP server certificate verification")
	rootCmd.Flags().String("ldap-bind-dn", "", "LDAP bind DN for searching users (optional)")
	rootCmd.Flags().String("ldap-bind-password", "", "LDAP bind password for searching users (optional)")
	rootCmd.Flags().String("ldap-user-dn-template", "", "LDAP user DN template (e.g., uid={username},ou=People,dc=example,dc=com) (optional)")
	rootCmd.Flags().String("ldap-user-base-dn", "", "LDAP user base DN (e.g., ou=People,dc=example,dc=com) (if not using UserDNTemplate)")
	rootCmd.Flags().String("ldap-user-filter", "", "LDAP user search filter (e.g., (uid={username})) (if not using UserDNTemplate)")
	rootCmd.Flags().String("ldap-username-attr", "uid", "LDAP username attribute (e.g., uid or sAMAccountName)")
	rootCmd.Flags().String("ldap-displayname-attr", "cn", "LDAP display name attribute (e.g., cn or displayName)")
	rootCmd.Flags().String("ldap-group-base-dn", "", "LDAP group base DN (e.g., ou=Groups,dc=example,dc=com) (optional)")
	rootCmd.Flags().String("ldap-group-filter", "", "LDAP group search filter (e.g., (member={userDN}) or (memberUid={username})) (optional)")
	rootCmd.Flags().String("ldap-group-attr", "cn", "LDAP group attribute to read as group name (default: cn) (optional)")
	rootCmd.Flags().StringToString("ldap-role-mapping", map[string]string{}, "LDAP group name or DN to Codespace role mapping (e.g., 'admins=admin,editors=editor')")
	rootCmd.Flags().StringSlice("ldap-default-roles", []string{}, "Default roles if no LDAP group matches (default: viewer)")
	rootCmd.Flags().Bool("ldap-to-lower-username", false, "Convert LDAP username to lower case")
	// RBAC flags
	rootCmd.Flags().String("rbac-model-path", "", "Path to Casbin model.conf (overrides default/env)")
	rootCmd.Flags().String("rbac-policy-path", "", "Path to Casbin policy.csv (overrides default/env)")

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
	overrideBool(&cfg.DeveloperMode, "developer-mode")

	if cmd.Flags().Changed("kube-qps") {
		cfg.KubeQPS, _ = cmd.Flags().GetFloat32("kube-qps")
	}
	if cmd.Flags().Changed("kube-burst") {
		cfg.KubeBurst, _ = cmd.Flags().GetInt("kube-burst")
	}

	// Authentication overrides
	overrideString(&cfg.AuthPath, "auth-path")
	overrideString(&cfg.AuthLogoutPath, "auth-logout-path")

	overrideString(&cfg.OIDCIssuerURL, "oidc-issuer-url")
	overrideString(&cfg.OIDCClientID, "oidc-client-id")
	overrideString(&cfg.OIDCClientSecret, "oidc-client-secret")
	overrideString(&cfg.OIDCRedirectURL, "oidc-redirect-url")
	if cmd.Flags().Changed("oidc-scopes") {
		cfg.OIDCScopes, _ = cmd.Flags().GetStringSlice("oidc-scopes")
	}
	overrideBool(&cfg.OIDCInsecureSkipVerify, "oidc-insecure-skip-verify")
	overrideBool(&cfg.EnableLocalLogin, "enable-local-login")
	overrideString(&cfg.LocalUsersPath, "local-users-path")
	overrideBool(&cfg.AllowTokenParam, "allow-token-param")
	overrideInt(&cfg.SessionTTLMinutes, "session-ttl-minutes")
	overrideString(&cfg.SessionCookieName, "session-cookie-name")
	overrideBool(&cfg.BootstrapLoginAllowed, "enable-bootstrap-login")

	// LDAP overrides
	overrideString(&cfg.LDAPURL, "ldap-url")
	overrideBool(&cfg.LDAPStartTLS, "ldap-start-tls")
	overrideBool(&cfg.LDAPInsecureSkipVerify, "ldap-insecure-skip-verify")
	overrideString(&cfg.LDAPBindDN, "ldap-bind-dn")
	overrideString(&cfg.LDAPBindPassword, "ldap-bind-password")
	overrideString(&cfg.LDAPUserDNTemplate, "ldap-user-dn-template")
	overrideString(&cfg.LDAPUserBaseDN, "ldap-user-base-dn")
	overrideString(&cfg.LDAPUserFilter, "ldap-user-filter")
	overrideString(&cfg.LDAPUsernameAttr, "ldap-username-attr")
	overrideString(&cfg.LDAPDisplayNameAttr, "ldap-displayname-attr")
	overrideString(&cfg.LDAPGroupBaseDN, "ldap-group-base-dn")
	overrideString(&cfg.LDAPGroupFilter, "ldap-group-filter")
	overrideString(&cfg.LDAPGroupAttr, "ldap-group-attr")
	if cmd.Flags().Changed("ldap-role-mapping") {
		raw, err := cmd.Flags().GetStringToString("ldap-role-mapping")
		if err == nil {
			cfg.LDAPRoleMapping = server.ParseLDAPRoleMapping(raw)
		}
	}

	if cmd.Flags().Changed("ldap-default-roles") {
		roles, err := cmd.Flags().GetStringSlice("ldap-default-roles")
		if err == nil {
			cfg.LDAPDefaultRoles = roles
		}
	}
	overrideBool(&cfg.LDAPToLowerUsername, "ldap-to-lower-username")
	// RBAC overrides
	overrideString(&cfg.RBACModelPath, "rbac-model-path")
	overrideString(&cfg.RBACPolicyPath, "rbac-policy-path")

	// Sanitize paths
	cfg.AuthPath = strings.TrimRight(cfg.AuthPath, "/")
	cfg.AuthLogoutPath = strings.TrimRight(cfg.AuthLogoutPath, "/")

	return cfg
}
