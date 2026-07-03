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
