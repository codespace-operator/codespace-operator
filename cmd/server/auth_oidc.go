package main

import (
	"context"
	"crypto/tls"
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
