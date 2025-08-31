import {
  PageSection,
  Card,
  CardBody,
  Title,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Grid,
  GridItem,
  Label,
  Spinner,
  EmptyState,
  EmptyStateBody,
} from "@patternfly/react-core";
import {
  ServerIcon,
  CogIcon,
  ShieldAltIcon,
  ExclamationTriangleIcon,
  CheckCircleIcon,
  TimesCircleIcon,
} from "@patternfly/react-icons";
import {
  useUserIntrospection,
  useServerIntrospection,
} from "../hooks/useIntrospection";

export function ClusterSettingsPage() {
  const { data: userInfo, loading: userLoading } = useUserIntrospection({
    discover: true,
    enabled: true,
  });

  const { data: serverInfo, loading: serverLoading } = useServerIntrospection({
    discover: true,
    enabled: true,
  });

  const loading = userLoading || serverLoading;

  const renderBool = (v: boolean) =>
    v ? (
      <CheckCircleIcon className="pf-u-color-success-400" />
    ) : (
      <TimesCircleIcon className="pf-u-color-danger-400" />
    );

  return (
    <PageSection className="info-page">
      <div className="info-header">
        <Title headingLevel="h1" size="2xl">
          Administration
        </Title>
      </div>

      <Grid hasGutter className="info-grid">
        {/* User Capabilities Card */}

        {/* Server Service Account Card */}
        <GridItem lg={4}>
          <Card className="info-card">
            <CardBody>
              <div className="card-header">
                <ShieldAltIcon />
                <Title headingLevel="h3" size="lg">
                  Server Service Account
                </Title>
              </div>

              {loading ? (
                <Spinner size="lg" />
              ) : serverInfo?.cluster?.serverServiceAccount ? (
                <DescriptionList isCompact>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Namespace List</DescriptionListTerm>
                    <DescriptionListDescription>
                      {renderBool(
                        !!serverInfo.cluster.serverServiceAccount.namespaces
                          ?.list,
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>Namespace Watch</DescriptionListTerm>
                    <DescriptionListDescription>
                      {renderBool(
                        !!serverInfo.cluster.serverServiceAccount.namespaces
                          ?.watch,
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      Session Permissions
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      <div
                        style={{
                          display: "flex",
                          flexWrap: "wrap",
                          gap: "4px",
                        }}
                      >
                        {Object.entries(
                          serverInfo.cluster.serverServiceAccount.session || {},
                        ).map(([action, allowed]) => (
                          <Label
                            key={action}
                            color={allowed ? "green" : "red"}
                            isCompact
                          >
                            {action}
                          </Label>
                        ))}
                      </div>
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>Casbin RBAC</DescriptionListTerm>
                    <DescriptionListDescription>
                      <div style={{ display: "flex", gap: "8px" }}>
                        <span>
                          List:{" "}
                          {renderBool(
                            !!serverInfo.cluster.casbin?.namespaces?.list,
                          )}
                        </span>
                        <span>
                          Watch:{" "}
                          {renderBool(
                            !!serverInfo.cluster.casbin?.namespaces?.watch,
                          )}
                        </span>
                      </div>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              ) : (
                <EmptyState isSmall>
                  <EmptyStateBody>
                    <ExclamationTriangleIcon className="error-icon" />
                    <div>Server service account info unavailable</div>
                  </EmptyStateBody>
                </EmptyState>
              )}
            </CardBody>
          </Card>
        </GridItem>

        {/* Server Capabilities Card */}
        <GridItem lg={4}>
          <Card className="info-card">
            <CardBody>
              <div className="card-header">
                <ServerIcon />
                <Title headingLevel="h3" size="lg">
                  Server Capabilities
                </Title>
              </div>

              {loading ? (
                <Spinner size="lg" />
              ) : serverInfo ? (
                <DescriptionList isCompact>
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      Cluster Scope Mode
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {renderBool(!!serverInfo.capabilities?.clusterScope)}
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>Multi-Tenant</DescriptionListTerm>
                    <DescriptionListDescription>
                      {renderBool(!!serverInfo.capabilities?.multiTenant)}
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      Server SA - Namespace List
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {renderBool(
                        !!serverInfo.cluster?.serverServiceAccount?.namespaces
                          ?.list,
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      Server SA - Session Actions
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      <div
                        style={{
                          display: "flex",
                          flexWrap: "wrap",
                          gap: "4px",
                        }}
                      >
                        {Object.entries(
                          serverInfo.cluster?.serverServiceAccount?.session ||
                            {},
                        ).map(([action, allowed]) => (
                          <Label
                            key={action}
                            color={allowed ? "green" : "red"}
                            isCompact
                          >
                            {action}
                          </Label>
                        ))}
                      </div>
                    </DescriptionListDescription>
                  </DescriptionListGroup>

                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      Discovered Namespaces
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      <Label color="blue" isCompact>
                        All: {serverInfo.namespaces?.all?.length || 0}
                      </Label>{" "}
                      <Label color="purple" isCompact>
                        With Sessions:{" "}
                        {serverInfo.namespaces?.withSessions?.length || 0}
                      </Label>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              ) : (
                <EmptyState isSmall>
                  <EmptyStateBody>
                    <ExclamationTriangleIcon className="error-icon" />
                    <div>
                      Server information unavailable (insufficient permissions)
                    </div>
                  </EmptyStateBody>
                </EmptyState>
              )}
            </CardBody>
          </Card>
        </GridItem>

        {/* System Info Card */}
        <GridItem lg={4}>
          <Card className="info-card">
            <CardBody>
              <div className="card-header">
                <CogIcon />
                <Title headingLevel="h3" size="lg">
                  System Info
                </Title>
              </div>

              <DescriptionList isCompact>
                <DescriptionListGroup>
                  <DescriptionListTerm>Version</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="green" isCompact>
                      {serverInfo?.version?.version || "1.0.0"}
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Build Date</DescriptionListTerm>
                  <DescriptionListDescription>
                    {serverInfo?.version?.buildDate || "Unknown"}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Git Commit</DescriptionListTerm>
                  <DescriptionListDescription>
                    <code>
                      {serverInfo?.version?.gitCommit?.substring(0, 8) ||
                        "Unknown"}
                    </code>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>API Version</DescriptionListTerm>
                  <DescriptionListDescription>
                    codespace.codespace.dev/v1
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Authentication</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="green" isCompact>
                      RBAC Enabled
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {/* Namespaces Detail - Full Width */}
        <GridItem span={12}>
          <Card className="info-card">
            <CardBody>
              <Title headingLevel="h3" size="lg" className="pf-u-mb-md">
                Namespace Access Matrix
              </Title>

              {loading ? (
                <Spinner size="lg" />
              ) : userInfo?.domains ? (
                <Grid hasGutter>
                  {Object.entries(userInfo.domains ?? {})
                    .filter(([ns]) => ns !== "*")
                    .map(([ns, permissions]) => {
                      const nsDisplay = ns === "*" ? "All Namespaces" : ns;
                      const sessionPerms = (permissions as any)?.session || {};

                      return (
                        <GridItem key={ns} md={6} lg={4}>
                          <Card isCompact>
                            <CardBody>
                              <Title
                                headingLevel="h4"
                                size="md"
                                className="pf-u-mb-sm"
                              >
                                {nsDisplay}
                              </Title>
                              <div
                                style={{
                                  display: "flex",
                                  flexWrap: "wrap",
                                  gap: "4px",
                                }}
                              >
                                {Object.entries(sessionPerms).map(
                                  ([action, allowed]) => (
                                    <Label
                                      key={action}
                                      color={allowed ? "green" : "red"}
                                      isCompact
                                    >
                                      {action}
                                    </Label>
                                  ),
                                )}
                              </div>
                            </CardBody>
                          </Card>
                        </GridItem>
                      );
                    })}
                </Grid>
              ) : (
                <EmptyState>
                  <EmptyStateBody>
                    No namespace permissions available
                  </EmptyStateBody>
                </EmptyState>
              )}
            </CardBody>
          </Card>
        </GridItem>
      </Grid>
    </PageSection>
  );
}
