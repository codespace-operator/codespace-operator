package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/swaggo/swag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

// embeds file next to main.go
//
//go:embed all:static
var staticFS embed.FS

// ! todo -> set the label/annotations system clearly
// set upon config load -> optional explicit labels for the controller (not server!)
var APP_NAME string

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
	client     client.Client
	dyn        dynamic.Interface
	scheme     *runtime.Scheme
	config     *ServerConfig
	rbac       *RBAC
	localUsers *LocalUsers
	instanceID string
	manager    ManagerMeta
}

func RunServer(cfg *ServerConfig, args []string) {
	APP_NAME = cfg.APP_NAME
	configureLogger(cfg.LogLevel)

	if cfg.LogLevel == "debug" {
		logger.Info("Configuration loaded", "config", cfg)
	}
	// Setup Kubernetes clients
	k8sCfg, err := helpers.BuildKubeConfig()
	if err != nil {
		logger.Fatal("Kubernetes config", "err", err)
	}
	k8sCfg.Timeout = 30 * time.Second
	k8sCfg.QPS = cfg.KubeQPS
	k8sCfg.Burst = cfg.KubeBurst

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		logger.Fatal("Add corev1 scheme", "err", err)
	}
	if err := codespacev1.AddToScheme(scheme); err != nil {
		logger.Fatal("Add codespace scheme", "err", err)
	}

	client, err := client.New(k8sCfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Fatal("client", "err", err)
	}
	dyn, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		logger.Fatal("Dynamic client", "err", err)
	}

	// Test Kubernetes connectivity
	if err := helpers.TestKubernetesConnection(client); err != nil {
		logger.Fatal("Kubernetes connection test failed", "err", err)
	}

	// Setup RBAC
	rbac, err := setupRBAC(cfg)
	if err != nil {
		logger.Fatal("RBAC init failed", "err", err)
	}

	// Setup local users
	users, err := loadLocalUsers(cfg.LocalUsersPath)
	if err != nil {
		logger.Fatal("Local users load failed", "err", err)
	}
	instanceID, err := ensureInstallationID(context.Background(), client)
	logger.Info(fmt.Sprintf("Ensured server installation ID: %s", instanceID), "instanceID", instanceID)
	if err != nil {
		logger.Error("failed to ensure server id", "err", err)
	}
	manager := getSelfManagerMeta(context.Background(), client)
	logger.Info("Detected manager", "kind", manager.Kind, "name", manager.Name, "namespace", manager.Namespace)
	// Create server dependencies
	deps := &serverDeps{
		client:     client,
		dyn:        dyn,
		scheme:     scheme,
		config:     cfg,
		rbac:       rbac,
		localUsers: users,
		instanceID: instanceID,
		manager:    manager,
	}

	// Setup HTTP handlers
	mux := setupHandlers(deps)

	// Build middleware chain: security → CORS → logging → auth → handlers
	var handler http.Handler = mux
	handler = corsMiddleware(cfg.AllowOrigin)(handler)
	handler = logRequests(handler)
	handler = authGate(&authConfigLike{
		JWTSecret:         cfg.JWTSecret,
		SessionCookieName: cfg.SessionCookieName,
		AllowTokenParam:   cfg.AllowTokenParam,
	}, handler)

	logger.Printf("🚀 Codespace Server starting on %s", cfg.GetAddr())
	if swagDocAvailable() {
		logger.Printf("📚 API Documentation available at http://%s/api/docs/", cfg.GetAddr())
	}

	// Report if running cluster-scoped
	if cfg.ClusterScope {
		logger.Info(" ----------------- Running in cluster-scoped mode ----------------- ")
	} else {
		logger.Info(" --------------- Running in instance-id scoped mode --------------- ")
	}

	if err := http.ListenAndServe(cfg.GetAddr(), handler); err != nil {
		logger.Fatal("ListenAndServe", "err", err)
	}
}

// setupRBAC initializes the RBAC system
func setupRBAC(cfg *ServerConfig) (*RBAC, error) {
	// Push explicit RBAC paths into environment if provided
	if cfg.RBACModelPath != "" {
		os.Setenv(envModelPath, cfg.RBACModelPath)
	}
	if cfg.RBACPolicyPath != "" {
		os.Setenv(envPolicyPath, cfg.RBACPolicyPath)
	}

	rbac, err := NewRBACFromEnv(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RBAC: %w", err)
	}

	logger.Info("RBAC system initialized",
		"modelPath", rbac.modelPath,
		"policyPath", rbac.policyPath,
	)

	return rbac, nil
}

// setupHandlers creates the HTTP handler with comprehensive RBAC
func setupHandlers(deps *serverDeps) *http.ServeMux {
	mux := http.NewServeMux()
	h := newHandlers(deps)

	// === Health and Status Endpoints (No Auth Required) ===
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)

	// === Authentication Endpoints (Handled separately) ===
	registerAuthHandlers(mux, deps)

	// === Session Operations ===
	mux.HandleFunc("/api/v1/server/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.handleListSessions(w, r)
		case http.MethodPost:
			h.handleCreateSession(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// === Adopt Session ===
	mux.HandleFunc("/api/v1/server/sessions/adopt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			h.handleAdoptSession(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// Session CRUD with path parameters
	mux.HandleFunc("/api/v1/server/sessions/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/")
		parts := strings.Split(path, "/")

		if len(parts) < 2 {
			http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}[/operation]", http.StatusBadRequest)
			return
		}

		// Check if this is a scale operation
		if len(parts) == 3 && parts[2] == "scale" {
			h.handleScaleSession(w, r)
			return
		}

		// Regular CRUD operations on specific session
		switch r.Method {
		case http.MethodGet:
			h.handleGetSession(w, r)
		case http.MethodPut:
			h.handleUpdateSession(w, r)
		case http.MethodPatch:
			h.handleUpdateSession(w, r)
		case http.MethodDelete:
			h.handleDeleteSession(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// === Session Streaming ===
	mux.HandleFunc("/api/v1/stream/sessions", h.handleStreamSessions)

	// === User and system introspection ===
	mux.HandleFunc("/api/v1/me", h.handleMe)
	mux.HandleFunc("/api/v1/introspect", h.handleIntrospect)
	mux.HandleFunc("/api/v1/introspect/user", h.handleUserIntrospect)
	mux.HandleFunc("/api/v1/introspect/server", h.handleServerIntrospect)
	mux.HandleFunc("/api/v1/user/permissions", h.handleUserPermissions)

	// === Admin endpoints ===
	mux.HandleFunc("/api/v1/admin/users", h.handleAdminUsers)
	mux.HandleFunc("/api/v1/admin/rbac/reload", h.handleRBACReload)
	mux.HandleFunc("/api/v1/admin/system/info", h.handleSystemInfo)

	// === OpenAPI Documentation (if enabled) ===
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
		if _, ok := mustCan(h.deps, w, r, "*", "admin", "*"); !ok {
			return
		}
		h.handleOpenAPISpec(w, r)
	})

	// Swagger UI (admin-protected)
	mux.HandleFunc("/api/docs/", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := mustCan(h.deps, w, r, "*", "admin", "*"); !ok {
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
		logger.Fatal("Failed to create static file system", "err", err)
	}
	files := http.FileServer(ui)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG") == "true" {
			logger.Printf("Static request: %s", r.URL.Path)
		}

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
