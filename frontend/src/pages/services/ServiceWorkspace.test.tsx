import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import { RuntimeArtifactsPage } from './RuntimeArtifactsPage';
import { ServicePacksPage } from './ServicePacksPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const pack = {
  key: 'xray_vless_reality',
  label: 'Xray VLESS / Reality',
  description: 'Create managed Xray VLESS instance',
  base_name_template: 'edge-xray',
  endpoint_hint: 'vpn.example.test',
  requires_endpoint_host: true,
  status: 'active',
  source: 'catalog',
  version: 3,
  components: [
    { label: 'Xray VLESS / Reality', service_code: 'xray-core', preset_key: 'reality_tcp', endpoint_port: 443, requires_endpoint_host: true, spec: { redacted: true } },
  ],
  platform_notes: ['<script>not html</script>'],
  recommendations: ['Check DNS before apply'],
};

const runtimeArtifact = {
  id: 'artifact-runtime-1',
  name: 'xray-linux-amd64',
  kind: 'runtime_binary',
  service_code: 'xray-core',
  version: '1.8.24',
  architecture: 'amd64',
  storage_path: 'sha256/aa/binary',
  size_bytes: 1024,
  sha256: 'abcdef1234567890',
  status: 'active',
  metadata: { release_notes: '<img src=x onerror=alert(1)>' },
  created_at: '2026-07-08T10:00:00Z',
  updated_at: '2026-07-08T10:05:00Z',
};

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function job(id: string, type = 'instance.apply') {
  return { id, type, status: 'queued', created_at: '2026-07-08T11:00:00Z', result: {} };
}

function renderWithQuery(element: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>{element}</QueryClientProvider>
    </MemoryRouter>,
  );
}

describe('Services workspace', () => {
  const calls: FetchCall[] = [];
  let createStatus = 201;

  beforeEach(async () => {
    calls.length = 0;
    createStatus = 201;
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/service-packs') return json([pack]);
      if (method === 'GET' && url.pathname === '/api/v1/nodes') return json([{ id: 'node-1', name: 'Edge One', status: 'active' }]);
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/')) return json(job(url.pathname.split('/')[4]));
      if (method === 'GET' && url.pathname.endsWith('/logs')) return json([]);
      if (method === 'PUT' && url.pathname === '/api/v1/service-packs/xray_vless_reality') return json({ ...pack, label: body?.label || pack.label });
      if (method === 'POST' && url.pathname === '/api/v1/service-packs/xray_vless_reality/disable') return json({ ...pack, status: 'disabled' });
      if (method === 'DELETE' && url.pathname === '/api/v1/service-packs/xray_vless_reality') return json({ ...pack, status: 'deleted' });
      if (method === 'POST' && url.pathname === '/api/v1/service-packs/xray_vless_reality/instances') {
        if (createStatus === 403) return json({ error: 'settings.manage permission required' }, 403);
        if (createStatus === 422) return json({ error: 'node_id is invalid' }, 422);
        if (createStatus === 409) return json({ error: 'endpoint port conflict' }, 409);
        return json({
          status: 'ok',
          service_pack_key: pack.key,
          created_instances: [{ id: 'instance-created', name: 'edge-xray', service_code: 'xray-core' }],
          existing_instances: [],
          apply_jobs: [job('job-apply')],
        }, 201);
      }

      if (method === 'GET' && url.pathname === '/api/v1/binary-artifacts') return json([runtimeArtifact]);
      if (method === 'POST' && url.pathname === '/api/v1/binary-artifacts/import-url') return json({ ...runtimeArtifact, id: 'artifact-imported', name: body?.name || runtimeArtifact.name }, 201);

      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders Services workspace tabs and opens service pack detail', async () => {
    renderWithQuery(<ServicePacksPage />);
    expect(await screen.findByRole('link', { name: 'Instances' })).toHaveAttribute('href', '/services/instances');
    expect(screen.getByRole('link', { name: 'Service Packs' })).toHaveAttribute('href', '/services/service-packs');
    expect(screen.getByRole('link', { name: 'Runtime Artifacts' })).toHaveAttribute('href', '/services/runtime-artifacts');
    expect((await screen.findAllByText('Xray VLESS / Reality')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await screen.findByText(/Check DNS before apply/);
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/service-packs')).toBe(true);
  });

  it('creates instances from a service pack and shows instance and job links', async () => {
    renderWithQuery(<ServicePacksPage />);
    expect((await screen.findAllByText('Xray VLESS / Reality')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Create from pack' })[0]);
    await userEvent.selectOptions(await screen.findByLabelText('Node'), 'node-1');
    await userEvent.clear(screen.getByLabelText('Endpoint host'));
    await userEvent.type(screen.getByLabelText('Endpoint host'), 'vpn.example.test');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create' }).at(-1)!);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/service-packs/xray_vless_reality/instances')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/service-packs/xray_vless_reality/instances')).toBe(true));
    await screen.findByText('job-apply');
    expect(screen.getByRole('link', { name: 'Service instances' })).toHaveAttribute('href', '/services/instances');
    expect(screen.getByRole('link', { name: 'Open Jobs' })).toHaveAttribute('href', '/operations/jobs');
  });

  it('shows service pack create errors distinctly for 403, 422 and 409', async () => {
    for (const [status, message] of [
      [403, /Permission denied: settings.manage permission required/],
      [422, /Validation failed: node_id is invalid/],
      [409, /Conflict: endpoint port conflict/],
    ] as const) {
      createStatus = status;
      renderWithQuery(<ServicePacksPage />);
      expect((await screen.findAllByText('Xray VLESS / Reality')).length).toBeGreaterThan(0);
      await userEvent.click(screen.getAllByRole('button', { name: 'Create from pack' })[0]);
      await userEvent.click((await screen.findAllByRole('button', { name: 'Create' })).at(-1)!);
      await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
      await screen.findByText(message);
      vi.clearAllMocks();
      calls.length = 0;
      cleanup();
    }
  });

  it('updates and deletes service packs through backend management endpoints', async () => {
    renderWithQuery(<ServicePacksPage />);
    expect((await screen.findAllByText('Xray VLESS / Reality')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await userEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/service-packs/xray_vless_reality')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Disabled' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/service-packs/xray_vless_reality/disable')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/service-packs/xray_vless_reality')).toBe(true));
  });

  it('lists, imports and safely renders runtime artifact metadata without delete support', async () => {
    renderWithQuery(<RuntimeArtifactsPage />);
    expect(await screen.findByRole('link', { name: 'Instances' })).toHaveAttribute('href', '/services/instances');
    expect((await screen.findAllByText('xray-linux-amd64')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await screen.findByText(/<img src=x onerror=alert\(1\)>/);
    expect(document.querySelector('img')).toBeNull();
    expect(screen.getByRole('button', { name: /Delete - Backend has no binary runtime artifact delete endpoint/ })).toBeDisabled();

    await userEvent.click(screen.getByRole('button', { name: 'Import artifact' }));
    await userEvent.type(await screen.findByLabelText('Source URL'), 'https://downloads.example.test/xray');
    await userEvent.type(screen.getByLabelText('Expected SHA-256'), 'abcdef1234567890');
    await userEvent.type(screen.getByLabelText('Name'), 'xray-imported');
    await userEvent.click(screen.getAllByRole('button', { name: 'Import artifact' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/binary-artifacts/import-url')).toBe(true));
  });

  it('keeps Services pages away from raw API calls and legacy workflow links', () => {
    expect(String(ServicePacksPage)).not.toContain('/api/v1');
    expect(String(RuntimeArtifactsPage)).not.toContain('/api/v1');
    expect(String(ServicePacksPage)).not.toContain('/legacy');
    expect(String(RuntimeArtifactsPage)).not.toContain('/legacy');
    expect(String(ServicePacksPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);
    expect(String(RuntimeArtifactsPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);
  });
});
