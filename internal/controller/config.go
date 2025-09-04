package controller

import (
	"fmt"

	"github.com/codespace-operator/common/common/pkg/common"
	"github.com/spf13/viper"
)

// -----------------------------
// Structs (snake_case tags)
// -----------------------------

// ControllerConfig holds configuration for the session controller
type ControllerConfig struct {
	// Controller settings
	MetricsAddr          string `mapstructure:"metrics_addr"`
	ProbeAddr            string `mapstructure:"probe_addr"`
	EnableLeaderElection bool   `mapstructure:"enable_leader_election"`
	LeaderElectionID     string `mapstructure:"leader_election_id"`
	// Certificate settings
	MetricsCertPath string `mapstructure:"metrics_cert_path"`
	MetricsCertName string `mapstructure:"metrics_cert_name"`
	MetricsCertKey  string `mapstructure:"metrics_cert_key"`
	WebhookCertPath string `mapstructure:"webhook_cert_path"`
	WebhookCertName string `mapstructure:"webhook_cert_name"`
	WebhookCertKey  string `mapstructure:"webhook_cert_key"`

	// Security settings
	SecureMetrics bool `mapstructure:"secure_metrics"`
	EnableHTTP2   bool `mapstructure:"enable_http2"`

	// Session settings
	SessionNamePrefix string `mapstructure:"session_name_prefix"`
	FieldOwner        string `mapstructure:"field_owner"`

	// Logging
	Debug bool `mapstructure:"debug"`
}

// -----------------------------
// Loader entry points
// -----------------------------

// LoadControllerConfig reads controller-config.yaml + env (CODESPACE_CONTROLLER_*) into ControllerConfig.
func LoadControllerConfig() (*ControllerConfig, error) {
	v := viper.New()

	// Defaults (unchanged from previous)
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

	common.SetupViper(v, "CODESPACE_CONTROLLER", "controller-config")

	var cfg ControllerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal controller config: %w", err)
	}
	return &cfg, nil
}
