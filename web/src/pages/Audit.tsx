import { useEffect, useState } from "react";
import { api } from "../api";
import { Card, Empty, ErrorBox } from "../ui";

export function AuditPage() {
  const [logs, setLogs] = useState<any[]>([]);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    api.get("/audit").then((r) => setLogs(r.auditLogs || [])).catch((e) => setErr(e.message));
  }, []);
  return (
    <>
      <div className="topbar"><h1>Audit log</h1></div>
      <ErrorBox error={err} />
      <Card>
        {logs.length === 0 ? <Empty text="No audit entries." /> : (
          <table>
            <thead><tr><th>When</th><th>User</th><th>Action</th><th>Status</th></tr></thead>
            <tbody>
              {logs.map((l) => (
                <tr key={l.id}>
                  <td className="muted">{new Date(l.createdAt).toLocaleString()}</td>
                  <td>{l.email}</td><td className="mono">{l.action}</td><td>{l.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </>
  );
}
