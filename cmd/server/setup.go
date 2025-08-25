package main

import (
	"log"
	"net/http"
	"os"
	"path"
)

func setupHandlers(deps *serverDeps) *http.ServeMux {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz(deps))

	// API (top-level middleware applies auth)
	mux.HandleFunc("/api/v1/stream/sessions", handleStreamSessions(deps))
	mux.HandleFunc("/api/v1/sessions", handleSessions(deps))
	mux.HandleFunc("/api/v1/sessions/", handleSessionsWithPath(deps))
	mux.HandleFunc("/api/v1/namespaces/sessions", handleNamespacesWithSessions(deps))
	mux.HandleFunc("/api/v1/namespaces/writable", handleWritableNamespaces(deps))

	// UI
	setupStaticUI(mux)
	return mux
}

// Serve SPA/static from embedded ui-dist/*
func setupStaticUI(mux *http.ServeMux) {
	ui, err := fsSub(staticFS, "ui-dist")
	if err != nil {
		log.Fatalf("Failed to create static file system: %v", err)
	}
	files := http.FileServer(ui)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG") == "true" {
			log.Printf("Static request: %s", r.URL.Path)
		}
		if path.Ext(r.URL.Path) != "" && r.URL.Path != "/" {
			files.ServeHTTP(w, r)
			return
		}
		index, err := staticFS.ReadFile("ui-dist/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}
