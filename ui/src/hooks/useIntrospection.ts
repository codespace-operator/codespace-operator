// useIntrospection.ts
import { useEffect, useState } from "react";
import { introspectApi } from "../api/client";
import type { Introspection } from "../types";

export function useIntrospection({
  discover = true,
  namespaces,
  roles,
  enabled = true,
}: {
  discover?: boolean;
  namespaces?: string[];
  roles?: string[];
  enabled?: boolean;
}) {
  const [data, setData] = useState<Introspection | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) {
      // when disabled, don't fetch and don't show a spurious "loading" state
      setLoading(false);
      return;
    }

    let cancelled = false;
    const ac = new AbortController();

    (async () => {
      try {
        setLoading(true);
        const res = await introspectApi.get({ discover, namespaces, roles });
        if (!cancelled) {
          setData(res);
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled) {
          setError(e?.message || "Failed to introspect");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [
    enabled,
    discover,
    JSON.stringify(namespaces || []),
    JSON.stringify(roles || []),
  ]);

  return { data, loading: enabled && loading, error };
}
