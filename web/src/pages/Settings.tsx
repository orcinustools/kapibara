import { useEffect, useState } from "react";
import { api } from "../api";
import { useAuth } from "../auth";
import { Card, Empty, ErrorBox } from "../ui";

export function SettingsPage() {
  return (
    <>
      <div className="topbar"><h1>Settings</h1></div>
      <TwoFA />
      <ApiTokens />
      <Notifications />
    </>
  );
}

function TwoFA() {
  const { user, refresh } = useAuth();
  const [secret, setSecret] = useState<string | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [err, setErr] = useState<string | null>(null);

  async function enroll() {
    setErr(null);
    const r = await api.post("/auth/2fa/enroll");
    setSecret(r.secret); setUrl(r.otpauthUrl);
  }
  async function verify() {
    setErr(null);
    try { await api.post("/auth/2fa/verify", { code }); setSecret(null); await refresh(); }
    catch (e) { setErr((e as Error).message); }
  }
  async function disable() {
    const c = prompt("Enter current 2FA code to disable:");
    if (!c) return;
    try { await api.post("/auth/2fa/disable", { code: c }); await refresh(); }
    catch (e) { alert((e as Error).message); }
  }

  return (
    <Card title="Two-factor authentication (TOTP)">
      <ErrorBox error={err} />
      {user?.twoFAEnabled ? (
        <div className="row">
          <span className="pill ok">Enabled</span>
          <button className="sm danger" onClick={disable}>Disable</button>
        </div>
      ) : secret ? (
        <>
          <p className="muted">Add this secret to your authenticator app, then enter a code to confirm.</p>
          <div className="kv"><div>Secret</div><div className="mono">{secret}</div></div>
          <div className="mono muted" style={{ fontSize: ".72rem", wordBreak: "break-all", margin: ".5rem 0" }}>{url}</div>
          <label>Code</label>
          <input value={code} onChange={(e) => setCode(e.target.value)} placeholder="123456" style={{ maxWidth: 160 }} />
          <div><button style={{ marginTop: ".6rem" }} onClick={verify}>Enable 2FA</button></div>
        </>
      ) : (
        <button onClick={enroll}>Enable 2FA</button>
      )}
    </Card>
  );
}

function ApiTokens() {
  const [tokens, setTokens] = useState<any[]>([]);
  const [name, setName] = useState("");
  const [created, setCreated] = useState<string | null>(null);
  async function load() { const r = await api.get("/tokens"); setTokens(r.tokens || []); }
  useEffect(() => { load(); }, []);
  async function create() {
    const r = await api.post("/tokens", { name }); setCreated(r.token); setName(""); load();
  }
  return (
    <Card title="API tokens">
      {created && (
        <div className="err-box" style={{ background: "rgba(63,185,80,.1)", borderColor: "var(--ok)", color: "#a6f0b0" }}>
          Copy your token now (shown once): <span className="mono">{created}</span>
        </div>
      )}
      {tokens.length === 0 ? <Empty text="No tokens." /> : (
        <table>
          <thead><tr><th>Name</th><th>Last used</th></tr></thead>
          <tbody>{tokens.map((t) => <tr key={t.id}><td>{t.name || "—"}</td><td className="muted">{t.lastUsed || "never"}</td></tr>)}</tbody>
        </table>
      )}
      <div className="row" style={{ marginTop: ".8rem" }}>
        <input placeholder="token name" value={name} onChange={(e) => setName(e.target.value)} style={{ maxWidth: 220 }} />
        <button onClick={create}>Create token</button>
      </div>
    </Card>
  );
}

function Notifications() {
  const [orgs, setOrgs] = useState<any[]>([]);
  const [orgId, setOrgId] = useState("");
  const [list, setList] = useState<any[]>([]);
  const [form, setForm] = useState<any>({ name: "", type: "slack", url: "", onSuccess: true, onFailure: true });
  async function loadOrgs() { const r = await api.get("/orgs"); setOrgs(r.organizations || []); if (r.organizations?.[0]) setOrgId(r.organizations[0].id); }
  async function load(oid: string) { if (!oid) return; const r = await api.get(`/orgs/${oid}/notifications`); setList(r.notifications || []); }
  useEffect(() => { loadOrgs(); }, []);
  useEffect(() => { load(orgId); }, [orgId]);
  async function create() {
    const config: Record<string, string> = {};
    if (form.type === "telegram") { config.token = form.url; config.chatId = form.chatId || ""; }
    else config.url = form.url;
    await api.post(`/orgs/${orgId}/notifications`, { name: form.name, type: form.type, config, onSuccess: form.onSuccess, onFailure: form.onFailure });
    setForm({ ...form, name: "", url: "" }); load(orgId);
  }
  async function del(n: any) { await api.del(`/notifications/${n.id}`); load(orgId); }
  return (
    <Card title="Notifications">
      {list.length === 0 ? <Empty text="No notification channels." /> : (
        <table>
          <thead><tr><th>Name</th><th>Type</th><th>On</th><th></th></tr></thead>
          <tbody>
            {list.map((n) => (
              <tr key={n.id}>
                <td>{n.name || "—"}</td><td>{n.type}</td>
                <td className="muted">{[n.onSuccess && "success", n.onFailure && "failure"].filter(Boolean).join(", ")}</td>
                <td style={{ textAlign: "right" }}><button className="sm danger" onClick={() => del(n)}>✕</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="row" style={{ marginTop: ".8rem", alignItems: "flex-end" }}>
        <div><label>Name</label><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></div>
        <div><label>Type</label>
          <select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
            {["slack", "discord", "telegram", "webhook"].map((x) => <option key={x}>{x}</option>)}
          </select>
        </div>
        <div style={{ flex: 1 }}><label>{form.type === "telegram" ? "Bot token" : "Webhook URL"}</label><input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} /></div>
        <button onClick={create} disabled={!orgId}>Add</button>
      </div>
    </Card>
  );
}
