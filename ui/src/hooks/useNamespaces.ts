import { useEffect, useState } from "react";
import { nsApi } from "../api/client";

export function useNamespaces() {
  const [sessionNamespaces, setSessionNamespaces] = useState<string[]>([]);
  const [writableNamespaces, setWritableNamespaces] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        const [a, b] = await Promise.all([
          nsApi.listSessionNamespaces(),
          nsApi.listWritableNamespaces(),
        ]);
        if (!cancelled) {
          const uniq = (arr: string[]) => Array.from(new Set(arr)).sort();
          setSessionNamespaces(uniq(a || []));
          setWritableNamespaces(uniq(b || []));
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled) setError(e?.message || "Failed to load namespaces");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  return { sessionNamespaces, writableNamespaces, loading, error };
}
