// ui/src/hooks/useProjects.ts
import { useEffect, useMemo, useRef, useState } from "react";
import { projectsApi } from "../api/client";
import { useIx } from "../context/IntrospectionContext";
import { can as canDo } from "../lib/cap";
import type {
  UIProject,
  ProjectCreateRequest,
  ProjectUpdateRequest,
} from "../types/projects";

export function useProjects(
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

  const [rows, setRows] = useState<UIProject[]>([]);
  const [loading, setLoading] = useState(false);
  const [pendingTargets, setPendingTargets] = useState<Record<string, any>>({});

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
        const data = await projectsApi.list(effectiveNs);
        if (!cancelled) setRows(data);
      } catch (e: any) {
        onError?.(e?.message || "Failed to load projects");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [effectiveNs, enabled, ix]);

  // Guarded actions using RBAC
  const actions = {
    refresh: async () => {
      if (!enabled) return;
      setLoading(true);
      try {
        setRows((await projectsApi.list(effectiveNs)) as UIProject[]);
      } finally {
        setLoading(false);
      }
    },
    create: async (req: ProjectCreateRequest) => {
      // Resolve namespace for the generated request shape
      const candidate =
        req.namespace ?? (effectiveNs !== "All" ? effectiveNs : undefined);

      if (!candidate) {
        // either ask the user to pick, or choose a sensible default
        const firstCreatable = ix?.namespaces?.userCreatable?.[0];
        if (!firstCreatable) {
          throw new Error("Pick a namespace to create the project in.");
        }
        req = { ...req, namespace: firstCreatable };
      } else {
        // enforce guard on the resolved ns
        if (!canDo(ix!, candidate, "create")) {
          throw new Error(`Not allowed to create in ${candidate}`);
        }
        req = { ...req, namespace: candidate };
      }
      // Minimal validation of required fields
      if (!req.name) throw new Error("Name is required.");

      return projectsApi.create(req);
    },
    remove: async (ns: string, name: string) => {
      if (!canDo(ix!, ns, "delete"))
        throw new Error(`Not allowed to delete in ${ns}`);
      return projectsApi.remove(ns, name);
    },
    update: async (ns: string, name: string, body: ProjectUpdateRequest) => {
      if (!canDo(ix!, ns, "update"))
        throw new Error(`Not allowed to update in ${ns}`);
      const key = `${ns}/${name}`;
      setPendingTargets((p) => ({ ...p, [key]: body }));
      return projectsApi.update(ns, name, body);
    },
    can: {
      list: (ns = effectiveNs) => canDo(ix!, ns === "All" ? "*" : ns, "list"),
      watch: (ns = effectiveNs) => canDo(ix!, ns === "All" ? "*" : ns, "watch"),
      create: (ns = effectiveNs) =>
        canDo(ix!, ns === "All" ? "*" : ns, "create"),
      delete: (ns = effectiveNs) =>
        canDo(ix!, ns === "All" ? "*" : ns, "delete"),
      update: (ns = effectiveNs) =>
        canDo(ix!, ns === "All" ? "*" : ns, "update"),
    },
    effectiveNamespace: effectiveNs,
  };

  return { rows, loading, pendingTargets, ...actions };
}

export function useFilteredProjects(rows: UIProject[], query: string) {
  return useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((project) => {
      const displayName = project.spec.displayName || "";
      const description = project.spec.description || "";
      return (
        project.metadata.name.toLowerCase().includes(q) ||
        displayName.toLowerCase().includes(q) ||
        description.toLowerCase().includes(q) ||
        project.metadata.namespace.toLowerCase().includes(q)
      );
    });
  }, [rows, query]);
}
