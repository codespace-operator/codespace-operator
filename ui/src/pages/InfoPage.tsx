import React from "react";
import {
  PageSection,
  Card,
  CardBody,
  Title,
  List,
  ListItem,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Grid,
  GridItem,
  Label,
  CodeBlock,
  CodeBlockCode,
  Alert,
} from "@patternfly/react-core";
import {
  CubesIcon,
  ServerIcon,
  CogIcon,
  InfoCircleIcon,
  ExclamationTriangleIcon,
} from "@patternfly/react-icons";

export function InfoPage() {
  return (
    <PageSection isWidthLimited style={{ padding: "1rem" }}>
      <div className="pf-u-mb-lg">
        <Title headingLevel="h1" className="pf-u-mb-sm">
          Cluster Settings
        </Title>
        <p className="pf-u-color-200">
          View cluster information, operator configuration, and system status
        </p>
      </div>

      <Grid hasGutter>
        <GridItem lg={6}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" className="pf-u-mb-md">
                <ServerIcon className="pf-u-mr-sm" />
                Operator Information
              </Title>

              <DescriptionList isHorizontal>
                <DescriptionListGroup>
                  <DescriptionListTerm>Version</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="green">v0.1.0-dev</Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Namespace</DescriptionListTerm>
                  <DescriptionListDescription>
                    codespace-operator-system
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Controller Status</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="green">Running</Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>API Version</DescriptionListTerm>
                  <DescriptionListDescription>
                    codespace.codespace.dev/v1
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Webhook Status</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="orange">Disabled</Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>

          <Card className="pf-u-mt-md">
            <CardBody>
              <Title headingLevel="h2" className="pf-u-mb-md">
                <CubesIcon className="pf-u-mr-sm" />
                Resource Limits
              </Title>

              <Alert
                variant="info"
                isInline
                title="Default Configuration"
                className="pf-u-mb-md"
              >
                These are the default resource limits for new sessions. Users
                can override these in their session specifications.
              </Alert>

              <DescriptionList>
                <DescriptionListGroup>
                  <DescriptionListTerm>
                    Max Sessions per Namespace
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    Unlimited
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Default CPU Limit</DescriptionListTerm>
                  <DescriptionListDescription>1000m</DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>
                    Default Memory Limit
                  </DescriptionListTerm>
                  <DescriptionListDescription>2Gi</DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>
                    Default Storage Size
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    10Gi (Home), 5Gi (Scratch)
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        <GridItem lg={6}>
          <Card>
            <CardBody>
              <Title headingLevel="h2" className="pf-u-mb-md">
                <CogIcon className="pf-u-mr-sm" />
                Configuration
              </Title>

              <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                Supported IDEs
              </Title>
              <List className="pf-u-mb-lg">
                <ListItem>
                  <strong>JupyterLab</strong> - Interactive data science
                  environment
                  <CodeBlock className="pf-u-mt-xs">
                    <CodeBlockCode>
                      jupyter/minimal-notebook:latest
                    </CodeBlockCode>
                  </CodeBlock>
                </ListItem>
                <ListItem>
                  <strong>VS Code Server</strong> - Web-based VS Code editor
                  <CodeBlock className="pf-u-mt-xs">
                    <CodeBlockCode>codercom/code-server:latest</CodeBlockCode>
                  </CodeBlock>
                </ListItem>
                <ListItem>
                  <strong>Custom</strong> - Bring your own container image
                </ListItem>
              </List>

              <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                Authentication Modes
              </Title>
              <List className="pf-u-mb-lg">
                <ListItem>
                  <strong>None</strong> - Direct access (development only)
                </ListItem>
                <ListItem>
                  <strong>OAuth2 Proxy</strong> - OIDC-based authentication
                </ListItem>
              </List>

              <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                Storage Options
              </Title>
              <List>
                <ListItem>
                  <strong>Home Volume</strong> - Persistent user workspace
                </ListItem>
                <ListItem>
                  <strong>Scratch Volume</strong> - Temporary high-performance
                  storage
                </ListItem>
                <ListItem>
                  <strong>Storage Classes</strong> - Configurable per deployment
                </ListItem>
              </List>
            </CardBody>
          </Card>
        </GridItem>
      </Grid>

      <Card className="pf-u-mt-lg">
        <CardBody>
          <Title headingLevel="h2" className="pf-u-mb-md">
            <InfoCircleIcon className="pf-u-mr-sm" />
            Deployment Information
          </Title>

          <Grid hasGutter>
            <GridItem md={4}>
              <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                Current Features
              </Title>
              <List isPlain>
                <ListItem>✅ Session lifecycle management</ListItem>
                <ListItem>✅ Multi-IDE support</ListItem>
                <ListItem>✅ Persistent storage</ListItem>
                <ListItem>✅ Ingress configuration</ListItem>
                <ListItem>✅ Scaling support</ListItem>
                <ListItem>✅ OAuth2 Proxy integration</ListItem>
              </List>
            </GridItem>

            <GridItem md={4}>
              <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                Planned Features
              </Title>
              <List isPlain>
                <ListItem>🚧 Resource quotas</ListItem>
                <ListItem>🚧 Session templates</ListItem>
                <ListItem>🚧 Multi-tenancy</ListItem>
                <ListItem>🚧 Git integration</ListItem>
                <ListItem>🚧 Container registry integration</ListItem>
                <ListItem>🚧 Backup/restore</ListItem>
              </List>
            </GridItem>

            <GridItem md={4}>
              <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                Integration Roadmap
              </Title>
              <List isPlain>
                <ListItem>✅ Keycloak SSO</ListItem>
                <ListItem>📋 LDAP/Active Directory</ListItem>
                <ListItem>📋 Prometheus monitoring</ListItem>
                <ListItem>📋 Grafana dashboards</ListItem>
                <ListItem>✅ Helm chart</ListItem>
              </List>
            </GridItem>
          </Grid>
        </CardBody>
      </Card>

      <Card className="pf-u-mt-md">
        <CardBody>
          <Alert
            variant="warning"
            title="Development Environment"
            icon={<ExclamationTriangleIcon />}
            className="pf-u-mb-md"
          >
            This operator is currently in active development. Features and APIs
            may change without notice. Not recommended for production workloads
            at this time.
          </Alert>

          <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
            Getting Started
          </Title>
          <List>
            <ListItem>
              <strong>Documentation:</strong> Visit our GitHub repository for
              setup guides and examples
            </ListItem>
            <ListItem>
              <strong>Community:</strong> Join our discussions for support and
              feature requests
            </ListItem>
            <ListItem>
              <strong>Contributing:</strong> PRs and issues are welcome on our
              GitHub project
            </ListItem>
          </List>
        </CardBody>
      </Card>
    </PageSection>
  );
}
