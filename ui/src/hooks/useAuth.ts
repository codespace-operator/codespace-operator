// src/hooks/useAuth.ts
import { useEffect, useMemo, useRef, useState } from "react";

const USER_KEY = "co_user";
const TOKEN_KEY = "co_token";

// Minimal JWT decoder (payload only). Works with both real JWT and the demo token.
function decodeJwtPayload(token: string): any | null {
  const parts = token.split(".");
  if (parts.length < 2) return null;
  const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
  try {
    const json = atob(b64);
    return JSON.parse(json);
  } catch {
    return null;
  }
}

function isExpired(token: string | null): boolean {
  if (!token) return true;
  const payload = decodeJwtPayload(token);
  const exp = payload?.exp;
  if (!exp) return false; // if no exp, treat as non-expiring (demo token)
  const now = Math.floor(Date.now() / 1000);
  return now >= exp;
}

export function useAuth() {
  const [user, setUser] = useState<string | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const expiryTimer = useRef<number | null>(null);

  // Initialize from storage
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

    // React to global "auth required" (fired by api client on 401)
    const onAuthRequired = () => logout();
    window.addEventListener("co:auth:required" as any, onAuthRequired);
    // Multi-tab sign out
    const onStorage = (e: StorageEvent) => {
      if (e.key === TOKEN_KEY && e.newValue === null) logout();
    };
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener("co:auth:required" as any, onAuthRequired);
    };
  }, []);

  // Set (or clear) a timer to auto-logout at token expiry
  useEffect(() => {
    if (expiryTimer.current) window.clearTimeout(expiryTimer.current);
    if (!token) return;

    const payload = decodeJwtPayload(token);
    const exp = payload?.exp;
    if (!exp) return; // demo token: no timer

    const ms = Math.max(0, exp * 1000 - Date.now());
    expiryTimer.current = window.setTimeout(() => logout(), ms) as unknown as number;
  }, [token]);

  const login = async (username: string, password: string): Promise<void> => {
    if (!username.trim() || !password.trim()) {
      throw new Error("Username and password are required");
    }

    // If you have a real backend login, do it here. Example:
    // const res = await fetch(`${import.meta.env.VITE_API_BASE || ""}/api/v1/auth/login`, {
    //   method: "POST",
    //   headers: { "Content-Type": "application/json" },
    //   body: JSON.stringify({ username, password }),
    //   credentials: "include",
    // });
    // if (!res.ok) throw new Error(await res.text());
    // const { token } = await res.json();

    // Demo fallback: accept any credentials, 24h exp
    await new Promise(r => setTimeout(r, 300));
    const tokenPayload = {
      sub: username,
      iat: Math.floor(Date.now() / 1000),
      exp: Math.floor(Date.now() / 1000) + 24 * 60 * 60,
    };
    const token = `demo.${btoa(JSON.stringify(tokenPayload))}.signature`;

    localStorage.setItem(USER_KEY, username);
    localStorage.setItem(TOKEN_KEY, token);
    setUser(username);
    setToken(token);
  };

  const logout = () => {
    localStorage.removeItem(USER_KEY);
    localStorage.removeItem(TOKEN_KEY);
    setUser(null);
    setToken(null);
  };

  const isAuthenticated = useMemo(() => !!token && !isExpired(token), [token]);

  return {
    user,
    token,
    login,
    logout,
    isAuthenticated,
    isLoading,
  };
}
