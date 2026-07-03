import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import { Card, Empty, ErrorBox } from "../ui";

interface Org { id: string; name: string; slug: string; }
interface Project { id: string; name: string; orcinusProject: string; description: string; }

export function ProjectsPage() {
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [orgId, setOrgId] = useState("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [name, setName] = useState("");
  const [err, setErr] = useState<string | null>(null);

  async function loadOrgs() {
    const r = await api.get<{ organizations: Org[] }>("/orgs");
    setOrgs(r.organizations || []);
    if (!orgId && r.organizations?.[0]) setOrgId(r.organizations[0].id);
  }
  async function loadProjects(oid: string) {
    if (!oid) return;
    const r = await api.get<{ projects: Project[] }>(`/orgs/${oid}/projects`);
    setProjects(r.projects || []);
  }
  useEffect(() => { loadOrgs(); }, []);
  useEffect(() => { if (orgId) loadProjects(orgId); }, [orgId]);

  async function create() {
    setErr(null);
    try {
      await api.post(`/orgs/${orgId}/projects`, { name });
      setName("");
      loadProjects(orgId);
    } catch (e) { setErr((e as Error).message); }
  }
  async function del(id: string) {
    if (!confirm("Delete project and all its cluster resources?")) return;
    await api.del(`/projects/${id}`);
    loadProjects(orgId);
  }

  return (
    <>
      <div className="topbar">
        <h1>Projects</h1>
        <div className="sp" />
        {orgs.length > 1 && (
          <select value={orgId} onChange={(e) => setOrgId(e.target.value)} style={{ width: 200 }}>
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </select>
        )}
      </div>

      <Card title="Your projects">
        <ErrorBox error={err} />
        {projects.length === 0 ? (
          <Empty text="No projects yet — create one below." />
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Orcinus project</th><th></th></tr></thead>
            <tbody>
              {projects.map((p) => (
                <tr key={p.id}>
                  <td><Link to={`/projects/${p.id}`}>{p.name}</Link></td>
                  <td className="mono muted">{p.orcinusProject}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="danger sm" onClick={() => del(p.id)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row" style={{ marginTop: "1rem" }}>
          <input placeholder="new project name" value={name} onChange={(e) => setName(e.target.value)} style={{ maxWidth: 260 }} />
          <button onClick={create} disabled={!name || !orgId}>Create project</button>
        </div>
      </Card>
    </>
  );
}
