import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from './client';
import {
  downloadNodeBootstrapBundle,
  createEnrollmentToken,
  extractEnrollmentTokenSecret,
  getNodeStaleRotationPreview,
  listEnrollmentTokens,
  parseContentDispositionFilename,
  revealNodeBootstrapBundle,
  revokeNodeAgentIdentity,
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

describe('node enrollment token endpoints', () => {
  const calls: FetchCall[] = [];

  beforeEach(() => {
    calls.length = 0;
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
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/enrollment-tokens') {
        return json([{ id: 'token-safe', node_id: 'node-1', token_hint: 'safe...hint', status: 'active', expires_at: '2026-07-10T08:00:00Z', created_at: '2026-07-09T08:00:00Z' }]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/enrollment-token') {
        return json({ id: 'token-new', node_id: 'node-1', token: 'test-secret-token', token_hint: 'test...token', status: 'active', expires_at: '2026-07-10T08:00:00Z', created_at: '2026-07-09T08:00:00Z' }, 201);
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('keeps token list handling redacted and uses CSRF for issue requests', async () => {
    const listed = await listEnrollmentTokens('node-1');
    expect(listed[0]).toMatchObject({ id: 'token-safe', token_hint: 'safe...hint', status: 'active' });
    expect(Object.prototype.hasOwnProperty.call(listed[0], 'token')).toBe(false);
    expect(Object.prototype.hasOwnProperty.call(listed[0], 'enrollment_token')).toBe(false);

    const issued = await createEnrollmentToken('node-1', { ttl_hours: 12 });
    expect(extractEnrollmentTokenSecret(issued)).toBe('test-secret-token');
    expect(calls.find((call) => call.method === 'POST')).toMatchObject({
      method: 'POST',
      path: '/api/v1/nodes/node-1/enrollment-token?ttl_hours=12',
      body: {},
      credentials: 'include',
    });
    expect(calls.find((call) => call.method === 'POST')?.headers['x-megavpn-csrf']).toBe('1');
  });

  it('rejects empty issue responses without stringifying the response', () => {
    expect(() => extractEnrollmentTokenSecret({ id: 'token-empty', node_id: 'node-1', token_hint: 'empty', status: 'active' })).toThrow('enrollment token value was not returned');
  });

  it('accepts the legacy secret-bearing response alias only in issue responses', () => {
    expect(extractEnrollmentTokenSecret({
      id: 'token-alias',
      node_id: 'node-1',
      enrollment_token: 'alias-secret-token',
      token_hint: 'alias...token',
      status: 'active',
    })).toBe('alias-secret-token');
  });
});

describe('node stale rotation preview endpoint', () => {
  const calls: FetchCall[] = [];

  beforeEach(() => {
    calls.length = 0;
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
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node%2Fone/diagnostics/stale-rotation') {
        return json({
          node_id: 'node/one',
          stale_rotation_detected: true,
          token_rotation_status: 'rotating',
          evaluated_at: '2026-07-14T08:00:00Z',
          candidates: [{
            job_id: 'job-1',
            status: 'running',
            created_at: '2026-07-14T07:45:00Z',
            started_at: '2026-07-14T07:46:00Z',
            age_seconds: 900,
            stale_reason: 'claimed_without_result_and_agent_inactive',
            safe_to_clear: true,
          }],
        });
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('uses a read-only GET path without CSRF, body or destructive cleanup calls', async () => {
    const result = await getNodeStaleRotationPreview('node/one');

    expect(result.node_id).toBe('node/one');
    expect(result.candidates[0]).toMatchObject({
      job_id: 'job-1',
      stale_reason: 'claimed_without_result_and_agent_inactive',
      safe_to_clear: true,
    });
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      method: 'GET',
      path: '/api/v1/nodes/node%2Fone/diagnostics/stale-rotation',
      credentials: 'include',
    });
    expect(calls[0].body).toBeUndefined();
    expect(calls[0].headers.accept).toBe('application/json');
    expect(calls[0].headers['x-megavpn-csrf']).toBeUndefined();
    expect(calls.some((call) => call.method === 'POST')).toBe(false);
    expect(calls.some((call) => call.path.includes('clear-stale-rotation'))).toBe(false);
  });
});

describe('node agent identity revoke endpoint', () => {
  const calls: FetchCall[] = [];

  beforeEach(() => {
    calls.length = 0;
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

      if (method === 'POST' && url.pathname === '/api/v1/nodes/node%2Fone/agent-identity/revoke') {
        if (body?.reason === 'return conflict') {
          return json({ status: 'error', code: 'node_agent_revoke_conflict', error: 'secret_ref raw backend text' }, 409);
        }
        return json({
          status: 'revoked',
          node_id: 'node/one',
          agent_status: 'revoked',
          revoked_at: '2026-07-14T08:30:00Z',
          already_revoked: false,
          revoked_enrollment_tokens: 1,
        });
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('uses exact POST path and request body without UI-only or secret-like fields', async () => {
    const result = await revokeNodeAgentIdentity('node/one', {
      confirmation: 'Edge One',
      reason: 'incident response',
    });

    expect(result).toEqual({
      status: 'revoked',
      node_id: 'node/one',
      agent_status: 'revoked',
      revoked_at: '2026-07-14T08:30:00Z',
      already_revoked: false,
      revoked_enrollment_tokens: 1,
    });
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      method: 'POST',
      path: '/api/v1/nodes/node%2Fone/agent-identity/revoke',
      body: {
        confirmation: 'Edge One',
        reason: 'incident response',
      },
      credentials: 'include',
    });
    expect(Object.keys(calls[0].body || {}).sort()).toEqual(['confirmation', 'reason']);
    expect(calls[0].body).not.toHaveProperty('acknowledged');
    expect(calls[0].body).not.toHaveProperty('node_id');
    expect(calls[0].headers.accept).toBe('application/json');
    expect(calls[0].headers['content-type']).toBe('application/json');
    expect(calls[0].headers['x-megavpn-csrf']).toBe('1');
    expect(JSON.stringify(result)).not.toMatch(/token_hash|token_hint|secret_ref|signature|nonce|authorization|enrollment_token_ids/i);
    expect(calls.some((call) => call.path.endsWith('/reboot'))).toBe(false);
    expect(calls.some((call) => call.path.endsWith('/emergency-cleanup'))).toBe(false);
    expect(calls.some((call) => call.path.includes('clear-stale-rotation'))).toBe(false);
  });

  it('keeps already_revoked typed and preserves APIError on backend conflicts', async () => {
    vi.mocked(fetch).mockImplementationOnce(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({
        method,
        path: `${url.pathname}${url.search}`,
        body,
        headers: trackedHeaders(init?.headers),
        credentials: init?.credentials,
        cache: init?.cache,
      });
      return json({
        status: 'revoked',
        node_id: 'node/one',
        agent_status: 'revoked',
        revoked_at: '2026-07-14T08:31:00Z',
        already_revoked: true,
        revoked_enrollment_tokens: 0,
      });
    });
    const already = await revokeNodeAgentIdentity('node/one', { confirmation: 'Edge One', reason: 'second review' });
    expect(already.already_revoked).toBe(true);
    expect(already.revoked_enrollment_tokens).toBe(0);

    await expect(revokeNodeAgentIdentity('node/one', { confirmation: 'Edge One', reason: 'return conflict' })).rejects.toBeInstanceOf(APIError);
    expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node%2Fone/agent-identity/revoke')).toHaveLength(2);
  });
});
