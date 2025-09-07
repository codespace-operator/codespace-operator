package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/codespace-operator/common/auth/pkg/auth"
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

	// RBAC (Casbin) files
	RBACModelPath  string `mapstructure:"rbac_model_path"`
	RBACPolicyPath string `mapstructure:"rbac_policy_path"`

	// Auth config file path - note the correct mapstructure tag
	AuthConfigPath string `mapstructure:"auth_config_path"`
}

// -----------------------------
// Loader entry points
// -----------------------------

// LoadServerConfig builds the server config from defaults, file(s), env.
func LoadServerConfig() (*ServerConfig, *viper.Viper, error) {
	v := viper.New()
	setServerDefaults(v)

	// Standard project helper: config dir or file, env prefix, etc.
	// Only handles CODESPACE_SERVER_* variables
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

// Important for env registration as well for AutomaticEnv()
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

	v.SetDefault("rbac_model_path", "/etc/codespace-operator/rbac/model.conf")
	v.SetDefault("rbac_policy_path", "/etc/codespace-operator/rbac/policy.csv")

	v.SetDefault("local_users_path", "/etc/codespace-operator/auth/local-users.yaml")
	v.SetDefault("auth_config_path", "/etc/codespace-operator/auth/auth.yaml")
}

func (c *ServerConfig) BuildAuthConfig() (*auth.AuthConfig, error) {
	return auth.LoadAuthConfigWithEnv("AUTH", c.AuthConfigPath)
}

// Alternative: if you want to be more explicit about the steps
func (c *ServerConfig) BuildAuthConfigExplicit() (*auth.AuthConfig, error) {
	// Set up viper with AUTH_ environment variables
	authViper := auth.SetupAuthViper("AUTH")

	// Load file if specified
	if c.AuthConfigPath != "" {
		if _, err := os.Stat(c.AuthConfigPath); err == nil {
			authViper.SetConfigFile(c.AuthConfigPath)
			if err := authViper.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("read auth config file: %w", err)
			}
		}
	}
	logger.Info("Auth configuration loaded", "source", c.AuthConfigPath)
	// Convert to auth config
	return auth.LoadAuthConfigFromViper(authViper)
}
