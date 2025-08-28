package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

// Request/Response types for OpenAPI documentation
// These only exist for swagger generation - keep them simple

// SessionCreateRequest represents the request body for creating a session
// @Description Request body for creating a new codespace session
type SessionCreateRequest struct {
	Name      string                  `json:"name" validate:"required" example:"my-session"`
	Namespace string                  `json:"namespace" example:"default"`
	Profile   codespacev1.ProfileSpec `json:"profile" validate:"required"`
	Auth      *codespacev1.AuthSpec   `json:"auth,omitempty"`
	Home      *codespacev1.PVCSpec    `json:"home,omitempty"`
	Scratch   *codespacev1.PVCSpec    `json:"scratch,omitempty"`
	Network   *codespacev1.NetSpec    `json:"networking,omitempty"`
	Replicas  *int32                  `json:"replicas,omitempty" example:"1"`
}

// SessionScaleRequest represents the request body for scaling a session
// @Description Request body for scaling a session
type SessionScaleRequest struct {
	Replicas int32 `json:"replicas" validate:"min=0" example:"2"`
}

// SessionListResponse wraps the session list with metadata
// @Description Response containing list of sessions with metadata
type SessionListResponse struct {
	Items      []codespacev1.Session `json:"items"`
	Total      int                   `json:"total" example:"5"`
	Namespaces []string              `json:"namespaces,omitempty" example:"default,kube-system"`
	Filtered   bool                  `json:"filtered,omitempty"`
}

// handleSessionOperations handles the main session endpoint operations
func (h *handlers) handleSessionOperations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListSessions(w, r)
	case http.MethodPost:
		h.handleCreateSession(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionOperationsWithPath handles session operations that include namespace/name in path
func (h *handlers) handleSessionOperationsWithPath(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine the operation
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}[/operation]", http.StatusBadRequest)
		return
	}

	// Check if this is a scale operation
	if len(parts) == 3 && parts[2] == "scale" {
		h.handleScaleSession(w, r)
		return
	}

	// Regular CRUD operations on specific session
	switch r.Method {
	case http.MethodGet:
		h.handleGetSession(w, r)
	case http.MethodPut, http.MethodPatch:
		h.handleUpdateSession(w, r)
	case http.MethodDelete:
		h.handleDeleteSession(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// CRUD Operations for Sessions

// @Summary List sessions
// @ID listSessions
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
func (h *handlers) handleListSessions(w http.ResponseWriter, r *http.Request) {
	namespace := q(r, "namespace", "default")
	allNamespaces := r.URL.Query().Get("all") == "true"

	domain := namespace
	if allNamespaces {
		domain = "*"
	}
	cl, ok := mustCan(h.deps, w, r, "session", "list", domain)
	if !ok {
		return
	}

	var sessions []codespacev1.Session
	var namespaces []string

	if allNamespaces {
		var sl codespacev1.SessionList
		opts := []client.ListOption{}
		if !h.deps.config.ClusterScope {
			opts = append(opts, client.MatchingLabels{InstanceIDLabel: h.deps.instanceID})
		}

		// ↓ Push label filter to API
		if err := h.deps.client.List(r.Context(), &sl, opts...); err != nil {
			logger.Error("Failed to list sessions across all namespaces", "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to list sessions: %w", err))
			return
		}
		nsSet := make(map[string]struct{})
		for _, s := range sl.Items {
			// keep RBAC namespace filter for non-admins
			if canAccess, err := h.deps.rbac.CanAccessNamespace(cl.Sub, cl.Roles, s.Namespace); err != nil || !canAccess {
				continue
			}
			sessions = append(sessions, s)
			nsSet[s.Namespace] = struct{}{}
		}
		for ns := range nsSet {
			namespaces = append(namespaces, ns)
		}
	} else {
		var sessionList codespacev1.SessionList
		opts := []client.ListOption{
			client.InNamespace(namespace),
		}
		if !h.deps.config.ClusterScope {
			opts = append(opts, client.MatchingLabels{InstanceIDLabel: h.deps.instanceID})
		}
		if err := h.deps.client.List(
			r.Context(),
			&sessionList,
			opts...,
		); err != nil {
			logger.Error("Failed to list sessions", "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to list sessions in namespace %s: %w", namespace, err))
			return
		}
		sessions = sessionList.Items
		namespaces = []string{namespace}
	}

	writeJSON(w, SessionListResponse{
		Items: sessions, Total: len(sessions), Namespaces: namespaces, Filtered: allNamespaces,
	})
}

// @Summary Create session
// @Description Create a new codespace session
// @Tags sessions
// @Accept json
// @ID createSession
// @Produce json
// @Security BearerAuth
// @Security CookieAuth
// @Param request body SessionCreateRequest true "Session creation request"
// @Success 201 {object} codespacev1.Session
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/server/sessions [post]
func (h *handlers) handleCreateSession(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SessionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "session name is required", http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Profile.IDE == "" {
		http.Error(w, "IDE profile is required", http.StatusBadRequest)
		return
	}
	if req.Profile.Image == "" {
		http.Error(w, "container image is required", http.StatusBadRequest)
		return
	}

	// Check RBAC permissions for the target namespace
	cl, ok := mustCan(h.deps, w, r, "session", "create", req.Namespace)
	if !ok {
		return
	}

	creatorID := SubjectToLabelID(cl.Sub)
	ann := map[string]string{
		"codespace.dev/created-at": time.Now().Format(time.RFC3339),
		AnnotationCreatedBy:        cl.Sub, // raw, reversible
	}
	// Construct the session object
	session := &codespacev1.Session{
		TypeMeta: metav1.TypeMeta{
			APIVersion: codespacev1.GroupVersion.String(),
			Kind:       "Session",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "codespace-session",
				"app.kubernetes.io/instance":   req.Name,
				"app.kubernetes.io/managed-by": "codespace-operator",
				LabelCreatedBy:                 creatorID,
				InstanceIDLabel:                h.deps.instanceID,
			},
			Annotations: ann,
		},
		Spec: codespacev1.SessionSpec{
			Profile:    req.Profile,
			Auth:       codespacev1.AuthSpec{Mode: "none"},
			Home:       req.Home,
			Scratch:    req.Scratch,
			Networking: req.Network,
			Replicas:   req.Replicas,
		},
	}

	if req.Auth != nil {
		session.Spec.Auth = *req.Auth
	}
	if session.Spec.Replicas == nil {
		def := int32(1)
		session.Spec.Replicas = &def
	}

	if err := h.deps.client.Create(r.Context(), session); err != nil {
		logger.Error("Failed to create session", "name", req.Name, "namespace", req.Namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to create session: %w", err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, session)
}

// @Summary Get session
// @Description Get details of a specific session
// @Tags sessions
// @Accept json
// @ID getSession
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
func (h *handlers) handleGetSession(w http.ResponseWriter, r *http.Request) {

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}", http.StatusBadRequest)
		return
	}

	namespace, name := parts[0], parts[1]

	// Check RBAC permissions
	cl, ok := mustCan(h.deps, w, r, "session", "get", namespace)
	if !ok {
		return
	}

	var session codespacev1.Session
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
		logger.Error("Failed to get session", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("session not found: %w", err))
		return
	}

	if session.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, session)
}

// @Summary Delete session
// @Description Delete a codespace session
// @Tags sessions
// @Accept json
// @Produce json
// @ID deleteSession
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace path string true "Namespace"
// @Param name path string true "Session name"
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/server/sessions/{namespace}/{name} [delete]
func (h *handlers) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}", http.StatusBadRequest)
		return
	}
	namespace, name := parts[0], parts[1]

	// RBAC
	cl, ok := mustCan(h.deps, w, r, "session", "delete", namespace)
	if !ok {
		return
	}

	// FETCH then verify instance-id before deleting
	var session codespacev1.Session
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
		logger.Error("Failed to get session for delete", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("session not found: %w", err))
		return
	}
	if session.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := h.deps.client.Delete(r.Context(), &session); err != nil {
		logger.Error("Failed to delete session", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to delete session: %w", err))
		return
	}

	logger.Info("Deleted session", "name", name, "namespace", namespace, "user", cl.Sub)
	writeJSON(w, map[string]string{"status": "deleted", "name": name, "namespace": namespace})
}

// @Summary Scale session
// @ID scaleSession
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
func (h *handlers) handleScaleSession(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/"), "/")
	if len(parts) < 3 || parts[2] != "scale" {
		http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}/scale", http.StatusBadRequest)
		return
	}

	namespace, name := parts[0], parts[1]

	// Check RBAC permissions
	cl, ok := mustCan(h.deps, w, r, "session", "scale", namespace)
	if !ok {
		return
	}

	var req SessionScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Validate replicas value
	if req.Replicas < 0 {
		http.Error(w, "replicas cannot be negative", http.StatusBadRequest)
		return
	}

	// Get the current session
	var session codespacev1.Session
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
		logger.Error("Failed to get session for scaling", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("session not found: %w", err))
		return
	}

	// Check if CR belongs to this instance
	if session.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Update replicas with retry logic for conflicts
	session.Spec.Replicas = &req.Replicas
	if err := helpers.RetryOnConflict(func() error {
		return h.deps.client.Update(r.Context(), &session)
	}); err != nil {
		logger.Error("Failed to scale session", "name", name, "namespace", namespace, "replicas", req.Replicas, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to scale session: %w", err))
		return
	}

	logger.Info("Scaled session", "name", name, "namespace", namespace, "replicas", req.Replicas, "user", cl.Sub)
	writeJSON(w, session)
}

// @Summary Update session
// @Description Update a session (full replacement)
// @ID updateSession
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
func (h *handlers) handleUpdateSession(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/"), "/")
	if len(parts) < 2 {
		http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}", http.StatusBadRequest)
		return
	}

	namespace, name := parts[0], parts[1]

	// Check RBAC permissions
	cl, ok := mustCan(h.deps, w, r, "session", "update", namespace)
	if !ok {
		return
	}

	// Get the current session
	var session codespacev1.Session
	if err := h.deps.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
		logger.Error("Failed to get session for update", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("session not found: %w", err))
		return
	}

	// Check if CR belongs to this instance
	if session.Labels[InstanceIDLabel] != h.deps.instanceID && !h.deps.config.ClusterScope {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if r.Method == http.MethodPut {
		// Full replacement
		var req SessionCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		// Preserve metadata but update spec
		session.Spec = codespacev1.SessionSpec{
			Profile:    req.Profile,
			Auth:       codespacev1.AuthSpec{Mode: "none"},
			Home:       req.Home,
			Scratch:    req.Scratch,
			Networking: req.Network,
			Replicas:   req.Replicas,
		}

		if req.Auth != nil {
			session.Spec.Auth = *req.Auth
		}
	} else {
		// Partial update (PATCH)
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if session.Labels == nil {
			session.Labels = map[string]string{}
		}
		session.Labels[LabelCreatedBy] = SubjectToLabelID(session.Annotations[AnnotationCreatedBy])
		// Apply selective updates - simplified implementation
		if profile, ok := updates["profile"]; ok {
			if profileData, err := json.Marshal(profile); err == nil {
				json.Unmarshal(profileData, &session.Spec.Profile)
			}
		}
		if replicas, ok := updates["replicas"]; ok {
			if replicasInt, ok := replicas.(float64); ok {
				replicasInt32 := int32(replicasInt)
				session.Spec.Replicas = &replicasInt32
			}
		}
	}

	// Add update metadata
	if session.Annotations == nil {
		session.Annotations = make(map[string]string)
	}
	session.Annotations["codespace.dev/updated-at"] = time.Now().Format(time.RFC3339)
	session.Annotations["codespace.dev/updated-by"] = cl.Sub

	// Update with retry logic
	if err := helpers.RetryOnConflict(func() error {
		return h.deps.client.Update(r.Context(), &session)
	}); err != nil {
		logger.Error("Failed to update session", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to update session: %w", err))
		return
	}

	logger.Info("Updated session", "name", name, "namespace", namespace, "user", cl.Sub)
	writeJSON(w, session)
}

// @Summary Stream sessions
// @Description Stream real-time session updates via Server-Sent Events
// @Tags sessions
// /@ID streamSessions
// @Produce text/event-stream
// @Security BearerAuth
// @Security CookieAuth
// @Param namespace query string false "Target namespace" default(default)
// @Param all query boolean false "Stream sessions from all namespaces"
// @Success 200 {string} string "SSE stream"
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/stream/sessions [get]
// Determine domain for RBAC
func (h *handlers) handleStreamSessions(w http.ResponseWriter, r *http.Request) {

	namespace := q(r, "namespace", "default")
	allNamespaces := r.URL.Query().Get("all") == "true"
	domain := namespace
	if allNamespaces {
		domain = "*"
	}

	// Check RBAC permissions
	cl, ok := mustCan(h.deps, w, r, "session", "watch", domain)
	if !ok {
		return
	}

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Start watching
	var watcher watch.Interface
	var err error
	sel := labels.Set{"codespace.dev/instance-id": h.deps.instanceID}.AsSelector().String()

	if allNamespaces {
		watcher, err = h.deps.dyn.Resource(gvr).Watch(
			r.Context(),
			metav1.ListOptions{Watch: true, LabelSelector: sel},
		)
	} else {
		watcher, err = h.deps.dyn.Resource(gvr).Namespace(namespace).Watch(
			r.Context(),
			metav1.ListOptions{Watch: true, LabelSelector: sel},
		)
	}

	if err != nil {
		logger.Error("Failed to start session watch", "namespace", namespace, "all", allNamespaces, "err", err, "user", cl.Sub)
		errJSON(w, fmt.Errorf("failed to start watch: %w", err))
		return
	}
	defer watcher.Stop()

	// Send initial ping
	writeSSE(w, "ping", map[string]string{"status": "connected"})
	flusher.Flush()

	logger.Info("Started session stream", "namespace", namespace, "all", allNamespaces, "user", cl.Sub)

	// Keep-alive ticker
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			logger.Debug("Session stream ended by client", "user", cl.Sub)
			return
		case <-ticker.C:
			writeSSE(w, "ping", map[string]string{"timestamp": time.Now().Format(time.RFC3339)})
			flusher.Flush()
		case ev, ok := <-watcher.ResultChan():
			if !ok {
				logger.Debug("Session stream ended by server", "user", cl.Sub)
				return
			}

			if ev.Type == watch.Error {
				logger.Error("Watch error in session stream", "err", ev.Object, "user", cl.Sub)
				continue
			}

			u, ok := ev.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			// Convert to typed session
			var session codespacev1.Session
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &session); err != nil {
				logger.Warn("Failed to convert session in stream", "name", u.GetName(), "err", err)
				continue
			}

			// Apply namespace-level filtering for cross-namespace watches
			if allNamespaces {
				if canAccess, err := h.deps.rbac.CanAccessNamespace(cl.Sub, cl.Roles, session.Namespace); err != nil || !canAccess {
					continue
				}
			}

			// Send the event
			payload := map[string]interface{}{
				"type":   string(ev.Type),
				"object": session,
			}
			writeSSE(w, "message", payload)
			flusher.Flush()
		}
	}
}

func (h *handlers) handleGetAllSubjectSessions(w http.ResponseWriter, r *http.Request) {
	// Implementation for getting all sessions for a subject
	// Verify Casbin RBAC
	// Fetch all cl.subject sessions across all namespace
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
