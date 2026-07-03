import { useEffect, useState } from "react";
import { NavLink, useParams, useSearchParams } from "react-router-dom";
import { api, getText } from "../api";
import { Card, Empty, ErrorBox, StatusPill } from "../ui";

interface Project { id: string; name: string; orcinusProject: string; }

const TABS = ["overview", "applications", "databases", "compose", "deployments", "logs", "templates", "backups"];

export function ProjectDetailPage() {
  const { id = "" } = useParams();
  const [sp, setSp] = useSearchParams();
  const tab = sp.get("tab") || "overview";
  const [project, setProject] = useState<Project | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api.get<Project>(`/projects/${id}`).then(setProject).catch((e) => setErr(e.message));
  }, [id]);

  if (err) return <ErrorBox error={err} />;
  if (!project) return <div className="muted">loading…</div>;

  return (
    <>
      <div className="topbar">
        <h1>{project.name}</h1>
        <span className="mono muted">{project.orcinusProject}</span>
      </div>
      <div className="tabs">
        {TABS.map((t) => (
          <NavLink key={t} to={`?tab=${t}`} className={tab === t ? "active" : ""} onClick={(e) => { e.preventDefault(); setSp({ tab: t }); }}>
            {t[0].toUpperCase() + t.slice(1)}
          </NavLink>
        ))}
      </div>
      {tab === "overview" && <Overview projectId={id} />}
      {tab === "applications" && <Applications projectId={id} />}
      {tab === "databases" && <Databases projectId={id} />}
      {tab === "compose" && <Compose projectId={id} />}
      {tab === "deployments" && <Deployments projectId={id} />}
      {tab === "logs" && <Logs projectId={id} />}
      {tab === "templates" && <Templates projectId={id} />}
      {tab === "backups" && <Backups projectId={id} />}
    </>
  );
}

function Overview({ projectId }: { projectId: string }) {
  const [pods, setPods] = useState<any[]>([]);
  const [metrics, setMetrics] = useState<any[]>([]);
  useEffect(() => {
    api.get(`/projects/${projectId}/pods`).then((r) => setPods(r.pods || [])).catch(() => {});
    api.get(`/projects/${projectId}/metrics`).then((r) => setMetrics(r.metrics || [])).catch(() => {});
  }, [projectId]);
  const metricFor = (pod: string) => metrics.find((m) => m.pod === pod);
  return (
    <Card title="Pods">
      {pods.length === 0 ? <Empty text="No running pods." /> : (
        <table>
          <thead><tr><th>Service</th><th>Pod</th><th>Status</th><th>Ready</th><th>CPU</th><th>Memory</th></tr></thead>
          <tbody>
            {pods.map((p) => {
              const m = metricFor(p.name);
              return (
                <tr key={p.name}>
                  <td>{p.service}</td><td className="mono">{p.name}</td>
                  <td><StatusPill status={p.status} /></td><td>{p.ready}</td>
                  <td>{m ? `${m.cpuMillicores}m` : "—"}</td>
                  <td>{m ? `${Math.round(m.memoryBytes / 1048576)}Mi` : "—"}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </Card>
  );
}

function Applications({ projectId }: { projectId: string }) {
  const [apps, setApps] = useState<any[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState("");
  const [form, setForm] = useState<any>({ name: "", buildType: "image", image: "", repoUrl: "", branch: "main", port: 80, domain: "", tls: false });

  async function load() {
    const r = await api.get(`/projects/${projectId}/apps`);
    setApps(r.applications || []);
  }
  useEffect(() => { load(); }, [projectId]);

  async function create() {
    setErr(null);
    try { await api.post(`/projects/${projectId}/apps`, { ...form, port: Number(form.port) }); setForm({ ...form, name: "" }); load(); }
    catch (e) { setErr((e as Error).message); }
  }
  async function deploy(a: any) {
    setBusy(a.id);
    try { await api.post(`/apps/${a.id}/deploy`); alert("Deploy started for " + a.name); } finally { setBusy(""); }
  }
  async function scale(a: any) {
    const n = prompt(`Scale ${a.name} to how many replicas?`, "2");
    if (!n) return;
    await api.post(`/projects/${projectId}/services/${slug(a.name)}/scale`, { replicas: Number(n) });
    alert("Scaled");
  }
  async function rollback(a: any) {
    await api.post(`/projects/${projectId}/services/${slug(a.name)}/rollback`).then(() => alert("Rolled back")).catch((e) => alert(e.message));
  }
  async function del(a: any) { if (confirm("Delete app?")) { await api.del(`/apps/${a.id}`); load(); } }

  return (
    <>
      <Card title="Applications">
        {apps.length === 0 ? <Empty text="No applications yet." /> : (
          <table>
            <thead><tr><th>Name</th><th>Source</th><th>Domain</th><th>Image</th><th></th></tr></thead>
            <tbody>
              {apps.map((a) => (
                <tr key={a.id}>
                  <td>{a.name}</td>
                  <td className="muted">{a.buildType}{a.repoUrl ? ` · ${a.repoUrl}` : ""}</td>
                  <td>{a.domain || "—"}</td>
                  <td className="mono muted">{(a.currentImage || a.image || "").slice(0, 30) || "—"}</td>
                  <td style={{ textAlign: "right" }}>
                    <div className="row" style={{ justifyContent: "flex-end" }}>
                      <button className="sm" disabled={busy === a.id} onClick={() => deploy(a)}>Deploy</button>
                      <button className="sm sec" onClick={() => scale(a)}>Scale</button>
                      <button className="sm sec" onClick={() => rollback(a)}>Rollback</button>
                      <button className="sm danger" onClick={() => del(a)}>✕</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card title="New application">
        <ErrorBox error={err} />
        <div className="grid2">
          <div>
            <label>Name</label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <label>Build type</label>
            <select value={form.buildType} onChange={(e) => setForm({ ...form, buildType: e.target.value })}>
              <option value="image">Prebuilt image</option>
              <option value="dockerfile">Dockerfile (git)</option>
              <option value="nixpacks">Nixpacks (git)</option>
            </select>
            {form.buildType === "image" ? (
              <>
                <label>Image</label>
                <input value={form.image} onChange={(e) => setForm({ ...form, image: e.target.value })} placeholder="nginx:alpine" />
              </>
            ) : (
              <>
                <label>Repo URL</label>
                <input value={form.repoUrl} onChange={(e) => setForm({ ...form, repoUrl: e.target.value })} placeholder="https://github.com/user/repo" />
                <label>Branch</label>
                <input value={form.branch} onChange={(e) => setForm({ ...form, branch: e.target.value })} />
              </>
            )}
          </div>
          <div>
            <label>Container port</label>
            <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} />
            <label>Domain (optional → ingress)</label>
            <input value={form.domain} onChange={(e) => setForm({ ...form, domain: e.target.value })} placeholder="app.example.com" />
            <label className="row" style={{ marginTop: ".8rem" }}>
              <input type="checkbox" style={{ width: "auto" }} checked={form.tls} onChange={(e) => setForm({ ...form, tls: e.target.checked })} />
              <span>Enable TLS (cert-manager / ACME)</span>
            </label>
            <button style={{ marginTop: "1rem" }} onClick={create} disabled={!form.name}>Create application</button>
          </div>
        </div>
      </Card>
    </>
  );
}

function Databases({ projectId }: { projectId: string }) {
  const [dbs, setDbs] = useState<any[]>([]);
  const [form, setForm] = useState({ name: "", engine: "postgres", dbName: "app", username: "kapibara", volumeSize: "1Gi" });
  const [err, setErr] = useState<string | null>(null);
  async function load() { const r = await api.get(`/projects/${projectId}/databases`); setDbs(r.databases || []); }
  useEffect(() => { load(); }, [projectId]);
  async function create() {
    setErr(null);
    try { const d = await api.post(`/projects/${projectId}/databases`, form); await api.post(`/databases/${d.id}/deploy`); setForm({ ...form, name: "" }); load(); }
    catch (e) { setErr((e as Error).message); }
  }
  async function del(d: any) { if (confirm("Delete database?")) { await api.del(`/databases/${d.id}`); load(); } }
  return (
    <>
      <Card title="Databases">
        {dbs.length === 0 ? <Empty text="No databases yet." /> : (
          <table>
            <thead><tr><th>Name</th><th>Engine</th><th>Connection string</th><th></th></tr></thead>
            <tbody>
              {dbs.map((d) => (
                <tr key={d.id}>
                  <td>{d.name}</td><td>{d.engine}</td>
                  <td className="mono muted">{d.connectionString}</td>
                  <td style={{ textAlign: "right" }}><button className="sm danger" onClick={() => del(d)}>✕</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card title="New database (1-click)">
        <ErrorBox error={err} />
        <div className="grid2">
          <div>
            <label>Name</label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <label>Engine</label>
            <select value={form.engine} onChange={(e) => setForm({ ...form, engine: e.target.value })}>
              {["postgres", "mysql", "mariadb", "mongo", "redis"].map((x) => <option key={x}>{x}</option>)}
            </select>
          </div>
          <div>
            <label>Database name</label>
            <input value={form.dbName} onChange={(e) => setForm({ ...form, dbName: e.target.value })} />
            <label>Volume size</label>
            <input value={form.volumeSize} onChange={(e) => setForm({ ...form, volumeSize: e.target.value })} />
            <button style={{ marginTop: "1rem" }} onClick={create} disabled={!form.name}>Provision + Deploy</button>
          </div>
        </div>
      </Card>
    </>
  );
}

function Compose({ projectId }: { projectId: string }) {
  const [source, setSource] = useState("services:\n  web:\n    image: nginx:alpine\n    ports: [\"80\"]\n");
  const [out, setOut] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  async function deploy() {
    setBusy(true); setErr(null); setOut(null);
    try { const r = await api.post(`/projects/${projectId}/deploy`, { source, wait: true }); setOut(`Applied ${r.applied} objects.`); }
    catch (e) { setErr((e as Error).message); } finally { setBusy(false); }
  }
  async function preview() {
    setErr(null);
    try { const r = await api.post(`/projects/${projectId}/convert`, { source }); setOut(r.manifests); }
    catch (e) { setErr((e as Error).message); }
  }
  return (
    <Card title="Deploy Docker Compose">
      <ErrorBox error={err} />
      <textarea value={source} onChange={(e) => setSource(e.target.value)} style={{ minHeight: 240 }} />
      <div className="row" style={{ marginTop: ".6rem" }}>
        <button onClick={deploy} disabled={busy}>{busy ? "Deploying…" : "Deploy"}</button>
        <button className="sec" onClick={preview}>Preview manifests</button>
      </div>
      {out && <pre style={{ marginTop: ".8rem" }}>{out}</pre>}
    </Card>
  );
}

function Deployments({ projectId }: { projectId: string }) {
  const [deps, setDeps] = useState<any[]>([]);
  useEffect(() => { api.get(`/projects/${projectId}/deployments`).then((r) => setDeps(r.deployments || [])); }, [projectId]);
  return (
    <Card title="Deployment history">
      {deps.length === 0 ? <Empty text="No deployments yet." /> : (
        <table>
          <thead><tr><th>Kind</th><th>Status</th><th>Image</th><th>Commit</th><th>Applied</th></tr></thead>
          <tbody>
            {deps.map((d) => (
              <tr key={d.id}>
                <td>{d.kind}</td><td><StatusPill status={d.status} /></td>
                <td className="mono muted">{(d.imageRef || "").slice(0, 34)}</td>
                <td className="mono muted">{(d.commitSha || "").slice(0, 8)}</td>
                <td>{d.applied || 0}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}

function Logs({ projectId }: { projectId: string }) {
  const [service, setService] = useState("");
  const [text, setText] = useState("");
  async function load() {
    setText("loading…");
    const t = await getText(`/projects/${projectId}/logs?service=${encodeURIComponent(service)}&tail=200`);
    setText(t);
  }
  return (
    <Card title="Logs">
      <div className="row" style={{ marginBottom: ".6rem" }}>
        <input placeholder="service name (blank = first pod)" value={service} onChange={(e) => setService(e.target.value)} style={{ maxWidth: 260 }} />
        <button onClick={load}>Fetch logs</button>
      </div>
      <pre>{text || "Pick a service and fetch logs."}</pre>
    </Card>
  );
}

function Templates({ projectId }: { projectId: string }) {
  const [tpls, setTpls] = useState<any[]>([]);
  const [values, setValues] = useState<Record<string, Record<string, string>>>({});
  const [msg, setMsg] = useState<string | null>(null);
  useEffect(() => { api.get("/templates").then((r) => setTpls(r.templates || [])); }, []);
  async function deploy(t: any) {
    setMsg(null);
    try { const r = await api.post(`/projects/${projectId}/templates/${t.name}`, { values: values[t.name] || {} }); setMsg(`Deployed ${t.title}: applied ${r.applied} objects.`); }
    catch (e) { setMsg((e as Error).message); }
  }
  return (
    <>
      {msg && <div className="card">{msg}</div>}
      <div className="grid2">
        {tpls.map((t) => (
          <Card key={t.name} title={t.title}>
            <p className="muted" style={{ marginTop: 0 }}>{t.description}</p>
            {(t.params || []).map((p: any) => (
              <div key={p.key}>
                <label>{p.label || p.key}{p.required ? " *" : ""}</label>
                <input
                  placeholder={p.default}
                  onChange={(e) => setValues({ ...values, [t.name]: { ...(values[t.name] || {}), [p.key]: e.target.value } })}
                />
              </div>
            ))}
            <button style={{ marginTop: ".8rem" }} onClick={() => deploy(t)}>Deploy {t.title}</button>
          </Card>
        ))}
      </div>
    </>
  );
}

function Backups({ projectId }: { projectId: string }) {
  const [dbs, setDbs] = useState<any[]>([]);
  const [backups, setBackups] = useState<any[]>([]);
  const [dbId, setDbId] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  async function load() {
    const d = await api.get(`/projects/${projectId}/databases`); setDbs(d.databases || []);
    const b = await api.get(`/projects/${projectId}/backups`); setBackups(b.backups || []);
  }
  useEffect(() => { load(); }, [projectId]);
  async function create() {
    if (!dbId) return;
    await api.post(`/projects/${projectId}/backups`, { databaseId: dbId, destination: "local", enabled: true });
    load();
  }
  async function run(b: any) {
    setMsg(null);
    try { const r = await api.post(`/backups/${b.id}/run`); setMsg("Backup saved: " + r.path); load(); }
    catch (e) { setMsg((e as Error).message); }
  }
  return (
    <>
      <Card title="Backups">
        {msg && <div className="err-box" style={{ background: "var(--panel2)", borderColor: "var(--border)", color: "var(--fg)" }}>{msg}</div>}
        {backups.length === 0 ? <Empty text="No backup configs yet." /> : (
          <table>
            <thead><tr><th>Destination</th><th>Cron</th><th>Last status</th><th>Last path</th><th></th></tr></thead>
            <tbody>
              {backups.map((b) => (
                <tr key={b.id}>
                  <td>{b.destination}</td><td>{b.cron || "manual"}</td>
                  <td>{b.lastStatus ? <StatusPill status={b.lastStatus} /> : "—"}</td>
                  <td className="mono muted">{(b.lastPath || "").slice(-40)}</td>
                  <td style={{ textAlign: "right" }}><button className="sm" onClick={() => run(b)}>Run now</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card title="New backup config">
        <label>Database</label>
        <select value={dbId} onChange={(e) => setDbId(e.target.value)}>
          <option value="">select…</option>
          {dbs.map((d) => <option key={d.id} value={d.id}>{d.name} ({d.engine})</option>)}
        </select>
        <button style={{ marginTop: ".8rem" }} onClick={create} disabled={!dbId}>Create (local)</button>
      </Card>
    </>
  );
}

function slug(s: string) {
  return s.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "") || "app";
}
