package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
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

// --- server state (typed client + dynamic for watch) ---

type serverDeps struct {
	typed  client.Client
	dyn    dynamic.Interface
	scheme *runtime.Scheme
}

func main() {
	// In-cluster config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("in-cluster config: %v", err)
	}

	// Scheme with our API types
	scheme := runtime.NewScheme()
	if err := codespacev1.AddToScheme(scheme); err != nil {
		log.Fatalf("add scheme: %v", err)
	}

	// Typed client
	typed, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("typed client: %v", err)
	}

	// Dynamic (only for watch streaming)
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("dynamic client: %v", err)
	}

	deps := &serverDeps{typed: typed, dyn: dyn, scheme: scheme}

	mux := http.NewServeMux()

	// --- probes ---
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
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
	})

	// --- SSE stream (dynamic watch, convert to typed) ---
	mux.HandleFunc("/api/v1/stream/sessions", func(w http.ResponseWriter, r *http.Request) {
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
					// if conversion fails, skip the frame rather than poisoning the stream
					continue
				}
				payload := map[string]any{
					"type":   string(ev.Type),
					"object": s, // typed object marshals to JSON
				}
				writeSSE(w, "message", payload)
				flusher.Flush()
			}
		}
	})

	// --- REST: list/create ---
	mux.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			ns := q(r, "namespace", "default")
			var sl codespacev1.SessionList
			if err := deps.typed.List(r.Context(), &sl, client.InNamespace(ns)); err != nil {
				errJSON(w, err)
				return
			}
			// Return just items (what your UI expects)
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
			if err := deps.typed.Create(r.Context(), &s); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// --- REST: get/delete/scale ---
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/")
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ns, name := parts[0], parts[1]

		// scale subpath
		if len(parts) == 3 && parts[2] == "scale" {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
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
			if err := deps.typed.Update(r.Context(), &s); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)
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
	})

	// --- static UI ---
	uiFS, _ := fsSub(staticFS, "static")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") || path.Ext(r.URL.Path) == "" {
			r.URL.Path = "/index.html"
		}
		http.FileServer(uiFS).ServeHTTP(w, r)
	})

	addr := ":8080"
	srv := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("codespace-server listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// -------- helpers (typed) --------

func decodeSession(r io.Reader) (codespacev1.Session, error) {
	var s codespacev1.Session
	return s, json.NewDecoder(r).Decode(&s)
}

// Apply any server-side defaults/mutations you want to guarantee.
// Keep it small and deterministic; avoid controller-level logic here.
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
func withCORS(next http.Handler) http.Handler {
	allow := os.Getenv("ALLOW_ORIGIN") // empty = same-origin only
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow != "" {
			w.Header().Set("Access-Control-Allow-Origin", allow)
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
func itoa(i int32) string { return fmt.Sprintf("%d", i) }
