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
		if !cfg.EnableBootstrapLogin {
			http.Error(w, "login disabled", http.StatusForbidden)
			return
		}
		if cfg.BootstrapUser == "" || cfg.BootstrapPassword == "" || len(secret) == 0 {
			http.Error(w, "login disabled", http.StatusForbidden)
			return
		}

		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, err)
			return
		}
		if !constantTimeEqual(body.Username, cfg.BootstrapUser) || !constantTimeEqual(body.Password, cfg.BootstrapPassword) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		roles := []string{"codespace-operator:admin"}
		tok, err := makeJWT(body.Username, roles, "local", secret, 24*time.Hour, nil)
		if err != nil {
			errJSON(w, err)
			return
		}

		setAuthCookie(w, r, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		}, tok, 24*time.Hour)

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
			"sub":      cl.Sub,
			"username": cl.Username,
			"email":    cl.Email,
			"roles":    cl.Roles,
			"groups":   cl.Groups,
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

// cmd/server/handlers.go
// STREAM: GET /api/v1/stream/sessions
func handleStreamSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Determine domain (namespace) for RBAC
		dom := q(r, "namespace", "default")
		if r.URL.Query().Get("all") == "true" {
			dom = "*"
		}
		if _, ok := serverMustCan(deps, w, r, "session", "watch", dom); !ok {
			return
		}

		var watcher watch.Interface
		var err error
		if dom == "*" {
			watcher, err = deps.dyn.Resource(gvr).Watch(r.Context(), metav1.ListOptions{Watch: true})
		} else {
			watcher, err = deps.dyn.Resource(gvr).Namespace(dom).Watch(r.Context(), metav1.ListOptions{Watch: true})
		}
		if err != nil {
			errJSON(w, err)
			return
		}
		defer watcher.Stop()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

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
				if !ok || ev.Type == watch.Error {
					return
				}
				u, ok := ev.Object.(*unstructured.Unstructured)
				if !ok {
					continue
				}
				var s codespacev1.Session
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &s); err != nil {
					continue
				}
				payload := map[string]any{"type": string(ev.Type), "object": s}
				writeSSE(w, "message", payload)
				flusher.Flush()
			}
		}
	}
}

func handleServerSessions(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			dom := q(r, "namespace", "default")
			if r.URL.Query().Get("all") == "true" {
				dom = "*"
			}
			if _, ok := serverMustCan(deps, w, r, "session", "list", dom); !ok {
				return
			}

			if dom == "*" {
				ul, err := deps.dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{})
				if err != nil {
					errJSON(w, err)
					return
				}
				writeJSON(w, ul.Object)
				return
			}

			var out codespacev1.SessionList
			if err := deps.typed.List(r.Context(), &out, client.InNamespace(dom)); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, out)

		case http.MethodPost:
			var s codespacev1.Session
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			if s.Namespace == "" {
				s.Namespace = "default"
			}
			if _, ok := serverMustCan(deps, w, r, "session", "create", s.Namespace); !ok {
				return
			}

			u := &unstructured.Unstructured{}
			u.Object, _ = runtime.DefaultUnstructuredConverter.ToUnstructured(&s)
			created, err := deps.dyn.Resource(gvr).Namespace(s.Namespace).Create(r.Context(), u, metav1.CreateOptions{})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, created.Object)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// cmd/server/handlers.go
// GET/DELETE/SCALE: /api/v1/server/sessions/{ns}/{name}[ /scale ]
func handleServerSessionsWithPath(deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/server/sessions/"), "/")
		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		ns, name := parts[0], parts[1]

		if len(parts) == 3 && parts[2] == "scale" && r.Method == http.MethodPost {
			if _, ok := serverMustCan(deps, w, r, "session", "scale", ns); !ok {
				return
			}
			type scaleBody struct {
				Replicas int32 `json:"replicas"`
			}
			var b scaleBody
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}

			var s codespacev1.Session
			if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &s); err != nil {
				errJSON(w, err)
				return
			}
			s.Spec.Replicas = &b.Replicas
			if err := deps.typed.Update(r.Context(), &s); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if _, ok := serverMustCan(deps, w, r, "session", "get", ns); !ok {
				return
			}
			var s codespacev1.Session
			if err := deps.typed.Get(r.Context(), client.ObjectKey{Namespace: ns, Name: name}, &s); err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, s)

		case http.MethodDelete:
			if _, ok := serverMustCan(deps, w, r, "session", "delete", ns); !ok {
				return
			}
			err := deps.typed.Delete(r.Context(), &codespacev1.Session{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
			})
			if err != nil {
				errJSON(w, err)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
