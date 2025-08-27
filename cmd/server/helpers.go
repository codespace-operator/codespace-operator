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
			w.Header().Set("Access-Control-Allow-Credentials", "true")
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

// buildServerCapabilities manually checks what the server's service account can do
// by attempting to list namespaces and sessions
func buildServerCapabilities(ctx context.Context, deps *serverDeps) ServiceAccountInfo {
	// Check namespace permissions by trying to list namespaces
	nsPerms := NamespacePermissions{
		List: canListNamespaces(ctx, deps),
	}

	// Check session resource permissions by trying operations
	sessionPerms := make(map[string]bool)
	sessionVerbs := []string{"get", "list", "watch", "create", "update", "delete", "patch"}

	for _, verb := range sessionVerbs {
		allowed := canPerformSessionAction(ctx, deps, verb)
		sessionPerms[verb] = allowed
	}

	return ServiceAccountInfo{
		Namespaces: nsPerms,
		Session:    sessionPerms,
	}
}

// canListNamespaces tests if the server can list namespaces
func canListNamespaces(ctx context.Context, deps *serverDeps) bool {
	var nsList corev1.NamespaceList
	if err := deps.typed.List(ctx, &nsList); err != nil {
		logger.Debug("Server cannot list namespaces", "err", err)
		return false
	}
	return true
}

// canPerformSessionAction tests if the server can perform a specific action on sessions
func canPerformSessionAction(ctx context.Context, deps *serverDeps, verb string) bool {
	switch verb {
	case "list":
		// Try to list sessions in default namespace
		ul, err := deps.dyn.Resource(gvr).Namespace("default").List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			logger.Debug("Server cannot list sessions", "err", err)
			return false
		}
		return ul != nil

	case "get":
		// If we can list, we can likely get as well
		return canPerformSessionAction(ctx, deps, "list")

	case "watch":
		// If we can list, we can likely watch as well
		return canPerformSessionAction(ctx, deps, "list")

	case "create", "update", "delete", "patch":
		// These are write operations - we'll assume they follow list permissions
		// In a real implementation, you might want to check against specific RBAC rules
		// or try a dry-run operation
		return canPerformSessionAction(ctx, deps, "list")

	default:
		return false
	}
}

// discoverNamespaces finds all namespaces and those with sessions
func discoverNamespaces(ctx context.Context, deps *serverDeps) ([]string, []string, error) {
	// Get all namespaces - only if server has permission
	var allNamespaces []string

	if canListNamespaces(ctx, deps) {
		var nsList corev1.NamespaceList
		if err := deps.typed.List(ctx, &nsList); err != nil {
			logger.Warn("Failed to list namespaces despite permission check", "err", err)
		} else {
			allNamespaces = make([]string, 0, len(nsList.Items))
			for _, ns := range nsList.Items {
				allNamespaces = append(allNamespaces, ns.Name)
			}
			sort.Strings(allNamespaces)
			logger.Debug("Discovered namespaces", "count", len(allNamespaces), "namespaces", allNamespaces)
		}
	} else {
		logger.Debug("Server cannot list namespaces - no cluster permissions")
	}

	// Find namespaces with sessions - try cluster-wide list first
	var sessionNamespaces []string

	ul, err := deps.dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		// If cluster-wide list fails, try to discover from known namespaces
		logger.Debug("Cannot list sessions cluster-wide, trying per-namespace discovery", "err", err)

		// If we have namespace list, check each one
		if len(allNamespaces) > 0 {
			nsSet := make(map[string]struct{})
			for _, ns := range allNamespaces {
				nsList, err := deps.dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{Limit: 1})
				if err != nil {
					continue // Skip namespaces we can't access
				}
				if len(nsList.Items) > 0 {
					nsSet[ns] = struct{}{}
				}
			}

			for ns := range nsSet {
				sessionNamespaces = append(sessionNamespaces, ns)
			}
		} else {
			// Fallback: check common namespaces
			commonNamespaces := []string{"default", "kube-system", "kube-public", "codespace-operator-system"}
			nsSet := make(map[string]struct{})
			for _, ns := range commonNamespaces {
				nsList, err := deps.dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{Limit: 1})
				if err != nil {
					continue
				}
				if len(nsList.Items) > 0 {
					nsSet[ns] = struct{}{}
				}
			}

			for ns := range nsSet {
				sessionNamespaces = append(sessionNamespaces, ns)
			}
		}
	} else {
		// Cluster-wide list succeeded, extract unique namespaces
		nsSet := make(map[string]struct{})
		for _, item := range ul.Items {
			nsSet[item.GetNamespace()] = struct{}{}
		}

		for ns := range nsSet {
			sessionNamespaces = append(sessionNamespaces, ns)
		}
		logger.Debug("Discovered session namespaces via cluster-wide list", "count", len(sessionNamespaces), "namespaces", sessionNamespaces)
	}

	sort.Strings(sessionNamespaces)

	// If we have no namespaces, add some defaults to prevent empty dropdowns
	if len(allNamespaces) == 0 {
		allNamespaces = []string{"default"}
		logger.Debug("No namespaces discovered, using default fallback")
	}
	if len(sessionNamespaces) == 0 {
		sessionNamespaces = []string{"default"}
		logger.Debug("No session namespaces discovered, using default fallback")
	}

	return allNamespaces, sessionNamespaces, nil
}

// getAllowedNamespacesForUser determines which namespaces a user can access
// by checking their RBAC permissions against known namespaces
func getAllowedNamespacesForUser(ctx context.Context, deps *serverDeps, subject string, roles []string) ([]string, error) {
	// First, get all available namespaces
	allNamespaces, sessionNamespaces, err := discoverNamespaces(ctx, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to discover namespaces: %w", err)
	}

	// Combine all known namespaces (prioritize those with sessions)
	candidateNamespaces := make([]string, 0, len(allNamespaces)+len(sessionNamespaces))
	seen := make(map[string]bool)

	// Add session namespaces first
	for _, ns := range sessionNamespaces {
		if !seen[ns] {
			candidateNamespaces = append(candidateNamespaces, ns)
			seen[ns] = true
		}
	}

	// Add all other namespaces
	for _, ns := range allNamespaces {
		if !seen[ns] {
			candidateNamespaces = append(candidateNamespaces, ns)
			seen[ns] = true
		}
	}

	// Check RBAC permissions for each namespace
	allowedNamespaces := []string{}

	// Always check cluster-wide access first
	if hasClusterAccess, _ := deps.rbac.Enforce(subject, roles, "session", "list", "*"); hasClusterAccess {
		// User has cluster-wide access, return all namespaces
		return candidateNamespaces, nil
	}

	// Check each namespace individually
	for _, ns := range candidateNamespaces {
		if canAccess, _ := deps.rbac.CanAccessNamespace(subject, roles, ns); canAccess {
			allowedNamespaces = append(allowedNamespaces, ns)
		}
	}

	return allowedNamespaces, nil
}
func uniqueNamespaces(namespaces []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, ns := range namespaces {
		if ns != "" && !seen[ns] {
			seen[ns] = true
			result = append(result, ns)
		}
	}

	sort.Strings(result)
	return result
}

func filterNamespaces(allNamespaces, allowedNamespaces []string) []string {
	allowedSet := make(map[string]bool)
	for _, ns := range allowedNamespaces {
		allowedSet[ns] = true
	}

	filtered := []string{}
	for _, ns := range allNamespaces {
		if allowedSet[ns] {
			filtered = append(filtered, ns)
		}
	}

	return filtered
}
