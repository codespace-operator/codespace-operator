package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	httpSwagger "github.com/swaggo/http-swagger"
	"github.com/swaggo/swag"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
)

// Add these struct tags to your existing types for better OpenAPI generation

// SessionCreateRequest represents the request body for creating a session
// @Description Request body for creating a new codespace session
type SessionCreateRequest struct {
	Name      string                  `json:"name" validate:"required" example:"my-session" doc:"Session name"`
	Namespace string                  `json:"namespace" example:"default" doc:"Kubernetes namespace"`
	Profile   codespacev1.ProfileSpec `json:"profile" validate:"required"`
	Auth      *codespacev1.AuthSpec   `json:"auth,omitempty"`
	Home      *codespacev1.PVCSpec    `json:"home,omitempty"`
	Scratch   *codespacev1.PVCSpec    `json:"scratch,omitempty"`
	Network   *codespacev1.NetSpec    `json:"networking,omitempty"`
	Replicas  *int32                  `json:"replicas,omitempty" example:"1" doc:"Number of replicas"`
}

// SessionScaleRequest represents the request body for scaling a session
// @Description Request body for scaling a session
type SessionScaleRequest struct {
	Replicas int32 `json:"replicas" validate:"min=0" example:"2" doc:"Target number of replicas"`
}

// SessionListResponse wraps the session list with metadata
// @Description Response containing list of sessions with metadata
type SessionListResponse struct {
	Items      []codespacev1.Session `json:"items"`
	Total      int                   `json:"total" example:"5" doc:"Total number of sessions"`
	Namespaces []string              `json:"namespaces,omitempty" example:"default,kube-system"`
	Filtered   bool                  `json:"filtered,omitempty" doc:"Whether results were filtered by RBAC"`
}

// AuthFeatures represents available authentication methods
// @Description Available authentication features and endpoints
type AuthFeatures struct {
	SSOEnabled        bool   `json:"ssoEnabled" example:"true"`
	LocalLoginEnabled bool   `json:"localLoginEnabled" example:"false"`
	SSOLoginPath      string `json:"ssoLoginPath" example:"/auth/sso/login"`
	LocalLoginPath    string `json:"localLoginPath" example:"/auth/local/login"`
}

// LocalLoginRequest for username/password authentication
// @Description Local login credentials
type LocalLoginRequest struct {
	Username string `json:"username" validate:"required" example:"alice"`
	Password string `json:"password" validate:"required" example:"secretpassword"`
}

// LoginResponse after successful authentication
// @Description Successful authentication response
type LoginResponse struct {
	Token string   `json:"token" doc:"JWT token for API access"`
	User  string   `json:"user" example:"alice"`
	Roles []string `json:"roles" example:"editor,viewer"`
}

// UserInfo represents current user information
// @Description Current authenticated user information
type UserInfo struct {
	Subject       string   `json:"subject" example:"alice@company.com"`
	Username      string   `json:"username,omitempty" example:"alice"`
	Email         string   `json:"email,omitempty" example:"alice@company.com"`
	Roles         []string `json:"roles" example:"editor,viewer"`
	Provider      string   `json:"provider,omitempty" example:"oidc"`
	IssuedAt      int64    `json:"iat,omitempty" example:"1640995200"`
	ExpiresAt     int64    `json:"exp,omitempty" example:"1641081600"`
	ImplicitRoles []string `json:"implicitRoles,omitempty" example:"inherited-role"`
}

// UserIntrospectionResponse represents user-specific information only
type UserIntrospectionResponse struct {
	User         UserInfo                     `json:"user" example:"alice"`
	Domains      map[string]DomainPermissions `json:"domains"`
	Namespaces   NamespaceInfo                `json:"namespaces"`
	Capabilities UserCapabilities             `json:"capabilities"`
}

// ServerIntrospectionResponse represents server/cluster information only
type ServerIntrospectionResponse struct {
	Cluster      ClusterInfo         `json:"cluster"`
	Namespaces   ServerNamespaceInfo `json:"namespaces"`
	Capabilities SystemCapabilities  `json:"capabilities"`
	Version      ServerVersionInfo   `json:"version,omitempty"`
}

// ErrorResponse represents an API error
// @Description API error response
type ErrorResponse struct {
	Error string `json:"error" example:"Invalid request"`
}

// Add Swagger documentation annotations to your handlers

// @Summary List sessions
// @Description Get a list of codespace sessions, optionally across all namespaces
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace query string false "Target namespace" default(default)
// @Param all query boolean false "List sessions across all namespaces"
// @Success 200 {object} SessionListResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/server/sessions [get]
func handleListSessionsWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleListSessions(deps)
}

// @Summary Create session
// @Description Create a new codespace session
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param request body SessionCreateRequest true "Session creation request"
// @Success 201 {object} codespacev1.Session
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/server/sessions [post]
func handleCreateSessionWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleCreateSession(deps)
}

// @Summary Get session
// @Description Get details of a specific session
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Session name"
// @Success 200 {object} codespacev1.Session
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/sessions/{namespace}/{name} [get]
func handleGetSessionWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleGetSession(deps)
}

// @Summary Delete session
// @Description Delete a codespace session
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Session name"
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/sessions/{namespace}/{name} [delete]
func handleDeleteSessionWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleDeleteSession(deps)
}

// @Summary Scale session
// @Description Scale the number of replicas for a session
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Session name"
// @Param request body SessionScaleRequest true "Scale request"
// @Success 200 {object} codespacev1.Session
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/sessions/{namespace}/{name}/scale [post]
func handleScaleSessionWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleScaleSession(deps)
}

// @Summary Update session
// @Description Update a session (full replacement)
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Session name"
// @Param request body SessionCreateRequest true "Session update request"
// @Success 200 {object} codespacev1.Session
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/sessions/{namespace}/{name} [put]
func handleUpdateSessionPutWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleUpdateSession(deps)
}

// @Summary Patch session
// @Description Partially update a session
// @Tags sessions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Session name"
// @Param request body map[string]interface{} true "Partial session update"
// @Success 200 {object} codespacev1.Session
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/sessions/{namespace}/{name} [patch]
func handleUpdateSessionPatchWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleUpdateSession(deps)
}

// Authentication endpoints

// @Summary Get authentication features
// @Description Get available authentication methods and endpoints
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} AuthFeatures
// @Router /auth/features [get]
func handleAuthFeaturesWithDocs(cfg *config.ServerConfig, deps *serverDeps) http.HandlerFunc {
	return handleAuthFeatures(cfg, deps)
}

// @Summary Local login
// @Description Authenticate using username and password
// @Tags authentication
// @Accept json
// @Produce json
// @Param credentials body LocalLoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/local/login [post]
func handleLocalLoginWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleLocalLogin(deps)
}

// @Summary SSO login
// @Description Initiate SSO login flow
// @Tags authentication
// @Param next query string false "Redirect URL after login"
// @Success 302 "Redirect to OIDC provider"
// @Failure 400 {object} ErrorResponse
// @Router /auth/sso/login [get]
func handleSSOLoginStub() {
	// This is just for documentation - actual handler is registered conditionally
}

// @Summary Logout
// @Description Logout current user
// @Tags authentication
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} map[string]string
// @Success 302 "Redirect to OIDC logout"
// @Router /auth/logout [post]
func handleLogoutStub() {
	// This is just for documentation - actual handler varies by auth type
}

// User introspection endpoints

// @Summary Get current user
// @Description Get information about the current authenticated user
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} UserInfo
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/me [get]
func handleMeWithDocs() http.HandlerFunc {
	return handleMe()
}

// @Summary User introspection
// @Description Get user-specific permissions and capabilities
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespaces query string false "Comma-separated list of namespaces to check"
// @Param actions query string false "Comma-separated list of actions to check"
// @Success 200 {object} UserIntrospectionResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/introspect/user [get]
func handleUserIntrospectWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleUserIntrospect(deps)
}

// @Summary Server introspection
// @Description Get server and cluster information
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param discover query string false "Whether to discover namespaces (0 or 1)"
// @Success 200 {object} ServerIntrospectionResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/introspect/server [get]
func handleServerIntrospectWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleServerIntrospect(deps)
}

// @Summary User permissions
// @Description Get detailed user permissions
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespaces query string false "Comma-separated list of namespaces"
// @Param actions query string false "Comma-separated list of actions"
// @Success 200 {object} UserPermissions
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/user/permissions [get]
func handleUserPermissionsWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleUserPermissions(deps)
}

// Health endpoints

// @Summary Health check
// @Description Check if the service is healthy
// @Tags health
// @Produce text/plain
// @Success 200 {string} string "ok"
// @Router /healthz [get]
func handleHealthzWithDocs(w http.ResponseWriter, _ *http.Request) {
	handleHealthz(w, nil)
}

// @Summary Readiness check
// @Description Check if the service is ready to accept traffic
// @Tags health
// @Produce text/plain
// @Param namespace query string false "Namespace to test connectivity" default(default)
// @Success 200 {string} string "ready"
// @Failure 503 {string} string "not ready"
// @Router /readyz [get]
func handleReadyzWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleReadyz(deps)
}

// Stream endpoints documentation
// Note: SSE endpoints are harder to document in OpenAPI, but we can provide basic info

// @Summary Stream sessions
// @Description Stream real-time session updates via Server-Sent Events
// @Tags sessions
// @Produce text/event-stream
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace query string false "Target namespace" default(default)
// @Param all query boolean false "Stream sessions from all namespaces"
// @Success 200 {string} string "SSE stream"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/stream/sessions [get]
func handleStreamSessionsWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleStreamSessions(deps)
}

// Admin endpoints - these require admin privileges

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
func handleAdminUsersWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleAdminUsers(deps)
}

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
func handleRBACReloadWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleRBACReload(deps)
}

// @Summary System info (Admin)
// @Description Get system information (requires admin privileges)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Success 200 {object} map[string]interface{} "System information"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/admin/system/info [get]
func handleSystemInfoWithDocs(deps *serverDeps) http.HandlerFunc {
	return handleSystemInfo(deps)
}

// OpenAPI spec handler
func handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec, err := swag.ReadDoc()
	if err != nil {
		http.Error(w, "Failed to read OpenAPI spec", http.StatusInternalServerError)
		logger.Error("Error reading OpenAPI spec:", err)
		return
	}
	if spec == "" {
		http.Error(w, "OpenAPI spec not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour

	// Pretty print the JSON if requested
	if r.URL.Query().Get("pretty") == "1" {
		var specMap map[string]interface{}
		if json.Unmarshal([]byte(spec), &specMap) == nil {
			if prettyJSON, err := json.MarshalIndent(specMap, "", "  "); err == nil {
				w.Write(prettyJSON)
				return
			}
		}
	}

	w.Write([]byte(spec))
}

// Setup OpenAPI documentation
func setupOpenAPIHandlers(mux *http.ServeMux, deps *serverDeps) {
	// Serve OpenAPI spec at /api/v1/openapi.json
	mux.HandleFunc("/api/v1/openapi.json", handleOpenAPISpec)

	// Serve Swagger UI at /docs/
	mux.Handle("/docs/", httpSwagger.Handler(
		httpSwagger.URL("/api/v1/openapi.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	))

	// Redirect /docs to /docs/ for convenience
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
	})
}

// Update your setupHandlers function to register documented handlers
func setupHandlersWithDocs(deps *serverDeps) *http.ServeMux {
	mux := http.NewServeMux()

	// === Health and Status Endpoints (No Auth Required) ===
	mux.HandleFunc("/healthz", handleHealthzWithDocs)
	mux.HandleFunc("/readyz", handleReadyzWithDocs(deps))

	// === OpenAPI Documentation ===
	setupOpenAPIHandlers(mux, deps)

	// === API v1 Endpoints (Auth Required) ===

	// Session CRUD operations with OpenAPI docs
	mux.HandleFunc("/api/v1/server/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListSessionsWithDocs(deps)(w, r)
		case http.MethodPost:
			handleCreateSessionWithDocs(deps)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Session operations with path parameters
	mux.HandleFunc("/api/v1/server/sessions/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/")
		parts := strings.Split(path, "/")

		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		// Scale operation
		if len(parts) == 3 && parts[2] == "scale" {
			handleScaleSessionWithDocs(deps)(w, r)
			return
		}

		// Regular CRUD operations
		switch r.Method {
		case http.MethodGet:
			handleGetSessionWithDocs(deps)(w, r)
		case http.MethodPut:
			handleUpdateSessionPutWithDocs(deps)(w, r)
		case http.MethodPatch:
			handleUpdateSessionPatchWithDocs(deps)(w, r)
		case http.MethodDelete:
			handleDeleteSessionWithDocs(deps)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Session streaming (Server-Sent Events)
	mux.HandleFunc("/api/v1/stream/sessions", handleStreamSessionsWithDocs(deps))

	// User introspection endpoints
	mux.HandleFunc("/api/v1/me", handleMeWithDocs())
	mux.HandleFunc("/api/v1/introspect", handleIntrospect(deps)) // Keep legacy endpoint
	mux.HandleFunc("/api/v1/introspect/user", handleUserIntrospectWithDocs(deps))
	mux.HandleFunc("/api/v1/introspect/server", handleServerIntrospectWithDocs(deps))
	mux.HandleFunc("/api/v1/user/permissions", handleUserPermissionsWithDocs(deps))

	// Admin endpoints
	mux.HandleFunc("/api/v1/admin/users", handleAdminUsersWithDocs(deps))
	mux.HandleFunc("/api/v1/admin/rbac/reload", handleRBACReloadWithDocs(deps))
	mux.HandleFunc("/api/v1/admin/system/info", handleSystemInfoWithDocs(deps))

	// Static UI (keep your existing implementation)
	setupStaticUI(mux)

	return mux
}

// Wrapper function to inject build-time information
func getBuildInfo() map[string]string {
	// These would typically be injected at build time with -ldflags
	return map[string]string{
		"version":   "v1.0.0",                        // -X main.Version=$(git describe --tags)
		"gitCommit": "abc123",                        // -X main.GitCommit=$(git rev-parse HEAD)
		"buildDate": time.Now().Format(time.RFC3339), // -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
	}
}
