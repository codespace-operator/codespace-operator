import React, { useEffect, useMemo, useRef, useState } from "react";
import { toast, Toaster } from "sonner";
import { Loader2, RefreshCw, Plus, Trash2, Pencil, ExternalLink, Scale, ServerCog } from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";

type Session = {
  metadata: { name: string; namespace: string; creationTimestamp?: string };
  spec: {
    replicas?: number;
    profile: { ide: string; image: string; cmd?: string[] };
    networking?: { host?: string };
  };
  status?: { phase?: "Pending" | "Ready" | "Error"; url?: string };
};

const api = {
  async list(ns: string): Promise<Session[]> {
    const r = await fetch(`/api/sessions?namespace=${encodeURIComponent(ns)}`);
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  },
  async create(sess: Session): Promise<Session> {
    const r = await fetch(`/api/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sess),
    });
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  },
  async patch(ns: string, name: string, patch: any): Promise<Session> {
    const r = await fetch(`/api/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/merge-patch+json" },
      body: JSON.stringify(patch),
    });
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  },
  async remove(ns: string, name: string) {
    const r = await fetch(`/api/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, { method: "DELETE" });
    if (!r.ok) throw new Error(await r.text());
  },
  async scale(ns: string, name: string, replicas: number) {
    const r = await fetch(`/api/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ replicas }),
    });
    if (!r.ok) throw new Error(await r.text());
  },
};

function Badge({ children, color }: { children: React.ReactNode; color: "green" | "amber" | "rose" | "slate" }) {
  const map = {
    green: "bg-emerald-600",
    amber: "bg-amber-600",
    rose: "bg-rose-600",
    slate: "bg-slate-600",
  } as const;
  return <span className={`text-white text-xs px-2 py-0.5 rounded ${map[color]}`}>{children}</span>;
}

function Phase({ phase }: { phase?: string }) {
  if (phase === "Ready") return <Badge color="green">Ready</Badge>;
  if (phase === "Pending") return <Badge color="amber">Pending</Badge>;
  if (phase === "Error") return <Badge color="rose">Error</Badge>;
  return <Badge color="slate">—</Badge>;
}

export default function App() {
  const [ns, setNs] = useState<string>(localStorage.getItem("co_ns") || "default");
  const [items, setItems] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [q, setQ] = useState("");
  const [newOpen, setNewOpen] = useState(false);
  const [name, setName] = useState("");
  const [ide, setIde] = useState<"jupyterlab" | "vscode">("jupyterlab");
  const [image, setImage] = useState("jupyter/minimal-notebook:latest");
  const [cmd, setCmd] = useState("start-notebook.sh --NotebookApp.token=");
  const [host, setHost] = useState("");
  const [replicas, setReplicas] = useState(1);
  const sseRef = useRef<EventSource | null>(null);

  useEffect(() => {
    localStorage.setItem("co_ns", ns);
    (async () => {
      setLoading(true);
      try {
        setItems(await api.list(ns));
      } catch (e: any) {
        toast.error(e.message || "Failed to load sessions");
      } finally {
        setLoading(false);
      }
    })();
  }, [ns]);

  useEffect(() => {
    if (sseRef.current) sseRef.current.close();
    const es = new EventSource(`/api/watch/sessions?namespace=${encodeURIComponent(ns)}`);
    sseRef.current = es;
    es.onmessage = (ev) => {
      try {
        const evt = JSON.parse(ev.data) as { type: "ADDED" | "MODIFIED" | "DELETED"; object: Session };
        setItems((cur) => {
          const idx = cur.findIndex((x) => x.metadata.name === evt.object.metadata.name);
          if (evt.type === "DELETED") return cur.filter((x) => x.metadata.name !== evt.object.metadata.name);
          if (idx === -1) return [evt.object, ...cur];
          const next = [...cur];
          next[idx] = evt.object;
          return next;
        });
      } catch {}
    };
    es.onerror = () => {};
    return () => es.close();
  }, [ns]);

  const filtered = useMemo(
    () =>
      items.filter((s) => {
        if (!q) return true;
        const hay = `${s.metadata.name} ${s.spec.profile.image} ${s.spec.networking?.host || ""}`.toLowerCase();
        return hay.includes(q.toLowerCase());
      }),
    [items, q]
  );

  const create = async () => {
    const sess: Session = {
      metadata: { name, namespace: ns },
      spec: {
        replicas,
        profile: { ide, image, cmd: cmd.trim() ? cmd.split(/\s+/) : undefined },
        networking: host ? { host } : undefined,
      },
    };
    try {
      const out = await api.create(sess);
      toast.success(`Session ${out.metadata.name} created`);
      setNewOpen(false);
      setName("");
    } catch (e: any) {
      toast.error(e.message || "Create failed");
    }
  };

  const openURL = (s: Session) => {
    const url = s.status?.url || (s.spec.networking?.host ? `https://${s.spec.networking.host}` : "");
    if (!url) return toast.info("No URL yet");
    window.open(url, "_blank");
  };

  return (
    <div className="min-h-screen">
      <Toaster richColors closeButton />
      {/* Top bar */}
      <div className="flex items-center justify-between px-4 py-3 border-b bg-white sticky top-0 z-20">
        <div className="flex items-center gap-2">
          <ServerCog className="h-6 w-6" />
          <span className="font-semibold text-lg">Codespace Operator</span>
          <span className="text-xs text-slate-500 ml-1">Admin UI</span>
        </div>
        <div className="flex items-center gap-2">
          <input
            value={ns}
            onChange={(e) => setNs(e.target.value)}
            className="border rounded px-2 py-1 text-sm w-44"
            placeholder="namespace"
          />
          <button
            onClick={() => (async () => { setLoading(true); try { setItems(await api.list(ns)); } finally { setLoading(false); } })()}
            className="p-2 rounded hover:bg-slate-100"
            title="Refresh"
          >
            <RefreshCw className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Controls */}
      <main className="max-w-6xl mx-auto p-4">
        <div className="flex items-center justify-between mb-3">
          <input
            className="border rounded px-3 py-2 w-80"
            placeholder="Search by name / image / host"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <button
            className="inline-flex items-center gap-2 bg-black text-white px-3 py-2 rounded"
            onClick={() => setNewOpen(true)}
          >
            <Plus className="h-4 w-4" /> New Session
          </button>
        </div>

        {/* Table */}
        <div className="bg-white border rounded overflow-hidden">
          <div className="grid grid-cols-9 text-xs font-semibold text-slate-600 px-3 py-2 border-b">
            <div></div>
            <div>Name</div>
            <div>Namespace</div>
            <div>IDE</div>
            <div className="col-span-2">Image</div>
            <div>Host</div>
            <div>Phase</div>
            <div className="text-right">Actions</div>
          </div>
          {loading ? (
            <div className="flex items-center justify-center py-16 text-slate-500">
              <Loader2 className="h-5 w-5 animate-spin mr-2" /> Loading…
            </div>
          ) : (
            <div>
              <AnimatePresence initial={false}>
                {filtered.map((s) => (
                  <motion.div
                    key={s.metadata.name}
                    initial={{ opacity: 0, y: 6 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.15 }}
                    className="grid grid-cols-9 px-3 py-2 items-center border-t text-sm"
                  >
                    <div>
                      <span
                        className={`inline-block h-2 w-2 rounded-full ${
                          s.status?.phase === "Ready" ? "bg-emerald-600" : s.status?.phase === "Error" ? "bg-rose-600" : "bg-amber-600"
                        }`}
                      />
                    </div>
                    <div className="font-medium">{s.metadata.name}</div>
                    <div>{s.metadata.namespace}</div>
                    <div className="capitalize">{s.spec.profile.ide}</div>
                    <div className="col-span-2 truncate" title={s.spec.profile.image}>
                      {s.spec.profile.image}
                    </div>
                    <div>{s.spec.networking?.host || "—"}</div>
                    <div>
                      <Phase phase={s.status?.phase} />
                    </div>
                    <div className="text-right">
                      <div className="inline-flex items-center gap-1">
                        <button className="p-1 hover:bg-slate-100 rounded" title="Open" onClick={() => openURL(s)}>
                          <ExternalLink className="h-4 w-4" />
                        </button>
                        <button
                          className="p-1 hover:bg-slate-100 rounded"
                          title="Scale to 1"
                          onClick={async () => {
                            try {
                              await api.scale(s.metadata.namespace, s.metadata.name, 1);
                              toast.success("Scaled");
                            } catch (e: any) {
                              toast.error(e.message);
                            }
                          }}
                        >
                          <Scale className="h-4 w-4" />
                        </button>
                        <button
                          className="p-1 hover:bg-slate-100 rounded"
                          title="Patch: replicas=2"
                          onClick={async () => {
                            try {
                              await api.patch(s.metadata.namespace, s.metadata.name, { spec: { replicas: 2 } });
                              toast.success("Patched");
                            } catch (e: any) {
                              toast.error(e.message);
                            }
                          }}
                        >
                          <Pencil className="h-4 w-4" />
                        </button>
                        <button
                          className="p-1 hover:bg-slate-100 rounded"
                          title="Delete"
                          onClick={async () => {
                            if (!confirm(`Delete ${s.metadata.name}?`)) return;
                            try {
                              await api.remove(s.metadata.namespace, s.metadata.name);
                              toast.success("Deleted");
                            } catch (e: any) {
                              toast.error(e.message);
                            }
                          }}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    </div>
                  </motion.div>
                ))}
              </AnimatePresence>
              {filtered.length === 0 && <div className="py-8 text-center text-slate-500 text-sm">No sessions</div>}
            </div>
          )}
        </div>
      </main>

      {/* Simple modal */}
      {newOpen && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg w-full max-w-xl p-4 shadow">
            <div className="text-lg font-semibold mb-3">Create Session</div>
            <div className="grid grid-cols-2 gap-3">
              <label className="text-sm">Name
                <input className="mt-1 border rounded px-2 py-1 w-full" value={name} onChange={(e) => setName(e.target.value)} />
              </label>
              <label className="text-sm">Replicas
                <input type="number" min={1} className="mt-1 border rounded px-2 py-1 w-full" value={replicas}
                  onChange={(e) => setReplicas(parseInt(e.target.value || "1"))} />
              </label>
              <label className="text-sm">IDE
                <select
                  className="mt-1 border rounded px-2 py-1 w-full"
                  value={ide}
                  onChange={(e) => {
                    const v = e.target.value as "jupyterlab" | "vscode";
                    setIde(v);
                    if (v === "vscode") {
                      setImage("codercom/code-server:latest");
                      setCmd("--bind-addr 0.0.0.0:8080 --auth none");
                    } else {
                      setImage("jupyter/minimal-notebook:latest");
                      setCmd("start-notebook.sh --NotebookApp.token=");
                    }
                  }}
                >
                  <option value="jupyterlab">JupyterLab</option>
                  <option value="vscode">VS Code (code-server)</option>
                </select>
              </label>
              <label className="text-sm">Image
                <input className="mt-1 border rounded px-2 py-1 w-full" value={image} onChange={(e) => setImage(e.target.value)} />
              </label>
              <label className="text-sm col-span-2">Command
                <input className="mt-1 border rounded px-2 py-1 w-full" value={cmd} onChange={(e) => setCmd(e.target.value)} />
              </label>
              <label className="text-sm col-span-2">Public Host (optional)
                <input className="mt-1 border rounded px-2 py-1 w-full" placeholder="lab.example.com" value={host} onChange={(e) => setHost(e.target.value)} />
              </label>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <button className="px-3 py-2 rounded border" onClick={() => setNewOpen(false)}>Cancel</button>
              <button
                className="px-3 py-2 rounded bg-black text-white disabled:opacity-60"
                onClick={create}
                disabled={!name}
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
