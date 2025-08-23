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
import { InfoPage } from "./pages/Info";
import { LoginPage } from "./pages/Login";
import { useAuth } from "./hooks/useAuth";
import { Routes, Route, Navigate, useLocation, useNavigate, Link } from "react-router-dom";

type RouteKey = "sessions" | "info" | "login";

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth();
  const location = useLocation();
  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <>{children}</>;
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

  const Sidebar = (
    <PageSidebar
      isSidebarOpen={isNavOpen}
      sidebarContent={
        <Nav aria-label="Primary nav" theme="dark">
          <NavList>
            {isAuthenticated && (
              <>
                <NavItem isActive={location.pathname.startsWith("/sessions")}>
                  <Link className="pf-c-nav__link" to="/sessions">Sessions</Link>
                </NavItem>
                <NavItem isActive={location.pathname.startsWith("/info")}>
                  <Link className="pf-c-nav__link" to="/info">Info</Link>
                </NavItem>
              </>
            )}
            <NavItem isActive={location.pathname.startsWith("/login")}>
              <Link className="pf-c-nav__link" to="/login">
                {isAuthenticated ? `Account (${user})` : "Login"}
              </Link>
            </NavItem>
          </NavList>
        </Nav>
      }
    />
  );

  return (
    <Page
      masthead={
        <Header
          namespace={namespace}
          onNamespace={setNamespace}
          onRefresh={refresh}
          onToggleSidebar={() => setNavOpen((v) => !v)}
        />
      }
      sidebar={Sidebar}
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
                  <Toolbar>
                    <ToolbarContent>
                      <ToolbarItem>
                        <TextInput
                          aria-label="Search"
                          value={query}
                          onChange={(_, v) => setQuery(v)}
                          placeholder="Search by name / image / host"
                        />
                      </ToolbarItem>
                      <ToolbarItem>
                        <Button
                          icon={<PlusCircleIcon />}
                          variant="primary"
                          onClick={() => setCreateOpen(true)}
                        >
                          New Session
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
          path="/info"
          element={
            <RequireAuth>
              <InfoPage />
            </RequireAuth>
          }
        />

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

        <Route path="/" element={<Navigate to="/sessions" replace />} />
        <Route path="*" element={<Navigate to="/sessions" replace />} />
      </Routes>
    </Page>
  );
}
