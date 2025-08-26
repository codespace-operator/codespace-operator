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
import logoUrl from "../assets/codespace-operator.svg?url";

const base = import.meta.env.VITE_API_BASE || "";

export function LoginPage({ onLoggedIn }: { onLoggedIn?: () => void }) {
  const { login } = useAuth();
  const location = useLocation() as any;
  const next = (location?.state?.from?.pathname as string) || "/sessions";

  const [features, setFeatures] = useState<{
    oidcEnabled: boolean;
    localLoginPath: string;
  } | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const r = await fetch(`${base}/auth/features`, {
          credentials: "include",
        });
        if (r.ok) setFeatures(await r.json());
      } catch {
        setFeatures(null);
      }
    })();
  }, []);

  const onSubmit: React.FormEventHandler = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await login(username, password);
      onLoggedIn?.();
    } catch (e: any) {
      setError(e?.message || "Login failed");
    } finally {
      setBusy(false);
    }
  };

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

          {/* Optional: Add some feature highlights */}
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
                  Enter your credentials to access the platform
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

              {features?.oidcEnabled && (
                <div className="login-sso-section">
                  <Button
                    variant="primary"
                    isBlock
                    size="lg"
                    onClick={() => {
                      window.location.href = `${base}/auth/login?next=${encodeURIComponent(next)}`;
                    }}
                    className="login-sso-button"
                  >
                    Continue with Single Sign-On
                  </Button>

                  <div className="login-divider">
                    <span className="login-divider-text">
                      or sign in with credentials
                    </span>
                  </div>
                </div>
              )}

              <Form onSubmit={onSubmit} className="login-form">
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
                  variant="primary"
                  isBlock
                  size="lg"
                  isLoading={busy}
                  isDisabled={busy || !username || !password}
                  className="login-submit-button"
                >
                  {busy ? "Signing in..." : "Sign in"}
                </Button>
              </Form>

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
