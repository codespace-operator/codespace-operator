import React, { useState, useEffect } from "react";
import {
  Page,
  PageSection,
  PageSectionVariants,
  PageSidebar,
  Nav,
  NavList,
  NavItem,
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
import { useAlerts, useFilteredSessions, useSessions } from "./hooks/useSessions";
import type { Session } from "./types";
import { InfoPage } from "./pages/InfoPage";
import { LoginPage } from "./pages/LoginPage";
import { UserInfoPage } from "./pages/UserInfo";
import { useAuth } from "./hooks/useAuth";
import { Routes, Route, Navigate, useLocation, useNavigate, Link } from "react-router-dom";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth();
  const location = useLocation();
  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <>{children}</>;
}

// Full-screen login layout without sidebars
function LoginLayout({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ 
      minHeight: '100vh', 
      backgroundColor: 'var(--pf-v6-c-page--BackgroundColor, var(--pf-c-page--BackgroundColor))',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center'
    }}>
      {children}
    </div>
  );
}

export default function App() {
  const { isAuthenticated, user } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();

  const [namespace, setNamespace] = useState<string>(
    localStorage.getItem("co_ns") || "default"
  );
  const [query, setQuery] = useState("");
  const [isNavOpen, setNavOpen] = useState(true);
  const [isCreateOpen, setCreateOpen] = useState(false);

  const alerts = useAlerts();
  const { rows, loading, refresh, create, remove, scale } = useSessions(
    namespace,
    (msg) => alerts.push(msg, "danger")
  );
  const filtered = useFilteredSessions(rows, query);

  useEffect(() => localStorage.setItem("co_ns", namespace), [namespace]);

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
    } catch (e: any) {
      alerts.push(e?.message || "Delete failed", "danger");
    }
  };

  // Navigation items - only show when authenticated
  const getNavItems = () => {
    if (!isAuthenticated) return [];
    
    return [
      {
        id: "workloads",
        title: "Workloads",
        children: [
          { id: "sessions", title: "Sessions", to: "/sessions" }
        ]
      },
      {
        id: "administration", 
        title: "Administration",
        children: [
          { id: "user-info", title: "User Management", to: "/user-info" },
          { id: "cluster-info", title: "Cluster Settings", to: "/info" }
        ]
      }
    ];
  };

  const Sidebar = (
    <PageSidebar
      isSidebarOpen={isNavOpen}
      sidebarContent={
        <Nav aria-label="Primary nav" theme="dark">
          <NavList>
            {getNavItems().map((section) => (
              <React.Fragment key={section.id}>
                <NavItem className="pf-c-nav__section-title">
                  {section.title}
                </NavItem>
                {section.children.map((item) => (
                  <NavItem 
                    key={item.id} 
                    isActive={location.pathname.startsWith(item.to)}
                    className="pf-m-indent"
                  >
                    <Link className="pf-c-nav__link" to={item.to}>
                      {item.title}
                    </Link>
                  </NavItem>
                ))}
              </React.Fragment>
            ))}
          </NavList>
        </Nav>
      }
    />
  );

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
              actionClose={<AlertActionCloseButton onClose={() => alerts.close(a.key)} />}
            />
          ))}
        </AlertGroup>
        
        <Routes>
          <Route
            path="/login"
            element={
              <LoginPage
                onLoggedIn={() => {
                  const from = (location.state as any)?.from?.pathname || "/sessions";
                  navigate(from, { replace: true });
                }}
              />
            }
          />
        </Routes>
      </LoginLayout>
    );
  }

  // Main application layout with sidebar
  return (
    <Page
      masthead={
        <Header
          namespace={namespace}
          onNamespace={setNamespace}
          onRefresh={refresh}
          onToggleSidebar={() => setNavOpen((v) => !v)}
          user={user}
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
            actionClose={<AlertActionCloseButton onClose={() => alerts.close(a.key)} />}
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
                        onScale={async (s, d) => {
                          const current =
                            typeof s.spec.replicas === "number" ? s.spec.replicas : 1;
                          const next = Math.max(0, current + d);
                          try {
                            await scale(s.metadata.namespace, s.metadata.name, next);
                            alerts.push(`Scaled to ${next}`, "success");
                          } catch (e: any) {
                            alerts.push(e?.message || "Scale failed", "danger");
                          }
                        }}
                        onDelete={doDelete}
                        onOpen={openURL}
                      />
                    </CardBody>
                  </Card>

                  <CreateSessionModal
                    isOpen={isCreateOpen}
                    namespace={namespace}
                    onClose={() => setCreateOpen(false)}
                    onCreate={async (body) => {
                      try {
                        await create(body);
                        alerts.push(`Session ${body?.metadata?.name} created`, "success");
                        setCreateOpen(false);
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