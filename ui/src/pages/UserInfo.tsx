import React, { useEffect, useMemo, useState } from "react";
import {
  PageSection,
  Card,
  CardBody,
  Title,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Label,
  Button,
  Grid,
  GridItem,
  List,
  ListItem,
  Spinner,
  EmptyState,
  EmptyStateBody,
  Tooltip,
} from "@patternfly/react-core";
import {
  UserIcon,
  KeyIcon,
  ShieldAltIcon,
  ExclamationTriangleIcon,
  CheckCircleIcon,
  TimesCircleIcon,
  InfoCircleIcon,
} from "@patternfly/react-icons";
import { useAuth } from "../hooks/useAuth";
import { introspectApi } from "../api/client";
import type { Introspection } from "../types";

export function UserInfoPage() {
  const { user, token, logout } = useAuth();

  // Local state
  const [loading, setLoading] = useState(true);
  const [me, setMe] = useState<{
    user: string | null;
    roles: string[];
    provider?: string;
    email?: string;
    exp?: number;
    iat?: number;
  } | null>(null);
  const [rbac, setRBAC] = useState<Introspection | null>(null);
  const [error, setError] = useState<string | null>(null);

  // choose a small set of namespaces to display by default
  const namespacesToQuery = useMemo(() => ["default", "*"], []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        // Use a single source of truth: backend /introspect
        const perms: Introspection = await introspectApi.get({
          namespaces: namespacesToQuery,
        });

        const who = {
          user: perms?.user?.subject ?? null,
          roles: Array.isArray(perms?.user?.roles) ? perms.user.roles : [],
          provider: perms?.user?.provider,
          email: (perms as any)?.user?.email, // optional, if backend provides it
          exp: perms?.user?.exp,
          iat: perms?.user?.iat,
        };

        if (!cancelled) {
          setMe(who);
          setRBAC(perms);
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled)
          setError(e?.message || "Failed to load user permissions");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [namespacesToQuery]);

  // Decode JWT (if you still show it)
  const decodeJWTPayload = (t?: string | null) => {
    if (!t) return null;
    try {
      const parts = t.split(".");
      if (parts.length < 2) return null;
      const base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
      const json = decodeURIComponent(
        atob(base64)
          .split("")
          .map((c) => "%" + ("00" + c.charCodeAt(0).toString(16)).slice(-2))
          .join(""),
      );
      return JSON.parse(json);
    } catch {
      return null;
    }
  };

  const tokenPayload = useMemo(() => decodeJWTPayload(token), [token]);
  const isExpired = tokenPayload?.exp
    ? Date.now() / 1000 >= tokenPayload.exp
    : false;

  const renderBool = (v: boolean) =>
    v ? (
      <CheckCircleIcon className="pf-u-color-success-400" />
    ) : (
      <TimesCircleIcon className="pf-u-color-danger-400" />
    );

  // Ordered list of actions for namespace permissions table
  const actionOrder: Array<
    keyof NonNullable<Introspection["domains"][string]>["session"]
  > = ["get", "list", "watch", "create", "update", "delete", "scale"];

  return (
    <PageSection className="user-info-page">
      <div className="user-info-header">
        <Title headingLevel="h1" size="2xl">
          Account
        </Title>
      </div>

      <Grid hasGutter className="user-info-grid">
        {/* Account Card */}
        <GridItem lg={4}>
          <Card className="user-info-card">
            <CardBody>
              <div className="card-header">
                <UserIcon />
                <Title headingLevel="h3" size="lg">
                  Profile
                </Title>
              </div>

              <DescriptionList isCompact className="user-profile-list">
                <DescriptionListGroup>
                  <DescriptionListTerm>User</DescriptionListTerm>
                  <DescriptionListDescription>
                    <strong>{user || "Not authenticated"}</strong>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {me?.provider && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Provider</DescriptionListTerm>
                    <DescriptionListDescription>
                      {me.provider}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                {me?.email && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Email</DescriptionListTerm>
                    <DescriptionListDescription>
                      {me.email}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                {me?.roles?.length ? (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Roles</DescriptionListTerm>
                    <DescriptionListDescription>
                      <div className="role-tags">
                        {me.roles.map((r) => (
                          <Label key={r} color="blue" isCompact>
                            {r}
                          </Label>
                        ))}
                      </div>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ) : null}
              </DescriptionList>

              <Button variant="link" onClick={logout} className="logout-button">
                Sign out
              </Button>
            </CardBody>
          </Card>
        </GridItem>

        {/* Token Card */}
        <GridItem lg={4}>
          <Card className="user-info-card">
            <CardBody>
              <div className="card-header">
                <KeyIcon />
                <Title headingLevel="h3" size="lg">
                  Token
                </Title>
              </div>

              <DescriptionList isCompact>
                <DescriptionListGroup>
                  <DescriptionListTerm>Status</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color={isExpired ? "red" : "green"} isCompact>
                      {isExpired ? "Expired" : "Active"}
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {tokenPayload?.iat && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Issued</DescriptionListTerm>
                    <DescriptionListDescription>
                      {new Date(tokenPayload.iat * 1000).toLocaleString()}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                {tokenPayload?.exp && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Expires</DescriptionListTerm>
                    <DescriptionListDescription>
                      {new Date(tokenPayload.exp * 1000).toLocaleString()}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
              </DescriptionList>

              <div className="token-preview">
                <code>
                  {token ? `${token.substring(0, 40)}...` : "No token"}
                </code>
              </div>
            </CardBody>
          </Card>
        </GridItem>

        {/* Permissions Card */}
        <GridItem lg={4}>
          <Card className="user-info-card">
            <CardBody>
              <div className="card-header">
                <ShieldAltIcon />
                <Title headingLevel="h3" size="lg">
                  Permissions
                </Title>
              </div>

              {loading ? (
                <div className="loading-state">
                  <Spinner size="lg" />
                </div>
              ) : error ? (
                <EmptyState isSmall>
                  <EmptyStateBody>
                    <ExclamationTriangleIcon className="error-icon" />
                    <div>{error}</div>
                  </EmptyStateBody>
                </EmptyState>
              ) : rbac ? (
                <div className="permissions-content">
                  {/* Cluster permissions */}
                  <div className="permission-section">
                    <div className="permission-header">
                      <span>namespaces.list</span>
                      <Tooltip content="Required for 'All namespaces' discovery">
                        <InfoCircleIcon />
                      </Tooltip>
                    </div>
                    {renderBool(!!rbac?.cluster?.casbin?.namespaces?.list)}
                  </div>

                  {/* Namespace permissions */}
                  <div className="namespace-permissions">
                    <Title headingLevel="h4" size="md" className="pf-u-mb-sm">
                      Namespaces
                    </Title>
                    {Object.keys(rbac?.domains ?? {}).length === 0 ? (
                      <span className="no-namespaces">None</span>
                    ) : (
                      <div className="namespace-list">
                        {Object.entries(rbac.domains).map(([ns, obj]) => (
                          <div key={ns} className="namespace-item">
                            <div className="namespace-header">
                              <Label color="blue" isCompact>
                                {ns === "*" ? "All" : ns}
                              </Label>
                            </div>
                            <div className="namespace-actions">
                              {actionOrder.map((act) => (
                                <div key={act} className="action-item">
                                  <span>{act}</span>
                                  {renderBool(!!obj.session?.[act])}
                                </div>
                              ))}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              ) : null}
            </CardBody>
          </Card>
        </GridItem>
      </Grid>
    </PageSection>
  );
}
