import React, { useState, useEffect, useMemo } from "react";
import {
  PageSection,
  PageSectionVariants,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  TextInput,
  Button,
  Card,
  CardBody,
} from "@patternfly/react-core";
import { PlusCircleIcon } from "@patternfly/react-icons";
import { SessionsTable } from "../components/SessionsTable";
import { CreateSessionModal } from "../components/CreateSessionModal";
import {
  useAlerts,
  useFilteredSessions,
  useSessions,
} from "../hooks/useSessions";
import { useIntrospection } from "../hooks/useIntrospection";
import type { Session } from "../types";

type Props = {
  namespace: string;
  onAlert: (
    message: string,
    variant: "success" | "danger" | "warning" | "info",
  ) => void;
};

export function SessionsPage({ namespace, onAlert }: Props) {
  const { data: ix, loading: ixLoading } = useIntrospection({ discover: true });
  const [query, setQuery] = useState("");
  const [isCreateOpen, setCreateOpen] = useState(false);

  // Derive creatable namespaces from /introspect (single source of truth)
  const creatableNamespaces = useMemo(() => {
    if (!ix) return [];
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

  const openURL = (s: Session) => {
    const url =
      s.status?.url ||
      (s.spec.networking?.host ? `https://${s.spec.networking.host}` : "");
    if (!url) return onAlert("No URL yet", "info");
    window.open(url, "_blank");
  };

  const doDelete = async (s: Session) => {
    if (!confirm(`Delete ${s.metadata.name}?`)) return;
    try {
      await remove(s.metadata.namespace, s.metadata.name);
      onAlert("Deleted", "success");
      refresh();
    } catch (e: any) {
      onAlert(e?.message || "Delete failed", "danger");
    }
  };

  const handleScale = async (s: Session, delta: number) => {
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

  const handleCreate = async (body: Partial<Session>) => {
    try {
      await create(body);
      onAlert(`Session ${body?.metadata?.name || ""} created`, "success");
      setCreateOpen(false);
      refresh();
    } catch (e: any) {
      onAlert(e?.message || "Create failed", "danger");
    }
  };

  return (
    <>
      <PageSection isWidthLimited style={{ padding: "1rem" }}>
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
                isDisabled={creatableNamespaces.length === 0 || ixLoading}
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

      <PageSection
        isFilled
        isWidthLimited
        style={{ paddingLeft: "2rem", paddingRight: "2rem" }}
      >
        <Card>
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
}
