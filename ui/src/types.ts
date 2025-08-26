// Shared types that mirror the Session CRD (fields the UI needs)
export type Session = {
  metadata: { name: string; namespace: string };
  spec: {
    profile: {
      ide: "jupyterlab" | "vscode" | "rstudio" | "custom";
      image: string;
      cmd?: string[];
    };
    networking?: { host?: string };
    replicas?: number;
  };
  status?: { phase?: string; url?: string; reason?: string };
};

export type SessionEvent = { type: string; object: Session };

export type Introspection = {
  user: {
    subject: string;
    roles: string[];
    provider: string;
    exp?: number;
    iat?: number;
  };
  cluster: {
    casbin: { namespaces: { list: boolean } };
    serverServiceAccount: {
      namespaces: Record<"list" | "watch", boolean>;
      session: Record<
        "get" | "list" | "watch" | "create" | "update" | "delete" | "patch",
        boolean
      >;
    };
  };
  namespaces: {
    all?: string[];
    withSessions?: string[];
    userAllowed: string[];
  };
  domains: Record<string, { session: Record<string, boolean> }>;
  subjects?: Record<
    string,
    Record<string, { session: Record<string, boolean> }>
  >;
};
