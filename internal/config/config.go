package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// ServerConfig holds all configuration for the codespace server
type ServerConfig struct {
	// Server settings
	Port         int    `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`

	// CORS settings
	AllowOrigin string `mapstructure:"allow_origin"`

	// Kubernetes settings
	KubeQPS   float32 `mapstructure:"kube_qps"`
	KubeBurst int     `mapstructure:"kube_burst"`
	
	// Development/Debug settings
	Debug bool `mapstructure:"debug"`
}

// ControllerConfig holds configuration for the session controller
type ControllerConfig struct {
	// Controller settings
	MetricsAddr             string `mapstructure:"metrics_addr"`
	ProbeAddr               string `mapstructure:"probe_addr"`
	EnableLeaderElection    bool   `mapstructure:"enable_leader_election"`
	LeaderElectionID        string `mapstructure:"leader_election_id"`
	
	// Certificate settings
	MetricsCertPath  string `mapstructure:"metrics_cert_path"`
	MetricsCertName  string `mapstructure:"metrics_cert_name"`
	MetricsCertKey   string `mapstructure:"metrics_cert_key"`
	WebhookCertPath  string `mapstructure:"webhook_cert_path"`
	WebhookCertName  string `mapstructure:"webhook_cert_name"`
	WebhookCertKey   string `mapstructure:"webhook_cert_key"`
	
	// Security settings
	SecureMetrics bool `mapstructure:"secure_metrics"`
	EnableHTTP2   bool `mapstructure:"enable_http2"`
	
	// Session settings
	SessionNamePrefix string `mapstructure:"session_name_prefix"`
	FieldOwner        string `mapstructure:"field_owner"`
	
	// Development/Debug settings
	Debug bool `mapstructure:"debug"`
}

// LoadServerConfig loads configuration for the codespace server
func LoadServerConfig() (*ServerConfig, error) {
	v := viper.New()
	
	// Set defaults
	v.SetDefault("port", 8080)
	v.SetDefault("host", "")
	v.SetDefault("read_timeout", 5)
	v.SetDefault("write_timeout", 10)
	v.SetDefault("allow_origin", "")
	v.SetDefault("kube_qps", 50.0)
	v.SetDefault("kube_burst", 100)
	v.SetDefault("debug", false)
	
	// Configure viper
	setupViper(v, "CODESPACE_SERVER")
	
	var config ServerConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %w", err)
	}
	
	return &config, nil
}

// LoadControllerConfig loads configuration for the session controller
func LoadControllerConfig() (*ControllerConfig, error) {
	v := viper.New()
	
	// Set defaults
	v.SetDefault("metrics_addr", "0")
	v.SetDefault("probe_addr", ":8081")
	v.SetDefault("enable_leader_election", false)
	v.SetDefault("leader_election_id", "a51c5837.codespace.dev")
	v.SetDefault("metrics_cert_path", "")
	v.SetDefault("metrics_cert_name", "tls.crt")
	v.SetDefault("metrics_cert_key", "tls.key")
	v.SetDefault("webhook_cert_path", "")
	v.SetDefault("webhook_cert_name", "tls.crt")
	v.SetDefault("webhook_cert_key", "tls.key")
	v.SetDefault("secure_metrics", true)
	v.SetDefault("enable_http2", false)
	v.SetDefault("session_name_prefix", "cs-")
	v.SetDefault("field_owner", "codespace-operator")
	v.SetDefault("debug", false)
	
	// Configure viper
	setupViper(v, "CODESPACE_CONTROLLER")
	
	var config ControllerConfig
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal controller config: %w", err)
	}
	
	return &config, nil
}

// setupViper configures common viper settings
func setupViper(v *viper.Viper, envPrefix string) {
	// Environment variables
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	
	// Config file support
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/codespace-operator/")
	v.AddConfigPath("$HOME/.codespace-operator/")
	
	// Try to read config file (ignore if not found)
	_ = v.ReadInConfig()
}

// GetAddr returns the full address string for binding
func (c *ServerConfig) GetAddr() string {
	if c.Host == "" {
		return fmt.Sprintf(":%d", c.Port)
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}