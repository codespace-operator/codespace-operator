import { createContext, useContext, useMemo } from "react";
import { useIntrospection } from "../hooks/useIntrospection";
import type { Introspection } from "../types";

type Ctx = {
  data: Introspection | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
};

const IntrospectionContext = createContext<Ctx>({
  data: null,
  loading: true,
  error: null,
  refresh: () => {},
});

export function IntrospectionProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  // one fetch for the whole app
  const { data, loading, error } = useIntrospection({ discover: true });
  const value = useMemo<Ctx>(
    () => ({ data, loading, error, refresh: () => location.reload() }),
    [data, loading, error],
  );
  return (
    <IntrospectionContext.Provider value={value}>
      {children}
    </IntrospectionContext.Provider>
  );
}

export function useIx() {
  return useContext(IntrospectionContext);
}
