import React, { useState, useEffect, useMemo, useRef } from "react";
import {
  Nav,
  NavList,
  NavItem,
  NavGroup,
  Alert,
  AlertGroup,
  AlertActionCloseButton,
} from "@patternfly/react-core";
import { Header } from "./components/Header";
import { InfoPage } from "./pages/InfoPage";
import { LoginPage } from "./pages/LoginPage";
import { UserInfoPage } from "./pages/UserInfo";
import { SessionsPage } from "./pages/SessionsPage";
import { useAlerts } from "./hooks/useAlerts";
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
  const { data: ix, error: ixError } = useIntrospection({ discover: true });

  const alerts = useAlerts();
  const location = useLocation();
  const navigate = useNavigate();
  const sessionsPageRef = useRef<SessionsPageRef>(null);

  const [namespace, setNamespace] = useState<string>(
    localStorage.getItem("co_ns") || "All",
  );
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);

  // Button gating: hide "All" if user can't watch across "*"
  const allowAll = !!ix?.domains?.["*"]?.session?.watch;

  // Derive creatable namespaces for fallback logic
  const creatableNamespaces = useMemo(() => {
    if (!ix) return [];
    return Object.entries(ix.domains || {})
      .filter(([ns, perms]) => ns !== "*" && perms?.session?.create)
      .map(([ns]) => ns)
      .sort();
  }, [ix]);

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

  useEffect(() => {
    if (ixError) alerts.push(ixError, "danger");
  }, [ixError]);

  const handleToggleSidebar = () => {
    console.log("Hamburger clicked! Current sidebar state:", isSidebarOpen);
    setIsSidebarOpen(!isSidebarOpen);
  };

  const handleRefresh = () => {
    // Only refresh if we're on the sessions page
    if (location.pathname.startsWith("/sessions")) {
      sessionsPageRef.current?.refresh();
    }
  };

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

  // Custom layout instead of relying on PatternFly's Page sidebar management
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100vh",
        backgroundColor: "var(--pf-c-page--BackgroundColor, #151515)",
      }}
    >
      {/* Header - remove the wrapper div and let Masthead handle its own layout */}
      <Header
        namespace={namespace}
        onNamespace={(ns) => setNamespace(ns)}
        onRefresh={handleRefresh}
        onToggleSidebar={handleToggleSidebar}
        user={user}
      />

      {/* Main layout with sidebar and content */}
      <div
        style={{
          display: "flex",
          flex: 1,
          overflow: "hidden",
        }}
      >
        {/* Sidebar */}
        {isAuthenticated && (
          <div
            style={{
              width: isSidebarOpen ? "300px" : "0px",
              backgroundColor: "#0f0f0f",
              transition: "width 0.3s ease-in-out",
              overflow: "hidden",
              borderRight: isSidebarOpen
                ? "1px solid rgba(255,255,255,0.1)"
                : "none",
              flexShrink: 0,
            }}
          >
            <Nav
              aria-label="Primary navigation"
              theme="dark"
              style={{ padding: "1rem 0", width: "300px" }}
            >
              <NavList>
                <NavGroup title="Workloads">
                  <NavItem
                    itemId="/sessions"
                    isActive={location.pathname.startsWith("/sessions")}
                  >
                    <Link
                      to="/sessions"
                      style={{
                        display: "block",
                        color: "inherit",
                        textDecoration: "none",
                        padding: "0.5rem 1rem",
                      }}
                    >
                      Sessions
                    </Link>
                  </NavItem>
                </NavGroup>

                <NavGroup title="Administration">
                  <NavItem
                    itemId="/user-info"
                    isActive={location.pathname.startsWith("/user-info")}
                  >
                    <Link
                      to="/user-info"
                      style={{
                        display: "block",
                        color: "inherit",
                        textDecoration: "none",
                        padding: "0.5rem 1rem",
                      }}
                    >
                      User Management
                    </Link>
                  </NavItem>
                  <NavItem
                    itemId="/info"
                    isActive={location.pathname.startsWith("/info")}
                  >
                    <Link
                      to="/info"
                      style={{
                        display: "block",
                        color: "inherit",
                        textDecoration: "none",
                        padding: "0.5rem 1rem",
                      }}
                    >
                      Cluster Settings
                    </Link>
                  </NavItem>
                </NavGroup>
              </NavList>
            </Nav>
          </div>
        )}

        {/* Main content area */}
        <div
          style={{
            flex: 1,
            overflow: "auto",
            backgroundColor: "var(--pf-c-page--BackgroundColor, #151515)",
            padding: "0",
          }}
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
                  <SessionsPage
                    ref={sessionsPageRef}
                    namespace={namespace}
                    onAlert={(message, variant) =>
                      alerts.push(message, variant)
                    }
                  />
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
        </div>
      </div>
    </div>
  );
}
