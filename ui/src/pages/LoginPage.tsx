import React, { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import {
  Button,
  Form,
  FormGroup,
  TextInput,
  Title,
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
    <div style={{ maxWidth: 400, margin: "8vh auto", padding: 24 }}>
      <img
        src={logoUrl}
        alt="Codespace Operator"
        style={{ width: 64, height: 64, marginBottom: 16 }}
      />
      <Title headingLevel="h1">Sign in</Title>

      {features?.oidcEnabled && (
        <div style={{ margin: "16px 0" }}>
          <Button
            variant="primary"
            isBlock
            onClick={() => {
              window.location.href = `${base}/auth/login?next=${encodeURIComponent(next)}`;
            }}
          >
            Continue with Single Sign-On
          </Button>
        </div>
      )}

      <Form onSubmit={onSubmit}>
        <FormGroup label="Username" fieldId="u">
          <TextInput
            id="u"
            value={username}
            onChange={(_, v) => setUsername(String(v))}
            isRequired
          />
        </FormGroup>
        <FormGroup label="Password" fieldId="p">
          <TextInput
            id="p"
            type="password"
            value={password}
            onChange={(_, v) => setPassword(String(v))}
            isRequired
          />
        </FormGroup>
        {error && (
          <div
            style={{
              color: "var(--pf-global--danger-color--100)",
              marginBottom: 8,
            }}
          >
            {error}
          </div>
        )}
        <Button
          type="submit"
          isLoading={busy}
          isDisabled={busy}
          variant="secondary"
          isBlock
        >
          Sign in
        </Button>
      </Form>
    </div>
  );
}
