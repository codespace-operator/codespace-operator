package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type claims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

func makeJWT(username string, secret []byte, ttl time.Duration) (string, error) {
	c := claims{
		Sub: username,
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
	// Allow SSE token via query
	if t := r.URL.Query().Get("access_token"); t != "" {
		return t
	}
	// Optional cookie if you move to cookie-based auth later
	if c, err := r.Cookie("codespace_jwt"); err == nil {
		return c.Value
	}
	return ""
}

// FIXED: Only protect /api/* endpoints, with explicit exemptions for health probes
func requireAPIToken(secret []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Debug logging if enabled
		if os.Getenv("DEBUG") == "true" {
			log.Printf("Request: %s %s", r.Method, r.URL.Path)
		}

		// Always allow these paths without any auth checks
		if r.Method == http.MethodOptions ||
			r.URL.Path == "/healthz" ||
			r.URL.Path == "/readyz" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Only protect /api/* endpoints (except auth endpoints already handled above)
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		token := extractBearer(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate token if secret is configured
		if len(secret) > 0 {
			if _, err := parseJWT(token, secret); err != nil {
				if os.Getenv("DEBUG") == "true" {
					log.Printf("Token validation failed for %s: %v", r.URL.Path, err)
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
