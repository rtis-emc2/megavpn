import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from './client';
import {
  downloadNodeBootstrapBundle,
  parseContentDispositionFilename,
  revealNodeBootstrapBundle,
  sanitizeDownloadFilename,
} from './endpoints';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
  headers: Record<string, string>;
  credentials?: RequestCredentials;
  cache?: RequestCache;
};

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function trackedHeaders(headers: HeadersInit | undefined): Record<string, string> {
  const output: Record<string, string> = {};
  new Headers(headers || {}).forEach((value, key) => {
    output[key] = value;
  });
  return output;
}

describe('node bootstrap bundle endpoints', () => {
  const calls: FetchCall[] = [];
  let downloadContentType = 'text/plain';
  let revealContent: string;
  let downloadContent: string;

  beforeEach(() => {
    calls.length = 0;
    downloadContentType = 'text/plain';
    revealContent = '';
    downloadContent = '';
    window.localStorage.clear();
    window.sessionStorage.clear();
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const path = `${url.pathname}${url.search}`;
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({
        method,
        path,
        body,
        headers: trackedHeaders(init?.headers),
        credentials: init?.credentials,
        cache: init?.cache,
      });

      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal') {
        return json({
          node_id: 'node-1',
          bootstrap_run_id: 'run-1',
          filename: 'megavpn-agent-node-1-bootstrap.env',
          agent_bootstrapenv: revealContent,
          revealed_at: '2026-07-09T08:03:00Z',
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download') {
        return new Response(downloadContent, {
          status: 200,
          headers: {
            'content-type': downloadContentType,
            'content-disposition': "attachment; filename*=UTF-8''megavpn-agent-node-1-bootstrap.env",
          },
        });
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('uses the dedicated POST reveal endpoint with CSRF and without secret-ref calls', async () => {
    revealContent = 'MEGAVPN_BOOTSTRAP_MODE=manual\n';

    const result = await revealNodeBootstrapBundle('node-1', 'run-1');

    expect(result).toMatchObject({
      node_id: 'node-1',
      bootstrap_run_id: 'run-1',
      filename: 'megavpn-agent-node-1-bootstrap.env',
      agent_bootstrapenv: 'MEGAVPN_BOOTSTRAP_MODE=manual\n',
    });
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/reveal',
      body: {},
      credentials: 'include',
    });
    expect(calls[0].headers.accept).toBe('application/json');
    expect(calls[0].headers['x-megavpn-csrf']).toBe('1');
    expect(calls.some((call) => call.method === 'GET' && /\/bundle(?:$|\?)/.test(call.path))).toBe(false);
    expect(calls.some((call) => call.path === '/api/v1/secret-refs')).toBe(false);
  });

  it('uses the dedicated POST blob download endpoint and sanitizes filenames', async () => {
    downloadContent = 'MEGAVPN_DOWNLOAD_BUNDLE=manual\n';

    const result = await downloadNodeBootstrapBundle('node-1', 'run-1');

    expect(result.filename).toBe('megavpn-agent-node-1-bootstrap.env');
    expect(result.contentType).toBe('text/plain');
    expect(await result.blob.text()).toBe('MEGAVPN_DOWNLOAD_BUNDLE=manual\n');
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/nodes/node-1/bootstrap-runs/run-1/bundle/download',
      body: {},
      credentials: 'include',
      cache: 'no-store',
    });
    expect(calls[0].headers.accept).toBe('text/plain, application/octet-stream');
    expect(calls[0].headers['content-type']).toBe('application/json');
    expect(calls[0].headers['x-megavpn-csrf']).toBe('1');
  });

  it('rejects HTML download responses and keeps filename parsing path-safe', async () => {
    expect(parseContentDispositionFilename('attachment; filename="bundle.env"')).toBe('bundle.env');
    expect(parseContentDispositionFilename("attachment; filename*=UTF-8''bundle%20one.env")).toBe('bundle one.env');
    expect(sanitizeDownloadFilename('..', 'fallback.env')).toBe('fallback.env');
    expect(sanitizeDownloadFilename('../../operator-token', 'fallback.env')).toBe('operator-token.env');
    expect(sanitizeDownloadFilename('node bootstrap', 'fallback.env')).toBe('node-bootstrap.env');

    downloadContentType = 'text/html';
    await expect(downloadNodeBootstrapBundle('node-1', 'run-1')).rejects.toBeInstanceOf(APIError);
  });
});
