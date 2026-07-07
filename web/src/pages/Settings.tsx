import { useEffect, useState } from "react";
import { Trash2 } from "lucide-react";
import { api, membersApi, gitApi, type Member, type MemberRole, type GitProvider, type GitProviderType } from "../api";
import { useAuth } from "../auth";
import { Card, Empty, ErrorBox } from "../ui";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { toast } from "@/components/ui/sonner";

export function SettingsPage() {
  return (
    <>
      <div className="mb-6"><h1 className="text-xl font-semibold">Settings</h1></div>
      <TwoFA />
      <ApiTokens />
      <Members />
      <GitProviders />
      <Notifications />
    </>
  );
}

function GitProviders() {
  const [orgs, setOrgs] = useState<any[]>([]);
  const [orgId, setOrgId] = useState("");
  const [list, setList] = useState<GitProvider[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [form, setForm] = useState<{ type: GitProviderType; name: string; token: string; baseUrl: string }>(
    { type: "github", name: "", token: "", baseUrl: "" },
  );
  const [busy, setBusy] = useState(false);

  async function loadOrgs() {
    const r = await api.get("/orgs");
    setOrgs(r.organizations || []);
    if (r.organizations?.[0]) setOrgId(r.organizations[0].id);
  }
  async function load(oid: string) {
    if (!oid) return;
    setErr(null);
    try { setList(await gitApi.list(oid)); } catch (e) { setErr((e as Error).message); }
  }
  useEffect(() => { loadOrgs(); }, []);
  useEffect(() => { load(orgId); }, [orgId]);
  useEffect(() => {
    // Surface the OAuth callback result (?git=connected) after a redirect.
    if (new URLSearchParams(window.location.search).get("git") === "connected") {
      toast.success("Git provider connected");
      window.history.replaceState({}, "", window.location.pathname);
    }
  }, []);

  async function connect() {
    const token = form.token.trim();
    if (!token) { toast.error("Enter a personal access token"); return; }
    setBusy(true);
    try {
      await gitApi.connectPAT(orgId, { type: form.type, name: form.name || form.type, token, baseUrl: form.baseUrl || undefined });
      toast.success("Provider connected");
      setForm({ type: "github", name: "", token: "", baseUrl: "" });
      load(orgId);
    } catch (e) { toast.error((e as Error).message); }
    finally { setBusy(false); }
  }
  async function oauth() {
    try {
      const { authorizeUrl } = await gitApi.oauthStart(orgId, form.type, { name: form.name, baseUrl: form.baseUrl || undefined });
      window.location.href = authorizeUrl;
    } catch (e) { toast.error((e as Error).message); }
  }
  async function remove(p: GitProvider) {
    try { await gitApi.remove(p.id); toast.success(`Disconnected ${p.name}`); load(orgId); }
    catch (e) { toast.error((e as Error).message); }
  }

  return (
    <Card title="Git providers">
      <ErrorBox error={err} />
      {orgs.length > 1 && (
        <div className="mb-3 max-w-56">
          <Label>Organization</Label>
          <Select value={orgId} onChange={(e) => setOrgId(e.target.value)}>
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </Select>
        </div>
      )}
      {list.length === 0 ? <Empty text="No git providers connected." /> : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Account</TableHead>
              <TableHead>Auth</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((p) => (
              <TableRow key={p.id}>
                <TableCell>{p.name || "—"}</TableCell>
                <TableCell>{p.type}</TableCell>
                <TableCell className="text-muted-foreground">{p.accountLogin || "—"}</TableCell>
                <TableCell><Badge variant="outline">{p.authKind}</Badge></TableCell>
                <TableCell className="text-right">
                  <Button size="icon" variant="destructive" onClick={() => remove(p)} title="Disconnect">
                    <Trash2 className="size-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <div className="mt-4 grid gap-2 sm:grid-cols-2">
        <div>
          <Label>Type</Label>
          <Select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value as GitProviderType })}>
            {["github", "gitlab"].map((x) => <option key={x} value={x}>{x}</option>)}
          </Select>
        </div>
        <div>
          <Label>Name (optional)</Label>
          <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="e.g. Acme GitHub" />
        </div>
        <div>
          <Label>Personal access token</Label>
          <Input type="password" value={form.token} onChange={(e) => setForm({ ...form, token: e.target.value })} placeholder="ghp_… / glpat-…" />
        </div>
        <div>
          <Label>Base URL (self-hosted, optional)</Label>
          <Input value={form.baseUrl} onChange={(e) => setForm({ ...form, baseUrl: e.target.value })} placeholder="https://git.example.com" />
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button onClick={connect} disabled={!orgId || busy}>Connect with token</Button>
        <Button variant="outline" onClick={oauth} disabled={!orgId}>Connect with OAuth</Button>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        The token is stored securely and never shown again. OAuth requires server-side app credentials.
      </p>
    </Card>
  );
}

const ROLES: MemberRole[] = ["owner", "admin", "member"];

function Members() {
  const { user } = useAuth();
  const [orgs, setOrgs] = useState<any[]>([]);
  const [orgId, setOrgId] = useState("");
  const [members, setMembers] = useState<Member[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<MemberRole>("member");

  async function loadOrgs() {
    const r = await api.get("/orgs");
    setOrgs(r.organizations || []);
    if (r.organizations?.[0]) setOrgId(r.organizations[0].id);
  }
  async function load(oid: string) {
    if (!oid) return;
    setErr(null);
    try { setMembers(await membersApi.list(oid)); }
    catch (e) { setErr((e as Error).message); }
  }
  useEffect(() => { loadOrgs(); }, []);
  useEffect(() => { load(orgId); }, [orgId]);

  // Derive the caller's role in the selected org to gate management controls.
  const myRole = members.find((m) => m.email === user?.email)?.role;
  const canManage = myRole === "owner" || myRole === "admin";
  const ownerCount = members.filter((m) => m.role === "owner").length;

  async function add() {
    const e = email.trim();
    if (!e) { toast.error("Enter an email"); return; }
    try {
      await membersApi.add(orgId, e, role);
      toast.success(`Added ${e}`);
      setEmail(""); setRole("member"); load(orgId);
    } catch (ex) { toast.error((ex as Error).message); }
  }
  async function changeRole(m: Member, next: MemberRole) {
    try { await membersApi.setRole(orgId, m.userId, next); toast.success("Role updated"); load(orgId); }
    catch (ex) { toast.error((ex as Error).message); }
  }
  async function remove(m: Member) {
    try { await membersApi.remove(orgId, m.userId); toast.success(`Removed ${m.email}`); load(orgId); }
    catch (ex) { toast.error((ex as Error).message); }
  }

  return (
    <Card title="Organization members">
      <ErrorBox error={err} />
      {orgs.length > 1 && (
        <div className="mb-3 max-w-56">
          <Label>Organization</Label>
          <Select value={orgId} onChange={(e) => setOrgId(e.target.value)}>
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </Select>
        </div>
      )}
      {members.length === 0 ? <Empty text="No members." /> : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Email</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Role</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {members.map((m) => {
              const isLastOwner = m.role === "owner" && ownerCount <= 1;
              return (
                <TableRow key={m.userId}>
                  <TableCell>{m.email}{m.email === user?.email && <span className="ml-1 text-muted-foreground">(you)</span>}</TableCell>
                  <TableCell className="text-muted-foreground">{m.name || "—"}</TableCell>
                  <TableCell>
                    {canManage && !isLastOwner ? (
                      <Select value={m.role} onChange={(e) => changeRole(m, e.target.value as MemberRole)} className="max-w-32">
                        {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
                      </Select>
                    ) : <Badge variant="outline">{m.role}</Badge>}
                  </TableCell>
                  <TableCell className="text-right">
                    {canManage && !isLastOwner && (
                      <Button size="icon" variant="destructive" onClick={() => remove(m)} title="Remove member">
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
      {canManage ? (
        <div className="mt-4 flex flex-wrap items-end gap-2">
          <div className="flex-1">
            <Label>Add member by email</Label>
            <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="user@example.com" />
          </div>
          <div>
            <Label>Role</Label>
            <Select value={role} onChange={(e) => setRole(e.target.value as MemberRole)} className="max-w-32">
              {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
            </Select>
          </div>
          <Button onClick={add} disabled={!orgId}>Add</Button>
        </div>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">Only owners and admins can manage members. The user must already have an account.</p>
      )}
    </Card>
  );
}

function TwoFA() {
  const { user, refresh } = useAuth();
  const [secret, setSecret] = useState<string | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [disableOpen, setDisableOpen] = useState(false);
  const [disableCode, setDisableCode] = useState("");

  async function enroll() {
    setErr(null);
    const r = await api.post("/auth/2fa/enroll");
    setSecret(r.secret); setUrl(r.otpauthUrl);
  }
  async function verify() {
    setErr(null);
    try { await api.post("/auth/2fa/verify", { code }); setSecret(null); toast.success("Two-factor authentication enabled"); await refresh(); }
    catch (e) { setErr((e as Error).message); }
  }
  async function disable() {
    const c = disableCode.trim();
    if (!c) return;
    try {
      await api.post("/auth/2fa/disable", { code: c });
      toast.success("Two-factor authentication disabled");
      setDisableOpen(false); setDisableCode("");
      await refresh();
    } catch (e) { toast.error((e as Error).message); }
  }

  return (
    <Card title="Two-factor authentication (TOTP)">
      <ErrorBox error={err} />
      {user?.twoFAEnabled ? (
        <div className="flex items-center gap-3">
          <Badge variant="success">Enabled</Badge>
          <Button size="sm" variant="destructive" onClick={() => { setDisableCode(""); setDisableOpen(true); }}>Disable</Button>
        </div>
      ) : secret ? (
        <>
          <p className="text-sm text-muted-foreground">
            Add this secret to your authenticator app, then enter a code to confirm.
          </p>
          <dl className="mt-2 grid grid-cols-[130px_1fr] gap-x-4 gap-y-1.5 text-sm">
            <dt className="text-muted-foreground">Secret</dt>
            <dd className="font-mono">{secret}</dd>
          </dl>
          <div className="my-2 break-all font-mono text-[0.72rem] text-muted-foreground">{url}</div>
          <Label>Code</Label>
          <Input value={code} onChange={(e) => setCode(e.target.value)} placeholder="123456" className="max-w-40" />
          <div><Button className="mt-3" onClick={verify}>Enable 2FA</Button></div>
        </>
      ) : (
        <Button onClick={enroll}>Enable 2FA</Button>
      )}
      <Dialog open={disableOpen} onOpenChange={(o) => { if (!o) { setDisableOpen(false); setDisableCode(""); } }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Disable two-factor authentication</DialogTitle>
            <DialogDescription>Enter a current code from your authenticator app to confirm.</DialogDescription>
          </DialogHeader>
          <div>
            <Label>Current 2FA code</Label>
            <Input value={disableCode} onChange={(e) => setDisableCode(e.target.value)} placeholder="123456" className="max-w-40" autoFocus />
          </div>
          <DialogFooter>
            <Button variant="secondary" onClick={() => { setDisableOpen(false); setDisableCode(""); }}>Cancel</Button>
            <Button variant="destructive" onClick={disable} disabled={!disableCode.trim()}>Disable</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
        <div className="mb-3 rounded-md border border-success/40 bg-success/10 px-3 py-2 text-sm text-success">
          Copy your token now (shown once): <span className="font-mono">{created}</span>
        </div>
      )}
      {tokens.length === 0 ? <Empty text="No tokens." /> : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Last used</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tokens.map((t) => (
              <TableRow key={t.id}>
                <TableCell>{t.name || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{t.lastUsed || "never"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Input placeholder="token name" value={name} onChange={(e) => setName(e.target.value)} className="max-w-56" />
        <Button onClick={create}>Create token</Button>
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
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>On</TableHead>
              <TableHead className="w-0" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.map((n) => (
              <TableRow key={n.id}>
                <TableCell>{n.name || "—"}</TableCell>
                <TableCell>{n.type}</TableCell>
                <TableCell className="text-muted-foreground">
                  {[n.onSuccess && "success", n.onFailure && "failure"].filter(Boolean).join(", ")}
                </TableCell>
                <TableCell className="text-right">
                  <Button size="icon" variant="destructive" onClick={() => del(n)}>
                    <Trash2 className="size-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      <div className="mt-4 flex flex-wrap items-end gap-2">
        <div>
          <Label>Name</Label>
          <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        </div>
        <div>
          <Label>Type</Label>
          <Select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
            {["slack", "discord", "telegram", "webhook"].map((x) => <option key={x}>{x}</option>)}
          </Select>
        </div>
        <div className="flex-1">
          <Label>{form.type === "telegram" ? "Bot token" : "Webhook URL"}</Label>
          <Input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} />
        </div>
        <Button onClick={create} disabled={!orgId}>Add</Button>
      </div>
    </Card>
  );
}
