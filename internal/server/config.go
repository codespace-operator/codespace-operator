package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/codespace-operator/common/auth/pkg/auth"
	"github.com/codespace-operator/common/common/pkg/common"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v2"
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
	LogLevel         string `mapstructure:"log_level"`
	SameSiteOverride string `mapstructure:"same_site_override"`
	// RBAC (Casbin) files
	RBACModelPath  string `mapstructure:"rbac_model_path"`
	RBACPolicyPath string `mapstructure:"rbac_policy_path"`
	LocalUsersPath string `mapstructure:"local_users_path"`

	Auth AuthNode `mapstructure:"auth"`
}

// What the server accepts under the `auth:` key
// - `file`   : path to a YAML file with the canonical auth schema (AuthFileConfig)
// - `inline` : arbitrary YAML/JSON object to merge on top (same schema)
// - additional keys (e.g. providers.ldap.url) are allowed and merged too.
type AuthNode struct {
	File   string         `mapstructure:"file"`
	Inline map[string]any `mapstructure:"inline"`
	Extra  map[string]any `mapstructure:",remain"` // everything else under auth:
}

// -----------------------------
// Loader entry points
// -----------------------------

// LoadServerConfig builds the server config from defaults, file(s), env.
func LoadServerConfig() (*ServerConfig, *viper.Viper, error) {
	v := viper.New()
	setServerDefaults(v)

	// Standard project helper: config dir or file, env prefix, etc.
	common.SetupViper(v, "CODESPACE_SERVER", "server-config")

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal server config: %w", err)
	}
	return &cfg, v, nil
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

func setServerDefaults(v *viper.Viper) {
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

	// RBAC locations (can be overridden)
	v.SetDefault("rbac_model_path", "/etc/codespace-operator/rbac/model.conf")
	v.SetDefault("rbac_policy_path", "/etc/codespace-operator/rbac/policy.csv")

	// Local users path default (only used if auth.providers.local.enabled)
	v.SetDefault("local_users_path", "/etc/codespace-operator/local-users.yaml")

	// auth: defaults are set by the auth package itself; we don’t duplicate here
	v.SetDefault("auth.file", "")
	v.SetDefault("auth.inline", map[string]any{})
}

/*
	-----------------------------
	  Auth assembly (precedence)
	  defaults < auth.file < env/flags under auth.* < auth.inline

-----------------------------
*/
func (c *ServerConfig) BuildAuthConfig(v *viper.Viper) (*auth.AuthConfig, error) {
	merged := map[string]any{}
	// 1) file config
	if strings.TrimSpace(c.Auth.File) != "" {
		b, err := os.ReadFile(c.Auth.File)
		if err != nil {
			return nil, fmt.Errorf("read auth.file: %w", err)
		}
		var m map[string]any
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("parse auth.file: %w", err)
		}
		deepMerge(merged, m)
	}

	// 2) env / CLI (already in viper)
	if sub := v.Sub("auth"); sub != nil {
		envMap := sub.AllSettings()
		delete(envMap, "file")
		delete(envMap, "inline")
		deepMerge(merged, envMap)
	}

	// 3) inline
	if c.Auth.Inline != nil {
		deepMerge(merged, c.Auth.Inline)
	}

	// 4) re-unmarshal merged map into YAML, then let auth validate
	b, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged auth: %w", err)
	}

	return auth.LoadAuthConfigFromYAML(b)
}

/* -----------------------------
   tiny merge helper (map or struct)
----------------------------- */

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			continue
		}
		if sm, ok := v.(map[string]any); ok {
			if dm, ok2 := dst[k].(map[string]any); ok2 {
				deepMerge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}
