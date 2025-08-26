import { useEffect, useState } from "react";
import { introspectApi } from "../api/client";
import type { Introspection } from "../types";

export function useIntrospection({
  discover = true,
  namespaces,
  roles,
}: {
  discover?: boolean;
  namespaces?: string[];
  roles?: string[];
}) {
  const [data, setData] = useState<Introspection | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        const res = await introspectApi.get({ discover, namespaces, roles });
        if (!cancelled) {
          setData(res);
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled) setError(e?.message || "Failed to introspect");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [discover, JSON.stringify(namespaces || []), JSON.stringify(roles || [])]);

  return { data, loading, error };
}
