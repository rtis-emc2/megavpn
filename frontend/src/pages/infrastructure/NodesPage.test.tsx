import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { EnrollmentToken, NodeBootstrapRun, NodeDiagnostics, NodeInventorySnapshot, NodeStaleRotationPreview } from '../../shared/api/types';
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

const staleRotationPreview: NodeStaleRotationPreview = {
  node_id: 'node-1',
  stale_rotation_detected: true,
  token_rotation_status: 'rotating',
  evaluated_at: '2026-07-09T08:10:00Z',
  candidates: [{
    job_id: 'job-stale-rotation-1',
    status: 'running',
    created_at: '2026-07-09T07:50:00Z',
    started_at: '2026-07-09T07:51:00Z',
    last_claim_at: '2026-07-09T07:52:00Z',
    last_result_at: '2026-07-09T07:53:00Z',
    last_seen_at: '2026-07-09T07:40:00Z',
    last_poll_at: '2026-07-09T07:41:00Z',
    age_seconds: 1200,
    stale_reason: 'unclaimed_without_agent_progress',
    safe_to_clear: true,
  }],
};

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

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
  const queryClient = renderPage(auth);
  expect((await screen.findAllByText('Edge One')).length).toBeGreaterThan(0);
  await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
  await screen.findByRole('heading', { name: 'Edge One' });
  return queryClient;
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
  let currentDiagnostics: NodeDiagnostics;
  let currentInventory: NodeInventorySnapshot | null;
  let currentAccessMethods: typeof accessMethods;
  let currentEnrollmentTokens: EnrollmentToken[];
  let currentBootstrapRuns: NodeBootstrapRun[];
  let currentStaleRotationPreview: NodeStaleRotationPreview;
  let agentRevokeResponseNodeId: string;
  let agentRevokeAlreadyRevoked: boolean;
  let nodeRebootResponseNodeId: string;
  let nodeRebootResponseScopeId: string;
  let nodeRebootResponseType: string;
  let nodeRebootResponseStatus: string;
  let nodeRebootErrorCode: string;
  let delayNodeRebootResponse: boolean;
  let resolveNodeRebootResponse: (() => void) | null;
  let manualBundleRevealContent: string;
  let manualBundleDownloadContent: string;
  let emptyEnrollmentIssueResponse: boolean;
  let delayEnrollmentCreateResponse: boolean;
  let resolveEnrollmentCreateResponse: (() => void) | null;

  function setInventorySyncEligibleState(overrides: Partial<Omit<NodeDiagnostics, 'agent'>> & { agent?: Partial<NonNullable<NodeDiagnostics['agent']>> } = {}) {
    const baseAgent = {
      node_id: 'node-1',
      status: 'active',
      agent_version: '8.0.0-agent',
      protocol_version: 'v1',
      registered_at: '2026-07-09T08:00:00Z',
      last_seen_at: '2026-07-09T08:00:30Z',
      token_rotation_status: 'active',
    };
    const { agent: agentOverrides, ...diagnosticOverrides } = overrides;
    currentNode = {
      ...currentNode,
      agent_status: 'online',
      agent_last_seen_at: '2026-07-09T08:00:30Z',
      last_heartbeat_at: '2026-07-09T08:00:30Z',
    };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'online',
      heartbeat_drift_seconds: 4,
      communication_state: 'healthy',
      latest_inventory: undefined,
      ...diagnosticOverrides,
      agent: {
        ...baseAgent,
        ...(agentOverrides || {}),
      },
    };
    currentInventory = null;
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
  }

  beforeEach(async () => {
    calls.length = 0;
    actionErrors = {};
    currentNode = { ...node };
    currentDiagnostics = clone(diagnostics);
    currentInventory = clone(inventory);
    createdNode = null;
    nodeList = [currentNode];
    currentAccessMethods = accessMethods.map((method) => ({ ...method }));
    currentEnrollmentTokens = enrollmentTokens.map((token) => ({ ...token }));
    currentBootstrapRuns = bootstrapRuns.map((run) => ({ ...run }));
    currentStaleRotationPreview = clone(staleRotationPreview);
    agentRevokeResponseNodeId = 'node-1';
    agentRevokeAlreadyRevoked = false;
    nodeRebootResponseNodeId = 'node-1';
    nodeRebootResponseScopeId = 'node-1';
    nodeRebootResponseType = 'node.reboot';
    nodeRebootResponseStatus = 'queued';
    nodeRebootErrorCode = 'node_reboot_agent_unavailable';
    delayNodeRebootResponse = false;
    resolveNodeRebootResponse = null;
    manualBundleRevealContent = '';
    manualBundleDownloadContent = '';
    emptyEnrollmentIssueResponse = false;
    delayEnrollmentCreateResponse = false;
    resolveEnrollmentCreateResponse = null;
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
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/diagnostics') return json(currentDiagnostics);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/diagnostics/stale-rotation') return json(currentStaleRotationPreview);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/inventory') {
        if (!currentInventory) return json({ error: 'inventory not found' }, 404);
        return json(currentInventory);
      }
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
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/enrollment-tokens') return json(currentEnrollmentTokens);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/access-methods') return json(currentAccessMethods);
      if (method === 'GET' && url.pathname === '/api/v1/nodes/node-1/bootstrap-runs') return json(currentBootstrapRuns);
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
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/inventory/sync') {
        const status = actionErrors.inventorySync;
        if (status) {
          const messages: Record<number, string> = {
            400: 'invalid inventory sync request',
            403: 'node.write permission required',
            404: 'node not found',
            409: 'active inventory job already exists',
            429: 'inventory sync rate limited',
            500: 'inventory sync queue failed',
            503: 'inventory sync service unavailable',
          };
          return json({ error: messages[status] || 'inventory sync failed' }, status);
        }
        return json(job('job-inventory', 'node.inventory.sync'), 202);
      }
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
        if (delayEnrollmentCreateResponse) {
          await new Promise<void>((resolve) => {
            resolveEnrollmentCreateResponse = resolve;
          });
        }
        if (actionErrors.enrollmentCreate) {
          const messages: Record<number, string> = {
            400: 'invalid ttl_hours',
            403: 'node.bootstrap permission required',
            404: 'node not found',
            409: 'node lifecycle conflict',
            429: 'rate limited',
            500: 'enrollment token create failed',
            503: 'enrollment token service unavailable',
          };
          return json({ error: messages[actionErrors.enrollmentCreate] || 'enrollment token create failed' }, actionErrors.enrollmentCreate);
        }
        if (emptyEnrollmentIssueResponse) return json({ id: 'token-empty', node_id: 'node-1', token_hint: 'empty...token', status: 'active', expires_at: '2026-07-10T08:05:00Z', created_at: '2026-07-09T08:05:00Z' }, 201);
        currentEnrollmentTokens = [{ id: 'token-new', node_id: 'node-1', token_hint: 'enroll...token', status: 'active', expires_at: '2026-08-10T08:05:00Z', created_at: '2026-07-09T08:05:00Z' }, ...currentEnrollmentTokens.map((token) => ({ ...token, status: token.status === 'active' ? 'revoked' : token.status }))];
        return json({ id: 'token-new', node_id: 'node-1', token: 'enroll-secret-token', token_hint: 'enroll...token', status: 'active', expires_at: '2026-08-10T08:05:00Z', created_at: '2026-07-09T08:05:00Z' }, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/enrollment-token/rotate') {
        if (actionErrors.enrollmentRotate) {
          const messages: Record<number, string> = {
            400: 'invalid ttl_hours',
            403: 'node.bootstrap permission required',
            404: 'node not found',
            409: 'credential conflict',
            429: 'rate limited',
            500: 'enrollment token rotate failed',
            503: 'enrollment token service unavailable',
          };
          return json({ error: messages[actionErrors.enrollmentRotate] || 'enrollment token rotate failed' }, actionErrors.enrollmentRotate);
        }
        if (emptyEnrollmentIssueResponse) return json({ id: 'token-empty-rotate', node_id: 'node-1', token_hint: 'empty...rotate', status: 'active', expires_at: '2026-07-10T08:06:00Z', created_at: '2026-07-09T08:06:00Z' });
        currentEnrollmentTokens = [{ id: 'token-rotated', node_id: 'node-1', token_hint: 'rotate...token', status: 'active', expires_at: '2026-08-10T08:06:00Z', created_at: '2026-07-09T08:06:00Z' }, ...currentEnrollmentTokens.map((token) => ({ ...token, status: token.status === 'active' ? 'revoked' : token.status }))];
        return json({ id: 'token-rotated', node_id: 'node-1', token: 'enroll-rotated-token', token_hint: 'rotate...token', status: 'active', expires_at: '2026-08-10T08:06:00Z', created_at: '2026-07-09T08:06:00Z' });
      }
      if (method === 'DELETE' && url.pathname.startsWith('/api/v1/nodes/node-1/enrollment-tokens/')) {
        const tokenId = url.pathname.split('/').pop() || '';
        currentEnrollmentTokens = currentEnrollmentTokens.map((token) => token.id === tokenId ? { ...token, status: 'revoked' } : token);
        return json(currentEnrollmentTokens.find((token) => token.id === tokenId) || { id: tokenId, node_id: 'node-1', status: 'revoked' });
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
        const status = actionErrors.bootstrap;
        if (status) {
          const messages: Record<number, string> = {
            400: 'invalid bootstrap mode',
            403: 'node.bootstrap permission required',
            404: 'node not found',
            409: 'node firewall is enforced and does not include an active input accept rule for SSH bootstrap port(s) 22',
            422: 'payload.bootstrap_mode must be ssh_bootstrap or manual_bundle',
            429: 'bootstrap rate limited',
            500: 'bootstrap worker queue failed',
            503: 'bootstrap service unavailable',
          };
          return json({ error: messages[status] || 'bootstrap failed' }, status);
        }
        const run = {
          id: 'bootstrap-run-new',
          node_id: 'node-1',
          job_id: 'job-bootstrap',
          status: 'queued',
          bootstrap_mode: String(body?.bootstrap_mode || 'ssh_bootstrap'),
          created_at: '2026-07-09T08:07:00Z',
        };
        currentBootstrapRuns = [run as NodeBootstrapRun, ...currentBootstrapRuns];
        return json({ job: job('job-bootstrap', 'node.bootstrap'), bootstrap_run: run }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/agent-identity/revoke') {
        const status = actionErrors.agentRevoke;
        if (status) {
          const messages: Record<number, { code: string; error: string }> = {
            400: { code: 'node_agent_revoke_request_invalid', error: 'invalid request contains token_hash raw text' },
            403: { code: 'node_agent_revoke_request_invalid', error: 'node.bootstrap permission required Authorization: Bearer raw' },
            404: { code: 'node_agent_revoke_node_not_found', error: 'node not found secret_ref raw' },
            409: { code: 'node_agent_revoke_confirmation_mismatch', error: 'node confirmation mismatch token_hash raw' },
            429: { code: 'rate_limited', error: 'rate limited token raw' },
            500: { code: 'node_agent_revoke_internal_error', error: 'store failed token_hash raw' },
            503: { code: 'node_agent_revoke_internal_error', error: 'service unavailable secret_ref raw' },
          };
          const payload = messages[status] || { code: 'node_agent_revoke_internal_error', error: 'agent revoke failed' };
          return json({ status: 'error', ...payload }, status);
        }
        currentNode = { ...currentNode, agent_status: 'revoked', updated_at: '2026-07-09T08:12:00Z' };
        currentDiagnostics = {
          ...currentDiagnostics,
          agent: {
            ...(currentDiagnostics.agent || {}),
            node_id: 'node-1',
            status: 'revoked',
          },
        };
        currentEnrollmentTokens = currentEnrollmentTokens.map((token) => token.status === 'active' ? { ...token, status: 'revoked' } : token);
        return json({
          status: 'revoked',
          node_id: agentRevokeResponseNodeId,
          agent_status: 'revoked',
          revoked_at: '2026-07-09T08:12:00Z',
          already_revoked: agentRevokeAlreadyRevoked,
          revoked_enrollment_tokens: agentRevokeAlreadyRevoked ? 0 : 1,
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/reboot') {
        if (delayNodeRebootResponse) {
          await new Promise<void>((resolve) => {
            resolveNodeRebootResponse = resolve;
          });
        }
        const status = actionErrors.nodeReboot;
        if (status) {
          const messages: Record<number, { code: string; error: string }> = {
            400: { code: 'node_reboot_request_invalid', error: 'invalid reboot request contains command output' },
            403: { code: 'node_reboot_request_invalid', error: 'node.bootstrap permission required Authorization: Bearer raw' },
            404: { code: 'node_reboot_node_not_found', error: 'node not found secret_ref raw' },
            409: { code: nodeRebootErrorCode, error: 'node reboot conflict token_hash raw command output' },
            429: { code: 'rate_limited', error: 'rate limited token raw' },
            500: { code: 'node_reboot_internal_error', error: 'store failed token_hash raw' },
            503: { code: 'node_reboot_internal_error', error: 'service unavailable secret_ref raw' },
          };
          const payload = messages[status] || { code: 'node_reboot_internal_error', error: 'node reboot failed' };
          return json({ status: 'error', ...payload }, status);
        }
        return json({
          id: 'job-reboot-1',
          type: nodeRebootResponseType,
          status: nodeRebootResponseStatus,
          scope_type: 'node',
          scope_id: nodeRebootResponseScopeId,
          node_id: nodeRebootResponseNodeId,
          created_at: '2026-07-09T08:13:00Z',
          payload: { command: 'raw-command-output-not-rendered' },
          result: { output: 'raw-command-output-not-rendered' },
        }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/agent-token/rotate') return json(job('job-agent-rotate', 'node.agent.rotate_token'), 202);
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/ssh/sessions') {
        return json({ session_id: 'ssh-session-ticket', node_id: 'node-1', expires_at: '2026-08-10T08:06:30Z', endpoint: { server_side_proxy_only: true } }, 201);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/nodes/node-1') return json({ ...node, status: 'retired' });
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/force-retire') return json({ status: 'retired', node: { ...node, status: 'retired' } });
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    vi.useRealTimers();
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
    const queryClient = await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Security' }));
    expect((await screen.findAllByText('enroll...hint')).length).toBeGreaterThan(0);
    await userEvent.clear(screen.getByLabelText('Token TTL hours'));
    await userEvent.type(screen.getByLabelText('Token TTL hours'), '48');
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=48')).toBe(false);
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await userEvent.click(screen.getByLabelText(/I understand that this enrollment token is sensitive/));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=48')).toBe(true));
    expect(screen.queryByText('enroll-secret-token')).not.toBeInTheDocument();
    expect(document.body.textContent || '').not.toContain('enroll-secret-token');
    const createMutation = queryClient.getMutationCache().getAll().find((mutation) => JSON.stringify(mutation.state.variables || {}).includes('node-1') && mutation.options.gcTime === 0);
    expect(createMutation?.state.data).toBeUndefined();
    expect(JSON.stringify(createMutation?.state.variables || {})).not.toContain('enroll-secret-token');
    expect(JSON.stringify(queryClient.getQueryCache().getAll().map((query) => query.state.data))).not.toContain('enroll-secret-token');
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(await screen.findByText('enroll-secret-token')).toBeInTheDocument();
    let closeButtons = screen.getAllByRole('button', { name: 'Close' });
    await userEvent.click(closeButtons[closeButtons.length - 1]);
    expect(screen.queryByText('enroll-secret-token')).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Rotate token' }));
    expect(screen.getByText('Existing active enrollment credentials may be revoked or replaced according to backend behavior.')).toBeInTheDocument();
    expect(screen.getByText('Reissue does not automatically update a remote machine, bootstrap file or running agent.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await userEvent.click(screen.getByLabelText(/previous enrollment credential may stop working/));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token/rotate?ttl_hours=48')).toBe(true));
    expect(screen.queryByText('enroll-rotated-token')).not.toBeInTheDocument();
    expect(JSON.stringify(queryClient.getQueryCache().getAll().map((query) => query.state.data))).not.toContain('enroll-rotated-token');
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(await screen.findByText('enroll-rotated-token')).toBeInTheDocument();
    closeButtons = screen.getAllByRole('button', { name: 'Close' });
    await userEvent.click(closeButtons[closeButtons.length - 1]);
    expect(screen.queryByText('enroll-rotated-token')).not.toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Revoke' })[0]);
    expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/nodes/node-1/enrollment-tokens/token-rotated')).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/nodes/node-1/enrollment-tokens/token-rotated')).toBe(true));

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

    expect(storageSet.mock.calls.some(([key, value]) => String(key).toLowerCase().includes('token') || String(value).includes('enroll-secret-token') || String(value).includes('enroll-rotated-token') || String(value).includes('ssh-session-ticket'))).toBe(false);
    expect(window.localStorage.getItem('enrollment_token')).toBeNull();
    expect(window.sessionStorage.getItem('enrollment_token')).toBeNull();
  });

  it('loads stale rotation preview only on the Lifecycle tab without destructive lifecycle wrappers', async () => {
    await openNode();

    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation')).toBe(false);
    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation')).toBe(true));

    const previewCall = calls.find((call) => call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation');
    expect(previewCall).toMatchObject({ method: 'GET', credentials: 'include' });
    expect(previewCall?.headers['x-megavpn-csrf']).toBeUndefined();
    await screen.findAllByText('Unclaimed rotation without agent progress.');
    expect(screen.getAllByText('job-stal...').length).toBeGreaterThan(0);
    expect(document.body.textContent || '').not.toContain('tok_...');

    await userEvent.click(screen.getByRole('button', { name: 'Refresh preview' }));
    await waitFor(() => expect(calls.filter((call) => call.method === 'GET' && call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation')).toHaveLength(2));
    for (const forbiddenPath of [
      '/api/v1/nodes/node-1/agent-identity/revoke',
      '/api/v1/nodes/node-1/reboot',
      '/api/v1/nodes/node-1/emergency-cleanup',
      '/api/v1/nodes/node-1/diagnostics/clear-stale-rotation',
    ]) {
      expect(calls.some((call) => call.method === 'POST' && call.path === forbiddenPath)).toBe(false);
    }
    expect(screen.getByRole('button', { name: /queue reboot/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /revoke agent identity/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /emergency cleanup/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /clear stale rotation/i })).not.toBeInTheDocument();
  });

  it('does not fetch stale rotation preview without node.read permission', async () => {
    await openNode({ ...authPayload, permissions: ['node.write', 'node.bootstrap'] });

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    expect(screen.getByRole('alert')).toHaveTextContent('Permission required: node.read');
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation')).toBe(false);
  });

  it('does not render stale rotation preview data for a different selected node', async () => {
    currentStaleRotationPreview = { ...currentStaleRotationPreview, node_id: 'node-previous' };
    await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await waitFor(() => expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation')).toBe(true));
    expect(screen.queryByText('Unclaimed rotation without agent progress.')).not.toBeInTheDocument();
    expect(screen.queryByText('job-stal...')).not.toBeInTheDocument();
    expect(screen.queryByText('node-previous')).not.toBeInTheDocument();
  });

  it('revokes agent identity through the dedicated dialog without legacy or adjacent destructive calls', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const queryClient = await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await waitFor(() => expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/diagnostics/stale-rotation')).toBe(true));
    await userEvent.click(screen.getByRole('button', { name: 'Revoke agent identity' }));

    const dialog = activeDialog();
    expect(within(dialog).getByText('This is a destructive trust operation. Backend validation is authoritative and the current agent credentials stop authenticating after success.')).toBeInTheDocument();
    expect(within(dialog).getByText('Uninstall the agent.')).toBeInTheDocument();
    expect(within(dialog).getByText('Automatically create a replacement enrollment token.')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Revoke identity' })).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(dialog).getByLabelText('Operator reason'), 'incident response');
    await userEvent.click(within(dialog).getByLabelText(/I understand that identity revocation/));
    await userEvent.dblClick(within(dialog).getByRole('button', { name: 'Revoke identity' }));

    await waitFor(() => expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/agent-identity/revoke')).toHaveLength(1));
    const revokeCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/agent-identity/revoke');
    expect(revokeCall?.body).toEqual({ confirmation: 'Edge One', reason: 'incident response' });
    expect(Object.keys(revokeCall?.body || {}).sort()).toEqual(['confirmation', 'reason']);
    expect(revokeCall?.body).not.toHaveProperty('acknowledged');
    expect(revokeCall?.body).not.toHaveProperty('node_id');
    expect(revokeCall?.headers['x-megavpn-csrf']).toBe('1');

    expect(await screen.findByText(/Agent identity revoked. The previous agent credentials can no longer authenticate./)).toBeInTheDocument();
    expect(screen.getByText(/Active enrollment credentials revoked: 1./)).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Lifecycle' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('dialog', { name: 'Revoke agent identity' })).not.toBeInTheDocument();
    expect(JSON.stringify(queryClient.getMutationCache().getAll().map((mutation) => mutation.state.variables))).not.toContain('incident response');
    expect(storageSet.mock.calls.some(([key, value]) => /revoke|agent|token|confirmation|reason/i.test(String(key)) || /incident response|Edge One/.test(String(value)))).toBe(false);
    expect(document.body.textContent || '').not.toContain('tok_...');
    expect(document.body.textContent || '').not.toContain('token_hash');
    expect(document.body.textContent || '').not.toContain('secret_ref');
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/reboot')).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/emergency-cleanup')).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/diagnostics/clear-stale-rotation')).toBe(false);
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path.includes('/enrollment-token'))).toBe(false);
  });

  it('handles already-revoked result as informational', async () => {
    agentRevokeAlreadyRevoked = true;
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Revoke agent identity' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'second review');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that identity revocation/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Revoke identity' }));
    expect(await screen.findByText(/Agent identity was already revoked./)).toBeInTheDocument();
    expect(screen.queryByText(/Active enrollment credentials revoked:/)).not.toBeInTheDocument();
  });

  it('handles safe backend errors and node-id mismatch without fake success', async () => {
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    actionErrors.agentRevoke = 409;
    await userEvent.click(screen.getByRole('button', { name: 'Revoke agent identity' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'incident response');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that identity revocation/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Revoke identity' }));
    expect(await screen.findByText('Backend rejected the confirmation. Type the current node name again.')).toBeInTheDocument();
    expect(document.body.textContent || '').not.toContain('token_hash raw');
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Cancel' }));

    actionErrors.agentRevoke = 0;
    agentRevokeAlreadyRevoked = false;
    agentRevokeResponseNodeId = 'node-other';
    await userEvent.click(screen.getByRole('button', { name: 'Revoke agent identity' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'incident response');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that identity revocation/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Revoke identity' }));
    expect(await screen.findByText('Backend returned a result for a different node. Current UI state was not marked successful; refresh node data.')).toBeInTheDocument();
    expect(within(activeDialog()).getByRole('heading', { name: 'Revoke agent identity' })).toBeInTheDocument();
  });

  it('keeps agent identity revoke disabled without node.bootstrap permission', async () => {
    await openNode({ ...authPayload, permissions: ['node.read', 'node.write'] });

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    expect(await screen.findByText('Permission required: node.bootstrap')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke agent identity' })).not.toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/agent-identity/revoke')).toBe(false);
  });

  it('queues node reboot through the dedicated dialog with queued-only success semantics', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const queryClient = await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Queue reboot' }));

    const dialog = activeDialog();
    expect(within(dialog).getByRole('heading', { name: 'Reboot node' })).toBeInTheDocument();
    expect(within(dialog).getByText('This confirms queueing only. It does not confirm that the node rebooted or returned online.')).toBeInTheDocument();
    expect(within(dialog).getByText('May interrupt VPN and proxy traffic handled by this node.')).toBeInTheDocument();
    expect(within(dialog).getByText('Revoke the agent identity.')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Queue reboot' })).toBeDisabled();

    await userEvent.type(within(dialog).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(dialog).getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(within(dialog).getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.dblClick(within(dialog).getByRole('button', { name: 'Queue reboot' }));

    await waitFor(() => expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/reboot')).toHaveLength(1));
    const rebootCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/reboot');
    expect(rebootCall?.body).toEqual({ confirmation: 'Edge One', reason: 'maintenance window' });
    expect(Object.keys(rebootCall?.body || {}).sort()).toEqual(['confirmation', 'reason']);
    expect(rebootCall?.body).not.toHaveProperty('acknowledged');
    expect(rebootCall?.body).not.toHaveProperty('node_id');
    expect(rebootCall?.body).not.toHaveProperty('maintenance');
    expect(rebootCall?.headers['x-megavpn-csrf']).toBe('1');

    expect((await screen.findAllByText('Node reboot job queued.')).length).toBeGreaterThan(0);
    expect(screen.getByText('This confirms queueing only. It does not confirm that the node rebooted or returned online.')).toBeInTheDocument();
    expect(screen.getByText('job-reboot-1')).toBeInTheDocument();
    expect(screen.getByText('node.reboot')).toBeInTheDocument();
    expect(screen.getAllByText('queued').length).toBeGreaterThan(0);
    expect(screen.getByRole('tab', { name: 'Lifecycle' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('dialog', { name: 'Reboot node' })).not.toBeInTheDocument();
    expect(document.body.textContent || '').not.toMatch(/reboot successful|node is restarting|node recovered|services restored/i);
    expect(document.body.textContent || '').not.toContain('raw-command-output-not-rendered');
    expect(storageSet.mock.calls.some(([key, value]) => /reboot|reason|confirmation|job|token|agent/i.test(String(key)) || /maintenance window|Edge One|raw-command-output/.test(String(value)))).toBe(false);
    await waitFor(() => expect(JSON.stringify(queryClient.getMutationCache().getAll().map((mutation) => mutation.state.variables))).not.toContain('maintenance window'));

    await userEvent.click(screen.getByRole('button', { name: 'Open Jobs' }));
    expect(screen.getByRole('tab', { name: 'Jobs / Activity' })).toHaveAttribute('aria-selected', 'true');
    expect(document.body.textContent || '').not.toContain('raw-command-output-not-rendered');
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/maintenance/enable')).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/emergency-cleanup')).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/diagnostics/clear-stale-rotation')).toBe(false);
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
  });

  it('does not treat unexpected reboot job type, node ownership or status as success', async () => {
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));

    nodeRebootResponseType = 'node.inventory.sync';
    await userEvent.click(await screen.findByRole('button', { name: 'Queue reboot' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Queue reboot' }));
    expect(await screen.findByText('Backend returned an unexpected reboot job response. Current UI state was not marked successful; refresh node data.')).toBeInTheDocument();
    expect(within(activeDialog()).getByRole('heading', { name: 'Reboot node' })).toBeInTheDocument();
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Cancel' }));

    nodeRebootResponseType = 'node.reboot';
    nodeRebootResponseNodeId = 'node-other';
    await userEvent.click(screen.getByRole('button', { name: 'Queue reboot' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Queue reboot' }));
    expect(await screen.findByText('Backend returned an unexpected reboot job response. Current UI state was not marked successful; refresh node data.')).toBeInTheDocument();
    expect(within(activeDialog()).getByRole('heading', { name: 'Reboot node' })).toBeInTheDocument();
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Cancel' }));

    nodeRebootResponseNodeId = 'node-1';
    nodeRebootResponseScopeId = 'node-other';
    await userEvent.click(screen.getByRole('button', { name: 'Queue reboot' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Queue reboot' }));
    expect(await screen.findByText('Backend returned an unexpected reboot job response. Current UI state was not marked successful; refresh node data.')).toBeInTheDocument();
    expect(within(activeDialog()).getByRole('heading', { name: 'Reboot node' })).toBeInTheDocument();
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Cancel' }));

    nodeRebootResponseScopeId = 'node-1';
    nodeRebootResponseStatus = 'running';
    await userEvent.click(screen.getByRole('button', { name: 'Queue reboot' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Queue reboot' }));
    expect(await screen.findByText('Backend returned an unexpected reboot job response. Current UI state was not marked successful; refresh node data.')).toBeInTheDocument();
    expect(screen.queryByText('Node reboot job queued.')).not.toBeInTheDocument();
  });

  it('maps node reboot backend errors safely and keeps the dialog open for correction', async () => {
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));

    const cases = [
      { status: 400, text: 'Invalid reboot request. Review confirmation and reason.' },
      { status: 403, text: 'node.bootstrap permission is required to queue node reboot.' },
      { status: 404, text: 'Node no longer exists. Refresh node state.' },
      { status: 409, code: 'node_reboot_agent_missing', text: 'Backend reports that the active agent identity is missing.' },
      { status: 409, code: 'node_reboot_agent_unavailable', text: 'Backend reports that the agent channel is unavailable for reboot queueing.' },
      { status: 409, code: 'node_reboot_conflict', text: 'Another active node operation prevents reboot queueing. Review Jobs and retry explicitly after the conflict is resolved.' },
      { status: 500, text: 'Node reboot queueing is unavailable or failed server-side. No successful queueing may be assumed.' },
    ];

    for (const item of cases) {
      actionErrors.nodeReboot = item.status;
      nodeRebootErrorCode = item.code || 'node_reboot_agent_unavailable';
      await userEvent.click(screen.getByRole('button', { name: 'Queue reboot' }));
      await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
      await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'maintenance window');
      await userEvent.click(within(activeDialog()).getByLabelText(/I understand that this queues a reboot job only/));
      await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Queue reboot' }));
      expect(await within(activeDialog()).findByText(item.text)).toBeInTheDocument();
      expect(activeDialog()).not.toHaveTextContent('token_hash raw');
      expect(activeDialog()).not.toHaveTextContent('Authorization: Bearer');
      expect(screen.queryByText('Node reboot job queued.')).not.toBeInTheDocument();
      await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Cancel' }));
    }
  });

  it('keeps node reboot disabled without node.bootstrap permission', async () => {
    await openNode({ ...authPayload, permissions: ['node.read', 'node.write'] });

    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    expect(await screen.findByText('node.bootstrap permission is required to queue a node reboot.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue reboot' })).not.toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/reboot')).toBe(false);
  });

  it('ignores a stale node reboot response after the dialog is cancelled', async () => {
    delayNodeRebootResponse = true;
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Lifecycle' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Queue reboot' }));
    await userEvent.type(within(activeDialog()).getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(within(activeDialog()).getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(within(activeDialog()).getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Queue reboot' }));
    await waitFor(() => expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/reboot')).toHaveLength(1));
    await userEvent.click(within(activeDialog()).getByRole('button', { name: 'Cancel' }));
    resolveNodeRebootResponse?.();
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Reboot node' })).not.toBeInTheDocument());
    expect(screen.queryByText('Node reboot job queued.')).not.toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Lifecycle' })).toHaveAttribute('aria-selected', 'true');
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

  it('renders read-only agent onboarding status with safe evidence only', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    const consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => undefined);
    const indexedOpen = vi.fn();
    vi.stubGlobal('indexedDB', { open: indexedOpen });
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'online',
      communication_state: 'inventory_ok',
      agent: {
        ...currentDiagnostics.agent,
        status: 'active',
        agent_version: '8.0.0-agent',
        protocol_version: 'v1',
        registered_at: '2026-07-09T08:00:00Z',
        last_seen_at: '2026-07-09T08:00:30Z',
        last_inventory_sync_at: '2026-07-09T08:01:00Z',
        token_rotation_status: 'active',
        token_hint: 'agent...hint',
      },
      active_enrollment_token: {
        ...currentEnrollmentTokens[0],
        token: 'opaque-value-a',
        enrollment_token: 'opaque-value-b',
        secret_ref_id: 'opaque-value-c',
      } as unknown as EnrollmentToken,
      last_bootstrap: {
        ...currentBootstrapRuns[1],
        result_payload: {
          agent_bootstrapenv: 'opaque-value-d',
          secret_ref_id: 'opaque-value-e',
        },
      },
      latest_inventory: currentInventory || undefined,
    };
    currentEnrollmentTokens = [{
      ...currentEnrollmentTokens[0],
      token: 'opaque-value-f',
      enrollment_token: 'opaque-value-g',
      secret_ref_id: 'opaque-value-h',
    } as unknown as EnrollmentToken];

    await openNode();
    expect(screen.getByRole('tab', { name: 'Onboarding' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Runtime / Agent' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Bootstrap' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Security' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Diagnostics' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(await screen.findByText('Agent onboarding')).toBeInTheDocument();
    expect(screen.getAllByText('Ready').length).toBeGreaterThan(0);
    const steps = screen.getByRole('list', { name: 'Agent onboarding' });
    expect(within(steps).getAllByRole('listitem')).toHaveLength(6);
    expect(within(steps).getByText('Control-plane node')).toBeInTheDocument();
    expect(within(steps).getByText('Enrollment credential')).toBeInTheDocument();
    expect(within(steps).getByText('Bootstrap preparation')).toBeInTheDocument();
    expect(within(steps).getByText('Agent registration')).toBeInTheDocument();
    expect(within(steps).getByText('Heartbeat')).toBeInTheDocument();
    expect(within(steps).getByText('Inventory')).toBeInTheDocument();
    expect(screen.getAllByText('Edge One').length).toBeGreaterThan(0);
    expect(screen.getAllByText('agent_managed').length).toBeGreaterThan(0);
    expect(screen.getAllByText('agent...hint').length).toBeGreaterThan(0);
    expect(screen.getAllByText('inventory_ok').length).toBeGreaterThan(0);
    expect(screen.getByText('Live external-node validation is not proven.')).toBeInTheDocument();

    const pageText = document.body.textContent || '';
    expect(pageText).not.toContain('opaque-value-a');
    expect(pageText).not.toContain('opaque-value-b');
    expect(pageText).not.toContain('opaque-value-c');
    expect(pageText).not.toContain('opaque-value-d');
    expect(pageText).not.toContain('opaque-value-e');
    expect(pageText).not.toContain('opaque-value-f');
    expect(pageText).not.toContain('opaque-value-g');
    expect(pageText).not.toContain('opaque-value-h');
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(storageSet).not.toHaveBeenCalled();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    expect(indexedOpen).not.toHaveBeenCalled();
    expect(consoleLog).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();
    expect(String(NodesPage)).not.toContain('dangerouslySetInnerHTML');
  });

  it('uses onboarding navigation buttons without executing onboarding mutations or agent routes', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect((await screen.findAllByText('Not started')).length).toBeGreaterThan(0);
    expect(await screen.findByText(/Inventory is unavailable/)).toBeInTheDocument();

    await userEvent.click(screen.getAllByRole('button', { name: 'Open Security' })[0]);
    expect(screen.getByRole('tab', { name: 'Security' })).toHaveAttribute('aria-selected', 'true');
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Open Bootstrap' })[0]);
    expect(screen.getByRole('tab', { name: 'Bootstrap' })).toHaveAttribute('aria-selected', 'true');
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Open Runtime' })[0]);
    expect(screen.getByRole('tab', { name: 'Runtime / Agent' })).toHaveAttribute('aria-selected', 'true');
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(screen.getAllByRole('button', { name: 'Open Inventory' })[0]);
    expect(screen.getByRole('tab', { name: 'Inventory' })).toHaveAttribute('aria-selected', 'true');

    currentDiagnostics = {
      ...currentDiagnostics,
      communication_state: 'auth_failure',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
        last_auth_failure_at: '2026-07-09T08:10:00Z',
      },
    };
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(screen.getByRole('button', { name: 'Refresh status' }));
    await screen.findByText('Status refreshed.');
    await userEvent.click(screen.getAllByRole('button', { name: 'Open Diagnostics' })[0]);
    expect(screen.getByRole('tab', { name: 'Diagnostics' })).toHaveAttribute('aria-selected', 'true');

    const forbiddenMutations = [
      '/api/v1/nodes/node-1/enrollment-token',
      '/api/v1/nodes/node-1/enrollment-token/rotate',
      '/api/v1/nodes/node-1/bootstrap',
      '/api/v1/nodes/node-1/inventory/sync',
    ];
    expect(calls.filter((call) => call.method !== 'GET' && forbiddenMutations.includes(call.path))).toHaveLength(0);
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
  });

  it('queues guided inventory synchronization only after acknowledgement and keeps onboarding active', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const consoleLog = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    const consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => undefined);
    setInventorySyncEligibleState();

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    const syncButton = await screen.findByRole('button', { name: 'Synchronize inventory' });
    expect(screen.getByText('Agent registration observed')).toBeInTheDocument();
    expect(screen.getByText('First heartbeat observed')).toBeInTheDocument();
    expect(screen.getByText('Inventory synchronization queues an asynchronous agent job.')).toBeInTheDocument();

    const syncPath = '/api/v1/nodes/node-1/inventory/sync';
    expect(calls.filter((call) => call.method === 'POST' && call.path === syncPath)).toHaveLength(0);
    await userEvent.click(syncButton);
    expect(calls.filter((call) => call.method === 'POST' && call.path === syncPath)).toHaveLength(0);

    const dialog = activeDialog();
    expect(within(dialog).getByText('Edge One')).toBeInTheDocument();
    expect(within(dialog).getByText('node-1')).toBeInTheDocument();
    expect(within(dialog).getByText('healthy')).toBeInTheDocument();
    expect(within(dialog).getByText('Job acceptance does not prove inventory synchronization.')).toBeInTheDocument();
    expect(within(dialog).queryByText(/secret_ref_id/i)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/request_payload/i)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/result_payload/i)).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Confirm' })).toBeDisabled();

    await userEvent.click(within(dialog).getByLabelText(/queues an asynchronous inventory job/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(calls.filter((call) => call.method === 'POST' && call.path === syncPath)).toHaveLength(1));
    expect(screen.getByRole('tab', { name: 'Onboarding' })).toHaveAttribute('aria-selected', 'true');
    expect((await screen.findAllByText('Inventory job accepted')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('Accepted').length).toBeGreaterThan(0);
    expect(screen.queryByText('Inventory synchronized.')).not.toBeInTheDocument();

    const postCount = calls.filter((call) => call.method === 'POST' && call.path === syncPath).length;
    await userEvent.click(screen.getAllByRole('button', { name: 'Open Jobs' })[0]);
    expect(screen.getByRole('tab', { name: 'Jobs / Activity' })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/jobs/job-inventory')).toBe(true));
    expect(calls.filter((call) => call.method === 'POST' && call.path === syncPath)).toHaveLength(postCount);
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(storageSet).not.toHaveBeenCalled();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
    expect(consoleLog).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();
  });

  it('uses node.write rather than node.bootstrap for guided inventory synchronization', async () => {
    setInventorySyncEligibleState();
    await openNode({ ...authPayload, permissions: ['node.read', 'node.bootstrap'] });
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect(await screen.findByText('node.write is required for inventory synchronization. Status remains visible with read permissions.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Synchronize inventory' })).not.toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('allows node.write-only operators to run guided inventory sync on an already registered node', async () => {
    setInventorySyncEligibleState();
    await openNode({ ...authPayload, permissions: ['node.read', 'node.write'] });
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect(await screen.findByText('Read-only onboarding status')).toBeInTheDocument();
    expect(screen.getAllByText('node.bootstrap is required for onboarding actions. Status remains visible with read permissions.').length).toBeGreaterThan(0);
    await userEvent.click(await screen.findByRole('button', { name: 'Synchronize inventory' }));
    const dialog = activeDialog();
    await userEvent.click(within(dialog).getByLabelText(/queues an asynchronous inventory job/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(true));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);
  });

  it('renders guided inventory running, stalled, failed and ready states from backend evidence', async () => {
    setInventorySyncEligibleState({
      communication_state: 'job_running',
      agent: {
        last_job_claim_job_id: 'job-inventory',
        last_job_claim_type: 'node.inventory.sync',
        last_job_claim_at: '2026-07-09T08:01:00Z',
      },
    });
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(await screen.findByText('Inventory job running.')).toBeInTheDocument();
    expect(screen.getAllByText('Running').length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: 'Synchronize inventory' })).not.toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('blocks duplicate guided inventory sync for stalled jobs and allows explicit retry after failed jobs', async () => {
    setInventorySyncEligibleState({
      communication_state: 'job_result_stalled',
      agent: {
        last_job_claim_job_id: 'job-inventory',
        last_job_claim_type: 'node.inventory.sync',
        last_job_claim_at: '2026-07-09T08:01:00Z',
      },
    });
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(await screen.findByText('Inventory result stalled. Review diagnostics before retrying.')).toBeInTheDocument();
    expect(screen.getAllByText('Stalled').length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: 'Synchronize inventory' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Retry inventory synchronization' })).not.toBeInTheDocument();

    setInventorySyncEligibleState({
      agent: {
        last_job_result_job_id: 'job-inventory',
        last_job_result_type: 'node.inventory.sync',
        last_job_result_status: 'failed',
        last_job_result_at: '2026-07-09T08:02:00Z',
      },
    });
    await userEvent.click(screen.getByRole('button', { name: 'Refresh status' }));
    expect(await screen.findByText('Inventory synchronization failed.')).toBeInTheDocument();
    await userEvent.click(await screen.findByRole('button', { name: 'Retry inventory synchronization' }));
    const dialog = activeDialog();
    expect(within(dialog).getByText('Retry inventory synchronization')).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('handles guided inventory conflicts without recording a local accepted job', async () => {
    actionErrors.inventorySync = 409;
    setInventorySyncEligibleState();
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Synchronize inventory' }));
    const dialog = activeDialog();
    await userEvent.click(within(dialog).getByLabelText(/queues an asynchronous inventory job/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await screen.findByText('Inventory synchronization conflicts with current node or job state.');
    expect(activeDialog()).toBe(dialog);
    expect(screen.queryByText('Inventory job accepted')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Open Jobs' })).not.toBeInTheDocument();
    expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toHaveLength(1);
  });

  it('renders ready state only after registration, heartbeat, inventory and healthy communication evidence', async () => {
    setInventorySyncEligibleState({
      communication_state: 'inventory_ok',
      latest_inventory: inventory,
      agent: {
        last_inventory_sync_at: '2026-07-09T08:01:00Z',
      },
    });
    currentInventory = clone(inventory);
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect((await screen.findAllByText('Ready')).length).toBeGreaterThan(0);
    expect(screen.getByText('Onboarding ready')).toBeInTheDocument();
    expect(screen.getByText('Control-plane onboarding milestones are complete.')).toBeInTheDocument();
    expect(screen.getByText('Live external-node validation is still required before production cutover.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Synchronize inventory' })).not.toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('issues an enrollment token from onboarding with explicit confirmation and one-time display', async () => {
    const clipboardWrite = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: clipboardWrite } });
    currentNode = { ...currentNode, execution_mode: 'local_managed' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;
    const queryClient = await openNode();

    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(await screen.findByRole('button', { name: 'Issue enrollment token' })).toBeInTheDocument();
    await userEvent.clear(screen.getByLabelText('Enrollment token TTL'));
    await userEvent.type(screen.getByLabelText('Enrollment token TTL'), '0');
    expect(await screen.findByText('Token TTL must be between 1 and 720 hours.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Issue enrollment token' })).toBeDisabled();
    await userEvent.clear(screen.getByLabelText('Enrollment token TTL'));
    await userEvent.type(screen.getByLabelText('Enrollment token TTL'), '12');
    await userEvent.click(screen.getByRole('button', { name: 'Issue enrollment token' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=12')).toBe(false);
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    expect(screen.getAllByText('The plaintext token is shown only in the immediate response.').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Issuing a token does not prove agent connectivity, registration or heartbeat.').length).toBeGreaterThan(0);

    await userEvent.click(screen.getByLabelText(/enrollment token is sensitive/));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=12')).toBe(true));
    expect(screen.getByRole('tab', { name: 'Onboarding' })).toHaveAttribute('aria-selected', 'true');
    expect(document.body.textContent || '').not.toContain('enroll-secret-token');
    expect(clipboardWrite).not.toHaveBeenCalled();
    expect(JSON.stringify(queryClient.getQueryCache().getAll().map((query) => query.state.data))).not.toContain('enroll-secret-token');
    const issueMutation = queryClient.getMutationCache().getAll().find((mutation) => mutation.options.gcTime === 0 && JSON.stringify(mutation.state.variables || {}).includes('node-1'));
    expect(issueMutation?.state.data).toBeUndefined();
    expect(JSON.stringify(issueMutation?.state.variables || {})).not.toContain('enroll-secret-token');

    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(await screen.findByText('enroll-secret-token')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));
    expect(clipboardWrite).toHaveBeenCalledWith('enroll-secret-token');
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('reissues enrollment from onboarding only for recovery states', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'offline',
      communication_state: 'auth_failure',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
        last_auth_failure_at: '2026-07-09T08:10:00Z',
      },
      active_enrollment_token: currentEnrollmentTokens[0],
    };
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect(await screen.findByRole('button', { name: 'Reissue enrollment token' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Reissue enrollment token' }));
    expect(screen.getByText('Existing active enrollment credentials may be revoked or replaced according to backend behavior.')).toBeInTheDocument();
    expect(screen.getByText('Reissue does not automatically update a remote machine, bootstrap file or running agent.')).toBeInTheDocument();
    expect(screen.getByText('Reissue does not prove recovery until registration and heartbeat are observed.')).toBeInTheDocument();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token/rotate?ttl_hours=24')).toBe(false);
    await userEvent.click(screen.getByLabelText(/previous enrollment credential may stop working/));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token/rotate?ttl_hours=24')).toBe(true));
    expect(screen.getByRole('tab', { name: 'Onboarding' })).toHaveAttribute('aria-selected', 'true');
    expect(document.body.textContent || '').not.toContain('enroll-rotated-token');
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }));
    expect(await screen.findByText('enroll-rotated-token')).toBeInTheDocument();
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('submits guided manual bootstrap only after explicit mode selection and acknowledgement', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect(await screen.findByText('Choose bootstrap method')).toBeInTheDocument();
    expect(screen.getByText('SSH bootstrap')).toBeInTheDocument();
    expect(screen.getByText('Manual bootstrap bundle')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start bootstrap' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Reveal bundle' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download bundle' })).not.toBeInTheDocument();

    await userEvent.click(screen.getByLabelText(/Manual bootstrap bundle/));
    await userEvent.click(screen.getByRole('button', { name: 'Start bootstrap' }));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);

    const dialog = activeDialog();
    expect(within(dialog).getByText('Manual bootstrap bundle')).toBeInTheDocument();
    expect(within(dialog).getByText('The job generates sensitive bootstrap material.')).toBeInTheDocument();
    expect(within(dialog).getByText('Bundle generation does not execute the bundle on the node.')).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await userEvent.click(within(dialog).getByLabelText(/this job generates sensitive bootstrap material/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(true));
    const bootstrapCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap');
    expect(bootstrapCall?.body).toEqual({ bootstrap_mode: 'manual_bundle' });
    expect(Object.keys(bootstrapCall?.body || {}).sort()).toEqual(['bootstrap_mode']);
    expect(screen.getByRole('tab', { name: 'Onboarding' })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByText('Bootstrap job accepted')).toBeInTheDocument();
    expect(await screen.findByText(/Bootstrap run tracked/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reveal bundle' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download bundle' })).not.toBeInTheDocument();

    const bootstrapPosts = calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap').length;
    await userEvent.click(screen.getByRole('button', { name: 'Open Jobs' }));
    expect(screen.getByRole('tab', { name: 'Jobs / Activity' })).toHaveAttribute('aria-selected', 'true');
    expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toHaveLength(bootstrapPosts);
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/inventory/sync')).toBe(false);
  });

  it('submits guided SSH bootstrap with safe target confirmation and mode-only request body', async () => {
    currentNode = { ...currentNode, execution_mode: 'ssh_bootstrap' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;
    currentAccessMethods = [{
      ...currentAccessMethods[0],
      secret_ref_id: 'opaque-value-not-for-ui',
    } as typeof currentAccessMethods[number]];

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect(await screen.findByText('Choose bootstrap method')).toBeInTheDocument();
    expect(screen.getByText('Recommended')).toBeInTheDocument();
    expect(screen.getByText('edge-one.example.test')).toBeInTheDocument();
    expect(document.body.textContent || '').not.toContain('opaque-value-not-for-ui');
    await userEvent.click(screen.getByRole('button', { name: 'Start bootstrap' }));

    const dialog = activeDialog();
    expect(within(dialog).getByText('SSH bootstrap')).toBeInTheDocument();
    expect(within(dialog).getByText('edge-one.example.test')).toBeInTheDocument();
    expect(within(dialog).getByText('ubuntu')).toBeInTheDocument();
    expect(within(dialog).getByText('The server will connect to the configured SSH target.')).toBeInTheDocument();
    expect(within(dialog).getByText('The pinned host fingerprint and configured SSH credential will be used.')).toBeInTheDocument();
    expect(within(dialog).queryByText('opaque-value-not-for-ui')).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/reinstall_agent/)).not.toBeInTheDocument();
    expect(within(dialog).queryByText(/force_reenroll/)).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await userEvent.click(within(dialog).getByLabelText(/SSH bootstrap uses the configured target/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(true));
    const bootstrapCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap');
    expect(bootstrapCall?.body).toEqual({ bootstrap_mode: 'ssh_bootstrap' });
    expect(JSON.stringify(bootstrapCall?.body || {})).not.toContain('opaque-value-not-for-ui');
    expect(JSON.stringify(bootstrapCall?.body || {})).not.toContain('enroll');
    expect(JSON.stringify(bootstrapCall?.body || {})).not.toContain('private');
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
  });

  it('blocks duplicate guided bootstrap while a run is active or unknown', async () => {
    currentNode = { ...currentNode, execution_mode: 'ssh_bootstrap' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [{
      id: 'bootstrap-run-active',
      node_id: 'node-1',
      job_id: 'job-bootstrap-active',
      status: 'queued',
      bootstrap_mode: 'ssh_bootstrap',
      created_at: '2026-07-09T08:10:00Z',
    }];
    currentInventory = null;

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(await screen.findByText('A bootstrap run is already queued or running; duplicate guided submission is disabled.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start bootstrap' })).toBeDisabled();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);

    currentBootstrapRuns = [{
      ...currentBootstrapRuns[0],
      id: 'bootstrap-run-unknown',
      status: 'mystery',
      created_at: '2026-07-09T08:11:00Z',
    }];
    await userEvent.click(screen.getByRole('button', { name: 'Refresh status' }));
    expect((await screen.findAllByText('Bootstrap status is unknown.')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('Latest bootstrap state is unknown. Refresh state or inspect Jobs before retrying.').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Start bootstrap' })).toBeDisabled();
  });

  it('shows successful manual bundle guidance without reveal or download in onboarding', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [{
      id: 'bootstrap-run-manual-ready',
      node_id: 'node-1',
      job_id: 'job-bootstrap-manual-ready',
      status: 'succeeded',
      bootstrap_mode: 'manual_bundle',
      manual_bundle_available: true,
      created_at: '2026-07-09T08:10:00Z',
      finished_at: '2026-07-09T08:11:00Z',
    }];
    currentInventory = null;

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(await screen.findByText('Manual bundle available')).toBeInTheDocument();
    expect(screen.getByText('Open Bootstrap to reveal or download the generated bundle through the audited bundle controls.')).toBeInTheDocument();
    expect(screen.getByText('Waiting for agent registration.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reveal bundle' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download bundle' })).not.toBeInTheDocument();
    await userEvent.click(screen.getAllByRole('button', { name: 'Open Bootstrap' })[0]);
    expect(screen.getByRole('tab', { name: 'Bootstrap' })).toHaveAttribute('aria-selected', 'true');
    expect(calls.some((call) => call.path.includes('/bundle/reveal'))).toBe(false);
    expect(calls.some((call) => call.path.includes('/bundle/download'))).toBe(false);
  });

  it('keeps guided SSH selection safe after firewall prerequisite rejection', async () => {
    actionErrors.bootstrap = 409;
    currentNode = { ...currentNode, execution_mode: 'ssh_bootstrap' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;

    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Start bootstrap' }));
    const dialog = activeDialog();
    await userEvent.click(within(dialog).getByLabelText(/SSH bootstrap uses the configured target/));
    await userEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('Firewall policy blocks SSH bootstrap. Review firewall policy before retrying.')).toBeInTheDocument();
    expect(activeDialog()).toBe(dialog);
    expect(within(dialog).getByText('SSH bootstrap')).toBeInTheDocument();
    expect(document.body.textContent || '').not.toContain('secret_ref_id');
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);
  });

  it('does not show onboarding token actions for healthy registered nodes', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'online',
      communication_state: 'inventory_ok',
      agent: {
        node_id: 'node-1',
        status: 'active',
        registered_at: '2026-07-09T08:00:00Z',
        last_seen_at: '2026-07-09T08:00:30Z',
        last_inventory_sync_at: '2026-07-09T08:01:00Z',
        token_rotation_status: 'active',
      },
      latest_inventory: inventory,
    };
    currentInventory = clone(inventory);
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect((await screen.findAllByText('Ready')).length).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: 'Issue enrollment token' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reissue enrollment token' })).not.toBeInTheDocument();
  });

  it('handles empty onboarding token responses safely without showing an empty secret panel', async () => {
    emptyEnrollmentIssueResponse = true;
    currentNode = { ...currentNode, execution_mode: 'local_managed' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Issue enrollment token' }));
    await userEvent.click(screen.getByLabelText(/enrollment token is sensitive/));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('The backend accepted the request but did not return a token value. Refresh status and retry only after checking backend logs.')).toBeInTheDocument();
    expect(screen.queryByText('********************************')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reveal' })).not.toBeInTheDocument();
  });

  it('discards stale onboarding token responses after the drawer closes', async () => {
    delayEnrollmentCreateResponse = true;
    currentNode = { ...currentNode, execution_mode: 'local_managed' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;
    await openNode();
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Issue enrollment token' }));
    await userEvent.click(screen.getByLabelText(/enrollment token is sensitive/));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(resolveEnrollmentCreateResponse).not.toBeNull());
    await userEvent.click(screen.getAllByRole('button', { name: 'Close' })[0]);
    resolveEnrollmentCreateResponse?.();
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/enrollment-token?ttl_hours=24')).toBe(true));

    expect(document.body.textContent || '').not.toContain('enroll-secret-token');
    expect(screen.queryByRole('button', { name: 'Reveal' })).not.toBeInTheDocument();
  });

  it('keeps onboarding visible without node.bootstrap and shows the read-only permission hint', async () => {
    currentNode = { ...currentNode, execution_mode: 'ssh_bootstrap' };
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'missing_agent_identity',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'missing',
      },
    };
    currentEnrollmentTokens = [];
    currentBootstrapRuns = [];
    currentInventory = null;
    await openNode({ ...authPayload, permissions: ['node.read', 'node.write'] });
    await userEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));

    expect(await screen.findByText('Read-only onboarding status')).toBeInTheDocument();
    expect(screen.getAllByText('node.bootstrap is required for onboarding actions. Status remains visible with read permissions.').length).toBeGreaterThan(0);
    expect(screen.getByText('Choose bootstrap method')).toBeInTheDocument();
    expect(screen.getByText('SSH bootstrap')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Create token' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Issue enrollment token' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reissue enrollment token' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue bootstrap' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start bootstrap' })).toBeDisabled();
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/bootstrap')).toBe(false);
  });

  it('polls safe onboarding queries only while active and stops when ready or closed', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'unknown',
      communication_state: 'awaiting_enrollment',
      agent: {
        node_id: 'node-1',
        status: 'missing',
        token_rotation_status: 'awaiting_enrollment',
      },
    };
    currentInventory = null;
    renderPage();
    expect((await screen.findAllByText('Edge One')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await screen.findByRole('heading', { name: 'Edge One' });
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    calls.length = 0;

    await vi.advanceTimersByTimeAsync(10_000);
    await Promise.resolve();
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/diagnostics')).toBe(true);
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/enrollment-tokens')).toBe(true);
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/bootstrap-runs?limit=25')).toBe(true);
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/inventory')).toBe(true);
    expect(calls.every((call) => call.method === 'GET')).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/agent'))).toBe(true);
    expect(calls.every((call) => !call.path.startsWith('/legacy'))).toBe(true);

    fireEvent.click(screen.getByRole('tab', { name: 'Runtime / Agent' }));
    calls.length = 0;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/enrollment-tokens')).toBe(false);
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/bootstrap-runs?limit=25')).toBe(false);
    expect(calls.some((call) => call.path === '/api/v1/nodes/node-1/inventory')).toBe(false);

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    calls.length = 0;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(calls).toHaveLength(0);
  });

  it('does not start onboarding polling for an already ready model', async () => {
    currentDiagnostics = {
      ...currentDiagnostics,
      heartbeat_state: 'online',
      communication_state: 'inventory_ok',
      agent: {
        node_id: 'node-1',
        status: 'active',
        registered_at: '2026-07-09T08:00:00Z',
        last_seen_at: '2026-07-09T08:00:30Z',
        last_inventory_sync_at: '2026-07-09T08:01:00Z',
        token_rotation_status: 'active',
      },
      latest_inventory: inventory,
    };
    currentInventory = clone(inventory);
    renderPage();
    expect((await screen.findAllByText('Edge One')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    await screen.findByRole('heading', { name: 'Edge One' });
    vi.useFakeTimers();
    fireEvent.click(screen.getByRole('tab', { name: 'Onboarding' }));
    expect(screen.getAllByText('Ready').length).toBeGreaterThan(0);
    calls.length = 0;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(calls).toHaveLength(0);
  });

  it('keeps raw API paths and legacy workflow links out of the Nodes page component', () => {
    expect(String(NodesPage)).not.toContain('/api/v1');
    expect(String(NodesPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);
    expect(String(NodesPage)).not.toContain('dangerouslySetInnerHTML');
    expect(String(NodesPage)).not.toContain('/legacy');
  });
});
