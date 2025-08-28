package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionJWT(t *testing.T) {
	secret := []byte("test-secret")
	token, err := makeJWT("u123", []string{"group-a"}, "oidc", secret, time.Minute, map[string]any{"email": "u@example.com"})
	if err != nil {
		t.Fatalf("makeJWT: %v", err)
	}
	c, err := parseJWT(token, secret)
	if err != nil {
		t.Fatalf("parseJWT: %v", err)
	}
	if c.Sub != "u123" || c.Email != "u@example.com" || c.Provider != "oidc" {
		t.Fatalf("claims mismatch: %+v", c)
	}

	// Expiry
	expired, _ := makeJWT("u", nil, "oidc", secret, -1*time.Second, nil)
	if _, err := parseJWT(expired, secret); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestRequireAPIToken_Cookie(t *testing.T) {
	cfg := &authConfigLike{
		JWTSecret:         "zzz",
		SessionCookieName: "codespace_session",
	}
	token, _ := makeJWT("sub", []string{"r"}, "oidc", []byte(cfg.JWTSecret), time.Minute, nil)
	h := requireAPIToken(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cl := fromContext(r)
		if cl == nil || cl.Sub != "sub" {
			t.Fatalf("missing claims in context")
		}
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: cfg.SessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestStateCookie(t *testing.T) {
	// simple check for secure attributes
	w := httptest.NewRecorder()
	setTempCookie(w, oidcStateCookie, "state")
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || !c.Secure || c.MaxAge <= 0 {
		t.Fatalf("want secure short-lived cookie, got %#v", c)
	}
}
