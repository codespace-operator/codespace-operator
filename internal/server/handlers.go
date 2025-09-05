package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	auth "github.com/codespace-operator/common/auth/pkg/auth"
	"github.com/codespace-operator/common/common/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
)

// handlers contains all HTTP handlers with their dependencies
type handlers struct {
	deps *serverDeps
}

// newHandlers creates a new handlers instance
func newHandlers(deps *serverDeps) *handlers {
	return &handlers{deps: deps}
}

func (h *handlers) wrapWithAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := h.deps.authManager.ValidateRequest(r)
		if err != nil {
			h.deps.logger.Debug("Authentication failed", "error", err, "path", r.URL.Path)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Add claims to request context
		r = r.WithContext(auth.WithClaims(r.Context(), claims))
		handler(w, r)
	}
}

// wrapWithRBAC wraps a handler with RBAC authorization
func (h *handlers) wrapWithRBAC(resource, action, domain string, handler http.HandlerFunc) http.HandlerFunc {
	return h.wrapWithAuth(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.deps.rbacMw.MustCan(w, r, resource, action, domain); !ok {
			logger.Debug("RBAC authorization failed", "resource", resource, "action", action, "domain", domain, "subject", auth.FromContext(r).Sub)
			return
		}
		handler(w, r)
	})
}

// wrapWithNamespaceRBAC wraps a handler with namespace-specific RBAC
func (h *handlers) wrapWithNamespaceRBAC(resource, action string, handler http.HandlerFunc) http.HandlerFunc {
	return h.wrapWithAuth(func(w http.ResponseWriter, r *http.Request) {
		namespace := h.extractNamespaceFromRequest(r)
		if namespace == "" {
			namespace = "default"
		}
		// Domain for session is namespace
		if _, ok := h.deps.rbacMw.MustCan(w, r, resource, action, namespace); !ok {
			logger.Debug("RBAC authorization failed", "resource", resource, "action", action, "domain", namespace, "subject", auth.FromContext(r).Sub)
			return
		}
		handler(w, r)
	})
}

// Healthz OK
// @Summary Health check
// @Description Check if the service is healthy
// @Tags health
// @Produce text/plain
// @Success 200 {string} string "ok"
// @Router /healthz [get]
func (h *handlers) handleHealthz(w http.ResponseWriter, _ *http.Request) {

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// @Summary Readiness check
// @Description Check if the service is ready to accept traffic
// @Tags health
// @Produce text/plain
// @Param namespace query string false "Namespace to test connectivity" default(default)
// @Success 200 {string} string "ready"
// @Failure 503 {string} string "not ready"
// @Router /readyz [get]
func (h *handlers) handleReadyz(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	readyCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	ns := q(r, "namespace", "default")
	var sl codespacev1.SessionList
	if err := h.deps.client.List(readyCtx, &sl, client.InNamespace(ns), client.Limit(1)); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready: " + err.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// handleAdminUsers - GET /api/v1/admin/users (requires admin privileges)
// @Summary List users (Admin)
// @Description Get list of users in the system (requires admin privileges)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} map[string]interface{} "User list with admin info"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/admin/users [get]
// Require admin permissions for user management
func (h *handlers) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	logger := common.LoggerFromContext(r.Context())
	claims := auth.FromContext(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// This would typically integrate with your user management system
	users := []map[string]interface{}{
		{
			"subject": claims.Sub,
			"roles":   claims.Roles,
			"active":  true,
		},
	}

	response := map[string]interface{}{
		"users": users,
		"total": len(users),
	}

	logger.Info("Listed users", "admin", claims.Sub)
	writeJSON(w, response)
}

// handleRBACReload - POST /api/v1/admin/rbac/reload (force reload RBAC policies)
// @Summary Reload RBAC (Admin)
// @Description Force reload of RBAC policies (requires admin privileges)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/admin/rbac/reload [post]
func (h *handlers) handleRBACReload(w http.ResponseWriter, r *http.Request) {
	logger := common.LoggerFromContext(r.Context())
	claims := auth.FromContext(r)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Force reload RBAC policies through interface
	if err := h.deps.rbac.Reload(); err != nil {
		logger.Error("Failed to reload RBAC", "error", err, "admin", claims.Sub)
		errJSON(w, fmt.Errorf("failed to reload RBAC policies: %w", err))
		return
	}

	logger.Info("RBAC policies reloaded", "admin", claims.Sub)
	writeJSON(w, map[string]string{
		"status":  "success",
		"message": "RBAC policies reloaded successfully",
	})
}

// handleSystemInfo - GET /api/v1/admin/system/info (system information for admins)
// Healthz OK
// @Summary System Info
// @Description Check system information (requires admin privileges)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/system/info [get]
// Require admin permissions for system info
func (h *handlers) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	logger := common.LoggerFromContext(r.Context())
	claims := auth.FromContext(r)

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Gather system information
	buildInfo := common.GetBuildInfo()

	info := map[string]interface{}{
		"version":      buildInfo["version"],
		"gitCommit":    buildInfo["gitCommit"],
		"buildDate":    buildInfo["buildDate"],
		"instanceID":   h.deps.instanceID,
		"clusterScope": h.deps.config.ClusterScope,
		"manager": map[string]string{
			"type":      h.deps.manager.Type,
			"name":      h.deps.manager.Name,
			"namespace": h.deps.manager.Namespace,
		},
		"authentication": map[string]interface{}{
			"providers":       h.deps.authManager.ListProviders(),
			"sessionTTL":      h.deps.config.SessionTTL(),
			"allowTokenParam": h.deps.config.AllowTokenParam,
		},
		"rbac": map[string]interface{}{
			"status": "active",
		},
		"kubernetes": map[string]interface{}{
			"gvr": map[string]string{
				"group":    gvr.Group,
				"version":  gvr.Version,
				"resource": gvr.Resource,
			},
		},
	}

	logger.Info("Retrieved system info", "admin", claims.Sub)
	writeJSON(w, info)
}
