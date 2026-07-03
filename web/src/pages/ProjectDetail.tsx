import { useEffect, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { Trash2 } from "lucide-react";
import { api, getText } from "../api";
import { Card, Empty, ErrorBox, StatusPill } from "../ui";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { TabBar } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { toast } from "@/components/ui/sonner";

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
  if (!project) return <div className="text-sm text-muted-foreground">loading…</div>;

  return (
    <>
      <div className="mb-5 flex items-center gap-3">
        <h1 className="text-xl font-semibold">{project.name}</h1>
        <span className="font-mono text-xs text-muted-foreground">{project.orcinusProject}</span>
      </div>
      <TabBar
        tabs={TABS.map((t) => ({ key: t, label: t[0].toUpperCase() + t.slice(1) }))}
        active={tab}
        onSelect={(t) => setSp({ tab: t })}
      />
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
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Service</TableHead>
              <TableHead>Pod</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Ready</TableHead>
              <TableHead>CPU</TableHead>
              <TableHead>Memory</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pods.map((p) => {
              const m = metricFor(p.name);
              return (
                <TableRow key={p.name}>
                  <TableCell>{p.service}</TableCell>
                  <TableCell className="font-mono text-xs">{p.name}</TableCell>
                  <TableCell><StatusPill status={p.status} /></TableCell>
                  <TableCell>{p.ready}</TableCell>
                  <TableCell>{m ? `${m.cpuMillicores}m` : "—"}</TableCell>
                  <TableCell>{m ? `${Math.round(m.memoryBytes / 1048576)}Mi` : "—"}</TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </Card>
  );
}

function Applications({ projectId }: { projectId: string }) {
  const [apps, setApps] = useState<any[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState("");
  const [form, setForm] = useState<any>({ name: "", buildType: "image", image: "", repoUrl: "", branch: "main", port: 80, domain: "", tls: false });
  const [confirmState, setConfirmState] = useState<ConfirmState>(null);
  const [scaleTarget, setScaleTarget] = useState<any | null>(null);
  const [replicas, setReplicas] = useState("2");

  async function load() {
    const r = await api.get(`/projects/${projectId}/apps`);
    setApps(r.applications || []);
  }
  useEffect(() => { load(); }, [projectId]);

  async function create() {
    setErr(null);
    try { await api.post(`/projects/${projectId}/apps`, { ...form, port: Number(form.port) }); toast.success(`Created ${form.name}`); setForm({ ...form, name: "" }); load(); }
    catch (e) { setErr((e as Error).message); }
  }
  async function deploy(a: any) {
    setBusy(a.id);
    try { await api.post(`/apps/${a.id}/deploy`); toast.success("Deploy started for " + a.name); }
    catch (e) { toast.error((e as Error).message); }
    finally { setBusy(""); }
  }
  async function doScale() {
    if (!scaleTarget) return;
    const n = Number(replicas);
    if (!Number.isFinite(n) || n < 0) { toast.error("Enter a valid replica count"); return; }
    try {
      await api.post(`/projects/${projectId}/services/${slug(scaleTarget.name)}/scale`, { replicas: n });
      toast.success(`Scaled ${scaleTarget.name} to ${n} replica${n === 1 ? "" : "s"}`);
      setScaleTarget(null);
    } catch (e) { toast.error((e as Error).message); }
  }
  function askRollback(a: any) {
    setConfirmState({
      title: `Roll back ${a.name}?`,
      description: "Reverts this service to its previous deployment revision.",
      confirmLabel: "Roll back",
      variant: "destructive",
      onConfirm: async () => {
        try { await api.post(`/projects/${projectId}/services/${slug(a.name)}/rollback`); toast.success("Rolled back " + a.name); }
        catch (e) { toast.error((e as Error).message); }
      },
    });
  }
  function askDelete(a: any) {
    setConfirmState({
      title: "Delete application?",
      description: `"${a.name}" and its deployed resources will be removed. This cannot be undone.`,
      confirmLabel: "Delete",
      variant: "destructive",
      onConfirm: async () => {
        try { await api.del(`/apps/${a.id}`); toast.success("Deleted " + a.name); load(); }
        catch (e) { toast.error((e as Error).message); }
      },
    });
  }

  return (
    <>
      <Card title="Applications">
        {apps.length === 0 ? <Empty text="No applications yet." /> : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Domain</TableHead>
                <TableHead>Image</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {apps.map((a) => (
                <TableRow key={a.id}>
                  <TableCell className="font-medium">{a.name}</TableCell>
                  <TableCell className="text-muted-foreground">{a.buildType}{a.repoUrl ? ` · ${a.repoUrl}` : ""}</TableCell>
                  <TableCell>{a.domain || "—"}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{(a.currentImage || a.image || "").slice(0, 30) || "—"}</TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1.5">
                      <Button size="sm" disabled={busy === a.id} onClick={() => deploy(a)}>Deploy</Button>
                      <Button size="sm" variant="secondary" onClick={() => { setScaleTarget(a); setReplicas("2"); }}>Scale</Button>
                      <Button size="sm" variant="secondary" onClick={() => askRollback(a)}>Rollback</Button>
                      <Button size="icon" variant="destructive" onClick={() => askDelete(a)}><Trash2 className="size-4" /></Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
      <Card title="New application">
        <ErrorBox error={err} />
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <Label>Name</Label>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <Label>Build type</Label>
            <Select value={form.buildType} onChange={(e) => setForm({ ...form, buildType: e.target.value })}>
              <option value="image">Prebuilt image</option>
              <option value="dockerfile">Dockerfile (git)</option>
              <option value="nixpacks">Nixpacks (git)</option>
            </Select>
            {form.buildType === "image" ? (
              <>
                <Label>Image</Label>
                <Input value={form.image} onChange={(e) => setForm({ ...form, image: e.target.value })} placeholder="nginx:alpine" />
              </>
            ) : (
              <>
                <Label>Repo URL</Label>
                <Input value={form.repoUrl} onChange={(e) => setForm({ ...form, repoUrl: e.target.value })} placeholder="https://github.com/user/repo" />
                <Label>Branch</Label>
                <Input value={form.branch} onChange={(e) => setForm({ ...form, branch: e.target.value })} />
              </>
            )}
          </div>
          <div>
            <Label>Container port</Label>
            <Input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} />
            <Label>Domain (optional → ingress)</Label>
            <Input value={form.domain} onChange={(e) => setForm({ ...form, domain: e.target.value })} placeholder="app.example.com" />
            <label className="mt-3 flex items-center gap-2 text-sm">
              <Checkbox checked={form.tls} onCheckedChange={(v) => setForm({ ...form, tls: v === true })} />
              <span>Enable TLS (cert-manager / ACME)</span>
            </label>
            <div><Button className="mt-4" onClick={create} disabled={!form.name}>Create application</Button></div>
          </div>
        </div>
      </Card>
      <ConfirmDialog state={confirmState} onClose={() => setConfirmState(null)} />
      <Dialog open={!!scaleTarget} onOpenChange={(o) => { if (!o) setScaleTarget(null); }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Scale {scaleTarget?.name}</DialogTitle>
            <DialogDescription>Set the desired number of running replicas for this service.</DialogDescription>
          </DialogHeader>
          <div>
            <Label>Replicas</Label>
            <Input type="number" min={0} value={replicas} onChange={(e) => setReplicas(e.target.value)} className="max-w-32" autoFocus />
          </div>
          <DialogFooter>
            <Button variant="secondary" onClick={() => setScaleTarget(null)}>Cancel</Button>
            <Button onClick={doScale}>Scale</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function Databases({ projectId }: { projectId: string }) {
  const [dbs, setDbs] = useState<any[]>([]);
  const [form, setForm] = useState({ name: "", engine: "postgres", dbName: "app", username: "kapibara", volumeSize: "1Gi" });
  const [err, setErr] = useState<string | null>(null);
  const [confirmState, setConfirmState] = useState<ConfirmState>(null);
  async function load() { const r = await api.get(`/projects/${projectId}/databases`); setDbs(r.databases || []); }
  useEffect(() => { load(); }, [projectId]);
  async function create() {
    setErr(null);
    try { const d = await api.post(`/projects/${projectId}/databases`, form); await api.post(`/databases/${d.id}/deploy`); toast.success(`Provisioning ${form.name}`); setForm({ ...form, name: "" }); load(); }
    catch (e) { setErr((e as Error).message); }
  }
  function askDelete(d: any) {
    setConfirmState({
      title: "Delete database?",
      description: `"${d.name}" and its volume will be permanently deleted. This cannot be undone.`,
      confirmLabel: "Delete",
      variant: "destructive",
      onConfirm: async () => {
        try { await api.del(`/databases/${d.id}`); toast.success("Deleted " + d.name); load(); }
        catch (e) { toast.error((e as Error).message); }
      },
    });
  }
  return (
    <>
      <Card title="Databases">
        {dbs.length === 0 ? <Empty text="No databases yet." /> : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Engine</TableHead>
                <TableHead>Connection string</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {dbs.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-medium">{d.name}</TableCell>
                  <TableCell>{d.engine}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{d.connectionString}</TableCell>
                  <TableCell className="text-right">
                    <Button size="icon" variant="destructive" onClick={() => askDelete(d)}><Trash2 className="size-4" /></Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
      <Card title="New database (1-click)">
        <ErrorBox error={err} />
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <Label>Name</Label>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            <Label>Engine</Label>
            <Select value={form.engine} onChange={(e) => setForm({ ...form, engine: e.target.value })}>
              {["postgres", "mysql", "mariadb", "mongo", "redis"].map((x) => <option key={x}>{x}</option>)}
            </Select>
          </div>
          <div>
            <Label>Database name</Label>
            <Input value={form.dbName} onChange={(e) => setForm({ ...form, dbName: e.target.value })} />
            <Label>Volume size</Label>
            <Input value={form.volumeSize} onChange={(e) => setForm({ ...form, volumeSize: e.target.value })} />
            <div><Button className="mt-4" onClick={create} disabled={!form.name}>Provision + Deploy</Button></div>
          </div>
        </div>
      </Card>
      <ConfirmDialog state={confirmState} onClose={() => setConfirmState(null)} />
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
      <Textarea value={source} onChange={(e) => setSource(e.target.value)} className="min-h-[240px]" />
      <div className="mt-3 flex gap-2">
        <Button onClick={deploy} disabled={busy}>{busy ? "Deploying…" : "Deploy"}</Button>
        <Button variant="secondary" onClick={preview}>Preview manifests</Button>
      </div>
      {out && <Pre className="mt-3">{out}</Pre>}
    </Card>
  );
}

function Deployments({ projectId }: { projectId: string }) {
  const [deps, setDeps] = useState<any[]>([]);
  useEffect(() => { api.get(`/projects/${projectId}/deployments`).then((r) => setDeps(r.deployments || [])); }, [projectId]);
  return (
    <Card title="Deployment history">
      {deps.length === 0 ? <Empty text="No deployments yet." /> : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Kind</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Image</TableHead>
              <TableHead>Commit</TableHead>
              <TableHead>Applied</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {deps.map((d) => (
              <TableRow key={d.id}>
                <TableCell>{d.kind}</TableCell>
                <TableCell><StatusPill status={d.status} /></TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{(d.imageRef || "").slice(0, 34)}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">{(d.commitSha || "").slice(0, 8)}</TableCell>
                <TableCell>{d.applied || 0}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
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
      <div className="mb-3 flex flex-wrap gap-2">
        <Input placeholder="service name (blank = first pod)" value={service} onChange={(e) => setService(e.target.value)} className="max-w-[260px]" />
        <Button onClick={load}>Fetch logs</Button>
      </div>
      <Pre>{text || "Pick a service and fetch logs."}</Pre>
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
      {msg && <Card><div className="text-sm">{msg}</div></Card>}
      <div className="grid gap-4 md:grid-cols-2">
        {tpls.map((t) => (
          <Card key={t.name} title={t.title}>
            <p className="text-sm text-muted-foreground">{t.description}</p>
            {(t.params || []).map((p: any) => (
              <div key={p.key}>
                <Label>{p.label || p.key}{p.required ? " *" : ""}</Label>
                <Input
                  placeholder={p.default}
                  onChange={(e) => setValues({ ...values, [t.name]: { ...(values[t.name] || {}), [p.key]: e.target.value } })}
                />
              </div>
            ))}
            <div><Button className="mt-3" onClick={() => deploy(t)}>Deploy {t.title}</Button></div>
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
        {msg && <div className="mb-3 rounded-md border border-border bg-muted px-3 py-2 text-sm">{msg}</div>}
        {backups.length === 0 ? <Empty text="No backup configs yet." /> : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Destination</TableHead>
                <TableHead>Cron</TableHead>
                <TableHead>Last status</TableHead>
                <TableHead>Last path</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {backups.map((b) => (
                <TableRow key={b.id}>
                  <TableCell>{b.destination}</TableCell>
                  <TableCell>{b.cron || "manual"}</TableCell>
                  <TableCell>{b.lastStatus ? <StatusPill status={b.lastStatus} /> : "—"}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{(b.lastPath || "").slice(-40)}</TableCell>
                  <TableCell className="text-right"><Button size="sm" onClick={() => run(b)}>Run now</Button></TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
      <Card title="New backup config">
        <Label>Database</Label>
        <Select value={dbId} onChange={(e) => setDbId(e.target.value)}>
          <option value="">select…</option>
          {dbs.map((d) => <option key={d.id} value={d.id}>{d.name} ({d.engine})</option>)}
        </Select>
        <div><Button className="mt-3" onClick={create} disabled={!dbId}>Create (local)</Button></div>
      </Card>
    </>
  );
}

type ConfirmState = {
  title: string;
  description?: string;
  confirmLabel?: string;
  variant?: "default" | "destructive";
  onConfirm: () => void | Promise<void>;
} | null;

function ConfirmDialog({ state, onClose }: { state: ConfirmState; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  async function handle() {
    if (!state) return;
    setBusy(true);
    try { await state.onConfirm(); onClose(); } finally { setBusy(false); }
  }
  return (
    <Dialog open={!!state} onOpenChange={(o) => { if (!o && !busy) onClose(); }}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{state?.title}</DialogTitle>
          {state?.description && <DialogDescription>{state.description}</DialogDescription>}
        </DialogHeader>
        <DialogFooter>
          <Button variant="secondary" onClick={onClose} disabled={busy}>Cancel</Button>
          <Button variant={state?.variant || "default"} onClick={handle} disabled={busy}>
            {busy ? "Working…" : state?.confirmLabel || "Confirm"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Pre({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <pre className={`max-h-[360px] overflow-auto rounded-md border border-border bg-background/60 p-3 font-mono text-xs ${className || ""}`}>
      {children}
    </pre>
  );
}

function slug(s: string) {
  return s.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "") || "app";
}
