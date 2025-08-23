import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { Session, SessionEvent } from "../types";

export function useAlerts(max = 5) {
  const [alerts, setAlerts] = useState<{ key: string; title: string; variant: any }[]>([]);
  return {
    push: (title: string, variant: any = "info") =>
      setAlerts((a) => [{ key: Math.random().toString(36).slice(2), title, variant }, ...a].slice(0, max)),
    close: (key: string) => setAlerts((s) => s.filter((x) => x.key !== key)),
    list: alerts,
  };
}

export function useSessions(namespace: string, onError?: (msg: string) => void, enabled = true) {
  const [rows, setRows] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const esRef = useRef<EventSource | null>(null);
  const [pendingTargets, setPendingTargets] = useState<Record<string, number>>({});
  // Initial list
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const data = await api.list(namespace);
        if (!cancelled) setRows(data);
      } catch (e: any) {
        onError?.(e?.message || "Failed to load sessions");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [namespace, enabled]);

  // Live updates via SSE
  useEffect(() => {
    if (!enabled) return;
    if (esRef.current) esRef.current.close();
    const es = api.watch(namespace, (m) => {
      try {
        const ev = JSON.parse(m.data) as SessionEvent;
        setRows((list) => {
          if (ev.type === "DELETED") return list.filter((x) => x.metadata.name !== ev.object.metadata.name);
          const ix = list.findIndex((x) => x.metadata.name === ev.object.metadata.name);
          if (ix === -1) return [ev.object, ...list];
          const next = [...list]; next[ix] = ev.object; return next;
        });
        // any event touching an object is a good moment to clear optimistic target
        setPendingTargets((p) => {
          const k = `${ev.object.metadata.namespace}/${ev.object.metadata.name}`;
          if (!(k in p)) return p;
          const { [k]: _, ...rest } = p;
          return rest;
        });
      } catch {}
    });
    es.onerror = () => {};
    esRef.current = es;
    return () => es.close();
  }, [namespace, enabled]);

  const actions = {
    refresh: async () => {
      if (!enabled) return;
      setLoading(true);
      try { setRows(await api.list(namespace)); }
      finally { setLoading(false); }
    },
    create: (body: Partial<Session>) => api.create(body),
    remove: (ns: string, name: string) => api.remove(ns, name),
    scale: async (ns: string, name: string, replicas: number) => {
      const key = `${ns}/${name}`;
      setPendingTargets((p) => ({ ...p, [key]: replicas }));
      const res = await api.scale(ns, name, replicas);
      return res;
    },
  };

  return { rows, loading, pendingTargets, ...actions };
}

export function useFilteredSessions(rows: Session[], query: string) {
  return useMemo(() => {
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
}
