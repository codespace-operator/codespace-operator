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

export async function getMe(): Promise<{
  user: string;
  roles: string[];
  provider: string;
  exp: number;
  iat: number;
  email?: string;
}> {
  return apiFetch("/api/v1/me");
}

export async function introspect(
  namespaces: string[],
): Promise<IntrospectResponse> {
  const qs =
    namespaces && namespaces.length > 0
      ? "?namespaces=" + encodeURIComponent(namespaces.join(","))
      : "";
  return apiFetch(`/api/v1/rbac/introspect${qs}`);
}
