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

// User-specific introspection response
export type UserIntrospection = {
  user: {
    subject: string;
    username: string;
    email?: string;
    roles: string[];
    provider: string;
    exp?: number;
    iat?: number;
    implicitRoles?: string[];
  };
  domains: Record<
    string,
    {
      session: Record<
        "get" | "list" | "watch" | "create" | "update" | "delete" | "scale",
        boolean
      >;
    }
  >;
  namespaces: {
    userAllowed: string[];
    userCreatable?: string[];
    userDeletable?: string[];
  };
  capabilities: {
    namespaceScope: string[];
    clusterScope: boolean;
    adminAccess: boolean;
  };
};

// Server-specific introspection response
export type ServerIntrospection = {
  cluster: {
    casbin: {
      namespaces: {
        list: boolean;
        watch: boolean;
      };
    };
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
  };
  capabilities: {
    multiTenant: boolean;
  };
  version?: {
    version?: string;
    gitCommit?: string;
    buildDate?: string;
  };
};

// Legacy combined introspection (deprecated)
export type Introspection = {
  user: UserIntrospection["user"];
  cluster?: ServerIntrospection["cluster"];
  domains: UserIntrospection["domains"];
  namespaces: {
    all?: string[];
    withSessions?: string[];
    userAllowed: string[];
    userCreatable?: string[];
    userDeletable?: string[];
  };
  capabilities: {
    namespaceScope: string[];
    clusterScope: boolean;
    adminAccess: boolean;
    multiTenant: boolean;
  };
  subjects?: Record<
    string,
    Record<
      string,
      {
        session: Record<
          "get" | "list" | "watch" | "create" | "update" | "delete" | "scale",
          boolean
        >;
      }
    >
  >;
};
