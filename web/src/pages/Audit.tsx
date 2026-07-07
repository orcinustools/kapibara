import { useEffect, useState } from "react";
import { api } from "../api";
import { Card, Empty, ErrorBox } from "../ui";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "@/components/ui/sonner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

/** Placeholder rows shown while a table's data is loading. */
function TableSkeleton({ rows = 4, cols }: { rows?: number; cols: number }) {
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

export function AuditPage() {
  const [logs, setLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    api.get("/audit")
      .then((r) => setLogs(r.auditLogs || []))
      .catch((e) => { setErr(e.message); toast.error("Failed to load audit log"); })
      .finally(() => setLoading(false));
  }, []);
  return (
    <>
      <div className="mb-6"><h1 className="text-xl font-semibold">Audit log</h1></div>
      <ErrorBox error={err} />
      <Card>
        {loading ? (
          <TableSkeleton cols={4} />
        ) : logs.length === 0 ? (
          !err && <Empty text="No audit entries." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>User</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="text-muted-foreground">{new Date(l.createdAt).toLocaleString()}</TableCell>
                  <TableCell>{l.email}</TableCell>
                  <TableCell className="font-mono text-xs">{l.action}</TableCell>
                  <TableCell>{l.status}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </>
  );
}
