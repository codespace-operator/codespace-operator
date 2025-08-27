package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacker not supported")
}

func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
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

func withCORS(allowOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true") // <-- add this
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

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host != "" {
		return host
	}
	return r.RemoteAddr
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// attach / propagate a request id
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = randB64(6)
			w.Header().Set("X-Request-Id", reqID)
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(rw, r)

		// include claims (when present for /api/*), request id (if any), and client ip
		user := "-"
		if cl := fromContext(r); cl != nil && cl.Sub != "" {
			user = cl.Sub
		}
		if reqID == "" {
			reqID = "-"
		}
		ip := clientIP(r)

		logger.Info("http",
			"method", r.Method,
			"path", r.URL.RequestURI(),
			"status", rw.statusCode,
			"bytes", rw.bytes,
			"dur", time.Since(start),
			"ip", ip,
			"ua", r.UserAgent(),
			"req_id", reqID,
			"user", user,
		)
	})
}
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func randB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkcePair() (verifier string, challenge string) {
	verifier = randB64(32)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

func isSafeRelative(p string) bool {
	if p == "" || strings.HasPrefix(p, "//") {
		return false
	}
	u, err := url.Parse(p)
	if err != nil {
		return false
	}
	return !u.IsAbs()
}
func q(r *http.Request, key, dflt string) string {
	if v := r.URL.Query().Get(key); v != "" {
		return v
	}
	return dflt
}

func splitCSVQuery(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// buildServerCapabilities checks what the server's service account can do
func buildServerCapabilities(ctx context.Context, deps *serverDeps) ServiceAccountInfo {
	// Check namespace permissions
	nsPerms := NamespacePermissions{
		List: k8sCan(ctx, deps.typed, authv1.ResourceAttributes{
			Group:    "",
			Resource: "namespaces",
			Verb:     "list",
		}),
	}

	// Check session resource permissions
	sessionPerms := make(map[string]bool)
	sessionVerbs := []string{"get", "list", "watch", "create", "update", "delete", "patch"}

	for _, verb := range sessionVerbs {
		allowed := k8sCan(ctx, deps.typed, authv1.ResourceAttributes{
			Group:    gvr.Group,
			Resource: gvr.Resource,
			Verb:     verb,
		})
		sessionPerms[verb] = allowed
	}

	return ServiceAccountInfo{
		Namespaces: nsPerms,
		Session:    sessionPerms,
	}
}

// discoverNamespaces finds all namespaces and those with sessions
func discoverNamespaces(ctx context.Context, deps *serverDeps) ([]string, []string, error) {
	// Get all namespaces
	var nsList corev1.NamespaceList
	if err := deps.typed.List(ctx, &nsList); err != nil {
		return nil, nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	allNamespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		allNamespaces = append(allNamespaces, ns.Name)
	}
	sort.Strings(allNamespaces)

	// Find namespaces with sessions
	sessionNamespaces := []string{}
	ul, err := deps.dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return allNamespaces, nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	nsSet := make(map[string]struct{})
	for _, item := range ul.Items {
		nsSet[item.GetNamespace()] = struct{}{}
	}

	for ns := range nsSet {
		sessionNamespaces = append(sessionNamespaces, ns)
	}
	sort.Strings(sessionNamespaces)

	return allNamespaces, sessionNamespaces, nil
}
