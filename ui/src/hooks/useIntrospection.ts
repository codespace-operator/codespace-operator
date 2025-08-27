import { useEffect, useState } from "react";
import { introspectApi } from "../api/client";
import type {
  UserIntrospection,
  ServerIntrospection,
  Introspection,
} from "../types";

// Hook for user-specific introspection
export function useUserIntrospection({
  namespaces,
  enabled = true,
}: {
  namespaces?: string[];
  enabled?: boolean;
} = {}) {
  const [data, setData] = useState<UserIntrospection | null>(null);
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
        const res = await introspectApi.getUser({ namespaces });
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
  }, [enabled, JSON.stringify(namespaces || [])]);

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
  const [data, setData] = useState<ServerIntrospection | null>(null);
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
        // Use the new split endpoints internally
        const [userRes, serverRes] = await Promise.allSettled([
          introspectApi.getUser({ namespaces }),
          introspectApi.getServer({ discover }),
        ]);

        if (!cancelled) {
          let combinedData: Introspection | null = null;

          if (userRes.status === "fulfilled") {
            const user = userRes.value;
            combinedData = {
              user: user.user,
              domains: user.domains,
              namespaces: {
                userAllowed: user.namespaces.userAllowed,
                userCreatable: user.namespaces.userCreatable,
                userDeletable: user.namespaces.userDeletable,
              },
              capabilities: {
                namespaceScope: user.capabilities.namespaceScope,
                clusterScope: user.capabilities.clusterScope,
                adminAccess: user.capabilities.adminAccess,
                multiTenant: false, // Default, will be overridden if server data available
              },
            };

            // Add server data if available
            if (serverRes.status === "fulfilled") {
              const server = serverRes.value;
              combinedData.cluster = server.cluster;
              combinedData.namespaces.all = server.namespaces.all;
              combinedData.namespaces.withSessions =
                server.namespaces.withSessions;
              combinedData.capabilities.multiTenant =
                server.capabilities.multiTenant;
            }
          } else {
            // If user request failed, we can't provide meaningful data
            setError(
              userRes.reason?.message || "Failed to get user information",
            );
            setData(null);
            return;
          }

          setData(combinedData);
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
