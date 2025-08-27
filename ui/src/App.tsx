// Updated ui/src/App.tsx - Key changes for split introspection

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
import {
  useUserIntrospection,
  useServerIntrospection,
} from "./hooks/useIntrospection"; // Updated import
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
  if (isLoading) return <div>Loading...</div>;
  if (!isAuthenticated)
    return <Navigate to="/login" state={{ from: location }} replace />;
  return <>{children}</>;
}

// Full-screen login layout without sidebars
function LoginLayout({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ height: "100vh", display: "flex" }}>
      <div style={{ flex: 1 }}>{children}</div>
    </div>
  );
}

/**
 * AppChrome: header + (auth-gated) sidebar + alert area +
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
    <div style={{ height: "100vh", display: "flex", flexDirection: "column" }}>
      <Header
        namespace={namespace}
        onNamespace={setNamespace}
        onRefresh={onRefresh}
        onToggleSidebar={() => setIsSidebarOpen(!isSidebarOpen)}
        user={user}
      />

      <div style={{ flex: 1, display: "flex" }}>
        {/* Sidebar only when authenticated */}
        {isAuthenticated && (
          <div
            style={{ width: isSidebarOpen ? "250px" : "0", overflow: "hidden" }}
          >
            <Nav>
              <NavList>
                <NavGroup title="Sessions">
                  <NavItem>
                    <Link to="/sessions">Sessions</Link>
                  </NavItem>
                </NavGroup>

                <NavGroup title="Management">
                  <NavItem>
                    <Link to="/user-info">User Management</Link>
                  </NavItem>
                  <NavItem>
                    <Link to="/info">Cluster Settings</Link>
                  </NavItem>
                </NavGroup>
              </NavList>
            </Nav>
          </div>
        )}

        {/* Main content */}
        <div style={{ flex: 1, overflow: "auto" }}>
          <AlertGroup isToast>
            {alerts.list.map((a) => (
              <Alert
                key={a.key}
                variant={a.variant}
                title={a.title}
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
  const { isAuthenticated, isLoading } = useAuth();

  const ixEnabled = isAuthenticated && !isLoading;

  // Use the new split introspection hooks
  const { data: userIx, error: userIxError } = useUserIntrospection({
    enabled: ixEnabled,
  });

  const { data: serverIx, error: serverIxError } = useServerIntrospection({
    discover: true,
    enabled: ixEnabled,
  });

  const alerts = useAlerts();

  useEffect(() => {
    // Toast errors once we *expect* introspection to work
    if (userIxError && ixEnabled) alerts.push(userIxError, "danger");
    if (serverIxError && ixEnabled) {
      // Don't show permission errors as critical alerts
      if (!serverIxError.includes("Insufficient permissions")) {
        alerts.push(serverIxError, "danger");
      }
    }
  }, [userIxError, serverIxError, ixEnabled]);

  const location = useLocation();
  const navigate = useNavigate();
  const sessionsPageRef = useRef<SessionsPageRef>(null);

  const [namespace, setNamespace] = useState(
    localStorage.getItem("co_ns") || "All",
  );
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);

  // Button gating: hide "All" if user can't watch across "*"
  const allowAll = !!userIx?.domains?.["*"]?.session?.watch;

  // Derive creatable namespaces for fallback logic
  const creatableNamespaces = useMemo(() => {
    if (!userIx) return [];
    return Object.entries(userIx.domains || {})
      .filter(([ns, perms]) => ns !== "*" && perms?.session?.create)
      .map(([ns]) => ns)
      .sort();
  }, [userIx]);

  // Persist namespace selection
  useEffect(() => {
    if (namespace === "All" && !allowAll) {
      const fallback =
        creatableNamespaces[0] ||
        userIx?.namespaces?.userAllowed?.[0] ||
        "default";
      setNamespace(fallback);
      return;
    }
    localStorage.setItem("co_ns", namespace);
  }, [
    namespace,
    allowAll,
    creatableNamespaces,
    userIx?.namespaces?.userAllowed,
  ]);

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
        <AlertGroup isToast>
          {alerts.list.map((a) => (
            <Alert
              key={a.key}
              variant={a.variant}
              title={a.title}
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
          <RequireAuth>
            <AppChrome
              namespace={namespace}
              setNamespace={setNamespace}
              isSidebarOpen={isSidebarOpen}
              setIsSidebarOpen={setIsSidebarOpen}
              onRefresh={handleRefresh}
              alerts={alerts}
            />
          </RequireAuth>
        }
      >
        <Route
          path="sessions"
          element={
            <SessionsPage
              ref={sessionsPageRef}
              namespace={namespace}
              onAlert={(message, variant) => alerts.push(message, variant)}
            />
          }
        />
        <Route path="user-info" element={<UserInfoPage />} />
        <Route path="info" element={<InfoPage />} />

        {/* Default "home" → sessions */}
        <Route index element={<Navigate to="/sessions" replace />} />
      </Route>

      {/* Fallbacks */}
      <Route path="*" element={<Navigate to="/sessions" replace />} />
    </Routes>
  );
}
