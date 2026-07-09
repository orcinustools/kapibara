import { useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { ReactFlow, Background, Controls, MiniMap, type Node, type Edge } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  Boxes, ChevronDown, Database as DatabaseIcon, Eye, EyeOff, FileCode2, GitBranch, Globe,
  Layers, Loader2, Plus, Rocket, RotateCcw, ScrollText, Scaling, ScrollText as LogIcon, Server, Trash2, X,
} from "lucide-react";
import { api, streamText, secretsApi, backupsApi, previewApi, gitApi, saveAppEnv, saveAppDomain, type SecretSummary, type BackupSummary, type BackupCreate, type GitProvider, type GitRepo } from "../api";
import { Card, Empty, ErrorBox, StatusPill } from "../ui";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { toast } from "@/components/ui/sonner";
import { layoutUnits, nodeTypes, type Unit } from "../canvas";

interface Project { id: string; name: string; orcinusProject: string; }

// Management surfaces reachable from the toolbar / node panel. Each maps to one
// of the existing project-scoped components, opened in a dialog so the canvas
// stays the primary view and no capability is lost in the revamp.
type ManageKind =
  | "applications" | "databases" | "compose" | "deployments"
  | "logs" | "config" | "domains" | "templates" | "backups" | "overview";

const MANAGE: Record<ManageKind, { title: string; render: (id: string) => JSX.Element }> = {
  applications: { title: "Applications", render: (id) => <Applications projectId={id} /> },
  databases: { title: "Databases", render: (id) => <Databases projectId={id} /> },
  compose: { title: "Docker Compose", render: (id) => <Compose projectId={id} /> },
  deployments: { title: "Deployments", render: (id) => <Deployments projectId={id} /> },
  logs: { title: "Logs", render: (id) => <Logs projectId={id} /> },
  config: { title: "Env & Secrets", render: (id) => <EnvSecrets projectId={id} /> },
  domains: { title: "Domains & TLS", render: (id) => <DomainsTLS projectId={id} /> },
  templates: { title: "Templates", render: (id) => <Templates projectId={id} /> },
  backups: { title: "Backups", render: (id) => <Backups projectId={id} /> },
  overview: { title: "Pods & metrics", render: (id) => <Overview projectId={id} /> },
};

export function ProjectDetailPage() {
  const { id = "" } = useParams();
  const [project, setProject] = useState<Project | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [units, setUnits] = useState<Unit[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [manage, setManage] = useState<ManageKind | null>(null);
  // Preselected build type when opening the Applications panel (e.g. "New ▸ From
  // Git repo" opens it defaulted to a Dockerfile git build).
  const [appInitialBuild, setAppInitialBuild] = useState("image");
  const [openDeployId, setOpenDeployId] = useState<string | null>(null);

  useEffect(() => {
    api.get<Project>(`/projects/${id}`).then(setProject).catch((e) => setErr(e.message));
  }, [id]);

  // Build the unit list (apps + databases + compose) and pod statuses.
  const reload = useCallback(async () => {
    const [a, d, c, p] = await Promise.all([
      api.get(`/projects/${id}/apps`).catch(() => ({ applications: [] })),
      api.get(`/projects/${id}/databases`).catch(() => ({ databases: [] })),
      api.get(`/projects/${id}/compose`).catch(() => ({ composeApps: [] })),
      api.get(`/projects/${id}/pods`).catch(() => ({ pods: [] })),
    ]);
    const pods: any[] = p.pods || [];
    const statusOf = (name: string) => pods.find((x) => x.service === slug(name))?.status;
    const list: Unit[] = [
      ...(a.applications || []).map((x: any): Unit => ({
        id: x.id, kind: "application", name: x.name,
        subtitle: x.buildType === "image" ? (x.image || "image") : `${x.buildType} · ${x.repoUrl || ""}`,
        status: statusOf(x.name), raw: x,
      })),
      ...(d.databases || []).map((x: any): Unit => ({
        id: x.id, kind: "database", name: x.name,
        subtitle: `${x.engine}${x.version ? " " + x.version : ""}`, status: statusOf(x.name), raw: x,
      })),
      ...(c.composeApps || []).map((x: any): Unit => ({
        id: x.id, kind: "compose", name: x.name, subtitle: "docker-compose", raw: x,
      })),
    ];
    setUnits(list);
  }, [id]);
  useEffect(() => { reload(); }, [reload]);

  // Recompute the graph layout whenever the units change. Edges connect every
  // application to every database (they can reach each other in-project).
  useEffect(() => {
    const apps = units.filter((u) => u.kind === "application");
    const dbs = units.filter((u) => u.kind === "database");
    const e: Edge[] = [];
    for (const app of apps) for (const db of dbs) {
      e.push({ id: `${app.id}-${db.id}`, source: app.id, target: db.id, animated: true, style: { strokeDasharray: "4 4" } });
    }
    let cancelled = false;
    layoutUnits(units, e.map((x) => ({ id: x.id, source: x.source, target: x.target })), selectedId).then((n) => {
      if (!cancelled) { setNodes(n); setEdges(e); }
    });
    return () => { cancelled = true; };
  }, [units, selectedId]);

  const selected = units.find((u) => u.id === selectedId) || null;

  function closeManage() { setManage(null); reload(); }

  if (err) return <ErrorBox error={err} />;
  if (!project) return <div className="text-sm text-muted-foreground">loading…</div>;

  return (
    <div className="flex h-[calc(100vh-3rem)] flex-col">
      {/* Toolbar */}
      <div className="flex items-center gap-3 border-b border-border pb-3">
        <Layers className="size-5 text-primary" />
        <h1 className="text-lg font-semibold">{project.name}</h1>
        <span className="font-mono text-xs text-muted-foreground">{project.orcinusProject}</span>
        <div className="flex-1" />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm"><Plus className="size-4" /> New <ChevronDown className="size-3.5" /></Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => { setAppInitialBuild("dockerfile"); setManage("applications"); }}><GitBranch className="size-4" /> From Git repo</DropdownMenuItem>
            <DropdownMenuItem onClick={() => { setAppInitialBuild("image"); setManage("applications"); }}><Boxes className="size-4" /> Application (image)</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("databases")}><DatabaseIcon className="size-4" /> Database</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("compose")}><FileCode2 className="size-4" /> Compose</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("templates")}><Layers className="size-4" /> From template</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" variant="secondary">Manage <ChevronDown className="size-3.5" /></Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => setManage("deployments")}><Rocket className="size-4" /> Deployments</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("logs")}><LogIcon className="size-4" /> Logs</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("overview")}><Server className="size-4" /> Pods &amp; metrics</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("config")}><ScrollText className="size-4" /> Env &amp; Secrets</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("domains")}><Globe className="size-4" /> Domains &amp; TLS</DropdownMenuItem>
            <DropdownMenuItem onClick={() => setManage("backups")}><DatabaseIcon className="size-4" /> Backups</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Canvas + detail panel */}
      <div className="flex min-h-0 flex-1">
        <div className="relative min-w-0 flex-1">
          {units.length === 0 ? (
            <div className="grid h-full place-items-center">
              <div className="text-center">
                <Layers className="mx-auto mb-3 size-10 text-muted-foreground/40" />
                <p className="mb-1 text-sm font-medium">No services yet</p>
                <p className="mb-4 text-xs text-muted-foreground">Add an application, database, or compose stack to this project.</p>
                <Button size="sm" onClick={() => setManage("applications")}><Plus className="size-4" /> Add a service</Button>
              </div>
            </div>
          ) : (
            <ReactFlow
              nodes={nodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onNodeClick={(_, n) => setSelectedId(n.id)}
              onPaneClick={() => setSelectedId(null)}
              fitView
              proOptions={{ hideAttribution: true }}
              className="bg-background"
            >
              <Background gap={18} className="!bg-background" />
              <Controls showInteractive={false} />
              <MiniMap pannable zoomable className="!bg-card" />
            </ReactFlow>
          )}
        </div>
        {selected && (
          <UnitPanel
            key={selected.id}
            projectId={id}
            unit={selected}
            onClose={() => setSelectedId(null)}
            onChanged={reload}
            onManage={setManage}
            onFollowDeploy={setOpenDeployId}
            onLogs={() => setManage("logs")}
          />
        )}
      </div>

      {manage && (
        <Dialog open onOpenChange={(o) => { if (!o) closeManage(); }}>
          <DialogContent className="max-h-[86vh] max-w-5xl overflow-auto">
            <DialogHeader>
              <DialogTitle>{MANAGE[manage].title}</DialogTitle>
            </DialogHeader>
            {manage === "applications"
              ? <Applications projectId={id} initialBuild={appInitialBuild} />
              : MANAGE[manage].render(id)}
          </DialogContent>
        </Dialog>
      )}
      <DeploymentDrawer deploymentId={openDeployId} onClose={() => setOpenDeployId(null)} />
    </div>
  );
}

// UnitPanel is the right-hand detail panel shown when a canvas node is selected.
// It exposes the unit's identity + the primary day-2 actions (deploy, scale,
// rollback, logs, delete) and deep-links into the full management surfaces.
function UnitPanel({
  projectId, unit, onClose, onChanged, onManage, onFollowDeploy, onLogs,
}: {
  projectId: string;
  unit: Unit;
  onClose: () => void;
  onChanged: () => void;
  onManage: (k: ManageKind) => void;
  onFollowDeploy: (id: string) => void;
  onLogs: (service: string) => void;
}) {
  const [busy, setBusy] = useState("");
  const [confirmState, setConfirmState] = useState<ConfirmState>(null);
  const [scaleOpen, setScaleOpen] = useState(false);
  const [replicas, setReplicas] = useState("2");
  const a = unit.raw;

  async function deploy() {
    setBusy("deploy");
    try {
      if (unit.kind === "application") {
        const dep = await api.post(`/apps/${unit.id}/deploy`);
        toast.success("Deploy started for " + unit.name);
        if (dep?.id) onFollowDeploy(dep.id);
      } else if (unit.kind === "database") {
        await api.post(`/databases/${unit.id}/deploy`);
        toast.success("Deploying " + unit.name);
      } else {
        const r = await api.post(`/projects/${projectId}/deploy`, { composeAppId: unit.id, wait: true });
        toast.success(`Applied ${r.applied} objects`);
      }
      onChanged();
    } catch (e) { toast.error((e as Error).message); }
    finally { setBusy(""); }
  }
  async function doScale() {
    const n = Number(replicas);
    if (!Number.isFinite(n) || n < 0) { toast.error("Enter a valid replica count"); return; }
    try {
      await api.post(`/projects/${projectId}/services/${slug(unit.name)}/scale`, { replicas: n });
      toast.success(`Scaled ${unit.name} to ${n} replica${n === 1 ? "" : "s"}`);
      setScaleOpen(false); onChanged();
    } catch (e) { toast.error((e as Error).message); }
  }
  function askRollback() {
    setConfirmState({
      title: `Roll back ${unit.name}?`,
      description: "Reverts this service to its previous deployment revision.",
      confirmLabel: "Roll back", variant: "destructive",
      onConfirm: async () => {
        try { await api.post(`/projects/${projectId}/services/${slug(unit.name)}/rollback`); toast.success("Rolled back " + unit.name); }
        catch (e) { toast.error((e as Error).message); }
      },
    });
  }
  function askDelete() {
    setConfirmState({
      title: `Delete ${unit.name}?`,
      description: "The unit and its deployed cluster resources will be removed. This cannot be undone.",
      confirmLabel: "Delete", variant: "destructive",
      onConfirm: async () => {
        try {
          await api.del(unit.kind === "application" ? `/apps/${unit.id}` : `/databases/${unit.id}`);
          toast.success("Deleted " + unit.name);
          onClose(); onChanged();
        } catch (e) { toast.error((e as Error).message); }
      },
    });
  }

  return (
    <aside className="flex w-[360px] shrink-0 flex-col overflow-y-auto border-l border-border bg-card">
      <div className="flex items-start gap-2 border-b border-border p-4">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-base font-semibold">{unit.name}</span>
            {unit.status && <StatusPill status={unit.status} />}
          </div>
          <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">{unit.subtitle}</div>
        </div>
        <Button size="icon" variant="ghost" className="size-7" onClick={onClose} aria-label="Close panel"><X className="size-4" /></Button>
      </div>

      <div className="flex flex-col gap-3 p-4">
        <Button onClick={deploy} disabled={busy === "deploy"}>
          {busy === "deploy" ? <Loader2 className="size-4 animate-spin" /> : <Rocket className="size-4" />} Deploy
        </Button>

        {unit.kind === "database" && a.connectionString && (
          <div className="rounded-md border border-border bg-background/60 p-2">
            <div className="mb-1 text-[11px] font-medium text-muted-foreground">Connection string</div>
            <code className="block break-all font-mono text-xs">{a.connectionString}</code>
          </div>
        )}
        {unit.kind === "application" && a.domain && (
          <a href={`${a.tls ? "https" : "http"}://${a.domain}`} target="_blank" rel="noreferrer"
             className="flex items-center gap-1.5 text-sm text-primary hover:underline">
            <Globe className="size-4" /> {a.domain}
          </a>
        )}

        {unit.kind !== "compose" && (
          <div className="grid grid-cols-2 gap-2">
            <Button size="sm" variant="secondary" onClick={() => { setReplicas("2"); setScaleOpen(true); }}><Scaling className="size-4" /> Scale</Button>
            <Button size="sm" variant="secondary" onClick={askRollback}><RotateCcw className="size-4" /> Rollback</Button>
          </div>
        )}
        <Button size="sm" variant="secondary" onClick={() => onLogs(slug(unit.name))}><LogIcon className="size-4" /> Logs</Button>

        {unit.kind === "application" && (
          <div className="grid grid-cols-2 gap-2">
            <Button size="sm" variant="outline" onClick={() => onManage("config")}><ScrollText className="size-4" /> Env</Button>
            <Button size="sm" variant="outline" onClick={() => onManage("domains")}><Globe className="size-4" /> Domain</Button>
          </div>
        )}
        {unit.kind === "database" && (
          <Button size="sm" variant="outline" onClick={() => onManage("backups")}><DatabaseIcon className="size-4" /> Backups</Button>
        )}

        <Button size="sm" variant="outline" onClick={() => onManage(unit.kind === "compose" ? "compose" : unit.kind === "database" ? "databases" : "applications")}>
          <GitBranch className="size-4" /> Open full management
        </Button>

        {unit.kind !== "compose" && (
          <Button size="sm" variant="destructive" onClick={askDelete}><Trash2 className="size-4" /> Delete</Button>
        )}
      </div>

      <ConfirmDialog state={confirmState} onClose={() => setConfirmState(null)} />
      <Dialog open={scaleOpen} onOpenChange={setScaleOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Scale {unit.name}</DialogTitle>
            <DialogDescription>Set the desired number of running replicas.</DialogDescription>
          </DialogHeader>
          <div><Label>Replicas</Label><Input type="number" min={0} value={replicas} onChange={(e) => setReplicas(e.target.value)} className="max-w-32" autoFocus /></div>
          <DialogFooter>
            <Button variant="secondary" onClick={() => setScaleOpen(false)}>Cancel</Button>
            <Button onClick={doScale}>Scale</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </aside>
  );
}

// Lightweight, dependency-free CSS meter for instantaneous CPU/RAM readings.
// `ratio` is 0..1 relative to the busiest pod in the current set (no k8s limits
// are exposed, so peers are the only honest reference). The numeric label keeps
// the absolute value readable next to the visual bar.
function MetricBar({ valueLabel, ratio, tone, srLabel }: { valueLabel: string; ratio: number; tone: "cpu" | "mem"; srLabel: string }) {
  const pct = Math.round(Math.max(0, Math.min(1, ratio)) * 100);
  const fill = tone === "cpu" ? "bg-primary" : "bg-success";
  return (
    <div className="flex items-center gap-2 min-w-[8rem]">
      <div
        className="h-1.5 flex-1 rounded-full bg-muted overflow-hidden"
        role="progressbar"
        aria-label={srLabel}
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div className={`h-full rounded-full ${fill} transition-[width] duration-300`} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs tabular-nums text-muted-foreground w-14 text-right shrink-0">{valueLabel}</span>
    </div>
  );
}

// Sparkline renders a dependency-free SVG line of a numeric series, scaled to
// its own peak. Used for the project's CPU/RAM history (time-series, M6).
function Sparkline({ values, tone, unitLabel }: { values: number[]; tone: "cpu" | "mem"; unitLabel: string }) {
  const w = 160, h = 34;
  if (values.length < 2) {
    return <div className="text-xs text-muted-foreground">Collecting history…</div>;
  }
  const max = Math.max(1, ...values);
  const stroke = tone === "cpu" ? "var(--primary, #6366f1)" : "var(--success, #10b981)";
  const step = w / (values.length - 1);
  const pts = values.map((v, i) => `${(i * step).toFixed(1)},${(h - (v / max) * (h - 4) - 2).toFixed(1)}`).join(" ");
  const last = values[values.length - 1];
  return (
    <div className="flex items-center gap-2">
      <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} role="img" aria-label={`${tone} history`} className="shrink-0">
        <polyline points={pts} fill="none" stroke={stroke} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
      </svg>
      <span className="text-xs tabular-nums text-muted-foreground w-20 text-right shrink-0">{last}{unitLabel}</span>
    </div>
  );
}

function Overview({ projectId }: { projectId: string }) {
  const [pods, setPods] = useState<any[]>([]);
  const [metrics, setMetrics] = useState<any[]>([]);
  const [history, setHistory] = useState<any[]>([]);
  useEffect(() => {
    let stop = false;
    const tick = () => {
      api.get(`/projects/${projectId}/pods`).then((r) => { if (!stop) setPods(r.pods || []); }).catch(() => {});
      api.get(`/projects/${projectId}/metrics`).then((r) => {
        if (stop) return;
        setMetrics(r.metrics || []);
        setHistory(r.history || []);
      }).catch(() => {});
    };
    tick();
    // Poll so the history series advances live while the tab is open.
    const iv = setInterval(tick, 5000);
    return () => { stop = true; clearInterval(iv); };
  }, [projectId]);
  const metricFor = (pod: string) => metrics.find((m) => m.pod === pod);
  const cpuSeries = history.map((s) => s.cpuMillicores || 0);
  const memSeries = history.map((s) => Math.round((s.memoryBytes || 0) / 1048576));
  const maxCpu = Math.max(1, ...metrics.map((m) => m.cpuMillicores || 0));
  const maxMem = Math.max(1, ...metrics.map((m) => m.memoryBytes || 0));
  return (
    <>
    <Card title="Resource usage (project total)">
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <div className="mb-1 text-xs font-medium text-muted-foreground">CPU (millicores)</div>
          <Sparkline values={cpuSeries} tone="cpu" unitLabel="m" />
        </div>
        <div>
          <div className="mb-1 text-xs font-medium text-muted-foreground">Memory (Mi)</div>
          <Sparkline values={memSeries} tone="mem" unitLabel="Mi" />
        </div>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">Live sum across the project's pods; history accrues while the server runs.</p>
    </Card>
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
                  <TableCell>
                    {m ? (
                      <MetricBar tone="cpu" ratio={m.cpuMillicores / maxCpu} valueLabel={`${m.cpuMillicores}m`} srLabel={`CPU ${m.cpuMillicores} millicores`} />
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    {m ? (
                      <MetricBar tone="mem" ratio={m.memoryBytes / maxMem} valueLabel={`${Math.round(m.memoryBytes / 1048576)}Mi`} srLabel={`Memory ${Math.round(m.memoryBytes / 1048576)} mebibytes`} />
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
    </Card>
    </>
  );
}

function Applications({ projectId, initialBuild = "image" }: { projectId: string; initialBuild?: string }) {
  const [apps, setApps] = useState<any[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState("");
  const [form, setForm] = useState<any>({ name: "", buildType: initialBuild, image: "", repoUrl: "", branch: "main", contextDir: "", dockerfilePath: "", envText: "", mountsText: "", volumeSize: "", gitProviderId: "", port: 80, domain: "", tls: false, autoscaleMin: "", autoscaleMax: "", autoscaleCpu: "", autoscaleMemory: "", rollout: "" });
  // Git provider connect + repo picker (M3). Providers are org-scoped, so we
  // resolve the project's org first, then offer its connected providers; picking
  // a repo fills repoUrl/branch and links the provider so its token is injected
  // into the clone.
  const [providers, setProviders] = useState<GitProvider[]>([]);
  const [repos, setRepos] = useState<GitRepo[]>([]);
  const [reposLoading, setReposLoading] = useState(false);
  const [confirmState, setConfirmState] = useState<ConfirmState>(null);
  const [scaleTarget, setScaleTarget] = useState<any | null>(null);
  const [replicas, setReplicas] = useState("2");
  // Ephemeral per-branch preview environments (M9). The backend doesn't list
  // previews, so we track the ones triggered this session and follow each via
  // its deployment id; teardown is keyed by project + branch.
  const [previews, setPreviews] = useState<PreviewEntry[]>([]);
  const [previewTarget, setPreviewTarget] = useState<any | null>(null);
  const [previewBranch, setPreviewBranch] = useState("");
  const [previewBusy, setPreviewBusy] = useState(false);
  const [openDeployId, setOpenDeployId] = useState<string | null>(null);

  async function load() {
    const r = await api.get(`/projects/${projectId}/apps`);
    setApps(r.applications || []);
  }
  useEffect(() => { load(); }, [projectId]);
  useEffect(() => {
    // Resolve the org from the project, then load its connected git providers.
    let stop = false;
    api.get(`/projects/${projectId}`).then((p) => {
      if (stop || !p?.organizationId) return;
      gitApi.list(p.organizationId).then((ps) => { if (!stop) setProviders(ps); }).catch(() => {});
    }).catch(() => {});
    return () => { stop = true; };
  }, [projectId]);

  async function pickProvider(providerId: string) {
    setForm((f: any) => ({ ...f, gitProviderId: providerId }));
    setRepos([]);
    if (!providerId) return;
    setReposLoading(true);
    try { setRepos(await gitApi.repos(providerId)); }
    catch (e) { toast.error((e as Error).message); }
    finally { setReposLoading(false); }
  }
  function pickRepo(cloneUrl: string) {
    const repo = repos.find((r) => r.cloneUrl === cloneUrl);
    if (!repo) return;
    setForm((f: any) => ({ ...f, repoUrl: repo.cloneUrl, branch: repo.defaultBranch || f.branch }));
  }

  async function create() {
    setErr(null);
    try {
      // Parse the "KEY=VALUE per line" env box into an env map.
      const env: Record<string, string> = {};
      for (const line of String(form.envText || "").split("\n")) {
        const t = line.trim();
        if (!t || t.startsWith("#")) continue;
        const i = t.indexOf("=");
        if (i > 0) env[t.slice(0, i).trim()] = t.slice(i + 1);
      }
      // Parse "name:path per line" into the mounts array the API expects.
      const mounts: { name: string; path: string }[] = [];
      for (const line of String(form.mountsText || "").split("\n")) {
        const t = line.trim();
        if (!t || t.startsWith("#")) continue;
        const i = t.indexOf(":");
        if (i > 0) mounts.push({ name: t.slice(0, i).trim(), path: t.slice(i + 1).trim() });
      }
      const { envText, mountsText, ...rest } = form;
      await api.post(`/projects/${projectId}/apps`, {
        ...rest,
        port: Number(form.port) || 0,
        autoscaleMin: Number(form.autoscaleMin) || 0,
        autoscaleMax: Number(form.autoscaleMax) || 0,
        autoscaleCpu: Number(form.autoscaleCpu) || 0,
        autoscaleMemory: Number(form.autoscaleMemory) || 0,
        ...(Object.keys(env).length ? { env } : {}),
        ...(mounts.length ? { mounts } : {}),
      });
      toast.success(`Created ${form.name}`);
      setForm({ ...form, name: "" });
      load();
    }
    catch (e) { setErr((e as Error).message); }
  }
  async function deploy(a: any) {
    setBusy(a.id);
    try {
      const dep = await api.post(`/apps/${a.id}/deploy`);
      toast.success("Deploy started for " + a.name);
      // Open the build-log drawer so git builds (clone → build → push) can be
      // followed live; prebuilt-image deploys finish near-instantly.
      if (dep?.id) setOpenDeployId(dep.id);
    }
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
  function askPreview(a: any) {
    setPreviewTarget(a);
    setPreviewBranch(a.branch || "main");
  }
  async function doPreview() {
    if (!previewTarget) return;
    const branch = previewBranch.trim();
    if (!branch) { toast.error("Enter a branch to preview"); return; }
    setPreviewBusy(true);
    try {
      const r = await previewApi.deploy(previewTarget.id, branch);
      const key = `${previewTarget.id}:${branch}`;
      setPreviews((prev) => [
        ...prev.filter((p) => p.key !== key),
        { key, appId: previewTarget.id, appName: previewTarget.name, branch, deploymentId: r.deploymentId, previewProject: r.previewProject },
      ]);
      toast.success(`Preview deploying: ${previewTarget.name} @ ${branch}`);
      setPreviewTarget(null);
    } catch (e) { toast.error((e as Error).message); }
    finally { setPreviewBusy(false); }
  }
  function askTeardown(p: PreviewEntry) {
    setConfirmState({
      title: "Tear down preview?",
      description: `The ephemeral environment "${p.previewProject}" (${p.appName} @ ${p.branch}) will be removed from the cluster.`,
      confirmLabel: "Tear down",
      variant: "destructive",
      onConfirm: async () => {
        try {
          await previewApi.teardown(projectId, p.branch);
          setPreviews((prev) => prev.filter((x) => x.key !== p.key));
          toast.success(`Torn down ${p.previewProject}`);
        } catch (e) { toast.error((e as Error).message); }
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
                      <Button size="sm" variant="secondary" onClick={() => askPreview(a)}><GitBranch className="size-4" />Preview</Button>
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
      {previews.length > 0 && (
        <Card title="Preview environments">
          <p className="mb-3 text-xs text-muted-foreground">
            Ephemeral per-branch deployments, isolated from the main project. Tracked for this session only —
            the cluster doesn't report existing previews back, so this list clears on reload.
          </p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Application</TableHead>
                <TableHead>Branch</TableHead>
                <TableHead>Preview project</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {previews.map((p) => (
                <TableRow key={p.key}>
                  <TableCell className="font-medium">{p.appName}</TableCell>
                  <TableCell className="font-mono text-xs">{p.branch}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{p.previewProject}</TableCell>
                  <TableCell><PreviewStatus deploymentId={p.deploymentId} /></TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1.5">
                      <Button size="sm" variant="ghost" onClick={() => setOpenDeployId(p.deploymentId)}>View log</Button>
                      <Button size="sm" variant="destructive" onClick={() => askTeardown(p)}>Tear down</Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
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
              <option value="railpack">Railpack (git)</option>
            </Select>
            {form.buildType === "image" ? (
              <>
                <Label>Image</Label>
                <Input value={form.image} onChange={(e) => setForm({ ...form, image: e.target.value })} placeholder="nginx:alpine" />
              </>
            ) : (
              <>
                {providers.length > 0 && (
                  <>
                    <Label>Git provider (optional)</Label>
                    <Select value={form.gitProviderId} onChange={(e) => pickProvider(e.target.value)}>
                      <option value="">— Public repo / manual URL —</option>
                      {providers.map((p) => (
                        <option key={p.id} value={p.id}>{p.name} ({p.type}: {p.accountLogin})</option>
                      ))}
                    </Select>
                    {form.gitProviderId && (
                      <>
                        <Label>Repository</Label>
                        <Select value={form.repoUrl} onChange={(e) => pickRepo(e.target.value)} disabled={reposLoading}>
                          <option value="">{reposLoading ? "Loading repositories…" : "— Select a repository —"}</option>
                          {repos.map((r) => (
                            <option key={r.cloneUrl} value={r.cloneUrl}>{r.fullName}{r.private ? " (private)" : ""}</option>
                          ))}
                        </Select>
                      </>
                    )}
                  </>
                )}
                <Label>Repo URL</Label>
                <Input value={form.repoUrl} onChange={(e) => setForm({ ...form, repoUrl: e.target.value, gitProviderId: form.gitProviderId })} placeholder="https://github.com/user/repo" />
                <Label>Branch</Label>
                <Input value={form.branch} onChange={(e) => setForm({ ...form, branch: e.target.value })} />
                <Label>Context dir (subfolder, optional)</Label>
                <Input value={form.contextDir} onChange={(e) => setForm({ ...form, contextDir: e.target.value })} placeholder="apps/backend (monorepos)" />
                {form.buildType === "dockerfile" && (
                  <>
                    <Label>Dockerfile path (optional)</Label>
                    <Input value={form.dockerfilePath} onChange={(e) => setForm({ ...form, dockerfilePath: e.target.value })} placeholder="Dockerfile (relative to context dir)" />
                  </>
                )}
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
            <Label className="mt-3">Environment variables</Label>
            <textarea
              value={form.envText}
              onChange={(e) => setForm({ ...form, envText: e.target.value })}
              placeholder={"KEY=VALUE per line\nDATABASE_URL=postgres://…@db:5432/app"}
              rows={4}
              className="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs"
            />
            <Label className="mt-3">Persistent volumes</Label>
            <textarea
              value={form.mountsText}
              onChange={(e) => setForm({ ...form, mountsText: e.target.value })}
              placeholder={"name:path per line\ndata:/var/lib/app\nuploads:/app/uploads"}
              rows={3}
              className="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs"
            />
            <Label className="mt-2">Volume size (PVC)</Label>
            <Input value={form.volumeSize} onChange={(e) => setForm({ ...form, volumeSize: e.target.value })} placeholder="1Gi" />
          </div>
        </div>
        <div className="mt-5 border-t border-border pt-4">
          <div className="mb-1 text-sm font-medium">Autoscaling &amp; rollout</div>
          <p className="mb-3 text-xs text-muted-foreground">
            Optional. Set min/max replicas and target utilization to enable a horizontal autoscaler — leave min/max at 0
            to run a fixed replica count. Rollout picks the progressive-delivery strategy.
          </p>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
            <div>
              <Label>Min replicas</Label>
              <Input type="number" min={0} value={form.autoscaleMin} onChange={(e) => setForm({ ...form, autoscaleMin: e.target.value })} placeholder="0" />
            </div>
            <div>
              <Label>Max replicas</Label>
              <Input type="number" min={0} value={form.autoscaleMax} onChange={(e) => setForm({ ...form, autoscaleMax: e.target.value })} placeholder="0" />
            </div>
            <div>
              <Label>Target CPU %</Label>
              <Input type="number" min={0} max={100} value={form.autoscaleCpu} onChange={(e) => setForm({ ...form, autoscaleCpu: e.target.value })} placeholder="0" />
            </div>
            <div>
              <Label>Target memory %</Label>
              <Input type="number" min={0} max={100} value={form.autoscaleMemory} onChange={(e) => setForm({ ...form, autoscaleMemory: e.target.value })} placeholder="0" />
            </div>
            <div>
              <Label>Rollout</Label>
              <Select value={form.rollout} onChange={(e) => setForm({ ...form, rollout: e.target.value })}>
                <option value="">Rolling (default)</option>
                <option value="canary">Canary</option>
                <option value="blue-green">Blue-green</option>
              </Select>
            </div>
          </div>
        </div>
        <div><Button className="mt-5" onClick={create} disabled={!form.name}>Create application</Button></div>
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
      <Dialog open={!!previewTarget} onOpenChange={(o) => { if (!o && !previewBusy) setPreviewTarget(null); }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Deploy preview · {previewTarget?.name}</DialogTitle>
            <DialogDescription>
              Spins up an isolated, ephemeral environment for the chosen branch, separate from the live project.
            </DialogDescription>
          </DialogHeader>
          <div>
            <Label>Branch</Label>
            <Input value={previewBranch} onChange={(e) => setPreviewBranch(e.target.value)} placeholder="feature/my-branch" autoFocus onKeyDown={(e) => { if (e.key === "Enter") doPreview(); }} />
          </div>
          <DialogFooter>
            <Button variant="secondary" onClick={() => setPreviewTarget(null)} disabled={previewBusy}>Cancel</Button>
            <Button onClick={doPreview} disabled={previewBusy || !previewBranch.trim()}>
              {previewBusy ? <><Loader2 className="size-4 animate-spin" />Deploying…</> : "Deploy preview"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <DeploymentDrawer deploymentId={openDeployId} onClose={() => setOpenDeployId(null)} />
    </>
  );
}

// A tracked preview environment (session-only — the backend doesn't list them).
type PreviewEntry = { key: string; appId: string; appName: string; branch: string; deploymentId: string; previewProject: string };

// Follows a preview's build/deploy status by polling its deployment while the
// deployment is still in flight, mirroring DeploymentDrawer's poll guards.
function PreviewStatus({ deploymentId }: { deploymentId: string }) {
  const [status, setStatus] = useState("pending");
  useEffect(() => {
    const ctrl = new AbortController();
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function tick() {
      try {
        const d = await api.get(`/deployments/${deploymentId}`, ctrl.signal);
        if (!active) return;
        setStatus(d.status);
        if (DEPLOY_LIVE.has(d.status)) timer = setTimeout(tick, DEPLOY_POLL_MS);
      } catch (e) {
        if (!active || (e as Error).name === "AbortError") return;
      }
    }
    tick();
    return () => { active = false; ctrl.abort(); if (timer) clearTimeout(timer); };
  }, [deploymentId]);
  const live = DEPLOY_LIVE.has(status);
  return (
    <span className="flex items-center gap-2">
      <StatusPill status={status} />
      {live && <Loader2 className="size-3.5 animate-spin text-muted-foreground" />}
    </span>
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

// ---- Backups (M8) -----------------------------------------------------------
// A backup config schedules (cron) or manually triggers a dump of one managed
// database to a local path or an S3-compatible bucket. The S3 endpoint +
// credentials are WRITE-ONLY server-side (json:"-"), so they are set on create
// and never read back — the list can only show cron/destination/enabled + the
// last-run status. Presets keep the cron field approachable without a full
// cron builder.
const CRON_PRESETS: { label: string; value: string }[] = [
  { label: "Manual only (no schedule)", value: "" },
  { label: "Hourly", value: "0 * * * *" },
  { label: "Daily at 02:00", value: "0 2 * * *" },
  { label: "Weekly (Sun 03:00)", value: "0 3 * * 0" },
  { label: "Monthly (1st 04:00)", value: "0 4 1 * *" },
];

const blankS3 = () => ({ endpoint: "", bucket: "", region: "", accessKey: "", secretKey: "", secure: true });

function Backups({ projectId }: { projectId: string }) {
  const [dbs, setDbs] = useState<any[]>([]);
  const [backups, setBackups] = useState<BackupSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [running, setRunning] = useState<string | null>(null);

  // Create-form state.
  const [dbId, setDbId] = useState("");
  const [cron, setCron] = useState("");
  const [cronPreset, setCronPreset] = useState("");
  const [destination, setDestination] = useState("local");
  const [enabled, setEnabled] = useState(true);
  const [s3, setS3] = useState(blankS3());
  const [saving, setSaving] = useState(false);

  async function load() {
    setErr(null);
    try {
      const [d, b] = await Promise.all([
        api.get(`/projects/${projectId}/databases`),
        backupsApi.list(projectId),
      ]);
      setDbs(d.databases || []);
      setBackups(b);
    } catch (e) {
      setErr((e as Error).message);
      toast.error("Failed to load backups: " + (e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, [projectId]);

  const dbName = (id: string) => {
    const d = dbs.find((x) => x.id === id);
    return d ? `${d.name} (${d.engine})` : id.slice(0, 8);
  };

  async function create() {
    if (!dbId) return;
    if (destination === "s3" && (!s3.endpoint.trim() || !s3.bucket.trim())) {
      toast.error("S3 destination needs an endpoint and a bucket.");
      return;
    }
    setSaving(true);
    try {
      const body: BackupCreate = { databaseId: dbId, cron: cron.trim(), destination, enabled };
      if (destination === "s3") {
        body.s3Config = {
          endpoint: s3.endpoint.trim(),
          bucket: s3.bucket.trim(),
          region: s3.region.trim(),
          accessKey: s3.accessKey,
          secretKey: s3.secretKey,
          secure: s3.secure ? "true" : "false",
        };
      }
      await backupsApi.create(projectId, body);
      toast.success(`Backup config created for ${dbName(dbId)}`);
      // Reset the form but keep the selected database for quick follow-ups.
      setCron(""); setCronPreset(""); setDestination("local"); setEnabled(true); setS3(blankS3());
      load();
    } catch (e) {
      toast.error("Create failed: " + (e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function run(b: BackupSummary) {
    setRunning(b.id);
    try {
      const r = await backupsApi.run(b.id);
      toast.success("Backup saved: " + r.path);
      load();
    } catch (e) {
      toast.error("Backup failed: " + (e as Error).message);
    } finally {
      setRunning(null);
    }
  }

  return (
    <>
      <Card title="Backup schedules">
        {loading ? (
          <div className="space-y-2">{[0, 1].map((i) => <div key={i} className="h-9 rounded-md bg-muted animate-pulse" />)}</div>
        ) : err ? (
          <ErrorBox error={err} />
        ) : backups.length === 0 ? (
          <Empty text="No backup configs yet. Create one below to schedule dumps or back up on demand." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Database</TableHead>
                <TableHead>Destination</TableHead>
                <TableHead>Schedule</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Last run</TableHead>
                <TableHead>Result</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {backups.map((b) => (
                <TableRow key={b.id}>
                  <TableCell>{dbName(b.databaseId)}</TableCell>
                  <TableCell>
                    <Badge variant={b.destination === "s3" ? "default" : "outline"}>{b.destination}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{b.cron || <span className="text-muted-foreground">manual</span>}</TableCell>
                  <TableCell>{b.enabled ? <StatusPill status="enabled" /> : <span className="text-muted-foreground text-xs">off</span>}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{b.lastRunAt ? new Date(b.lastRunAt).toLocaleString() : "never"}</TableCell>
                  <TableCell>
                    {b.lastStatus ? <StatusPill status={b.lastStatus} /> : "—"}
                    {b.lastError && <div className="mt-0.5 max-w-[16rem] truncate text-xs text-destructive" title={b.lastError}>{b.lastError}</div>}
                    {!b.lastError && b.lastPath && <div className="mt-0.5 max-w-[16rem] truncate font-mono text-xs text-muted-foreground" title={b.lastPath}>{b.lastPath}</div>}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button size="sm" variant="outline" onClick={() => run(b)} disabled={running === b.id}>
                      {running === b.id ? <Loader2 className="animate-spin" /> : "Run now"}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>

      <Card title="New backup config">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <Label>Database</Label>
            <Select value={dbId} onChange={(e) => setDbId(e.target.value)}>
              <option value="">select…</option>
              {dbs.map((d) => <option key={d.id} value={d.id}>{d.name} ({d.engine})</option>)}
            </Select>
          </div>
          <div>
            <Label>Schedule</Label>
            <Select
              value={cronPreset}
              onChange={(e) => {
                const v = e.target.value;
                setCronPreset(v);
                if (v !== "custom") setCron(v);
              }}
            >
              {CRON_PRESETS.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
              <option value="custom">Custom…</option>
            </Select>
          </div>
        </div>

        {cronPreset === "custom" && (
          <div className="mt-3">
            <Label>Cron expression</Label>
            <Input value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 2 * * *" className="font-mono" />
            <p className="mt-1 text-xs text-muted-foreground">Standard 5-field cron (min hour day month weekday). Leave blank for manual-only.</p>
          </div>
        )}

        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          <div>
            <Label>Destination</Label>
            <Select value={destination} onChange={(e) => setDestination(e.target.value)}>
              <option value="local">Local (server data dir)</option>
              <option value="s3">S3-compatible bucket</option>
            </Select>
          </div>
          <label className="flex items-end gap-2 pb-2 text-sm">
            <Checkbox checked={enabled} onCheckedChange={(v) => setEnabled(v === true)} />
            <span>Enable scheduled runs</span>
          </label>
        </div>

        {destination === "s3" && (
          <div className="mt-3 rounded-md border border-border bg-muted/40 p-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label>Endpoint</Label>
                <Input value={s3.endpoint} onChange={(e) => setS3({ ...s3, endpoint: e.target.value })} placeholder="s3.amazonaws.com" />
              </div>
              <div>
                <Label>Bucket</Label>
                <Input value={s3.bucket} onChange={(e) => setS3({ ...s3, bucket: e.target.value })} placeholder="my-backups" />
              </div>
              <div>
                <Label>Region</Label>
                <Input value={s3.region} onChange={(e) => setS3({ ...s3, region: e.target.value })} placeholder="us-east-1" />
              </div>
              <label className="flex items-end gap-2 pb-2 text-sm">
                <Checkbox checked={s3.secure} onCheckedChange={(v) => setS3({ ...s3, secure: v === true })} />
                <span>Use TLS (HTTPS)</span>
              </label>
              <div>
                <Label>Access key</Label>
                <Input value={s3.accessKey} onChange={(e) => setS3({ ...s3, accessKey: e.target.value })} autoComplete="off" />
              </div>
              <div>
                <Label>Secret key</Label>
                <Input type="password" value={s3.secretKey} onChange={(e) => setS3({ ...s3, secretKey: e.target.value })} autoComplete="new-password" />
              </div>
            </div>
            <p className="mt-2 text-xs text-muted-foreground">Credentials are stored server-side and never shown back for security.</p>
          </div>
        )}

        <div className="mt-4">
          <Button onClick={create} disabled={!dbId || saving}>
            {saving ? <Loader2 className="animate-spin" /> : <Plus />}
            Create backup config
          </Button>
        </div>
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
//   • The backend models `domain` as nullable: sending an empty string
//     explicitly clears the host, so "Remove" both disables TLS AND detaches
//     the domain (removed from the ingress on the next deploy).
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
