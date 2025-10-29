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
	InitImage        string `mapstructure:"init_image"`
	InitUser         int64  `mapstructure:"init_user"`
	InitGroup        int64  `mapstructure:"init_group"`
	InitExtraEnv     map[string]string `mapstructure:"init_extra_env"`

	SessionNamePrefix string `mapstructure:"session_name_prefix"`
	FieldOwner        string `mapstructure:"field_owner"`
	ProfileStoreKind    string            `mapstructure:"profile_store_kind"`      // http | postgres | memory
	ProfileStoreBaseURL string            `mapstructure:"profile_store_base_url"`  // when kind=http, e.g. "http://codespace-server:8443"
	ProfileStoreToken   string            `mapstructure:"profile_store_token"`     // when kind=http
	ProfileStoreDSN     string            `mapstructure:"profile_store_dsn"`       // when kind=postgres

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

	v.SetDefault("auth_config_path", "")
	v.SetDefault("profile_store_kind", "http")
	v.SetDefault("profile_store_base_url", "")
	v.SetDefault("profile_store_token", "")
	v.SetDefault("profile_store_dsn", "")

	v.SetDefault("init_image", "alpine/git:2.45.2")
	v.SetDefault("init_user", 1000)
	v.SetDefault("init_group", 1000)
	v.SetDefault("init_extra_env", map[string]string{})


	v.SetDefault("debug", false)
	common.SetupViper(v, "CODESPACE_CONTROLLER", "controller-config")

	var cfg ControllerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal controller config: %w", err)
	}
	return &cfg, nil
}


