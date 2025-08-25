package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

var (
	//go:embed ui-dist/*
	uiFS embed.FS
)

type serverDeps struct {
	typed  client.Client
	dyn    dynamic.Interface
	scheme *runtime.Scheme
	config *config.ServerConfig
	rbac   *RBAC // Casbin-backed RBAC
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

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Command execution failed: %v", err)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cmd.Flags().Changed("port") {
		if port, _ := cmd.Flags().GetInt("port"); port != 0 {
			cfg.Port = port
		}
	}
	if cmd.Flags().Changed("host") {
		if host, _ := cmd.Flags().GetString("host"); host != "" {
			cfg.Host = host
		}
	}
	if cmd.Flags().Changed("allow-origin") {
		if origin, _ := cmd.Flags().GetString("allow-origin"); origin != "" {
			cfg.AllowOrigin = origin
		}
	}
	if cmd.Flags().Changed("debug") {
		cfg.Debug, _ = cmd.Flags().GetBool("debug")
	}
	if cmd.Flags().Changed("kube-qps") {
		cfg.KubeQPS, _ = cmd.Flags().GetFloat32("kube-qps")
	}
	if cmd.Flags().Changed("kube-burst") {
		cfg.KubeBurst, _ = cmd.Flags().GetInt("kube-burst")
	}

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

	// Initialize RBAC (Casbin) and start hot-reload watcher
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

	// Routes
	mux := http.NewServeMux()
	setupHandlers(mux, deps, uiFS)
	addr := cfg.GetAddr()

	// CORS + auth middleware
	var handler http.Handler = mux
	handler = withCORS(cfg.AllowOrigin, handler)
	handler = requireAPIToken([]byte(cfg.JWTSecret), handler)

	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

func setupHandlers(deps *serverDeps) *http.ServeMux {
	mux := http.NewServeMux()

	// Auth endpoints
	mux.HandleFunc("/api/v1/auth/login", handleLogin(deps.config))
	mux.Handle("/api/v1/auth/me", requireAPIToken([]byte(deps.config.JWTSecret), http.HandlerFunc(handleMe())))

	// Health endpoints
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz(deps))

	// API endpoints (protected by top-level middleware too)
	mux.HandleFunc("/api/v1/stream/sessions", handleStreamSessions(deps))
	mux.HandleFunc("/api/v1/sessions", handleSessions(deps))
	mux.HandleFunc("/api/v1/sessions/", handleSessionsWithPath(deps))
	mux.HandleFunc("/api/v1/namespaces/sessions", handleNamespacesWithSessions(deps))
	mux.HandleFunc("/api/v1/namespaces/writable", handleWritableNamespaces(deps))

	setupStaticUI(mux)
	if deps.config.Debug {
		mux.HandleFunc("/debug/static-files", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			err := fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "%s (dir: %v)\n", path, d.IsDir())
				return nil
			})
			if err != nil {
				fmt.Fprintf(w, "Error walking static files: %v\n", err)
			}
		})
	}
	return mux
}

// SPA/static serving with simple index fallback
func setupStaticUI(mux *http.ServeMux) {
	uiFS, err := fsSub(staticFS, "static")
	if err != nil {
		log.Fatalf("Failed to create static file system: %v", err)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG") == "true" {
			log.Printf("Static request: %s", r.URL.Path)
		}
		if path.Ext(r.URL.Path) != "" && r.URL.Path != "/" {
			http.FileServer(uiFS).ServeHTTP(w, r)
			return
		}
		indexContent, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		w.Write(indexContent)
	})
}
