export type IntrospectResponse = {
  subject: string;
  roles: string[];
  cluster: { namespaces: { list: boolean } };
  domains: Record<
    string, // namespace or "*"
    {
      session: {
        get: boolean;
        list: boolean;
        watch: boolean;
        create: boolean;
        update: boolean;
        delete: boolean;
        scale: boolean;
      };
    }
  >;
};

async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: "include", // send session cookie
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

export type Me = {
  user: string | null;
  roles: string[];
  provider?: string;
  email?: string; // <-- add this
  exp?: number;
  iat?: number;
};

// DEPRECATED: prefer introspectApi from api/client.ts

export async function getMe() {
  const r = await fetch("/api/v1/introspect", { credentials: "include" });
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  const j = await r.json();
  return {
    user: j?.user?.subject,
    roles: j?.user?.roles || [],
    provider: j?.user?.provider,
    email: j?.user?.email,
    exp: j?.user?.exp,
    iat: j?.user?.iat,
  };
}

export async function introspect(namespaces: string[]) {
  const p = new URLSearchParams();
  if (namespaces?.length) p.set("namespaces", namespaces.join(","));
  const r = await fetch(`/api/v1/introspect?${p.toString()}`, {
    credentials: "include",
  });
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  return r.json();
}
