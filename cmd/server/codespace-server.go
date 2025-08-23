package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/spf13/cobra"
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
		port, _ := cmd.Flags().GetInt("port")
		cfg.Port = port
	}
	if cmd.Flags().Changed("host") {
		host, _ := cmd.Flags().GetString("host")
		cfg.Host = host
	}
	if cmd.Flags().Changed("allow-origin") {
		origin, _ := cmd.Flags().GetString("allow-origin")
		cfg.AllowOrigin = origin
	}
	if cmd.Flags().Changed("debug") {
		debug, _ := cmd.Flags().GetBool("debug")
		cfg.Debug = debug
	}
	if cmd.Flags().Changed("kube-qps") {
		qps, _ := cmd.Flags().GetFloat32("kube-qps")
		cfg.KubeQPS = qps
	}
	if cmd.Flags().Changed("kube-burst") {
		burst, _ := cmd.Flags().GetInt("kube-burst")
		cfg.KubeBurst = burst
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

	deps := &serverDeps{
		typed:  typed,
		dyn:    dyn,
		scheme: scheme,
		config: cfg,
	}

	if err := helpers.TestKubernetesConnection(deps.typed); err != nil {
		log.Fatalf("Kubernetes connection test failed: %v", err)
	}
	if cfg.Debug {
		log.Println("Kubernetes connection established successfully")
	}

	mux := setupHandlers(deps)

	handler := logRequests(withCORS(
		requireAPIToken([]byte(cfg.JWTSecret), mux),
		cfg.AllowOrigin,
	))
	srv := &http.Server{
		Addr:              cfg.GetAddr(),
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.WriteTimeout) * time.Second,
	}

	log.Printf("Codespace server starting on %s", cfg.GetAddr())
	if cfg.Debug {
		log.Printf("Debug mode enabled")
		log.Printf("CORS allow origin: %s", cfg.AllowOrigin)
	}
	log.Fatal(srv.ListenAndServe())
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
