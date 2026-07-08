import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import { ClientsPage } from './ClientsPage';

type FetchCall = {
  method: string;
  path: string;
  body?: unknown;
};

const client = {
  id: 'client-1',
  username: 'alpha',
  display_name: 'Alpha User',
  email: 'alpha@example.test',
  status: 'active',
  notes: 'Primary test client',
  created_at: '2026-07-08T10:00:00Z',
  updated_at: '2026-07-08T10:05:00Z',
  summary: {
    service_access_count: 1,
    active_service_access_count: 1,
    artifact_count: 1,
    ready_artifact_count: 1,
  },
};

const group = {
  id: 'group-1',
  service_code: 'vless',
  group_key: 'core',
  display_name: 'Core access',
  status: 'active',
  member_count: 1,
  active_member_count: 1,
  affected_instances: 2,
  pending_sync_count: 0,
  failed_sync_count: 0,
  applied_sync_count: 2,
};

const artifact = {
  id: 'artifact-1',
  client_account_id: 'client-1',
  service_access_id: 'access-1',
  artifact_type: 'vless_url',
  status: 'ready',
  size_bytes: 123,
  created_at: '2026-07-08T10:10:00Z',
};

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <ClientsPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return queryClient;
}

async function openClient() {
  renderPage();
  expect((await screen.findAllByText('alpha@example.test')).length).toBeGreaterThan(0);
  await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
  await screen.findByRole('heading', { name: 'Alpha User' });
}

describe('ClientsPage', () => {
  const calls: FetchCall[] = [];
  let openSpy: ReturnType<typeof vi.spyOn>;
  let consoleSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(async () => {
    calls.length = 0;
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/clients') {
        return json([client]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients') {
        if (body.username === 'exists') return json({ status: 'error', error: 'client username already exists' }, 409);
        if (body.username === 'invalid') return json({ status: 'error', error: 'invalid client payload', fields: { username: 'Username is invalid' } }, 422);
        return json({ ...client, id: 'client-2', username: body.username, display_name: body.display_name, email: body.email }, 201);
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1') {
        return json(client);
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-2') {
        return json({ ...client, id: 'client-2', username: 'bravo', display_name: 'Bravo User', email: 'bravo@example.test' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/suspend') {
        return json({ ...client, status: 'suspended' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/activate') {
        return json({ ...client, status: 'active' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/revoke') {
        return json({ id: 'job-revoke', type: 'client.revoke', status: 'queued', scope_type: 'client', scope_id: 'client-1', result: {} }, 202);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/clients/client-1') {
        return json({ client_id: 'client-1', deleted: true, config_cleanup: {} });
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/accesses') {
        return json([{ id: 'access-1', client_account_id: 'client-1', instance_id: 'instance-1', status: 'active', provision_mode: 'managed', metadata: { xray_uuid: 'uuid-safe' } }]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/access-groups') {
        return json([group]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/client-access-groups') {
        return json([group]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups/group-1/members:preview') {
        return json({
          group_id: 'group-1',
          group_key: 'core',
          service_code: 'vless',
          dry_run: true,
          created_memberships: 1,
          moved_memberships: body.mode === 'add_or_move' ? 1 : 0,
          skipped_existing: 0,
          affected_instances: 2,
          materialized_created: 1,
          materialized_updated: 0,
          materialized_disabled: 0,
          apply_job_count: 1,
          apply_job_ids: ['job-apply'],
          clients: [{ client_id: 'client-1', username: 'alpha', group_id: 'group-1', group_key: 'core', xray_uuid: 'uuid-safe' }],
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups/group-1/members:bulk-apply') {
        return json({
          group_id: 'group-1',
          group_key: 'core',
          service_code: 'vless',
          created_memberships: 1,
          moved_memberships: body.mode === 'add_or_move' ? 1 : 0,
          skipped_existing: 0,
          affected_instances: 2,
          materialized_created: 1,
          materialized_updated: 0,
          materialized_disabled: 0,
          sync_job_id: 'job-sync',
          apply_job_count: 1,
          apply_job_ids: ['job-apply'],
        });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/client-access-groups/group-1/members/client-1') {
        return json({ group_id: 'group-1', group_key: 'core', service_code: 'vless', created_memberships: 0, moved_memberships: 0, skipped_existing: 0, apply_job_count: 0 });
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/artifacts') {
        return json([artifact]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/artifacts') {
        return json({ job: { id: 'job-artifact', type: 'artifact.build', status: 'queued' }, requested_type: body.type, message: 'artifact build queued' }, 202);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/clients/client-1/artifacts/artifact-1') {
        return json({ client_id: 'client-1', artifact_id: 'artifact-1', artifact_type: 'vless_url', deleted: true });
      }
      if (method === 'GET' && url.pathname === '/api/v1/jobs') {
        return json([
          { id: 'job-artifact', type: 'artifact.build', status: 'queued', scope_type: 'client', scope_id: 'client-1', payload: { client_id: 'client-1' }, created_at: '2026-07-08T10:20:00Z', result: {} },
        ]);
      }
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/') && url.pathname.endsWith('/logs')) {
        return json([]);
      }
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/')) {
        return json({ id: url.pathname.split('/')[4], type: 'artifact.build', status: 'queued', result: {} });
      }
      return json({ status: 'error', error: `unhandled ${method} ${url.pathname}` }, 500);
    }));
  });

  afterEach(() => {
    openSpy.mockRestore();
    consoleSpy.mockRestore();
    vi.unstubAllGlobals();
  });

  it('loads the client list and opens a real detail drawer', async () => {
    await openClient();
    expect(screen.getByText('Primary test client')).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/clients')).toBe(true);
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/clients/client-1')).toBe(true);
  });

  it('creates clients through the backend and handles 409 and 422 responses', async () => {
    renderPage();
    expect((await screen.findAllByText('alpha@example.test')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: 'Create client' }));
    await userEvent.type(screen.getByLabelText('Username'), 'bravo');
    await userEvent.type(screen.getByLabelText('Display name'), 'Bravo User');
    await userEvent.type(screen.getByLabelText('Email'), 'bravo@example.test');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create client' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/clients')?.body).toMatchObject({ username: 'bravo' });
    await screen.findByRole('heading', { name: 'Bravo User' });
    await userEvent.click(screen.getByRole('button', { name: 'Close' }));

    await userEvent.click(screen.getByRole('button', { name: 'Create client' }));
    await userEvent.clear(screen.getByLabelText('Username'));
    await userEvent.type(screen.getByLabelText('Username'), 'exists');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create client' }).at(-1)!);
    await screen.findByText(/Conflict \(409\)/);

    await userEvent.clear(screen.getByLabelText('Username'));
    await userEvent.type(screen.getByLabelText('Username'), 'invalid');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create client' }).at(-1)!);
    await screen.findByText('Username is invalid');
  });

  it('runs status, revoke and delete actions only through confirmed backend mutations', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('button', { name: 'Suspend' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/suspend')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Revoke' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/revoke')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/revoke')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1')).toBe(true));
  });

  it('assigns a single client to a VLESS group with preview, stale guard and apply', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Access' }));
    expect((await screen.findAllByText('Core access')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('uuid-safe').length).toBeGreaterThan(0);

    await userEvent.selectOptions(screen.getByLabelText('Group key'), 'group-1');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups/group-1/members:preview')).toBe(true));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled());

    await userEvent.selectOptions(screen.getByLabelText('Assignment mode'), 'add_or_move');
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled());
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups/group-1/members:bulk-apply')).toBe(true));
    expect(screen.getByText('job-sync')).toBeInTheDocument();
  });

  it('removes VLESS membership through the backend after confirmation', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Access' }));
    expect((await screen.findAllByText('Core access')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: 'Remove VLESS membership' }));
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/client-access-groups/group-1/members/client-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/client-access-groups/group-1/members/client-1')).toBe(true));
  });

  it('lists, builds, downloads and deletes client artifacts through backend endpoints', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Artifacts' }));
    expect((await screen.findAllByText('vless_url')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole('button', { name: 'Build artifact' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/artifacts')).toBe(true));
    expect(screen.getByText('job-artifact')).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Download' })[0]);
    expect(openSpy).toHaveBeenCalledWith('/api/v1/clients/client-1/artifacts/artifact-1/download', '_blank', 'noopener,noreferrer');
    expect(consoleSpy).not.toHaveBeenCalled();

    await userEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/artifacts/artifact-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/artifacts/artifact-1')).toBe(true));
  });

  it('shows permission errors safely and keeps Clients workflows away from legacy', async () => {
    vi.mocked(fetch).mockImplementationOnce(async () => json([client]));
    vi.mocked(fetch).mockImplementationOnce(async () => json(client));
    vi.mocked(fetch).mockImplementationOnce(async () => json({ status: 'error', error: 'client.write permission required' }, 403));

    renderPage();
    expect((await screen.findAllByText('alpha@example.test')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await screen.findByRole('heading', { name: 'Alpha User' });
    await userEvent.click(screen.getByRole('button', { name: 'Suspend' }));
    await screen.findByText(/Permission denied \(403\)/);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(screen.queryByText('/legacy')).not.toBeInTheDocument();
  });

  it('keeps raw API paths out of the Clients page component', () => {
    expect(String(ClientsPage)).not.toContain('/api/v1');
    expect(String(ClientsPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);
  });
});
