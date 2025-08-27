import { useEffect, useMemo, useState } from "react";

const base = import.meta.env.VITE_API_BASE || "";
const USER_KEY = "co_user";
const TOKEN_KEY = "co_token"; // kept for SSE/headers fallback; cookie is primary.

function decodeJwtPayload(token: string): any | null {
  const parts = token.split(".");
  if (parts.length < 2) return null;
  const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
  try {
    return JSON.parse(atob(b64));
  } catch {
    return null;
  }
}

function isExpired(token: string | null): boolean {
  if (!token) return true;
  const payload = decodeJwtPayload(token);
  const exp = payload?.exp;
  if (!exp) return false;
  const now = Math.floor(Date.now() / 1000);
  return now >= exp;
}

function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function useAuth() {
  const [user, setUser] = useState<string | null>(
    localStorage.getItem(USER_KEY),
  );
  const [roles, setRoles] = useState<string[]>([]);
  const [token, setToken] = useState<string | null>(getToken());
  const [isLoading, setLoading] = useState(true);

  // Try to restore from cookie session (OIDC/local) via /introspect
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await fetch("/api/v1/introspect", {
          credentials: "include",
        });
        if (resp.status === 401) {
          // Not authenticated. Clear local state and finish loading.
          if (!cancelled) {
            localStorage.removeItem(USER_KEY);
            localStorage.removeItem(TOKEN_KEY);
            setUser(null);
            setRoles([]);
            setToken(null);
          }
          return;
        }
        const data = await resp.json();
        if (cancelled) return;
        const display = data?.user?.username ?? data?.user?.subject ?? null;
        const rs: string[] = Array.isArray(data?.user?.roles)
          ? data.user.roles
          : [];
        setUser(display);
        setRoles(rs);
        // Optionally set token if backend returns one
      } catch {
        // Treat like unauth
        if (!cancelled) {
          setUser(null);
          setRoles([]);
          setToken(null);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Local login using the dedicated endpoint
  async function loginLocal(username: string, password: string) {
    const body = JSON.stringify({ username, password });
    const r = await fetch(`${base}/auth/local/login`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body,
      credentials: "include",
    });

    if (!r.ok) {
      const errorText = await r.text();
      throw new Error(errorText || "Local login failed");
    }

    const data = await r.json();
    const display = data?.username ?? data?.user ?? username;
    if (display) localStorage.setItem(USER_KEY, display);
    if (data.token) localStorage.setItem(TOKEN_KEY, data.token);
    setUser(display);
    setRoles(Array.isArray(data.roles) ? data.roles : []);
    setToken(data.token || null);
  }

  // SSO login - redirect to the SSO endpoint
  function loginSSO(next?: string) {
    const params = new URLSearchParams();
    if (next) params.set("next", next);
    window.location.href = `${base}/auth/sso/login${params.toString() ? "?" + params.toString() : ""}`;
  }

  async function logout() {
    try {
      await fetch(`${base}/auth/logout`, {
        method: "POST",
        credentials: "include",
      });
    } catch {}
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem(TOKEN_KEY);
    setUser(null);
    setRoles([]);
    setToken(null);
  }

  const isAuthenticated = useMemo(() => !!user, [user]);

  return {
    user,
    roles,
    token,
    isAuthenticated,
    isLoading,
    loginLocal,
    loginSSO,
    logout,
  };
}
