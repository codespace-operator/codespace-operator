import React, { useState, useEffect, useMemo } from "react";
import {
  Page,
  PageSection,
  PageSectionVariants,
  PageSidebar,
  Nav,
  NavList,
  NavItem,
  NavGroup,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  TextInput,
  Alert,
  AlertGroup,
  AlertActionCloseButton,
  Button,
  Card,
  CardBody,
} from "@patternfly/react-core";
import { PlusCircleIcon } from "@patternfly/react-icons";
import { Header } from "./components/Header";
import { SessionsTable } from "./components/SessionsTable";
import { CreateSessionModal } from "./components/CreateSessionModal";
import {
  useAlerts,
  useFilteredSessions,
  useSessions,
} from "./hooks/useSessions";
import type { Session } from "./types";
import { InfoPage } from "./pages/InfoPage";
import { LoginPage } from "./pages/LoginPage";
import { UserInfoPage } from "./pages/UserInfo";
import { useAuth } from "./hooks/useAuth";
import { useIntrospection } from "./hooks/useIntrospection";
import {
  Routes,
  Route,
  Navigate,
  useLocation,
  useNavigate,
  Link,
} from "react-router-dom";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();
  const location = useLocation();
  if (isLoading) return <div style={{ padding: 24 }}>Loading...</div>;
  if (!isAuthenticated)
    return <Navigate to="/login" state={{ from: location }} replace />;
  return <>{children}</>;
}

// Full-screen login layout without sidebars
function LoginLayout({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        minHeight: "100vh",
        backgroundColor:
          "var(--pf-v6-c-page--BackgroundColor, var(--pf-c-page--BackgroundColor))",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      {children}
    </div>
  );
}

export default function App() {
  const { isAuthenticated, user } = useAuth();
  const {
    data: ix,
    loading: ixLoading,
    error: ixError,
  } = useIntrospection({ discover: true });

  const alerts = useAlerts();
  const location = useLocation();
  const navigate = useNavigate();

  const [namespace, setNamespace] = useState<string>(
    localStorage.getItem("co_ns") || "All",
  );
  const [query, setQuery] = useState("");
  const [isNavOpen, setNavOpen] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);

  // Derive creatable namespaces from /introspect (single source of truth)
  const creatableNamespaces = useMemo(() => {
    if (!ix) return [];
    return Object.entries(ix.domains || {})
      .filter(([ns, perms]) => ns !== "*" && perms?.session?.create)
      .map(([ns]) => ns)
      .sort();
  }, [ix]);

  // Button gating: hide "All" if user can't watch across "*"
  const allowAll = !!ix?.domains?.["*"]?.session?.watch;

  // Persist namespace selection
  useEffect(() => {
    // If user picked "All" but it's not allowed, fall back to first allowed ns
    if (namespace === "All" && !allowAll) {
      const fallback =
        creatableNamespaces[0] || ix?.namespaces?.userAllowed?.[0] || "default";
      setNamespace(fallback);
      return;
    }
    localStorage.setItem("co_ns", namespace);
  }, [namespace, allowAll, creatableNamespaces, ix?.namespaces?.userAllowed]);

  // Sessions data (hook already guards actions; will error if forbidden)
  const { rows, loading, refresh, create, remove, scale, pendingTargets } =
    useSessions(namespace, (msg) => alerts.push(msg, "danger"));
  const filtered = useFilteredSessions(rows, query);

  useEffect(() => {
    if (ixError) alerts.push(ixError, "danger");
  }, [ixError]);

  const openURL = (s: Session) => {
    const url =
      s.status?.url ||
      (s.spec.networking?.host ? `https://${s.spec.networking.host}` : "");
    if (!url) return alerts.push("No URL yet", "info");
    window.open(url, "_blank");
  };

  const doDelete = async (s: Session) => {
    if (!confirm(`Delete ${s.metadata.name}?`)) return;
    try {
      await remove(s.metadata.namespace, s.metadata.name);
      alerts.push("Deleted", "success");
      refresh();
    } catch (e: any) {
      alerts.push(e?.message || "Delete failed", "danger");
    }
  };

  // Nav sections — render even if auth is pending; RequireAuth guards routes.
  const nav = (
    <Nav aria-label="Primary nav" theme="dark">
      <NavList>
        <NavGroup title="Workloads">
          <NavItem
            itemId="/sessions"
            isActive={location.pathname.startsWith("/sessions")}
            // Use PF's `component` to integrate with React Router cleanly
            component={(props) => <Link {...props} to="/sessions" />}
          >
            Sessions
          </NavItem>
        </NavGroup>

        <NavGroup title="Administration">
          <NavItem
            itemId="/user-info"
            isActive={location.pathname.startsWith("/user-info")}
            component={(props) => <Link {...props} to="/user-info" />}
          >
            User Management
          </NavItem>
          <NavItem
            itemId="/info"
            isActive={location.pathname.startsWith("/info")}
            component={(props) => <Link {...props} to="/info" />}
          >
            Cluster Settings
          </NavItem>
        </NavGroup>
      </NavList>
    </Nav>
  );

  const Sidebar = <PageSidebar isSidebarOpen={isNavOpen} nav={nav} />;

  // If on login page, show full-screen login layout
  if (location.pathname === "/login") {
    return (
      <LoginLayout>
        <AlertGroup isToast isLiveRegion>
          {alerts.list.map((a) => (
            <Alert
              key={a.key}
              title={a.title}
              variant={a.variant}
              timeout={6000}
              actionClose={
                <AlertActionCloseButton onClose={() => alerts.close(a.key)} />
              }
            />
          ))}
        </AlertGroup>

        <Routes>
          <Route
            path="/login"
            element={
              <LoginPage
                onLoggedIn={() => {
                  const from =
                    (location.state as any)?.from?.pathname || "/sessions";
                  navigate(from, { replace: true });
                }}
              />
            }
          />
        </Routes>
      </LoginLayout>
    );
  }

  return (
    <Page
      masthead={
        <Header
          namespace={namespace}
          onNamespace={(ns) => setNamespace(ns)}
          onRefresh={refresh}
          onToggleSidebar={() => setNavOpen((v) => !v)}
          user={user}
          // (Optional) if Header renders a namespace picker, you can pass options:
          // allowedNamespaces={[...(allowAll ? ["All"] : []), ...(ix?.namespaces?.userAllowed || [])]}
        />
      }
      sidebar={isAuthenticated ? Sidebar : undefined}
      isManagedSidebar
    >
      <AlertGroup isToast isLiveRegion>
        {alerts.list.map((a) => (
          <Alert
            key={a.key}
            title={a.title}
            variant={a.variant}
            timeout={6000}
            actionClose={
              <AlertActionCloseButton onClose={() => alerts.close(a.key)} />
            }
          />
        ))}
      </AlertGroup>

      <Routes>
        <Route
          path="/sessions"
          element={
            <RequireAuth>
              <>
                <PageSection variant={PageSectionVariants.light} isWidthLimited>
                  <div className="pf-u-display-flex pf-u-justify-content-space-between pf-u-align-items-center">
                    <div>
                      <h1 className="pf-c-title pf-m-2xl">Sessions</h1>
                      <p className="pf-u-color-200">
                        Manage your development environments and IDE sessions
                      </p>
                    </div>
                  </div>
                  <Toolbar>
                    <ToolbarContent>
                      <ToolbarItem>
                        <TextInput
                          aria-label="Search sessions"
                          value={query}
                          onChange={(_, v) => setQuery(v)}
                          placeholder="Filter by name, image, or host..."
                        />
                      </ToolbarItem>
                      <ToolbarItem>
                        <Button
                          icon={<PlusCircleIcon />}
                          variant="primary"
                          onClick={() => setCreateOpen(true)}
                          isDisabled={
                            creatableNamespaces.length === 0 || ixLoading
                          }
                          title={
                            creatableNamespaces.length === 0
                              ? "You lack 'create' permission in any namespace"
                              : undefined
                          }
                        >
                          Create Session
                        </Button>
                      </ToolbarItem>
                    </ToolbarContent>
                  </Toolbar>
                </PageSection>

                <PageSection isFilled isWidthLimited>
                  <Card>
                    <CardBody>
                      <SessionsTable
                        loading={loading}
                        rows={filtered}
                        pendingTargets={pendingTargets}
                        onScale={async (s, d) => {
                          const current =
                            typeof s.spec.replicas === "number"
                              ? s.spec.replicas
                              : 1;
                          const next = Math.max(0, current + d);
                          try {
                            await scale(
                              s.metadata.namespace,
                              s.metadata.name,
                              next,
                            );
                            alerts.push(`Scaled to ${next}`, "success");
                            refresh();
                          } catch (e: any) {
                            alerts.push(e?.message || "Scale failed", "danger");
                          }
                        }}
                        onDelete={async (s) => doDelete(s)}
                        onOpen={openURL}
                      />
                    </CardBody>
                  </Card>

                  <CreateSessionModal
                    isOpen={isCreateOpen}
                    namespace={namespace}
                    // pass creatable namespaces derived from introspection
                    writableNamespaces={creatableNamespaces}
                    onClose={() => setCreateOpen(false)}
                    onCreate={async (body) => {
                      try {
                        await create(body);
                        alerts.push(
                          `Session ${body?.metadata?.name || ""} created`,
                          "success",
                        );
                        setCreateOpen(false);
                        refresh();
                      } catch (e: any) {
                        alerts.push(e?.message || "Create failed", "danger");
                      }
                    }}
                  />
                </PageSection>
              </>
            </RequireAuth>
          }
        />

        <Route
          path="/user-info"
          element={
            <RequireAuth>
              <UserInfoPage />
            </RequireAuth>
          }
        />

        <Route
          path="/info"
          element={
            <RequireAuth>
              <InfoPage />
            </RequireAuth>
          }
        />

        <Route path="/" element={<Navigate to="/sessions" replace />} />
        <Route path="*" element={<Navigate to="/sessions" replace />} />
      </Routes>
    </Page>
  );
}
