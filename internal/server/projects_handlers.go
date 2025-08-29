package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

// ProjectCreateRequest represents the request body for creating a project
// @Description Request body for creating a new codespace project
type ProjectCreateRequest struct {
	Name                  string                      `json:"name" validate:"required" example:"my-project"`
	Namespace             string                      `json:"namespace" example:"default"`
	DisplayName           string                      `json:"displayName,omitempty" example:"My Project"`
	Description           string                      `json:"description,omitempty" example:"Development project for team alpha"`
	Members               []codespacev1.ProjectMember `json:"members,omitempty"`
	Namespaces            []string                    `json:"namespaces,omitempty"`
	ResourceQuotas        *codespacev1.ResourceQuotas `json:"resourceQuotas,omitempty"`
	DefaultSessionProfile *codespacev1.ProfileSpec    `json:"defaultSessionProfile,omitempty"`
	ImageAllowlist        []string                    `json:"imageAllowlist,omitempty"`
	ImageDenylist         []string                    `json:"imageDenylist,omitempty"`
}

// ProjectUpdateRequest represents the request body for updating a project
type ProjectUpdateRequest = ProjectCreateRequest

// ProjectListResponse wraps the project list with metadata
// @Description Response containing list of projects with metadata
type ProjectListResponse struct {
	Items      []codespacev1.CodespaceProject `json:"items"`
	Total      int                            `json:"total" example:"5"`
	Namespaces []string                       `json:"namespaces,omitempty" example:"default,team-alpha"`
	Filtered   bool                           `json:"filtered,omitempty"`
}

// handleProjectOperations handles the main project endpoint operations
func (h *handlers) handleProjectOperations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListProjects(w, r)
	case http.MethodPost:
		h.handleCreateProject(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectOperationsWithPath handles project operations that include namespace/name in path
func (h *handlers) handleProjectOperationsWithPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/server/projects/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/projects/{namespace}/{name}[/operation]", http.StatusBadRequest)
		return
	}

	// Check for member operations
	if len(parts) == 3 && parts[2] == "members" {
		h.handleProjectMembers(w, r)
		return
	}

	if len(parts) == 4 && parts[2] == "members" {
		h.handleProjectMemberOperations(w, r)
		return
	}

	// Regular CRUD operations on specific project
	switch r.Method {
	case http.MethodGet:
		h.handleGetProject(w, r)
	case http.MethodPut:
		h.handleUpdateProject(w, r)
	case http.MethodDelete:
		h.handleDeleteProject(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// @Summary List projects
// @ID listProjects
// @Description Get a list of codespace projects, optionally across all namespaces
// @Tags projects
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace query string false "Target namespace" default(default)
// @Param all query boolean false "List projects across all namespaces"
// @Success 200 {object} ProjectListResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/server/projects [get]
func (h *handlers) handleListProjects(w http.ResponseWriter, r *http.Request) {
	namespace := q(r, "namespace", "default")
	allNamespaces := r.URL.Query().Get("all") == "true"

	domain := namespace
	if allNamespaces {
		domain = "*"
	}
	cl, ok := mustCan(h.deps, w, r, "project", "list", domain)
	if !ok {
		return
	}

	var projects []codespacev1.CodespaceProject
	var namespaces []string

	if allNamespaces {
		var pl codespacev1.CodespaceProjectList
		opts := []client.ListOption{}
		if !h.deps.config.ClusterScope {
			opts = append(opts, client.MatchingLabels{InstanceIDLabel: h.deps.instanceID})
		}

		if err := h.deps.client.List(r.Context(), &pl, opts...); err != nil {
			logger.Error("Failed to list projects across all namespaces", "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to list projects: %w", err))
			return
		}
		nsSet := make(map[string]struct{})
		for _, p := range pl.Items {
			// keep RBAC namespace filter for non-admins
			if canAccess, err := h.deps.rbac.CanAccessNamespace(cl.Sub, cl.Roles, p.Namespace); err != nil || !canAccess {
				continue
			}
			projects = append(projects, p)
			nsSet[p.Namespace] = struct{}{}
		}
		for ns := range nsSet {
			namespaces = append(namespaces, ns)
		}
	} else {
		var projectList codespacev1.CodespaceProjectList
		opts := []client.ListOption{
			client.InNamespace(namespace),
		}
		if !h.deps.config.ClusterScope {
			opts = append(opts, client.MatchingLabels{InstanceIDLabel: h.deps.instanceID})
		}
		if err := h.deps.client.List(r.Context(), &projectList, opts...); err != nil {
			logger.Error("Failed to list projects", "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to list projects in namespace %s: %w", namespace, err))
			return
		}
		projects = projectList.Items
		namespaces = []string{namespace}
	}

	// Ensure Manager labels are present on every returned item
	var idx map[string]ManagerMeta
	if h.deps.config.ClusterScope {
		idx = h.buildInstanceMetaIndex(r)
	} else {
		idx = map[string]ManagerMeta{}
	}

	for i := range projects {
		p := &projects[i]
		if p.Labels == nil {
			p.Labels = map[string]string{}
		}
		// Best-effort: ensure instance-id is set on older objects in non-cluster mode
		if !h.deps.config.ClusterScope && p.Labels[InstanceIDLabel] == "" {
			p.Labels[InstanceIDLabel] = h.deps.instanceID
		}

		// Pick manager meta: index (cluster-scope) or server's own manager
		meta, ok := idx[p.Labels[InstanceIDLabel]]
		if !ok {
			meta = h.deps.manager
		}

		if p.Labels[LabelManagerKind] == "" && meta.Kind != "" {
			p.Labels[LabelManagerKind] = meta.Kind
		}
		if p.Labels[LabelManagerNamespace] == "" && meta.Namespace != "" {
			p.Labels[LabelManagerNamespace] = meta.Namespace
		}
		if p.Labels[LabelManagerName] == "" && meta.Name != "" {
			p.Labels[LabelManagerName] = meta.Name
		}

		// Update status counts
		p.Status.MemberCount = int32(len(p.Spec.Members))
	}

	writeJSON(w, ProjectListResponse{
		Items: projects, Total: len(projects), Namespaces: namespaces, Filtered: allNamespaces,
	})
}

// @Summary Create project
// @Description Create a new codespace project
// @Tags projects
// @Accept json
// @ID createProject
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param request body ProjectCreateRequest true "Project creation request"
// @Success 201 {object} codespacev1.CodespaceProject
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/server/projects [post]
func (h *handlers) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ProjectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "project name is required", http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	// Check RBAC permissions for the target namespace
	cl, ok := mustCan(h.deps, w, r, "project", "create", req.Namespace)
	if !ok {
		return
	}

	creatorID := SubjectToLabelID(cl.Sub)
	ann := map[string]string{
		"codespace.dev/created-at": time.Now().Format(time.RFC3339),
		AnnotationCreatedBy:        cl.Sub, // raw, reversible
	}

	// Add creator as owner if not in members list
	members := req.Members
	hasCreator := false
	for _, member := range members {
		if member.Subject == cl.Sub {
			hasCreator = true
			break
		}
	}
	if !hasCreator {
		now := metav1.Now()
		members = append([]codespacev1.ProjectMember{{
			Subject: cl.Sub,
			Role:    "owner",
			AddedAt: &now,
			AddedBy: cl.Sub,
		}}, members...)
	}

	// Construct the project object
	project := &codespacev1.CodespaceProject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: codespacev1.GroupVersion.String(),
			Kind:       "CodespaceProject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "codespace-project",
				"app.kubernetes.io/instance":   req.Name,
				"app.kubernetes.io/managed-by": APP_NAME,
				LabelCreatedBy:                 creatorID,
				InstanceIDLabel:                h.deps.instanceID,
				// Manager identity
				LabelManagerKind:      h.deps.manager.Kind,
				LabelManagerNamespace: h.deps.manager.Namespace,
				LabelManagerName:      h.deps.manager.Name,
			},
			Annotations: ann,
		},
		Spec: codespacev1.ProjectSpec{
			DisplayName:           req.DisplayName,
			Description:           req.Description,
			Members:               members,
			Namespaces:            req.Namespaces,
			ResourceQuotas:        req.ResourceQuotas,
			DefaultSessionProfile: req.DefaultSessionProfile,
			ImageAllowlist:        req.ImageAllowlist,
			ImageDenylist:         req.ImageDenylist,
		},
		Status: codespacev1.ProjectStatus{
			Phase:       "Active",
			MemberCount: int32(len(members)),
			LastUpdated: &metav1.Time{Time: time.Now()},
		},
	}

	if err := h.deps.client.Create(r.Context(), project); err != nil {
		logger.Error("Failed to create project", "name", req.Name, "namespace", req.Namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to create project: %w", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, project)
}

// @Summary Get project
// @Description Get details of a specific project
// @Tags projects
// @Accept json
// @ID getProject
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Project name"
// @Success 200 {object} codespacev1.CodespaceProject
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/projects/{namespace}/{name} [get]
func (h *handlers) handleGetProject(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/projects/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/projects/{namespace}/{name}", http.StatusBadRequest)
		return
	}

	namespace, name := parts[0], parts[1]

	// Check RBAC permissions
	cl, ok := mustCan(h.deps, w, r, "project", "get", namespace)
	if !ok {
		return
	}

	var project codespacev1.CodespaceProject
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &project); err != nil {
		logger.Error("Failed to get project", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("project not found: %w", err))
		return
	}

	if project.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, project)
}

// @Summary Update project
// @Description Update a project (full replacement)
// @ID updateProject
// @Tags projects
// @Accept json
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Project name"
// @Param request body ProjectUpdateRequest true "Project update request"
// @Success 200 {object} codespacev1.CodespaceProject
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/projects/{namespace}/{name} [put]
func (h *handlers) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/projects/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/projects/{namespace}/{name}", http.StatusBadRequest)
		return
	}

	namespace, name := parts[0], parts[1]

	// Check RBAC permissions
	cl, ok := mustCan(h.deps, w, r, "project", "update", namespace)
	if !ok {
		return
	}

	// Get the current project
	var project codespacev1.CodespaceProject
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &project); err != nil {
		logger.Error("Failed to get project for update", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("project not found: %w", err))
		return
	}

	// Check if project belongs to this instance
	if project.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req ProjectUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Update spec (preserve metadata)
	project.Spec = codespacev1.ProjectSpec{
		DisplayName:           req.DisplayName,
		Description:           req.Description,
		Members:               req.Members,
		Namespaces:            req.Namespaces,
		ResourceQuotas:        req.ResourceQuotas,
		DefaultSessionProfile: req.DefaultSessionProfile,
		ImageAllowlist:        req.ImageAllowlist,
		ImageDenylist:         req.ImageDenylist,
	}

	// Update status
	project.Status.MemberCount = int32(len(req.Members))
	project.Status.LastUpdated = &metav1.Time{Time: time.Now()}

	// Add update metadata
	if project.Annotations == nil {
		project.Annotations = make(map[string]string)
	}
	project.Annotations["codespace.dev/updated-at"] = time.Now().Format(time.RFC3339)
	project.Annotations["codespace.dev/updated-by"] = cl.Sub

	// Update with retry logic
	if err := helpers.RetryOnConflict(func() error {
		return h.deps.client.Update(r.Context(), &project)
	}); err != nil {
		logger.Error("Failed to update project", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to update project: %w", err))
		return
	}

	logger.Info("Updated project", "name", name, "namespace", namespace, "user", cl.Sub)
	writeJSON(w, project)
}

// @Summary Delete project
// @Description Delete a codespace project
// @Tags projects
// @Accept json
// @Produce json
// @ID deleteProject
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Project name"
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/projects/{namespace}/{name} [delete]
func (h *handlers) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/projects/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/projects/{namespace}/{name}", http.StatusBadRequest)
		return
	}
	namespace, name := parts[0], parts[1]

	// RBAC
	cl, ok := mustCan(h.deps, w, r, "project", "delete", namespace)
	if !ok {
		return
	}

	// FETCH then verify instance-id before deleting
	var project codespacev1.CodespaceProject
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &project); err != nil {
		logger.Error("Failed to get project for delete", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("project not found: %w", err))
		return
	}
	if project.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := h.deps.client.Delete(r.Context(), &project); err != nil {
		logger.Error("Failed to delete project", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to delete project: %w", err))
		return
	}

	logger.Info("Deleted project", "name", name, "namespace", namespace, "user", cl.Sub)
	writeJSON(w, map[string]string{"status": "deleted", "name": name, "namespace": namespace})
}

// Member management handlers (simplified for now)
func (h *handlers) handleProjectMembers(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement add member functionality
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (h *handlers) handleProjectMemberOperations(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement remove member functionality
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
