import { Table, Thead, Tbody, Tr, Th, Td } from "@patternfly/react-table";
import { Button, Label, Tooltip, Spinner } from "@patternfly/react-core";
import {
  ExternalLinkAltIcon,
  TrashIcon,
  ArrowUpIcon,
  ArrowDownIcon,
} from "@patternfly/react-icons";
import type { UISession } from "../types";

function PhaseLabel({ phase }: { phase?: string }) {
  const intent: Record<
    string,
    "success" | "info" | "warning" | "danger" | "none"
  > = {
    Ready: "success",
    Pending: "warning",
    Error: "danger",
    none: "info",
  } as any;
  const pfColor = intent[phase || "none"] || "info";
  return <Label color={pfColor as any}>{phase || "-"}</Label>;
}
function getManagerInfo(s: UISession) {
  const L = s.metadata.labels || {};
  const A = s.metadata.annotations || {};

  // prefer explicit codespace-operator labels; fall back to the k8s conventional one
  const managerName = L["codespace.dev/manager-name"] || "";
  const managerNamespace = L["codespace.dev/manager-namespace"] || "";
  const managerType = L["codespace.dev/manager-type"] || "";
  const instanceId = L["codespace.dev/instance-id"] || "";

  const adoptedFrom = A["codespace.dev/adopted-from"] || "";
  const adoptedAt = A["codespace.dev/adopted-at"] || "";
  const adoptedBy = A["codespace.dev/adopted-by"] || "";

  return {
    managerName,
    managerNamespace,
    managerType,
    instanceId,
    adoptedFrom,
    adoptedAt,
    adoptedBy,
  };
}

type Props = {
  loading: boolean;
  rows: UISession[];
  pendingTargets?: Record<string, number>;
  onScale: (s: UISession, delta: number) => void;
  onDelete: (s: UISession) => void;
  onOpen: (s: UISession) => void;
};

export function SessionsTable({
  loading,
  rows,
  pendingTargets = {},
  onScale,
  onDelete,
  onOpen,
}: Props) {
  return (
    <div style={{ borderRadius: 6, overflow: "hidden" }}>
      <Table aria-label="Sessions" variant="compact" borders>
        <Thead>
          <Tr>
            <Th>Name</Th>
            <Th>Namespace</Th>
            <Th>IDE</Th>
            <Th>Image</Th>
            <Th>Host</Th>
            <Th>Managed By</Th>
            <Th>Phase</Th>
            <Th textCenter>Replicas</Th>
            <Th modifier="fitContent">Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {loading ? (
            <Tr>
              <Td colSpan={9}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <Spinner size="md" /> Loading...
                </div>
              </Td>
            </Tr>
          ) : rows.length === 0 ? (
            <Tr>
              <Td colSpan={9}>No sessions</Td>
            </Tr>
          ) : (
            rows.map((s) => (
              <Tr key={`${s.metadata.namespace}/${s.metadata.name}`}>
                <Td dataLabel="Name">{s.metadata.name}</Td>
                <Td dataLabel="Namespace">{s.metadata.namespace}</Td>
                <Td dataLabel="IDE">{s.spec.profile.ide}</Td>
                <Td dataLabel="Image" modifier="truncate">
                  {s.spec.profile.image}
                </Td>
                <Td dataLabel="Host" modifier="truncate">
                  {s.spec.networking?.host || ""}
                </Td>
                <Td dataLabel="Managed by">
                  {(() => {
                    const m = getManagerInfo(s);
                    if (!m.managerName)
                      return <span className="pf-u-color-200">—</span>;
                    const labelText = m.managerNamespace
                      ? `${m.managerNamespace}/${m.managerName}`
                      : m.managerName;
                    return (
                      <div
                        style={{
                          display: "inline-flex",
                          gap: 6,
                          alignItems: "center",
                        }}
                      >
                        <Tooltip
                          content={
                            <div style={{ lineHeight: 1.4 }}>
                              {m.managerType && (
                                <div>
                                  <strong>Type:</strong> {m.managerType}
                                </div>
                              )}
                              {m.instanceId && (
                                <div>
                                  <strong>InstanceID:</strong> {m.instanceId}
                                </div>
                              )}
                              {m.managerNamespace && (
                                <div>
                                  <strong>Manager Namespace:</strong>{" "}
                                  {m.managerNamespace}
                                </div>
                              )}
                              {m.adoptedFrom && (
                                <div>
                                  <strong>Adopted from:</strong> {m.adoptedFrom}
                                </div>
                              )}
                              {m.adoptedBy && (
                                <div>
                                  <strong>Adopted by:</strong> {m.adoptedBy}
                                </div>
                              )}
                              {m.adoptedAt && (
                                <div>
                                  <strong>Adopted at:</strong> {m.adoptedAt}
                                </div>
                              )}
                            </div>
                          }
                        >
                          <Label isCompact color="blue">
                            {labelText}
                          </Label>
                        </Tooltip>
                        {m.adoptedFrom && (
                          <Tooltip content="This session was adopted by the current manager">
                            <Label isCompact color="purple">
                              adopted
                            </Label>
                          </Tooltip>
                        )}
                      </div>
                    );
                  })()}
                </Td>
                <Td dataLabel="Phase">
                  <PhaseLabel phase={s.status?.phase} />
                </Td>
                <Td dataLabel="Replicas" textCenter>
                  <div
                    style={{
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 8,
                    }}
                  >
                    <Tooltip content="Scale down">
                      <Button
                        variant="plain"
                        onClick={() => onScale(s, -1)}
                        aria-label="Scale down"
                      >
                        <ArrowDownIcon />
                      </Button>
                    </Tooltip>
                    {(() => {
                      const key = `${s.metadata.namespace}/${s.metadata.name}`;
                      const current =
                        typeof s.spec.replicas === "number"
                          ? s.spec.replicas
                          : 1;
                      const target = pendingTargets[key];
                      if (typeof target === "number" && target !== current) {
                        return (
                          <Tooltip content="Applying…">
                            <strong>
                              {current}{" "}
                              <span className="pf-u-color-200">
                                (→ {target})
                              </span>
                            </strong>
                          </Tooltip>
                        );
                      }
                      return <strong>{current}</strong>;
                    })()}
                    <Tooltip content="Scale up">
                      <Button
                        variant="plain"
                        onClick={() => onScale(s, +1)}
                        aria-label="Scale up"
                      >
                        <ArrowUpIcon />
                      </Button>
                    </Tooltip>
                  </div>
                </Td>
                <Td dataLabel="Actions" modifier="fitContent">
                  <div style={{ display: "inline-flex", gap: 8 }}>
                    <Tooltip content="Open">
                      <Button
                        variant="secondary"
                        icon={<ExternalLinkAltIcon />}
                        onClick={() => onOpen(s)}
                      />
                    </Tooltip>
                    <Tooltip content="Delete">
                      <Button
                        variant="danger"
                        icon={<TrashIcon />}
                        onClick={() => onDelete(s)}
                      />
                    </Tooltip>
                  </div>
                </Td>
              </Tr>
            ))
          )}
        </Tbody>
      </Table>
    </div>
  );
}
