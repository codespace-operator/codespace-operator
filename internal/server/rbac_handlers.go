package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"

	auth "github.com/codespace-operator/common/auth/pkg/auth"
	"github.com/codespace-operator/common/common/pkg/common"
)

const SESSION_RESOURCE_STRING = "session"
const NAMESPACE_RESOURCE_STRING = "namespace"

// ClusterInfo contains cluster-level permission information
type ClusterInfo struct {
	Casbin               CasbinPermissions  `json:"casbin"`
	ServerServiceAccount ServiceAccountInfo `json:"serverServiceAccount"`
}
type PermissionCheck struct {
	Resource  string `json:"resource" example:"session" description:"Resource type being checked"`
	Action    string `json:"action" example:"create" description:"Action being performed"`
	Namespace string `json:"namespace" example:"default" description:"Namespace scope"`
	Allowed   bool   `json:"allowed" example:"true" description:"Whether action is permitted"`
}

// CasbinPermissions contains Casbin-managed cluster permissions
type CasbinPermissions struct {
	Namespaces NamespacePermissions `json:"namespaces"`
}

// NamespacePermissions contains namespace-level operations
type NamespacePermissions struct {
	List  bool `json:"list"`
	Watch bool `json:"watch"`
}

// ServiceAccountInfo contains server SA capabilities
type ServiceAccountInfo struct {
	Namespaces NamespacePermissions `json:"namespaces"`
	Session    map[string]bool      `json:"session"`
}

// DomainPermissions contains resource permissions for a domain/namespace
type DomainPermissions struct {
	Session map[string]bool `json:"session"`
}

// UserPermissions represents comprehensive user permissions
type UserPermissions struct {
	Subject     string              `json:"subject" example:"alice@company.com" description:"User identifier"`
	Roles       []string            `json:"roles" example:"editor,viewer" description:"User's roles"`
	Permissions []PermissionCheck   `json:"permissions" description:"Detailed permission matrix"`
	Namespaces  map[string][]string `json:"namespaces" description:"Namespace to allowed actions mapping"`
}

// NamespaceInfo contains user-specific namespace access information
type NamespaceInfo struct {
	UserAllowed   []string `json:"userAllowed"`             // Namespaces user can access
	UserCreatable []string `json:"userCreatable,omitempty"` // Namespaces user can create sessions in
	UserDeletable []string `json:"userDeletable,omitempty"` // Namespaces user can delete sessions from
}

// ServerNamespaceInfo contains server-discoverable namespace information
type ServerNamespaceInfo struct {
	All          []string `json:"all,omitempty"`          // All namespaces (if discoverable)
	WithSessions []string `json:"withSessions,omitempty"` // Namespaces containing sessions
}

// UserCapabilities contains user-specific capability information
type UserCapabilities struct {
	NamespaceScope []string `json:"namespaceScope"` // Effective namespace scope for user
	ClusterScope   bool     `json:"clusterScope"`   // Whether user has any cluster-level access
	AdminAccess    bool     `json:"adminAccess"`    // Whether user has admin privileges
}

// SystemCapabilities contains system-wide capability information
type SystemCapabilities struct {
	ClusterScope bool `json:"clusterScope"` // Displays cluster_scope: true or false
	MultiTenant  bool `json:"multiTenant"`  // Whether system supports multiple tenants
}

// UserIntrospectionResponse represents user-specific information only
type UserIntrospectionResponse struct {
	User         UserInfo                     `json:"user"`
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

	InstanceID string            `json:"instanceID,omitempty"`
	Manager    common.AnchorMeta `json:"manager,omitempty"`
}

// handleUserIntrospect provides user-specific RBAC and permission information
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
func (h *handlers) handleUserIntrospect(w http.ResponseWriter, r *http.Request) {

	cl := auth.FromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	// Parse query parameters
	requestedNamespaces := splitCSVQuery(r.URL.Query().Get("namespaces"))
	actions := splitCSVQuery(r.URL.Query().Get("actions"))

	// Default actions if not specified
	if len(actions) == 0 {
		actions = []string{"get", "list", "watch", "create", "update", "delete", "scale"}
	}

	// Get implicit roles from Casbin
	implicitRoles, _ := h.deps.rbac.GetRolesForUser(cl.Sub)

	// Build user info
	userInfo := UserInfo{
		Subject:       cl.Sub,
		Username:      cl.Username,
		Email:         cl.Email,
		Roles:         cl.Roles,
		Provider:      cl.Provider,
		IssuedAt:      cl.IssuedAt,
		ExpiresAt:     cl.ExpiresAt,
		ImplicitRoles: implicitRoles,
	}

	// Check cluster-level permissions for user
	hasClusterList, _ := h.deps.rbac.Enforce(cl.Sub, cl.Roles, SESSION_RESOURCE_STRING, "list", "*")
	hasClusterWatch, _ := h.deps.rbac.Enforce(cl.Sub, cl.Roles, SESSION_RESOURCE_STRING, "watch", "*")
	nsListAllowed, _ := h.deps.rbac.Enforce(cl.Sub, cl.Roles, NAMESPACE_RESOURCE_STRING, "list", "*")

	// Determine target namespaces for permission checking
	var targetNamespaces []string

	if len(requestedNamespaces) > 0 {
		// Use requested namespaces
		targetNamespaces = requestedNamespaces
	} else {
		// Discover all available namespaces for this user
		discoveredNamespaces, err := getAllowedNamespacesForUser(ctx, h.deps, cl.Sub, cl.Roles)
		if err != nil {
			logger.Warn("Failed to discover allowed namespaces for user", "user", cl.Sub, "err", err)
			// Fallback to common namespaces
			targetNamespaces = []string{"default", "kube-system", "kube-public"}
		} else {
			targetNamespaces = discoveredNamespaces
		}

		// Always include "*" for cluster-wide permissions check
		targetNamespaces = append(targetNamespaces, "*")
	}

	// Remove duplicates and sort
	targetNamespaces = uniqueNamespaces(targetNamespaces)

	// Build domain permissions for user
	domains := make(map[string]DomainPermissions)
	userAllowed := []string{}
	userCreatable := []string{}
	userDeletable := []string{}

	for _, ns := range targetNamespaces {
		sessionPerms := make(map[string]bool)
		hasAnyPermission := false
		canCreate := false
		canDelete := false

		for _, action := range actions {
			allowed, err := h.deps.rbac.Enforce(cl.Sub, cl.Roles, SESSION_RESOURCE_STRING, action, ns)
			if err != nil {
				logger.Warn("RBAC enforcement error", "subject", cl.Sub, "action", action, "namespace", ns, "err", err)
				allowed = false
			}
			sessionPerms[action] = allowed

			if allowed {
				hasAnyPermission = true
				if action == "create" {
					canCreate = true
				}
				if action == "delete" {
					canDelete = true
				}
			}
		}

		domains[ns] = DomainPermissions{
			Session: sessionPerms,
		}

		// Track accessible namespaces (excluding "*" from user lists)
		if hasAnyPermission && ns != "*" {
			userAllowed = append(userAllowed, ns)
		}
		if canCreate && ns != "*" {
			userCreatable = append(userCreatable, ns)
		}
		if canDelete && ns != "*" {
			userDeletable = append(userDeletable, ns)
		}
	}

	sort.Strings(userAllowed)
	sort.Strings(userCreatable)
	sort.Strings(userDeletable)

	namespaceInfo := NamespaceInfo{
		UserAllowed:   userAllowed,
		UserCreatable: userCreatable,
		UserDeletable: userDeletable,
	}

	// Build user-specific capabilities
	capabilities := UserCapabilities{
		NamespaceScope: userAllowed,
		ClusterScope:   hasClusterList || hasClusterWatch || nsListAllowed,
		AdminAccess:    hasClusterList && hasClusterWatch, // Basic admin check
	}

	// Enhanced admin access check - user who can create/delete anywhere
	if starDomain, exists := domains["*"]; exists {
		if starDomain.Session["create"] && starDomain.Session["delete"] {
			capabilities.AdminAccess = true
		}
	}

	// Build user response
	response := UserIntrospectionResponse{
		User:         userInfo,
		Domains:      domains,
		Namespaces:   namespaceInfo,
		Capabilities: capabilities,
	}

	logger.Debug("User introspection completed", "user", cl.Sub, "namespaces", len(targetNamespaces), "allowed", len(userAllowed))
	writeJSON(w, response)
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
func (h *handlers) handleServerIntrospect(w http.ResponseWriter, r *http.Request) {
	cl := auth.FromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Require at least some level of cluster access to see server info
	hasClusterAccess, _ := h.deps.rbac.Enforce(cl.Sub, cl.Roles, NAMESPACE_RESOURCE_STRING, "list", "*")
	hasSessionAccess, _ := h.deps.rbac.Enforce(cl.Sub, cl.Roles, SESSION_RESOURCE_STRING, "list", "*")
	if !hasClusterAccess && !hasSessionAccess {
		http.Error(w, "insufficient permissions to view server information", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	discover := r.URL.Query().Get("discover") == "1"

	// Check server service account capabilities
	serverCapabilities := buildServerCapabilities(ctx, h.deps)

	clusterInfo := ClusterInfo{
		Casbin: CasbinPermissions{
			Namespaces: NamespacePermissions{
				List:  serverCapabilities.Namespaces.List,
				Watch: serverCapabilities.Namespaces.List, // Assume watch follows list
			},
		},
		ServerServiceAccount: serverCapabilities,
	}

	// Server namespace info
	serverNamespaceInfo := ServerNamespaceInfo{}

	// Discover namespaces if requested and either user or server has permissions
	if discover {
		allNs, err := discoverNamespaces(ctx, h.deps)
		if err != nil {
			logger.Warn("Failed to discover namespaces", "err", err, "user", cl.Sub)
		}
		sessNs, err := discoverNamespacesWithSessions(ctx, h.deps)
		if err != nil {
			logger.Warn("Failed to discover session namespaces", "err", err, "user", cl.Sub)
		} else {
			// Filter namespaces based on user permissions if user doesn't have cluster access
			if !hasClusterAccess {
				userAllowedNs, err := getAllowedNamespacesForUser(ctx, h.deps, cl.Sub, cl.Roles)
				if err == nil {
					allNs = filterNamespaces(allNs, userAllowedNs)
					sessNs = filterNamespaces(sessNs, userAllowedNs)
				}
			}
			serverNamespaceInfo.All = allNs
			serverNamespaceInfo.WithSessions = sessNs
		}
	}

	// Build system capabilities
	systemCapabilities := SystemCapabilities{
		MultiTenant:  len(serverNamespaceInfo.All) > 5, // heuristic
		ClusterScope: h.deps.config.ClusterScope,
	}

	// Version info (could be populated from build-time variables)
	versionInfo := ServerVersionInfo{
		Version: "1.0.0",
	}

	// Include server identity for Cluster Settings page
	response := ServerIntrospectionResponse{
		Cluster:      clusterInfo,
		Namespaces:   serverNamespaceInfo,
		Capabilities: systemCapabilities,
		Version:      versionInfo,
		InstanceID:   h.deps.instanceID,
		Manager:      h.deps.manager,
	}

	logger.Debug("Server introspection completed", "user", cl.Sub, "discover", discover)
	writeJSON(w, response)
}

// Legacy handleIntrospect maintains backward compatibility by combining both responses
// @Summary Introspect (legacy combined)
// @Description Deprecated: prefer /api/v1/introspect/user or /api/v1/introspect/server
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Deprecated
// @Success 200 {object} map[string]interface{} "Combined user+server info"
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/introspect [get]
func (h *handlers) handleIntrospect(w http.ResponseWriter, r *http.Request) {

	cl := auth.FromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if this is a request for user or server info specifically
	infoType := r.URL.Query().Get("type")

	switch infoType {
	case "user":
		h.handleUserIntrospect(w, r)
		return
	case "server":
		h.handleServerIntrospect(w, r)
		return
	}

	// Legacy behavior: return combined response but log deprecation warning
	logger.Warn("Using deprecated combined introspect endpoint", "user", cl.Sub, "recommendation", "Use /api/v1/introspect/user or /api/v1/introspect/server")

	// For backward compatibility, provide a combined response
	// Get user info
	userReq := r.Clone(r.Context())
	userReq.URL.RawQuery = r.URL.Query().Encode()
	userRec := httptest.NewRecorder()
	h.handleUserIntrospect(userRec, userReq)

	if userRec.Code != 200 {
		http.Error(w, "failed to get user info", userRec.Code)
		return
	}

	var userResp UserIntrospectionResponse
	if err := json.NewDecoder(userRec.Body).Decode(&userResp); err != nil {
		http.Error(w, "failed to parse user info", http.StatusInternalServerError)
		return
	}

	// Try to get server info (may fail due to permissions)
	serverReq := r.Clone(r.Context())
	serverReq.URL.RawQuery = r.URL.Query().Encode()
	serverRec := httptest.NewRecorder()
	h.handleServerIntrospect(serverRec, serverReq)

	var serverResp ServerIntrospectionResponse
	if serverRec.Code == 200 {
		_ = json.NewDecoder(serverRec.Body).Decode(&serverResp)
	}

	// Combine into legacy format
	legacyResponse := map[string]interface{}{
		"user":         userResp.User,
		"domains":      userResp.Domains,
		"namespaces":   combineNamespaceInfo(userResp.Namespaces, serverResp.Namespaces),
		"capabilities": combineCapabilities(userResp.Capabilities, serverResp.Capabilities),
	}

	// Add server info if available
	if serverRec.Code == 200 {
		legacyResponse["cluster"] = serverResp.Cluster
	}

	writeJSON(w, legacyResponse)

}

// Helper functions for legacy compatibility
func combineNamespaceInfo(user NamespaceInfo, server ServerNamespaceInfo) map[string]interface{} {
	combined := map[string]interface{}{
		"userAllowed": user.UserAllowed,
	}

	if len(user.UserCreatable) > 0 {
		combined["userCreatable"] = user.UserCreatable
	}
	if len(user.UserDeletable) > 0 {
		combined["userDeletable"] = user.UserDeletable
	}
	if len(server.All) > 0 {
		combined["all"] = server.All
	}
	if len(server.WithSessions) > 0 {
		combined["withSessions"] = server.WithSessions
	}

	return combined
}

func combineCapabilities(user UserCapabilities, system SystemCapabilities) map[string]interface{} {
	return map[string]interface{}{
		"namespaceScope": user.NamespaceScope,
		"clusterScope":   user.ClusterScope,
		"adminAccess":    user.AdminAccess,
		"multiTenant":    system.MultiTenant,
	}
}

// Returns the current subject + roles from the JWT.
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
func (h *handlers) handleMe(w http.ResponseWriter, r *http.Request) {

	cl := auth.FromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{
		"sub":      cl.Sub,
		"username": cl.Username,
		"email":    cl.Email,
		"roles":    cl.Roles,
		"provider": cl.Provider,
		"exp":      cl.ExpiresAt,
		"iat":      cl.IssuedAt,
	})
}

// handleUserPermissions - GET /api/v1/user/permissions (detailed user permission info)
// @Summary User permissions
// @Description Get detailed user permissions matrix
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespaces query string false "Comma-separated list of namespaces" example:"default,prod"
// @Param actions query string false "Comma-separated list of actions" example:"create,delete,list"
// @Success 200 {object} UserPermissions "Detailed permission matrix"
// @Failure 401 {object} ErrorResponse "Authentication required"
// @Router /api/v1/user/permissions [get]
func (h *handlers) handleUserPermissions(w http.ResponseWriter, r *http.Request) {
	cl := auth.FromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse parameters
	namespaces := splitCSVQuery(r.URL.Query().Get("namespaces"))
	actions := splitCSVQuery(r.URL.Query().Get("actions"))

	if len(actions) == 0 {
		actions = []string{"get", "list", "watch", "create", "update", "delete", "scale"}
	}

	// If no namespaces specified, discover user's allowed namespaces
	if len(namespaces) == 0 {
		ctx := r.Context()
		userAllowedNamespaces, err := getAllowedNamespacesForUser(ctx, h.deps, cl.Sub, cl.Roles)
		if err != nil {
			logger.Warn("Failed to fetch namespaces", "err", err)
			errJSON(w, fmt.Errorf("failed to retrieve permissions: %w", err))
			return
		}
		if err != nil {
			logger.Warn("Failed to discover namespaces for user permissions", "user", cl.Sub, "err", err)
			errJSON(w, fmt.Errorf("failed to retrieve permissions: %w", err))
			return
		} else {
			namespaces = append(userAllowedNamespaces, "*") // always include cluster-wide
		}
	}

	// Get comprehensive user permissions
	permissions, err := h.deps.rbac.GetUserPermissions(cl.Sub, cl.Roles, SESSION_RESOURCE_STRING, namespaces, actions)
	if err != nil {
		logger.Error("Failed to get user permissions", "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to retrieve permissions: %w", err))
		return
	}

	writeJSON(w, permissions)
}
