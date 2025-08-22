package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	grpcapi "github.com/codespace-operator/codespace-operator/cmd/server/grpcapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

//go:embed all:static
var staticFS embed.FS

var gvr = schema.GroupVersionResource{
	Group:    codespacev1.GroupVersion.Group,
	Version:  codespacev1.GroupVersion.Version,
	Resource: "sessions",
}

func main() {
	ctx := context.Background()

	// In-cluster client for K8s API (used by readiness + SSE)
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("in-cluster config: %v", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("dynamic client: %v", err)
	}

	mux := http.NewServeMux()

	// Start gRPC (:9090) and mount the JSON gRPC-Gateway under /api/v1/*
	svr := grpcapi.New(dyn)
	if err := grpcapi.Start(ctx, ":9090", mux, svr); err != nil {
		log.Fatal(err)
	}

	// --- Probes ---
	// Liveness: always fast and local
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Readiness: quick API reachability check (bounded to 1s)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		readyCtx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		ns := q(r, "namespace", "default")
		if _, err := dyn.Resource(gvr).Namespace(ns).List(readyCtx, metav1.ListOptions{Limit: 1}); err != nil {
			errJSON(w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	// --- Server-Sent Events stream (UI) ---
	// Frontend should listen on: GET /api/v1/stream/sessions?namespace=<ns>
	mux.HandleFunc("/api/v1/stream/sessions", func(w http.ResponseWriter, r *http.Request) {
		ns := q(r, "namespace", "default")
		watcher, err := dyn.Resource(gvr).Namespace(ns).Watch(r.Context(), metav1.ListOptions{Watch: true})
		if err != nil {
			errJSON(w, err)
			return
		}
		defer watcher.Stop()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return
				}
				if ev.Type == watch.Error {
					// Optionally emit an error event; for now, skip.
					continue
				}
				out := map[string]any{"type": ev.Type, "object": ev.Object}
				b, _ := json.Marshal(out)
				fmt.Fprintf(w, "data: %s\n\n", b)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	})

	// --- Static UI (embedded) ---
	uiFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	// SPA fallback for client-side routes: serve index.html on "directory" or route-like paths.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") || path.Ext(r.URL.Path) == "" {
			r.URL.Path = "/index.html"
		}
		http.FileServer(http.FS(uiFS)).ServeHTTP(w, r)
	})

	addr := ":8080"
	srv := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("gRPC listening on :9090")
	log.Printf("codespace-server listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// --- helpers ---

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

func errJSON(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Frontend calls the JSON API via the same 8080 origin (or through an Ingress).
		// Keep permissive for now; tighten as needed.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
