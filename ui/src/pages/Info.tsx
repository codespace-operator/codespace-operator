import React from "react";
import { PageSection, Card, CardBody, Title, List, ListItem } from "@patternfly/react-core";

export function InfoPage() {
  return (
    <PageSection isWidthLimited>
      <Card>
        <CardBody>
          <Title headingLevel="h2">About</Title>
          <p style={{ margin: "8px 0 16px" }}>
            This console manages developer sessions (JupyterLab / VS Code) via the
            Codespace Operator. The UI is styled to resemble the OpenShift/Red Hat ACS console.
          </p>

          <Title headingLevel="h3" style={{ marginTop: 16 }}>Roadmap</Title>
          <List>
            <ListItem>Authentication: local login stub → OAuth/OIDC (Keycloak, GitHub, etc.)</ListItem>
            <ListItem>Enterprise: LDAP directory lookup, SSO, and RBAC guardrails</ListItem>
            <ListItem>Namespace scoping, quotas, and admin dashboards</ListItem>
          </List>
        </CardBody>
      </Card>
    </PageSection>
  );
}
