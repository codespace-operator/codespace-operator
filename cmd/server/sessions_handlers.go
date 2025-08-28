package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

// handleSessionOperations handles the main session endpoint operations
func handleSessionOperations(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListSessions(deps)(w, r)
		case http.MethodPost:
			handleCreateSession(deps)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handleSessionOperationsWithPath handles session operations that include namespace/name in path
func handleSessionOperationsWithPath(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the path to determine the operation
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/")
		parts := strings.Split(path, "/")

		if len(parts) < 2 {
			http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}[/operation]", http.StatusBadRequest)
			return
		}

		// Check if this is a scale operation
		if len(parts) == 3 && parts[2] == "scale" {
			handleScaleSession(deps)(w, r)
			return
		}

		// Regular CRUD operations on specific session
		switch r.Method {
		case http.MethodGet:
			handleGetSession(deps)(w, r)
		case http.MethodPut, http.MethodPatch:
			handleUpdateSession(deps)(w, r)
		case http.MethodDelete:
			handleDeleteSession(deps)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// CRUD Operations for Sessions

// handleListSessions - GET /api/v1/server/sessions
func handleListSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine target namespace(s)
		namespace := q(r, "namespace", "default")
		allNamespaces := r.URL.Query().Get("all") == "true"

		// For "all namespaces", use "*" as domain
		domain := namespace
		if allNamespaces {
			domain = "*"
		}

		// Check RBAC permissions
		cl, ok := mustCan(deps, w, r, "session", "list", domain)
		if !ok {
			return
		}

		var sessions []codespacev1.Session
		var namespaces []string

		if allNamespaces {
			// List across all namespaces (requires cluster-level or * domain permission)
			ul, err := deps.dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{})
			if err != nil {
				logger.Error("Failed to list sessions across all namespaces", "err", err, "user", cl.Sub)
				errJSON(w, fmt.Errorf("failed to list sessions: %w", err))
				return
			}

			// Convert unstructured to typed sessions and collect unique namespaces
			nsSet := make(map[string]struct{})
			for _, item := range ul.Items {
				var session codespacev1.Session
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &session); err != nil {
					logger.Warn("Failed to convert session", "name", item.GetName(), "namespace", item.GetNamespace(), "err", err)
					continue
				}

				// Apply namespace-level filtering for non-admin users
				if canAccess, err := deps.rbac.CanAccessNamespace(cl.Sub, cl.Roles, session.Namespace); err != nil || !canAccess {
					continue // Skip sessions in namespaces user can't access
				}

				sessions = append(sessions, session)
				nsSet[session.Namespace] = struct{}{}
			}

			// Convert namespace set to slice
			for ns := range nsSet {
				namespaces = append(namespaces, ns)
			}
		} else {
			// List within specific namespace
			var sessionList codespacev1.SessionList
			if err := deps.typed.List(r.Context(), &sessionList, client.InNamespace(namespace)); err != nil {
				logger.Error("Failed to list sessions in namespace", "namespace", namespace, "err", err, "user", cl.Sub)
				errJSON(w, fmt.Errorf("failed to list sessions in namespace %s: %w", namespace, err))
				return
			}

			sessions = sessionList.Items
			namespaces = []string{namespace}
		}

		response := SessionListResponse{
			Items:      sessions,
			Total:      len(sessions),
			Namespaces: namespaces,
			Filtered:   allNamespaces, // Indicate if results were filtered by RBAC
		}

		logger.Info("Listed sessions", "count", len(sessions), "namespaces", len(namespaces), "user", cl.Sub)
		writeJSON(w, response)
	}
}

// handleCreateSession - POST /api/v1/server/sessions
func handleCreateSession(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		cl, ok := mustCan(deps, w, r, "session", "create", req.Namespace)
		if !ok {
			return
		}

		// Construct the session object
		session := &codespacev1.Session{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "codespace-session",
					"app.kubernetes.io/instance":   req.Name,
					"app.kubernetes.io/managed-by": "codespace-operator",
					"codespace.dev/created-by":     cl.Sub,
				},
				Annotations: map[string]string{
					"codespace.dev/created-at": time.Now().Format(time.RFC3339),
					"codespace.dev/created-by": cl.Sub,
				},
			},
			Spec: codespacev1.SessionSpec{
				Profile:    req.Profile,
				Auth:       codespacev1.AuthSpec{Mode: "none"}, // Default auth mode
				Home:       req.Home,
				Scratch:    req.Scratch,
				Networking: req.Network,
				Replicas:   req.Replicas,
			},
		}

		// Set auth if provided
		if req.Auth != nil {
			session.Spec.Auth = *req.Auth
		}

		// Set default replicas if not specified
		if session.Spec.Replicas == nil {
			defaultReplicas := int32(1)
			session.Spec.Replicas = &defaultReplicas
		}

		// Create the session using the dynamic client for better error handling
		u := &unstructured.Unstructured{}
		u.Object, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(session)

		created, err := deps.dyn.Resource(gvr).Namespace(req.Namespace).Create(r.Context(), u, metav1.CreateOptions{})
		if err != nil {
			logger.Error("Failed to create session", "name", req.Name, "namespace", req.Namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to create session: %w", err))
			return
		}

		logger.Info("Created session", "name", req.Name, "namespace", req.Namespace, "user", cl.Sub)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, created.Object)
	}
}

// handleGetSession - GET /api/v1/server/sessions/{namespace}/{name}
func handleGetSession(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/"), "/")
		if len(parts) < 2 {
			http.Error(w, "invalid path - expected /api/v1/server/sessions/{namespace}/{name}", http.StatusBadRequest)
			return
		}

		namespace, name := parts[0], parts[1]

		// Check RBAC permissions
		cl, ok := mustCan(deps, w, r, "session", "get", namespace)
		if !ok {
			return
		}

		var session codespacev1.Session
		if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
			logger.Error("Failed to get session", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("session not found: %w", err))
			return
		}

		logger.Debug("Retrieved session", "name", name, "namespace", namespace, "user", cl.Sub)
		writeJSON(w, session)
	}
}

// handleDeleteSession - DELETE /api/v1/server/sessions/{namespace}/{name}
func handleDeleteSession(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		// Check RBAC permissions
		cl, ok := mustCan(deps, w, r, "session", "delete", namespace)
		if !ok {
			return
		}

		// Use the dynamic client for deletion to get better error details
		err := deps.dyn.Resource(gvr).Namespace(namespace).Delete(r.Context(), name, metav1.DeleteOptions{})
		if err != nil {
			logger.Error("Failed to delete session", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to delete session: %w", err))
			return
		}

		logger.Info("Deleted session", "name", name, "namespace", namespace, "user", cl.Sub)
		writeJSON(w, map[string]string{
			"status":    "deleted",
			"name":      name,
			"namespace": namespace,
		})
	}
}

// handleScaleSession - POST /api/v1/server/sessions/{namespace}/{name}/scale
func handleScaleSession(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		cl, ok := mustCan(deps, w, r, "session", "scale", namespace)
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
		if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
			logger.Error("Failed to get session for scaling", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("session not found: %w", err))
			return
		}

		// Update replicas with retry logic for conflicts
		session.Spec.Replicas = &req.Replicas
		if err := helpers.RetryOnConflict(func() error {
			return deps.typed.Update(r.Context(), &session)
		}); err != nil {
			logger.Error("Failed to scale session", "name", name, "namespace", namespace, "replicas", req.Replicas, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to scale session: %w", err))
			return
		}

		logger.Info("Scaled session", "name", name, "namespace", namespace, "replicas", req.Replicas, "user", cl.Sub)
		writeJSON(w, session)
	}
}

// handleUpdateSession - PUT/PATCH /api/v1/server/sessions/{namespace}/{name}
func handleUpdateSession(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		cl, ok := mustCan(deps, w, r, "session", "update", namespace)
		if !ok {
			return
		}

		// Get the current session
		var session codespacev1.Session
		if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &session); err != nil {
			logger.Error("Failed to get session for update", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("session not found: %w", err))
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

			// Apply selective updates - this is a simplified implementation
			// In production, you'd want more sophisticated JSON patch logic
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
			return deps.typed.Update(r.Context(), &session)
		}); err != nil {
			logger.Error("Failed to update session", "name", name, "namespace", namespace, "err", err, "user", cl.Sub)
			errJSON(w, fmt.Errorf("failed to update session: %w", err))
			return
		}

		logger.Info("Updated session", "name", name, "namespace", namespace, "user", cl.Sub)
		writeJSON(w, session)
	}
}

// Stream handlers for real-time updates

// handleStreamSessions - GET /api/v1/stream/sessions (Server-Sent Events)
func handleStreamSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine domain for RBAC
		namespace := q(r, "namespace", "default")
		allNamespaces := r.URL.Query().Get("all") == "true"
		domain := namespace
		if allNamespaces {
			domain = "*"
		}

		// Check RBAC permissions
		cl, ok := mustCan(deps, w, r, "session", "watch", domain)
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

		if allNamespaces {
			watcher, err = deps.dyn.Resource(gvr).Watch(r.Context(), metav1.ListOptions{Watch: true})
		} else {
			watcher, err = deps.dyn.Resource(gvr).Namespace(namespace).Watch(r.Context(), metav1.ListOptions{Watch: true})
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
					if canAccess, err := deps.rbac.CanAccessNamespace(cl.Sub, cl.Roles, session.Namespace); err != nil || !canAccess {
						continue // Skip sessions in namespaces user can't access
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
}
