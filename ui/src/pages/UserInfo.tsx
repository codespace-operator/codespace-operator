// Updated ui/src/pages/UserInfo.tsx

import { useEffect, useMemo, useState } from "react";
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
  Spinner,
  EmptyState,
  EmptyStateBody,
  Tooltip,
  Alert,
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
import {
  useUserIntrospection,
  useServerIntrospection,
} from "../hooks/useIntrospection";

export function UserInfoPage() {
  const { user, token, logout } = useAuth();

  // Use the new split hooks
  const {
    data: userInfo,
    loading: userLoading,
    error: userError,
  } = useUserIntrospection({
    namespaces: ["default", "*"], // Small set for display
    enabled: !!user,
  });

  const {
    data: serverInfo,
    loading: serverLoading,
    error: serverError,
  } = useServerIntrospection({
    discover: false, // Don't need full discovery for user info page
    enabled: !!user,
  });

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
    v ? <CheckCircleIcon color="green" /> : <TimesCircleIcon color="red" />;

  // Ordered list of actions for namespace permissions table
  const actionOrder = [
    "get",
    "list",
    "watch",
    "create",
    "update",
    "delete",
    "scale",
  ] as const;

  return (
    <PageSection>
      <Title headingLevel="h1" size="2xl">
        <UserIcon /> User Information
      </Title>

      <Grid hasGutter>
        {/* Account Card */}
        <GridItem span={12} md={6}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" size="lg">
                <UserIcon />
                Account
              </Title>

              {userLoading ? (
                <Spinner size="md" />
              ) : userError ? (
                <Alert variant="danger" title="Failed to load user information">
                  {userError}
                </Alert>
              ) : userInfo ? (
                <>
                  <DescriptionList>
                    <DescriptionListGroup>
                      <DescriptionListTerm>User</DescriptionListTerm>
                      <DescriptionListDescription>
                        {userInfo.user.username || user || "Not authenticated"}
                      </DescriptionListDescription>
                    </DescriptionListGroup>

                    {userInfo.user.subject && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>Subject</DescriptionListTerm>
                        <DescriptionListDescription>
                          {userInfo.user.subject}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}

                    {userInfo.user.provider && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>Provider</DescriptionListTerm>
                        <DescriptionListDescription>
                          {userInfo.user.provider}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}

                    {userInfo.user.email && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>Email</DescriptionListTerm>
                        <DescriptionListDescription>
                          {userInfo.user.email}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}

                    {userInfo.user.roles?.length ? (
                      <DescriptionListGroup>
                        <DescriptionListTerm>Roles</DescriptionListTerm>
                        <DescriptionListDescription>
                          <div>
                            {userInfo.user.roles.map((r) => (
                              <Label key={r} color="blue">
                                {r}
                              </Label>
                            ))}
                          </div>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    ) : null}

                    {userInfo.user.implicitRoles?.length ? (
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          <Tooltip content="Roles inherited through RBAC group membership">
                            <span>Inherited Roles</span>
                          </Tooltip>
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          <div>
                            {userInfo.user.implicitRoles.map((r) => (
                              <Label key={r} color="purple">
                                {r}
                              </Label>
                            ))}
                          </div>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    ) : null}
                  </DescriptionList>

                  <Button variant="secondary" onClick={logout}>
                    Sign out
                  </Button>
                </>
              ) : null}
            </CardBody>
          </Card>
        </GridItem>

        {/* Token Card */}
        <GridItem span={12} md={6}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" size="lg">
                <KeyIcon />
                Session Token
              </Title>

              <DescriptionList>
                <DescriptionListGroup>
                  <DescriptionListTerm>Status</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color={isExpired ? "red" : "green"}>
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

              <div>
                <code style={{ fontSize: "12px" }}>
                  {token ? `${token.substring(0, 40)}...` : "No token"}
                </code>
              </div>
            </CardBody>
          </Card>
        </GridItem>

        {/* User Permissions Card */}
        <GridItem span={12}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" size="lg">
                <ShieldAltIcon />
                Your Permissions
              </Title>

              {userLoading ? (
                <EmptyState>
                  <Spinner size="lg" />
                </EmptyState>
              ) : userError ? (
                <Alert variant="danger" title="Permission Error">
                  <EmptyStateBody>{userError}</EmptyStateBody>
                </Alert>
              ) : userInfo ? (
                <>
                  {/* Capabilities Summary */}
                  <Grid hasGutter style={{ marginBottom: "1rem" }}>
                    <GridItem span={6} md={3}>
                      <div>
                        <strong>Cluster Access</strong>
                        {renderBool(userInfo.capabilities.clusterScope)}
                      </div>
                    </GridItem>
                    <GridItem span={6} md={3}>
                      <div>
                        <strong>Admin Access</strong>
                        {renderBool(userInfo.capabilities.adminAccess)}
                      </div>
                    </GridItem>
                    <GridItem span={12} md={6}>
                      <div>
                        <strong>Accessible Namespaces</strong>
                        <span>
                          {" "}
                          ({userInfo.capabilities.namespaceScope.length})
                        </span>
                      </div>
                    </GridItem>
                  </Grid>

                  {/* Namespace permissions */}
                  <Title headingLevel="h3" size="md">
                    Session Permissions by Namespace
                  </Title>
                  {Object.keys(userInfo.domains).length === 0 ? (
                    <EmptyStateBody>No permissions found</EmptyStateBody>
                  ) : (
                    <div style={{ overflowX: "auto" }}>
                      {Object.entries(userInfo.domains).map(([ns, obj]) => (
                        <div key={ns} style={{ marginBottom: "1rem" }}>
                          <div>
                            <strong>
                              {ns === "*" ? "All Namespaces" : ns}
                            </strong>
                          </div>
                          <div>
                            {actionOrder.map((act) => (
                              <span key={act} style={{ marginRight: "1rem" }}>
                                <strong>{act}</strong>
                                {renderBool(!!obj.session?.[act])}
                              </span>
                            ))}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              ) : null}
            </CardBody>
          </Card>
        </GridItem>

        {/* Server Information Card (if available) */}
        {serverInfo && (
          <GridItem span={12}>
            <Card>
              <CardBody>
                <Title headingLevel="h2" size="lg">
                  <InfoCircleIcon />
                  Cluster Information
                </Title>

                {serverLoading ? (
                  <Spinner size="md" />
                ) : serverError ? (
                  <Alert variant="info" title="Server Information Unavailable">
                    {serverError ===
                    "Insufficient permissions to view server information"
                      ? "You don't have permissions to view cluster-level information."
                      : serverError}
                  </Alert>
                ) : (
                  <DescriptionList>
                    <DescriptionListGroup>
                      <DescriptionListTerm>
                        Server Can List Namespaces
                      </DescriptionListTerm>
                      <DescriptionListDescription>
                        {renderBool(
                          !!serverInfo.cluster?.casbin?.namespaces?.list,
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>

                    {serverInfo.namespaces?.all?.length && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          Total Namespaces
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          {serverInfo.namespaces.all.length}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}

                    {serverInfo.namespaces?.withSessions?.length && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          Namespaces with Sessions
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          {serverInfo.namespaces.withSessions.length}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}

                    <DescriptionListGroup>
                      <DescriptionListTerm>
                        Multi-Tenant Setup
                      </DescriptionListTerm>
                      <DescriptionListDescription>
                        {renderBool(serverInfo.capabilities.multiTenant)}
                      </DescriptionListDescription>
                    </DescriptionListGroup>

                    {serverInfo.version && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          Server Version
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          {serverInfo.version.version || "Unknown"}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}
                  </DescriptionList>
                )}
              </CardBody>
            </Card>
          </GridItem>
        )}
      </Grid>
    </PageSection>
  );
}
