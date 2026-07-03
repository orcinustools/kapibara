import { useEffect, useState } from "react";
import { api } from "../api";
import { Card, Empty, ErrorBox, StatusPill } from "../ui";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "@/components/ui/sonner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

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

export function ClusterPage() {
  const [nodes, setNodes] = useState<any[]>([]);
  const [cluster, setCluster] = useState<any>(null);
  const [plugins, setPlugins] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    try {
      const n = await api.get("/nodes"); setNodes(n.nodes || []);
      const c = await api.get("/cluster"); setCluster(c);
      const p = await api.get("/plugins"); setPlugins(p.plugins || []);
      setErr(null);
    } catch (e) {
      setErr((e as Error).message);
      toast.error("Failed to load cluster info");
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function togglePlugin(p: any) {
    try {
      if (p.installed) {
        await api.del(`/plugins/${p.name}`);
        toast.success(`Removed ${p.name}`);
      } else {
        await api.post(`/plugins/${p.name}`, {});
        toast.success(`Installing ${p.name}…`);
      }
      setTimeout(load, 500);
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  return (
    <>
      <div className="mb-6"><h1 className="text-xl font-semibold">Cluster</h1></div>
      <ErrorBox error={err} />
      {loading ? (
        <>
          <Card title="Cluster"><TableSkeleton rows={2} cols={2} /></Card>
          <Card title="Nodes"><TableSkeleton cols={5} /></Card>
          <Card title="Plugins / add-ons"><TableSkeleton cols={4} /></Card>
        </>
      ) : (
        <>
          {cluster && (
            <Card title="Cluster">
              <dl className="grid grid-cols-[130px_1fr] gap-x-4 gap-y-1.5 text-sm">
                <dt className="text-muted-foreground">Name</dt>
                <dd>{cluster.name}</dd>
                <dt className="text-muted-foreground">Kubeconfig</dt>
                <dd className="font-mono text-xs">{cluster.kubeconfig}</dd>
              </dl>
            </Card>
          )}
          <Card title="Nodes">
            {nodes.length === 0 ? <Empty text="No nodes / cluster unreachable." /> : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Ready</TableHead>
                    <TableHead>Version</TableHead>
                    <TableHead>Internal IP</TableHead>
                    <TableHead>OS</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {nodes.map((n) => (
                    <TableRow key={n.name}>
                      <TableCell className="font-mono">{n.name}</TableCell>
                      <TableCell><StatusPill status={n.ready ? "Ready" : "NotReady"} /></TableCell>
                      <TableCell>{n.version}</TableCell>
                      <TableCell>{n.internalIP}</TableCell>
                      <TableCell className="text-muted-foreground">{n.os}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Card>
          <Card title="Plugins / add-ons">
            {plugins.length === 0 ? <Empty text="No plugins available." /> : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead>State</TableHead>
                    <TableHead className="w-0" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {plugins.map((p) => (
                    <TableRow key={p.name}>
                      <TableCell className="font-medium">{p.name}</TableCell>
                      <TableCell className="text-muted-foreground">{p.description}</TableCell>
                      <TableCell>
                        {p.installed ? (
                          <StatusPill status={p.ready ? "Ready" : "running"} />
                        ) : (
                          <Badge variant="outline">not installed</Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          size="sm"
                          variant={p.installed ? "destructive" : "secondary"}
                          onClick={() => togglePlugin(p)}
                        >
                          {p.installed ? "Remove" : "Install"}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Card>
        </>
      )}
    </>
  );
}
