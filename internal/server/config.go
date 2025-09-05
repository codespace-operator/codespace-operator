package server

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/codespace-operator/common/common/pkg/common"
	"github.com/spf13/viper"
)

// -----------------------------
// Structs (snake_case tags)
// -----------------------------

// ServerConfig holds all configuration for the codespace server
type ServerConfig struct {
	ClusterScope bool `mapstructure:"cluster_scope"`
	// Network
	AppName       string `mapstructure:"app_name"`
	DeveloperMode bool   `mapstructure:"developer_mode"`

	Port         int    `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`

	// CORS
	AllowOrigin string `mapstructure:"allow_origin"`

	// Kubernetes
	KubeQPS   float32 `mapstructure:"kube_qps"`
	KubeBurst int     `mapstructure:"kube_burst"`

	// Logging
	LogLevel string `mapstructure:"log_level"`

	// Sessions / auth
	JWTSecret string `mapstructure:"jwt_secret"`

	// Local login
	EnableLocalLogin      bool   `mapstructure:"enable_local_login"`
	BootstrapLoginAllowed bool   `mapstructure:"bootstrap_login_allowed"`
	BootstrapUser         string `mapstructure:"bootstrap_user"`
	BootstrapPassword     string `mapstructure:"bootstrap_password"`

	// Session cookie
	SessionCookieName string `mapstructure:"session_cookie_name"`
	SessionTTLMinutes int    `mapstructure:"session_ttl_minutes"`
	AllowTokenParam   bool   `mapstructure:"allow_token_param"`

	// Auth Paths
	AuthPath       string `mapstructure:"auth_path"`
	AuthLogoutPath string `mapstructure:"auth_logout_path"`

	// OIDC
	OIDCInsecureSkipVerify bool     `mapstructure:"oidc_insecure_skip_verify"`
	OIDCIssuerURL          string   `mapstructure:"oidc_issuer_url"`
	OIDCClientID           string   `mapstructure:"oidc_client_id"`
	OIDCClientSecret       string   `mapstructure:"oidc_client_secret"`
	OIDCRedirectURL        string   `mapstructure:"oidc_redirect_url"`
	OIDCScopes             []string `mapstructure:"oidc_scopes"`

	// LDAP
	LDAPEnabled            bool                `mapstructure:"ldap_enabled"`
	LDAPURL                string              `mapstructure:"ldap_url"`
	LDAPStartTLS           bool                `mapstructure:"ldap_start_tls"`
	LDAPInsecureSkipVerify bool                `mapstructure:"ldap_insecure_skip_verify"`
	LDAPBindDN             string              `mapstructure:"ldap_bind_dn"`
	LDAPBindPassword       string              `mapstructure:"ldap_bind_password"`
	LDAPUserDNTemplate     string              `mapstructure:"ldap_user_dn_template"`
	LDAPUserBaseDN         string              `mapstructure:"ldap_user_base_dn"`
	LDAPUserFilter         string              `mapstructure:"ldap_user_filter"`
	LDAPUsernameAttr       string              `mapstructure:"ldap_username_attr"`
	LDAPEmailAttr          string              `mapstructure:"ldap_email_attr"`
	LDAPDisplayNameAttr    string              `mapstructure:"ldap_display_name_attr"`
	LDAPGroupBaseDN        string              `mapstructure:"ldap_group_base_dn"`
	LDAPGroupFilter        string              `mapstructure:"ldap_group_filter"`
	LDAPGroupAttr          string              `mapstructure:"ldap_group_attr"`
	LDAPRoleMapping        map[string][]string `mapstructure:"ldap_role_mapping"`
	LDAPDefaultRoles       []string            `mapstructure:"ldap_default_roles"`
	LDAPToLowerUsername    bool                `mapstructure:"ldap_to_lower_username"`

	// RBAC (Casbin) files
	RBACModelPath  string `mapstructure:"rbac_model_path"`
	RBACPolicyPath string `mapstructure:"rbac_policy_path"`
	LocalUsersPath string `mapstructure:"local_users_path"`
}

// -----------------------------
// Loader entry points
// -----------------------------

// LoadServerConfig reads server-config.yaml + env (CODESPACE_SERVER_*) into ServerConfig.
func LoadServerConfig() (*ServerConfig, error) {
	v := viper.New()

	// Defaults (match prior behavior)
	v.SetDefault("cluster_scope", false)
	v.SetDefault("host", "")
	v.SetDefault("port", 8080)
	v.SetDefault("read_timeout", 0)
	v.SetDefault("write_timeout", 0)
	v.SetDefault("app_name", "codespace-server")
	v.SetDefault("developer_mode", false)

	v.SetDefault("allow_origin", "")

	v.SetDefault("kube_qps", 50.0)
	v.SetDefault("kube_burst", 100)

	v.SetDefault("log_level", "info")
	v.SetDefault("jwt_secret", "change-me")

	v.SetDefault("enable_local_login", false)
	v.SetDefault("local_users_path", "/etc/codespace-operator/local-users.yaml")
	v.SetDefault("bootstrap_login_allowed", false)
	v.SetDefault("bootstrap_user", "")
	v.SetDefault("bootstrap_password", "")
	v.SetDefault("session_cookie_name", "")
	v.SetDefault("session_ttl_minutes", 60)
	v.SetDefault("allow_token_param", false)

	v.SetDefault("oidc_insecure_skip_verify", false)
	v.SetDefault("oidc_issuer_url", "")
	v.SetDefault("oidc_client_id", "")
	v.SetDefault("oidc_client_secret", "")
	v.SetDefault("oidc_redirect_url", "")
	v.SetDefault("oidc_scopes", []string{}) // default scopes applied later in code if empty

	v.SetDefault("rbac_model_path", "")
	v.SetDefault("rbac_policy_path", "")

	common.SetupViper(v, "CODESPACE_SERVER", "server-config")

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %w", err)
	}

	// Allow a comma-separated env override for OIDC scopes
	// (e.g., CODESPACE_SERVER_OIDC_SCOPES="openid,profile,email")
	if len(cfg.OIDCScopes) == 0 {
		if raw := strings.TrimSpace(os.Getenv("CODESPACE_SERVER_OIDC_SCOPES")); raw != "" {
			cfg.OIDCScopes = common.SplitCSV(raw)
		}
	}

	return &cfg, nil
}

// -----------------------------
// Helpers / methods
// -----------------------------

func (c *ServerConfig) GetAddr() string {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Sprintf(":%d", c.Port)
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c *ServerConfig) SessionTTL() time.Duration {
	min := c.SessionTTLMinutes
	if min <= 0 {
		min = 60
	}
	return time.Duration(min) * time.Minute
}
