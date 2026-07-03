import { useEffect, useState } from "react";
import { api } from "../api";
import { Card, Empty, ErrorBox, StatusPill } from "../ui";

export function ClusterPage() {
  const [nodes, setNodes] = useState<any[]>([]);
  const [cluster, setCluster] = useState<any>(null);
  const [plugins, setPlugins] = useState<any[]>([]);
  const [err, setErr] = useState<string | null>(null);

  async function load() {
    try {
      const n = await api.get("/nodes"); setNodes(n.nodes || []);
      const c = await api.get("/cluster"); setCluster(c);
      const p = await api.get("/plugins"); setPlugins(p.plugins || []);
    } catch (e) { setErr((e as Error).message); }
  }
  useEffect(() => { load(); }, []);

  async function togglePlugin(p: any) {
    if (p.installed) { await api.del(`/plugins/${p.name}`); }
    else { await api.post(`/plugins/${p.name}`, {}); }
    setTimeout(load, 500);
  }

  return (
    <>
      <div className="topbar"><h1>Cluster</h1></div>
      <ErrorBox error={err} />
      {cluster && (
        <Card title="Cluster">
          <div className="kv">
            <div>Name</div><div>{cluster.name}</div>
            <div>Kubeconfig</div><div className="mono">{cluster.kubeconfig}</div>
          </div>
        </Card>
      )}
      <Card title="Nodes">
        {nodes.length === 0 ? <Empty text="No nodes / cluster unreachable." /> : (
          <table>
            <thead><tr><th>Name</th><th>Ready</th><th>Version</th><th>Internal IP</th><th>OS</th></tr></thead>
            <tbody>
              {nodes.map((n) => (
                <tr key={n.name}>
                  <td className="mono">{n.name}</td>
                  <td><StatusPill status={n.ready ? "Ready" : "NotReady"} /></td>
                  <td>{n.version}</td><td>{n.internalIP}</td><td className="muted">{n.os}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card title="Plugins / add-ons">
        <table>
          <thead><tr><th>Name</th><th>Description</th><th>State</th><th></th></tr></thead>
          <tbody>
            {plugins.map((p) => (
              <tr key={p.name}>
                <td>{p.name}</td><td className="muted">{p.description}</td>
                <td>{p.installed ? <StatusPill status={p.ready ? "Ready" : "running"} /> : <span className="pill gray">not installed</span>}</td>
                <td style={{ textAlign: "right" }}>
                  <button className={"sm " + (p.installed ? "danger" : "")} onClick={() => togglePlugin(p)}>
                    {p.installed ? "Remove" : "Install"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </>
  );
}
