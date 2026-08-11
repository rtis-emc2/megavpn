import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../../shared/auth/AuthProvider';
import i18n from '../../shared/i18n';
import { AuditPage } from './AuditPage';

const events = [
  {
    id: 'audit-event-1',
    actor_user_id: 'operator-1',
    actor_username: 'alice',
    actor_email: 'alice@example.com',
    actor_display_name: 'Alice Operator',
    actor_type: 'platform_user',
    action: 'firewall.policy.update',
    resource_type: 'firewall',
    resource_id: 'firewall-policy-node-base',
    summary: 'Updated firewall policy node_base',
    created_at: '2026-08-11T09:00:00Z',
  },
  {
    id: 'audit-event-2',
    actor_type: 'system',
    action: 'schema.migration.apply',
    resource_type: 'platform',
    resource_id: null,
    summary: 'Applied database schema migration',
    created_at: '2026-08-11T08:00:00Z',
  },
];

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
        <AuditPage />
      </AuthProvider>
    </QueryClientProvider>,
  );
}

describe('AuditPage', () => {
  beforeEach(async () => {
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      if (method === 'GET' && url.pathname === '/api/v1/auth/me') {
        return json({
          user: { id: 'operator-1', username: 'alice', status: 'active' },
          session: { id: 'session-1', expires_at: '2026-08-12T00:00:00Z' },
          roles: ['auditor'],
          permissions: ['audit.read'],
        });
      }
      if (method === 'GET' && url.pathname === '/api/v1/audit') return json(events);
      return json({ error: `unhandled ${method} ${url.pathname}` }, 500);
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders the real audit contract instead of generic n/a and recorded cells', async () => {
    renderPage();

    expect((await screen.findAllByText('Updated firewall policy node_base')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('firewall.policy.update').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Alice Operator').length).toBeGreaterThan(0);
    expect(screen.getAllByText('alice@example.com').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Firewall').length).toBeGreaterThan(0);
    expect(screen.queryByText('recorded')).not.toBeInTheDocument();
    expect(screen.queryByText('n/a')).not.toBeInTheDocument();
  });

  it('filters events and exposes complete identifiers only in event details', async () => {
    const user = userEvent.setup();
    renderPage();

    const search = await screen.findByPlaceholderText('Action, description, operator, object or ID');
    await user.type(search, 'schema migration');
    expect(screen.getAllByText('Applied database schema migration').length).toBeGreaterThan(0);
    expect(screen.queryByText('Updated firewall policy node_base')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    const openButtons = await screen.findAllByRole('button', { name: 'Open' });
    await user.click(openButtons[0]!);

    const dialog = screen.getByRole('dialog', { name: 'Updated firewall policy node_base' });
    expect(within(dialog).getByText('audit-event-1')).toBeInTheDocument();
    expect(within(dialog).getByText('firewall-policy-node-base')).toBeInTheDocument();
    expect(within(dialog).getByText('operator-1')).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });
});
