package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	defaultSessionCookie = "codespace_session"
)

type authConfigLike struct {
	JWTSecret         string
	SessionCookieName string
	AllowTokenParam   bool
}
type ctxKey string

const ctxClaimsKey ctxKey = "claims"

type claims struct {
	Username  string   `json:"username,omitempty"`
	Sub       string   `json:"sub"`             // Reliable
	Roles     []string `json:"roles,omitempty"` // mapped from OIDC groups
	Email     string   `json:"email,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}

// === Minimal JWT HS256 for the server-issued session cookie ===============

func makeJWT(sub string, roles []string, provider string, secret []byte, ttl time.Duration, extras map[string]any) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	now := time.Now().Unix()
	exp := time.Now().Add(ttl).Unix()

	payloadMap := map[string]any{
		"sub": sub, "roles": roles, "provider": provider,
		"iat": now, "exp": exp,
	}
	if extras != nil {
		for k, v := range extras {
			payloadMap[k] = v
		}
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	msg := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return msg + "." + sig, nil
}

func parseJWT(token string, secret []byte) (*claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token")
	}
	msg := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("bad signature encoding")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	want := mac.Sum(nil)
	if subtle.ConstantTimeCompare(sig, want) != 1 {
		return nil, errors.New("signature mismatch")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("bad payload encoding")
	}
	var c claims
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, errors.New("bad payload json")
	}
	if c.ExpiresAt == 0 || time.Now().Unix() > c.ExpiresAt {
		return nil, errors.New("expired")
	}
	return &c, nil
}

// === Cookie helpers ========================================================

func setAuthCookie(w http.ResponseWriter, r *http.Request, cfg *authConfigLike, token string, ttl time.Duration) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(cfg),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearAuthCookie(w http.ResponseWriter, cfg *authConfigLike) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(cfg),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func cookieName(cfg *authConfigLike) string {
	if cfg.SessionCookieName != "" {
		return cfg.SessionCookieName
	}
	return defaultSessionCookie
}

// === Context helpers =======================================================

func withClaims(r *http.Request, c *claims) *http.Request {
	ctx := context.WithValue(r.Context(), ctxClaimsKey, c)
	return r.WithContext(ctx)
}

func fromContext(r *http.Request) *claims {
	v := r.Context().Value(ctxClaimsKey)
	if v == nil {
		return nil
	}
	c, _ := v.(*claims)
	return c
}

// === Middleware to require session or bearer token =========================

func requireAPIToken(cfg *authConfigLike, next http.Handler) http.Handler {
	secret := []byte(cfg.JWTSecret)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tok string

		// 1) Cookie session
		if c, err := r.Cookie(cookieName(cfg)); err == nil && c.Value != "" {
			tok = c.Value
		}

		// 2) Authorization: Bearer
		if tok == "" {
			h := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(h), "bearer ") {
				tok = strings.TrimSpace(h[len("bearer "):])
			}
		}

		// 3) Optional query param (discouraged - behind a flag)
		if tok == "" && cfg.AllowTokenParam {
			tok = r.URL.Query().Get("access_token")
		}

		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		c, err := parseJWT(tok, secret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, withClaims(r, c))
	})
}

// corsMiddleware adds CORS headers with credentials support
func corsMiddleware(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// authGate provides authentication routing
func authGate(cfg *authConfigLike, next http.Handler) http.Handler {
	authed := requireAPIToken(cfg, next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public endpoints (no auth required)
		publicPaths := []string{
			"/healthz",
			"/readyz",
			"/",
		}

		publicPrefixes := []string{
			"/auth/",
			"/assets/",
			"/static/",
		}

		// Check if this is a public path
		for _, publicPath := range publicPaths {
			if path == publicPath {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check if this is a public prefix
		for _, prefix := range publicPrefixes {
			if strings.HasPrefix(path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// API endpoints require authentication
		if strings.HasPrefix(path, "/api/") {
			authed.ServeHTTP(w, r)
			return
		}

		// Default to serving static content (SPA)
		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware provides basic rate limiting (placeholder implementation)
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement proper rate limiting based on user/IP
		// For now, just pass through
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds security headers
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Only add HSTS if we're on HTTPS
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// requireAdminAccess middleware that requires admin-level permissions
func requireAdminAccess(deps *serverDeps, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := mustCan(deps, w, r, "*", "admin", "*"); !ok {
			return
		}
		next(w, r)
	}
}

// requireNamespaceAccess middleware that checks namespace-level permissions
func requireNamespaceAccess(deps *serverDeps, resource, action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract namespace from URL path or query parameters
		namespace := extractNamespaceFromRequest(r)
		if namespace == "" {
			namespace = "default"
		}

		if _, ok := mustCan(deps, w, r, resource, action, namespace); !ok {
			return
		}
		next(w, r)
	}
}

func setTempCookie(w http.ResponseWriter, name, val string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    val,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})
}

func expireTempCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:16]
}
