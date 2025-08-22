import React from "react";
import {
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadMain,
  MastheadToggle,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  TextInput,
  Title,
  Button,
  Brand,
} from "@patternfly/react-core";
import { BarsIcon, SyncIcon } from "@patternfly/react-icons";
import logoUrl from "../assets/codespace-operator.svg?url";

// vite-friendly import of static asset

type Props = {
  namespace: string;
  onNamespace: (ns: string) => void;
  onRefresh: () => void;
  onToggleSidebar: () => void; // NEW
};

export function Header({ namespace, onNamespace, onRefresh, onToggleSidebar }: Props) {
  return (
    <Masthead backgroundColor="dark" display="inline">
      <MastheadToggle>
        <Button variant="plain" aria-label="Global navigation" onClick={onToggleSidebar}>
          <BarsIcon />
        </Button>
      </MastheadToggle>

      <MastheadMain>
        <MastheadBrand>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Brand src={logoUrl} alt="Codespace Operator" style={{ width: 28, height: 28 }} />
            <Title headingLevel="h2" style={{ color: "white", margin: 0 }}>
              Codespace Operator
            </Title>
          </div>
        </MastheadBrand>
      </MastheadMain>

      <MastheadContent>
        <Toolbar isFullHeight isStatic>
          <ToolbarContent>
            <ToolbarItem>
              <TextInput
                aria-label="Namespace"
                value={namespace}
                onChange={(_, v) => onNamespace(v)}
                placeholder="namespace"
              />
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
