package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	oidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/codespace-operator/codespace-operator/cmd/config"
)

const (
	oidcStateCookie = "oidc_state"
	oidcNonceCookie = "oidc_nonce"
	oidcPKCECookie  = "oidc_pkce"
)

type oidcDeps struct {
	provider   *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	oauth2     *oauth2.Config
	httpClient *http.Client
}

func registerAuthHandlers(mux *http.ServeMux, deps *serverDeps) {
	cfg := deps.config
	oidcEnabled := false
	if cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" && cfg.OIDCRedirectURL != "" {
		od, err := newOIDCDeps(context.Background(), cfg)
		if err != nil {
			panic(fmt.Errorf("oidc init: %w", err))
		}
		mux.Handle("/auth/login", handleOIDCStart(cfg, od))
		mux.Handle("/auth/callback", handleOIDCCallback(cfg, od))
		oidcEnabled = true
	}
	if cfg.EnableBootstrapLogin {
		if oidcEnabled {
			mux.Handle("/auth/local-login", handleLogin(cfg))
		} else {
			mux.Handle("/auth/login", handleLogin(cfg))
		}
	}
	mux.Handle("/auth/logout", handleLogout(deps.config))
	mux.Handle("/api/v1/me", handleMe())

	mux.Handle("/auth/features", handleAuthFeatures(cfg, oidcEnabled))
}

func newOIDCDeps(ctx context.Context, cfg *config.ServerConfig) (*oidcDeps, error) {
	var hc *http.Client
	if cfg.OIDCInsecureSkipVerify {
		// Dev-only: accept self-signed or name-mismatched certs for OIDC calls
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 (intentional; guarded by config)
		}
		hc = &http.Client{Transport: tr, Timeout: 15 * time.Second}
		logger.Warn("OIDCInsecureSkipVerify is enabled - do not use in production")
		ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	}

	logger.Info("Constructing provider...")
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		logger.Errorf("Fail constructing OIDC provider: %s", err.Error())
		return nil, err
	}
	logger.Info("Verifying realm...")
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID})
	scopes := cfg.OIDCScopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	oauth2cfg := &oauth2.Config{
		ClientID:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.OIDCRedirectURL,
		Scopes:       scopes,
	}
	return &oidcDeps{provider: provider, verifier: verifier, oauth2: oauth2cfg, httpClient: hc}, nil
}

func handleOIDCStart(cfg *config.ServerConfig, od *oidcDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := randB64(32)
		nonce := randB64(32)
		verifier, challenge := pkcePair()

		setTempCookie(w, oidcStateCookie, state)
		setTempCookie(w, oidcNonceCookie, nonce)
		setTempCookie(w, oidcPKCECookie, verifier)

		next := r.URL.Query().Get("next")
		if next != "" && strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
			state = state + "|" + base64.RawURLEncoding.EncodeToString([]byte(next))
			setTempCookie(w, oidcStateCookie, strings.Split(state, "|")[0])
		}

		authURL := od.oauth2.AuthCodeURL(state,
			oauth2.SetAuthURLParam("code_challenge", challenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
			oauth2.SetAuthURLParam("nonce", nonce),
			oauth2.AccessTypeOffline,
		)
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func handleOIDCCallback(cfg *config.ServerConfig, od *oidcDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		gotState := q.Get("state")
		if gotState == "" {
			http.Error(w, "missing state", http.StatusBadRequest)
			return
		}
		cState, err := r.Cookie(oidcStateCookie)
		if err != nil || cState.Value == "" {
			http.Error(w, "state cookie missing", http.StatusBadRequest)
			return
		}
		rawState := gotState
		var after string
		if parts := strings.SplitN(gotState, "|", 2); len(parts) == 2 {
			rawState = parts[0]
			if nextBytes, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
				after = string(nextBytes)
			}
		}
		if !constantTimeEqual(rawState, cState.Value) {
			http.Error(w, "state mismatch", http.StatusUnauthorized)
			return
		}

		cPKCE, err := r.Cookie(oidcPKCECookie)
		if err != nil || cPKCE.Value == "" {
			http.Error(w, "pkce missing", http.StatusBadRequest)
			return
		}

		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		// ensure the same HTTP client (possibly insecure) is used for token exchange
		ctx := r.Context()
		if od.httpClient != nil {
			ctx = context.WithValue(ctx, oauth2.HTTPClient, od.httpClient)
		}
		oauth2Token, err := od.oauth2.Exchange(ctx, code,
			oauth2.SetAuthURLParam("code_verifier", cPKCE.Value))
		if err != nil {
			http.Error(w, "code exchange failed", http.StatusUnauthorized)
			return
		}

		rawIDToken, _ := oauth2Token.Extra("id_token").(string)
		if rawIDToken == "" {
			http.Error(w, "id_token missing", http.StatusUnauthorized)
			return
		}
		cNonce, err := r.Cookie(oidcNonceCookie)
		if err != nil || cNonce.Value == "" {
			http.Error(w, "nonce missing", http.StatusBadRequest)
			return
		}
		idt, err := od.verifier.Verify(r.Context(), rawIDToken)
		if err != nil {
			http.Error(w, "id_token verify failed", http.StatusUnauthorized)
			return
		}
		var idc struct {
			Email    string   `json:"email"`
			Verified bool     `json:"email_verified"`
			Groups   []string `json:"groups"`
			Roles    []string `json:"roles"`
			Nonce    string   `json:"nonce"`
			Username string   `json:"preferred_username"`
		}
		if err := idt.Claims(&idc); err != nil {
			http.Error(w, "claims parse failed", http.StatusUnauthorized)
			return
		}
		if idc.Nonce != "" && idc.Nonce != cNonce.Value {
			http.Error(w, "nonce mismatch", http.StatusUnauthorized)
			return
		}

		roles := idc.Groups
		if len(roles) == 0 && len(idc.Roles) > 0 {
			roles = idc.Roles
		}

		extra := map[string]any{"email": idc.Email}
		sessionTTL := time.Duration(cfg.SessionTTLMinutes) * time.Minute
		if sessionTTL <= 0 {
			sessionTTL = 60 * time.Minute
		}
		tok, err := makeJWT(idt.Subject, roles, "oidc", []byte(cfg.JWTSecret), sessionTTL, extra)
		if err != nil {
			http.Error(w, "session mint failed", http.StatusInternalServerError)
			return
		}
		setAuthCookie(w, r, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		}, tok, sessionTTL)

		expireTempCookie(w, oidcStateCookie)
		expireTempCookie(w, oidcNonceCookie)
		expireTempCookie(w, oidcPKCECookie)

		dest := "/"
		if after != "" && isSafeRelative(after) {
			dest = after
		}
		http.Redirect(w, r, dest, http.StatusFound)
	}
}

func handleAuthFeatures(cfg *config.ServerConfig, oidcEnabled bool) http.HandlerFunc {
	local := "/auth/login"
	if oidcEnabled {
		local = "/auth/local-login"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"oidcEnabled":      oidcEnabled,
			"bootstrapEnabled": cfg.EnableBootstrapLogin,
			"localLoginPath":   local,
		})
	}
}

func handleLogout(cfg *config.ServerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clearAuthCookie(w, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		})
		writeJSON(w, map[string]string{"status": "logged_out"})
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
