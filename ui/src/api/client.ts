import type { Session } from "../types";

// Small helpers to tolerate different gateway shapes
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

const base = "";

export const api = {
  async list(ns: string): Promise<Session[]> {
    const r = await fetch(`${base}/api/v1/sessions?namespace=${encodeURIComponent(ns)}`);
    if (!r.ok) throw new Error(await r.text());
    return normalizeList<Session>(await r.json());
  },

  async create(body: Partial<Session>): Promise<Session> {
    const r = await fetch(`${base}/api/v1/sessions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!r.ok) throw new Error(await r.text());
    return normalizeObject<Session>(await r.json());
  },

  async remove(ns: string, name: string): Promise<void> {
    const r = await fetch(
      `${base}/api/v1/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      { method: "DELETE" }
    );
    if (!r.ok) throw new Error(await r.text());
  },

  async scale(ns: string, name: string, replicas: number): Promise<Session> {
    const r = await fetch(
      `${base}/api/v1/sessions/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/scale`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      }
    );
    if (!r.ok) throw new Error(await r.text());
    return normalizeObject<Session>(await r.json());
  },

  watch(ns: string, onEvent: (ev: MessageEvent) => void): EventSource {
    const es = new EventSource(`${base}/api/v1/stream/sessions?namespace=${encodeURIComponent(ns)}`);
    es.onmessage = onEvent;
    return es;
  },
};
