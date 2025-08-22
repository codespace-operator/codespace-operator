import React from "react";
import {
  Masthead, MastheadMain, MastheadBrand, MastheadContent,
  Toolbar, ToolbarContent, ToolbarItem, TextInput, Title, Button
} from "@patternfly/react-core";
import { SyncIcon } from "@patternfly/react-icons";

type Props = { namespace: string; onNamespace: (ns: string) => void; onRefresh: () => void; };

export function Header({ namespace, onNamespace, onRefresh }: Props) {
  return (
    <Masthead backgroundColor="dark" display="inline">
      <MastheadMain>
        <MastheadBrand>
          <Title headingLevel="h2" style={{ marginLeft: 12, color: "white" }}>
            Codespace Operator
          </Title>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar isFullHeight isStatic>
          <ToolbarContent>
            <ToolbarItem>
              <TextInput aria-label="Namespace" value={namespace} onChange={(_, v) => onNamespace(v)} placeholder="namespace" />
            </ToolbarItem>
            <ToolbarItem>
              <Button variant="secondary" icon={<SyncIcon />} onClick={onRefresh}>
                Refresh
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );
}
