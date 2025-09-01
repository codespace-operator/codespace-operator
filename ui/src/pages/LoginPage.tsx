import React, { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import {
  Button,
  Form,
  FormGroup,
  TextInput,
  Title,
  Card,
  CardBody,
  Alert,
  Brand,
} from "@patternfly/react-core";
import { useAuth } from "../hooks/useAuth";
import { authApi } from "../api/client";
import type { components } from "../types/api.gen";
import logoUrl from "../assets/codespace-operator.svg?url";

type AuthFeatures = components["schemas"]["internal_server.AuthFeatures"];

export function LoginPage({ onLoggedIn }: { onLoggedIn?: () => void }) {
  const { loginLocal, loginSSO } = useAuth();
  const location = useLocation() as any;
  const next = (location?.state?.from?.pathname as string) || "/sessions";

  const [features, setFeatures] = useState<AuthFeatures | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Load authentication features using the new API
  useEffect(() => {
    (async () => {
      try {
        const featuresData = await authApi.getFeatures();
        setFeatures(featuresData);
      } catch {
        setFeatures(null);
      }
    })();
  }, []);

  const onLocalSubmit: React.FormEventHandler = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await loginLocal(username, password);
      onLoggedIn?.();
    } catch (e: any) {
      setError(e?.message || "Login failed");
    } finally {
      setBusy(false);
    }
  };

  const onSSOClick = () => {
    loginSSO(next);
  };
  // Show loading state while features are being fetched
  if (!features) {
    return (
      <div className="login-container">
        <div className="login-content">
          <div className="login-brand-section">
            <div className="login-brand-content">
              <Brand
                src={logoUrl}
                alt="Codespace Operator"
                className="login-logo"
              />
              <div className="login-brand-text">
                <Title
                  headingLevel="h1"
                  size="4xl"
                  className="login-brand-title"
                >
                  Codespace Operator
                </Title>
                <p className="login-brand-subtitle">Loading...</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="login-container">
      <div className="login-content">
        {/* Left side - Branding and info */}
        <div className="login-brand-section">
          <div className="login-brand-content">
            <Brand
              src={logoUrl}
              alt="Codespace Operator"
              className="login-logo"
            />
            <div className="login-brand-text">
              <Title headingLevel="h1" size="4xl" className="login-brand-title">
                Codespace Operator
              </Title>
              <p className="login-brand-subtitle">
                Development environments orchestrator.
              </p>
            </div>
          </div>

          {/* Feature highlights */}
          <div className="login-features">
            <div className="login-feature-item">
              <strong>Secure Access</strong>
              <span>Enterprise authentication and authorization</span>
            </div>
            <div className="login-feature-item">
              <strong>Scalable Infrastructure</strong>
              <span>Dynamic resource allocation and management</span>
            </div>
            <div className="login-feature-item">
              <strong>Developer Ready</strong>
              <span>Pre-configured development environments</span>
            </div>
          </div>
        </div>

        {/* Right side - Login form */}
        <div className="login-form-section">
          <Card className="login-card">
            <CardBody className="login-card-body">
              <div className="login-header">
                <Title headingLevel="h2" size="2xl" className="login-title">
                  Sign in to your account
                </Title>
                <p className="login-subtitle">
                  Choose your preferred authentication method
                </p>
              </div>

              {error && (
                <Alert
                  variant="danger"
                  isInline
                  title="Authentication failed"
                  className="login-error"
                >
                  {error}
                </Alert>
              )}

              {!features.ssoEnabled && !features.localLoginEnabled && (
                <Alert
                  variant="warning"
                  isInline
                  title="No authentication methods available"
                  className="login-error"
                >
                  Please contact your administrator to configure authentication.
                </Alert>
              )}

              {features.ssoEnabled && (
                <div className="login-sso-section">
                  <Button
                    variant="primary"
                    isBlock
                    size="lg"
                    onClick={onSSOClick}
                    className="login-sso-button"
                  >
                    Continue with Single Sign-On
                  </Button>

                  {features.localLoginEnabled && (
                    <div className="login-divider">
                      <span className="login-divider-text">
                        or sign in with credentials
                      </span>
                    </div>
                  )}
                </div>
              )}

              {features.localLoginEnabled && (
                <Form onSubmit={onLocalSubmit} className="login-form">
                  <FormGroup
                    label="Username"
                    fieldId="username"
                    isRequired
                    className="login-form-group"
                  >
                    <TextInput
                      id="username"
                      name="username"
                      value={username}
                      onChange={(_, v) => setUsername(String(v))}
                      isRequired
                      placeholder="Enter your username"
                      className="login-input"
                    />
                  </FormGroup>

                  <FormGroup
                    label="Password"
                    fieldId="password"
                    isRequired
                    className="login-form-group"
                  >
                    <TextInput
                      id="password"
                      name="password"
                      type="password"
                      value={password}
                      onChange={(_, v) => setPassword(String(v))}
                      isRequired
                      placeholder="Enter your password"
                      className="login-input"
                    />
                  </FormGroup>

                  <Button
                    type="submit"
                    variant={features.ssoEnabled ? "secondary" : "primary"}
                    isBlock
                    size="lg"
                    isLoading={busy}
                    isDisabled={busy || !username || !password}
                    className="login-submit-button"
                  >
                    {busy ? "Signing in..." : "Sign in with Credentials"}
                  </Button>
                </Form>
              )}

              <div className="login-footer">
                <p className="login-help-text">
                  Having trouble signing in? Contact your administrator.
                </p>
              </div>
            </CardBody>
          </Card>
        </div>
      </div>
    </div>
  );
}
