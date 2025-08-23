import React, { useState, useEffect } from "react";
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
} from "@patternfly/react-core";
import { UserIcon, KeyIcon, ShieldAltIcon, ExclamationTriangleIcon } from "@patternfly/react-icons";
import { useAuth } from "../hooks/useAuth";

// Mock RBAC data - in a real implementation, this would come from your API
const mockRBACData = {
  roles: [
    {
      name: "codespace-operator:admin",
      namespace: "*",
      permissions: ["sessions.create", "sessions.delete", "sessions.update", "sessions.list", "sessions.watch"]
    },
    {
      name: "codespace-user",
      namespace: "default",
      permissions: ["sessions.create", "sessions.list", "sessions.watch"]
    }
  ],
  clusterRoles: [
    {
      name: "system:authenticated",
      permissions: ["system.info"]
    }
  ]
};

export function UserInfoPage() {
  const { user, token, logout } = useAuth();
  const [loading, setLoading] = useState(true);
  const [rbacData, setRBACData] = useState<typeof mockRBACData | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Simulate loading RBAC data
    const loadRBACData = async () => {
      setLoading(true);
      try {
        // In a real implementation, you would fetch this from your backend:
        // const response = await api.getUserRBAC();
        // setRBACData(response);
        
        // For now, simulate API call
        await new Promise(resolve => setTimeout(resolve, 800));
        setRBACData(mockRBACData);
      } catch (err: any) {
        setError(err.message || "Failed to load user permissions");
      } finally {
        setLoading(false);
      }
    };

    loadRBACData();
  }, []);

  const decodeJWTPayload = (token: string) => {
    try {
      const parts = token.split(".");
      if (parts.length < 2) return null;
      const payload = JSON.parse(atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")));
      return payload;
    } catch {
      return null;
    }
  };

  const tokenPayload = token ? decodeJWTPayload(token) : null;
  const isExpired = tokenPayload?.exp ? Date.now() / 1000 >= tokenPayload.exp : false;

  return (
    <PageSection isWidthLimited>
      <div className="pf-u-mb-lg">
        <Title headingLevel="h1" className="pf-u-mb-sm">User Management</Title>
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
                        {tokenPayload.iat ? 
                          new Date(tokenPayload.iat * 1000).toLocaleString() : 
                          "Unknown"
                        }
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    
                    <DescriptionListGroup>
                      <DescriptionListTerm>Expires</DescriptionListTerm>
                      <DescriptionListDescription>
                        {tokenPayload.exp ? 
                          new Date(tokenPayload.exp * 1000).toLocaleString() : 
                          "Never"
                        }
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  </>
                )}
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
                  {token ? `${token.substring(0, 50)}...` : "No token available"}
                </CodeBlockCode>
              </CodeBlock>
              
              <p className="pf-u-mt-sm pf-u-color-200 pf-u-font-size-sm">
                JWT tokens contain your identity information and are used to authenticate API requests.
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
                    <ExclamationTriangleIcon className="pf-u-mb-md" style={{ fontSize: '2rem', color: 'var(--pf-global--warning-color--100)' }} />
                    <div>{error}</div>
                  </EmptyStateBody>
                </EmptyState>
              ) : rbacData ? (
                <>
                  <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                    Namespace Roles
                  </Title>
                  {rbacData.roles.length > 0 ? (
                    <div className="pf-u-mb-lg">
                      {rbacData.roles.map((role, index) => (
                        <div key={index} className="pf-u-mb-md pf-u-p-md" 
                             style={{ backgroundColor: "rgba(0,0,0,0.05)", borderRadius: "4px" }}>
                          <div className="pf-u-display-flex pf-u-justify-content-space-between pf-u-align-items-center pf-u-mb-sm">
                            <strong>{role.name}</strong>
                            <Label color="blue" variant="filled">
                              {role.namespace === "*" ? "All Namespaces" : role.namespace}
                            </Label>
                          </div>
                          <List isPlain>
                            {role.permissions.map((permission, pIndex) => (
                              <ListItem key={pIndex} className="pf-u-font-size-sm pf-u-color-200">
                                {permission}
                              </ListItem>
                            ))}
                          </List>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="pf-u-color-200 pf-u-mb-lg">No namespace roles assigned.</p>
                  )}

                  <Title headingLevel="h3" size="md" className="pf-u-mb-sm">
                    Cluster Roles
                  </Title>
                  {rbacData.clusterRoles.length > 0 ? (
                    <div>
                      {rbacData.clusterRoles.map((role, index) => (
                        <div key={index} className="pf-u-mb-md pf-u-p-md" 
                             style={{ backgroundColor: "rgba(0,0,0,0.05)", borderRadius: "4px" }}>
                          <div className="pf-u-display-flex pf-u-justify-content-space-between pf-u-align-items-center pf-u-mb-sm">
                            <strong>{role.name}</strong>
                            <Label color="purple" variant="filled">Cluster-wide</Label>
                          </div>
                          <List isPlain>
                            {role.permissions.map((permission, pIndex) => (
                              <ListItem key={pIndex} className="pf-u-font-size-sm pf-u-color-200">
                                {permission}
                              </ListItem>
                            ))}
                          </List>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="pf-u-color-200">No cluster roles assigned.</p>
                  )}
                </>
              ) : null}
            </CardBody>
          </Card>
        </GridItem>
      </Grid>

      <Card className="pf-u-mt-lg">
        <CardBody>
          <Title headingLevel="h2" className="pf-u-mb-md">Integration Information</Title>
          
          <Alert
            variant="info"
            isInline
            title="Development Environment"
            className="pf-u-mb-md"
          >
            This is currently running in development mode with mock authentication. 
            In production, this would integrate with your organization's identity provider 
            (LDAP, Active Directory, OIDC, etc.) and show real RBAC data from Kubernetes.
          </Alert>

          <DescriptionList isHorizontal>
            <DescriptionListGroup>
              <DescriptionListTerm>Authentication Method</DescriptionListTerm>
              <DescriptionListDescription>Demo JWT (Development)</DescriptionListDescription>
            </DescriptionListGroup>
            <DescriptionListGroup>
              <DescriptionListTerm>Future Integrations</DescriptionListTerm>
              <DescriptionListDescription>
                <List isPlain>
                  <ListItem>• OIDC (Keycloak, Auth0, etc.)</ListItem>
                  <ListItem>• LDAP/Active Directory</ListItem>
                  <ListItem>• Kubernetes ServiceAccount tokens</ListItem>
                  <ListItem>• OAuth2 providers (GitHub, Google, etc.)</ListItem>
                </List>
              </DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        </CardBody>
      </Card>
    </PageSection>
  );
}