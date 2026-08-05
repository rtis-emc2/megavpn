import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import backhaulSource from '../infrastructure/BackhaulPage.tsx?raw';
import { BackhaulPage } from '../infrastructure/BackhaulPage';
import routePolicySource from './RoutePolicyPage.tsx?raw';
import { RoutePolicyPage } from './RoutePolicyPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const backhaulSecret = 'do-not-render-backhaul-secret';
const backhaulSecretRef = 'secret-ref-backhaul';

const backhaulLink = {
  id: 'link-1',
  name: 'edge-backhaul',
  status: 'active',
  ingress_node_id: 'node-1',
  egress_node_id: 'node-2',
  selected_transport_id: 'transport-1',
  desired_driver: 'wireguard',
  routing_table: 21001,
  route_metric: 50,
  updated_at: '2026-07-09T08:00:00Z',
  transports: [
    {
      id: 'transport-1',
      link_id: 'link-1',
      driver: 'wireguard',
      status: 'active',
      endpoint_host: '198.51.100.10',
      endpoint_port: 51820,
      protocol: 'udp',
      interface_name: 'mgbh0',
      tunnel_cidr: '10.240.1.0/30',
      ingress_address: '10.240.1.1',
      egress_address: '10.240.1.2',
      health: { status: 'healthy' },
      config: { private_key: backhaulSecret },
      secret_refs: { private_key: backhaulSecretRef },
    },
    {
      id: 'transport-2',
      link_id: 'link-1',
      driver: 'openvpn_udp',
      status: 'active',
      endpoint_host: '198.51.100.11',
      endpoint_port: 1194,
      protocol: 'udp',
      interface_name: 'mgbh1',
      tunnel_cidr: '10.240.2.0/30',
      ingress_address: '10.240.2.1',
      egress_address: '10.240.2.2',
      health: { status: 'standby' },
      config: { psk: backhaulSecret },
      secret_refs: { psk: backhaulSecretRef },
    },
  ],
};

const nodes = [
  { id: 'node-1', name: 'ingress-a', role: 'vpn_ingress', address: '203.0.113.10', status: 'active', updated_at: '2026-07-09T08:00:00Z' },
  { id: 'node-2', name: 'egress-b', role: 'egress', address: '203.0.113.11', status: 'active', updated_at: '2026-07-09T08:01:00Z' },
];

const routePreview = {
  status: 'ok',
  node_id: 'node-1',
  node_name: 'ingress-a',
  node_role: 'vpn_ingress',
  node_address: '203.0.113.10',
  generated_at: '2026-07-09T08:02:00Z',
  revision: 'route-rev-1',
  output_path: '/run/megavpn/routes.json',
  summary: { route_count: 1, system_route_count: 1 },
  kernel: { managed_table: 21001, primitives: ['ip rule'] },
  warnings: [{ type: 'safety', message: 'review asymmetric routing before apply' }],
  routes: [{ destination: '10.80.0.0/16', table: 21001, interface_name: 'mgbh0', source_identity: '[redacted]', reasons: ['managed backhaul'] }],
  system_routes: [{ destination: '<script>not-html</script>', table: 'main', interface_name: 'eth0', status: 'observed' }],
};

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function renderWithQuery(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        {ui}
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return queryClient;
}

function firstEnabledButton(name: string | RegExp): HTMLButtonElement {
  const button = screen.getAllByRole('button', { name }).find((item) => !(item as HTMLButtonElement).disabled);
  if (!button) throw new Error(`enabled button not found: ${String(name)}`);
  return button as HTMLButtonElement;
}

function latestDialog() {
  const dialogs = screen.getAllByRole('dialog');
  return dialogs[dialogs.length - 1];
}

describe('Backhaul and RoutePolicy workflows', () => {
  const calls: FetchCall[] = [];
  const failures: Record<string, number> = {};
  let consoleSpy: ReturnType<typeof vi.spyOn>;
  let storageSetSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(async () => {
    calls.length = 0;
    Object.keys(failures).forEach((key) => delete failures[key]);
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    storageSetSpy = vi.spyOn(Storage.prototype, 'setItem');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      const path = `${url.pathname}${url.search}`;
      calls.push({ method, path, body });

      const failureStatus = failures[`${method} ${url.pathname}`];
      if (failureStatus) return json({ error: `backend returned ${failureStatus}` }, failureStatus);

      if (method === 'GET' && url.pathname === '/api/v1/backhaul-links') return json([backhaulLink]);
      if (method === 'GET' && url.pathname === '/api/v1/backhaul-links/link-1') return json(backhaulLink);
      if (method === 'POST' && url.pathname === '/api/v1/backhaul-links/link-1/apply') {
        return json({ jobs: [{ id: 'job-backhaul-apply', type: 'node.backhaul.apply', status: 'queued' }], job_count: 1 }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/backhaul-links/link-1/probe') {
        return json({ jobs: [{ id: 'job-backhaul-probe', type: 'node.backhaul.probe', status: 'queued' }], job_count: 1 }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/backhaul-links/link-1/promote') {
        return json({ link: { ...backhaulLink, selected_transport_id: body?.transport_id }, jobs: [{ id: 'job-backhaul-promote', type: 'node.route_policy.apply', status: 'queued' }], job_count: 1 }, 202);
      }
      if (method === 'PATCH' && url.pathname === '/api/v1/backhaul-links/link-1/route') {
        return json({ link: { ...backhaulLink, route_enabled: body?.enabled }, jobs: [{ id: 'job-backhaul-route', type: 'node.route_policy.apply', status: 'queued' }], job_count: 1 }, 202);
      }

      if (method === 'GET' && url.pathname === '/api/v1/nodes') return json(nodes);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1') return json(nodes[0]);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-2') return json(nodes[1]);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/routes/preview') return json(routePreview);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/routes/apply') {
        return json({ status: 'queued', message: 'route policy apply queued', job: { id: 'job-route-apply', type: 'node.route_policy.apply', status: 'queued' } }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/routes/cleanup') {
        return json({ status: 'queued', message: 'route policy cleanup queued', job: { id: 'job-route-cleanup', type: 'node.route_policy.cleanup', status: 'queued' } }, 202);
      }
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/')) {
        const jobId = url.pathname.split('/').at(-1) || 'job';
        if (url.pathname.endsWith('/logs')) return json([]);
        return json({ id: jobId, type: jobId.includes('backhaul') ? 'node.backhaul.apply' : 'node.route_policy.apply', status: 'queued', result: {} });
      }

      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    expect(calls.every((call) => !call.path.includes('/legacy'))).toBe(true);
    expect(JSON.stringify(consoleSpy.mock.calls)).not.toContain(backhaulSecret);
    expect(JSON.stringify(storageSetSpy.mock.calls)).not.toContain(backhaulSecret);
    consoleSpy.mockRestore();
    storageSetSpy.mockRestore();
    vi.unstubAllGlobals();
    cleanup();
  });

  it('loads Backhaul list/detail safely and does not render transport secrets', async () => {
    renderWithQuery(<BackhaulPage />);
    expect((await screen.findAllByText('edge-backhaul')).length).toBeGreaterThan(0);

    await userEvent.click(firstEnabledButton('Open'));
    expect(await screen.findByText('Secret-safe detail')).toBeInTheDocument();
    expect(screen.getAllByText('transport-1').length).toBeGreaterThan(0);
    expect(screen.queryByText(backhaulSecret)).not.toBeInTheDocument();
    expect(screen.queryByText(backhaulSecretRef)).not.toBeInTheDocument();
  });

  it('runs Backhaul apply, probe, promote and route-state actions through backend confirmations', async () => {
    renderWithQuery(<BackhaulPage />);
    expect((await screen.findAllByText('edge-backhaul')).length).toBeGreaterThan(0);

    await userEvent.click(firstEnabledButton('Apply'));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/backhaul-links/link-1/apply')).toBe(false);
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/backhaul-links/link-1/apply')).toBe(true));
    expect(await screen.findByText('job-backhaul-apply')).toBeInTheDocument();

    await userEvent.click(firstEnabledButton('Open'));
    await userEvent.click(firstEnabledButton('Probe'));
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/backhaul-links/link-1/probe')).toBe(true));

    await userEvent.click(firstEnabledButton('Promote'));
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/backhaul-links/link-1/promote' && call.body?.transport_id === 'transport-2')).toBe(true));

    await userEvent.click(firstEnabledButton('Disable route projection'));
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PATCH' && call.path === '/api/v1/backhaul-links/link-1/route' && call.body?.enabled === false)).toBe(true));
  });

  it('loads Route Policy list/detail, previews before apply and shows route jobs', async () => {
    renderWithQuery(<RoutePolicyPage />);
    expect((await screen.findAllByText('ingress-a')).length).toBeGreaterThan(0);

    await userEvent.click(firstEnabledButton('Preview'));
    await waitFor(() => expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/nodes/node-1/routes/preview')).toBe(true));
    expect(await screen.findByText('Preview is fresh')).toBeInTheDocument();
    expect(screen.getByText('route-rev-1')).toBeInTheDocument();
    expect(screen.getAllByText('<script>not-html</script>').length).toBeGreaterThan(0);

    await userEvent.click(firstEnabledButton('Apply route policy'));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/routes/apply')).toBe(false);
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/routes/apply')).toBe(true));
    expect(await screen.findByText('job-route-apply')).toBeInTheDocument();

    await userEvent.click(firstEnabledButton('Cleanup route policy'));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/routes/cleanup')).toBe(false);
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/routes/cleanup')).toBe(true));
  });

  it('keeps Route Policy apply disabled when preview becomes stale', async () => {
    renderWithQuery(<RoutePolicyPage />);
    expect((await screen.findAllByText('ingress-a')).length).toBeGreaterThan(0);
    await userEvent.click(firstEnabledButton('Preview'));
    expect(await screen.findByText('Preview is fresh')).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[1]);
    expect(await screen.findByText('Preview is stale')).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Apply route policy' }).every((button) => (button as HTMLButtonElement).disabled)).toBe(true);
  });

  it('surfaces 403, 409 and 422 backend errors without legacy fallback', async () => {
    failures['POST /api/v1/backhaul-links/link-1/apply'] = 403;
    renderWithQuery(<BackhaulPage />);
    expect((await screen.findAllByText('edge-backhaul')).length).toBeGreaterThan(0);
    await userEvent.click(firstEnabledButton('Apply'));
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    expect(await screen.findByText('backend returned 403')).toBeInTheDocument();
    cleanup();

    failures['POST /api/v1/backhaul-links/link-1/apply'] = 0;
    failures['POST /api/v1/backhaul-links/link-1/promote'] = 409;
    renderWithQuery(<BackhaulPage />);
    expect((await screen.findAllByText('edge-backhaul')).length).toBeGreaterThan(0);
    await userEvent.click(firstEnabledButton('Open'));
    await userEvent.click(firstEnabledButton('Promote'));
    await userEvent.click(within(latestDialog()).getByRole('button', { name: 'Confirm' }));
    expect(await screen.findByText('backend returned 409')).toBeInTheDocument();
    cleanup();

    failures['POST /api/v1/backhaul-links/link-1/promote'] = 0;
    failures['GET /api/v1/nodes/node-1/routes/preview'] = 422;
    renderWithQuery(<RoutePolicyPage />);
    expect((await screen.findAllByText('ingress-a')).length).toBeGreaterThan(0);
    await userEvent.click(firstEnabledButton('Preview'));
    expect(await screen.findByText('backend returned 422')).toBeInTheDocument();
  });

  it('keeps page components free from raw API and legacy calls', () => {
    const rawCallPattern = /(^|[^A-Za-z0-9_])fetch\s*\(|apiRequest|sendJSON|\/api\/v1|\/legacy/;
    expect(backhaulSource).not.toMatch(rawCallPattern);
    expect(routePolicySource).not.toMatch(rawCallPattern);
    expect(routePolicySource).not.toMatch(/dangerouslySetInnerHTML/);
  });
});
