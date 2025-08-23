import React, { useState } from "react";
import {
  PageSection,
  Card,
  CardBody,
  Title,
  Form,
  FormGroup,
  TextInput,
  Button,
  HelperText,
  HelperTextItem,
  Alert,
  Spinner,
} from "@patternfly/react-core";
import { useAuth } from "../hooks/useAuth";

export function LoginPage({ onLoggedIn }: { onLoggedIn?: () => void }) {
  const { user, login, logout, isLoading } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (isLoading) {
    return (
      <PageSection isWidthLimited>
        <Card>
          <CardBody>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Spinner size="md" />
              <span>Checking authentication...</span>
            </div>
          </CardBody>
        </Card>
      </PageSection>
    );
  }

  if (user) {
    return (
      <PageSection isWidthLimited>
        <Card>
          <CardBody>
            <Title headingLevel="h2" style={{ marginBottom: 16 }}>
              Signed in as <strong>{user}</strong>
            </Title>
            <div style={{ display: 'flex', gap: 12 }}>
              <Button variant="primary" onClick={() => onLoggedIn?.()}>
                Go to Sessions
              </Button>
              <Button variant="secondary" onClick={logout}>
                Sign out
              </Button>
            </div>
          </CardBody>
        </Card>
      </PageSection>
    );
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      await login(username, password);
      onLoggedIn?.();
    } catch (err: any) {
      setError(err.message || "Login failed. Please try again.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <PageSection isWidthLimited>
      <Card>
        <CardBody>
          <Title headingLevel="h2" style={{ marginBottom: 24 }}>
            Sign in to Codespace Operator
          </Title>
          
          {error && (
            <Alert 
              variant="danger" 
              title="Login Error" 
              style={{ marginBottom: 16 }}
              isInline
            >
              {error}
            </Alert>
          )}

          <Form onSubmit={handleSubmit}>
            <FormGroup label="Username" fieldId="username" isRequired>
              <TextInput
                id="username"
                value={username}
                onChange={(_, v) => setUsername(v)}
                autoComplete="username"
                isDisabled={isSubmitting}
                placeholder="Enter your username"
              />
            </FormGroup>
            
            <FormGroup label="Password" fieldId="password" isRequired>
              <TextInput
                id="password"
                type="password"
                value={password}
                onChange={(_, v) => setPassword(v)}
                autoComplete="current-password"
                isDisabled={isSubmitting}
                placeholder="Enter your password"
              />
              <HelperText>
                <HelperTextItem variant="indeterminate">
                  Demo environment: accepts any username/password combination
                </HelperTextItem>
              </HelperText>
            </FormGroup>
            
            <Button 
              type="submit" 
              variant="primary" 
              isDisabled={!username.trim() || !password.trim() || isSubmitting}
              isLoading={isSubmitting}
            >
              {isSubmitting ? "Signing in..." : "Sign in"}
            </Button>
          </Form>
        </CardBody>
      </Card>
    </PageSection>
  );
}