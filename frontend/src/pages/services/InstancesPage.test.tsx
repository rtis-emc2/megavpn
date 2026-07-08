import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import { InstancesPage } from './InstancesPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const instance = {
  id: 'instance-1',
  node_id: 'node-1',
  service_code: 'xray-core',
  name: 'Xray edge',
  slug: 'xray-edge',
  systemd_unit: 'xray@xray-edge.service',
  status: 'active',
  enabled: true,
  endpoint_host: 'vpn.example.test',
  endpoint_port: 443,
  current_revision_id: 'rev-2',
  last_applied_revision_id: 'rev-1',
  created_at: '2026-07-08T10:00:00Z',
  updated_at: '2026-07-08T11:00:00Z',
  spec: { redacted: true },
};

const runtimeState = {
  id: 'state-1',
  instance_id: 'instance-1',
  node_id: 'node-1',
  service_code: 'xray-core',
  systemd_unit: 'xray@xray-edge.service',
  desired_status: 'running',
  runtime_status: 'running',
  health_status: 'healthy',
  drift_status: 'in_sync',
  active_state: 'active',
  enabled_state: 'enabled',
  config_hash: 'hash-current',
  last_job_id: 'job-last',
  last_job_type: 'instance.apply',
  last_job_status: 'succeeded',
  applied_revision_id: 'rev-1',
  observed_revision_id: 'rev-1',
  endpoint_host: 'vpn.example.test',
  endpoint_port: 443,
  health_checks: [{ code: 'unit_active', status: 'ok', message: 'unit is active' }],
  health_reasons: ['systemd unit active'],
  drift_reasons: [],
  listening_ports: [{ port: 443, protocol: 'tcp' }],
  result: { rendered: '<strong>not html</strong>' },
  checked_at: '2026-07-08T11:10:00Z',
  updated_at: '2026-07-08T11:10:00Z',
};

const revisions = [
  { id: 'rev-2', instance_id: 'instance-1', revision_no: 2, status: 'validated', rendered_hash: 'hash-current', is_current: true, is_last_applied: false, created_at: '2026-07-08T11:00:00Z', validation_errors: [], spec: {} },
  { id: 'rev-1', instance_id: 'instance-1', revision_no: 1, status: 'applied', rendered_hash: 'hash-old', is_current: false, is_last_applied: true, created_at: '2026-07-08T10:00:00Z', applied_at: '2026-07-08T10:05:00Z', validation_errors: [], spec: {} },
];

const accessGroups = {
  instance_id: 'instance-1',
  instance_name: 'Xray edge',
  service_code: 'xray-core',
  available_keys: ['core'],
  groups: [
    { key: 'core', label: 'Core access', status: 'active', member_count: 2, pending_count: 0, active_count: 2, disabled_count: 0 },
  ],
};

const observations = [
  {
    ...runtimeState,
    id: 'obs-1',
    source: 'agent',
    health_status: 'warning',
    drift_status: 'drifted',
    health_reasons: ['<img src=x onerror=alert(1)>'],
    error_text: '<script>alert(1)</script>',
    observed_at: '2026-07-08T11:11:00Z',
    received_at: '2026-07-08T11:11:05Z',
  },
];

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function job(id: string, type = 'instance.apply') {
  return {
    id,
    type,
    status: 'queued',
    scope_type: 'instance',
    scope_id: 'instance-1',
    instance_id: 'instance-1',
    result: { queued: true },
    created_at: '2026-07-08T11:20:00Z',
  };
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
        <InstancesPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return queryClient;
}

async function openInstance() {
  renderPage();
  expect((await screen.findAllByText('Xray edge')).length).toBeGreaterThan(0);
  await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
  await screen.findByRole('heading', { name: 'Xray edge' });
}

describe('InstancesPage', () => {
  const calls: FetchCall[] = [];
  let actionErrors: Record<string, number>;

  beforeEach(async () => {
    calls.length = 0;
    actionErrors = {};
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/instances') return json([instance]);
      if (method === 'GET' && url.pathname === '/api/v1/instances/runtime-states') return json([runtimeState]);
      if (method === 'GET' && url.pathname === '/api/v1/nodes') return json([{ id: 'node-1', name: 'Edge One', status: 'active' }]);
      if (method === 'GET' && url.pathname === '/api/v1/services') return json([{ id: 'svc-1', code: 'xray-core', name: 'Xray Core', supports_instances: true, enabled: true }]);
      if (method === 'GET' && url.pathname === '/api/v1/instances/instance-1') return json(instance);
      if (method === 'GET' && url.pathname === '/api/v1/instances/instance-1/runtime-state') return json(runtimeState);
      if (method === 'GET' && url.pathname === '/api/v1/instances/instance-1/revisions') return json(revisions);
      if (method === 'GET' && url.pathname === '/api/v1/instances/instance-1/runtime-observations') return json(observations);
      if (method === 'GET' && url.pathname === '/api/v1/instances/instance-1/vless-groups/members') return json(accessGroups);
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/')) return json(job(url.pathname.split('/')[4]));
      if (method === 'GET' && url.pathname.endsWith('/logs')) return json([{ level: 'info', message: 'queued' }]);
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/apply') {
        if (actionErrors.apply) return json({ error: 'instance.apply permission required' }, actionErrors.apply);
        return json(job('job-apply'), 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/restart') {
        if (actionErrors.restart) return json({ error: 'restart validation failed' }, actionErrors.restart);
        return json(job('job-restart', 'instance.restart'), 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/start') return json(job('job-start', 'instance.start'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/stop') return json(job('job-stop', 'instance.stop'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/enable') return json(job('job-enable', 'instance.enable'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/disable') return json(job('job-disable', 'instance.disable'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/diagnose') return json(job('job-diagnose', 'instance.diagnose'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/rollback') {
        return json({ revision: { ...revisions[0], id: 'rev-rollback' }, can_apply: true, message: 'rollback revision created and is apply-ready' });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/instances/instance-1') {
        if (actionErrors.delete) return json({ error: 'instance has active cleanup guard' }, actionErrors.delete);
        return json({ ...instance, status: 'deleting' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/instances/instance-1/force-delete') return json({ status: 'deleted', instance: { ...instance, status: 'deleted' } });
      if (method === 'POST' && url.pathname === '/api/v1/instances') return json({ ...instance, id: 'instance-new', name: body?.name || 'Manual edge' }, 201);
      if (method === 'PUT' && url.pathname === '/api/v1/instances/instance-1/spec') return json({ revision: { ...revisions[0], id: 'rev-spec' }, can_apply: true, message: 'instance revision saved as apply-ready', issue_count: 0 });
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads instance list, opens detail, and shows runtime state', async () => {
    await openInstance();
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/instances')).toBe(true);
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/instances/instance-1')).toBe(true);

    await userEvent.click(screen.getByRole('tab', { name: 'Runtime' }));
    expect(screen.getAllByText('running').length).toBeGreaterThan(0);
    expect(screen.getByText('hash-current')).toBeInTheDocument();
  });

  it('requires confirmation for apply and shows the returned job', async () => {
    await openInstance();
    await userEvent.click(screen.getAllByRole('button', { name: 'Apply' }).at(-1)!);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/apply')).toBe(false);

    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/apply')).toBe(true));

    const applyCalls = calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/apply').length;
    await userEvent.click(screen.getByRole('tab', { name: 'Runtime' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Reapply' }).at(-1)!);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/apply').length).toBe(applyCalls + 1));
    await userEvent.click(screen.getByRole('tab', { name: 'Jobs / Activity' }));
    await screen.findByText('job-apply');
  });

  it('rolls back an explicit revision and queues a real apply job', async () => {
    await openInstance();
    await userEvent.click(screen.getByRole('tab', { name: 'Revisions / Rollback' }));
    await userEvent.selectOptions(await screen.findByLabelText('Rollback target'), 'rev-1');
    await userEvent.click(screen.getByRole('button', { name: 'Rollback' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/rollback')).toBe(true));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/apply')).toBe(true));
    await screen.findByText('job-apply');
  });

  it('renders diagnostics as text and runs backend diagnostics after confirmation', async () => {
    await openInstance();
    await userEvent.click(screen.getByRole('tab', { name: 'Diagnostics' }));
    expect((await screen.findAllByText(/<script>alert\(1\)<\/script>/)).length).toBeGreaterThan(0);
    expect(document.querySelector('script')).toBeNull();

    await userEvent.click(screen.getByRole('button', { name: 'Run diagnostics' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/diagnose')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/diagnose')).toBe(true));
  });

  it('runs lifecycle, delete and force-delete only after confirmation', async () => {
    await openInstance();
    await userEvent.click(screen.getByRole('tab', { name: 'Runtime' }));
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/restart')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/restart')).toBe(true));

    for (const [label, path] of [
      ['Start', '/api/v1/instances/instance-1/start'],
      ['Stop', '/api/v1/instances/instance-1/stop'],
      ['Enable', '/api/v1/instances/instance-1/enable'],
      ['Disable', '/api/v1/instances/instance-1/disable'],
    ] as const) {
      await userEvent.click(screen.getByRole('button', { name: label }));
      expect(calls.some((call) => call.method === 'POST' && call.path === path)).toBe(false);
      await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
      await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === path)).toBe(true));
    }

    await userEvent.click(screen.getByRole('button', { name: 'Delete instance' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/instances/instance-1')).toBe(true));

    await userEvent.type(screen.getByLabelText('Force confirmation'), 'DELETE xray-edge');
    await userEvent.type(screen.getByLabelText('Force reason'), 'stale runtime cleanup');
    await userEvent.click(screen.getByRole('button', { name: 'Force-delete' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/force-delete')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/instances/instance-1/force-delete')?.body).toMatchObject({ confirmation: 'DELETE xray-edge', reason: 'stale runtime cleanup' });
  });

  it('creates manual instances and replaces specs through backend endpoints', async () => {
    renderPage();
    expect((await screen.findAllByText('Xray edge')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: 'Manual instance' }));
    await screen.findByRole('heading', { name: 'Manual instance' });
    await userEvent.selectOptions(screen.getAllByLabelText('Node').at(-1)!, 'node-1');
    await userEvent.selectOptions(screen.getAllByLabelText('Service').at(-1)!, 'xray-core');
    await userEvent.type(screen.getByLabelText('Name'), 'Manual edge');
    await userEvent.type(screen.getByLabelText('Slug'), 'manual-edge');
    await userEvent.type(screen.getByLabelText('Endpoint host'), 'manual.example.test');
    await userEvent.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/instances')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/instances')?.body).toMatchObject({ node_id: 'node-1', service_code: 'xray-core', name: 'Manual edge' });

    await userEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!);
    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await screen.findByRole('heading', { name: 'Xray edge' });
    await userEvent.click(screen.getByRole('tab', { name: 'Spec' }));
    fireEvent.change(screen.getByLabelText('Spec JSON'), { target: { value: '{"redacted":true,"port":443}' } });
    await userEvent.click(screen.getByRole('button', { name: 'Replace spec' }));
    expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/instances/instance-1/spec')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/instances/instance-1/spec')).toBe(true));
  });

  it('keeps access groups read-only and links management to Clients Groups', async () => {
    await openInstance();
    await userEvent.click(screen.getByRole('tab', { name: 'Access groups' }));
    expect((await screen.findAllByText('Core access')).length).toBeGreaterThan(0);
    expect(screen.getByText('Manage in Clients -> Groups')).toHaveAttribute('href', '/clients/groups');
    expect(screen.queryByText(/Create VLESS group/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Add clients/i)).not.toBeInTheDocument();
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
  });

  it('shows backend 403, 422 and 409 errors safely', async () => {
    actionErrors.apply = 403;
    actionErrors.restart = 422;
    actionErrors.delete = 409;
    await openInstance();

    await userEvent.click(screen.getAllByRole('button', { name: 'Apply' }).at(-1)!);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await screen.findByText(/Permission denied: instance.apply permission required/);

    await userEvent.click(screen.getByRole('tab', { name: 'Runtime' }));
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await screen.findByText(/Validation failed: restart validation failed/);

    await userEvent.click(screen.getByRole('button', { name: 'Delete instance' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await screen.findByText(/Conflict: instance has active cleanup guard/);
  });

  it('keeps raw API paths and legacy workflow links out of the Instances page component', () => {
    expect(String(InstancesPage)).not.toContain('/api/v1');
    expect(String(InstancesPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);
    expect(String(InstancesPage)).not.toContain('dangerouslySetInnerHTML');
    expect(String(InstancesPage)).not.toContain('/legacy');
  });
});
