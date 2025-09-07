// Package server provides HTTP handlers for authentication endpoints,
// including local login, OIDC SSO login, logout, session refresh, and
// feature detection. It defines request/response types for authentication
// flows and registers handlers based on configuration and available providers.
//
// Handlers include:
//   - /auth/features: Returns available authentication methods and endpoints.
//   - /auth/login: Authenticates users via username/password.
//   - /auth/sso/login: Initiates OIDC SSO login flow.
//   - /auth/sso/callback: Handles OIDC provider callback and session creation.
//   - /auth/logout: Logs out user, clearing session and optionally redirecting to OIDC end-session.
//   - /auth/refresh: Refreshes session token if valid.
//
// The package supports both local and OIDC authentication, with feature
// detection and endpoint registration based on runtime configuration.
package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	auth "github.com/codespace-operator/common/auth/pkg/auth"
)

// AuthFeatures represents available authentication methods
// @Description Available authentication features and endpoints
type AuthFeatures struct {
	SSOEnabled        bool   `json:"ssoEnabled" example:"true"`
	LocalLoginEnabled bool   `json:"localLoginEnabled" example:"false"`
	SSOLoginPath      string `json:"ssoLoginPath" example:"/auth/sso/login"`
	LocalLoginPath    string `json:"localLoginPath" example:"/auth/login"`
}

// PasswordLoginRequest for username/password authentication
// @Description Local login credentials
type PasswordLoginRequest struct {
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

// imports: trim anything oauth2/oidc-specific that handlers no longer need.
// Keep: net/http, encoding/json, strings, time, etc., plus internal deps.
func registerAuthHandlers(mux *http.ServeMux, h *handlers) {
	base := h.deps.authCfg.AuthPath

	// Public feature probe
	mux.HandleFunc(base+"/features", h.handleAuthFeatures)

	// Local login (if enabled)
	if h.deps.authCfg != nil && h.deps.authCfg.Local != nil && h.deps.authCfg.Local.Enabled {
		mux.HandleFunc(base+"/login", h.handlePasswordLogin)
		// If NO OIDC provider is registered, local handles /logout
		if h.deps.authManager.GetProvider(auth.OIDC_PROVIDER) == nil {
			mux.HandleFunc(base+"/logout", h.handleLocalLogout)
		}
	}

	// OIDC endpoints only if provider exists
	if h.deps.authManager.GetProvider(auth.OIDC_PROVIDER) != nil {
		mux.HandleFunc(base+"/sso/login", h.handleOIDCStart)
		mux.HandleFunc(base+"/sso/callback", h.handleOIDCCallback)
		// OIDC logout wins when both exist
		mux.HandleFunc(base+"/logout", h.handleLogout)
	}

	mux.HandleFunc(base+"/refresh", h.handleRefresh)
}

// --- OIDC handlers (provider-based) ---

// @Summary Start OIDC login
// @Description Redirects to OIDC provider for authentication
// @Tags authentication
// @Produce json
// @Param next query string false "Post-login relative redirect (safe, same-origin)"
// @Success 302 "Redirect to provider"
// @Router /auth/sso/login [get]
func (h *handlers) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	p := h.deps.authManager.GetProvider(auth.OIDC_PROVIDER)
	if p == nil {
		http.NotFound(w, r)
		return
	}
	// Provider manages temp cookies; may include encoded redirect in state.
	if err := p.StartAuth(w, r, r.URL.Query().Get("next")); err != nil {
		errJSON(w, err)
		return
	}
}

// @Summary OIDC callback
// @Description Completes OIDC login, mints session cookie, then redirects
// @Tags authentication
// @Produce json
// @Success 302 "Redirect to / or provided 'next'"
// @Failure 401 {object} ErrorResponse
// @Router /auth/sso/callback [get]

func (h *handlers) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	p := h.deps.authManager.GetProvider(auth.OIDC_PROVIDER)
	if p == nil {
		http.NotFound(w, r)
		return
	}

	// Provider verifies and returns claims
	claims, err := p.HandleCallback(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Simplest: use IssueSession – mints JWT and sets session cookie in one place
	if _, err := h.deps.authManager.IssueSession(w, r, claims); err != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	// Compute post-login redirect from state suffix, safely
	dest := "/"
	if s := r.URL.Query().Get("state"); s != "" {
		parts := strings.SplitN(s, "|", 2)
		if len(parts) == 2 {
			if b, decErr := base64.RawURLEncoding.DecodeString(parts[1]); decErr == nil {
				if isSafeRelative(string(b)) {
					dest = string(b)
				}
			}
		}
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// @Summary Logout
// @Description Clears session and, if OIDC is configured, redirects to provider end-session
// @Tags authentication
// @Success 302 "Redirect to provider end-session if available"
// @Success 200 {object} map[string]string "JSON {status: logged_out} if local"
// @Router /auth/logout [get]
func (h *handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.deps.authManager.ClearAuthCookie(w)
	if p := h.deps.authManager.GetProvider("oidc"); p != nil {
		_ = p.Logout(w, r) // may redirect to end_session
		return
	}
	writeJSON(w, map[string]string{"status": "logged_out"})
}

func (h *handlers) handleLocalLogout(w http.ResponseWriter, r *http.Request) {
	h.deps.authManager.ClearAuthCookie(w)
	writeJSON(w, map[string]string{"status": "logged_out"})
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
	cfg := h.deps.authCfg
	hasOIDC := h.deps.authManager.GetProvider(auth.OIDC_PROVIDER) != nil

	ssoEnabled := hasOIDC
	if ssoEnabled && cfg != nil && cfg.OIDC != nil {
		// keep the stricter check but guard nils:
		ssoEnabled = cfg.OIDC.IssuerURL != "" && cfg.OIDC.ClientID != "" && cfg.OIDC.RedirectURL != ""
	}

	ldapEnabled := h.deps.authManager.GetProvider(auth.LDAP_PROVIDER) != nil
	localEnabled := cfg != nil && cfg.Local != nil && cfg.Local.Enabled &&
		h.deps.authManager.GetProvider(auth.LOCAL_PROVIDER) != nil

	writeJSON(w, map[string]any{
		"ssoEnabled":            ssoEnabled,
		"ldapLoginEnabled":      ldapEnabled,
		"localLoginEnabled":     localEnabled,
		"bootstrapLoginAllowed": cfg != nil && cfg.Local != nil && cfg.Local.BootstrapLoginAllowed,
		"ssoLoginPath":          cfg.AuthPath + "/sso/login",
		"passwordLoginPath":     cfg.AuthPath + "/login",
	})
}

// @Summary Local login
// @Description Authenticate using username and password
// @Tags authentication
// @Accept json
// @Produce json
// @Param credentials body PasswordLoginRequest true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/login [post]
func (h *handlers) handlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body PasswordLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, err)
		return
	}

	// Build the try-order. Default: Local then LDAP.
	tryOrder := []string{}
	if lp := h.deps.authManager.GetLocalProvider(); lp != nil {
		tryOrder = append(tryOrder, auth.LOCAL_PROVIDER)
	}
	if p := h.deps.authManager.GetProvider(auth.LDAP_PROVIDER); p != nil {
		tryOrder = append(tryOrder, auth.LDAP_PROVIDER)
	}

	// If neither provider is available, behave like disabled.
	if len(tryOrder) == 0 {
		http.Error(w, "password authentication not enabled", http.StatusNotFound)
		return
	}

	var claims *auth.TokenClaims
	var err error

	// Try providers in order, stop at first success.
	for _, prov := range tryOrder {
		switch prov {
		case auth.LOCAL_PROVIDER:
			if lp := h.deps.authManager.GetLocalProvider(); lp != nil {
				claims, err = lp.Authenticate(body.Username, body.Password) // existing local path
				if err == nil {
					goto ISSUE
				}
			}
		case auth.LDAP_PROVIDER:
			if p := h.deps.authManager.GetProvider(auth.LDAP_PROVIDER); p != nil {
				if ldap, ok := p.(auth.LDAPAuthProvider); ok {
					claims, err = ldap.Authenticate(body.Username, body.Password)
					if err == nil {
						goto ISSUE
					}
				}
			}
		}
	}

	// All backends failed → generic error (don’t leak which one failed)
	http.Error(w, "invalid credentials", http.StatusUnauthorized)
	return

ISSUE:
	token, ierr := h.deps.authManager.IssueSession(w, r, claims)
	if ierr != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}
	writeJSON(w, LoginResponse{Token: token, User: claims.Username, Roles: claims.Roles})
}

// POST /auth/refresh
// @Summary Refresh session
// @Description Refresh session token if valid
// @Tags authentication
// @Accept json
// @Produce json
// @Success 204 "No Content on success"
// @Failure 401 {object} ErrorResponse
// @Router /auth/refresh [post]
func (h *handlers) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, err := h.deps.authManager.ValidateRequest(r) // read current cookie/header
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if _, err := h.deps.authManager.IssueSession(w, r, claims); err != nil {
		http.Error(w, "failed to refresh session", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
