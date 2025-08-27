package main

import (
	"crypto/tls"
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
	controller "github.com/codespace-operator/codespace-operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(codespacev1.AddToScheme(scheme))
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "session-controller",
		Short: "Codespace Operator Session Controller",
		Long:  `The controller component that manages Codespace Session resources in Kubernetes.`,
		Run:   runController,
	}

	// Add flags
	rootCmd.Flags().String("config", "", "Path to config file or directory (highest precedence)")
	rootCmd.Flags().String("metrics-bind-address", "0",
		"The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable.")
	rootCmd.Flags().String("health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	rootCmd.Flags().Bool("leader-elect", false,
		"Enable leader election for controller session-controller.")
	rootCmd.Flags().Bool("metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS.")
	rootCmd.Flags().String("webhook-cert-path", "",
		"The directory that contains the webhook certificate.")
	rootCmd.Flags().String("webhook-cert-name", "tls.crt",
		"The name of the webhook certificate file.")
	rootCmd.Flags().String("webhook-cert-key", "tls.key",
		"The name of the webhook key file.")
	rootCmd.Flags().String("metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	rootCmd.Flags().String("metrics-cert-name", "tls.crt",
		"The name of the metrics server certificate file.")
	rootCmd.Flags().String("metrics-cert-key", "tls.key",
		"The name of the metrics server key file.")
	rootCmd.Flags().Bool("enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	rootCmd.Flags().String("session-name-prefix", "cs-",
		"Prefix for generated session resource names")
	rootCmd.Flags().String("field-owner", "codespace-operator",
		"Field manager name for server-side apply operations")
	rootCmd.Flags().Bool("debug", false, "Enable debug logging")
	rootCmd.Flags().String("log-level", "info", "Log level (debug, info, warn, error)")
	// Add zap flags to a separate FlagSet that we can bind
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	opts := zap.Options{Development: true}
	opts.BindFlags(fs)

	// Add the zap flags to cobra by iterating through them
	fs.VisitAll(func(f *flag.Flag) {
		rootCmd.Flags().AddGoFlag(f)
	})

	if err := rootCmd.Execute(); err != nil {
		setupLog.Error(err, "Command execution failed")
		os.Exit(1)
	}
}

func runController(cmd *cobra.Command, args []string) {
	// Load configuration
	// Highest precedence: --config (file or dir)
	if cmd.Flags().Changed("config") {
		p, _ := cmd.Flags().GetString("config")
		if strings.TrimSpace(p) != "" {
			// make it visible to config.LoadControllerConfig/setupViper
			_ = os.Setenv("CODESPACE_CONTROLLER_CONFIG_DEFAULT_PATH", p)
		}
	}

	cfg, err := config.LoadControllerConfig()

	if err != nil {
		setupLog.Error(err, "Failed to load configuration")
		os.Exit(1)
	}

	// Override config with command line flags
	if cmd.Flags().Changed("metrics-bind-address") {
		addr, _ := cmd.Flags().GetString("metrics-bind-address")
		cfg.MetricsAddr = addr
	}
	if cmd.Flags().Changed("health-probe-bind-address") {
		addr, _ := cmd.Flags().GetString("health-probe-bind-address")
		cfg.ProbeAddr = addr
	}
	if cmd.Flags().Changed("leader-elect") {
		enable, _ := cmd.Flags().GetBool("leader-elect")
		cfg.EnableLeaderElection = enable
	}
	if cmd.Flags().Changed("metrics-secure") {
		secure, _ := cmd.Flags().GetBool("metrics-secure")
		cfg.SecureMetrics = secure
	}
	if cmd.Flags().Changed("webhook-cert-path") {
		path, _ := cmd.Flags().GetString("webhook-cert-path")
		cfg.WebhookCertPath = path
	}
	if cmd.Flags().Changed("webhook-cert-name") {
		name, _ := cmd.Flags().GetString("webhook-cert-name")
		cfg.WebhookCertName = name
	}
	if cmd.Flags().Changed("webhook-cert-key") {
		key, _ := cmd.Flags().GetString("webhook-cert-key")
		cfg.WebhookCertKey = key
	}
	if cmd.Flags().Changed("metrics-cert-path") {
		path, _ := cmd.Flags().GetString("metrics-cert-path")
		cfg.MetricsCertPath = path
	}
	if cmd.Flags().Changed("metrics-cert-name") {
		name, _ := cmd.Flags().GetString("metrics-cert-name")
		cfg.MetricsCertName = name
	}
	if cmd.Flags().Changed("metrics-cert-key") {
		key, _ := cmd.Flags().GetString("metrics-cert-key")
		cfg.MetricsCertKey = key
	}
	if cmd.Flags().Changed("enable-http2") {
		enable, _ := cmd.Flags().GetBool("enable-http2")
		cfg.EnableHTTP2 = enable
	}
	if cmd.Flags().Changed("session-name-prefix") {
		prefix, _ := cmd.Flags().GetString("session-name-prefix")
		cfg.SessionNamePrefix = prefix
	}
	if cmd.Flags().Changed("field-owner") {
		owner, _ := cmd.Flags().GetString("field-owner")
		cfg.FieldOwner = owner
	}
	if cmd.Flags().Changed("debug") {
		debug, _ := cmd.Flags().GetBool("debug")
		cfg.Debug = debug
	}

	// Setup logging - create new zap options and configure from flags
	opts := zap.Options{Development: cfg.Debug}

	// We need to bind the zap flags again to get their values
	// First, add pflag values to the Go flag package
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Only process flags that are zap-related
		if goFlag := flag.Lookup(f.Name); goFlag != nil {
			goFlag.Value.Set(f.Value.String())
		}
	})

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if cfg.Debug {
		setupLog.Info("Configuration loaded", "config", cfg)
	}

	// Configure TLS options
	var tlsOpts []func(*tls.Config)
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !cfg.EnableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create certificate watchers
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher
	webhookTLSOpts := tlsOpts

	if len(cfg.WebhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher",
			"path", cfg.WebhookCertPath,
			"cert", cfg.WebhookCertName,
			"key", cfg.WebhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(cfg.WebhookCertPath, cfg.WebhookCertName),
			filepath.Join(cfg.WebhookCertPath, cfg.WebhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	// Setup webhook server
	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Setup metrics server
	metricsServerOptions := metricsserver.Options{
		BindAddress:   cfg.MetricsAddr,
		SecureServing: cfg.SecureMetrics,
		TLSOpts:       tlsOpts,
	}

	if cfg.SecureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(cfg.MetricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher",
			"path", cfg.MetricsCertPath,
			"cert", cfg.MetricsCertName,
			"key", cfg.MetricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(cfg.MetricsCertPath, cfg.MetricsCertName),
			filepath.Join(cfg.MetricsCertPath, cfg.MetricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize metrics certificate watcher")
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: cfg.ProbeAddr,
		LeaderElection:         cfg.EnableLeaderElection,
		LeaderElectionID:       cfg.LeaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start session-controller")
		os.Exit(1)
	}

	// Setup controller with configuration
	if err := (&controller.SessionReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Session")
		os.Exit(1)
	}

	// Add certificate watchers to manager
	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to session-controller")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "Unable to add metrics certificate watcher")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to session-controller")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "Unable to add webhook certificate watcher")
			os.Exit(1)
		}
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	// Update global configuration for controller
	os.Setenv("SESSION_NAME_PREFIX", cfg.SessionNamePrefix)
	os.Setenv("FIELD_OWNER", cfg.FieldOwner)

	setupLog.Info("Starting session-controller")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running session-controller")
		os.Exit(1)
	}
}
