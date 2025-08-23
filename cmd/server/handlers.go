package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
	"github.com/codespace-operator/codespace-operator/internal/helpers"
)

func handleLogin(cfg *config.ServerConfig) http.HandlerFunc {
	secret := []byte(cfg.JWTSecret)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Only allow a single bootstrap admin when local auth is enabled.
		if cfg.BootstrapUser == "" || cfg.BootstrapPassword == "" || len(secret) == 0 {
			http.Error(w, "login disabled", http.StatusForbidden)
			return
		}

		var body struct{ Username, Password string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, err)
			return
		}
		// Constant-time compare to avoid tiny leaks.
		if !constantTimeEqual(body.Username, cfg.BootstrapUser) || !constantTimeEqual(body.Password, cfg.BootstrapPassword) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// Issue admin role for the single bootstrap user.
		roles := []string{"codespace-operator:admin"}
		tok, err := makeJWT(body.Username, roles, "local", secret, 24*time.Hour)
		if err != nil {
			errJSON(w, err)
			return
		}

		// HttpOnly cookie for SPA; also return token to support Authorization header/SSE fallback.
		setAuthCookie(w, r, tok)
		writeJSON(w, map[string]any{
			"token": tok,
			"user":  body.Username,
			"roles": roles,
		})
	}
}

// Returns the current subject + roles from the JWT.
func handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl := fromContext(r)
		if cl == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, map[string]any{
			"user":     cl.Sub,
			"roles":    cl.Roles,
			"provider": cl.Provider,
			"exp":      cl.ExpiresAt,
			"iat":      cl.IssuedAt,
		})
	}
}

// Healthz OK
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		readyCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		ns := q(r, "namespace", "default")
		var sl codespacev1.SessionList
		if err := deps.typed.List(readyCtx, &sl, client.InNamespace(ns), client.Limit(1)); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready: " + err.Error()))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	}
}

func handleStreamSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := mustHavePermission(w, r, PermSessionsWatch); !ok {
			return
		}

		ns := q(r, "namespace", "default")
		watcher, err := deps.dyn.Resource(gvr).Namespace(ns).Watch(r.Context(), metav1.ListOptions{Watch: true})
		if err != nil {
			errJSON(w, err)
			return
		}
		defer watcher.Stop()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return
				}
				if ev.Type == watch.Error {
					continue
				}
				u, ok := ev.Object.(*unstructured.Unstructured)
				if !ok {
					continue
				}
				var s codespacev1.Session
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &s); err != nil {
					continue
				}
				payload := map[string]any{
					"type":   string(ev.Type),
					"object": s,
				}
				writeSSE(w, "message", payload)
				flusher.Flush()
			}
		}
	}
}

func handleSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if _, ok := mustHavePermission(w, r, PermSessionsList); !ok {
				return
			}
			ns := q(r, "namespace", "default")
			var sl codespacev1.SessionList
			if err := deps.typed.List(r.Context(), &sl, client.InNamespace(ns)); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, sl.Items)

		case http.MethodPost:
			if _, ok := mustHavePermission(w, r, PermSessionsCreate); !ok {
				return
			}
			s, err := decodeSession(r.Body)
			if err != nil {
				errJSON(w, err)
				return
			}
			if s.Namespace == "" {
				s.Namespace = "default"
			}
			applyDefaults(&s)

			if err := helpers.RetryOnConflict(func() error {
				return deps.typed.Create(r.Context(), &s)
			}); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func handleSessionsWithPath(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/")
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ns, name := parts[0], parts[1]

		// scale subpath
		if len(parts) == 3 && parts[2] == "scale" {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if _, ok := mustHavePermission(w, r, PermSessionsScale); !ok {
				return
			}
			handleScale(deps, w, r, ns, name)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if _, ok := mustHavePermission(w, r, PermSessionsGet); !ok {
				return
			}
			var s codespacev1.Session
			if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &s); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)

		case http.MethodDelete:
			if _, ok := mustHavePermission(w, r, PermSessionsDelete); !ok {
				return
			}
			s := codespacev1.Session{}
			s.Name, s.Namespace = name, ns
			if err := deps.typed.Delete(r.Context(), &s); err != nil {
				errJSON(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func handleScale(deps *serverDeps, w http.ResponseWriter, r *http.Request, ns, name string) {
	var body struct {
		Replicas *int32 `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, err)
		return
	}
	if body.Replicas == nil {
		errJSON(w, errors.New("replicas is required"))
		return
	}

	var s codespacev1.Session
	if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &s); err != nil {
		errJSON(w, err)
		return
	}

	s.Spec.Replicas = body.Replicas
	if err := helpers.RetryOnConflict(func() error {
		return deps.typed.Update(r.Context(), &s)
	}); err != nil {
		errJSON(w, err)
		return
	}
	writeJSON(w, s)
}
