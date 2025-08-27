package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
)

// setupHandlers creates the HTTP handler with comprehensive RBAC
func setupHandlers(deps *serverDeps) *http.ServeMux {
	mux := http.NewServeMux()

	// === Health and Status Endpoints (No Auth Required) ===
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz(deps))

	// === Authentication Endpoints (Handled separately) ===
	// These are registered by registerAuthHandlers()

	// === API v1 Endpoints (Auth Required) ===

	// Session CRUD operations
	mux.HandleFunc("/api/v1/server/sessions", handleSessionOperations(deps))
	mux.HandleFunc("/api/v1/server/sessions/", handleSessionOperationsWithPath(deps))

	// Session streaming (Server-Sent Events)
	mux.HandleFunc("/api/v1/stream/sessions", handleStreamSessions(deps))

	// User and system introspection - UPDATED WITH SPLIT ENDPOINTS
	mux.HandleFunc("/api/v1/introspect", handleIntrospect(deps))
	mux.HandleFunc("/api/v1/introspect/user", handleUserIntrospect(deps))
	mux.HandleFunc("/api/v1/introspect/server", handleServerIntrospect(deps))
	mux.HandleFunc("/api/v1/user/permissions", handleUserPermissions(deps))

	// Admin endpoints (require elevated permissions)
	mux.HandleFunc("/api/v1/admin/users", handleAdminUsers(deps))
	mux.HandleFunc("/api/v1/admin/rbac/reload", handleRBACReload(deps))
	mux.HandleFunc("/api/v1/admin/system/info", handleSystemInfo(deps))

	// === Static UI ===
	setupStaticUI(mux)

	return mux
}

// === Admin Endpoints ===

// handleAdminUsers - GET /api/v1/admin/users (requires admin privileges)
func handleAdminUsers(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Require admin permissions for user management
		cl, ok := mustCan(deps, w, r, "*", "admin", "*")
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
}

// handleRBACReload - POST /api/v1/admin/rbac/reload (force reload RBAC policies)
func handleRBACReload(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Require admin permissions for RBAC management
		cl, ok := mustCan(deps, w, r, "*", "admin", "*")
		if !ok {
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Force reload RBAC policies
		if err := deps.rbac.reload(); err != nil {
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
}

// handleSystemInfo - GET /api/v1/admin/system/info (system information for admins)
func handleSystemInfo(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Require admin permissions for system info
		cl, ok := mustCan(deps, w, r, "*", "admin", "*")
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
				"modelPath":  deps.rbac.modelPath,
				"policyPath": deps.rbac.policyPath,
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
				"localLoginEnabled": deps.config.EnableLocalLogin,
				"oidcConfigured":    deps.config.OIDCIssuerURL != "",
			},
		}

		logger.Info("Retrieved system info", "admin", cl.Sub)
		writeJSON(w, info)
	}
}

// === RBAC Middleware and Helpers ===

// requireAdminAccess middleware that requires admin-level permissions
func requireAdminAccess(deps *serverDeps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := mustCan(deps, w, r, "*", "admin", "*"); !ok {
			return
		}
		next(w, r)
	}
}

// requireNamespaceAccess middleware that checks namespace-level permissions
func requireNamespaceAccess(deps *serverDeps, resource, action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract namespace from URL path or query parameters
		namespace := extractNamespaceFromRequest(r)
		if namespace == "" {
			namespace = "default"
		}

		if _, ok := mustCan(deps, w, r, resource, action, namespace); !ok {
			return
		}
		next(w, r)
	}
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

// === Utility Functions ===

// corsMiddleware adds CORS headers with credentials support
func corsMiddleware(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitMiddleware provides basic rate limiting (placeholder implementation)
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement proper rate limiting based on user/IP
		// For now, just pass through
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds security headers
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Only add HSTS if we're on HTTPS
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// === Enhanced Auth Gate ===

// authGateEnhanced provides more sophisticated authentication routing
func authGateEnhanced(cfg *configLike, next http.Handler) http.Handler {
	authed := requireAPIToken(cfg, next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public endpoints (no auth required)
		publicPaths := []string{
			"/healthz",
			"/readyz",
			"/",
		}

		publicPrefixes := []string{
			"/auth/",
			"/assets/",
			"/static/",
		}

		// Check if this is a public path
		for _, publicPath := range publicPaths {
			if path == publicPath {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check if this is a public prefix
		for _, prefix := range publicPrefixes {
			if strings.HasPrefix(path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// API endpoints require authentication
		if strings.HasPrefix(path, "/api/") {
			authed.ServeHTTP(w, r)
			return
		}

		// Default to serving static content (SPA)
		next.ServeHTTP(w, r)
	})
}

// === Integration Point ===

// buildMiddlewareChain creates the complete middleware chain for the server
func buildMiddlewareChain(cfg *configLike, allowOrigin string, handler http.Handler) http.Handler {
	// Build the chain from outside to inside:
	// securityHeaders( cors( rateLimit( authGate( logRequests( handler )))))

	chain := handler
	chain = logRequests(chain)
	chain = authGateEnhanced(cfg, chain)
	chain = rateLimitMiddleware(chain)
	chain = corsMiddleware(allowOrigin)(chain)
	chain = securityHeadersMiddleware(chain)

	return chain
}

// Serve SPA/static from embedded ui-dist/*
func setupStaticUI(mux *http.ServeMux) {
	ui, err := fsSub(staticFS, "static")
	if err != nil {
		logger.Fatal("Failed to create static file system", "err", err)
	}
	files := http.FileServer(ui)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG") == "true" {
			logger.Printf("Static request: %s", r.URL.Path)
		}
		if path.Ext(r.URL.Path) != "" && r.URL.Path != "/" {
			files.ServeHTTP(w, r)
			return
		}
		index, err := staticFS.ReadFile("static/index.html")
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
