package server

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/codespace-operator/common/auth/pkg/auth"
	"github.com/codespace-operator/common/common/pkg/common"
	"github.com/spf13/viper"
)

// Helper: build viper like LoadServerConfig does, but for tests
func newTestServerViper(t *testing.T) *viper.Viper {
	t.Helper()
	v := viper.New()
	setServerDefaults(v)
	common.SetupViper(v, "CODESPACE_SERVER", "server-config")
	return v
}

// Helper: write an auth config file
func writeAuthFile(t *testing.T, dir string, contents string) string {
	t.Helper()
	fp := filepath.Join(dir, "auth.yaml")
	if err := os.WriteFile(fp, []byte(contents), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	return fp
}

func TestBuildAuthConfig_EnvOverridesFile_ForLDAPPassword(t *testing.T) {
	// Clear any config path interference
	t.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", "")

	dir := t.TempDir()
	authYAML := `
manager:
  jwt_secret: "file-secret"
providers:
  ldap:
    enabled: true
    url: "ldap://openldap.ldap.svc.cluster.local:389"
    start_tls: false
    insecure_skip_verify: true
    bind_dn: "cn=admin,dc=codespace,dc=test"
    bind_password: ""   # to be overridden via AUTH_ ENV
    user:
      base_dn: "ou=people,dc=codespace,dc=test"
      filter: "(|(uid={username})(mail={username}))"
      attrs:
        username: "uid"
        email: "mail"
        display_name: "cn"
      to_lower_username: true
    group:
      base_dn: "ou=groups,dc=codespace,dc=test"
      filter: "(member={userDN})"
      attr: "cn"
    roles:
      mapping:
        "codespace-operator:admin": ["admin"]
      default: ["viewer"]
  local:
    enabled: false
  oidc:
    enabled: false
`
	authFilePath := writeAuthFile(t, dir, authYAML)

	// Set server config to point to our auth file - correct environment variable name
	t.Setenv("CODESPACE_SERVER_AUTH_CONFIG_PATH", authFilePath)

	// Set AUTH_ environment variable to override the empty password
	t.Setenv("AUTH_PROVIDERS_LDAP_BIND_PASSWORD", "admin")

	// Debug: verify environment variables are set
	t.Logf("Auth file path: %s", authFilePath)
	t.Logf("CODESPACE_SERVER_AUTH_CONFIG_PATH: %s", os.Getenv("CODESPACE_SERVER_AUTH_CONFIG_PATH"))
	t.Logf("AUTH_PROVIDERS_LDAP_BIND_PASSWORD: %s", os.Getenv("AUTH_PROVIDERS_LDAP_BIND_PASSWORD"))

	// Build server config
	v := newTestServerViper(t)
	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal server config: %v", err)
	}

	// Debug: verify config was loaded correctly
	t.Logf("Server config AuthConfigPath: %s", cfg.AuthConfigPath)

	// Build auth config using the simplified method
	ac, err := cfg.BuildAuthConfig()
	if err != nil {
		t.Fatalf("BuildAuthConfig failed: %v", err)
	}

	if ac.LDAP == nil || !ac.LDAP.Enabled {
		t.Fatalf("expected LDAP to be enabled")
	}
	if got := ac.LDAP.BindDN; got != "cn=admin,dc=codespace,dc=test" {
		t.Fatalf("BindDN unexpected: %q", got)
	}
	if got := ac.LDAP.BindPassword; got != "admin" {
		t.Fatalf("expected AUTH_ env to override bind_password, got %q", got)
	}
}

func TestBuildAuthConfig_EnvOnly_NoFile(t *testing.T) {
	t.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", "")

	// Configure everything via AUTH_ environment variables
	t.Setenv("AUTH_MANAGER_JWT_SECRET", "env-secret")
	t.Setenv("AUTH_MANAGER_SESSION_TTL", "30m")
	t.Setenv("AUTH_PROVIDERS_LOCAL_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_LOCAL_USERS_PATH", "/tmp/users.yaml")

	v := newTestServerViper(t)
	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal server config: %v", err)
	}

	// AuthConfigPath should be empty - using env only
	ac, err := cfg.BuildAuthConfig()
	if err != nil {
		t.Fatalf("BuildAuthConfig failed: %v", err)
	}

	if got := ac.JWTSecret; got != "env-secret" {
		t.Fatalf("AUTH_ env should work without file: want 'env-secret', got %q", got)
	}
	if got := ac.SessionTTL; got != 30*time.Minute {
		t.Fatalf("AUTH_ env should work without file: want 30m, got %v", got)
	}
	if ac.Local == nil || !ac.Local.Enabled {
		t.Fatalf("local provider should be enabled via AUTH_ env")
	}
}

// Test the auth package functions directly
func TestAuthPackage_LoadAuthConfigWithEnv(t *testing.T) {
	dir := t.TempDir()
	authYAML := `
manager:
  jwt_secret: "file-jwt"
  session_ttl: "15m"
providers:
  local:
    enabled: false
`
	authFilePath := writeAuthFile(t, dir, authYAML)

	// Environment should override some values
	t.Setenv("AUTH_MANAGER_SESSION_TTL", "45m")
	t.Setenv("AUTH_PROVIDERS_LOCAL_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_LOCAL_USERS_PATH", "/env/users.yaml")

	// Use the auth package directly
	ac, err := auth.LoadAuthConfigWithEnv("AUTH", authFilePath)
	if err != nil {
		t.Fatalf("LoadAuthConfigWithEnv failed: %v", err)
	}

	// File value should be preserved where not overridden
	if got := ac.JWTSecret; got != "file-jwt" {
		t.Fatalf("JWT secret from file: want 'file-jwt', got %q", got)
	}

	// Environment should override file
	if got := ac.SessionTTL; got != 45*time.Minute {
		t.Fatalf("Session TTL from env: want 45m, got %v", got)
	}

	// Environment-only values should work
	if ac.Local == nil || !ac.Local.Enabled {
		t.Fatalf("Local should be enabled via env")
	}
	if got := ac.Local.UsersPath; got != "/env/users.yaml" {
		t.Fatalf("Users path from env: want '/env/users.yaml', got %q", got)
	}
}

func TestAuthPackage_LoadAuthConfigFromEnvOnly(t *testing.T) {
	// Pure environment configuration
	t.Setenv("AUTH_MANAGER_JWT_SECRET", "pure-env-secret")
	t.Setenv("AUTH_MANAGER_SESSION_TTL", "2h")
	t.Setenv("AUTH_PROVIDERS_OIDC_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_OIDC_ISSUER_URL", "https://auth.example.com")
	t.Setenv("AUTH_PROVIDERS_OIDC_CLIENT_ID", "env-client")
	t.Setenv("AUTH_PROVIDERS_OIDC_CLIENT_SECRET", "env-secret")
	t.Setenv("AUTH_PROVIDERS_OIDC_REDIRECT_URL", "https://app.example.com/callback")

	// Use the auth package directly with no file
	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnvOnly failed: %v", err)
	}

	if got := ac.JWTSecret; got != "pure-env-secret" {
		t.Fatalf("JWT secret: want 'pure-env-secret', got %q", got)
	}

	if got := ac.SessionTTL; got != 2*time.Hour {
		t.Fatalf("Session TTL: want 2h, got %v", got)
	}

	if ac.OIDC == nil || !ac.OIDC.Enabled {
		t.Fatalf("OIDC should be enabled")
	}

	if got := ac.OIDC.ClientID; got != "env-client" {
		t.Fatalf("OIDC client ID: want 'env-client', got %q", got)
	}
}

func TestServerConfig_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		verifyFn func(t *testing.T, cfg *ServerConfig)
	}{
		{
			name: "basic network settings",
			envVars: map[string]string{
				"CODESPACE_SERVER_HOST":          "0.0.0.0",
				"CODESPACE_SERVER_PORT":          "9090",
				"CODESPACE_SERVER_READ_TIMEOUT":  "30",
				"CODESPACE_SERVER_WRITE_TIMEOUT": "60",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if cfg.Host != "0.0.0.0" {
					t.Errorf("Host: want '0.0.0.0', got %q", cfg.Host)
				}
				if cfg.Port != 9090 {
					t.Errorf("Port: want 9090, got %d", cfg.Port)
				}
				if cfg.ReadTimeout != 30 {
					t.Errorf("ReadTimeout: want 30, got %d", cfg.ReadTimeout)
				}
				if cfg.WriteTimeout != 60 {
					t.Errorf("WriteTimeout: want 60, got %d", cfg.WriteTimeout)
				}
				if cfg.GetAddr() != "0.0.0.0:9090" {
					t.Errorf("GetAddr(): want '0.0.0.0:9090', got %q", cfg.GetAddr())
				}
			},
		},
		{
			name: "cluster and app settings",
			envVars: map[string]string{
				"CODESPACE_SERVER_CLUSTER_SCOPE":  "true",
				"CODESPACE_SERVER_APP_NAME":       "my-codespace",
				"CODESPACE_SERVER_DEVELOPER_MODE": "true",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if !cfg.ClusterScope {
					t.Error("ClusterScope: want true, got false")
				}
				if cfg.AppName != "my-codespace" {
					t.Errorf("AppName: want 'my-codespace', got %q", cfg.AppName)
				}
				if !cfg.DeveloperMode {
					t.Error("DeveloperMode: want true, got false")
				}
			},
		},
		{
			name: "CORS and logging",
			envVars: map[string]string{
				"CODESPACE_SERVER_ALLOW_ORIGIN": "https://example.com",
				"CODESPACE_SERVER_LOG_LEVEL":    "debug",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if cfg.AllowOrigin != "https://example.com" {
					t.Errorf("AllowOrigin: want 'https://example.com', got %q", cfg.AllowOrigin)
				}
				if cfg.LogLevel != "debug" {
					t.Errorf("LogLevel: want 'debug', got %q", cfg.LogLevel)
				}
			},
		},
		{
			name: "Kubernetes settings",
			envVars: map[string]string{
				"CODESPACE_SERVER_KUBE_QPS":   "100.5",
				"CODESPACE_SERVER_KUBE_BURST": "200",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if cfg.KubeQPS != 100.5 {
					t.Errorf("KubeQPS: want 100.5, got %f", cfg.KubeQPS)
				}
				if cfg.KubeBurst != 200 {
					t.Errorf("KubeBurst: want 200, got %d", cfg.KubeBurst)
				}
			},
		},
		{
			name: "RBAC paths",
			envVars: map[string]string{
				"CODESPACE_SERVER_RBAC_MODEL_PATH":  "/custom/rbac/model.conf",
				"CODESPACE_SERVER_RBAC_POLICY_PATH": "/custom/rbac/policy.csv",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if cfg.RBACModelPath != "/custom/rbac/model.conf" {
					t.Errorf("RBACModelPath: want '/custom/rbac/model.conf', got %q", cfg.RBACModelPath)
				}
				if cfg.RBACPolicyPath != "/custom/rbac/policy.csv" {
					t.Errorf("RBACPolicyPath: want '/custom/rbac/policy.csv', got %q", cfg.RBACPolicyPath)
				}
			},
		},
		{
			name: "auth config path",
			envVars: map[string]string{
				"CODESPACE_SERVER_AUTH_CONFIG_PATH": "/etc/auth/config.yaml",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if cfg.AuthConfigPath != "/etc/auth/config.yaml" {
					t.Errorf("AuthConfigPath: want '/etc/auth/config.yaml', got %q", cfg.AuthConfigPath)
				}
			},
		},
		{
			name:    "defaults when no env vars set",
			envVars: map[string]string{},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				// Verify defaults
				if cfg.ClusterScope {
					t.Error("ClusterScope: want false by default")
				}
				if cfg.Port != 8080 {
					t.Errorf("Port: want default 8080, got %d", cfg.Port)
				}
				if cfg.Host != "" {
					t.Errorf("Host: want empty by default, got %q", cfg.Host)
				}
				if cfg.AppName != "codespace-server" {
					t.Errorf("AppName: want default 'codespace-server', got %q", cfg.AppName)
				}
				if cfg.LogLevel != "info" {
					t.Errorf("LogLevel: want default 'info', got %q", cfg.LogLevel)
				}
				if cfg.KubeQPS != 50.0 {
					t.Errorf("KubeQPS: want default 50.0, got %f", cfg.KubeQPS)
				}
				if cfg.KubeBurst != 100 {
					t.Errorf("KubeBurst: want default 100, got %d", cfg.KubeBurst)
				}
				if cfg.GetAddr() != ":8080" {
					t.Errorf("GetAddr(): want default ':8080', got %q", cfg.GetAddr())
				}
			},
		},
		{
			name: "boolean parsing variations",
			envVars: map[string]string{
				"CODESPACE_SERVER_CLUSTER_SCOPE":  "1",
				"CODESPACE_SERVER_DEVELOPER_MODE": "false",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				if !cfg.ClusterScope {
					t.Error("ClusterScope: should accept '1' as true")
				}
				if cfg.DeveloperMode {
					t.Error("DeveloperMode: should be false when set to 'false'")
				}
			},
		},
		{
			name: "GetAddr() with different host configurations",
			envVars: map[string]string{
				"CODESPACE_SERVER_HOST": "   ", // whitespace only
				"CODESPACE_SERVER_PORT": "3000",
			},
			verifyFn: func(t *testing.T, cfg *ServerConfig) {
				// Should treat whitespace-only host as empty
				if cfg.GetAddr() != ":3000" {
					t.Errorf("GetAddr() with whitespace host: want ':3000', got %q", cfg.GetAddr())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any existing CODESPACE_SERVER_ env vars
			t.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", "")

			// Set test environment variables
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Load config
			cfg, _, err := LoadServerConfig()
			if err != nil {
				t.Fatalf("LoadServerConfig() failed: %v", err)
			}

			// Run verification
			tt.verifyFn(t, cfg)
		})
	}
}

func TestServerConfig_CompleteIntegration(t *testing.T) {
	// Test a complete configuration with all fields set via environment
	envVars := map[string]string{
		"CODESPACE_SERVER_CLUSTER_SCOPE":      "true",
		"CODESPACE_SERVER_APP_NAME":           "production-codespace",
		"CODESPACE_SERVER_DEVELOPER_MODE":     "false",
		"CODESPACE_SERVER_HOST":               "192.168.1.100",
		"CODESPACE_SERVER_PORT":               "8443",
		"CODESPACE_SERVER_READ_TIMEOUT":       "120",
		"CODESPACE_SERVER_WRITE_TIMEOUT":      "180",
		"CODESPACE_SERVER_ALLOW_ORIGIN":       "https://app.example.com",
		"CODESPACE_SERVER_KUBE_QPS":           "75.5",
		"CODESPACE_SERVER_KUBE_BURST":         "150",
		"CODESPACE_SERVER_LOG_LEVEL":          "warn",
		"CODESPACE_SERVER_SAME_SITE_OVERRIDE": "strict",
		"CODESPACE_SERVER_RBAC_MODEL_PATH":    "/opt/rbac/model.conf",
		"CODESPACE_SERVER_RBAC_POLICY_PATH":   "/opt/rbac/policy.csv",
		"CODESPACE_SERVER_AUTH_CONFIG_PATH":   "/opt/auth/config.yaml",
	}

	// Clear config path
	t.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", "")

	// Set all environment variables
	for k, v := range envVars {
		t.Setenv(k, v)
	}

	cfg, v, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() failed: %v", err)
	}

	// Verify all fields
	if !cfg.ClusterScope {
		t.Error("ClusterScope not set correctly")
	}
	if cfg.AppName != "production-codespace" {
		t.Errorf("AppName: want 'production-codespace', got %q", cfg.AppName)
	}
	if cfg.DeveloperMode {
		t.Error("DeveloperMode should be false")
	}
	if cfg.Host != "192.168.1.100" {
		t.Errorf("Host: want '192.168.1.100', got %q", cfg.Host)
	}
	if cfg.Port != 8443 {
		t.Errorf("Port: want 8443, got %d", cfg.Port)
	}
	if cfg.ReadTimeout != 120 {
		t.Errorf("ReadTimeout: want 120, got %d", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 180 {
		t.Errorf("WriteTimeout: want 180, got %d", cfg.WriteTimeout)
	}
	if cfg.AllowOrigin != "https://app.example.com" {
		t.Errorf("AllowOrigin incorrect: %q", cfg.AllowOrigin)
	}
	if cfg.KubeQPS != 75.5 {
		t.Errorf("KubeQPS: want 75.5, got %f", cfg.KubeQPS)
	}
	if cfg.KubeBurst != 150 {
		t.Errorf("KubeBurst: want 150, got %d", cfg.KubeBurst)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel: want 'warn', got %q", cfg.LogLevel)
	}
	if cfg.RBACModelPath != "/opt/rbac/model.conf" {
		t.Errorf("RBACModelPath incorrect: %q", cfg.RBACModelPath)
	}
	if cfg.RBACPolicyPath != "/opt/rbac/policy.csv" {
		t.Errorf("RBACPolicyPath incorrect: %q", cfg.RBACPolicyPath)
	}
	if cfg.AuthConfigPath != "/opt/auth/config.yaml" {
		t.Errorf("AuthConfigPath incorrect: %q", cfg.AuthConfigPath)
	}
	if cfg.GetAddr() != "192.168.1.100:8443" {
		t.Errorf("GetAddr(): want '192.168.1.100:8443', got %q", cfg.GetAddr())
	}

	// Verify viper has the values
	if v == nil {
		t.Fatal("Viper instance is nil")
	}
	if v.GetBool("cluster_scope") != true {
		t.Error("Viper doesn't have correct cluster_scope")
	}
	if v.GetString("app_name") != "production-codespace" {
		t.Error("Viper doesn't have correct app_name")
	}
}

func TestServerConfig_AuthConfigIntegration(t *testing.T) {
	// Test that auth config can be loaded via environment variable
	dir := t.TempDir()
	authYAML := `
manager:
  jwt_secret: "test-secret"
  session_ttl: "2h"
providers:
  local:
    enabled: true
    users_path: "/tmp/users.yaml"
`
	authPath := writeAuthFile(t, dir, authYAML)

	t.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", "")
	t.Setenv("CODESPACE_SERVER_AUTH_CONFIG_PATH", authPath)

	// Also test that AUTH_ env vars can override file values
	t.Setenv("AUTH_MANAGER_SESSION_TTL", "4h")

	cfg, _, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("LoadServerConfig() failed: %v", err)
	}

	if cfg.AuthConfigPath != authPath {
		t.Errorf("AuthConfigPath: want %q, got %q", authPath, cfg.AuthConfigPath)
	}

	// Build and verify auth config
	authCfg, err := cfg.BuildAuthConfig()
	if err != nil {
		t.Fatalf("BuildAuthConfig() failed: %v", err)
	}

	if authCfg.JWTSecret != "test-secret" {
		t.Errorf("JWTSecret from file: want 'test-secret', got %q", authCfg.JWTSecret)
	}

	// Should be overridden by AUTH_ env var
	if authCfg.SessionTTL != 4*time.Hour {
		t.Errorf("SessionTTL from env override: want 4h, got %v", authCfg.SessionTTL)
	}

	if !authCfg.Local.Enabled {
		t.Error("Local auth should be enabled")
	}
}

func TestServerConfig_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "invalid port number",
			envVars: map[string]string{
				"CODESPACE_SERVER_PORT": "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "invalid boolean",
			envVars: map[string]string{
				"CODESPACE_SERVER_CLUSTER_SCOPE": "maybe",
			},
			wantErr: true, // Viper treats non-true values as false
		},
		{
			name: "invalid float",
			envVars: map[string]string{
				"CODESPACE_SERVER_KUBE_QPS": "abc",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", "")

			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			_, _, err := LoadServerConfig()
			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestAuthEnv_ManagerBasics(t *testing.T) {
	t.Setenv("AUTH_MANAGER_JWT_SECRET", "secret-from-env")
	t.Setenv("AUTH_MANAGER_SESSION_COOKIE_NAME", "codespace_session")
	t.Setenv("AUTH_MANAGER_SESSION_TTL", "1h30m")
	t.Setenv("AUTH_MANAGER_ABSOLUTE_SESSION_MAX", "24h")
	t.Setenv("AUTH_MANAGER_SAME_SITE", "None")
	t.Setenv("AUTH_MANAGER_ALLOW_TOKEN_PARAM", "1") // bool variations

	// Also check path trimming
	t.Setenv("AUTH_MANAGER_AUTH_PATH", "/api/auth/")
	t.Setenv("AUTH_MANAGER_AUTH_LOGOUT_PATH", "/api/logout/")

	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnvOnly: %v", err)
	}

	if ac.JWTSecret != "secret-from-env" {
		t.Errorf("JWTSecret: want 'secret-from-env', got %q", ac.JWTSecret)
	}
	if ac.SessionCookieName != "codespace_session" {
		t.Errorf("SessionCookieName: want 'codespace_session', got %q", ac.SessionCookieName)
	}
	if ac.SessionTTL != 90*time.Minute {
		t.Errorf("SessionTTL: want 1h30m, got %v", ac.SessionTTL)
	}
	if ac.AbsoluteSessionMax != 24*time.Hour {
		t.Errorf("AbsoluteSessionMax: want 24h, got %v", ac.AbsoluteSessionMax)
	}
	if ac.SameSiteMode != http.SameSiteNoneMode {
		t.Errorf("SameSite: want None, got %v", ac.SameSiteMode)
	}
	if !ac.AllowTokenParam {
		t.Errorf("AllowTokenParam: want true")
	}
	if ac.AuthPath != "/api/auth" {
		t.Errorf("AuthPath: want '/api/auth', got %q", ac.AuthPath)
	}
	if ac.AuthLogoutPath != "/api/logout" {
		t.Errorf("AuthLogoutPath: want '/api/logout', got %q", ac.AuthLogoutPath)
	}
}

func TestAuthEnv_OIDCScopesCSV(t *testing.T) {
	t.Setenv("AUTH_MANAGER_JWT_SECRET", "jwt")
	t.Setenv("AUTH_PROVIDERS_OIDC_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_OIDC_ISSUER_URL", "https://issuer.example.com")
	t.Setenv("AUTH_PROVIDERS_OIDC_CLIENT_ID", "client-id")
	t.Setenv("AUTH_PROVIDERS_OIDC_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_PROVIDERS_OIDC_REDIRECT_URL", "https://app.example.com/callback")
	t.Setenv("AUTH_PROVIDERS_OIDC_SCOPES", "openid,profile,email")
	t.Setenv("AUTH_PROVIDERS_OIDC_INSECURE_SKIP_VERIFY", "true")

	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnvOnly: %v", err)
	}
	if ac.OIDC == nil || !ac.OIDC.Enabled {
		t.Fatalf("OIDC should be enabled")
	}
	wantScopes := []string{"openid", "profile", "email"}
	if !reflect.DeepEqual(ac.OIDC.Scopes, wantScopes) {
		t.Errorf("OIDC scopes: want %v, got %v", wantScopes, ac.OIDC.Scopes)
	}
	if !ac.OIDC.InsecureSkipVerify {
		t.Errorf("OIDC InsecureSkipVerify: want true")
	}
}

func TestAuthEnv_LDAP_WithBindDNAndPassword(t *testing.T) {
	t.Setenv("AUTH_MANAGER_JWT_SECRET", "jwt")
	t.Setenv("AUTH_PROVIDERS_LDAP_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_LDAP_URL", "ldap://ldap.internal:389")
	t.Setenv("AUTH_PROVIDERS_LDAP_START_TLS", "false")
	t.Setenv("AUTH_PROVIDERS_LDAP_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("AUTH_PROVIDERS_LDAP_BIND_DN", "cn=admin,dc=example,dc=org")
	t.Setenv("AUTH_PROVIDERS_LDAP_BIND_PASSWORD", "s3cr3t")

	t.Setenv("AUTH_PROVIDERS_LDAP_USER_BASE_DN", "ou=people,dc=example,dc=org")
	t.Setenv("AUTH_PROVIDERS_LDAP_USER_FILTER", "(|(uid={username})(mail={username}))")
	t.Setenv("AUTH_PROVIDERS_LDAP_USER_ATTRS_USERNAME", "uid")
	t.Setenv("AUTH_PROVIDERS_LDAP_USER_ATTRS_EMAIL", "mail")
	t.Setenv("AUTH_PROVIDERS_LDAP_USER_ATTRS_DISPLAY_NAME", "cn")
	t.Setenv("AUTH_PROVIDERS_LDAP_USER_TO_LOWER_USERNAME", "true")

	t.Setenv("AUTH_PROVIDERS_LDAP_GROUP_BASE_DN", "ou=groups,dc=example,dc=org")
	t.Setenv("AUTH_PROVIDERS_LDAP_GROUP_FILTER", "(member={userDN})")
	t.Setenv("AUTH_PROVIDERS_LDAP_GROUP_ATTR", "cn")

	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnvOnly: %v", err)
	}
	if ac.LDAP == nil || !ac.LDAP.Enabled {
		t.Fatalf("LDAP should be enabled")
	}
	if ac.LDAP.BindDN != "cn=admin,dc=example,dc=org" || ac.LDAP.BindPassword != "s3cr3t" {
		t.Errorf("Bind DN/password not applied from env")
	}
	if !ac.LDAP.ToLowerUsername {
		t.Errorf("ToLowerUsername: want true")
	}
	if ac.LDAP.UsernameAttr != "uid" || ac.LDAP.EmailAttr != "mail" || ac.LDAP.DisplayNameAttr != "cn" {
		t.Errorf("User attrs not applied correctly: %s %s %s", ac.LDAP.UsernameAttr, ac.LDAP.EmailAttr, ac.LDAP.DisplayNameAttr)
	}
}

func TestAuthEnv_LDAP_ErrorWhenMissingURL(t *testing.T) {
	t.Setenv("AUTH_PROVIDERS_LDAP_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_LDAP_BIND_DN", "cn=x,dc=example,dc=org")
	t.Setenv("AUTH_PROVIDERS_LDAP_BIND_PASSWORD", "pw")
	_, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err == nil || !strings.Contains(err.Error(), "url is empty") {
		t.Fatalf("want error about empty url, got: %v", err)
	}
}

func TestAuthEnv_LDAP_ErrorWhenBindDNWithoutPassword(t *testing.T) {
	t.Setenv("AUTH_PROVIDERS_LDAP_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_LDAP_URL", "ldap://x")
	t.Setenv("AUTH_PROVIDERS_LDAP_BIND_DN", "cn=x,dc=example,dc=org")
	_, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err == nil || !strings.Contains(err.Error(), "bind_dn provided but bind_password is empty") {
		t.Fatalf("want error about missing bind_password, got: %v", err)
	}
}

func TestAuthEnv_LDAP_UsingUserDNTemplate_NoBindDNNeeded(t *testing.T) {
	t.Setenv("AUTH_PROVIDERS_LDAP_ENABLED", "true")
	t.Setenv("AUTH_PROVIDERS_LDAP_URL", "ldap://x")
	t.Setenv("AUTH_PROVIDERS_LDAP_USER_DN_TEMPLATE", "uid={username},ou=people,dc=example,dc=org")

	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ac.LDAP == nil || !ac.LDAP.Enabled {
		t.Fatalf("LDAP should be enabled")
	}
	if ac.LDAP.BindDN != "" {
		t.Errorf("BindDN should be empty when using user.dn_template")
	}
	if ac.LDAP.UserDNTemplate == "" {
		t.Errorf("UserDNTemplate should be set")
	}
}

func TestAuthEnv_InvalidDurationsProduceError(t *testing.T) {
	t.Setenv("AUTH_MANAGER_SESSION_TTL", "banana")
	_, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err == nil || !strings.Contains(err.Error(), "invalid session_ttl") {
		t.Fatalf("expected invalid session_ttl error, got: %v", err)
	}
}

func TestAuthEnv_SameSite_InvalidFallsBackToLax(t *testing.T) {
	t.Setenv("AUTH_MANAGER_SAME_SITE", "weird")
	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("LoadAuthConfigFromEnvOnly: %v", err)
	}
	if ac.SameSiteMode != http.SameSiteLaxMode {
		t.Errorf("SameSite invalid -> Lax fallback expected, got %v", ac.SameSiteMode)
	}
}

func TestAuthEnv_PrefixIsolation(t *testing.T) {
	// Set XAUTH_* only
	t.Setenv("XAUTH_MANAGER_JWT_SECRET", "xsecret")

	// Using AUTH prefix should NOT see XAUTH_ values
	ac, err := auth.LoadAuthConfigFromEnvOnly("AUTH")
	if err != nil {
		t.Fatalf("AUTH load: %v", err)
	}
	if ac.JWTSecret == "xsecret" {
		t.Fatalf("AUTH prefix should not read XAUTH_ variables")
	}

	// Using XAUTH prefix should see them
	ac2, err := auth.LoadAuthConfigFromEnvOnly("XAUTH")
	if err != nil {
		t.Fatalf("XAUTH load: %v", err)
	}
	if ac2.JWTSecret != "xsecret" {
		t.Fatalf("XAUTH prefix failed to read env, got %q", ac2.JWTSecret)
	}
}
