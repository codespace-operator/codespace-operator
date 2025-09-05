import { useEffect, useMemo, useState } from "react";
import { authApi, userApi } from "../api/client";

const USER_KEY = "co_user";

export function useAuth() {
  const [user, setUser] = useState<string | null>(localStorage.getItem(USER_KEY));
  const [roles, setRoles] = useState<string[]>([]);
  // Keep a token slot for compatibility with callers, but never store a JWT
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setLoading] = useState(true);

  // Restore from cookie session via /api/v1/me
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const userData = await userApi.getCurrentUser();
        if (cancelled) return;

        const display =
          (userData as any)?.username ??
          (userData as any)?.subject ??
          (userData as any)?.sub ??
          null;
        const rs: string[] = Array.isArray((userData as any)?.roles)
          ? (userData as any).roles
          : [];

        setUser(display);
        setRoles(rs);
        if (display) localStorage.setItem(USER_KEY, display);

        // Never store JWTs
        setToken(null);
      } catch (error: any) {
        if (!cancelled) {
          if (
            error?.message?.includes("401") ||
            error?.message?.toLowerCase().includes("unauthorized")
          ) {
            localStorage.removeItem(USER_KEY);
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

  // Local password login (server sets cookie; may return JSON)
  async function loginPassword(username: string, password: string) {
    try {
      const data = await authApi.localLogin(username, password);
      const display = data?.user ?? username;
      if (display) localStorage.setItem(USER_KEY, display);
      setUser(display);
      setRoles(Array.isArray(data.roles) ? data.roles : []);
      setToken(null); // do not store JWT
    } catch (error: any) {
      const message = error?.message || "Authentication failed";
      if (message.includes("account locked")) {
        throw new Error("Account is locked. Contact your administrator.");
      }
      if (message.includes("password expired")) {
        throw new Error("Password has expired. Please contact your administrator.");
      }
      throw new Error(message);
    }
  }

  // OIDC/SSO login — just redirect; cookie will be set by the server
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
    setUser(null);
    setRoles([]);
    setToken(null);
  }

  const isAuthenticated = useMemo(() => !!user, [user]);

  return {
    user,
    roles,
    token, // always null in cookie-mode
    isAuthenticated,
    isLoading,
    loginPassword,
    loginSSO,
    logout,
  };
}
