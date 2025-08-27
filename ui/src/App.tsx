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
} from "./hooks/useIntrospection";
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
    discover: true,
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

  // Smart namespace management with RBAC awareness
  const [namespace, setNamespace] = useState(() => {
    // Try to get from localStorage, but don't rely on it completely
    return localStorage.getItem("co_ns") || "default";
  });

  const [isSidebarOpen, setIsSidebarOpen] = useState(true);

  // Button gating: hide "All" if user can't watch across "*"
  const allowAll = useMemo(() => {
    return !!userIx?.domains?.["*"]?.session?.watch;
  }, [userIx]);

  // Derive user's accessible namespaces
  const userAccessibleNamespaces = useMemo(() => {
    if (!userIx?.namespaces?.userAllowed) return [""];
    return userIx.namespaces.userAllowed;
  }, [userIx]);

  // Derive creatable namespaces for fallback logic
  const creatableNamespaces = useMemo(() => {
    if (!userIx?.namespaces?.userCreatable) return [];
    return userIx.namespaces.userCreatable.sort();
  }, [userIx]);

  // Smart namespace validation and fallback
  useEffect(() => {
    if (!userIx) return; // Wait for user data

    const isCurrentNamespaceValid =
      namespace === "All"
        ? allowAll
        : userAccessibleNamespaces.includes(namespace);

    if (!isCurrentNamespaceValid) {
      // Current namespace is not accessible, find a fallback
      let fallback: string = "";

      if (allowAll) {
        fallback = "All";
      } else if (userAccessibleNamespaces.length > 0) {
        // Prefer namespaces where user can create sessions
        const preferredNamespace = creatableNamespaces.find((ns) =>
          userAccessibleNamespaces.includes(ns),
        );
        fallback = preferredNamespace || userAccessibleNamespaces[0];
      }

      if (fallback !== namespace) {
        console.log(
          `Switching namespace from ${namespace} to ${fallback} due to RBAC restrictions`,
        );
        setNamespace(fallback);
      }
    }
  }, [
    namespace,
    allowAll,
    userAccessibleNamespaces,
    creatableNamespaces,
    userIx,
  ]);

  // Persist namespace selection (but validate it on load)
  useEffect(() => {
    if (userIx) {
      // Only persist after we have user data
      localStorage.setItem("co_ns", namespace);
    }
  }, [namespace, userIx]);

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
