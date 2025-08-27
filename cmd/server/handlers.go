package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	codespacev1 "github.com/codespace-operator/codespace-operator/api/v1"
	"github.com/codespace-operator/codespace-operator/cmd/config"
)

func handleLogin(cfg *config.ServerConfig) http.HandlerFunc {
	secret := []byte(cfg.JWTSecret)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// Parse input
		var body struct{ Username, Password string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, err)
			return
		}

		// Prefer file-backed users if configured
		var (
			okUser bool
			email  string
		)
		if deps := r.Context().Value("deps"); deps != nil {
			if sd, _ := deps.(*serverDeps); sd != nil && sd.localUsers != nil && sd.config.LocalUsersPath != "" {
				if u, err := sd.localUsers.verify(body.Username, body.Password); err == nil {
					okUser = true
					email = u.Email
				}
			}
		}

		if !okUser {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// === Get roles from Casbin (implicit roles) ===
		roles := []string{"viewer"} // minimal default
		if deps := r.Context().Value("deps"); deps != nil {
			if sd, _ := deps.(*serverDeps); sd != nil && sd.rbac != nil {
				if enf := sd.rbac.enf; enf != nil {
					if rs, err := enf.GetImplicitRolesForUser(body.Username); err == nil && len(rs) > 0 {
						roles = rs
					}
				}
			}
		}

		// Session
		ttl := 24 * time.Hour
		tok, err := makeJWT(body.Username, roles, "local", secret, ttl, map[string]any{
			"email":    email,
			"username": body.Username,
		})
		if err != nil {
			errJSON(w, err)
			return
		}

		setAuthCookie(
			w, r,
			&configLike{
				JWTSecret:         cfg.JWTSecret,
				SessionCookieName: cfg.SessionCookieName,
				AllowTokenParam:   cfg.AllowTokenParam,
			},
			tok, ttl)
		writeJSON(w, map[string]any{"token": tok, "user": body.Username, "roles": roles})
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
