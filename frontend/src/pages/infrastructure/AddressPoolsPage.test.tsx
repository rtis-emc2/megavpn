import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../../shared/auth/AuthProvider';
import i18n from '../../shared/i18n';
import { AddressPoolsPage } from './AddressPoolsPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const pool = {
  id: 'pool-1',
  key: 'remote_access_v4',
  label: 'Remote access IPv4',
  description: 'Managed client subnets',
  family: 'ipv4',
  base_cidr: '172.20.0.0/16',
  start_cidr: '172.20.1.0/24',
  allocation_prefix: 24,
  service_scope: 'remote_access',
  routing_enabled: false,
  status: 'active',
  display_order: 10,
  capacity: 255,
  used: 1,
  free: 254,
  created_at: '2026-08-01T08:00:00Z',
  updated_at: '2026-08-01T08:00:00Z',
};

const allocation = {
  id: 'allocation-1',
  pool_space_id: 'pool-1',
  pool_space_key: 'remote_access_v4',
  pool_space_label: 'Remote access IPv4',
  cidr: '172.20.1.0/24',
  node_id: 'node-1',
  node_name: 'Ingress One',
  instance_id: 'instance-1',
  instance_name: 'OpenVPN UDP',
  service_code: 'openvpn',
  purpose: 'client_pool',
  status: 'active',
  route_export: false,
  metadata: {},
  created_at: '2026-08-01T08:00:00Z',
  updated_at: '2026-08-01T08:00:00Z',
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
      <AuthProvider>
        <AddressPoolsPage />
      </AuthProvider>
    </QueryClientProvider>,
  );
}

describe('AddressPoolsPage', () => {
  const calls: FetchCall[] = [];
  let permissions: string[];

  beforeEach(async () => {
    calls.length = 0;
    permissions = ['instance.read', 'settings.manage'];
    window.localStorage.clear();
    await i18n.changeLanguage('en');

    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({ method, path: url.pathname, body });

      if (method === 'GET' && url.pathname === '/api/v1/auth/me') {
        return json({
          user: { id: 'operator-1', username: 'operator', status: 'active' },
          session: { id: 'session-1', expires_at: '2026-08-12T00:00:00Z' },
          roles: ['operator'],
          permissions,
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/address-pools') {
        return json({ spaces: [pool], allocations: [allocation] });
      }
      if (method === 'POST' && url.pathname === '/api/v1/address-pools/spaces') {
        return json({ ...pool, id: 'pool-created', ...body }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/address-pools/spaces/pool-1/routing') {
        return json({ ...pool, routing_enabled: body?.routing_enabled });
      }
      if (method === 'PUT' && url.pathname === '/api/v1/address-pools/spaces/pool-1') {
        return json({ ...pool, ...body });
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 500);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows pool capacity and existing automatic allocations', async () => {
    renderPage();

    expect((await screen.findAllByText('Remote access IPv4')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('172.20.1.0/24').length).toBeGreaterThan(0);
    expect(screen.getAllByText('OpenVPN UDP').length).toBeGreaterThan(0);
    expect(screen.getAllByText('254').length).toBeGreaterThan(0);
  });

  it('creates a managed address space with validated structured input', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'New address pool' }));
    const dialog = screen.getByRole('dialog', { name: 'New address pool' });
    await user.type(within(dialog).getByLabelText('Name'), 'Partner VPN IPv4');
    await user.clear(within(dialog).getByLabelText('Supernet CIDR'));
    await user.type(within(dialog).getByLabelText('Supernet CIDR'), '10.80.0.0/16');
    await user.clear(within(dialog).getByLabelText('First allocatable subnet'));
    await user.type(within(dialog).getByLabelText('First allocatable subnet'), '10.80.10.0/24');
    await user.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/address-pools/spaces')).toBe(true);
    });
    const createCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/address-pools/spaces');
    expect(createCall?.body).toMatchObject({
      label: 'Partner VPN IPv4',
      family: 'ipv4',
      base_cidr: '10.80.0.0/16',
      start_cidr: '10.80.10.0/24',
      allocation_prefix: 24,
      service_scope: 'remote_access',
      routing_enabled: false,
      status: 'active',
    });
  });

  it('updates route export without editing the pool definition', async () => {
    const user = userEvent.setup();
    renderPage();

    const routeButtons = await screen.findAllByRole('button', { name: 'Enable routes' });
    await user.click(routeButtons[0]!);

    await waitFor(() => {
      expect(calls.some((call) => call.path === '/api/v1/address-pools/spaces/pool-1/routing')).toBe(true);
    });
    expect(calls.find((call) => call.path === '/api/v1/address-pools/spaces/pool-1/routing')?.body).toEqual({ routing_enabled: true });
  });

  it('locks structural fields while a pool has active allocations', async () => {
    const user = userEvent.setup();
    renderPage();

    const editButtons = await screen.findAllByRole('button', { name: 'Edit' });
    await user.click(editButtons[0]!);
    const dialog = screen.getByRole('dialog', { name: 'Edit address pool' });

    expect(within(dialog).getByLabelText('Supernet CIDR')).toBeDisabled();
    expect(within(dialog).getByLabelText('First allocatable subnet')).toBeDisabled();
    expect(within(dialog).getByLabelText('Allocated subnet prefix')).toBeDisabled();
    expect(within(dialog).getByLabelText('Scope')).toBeDisabled();
    expect(within(dialog).getByText(/1 active allocations/)).toBeInTheDocument();
  });

  it('keeps mutating actions unavailable without settings.manage', async () => {
    permissions = ['instance.read'];
    renderPage();

    expect(await screen.findByRole('button', { name: 'New address pool' })).toBeDisabled();
    expect((await screen.findAllByRole('button', { name: 'Edit' }))[0]).toBeDisabled();
    expect((await screen.findAllByRole('button', { name: 'Enable routes' }))[0]).toBeDisabled();
    expect(screen.getByText(/settings\.manage/)).toBeInTheDocument();
  });
});
