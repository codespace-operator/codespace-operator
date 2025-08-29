package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// AuthFeatures represents available authentication methods
// @Description Available authentication features and endpoints
type AuthFeatures struct {
	SSOEnabled        bool   `json:"ssoEnabled" example:"true"`
	LocalLoginEnabled bool   `json:"localLoginEnabled" example:"false"`
	SSOLoginPath      string `json:"ssoLoginPath" example:"/auth/sso/login"`
	LocalLoginPath    string `json:"localLoginPath" example:"/auth/local/login"`
}

// LocalLoginRequest for username/password authentication
// @Description Local login credentials
type LocalLoginRequest struct {
	Username string `json:"username" validate:"required" example:"alice"`
	Password string `json:"password" validate:"required" example:"secretpassword"`
}

// LoginResponse after successful authentication
// @Description Successful authentication response
type LoginResponse struct {
	Token string   `json:"token"`
	User  string   `json:"user" example:"alice"`
	Roles []string `json:"roles" example:"editor,viewer"`
}

// UserInfo represents current user information
// @Description Current authenticated user information
type UserInfo struct {
	Subject       string   `json:"subject" example:"alice@company.com"`
	Username      string   `json:"username,omitempty" example:"alice"`
	Email         string   `json:"email,omitempty" example:"alice@company.com"`
	Roles         []string `json:"roles" example:"editor,viewer"`
	Provider      string   `json:"provider,omitempty" example:"oidc"`
	IssuedAt      int64    `json:"iat,omitempty" example:"1640995200"`
	ExpiresAt     int64    `json:"exp,omitempty" example:"1641081600"`
	ImplicitRoles []string `json:"implicitRoles,omitempty" example:"inherited-role"`
}

func registerAuthHandlers(mux *http.ServeMux, deps *serverDeps) {
	h := newHandlers(deps)
	cfg := deps.config

	// Always register the features endpoint
	mux.HandleFunc("/auth/features", h.handleAuthFeatures)

	// SSO endpoints (only if OIDC is configured)
	if cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" && cfg.OIDCRedirectURL != "" {
		od, err := newOIDCDeps(context.Background(), cfg)
		if err != nil {
			panic(fmt.Errorf("oidc init: %w", err))
		}
		// Store OIDC deps in handlers for these specific endpoints
		mux.HandleFunc("/auth/sso/login", func(w http.ResponseWriter, r *http.Request) {
			h.handleOIDCStart(w, r, od)
		})
		mux.HandleFunc("/auth/sso/callback", func(w http.ResponseWriter, r *http.Request) {
			h.handleOIDCCallback(w, r, od)
		})
		mux.HandleFunc("/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			h.handleLogout(w, r, od)
		})
	}

	// Local login endpoints (only if local login is enabled)
	if cfg.EnableLocalLogin {
		mux.HandleFunc("/auth/local/login", h.handleLocalLogin)
		// If SSO isn't available, also handle generic logout
		if cfg.OIDCIssuerURL == "" {
			mux.HandleFunc("/auth/logout", h.handleLocalLogout)
		}
	}
}

// === Feature Detection ===
// @Summary Get authentication features
// @Description Get available authentication methods and endpoints
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} AuthFeatures
// @Router /auth/features [get]
func (h *handlers) handleAuthFeatures(w http.ResponseWriter, r *http.Request) {

	cfg := h.deps.config

	// SSO available if OIDC is fully configured
	ssoEnabled := cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" && cfg.OIDCRedirectURL != ""

	// Local login available if enabled AND has either bootstrap or user file
	localEnabled := cfg.EnableLocalLogin && ((cfg.BootstrapUser != "" && cfg.BootstrapPassword != "") ||
		(cfg.LocalUsersPath != "" && h.deps.localUsers != nil))

	writeJSON(w, map[string]any{
		"ssoEnabled":        ssoEnabled,
		"localLoginEnabled": localEnabled,
		"ssoLoginPath":      "/auth/sso/login",
		"localLoginPath":    "/auth/local/login",
	})
}

// @Summary OIDC start
// @Description Redirect to OIDC provider with PKCE/state. Optional `next` query to return to path.
// @Tags authentication
// @Param next query string false "Relative path to return to after login"
// @Success 302 {string} string "Redirect"
// @Router /auth/sso/login [get]
func (h *handlers) handleOIDCStart(w http.ResponseWriter, r *http.Request, od *oidcDeps) {
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

// @Summary OIDC callback
// @Description Handles provider callback, mints session cookie, then redirects.
// @Tags authentication
// @Param state query string true "OIDC state"
// @Param code query string true "Authorization code"
// @Success 302 {string} string "Redirect"
// @Failure 401 {string} string "unauthorized"
// @Router /auth/sso/callback [get]
func (h *handlers) handleOIDCCallback(w http.ResponseWriter, r *http.Request, od *oidcDeps) {

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

	sessionTTL := h.deps.config.SessionTTL()
	canonSub := "oidc:" + od.issuerID + ":" + idt.Subject
	tok, err := makeJWT(canonSub, roles, "oidc", []byte(h.deps.config.JWTSecret), sessionTTL, extra)
	if err != nil {
		errJSON(w, err)
		http.Error(w, "session mint failed", http.StatusInternalServerError)
		return
	}

	setAuthCookie(w, r, &authConfigLike{
		JWTSecret:         h.deps.config.JWTSecret,
		SessionCookieName: h.deps.config.SessionCookieName,
		AllowTokenParam:   h.deps.config.AllowTokenParam,
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

// @Summary Local login
// @Description Authenticate using username and password
// @Tags authentication
// @Accept json
// @Produce json
// @Param credentials body LocalLoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/local/login [post]
func (h *handlers) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	secret := []byte(h.deps.config.JWTSecret)

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body struct{ Username, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, err)
		return
	}

	var (
		okUser bool
		email  string
	)

	// Try file-backed users first
	if h.deps.localUsers != nil && h.deps.config.LocalUsersPath != "" {
		if u, err := h.deps.localUsers.verify(body.Username, body.Password); err == nil {
			okUser = true
			email = u.Email
		}
	}

	// Fallback to bootstrap user
	if !okUser && h.deps.config.BootstrapUser != "" && h.deps.config.BootstrapPassword != "" {
		if body.Username == h.deps.config.BootstrapUser && body.Password == h.deps.config.BootstrapPassword {
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
	if h.deps.rbac != nil && h.deps.rbac.enf != nil {
		if rs, err := h.deps.rbac.enf.GetImplicitRolesForUser(body.Username); err == nil && len(rs) > 0 {
			roles = rs
		}
	}

	sessionTTL := h.deps.config.SessionTTL()
	canonSub := "local:" + body.Username
	tok, err := makeJWT(canonSub, roles, "local", secret, sessionTTL, map[string]any{
		"email":    email,
		"username": body.Username,
	})
	if err != nil {
		errJSON(w, err)
		return
	}

	setAuthCookie(w, r, &authConfigLike{
		JWTSecret:         h.deps.config.JWTSecret,
		SessionCookieName: h.deps.config.SessionCookieName,
		AllowTokenParam:   h.deps.config.AllowTokenParam,
	}, tok, sessionTTL)

	writeJSON(w, map[string]any{
		"token": tok,
		"user":  body.Username,
		"roles": roles,
	})
}

// @Summary Logout
// @Description Clears session cookie. If SSO configured, redirects to the provider’s end-session endpoint.
// @Tags authentication
// @Produce json
// @Success 200 {object} map[string]string
// @Router /auth/logout [get]
func (h *handlers) handleLogout(w http.ResponseWriter, r *http.Request, od *oidcDeps) {

	clearAuthCookie(w, &authConfigLike{
		JWTSecret:         h.deps.config.JWTSecret,
		SessionCookieName: h.deps.config.SessionCookieName,
		AllowTokenParam:   h.deps.config.AllowTokenParam,
	})

	// SSO logout
	if od != nil && od.endSession != "" {
		var hint string
		if c, err := r.Cookie("oidc_id_token_hint"); err == nil {
			hint = c.Value
		}
		post := h.deps.config.OIDCRedirectURL
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

func (h *handlers) handleLocalLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w, &authConfigLike{
		JWTSecret:         h.deps.config.JWTSecret,
		SessionCookieName: h.deps.config.SessionCookieName,
		AllowTokenParam:   h.deps.config.AllowTokenParam,
	})
	writeJSON(w, map[string]string{"status": "logged_out"})
}
