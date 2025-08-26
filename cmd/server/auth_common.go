package main

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

type ctxKey string

const ctxClaimsKey ctxKey = "claims"

type claims struct {
	Username  string   `json:"username,omitempty"`
	Sub       string   `json:"sub"` // Reliable
	Groups    []string `json:"groups,omitempty"`
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

func setAuthCookie(w http.ResponseWriter, r *http.Request, cfg *configLike, token string, ttl time.Duration) {
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

func clearAuthCookie(w http.ResponseWriter, cfg *configLike) {
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

func cookieName(cfg *configLike) string {
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

type configLike struct {
	JWTSecret         string
	SessionCookieName string
	AllowTokenParam   bool
}

func requireAPIToken(cfg *configLike, next http.Handler) http.Handler {
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
