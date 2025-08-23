package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const ctxClaimsKey ctxKey = "claims"

// Claims carries identity + roles and (later) provider info.
type claims struct {
	Sub      string   `json:"sub"`
	Roles    []string `json:"roles,omitempty"`
	Provider string   `json:"provider,omitempty"`
	jwt.RegisteredClaims
}

func makeJWT(username string, roles []string, provider string, secret []byte, ttl time.Duration) (string, error) {
	c := claims{
		Sub:      username,
		Roles:    roles,
		Provider: provider,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(secret)
}

func parseJWT(tokenStr string, secret []byte) (*claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &claims{}, func(t *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	if cl, ok := t.Claims.(*claims); ok && t.Valid {
		return cl, nil
	}
	return nil, errors.New("invalid token")
}

func extractBearer(r *http.Request) string {
	ah := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(ah), "bearer ") {
		return strings.TrimSpace(ah[7:])
	}
	// Allow SSE token via query only for EventSource fallback.
	if t := r.URL.Query().Get("access_token"); t != "" {
		return t
	}
	// Prefer HttpOnly cookie for same-origin SPA.
	if c, err := r.Cookie("codespace_jwt"); err == nil {
		return c.Value
	}
	return ""
}

// attachClaims stores claims on the request context for downstream authZ checks.
func attachClaims(r *http.Request, cl *claims) *http.Request {
	ctx := context.WithValue(r.Context(), ctxClaimsKey, cl)
	return r.WithContext(ctx)
}

// fromContext retrieves claims (if any).
func fromContext(r *http.Request) *claims {
	if v := r.Context().Value(ctxClaimsKey); v != nil {
		if cl, ok := v.(*claims); ok {
			return cl
		}
	}
	return nil
}

// constantTimeEqual avoids timing leaks for small secret comparisons (used for username/password checks too).
func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// isHTTPS helps set cookie flags behind TLS / reverse proxies.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Common proxy headers
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Protocol"), "https") {
		return true
	}
	return false
}

// setAuthCookie writes the JWT as a secure, HttpOnly cookie.
func setAuthCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := isHTTPS(r)
	c := &http.Cookie{
		Name:     "codespace_jwt",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		// Session cookie; let JWT exp control validity.
	}
	http.SetCookie(w, c)
}

// requireAPIToken protects /api/* (except auth + health) and attaches claims to context.
func requireAPIToken(secret []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG") == "true" {
			log.Printf("Request: %s %s", r.Method, r.URL.Path)
		}

		// Always allow these paths without auth
		if r.Method == http.MethodOptions ||
			r.URL.Path == "/healthz" ||
			r.URL.Path == "/readyz" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Only protect /api/*
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		token := extractBearer(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if len(secret) > 0 {
			cl, err := parseJWT(token, secret)
			if err != nil {
				if os.Getenv("DEBUG") == "true" {
					log.Printf("Token validation failed for %s: %v", r.URL.Path, err)
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			r = attachClaims(r, cl)
		}

		next.ServeHTTP(w, r)
	})
}
