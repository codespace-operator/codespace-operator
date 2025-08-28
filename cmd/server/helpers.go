package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

const (
	LabelCreatedBy         = "codespace.dev/created-by"     // hashed, label-safe
	AnnotationCreatedBy    = "codespace.dev/created-by"     // raw subject
	AnnotationCreatedBySig = "codespace.dev/created-by.sig" // optional: HMAC of raw subject
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
	nsPerms := NamespacePermissions{
		List: canListNamespaces(ctx, deps),
	}
	sessionPerms := map[string]bool{}
	for _, verb := range []string{"get", "list", "watch", "create", "update", "delete", "patch"} {
		sessionPerms[verb] = canPerformSessionAction(ctx, deps, verb)
	}
	return ServiceAccountInfo{Namespaces: nsPerms, Session: sessionPerms}
}

// canListNamespaces tests if the server can list namespaces
func canListNamespaces(ctx context.Context, deps *serverDeps) bool {
	var nsList corev1.NamespaceList
	if err := deps.client.List(ctx, &nsList); err != nil {
		logger.Debug("Server cannot list namespaces", "err", err)
		return false
	}
	return true
}

// canPerformSessionAction tests if the server can perform a specific action on sessions
func canPerformSessionAction(ctx context.Context, deps *serverDeps, verb string) bool {
	switch verb {
	case "list":
		var sl codespacev1.SessionList
		if err := deps.client.List(ctx, &sl, client.InNamespace("default")); err != nil {
			logger.Debug("Server cannot list sessions", "err", err)
			return false
		}
		return true
	case "get", "watch", "create", "update", "delete", "patch":
		// Heuristic: if we can list, we assume the service account is set up for the rest per your RBAC policy.
		return canPerformSessionAction(ctx, deps, "list")
	default:
		return false
	}
}

// discoverNamespaces finds all namespaces server has rights to (by given ServiceAccount) also finds namespaces with sessions
func discoverNamespaces(ctx context.Context, deps *serverDeps) ([]string, error) {
	// all namespaces (typed)
	var allNamespaces []string
	if canListNamespaces(ctx, deps) {
		var nsList corev1.NamespaceList
		if err := deps.client.List(ctx, &nsList); err == nil {
			allNamespaces = make([]string, 0, len(nsList.Items))
			for _, ns := range nsList.Items {
				allNamespaces = append(allNamespaces, ns.Name)
			}
			sort.Strings(allNamespaces)
		} else {
			logger.Warn("Failed to list namespaces despite permission check", "err", err)
		}
	}

	if len(allNamespaces) == 0 {
		allNamespaces = []string{"default"}

	}
	return allNamespaces, nil
}

// discoverNamespacesWithSessions finds namespaces with sessions
func discoverNamespacesWithSessions(ctx context.Context, deps *serverDeps) ([]string, error) {
	// Find namespaces with sessions - try cluster-wide list first
	var allNamespaces []string
	if canListNamespaces(ctx, deps) {
		var nsList corev1.NamespaceList
		if err := deps.client.List(ctx, &nsList); err == nil {
			allNamespaces = make([]string, 0, len(nsList.Items))
			for _, ns := range nsList.Items {
				allNamespaces = append(allNamespaces, ns.Name)
			}
			sort.Strings(allNamespaces)
		} else {
			logger.Warn("Failed to list namespaces despite permission check", "err", err)
		}
	}
	var sessionNamespaces []string
	var sl codespacev1.SessionList
	if err := deps.client.List(
		ctx,
		&sl,
		client.MatchingLabels{InstanceIDLabel: deps.instanceID},
	); err == nil {
		nsSet := map[string]struct{}{}
		for _, s := range sl.Items {
			nsSet[s.Namespace] = struct{}{}
		}
		for ns := range nsSet {
			sessionNamespaces = append(sessionNamespaces, ns)
		}
		sort.Strings(sessionNamespaces)
	} else {
		// fallback: try per-namespace (only if we discovered them)
		nsSet := map[string]struct{}{}
		for _, ns := range allNamespaces {
			var one codespacev1.SessionList
			if err := deps.client.List(
				ctx, &one,
				client.InNamespace(ns),
				client.MatchingLabels{InstanceIDLabel: deps.instanceID},
				client.Limit(1),
			); err == nil && len(one.Items) > 0 {
				nsSet[ns] = struct{}{}
			}
		}
		for ns := range nsSet {
			sessionNamespaces = append(sessionNamespaces, ns)
		}
		sort.Strings(sessionNamespaces)
	}

	if len(sessionNamespaces) == 0 {
		sessionNamespaces = []string{"default"}
	}
	return sessionNamespaces, nil
}

// getAllowedNamespacesForUser determines which namespaces a user can access
// by checking their RBAC permissions against known namespaces
func getAllowedNamespacesForUser(ctx context.Context, deps *serverDeps, subject string, roles []string) ([]string, error) {
	// First, get all available namespaces
	allNamespaces, err := discoverNamespaces(ctx, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to discover namespaces: %w", err)
	}
	// Check RBAC permissions for each namespace
	allowedNamespaces := []string{}

	// Always check cluster-wide access first
	if hasClusterAccess, _ := deps.rbac.Enforce(subject, roles, "session", "list", "*"); hasClusterAccess {
		// User has cluster-wide access, return all namespaces
		return allNamespaces, nil
	}

	// Check each namespace individually
	for _, ns := range allNamespaces {
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

func hostOrHash(issuer string) string {
	u, err := url.Parse(issuer)
	if err == nil && u.Host != "" {
		return strings.ToLower(u.Host) // okta.example.com
	}
	// Fallback: short hash for weird issuers
	sum := sha256.Sum256([]byte(issuer))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:16]
}

// extractNamespaceFromRequest extracts namespace from URL path or query parameters
func extractNamespaceFromRequest(r *http.Request) string {
	// Try query parameter first
	if ns := r.URL.Query().Get("namespace"); ns != "" {
		return ns
	}

	// Try to extract from path
	if strings.Contains(r.URL.Path, "/sessions/") {
		parts := strings.Split(r.URL.Path, "/")
		for i, part := range parts {
			if part == "sessions" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	return ""
}

func inClusterNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	if b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return "default"
}

// Try to resolve a stable owner (Deployment > StatefulSet > ReplicaSet > Pod).
func owningControllerAnchor(ctx context.Context, cl client.Client) (kind, name, uid string) {
	ns := inClusterNamespace()
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname() // usually equals pod name in k8s
	}
	var pod corev1.Pod
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: podName}, &pod); err != nil {
		return "Pod", podName, "" // fallback: still deterministic-ish
	}
	// default to the pod itself
	kind, name, uid = "Pod", pod.Name, string(pod.UID)
	for _, or := range pod.OwnerReferences {
		if or.Controller == nil || !*or.Controller {
			continue
		}
		switch or.Kind {
		case "ReplicaSet":
			var rs appsv1.ReplicaSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: or.Name}, &rs); err == nil {
				// prefer Deployment if present
				for _, rsor := range rs.OwnerReferences {
					if rsor.Controller != nil && *rsor.Controller && rsor.Kind == "Deployment" {
						var dep appsv1.Deployment
						if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: rsor.Name}, &dep); err == nil {
							return "Deployment", dep.Name, string(dep.UID)
						}
					}
				}
				return "ReplicaSet", rs.Name, string(rs.UID)
			}
		case "StatefulSet":
			var sts appsv1.StatefulSet
			if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: or.Name}, &sts); err == nil {
				return "StatefulSet", sts.Name, string(sts.UID)
			}
		}
	}
	return kind, name, uid
}

// ensureInstallationID creates/gets a per-instance ConfigMap and returns a stable id.
// The ConfigMap name is derived from the owning controller UID to avoid collisions
// between multiple instances in the same namespace.
func ensureInstallationID(ctx context.Context, cl client.Client) (string, error) {
	ns := inClusterNamespace()
	kind, name, uid := owningControllerAnchor(ctx, cl)

	anchorParts := []string{ns, "server", kind}
	if uid != "" {
		anchorParts = append(anchorParts, uid)
	} else {
		anchorParts = append(anchorParts, name)
	}
	anchor := strings.Join(anchorParts, ":")
	cmName := fmt.Sprintf("%s-%s", cmPrefixName, shortHash(anchor))

	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Namespace: ns, Name: cmName}
	if err := cl.Get(ctx, key, cm); err == nil {
		if id := cm.Data["id"]; id != "" {
			return id, nil
		}
	} else if !apierrors.IsNotFound(err) {
		return "", err
	}

	id := randB64(18)
	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: ns,
			Labels: map[string]string{
				"app.kubernetes.io/part-of":    "codespace-operator",
				"app.kubernetes.io/component":  "server",
				"app.kubernetes.io/managed-by": "codespace-operator",
				"codespace.dev/owner-kind":     kind,
				"codespace.dev/owner-name":     name,
			},
		},
		Data: map[string]string{"id": id},
	}
	if err := cl.Create(ctx, cm); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// concurrent creator—re-read and return
			if err := cl.Get(ctx, key, cm); err == nil && cm.Data["id"] != "" {
				return cm.Data["id"], nil
			}
		}
		return "", err
	}
	return id, nil
}

// SubjectToLabelID returns a stable, label-safe ID for a user/subject.
// Format: s256-<40 hex> (first 20 bytes of SHA-256 => 40 hex chars). Total length 45.
func SubjectToLabelID(sub string) string {
	if sub == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sub))
	return "s256-" + hex.EncodeToString(sum[:20]) // 160-bit truncation; label-safe; <=63
}
