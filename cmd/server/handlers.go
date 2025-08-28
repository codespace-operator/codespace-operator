package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

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

	cl, ok := mustCan(h.deps, w, r, "*", "admin", "*")
	if !ok {
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// This would typically integrate with your user management system
	// For now, return basic info about roles and permissions
	users := []map[string]interface{}{
		{
			"subject": cl.Sub,
			"roles":   cl.Roles,
			"active":  true,
		},
	}

	response := map[string]interface{}{
		"users": users,
		"total": len(users),
	}

	logger.Info("Listed users", "admin", cl.Sub)
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
	// Require admin permissions for RBAC management
	cl, ok := mustCan(h.deps, w, r, "*", "admin", "*")
	if !ok {
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Force reload RBAC policies
	if err := h.deps.rbac.reload(); err != nil {
		logger.Error("Failed to reload RBAC", "err", err, "admin", cl.Sub)
		errJSON(w, fmt.Errorf("failed to reload RBAC policies: %w", err))
		return
	}

	logger.Info("RBAC policies reloaded", "admin", cl.Sub)
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

	cl, ok := mustCan(h.deps, w, r, "*", "admin", "*")
	if !ok {
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Gather system information
	info := map[string]interface{}{
		"version": "1.0.0", // This should come from build info
		"rbac": map[string]interface{}{
			"modelPath":  h.deps.rbac.modelPath,
			"policyPath": h.deps.rbac.policyPath,
			"status":     "active",
		},
		"kubernetes": map[string]interface{}{
			"gvr": map[string]string{
				"group":    gvr.Group,
				"version":  gvr.Version,
				"resource": gvr.Resource,
			},
		},
		"authentication": map[string]interface{}{
			"localLoginEnabled": h.deps.config.EnableLocalLogin,
			"oidcConfigured":    h.deps.config.OIDCIssuerURL != "",
		},
		"documentation": map[string]interface{}{
			"available": swagDocAvailable(),
		},
	}

	logger.Info("Retrieved system info", "admin", cl.Sub)
	writeJSON(w, info)
}
