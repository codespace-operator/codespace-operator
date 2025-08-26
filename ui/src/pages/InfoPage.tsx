import React from "react";
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
} from "@patternfly/react-core";
import { CubesIcon, ServerIcon, CogIcon } from "@patternfly/react-icons";

export function InfoPage() {
  return (
    <PageSection className="info-page">
      <div className="info-header">
        <Title headingLevel="h1" size="2xl">
          System
        </Title>
      </div>

      <Grid hasGutter className="info-grid">
        {/* Operator Card */}
        <GridItem lg={4}>
          <Card className="info-card">
            <CardBody>
              <div className="card-header">
                <ServerIcon />
                <Title headingLevel="h3" size="lg">
                  Operator
                </Title>
              </div>

              <DescriptionList isCompact>
                <DescriptionListGroup>
                  <DescriptionListTerm>Version</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="green" isCompact>
                      {/* !TODO: stamp during build */}
                      1.0.0
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Namespace</DescriptionListTerm>
                  <DescriptionListDescription>
                    codespace-operator-system
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Controller</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="green" isCompact>
                      Running
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>API</DescriptionListTerm>
                  <DescriptionListDescription>
                    codespace.codespace.dev/v1
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Webhook</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="orange" isCompact>
                      Disabled
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {/* Resources Card */}
        <GridItem lg={4}>
          <Card className="info-card">
            <CardBody>
              <div className="card-header">
                <CubesIcon />
                <Title headingLevel="h3" size="lg">
                  Resources
                </Title>
              </div>

              <DescriptionList isCompact>
                <DescriptionListGroup>
                  <DescriptionListTerm>CPU Limit</DescriptionListTerm>
                  <DescriptionListDescription>1000m</DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Memory Limit</DescriptionListTerm>
                  <DescriptionListDescription>2Gi</DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Home Storage</DescriptionListTerm>
                  <DescriptionListDescription>10Gi</DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Scratch Storage</DescriptionListTerm>
                  <DescriptionListDescription>5Gi</DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Max Sessions</DescriptionListTerm>
                  <DescriptionListDescription>
                    Unlimited
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {/* Configuration Card */}
        <GridItem lg={4}>
          <Card className="info-card">
            <CardBody>
              <div className="card-header">
                <CogIcon />
                <Title headingLevel="h3" size="lg">
                  Configuration
                </Title>
              </div>

              <div className="config-section">
                <Title headingLevel="h4" size="md">
                  IDEs
                </Title>
                <div className="config-items">
                  <div className="config-item">
                    <span>JupyterLab</span>
                    <Label color="blue" isCompact>
                      Available
                    </Label>
                  </div>
                  <div className="config-item">
                    <span>VS Code</span>
                    <Label color="blue" isCompact>
                      Available
                    </Label>
                  </div>
                  <div className="config-item">
                    <span>Custom</span>
                    <Label color="grey" isCompact>
                      Supported
                    </Label>
                  </div>
                </div>
              </div>

              <div className="config-section">
                <Title headingLevel="h4" size="md">
                  Authentication
                </Title>
                <div className="config-items">
                  <div className="config-item">
                    <span>OAuth2 Proxy</span>
                    <Label color="green" isCompact>
                      Enabled
                    </Label>
                  </div>
                  <div className="config-item">
                    <span>Direct Access</span>
                    <Label color="orange" isCompact>
                      Dev Only
                    </Label>
                  </div>
                </div>
              </div>

              <div className="config-section">
                <Title headingLevel="h4" size="md">
                  Storage
                </Title>
                <div className="config-items">
                  <div className="config-item">
                    <span>Persistent</span>
                    <Label color="green" isCompact>
                      Available
                    </Label>
                  </div>
                  <div className="config-item">
                    <span>Scratch</span>
                    <Label color="blue" isCompact>
                      Available
                    </Label>
                  </div>
                </div>
              </div>
            </CardBody>
          </Card>
        </GridItem>

        {/* Features Status - Full Width */}
        <GridItem span={12}>
          <Card className="info-card">
            <CardBody>
              <Title headingLevel="h3" size="lg" className="features-title">
                Feature Status
              </Title>

              <Grid hasGutter>
                <GridItem md={4}>
                  <div className="feature-group">
                    <Title headingLevel="h4" size="md">
                      Core Features
                    </Title>
                    <div className="feature-list">
                      <div className="feature-item available">
                        <span>Session Management</span>
                      </div>
                      <div className="feature-item available">
                        <span>Multi-IDE Support</span>
                      </div>
                      <div className="feature-item available">
                        <span>Persistent Storage</span>
                      </div>
                      <div className="feature-item available">
                        <span>Ingress Config</span>
                      </div>
                      <div className="feature-item available">
                        <span>Scaling</span>
                      </div>
                    </div>
                  </div>
                </GridItem>

                <GridItem md={4}>
                  <div className="feature-group">
                    <Title headingLevel="h4" size="md">
                      In Development
                    </Title>
                    <div className="feature-list">
                      <div className="feature-item development">
                        <span>Resource Quotas</span>
                      </div>
                      <div className="feature-item development">
                        <span>Session Templates</span>
                      </div>
                      <div className="feature-item development">
                        <span>Multi-tenancy</span>
                      </div>
                      <div className="feature-item development">
                        <span>Git Integration</span>
                      </div>
                      <div className="feature-item development">
                        <span>Registry Integration</span>
                      </div>
                    </div>
                  </div>
                </GridItem>

                <GridItem md={4}>
                  <div className="feature-group">
                    <Title headingLevel="h4" size="md">
                      Integrations
                    </Title>
                    <div className="feature-list">
                      <div className="feature-item available">
                        <span>Keycloak SSO</span>
                      </div>
                      <div className="feature-item available">
                        <span>Helm Charts</span>
                      </div>
                      <div className="feature-item planned">
                        <span>LDAP/AD</span>
                      </div>
                      <div className="feature-item planned">
                        <span>Prometheus</span>
                      </div>
                      <div className="feature-item planned">
                        <span>Grafana</span>
                      </div>
                    </div>
                  </div>
                </GridItem>
              </Grid>
            </CardBody>
          </Card>
        </GridItem>
      </Grid>
    </PageSection>
  );
}
