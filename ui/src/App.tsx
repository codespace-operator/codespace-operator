import React from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import "@patternfly/react-core/dist/styles/base.css";
import {
  Page,
  PageSection,
  Masthead,
  MastheadMain,
  MastheadBrand,
  MastheadContent,
  Brand,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  TextInput,
  Button,
  Modal,
  Form,
  FormGroup,
  Select,
  SelectOption,
  Checkbox,
  Spinner,
  Title,
  Alert,
  AlertActionCloseButton,
  AlertGroup,
  Label,
  Tooltip
} from "@patternfly/react-core";
import { Table, Thead, Tbody, Tr, Th, Td } from "@patternfly/react-table";
import {
  PlusCircleIcon,
  ExternalLinkAltIcon,
  TrashIcon,
  SyncIcon,
  SearchIcon,
  ArrowUpIcon,
  ArrowDownIcon
} from "@patternfly/react-icons";

// ---- Types that match the Session CRD ----
// Minimal shape used by the UI
export type Session = {
  metadata: { name: string; namespace: string };
  spec: {
    profile: { ide: "jupyterlab" | "vscode" | "rstudio" | "custom"; image: string; cmd?: string[] };
    networking?: { host?: string };
    replicas?: number;
  };
  status?: { phase?: string; url?: string; reason?: string };
};

// ---- Simple API client around the gateway endpoints ----
const api = {
  async list(ns: string): Promise<Session[]> {
    const r = await fetch(`/api/sessions?namespace=${encodeURIComponent(ns)}`);
    if (!r.ok) throw new Error(await r.text());
    return (await r.json()) as Session[];
  },
  async create(body: Partial<Session>): Promise<Session> {
    const r = await fetch(`/api/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
    if (!r.ok) throw new Error(await r.text());
    return (await r.json()) as Session;
  },
  async remove(ns: string, name: string): Promise<void> {
    const r = await fetch(`/api/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, { method: "DELETE" });
    if (!r.ok) throw new Error(await r.text());
  },
  async scale(ns: string, name: string, replicas: number): Promise<Session> {
    const r = await fetch(`/api/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ replicas })
    });
    if (!r.ok) throw new Error(await r.text());
    return (await r.json()) as Session;
  },
  watch(ns: string, onEvent: (ev: { type: string; object: Session }) => void): EventSource {
    const es = new EventSource(`/api/watch/sessions?namespace=${encodeURIComponent(ns)}`);
    es.onmessage = (m) => {
      try {
        const data = JSON.parse(m.data);
        onEvent(data);
      } catch {}
    };
    return es;
  }
};

function PhaseLabel({ phase }: { phase?: string }) {
  const color: Record<string, "green" | "blue" | "orange" | "red" | "grey"> = {
    Ready: "green",
    Pending: "orange",
    Error: "red"
  };
  const intent: Record<string, "success" | "info" | "warning" | "danger" | "none"> = {
    Ready: "success",
    Pending: "warning",
    Error: "danger"
  } as any;
  const pfColor = intent[phase || "none"] || "info";
  return <Label color={pfColor as any}>{phase || "—"}</Label>;
}

function useAlerts() {
  const [alerts, setAlerts] = useState<{ key: string; title: string; variant: any }[]>([]);
  return {
    push: (title: string, variant: any = "info") =>
      setAlerts((a) => [{ key: Math.random().toString(36).slice(2), title, variant }, ...a].slice(0, 5)),
    Alerts: (
      <AlertGroup isToast isLiveRegion>
        {alerts.map((a) => (
          <Alert
            key={a.key}
            title={a.title}
            variant={a.variant}
            timeout={6000}
            actionClose={<AlertActionCloseButton onClose={() => setAlerts((s) => s.filter((x) => x.key !== a.key))} />}
          />
        ))}
      </AlertGroup>
    )
  } as const;
}

export default function App() {
  // State
  const [namespace, setNamespace] = useState<string>(localStorage.getItem("co_ns") || "default");
  const [rows, setRows] = useState<Session[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [query, setQuery] = useState<string>("");

  // Create modal
  const [isCreateOpen, setCreateOpen] = useState(false);
  const [cName, setCName] = useState("");
  const [cReplicas, setCReplicas] = useState<number>(1);
  const [cIDEOpen, setCIDEOpen] = useState<boolean>(false);
  const [cIDE, setCIDE] = useState<"jupyterlab" | "vscode">("jupyterlab");
  const [cImage, setCImage] = useState<string>("jupyter/minimal-notebook:latest");
  const [cCmd, setCCmd] = useState<string>("start-notebook.sh --NotebookApp.token=");
  const [cHost, setCHost] = useState<string>("");
  const [cConfirm, setCConfirm] = useState<boolean>(true);

  const alerts = useAlerts();
  const esRef = useRef<EventSource | null>(null);

  // Load list
  useEffect(() => {
    localStorage.setItem("co_ns", namespace);
    (async () => {
      setLoading(true);
      try {
        const data = await api.list(namespace);
        setRows(data);
      } catch (e: any) {
        alerts.push(e?.message || "Failed to load sessions", "danger");
      } finally {
        setLoading(false);
      }
    })();
  }, [namespace]);

  // Watch updates via SSE
  useEffect(() => {
    if (esRef.current) esRef.current.close();
    const es = api.watch(namespace, (ev) => {
      setRows((list) => {
        const ix = list.findIndex((x) => x.metadata.name === ev.object.metadata.name);
        if (ev.type === "DELETED") return list.filter((x) => x.metadata.name !== ev.object.metadata.name);
        if (ix === -1) return [ev.object, ...list];
        const next = [...list];
        next[ix] = ev.object;
        return next;
      });
    });
    es.onerror = () => {};
    esRef.current = es;
    return () => es.close();
  }, [namespace]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((s) => {
      const host = s.spec.networking?.host || "";
      return (
        s.metadata.name.toLowerCase().includes(q) ||
        s.spec.profile.image.toLowerCase().includes(q) ||
        host.toLowerCase().includes(q)
      );
    });
  }, [rows, query]);

  const openURL = (s: Session) => {
    const url = s.status?.url || (s.spec.networking?.host ? `https://${s.spec.networking.host}` : "");
    if (!url) return alerts.push("No URL yet", "info");
    window.open(url, "_blank");
  };

  const doCreate = async () => {
    if (!cName) return alerts.push("Name is required", "warning");
    const body: Partial<Session> = {
      metadata: { name: cName, namespace },
      spec: {
        replicas: cReplicas,
        profile: { ide: cIDE, image: cImage, cmd: cCmd.trim() ? cCmd.split(/\s+/) : undefined },
        networking: cHost ? { host: cHost } : undefined
      }
    } as any;
    try {
      const created = await api.create(body);
      alerts.push(`Session ${created.metadata.name} created`, "success");
      setCreateOpen(false);
      setCName("");
    } catch (e: any) {
      alerts.push(e?.message || "Create failed", "danger");
    }
  };

  const doScale = async (s: Session, delta: number) => {
    const current = typeof s.spec.replicas === "number" ? s.spec.replicas : 1;
    const next = Math.max(0, current + delta);
    try {
      await api.scale(s.metadata.namespace, s.metadata.name, next);
      alerts.push(`Scaled to ${next}`, "success");
    } catch (e: any) {
      alerts.push(e?.message || "Scale failed", "danger");
    }
  };

  const doDelete = async (s: Session) => {
    if (!cConfirm) return alerts.push("Enable delete confirmation checkbox first", "warning");
    if (!confirm(`Delete ${s.metadata.name}?`)) return;
    try {
      await api.remove(s.metadata.namespace, s.metadata.name);
      alerts.push("Deleted", "success");
    } catch (e: any) {
      alerts.push(e?.message || "Delete failed", "danger");
    }
  };

  return (
    <Page
      header={
        <Masthead backgroundColor="light">
          <MastheadMain>
            <MastheadBrand>
              <Brand src="" alt="Codespace Operator" />
              <Title headingLevel="h2" style={{ marginLeft: 12 }}>
                Codespace Operator
              </Title>
            </MastheadBrand>
          </MastheadMain>
          <MastheadContent>
            <Toolbar isFullHeight isStatic>
              <ToolbarContent>
                <ToolbarItem>
                  <TextInput
                    aria-label="Namespace"
                    value={namespace}
                    onChange={(_, v) => setNamespace(v)}
                    placeholder="namespace"
                  />
                </ToolbarItem>
                <ToolbarItem>
                  <Button
                    variant="secondary"
                    icon={<SyncIcon />}
                    onClick={async () => {
                      setLoading(true);
                      try {
                        setRows(await api.list(namespace));
                      } finally {
                        setLoading(false);
                      }
                    }}
                  >
                    Refresh
                  </Button>
                </ToolbarItem>
              </ToolbarContent>
            </Toolbar>
          </MastheadContent>
        </Masthead>
      }
    >
      {alerts.Alerts}

      <PageSection isWidthLimited>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <TextInput
                aria-label="Search"
                iconVariant="search"
                value={query}
                onChange={(_, v) => setQuery(v)}
                placeholder="Search by name / image / host"
              />
            </ToolbarItem>
            <ToolbarItem>
              <Button icon={<PlusCircleIcon />} variant="primary" onClick={() => setCreateOpen(true)}>
                New Session
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>

        <div style={{ border: "1px solid var(--pf-global--BorderColor--100)", borderRadius: 6, overflow: "hidden" }}>
          <Table aria-label="Sessions table" variant="compact" borders>
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
              ) : filtered.length === 0 ? (
                <Tr>
                  <Td colSpan={8}>No sessions</Td>
                </Tr>
              ) : (
                filtered.map((s) => (
                  <Tr key={s.metadata.name}>
                    <Td dataLabel="Name">{s.metadata.name}</Td>
                    <Td dataLabel="Namespace">{s.metadata.namespace}</Td>
                    <Td dataLabel="IDE">{s.spec.profile.ide}</Td>
                    <Td dataLabel="Image" modifier="truncate">{s.spec.profile.image}</Td>
                    <Td dataLabel="Host" modifier="truncate">{s.spec.networking?.host || ""}</Td>
                    <Td dataLabel="Phase">
                      <PhaseLabel phase={s.status?.phase} />
                    </Td>
                    <Td dataLabel="Replicas" textCenter>
                      <div style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                        <Tooltip content="Scale down">
                          <Button variant="plain" onClick={() => doScale(s, -1)} aria-label="Scale down">
                            <ArrowDownIcon />
                          </Button>
                        </Tooltip>
                        <strong>{typeof s.spec.replicas === "number" ? s.spec.replicas : 1}</strong>
                        <Tooltip content="Scale up">
                          <Button variant="plain" onClick={() => doScale(s, +1)} aria-label="Scale up">
                            <ArrowUpIcon />
                          </Button>
                        </Tooltip>
                      </div>
                    </Td>
                    <Td dataLabel="Actions" modifier="fitContent">
                      <div style={{ display: "inline-flex", gap: 8 }}>
                        <Tooltip content="Open">
                          <Button variant="secondary" icon={<ExternalLinkAltIcon />} onClick={() => openURL(s)} />
                        </Tooltip>
                        <Tooltip content="Delete (enable confirmation below)">
                          <Button variant="danger" icon={<TrashIcon />} onClick={() => doDelete(s)} />
                        </Tooltip>
                      </div>
                    </Td>
                  </Tr>
                ))
              )}
            </Tbody>
          </Table>
        </div>

        <div style={{ marginTop: 8 }}>
          <Checkbox
            id="confirm-delete"
            label="Require confirmation to delete"
            isChecked={cConfirm}
            onChange={(_, v) => setCConfirm(v)}
          />
        </div>
      </PageSection>

      {/* Create modal */}
      <Modal
        title="Create Session"
        isOpen={isCreateOpen}
        onClose={() => setCreateOpen(false)}
        actions={[
          <Button key="create" variant="primary" onClick={doCreate}>
            Create
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setCreateOpen(false)}>
            Cancel
          </Button>
        ]}
      >
        <Form>
          <FormGroup label="Name" isRequired fieldId="name">
            <TextInput id="name" value={cName} onChange={(_, v) => setCName(v)} />
          </FormGroup>
          <FormGroup label="Replicas" fieldId="replicas">
            <TextInput id="replicas" type="number" value={String(cReplicas)} onChange={(_, v) => setCReplicas(Math.max(0, parseInt(v || "0")))} />
          </FormGroup>
          <FormGroup label="IDE" fieldId="ide">
            <Select
              aria-label="IDE"
              isOpen={cIDEOpen}
              onToggle={(v) => setCIDEOpen(v)}
              onSelect={(_, v) => {
                const val = String(v) as any;
                setCIDE(val);
                setCIDEOpen(false);
                if (val === "vscode") {
                  setCImage("codercom/code-server:latest");
                  setCCmd("--bind-addr 0.0.0.0:8080 --auth none");
                } else {
                  setCImage("jupyter/minimal-notebook:latest");
                  setCCmd("start-notebook.sh --NotebookApp.token=");
                }
              }}
              selections={cIDE}
            >
              <SelectOption value="jupyterlab">JupyterLab</SelectOption>
              <SelectOption value="vscode">VS Code (code-server)</SelectOption>
            </Select>
          </FormGroup>
          <FormGroup label="Image" isRequired fieldId="image">
            <TextInput id="image" value={cImage} onChange={(_, v) => setCImage(v)} />
          </FormGroup>
          <FormGroup label="Command" fieldId="cmd">
            <TextInput id="cmd" value={cCmd} onChange={(_, v) => setCCmd(v)} />
          </FormGroup>
          <FormGroup label="Public Host (optional)" fieldId="host">
            <TextInput id="host" placeholder="lab.example.dev" value={cHost} onChange={(_, v) => setCHost(v)} />
          </FormGroup>
        </Form>
      </Modal>
    </Page>
  );
}
