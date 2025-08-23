package main

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"time"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

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

// In cmd/server/codespace-server.go, wrap your handler:
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("REQUEST: %s %s %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Wrap ResponseWriter to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(wrapped, r)

		log.Printf("RESPONSE: %s %s -> %d (%v)", r.Method, r.URL.Path, wrapped.statusCode, time.Since(start))
	})
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
