import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";
import { health } from "./api";
import { AuthPage } from "./pages/Auth";
import { ProjectsPage } from "./pages/Projects";
import { ProjectDetailPage } from "./pages/ProjectDetail";
import { ClusterPage } from "./pages/Cluster";
import { SettingsPage } from "./pages/Settings";
import { AuditPage } from "./pages/Audit";

export function App() {
  const { user, loading } = useAuth();
  if (loading) return <div className="center muted">loading…</div>;
  if (!user) return <AuthPage />;
  return (
    <div className="layout">
      <Sidebar />
      <div className="content">
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
  );
}

function Sidebar() {
  const { user, logout } = useAuth();
  const [engine, setEngine] = useState<boolean | null>(null);
  useEffect(() => {
    health().then((h) => setEngine(h.engineHealthy));
  }, []);
  return (
    <div className="sidebar">
      <div className="brand">🦫 Kapibara</div>
      <nav>
        <NavLink to="/" end>Projects</NavLink>
        <NavLink to="/cluster">Cluster</NavLink>
        <NavLink to="/settings">Settings</NavLink>
        {user?.isAdmin && <NavLink to="/audit">Audit log</NavLink>}
      </nav>
      <div className="spacer" />
      <div className="foot">
        <div className="row" style={{ gap: ".4rem" }}>
          <span className="badge-dot" style={{ background: engine ? "var(--ok)" : "var(--err)" }} />
          engine {engine ? "connected" : "down"}
        </div>
        <div style={{ marginTop: ".4rem" }}>{user?.email}</div>
        <button className="sec sm" style={{ marginTop: ".5rem", width: "100%" }} onClick={logout}>
          Logout
        </button>
      </div>
    </div>
  );
}
