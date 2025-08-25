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

  // Try to restore from cookie session (OIDC/local)
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const r = await fetch(`${base}/api/v1/me`, { credentials: "include" });
        if (!r.ok) return; // unauthenticated -> ignore
        const data = await r.json();
        if (cancelled) return;
        setUser(data.user || null);
        setRoles(Array.isArray(data.roles) ? data.roles : []);
      } catch {
        /* noop */
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Local (bootstrap) login: try /auth/local-login, then /auth/login
  async function login(username: string, password: string) {
    const body = JSON.stringify({ username, password });
    const tryPaths = ["/auth/local-login", "/auth/login"];
    let lastErr: any;
    for (const p of tryPaths) {
      try {
        const r = await fetch(`${base}${p}`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Accept: "application/json",
          },
          body,
          credentials: "include",
        });
        if (!r.ok) throw new Error((await r.text()) || "Login failed");
        const data = await r.json();
        if (data.user) localStorage.setItem(USER_KEY, data.user);
        if (data.token) localStorage.setItem(TOKEN_KEY, data.token);
        setUser(data.user || username);
        setRoles(Array.isArray(data.roles) ? data.roles : []);
        setToken(data.token || null);
        return;
      } catch (e) {
        lastErr = e;
      }
    }
    throw lastErr;
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
  return { user, roles, token, isAuthenticated, isLoading, login, logout };
}
