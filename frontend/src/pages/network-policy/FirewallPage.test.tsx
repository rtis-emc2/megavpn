import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import { FirewallPage } from './FirewallPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const nodes = [
  { id: 'node-1', name: 'Edge One', role: 'edge', status: 'active' },
  { id: 'node-2', name: 'Edge Two', role: 'edge', status: 'active' },
];

const baseInventory = {
  address_lists: [
    { id: 'group-1', key: 'trusted_operators', label: 'Trusted operators', scope: 'global', status: 'active', entry_count: 1, updated_at: '2026-07-08T10:00:00Z' },
    { id: 'group-dns', key: 'dns_only', label: 'DNS only', scope: 'global', status: 'active', entry_count: 1, updated_at: '2026-07-08T10:00:00Z' },
  ],
  entries: [
    { id: 'entry-1', list_id: 'group-1', list_key: 'trusted_operators', value: '10.10.0.0/24', value_type: 'cidr', status: 'active', updated_at: '2026-07-08T10:00:00Z' },
    { id: 'entry-dns', list_id: 'group-dns', list_key: 'dns_only', value: 'ops.example.test', value_type: 'dns', status: 'active', updated_at: '2026-07-08T10:00:00Z' },
  ],
  policies: [
    {
      id: 'policy-1',
      key: 'node_base',
      label: 'Default node firewall',
      scope: 'global',
      status: 'active',
      default_input_policy: 'drop',
      default_forward_policy: 'drop',
      default_output_policy: 'accept',
      rule_count: 2,
      updated_at: '2026-07-08T10:00:00Z',
    },
    {
      id: 'policy-2',
      key: 'observe',
      label: 'Observe only',
      scope: 'global',
      status: 'active',
      default_input_policy: 'accept',
      default_forward_policy: 'accept',
      default_output_policy: 'accept',
      rule_count: 0,
      updated_at: '2026-07-08T10:00:00Z',
    },
  ],
  rules: [
    {
      id: 'rule-1',
      policy_id: 'policy-1',
      priority: 100,
      chain: 'input',
      action: 'accept',
      direction: 'in',
      protocol: 'tcp',
      src_list_id: 'group-1',
      src_list_key: 'trusted_operators',
      dst_ports: '22',
      state_match: ['new', 'established', 'related'],
      comment: 'SSH bootstrap',
      enabled: true,
      status: 'active',
      updated_at: '2026-07-08T10:00:00Z',
    },
    {
      id: 'rule-dns',
      policy_id: 'policy-1',
      priority: 200,
      chain: 'forward',
      action: 'accept',
      src_list_id: 'group-dns',
      src_list_key: 'dns_only',
      protocol: 'tcp',
      state_match: ['new'],
      comment: 'DNS-only source',
      enabled: true,
      status: 'active',
      updated_at: '2026-07-08T10:00:00Z',
    },
  ],
  node_states: [
    { id: 'state-1', node_id: 'node-1', node_name: 'Edge One', policy_id: 'policy-1', policy_key: 'node_base', status: 'applied', last_job_id: 'job-state', observed: {}, updated_at: '2026-07-08T10:00:00Z' },
  ],
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
        <FirewallPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return queryClient;
}

describe('FirewallPage', () => {
  const calls: FetchCall[] = [];
  let inventory: typeof baseInventory;
  let previewResult: Record<string, unknown>;
  let endpointError: { path: string; method: string; status: number; message: string } | null;

  beforeEach(async () => {
    calls.length = 0;
    inventory = structuredClone(baseInventory);
    endpointError = null;
    previewResult = {
      id: 'job-preview',
      type: 'node.firewall.preview',
      status: 'queued',
      payload: {
        policy_id: 'policy-1',
        policy_key: 'node_base',
        firewall_payload_hash: 'preview-hash',
        safety_mode: 'strict',
        default_input_policy: 'drop',
        default_forward_policy: 'drop',
        default_output_policy: 'accept',
        rules: [{ id: 'rule-1' }],
        address_lists: [{ key: 'trusted_operators' }],
        ssh_bootstrap_ports: [22],
        node_requires_forward_preservation: true,
      },
      result: {
        rendered_hash: 'rendered-hash',
        warnings: ['Forward policy can affect transit traffic.'],
        ssh_bootstrap_preserved: true,
        control_plane_egress_preserved: true,
        forward_egress_preserved: true,
      },
    };
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (endpointError && endpointError.path === url.pathname && endpointError.method === method) {
        return json({ status: 'error', error: endpointError.message }, endpointError.status);
      }
      if (method === 'GET' && url.pathname === '/api/v1/firewall') return json(inventory);
      if (method === 'GET' && url.pathname === '/api/v1/nodes') return json(nodes);
      if (method === 'GET' && url.pathname === '/api/v1/firewall/management-settings') {
        return json({
          control_plane_source_cidrs: ['10.10.0.0/24'],
          ssh_bootstrap_source_cidrs: ['10.10.0.0/24'],
          trusted_operator_cidrs: ['10.10.0.0/24'],
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/firewall/address-lists') {
        return json({ id: 'group-new', ...body, entry_count: 0 }, 201);
      }
      if (method === 'PUT' && url.pathname === '/api/v1/firewall/address-lists/group-1') {
        return json({ id: 'group-1', ...body, entry_count: 1 });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/firewall/address-lists/group-1') {
        return json({ ...inventory.address_lists[0], status: 'deleted' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/firewall/address-lists/group-1/entries') {
        return json({ id: 'entry-new', list_id: 'group-1', ...body }, 201);
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/firewall/address-lists/group-1/entries/entry-1') {
        return json({ ...inventory.entries[0], status: 'deleted' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/firewall/policies') {
        return json({ id: 'policy-new', ...body }, 201);
      }
      if (method === 'PUT' && url.pathname === '/api/v1/firewall/policies/policy-1') {
        return json({ id: 'policy-1', ...body });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/firewall/policies/policy-1') {
        return json({ ...inventory.policies[0], status: 'deleted' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/firewall/policies/policy-1/rules') {
        return json({ id: 'rule-new', policy_id: 'policy-1', ...body }, 201);
      }
      if (method === 'PUT' && url.pathname === '/api/v1/firewall/policies/policy-1/rules/rule-1') {
        return json({ id: 'rule-1', ...body });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/firewall/policies/policy-1/rules/rule-1') {
        return json({ ...inventory.rules[0], status: 'deleted' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/firewall/preview') {
        return json(previewResult, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/firewall/apply') {
        return json({
          id: 'job-apply',
          type: 'node.firewall.apply',
          status: 'queued',
          payload: { policy_id: body.policy_id, firewall_payload_hash: 'preview-hash' },
          result: {},
        }, 202);
      }
      if (method === 'POST' && url.pathname === '/api/v1/nodes/node-1/firewall/disable') {
        return json({ id: 'job-disable', type: 'node.firewall.disable', status: 'queued', payload: {}, result: {} }, 202);
      }
      if (method === 'GET' && url.pathname.startsWith('/api/v1/jobs/')) {
        return json({ id: url.pathname.split('/').at(-1), type: 'node.firewall.preview', status: 'queued', result: {} });
      }
      if (method === 'GET' && url.pathname.endsWith('/logs')) return json([]);
      return json({ status: 'error', error: `unhandled ${method} ${url.pathname}` }, 500);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  async function loaded() {
    renderPage();
    await screen.findByText('Default node firewall');
  }

  async function selectNodeAndPolicy() {
    await userEvent.selectOptions(screen.getByLabelText('Target node'), 'node-1');
    await userEvent.selectOptions(screen.getByLabelText('Policy'), 'policy-1');
  }

  async function openTab(name: string) {
    await userEvent.click(screen.getByRole('tab', { name }));
  }

  it('loads policies and address groups from mocked API', async () => {
    await loaded();
    expect(screen.getAllByText('Trusted operators').length).toBeGreaterThan(0);
    expect(calls.some((call) => call.method === 'GET' && call.path === '/api/v1/firewall')).toBe(true);
  });

  it('creates address group through the real API wrapper path', async () => {
    await loaded();
    await userEvent.type(screen.getByLabelText('Address group key'), 'ops');
    await userEvent.type(screen.getByLabelText('Address group label'), 'Ops CIDRs');
    await userEvent.click(screen.getByRole('button', { name: /create address group/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/firewall/address-lists' && call.body?.label === 'Ops CIDRs')).toBe(true));
  });

  it('updates address group through the real API wrapper path', async () => {
    await loaded();
    await userEvent.click(screen.getAllByRole('button', { name: /edit group/i })[0]);
    await userEvent.clear(screen.getByLabelText('Address group label'));
    await userEvent.type(screen.getByLabelText('Address group label'), 'Operators updated');
    await userEvent.click(screen.getByRole('button', { name: /update address group/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/firewall/address-lists/group-1' && call.body?.label === 'Operators updated')).toBe(true));
  });

  it('deletes address group through the real API wrapper path', async () => {
    await loaded();
    await userEvent.click(screen.getAllByRole('button', { name: /delete group/i })[0]);
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/firewall/address-lists/group-1')).toBe(true));
  });

  it('shows DNS-only and empty renderable address group warnings', async () => {
    await loaded();
    expect(screen.getAllByText(/DNS-only entries are not rendered into nftables/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Blocking warning: active accept rule references this group without renderable entries/i).length).toBeGreaterThan(0);
  });

  it('creates rule through the real API wrapper path', async () => {
    await loaded();
    await userEvent.selectOptions(screen.getByLabelText('Policy'), 'policy-1');
    await openTab('Rules');
    await userEvent.clear(screen.getByLabelText('Rule priority'));
    await userEvent.type(screen.getByLabelText('Rule priority'), '300');
    await userEvent.type(screen.getByLabelText('Rule comment'), 'Allow HTTPS');
    await userEvent.click(screen.getByRole('button', { name: /create rule/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/firewall/policies/policy-1/rules' && call.body?.comment === 'Allow HTTPS')).toBe(true));
  });

  it('updates rule through the real API wrapper path', async () => {
    await loaded();
    await userEvent.selectOptions(screen.getByLabelText('Policy'), 'policy-1');
    await openTab('Rules');
    await userEvent.click(screen.getAllByRole('button', { name: /edit rule/i })[0]);
    await userEvent.clear(screen.getByLabelText('Rule comment'));
    await userEvent.type(screen.getByLabelText('Rule comment'), 'SSH bootstrap updated');
    await userEvent.click(screen.getByRole('button', { name: /update rule/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/firewall/policies/policy-1/rules/rule-1' && call.body?.comment === 'SSH bootstrap updated')).toBe(true));
  });

  it('deletes rule through the real API wrapper path', async () => {
    await loaded();
    await userEvent.selectOptions(screen.getByLabelText('Policy'), 'policy-1');
    await openTab('Rules');
    await userEvent.click(screen.getAllByRole('button', { name: /delete rule/i })[0]);
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/firewall/policies/policy-1/rules/rule-1')).toBe(true));
  });

  it('keeps Preview disabled until node and policy are selected', async () => {
    await loaded();
    await openTab('Preview & Apply');
    expect(screen.getByRole('button', { name: 'Preview' })).toBeDisabled();
    await selectNodeAndPolicy();
    expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled();
  });

  it('enables Apply after successful backend preview', async () => {
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled());
    expect(screen.getByText('preview-hash')).toBeInTheDocument();
  });

  it('marks preview stale and disables Apply after policy changes', async () => {
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled());
    await userEvent.selectOptions(screen.getByLabelText('Policy'), 'policy-2');
    expect(screen.getByText('Preview is stale')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('blocks Apply when preview returns blocking errors', async () => {
    previewResult = {
      ...previewResult,
      result: { blocking_errors: ['strict input drop without management CIDR is blocked'] },
    };
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(screen.getByText(/strict input drop/i)).toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('opens Apply confirmation and sends real backend apply request', async () => {
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled());
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await userEvent.click(screen.getByRole('button', { name: /confirm apply/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/firewall/apply' && call.body?.policy_id === 'policy-1')).toBe(true));
  });

  it('shows Apply job link after backend accepts apply', async () => {
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled());
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await userEvent.click(screen.getByRole('button', { name: /confirm apply/i }));
    await screen.findByText('job-apply');
    expect(screen.getAllByRole('link', { name: /open jobs page/i }).length).toBeGreaterThan(0);
  });

  it('opens Disable confirmation and sends real backend disable request', async () => {
    await loaded();
    await userEvent.selectOptions(screen.getByLabelText('Target node'), 'node-1');
    await openTab('Node state');
    await userEvent.click(screen.getByRole('button', { name: /emergency disable/i }));
    expect(screen.getAllByText('Disable removes only managed table inet megavpn_firewall and does not remove instances/backhaul/route policy.').length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: /confirm disable/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/nodes/node-1/firewall/disable')).toBe(true));
  });

  it('shows permission error for backend 403', async () => {
    endpointError = { method: 'POST', path: '/api/v1/nodes/node-1/firewall/preview', status: 403, message: 'firewall.apply permission required' };
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByText(/Permission denied \(403\): firewall.apply permission required/i);
  });

  it('maps backend 422 validation error', async () => {
    endpointError = { method: 'POST', path: '/api/v1/firewall/address-lists', status: 422, message: 'invalid CIDR' };
    await loaded();
    await userEvent.type(screen.getByLabelText('Address group label'), 'Broken');
    await userEvent.click(screen.getByRole('button', { name: /create address group/i }));
    await screen.findByText(/Validation error \(422\): invalid CIDR/i);
  });

  it('shows backend 409 conflict', async () => {
    endpointError = { method: 'PUT', path: '/api/v1/firewall/policies/policy-1/rules/rule-1', status: 409, message: 'policy changed after preview' };
    await loaded();
    await userEvent.selectOptions(screen.getByLabelText('Policy'), 'policy-1');
    await openTab('Rules');
    await userEvent.click(screen.getAllByRole('button', { name: /edit rule/i })[0]);
    await userEvent.click(screen.getByRole('button', { name: /update rule/i }));
    await screen.findByText(/Conflict \(409\): policy changed after preview/i);
  });

  it('renders backend rendered output as text, not HTML', async () => {
    previewResult = {
      ...previewResult,
      result: { rendered_nftables: '<img src=x onerror=alert(1)> table inet megavpn_firewall' },
    };
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Preview & Apply');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByText(/<img src=x onerror=alert\(1\)> table inet megavpn_firewall/i);
    expect(document.querySelector('img')).toBeNull();
  });

  it('does not expose /legacy for Firewall core workflow', async () => {
    await loaded();
    expect(document.querySelector('a[href="/legacy/"]')).toBeNull();
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
  });

  it('supports adding IP/CIDR/range entries through the backend entry route', async () => {
    await loaded();
    await userEvent.selectOptions(screen.getByLabelText('Entry address group'), 'group-1');
    await userEvent.selectOptions(screen.getByLabelText('Entry value type'), 'range');
    await userEvent.type(screen.getByLabelText('Entry value'), '10.10.0.10-10.10.0.20');
    await userEvent.click(screen.getByRole('button', { name: /add entry/i }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/firewall/address-lists/group-1/entries' && call.body?.value_type === 'range')).toBe(true));
  });

  it('shows strict safety state and selected node state', async () => {
    await loaded();
    await selectNodeAndPolicy();
    await openTab('Safety');
    expect(screen.getByText(/trusted_control_plane: present/i)).toBeInTheDocument();
    expect(screen.getByText(/vpn_client_sources: absent/i)).toBeInTheDocument();
    await openTab('Node state');
    const edgeRow = screen.getAllByText('Edge One').map((element) => element.closest('tr')).find(Boolean) as HTMLElement;
    expect(within(edgeRow).getByText('applied')).toBeInTheDocument();
  });
});
