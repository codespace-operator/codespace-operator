package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"time"

	auth "github.com/codespace-operator/common/auth/pkg/auth"
	"github.com/codespace-operator/common/common/pkg/common"
	rbac "github.com/codespace-operator/common/rbac/pkg/rbac"
	"github.com/spf13/viper"
	"github.com/swaggo/swag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

// available for all
var logger = common.GetLogger()

// embeds file next to main.go
//
//go:embed all:static
var staticFS embed.FS

var (
	gvr = schema.GroupVersionResource{
		Group:    codespacev1.GroupVersion.Group,
		Version:  codespacev1.GroupVersion.Version,
		Resource: "sessions",
	}
)

type ErrorResponse struct {
	Error string `json:"error" example:"Invalid request"`
}
type serverDeps struct {
	client      client.Client
	dyn         dynamic.Interface
	scheme      *runtime.Scheme
	config      *ServerConfig
	rbac        rbac.RBACInterface
	rbacMw      *rbac.Middleware
	authManager *auth.AuthManager
	authMw      *auth.Middleware
	authCfg     *auth.AuthConfig
	instanceID  string
	manager     common.AnchorMeta
	logger      *slog.Logger
}

// ServerVersionInfo contains server version and build information
type ServerVersionInfo struct {
	Version   string `json:"version,omitempty"`
	GitCommit string `json:"gitCommit,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
}

// RunServer starts the server with proper dependency injection
func RunServer(cfg *ServerConfig, args []string, v *viper.Viper) {
	// Initialize logging first
	logConfig := common.LogConfig{
		Level:      common.LogLevel(cfg.LogLevel),
		TimeFormat: time.Kitchen,
		AddSource:  cfg.LogLevel == "debug",
		Writer:     os.Stderr,
		NoColor:    false, // Enable color by default
	}

	if os.Getenv("ENVIRONMENT") == "production" {
		logConfig = common.LogConfig{
			Level:      common.LogLevel(cfg.LogLevel),
			TimeFormat: time.RFC3339,
			AddSource:  false,
			Writer:     os.Stderr,
			NoColor:    true, // Disable color in production
		}
	}

	logger := common.InitializeLogging(logConfig)
	logger = common.LoggerWithComponent(logger, "server")

	if cfg.LogLevel == "debug" {
		logger.Info("Configuration loaded", "config", cfg)
	}

	// Setup Kubernetes clients
	k8sCfg, err := common.BuildKubeConfig()
	if err != nil {
		logger.Error("Failed to build Kubernetes config", "error", err)
		os.Exit(1)
	}
	k8sCfg.Timeout = 30 * time.Second
	k8sCfg.QPS = cfg.KubeQPS
	k8sCfg.Burst = cfg.KubeBurst

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Error("Failed to add corev1 scheme", "error", err)
		os.Exit(1)
	}
	if err := codespacev1.AddToScheme(scheme); err != nil {
		logger.Error("Failed to add codespace scheme", "error", err)
		os.Exit(1)
	}

	k8sClient, err := client.New(k8sCfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error("Failed to create Kubernetes client", "error", err)
		os.Exit(1)
	}

	dynClient, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		logger.Error("Failed to create dynamic client", "error", err)
		os.Exit(1)
	}

	// Test Kubernetes connectivity
	if err := testKubernetesConnection(k8sClient); err != nil {
		logger.Error("Kubernetes connection test failed", "error", err)
		os.Exit(1)
	}

	// Setup RBAC with proper interface
	rbacSystem, err := setupRBAC(cfg, logger)
	if err != nil {
		logger.Error("RBAC initialization failed", "error", err)
		os.Exit(1)
	}

	authCfg, err := cfg.BuildAuthConfig(v)
	if err != nil {
		logger.Error("Authentication config invalid", "error", err)
		os.Exit(1)
	}

	if cfg.DeveloperMode {
		// override SameSite if you want to relax for dev
		authCfg.SameSiteMode = http.SameSiteLaxMode
	}

	authManager, err := auth.NewAuthManager(authCfg, common.LoggerWithComponent(logger, "auth"))
	if err != nil {
		logger.Error("Authentication setup failed", "error", err)
		os.Exit(1)
	}

	// Get instance and manager identity
	manager, instanceID, err := ensureInstallationID(context.Background(), k8sClient, cfg)
	if err != nil {
		logger.Error("Failed to ensure server installation ID", "error", err)
		logger.Warn("This instance will not guarantee correct instance separation")
	} else {
		logger.Info("Server installation ID ensured", "instanceID", instanceID)
		logger.Info("Detected manager identity",
			"type", manager.Type,
			"name", manager.Name,
			"namespace", manager.Namespace)
	}
	rbacMW := rbac.NewMiddleware(rbacSystem, ExtractFromAuth, logger)
	// Create server dependencies with proper interfaces
	deps := &serverDeps{
		client:      k8sClient,
		dyn:         dynClient,
		scheme:      scheme,
		config:      cfg,
		rbac:        rbacSystem,
		rbacMw:      rbacMW,
		authManager: authManager,
		authCfg:     authCfg,
		authMw:      auth.NewMiddleware(authManager, logger),
		instanceID:  instanceID,
		manager:     manager,
		logger:      logger,
	}

	// Setup HTTP handlers
	mux := setupHandlers(deps)

	// Build middleware chain with proper interfaces
	var handler http.Handler = mux
	handler = corsMiddleware(cfg.AllowOrigin)(handler)
	handler = requestLoggingMiddleware(logger)(handler)
	handler = deps.authMw.AuthGate(handler)
	handler = securityHeadersMiddleware()(handler)

	logger.Info("Codespace Server starting", "address", cfg.GetAddr())

	// Report if running cluster-scoped
	if cfg.ClusterScope {
		logger.Info("Running in cluster-scoped mode")
	} else {
		logger.Info("Running in instance-scoped mode", "instanceID", instanceID)
	}

	// Start server
	server := &http.Server{
		Addr:         cfg.GetAddr(),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	if err := server.ListenAndServe(); err != nil {
		logger.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}

// setupRBAC initializes the RBAC system with proper interface
func setupRBAC(cfg *ServerConfig, logger *slog.Logger) (rbac.RBACInterface, error) {
	rbacLogger := common.LoggerWithComponent(logger, "rbac")

	config := rbac.RBACConfig{
		ModelPath:  cfg.RBACModelPath,
		PolicyPath: cfg.RBACPolicyPath,
		Logger:     rbacLogger,
	}

	// Set defaults if not provided
	if config.ModelPath == "" {
		config.ModelPath = "/etc/codespace-operator/rbac/model.conf"
	}
	if config.PolicyPath == "" {
		config.PolicyPath = "/etc/codespace-operator/rbac/policy.csv"
	}

	rbacSystem, err := rbac.NewRBAC(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RBAC: %w", err)
	}

	rbacLogger.Info("RBAC system initialized",
		"modelPath", config.ModelPath,
		"policyPath", config.PolicyPath)

	return rbacSystem, nil
}

// setupHandlers creates the HTTP handler with proper interface integration
func setupHandlers(deps *serverDeps) *http.ServeMux {
	mux := http.NewServeMux()
	h := newHandlers(deps)

	// === Health and Status Endpoints ===
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)

	// === Session Operations with RBAC ===
	mux.HandleFunc("/api/v1/server/sessions", h.wrapWithAuth(h.handleSessionOperations))
	mux.HandleFunc("/api/v1/server/sessions/adopt", h.wrapWithRBAC("*", "admin", "*", h.handleAdoptSession))
	mux.HandleFunc("/api/v1/server/sessions/", h.wrapWithAuth(h.handleSessionOperationsWithPath))

	// === Session Streaming ===
	mux.HandleFunc("/api/v1/stream/sessions", h.wrapWithAuth(h.handleStreamSessions))

	// === User and System Introspection ===
	mux.HandleFunc("/api/v1/me", h.wrapWithAuth(h.handleMe))
	mux.HandleFunc("/api/v1/introspect", h.wrapWithAuth(h.handleIntrospect))
	mux.HandleFunc("/api/v1/introspect/user", h.wrapWithAuth(h.handleUserIntrospect))
	mux.HandleFunc("/api/v1/introspect/server", h.wrapWithAuth(h.handleServerIntrospect))
	mux.HandleFunc("/api/v1/user/permissions", h.wrapWithAuth(h.handleUserPermissions))

	// === Admin Endpoints ===
	mux.HandleFunc("/api/v1/admin/users", h.wrapWithRBAC("*", "admin", "*", h.handleAdminUsers))
	mux.HandleFunc("/api/v1/admin/rbac/reload", h.wrapWithRBAC("*", "admin", "*", h.handleRBACReload))
	mux.HandleFunc("/api/v1/admin/system/info", h.wrapWithRBAC("*", "admin", "*", h.handleSystemInfo))

	// === Authentication Endpoints ===
	registerAuthHandlers(mux, h)

	// === OpenAPI Documentation ===
	setupOpenAPIHandlers(mux, h)

	// === Static UI ===
	setupStaticUI(mux)

	return mux
}

// setupOpenAPIHandlers conditionally sets up OpenAPI based on build
func setupOpenAPIHandlers(mux *http.ServeMux, h *handlers) {
	// Check if docs are available (swag will be empty if not built with docs)
	spec, err := swag.ReadDoc()
	if err != nil || spec == "" {
		logger.Debug("OpenAPI documentation not available - build with -tags docs to enable")
		return
	}

	logger.Info("OpenAPI documentation enabled")

	// OpenAPI spec endpoint (admin-protected)
	mux.HandleFunc("/api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.deps.rbacMw.MustCan(w, r, "*", "admin", "*"); !ok {
			return
		}
		h.handleOpenAPISpec(w, r)
	})

	// Swagger UI (admin-protected)
	mux.HandleFunc("/api/docs/", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.deps.rbacMw.MustCan(w, r, "*", "admin", "*"); !ok {
			return
		}
		h.handleSwaggerUI(w, r)
	})

	// Redirect /api/docs -> /api/docs/
	mux.HandleFunc("/api/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/docs/", http.StatusMovedPermanently)
	})
}

// handleSwaggerUI serves the Swagger UI
func (h *handlers) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	// Simple embedded Swagger UI HTML
	html := `<!DOCTYPE html>
<html>
<head>
  <title>Codespace API Documentation</title>
  <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.15.5/swagger-ui.min.css" />
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin:0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.15.5/swagger-ui-bundle.min.js"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.15.5/swagger-ui-standalone-preset.min.js"></script>
  <script>
    window.onload = function() {
      const ui = SwaggerUIBundle({
        url: '/api/openapi.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout",
        docExpansion: "list",
        tagsSorter: "alpha",
        operationsSorter: "alpha"
      });
    };
  </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(html))
}

// swagDocAvailable checks if swagger docs are available
func swagDocAvailable() bool {
	spec, err := swag.ReadDoc()
	return err == nil && spec != ""
}

// setupStaticUI serves the SPA/static files
func setupStaticUI(mux *http.ServeMux) {
	ui, err := fsSub(staticFS, "static")
	if err != nil {
		logger.Error("Failed to create static file system", "err", err)
	}
	files := http.FileServer(ui)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Static request", "r.URL.Path", r.URL.Path)

		// Serve actual files (with extensions) directly
		if path.Ext(r.URL.Path) != "" && r.URL.Path != "/" {
			files.ServeHTTP(w, r)
			return
		}

		// For routes without extensions, serve index.html (SPA behavior)
		index, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		w.Write(index)
	})
}
func (h *handlers) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec, err := swag.ReadDoc()
	if err != nil || spec == "" {
		http.Error(w, "OpenAPI spec not available", http.StatusNotFound)
		if err != nil {
			logger.Error("Error reading OpenAPI spec:", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	if r.URL.Query().Get("pretty") == "1" {
		var specMap map[string]interface{}
		if json.Unmarshal([]byte(spec), &specMap) == nil {
			if prettyJSON, err := json.MarshalIndent(specMap, "", "  "); err == nil {
				w.Write(prettyJSON)
				return
			}
		}
	}
	w.Write([]byte(spec))
}

func corsMiddleware(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func requestLoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Generate or extract request ID
			reqID := r.Header.Get("X-Request-Id")
			if reqID == "" {
				reqID = common.RandB64(6)
			}
			w.Header().Set("X-Request-Id", reqID)

			// Create request-scoped logger
			reqLogger := common.LoggerWithRequestID(logger, reqID)
			r = r.WithContext(common.WithLogger(r.Context(), reqLogger))

			// Wrap response writer to capture status code
			rw := &common.ResponseWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)

			// Extract user info for logging
			user := "-"
			if cl := auth.FromContext(r); cl != nil && cl.Sub != "" {
				user = cl.Sub
			}

			// Log the request
			reqLogger.Info("http",
				"method", r.Method,
				"path", r.URL.RequestURI(),
				"status", rw.StatusCode(),
				"bytes", rw.BytesWritten(),
				"duration", time.Since(start),
				"ip", clientIP(r),
				"user_agent", r.UserAgent(),
				"user", user,
			)
		})
	}
}

func securityHeadersMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Only add HSTS if we're on HTTPS
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}
