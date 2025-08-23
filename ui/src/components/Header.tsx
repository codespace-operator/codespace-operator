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
  Title,
  Button,
  Brand,
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
  FormSelect,
  FormSelectOption
} from "@patternfly/react-core";
import { BarsIcon, SyncIcon, UserIcon, CogIcon } from "@patternfly/react-icons";
import { useAuth } from "../hooks/useAuth";
import { useNavigate } from "react-router-dom";
import { useNamespaces } from "../hooks/useNamespaces";
import logoUrl from "../assets/codespace-operator.svg?url";

type Props = {
  namespace: string;
  onNamespace: (ns: string) => void;
  onRefresh: () => void;
  onToggleSidebar: () => void;
  user?: string | null;
};

// Helper to load namespaces list from env or fallback
function getNamespaceOptions(): string[] {
  const raw = import.meta.env.VITE_NAMESPACES as string | undefined;
  if (raw && raw.trim()) {
    const namespaces = raw.split(",").map(s => s.trim()).filter(Boolean);
    // Add "All" as the first option if not already present
    if (!namespaces.includes("All")) {
      return ["All", ...namespaces];
    }
    return namespaces;
  }
  // Default namespaces with "All" option
  return ["All", "default", "dev", "staging", "prod"];
}

export function Header({ namespace, onNamespace, onRefresh, onToggleSidebar, user }: Props) {
  const { logout } = useAuth();
  const navigate = useNavigate();
  const [isUserMenuOpen, setUserMenuOpen] = React.useState(false);
  const { sessionNamespaces, loading: nsLoading } = useNamespaces();
  const listOptions = ["All", ...sessionNamespaces];

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  const userMenuItems = [
    <DropdownItem key="user-info" icon={<UserIcon />} onClick={() => navigate("/user-info")}>
      User Management
    </DropdownItem>,
    <DropdownItem key="settings" icon={<CogIcon />} onClick={() => navigate("/info")}>
      Settings
    </DropdownItem>,
    <DropdownItem key="logout" onClick={handleLogout}>
      Sign out
    </DropdownItem>
  ];

  return (
    <Masthead
      backgroundColor={{ default: "dark" }}
      display={{ default: "inline" }}
      style={{
        boxShadow: "var(--pf-c-masthead--BoxShadow)",
        borderBottom: "3px solid var(--pf-global--primary-color--100)"
      }}
    >
      <MastheadToggle>
        <Button variant="plain" aria-label="Global navigation" onClick={onToggleSidebar}>
          <BarsIcon style={{ color: 'var(--pf-global--Color--light-100)' }} />
        </Button>
      </MastheadToggle>

      <MastheadMain>
        <MastheadBrand>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Brand src={logoUrl} alt="Codespace Operator" style={{ width: 32, height: 32 }} />
            <div>
              <Title headingLevel="h1" style={{
                color: "var(--pf-global--Color--light-100)",
                margin: 0,
                fontWeight: 600,
                fontSize: "1.25rem"
              }}>
                Codespace Operator
              </Title>
              <div style={{
                color: "var(--pf-global--Color--light-200)",
                fontSize: "0.75rem",
                marginTop: "-2px"
              }}>
                Developer Environments
              </div>
            </div>
          </div>
        </MastheadBrand>
      </MastheadMain>

      <MastheadContent>
        <Toolbar isFullHeight isStatic>
          <ToolbarContent>
            {/* Namespace selector */}
            <ToolbarItem>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{
                  color: "var(--pf-global--Color--light-200)",
                  fontSize: "0.875rem",
                  whiteSpace: "nowrap"
                }}>
                  Namespace:
                </span>
               <FormSelect
                value={namespace}
                onChange={(_, v) => onNamespace(v)}
                aria-label="Select namespace"
                style={{ minWidth: 200 }}
                isDisabled={nsLoading}
              />
                {listOptions.map(ns => (
                  <FormSelectOption key={ns} value={ns} label={ns === "All" ? "All Namespaces" : ns} />
                ))}
              </FormSelect>
              </div>
            </ToolbarItem>

            <ToolbarItem>
              <Button
                variant="secondary"
                onClick={onRefresh}
                size="sm"
                style={{
                  backgroundColor: "rgba(255,255,255,0.1)",
                  borderColor: "rgba(255,255,255,0.3)",
                  color: "var(--pf-global--Color--light-100)"
                }}
              >
                <SyncIcon style={{ marginRight: 4 }} />
                Refresh
              </Button>
            </ToolbarItem>

            {user && (
              <ToolbarItem>
                <Dropdown
                  isOpen={isUserMenuOpen}
                  onSelect={() => setUserMenuOpen(false)}
                  toggle={(toggleRef: React.Ref<any>) => (
                    <MenuToggle
                      ref={toggleRef}
                      onClick={() => setUserMenuOpen(!isUserMenuOpen)}
                      isExpanded={isUserMenuOpen}
                      style={{
                        color: "var(--pf-global--Color--light-100)",
                        backgroundColor: "transparent",
                        borderColor: "rgba(255,255,255,0.3)"
                      }}
                    >
                      <UserIcon style={{ marginRight: 6 }} />
                      {user}
                    </MenuToggle>
                  )}
                  shouldFocusToggleOnSelect
                >
                  <DropdownList>
                    {userMenuItems}
                  </DropdownList>
                </Dropdown>
              </ToolbarItem>
            )}
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );
}
