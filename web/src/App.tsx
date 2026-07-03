import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { useCallback, useEffect, useState } from "react";
import { FolderGit2, Server, Settings, ScrollText, LogOut, Loader2, Moon, Sun } from "lucide-react";
import { useAuth } from "./auth";
import { health } from "./api";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { AuthPage } from "./pages/Auth";
import { ProjectsPage } from "./pages/Projects";
import { ProjectDetailPage } from "./pages/ProjectDetail";
import { ClusterPage } from "./pages/Cluster";
import { SettingsPage } from "./pages/Settings";
import { AuditPage } from "./pages/Audit";

type Theme = "dark" | "light";

/** Theme state persisted to localStorage; applies the `.light` class on <html>. */
function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = typeof localStorage !== "undefined" ? localStorage.getItem("kapibara-theme") : null;
    return saved === "light" ? "light" : "dark";
  });
  useEffect(() => {
    document.documentElement.classList.toggle("light", theme === "light");
    localStorage.setItem("kapibara-theme", theme);
  }, [theme]);
  const toggle = useCallback(() => setTheme((t) => (t === "dark" ? "light" : "dark")), []);
  return [theme, toggle];
}

export function App() {
  const { user, loading } = useAuth();
  const [theme, toggleTheme] = useTheme();
  return (
    <>
      <Toaster />
      {loading ? (
        <div className="grid min-h-screen place-items-center text-muted-foreground">
          <Loader2 className="size-6 animate-spin" />
        </div>
      ) : !user ? (
        <AuthPage />
      ) : (
        <div className="grid min-h-screen grid-cols-1 md:grid-cols-[240px_1fr]">
          <Sidebar theme={theme} onToggleTheme={toggleTheme} />
          <div className="min-w-0 overflow-auto px-6 py-6 md:px-8">
            <Routes>
              <Route path="/" element={<ProjectsPage />} />
              <Route path="/projects/:id" element={<ProjectDetailPage />} />
              <Route path="/cluster" element={<ClusterPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/audit" element={<AuditPage />} />
              <Route path="*" element={<Navigate to="/" />} />
            </Routes>
          </div>
        </div>
      )}
    </>
  );
}

const navItems = [
  { to: "/", label: "Projects", icon: FolderGit2, end: true },
  { to: "/cluster", label: "Cluster", icon: Server },
  { to: "/settings", label: "Settings", icon: Settings },
];

function Sidebar({ theme, onToggleTheme }: { theme: Theme; onToggleTheme: () => void }) {
  const { user, logout } = useAuth();
  const [engine, setEngine] = useState<boolean | null>(null);
  useEffect(() => {
    health()
      .then((h) => setEngine(h.engineHealthy))
      .catch(() => setEngine(false));
  }, []);
  return (
    <aside className="flex flex-col gap-1 border-r border-border bg-card px-3 py-4">
      <div className="flex items-center gap-2 px-2 pb-5 pt-1 text-lg font-bold">
        <span className="text-xl">🦫</span> Kapibara
      </div>
      <nav className="flex flex-col gap-0.5">
        {navItems.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-accent text-foreground"
                  : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
              )
            }
          >
            <Icon className="size-4" /> {label}
          </NavLink>
        ))}
        {user?.isAdmin && (
          <NavLink
            to="/audit"
            className={({ isActive }) =>
              cn(
                "flex items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-accent text-foreground"
                  : "text-muted-foreground hover:bg-accent/60 hover:text-foreground"
              )
            }
          >
            <ScrollText className="size-4" /> Audit log
          </NavLink>
        )}
      </nav>
      <div className="flex-1" />
      <div className="space-y-2 border-t border-border px-2 pt-3 text-xs text-muted-foreground">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span
              className={cn("size-2 rounded-full", engine ? "bg-success" : "bg-destructive")}
            />
            engine {engine == null ? "…" : engine ? "connected" : "down"}
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="size-7 text-muted-foreground hover:text-foreground"
            onClick={onToggleTheme}
            title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            aria-label="Toggle theme"
          >
            {theme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </Button>
        </div>
        <div className="truncate">{user?.email}</div>
        <Button variant="secondary" size="sm" className="w-full" onClick={logout}>
          <LogOut className="size-3.5" /> Logout
        </Button>
      </div>
    </aside>
  );
}
