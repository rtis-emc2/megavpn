import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../../shared/auth/AuthProvider';
import i18n from '../../shared/i18n';
import { BackupRestorePage } from './BackupRestorePage';
import { DiagnosticsPage } from './DiagnosticsPage';

type FetchCall = { method: string; path: string };

const authPayload = {
  user: { id: 'admin-1', username: 'admin', email: 'admin@example.test', status: 'active' },
  session: { id: 'current-session', expires_at: '2099-08-11T00:00:00Z' },
  roles: ['superadmin'],
  permissions: ['settings.manage', 'auth.manage'],
};

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), { status, headers: { 'content-type': 'application/json' } });
}

function renderPage(element: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{element}</AuthProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

describe('Operational diagnostics and protected backups', () => {
  const calls: FetchCall[] = [];
  let backups: Record<string, unknown>[];

  beforeEach(async () => {
    calls.length = 0;
    backups = [{
      id: 'backup-20260811T101500Z-1234abcd',
      filename: 'backup-20260811T101500Z-1234abcd.dump',
      size_bytes: 2048,
      sha256: 'a'.repeat(64),
      created_at: '2026-08-11T10:15:00Z',
      verified: true,
    }];
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      calls.push({ method, path: `${url.pathname}${url.search}` });

      if (method === 'GET' && url.pathname === '/api/v1/auth/me') return json(authPayload);
      if (method === 'GET' && url.pathname === '/api/v1/ready') return json({ status: 'ready', version: '8.0.0-pre.2' });
      if (method === 'GET' && url.pathname === '/api/v1/runtime/preflight') {
        return json({
          status: 'degraded',
          version: '8.0.0-pre.2',
          production_mode: true,
          generated_at: '2026-08-11T10:15:00Z',
          checks: [
            { code: 'database', status: 'ok', summary: 'database is reachable' },
            { code: 'secret_storage', status: 'failed', summary: 'secret storage unavailable', detail: 'master key is missing' },
          ],
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/dashboard') return json({ version: '8.0.0-pre.2', jobs_active: 1, jobs_failed: 1 });
      if (method === 'GET' && url.pathname === '/api/v1/nodes') {
        return json([
          { id: 'node-ok', name: 'node-ok', role: 'ingress', status: 'online', agent_status: 'online', address: '192.0.2.10' },
          { id: 'node-broken', name: 'node-broken', role: 'egress', status: 'online', agent_status: 'degraded', address: '192.0.2.11', agent_last_seen_at: '2026-08-11T09:00:00Z' },
        ]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/jobs') {
        return json([
          { id: 'job-failed', type: 'node.inventory.sync', scope_type: 'node', scope_id: 'node-broken', status: 'failed', created_at: '2026-08-11T10:14:00Z', result: { message: 'agent heartbeat stale' } },
          { id: 'job-done', type: 'node.inventory.sync', scope_type: 'node', scope_id: 'node-ok', status: 'succeeded', created_at: '2026-08-11T10:13:00Z' },
        ]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/backups') return json(backups);
      if (method === 'POST' && url.pathname === '/api/v1/backups') {
        const created = { ...backups[0], id: 'backup-20260811T102000Z-5678efab', filename: 'backup-20260811T102000Z-5678efab.dump' };
        backups = [created, ...backups];
        return json(created, 201);
      }
      const verifyMatch = url.pathname.match(/^\/api\/v1\/backups\/([^/]+)\/verify$/);
      if (method === 'POST' && verifyMatch) return json(backups.find((backup) => backup.id === verifyMatch[1]));
      const deleteMatch = url.pathname.match(/^\/api\/v1\/backups\/([^/]+)$/);
      if (method === 'DELETE' && deleteMatch) {
        backups = backups.filter((backup) => backup.id !== deleteMatch[1]);
        return json({ status: 'ok', deleted_backup_id: deleteMatch[1] });
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    cleanup();
  });

  it('explains degraded control-plane, node and job evidence', async () => {
    renderPage(<DiagnosticsPage />);

    expect(await screen.findByText('Control-plane preflight')).toBeInTheDocument();
    expect(screen.getAllByText('Secret storage').length).toBeGreaterThan(0);
    expect(screen.getAllByText('secret storage unavailable').length).toBeGreaterThan(0);
    expect(screen.getAllByText('master key is missing').length).toBeGreaterThan(0);
    expect(screen.getAllByText('node-broken').length).toBeGreaterThan(0);
    expect(screen.getAllByText('agent heartbeat stale').length).toBeGreaterThan(0);
    expect(screen.queryByText('node-ok')).not.toBeInTheDocument();
  });

  it('creates, verifies and deletes protected archives without exposing an online restore action', async () => {
    renderPage(<BackupRestorePage />);

    expect((await screen.findAllByText('backup-20260811T101500Z-1234abcd.dump')).length).toBeGreaterThan(0);
    expect(screen.getByText(/Restore is intentionally not available from the browser/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /restore/i })).not.toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Verify' })[0]);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path.endsWith('/verify'))).toBe(true));
    expect(await screen.findByText(/passed checksum and PostgreSQL format verification/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Create backup' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/backups')).toBe(true));
    expect(await screen.findByText('Backup archive created.')).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
    expect(calls.some((call) => call.method === 'DELETE')).toBe(false);
    await userEvent.click(screen.getAllByRole('button', { name: 'Delete' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE')).toBe(true));
    expect(await screen.findByText('Backup archive deleted.')).toBeInTheDocument();
  });
});
