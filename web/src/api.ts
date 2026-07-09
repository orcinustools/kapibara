// Thin fetch wrapper around the kapibara REST API.

const BASE = "/api/v1";

export function getToken(): string {
  return localStorage.getItem("kap_token") || "";
}
export function setToken(t: string) {
  localStorage.setItem("kap_token", t);
}
export function clearToken() {
  localStorage.removeItem("kap_token");
}

export class ApiError extends Error {
  status: number;
  body: any;
  constructor(status: number, body: any) {
    super(body?.error || `HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

async function req<T = any>(method: string, path: string, body?: any, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const tok = getToken();
  if (tok) headers["Authorization"] = "Bearer " + tok;
  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    signal,
  });
  const txt = await res.text();
  let data: any = {};
  try {
    data = txt ? JSON.parse(txt) : {};
  } catch {
    data = { raw: txt };
  }
  if (!res.ok) throw new ApiError(res.status, data);
  return data as T;
}

export const api = {
  // `signal` lets callers abort an in-flight GET (e.g. a polling drawer closing).
  get: <T = any>(p: string, signal?: AbortSignal) => req<T>("GET", p, undefined, signal),
  post: <T = any>(p: string, b?: any) => req<T>("POST", p, b),
  put: <T = any>(p: string, b?: any) => req<T>("PUT", p, b),
  del: <T = any>(p: string, b?: any) => req<T>("DELETE", p, b),
};

// Cluster secrets. The list endpoint returns only a name + key COUNT — the
// values are never sent back, so secret values are inherently masked. PUT
// replaces the whole data map for a given name (create-or-update); there is no
// per-key merge server-side.
export interface SecretSummary {
  name: string;
  keys: number;
}

export const secretsApi = {
  list: (signal?: AbortSignal) =>
    api.get<{ secrets: SecretSummary[] }>("/secrets", signal).then((r) => r.secrets || []),
  put: (name: string, data: Record<string, string>) => api.post("/secrets", { name, data }),
  del: (name: string) => api.del(`/secrets/${encodeURIComponent(name)}`),
};

// Persist an application's environment variables + which keys are secret. Env
// and secretKeys are write-only on the backend (json:"-"), so this is a
// set-on-save (replace) operation — sending both fields fully replaces them.
export function saveAppEnv(appId: string, env: Record<string, string>, secretKeys: string[]) {
  return api.put(`/apps/${appId}`, { env, secretKeys });
}

// Persist an application's ingress domain + TLS. Unlike env, `domain`/`tls`
// ARE returned by the app list/get (json:"domain"/"tls"), so the UI can read
// current values back. Update is via PUT /apps/{id}. The backend models
// `domain` as a nullable field: sending an empty string explicitly CLEARS the
// ingress host (removes the domain on the next deploy); `tls` is always applied.
export function saveAppDomain(appId: string, domain: string, tls: boolean) {
  return api.put(`/apps/${appId}`, { domain, tls });
}

// Org members / RBAC (M1). Members are existing kapibara users linked to an org
// with a role (owner | admin | member). There is no invite flow: `add` links a
// user that already registered, by email. Only owners/admins may add, change
// roles, or remove; the last owner is protected server-side.
export type MemberRole = "owner" | "admin" | "member";
export interface Member {
  userId: string;
  email: string;
  name: string;
  role: MemberRole;
}

export const membersApi = {
  list: (orgId: string, signal?: AbortSignal) =>
    api.get<{ members: Member[] }>(`/orgs/${orgId}/members`, signal).then((r) => r.members || []),
  add: (orgId: string, email: string, role: MemberRole) =>
    api.post<Member>(`/orgs/${orgId}/members`, { email, role }),
  setRole: (orgId: string, userId: string, role: MemberRole) =>
    api.put(`/orgs/${orgId}/members/${userId}`, { role }),
  remove: (orgId: string, userId: string) => api.del(`/orgs/${orgId}/members/${userId}`),
};

// Git providers (M3). A connected source-control account (GitHub/GitLab) whose
// access token is stored write-only server-side (json:"-") and injected into
// clone URLs for private repos. Connect via a personal access token, or via the
// OAuth flow when server credentials are configured. `repos` powers the app
// create repo picker.
export type GitProviderType = "github" | "gitlab";
export interface GitProvider {
  id: string;
  organizationId: string;
  type: GitProviderType;
  name: string;
  accountLogin: string;
  baseUrl: string;
  authKind: string; // pat | oauth
}
export interface GitRepo {
  fullName: string;
  cloneUrl: string;
  private: boolean;
  defaultBranch: string;
}

export const gitApi = {
  list: (orgId: string, signal?: AbortSignal) =>
    api.get<{ providers: GitProvider[] }>(`/orgs/${orgId}/git-providers`, signal).then((r) => r.providers || []),
  connectPAT: (orgId: string, body: { type: GitProviderType; name: string; token: string; baseUrl?: string }) =>
    api.post<GitProvider>(`/orgs/${orgId}/git-providers`, body),
  remove: (providerId: string) => api.del(`/git-providers/${providerId}`),
  repos: (providerId: string, signal?: AbortSignal) =>
    api.get<{ repositories: GitRepo[] }>(`/git-providers/${providerId}/repos`, signal).then((r) => r.repositories || []),
  // Returns { authorizeUrl } when OAuth is configured, else throws (501).
  oauthStart: (orgId: string, type: GitProviderType, params?: { name?: string; baseUrl?: string }) => {
    const q = new URLSearchParams();
    if (params?.name) q.set("name", params.name);
    if (params?.baseUrl) q.set("baseUrl", params.baseUrl);
    const qs = q.toString();
    return api.get<{ authorizeUrl: string }>(`/orgs/${orgId}/git-providers/oauth/${type}/start${qs ? "?" + qs : ""}`);
  },
};

// Database backup configs (M8). A config schedules (or manually triggers) a dump
// of one managed database to a local path or an S3-compatible bucket. NOTE: the
// backend stores `s3Config` as write-only (json:"-"), so the endpoint/credentials
// are ACCEPTED on create but NEVER returned by the list — the UI can't prefill
// them back (same pattern as app env / cluster secrets). `cron`, `destination`,
// `enabled` and the last-run status fields ARE serialized and read back.
export interface BackupSummary {
  id: string;
  databaseId: string;
  cron: string;
  destination: string; // local | s3
  enabled: boolean;
  lastRunAt: string | null;
  lastStatus: string;
  lastPath: string;
  lastError: string;
}

export interface BackupCreate {
  databaseId: string;
  cron: string;
  destination: string; // local | s3
  s3Config?: Record<string, string>;
  enabled: boolean;
}

export const backupsApi = {
  list: (projectId: string, signal?: AbortSignal) =>
    api
      .get<{ backups: BackupSummary[] }>(`/projects/${projectId}/backups`, signal)
      .then((r) => r.backups || []),
  create: (projectId: string, body: BackupCreate) =>
    api.post<BackupSummary>(`/projects/${projectId}/backups`, body),
  run: (backupId: string) =>
    api.post<{ status: string; path: string; backup: BackupSummary }>(`/backups/${backupId}/run`),
};

// Preview deployments (M9). `deploy` pushes an app's chosen branch into an
// ephemeral, isolated orcinus project (per-branch/PR) and returns the tracking
// deployment id + the generated preview project name; `teardown` removes that
// environment. NOTE: the backend does NOT persist or list previews — there is
// no GET route — so the UI tracks triggered previews in-session only and follows
// their progress via the returned deploymentId (GET /deployments/{id}). Teardown
// is keyed by project + branch, not by app.
export interface PreviewDeploy {
  deploymentId: string;
  previewProject: string;
  branch: string;
  status: string;
}

export const previewApi = {
  deploy: (appId: string, branch: string) =>
    api.post<PreviewDeploy>(`/apps/${appId}/preview`, { branch }),
  teardown: (projectId: string, branch: string) =>
    api.del(`/projects/${projectId}/preview/${encodeURIComponent(branch)}`),
};

// Raw text fetch (used for log streaming / plain-text responses).
export async function getText(path: string): Promise<string> {
  const res = await fetch(BASE + path, {
    headers: { Authorization: "Bearer " + getToken() },
  });
  return res.text();
}

// Incrementally consume a chunked/flushed text response (e.g. follow=true logs).
// `onChunk` fires for each decoded piece as it arrives; the promise resolves when
// the server closes the stream. Pass an AbortSignal to stop early — aborting
// rejects with a DOMException whose name is "AbortError", which callers should
// swallow. Always resolves/rejects after releasing the underlying reader, so no
// reader is leaked on unmount or Stop.
export async function streamText(
  path: string,
  opts: { signal: AbortSignal; onChunk: (chunk: string) => void },
): Promise<void> {
  const res = await fetch(BASE + path, {
    headers: { Authorization: "Bearer " + getToken() },
    signal: opts.signal,
  });
  if (!res.ok || !res.body) {
    const txt = await res.text().catch(() => "");
    throw new ApiError(res.status, txt ? { error: txt } : {});
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      if (value) opts.onChunk(decoder.decode(value, { stream: true }));
    }
    const tail = decoder.decode();
    if (tail) opts.onChunk(tail);
  } finally {
    reader.releaseLock();
  }
}

export async function health(): Promise<{ engineHealthy: boolean; status: string }> {
  try {
    const res = await fetch("/healthz");
    return res.json();
  } catch {
    return { engineHealthy: false, status: "down" };
  }
}
