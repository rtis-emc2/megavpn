import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiBlobRequest, apiRequest, apiURL } from './client';

describe('same-origin API boundary', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('accepts application-relative paths only', () => {
    expect(apiURL('/api/v1/ready?probe=1')).toBe('/api/v1/ready?probe=1');

    for (const path of [
      'api/v1/ready',
      'https://attacker.example/api',
      '//attacker.example/api',
      '/\\attacker.example/api',
      '/api/v1/ready\nX-Injected: true',
    ]) {
      expect(() => apiURL(path)).toThrow('API path must be same-origin and absolute');
    }
  });

  it('does not allow callers to disable credentials or blob cache protection', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ status: 'ok' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response('artifact', {
        status: 200,
        headers: { 'content-type': 'application/octet-stream' },
      }));
    vi.stubGlobal('fetch', fetchMock);

    await apiRequest('/api/v1/ready', { credentials: 'omit' });
    await apiBlobRequest('/api/v1/artifacts/example', { credentials: 'omit', cache: 'force-cache' });

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ credentials: 'include' });
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({ credentials: 'include', cache: 'no-store' });
  });
});
