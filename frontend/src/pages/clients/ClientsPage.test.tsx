import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
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

const access = {
  id: 'access-1',
  client_account_id: 'client-1',
  instance_id: 'instance-1',
  status: 'active',
  provision_mode: 'managed',
  metadata: { xray_uuid: 'uuid-secret-full', service_code: 'xray-core' },
};

const route = {
  id: 'route-1',
  client_account_id: 'client-1',
  service_access_id: 'access-1',
  instance_id: 'instance-1',
  node_id: 'node-1',
  name: 'office-cidr',
  status: 'active',
  action: 'allow',
  destination_type: 'cidr',
  destination: '10.42.0.0/16',
  protocol: 'any',
  ports: '*',
  description: 'Office network',
  created_at: '2026-07-08T10:15:00Z',
  updated_at: '2026-07-08T10:15:00Z',
};

const shareLink = {
  id: 'share-1',
  client_account_id: 'client-1',
  target_type: 'artifact',
  target_id: 'artifact-1',
  token_hint: 'share...hint',
  status: 'active',
  expires_at: '2026-07-09T10:10:00Z',
  download_count: 0,
  created_at: '2026-07-08T10:12:00Z',
};

const subscription = {
  id: 'sub-1',
  client_account_id: 'client-1',
  token_hint: 'sub...hint',
  status: 'active',
  expires_at: '2026-08-08T10:10:00Z',
  download_count: 1,
  last_used_at: '2026-07-08T10:30:00Z',
  created_at: '2026-07-08T10:12:00Z',
  updated_at: '2026-07-08T10:12:00Z',
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
  let storageSetSpy: ReturnType<typeof vi.spyOn>;
  let clipboardWrite: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    calls.length = 0;
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
    consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    storageSetSpy = vi.spyOn(Storage.prototype, 'setItem');
    clipboardWrite = vi.fn(async () => undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: clipboardWrite },
    });
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
      if (method === 'PATCH' && url.pathname === '/api/v1/clients/client-1') {
        if (body.email === 'blocked@example.test') return json({ status: 'error', error: 'email already exists' }, 409);
        return json({ ...client, display_name: body.display_name, email: body.email, notes: body.notes, expires_at: body.expires_at || null, updated_at: '2026-07-08T10:45:00Z' });
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
        return json([access]);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/clients/client-1/accesses/access-1') {
        return json({
          client_id: 'client-1',
          service_access_id: 'access-1',
          instance_id: 'instance-1',
          deleted: true,
          config_cleanup: { client_id: 'client-1', artifacts_deleted: 1, share_links_deleted: 1, subscriptions_deleted: 1, files_deleted: 1 },
          service_accesses_deleted: 1,
          access_routes_deleted: 1,
          secret_refs_deleted: 1,
          instance_apply_jobs_queued: 1,
          route_policy_jobs_queued: 1,
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/accesses/access-1/revoke') {
        return json({
          client_id: 'client-1',
          service_access_id: 'access-1',
          instance_id: 'instance-1',
          revoked: true,
          already_revoked: false,
          access_routes_revoked: 1,
          share_links_revoked: 1,
          subscriptions_revoked: 0,
          instance_apply_jobs_queued: 1,
          route_policy_jobs_queued: 1,
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/accesses/access-1/rotate-xray') {
        return json({ id: 'job-rotate', type: 'client.access.rotate', status: 'queued', scope_type: 'client', scope_id: 'client-1', token: 'rotated-access-secret', result: { token: 'nested-rotated-secret' } }, 202);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/clients/client-1/configs') {
        return json({ client_id: 'client-1', artifacts_deleted: 1, share_links_deleted: 1, subscriptions_deleted: 1, files_deleted: 1 });
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/routes') {
        return json([route]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/routes') {
        return json({ ...route, id: 'route-2', name: body.name, destination: body.destination, service_access_id: body.service_access_id }, 201);
      }
      if (method === 'PATCH' && url.pathname === '/api/v1/clients/client-1/routes/route-1') {
        if (body.destination === 'bad route') return json({ status: 'error', error: 'destination must be a valid CIDR prefix' }, 400);
        return json({ ...route, name: body.name, destination: body.destination, ports: body.ports, description: body.description, updated_at: '2026-07-08T10:50:00Z' });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/clients/client-1/routes/route-1') {
        return json({ ...route, status: 'revoked' });
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
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/share-links') {
        return json([shareLink]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/share-links') {
        return json({ ...shareLink, id: 'share-2', token: 'share-once-token', token_hint: 'share...token', target_id: body.target_id, expires_at: '2026-07-11T10:10:00Z' }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/share-links/share-1/revoke') {
        return json({ ...shareLink, status: 'revoked' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/share-links/share-1/rotate') {
        return json({ ...shareLink, id: 'share-3', token: 'share-rotated-token', token_hint: 'share...ated', expires_at: '2026-07-12T10:10:00Z' }, 201);
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/subscriptions') {
        return json([subscription]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/subscriptions/rotate') {
        return json({ subscription: { ...subscription, id: 'sub-2', token_hint: 'sub...ated' }, subscription_url: 'https://control.example/subscribe/vless/sub-once-token', message: 'copy now' }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/subscriptions/sub-1/revoke') {
        return json({ ...subscription, status: 'revoked' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/clients/client-1/deliver-email') {
        return json({ status: 'ok', delivery: { id: 'delivery-1', client_account_id: 'client-1', email: 'alpha@example.test', subject: body.subject, status: 'sent', artifact_ids: ['artifact-1'], share_link_ids: body.create_share_link ? ['share-2'] : [], created_at: '2026-07-08T10:40:00Z' } });
      }
      if (method === 'GET' && url.pathname === '/api/v1/clients/client-1/deliveries') {
        return json([
          {
            id: 'delivery-1',
            client_account_id: 'client-1',
            delivery_type: 'client_access_email',
            channel: 'email',
            destination_hint: 'a***@example.test',
            status: 'sent',
            artifact_count: 1,
            share_link_count: 1,
            safe_error_summary: '',
            related_artifact_ids: ['artifact-1'],
            related_share_link_ids: ['share-2'],
            created_at: '2026-07-08T10:40:00Z',
            sent_at: '2026-07-08T10:41:00Z',
            completed_at: '2026-07-08T10:41:00Z',
          },
          {
            id: 'delivery-2',
            client_account_id: 'client-1',
            delivery_type: 'client_access_email',
            channel: 'email',
            destination_hint: 'b***@example.test',
            status: 'failed',
            artifact_count: 0,
            share_link_count: 0,
            safe_error_summary: 'delivery failed; sensitive error details are redacted',
            created_at: '2026-07-08T10:35:00Z',
            failed_at: '2026-07-08T10:35:30Z',
          },
        ]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/jobs') {
        return json([
          { id: 'job-artifact', type: 'artifact.build', status: 'queued', scope_type: 'client', scope_id: 'client-1', payload: { client_id: 'client-1' }, created_at: '2026-07-08T10:20:00Z', result: {} },
          { id: 'job-rotate', type: 'client.access.rotate', status: 'queued', scope_type: 'client', scope_id: 'client-1', payload: { client_id: 'client-1' }, created_at: '2026-07-08T10:25:00Z', result: {} },
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
    storageSetSpy.mockRestore();
    delete (navigator as { clipboard?: unknown }).clipboard;
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
    await userEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!);

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

  it('edits generic client metadata through the backend PATCH endpoint', async () => {
    await openClient();
    await userEvent.click(screen.getAllByRole('button', { name: 'Edit' }).at(-1)!);
    const dialog = screen.getAllByRole('dialog').at(-1)!;
    await userEvent.clear(within(dialog).getByLabelText('Display name'));
    await userEvent.type(within(dialog).getByLabelText('Display name'), 'Alpha Renamed');
    await userEvent.clear(within(dialog).getByLabelText('Email'));
    await userEvent.type(within(dialog).getByLabelText('Email'), 'alpha.renamed@example.test');
    await userEvent.clear(within(dialog).getByLabelText('Description'));
    await userEvent.type(within(dialog).getByLabelText('Description'), 'Updated operator note');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PATCH' && call.path === '/api/v1/clients/client-1')).toBe(true));
    expect(calls.find((call) => call.method === 'PATCH' && call.path === '/api/v1/clients/client-1')?.body).toMatchObject({
      display_name: 'Alpha Renamed',
      email: 'alpha.renamed@example.test',
      notes: 'Updated operator note',
    });
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
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
    expect(screen.getAllByText('Backend-held identity (redacted)').length).toBeGreaterThan(0);
    expect(screen.queryByText('uuid-secret-full')).not.toBeInTheDocument();

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

  it('loads, creates, updates and deletes client routes through the backend', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Routes' }));
    expect((await screen.findAllByText('office-cidr')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('10.42.0.0/16').length).toBeGreaterThan(0);

    await userEvent.type(screen.getByLabelText('Name'), 'branch-cidr');
    await userEvent.clear(screen.getByLabelText('Destination'));
    await userEvent.type(screen.getByLabelText('Destination'), '10.43.0.0/16');
    await userEvent.click(screen.getByRole('button', { name: 'Create route' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/routes')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/routes')?.body).toMatchObject({
      service_access_id: 'access-1',
      name: 'branch-cidr',
      destination: '10.43.0.0/16',
    });

    await userEvent.click(screen.getAllByRole('button', { name: 'Edit' }).at(-1)!);
    const dialog = screen.getAllByRole('dialog').at(-1)!;
    await userEvent.clear(within(dialog).getByLabelText('Name'));
    await userEvent.type(within(dialog).getByLabelText('Name'), 'office-cidr-updated');
    await userEvent.clear(within(dialog).getByLabelText('Destination'));
    await userEvent.type(within(dialog).getByLabelText('Destination'), '10.44.0.0/16');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PATCH' && call.path === '/api/v1/clients/client-1/routes/route-1')).toBe(true));
    expect(calls.find((call) => call.method === 'PATCH' && call.path === '/api/v1/clients/client-1/routes/route-1')?.body).toMatchObject({
      service_access_id: 'access-1',
      name: 'office-cidr-updated',
      destination: '10.44.0.0/16',
    });

    await userEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/routes/route-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/routes/route-1')).toBe(true));
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
  });

  it('rotates, revokes and deletes access and cleans configs with confirmation and job tracking', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Maintenance' }));
    expect((await screen.findAllByText('Backend-held identity (redacted)')).length).toBeGreaterThan(0);
    expect(screen.queryByText('uuid-secret-full')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Revoke access' })).toBeEnabled();

    await userEvent.click(screen.getByRole('button', { name: 'Rotate access' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/accesses/access-1/rotate-xray')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/accesses/access-1/rotate-xray')).toBe(true));
    expect((await screen.findAllByText('job-rotate')).length).toBeGreaterThan(0);
    expect(screen.queryByText('rotated-access-secret')).not.toBeInTheDocument();
    expect(screen.queryByText('nested-rotated-secret')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(screen.getByText('rotated-access-secret')).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!);
    expect(screen.queryByText('rotated-access-secret')).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Revoke access' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/accesses/access-1/revoke')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/accesses/access-1/revoke')).toBe(true));
    expect(screen.getByText('Routes revoked')).toBeInTheDocument();
    expect(screen.getByText('Share links revoked')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Delete access' }));
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/accesses/access-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/accesses/access-1')).toBe(true));
    expect(screen.getByText('Routes deleted')).toBeInTheDocument();
    expect(screen.getByText('Open jobs')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Cleanup generated configs' }));
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/configs')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/clients/client-1/configs')).toBe(true));
    expect(screen.getByText('Artifacts deleted')).toBeInTheDocument();

    expect(storageSetSpy.mock.calls.some((call: unknown[]) => {
      const key = String(call[0]).toLowerCase();
      const value = String(call[1]);
      return key.includes('token') ||
        value.includes('uuid-secret-full') ||
        value.includes('rotated-access-secret') ||
        value.includes('nested-rotated-secret');
    })).toBe(false);
    expect(consoleSpy).not.toHaveBeenCalled();
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
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

  it('opens delivery from an artifact row and creates a one-time share link safely', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Artifacts' }));
    expect((await screen.findAllByText('vless_url')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Create share link' })[0]);
    await screen.findByRole('tab', { name: 'Delivery' });
    expect((await screen.findAllByText('share...hint')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getAllByRole('button', { name: 'Create share link' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/share-links')).toBe(true));
    expect(await screen.findByText('This value may be shown only once. Copy it now and store it securely.')).toBeInTheDocument();
    expect(screen.queryByText('/share/share-once-token')).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(screen.getByText('/share/share-once-token')).toBeInTheDocument();
    expect(window.localStorage.getItem('share-once-token')).toBeNull();
    expect(window.sessionStorage.getItem('share-once-token')).toBeNull();

    expect(clipboardWrite).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));
    expect(clipboardWrite).toHaveBeenCalledWith('/share/share-once-token');
    await userEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!);
    expect(screen.queryByText('/share/share-once-token')).not.toBeInTheDocument();
    expect(consoleSpy).not.toHaveBeenCalled();
  });

  it('requires confirmation for share link revoke and rotate', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Delivery' }));
    expect((await screen.findAllByText('share...hint')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getAllByRole('button', { name: 'Rotate' })[0]);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/share-links/share-1/rotate')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/share-links/share-1/rotate')).toBe(true));
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(screen.getByText('/share/share-rotated-token')).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!);

    await userEvent.click(screen.getAllByRole('button', { name: 'Revoke' })[0]);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/share-links/share-1/revoke')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/share-links/share-1/revoke')).toBe(true));
  });

  it('manages VLESS subscriptions with one-time URL display and confirmed revoke', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Delivery' }));
    expect((await screen.findAllByText('sub...hint')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole('button', { name: 'Create / rotate VLESS subscription' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/subscriptions/rotate')).toBe(true));
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(screen.getByText('https://control.example/subscribe/vless/sub-once-token')).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!);

    await userEvent.click(screen.getAllByRole('button', { name: 'Revoke' }).at(-1)!);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/subscriptions/sub-1/revoke')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/subscriptions/sub-1/revoke')).toBe(true));
  });

  it('sends client delivery email through the backend and renders status safely', async () => {
    await openClient();
    await userEvent.click(screen.getByRole('tab', { name: 'Delivery' }));
    await screen.findByText('Target email: alpha@example.test');
    expect((await screen.findAllByText('a***@example.test')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('delivery failed; sensitive error details are redacted').length).toBeGreaterThan(0);
    expect(screen.queryByText('token=')).not.toBeInTheDocument();
    await userEvent.clear(screen.getByLabelText('Subject'));
    await userEvent.type(screen.getByLabelText('Subject'), 'Access package');
    await userEvent.click(screen.getByLabelText('Create a share link for email delivery if needed'));
    await userEvent.click(screen.getByRole('button', { name: 'Send by email' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/deliver-email')).toBe(true));
    await waitFor(() => expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/clients/client-1/deliveries?limit=50')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/clients/client-1/deliver-email')?.body).toMatchObject({ subject: 'Access package', create_share_link: true });
    expect(screen.getByText('delivery-1')).toBeInTheDocument();
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
    expect(String(ClientsPage)).not.toContain('dangerouslySetInnerHTML');
  });
});
