// cmd/server/rbac.go - Enhanced version
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v2"
	cmodel "github.com/casbin/casbin/v2/model"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/fsnotify/fsnotify"
	authv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultModelPath  = "/etc/codespace-operator/rbac/model.conf"
	defaultPolicyPath = "/etc/codespace-operator/rbac/policy.csv"
	envModelPath      = "CODESPACE_SERVER_RBAC_MODEL_PATH"
	envPolicyPath     = "CODESPACE_SERVER_RBAC_POLICY_PATH"
)

// RBAC provides Casbin-based authorization with hot reload.
type RBAC struct {
	mu         sync.RWMutex
	enf        *casbin.Enforcer
	modelPath  string
	policyPath string
}

// PermissionCheck represents a single permission check result
type PermissionCheck struct {
	Resource  string `json:"resource"`
	Action    string `json:"action"`
	Namespace string `json:"namespace"`
	Allowed   bool   `json:"allowed"`
}

// UserPermissions represents all permissions for a user
type UserPermissions struct {
	Subject     string              `json:"subject"`
	Roles       []string            `json:"roles"`
	Permissions []PermissionCheck   `json:"permissions"`
	Namespaces  map[string][]string `json:"namespaces"` // namespace -> allowed actions
}

// NewRBACFromEnv loads model/policy from disk and starts a file watcher
func NewRBACFromEnv(ctx context.Context) (*RBAC, error) {
	modelPath := strings.TrimSpace(os.Getenv(envModelPath))
	if modelPath == "" {
		modelPath = defaultModelPath
	}
	policyPath := strings.TrimSpace(os.Getenv(envPolicyPath))
	if policyPath == "" {
		policyPath = defaultPolicyPath
	}

	r := &RBAC{modelPath: modelPath, policyPath: policyPath}
	if err := r.reload(); err != nil {
		return nil, fmt.Errorf("initial RBAC load failed: %w", err)
	}

	// Watch both files + their parent dir (K8s ConfigMap mounts replace symlinks)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	watchPaths := uniqueNonEmpty([]string{
		modelPath,
		policyPath,
		filepath.Dir(modelPath),
		filepath.Dir(policyPath),
	})

	for _, p := range watchPaths {
		_ = watcher.Add(p) // best-effort
	}

	go func() {
		defer watcher.Close()

		// Simple debounce to coalesce flurries of writes from kubelet
		var last time.Time
		const debounce = 250 * time.Millisecond

		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-watcher.Events:
				// React to any change on either file path or their dirs
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) == 0 {
					continue
				}
				now := time.Now()
				if now.Sub(last) < debounce {
					continue
				}
				last = now
				if err := r.reload(); err != nil && logger != nil {
					logger.Info("rbac reload failed", "err", err)
				} else if logger != nil {
					logger.Info("rbac reloaded after fsnotify", "event", ev.Name)
				}
			case err := <-watcher.Errors:
				if logger != nil && err != nil {
					logger.Info("rbac watcher error", "err", err)
				}
			}
		}
	}()

	return r, nil
}

// reload rebuilds the Enforcer from the current files.
func (r *RBAC) reload() error {
	// Load model
	m, err := cmodel.NewModelFromFile(r.modelPath)
	if err != nil {
		return fmt.Errorf("failed to load model from %s: %w", r.modelPath, err)
	}

	// Load policy via file adapter
	adapter := fileadapter.NewAdapter(r.policyPath)
	enf, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return fmt.Errorf("failed to create enforcer: %w", err)
	}

	// Enable logging in debug builds
	if os.Getenv("CASBIN_LOG_ENABLED") == "1" {
		enf.EnableLog(true)
	}

	r.mu.Lock()
	r.enf = enf
	r.mu.Unlock()

	if logger != nil {
		logger.Info("RBAC policies reloaded successfully")
	}
	return nil
}

// Enforce checks (sub,obj,act,dom) with support for a user's explicit roles.
func (r *RBAC) Enforce(sub string, roles []string, obj, act, dom string) (bool, error) {
	r.mu.RLock()
	enf := r.enf
	r.mu.RUnlock()

	if enf == nil {
		return false, errors.New("rbac not initialized")
	}

	// Log the enforcement request in debug mode
	if os.Getenv("CASBIN_LOG_ENABLED") == "1" && logger != nil {
		logger.Debug("RBAC enforce", "subject", sub, "roles", roles, "object", obj, "action", act, "domain", dom)
	}

	// Try the concrete identity first
	ok, err := enf.Enforce(sub, obj, act, dom)
	if err != nil {
		return false, fmt.Errorf("enforcement failed for subject %s: %w", sub, err)
	}
	if ok {
		return true, nil
	}

	// Then try each role as a subject
	for _, role := range roles {
		if role == "" {
			continue
		}
		ok, err = enf.Enforce(role, obj, act, dom)
		if err != nil {
			return false, fmt.Errorf("enforcement failed for role %s: %w", role, err)
		}
		if ok {
			return true, nil
		}
	}

	return false, nil
}

// EnforceAny checks alternative identities.
func (r *RBAC) EnforceAny(subjects []string, roles []string, obj, act, dom string) (bool, error) {
	r.mu.RLock()
	enf := r.enf
	r.mu.RUnlock()
	if enf == nil {
		return false, errors.New("rbac not initialized")
	}

	// Try concrete subjects (sub, email, username)
	for _, s := range uniqueNonEmpty(subjects) {
		if ok, err := enf.Enforce(s, obj, act, dom); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	// Then try roles as subjects (leveraging g(role, roleAlias) in matcher)
	for _, role := range uniqueNonEmpty(roles) {
		if ok, err := enf.Enforce(role, obj, act, dom); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
	}
	return false, nil
}

// EnforceAsSubject checks permissions for a specific subject (used for role introspection)
func (r *RBAC) EnforceAsSubject(sub, obj, act, dom string) (bool, error) {
	r.mu.RLock()
	enf := r.enf
	r.mu.RUnlock()

	if enf == nil {
		return false, errors.New("rbac not initialized")
	}

	return enf.Enforce(sub, obj, act, dom)
}

// GetUserPermissions returns comprehensive permission information for a user
func (r *RBAC) GetUserPermissions(subject string, roles []string, namespaces []string, actions []string) (*UserPermissions, error) {
	r.mu.RLock()
	enf := r.enf
	r.mu.RUnlock()

	if enf == nil {
		return nil, errors.New("rbac not initialized")
	}

	if len(actions) == 0 {
		actions = []string{"get", "list", "watch", "create", "update", "delete", "scale"}
	}

	permissions := &UserPermissions{
		Subject:     subject,
		Roles:       roles,
		Permissions: make([]PermissionCheck, 0),
		Namespaces:  make(map[string][]string),
	}

	// Check each combination of resource, action, and namespace
	for _, ns := range namespaces {
		allowedActions := make([]string, 0)

		for _, action := range actions {
			allowed, err := r.Enforce(subject, roles, "session", action, ns)
			if err != nil {
				return nil, fmt.Errorf("failed to check permission for %s/%s/%s: %w", subject, action, ns, err)
			}

			permissions.Permissions = append(permissions.Permissions, PermissionCheck{
				Resource:  "session",
				Action:    action,
				Namespace: ns,
				Allowed:   allowed,
			})

			if allowed {
				allowedActions = append(allowedActions, action)
			}
		}

		if len(allowedActions) > 0 {
			permissions.Namespaces[ns] = allowedActions
		}
	}

	return permissions, nil
}

// GetAllowedNamespaces returns namespaces where the user has at least one permission
func (r *RBAC) GetAllowedNamespaces(subject string, roles []string, namespaces []string, action string) ([]string, error) {
	allowed := make([]string, 0)

	for _, ns := range namespaces {
		ok, err := r.Enforce(subject, roles, "session", action, ns)
		if err != nil {
			return nil, fmt.Errorf("failed to check namespace %s: %w", ns, err)
		}
		if ok {
			allowed = append(allowed, ns)
		}
	}

	return allowed, nil
}

// CanAccessNamespace checks if user has any session permissions in a namespace
func (r *RBAC) CanAccessNamespace(subject string, roles []string, namespace string) (bool, error) {
	actions := []string{"get", "list", "watch", "create", "update", "delete", "scale"}

	for _, action := range actions {
		ok, err := r.Enforce(subject, roles, "session", action, namespace)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}

	return false, nil
}

// GetRolesForUser returns all roles (including inherited) for a user
func (r *RBAC) GetRolesForUser(subject string) ([]string, error) {
	r.mu.RLock()
	enf := r.enf
	r.mu.RUnlock()

	if enf == nil {
		return nil, errors.New("rbac not initialized")
	}

	return enf.GetImplicitRolesForUser(subject)
}

// Helper middleware functions

// mustCan checks authorization and writes 403 if denied.
func mustCan(deps *serverDeps, w http.ResponseWriter, r *http.Request, obj, act, dom string) (*claims, bool) {
	cl := fromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}

	ok, err := deps.rbac.Enforce(cl.Sub, cl.Roles, obj, act, dom)
	if err != nil {
		logger.Error("RBAC enforcement error", "err", err, "subject", cl.Sub, "object", obj, "action", act, "domain", dom)
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}

	if !ok {
		logger.Debug("RBAC access denied", "subject", cl.Sub, "roles", cl.Roles, "object", obj, "action", act, "domain", dom)
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}

	return cl, true
}

// canAny checks if user has any of the specified permissions
func canAny(deps *serverDeps, cl *claims, obj string, actions []string, dom string) bool {
	for _, act := range actions {
		if ok, err := deps.rbac.Enforce(cl.Sub, cl.Roles, obj, act, dom); err == nil && ok {
			return true
		}
	}
	return false
}

// mustCanAny checks if user has any of the specified permissions, returns 403 if not
func mustCanAny(deps *serverDeps, w http.ResponseWriter, r *http.Request, obj string, actions []string, dom string) (*claims, bool) {
	cl := fromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}

	if !canAny(deps, cl, obj, actions, dom) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}

	return cl, true
}

// k8sCan checks if the server's service account can perform a Kubernetes operation
func k8sCan(ctx context.Context, c client.Client, ra authv1.ResourceAttributes) bool {
	ssar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &ra,
		},
	}

	if err := c.Create(ctx, ssar); err != nil {
		if logger != nil {
			logger.Error("Failed to check Kubernetes permissions", "err", err, "resource", ra.Resource, "verb", ra.Verb)
		}
		return false
	}

	return ssar.Status.Allowed
}

// uniqueNonEmpty removes duplicates and empty strings
func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
