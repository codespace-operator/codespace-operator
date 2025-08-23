package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/internal/config"
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

// serverDeps holds the server dependencies
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

	// Add flags
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
	// Load configuration
	cfg, err := config.LoadServerConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override config with command line flags
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

	// Kubernetes client setup
	k8sCfg, err := helpers.BuildKubeConfig()
	if err != nil {
		log.Fatalf("Kubernetes config: %v", err)
	}

	// Configure client performance settings
	k8sCfg.Timeout = 30 * time.Second
	k8sCfg.QPS = cfg.KubeQPS
	k8sCfg.Burst = cfg.KubeBurst

	// Scheme with our API types
	scheme := runtime.NewScheme()
	if err := codespacev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Add scheme: %v", err)
	}

	// Typed client
	typed, err := client.New(k8sCfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("Typed client: %v", err)
	}

	// Dynamic client (only for watch streaming)
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

	// Test the connection
	if err := helpers.TestKubernetesConnection(deps.typed); err != nil {
		log.Fatalf("Kubernetes connection test failed: %v", err)
	}

	if cfg.Debug {
		log.Println("Kubernetes connection established successfully")
	}

	// Setup HTTP handlers
	mux := setupHandlers(deps)

	// Create server with configured timeouts
	srv := &http.Server{
		Addr:              cfg.GetAddr(),
		Handler:           withCORS(mux, cfg.AllowOrigin),
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

	// Health probes
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz(deps))

	// API endpoints
	mux.HandleFunc("/api/v1/stream/sessions", handleStreamSessions(deps))
	mux.HandleFunc("/api/v1/sessions", handleSessions(deps))
	mux.HandleFunc("/api/v1/sessions/", handleSessionsWithPath(deps))

	// Static UI
	setupStaticUI(mux)

	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		readyCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		
		ns := q(r, "namespace", "default")
		var sl codespacev1.SessionList
		if err := deps.typed.List(readyCtx, &sl, client.InNamespace(ns), client.Limit(1)); err != nil {
			errJSON(w, err)
			return
		}
		
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}
}

func handleStreamSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := q(r, "namespace", "default")
		watcher, err := deps.dyn.Resource(gvr).Namespace(ns).Watch(r.Context(), metav1.ListOptions{Watch: true})
		if err != nil {
			errJSON(w, err)
			return
		}
		defer watcher.Stop()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return
				}
				if ev.Type == watch.Error {
					continue
				}
				u, ok := ev.Object.(*unstructured.Unstructured)
				if !ok {
					continue
				}
				var s codespacev1.Session
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &s); err != nil {
					continue
				}
				payload := map[string]any{
					"type":   string(ev.Type),
					"object": s,
				}
				writeSSE(w, "message", payload)
				flusher.Flush()
			}
		}
	}
}

func handleSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			ns := q(r, "namespace", "default")
			var sl codespacev1.SessionList
			if err := deps.typed.List(r.Context(), &sl, client.InNamespace(ns)); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, sl.Items)

		case http.MethodPost:
			s, err := decodeSession(r.Body)
			if err != nil {
				errJSON(w, err)
				return
			}
			if s.Namespace == "" {
				s.Namespace = "default"
			}
			applyDefaults(&s)
			
			if err := helpers.RetryOnConflict(func() error {
				return deps.typed.Create(r.Context(), &s)
			}); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func handleSessionsWithPath(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/")
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ns, name := parts[0], parts[1]

		// Handle scale subpath
		if len(parts) == 3 && parts[2] == "scale" {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			handleScale(deps, w, r, ns, name)
			return
		}

		switch r.Method {
		case http.MethodGet:
			var s codespacev1.Session
			if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &s); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)

		case http.MethodDelete:
			s := codespacev1.Session{}
			s.Name, s.Namespace = name, ns
			if err := deps.typed.Delete(r.Context(), &s); err != nil {
				errJSON(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func handleScale(deps *serverDeps, w http.ResponseWriter, r *http.Request, ns, name string) {
	var body struct{ Replicas *int32 `json:"replicas"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, err)
		return
	}
	if body.Replicas == nil {
		errJSON(w, errors.New("replicas is required"))
		return
	}

	var s codespacev1.Session
	if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &s); err != nil {
		errJSON(w, err)
		return
	}
	
	s.Spec.Replicas = body.Replicas
	if err := helpers.RetryOnConflict(func() error {
		return deps.typed.Update(r.Context(), &s)
	}); err != nil {
		errJSON(w, err)
		return
	}
	writeJSON(w, s)
}

func setupStaticUI(mux *http.ServeMux) {
	uiFS, _ := fsSub(staticFS, "static")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") || path.Ext(r.URL.Path) == "" {
			r.URL.Path = "/index.html"
		}
		http.FileServer(uiFS).ServeHTTP(w, r)
	})
}

// -------- Helper functions --------

func decodeSession(r io.Reader) (codespacev1.Session, error) {
	var s codespacev1.Session
	return s, json.NewDecoder(r).Decode(&s)
}

func applyDefaults(s *codespacev1.Session) {
	if s.Spec.Replicas == nil {
		var one int32 = 1
		s.Spec.Replicas = &one
	}
	if len(s.Spec.Profile.Cmd) == 0 {
		s.Spec.Profile.Cmd = nil
	}
}

func q(r *http.Request, key, dflt string) string {
	if v := r.URL.Query().Get(key); v != "" {
		return v
	}
	return dflt
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, event string, v any) {
	_, _ = w.Write([]byte("event: " + event + "\n"))
	b, _ := json.Marshal(v)
	_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
}

func errJSON(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func withCORS(next http.Handler, allowOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		}
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fsSub(fsys embed.FS, dir string) (http.FileSystem, error) {
	sub, err := fs.Sub(fsys, dir)
	return http.FS(sub), err
}