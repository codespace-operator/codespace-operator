import React, { useMemo } from "react";
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
import {
  useUserIntrospection,
  useServerIntrospection,
} from "../hooks/useIntrospection";

export function UserInfoPage() {
  const { user, token, logout } = useAuth();

  // New split hooks (API)
  const {
    data: userInfo,
    loading: userLoading,
    error: userError,
  } = useUserIntrospection({
    namespaces: ["default", "*"],
    enabled: !!user,
  });

  const {
    data: serverInfo,
    loading: serverLoading,
    error: serverError,
  } = useServerIntrospection({
    discover: false,
    enabled: !!user,
  });

  // JWT decode (unchanged)
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

  // Ordered list of actions for per-namespace table
  const actionOrder = [
    "get",
    "list",
    "watch",
    "create",
    "update",
    "delete",
    "scale",
  ] as const;

  const loading = userLoading || serverLoading;
  const fatalError = userError && !userInfo ? userError : null;

  // Helper to render an action bag with existing classes only
  const renderActionBag = (
    bag?: Partial<Record<(typeof actionOrder)[number], boolean>> | null,
  ) => {
    if (!bag) return null;
    return (
      <div className="namespace-actions">
        {actionOrder.map((act) => (
          <div key={act} className="action-item">
            <span>{act}</span>
            {renderBool(!!bag[act])}
          </div>
        ))}
      </div>
    );
  };

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
                    <strong>
                      {userInfo?.user?.username || user || "Not authenticated"}
                    </strong>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {userInfo?.user?.subject && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Subject</DescriptionListTerm>
                    <DescriptionListDescription>
                      {userInfo.user.subject}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                {userInfo?.user?.provider && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Provider</DescriptionListTerm>
                    <DescriptionListDescription>
                      {userInfo.user.provider}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                {userInfo?.user?.email && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Email</DescriptionListTerm>
                    <DescriptionListDescription>
                      {userInfo.user.email}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                {userInfo?.user?.roles?.length ? (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Roles</DescriptionListTerm>
                    <DescriptionListDescription>
                      <div className="role-tags">
                        {userInfo.user.roles.map((r: string) => (
                          <Label key={r} color="blue" isCompact>
                            {r}
                          </Label>
                        ))}
                      </div>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ) : null}

                {userInfo?.user?.implicitRoles?.length ? (
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      <Tooltip content="Roles inherited through RBAC group membership">
                        <span>Inherited Roles</span>
                      </Tooltip>
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      <div className="role-tags">
                        {userInfo.user.implicitRoles.map((r: string) => (
                          <Label key={r} color="purple" isCompact>
                            {r}
                          </Label>
                        ))}
                      </div>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ) : null}
              </DescriptionList>
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
              ) : fatalError ? (
                <EmptyState isSmall>
                  <EmptyStateBody>
                    <ExclamationTriangleIcon className="error-icon" />
                    <div>{fatalError}</div>
                  </EmptyStateBody>
                </EmptyState>
              ) : userInfo ? (
                <div className="permissions-content">
                  {/* Capabilities summary (no new classes) */}
                  <div className="permission-section">
                    <div className="permission-header">
                      <span>Cluster Access</span>
                    </div>
                    {renderBool(!!userInfo.capabilities?.clusterScope)}
                  </div>

                  <div className="permission-section">
                    <div className="permission-header">
                      <span>Admin Access</span>
                    </div>
                    {renderBool(!!userInfo.capabilities?.adminAccess)}
                  </div>

                  <div className="permission-section">
                    <div className="permission-header">
                      <span>Accessible Namespaces</span>
                    </div>
                    <span>
                      {Array.isArray(userInfo.capabilities?.namespaceScope)
                        ? userInfo.capabilities.namespaceScope.length
                        : 0}
                    </span>
                  </div>

                  {/* Cluster-level bit from server introspection */}
                  <div className="permission-section">
                    <div className="permission-header">
                      <span>namespaces.list</span>
                      <Tooltip content="Required for 'All namespaces' discovery">
                        <InfoCircleIcon />
                      </Tooltip>
                    </div>
                    {serverInfo ? (
                      renderBool(
                        !!serverInfo?.cluster?.casbin?.namespaces?.list,
                      )
                    ) : (
                      <Tooltip content={serverError || "Unavailable"}>
                        <InfoCircleIcon />
                      </Tooltip>
                    )}
                  </div>

                  {/* Per-namespace permissions */}
                  <div className="namespace-permissions">
                    <Title headingLevel="h4" size="md" className="pf-u-mb-sm">
                      Namespaces
                    </Title>

                    {Object.keys(userInfo?.domains ?? {}).length === 0 ? (
                      <span className="no-namespaces">None</span>
                    ) : (
                      <div className="namespace-list">
                        {Object.entries(userInfo.domains).map(([ns, obj]) => {
                          const domain = obj as any;
                          const sessionBag =
                            (domain?.session as Partial<
                              Record<(typeof actionOrder)[number], boolean>
                            >) || null;

                          // Optional extra bag if your API provides it (rbac/granted/static)
                          const nonSessionBag =
                            (domain?.rbac ||
                              domain?.granted ||
                              domain?.static) ??
                            null;

                          return (
                            <div key={ns} className="namespace-item">
                              <div className="namespace-header">
                                <Label color="blue" isCompact>
                                  {ns === "*" ? "All" : ns}
                                </Label>
                              </div>

                              {/* Session permissions */}
                              {sessionBag ? (
                                <>
                                  <div>Session</div>
                                  {renderActionBag(sessionBag)}
                                </>
                              ) : null}

                              {/* Optional: RBAC / Static */}
                              {nonSessionBag ? (
                                <>
                                  <div>RBAC / Static</div>
                                  {renderActionBag(nonSessionBag)}
                                </>
                              ) : null}
                            </div>
                          );
                        })}
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
