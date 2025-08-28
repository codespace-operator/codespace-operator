import { useEffect, useState } from "react";
import { introspectApi } from "../api/client";
import type { components } from "../types/api.gen";
import type { Introspection } from "../types";

type UserIntrospectionResponse =
  components["schemas"]["cmd_server.UserIntrospectionResponse"];
type ServerIntrospectionResponse =
  components["schemas"]["cmd_server.ServerIntrospectionResponse"];
// Hook for user-specific introspection
export function useUserIntrospection({
  namespaces,
  discover,
  enabled = true,
}: { namespaces?: string[]; discover?: boolean; enabled?: boolean } = {}) {
  const [data, setData] = useState<UserIntrospectionResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    const ac = new AbortController();

    (async () => {
      try {
        setLoading(true);
        const res = await introspectApi.getUser({ namespaces, discover });
        if (!cancelled) {
          setData(res);
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled) {
          setError(e?.message || "Failed to get user information");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [enabled, discover, JSON.stringify(namespaces || [])]);

  return { data, loading: enabled && loading, error };
}

// Hook for server-specific introspection
export function useServerIntrospection({
  discover = false,
  enabled = true,
}: {
  discover?: boolean;
  enabled?: boolean;
} = {}) {
  const [data, setData] = useState<ServerIntrospectionResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    const ac = new AbortController();

    (async () => {
      try {
        setLoading(true);
        const res = await introspectApi.getServer({ discover });
        if (!cancelled) {
          setData(res);
          setError(null);
        }
      } catch (e: any) {
        if (!cancelled) {
          setError(e?.message || "Failed to get server information");
          // Don't treat permission errors as critical
          if (e?.message?.includes("Insufficient permissions")) {
            setData(null); // Clear any previous data
            setError(null); // Clear error for permission issues
          }
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
      ac.abort();
    };
  }, [enabled, discover]);

  return { data, loading: enabled && loading, error };
}

// Legacy hook that combines both (deprecated but maintained for backward compatibility)
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
} = {}) {
  const [data, setData] = useState<Introspection | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    const ac = new AbortController();

    (async () => {
      try {
        setLoading(true);
        // Keep using the legacy endpoint for backward compatibility
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
