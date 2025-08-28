import { useState, useMemo, useImperativeHandle, forwardRef } from "react";
import {
  PageSection,
  TextInput,
  Button,
  Card,
  CardBody,
  Title,
} from "@patternfly/react-core";
import { PlusCircleIcon } from "@patternfly/react-icons";
import { SessionsTable } from "../components/SessionsTable";
import { CreateSessionModal } from "../components/CreateSessionModal";
import { useFilteredSessions, useSessions } from "../hooks/useSessions";
import type { components } from "../types/api.gen";

import { useIntrospection } from "../hooks/useIntrospection";
import type { UISession } from "../types";

type SessionCreateRequest =
  components["schemas"]["cmd_server.SessionCreateRequest"];

type Props = {
  namespace: string;
  onAlert: (
    message: string,
    variant: "success" | "danger" | "warning" | "info",
  ) => void;
};

export type SessionsPageRef = {
  refresh: () => void;
};

export const SessionsPage = forwardRef<SessionsPageRef, Props>(
  ({ namespace, onAlert }, ref) => {
    const { data: ix, loading: ixLoading } = useIntrospection({
      discover: true,
    });
    const [query, setQuery] = useState("");
    const [isCreateOpen, setCreateOpen] = useState(false);

    // Derive creatable namespaces from /introspect (single source of truth)
    const creatableNamespaces = useMemo(() => {
      if (!ix) return [];
      // 1) Use server-provided list if present
      const fromApi = ix.namespaces?.userCreatable;
      if (fromApi?.length) return [...fromApi].sort();

      // 2) If wildcard create is granted, allow all user-allowed namespaces
      const starCreate = !!ix.domains?.["*"]?.session?.create;
      if (starCreate) {
        const allowed = ix.namespaces?.userAllowed ?? [];
        return [...allowed].sort();
      }

      // 3) Fallback: scan per-namespace flags
      return Object.entries(ix.domains || {})
        .filter(([ns, perms]) => ns !== "*" && perms?.session?.create)
        .map(([ns]) => ns)
        .sort();
    }, [ix]);

    // Sessions data (hook already guards actions; will error if forbidden)
    const { rows, loading, refresh, create, remove, scale, pendingTargets } =
      useSessions(
        namespace,
        (msg) => onAlert(msg, "danger"), // Convert single string to our callback format
      );
    const filtered = useFilteredSessions(rows, query);

    // Expose refresh function to parent component
    useImperativeHandle(
      ref,
      () => ({
        refresh,
      }),
      [refresh],
    );

    const openURL = (s: UISession) => {
      const url =
        s.status?.url ||
        (s.spec.networking?.host ? `https://${s.spec.networking.host}` : "");
      if (!url) return onAlert("No URL yet", "info");
      window.open(url, "_blank");
    };

    const doDelete = async (s: UISession) => {
      if (!confirm(`Delete ${s.metadata.name}?`)) return;
      try {
        await remove(s.metadata.namespace, s.metadata.name);
        onAlert("Deleted", "success");
        refresh();
      } catch (e: any) {
        onAlert(e?.message || "Delete failed", "danger");
      }
    };

    const handleScale = async (s: UISession, delta: number) => {
      const current = typeof s.spec.replicas === "number" ? s.spec.replicas : 1;
      const next = Math.max(0, current + delta);
      try {
        await scale(s.metadata.namespace, s.metadata.name, next);
        onAlert(`Scaled to ${next}`, "success");
        refresh();
      } catch (e: any) {
        onAlert(e?.message || "Scale failed", "danger");
      }
    };

    const handleCreate = async (body: SessionCreateRequest) => {
      try {
        await create(body);
        onAlert(`Session ${body.name} created`, "success");
        setCreateOpen(false);
        refresh();
      } catch (e: any) {
        onAlert(e?.message || "Create failed", "danger");
      }
    };

    return (
      <>
        <PageSection className="sessions-header">
          <div className="sessions-header-content">
            <Title headingLevel="h1" size="2xl">
              Sessions
            </Title>
            <div className="sessions-actions">
              <TextInput
                aria-label="Search sessions"
                value={query}
                onChange={(_, v) => setQuery(v)}
                placeholder="Filter sessions..."
                className="sessions-search"
              />
              <Button
                icon={<PlusCircleIcon />}
                variant="primary"
                onClick={() => setCreateOpen(true)}
                isDisabled={creatableNamespaces.length === 0 || ixLoading}
                title={
                  creatableNamespaces.length === 0
                    ? "You lack 'create' permission in any namespace"
                    : undefined
                }
              >
                Create Session
              </Button>
            </div>
          </div>
        </PageSection>

        <PageSection isFilled className="sessions-content">
          <Card className="sessions-table-card">
            <CardBody>
              <SessionsTable
                loading={loading}
                rows={filtered}
                pendingTargets={pendingTargets}
                onScale={handleScale}
                onDelete={doDelete}
                onOpen={openURL}
              />
            </CardBody>
          </Card>

          <CreateSessionModal
            isOpen={isCreateOpen}
            namespace={namespace}
            writableNamespaces={creatableNamespaces}
            onClose={() => setCreateOpen(false)}
            onCreate={handleCreate}
          />
        </PageSection>
      </>
    );
  },
);
