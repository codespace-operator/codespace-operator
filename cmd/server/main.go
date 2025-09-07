//go:generate swag init -g codespace_server.go -o ../../gen/api --parseDependency --parseInternal -ot json,yaml
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
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadConfigWithOverrides(cmd)
			runServer(cfg, args)
		},
	}

	// Core server flags
	rootCmd.Flags().String("config", "", "Path to config directory or file (highest precedence among files)")
	rootCmd.Flags().IntP("port", "p", 8080, "Server port")
	rootCmd.Flags().String("host", "", "Server host (empty for all interfaces)")
	rootCmd.Flags().String("allow-origin", "", "CORS allow origin")
	rootCmd.Flags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().Float32("kube-qps", 50.0, "Kubernetes client QPS")
	rootCmd.Flags().Int("kube-burst", 100, "Kubernetes client burst")
	rootCmd.Flags().Bool("cluster-scope", false, "Cluster-scoped mode")
	rootCmd.Flags().String("app-name", DEFAULT_APP_NAME, "Application name")
	rootCmd.Flags().Bool("developer-mode", false, "Developer mode (relaxes cookies)")

	// RBAC files
	rootCmd.Flags().String("rbac-model-path", "", "Casbin model.conf")
	rootCmd.Flags().String("rbac-policy-path", "", "Casbin policy.csv")

	// Local users file
	rootCmd.Flags().String("local-users-path", "", "Path to local users file")

	// Auth node pointers (the canonical way to configure auth)
	rootCmd.Flags().String("auth-config-file", "", "Path to auth configuration YAML file")

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
	// Honor --config for where viper should look
	if cmd.Flags().Changed("config") {
		if p, _ := cmd.Flags().GetString("config"); strings.TrimSpace(p) != "" {
			os.Setenv("CODESPACE_SERVER_CONFIG_DEFAULT_PATH", p)
		}
	}

	cfg, _, err := server.LoadServerConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "err", err)
	}

	// primitive typed override helpers
	ovStr := func(p *string, f string) {
		if cmd.Flags().Changed(f) {
			*p, _ = cmd.Flags().GetString(f)
		}
	}
	ovInt := func(p *int, f string) {
		if cmd.Flags().Changed(f) {
			*p, _ = cmd.Flags().GetInt(f)
		}
	}
	ovF32 := func(p *float32, f string) {
		if cmd.Flags().Changed(f) {
			*p, _ = cmd.Flags().GetFloat32(f)
		}
	}
	ovBool := func(p *bool, f string) {
		if cmd.Flags().Changed(f) {
			*p, _ = cmd.Flags().GetBool(f)
		}
	}

	// server
	ovBool(&cfg.ClusterScope, "cluster-scope")
	ovInt(&cfg.Port, "port")
	ovStr(&cfg.Host, "host")
	ovStr(&cfg.AllowOrigin, "allow-origin")
	ovStr(&cfg.LogLevel, "log-level")
	ovStr(&cfg.AppName, "app-name")
	ovBool(&cfg.DeveloperMode, "developer-mode")
	ovF32(&cfg.KubeQPS, "kube-qps")
	ovInt(&cfg.KubeBurst, "kube-burst")

	// rbac
	ovStr(&cfg.RBACModelPath, "rbac-model-path")
	ovStr(&cfg.RBACPolicyPath, "rbac-policy-path")

	// auth
	ovStr(&cfg.AuthConfigPath, "auth-config-file")
	return cfg
}
