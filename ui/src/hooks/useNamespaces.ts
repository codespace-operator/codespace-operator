import { useEffect, useState, useMemo } from "react";
import { introspectApi } from "../api/client";
import { useUserIntrospection } from "./useIntrospection";

export function useNamespaces() {
  const [sessionNamespaces, setSessionNamespaces] = useState<string[]>([]);
  const [allNamespaces, setAllNamespaces] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Get user permissions to filter available namespaces
  const { data: userInfo, loading: userLoading } = useUserIntrospection({
    enabled: true,
  });

  useEffect(() => {
    let cancelled = false;

    const loadNamespaces = async () => {
      try {
        setLoading(true);
        setError(null);

        // Try to get server introspection data (may fail due to permissions)
        let serverData = null;
        try {
          serverData = await introspectApi.getServer({ discover: true });
        } catch (err) {
          // User might not have permissions to see server data, that's okay
          console.debug("Cannot access server introspection:", err);
        }

        if (cancelled) return;

        // Extract namespaces from server data if available
        let discoveredSessionNamespaces: string[] = [];
        let discoveredAllNamespaces: string[] = [];

        if (serverData?.namespaces) {
          discoveredSessionNamespaces =
            serverData.namespaces.withSessions || [];
          discoveredAllNamespaces = serverData.namespaces.all || [];
        }

        // If server data is empty, provide fallback namespaces
        if (discoveredSessionNamespaces.length === 0) {
          discoveredSessionNamespaces = ["default"];
        }
        if (discoveredAllNamespaces.length === 0) {
          discoveredAllNamespaces = ["default"];
        }

        // Filter namespaces based on user permissions if user data is available
        if (userInfo && userInfo.namespaces?.userAllowed) {
          const userAllowedSet = new Set(userInfo.namespaces.userAllowed);

          // Filter session namespaces to only those user can access
          discoveredSessionNamespaces = discoveredSessionNamespaces.filter(
            (ns) => userAllowedSet.has(ns),
          );

          // Filter all namespaces to only those user can access
          discoveredAllNamespaces = discoveredAllNamespaces.filter((ns) =>
            userAllowedSet.has(ns),
          );

          // Also add user's allowed namespaces that might not be in the discovered lists
          const additionalNamespaces = userInfo.namespaces.userAllowed.filter(
            (ns) =>
              !discoveredSessionNamespaces.includes(ns) &&
              !discoveredAllNamespaces.includes(ns),
          );

          discoveredAllNamespaces.push(...additionalNamespaces);
        }

        // Remove duplicates and sort
        const uniqueSessionNs = Array.from(
          new Set(discoveredSessionNamespaces),
        ).sort();
        const uniqueAllNs = Array.from(new Set(discoveredAllNamespaces)).sort();

        // Ensure we always have at least default namespace
        if (uniqueSessionNs.length === 0) {
          uniqueSessionNs.push("default");
        }
        if (uniqueAllNs.length === 0) {
          uniqueAllNs.push("default");
        }

        if (!cancelled) {
          setSessionNamespaces(uniqueSessionNs);
          setAllNamespaces(uniqueAllNs);
        }
      } catch (err: any) {
        if (!cancelled) {
          console.error("Failed to load namespaces:", err);
          setError(err?.message || "Failed to load namespaces");

          // Provide fallback based on user permissions if available
          if (userInfo?.namespaces?.userAllowed) {
            setSessionNamespaces(userInfo.namespaces.userAllowed.sort());
            setAllNamespaces(userInfo.namespaces.userAllowed.sort());
          } else {
            // Final fallback
            setSessionNamespaces(["default"]);
            setAllNamespaces(["default"]);
          }
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    // Wait for user info to load before loading namespaces
    if (!userLoading) {
      loadNamespaces();
    }

    return () => {
      cancelled = true;
    };
  }, [userInfo, userLoading]);

  // Computed properties based on user permissions
  const writableNamespaces = useMemo(() => {
    if (!userInfo?.namespaces?.userCreatable) {
      return [];
    }
    return userInfo.namespaces.userCreatable.sort();
  }, [userInfo]);

  const deletableNamespaces = useMemo(() => {
    if (!userInfo?.namespaces?.userDeletable) {
      return [];
    }
    return userInfo.namespaces.userDeletable.sort();
  }, [userInfo]);

  // Helper to check if user can perform action in namespace
  const canPerformAction = useMemo(() => {
    return (
      action:
        | "create"
        | "delete"
        | "list"
        | "get"
        | "update"
        | "watch"
        | "scale",
      namespace: string,
    ) => {
      if (!userInfo?.domains) return false;

      // Check specific namespace permissions
      const nsPerms = userInfo.domains[namespace];
      if (nsPerms?.session?.[action]) return true;

      // Check cluster-wide permissions (translated from "*" to "All")
      const clusterPerms = userInfo.domains["*"];
      if (clusterPerms?.session?.[action]) return true;

      return false;
    };
  }, [userInfo]);

  return {
    sessionNamespaces,
    allNamespaces,
    writableNamespaces,
    deletableNamespaces,
    loading: loading || userLoading,
    error,
    canPerformAction,
  };
}
