import type { Session } from "./types";

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
    try { message = (await response.text()) || ""; } catch { /* ignore */ }
    const friendly = message || `HTTP ${response.status}: ${response.statusText}`;
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

function normalizeList<T = any>(x: any): T[] {
  if (!x) return [];
  if (Array.isArray(x)) return x as T[];
  if (Array.isArray(x.items)) return x.items as T[];
  if (x.data && Array.isArray(x.data.items)) return x.data.items as T[];
  return [];
}
function normalizeObject<T = any>(x: any): T {
  if (!x) throw new Error("empty response");
  if (x.object) return x.object as T;
  return x as T;
}

export const api = {
  async list(ns: string): Promise<Session[]> {
    const r = await apiFetch(`/api/v1/sessions?namespace=${encodeURIComponent(ns)}`);
    return normalizeList<Session>(await r.json());
  },

  async create(body: Partial<Session>): Promise<Session> {
    const r = await apiFetch(`/api/v1/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return normalizeObject<Session>(await r.json());
  },

  async remove(ns: string, name: string): Promise<void> {
    await apiFetch(`/api/v1/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`, {
      method: "DELETE",
    });
  },

  async scale(ns: string, name: string, replicas: number): Promise<Session> {
    const r = await apiFetch(
      `/api/v1/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      }
    );
    return normalizeObject<Session>(await r.json());
  },

  // SSE: prefer cookie; attach token as query only if no cookie is present
  watch(ns: string, onEvent: (ev: MessageEvent) => void): EventSource {
    const token = getToken();
    const hasCookie = document.cookie.includes("codespace_jwt=");
    const url =
      `${base}/api/v1/stream/sessions` +
      `?namespace=${encodeURIComponent(ns)}` +
      (!hasCookie && token ? `&access_token=${encodeURIComponent(token)}` : "");

    const es = new EventSource(url, { withCredentials: true as any });
    es.onmessage = onEvent;
    return es;
  },
};
