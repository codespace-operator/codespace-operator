import type { components } from "../types/api.gen";
import type { UISession, SessionDeleteResponse, Introspection } from "../types";

// OpenAPI-generated types (API wire format)
type APISession =
  components["schemas"]["github_com_codespace-operator_codespace-operator_api_v1.Session"];
type SessionCreateRequest =
  components["schemas"]["internal_server.SessionCreateRequest"];
type UserIntrospectionResponse =
  components["schemas"]["internal_server.UserIntrospectionResponse"];
type ServerIntrospectionResponse =
  components["schemas"]["internal_server.ServerIntrospectionResponse"];
type AuthFeatures = components["schemas"]["internal_server.AuthFeatures"];
type UserInfo = components["schemas"]["internal_server.UserInfo"];

const base = import.meta.env.VITE_API_BASE || "";

// --- Response handler (centralized) ---
async function handleResponse(response: Response) {
  if (!response.ok) {
    let message = "";
    try {
      const data = await response.json();
      message =
        typeof (data as any)?.error === "string"
          ? (data as any).error
          : (data as any)?.message || JSON.stringify(data);
    } catch {
      // ignore parse errors; use status text
    }
    const friendly = message || `HTTP ${response.status}: ${response.statusText}`;

    if (response.status === 401) {
      // Clear display-only user; cookie is HttpOnly and will expire server-side.
      localStorage.removeItem("co_user");
      window.dispatchEvent(new CustomEvent("co:auth:required"));
    }
    throw new Error(friendly);
  }
  return response;
}

// --- ALWAYS send credentials; rely on HttpOnly cookie for auth ---
async function apiFetch(path: string, init?: RequestInit) {
  const res = await fetch(`${base}${path}`, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.headers || {}),
      // CRITICAL: no Authorization header (no JWT in JS)
    },
    credentials: "include",
  });
  return handleResponse(res);
}

// -------- Low-level API helpers --------

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

// -------- Sessions --------

export const api = {
  async list(ns: string): Promise<UISession[]> {
    const url =
      ns === "All"
        ? `/api/v1/server/sessions?all=true`
        : `/api/v1/server/sessions?namespace=${encodeURIComponent(ns)}`;
    const r = await apiFetch(url);
    const data = await r.json();
    if (data.items) return data.items as UISession[];
    return normalizeList<UISession>(data);
  },

  async create(
    body: SessionCreateRequest | Partial<APISession> | Partial<UISession>,
  ): Promise<UISession> {
    let createRequest: SessionCreateRequest;
    if ("name" in body && !("metadata" in body)) {
      createRequest = body as SessionCreateRequest;
    } else {
      const b = body as Partial<APISession> & Partial<UISession>;
      createRequest = {
        name: b.metadata?.name || "",
        namespace: b.metadata?.namespace || "default",
        profile: (b as any).spec?.profile || { ide: "vscode", image: "" },
        auth: (b as any).spec?.auth,
        home: (b as any).spec?.home,
        scratch: (b as any).spec?.scratch,
        networking: (b as any).spec?.networking,
        replicas: (b as any).spec?.replicas,
      };
    }

    const r = await apiFetch(`/api/v1/server/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(createRequest),
    });
    return normalizeObject<APISession>(await r.json()) as unknown as UISession;
  },

  async remove(ns: string, name: string): Promise<SessionDeleteResponse> {
    const r = await apiFetch(
      `/api/v1/server/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      { method: "DELETE" },
    );
    if (r.headers.get("content-length") === "0" || r.status === 204) {
      return { status: "deleted", name, namespace: ns };
    }
    return r.json();
  },

  async scale(ns: string, name: string, replicas: number): Promise<UISession> {
    const r = await apiFetch(
      `/api/v1/server/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      },
    );
    return normalizeObject<APISession>(await r.json()) as unknown as UISession;
  },

  // Live updates via SSE — cookies only, no query token leakage
  watch(ns: string, onEvent: (ev: MessageEvent) => void): EventSource {
    const baseUrl = `${base}/api/v1/stream/sessions`;
    const query =
      ns === "All" ? `?all=true` : `?namespace=${encodeURIComponent(ns)}`;
    const url = baseUrl + query;
    const es = new EventSource(url, { withCredentials: true as any });
    es.onmessage = onEvent;
    return es;
  },
};

// -------- Public API surface used by hooks/components --------

export const userApi = {
  async getCurrentUser(): Promise<UserInfo | null> {
    const res = await apiFetch("/api/v1/me", { method: "GET" });
    return res.json();
  },
};

export const authApi = {
  async getFeatures(): Promise<AuthFeatures> {
    const r = await fetch(`${base}/auth/features`, { credentials: "include" });
    if (!r.ok) throw new Error(`Failed to get auth features: ${r.status}`);
    return r.json();
  },

  // Local / unified password login — server sets HttpOnly cookie (+ may return JSON)
  async localLogin(
    username: string,
    password: string,
  ): Promise<{ token?: string; user?: string; roles?: string[] }> {
    const res = await apiFetch("/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    // Normalize
    if (res.status === 204) return { user: username, roles: [] };
    return res.json();
  },

  async logout(): Promise<void> {
    // Keep GET here to match common server handlers
    await fetch(`${base}/auth/logout`, { method: "GET", credentials: "include" });
  },

  // Optional: server-side session echo
  async sessionInfo(): Promise<{
    subject?: string;
    username?: string;
    email?: string;
    provider?: string;
    roles?: string[];
    issuedAt?: number;
    expiresAt?: number;
  }> {
    const res = await apiFetch("/api/v1/user/session", { method: "POST" });
    return res.json();
  },
};

// -------- Introspection --------

export const introspectApi = {
  get: async (opts?: {
    discover?: boolean;
    namespaces?: string[];
    roles?: string[];
    actions?: string[];
  }): Promise<Introspection> => {
    const p = new URLSearchParams();
    if (opts?.discover) p.set("discover", "1");
    if (opts?.namespaces?.length) p.set("namespaces", opts.namespaces.join(","));
    if (opts?.roles?.length) p.set("roles", opts.roles.join(","));
    if (opts?.actions?.length) p.set("actions", opts.actions.join(","));
    const url = `${base}/api/v1/introspect?${p.toString()}`;
    const r = await fetch(url, { credentials: "include" });
    if (!r.ok) throw new Error(`introspect failed: ${r.status}`);
    return r.json();
  },

  getUser: async (opts?: {
    namespaces?: string[];
    actions?: string[];
    discover?: boolean;
  }): Promise<UserIntrospectionResponse> => {
    const p = new URLSearchParams();
    if (opts?.namespaces?.length) p.set("namespaces", opts.namespaces.join(","));
    if (opts?.actions?.length) p.set("actions", opts.actions.join(","));
    if (opts?.discover) p.set("discover", "1");
    const url = `${base}/api/v1/introspect/user?${p.toString()}`;
    const r = await fetch(url, { credentials: "include" });
    if (!r.ok) throw new Error(`user introspect failed: ${r.status}`);
    return r.json();
  },

  getServer: async (opts?: { discover?: boolean }): Promise<ServerIntrospectionResponse> => {
    const p = new URLSearchParams();
    if (opts?.discover) p.set("discover", "1");
    const url = `${base}/api/v1/introspect/server?${p.toString()}`;
    const r = await fetch(url, { credentials: "include" });
    if (!r.ok) {
      if (r.status === 403) {
        throw new Error("Insufficient permissions to view server information");
      }
      throw new Error(`server introspect failed: ${r.status}`);
    }
    return r.json();
  },
};
