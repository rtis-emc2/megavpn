import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import { ClientGroupsPage } from './ClientGroupsPage';

type FetchCall = {
  method: string;
  path: string;
  body?: unknown;
};

const group = {
  id: 'group-1',
  service_code: 'vless',
  group_key: 'core',
  display_name: 'Core access',
  description: 'Default VLESS access',
  status: 'active',
  policy_json: {
    access_mode: 'instance_default',
    outbound_tag: 'direct',
    ad_block: false,
  },
  scope_mode: 'all_active_instances',
  auto_apply_new_instances: true,
  member_count: 1,
  active_member_count: 1,
  affected_instances: 2,
  pending_sync_count: 0,
  failed_sync_count: 0,
  applied_sync_count: 2,
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
    <QueryClientProvider client={queryClient}>
      <ClientGroupsPage />
    </QueryClientProvider>,
  );
  return queryClient;
}

describe('ClientGroupsPage', () => {
  const calls: FetchCall[] = [];

  beforeEach(async () => {
    calls.length = 0;
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/client-access-services') {
        return json([
          {
            service_code: 'vless',
            display_name: 'VLESS / Xray',
            description: 'Fleet VLESS groups',
            status: 'active',
            supports_groups: true,
            supports_membership: true,
            supports_materialization: true,
          },
          {
            service_code: 'wireguard',
            display_name: 'WireGuard',
            status: 'coming_soon',
            supports_groups: false,
            supports_membership: false,
            supports_materialization: false,
          },
        ]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/client-access-groups') {
        return json([group]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups') {
        return json({ ...group, id: 'group-2', group_key: body.group_key, display_name: body.display_name }, 201);
      }
      if (method === 'GET' && url.pathname === '/api/v1/client-access-groups/available-clients') {
        return json({
          service_code: 'vless',
          assignment: url.searchParams.get('assignment') || 'unassigned',
          total: 1,
          limit: 50,
          offset: 0,
          items: [
            {
              client_id: 'client-1',
              username: 'alpha',
              display_name: 'Alpha User',
              email: 'alpha@example.test',
              client_status: 'active',
            },
          ],
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/client-access-groups/group-1/members') {
        return json({
          group_id: 'group-1',
          service_code: 'vless',
          group_key: 'core',
          total: 1,
          limit: 25,
          offset: 0,
          items: [
            {
              client_id: 'client-existing',
              username: 'existing',
              display_name: 'Existing User',
              client_status: 'active',
              membership_status: 'active',
              xray_uuid: 'uuid-existing',
            },
          ],
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/client-access-groups/group-1/scope') {
        return json({
          group_id: 'group-1',
          scope_mode: 'all_active_instances',
          auto_apply_new_instances: true,
          include_instance_ids: [],
          exclude_instance_ids: [],
          affected_instances: 2,
        });
      }
      if (method === 'PATCH' && url.pathname === '/api/v1/client-access-groups/group-1/scope') {
        return json({
          group_id: 'group-1',
          scope_mode: body.scope_mode,
          auto_apply_new_instances: body.auto_apply_new_instances,
          include_instance_ids: body.include_instance_ids,
          exclude_instance_ids: body.exclude_instance_ids,
          affected_instances: 1,
          materialized_created: 1,
          materialized_updated: 0,
          materialized_disabled: 1,
          apply_job_count: 1,
          apply_job_ids: ['job-scope'],
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/instances') {
        return json([
          {
            id: 'instance-1',
            name: 'Xray Edge',
            slug: 'xray-edge',
            service_code: 'xray-core',
            status: 'active',
          },
        ]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/client-access-groups/group-1/sync-state') {
        return json([
          {
            group_id: 'group-1',
            instance_id: 'instance-1',
            desired_hash: 'hash-current',
            last_applied_hash: 'hash-current',
            status: 'applied',
            updated_at: '2026-07-08T10:00:00Z',
          },
        ]);
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups/group-1/sync:preview') {
        return json({
          group_id: 'group-1',
          group_key: 'core',
          service_code: 'vless',
          desired_hash: 'hash-next',
          affected_instances: 1,
          member_count: 1,
          pending_instances: 0,
          applied_instances: 1,
          failed_instances: 0,
          instance_ids: ['instance-1'],
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups/group-1/sync:apply') {
        return json({
          group_id: 'group-1',
          group_key: 'core',
          service_code: 'vless',
          dry_run: false,
          created_memberships: 0,
          moved_memberships: 0,
          skipped_existing: 0,
          affected_instances: 1,
          materialized_created: 0,
          materialized_updated: 1,
          materialized_disabled: 0,
          apply_job_count: 1,
          apply_job_ids: ['job-sync'],
        }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups/group-1/members:preview') {
        return json({
          group_id: 'group-1',
          group_key: 'core',
          service_code: 'vless',
          dry_run: true,
          created_memberships: 1,
          moved_memberships: 0,
          skipped_existing: 0,
          affected_instances: 2,
          materialized_created: 0,
          materialized_updated: 0,
          materialized_disabled: 0,
          apply_job_count: 2,
          clients: [{ client_id: 'client-1', username: 'alpha' }],
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/client-access-groups/group-1/members:bulk-apply') {
        return json({
          group_id: 'group-1',
          group_key: 'core',
          service_code: 'vless',
          dry_run: false,
          created_memberships: 1,
          moved_memberships: 0,
          skipped_existing: 0,
          affected_instances: 2,
          materialized_created: 2,
          materialized_updated: 0,
          materialized_disabled: 0,
          apply_job_count: 2,
          apply_job_ids: ['job-1', 'job-2'],
        }, 202);
      }
      return json({ error: `unexpected ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('creates VLESS groups through the client access group API', async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText('Core access');
    await user.click(screen.getByRole('button', { name: 'Create VLESS group' }));
    await user.type(screen.getByLabelText('Group key'), 'new-team');
    await user.type(screen.getByLabelText('Name'), 'New Team');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups')).toBe(true));
    const createCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups');
    expect(createCall?.body).toMatchObject({
      service_code: 'vless',
      group_key: 'new-team',
      display_name: 'New Team',
      scope_mode: 'all_active_instances',
    });
    expect(createCall?.body).toHaveProperty('policy_json.access_mode', 'instance_default');
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
  });

  it('previews and applies VLESS membership with backend bulk endpoints', async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText('Core access');
    await user.click(screen.getAllByRole('button', { name: 'Members' })[0]);
    await screen.findAllByText('Alpha User');

    await user.click(screen.getByRole('button', { name: 'Select visible' }));
    const applyButton = screen.getByRole('button', { name: 'Apply' });
    expect(applyButton).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Preview' }));

    await screen.findByText('Preview result');
    expect(applyButton).toBeEnabled();
    await user.click(applyButton);

    await screen.findByText('Apply result');
    const previewCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups/group-1/members:preview');
    const applyCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups/group-1/members:bulk-apply');
    expect(previewCall?.body).toMatchObject({
      client_ids: ['client-1'],
      mode: 'add_only',
      queue_apply: true,
      dry_run: true,
      all_filtered: false,
      filter_assignment: 'unassigned',
      filter_group_id: 'group-1',
    });
    expect(applyCall?.body).toMatchObject({
      client_ids: ['client-1'],
      mode: 'add_only',
      queue_apply: true,
      dry_run: false,
    });
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
  });

  it('updates VLESS group scope through the backend scope endpoint', async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText('Core access');
    await user.click(screen.getAllByRole('button', { name: 'Scope' })[0]);
    await screen.findByText('2 affected instances');

    await user.selectOptions(screen.getByLabelText('Scope'), 'selected_instances');
    await user.click(screen.getAllByLabelText('Select Xray Edge')[0]);
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'PATCH' && call.path === '/api/v1/client-access-groups/group-1/scope')).toBe(true));
    const scopeCall = calls.find((call) => call.method === 'PATCH' && call.path === '/api/v1/client-access-groups/group-1/scope');
    expect(scopeCall?.body).toMatchObject({
      group_id: 'group-1',
      scope_mode: 'selected_instances',
      auto_apply_new_instances: true,
      include_instance_ids: ['instance-1'],
      exclude_instance_ids: [],
    });
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
  });

  it('previews and applies VLESS group sync with backend sync endpoints', async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText('Core access');
    await user.click(screen.getAllByRole('button', { name: 'Sync' })[0]);
    await screen.findAllByText('Sync state');

    const applyButton = screen.getByRole('button', { name: 'Apply' });
    expect(applyButton).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Preview' }));

    await screen.findByText('Sync preview');
    expect(applyButton).toBeEnabled();
    await user.click(applyButton);

    await screen.findByText('Apply result');
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups/group-1/sync:preview')).toBe(true);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/client-access-groups/group-1/sync:apply')).toBe(true);
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
  });
});
