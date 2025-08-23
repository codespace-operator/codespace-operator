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

type Props = {
  namespace: string;
  onNamespace: (ns: string) => void;
  onRefresh: () => void;
  onToggleSidebar: () => void;
};

export function Header({ namespace, onNamespace, onRefresh, onToggleSidebar }: Props) {
  return (
    <Masthead
      backgroundColor={{ default: "dark" }}
      display={{ default: "inline" }}
      style={{ boxShadow: "var(--pf-c-masthead--BoxShadow)" }}
    >
      <MastheadToggle>
        <Button variant="plain" aria-label="Global navigation" onClick={onToggleSidebar}>
          <BarsIcon />
        </Button>
      </MastheadToggle>

      <MastheadMain>
        <MastheadBrand>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Brand src={logoUrl} alt="Codespace Operator" style={{ width: 28, height: 28 }} />
            <Title headingLevel="h2" style={{ color: "var(--pf-global--Color--light-100)", margin: 0, fontWeight: 600 }}>
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
              <Button variant="secondary" onClick={onRefresh}>
                <SyncIcon /> Refresh
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );
}
