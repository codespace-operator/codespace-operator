import { useEffect, useState, useMemo } from "react";
import {
  useUserIntrospection,
  useServerIntrospection,
} from "./useIntrospection";

export function useNamespaces() {
  const [sessionNamespaces, setSessionNamespaces] = useState<string[]>([]); // Namespaces containing sessions
  const [allNamespaces, setAllNamespaces] = useState<string[]>([]); // All accessible namespaces
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Get user permissions using new split API
  const { data: userInfo, loading: userLoading } = useUserIntrospection({
    discover: true, // Enable namespace discovery
    enabled: true,
  });

  // Get server capabilities using new split API
  const { data: serverInfo, loading: serverLoading } = useServerIntrospection({
    discover: true, // Enable namespace discovery
    enabled: true, // Always try, will gracefully handle permission errors
  });

  useEffect(() => {
    if (userLoading || serverLoading) {
      setLoading(true);
      return;
    }

    try {
      setError(null);

      // Start with user's accessible namespaces
      let userAccessible: string[] = [];
      let serverDiscovered: string[] = [];
      let sessionsDiscovered: string[] = [];

      // Extract user-accessible namespaces
      if (userInfo?.namespaces?.userAllowed) {
        userAccessible = [...userInfo.namespaces.userAllowed];
      }

      // Extract server-discovered namespaces (if available)
      if (serverInfo?.namespaces) {
        serverDiscovered = [...(serverInfo.namespaces.all || [])];
        sessionsDiscovered = [...(serverInfo.namespaces.withSessions || [])];
      }

      // Merge and filter namespaces based on user permissions
      let finalAllNamespaces: string[];
      let finalSessionNamespaces: string[];

      if (userInfo?.capabilities?.clusterScope) {
        // User has cluster access - include all discovered namespaces
        const combined = new Set([...userAccessible, ...serverDiscovered]);
        finalAllNamespaces = Array.from(combined).sort();

        // For session namespaces, prefer server data if available, otherwise user data
        finalSessionNamespaces =
          sessionsDiscovered.length > 0
            ? sessionsDiscovered.sort()
            : userAccessible.sort();
      } else {
        // User has limited access - only show namespaces they can access
        finalAllNamespaces = userAccessible.sort();

        // Filter session namespaces to only those user can access
        const userAllowedSet = new Set(userAccessible);
        finalSessionNamespaces = sessionsDiscovered
          .filter((ns) => userAllowedSet.has(ns))
          .sort();

        // If no session namespaces after filtering, use user accessible
        if (finalSessionNamespaces.length === 0) {
          finalSessionNamespaces = userAccessible.sort();
        }
      }

      // Ensure we always have at least default
      if (finalAllNamespaces.length === 0) {
        finalAllNamespaces = ["default"];
      }
      if (finalSessionNamespaces.length === 0) {
        finalSessionNamespaces = ["default"];
      }

      setAllNamespaces(finalAllNamespaces);
      setSessionNamespaces(finalSessionNamespaces);
    } catch (err: any) {
      console.error("Failed to process namespaces:", err);
      setError(err?.message || "Failed to load namespaces");

      // Fallback to user data or default
      const fallback = userInfo?.namespaces?.userAllowed?.sort() || ["default"];
      setAllNamespaces(fallback);
      setSessionNamespaces(fallback);
    } finally {
      setLoading(false);
    }
  }, [userInfo, serverInfo, userLoading, serverLoading]);

  // Computed properties based on user permissions
  const writableNamespaces = useMemo(() => {
    return userInfo?.namespaces?.userCreatable?.sort() || [];
  }, [userInfo]);

  const deletableNamespaces = useMemo(() => {
    return userInfo?.namespaces?.userDeletable?.sort() || [];
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

      // Check cluster-wide permissions
      const clusterPerms = userInfo.domains["*"];
      if (clusterPerms?.session?.[action]) return true;

      return false;
    };
  }, [userInfo]);

  return {
    sessionNamespaces, // Namespaces containing sessions
    allNamespaces, // All accessible namespaces
    writableNamespaces, // Namespaces user can create in
    deletableNamespaces, // Namespaces user can delete from
    loading,
    error,
    canPerformAction,
  };
}
