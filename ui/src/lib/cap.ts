import type { Introspection } from "../types";

export function can(
  ix: Introspection | null | undefined,
  ns: string,
  action: keyof NonNullable<Introspection["domains"][string]>["session"],
) {
  if (!ix) return false;
  // Allow "All" only if user can watch across "*"
  if (ns === "All") return !!ix.domains["*"]?.session?.watch;
  return !!ix.domains?.[ns]?.session?.[action];
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
