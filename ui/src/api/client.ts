import type {
  Session,
  SessionListResponse,
  SessionCreateRequest,
  SessionDeleteResponse,
  UserIntrospectionResponse,
  ServerIntrospectionResponse,
  LegacyIntrospectionResponse,
  AuthFeatures,
  UserInfo,
} from "../api-types";

const base = import.meta.env.VITE_API_BASE || "";
const TOKEN_KEY = "co_token";

function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

function authHeaders() {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function handleResponse(response: Response) {
  if (!response.ok) {
    let message = "";
    try {
      message = (await response.text()) || "";
    } catch {
      /* ignore */
    }
    const friendly =
      message || `HTTP ${response.status}: ${response.statusText}`;
    if (response.status === 401) {
      localStorage.removeItem("co_user");
      localStorage.removeItem("co_token");
      window.dispatchEvent(new CustomEvent("co:auth:required"));
    }
    throw new Error(friendly);
  }
  return response;
}

async function apiFetch(path: string, init?: RequestInit) {
  const res = await fetch(`${base}${path}`, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.headers || {}),
      ...authHeaders(),
    },
    credentials: "include", // prefer secure cookie for same-origin
  });
  await handleResponse(res);
  return res;
}

// Helper functions - keep your existing logic
function normalizeList<T = unknown>(x: any): T[] {
  if (!x) return [];
  if (Array.isArray(x)) return x as T[];
  if (Array.isArray(x.items)) return x.items as T[];
  if (x.data && Array.isArray(x.data.items)) return x.data.items as T[];
  return [];
}

function normalizeObject<T = unknown>(x: any): T {
  if (!x) throw new Error("empty response");
  if (x.object) return x.object as T;
  return x as T;
}

export const api = {
  async list(ns: string): Promise<Session[]> {
    // For the UI's "All", ask the server for everything
    const url =
      ns === "All"
        ? `/api/v1/server/sessions?all=true`
        : `/api/v1/server/sessions?namespace=${encodeURIComponent(ns)}`;
    const r = await apiFetch(url);
    const data = await r.json();

    // Handle both direct arrays and wrapped responses
    if (data.items) {
      return data.items as Session[];
    }
    return normalizeList<Session>(data);
  },

  async create(body: Partial<Session>): Promise<Session> {
    // Convert your existing Session format to the API request format
    const createRequest: SessionCreateRequest = {
      name: body.metadata?.name || "",
      namespace: body.metadata?.namespace || "default",
      profile: body.spec?.profile || { ide: "vscode", image: "" },
      auth: body.spec?.auth,
      home: body.spec?.home,
      scratch: body.spec?.scratch,
      networking: body.spec?.networking,
      replicas: body.spec?.replicas,
    };

    const r = await apiFetch(`/api/v1/server/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(createRequest),
    });
    return normalizeObject<Session>(await r.json());
  },

  async remove(ns: string, name: string): Promise<SessionDeleteResponse> {
    const r = await apiFetch(
      `/api/v1/server/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      {
        method: "DELETE",
      },
    );

    // Handle both void and response body
    if (r.headers.get("content-length") === "0" || r.status === 204) {
      return { status: "deleted", name, namespace: ns };
    }
    return await r.json();
  },

  async scale(ns: string, name: string, replicas: number): Promise<Session> {
    const r = await apiFetch(
      `/api/v1/server/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      },
    );
    return normalizeObject<Session>(await r.json());
  },

  // Server-Sent Events stream of Session updates - keep existing logic
  watch(ns: string, onEvent: (ev: MessageEvent) => void): EventSource {
    const token = getToken();

    // Prefer cookie session; only fall back to access_token param when there is no session cookie.
    const hasCookie =
      document.cookie.includes("codespace_session=") ||
      document.cookie.includes("codespace_jwt="); // back-compat

    const baseUrl = `${base}/api/v1/stream/sessions`;
    const query =
      ns === "All" ? `?all=true` : `?namespace=${encodeURIComponent(ns)}`;
    const tokenParam =
      !hasCookie && token ? `&access_token=${encodeURIComponent(token)}` : "";

    const url = baseUrl + query + tokenParam;
    const es = new EventSource(url, { withCredentials: true as any });
    es.onmessage = onEvent;
    return es;
  },
};

export const authApi = {
  async getFeatures(): Promise<AuthFeatures> {
    const r = await fetch(`${base}/auth/features`, { credentials: "include" });
    if (!r.ok) throw new Error(`Failed to get auth features: ${r.status}`);
    return r.json();
  },

  async localLogin(
    username: string,
    password: string,
  ): Promise<{ token: string; user: string; roles: string[] }> {
    const r = await fetch(`${base}/auth/local/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
      credentials: "include",
    });
    if (!r.ok) {
      const error = await r.text();
      throw new Error(error || "Login failed");
    }
    return r.json();
  },

  async logout(): Promise<void> {
    await fetch(`${base}/auth/logout`, {
      method: "POST",
      credentials: "include",
    });
  },
};

export const userApi = {
  async getCurrentUser(): Promise<UserInfo> {
    const r = await apiFetch("/api/v1/me");
    return r.json();
  },
};

export const introspectApi = {
  /**
   * Legacy combined endpoint (kept for back-compat with useIntrospection)
   */
  get: async (opts?: {
    discover?: boolean;
    namespaces?: string[];
    roles?: string[];
    actions?: string[];
  }): Promise<LegacyIntrospectionResponse> => {
    const p = new URLSearchParams();
    if (opts?.discover) p.set("discover", "1");
    if (opts?.namespaces?.length)
      p.set("namespaces", opts.namespaces.join(","));
    if (opts?.roles?.length) p.set("roles", opts.roles.join(","));
    if (opts?.actions?.length) p.set("actions", opts.actions.join(","));

    const url = `${base}/api/v1/introspect?${p.toString()}`;
    const r = await fetch(url, { credentials: "include" });
    if (!r.ok) throw new Error(`introspect failed: ${r.status}`);
    return r.json();
  },

  /**
   * User-scoped introspection (permissions, roles, allowed namespaces)
   */
  getUser: async (opts?: {
    namespaces?: string[];
    actions?: string[];
    discover?: boolean;
  }): Promise<UserIntrospectionResponse> => {
    const p = new URLSearchParams();
    if (opts?.namespaces?.length)
      p.set("namespaces", opts.namespaces.join(","));
    if (opts?.actions?.length) p.set("actions", opts.actions.join(","));
    if (opts?.discover) p.set("discover", "1");

    const url = `${base}/api/v1/introspect/user?${p.toString()}`;
    const r = await fetch(url, { credentials: "include" });
    if (!r.ok) throw new Error(`user introspect failed: ${r.status}`);
    return r.json();
  },

  /**
   * Server/cluster-scoped introspection (may require elevated permissions)
   */
  getServer: async (opts?: {
    discover?: boolean;
  }): Promise<ServerIntrospectionResponse> => {
    const p = new URLSearchParams();
    if (opts?.discover) p.set("discover", "1");

    const url = `${base}/api/v1/introspect/server?${p.toString()}`;
    const r = await fetch(url, { credentials: "include" });

    if (!r.ok) {
      if (r.status === 403) {
        // bubble a friendly error that callers can optionally suppress
        throw new Error("Insufficient permissions to view server information");
      }
      throw new Error(`server introspect failed: ${r.status}`);
    }
    return r.json();
  },
};
