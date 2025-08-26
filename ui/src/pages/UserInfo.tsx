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
  Alert,
  Grid,
  GridItem,
  List,
  ListItem,
  CodeBlock,
  CodeBlockCode,
  Spinner,
  EmptyState,
  EmptyStateBody,
  Divider,
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
import { introspect, getMe, IntrospectResponse } from "../api/rbac";

export function UserInfoPage() {
  const { user, token, logout } = useAuth();
  const [loading, setLoading] = useState(true);
  const [me, setMe] = useState<Awaited<ReturnType<typeof getMe>> | null>(null);
  const [rbac, setRBAC] = useState<IntrospectResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // choose a small set of namespaces to display by default
  const namespacesToQuery = useMemo(() => ["default", "*"], []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        const [who, perms] = await Promise.all([
          getMe(),
          introspect(namespacesToQuery),
        ]);
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

  const renderBool = (v: boolean, hint?: string) =>
    v ? (
      <span className="pf-u-color-success-400">
        <CheckCircleIcon className="pf-u-mr-xs" />
        Allowed
      </span>
    ) : (
      <span className="pf-u-color-danger-400">
        <TimesCircleIcon className="pf-u-mr-xs" />
        Denied
      </span>
    );

  const actionOrder: Array<
    keyof IntrospectResponse["domains"][string]["session"]
  > = ["get", "list", "watch", "create", "update", "delete", "scale"];

  return (
    <PageSection isWidthLimited>
      <div className="pf-u-mb-lg">
        <Title headingLevel="h1" className="pf-u-mb-sm">
          User Management
        </Title>
        <p className="pf-u-color-200">
          View your account information, permissions, and authentication details
        </p>
      </div>

      <Grid hasGutter>
        <GridItem lg={6}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" className="pf-u-mb-md">
                <UserIcon className="pf-u-mr-sm" />
                Account Information
              </Title>

              <DescriptionList isHorizontal>
                <DescriptionListGroup>
                  <DescriptionListTerm>Username</DescriptionListTerm>
                  <DescriptionListDescription>
                    <strong>{user || "Not authenticated"}</strong>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {me?.provider && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Auth Provider</DescriptionListTerm>
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

                {tokenPayload && (
                  <>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Token Status</DescriptionListTerm>
                      <DescriptionListDescription>
                        <Label color={isExpired ? "red" : "green"}>
                          {isExpired ? "Expired" : "Active"}
                        </Label>
                      </DescriptionListDescription>
                    </DescriptionListGroup>

                    <DescriptionListGroup>
                      <DescriptionListTerm>Issued</DescriptionListTerm>
                      <DescriptionListDescription>
                        {tokenPayload.iat
                          ? new Date(tokenPayload.iat * 1000).toLocaleString()
                          : "Unknown"}
                      </DescriptionListDescription>
                    </DescriptionListGroup>

                    <DescriptionListGroup>
                      <DescriptionListTerm>Expires</DescriptionListTerm>
                      <DescriptionListDescription>
                        {tokenPayload.exp
                          ? new Date(tokenPayload.exp * 1000).toLocaleString()
                          : "Never"}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  </>
                )}

                {me?.roles?.length ? (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Roles / Groups</DescriptionListTerm>
                    <DescriptionListDescription>
                      {me.roles.map((r) => (
                        <Label
                          key={r}
                          color="blue"
                          className="pf-u-mr-sm pf-u-mb-sm"
                        >
                          {r}
                        </Label>
                      ))}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ) : null}
              </DescriptionList>

              <div className="pf-u-mt-lg">
                <Button variant="secondary" onClick={logout}>
                  Sign out
                </Button>
              </div>
            </CardBody>
          </Card>

          <Card className="pf-u-mt-md">
            <CardBody>
              <Title headingLevel="h2" className="pf-u-mb-md">
                <KeyIcon className="pf-u-mr-sm" />
                Authentication Token
              </Title>

              {isExpired && (
                <Alert
                  variant="warning"
                  isInline
                  title="Token Expired"
                  className="pf-u-mb-md"
                >
                  Your authentication token has expired. Please sign in again.
                </Alert>
              )}

              <CodeBlock>
                <CodeBlockCode>
                  {token
                    ? `${token.substring(0, 50)}...`
                    : "No token available"}
                </CodeBlockCode>
              </CodeBlock>

              <p className="pf-u-mt-sm pf-u-color-200 pf-u-font-size-sm">
                JWT tokens contain your identity information and are used to
                authenticate API requests.
              </p>
            </CardBody>
          </Card>
        </GridItem>

        <GridItem lg={6}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" className="pf-u-mb-md">
                <ShieldAltIcon className="pf-u-mr-sm" />
                Permissions & RBAC
              </Title>

              {loading ? (
                <div className="pf-u-text-align-center pf-u-py-lg">
                  <Spinner size="lg" />
                  <div className="pf-u-mt-md">Loading permissions...</div>
                </div>
              ) : error ? (
                <EmptyState>
                  <EmptyStateBody>
                    <ExclamationTriangleIcon
                      className="pf-u-mb-md"
                      style={{
                        fontSize: "2rem",
                        color: "var(--pf-global--warning-color--100)",
                      }}
                    />
                    <div>{error}</div>
                  </EmptyStateBody>
                </EmptyState>
              ) : rbac ? (
                <>
                  <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                    Cluster Permissions
                  </Title>
                  <div
                    className="pf-u-mb-lg pf-u-p-md"
                    style={{
                      backgroundColor: "rgba(0,0,0,0.05)",
                      borderRadius: 4,
                    }}
                  >
                    <div className="pf-u-display-flex pf-u-align-items-center pf-u-mb-sm">
                      <strong>namespaces.list</strong>
                      <Tooltip content="Required for 'All namespaces' discovery">
                        <InfoCircleIcon className="pf-u-ml-sm pf-u-color-200" />
                      </Tooltip>
                    </div>
                    {renderBool(rbac.cluster.namespaces.list)}
                  </div>

                  <Divider className="pf-u-mb-md" />

                  <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                    Effective Permissions by Namespace
                  </Title>

                  {Object.keys(rbac.domains).length === 0 ? (
                    <p className="pf-u-color-200">No namespaces to display.</p>
                  ) : (
                    <div>
                      {Object.entries(rbac.domains).map(([ns, obj]) => (
                        <div
                          key={ns}
                          className="pf-u-mb-md pf-u-p-md"
                          style={{
                            backgroundColor: "rgba(0,0,0,0.05)",
                            borderRadius: 4,
                          }}
                        >
                          <div className="pf-u-display-flex pf-u-justify-content-space-between pf-u-align-items-center pf-u-mb-sm">
                            <strong>Namespace</strong>
                            <Label color="blue" variant="filled">
                              {ns === "*" ? "All namespaces" : ns}
                            </Label>
                          </div>

                          <List isPlain>
                            {actionOrder.map((act) => (
                              <ListItem
                                key={act}
                                className="pf-u-font-size-sm pf-u-color-200"
                              >
                                <span className="pf-u-mr-sm">
                                  session.{act}
                                </span>
                                {renderBool(obj.session[act])}
                              </ListItem>
                            ))}
                          </List>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              ) : null}
            </CardBody>
          </Card>
        </GridItem>
      </Grid>

      <Card className="pf-u-mt-lg">
        <CardBody>
          <Title headingLevel="h2" className="pf-u-mb-md">
            Integration Information
          </Title>

          <Alert
            variant="info"
            isInline
            title="OIDC + Casbin"
            className="pf-u-mb-md"
          >
            The server validates OIDC tokens and issues a short-lived session
            cookie. Casbin evaluates permissions dynamically from a ConfigMap
            and supports hot-reload.
          </Alert>

          <DescriptionList isHorizontal>
            <DescriptionListGroup>
              <DescriptionListTerm>Authentication</DescriptionListTerm>
              <DescriptionListDescription>
                {me?.provider || "N/A"}
              </DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>RBAC Engine</DescriptionListTerm>
              <DescriptionListDescription>
                Casbin (model/policy from ConfigMap)
              </DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        </CardBody>
      </Card>
    </PageSection>
  );
}
