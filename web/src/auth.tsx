import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { api, getToken, setToken, clearToken } from "./api";

export interface User {
  id: string;
  email: string;
  name: string;
  isAdmin: boolean;
  twoFAEnabled: boolean;
}

interface AuthCtx {
  user: User | null;
  loading: boolean;
  login: (email: string, password: string, totpCode?: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  logout: () => void;
  refresh: () => Promise<void>;
}

const Ctx = createContext<AuthCtx>(null as any);
export const useAuth = () => useContext(Ctx);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  async function refresh() {
    if (!getToken()) {
      setUser(null);
      setLoading(false);
      return;
    }
    try {
      const u = await api.get<User>("/auth/me");
      setUser(u);
    } catch {
      clearToken();
      setUser(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function login(email: string, password: string, totpCode?: string) {
    const r = await api.post<{ token: string; user: User }>("/auth/login", { email, password, totpCode });
    setToken(r.token);
    setUser(r.user);
  }
  async function register(email: string, password: string, name: string) {
    const r = await api.post<{ token: string; user: User }>("/auth/register", { email, password, name });
    setToken(r.token);
    setUser(r.user);
  }
  function logout() {
    clearToken();
    setUser(null);
  }

  return (
    <Ctx.Provider value={{ user, loading, login, register, logout, refresh }}>
      {children}
    </Ctx.Provider>
  );
}
