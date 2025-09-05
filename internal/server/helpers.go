package server

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	auth "github.com/codespace-operator/common/auth/pkg/auth"
	"github.com/codespace-operator/common/common/pkg/common"
	rbac "github.com/codespace-operator/common/rbac/pkg/rbac"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

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

func fsSub(fsys embed.FS, dir string) (http.FileSystem, error) {
	sub, err := fs.Sub(fsys, dir)
	return http.FS(sub), err
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

// extractNamespaceFromRequest extracts namespace from request path or query parameters
func (h *handlers) extractNamespaceFromRequest(r *http.Request) string {
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
	var sessionNamespaces []string
	var sl codespacev1.SessionList
	opts := []client.ListOption{}
	if !deps.config.ClusterScope {
		opts = append(opts, client.MatchingLabels{common.InstanceIDLabel: deps.instanceID})
	}

	// Try cluster-wide list of sessions first
	if err := deps.client.List(ctx, &sl, opts...); err == nil {
		nsSet := map[string]struct{}{}
		for _, s := range sl.Items {
			nsSet[s.Namespace] = struct{}{}
		}
		for ns := range nsSet {
			sessionNamespaces = append(sessionNamespaces, ns)
		}
		sort.Strings(sessionNamespaces)
	} else {
		// Fallback: try per-namespace only if we can discover namespaces
		if canListNamespaces(ctx, deps) {
			var nsList corev1.NamespaceList
			if err := deps.client.List(ctx, &nsList); err == nil {
				nsSet := map[string]struct{}{}
				for _, ns := range nsList.Items {
					var one codespacev1.SessionList
					opts := []client.ListOption{
						client.InNamespace(ns.Name),
						client.Limit(1),
					}
					if !deps.config.ClusterScope {
						opts = append(opts, client.MatchingLabels{common.InstanceIDLabel: deps.instanceID})
					}
					if err := deps.client.List(ctx, &one, opts...); err == nil && len(one.Items) > 0 {
						nsSet[ns.Name] = struct{}{}
					}
				}
				for ns := range nsSet {
					sessionNamespaces = append(sessionNamespaces, ns)
				}
				sort.Strings(sessionNamespaces)
			}
		}
	}

	// Only add default fallback if we truly found no namespaces with sessions
	// Remove this if you want an empty list when no sessions exist
	if len(sessionNamespaces) == 0 {
		sessionNamespaces = []string{""}
	}

	return sessionNamespaces, nil
}

// testKubernetesConnection verifies that we can connect to the Kubernetes API
func testKubernetesConnection(c client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var sessionList codespacev1.SessionList
	// It would make more sense to test to its own namespace
	ns, _ := common.ResolveAnchorNamespace()
	if err := c.List(ctx, &sessionList, client.InNamespace(ns), client.Limit(1)); err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API: %w", err)
	}
	log.Printf("✅ Successfully connected to Kubernetes API (found %d sessions in %s namespace)", len(sessionList.Items), ns)
	return nil
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
		if canAccess, _ := deps.rbac.Enforce(subject, roles, "session", "list", ns); canAccess {
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

// ParseLDAPRoleMapping converts a map[string]string (as returned by Cobra's
// GetStringToString) into map[string][]string for LDAP role mapping.
// Each value can contain a comma-separated list of roles.
func ParseLDAPRoleMapping(raw map[string]string) map[string][]string {
	out := make(map[string][]string, len(raw))
	for group, csv := range raw {
		parts := strings.Split(csv, ",")
		roles := make([]string, 0, len(parts))
		seen := map[string]struct{}{}
		for _, p := range parts {
			r := strings.TrimSpace(p)
			if r == "" {
				continue
			}
			if _, dup := seen[r]; dup {
				continue
			}
			seen[r] = struct{}{}
			roles = append(roles, r)
		}
		if len(roles) > 0 {
			out[group] = roles
		}
	}
	return out
}
func ExtractFromAuth(r *http.Request) (*rbac.Principal, error) {
	cl := auth.FromContext(r)
	if cl == nil {
		return nil, errors.New("no auth")
	}
	return &rbac.Principal{Subject: cl.Sub, Roles: cl.Roles}, nil
}
