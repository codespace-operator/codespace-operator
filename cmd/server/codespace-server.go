package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

//go:embed all:static
var staticFS embed.FS

var (
	gvr = schema.GroupVersionResource{
		Group:    codespacev1.GroupVersion.Group,
		Version:  codespacev1.GroupVersion.Version,
		Resource: "sessions",
	}
)

type serverDeps struct {
	typed  client.Client
	dyn    dynamic.Interface
	scheme *runtime.Scheme
	config *config.ServerConfig
	rbac   *RBAC
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "codespace-server",
		Short: "Codespace Operator Web Server",
		Long:  `A web server that provides a REST API and UI for managing Codespace sessions.`,
		Run:   runServer,
	}

	rootCmd.Flags().IntP("port", "p", 8080, "Server port")
	rootCmd.Flags().String("host", "", "Server host (empty for all interfaces)")
	rootCmd.Flags().String("allow-origin", "", "CORS allow origin")
	rootCmd.Flags().Bool("debug", false, "Enable debug logging")
	rootCmd.Flags().Float32("kube-qps", 50.0, "Kubernetes client QPS limit")
	rootCmd.Flags().Int("kube-burst", 100, "Kubernetes client burst limit")

	// OIDC + feature flags
	rootCmd.Flags().String("oidc-issuer", "", "OIDC issuer URL (e.g., https://dev-xxxx.okta.com)")
	rootCmd.Flags().String("oidc-client-id", "", "OIDC client ID")
	rootCmd.Flags().String("oidc-client-secret", "", "OIDC client secret")
	rootCmd.Flags().String("oidc-redirect-url", "", "OIDC redirect URL (https://host/auth/callback)")
	rootCmd.Flags().StringSlice("oidc-scopes", []string{}, "OIDC scopes (default: openid profile email)")
	rootCmd.Flags().Bool("enable-bootstrap-login", false, "Enable legacy bootstrap login (dev only)")
	rootCmd.Flags().Bool("allow-token-param", false, "Allow ?access_token=... on URLs (NOT recommended)")
	rootCmd.Flags().Int("session-ttl-minutes", 60, "Session cookie TTL in minutes")
	rootCmd.Flags().String("session-cookie-name", "", "Override session cookie name")

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// CLI overrides
	setString := func(name *string, flag string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetString(flag)
			if v != "" {
				*name = v
			}
		}
	}
	setBool := func(name *bool, flag string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetBool(flag)
			*name = v
		}
	}
	setInt := func(name *int, flag string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetInt(flag)
			*name = v
		}
	}

	setString(&cfg.AllowOrigin, "allow-origin")
	setBool(&cfg.Debug, "debug")
	if cmd.Flags().Changed("kube-qps") {
		cfg.KubeQPS, _ = cmd.Flags().GetFloat32("kube-qps")
	}
	if cmd.Flags().Changed("kube-burst") {
		cfg.KubeBurst, _ = cmd.Flags().GetInt("kube-burst")
	}
	setString(&cfg.OIDCIssuerURL, "oidc-issuer")
	setString(&cfg.OIDCClientID, "oidc-client-id")
	setString(&cfg.OIDCClientSecret, "oidc-client-secret")
	setString(&cfg.OIDCRedirectURL, "oidc-redirect-url")
	if cmd.Flags().Changed("oidc-scopes") {
		cfg.OIDCScopes, _ = cmd.Flags().GetStringSlice("oidc-scopes")
	}
	setBool(&cfg.EnableBootstrapLogin, "enable-bootstrap-login")
	setBool(&cfg.AllowTokenParam, "allow-token-param")
	setInt(&cfg.SessionTTLMinutes, "session-ttl-minutes")
	setString(&cfg.SessionCookieName, "session-cookie-name")

	if cfg.Debug {
		log.Printf("Configuration: %+v", cfg)
	}

	k8sCfg, err := helpers.BuildKubeConfig()
	if err != nil {
		log.Fatalf("Kubernetes config: %v", err)
	}
	k8sCfg.Timeout = 30 * time.Second
	k8sCfg.QPS = cfg.KubeQPS
	k8sCfg.Burst = cfg.KubeBurst

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Add corev1 scheme: %v", err)
	}
	if err := codespacev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Add scheme: %v", err)
	}

	typed, err := client.New(k8sCfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Typed client: %v", err)
	}
	dyn, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		log.Fatalf("Dynamic client: %v", err)
	}

	rbac, err := NewRBACFromEnv(context.Background(), log.Default())
	if err != nil {
		log.Fatalf("RBAC init failed: %v", err)
	}

	deps := &serverDeps{
		typed:  typed,
		dyn:    dyn,
		scheme: scheme,
		config: cfg,
		rbac:   rbac,
	}

	mux := setupHandlers(deps)
	registerAuthHandlers(mux, deps)

	var handler http.Handler = mux
	handler = withCORS(cfg.AllowOrigin, handler)
	handler = requireAPIToken(&configLike{
		JWTSecret:         cfg.JWTSecret,
		SessionCookieName: cfg.SessionCookieName,
		AllowTokenParam:   cfg.AllowTokenParam,
	}, handler)

	log.Printf("Listening on %s", cfg.GetAddr())
	if err := http.ListenAndServe(cfg.GetAddr(), handler); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
