const safeMethods = new Set(['GET', 'HEAD', 'OPTIONS', 'TRACE']);

export class APIError extends Error {
  readonly status: number;
  readonly payload: unknown;

  constructor(message: string, status: number, payload: unknown) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.payload = payload;
  }
}

export function getAPIBase(): string {
  return window.localStorage.getItem('megavpn.apiBase')?.trim().replace(/\/$/, '') || '';
}

export function setAPIBase(value: string): void {
  const normalized = value.trim().replace(/\/$/, '');
  if (normalized) {
    window.localStorage.setItem('megavpn.apiBase', normalized);
  } else {
    window.localStorage.removeItem('megavpn.apiBase');
  }
}

export function apiURL(path: string): string {
  return `${getAPIBase()}${path}`;
}

type RequestOptions = RequestInit & {
  parseAs?: 'json' | 'text' | 'empty';
};

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = String(options.method || 'GET').toUpperCase();
  const headers = new Headers(options.headers || {});
  headers.set('Accept', headers.get('Accept') || 'application/json');
  if (!safeMethods.has(method)) {
    headers.set('X-MegaVPN-CSRF', '1');
  }

  const response = await fetch(apiURL(path), {
    credentials: 'include',
    ...options,
    method,
    headers,
  });

  const contentType = response.headers.get('content-type') || '';
  const parseAs = options.parseAs || (contentType.includes('application/json') ? 'json' : 'text');
  const payload = parseAs === 'empty'
    ? null
    : parseAs === 'json'
      ? await response.json().catch(() => null)
      : await response.text().catch(() => '');

  if (!response.ok) {
    const message = typeof payload === 'object' && payload && 'error' in payload
      ? String((payload as { error?: unknown }).error || '')
      : typeof payload === 'string' && payload
        ? payload
        : `${path}: HTTP ${response.status}`;
    throw new APIError(message, response.status, payload);
  }

  return payload as T;
}

export function sendJSON<T>(path: string, method: string, payload?: unknown): Promise<T> {
  return apiRequest<T>(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: payload == null ? null : JSON.stringify(payload),
  });
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof APIError && error.status === 401;
}

export function isForbidden(error: unknown): boolean {
  return error instanceof APIError && error.status === 403;
}
