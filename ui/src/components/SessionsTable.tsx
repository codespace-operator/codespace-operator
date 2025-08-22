import React from "react";
import { Table, Thead, Tbody, Tr, Th, Td } from "@patternfly/react-table";
import { Button, Label, Tooltip, Spinner } from "@patternfly/react-core";
import { ExternalLinkAltIcon, TrashIcon, ArrowUpIcon, ArrowDownIcon } from "@patternfly/react-icons";
import type { Session } from "../types";

function PhaseLabel({ phase }: { phase?: string }) {
  const intent: Record<string, "success" | "info" | "warning" | "danger" | "none"> = {
    Ready: "success",
    Pending: "warning",
    Error: "danger",
    none: "info",
  } as any;
  const pfColor = intent[phase || "none"] || "info";
  return <Label color={pfColor as any}>{phase || "—"}</Label>;
}

type Props = {
  loading: boolean;
  rows: Session[];
  onScale: (s: Session, delta: number) => void;
  onDelete: (s: Session) => void;
  onOpen: (s: Session) => void;
};

export function SessionsTable({ loading, rows, onScale, onDelete, onOpen }: Props) {
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
            <Th>Phase</Th>
            <Th textCenter>Replicas</Th>
            <Th modifier="fitContent">Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {loading ? (
            <Tr>
              <Td colSpan={8}>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <Spinner size="md" /> Loading…
                </div>
              </Td>
            </Tr>
          ) : rows.length === 0 ? (
            <Tr>
              <Td colSpan={8}>No sessions</Td>
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
                <Td dataLabel="Phase">
                  <PhaseLabel phase={s.status?.phase} />
                </Td>
                <Td dataLabel="Replicas" textCenter>
                  <div style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                    <Tooltip content="Scale down">
                      <Button variant="plain" onClick={() => onScale(s, -1)} aria-label="Scale down">
                        <ArrowDownIcon />
                      </Button>
                    </Tooltip>
                    <strong>{typeof s.spec.replicas === "number" ? s.spec.replicas : 1}</strong>
                    <Tooltip content="Scale up">
                      <Button variant="plain" onClick={() => onScale(s, +1)} aria-label="Scale up">
                        <ArrowUpIcon />
                      </Button>
                    </Tooltip>
                  </div>
                </Td>
                <Td dataLabel="Actions" modifier="fitContent">
                  <div style={{ display: "inline-flex", gap: 8 }}>
                    <Tooltip content="Open">
                      <Button variant="secondary" icon={<ExternalLinkAltIcon />} onClick={() => onOpen(s)} />
                    </Tooltip>
                    <Tooltip content="Delete">
                      <Button variant="danger" icon={<TrashIcon />} onClick={() => onDelete(s)} />
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
