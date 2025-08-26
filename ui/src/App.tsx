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
import { SessionsPage, SessionsPageRef } from "./pages/SessionsPage";
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
  Outlet,
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

/**
 * AppChrome: header + (auth-gated) sidebar + alert area + <Outlet />
 * This renders only for "app" routes, not for /login (which uses LoginLayout).
 */
function AppChrome({
  namespace,
  setNamespace,
  isSidebarOpen,
  setIsSidebarOpen,
  onRefresh,
  alerts,
}: {
  namespace: string;
  setNamespace: (ns: string) => void;
  isSidebarOpen: boolean;
  setIsSidebarOpen: (v: boolean) => void;
  onRefresh: () => void;
  alerts: ReturnType<typeof useAlerts>;
}) {
  const { isAuthenticated, user } = useAuth();
  const location = useLocation();

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100vh",
        backgroundColor: "var(--pf-c-page--BackgroundColor, #151515)",
      }}
    >
      <Header
        namespace={namespace}
        onNamespace={setNamespace}
        onRefresh={onRefresh}
        onToggleSidebar={() => setIsSidebarOpen(!isSidebarOpen)}
        user={user}
      />

      <div style={{ display: "flex", flex: 1, overflow: "hidden" }}>
        {/* Sidebar only when authenticated */}
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

        {/* Main content */}
        <div
          style={{
            flex: 1,
            overflow: "auto",
            backgroundColor: "var(--pf-c-page--BackgroundColor, #151515)",
            padding: 0,
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

          {/* Child route outlet */}
          <Outlet />
        </div>
      </div>
    </div>
  );
}

export default function App() {
  const { user } = useAuth();
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

  const handleRefresh = () => {
    if (location.pathname.startsWith("/sessions")) {
      sessionsPageRef.current?.refresh();
    }
  };

  // Helper component so we can keep Login fullscreen + alerts + navigate-after-login.
  const LoginRoute = () => {
    const loc = useLocation();
    const nav = useNavigate();
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

        <LoginPage
          onLoggedIn={() => {
            const from = (loc.state as any)?.from?.pathname || "/sessions";
            nav(from, { replace: true });
          }}
        />
      </LoginLayout>
    );
  };

  return (
    <Routes>
      {/* Public, full-screen login */}
      <Route path="/login" element={<LoginRoute />} />

      {/* App chrome wraps protected routes */}
      <Route
        path="/"
        element={
          <AppChrome
            namespace={namespace}
            setNamespace={setNamespace}
            isSidebarOpen={isSidebarOpen}
            setIsSidebarOpen={setIsSidebarOpen}
            onRefresh={handleRefresh}
            alerts={alerts}
          />
        }
      >
        <Route
          path="sessions"
          element={
            <RequireAuth>
              <SessionsPage
                ref={sessionsPageRef}
                namespace={namespace}
                onAlert={(message, variant) => alerts.push(message, variant)}
              />
            </RequireAuth>
          }
        />
        <Route
          path="user-info"
          element={
            <RequireAuth>
              <UserInfoPage />
            </RequireAuth>
          }
        />
        <Route
          path="info"
          element={
            <RequireAuth>
              <InfoPage />
            </RequireAuth>
          }
        />

        {/* Default “home” → sessions */}
        <Route index element={<Navigate to="/sessions" replace />} />
      </Route>

      {/* Fallbacks */}
      <Route path="*" element={<Navigate to="/sessions" replace />} />
    </Routes>
  );
}
