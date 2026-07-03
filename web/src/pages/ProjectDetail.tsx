import { useEffect, useRef, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { Eye, EyeOff, Globe, Loader2, Plus, Trash2 } from "lucide-react";
import { api, streamText, secretsApi, saveAppEnv, saveAppDomain, type SecretSummary } from "../api";
import { Card, Empty, ErrorBox, StatusPill } from "../ui";
import { Badge } from "@/components/ui/badge";
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

const TABS = ["overview", "applications", "databases", "config", "domains", "compose", "deployments", "logs", "templates", "backups"];
const TAB_LABELS: Record<string, string> = { config: "Env & Secrets", domains: "Domains & TLS" };

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
        tabs={TABS.map((t) => ({ key: t, label: TAB_LABELS[t] || t[0].toUpperCase() + t.slice(1) }))}
        active={tab}
        onSelect={(t) => setSp({ tab: t })}
      />
      {tab === "overview" && <Overview projectId={id} />}
      {tab === "applications" && <Applications projectId={id} />}
      {tab === "databases" && <Databases projectId={id} />}
      {tab === "config" && <EnvSecrets projectId={id} />}
      {tab === "domains" && <DomainsTLS projectId={id} />}
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
  const [openId, setOpenId] = useState<string | null>(null);
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
              <TableRow
                key={d.id}
                className="cursor-pointer"
                title="View build log"
                onClick={() => setOpenId(d.id)}
              >
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
      <DeploymentDrawer deploymentId={openId} onClose={() => setOpenId(null)} />
    </Card>
  );
}

// Statuses that are still in flight — the drawer polls /deployments/{id} while
// the deployment is in one of these so the build log grows live.
const DEPLOY_LIVE = new Set(["pending", "running"]);
const DEPLOY_POLL_MS = 1500;

// DeploymentDrawer opens a dialog for a single deployment and follows its build
// log. /deployments/{id} returns a JSON snapshot (status + log + error), not a
// chunked stream, so "streaming" is done by polling while the deployment is
// live. The in-flight GET is aborted and the poll timer cleared on close.
function DeploymentDrawer({ deploymentId, onClose }: { deploymentId: string | null; onClose: () => void }) {
  const [dep, setDep] = useState<any | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const preRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    if (!deploymentId) return;
    const ctrl = new AbortController();
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    setDep(null);
    setErr(null);
    setLoading(true);

    async function tick() {
      try {
        const d = await api.get(`/deployments/${deploymentId}`, ctrl.signal);
        if (!active) return;
        setDep(d);
        setLoading(false);
        if (DEPLOY_LIVE.has(d.status)) timer = setTimeout(tick, DEPLOY_POLL_MS);
      } catch (e) {
        // Aborting on close rejects with an AbortError — swallow it.
        if (!active || (e as Error).name === "AbortError") return;
        const msg = (e as Error).message;
        setErr(msg);
        setLoading(false);
        toast.error("Failed to load deployment: " + msg);
      }
    }
    tick();

    return () => {
      active = false;
      ctrl.abort();
      if (timer) clearTimeout(timer);
    };
  }, [deploymentId]);

  // Pin the build-log viewport to the newest output as it streams in.
  useEffect(() => {
    if (preRef.current) preRef.current.scrollTop = preRef.current.scrollHeight;
  }, [dep?.log]);

  const live = !!dep && DEPLOY_LIVE.has(dep.status);
  return (
    <Dialog open={!!deploymentId} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            Deployment
            {dep && <StatusPill status={dep.status} />}
            {live && <Loader2 className="size-4 animate-spin text-muted-foreground" />}
          </DialogTitle>
          <DialogDescription>
            {dep
              ? `${dep.kind} · ${live ? "following build log…" : "build finished"}`
              : "Loading deployment…"}
          </DialogDescription>
        </DialogHeader>
        {dep && (dep.imageRef || dep.commitSha) && (
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
            {dep.imageRef && <span>image <span className="font-mono text-foreground">{dep.imageRef}</span></span>}
            {dep.commitSha && <span>commit <span className="font-mono text-foreground">{dep.commitSha.slice(0, 12)}</span></span>}
            {dep.applied > 0 && <span>applied <span className="font-mono text-foreground">{dep.applied}</span></span>}
          </div>
        )}
        {err && <ErrorBox error={err} />}
        {dep?.error && <ErrorBox error={dep.error} />}
        <pre
          ref={preRef}
          className="max-h-[52vh] min-h-[200px] overflow-auto rounded-md border border-border bg-background/60 p-3 font-mono text-xs"
        >
          {dep?.log || (loading ? "Connecting…" : "No build output.")}
        </pre>
        <DialogFooter>
          <Button variant="secondary" onClick={onClose}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// Cap the live-log buffer so a long-running follow can't grow unbounded in memory.
const LOG_BUFFER_MAX = 200_000; // ~200 KB of tail text

function Logs({ projectId }: { projectId: string }) {
  const [service, setService] = useState("");
  const [text, setText] = useState("");
  const [following, setFollowing] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const abortRef = useRef<AbortController | null>(null);
  const preRef = useRef<HTMLPreElement>(null);

  function stop() {
    abortRef.current?.abort();
    abortRef.current = null;
    setFollowing(false);
  }

  async function follow() {
    abortRef.current?.abort(); // drop any in-flight reader before starting a new one
    setErr(null);
    setText("");
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setFollowing(true);
    try {
      await streamText(
        `/projects/${projectId}/logs?service=${encodeURIComponent(service)}&tail=200&follow=true`,
        {
          signal: ctrl.signal,
          onChunk: (chunk) =>
            setText((prev) => {
              const next = prev + chunk;
              return next.length > LOG_BUFFER_MAX ? next.slice(next.length - LOG_BUFFER_MAX) : next;
            }),
        },
      );
    } catch (e) {
      if ((e as Error).name !== "AbortError") {
        const msg = (e as Error).message;
        setErr(msg);
        toast.error("Log stream failed: " + msg);
      }
    } finally {
      // Only clear following state if this reader is still the active one — a
      // newer follow() may have replaced abortRef while we were awaiting.
      if (abortRef.current === ctrl) {
        abortRef.current = null;
        setFollowing(false);
      }
    }
  }

  // Abort the reader when the tab unmounts / project switches so nothing leaks.
  useEffect(() => () => abortRef.current?.abort(), []);

  // Keep the viewport pinned to the newest lines while auto-scroll is on.
  useEffect(() => {
    if (autoScroll && preRef.current) preRef.current.scrollTop = preRef.current.scrollHeight;
  }, [text, autoScroll]);

  return (
    <Card title="Logs">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Input
          placeholder="service name (blank = first pod)"
          value={service}
          onChange={(e) => setService(e.target.value)}
          disabled={following}
          className="max-w-[260px]"
        />
        {following ? (
          <Button variant="destructive" onClick={stop}>Stop</Button>
        ) : (
          <Button onClick={follow}>Follow logs</Button>
        )}
        <Button variant="outline" onClick={() => { setText(""); setErr(null); }} disabled={!text && !err}>Clear</Button>
        <label className="ml-auto flex cursor-pointer select-none items-center gap-2 text-sm text-muted-foreground">
          <Checkbox checked={autoScroll} onCheckedChange={(v) => setAutoScroll(v === true)} />
          Auto-scroll
        </label>
      </div>
      {err && <ErrorBox error={err} />}
      <pre
        ref={preRef}
        className="max-h-[420px] overflow-auto rounded-md border border-border bg-background/60 p-3 font-mono text-xs"
      >
        {text || (following ? "Connecting…" : "Pick a service and follow logs.")}
      </pre>
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

// ---- Env & Secrets (M4) -----------------------------------------------------
// Two surfaces live under the "Env & Secrets" tab:
//   • AppEnvEditor — per-app KEY=VALUE rows with a per-row "secret" flag (masks
//     the value + adds the key to the app's secretKeys). Env/secretKeys are
//     write-only on the backend (json:"-"), so this is SET-ON-SAVE: saving
//     replaces the app's whole environment and existing values are never read
//     back (by design — secret values must not leak).
//   • ClusterSecrets — full CRUD against /secrets. The list exposes only a key
//     COUNT per secret (never the values), so secret values stay masked; a PUT
//     replaces all keys of a secret.

let rowSeq = 0;
const nextRowId = () => ++rowSeq;
type KVRow = { id: number; key: string; value: string; secret: boolean; reveal: boolean };
const blankRow = (): KVRow => ({ id: nextRowId(), key: "", value: "", secret: false, reveal: false });

function EnvSecrets({ projectId }: { projectId: string }) {
  return (
    <>
      <AppEnvEditor projectId={projectId} />
      <ClusterSecrets />
    </>
  );
}

function AppEnvEditor({ projectId }: { projectId: string }) {
  const [apps, setApps] = useState<any[]>([]);
  const [appId, setAppId] = useState("");
  const [rows, setRows] = useState<KVRow[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.get(`/projects/${projectId}/apps`)
      .then((r) => setApps(r.applications || []))
      .catch((e) => toast.error("Failed to load apps: " + (e as Error).message));
  }, [projectId]);

  function selectApp(id: string) {
    setAppId(id);
    setRows(id ? [blankRow()] : []); // env is write-only → start from a blank row
  }
  function setRow(id: number, patch: Partial<KVRow>) {
    setRows((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  }
  function addRow() { setRows((rs) => [...rs, blankRow()]); }
  function removeRow(id: number) { setRows((rs) => rs.filter((r) => r.id !== id)); }

  async function save() {
    const filled = rows.filter((r) => r.key.trim());
    const keys = filled.map((r) => r.key.trim());
    if (new Set(keys).size !== keys.length) { toast.error("Duplicate variable names"); return; }
    const env: Record<string, string> = {};
    const secretKeys: string[] = [];
    for (const r of filled) {
      env[r.key.trim()] = r.value;
      if (r.secret) secretKeys.push(r.key.trim());
    }
    setBusy(true);
    try {
      await saveAppEnv(appId, env, secretKeys);
      toast.success(`Saved ${filled.length} variable${filled.length === 1 ? "" : "s"}`);
    } catch (e) { toast.error((e as Error).message); }
    finally { setBusy(false); }
  }

  const selected = apps.find((a) => a.id === appId);
  return (
    <Card title="Application environment">
      {apps.length === 0 ? (
        <Empty text="No applications yet — create one first." />
      ) : (
        <>
          <div className="max-w-sm">
            <Label>Application</Label>
            <Select value={appId} onChange={(e) => selectApp(e.target.value)}>
              <option value="">select an application…</option>
              {apps.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
            </Select>
          </div>
          {selected && (
            <div className="mt-4">
              <p className="mb-3 text-xs text-muted-foreground">
                Set the environment for <span className="font-medium text-foreground">{selected.name}</span>. Mark a row{" "}
                <em>secret</em> to mask its value and store the key as a cluster Secret. Saving replaces the app's full
                environment — stored values aren't shown back for security.
              </p>
              <div className="space-y-2">
                {rows.map((r) => (
                  <div key={r.id} className="flex items-center gap-2">
                    <Input placeholder="KEY" value={r.key} onChange={(e) => setRow(r.id, { key: e.target.value })} className="max-w-[220px] font-mono" />
                    <span className="text-muted-foreground">=</span>
                    <div className="relative flex-1">
                      <Input
                        type={r.secret && !r.reveal ? "password" : "text"}
                        placeholder="value"
                        value={r.value}
                        onChange={(e) => setRow(r.id, { value: e.target.value })}
                        className="pr-9 font-mono"
                      />
                      {r.secret && (
                        <button
                          type="button"
                          onClick={() => setRow(r.id, { reveal: !r.reveal })}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                          aria-label={r.reveal ? "Hide value" : "Reveal value"}
                        >
                          {r.reveal ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                        </button>
                      )}
                    </div>
                    <label className="flex cursor-pointer select-none items-center gap-1.5 text-xs text-muted-foreground">
                      <Checkbox checked={r.secret} onCheckedChange={(v) => setRow(r.id, { secret: v === true })} />
                      secret
                    </label>
                    <Button size="icon" variant="ghost" onClick={() => removeRow(r.id)} aria-label="Remove variable"><Trash2 className="size-4" /></Button>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex gap-2">
                <Button variant="outline" size="sm" onClick={addRow}><Plus /> Add variable</Button>
                <Button size="sm" onClick={save} disabled={busy}>{busy ? "Saving…" : "Save environment"}</Button>
              </div>
            </div>
          )}
        </>
      )}
    </Card>
  );
}

function ClusterSecrets() {
  const [secrets, setSecrets] = useState<SecretSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [editing, setEditing] = useState<{ name: string; isNew: boolean } | null>(null);
  const [rows, setRows] = useState<KVRow[]>([]);
  const [busy, setBusy] = useState(false);
  const [confirmState, setConfirmState] = useState<ConfirmState>(null);

  async function load() {
    setLoading(true); setErr(null);
    try { setSecrets(await secretsApi.list()); }
    catch (e) { const m = (e as Error).message; setErr(m); toast.error("Failed to load secrets: " + m); }
    finally { setLoading(false); }
  }
  useEffect(() => { load(); }, []);

  function openNew() { setEditing({ name: "", isNew: true }); setRows([blankRow()]); }
  function openEdit(s: SecretSummary) { setEditing({ name: s.name, isNew: false }); setRows([blankRow()]); }
  function close() { if (busy) return; setEditing(null); setRows([]); }
  function setRow(id: number, patch: Partial<KVRow>) { setRows((rs) => rs.map((r) => (r.id === id ? { ...r, ...patch } : r))); }
  function addRow() { setRows((rs) => [...rs, blankRow()]); }
  function removeRow(id: number) { setRows((rs) => rs.filter((r) => r.id !== id)); }

  async function save() {
    if (!editing) return;
    const name = editing.name.trim();
    if (!name) { toast.error("Secret name required"); return; }
    const filled = rows.filter((r) => r.key.trim());
    if (filled.length === 0) { toast.error("Add at least one key"); return; }
    const keys = filled.map((r) => r.key.trim());
    if (new Set(keys).size !== keys.length) { toast.error("Duplicate keys"); return; }
    const data: Record<string, string> = {};
    for (const r of filled) data[r.key.trim()] = r.value;
    setBusy(true);
    try {
      await secretsApi.put(name, data);
      toast.success(`Saved secret ${name}`);
      setEditing(null); setRows([]);
      load();
    } catch (e) { toast.error((e as Error).message); }
    finally { setBusy(false); }
  }

  function askDelete(s: SecretSummary) {
    setConfirmState({
      title: "Delete secret?",
      description: `"${s.name}" and all its keys will be removed from the cluster. This cannot be undone.`,
      confirmLabel: "Delete",
      variant: "destructive",
      onConfirm: async () => {
        try { await secretsApi.del(s.name); toast.success("Deleted " + s.name); load(); }
        catch (e) { toast.error((e as Error).message); }
      },
    });
  }

  return (
    <Card title="Secrets">
      <p className="mb-3 text-xs text-muted-foreground">
        Cluster secrets available to apps. Values are write-only — the list shows only how many keys each holds. Saving a
        secret replaces all of its keys.
      </p>
      {loading ? (
        <div className="text-sm text-muted-foreground">loading…</div>
      ) : err ? (
        <ErrorBox error={err} />
      ) : secrets.length === 0 ? (
        <Empty text="No secrets yet." />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Keys</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {secrets.map((s) => (
              <TableRow key={s.name} className="cursor-pointer" title="Replace keys" onClick={() => openEdit(s)}>
                <TableCell className="font-mono">{s.name}</TableCell>
                <TableCell className="text-muted-foreground">{s.keys}</TableCell>
                <TableCell>
                  <div className="flex justify-end">
                    <Button size="icon" variant="destructive" onClick={(e) => { e.stopPropagation(); askDelete(s); }} aria-label="Delete secret"><Trash2 className="size-4" /></Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <div className="mt-3"><Button size="sm" onClick={openNew}><Plus /> New secret</Button></div>

      <Dialog open={!!editing} onOpenChange={(o) => { if (!o) close(); }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing?.isNew ? "New secret" : `Edit ${editing?.name}`}</DialogTitle>
            <DialogDescription>
              {editing?.isNew
                ? "Create a cluster secret with one or more keys."
                : "Re-enter every key — saving replaces the secret's full contents."}
            </DialogDescription>
          </DialogHeader>
          <div>
            <Label>Name</Label>
            <Input
              value={editing?.name || ""}
              onChange={(e) => setEditing((ed) => (ed ? { ...ed, name: e.target.value } : ed))}
              disabled={!editing?.isNew}
              placeholder="my-secret"
              className="font-mono"
            />
          </div>
          <div className="space-y-2">
            {rows.map((r) => (
              <div key={r.id} className="flex items-center gap-2">
                <Input placeholder="KEY" value={r.key} onChange={(e) => setRow(r.id, { key: e.target.value })} className="max-w-[180px] font-mono" />
                <span className="text-muted-foreground">=</span>
                <div className="relative flex-1">
                  <Input
                    type={r.reveal ? "text" : "password"}
                    placeholder="value"
                    value={r.value}
                    onChange={(e) => setRow(r.id, { value: e.target.value })}
                    className="pr-9 font-mono"
                  />
                  <button
                    type="button"
                    onClick={() => setRow(r.id, { reveal: !r.reveal })}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    aria-label={r.reveal ? "Hide value" : "Reveal value"}
                  >
                    {r.reveal ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
                <Button size="icon" variant="ghost" onClick={() => removeRow(r.id)} aria-label="Remove key"><Trash2 className="size-4" /></Button>
              </div>
            ))}
          </div>
          <Button variant="outline" size="sm" className="w-fit" onClick={addRow}><Plus /> Add key</Button>
          <DialogFooter>
            <Button variant="secondary" onClick={close} disabled={busy}>Cancel</Button>
            <Button onClick={save} disabled={busy}>{busy ? "Saving…" : "Save secret"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog state={confirmState} onClose={() => setConfirmState(null)} />
    </Card>
  );
}

// ---- Domains & TLS (M4) -----------------------------------------------------
// Each application carries a single ingress domain (`domain`) + a `tls` flag.
// Both ARE serialized by the app list/get (json:"domain"/"tls"), so — unlike
// env — current values can be read back and prefilled. Update is a PUT
// /apps/{id} {domain, tls}. Backend caveats worth knowing:
//   • One domain per app (no multi-domain / no separate Domain entity), so this
//     surface lists each app's single host — "add" sets it, "edit" changes it.
//   • orDefault on the backend keeps the old domain when an empty string is
//     sent, so "Remove" reliably disables TLS but the host itself may persist
//     server-side until a real value replaces it (tracked as a backend gap).
//   • No cert-status field/endpoint exists; certificate state below is DERIVED
//     from domain+tls (cert-manager/ACME does the actual issuance out-of-band).

// certStatus maps an app's domain+tls into a badge label + variant. Honest
// derivation only — the backend doesn't report real ACME issuance state.
function certStatus(app: any): { label: string; variant: "success" | "warning" | "outline" } {
  if (!app.domain) return { label: "No domain", variant: "outline" };
  if (!app.tls) return { label: "HTTP only", variant: "warning" };
  return { label: "TLS · ACME managed", variant: "success" };
}

function DomainsTLS({ projectId }: { projectId: string }) {
  const [apps, setApps] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [editing, setEditing] = useState<{ app: any; host: string; tls: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmState, setConfirmState] = useState<ConfirmState>(null);

  async function load() {
    setLoading(true); setErr(null);
    try { const r = await api.get(`/projects/${projectId}/apps`); setApps(r.applications || []); }
    catch (e) { const m = (e as Error).message; setErr(m); toast.error("Failed to load apps: " + m); }
    finally { setLoading(false); }
  }
  useEffect(() => { load(); }, [projectId]);

  function openEdit(app: any) { setEditing({ app, host: app.domain || "", tls: !!app.tls }); }
  function close() { if (busy) return; setEditing(null); }

  async function save() {
    if (!editing) return;
    const host = editing.host.trim();
    if (!host) { toast.error("Enter a domain host"); return; }
    setBusy(true);
    try {
      await saveAppDomain(editing.app.id, host, editing.tls);
      toast.success(`Domain saved for ${editing.app.name}`);
      setEditing(null);
      load();
    } catch (e) { toast.error((e as Error).message); }
    finally { setBusy(false); }
  }

  function askRemove(app: any) {
    setConfirmState({
      title: "Remove domain?",
      description: `Detach ${app.domain} from "${app.name}" and disable TLS for it.`,
      confirmLabel: "Remove",
      variant: "destructive",
      onConfirm: async () => {
        try { await saveAppDomain(app.id, "", false); toast.success("Removed domain from " + app.name); load(); }
        catch (e) { toast.error((e as Error).message); }
      },
    });
  }

  return (
    <Card title="Domains & TLS">
      <p className="mb-3 text-xs text-muted-foreground">
        Each application exposes one ingress host. Toggle TLS to have cert-manager/ACME issue a certificate for the
        host. Certificate state is derived from the domain + TLS setting — the backend doesn't report live issuance.
      </p>
      {loading ? (
        <div className="text-sm text-muted-foreground">loading…</div>
      ) : err ? (
        <ErrorBox error={err} />
      ) : apps.length === 0 ? (
        <Empty text="No applications yet — create one first." />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Application</TableHead>
              <TableHead>Host</TableHead>
              <TableHead>TLS</TableHead>
              <TableHead>Certificate</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {apps.map((a) => {
              const cert = certStatus(a);
              return (
                <TableRow key={a.id}>
                  <TableCell className="font-medium">{a.name}</TableCell>
                  <TableCell>
                    {a.domain ? (
                      <a
                        href={`${a.tls ? "https" : "http"}://${a.domain}`}
                        target="_blank"
                        rel="noreferrer"
                        className="font-mono text-xs text-primary hover:underline"
                      >
                        {a.domain}
                      </a>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant={a.tls ? "success" : "outline"}>{a.tls ? "on" : "off"}</Badge>
                  </TableCell>
                  <TableCell><Badge variant={cert.variant}>{cert.label}</Badge></TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1.5">
                      {a.domain ? (
                        <>
                          <Button size="sm" variant="secondary" onClick={() => openEdit(a)}>Edit</Button>
                          <Button size="icon" variant="destructive" onClick={() => askRemove(a)} aria-label="Remove domain"><Trash2 className="size-4" /></Button>
                        </>
                      ) : (
                        <Button size="sm" onClick={() => openEdit(a)}><Globe /> Add domain</Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}

      <Dialog open={!!editing} onOpenChange={(o) => { if (!o) close(); }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editing?.app?.domain ? "Edit domain" : "Add domain"}</DialogTitle>
            <DialogDescription>
              Route a hostname to <span className="font-medium text-foreground">{editing?.app?.name}</span> via ingress.
              Point the host's DNS at the cluster, then enable TLS for an ACME-issued certificate.
            </DialogDescription>
          </DialogHeader>
          <div>
            <Label>Host</Label>
            <Input
              value={editing?.host || ""}
              onChange={(e) => setEditing((ed) => (ed ? { ...ed, host: e.target.value } : ed))}
              placeholder="app.example.com"
              className="font-mono"
              autoFocus
            />
          </div>
          <label className="flex cursor-pointer select-none items-center gap-2 text-sm">
            <Checkbox checked={!!editing?.tls} onCheckedChange={(v) => setEditing((ed) => (ed ? { ...ed, tls: v === true } : ed))} />
            <span>Enable TLS (cert-manager / ACME)</span>
          </label>
          <DialogFooter>
            <Button variant="secondary" onClick={close} disabled={busy}>Cancel</Button>
            <Button onClick={save} disabled={busy}>{busy ? "Saving…" : "Save domain"}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog state={confirmState} onClose={() => setConfirmState(null)} />
    </Card>
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
