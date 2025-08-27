// cmd/server/auth_handlers.go
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	issuerID   string
	verifier   *oidc.IDTokenVerifier
	oauth2     *oauth2.Config
	httpClient *http.Client
	endSession string
}

func registerAuthHandlers(mux *http.ServeMux, deps *serverDeps) {
	cfg := deps.config

	// Always register the features endpoint
	mux.Handle("/auth/features", handleAuthFeatures(cfg, deps))

	// SSO endpoints (only if OIDC is configured)
	if cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" && cfg.OIDCRedirectURL != "" {
		od, err := newOIDCDeps(context.Background(), cfg)
		if err != nil {
			panic(fmt.Errorf("oidc init: %w", err))
		}
		mux.Handle("/auth/sso/login", handleOIDCStart(cfg, od))
		mux.Handle("/auth/sso/callback", handleOIDCCallback(cfg, od))
		mux.Handle("/auth/logout", handleLogout(cfg, od)) // SSO logout
	}

	// Local login endpoints (only if local login is enabled)
	if cfg.EnableLocalLogin {
		mux.Handle("/auth/local/login", handleLocalLogin(deps))
		// If SSO isn't available, also handle generic logout
		if cfg.OIDCIssuerURL == "" {
			mux.Handle("/auth/logout", handleLocalLogout(cfg))
		}
	}

	// Always available
	mux.Handle("/api/v1/me", handleMe())
}

func newOIDCDeps(ctx context.Context, cfg *config.ServerConfig) (*oidcDeps, error) {
	var hc *http.Client
	if cfg.OIDCInsecureSkipVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402
		}
		hc = &http.Client{Transport: tr, Timeout: 15 * time.Second}
		logger.Warn("OIDCInsecureSkipVerify is enabled - do not use in production")
		ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	}

	logger.Info("Constructing OIDC provider...")
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		logger.Errorf("Failed constructing OIDC provider: %s", err.Error())
		return nil, err
	}

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

	var meta struct {
		Issuer             string `json:"issuer"`
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	_ = provider.Claims(&meta)

	iss := meta.Issuer
	if iss == "" {
		iss = cfg.OIDCIssuerURL
	}

	return &oidcDeps{
		provider:   provider,
		verifier:   verifier,
		oauth2:     oauth2cfg,
		httpClient: hc,
		endSession: meta.EndSessionEndpoint,
		issuerID:   issuerIDFrom(iss),
	}, nil
}

// === SSO (OIDC) Handlers ===

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

		roles := idc.Roles
		extra := map[string]any{
			"email":    idc.Email,
			"username": idc.Username,
		}

		sessionTTL := cfg.SessionTTL()
		canonSub := "oidc:" + od.issuerID + ":" + idt.Subject
		tok, err := makeJWT(canonSub, roles, "oidc", []byte(cfg.JWTSecret), sessionTTL, extra)
		if err != nil {
			errJSON(w, err)
			http.Error(w, "session mint failed", http.StatusInternalServerError)
			return
		}

		setAuthCookie(w, r, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		}, tok, sessionTTL)

		// Store id_token for logout
		http.SetCookie(w, &http.Cookie{
			Name:     "oidc_id_token_hint",
			Value:    rawIDToken,
			Path:     "/auth",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300,
		})

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

// === Local Login Handlers ===

func handleLocalLogin(deps *serverDeps) http.HandlerFunc {
	cfg := deps.config
	secret := []byte(cfg.JWTSecret)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var body struct{ Username, Password string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, err)
			http.Error(w, "session mint failed", http.StatusInternalServerError)
			return
		}

		var (
			okUser bool
			email  string
		)

		// Try file-backed users first
		if deps.localUsers != nil && cfg.LocalUsersPath != "" {
			if u, err := deps.localUsers.verify(body.Username, body.Password); err == nil {
				okUser = true
				email = u.Email
			}
		}

		// Fallback to bootstrap user
		if !okUser && cfg.BootstrapUser != "" && cfg.BootstrapPassword != "" {
			if body.Username == cfg.BootstrapUser && body.Password == cfg.BootstrapPassword {
				okUser = true
				email = "bootstrap@localhost"
			}
		}

		if !okUser {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// Get roles from Casbin
		roles := []string{"viewer"} // default
		if deps.rbac != nil && deps.rbac.enf != nil {
			if rs, err := deps.rbac.enf.GetImplicitRolesForUser(body.Username); err == nil && len(rs) > 0 {
				roles = rs
			}
		}

		sessionTTL := cfg.SessionTTL()
		// cmd/server/auth_local.go (in handleLocalLogin)
		canonSub := "local:" + body.Username
		tok, err := makeJWT(canonSub, roles, "local", secret, sessionTTL, map[string]any{
			"email":    email,
			"username": body.Username,
		})
		if err != nil {
			errJSON(w, err)
			return
		}

		setAuthCookie(w, r, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		}, tok, sessionTTL)

		writeJSON(w, map[string]any{
			"token": tok,
			"user":  body.Username,
			"roles": roles,
		})
	}
}

// === Logout Handlers ===

func handleLogout(cfg *config.ServerConfig, od *oidcDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clearAuthCookie(w, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		})

		// SSO logout
		if od != nil && od.endSession != "" {
			var hint string
			if c, err := r.Cookie("oidc_id_token_hint"); err == nil {
				hint = c.Value
			}
			post := cfg.OIDCRedirectURL
			u := od.endSession + "?post_logout_redirect_uri=" + url.QueryEscape(post)
			if hint != "" {
				u += "&id_token_hint=" + url.QueryEscape(hint)
			}
			expireTempCookie(w, "oidc_id_token_hint")
			http.Redirect(w, r, u, http.StatusFound)
			return
		}

		writeJSON(w, map[string]string{"status": "logged_out"})
	}
}

func handleLocalLogout(cfg *config.ServerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clearAuthCookie(w, &configLike{
			JWTSecret:         cfg.JWTSecret,
			SessionCookieName: cfg.SessionCookieName,
			AllowTokenParam:   cfg.AllowTokenParam,
		})
		writeJSON(w, map[string]string{"status": "logged_out"})
	}
}

// === Feature Detection ===

func handleAuthFeatures(cfg *config.ServerConfig, deps *serverDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// SSO available if OIDC is fully configured
		ssoEnabled := cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" && cfg.OIDCRedirectURL != ""

		// Local login available if enabled AND has either bootstrap or user file
		localEnabled := cfg.EnableLocalLogin && ((cfg.BootstrapUser != "" && cfg.BootstrapPassword != "") ||
			(cfg.LocalUsersPath != "" && deps.localUsers != nil))

		writeJSON(w, map[string]any{
			"ssoEnabled":        ssoEnabled,
			"localLoginEnabled": localEnabled,
			"ssoLoginPath":      "/auth/sso/login",
			"localLoginPath":    "/auth/local/login",
		})
	}
}

// === Helper Functions ===

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
func issuerIDFrom(issuer string) string {
	u, err := url.Parse(issuer)
	if err != nil {
		return shortHash(issuer)
	}
	s := strings.ToLower(strings.TrimSuffix(u.Host+u.Path, "/"))
	s = strings.ReplaceAll(s, "/", "~") // keycloak.example.com~realms~prod
	s = strings.ReplaceAll(s, ":", "-") // avoid delimiter collision
	if s == "" {
		return shortHash(issuer)
	}
	return s
}
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:16]
}
