import React, { useState } from "react";
import {
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
  Brand,
  List,
  ListItem,
} from "@patternfly/react-core";
import { ExclamationCircleIcon, KeyIcon, UserIcon } from "@patternfly/react-icons";
import { useAuth } from "../hooks/useAuth";
import logoUrl from "../assets/codespace-operator.svg?url";

export function LoginPage({ onLoggedIn }: { onLoggedIn?: () => void }) {
  const { user, login, logout, isLoading } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (isLoading) {
    return (
      <div className="login-container">
        <Card className="login-card">
          <CardBody className="pf-u-text-align-center">
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, justifyContent: 'center' }}>
              <Spinner size="md" />
              <span>Checking authentication...</span>
            </div>
          </CardBody>
        </Card>
      </div>
    );
  }

  if (user) {
    return (
      <div className="login-container">
        <Card className="login-card">
          <CardBody>
            <div className="pf-u-text-align-center pf-u-mb-lg">
              <Brand src={logoUrl} alt="Codespace Operator" style={{ width: 64, height: 64 }} />
              <Title headingLevel="h1" className="pf-u-mt-md pf-u-mb-sm">
                Welcome back
              </Title>
            </div>
            
            <div className="pf-u-text-align-center">
              <div className="pf-u-mb-md">
                <UserIcon className="pf-u-mr-sm" />
                <strong>{user}</strong>
              </div>
              <p className="pf-u-color-200 pf-u-mb-lg">
                You are already signed in to Codespace Operator
              </p>
              
              <div style={{ display: 'flex', gap: 12, justifyContent: 'center' }}>
                <Button variant="primary" onClick={() => onLoggedIn?.()}>
                  Continue to Dashboard
                </Button>
                <Button variant="secondary" onClick={logout}>
                  Sign out
                </Button>
              </div>
            </div>
          </CardBody>
        </Card>
      </div>
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
    <div className="login-container">
      <Card className="login-card">
        <CardBody>
          {/* Header */}
          <div className="pf-u-text-align-center pf-u-mb-lg">
            <Brand src={logoUrl} alt="Codespace Operator" style={{ width: 64, height: 64 }} />
            <Title headingLevel="h1" className="pf-u-mt-md pf-u-mb-sm">
              Sign in to Codespace Operator
            </Title>
            <p className="pf-u-color-200">
              Manage your development environments and IDE sessions
            </p>
          </div>
          
          {error && (
            <Alert 
              variant="danger" 
              title="Authentication failed" 
              className="pf-u-mb-md"
              isInline
              icon={<ExclamationCircleIcon />}
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
            </FormGroup>
            
            <Button 
              type="submit" 
              variant="primary" 
              isBlock
              isDisabled={!username.trim() || !password.trim() || isSubmitting}
              isLoading={isSubmitting}
              className="pf-u-mt-md"
            >
              {isSubmitting ? "Signing in..." : "Sign in"}
            </Button>
          </Form>
        </CardBody>
      </Card>
    </div>
  );
}