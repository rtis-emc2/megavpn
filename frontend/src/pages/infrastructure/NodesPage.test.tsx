import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../../shared/auth/AuthProvider';
import i18n from '../../shared/i18n';
import { NodesPage } from './NodesPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
  headers: Record<string, string>;
  credentials?: RequestCredentials;
  cache?: RequestCache;
};

const node = {
  id: 'node-1',
  name: 'Edge One',
  kind: 'remote',
  role: 'ingress',
  status: 'online',
  address: '203.0.113.10',
  location_label: 'EU edge',
  os_family: 'ubuntu',
  os_version: '24.04',
  architecture: 'amd64',
  execution_mode: 'agent_managed',
  agent_status: 'online',
  agent_version: '8.0.0-agent',
  agent_protocol_version: 'v1',
  agent_last_seen_at: '2026-07-09T08:00:00Z',
  last_heartbeat_at: '2026-07-09T08:00:00Z',
  created_at: '2026-07-08T10:00:00Z',
  updated_at: '2026-07-09T08:01:00Z',
};

const authPayload = {
  user: {
    id: 'operator-1',
    username: 'admin',
    status: 'active',
  },
  session: {
    id: 'session-1',
    expires_at: '2026-08-10T00:00:00Z',
  },
  roles: ['operator'],
  permissions: ['node.read', 'node.write', 'node.bootstrap'],
};

const diagnostics = {
  node,
  heartbeat_state: 'healthy',
  heartbeat_drift_seconds: 4,
  communication_state: 'connected',
  communication_hint: '<script>alert(1)</script>',
  agent: {
    node_id: 'node-1',
    status: 'online',
    agent_version: '8.0.0-agent',
    protocol_version: 'v1',
    token_hint: 'tok_...',
    token_rotation_status: 'ok',
    last_seen_at: '2026-07-09T08:00:00Z',
    last_job_poll_at: '2026-07-09T08:00:10Z',
    last_job_claim_job_id: 'job-last',
    last_job_result_job_id: 'job-last',
    last_job_result_type: 'node.inventory.sync',
    last_job_result_status: 'succeeded',
    last_inventory_sync_at: '2026-07-09T07:58:00Z',
    last_discovery_sync_at: '2026-07-09T07:59:00Z',
    last_runtime_sync_at: '2026-07-09T08:00:00Z',
  },
  latest_inventory: {
    id: 'inventory-1',
    node_id: 'node-1',
    created_at: '2026-07-09T07:58:00Z',
  },
  discovery_summary: {
    node_id: 'node-1',
    total: 1,
    discovered: 1,
    imported: 0,
    ignored: 0,
    importable_count: 1,
    by_service: { 'xray-core': 1 },
  },
  recent_discoveries: [],
};

const inventory = {
  id: 'inventory-1',
  node_id: 'node-1',
  payload: {
    hostname: 'edge-one',
    kernel: '<script>inventory()</script>',
    packages: ['xray-core', 'nginx'],
  },
  created_at: '2026-07-09T07:58:00Z',
};

const capabilities = [
  {
    id: 'cap-1',
    node_id: 'node-1',
    capability_code: 'xray-core',
    version: '25.6.8',
    status: 'available',
    source: 'agent',
    detected_at: '2026-07-09T07:58:30Z',
  },
];

const discoveries = [
  {
    id: 'discovery-1',
    node_id: 'node-1',
    service_code: 'xray-core',
    name: 'xray-live',
    systemd_unit: 'xray.service',
    config_path: '/etc/xray/config.json',
    status: 'discovered',
    source: 'agent',
    confidence: 95,
    endpoint_host: 'vpn.example.test',
    endpoint_port: 443,
    payload: { rendered: '<b>not html</b>' },
    detected_at: '2026-07-09T07:59:00Z',
  },
];

const accessMethods = [
  {
    id: 'access-1',
    node_id: 'node-1',
    method: 'ssh',
    is_enabled: true,
    ssh_host: 'edge-one.example.test',
    ssh_port: 22,
    ssh_user: 'ubuntu',
    ssh_host_key_sha256: 'SHA256:oldfingerprint',
    auth_type: 'ssh_key',
    secret_configured: true,
    created_at: '2026-07-08T10:00:00Z',
    updated_at: '2026-07-08T10:00:00Z',
  },
];

const enrollmentTokens = [
  {
    id: 'token-1',
    node_id: 'node-1',
    token_hint: 'enroll...hint',
    status: 'active',
    expires_at: '2026-07-10T08:00:00Z',
    created_at: '2026-07-09T08:00:00Z',
  },
];

const bootstrapRuns = [
  {
    id: 'bootstrap-run-1',
    node_id: 'node-1',
    job_id: 'job-bootstrap-old',
    status: 'queued',
    bootstrap_mode: 'ssh_bootstrap',
    request_payload: { bootstrap_mode: 'ssh_bootstrap' },
    created_at: '2026-07-09T08:00:00Z',
  },
  {
    id: 'bootstrap-run-manual',
    node_id: 'node-1',
    job_id: 'job-bootstrap-manual',
    status: 'succeeded',
    bootstrap_mode: 'manual_bundle',
    request_payload: { bootstrap_mode: 'manual_bundle' },
    result_payload: { manual_bundle_available: true },
    manual_bundle_available: true,
    started_at: '2026-07-09T08:01:00Z',
    finished_at: '2026-07-09T08:02:00Z',
    created_at: '2026-07-09T08:00:30Z',
  },
];

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function job(id: string, type = 'node.inventory.sync') {
  return {
    id,
    type,
    status: 'queued',
    scope_type: 'node',
    scope_id: 'node-1',
    node_id: 'node-1',
    result: { queued: true },
    created_at: '2026-07-09T08:05:00Z',
  };
}

function trackedBody(method: string, path: string, body?: Record<string, unknown>): Record<string, unknown> | undefined {
  if (!body) return undefined;
  if (method === 'POST' && path === '/api/v1/nodes/node-1/access-methods/ssh') {
    const fields = Object.keys(body).sort();
    const { private_key: privateKey, ...safeBody } = body;
    return {
      ...safeBody,
      private_key_present: typeof privateKey === 'string' && privateKey.length > 0,
      request_fields: fields,
    };
  }
  return body;
}

function trackedHeaders(headers: HeadersInit | undefined): Record<string, string> {
  const output: Record<string, string> = {};
  new Headers(headers || {}).forEach((value, key) => {
    output[key] = value;
  });
  return output;
}

function renderPage(auth: typeof authPayload = authPayload) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  queryClient.setQueryData(['auth', 'session'], auth);
  render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <NodesPage />
        </AuthProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return queryClient;
}

async function openNode(auth: typeof authPayload = authPayload) {
  renderPage(auth);
  expect((await screen.findAllByText('Edge One')).length).toBeGreaterThan(0);
  await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
  await screen.findByRole('heading', { name: 'Edge One' });
}

function activeDialog(): HTMLElement {
  const dialogs = screen.getAllByRole('dialog');
  const dialog = dialogs[dialogs.length - 1];
  if (!dialog) throw new Error('dialog not found');
  return dialog;
}

describe('NodesPage', () => {
  const calls: FetchCall[] = [];
  let actionErrors: Record<string, number>;
  let nodeList: Array<typeof node>;
  let createdNode: typeof node | null;
  let currentNode: typeof node;
  let currentAccessMethods: typeof accessMethods;
  let manualBundleRevealContent: string;
  let manualBundleDownloadContent: string;

  beforeEach(async () => {
    calls.length = 0;
    actionErrors = {};
    currentNode = { ...node };
    createdNode = null;
    nodeList = [currentNode];
    currentAccessMethods = accessMethods.map((method) => ({ ...method }));
    manualBundleRevealContent = '';
    manualBundleDownloadContent = '';
    window.localStorage.clear();
    window.sessionStorage.clear();
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const path = `${url.pathname}${url.search}`;
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({
        method,
        path,
        body: trackedBody(method, path, body),
        headers: trackedHeaders(init?.headers),
        credentials: init?.credentials,
        cache: init?.cache,
      });

      if (method === 'GET' && url.pathname === '/api/v1/auth/me') return json(authPayload);
      if (method === 'GET' && url.pathname === '/api/v1/nodes') return json(nodeList);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1') return json(currentNode);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-created' && createdNode) return json(createdNode);
      if (method === 'POST' && url.pathname === '/api/v1/nodes') {
        if (actionErrors.create === 403) return json({ error: 'node.write permission required' }, 403);
        if (actionErrors.create === 409) return json({ error: 'node name "Edge Two" is already used by an active node' }, 409);
        if (actionErrors.create === 422) return json({ error: 'invalid node payload', fields: { address: 'Address is invalid' } }, 422);
        createdNode = {
          ...node,
          id: 'node-created',
          name: String(body?.name || ''),
          kind: String(body?.kind || 'remote'),
          role: String(body?.role || 'egress'),
          status: 'draft',
          address: String(body?.address || ''),
          location_label: String(body?.location_label || ''),
          os_family: String(body?.os_family || 'linux'),
          os_version: String(body?.os_version || 'unknown'),
          architecture: String(body?.architecture || 'amd64'),
          execution_mode: String(body?.execution_mode || 'agent_managed'),
          agent_status: 'unknown',
          agent_version: '',
          agent_protocol_version: '',
          agent_last_seen_at: '',
          last_heartbeat_at: '',
          created_at: '2026-07-09T08:10:00Z',
          updated_at: '2026-07-09T08:10:00Z',
        };
        nodeList = [currentNode, createdNode];
        return json(createdNode, 201);
      }
      if (method === 'PUT' && url.pathname === '/api/v1/nodes/node-1') {
        if (actionErrors.update === 404) return json({ error: 'node not found' }, 404);
        if (actionErrors.update === 409) return json({ error: 'node name "Edge Conflict" is already used by an active node' }, 409);
        if (actionErrors.update === 422) return json({ error: 'invalid node payload', fields: { name: 'Name is invalid' } }, 422);
        currentNode = {
          ...currentNode,
          name: String(body?.name || currentNode.name),
          kind: String(body?.kind || currentNode.kind),
          role: String(body?.role || currentNode.role),
          address: String(body?.address || currentNode.address),
          location_label: String(body?.location_label || ''),
          os_family: String(body?.os_family || currentNode.os_family),
          os_version: String(body?.os_version || currentNode.os_version),
          architecture: String(body?.architecture || currentNode.architecture),
          execution_mode: String(body?.execution_mode || currentNode.execution_mode),
          updated_at: '2026-07-09T08:11:00Z',
        };
        nodeList = createdNode ? [currentNode, createdNode] : [currentNode];
        return json(currentNode);
      }
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/diagnostics') return json(diagnostics);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/inventory') return json(inventory);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/capabilities') return json(capabilities);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/capabilities/drift') {
        return json({ node_id: 'node-1', required: ['nginx', 'xray-core'], drift: [{ capability_code: 'xray-core', desired: 'available', actual: 'available', in_sync: true }] });
      }
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/capabilities/install-events') {
        return json([{ id: 'event-1', node_id: 'node-1', job_id: 'job-install', capability_code: 'xray-core', strategy: 'binary_repository', status: 'queued', summary: 'capability install queued', created_at: '2026-07-09T08:05:00Z' }]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/services/installers') {
        return json([{ service_code: 'xray-core', strategy: 'binary_repository', channel: 'stable', description: 'Install from repository' }]);
      }
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/services/discovered') return json(discoveries);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/services/discovery-summary') {
        return json({ node_id: 'node-1', total: 1, discovered: 1, imported: 0, ignored: 0, importable_count: 1, by_service: { 'xray-core': 1 } });
      }
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/enrollment-tokens') return json(enrollmentTokens);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/access-methods') return json(currentAccessMethods);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/bootstrap-runs') return json(bootstrapRuns);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/bootstrap-runs/bootstrap-run-manual/bundle/reveal') {
        const status = actionErrors.bundleReveal;
        if (status) {
          const messages: Record<number, string> = {
            400: 'invalid bootstrap bundle request',
            403: 'node.bootstrap permission required',
            404: 'manual bundle no longer available',
            409: 'manual bundle is unresolved',
            413: 'manual bundle is too large',
            500: 'audit sink unavailable',
            503: 'secret storage is unavailable',
          };
          return json({ error: messages[status] || 'manual bundle reveal failed' }, status);
        }
        return json({
          node_id: 'node-1',
          bootstrap_run_id: 'bootstrap-run-manual',
          filename: 'megavpn-agent-edge-one-bootstrap.env',
          agent_bootstrapenv: manualBundleRevealContent,
          revealed_at: '2026-07-09T08:03:00Z',
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/bootstrap-runs/bootstrap-run-manual/bundle/download') {
        const status = actionErrors.bundleDownload;
        if (status) return json({ error: 'manual bundle download failed' }, status);
        return new Response(manualBundleDownloadContent, {
          status: 200,
          headers: {
            'content-type': 'text/plain',
            'content-disposition': 'attachment; filename="megavpn-agent-edge-one-bootstrap.env"',
          },
        });
      }
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/')) return json(job(url.pathname.split('/')[4]));
      if (method === 'GET' && url.pathname.endsWith('/logs')) return json([{ level: 'info', message: 'queued' }]);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/maintenance/enable') {
        if (actionErrors.maintenance) return json({ error: 'node.write permission required' }, actionErrors.maintenance);
        return json({ ...node, status: 'maintenance' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/maintenance/disable') return json(node);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/inventory/sync') return json(job('job-inventory', 'node.inventory.sync'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/capabilities/install') return json(job('job-install', 'node.capability.install'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/capabilities/verify') {
        if (actionErrors.verify) return json({ error: 'service_code is required' }, actionErrors.verify);
        return json(job('job-verify', 'node.capability.verify'), 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/diagnostics/retry-inventory') return json({ status: 'queued', message: 'inventory sync queued', job: job('job-retry-inventory') }, 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/diagnostics/retry-discovery') return json({ status: 'queued', message: 'discovery sync queued', job: job('job-retry-discovery', 'node.discovery.sync') }, 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/diagnostics/channel-probe') return json({ status: 'queued', message: 'probe queued', job: job('job-probe', 'node.channel.probe') }, 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/diagnostics/requeue-stuck-job') return json({ status: 'queued', message: 'requeued', job: job('job-requeued', 'node.stuck.requeue') }, 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/diagnostics/reconcile-runtime') return json({ queued: 2, jobs: [job('job-reconcile-1', 'node.inventory.sync'), job('job-reconcile-2', 'node.discovery.sync')], warnings: [] }, 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/services/discover') return json(job('job-discover', 'node.services.discover'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/services/discovered/discovery-1/import') {
        if (actionErrors.import) return json({ error: 'service already imported' }, actionErrors.import);
        return json({ id: 'instance-imported', node_id: 'node-1', service_code: 'xray-core', name: 'xray-live', status: 'active' }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/services/import-all') return json([{ id: 'instance-imported', node_id: 'node-1', service_code: 'xray-core', name: 'xray-live', status: 'active' }], 201);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/enrollment-token') {
        return json({ id: 'token-new', node_id: 'node-1', token: 'enroll-secret-token', token_hint: 'enroll...token', status: 'active', expires_at: '2026-07-10T08:05:00Z', created_at: '2026-07-09T08:05:00Z' }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/enrollment-token/rotate') {
        return json({ id: 'token-rotated', node_id: 'node-1', token: 'enroll-rotated-token', token_hint: 'rotate...token', status: 'active', expires_at: '2026-07-10T08:06:00Z', created_at: '2026-07-09T08:06:00Z' });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/nodes/node-1/enrollment-tokens/token-1') {
        return json({ ...enrollmentTokens[0], status: 'revoked' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/ssh/host-key-scan') {
        if (actionErrors.scanHostKey === 500) return json({ error: 'ssh-keyscan failed' }, 500);
        if (actionErrors.scanHostKey === 204) return json({ host: body?.ssh_host || 'edge-one.example.test', port: body?.ssh_port || 22, fingerprints: [] });
        return json({ host: 'edge-one.example.test', port: 22, fingerprints: [{ fingerprint: 'SHA256:newfingerprint', algorithm: 'ssh-ed25519', bits: 256, known_host_line: 'edge-one.example.test ssh-ed25519 AAAA' }] });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/access-methods/ssh') {
        if (actionErrors.createSSHAccess) {
          const messages: Record<number, string> = {
            400: 'private_key is not a valid unencrypted SSH private key',
            403: 'node.bootstrap permission required',
            404: 'node not found',
            409: 'ssh access method already exists',
            422: 'invalid ssh access method payload',
            503: 'secret storage is not configured',
            500: 'ssh access method create failed',
          };
          return json({ error: messages[actionErrors.createSSHAccess] || 'ssh access method create failed' }, actionErrors.createSSHAccess);
        }
        const createdAccessMethod = {
          id: 'access-created',
          node_id: 'node-1',
          method: 'ssh',
          is_enabled: body?.is_enabled !== false,
          ssh_host: String(body?.ssh_host || ''),
          ssh_port: Number(body?.ssh_port || 22),
          ssh_user: String(body?.ssh_user || ''),
          ssh_host_key_sha256: String(body?.ssh_host_key_sha256 || ''),
          auth_type: 'ssh_key',
          secret_configured: true,
          created_at: '2026-07-09T08:09:00Z',
          updated_at: '2026-07-09T08:09:00Z',
        };
        currentAccessMethods = [...currentAccessMethods, createdAccessMethod];
        return json(createdAccessMethod, 201);
      }
      if (method === 'PUT' && url.pathname === '/api/v1/nodes/node-1/access-methods') {
        currentAccessMethods = ((body?.items as typeof accessMethods | undefined) || currentAccessMethods).map((method) => ({ ...method }));
        return json(currentAccessMethods);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/bootstrap') {
        return json({ job: job('job-bootstrap', 'node.bootstrap'), bootstrap_run: { id: 'bootstrap-run-new', node_id: 'node-1', job_id: 'job-bootstrap', status: 'queued', bootstrap_mode: body?.bootstrap_mode || 'ssh_bootstrap', created_at: '2026-07-09T08:07:00Z' } }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/agent-token/rotate') return json(job('job-agent-rotate', 'node.agent.rotate_token'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/ssh/sessions') {
        return json({ session_id: 'ssh-session-ticket', node_id: 'node-1', expires_at: '2026-07-09T08:06:30Z', endpoint: { server_side_proxy_only: true } }, 201);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/nodes/node-1') return json({ ...node, status: 'retired' });
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/force-retire') return json({ status: 'retired', node: { ...node, status: 'retired' } });
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('creates a node through the typed API layer and keeps backend lifecycle state authoritative', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    const consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => undefined);

    renderPage();
    const createButton = await screen.findByRole('button', { name: 'Create node' });
    expect(createButton).toBeEnabled();
    await userEvent.click(createButton);

    const dialog = activeDialog();
    await userEvent.type(within(dialog).getByLabelText('Name'), 'Edge Two');
    await userEvent.type(within(dialog).getByLabelText('Address'), '198.51.100.20');
    await userEvent.type(within(dialog).getByLabelText('Location label'), 'US edge');
    await userEvent.selectOptions(within(dialog).getByLabelText('Node kind'), 'remote');
    await userEvent.selectOptions(within(dialog).getByLabelText('Role'), 'egress');
    await userEvent.selectOptions(within(dialog).getByLabelText('Execution mode'), 'agent_managed');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create node' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes')).toBe(true));
    const createCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes');
    expect(createCall?.body).toMatchObject({
      name: 'Edge Two',
      address: '198.51.100.20',
      kind: 'remote',
      role: 'egress',
      location_label: 'US edge',
      os_family: 'linux',
      os_version: 'unknown',
      architecture: 'amd64',
      execution_mode: 'agent_managed',
    });
    expect(createCall?.body).not.toHaveProperty('id');
    expect(createCall?.body).not.toHaveProperty('status');
    expect(createCall?.body).not.toHaveProperty('agent_status');
    expect(createCall?.body).not.toHaveProperty('secret_ref_id');

    await screen.findByRole('heading', { name: 'Edge Two' });
    expect((await screen.findAllByText('draft')).length).toBeGreaterThan(0);
    expect((await screen.findAllByText('unknown')).length).toBeGreaterThan(0);
    expect(calls.filter((call) => call.method === 'GET' && call.path === '/api/v1/nodes').length).toBeGreaterThan(1);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(storageSet).not.toHaveBeenCalled();
    expect(consoleLog).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();
  });

  it('keeps node create disabled without node.write permission', async () => {
    renderPage({ ...authPayload, permissions: ['node.read'] });
    const createButton = await screen.findByRole('button', { name: 'Create node' });
    expect(createButton).toBeDisabled();
    expect(createButton).toHaveAttribute('title', 'Permission required: node.write');
  });

  it('preserves create input on conflict and maps backend field validation', async () => {
    actionErrors.create = 409;
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: 'Create node' }));
    let dialog = activeDialog();
    await userEvent.type(within(dialog).getByLabelText('Name'), 'Edge Two');
    await userEvent.type(within(dialog).getByLabelText('Address'), '198.51.100.20');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create node' }));
    await within(dialog).findByText(/Conflict: node name "Edge Two" is already used by an active node/);
    expect(within(dialog).getByLabelText('Name')).toHaveValue('Edge Two');
    expect(within(dialog).getByLabelText('Address')).toHaveValue('198.51.100.20');

    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    actionErrors.create = 422;
    await userEvent.click(screen.getByRole('button', { name: 'Create node' }));
    dialog = activeDialog();
    await userEvent.type(within(dialog).getByLabelText('Name'), 'Field Error');
    await userEvent.type(within(dialog).getByLabelText('Address'), 'bad address');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create node' }));
    await within(dialog).findByText(/Validation failed: invalid node payload/);
    expect(await within(dialog).findByText('Address is invalid')).toBeInTheDocument();
  });

  it('edits safe node metadata without mutating runtime, lifecycle or secret fields', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    await openNode();

    await userEvent.click(screen.getByRole('button', { name: 'Edit node' }));
    const dialog = activeDialog();
    expect(within(dialog).getByLabelText('Name')).toHaveValue('Edge One');
    expect(within(dialog).getByLabelText('Address')).toHaveValue('203.0.113.10');
    expect(within(dialog).queryByLabelText('Status')).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText('Agent status')).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText(/secret/i)).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText(/token/i)).not.toBeInTheDocument();

    await userEvent.clear(within(dialog).getByLabelText('Name'));
    await userEvent.type(within(dialog).getByLabelText('Name'), 'Edge One Renamed');
    await userEvent.clear(within(dialog).getByLabelText('Address'));
    await userEvent.type(within(dialog).getByLabelText('Address'), '203.0.113.11');
    await userEvent.selectOptions(within(dialog).getByLabelText('Role'), 'egress');
    await userEvent.selectOptions(within(dialog).getByLabelText('Execution mode'), 'ssh_bootstrap');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/nodes/node-1')).toBe(true));
    const updateCall = calls.find((call) => call.method === 'PUT' && call.path === '/api/v1/nodes/node-1');
    expect(updateCall?.body).toMatchObject({
      name: 'Edge One Renamed',
      address: '203.0.113.11',
      role: 'egress',
      execution_mode: 'ssh_bootstrap',
    });
    expect(updateCall?.body).not.toHaveProperty('id');
    expect(updateCall?.body).not.toHaveProperty('status');
    expect(updateCall?.body).not.toHaveProperty('agent_status');
    expect(updateCall?.body).not.toHaveProperty('last_heartbeat_at');
    expect(updateCall?.body).not.toHaveProperty('secret_ref_id');
    await screen.findByText('Node profile updated.');
    expect(calls.filter((call) => call.method === 'GET' && call.path === '/api/v1/nodes/node-1').length).toBeGreaterThan(1);
    expect(calls.filter((call) => call.method === 'GET' && call.path === '/api/v1/nodes').length).toBeGreaterThan(1);
    expect(storageSet).not.toHaveBeenCalled();
    expect(consoleLog).not.toHaveBeenCalled();
  });

  it('handles node edit 404, 409 and 422 safely', async () => {
    actionErrors.update = 404;
    await openNode();
    await userEvent.click(screen.getByRole('button', { name: 'Edit node' }));
    let dialog = activeDialog();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await within(dialog).findByText(/HTTP 404: node not found/);

    actionErrors.update = 409;
    await userEvent.clear(within(dialog).getByLabelText('Name'));
    await userEvent.type(within(dialog).getByLabelText('Name'), 'Edge Conflict');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await within(dialog).findByText(/Conflict: node name "Edge Conflict" is already used by an active node/);
    expect(within(dialog).getByLabelText('Name')).toHaveValue('Edge Conflict');

    actionErrors.update = 422;
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    dialog = activeDialog();
    await within(dialog).findByText(/Validation failed: invalid node payload/);
    expect(await within(dialog).findByText('Name is invalid')).toBeInTheDocument();
  });

  it('loads node detail, observability data and renders backend text safely', async () => {
    await openNode();
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/nodes')).toBe(true);
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/nodes/node-1')).toBe(true);

    await userEvent.click(screen.getByRole('tab', { name: 'Runtime / Agent' }));
    expect((await screen.findAllByText('connected')).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/<script>alert\(1\)<\/script>/).length).toBeGreaterThan(0);
    expect(document.querySelector('script')).toBeNull();

    await userEvent.click(screen.getByRole('tab', { name: 'Inventory' }));
    expect(await screen.findByText(/<script>inventory\(\)<\/script>/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole('tab', { name: 'Capabilities' }));
    expect((await screen.findAllByText('xray-core')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole('tab', { name: 'Service discovery' }));
    expect((await screen.findAllByText('xray-live')).length).toBeGreaterThan(0);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
  });

  it('runs maintenance, inventory, capabilities, diagnostics and discovery only after confirmation', async () => {
    await openNode();

    await userEvent.click(screen.getByRole('button', { name: 'Enable maintenance' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/maintenance/enable')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/maintenance/enable')).toBe(true));

    await userEvent.click(screen.getByRole('tab', { name: 'Inventory' }));
    await userEvent.click(screen.getByRole('button', { name: 'Sync inventory' }));
    expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toHaveLength(0);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(true));
    await screen.findByText('job-inventory');

    await userEvent.click(screen.getByRole('tab', { name: 'Capabilities' }));
    await userEvent.type(screen.getByLabelText('Runtime capability'), 'xray-core');
    await userEvent.click(screen.getByRole('button', { name: 'Install runtime' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/capabilities/install')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/capabilities/install')?.body).toMatchObject({ service_code: 'xray-core' });

    await userEvent.click(screen.getByRole('tab', { name: 'Capabilities' }));
    await userEvent.type(screen.getByLabelText('Runtime capability'), 'xray-core');
    await userEvent.click(screen.getAllByRole('button', { name: 'Verify' })[0]);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/capabilities/verify')).toBe(true));

    for (const [label, path] of [
      ['Probe channel', '/api/v1/nodes/node-1/diagnostics/channel-probe'],
      ['Retry inventory', '/api/v1/nodes/node-1/diagnostics/retry-inventory'],
      ['Retry discovery', '/api/v1/nodes/node-1/diagnostics/retry-discovery'],
      ['Reconcile runtime', '/api/v1/nodes/node-1/diagnostics/reconcile-runtime'],
      ['Requeue stuck job', '/api/v1/nodes/node-1/diagnostics/requeue-stuck-job'],
    ] as const) {
      await userEvent.click(screen.getByRole('tab', { name: 'Diagnostics' }));
      await userEvent.click(screen.getByRole('button', { name: label }));
      expect(calls.some((call) => call.method === 'POST' && call.path === path)).toBe(false);
      await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
      await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === path)).toBe(true));
    }

    await userEvent.click(screen.getByRole('tab', { name: 'Service discovery' }));
    await userEvent.click(screen.getByRole('button', { name: 'Discover services' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/services/discover')).toBe(true));

    await userEvent.click(screen.getByRole('tab', { name: 'Service discovery' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Import' })[0]);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/services/discovered/discovery-1/import')).toBe(true));
  });

  it('runs node bootstrap, security and lifecycle workflows safely', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Security' }));
    expect((await screen.findAllByText('enroll...hint')).length).toBeGreaterThan(0);
    await userEvent.clear(screen.getByLabelText('Token TTL hours'));
    await userEvent.type(screen.getByLabelText('Token TTL hours'), '48');
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=48')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=48')).toBe(true));
    expect(screen.queryByText('enroll-secret-token')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(await screen.findByText('enroll-secret-token')).toBeInTheDocument();
    let closeButtons = screen.getAllByRole('button', { name: 'Close' });
    await userEvent.click(closeButtons[closeButtons.length - 1]);
    expect(screen.queryByText('enroll-secret-token')).not.toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Revoke' })[0]);
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/nodes/node-1/enrollment-tokens/token-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/nodes/node-1/enrollment-tokens/token-1')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Scan host key' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/ssh/host-key-scan')).toBe(true));
    expect(await screen.findByText('Scanned host key differs from the currently pinned fingerprint. Verify this out-of-band before pinning.')).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole('button', { name: 'Pin fingerprint' })[0]);
    expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/nodes/node-1/access-methods')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/nodes/node-1/access-methods')).toBe(true));
    const pinCall = calls.find((call) => call.method === 'PUT' && call.path === '/api/v1/nodes/node-1/access-methods');
    expect(pinCall?.body?.items).toMatchObject([{ id: 'access-1', ssh_host_key_sha256: 'SHA256:newfingerprint' }]);

    await userEvent.click(screen.getByRole('button', { name: 'Rotate agent token' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/agent-token/rotate')).toBe(true));

    await userEvent.click(screen.getByRole('tab', { name: 'Bootstrap' }));
    await userEvent.click(screen.getByRole('button', { name: 'Queue bootstrap' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(true));

    await userEvent.click(screen.getByRole('tab', { name: 'Terminal / Access' }));
    await userEvent.click(screen.getByRole('button', { name: 'Launch SSH session' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/ssh/sessions')).toBe(true));
    expect(screen.queryByText(/ssh-session-ticket/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(await screen.findByText(/ssh-session-ticket/)).toBeInTheDocument();
    closeButtons = screen.getAllByRole('button', { name: 'Close' });
    await userEvent.click(closeButtons[closeButtons.length - 1]);
    expect(screen.queryByText(/ssh-session-ticket/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await userEvent.click(screen.getByRole('button', { name: 'Retire node' }));
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/nodes/node-1')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/nodes/node-1')).toBe(true));

    await userEvent.type(screen.getByLabelText('Typed confirmation'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Reason'), 'lost node');
    await userEvent.click(screen.getByRole('button', { name: 'Force retire node' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/force-retire')).toBe(true));
    expect(calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/force-retire')?.body).toMatchObject({ confirmation: 'Edge One', reason: 'lost node' });

    expect(storageSet.mock.calls.some(([key, value]) => String(key).toLowerCase().includes('token') || String(value).includes('enroll-secret-token') || String(value).includes('ssh-session-ticket'))).toBe(false);
    expect(window.localStorage.getItem('enrollment_token')).toBeNull();
    expect(window.sessionStorage.getItem('enrollment_token')).toBeNull();
  });

  it('reveals, copies and downloads manual bootstrap bundles only after explicit acknowledgement', async () => {
    manualBundleRevealContent = 'MEGAVPN_BOOTSTRAP_MODE=manual\nMEGAVPN_NODE=edge-one\n';
    manualBundleDownloadContent = 'MEGAVPN_DOWNLOAD_BUNDLE=manual\n';
    const clipboardWrite = vi.fn().mockResolvedValue(undefined);
    const createObjectURL = vi.fn(() => 'blob:manual-bootstrap-bundle');
    const revokeObjectURL = vi.fn();
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    const consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => undefined);
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboardWrite } });
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Bootstrap' }));
    expect((await screen.findAllByText('Available')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getAllByRole('button', { name: 'Reveal bundle' })[0]);
    let dialog = activeDialog();
    expect(within(dialog).getByText('Edge One')).toBeInTheDocument();
    expect(within(dialog).getByText('bootstra...')).toBeInTheDocument();
    expect(within(dialog).getByText('Action')).toBeInTheDocument();
    expect(within(dialog).queryByText(/MEGAVPN_BOOTSTRAP_MODE/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/secret_ref/i)).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Reveal bundle' })).toBeDisabled();
    expect(calls.some((call) => call.method === 'POST' && call.path.includes('/bundle/reveal'))).toBe(false);

    await userEvent.click(within(dialog).getByLabelText(/I understand this is sensitive one-time bootstrap material/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Reveal bundle' }));
    const revealPath = '/api/v1/nodes/node-1/bootstrap-runs/bootstrap-run-manual/bundle/reveal';
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === revealPath)).toBe(true));
    const revealCall = calls.find((call) => call.method === 'POST' && call.path === revealPath);
    expect(revealCall?.body).toEqual({});
    expect(revealCall?.headers['x-megavpn-csrf']).toBe('1');
    expect(revealCall?.headers.accept).toBe('application/json');
    expect(revealCall?.credentials).toBe('include');
    expect(screen.getByLabelText('Manual bootstrap bundle content')).toHaveValue('MEGAVPN_BOOTSTRAP_MODE=manual\nMEGAVPN_NODE=edge-one\n');
    expect(screen.queryByText(/secret_ref_id/i)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Copy bundle' }));
    await waitFor(() => expect(clipboardWrite).toHaveBeenCalledWith('MEGAVPN_BOOTSTRAP_MODE=manual\nMEGAVPN_NODE=edge-one\n'));
    expect(await screen.findByText('Manual bootstrap bundle copied to clipboard.')).toBeInTheDocument();

    const downloadButtons = screen.getAllByRole('button', { name: 'Download bundle' });
    await userEvent.click(downloadButtons[0]);
    dialog = activeDialog();
    expect(within(dialog).queryByText(/MEGAVPN_BOOTSTRAP_MODE/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/secret_ref/i)).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Download bundle' })).toBeDisabled();
    await userEvent.click(within(dialog).getByLabelText(/I understand this is sensitive one-time bootstrap material/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Download bundle' }));

    const downloadPath = '/api/v1/nodes/node-1/bootstrap-runs/bootstrap-run-manual/bundle/download';
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === downloadPath)).toBe(true));
    const downloadCall = calls.find((call) => call.method === 'POST' && call.path === downloadPath);
    expect(downloadCall?.body).toEqual({});
    expect(downloadCall?.headers['x-megavpn-csrf']).toBe('1');
    expect(downloadCall?.headers.accept).toBe('text/plain, application/octet-stream');
    expect(downloadCall?.credentials).toBe('include');
    expect(downloadCall?.cache).toBe('no-store');
    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(anchorClick).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:manual-bootstrap-bundle');
    expect(await screen.findByText('Manual bootstrap bundle download was started.')).toBeInTheDocument();

    expect(calls.some((call) => call.method === 'GET' && /\/bundle(?:$|\?)/.test(call.path))).toBe(false);
    expect(calls.some((call) => call.path === '/api/v1/secret-refs')).toBe(false);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(JSON.stringify(calls)).not.toContain('MEGAVPN_BOOTSTRAP_MODE');
    expect(window.localStorage.getItem('manual_bundle')).toBeNull();
    expect(window.sessionStorage.getItem('manual_bundle')).toBeNull();
    expect(storageSet.mock.calls.some(([key, value]) => `${key} ${value}`.includes('MEGAVPN_BOOTSTRAP_MODE'))).toBe(false);
    expect(consoleLog).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: 'Close and clear' }));
    expect(screen.queryByLabelText('Manual bootstrap bundle content')).not.toBeInTheDocument();
    expect(screen.queryByText(/MEGAVPN_BOOTSTRAP_MODE/)).not.toBeInTheDocument();
  });

  it('keeps manual bootstrap bundle reveal and download disabled without node.bootstrap permission', async () => {
    await openNode({ ...authPayload, permissions: ['node.read', 'node.write'] });
    await userEvent.click(screen.getByRole('tab', { name: 'Bootstrap' }));

    expect(await screen.findByText('Permission required: node.bootstrap to reveal or download manual bootstrap bundles.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reveal bundle' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download bundle' })).not.toBeInTheDocument();
    expect(calls.some((call) => call.path.includes('/bundle/reveal') || call.path.includes('/bundle/download'))).toBe(false);
  });

  it('clears stale revealed bundle content and refetches runs on manual bundle 404', async () => {
    manualBundleRevealContent = 'MEGAVPN_BOOTSTRAP_MODE=manual\nMEGAVPN_NODE=edge-one\n';

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Bootstrap' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Reveal bundle' })[0]);
    let dialog = activeDialog();
    await userEvent.click(within(dialog).getByLabelText(/I understand this is sensitive one-time bootstrap material/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Reveal bundle' }));
    await screen.findByLabelText('Manual bootstrap bundle content');

    actionErrors.bundleReveal = 404;
    await userEvent.click(screen.getAllByRole('button', { name: 'Reveal bundle' })[0]);
    dialog = activeDialog();
    await userEvent.click(within(dialog).getByLabelText(/I understand this is sensitive one-time bootstrap material/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Reveal bundle' }));

    await screen.findByText('Manual bootstrap bundle is no longer available. The bootstrap runs list was refreshed.');
    expect(screen.queryByLabelText('Manual bootstrap bundle content')).not.toBeInTheDocument();
    expect(calls.filter((call) => call.method === 'GET' && call.path === '/api/v1/nodes/node-1/bootstrap-runs?limit=25').length).toBeGreaterThan(1);
  });

  it('keeps SSH access method creation gated by node.bootstrap permission', async () => {
    await openNode({ ...authPayload, permissions: ['node.read', 'node.write'] });
    await userEvent.click(screen.getByRole('tab', { name: 'Security' }));

    const addButton = await screen.findByRole('button', { name: 'Add SSH access method' });
    expect(addButton).toBeDisabled();
    expect(addButton).toHaveAttribute('title', 'Permission required: node.bootstrap');
    expect(screen.getByText('Permission required: node.bootstrap.')).toBeInTheDocument();
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/access-methods/ssh')).toBe(false);
  });

  it('creates an SSH access method through the dedicated endpoint after explicit fingerprint verification', async () => {
    const privateKey = [
      '-----BEGIN OPENSSH PRIVATE KEY-----',
      'test-private-key-material',
      '-----END OPENSSH PRIVATE KEY-----',
    ].join('\n');
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    const consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => undefined);
    await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Security' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Add SSH access method' }));
    const dialog = activeDialog();
    expect(within(dialog).getByLabelText('SSH host')).toHaveValue('203.0.113.10');
    expect(within(dialog).getByLabelText('SSH port')).toHaveValue(22);
    expect(within(dialog).queryByLabelText('Private key')).not.toBeInTheDocument();

    await userEvent.clear(within(dialog).getByLabelText('SSH host'));
    await userEvent.type(within(dialog).getByLabelText('SSH host'), '198.51.100.50');
    await userEvent.type(within(dialog).getByLabelText('SSH user'), 'deploy');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Scan fingerprints' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/ssh/host-key-scan')).toBe(true));
    const scanCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/ssh/host-key-scan');
    expect(scanCall?.body).toEqual({ ssh_host: '198.51.100.50', ssh_port: 22 });
    expect(scanCall?.body).not.toHaveProperty('ssh_user');
    expect(scanCall?.body).not.toHaveProperty('private_key');

    const radio = (await within(dialog).findAllByRole('radio', { name: 'Select fingerprint SHA256:newfingerprint' }))[0];
    expect(radio).not.toBeChecked();
    expect(within(dialog).getByRole('button', { name: 'Create SSH access method' })).toBeDisabled();
    await userEvent.click(radio);
    expect(within(dialog).queryByLabelText('Private key')).not.toBeInTheDocument();
    await userEvent.click(within(dialog).getByLabelText('I verified this fingerprint through an independent trusted channel.'));
    const privateKeyInput = within(dialog).getByLabelText('Private key');
    fireEvent.change(privateKeyInput, { target: { value: privateKey } });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create SSH access method' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/access-methods/ssh')).toBe(true));
    const createCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/access-methods/ssh');
    expect(createCall?.body).toMatchObject({
      ssh_host: '198.51.100.50',
      ssh_port: 22,
      ssh_user: 'deploy',
      ssh_host_key_sha256: 'SHA256:newfingerprint',
      is_enabled: true,
      private_key_present: true,
    });
    expect(createCall?.body?.request_fields).toEqual(['is_enabled', 'private_key', 'ssh_host', 'ssh_host_key_sha256', 'ssh_port', 'ssh_user']);
    expect(createCall?.body).not.toHaveProperty('private_key');
    expect(createCall?.body).not.toHaveProperty('secret_ref_id');
    expect(createCall?.body).not.toHaveProperty('method');
    expect(createCall?.body).not.toHaveProperty('auth_type');
    expect(createCall?.body).not.toHaveProperty('secret_type');
    expect(calls.some((call) => call.path === '/api/v1/secret-refs')).toBe(false);
    expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/nodes/node-1/access-methods')).toBe(false);

    await screen.findByText('Backend created the SSH access method. Connectivity is not implied until bootstrap or terminal checks run.');
    expect(screen.queryByRole('dialog', { name: 'Add SSH access method' })).not.toBeInTheDocument();
    expect((await screen.findAllByText('198.51.100.50')).length).toBeGreaterThan(0);
    expect(screen.queryByText(privateKey)).not.toBeInTheDocument();
    expect(JSON.stringify(calls)).not.toContain('test-private-key-material');
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(storageSet.mock.calls.some(([key, value]) => String(key).toLowerCase().includes('ssh') || String(value).includes('test-private-key-material'))).toBe(false);
    expect(window.localStorage.getItem('ssh_private_key')).toBeNull();
    expect(window.sessionStorage.getItem('ssh_private_key')).toBeNull();
    expect(consoleLog).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();
  });

  it('blocks SSH access creation on stale scans and clears private key material after backend errors', async () => {
    const privateKey = '-----BEGIN OPENSSH PRIVATE KEY-----\nfailed-key-material\n-----END OPENSSH PRIVATE KEY-----';
    actionErrors.scanHostKey = 204;
    await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Security' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Add SSH access method' }));
    let dialog = activeDialog();
    await userEvent.type(within(dialog).getByLabelText('SSH user'), 'deploy');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Scan fingerprints' }));
    await within(dialog).findByText('The scan returned no host fingerprints. Creation is blocked until a fingerprint is available.');
    expect(within(dialog).queryByLabelText('Private key')).not.toBeInTheDocument();

    actionErrors.scanHostKey = 0;
    await userEvent.click(within(dialog).getByRole('button', { name: 'Scan fingerprints' }));
    const radio = (await within(dialog).findAllByRole('radio', { name: 'Select fingerprint SHA256:newfingerprint' }))[0];
    await userEvent.click(radio);
    await userEvent.click(within(dialog).getByLabelText('I verified this fingerprint through an independent trusted channel.'));
    expect(within(dialog).getByLabelText('Private key')).toBeInTheDocument();
    await userEvent.clear(within(dialog).getByLabelText('SSH port'));
    await userEvent.type(within(dialog).getByLabelText('SSH port'), '2222');
    expect(within(dialog).queryByLabelText('Private key')).not.toBeInTheDocument();

    await userEvent.click(within(dialog).getByRole('button', { name: 'Scan fingerprints' }));
    await userEvent.click((await within(dialog).findAllByRole('radio', { name: 'Select fingerprint SHA256:newfingerprint' }))[0]);
    await userEvent.click(within(dialog).getByLabelText('I verified this fingerprint through an independent trusted channel.'));
    const privateKeyInput = within(dialog).getByLabelText('Private key');
    fireEvent.change(privateKeyInput, { target: { value: privateKey } });
    actionErrors.createSSHAccess = 503;
    await userEvent.click(within(dialog).getByRole('button', { name: 'Create SSH access method' }));
    await within(dialog).findByText(/HTTP 503: secret storage is not configured/);
    dialog = activeDialog();
    expect(within(dialog).getByLabelText('Private key')).toHaveValue('');
    expect(JSON.stringify(calls)).not.toContain('failed-key-material');
  });

  it('shows backend 403, 422 and 409 errors safely', async () => {
    actionErrors.maintenance = 403;
    actionErrors.verify = 422;
    actionErrors.import = 409;
    await openNode();

    await userEvent.click(screen.getByRole('button', { name: 'Enable maintenance' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await screen.findByText(/Permission denied: node.write permission required/);
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await userEvent.click(screen.getByRole('tab', { name: 'Capabilities' }));
    await userEvent.type(screen.getByLabelText('Runtime capability'), 'xray-core');
    await userEvent.click(screen.getAllByRole('button', { name: 'Verify' })[0]);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await screen.findByText(/Validation failed: service_code is required/);
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    await userEvent.click(screen.getByRole('tab', { name: 'Service discovery' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Import' })[0]);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await screen.findByText(/Conflict: service already imported/);
  });

  it('keeps raw API paths and legacy workflow links out of the Nodes page component', () => {
    expect(String(NodesPage)).not.toContain('/api/v1');
    expect(String(NodesPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);
    expect(String(NodesPage)).not.toContain('dangerouslySetInnerHTML');
    expect(String(NodesPage)).not.toContain('/legacy');
  });
});
