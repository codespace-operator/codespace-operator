import { useEffect, useState } from "react";
import { introspectApi } from "../api/client";

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
        // One call to rule them all
        const ix = await introspectApi.get({ discover: true });
        if (cancelled) return;
        const uniq = (arr?: string[]) => Array.from(new Set(arr || [])).sort();

        setSessionNamespaces(uniq(ix?.namespaces?.withSessions));
        // "writable" = namespaces the *server* can list, but if to list user-centric "where can I act",
        // use ix.namespaces.userAllowed instead (or expose a separate list from the backend).
        setWritableNamespaces(uniq(ix?.namespaces?.all));
        setError(null);
      } catch (e: any) {
        if (!cancelled) setError(e?.message || "Failed to load namespaces");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return { sessionNamespaces, writableNamespaces, loading, error };
}
