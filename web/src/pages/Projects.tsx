import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Trash2 } from "lucide-react";
import { api } from "../api";
import { Card, Empty, ErrorBox } from "../ui";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "@/components/ui/sonner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

interface Org { id: string; name: string; slug: string; }
interface Project { id: string; name: string; orcinusProject: string; description: string; }

/** Placeholder rows shown while a table's data is loading. */
function TableSkeleton({ rows = 3, cols }: { rows?: number; cols: number }) {
  return (
    <div className="space-y-2" aria-hidden>
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex gap-3">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton key={c} className="h-8 flex-1" />
          ))}
        </div>
      ))}
    </div>
  );
}

export function ProjectsPage() {
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [orgId, setOrgId] = useState("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  async function loadOrgs() {
    try {
      const r = await api.get<{ organizations: Org[] }>("/orgs");
      setOrgs(r.organizations || []);
      if (!orgId && r.organizations?.[0]) setOrgId(r.organizations[0].id);
      else if (!r.organizations?.length) setLoading(false);
    } catch (e) {
      setErr((e as Error).message);
      toast.error("Failed to load organizations");
      setLoading(false);
    }
  }
  async function loadProjects(oid: string) {
    if (!oid) return;
    setLoading(true);
    setErr(null);
    try {
      const r = await api.get<{ projects: Project[] }>(`/orgs/${oid}/projects`);
      setProjects(r.projects || []);
    } catch (e) {
      setErr((e as Error).message);
      toast.error("Failed to load projects");
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { loadOrgs(); }, []);
  useEffect(() => { if (orgId) loadProjects(orgId); }, [orgId]);

  async function create() {
    setErr(null);
    try {
      await api.post(`/orgs/${orgId}/projects`, { name });
      toast.success(`Project “${name}” created`);
      setName("");
      loadProjects(orgId);
    } catch (e) {
      setErr((e as Error).message);
      toast.error((e as Error).message);
    }
  }
  async function del(id: string) {
    if (!confirm("Delete project and all its cluster resources?")) return;
    try {
      await api.del(`/projects/${id}`);
      toast.success("Project deleted");
      loadProjects(orgId);
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <>
      <div className="mb-6 flex items-center gap-3">
        <h1 className="text-xl font-semibold">Projects</h1>
        <div className="flex-1" />
        {orgs.length > 1 && (
          <Select value={orgId} onChange={(e) => setOrgId(e.target.value)} className="w-52">
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </Select>
        )}
      </div>

      <Card title="Your projects">
        <ErrorBox error={err} />
        {loading ? (
          <TableSkeleton cols={3} />
        ) : projects.length === 0 ? (
          !err && <Empty text="No projects yet — create one below." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Orcinus project</TableHead>
                <TableHead className="w-0" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {projects.map((p) => (
                <TableRow key={p.id}>
                  <TableCell>
                    <Link to={`/projects/${p.id}`} className="font-medium text-primary hover:underline">
                      {p.name}
                    </Link>
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{p.orcinusProject}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="destructive" size="icon" onClick={() => del(p.id)}>
                      <Trash2 className="size-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Input
            placeholder="new project name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="max-w-[260px]"
          />
          <Button onClick={create} disabled={!name || !orgId}>Create project</Button>
        </div>
      </Card>
    </>
  );
}
