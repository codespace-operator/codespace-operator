import { useEffect, useMemo, useRef, useState } from "react";

const USER_KEY = "co_user";
const TOKEN_KEY = "co_token"; // kept for SSE/headers fallback; cookie is primary.

function decodeJwtPayload(token: string): any | null {
  const parts = token.split(".");
  if (parts.length < 2) return null;
  const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
  try { return JSON.parse(atob(b64)); } catch { return null; }
}

function isExpired(token: string | null): boolean {
  if (!token) return true;
  const payload = decodeJwtPayload(token);
  const exp = payload?.exp;
  if (!exp) return false;
  const now = Math.floor(Date.now() / 1000);
  return now >= exp;
}

export function useAuth() {
  const [user, setUser] = useState<string | null>(null);
  const [roles, setRoles] = useState<string[]>([]);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const expiryTimer = useRef<number | null>(null);

  // Initialize from storage (token is optional because cookie is primary)
  useEffect(() => {
    const u = localStorage.getItem(USER_KEY);
    const t = localStorage.getItem(TOKEN_KEY);
    if (u && t && !isExpired(t)) {
      setUser(u);
      setToken(t);
    } else {
      localStorage.removeItem(USER_KEY);
      localStorage.removeItem(TOKEN_KEY);
    }
    setIsLoading(false);

    const onAuthRequired = () => logout();
    window.addEventListener("co:auth:required" as any, onAuthRequired);
    const onStorage = (e: StorageEvent) => {
      if (e.key === TOKEN_KEY && e.newValue === null) logout();
    };
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener("co:auth:required" as any, onAuthRequired);
    };
  }, []);

  useEffect(() => {
    if (expiryTimer.current) window.clearTimeout(expiryTimer.current);
    if (!token) return;
    const payload = decodeJwtPayload(token);
    const exp = payload?.exp;
    if (!exp) return;
    const ms = Math.max(0, exp * 1000 - Date.now());
    expiryTimer.current = window.setTimeout(() => logout(), ms) as unknown as number;
  }, [token]);

  const base = import.meta.env.VITE_API_BASE || "";

  const login = async (username: string, password: string): Promise<void> => {
    if (!username.trim() || !password.trim()) {
      throw new Error("Username and password are required");
    }
    const res = await fetch(`${base}/api/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include", // receive HttpOnly cookie
      body: JSON.stringify({ username, password }),
    });
    if (!res.ok) throw new Error((await res.text()) || "Login failed");
    const data = await res.json();
    // Keep a copy to support Authorization header and SSE fallback; cookie remains source of truth.
    localStorage.setItem(USER_KEY, data.user || username);
    if (data.token) localStorage.setItem(TOKEN_KEY, data.token);
    setUser(data.user || username);
    setRoles(Array.isArray(data.roles) ? data.roles : []);
    setToken(data.token || null);
  };

  const logout = () => {
    // Just drop local state + cookies (cookie is session-lifetime JWT; let it expire)
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem(TOKEN_KEY);
    setUser(null);
    setRoles([]);
    setToken(null);
  };

  const isAuthenticated = useMemo(() => !!(user && (token ? !isExpired(token) : true)), [user, token]);

  return {
    user,
    roles,
    token,
    login,
    logout,
    isAuthenticated,
    isLoading,
  };
}
