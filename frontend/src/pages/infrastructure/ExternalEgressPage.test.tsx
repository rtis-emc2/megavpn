import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../../shared/auth/AuthProvider';
import i18n from '../../shared/i18n';
import { ExternalEgressPage } from './ExternalEgressPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const catalog = [
  {
    code: 'openvpn',
    label: 'OpenVPN',
    category: 'vpn',
    runtime_support: 'ready',
    import_formats: ['ovpn'],
    credential_modes: ['file'],
  },
  {
    code: 'l2tp_ipsec',
    label: 'L2TP over IPsec',
    category: 'vpn',
    runtime_support: 'ready',
    import_formats: ['json'],
    credential_modes: ['username_password_psk', 'certificate'],
  },
  {
    code: 'planned_protocol',
    label: 'Planned protocol',
    category: 'vpn',
    runtime_support: 'planned',
    import_formats: ['json'],
    credential_modes: ['file'],
  },
];

const profile = {
  id: 'profile-1',
  display_name: 'Provider Dallas',
  description: 'Corporate provider gateway',
  protocol: 'l2tp_ipsec',
  transport: 'udp_ipsec',
  runtime_support: 'ready',
  status: 'active',
  import_format: 'json',
  endpoint_host: 'vpn.provider.example',
  endpoint_port: 1701,
  secret_purposes: ['config', 'username', 'password', 'preshared_key'],
  deployments: [],
  updated_at: '2026-07-24T08:00:00Z',
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
        <ExternalEgressPage />
      </AuthProvider>
    </QueryClientProvider>,
  );
  return queryClient;
}

describe('ExternalEgressPage', () => {
  const calls: FetchCall[] = [];
  let permissions: string[];
  let profileTotal: number;

  beforeEach(async () => {
    calls.length = 0;
    permissions = ['node.read', 'node.write', 'access_group.read', 'access_group.policy.write'];
    profileTotal = 1;
    window.localStorage.clear();
    await i18n.changeLanguage('en');

    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/auth/me') {
        return json({
          user: { id: 'operator-1', username: 'operator', status: 'active' },
          session: { id: 'session-1', expires_at: '2026-08-08T00:00:00Z' },
          roles: ['operator'],
          permissions,
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/external-egress/catalog') return json(catalog);
      if (method === 'GET' && url.pathname === '/api/v1/external-egress/profiles') return json([profile]);
      if (method === 'GET' && url.pathname === '/api/v1/external-egress/profiles:page') {
        return json({
          items: [{
            ...profile,
            deployments: undefined,
            deployment_count: 1,
            active_deployment_count: 0,
            pending_deployment_count: 0,
            attention_deployment_count: 1,
          }],
          total: profileTotal,
          limit: Number(url.searchParams.get('limit') || 25),
          offset: Number(url.searchParams.get('offset') || 0),
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/external-egress/profiles/profile-1') return json(profile);
      if (method === 'GET' && url.pathname === '/api/v1/external-egress/profiles/profile-created') {
        return json({ ...profile, id: 'profile-created', display_name: 'Primary L2TP' });
      }
      if (method === 'GET' && url.pathname === '/api/v1/nodes') {
        return json([
          { id: 'node-1', name: 'Ingress One', role: 'ingress', status: 'online' },
        ]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/instances') return json([]);
      if (method === 'POST' && url.pathname === '/api/v1/external-egress/import:preview') {
        return json({
          protocol: body?.protocol,
          transport: 'udp_ipsec',
          endpoint_host: 'vpn.provider.example',
          endpoint_port: 1701,
          runtime_support: 'ready',
          import_format: 'json',
          required_secrets: ['username', 'password', 'preshared_key'],
          warnings: [],
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/external-egress/profiles') {
        return json({
          ...profile,
          id: 'profile-created',
          display_name: body?.display_name,
          description: body?.description,
        }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/external-egress/profiles/profile-1/deployments') {
        return json({
          id: 'deployment-1',
          profile_id: 'profile-1',
          node_id: 'node-1',
          node_name: 'Ingress One',
          status: 'pending',
        }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/external-egress/deployments/deployment-1/apply') {
        return json({ id: 'job-1', type: 'node.external_egress.apply', status: 'queued' }, 202);
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 500);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('creates an L2TP/IPsec profile through preview without a client-supplied profile key', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'New profile' }));
    const dialog = screen.getByRole('dialog', { name: 'New profile' });
    expect(within(dialog).queryByRole('option', { name: 'Planned protocol' })).not.toBeInTheDocument();
    await user.type(within(dialog).getByLabelText('Name'), 'Primary L2TP');
    await user.selectOptions(within(dialog).getByLabelText('Protocol'), 'l2tp_ipsec');
    await user.type(within(dialog).getByLabelText('Provider server'), 'vpn.provider.example');
    await user.type(within(dialog).getByLabelText('Username'), 'provider-user');
    await user.type(within(dialog).getByLabelText('Password'), 'provider-password');
    await user.type(within(dialog).getByLabelText('Pre-shared key'), 'provider-psk');
    await user.click(within(dialog).getByRole('button', { name: 'Validate settings' }));

    await within(dialog).findByText('Settings validated');
    await user.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/external-egress/profiles')).toBe(true);
    });
    const createCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/external-egress/profiles');
    expect(createCall?.body).toMatchObject({
      display_name: 'Primary L2TP',
      protocol: 'l2tp_ipsec',
      status: 'active',
      endpoint_host: 'vpn.provider.example',
      endpoint_port: 1701,
      transport: 'udp_ipsec',
      import_format: 'json',
      config_json: {
        auth_method: 'psk',
      },
      secrets: {
        username: 'provider-user',
        password: 'provider-password',
        preshared_key: 'provider-psk',
      },
    });
    expect(createCall?.body).not.toHaveProperty('profile_key');
  });

  it('creates one node deployment and then queues its apply job', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click((await screen.findAllByRole('button', { name: 'Open' }))[0]!);
    const details = await screen.findByRole('dialog', { name: 'Provider Dallas' });
    await user.click(within(details).getByRole('button', { name: 'Deploy' }));
    const dialog = screen.getByRole('dialog', { name: 'Deploy: Provider Dallas' });
    await user.selectOptions(within(dialog).getByLabelText('Node'), 'node-1');
    await user.click(within(dialog).getByRole('button', { name: 'Deploy and apply' }));

    await waitFor(() => {
      expect(calls.some((call) => call.path === '/api/v1/external-egress/deployments/deployment-1/apply')).toBe(true);
    });
    const deployCall = calls.find((call) => call.path === '/api/v1/external-egress/profiles/profile-1/deployments');
    expect(deployCall?.body).toEqual({
      node_id: 'node-1',
      desired_status: 'active',
      routing_table: 'auto',
      route_metric: 100,
      config_json: {},
    });
    const deploymentIndex = calls.findIndex((call) => call.path === '/api/v1/external-egress/profiles/profile-1/deployments');
    const applyIndex = calls.findIndex((call) => call.path === '/api/v1/external-egress/deployments/deployment-1/apply');
    expect(deploymentIndex).toBeGreaterThan(-1);
    expect(applyIndex).toBeGreaterThan(deploymentIndex);
  });

  it('requires both node and access-group write permissions', async () => {
    permissions = ['node.read', 'node.write', 'access_group.read'];
    renderPage();

    expect(await screen.findByText(/write operations require node\.write and access_group\.policy\.write/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'New profile' })).toBeDisabled();
  });

  it('opens full profile diagnostics in a closeable drawer', async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click((await screen.findAllByRole('button', { name: 'Open' }))[0]!);
    const details = await screen.findByRole('dialog', { name: 'Provider Dallas' });
    expect(within(details).getByText('Node deployments')).toBeInTheDocument();
    expect(calls.some((call) => call.path === '/api/v1/external-egress/profiles/profile-1')).toBe(true);

    await user.click(within(details).getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('dialog', { name: 'Provider Dallas' })).not.toBeInTheDocument();
  });

  it('surfaces deployment attention in the compact catalog without loading full diagnostics', async () => {
    renderPage();

    expect(await screen.findAllByText('Requires attention')).not.toHaveLength(0);
    expect(screen.getAllByText('1 node')).not.toHaveLength(0);
    expect(calls.some((call) => call.path === '/api/v1/external-egress/profiles/profile-1')).toBe(false);
  });

  it('paginates the profile catalog on the server', async () => {
    const user = userEvent.setup();
    profileTotal = 26;
    renderPage();

    expect(await screen.findByText('Page 1 of 2 · 26 profiles')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Next' }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'GET' && call.path.includes('/api/v1/external-egress/profiles:page') && call.path.includes('offset=25'))).toBe(true);
    });
  });

  it('filters deployment health on the server', async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findAllByText('Requires attention');
    await user.selectOptions(screen.getByLabelText('Deployment state'), 'attention');
    await user.click(screen.getByRole('button', { name: 'Apply filters' }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'GET' && call.path.includes('/api/v1/external-egress/profiles:page') && call.path.includes('health=attention'))).toBe(true);
    });
  });
});
