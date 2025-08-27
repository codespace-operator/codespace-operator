import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { Session, SessionEvent } from "../types";
import { useIx } from "../context/IntrospectionContext";
import { can as canDo } from "../lib/cap";

export function useSessions(
  namespace: string,
  onError?: (msg: string) => void,
  enabled = true,
) {
  const { data: ix } = useIx();

  // If namespace is "All" (from UI), translate to "*" for backend, but user can't watch "*", fallback to first allowed namespace
  const effectiveNs = useMemo(() => {
    if (namespace !== "All") return namespace;

    // Check if user can watch "*" (cluster-wide)
    const ok = canDo(ix!, "*", "watch");
    if (ok) return "All"; // Keep "All" for UI, will be translated to "*" in API calls

    // fallback: prefer one where list is allowed
    const allowed = Object.entries(ix?.domains || {}).find(
      ([ns, v]) => ns !== "*" && v.session?.list,
    );
    return allowed?.[0] || "default";
  }, [namespace, ix]);

  const [rows, setRows] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);
  const esRef = useRef<EventSource | null>(null);
  const [pendingTargets, setPendingTargets] = useState<Record<string, number>>(
    {},
  );

  // Initial list
  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        if (effectiveNs === "All" && !canDo(ix!, "*", "list")) {
          throw new Error(
            "You do not have list permissions across all namespaces.",
          );
        }
        const data = await api.list(effectiveNs);
        if (!cancelled) setRows(data);
      } catch (e: any) {
        onError?.(e?.message || "Failed to load sessions");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [effectiveNs, enabled, ix]);

  // Live updates via SSE
  useEffect(() => {
    if (!enabled) return;
    if (esRef.current) esRef.current.close();
    const es = api.watch(effectiveNs, (m) => {
      try {
        const ev = JSON.parse(m.data) as SessionEvent;
        setRows((list) => {
          if (ev.type === "DELETED")
            return list.filter(
              (x) => x.metadata.name !== ev.object.metadata.name,
            );
          const ix = list.findIndex(
            (x) => x.metadata.name === ev.object.metadata.name,
          );
          if (ix === -1) return [ev.object, ...list];
          const next = [...list];
          next[ix] = ev.object;
          return next;
        });
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
  }, [effectiveNs, enabled]);

  // Guarded actions using RBAC
  const actions = {
    refresh: async () => {
      if (!enabled) return;
      setLoading(true);
      try {
        setRows(await api.list(effectiveNs));
      } finally {
        setLoading(false);
      }
    },
    create: async (body: Partial<Session>) => {
      // Resolve namespace
      const candidate =
        body?.metadata?.namespace ??
        (effectiveNs !== "All" ? effectiveNs : undefined);

      if (!candidate) {
        // either ask the user to pick, or choose a sensible default
        const firstCreatable = ix?.namespaces?.userCreatable?.[0];
        if (!firstCreatable) {
          throw new Error("Pick a namespace to create the session in.");
        }
        body = {
          ...body,
          metadata: { ...(body?.metadata || {}), namespace: firstCreatable },
        } as Partial<Session>;
      } else {
        // enforce guard on the resolved ns
        if (!canDo(ix!, candidate, "create")) {
          throw new Error(`Not allowed to create in ${candidate}`);
        }
        body = {
          ...body,
          metadata: { ...(body?.metadata || {}), namespace: candidate },
        } as Partial<Session>;
      }
      return api.create(body);
    },
    remove: async (ns: string, name: string) => {
      if (!canDo(ix!, ns, "delete"))
        throw new Error(`Not allowed to delete in ${ns}`);
      return api.remove(ns, name);
    },
    scale: async (ns: string, name: string, replicas: number) => {
      if (!canDo(ix!, ns, "scale"))
        throw new Error(`Not allowed to scale in ${ns}`);
      const key = `${ns}/${name}`;
      setPendingTargets((p) => ({ ...p, [key]: replicas }));
      return api.scale(ns, name, replicas);
    },
    can: {
      list: (ns = effectiveNs) => canDo(ix!, ns === "All" ? "*" : ns, "list"),
      watch: (ns = effectiveNs) => canDo(ix!, ns === "All" ? "*" : ns, "watch"),
      create: (ns = effectiveNs) =>
        canDo(ix!, ns === "All" ? "*" : ns, "create"),
      delete: (ns = effectiveNs) =>
        canDo(ix!, ns === "All" ? "*" : ns, "delete"),
      scale: (ns = effectiveNs) => canDo(ix!, ns === "All" ? "*" : ns, "scale"),
    },
    effectiveNamespace: effectiveNs,
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
