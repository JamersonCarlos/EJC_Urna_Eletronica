declare global {
  interface Window {
    __ENV__?: Record<string, string | undefined>;
  }
}

/** URL da API: runtime (env-config.js do container) → build (VITE_API_URL) → local. */
export const API_URL = (
  window.__ENV__?.VITE_API_URL ||
  (import.meta.env.VITE_API_URL as string | undefined) ||
  'http://localhost:8080'
).replace(/\/$/, '');

export const WS_URL = API_URL.replace(/^http/, 'ws');

const TOKEN_KEY = 'urna_admin_token';

export const tokenStore = {
  get: () => localStorage.getItem(TOKEN_KEY),
  set: (t: string) => localStorage.setItem(TOKEN_KEY, t),
  clear: () => localStorage.removeItem(TOKEN_KEY),
};

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

let onUnauthorized: (() => void) | null = null;
export const setUnauthorizedHandler = (fn: () => void) => {
  onUnauthorized = fn;
};

export async function api<T = unknown>(path: string, opts: { method?: string; body?: unknown; auth?: boolean } = {}): Promise<T> {
  // em upload (FormData) o navegador define o Content-Type com o boundary
  const isForm = opts.body instanceof FormData;
  const headers: Record<string, string> = isForm ? {} : { 'Content-Type': 'application/json' };
  const token = tokenStore.get();
  if (opts.auth !== false && token) headers.Authorization = `Bearer ${token}`;

  let res: Response;
  try {
    res = await fetch(API_URL + path, {
      method: opts.method || 'GET',
      headers,
      body: isForm ? (opts.body as FormData) : opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
  } catch {
    throw new ApiError(0, 'Sem conexão com o servidor');
  }
  if (res.status === 401 && opts.auth !== false && onUnauthorized) onUnauthorized();
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new ApiError(res.status, data?.error || `Erro ${res.status}`);
  return data as T;
}

export const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));
