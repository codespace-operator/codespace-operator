package main

import (
	"errors"
	"net/http"
	"path"
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
  // allow SSE query token
  if t := r.URL.Query().Get("access_token"); t != "" {
    return t
  }
  // optional cookie
  if c, err := r.Cookie("codespace_jwt"); err == nil {
    return c.Value
  }
  return ""
}
func requireAuth(secret []byte, next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // public paths
    if r.Method == http.MethodOptions ||
       strings.HasPrefix(r.URL.Path, "/healthz") ||
       strings.HasPrefix(r.URL.Path, "/readyz") ||
       strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
       r.URL.Path == "/" ||
       path.Ext(r.URL.Path) != "" { // static files
      next.ServeHTTP(w, r)
      return
    }

    token := extractBearer(r)
    if token == "" {
      http.Error(w, "unauthorized", http.StatusUnauthorized)
      return
    }
    if _, err := parseJWT(token, secret); err != nil {
      http.Error(w, "forbidden", http.StatusForbidden)
      return
    }
    next.ServeHTTP(w, r)
  })
}