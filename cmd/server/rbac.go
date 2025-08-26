// cmd/server/rbac.go
package main

import (
	"context"
	"errors"
	"log"
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
)

// Default on-disk paths; overridden by env if set.
// These are mounted by the Helm chart via the codespace-rbac-cm ConfigMap.
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

// NewRBACFromEnv loads model/policy from disk and starts a file watcher
// to hot-reload on ConfigMap updates (K8s updates the symlink atomically).
func NewRBACFromEnv(ctx context.Context, logger *log.Logger) (*RBAC, error) {
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
		return nil, err
	}

	// Watch both files + their parent dir (K8s ConfigMap mounts replace symlinks)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
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
					logger.Printf("rbac reload failed: %v", err)
				} else if logger != nil {
					logger.Printf("rbac reloaded after fsnotify: %s", ev.Name)
				}
			case err := <-watcher.Errors:
				if logger != nil && err != nil {
					logger.Printf("rbac watcher error: %v", err)
				}
			}
		}
	}()

	return r, nil
}

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

// reload rebuilds the Enforcer from the current files.
func (r *RBAC) reload() error {
	// Load model
	m, err := cmodel.NewModelFromFile(r.modelPath)
	if err != nil {
		return err
	}

	// Load policy via file adapter
	adapter := fileadapter.NewAdapter(r.policyPath)
	enf, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return err
	}

	// Enable logging in debug builds via CASBIN_LOG_ENABLED=1
	if os.Getenv("CASBIN_LOG_ENABLED") == "1" {
		enf.EnableLog(true)
	}

	r.mu.Lock()
	r.enf = enf
	r.mu.Unlock()
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

	// Try the concrete identity first
	ok, err := enf.Enforce(sub, obj, act, dom)
	if err != nil || ok {
		return ok, err
	}
	// Then each role as a subject (policy can list either usernames or role names)
	for _, role := range roles {
		if role == "" {
			continue
		}
		ok, err = enf.Enforce(role, obj, act, dom)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// serverMustCan checks authorization and writes 403 if denied.
// Returns claims when authorized.
func serverMustCan(deps *serverDeps, w http.ResponseWriter, r *http.Request, obj, act, dom string) (*claims, bool) {
	cl := fromContext(r)
	if cl == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	ok, err := deps.rbac.Enforce(cl.Sub, cl.Roles, obj, act, dom)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return cl, true
}
