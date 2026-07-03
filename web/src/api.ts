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

async function req<T = any>(method: string, path: string, body?: any): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const tok = getToken();
  if (tok) headers["Authorization"] = "Bearer " + tok;
  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
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
  get: <T = any>(p: string) => req<T>("GET", p),
  post: <T = any>(p: string, b?: any) => req<T>("POST", p, b),
  put: <T = any>(p: string, b?: any) => req<T>("PUT", p, b),
  del: <T = any>(p: string, b?: any) => req<T>("DELETE", p, b),
};

// Raw text fetch (used for log streaming / plain-text responses).
export async function getText(path: string): Promise<string> {
  const res = await fetch(BASE + path, {
    headers: { Authorization: "Bearer " + getToken() },
  });
  return res.text();
}

export async function health(): Promise<{ engineHealthy: boolean; status: string }> {
  try {
    const res = await fetch("/healthz");
    return res.json();
  } catch {
    return { engineHealthy: false, status: "down" };
  }
}
