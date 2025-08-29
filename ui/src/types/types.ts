/**
 * UI VIEW-MODEL TYPES
 * -----------------------------------------------------------------------------
 * These are *UI-only* shapes (what components/hooks need to render).
 * @deprecated Do NOT mirror API contracts here. For API types, import from
 * ./types/api.gen.ts (or temporarily via ./api-types shim).
 * Over time, keep this file limited strictly to UI view models.
 */

// ---- Loose server delete response (server returns generic map) ---------------
/**
 * The DELETE /server/sessions/{ns}/{name} endpoint may return a generic map.
 * Keep a narrow, UI-friendly shape for callers that rely on it.
 */
export interface SessionDeleteResponse {
  status: string;
  name: string;
  namespace: string;
}

// Shared types that mirror the Session CRD (fields the UI needs)
export type UISession = {
  kind: "Session";
  apiVersion: "codespace.codespace.dev/v1";
  metadata: {
    name: string;
    namespace: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
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

export type SessionEvent = { type: string; object: UISession };

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
