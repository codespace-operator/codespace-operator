import { useEffect, useMemo, useState } from "react";
import { authApi, userApi } from "../api/client";

const USER_KEY = "co_user";
const TOKEN_KEY = "co_token";

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

  // Try to restore from cookie session (OIDC/local) via /api/v1/me
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        // Use the new userApi instead of direct introspect call
        const userData = await userApi.getCurrentUser();
        if (cancelled) return;

        const display = userData?.username ?? userData?.subject ?? null;
        const rs: string[] = Array.isArray(userData?.roles)
          ? userData.roles
          : [];

        setUser(display);
        setRoles(rs);

        // Update localStorage if we got valid user data
        if (display) {
          localStorage.setItem(USER_KEY, display);
        }
      } catch (error: any) {
        // Treat like unauth
        if (!cancelled) {
          if (
            error?.message?.includes("401") ||
            error?.message?.includes("unauthorized")
          ) {
            localStorage.removeItem(USER_KEY);
            localStorage.removeItem(TOKEN_KEY);
          }
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
    try {
      const data = await authApi.localLogin(username, password);

      const display = data?.user ?? username;
      if (display) localStorage.setItem(USER_KEY, display);
      if (data.token) localStorage.setItem(TOKEN_KEY, data.token);

      setUser(display);
      setRoles(Array.isArray(data.roles) ? data.roles : []);
      setToken(data.token || null);
    } catch (error: any) {
      throw new Error(error?.message || "Local login failed");
    }
  }

  // SSO login - redirect to the SSO endpoint
  function loginSSO(next?: string) {
    const params = new URLSearchParams();
    if (next) params.set("next", next);
    const base = import.meta.env.VITE_API_BASE || "";
    window.location.href = `${base}/auth/sso/login${params.toString() ? "?" + params.toString() : ""}`;
  }

  async function logout() {
    try {
      await authApi.logout();
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
