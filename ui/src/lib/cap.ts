import type { Introspection } from "../types/types";

export function can(
  ix: Introspection | null | undefined,
  ns: string,
  action: keyof NonNullable<Introspection["domains"][string]>["session"],
) {
  if (!ix) return false;
  // Treat UI's "All" as wildcard and check the requested action
  const key = ns === "All" ? "*" : ns;
  // 1) Try exact namespace (or "*")
  const direct = ix.domains?.[key]?.session?.[action];
  if (direct !== undefined) return !!direct;
  // 2) Fall back to cluster-wide "*" if per-namespace wasn’t enumerated
  if (key !== "*") return !!ix.domains?.["*"]?.session?.[action];
  return false;
}

// List namespaces where an action is allowed for the user
export function namespacesWhere(
  ix: Introspection | null | undefined,
  action: keyof NonNullable<Introspection["domains"][string]>["session"],
) {
  if (!ix) return [];
  return Object.entries(ix.domains)
    .filter(([ns, v]) => ns !== "*" && !!v?.session?.[action])
    .map(([ns]) => ns)
    .sort();
}
