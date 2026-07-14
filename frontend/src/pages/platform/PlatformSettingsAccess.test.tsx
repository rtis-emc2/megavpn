import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../../shared/auth/AuthProvider';
import i18n from '../../shared/i18n';
import { AccessPage } from './AccessPage';
import { MailPage } from './MailPage';
import { SettingsPage } from './SettingsPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const smtpSecret = 'smtp-secret-value';
const smtpSecretRef = 'secret-smtp-ref';
const inviteSecret = 'super-secret-invite-token';

const authPayload = {
  user: {
    id: 'operator-1',
    username: 'admin',
    email: 'admin@example.test',
    display_name: 'Admin',
    status: 'active',
  },
  session: {
    id: 'session-current',
    expires_at: '2026-08-10T00:00:00Z',
  },
  roles: ['superadmin'],
  permissions: ['settings.manage', 'auth.manage'],
};

const edgeCertificate = {
  id: 'cert-edge-1',
  name: 'edge.example.test',
  description: 'Default edge certificate',
  source: 'imported',
  kind: 'leaf',
  status: 'active',
  common_name: 'edge.example.test',
  sans: ['edge.example.test'],
  issuer_name: 'MegaVPN Managed CA',
  cert_secret_ref_id: 'secret-cert-ref',
  key_secret_ref_id: 'secret-key-ref',
  not_before: '2026-07-08T00:00:00Z',
  not_after: '2027-07-08T00:00:00Z',
  is_default: true,
  created_at: '2026-07-08T00:00:00Z',
  updated_at: '2026-07-08T00:00:00Z',
};

function json(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function renderPage(element: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{element}</AuthProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return queryClient;
}

function enabledButton(name: string | RegExp): HTMLButtonElement {
  const button = screen.getAllByRole('button', { name }).find((candidate) => !(candidate as HTMLButtonElement).disabled);
  if (!button) throw new Error(`enabled button not found: ${String(name)}`);
  return button as HTMLButtonElement;
}

function serializedCalls(spy: { mock: { calls: unknown[][] } }) {
  return JSON.stringify(spy.mock.calls);
}

describe('Platform settings, mail and access pages', () => {
  const calls: FetchCall[] = [];
  let tlsSettings: Record<string, unknown>;
  let mailSettings: Record<string, unknown>;
  let users: Record<string, unknown>[];
  let invites: Record<string, unknown>[];
  let sessions: Record<string, unknown>[];
  let tlsUpdateStatus = 200;
  let inviteCreateStatus = 201;
  let consoleLogSpy: ReturnType<typeof vi.spyOn>;
  let consoleWarnSpy: ReturnType<typeof vi.spyOn>;
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;
  let storageSetSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(async () => {
    calls.length = 0;
    tlsUpdateStatus = 200;
    inviteCreateStatus = 201;
    tlsSettings = {
      enabled: true,
      mode: 'managed_certificate',
      public_base_url: 'https://console.example.test',
      server_name: 'console.example.test',
      listen_port: 443,
      upstream_url: 'http://127.0.0.1:8080',
      certificate_id: 'cert-edge-1',
      self_signed_common_name: '',
      self_signed_dns_names: [],
      last_applied_at: '2026-07-08T02:00:00Z',
      last_error: '',
      created_at: '2026-07-08T01:00:00Z',
      updated_at: '2026-07-08T01:00:00Z',
    };
    mailSettings = {
      enabled: true,
      smtp_host: 'smtp.example.test',
      smtp_port: 587,
      smtp_username: 'mailer',
      smtp_password_secret_ref_id: smtpSecretRef,
      smtp_password_configured: true,
      smtp_auth_mode: 'plain',
      smtp_tls_mode: 'starttls',
      from_email: 'noreply@example.test',
      from_name: 'MegaVPN',
      reply_to_email: 'support@example.test',
      invite_url_base: 'https://console.example.test/invite',
      last_test_at: '2026-07-08T03:00:00Z',
      last_error: '',
      created_at: '2026-07-08T01:00:00Z',
      updated_at: '2026-07-08T01:00:00Z',
    };
    users = [{
      id: 'user-1',
      username: 'operator',
      email: 'operator@example.test',
      display_name: 'Operator One',
      status: 'active',
      auth_source: 'local',
      roles: ['operator'],
      last_login_at: '2026-07-08T04:00:00Z',
      created_at: '2026-07-08T01:00:00Z',
      updated_at: '2026-07-08T01:00:00Z',
    }];
    invites = [{
      id: 'invite-1',
      user_id: 'user-2',
      username: 'new-user',
      email: 'new-user@example.test',
      display_name: 'New User',
      token_hint: 'tok...abcd',
      status: 'sent',
      expires_at: '2026-08-10T00:00:00Z',
      sent_at: '2026-07-08T05:00:00Z',
      delivery_error: '',
      created_by: 'operator-1',
      created_at: '2026-07-08T05:00:00Z',
    }];
    sessions = [{
      id: 'sess-2',
      user_id: 'user-1',
      username: 'operator',
      email: 'operator@example.test',
      display_name: 'Operator One',
      ip: '203.0.113.10',
      user_agent: 'Firefox',
      expires_at: '2026-08-10T00:00:00Z',
      revoked_at: null,
      created_at: '2026-07-08T06:00:00Z',
    }];
    await i18n.changeLanguage('en');
    window.localStorage.clear();
    consoleLogSpy = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    storageSetSpy = vi.spyOn(Storage.prototype, 'setItem');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/auth/me') return json(authPayload);
      if (method === 'GET' && url.pathname === '/api/v1/runtime/preflight') return json({ database: { status: 'ok' }, tls: { status: 'ok' } });
      if (method === 'GET' && url.pathname === '/api/v1/platform/certificates') return json([edgeCertificate]);
      if (method === 'GET' && url.pathname === '/api/v1/settings/control-plane-tls') return json(tlsSettings);
      if (method === 'PUT' && url.pathname === '/api/v1/settings/control-plane-tls') {
        if (tlsUpdateStatus === 422) return json({ error: 'invalid TLS settings', fields: { public_base_url: 'must use https' } }, 422);
        if (tlsUpdateStatus === 403) return json({ error: 'settings.manage permission required' }, 403);
        if (tlsUpdateStatus === 409) return json({ error: 'certificate is not active' }, 409);
        tlsSettings = { ...tlsSettings, ...body, updated_at: '2026-07-08T07:00:00Z' };
        return json(tlsSettings);
      }
      if (method === 'POST' && url.pathname === '/api/v1/settings/control-plane-tls/apply') {
        return json({ id: 'job-tls-1', type: 'control_plane_tls.apply', status: 'queued', result: { target: 'edge' } }, 202);
      }
      if (method === 'GET' && url.pathname === '/api/v1/jobs/job-tls-1') {
        return json({ id: 'job-tls-1', type: 'control_plane_tls.apply', status: 'queued', result: { target: 'edge' } });
      }
      if (method === 'GET' && url.pathname === '/api/v1/jobs/job-tls-1/logs') return json([]);

      if (method === 'GET' && url.pathname === '/api/v1/settings/mail') return json(mailSettings);
      if (method === 'PUT' && url.pathname === '/api/v1/settings/mail') {
        if (body?.smtp_password === smtpSecret) {
          mailSettings = {
            ...mailSettings,
            ...body,
            smtp_password_secret_ref_id: 'secret-smtp-new-ref',
            smtp_password_configured: true,
            updated_at: '2026-07-08T08:00:00Z',
          };
        } else {
          mailSettings = { ...mailSettings, ...body, updated_at: '2026-07-08T08:00:00Z' };
        }
        return json(mailSettings);
      }
      if (method === 'POST' && url.pathname === '/api/v1/settings/mail/test') {
        return json({ status: 'ok', message: 'test email sent', test: { recipient: body?.email } });
      }

      if (method === 'GET' && url.pathname === '/api/v1/admin/users') return json(users);
      if (method === 'GET' && url.pathname === '/api/v1/admin/user-invites') return json(invites);
      if (method === 'POST' && url.pathname === '/api/v1/admin/users/invite') {
        if (inviteCreateStatus === 403) return json({ error: 'auth.manage permission required' }, 403);
        if (inviteCreateStatus === 409) return json({ error: 'mail settings are disabled' }, 409);
        if (inviteCreateStatus === 422) return json({ error: 'invalid invite payload' }, 422);
        const user = {
          id: 'user-created',
          username: String(body?.username || ''),
          email: String(body?.email || ''),
          display_name: String(body?.display_name || ''),
          status: 'pending_invite',
          auth_source: 'local',
          roles: Array.isArray(body?.role_codes) ? body.role_codes : [],
          created_at: '2026-07-08T09:00:00Z',
          updated_at: '2026-07-08T09:00:00Z',
        };
        const invite = {
          id: 'invite-created',
          user_id: user.id,
          username: user.username,
          email: user.email,
          display_name: user.display_name,
          token_hint: 'tok...wxyz',
          status: 'sent',
          expires_at: '2026-07-10T00:00:00Z',
          sent_at: '2026-07-08T09:00:00Z',
          created_by: 'operator-1',
          created_at: '2026-07-08T09:00:00Z',
        };
        users = [user, ...users];
        invites = [invite, ...invites];
        return json({ status: 'ok', user, invite, invite_url: `https://console.example.test/invite/${inviteSecret}` }, 201);
      }
      if (method === 'GET' && url.pathname === '/api/v1/admin/sessions') return json(sessions);
      if (method === 'POST' && url.pathname === '/api/v1/admin/sessions/sess-2/revoke') {
        sessions = sessions.map((session) => session.id === 'sess-2' ? { ...session, revoked_at: '2026-07-08T10:00:00Z' } : session);
        return json({ status: 'ok' });
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    consoleLogSpy.mockRestore();
    consoleWarnSpy.mockRestore();
    consoleErrorSpy.mockRestore();
    storageSetSpy.mockRestore();
    vi.unstubAllGlobals();
    cleanup();
  });

  it('loads and saves control-plane TLS settings and applies them only after confirmation', async () => {
    renderPage(<SettingsPage />);

    await screen.findByLabelText('Public base URL');
    await userEvent.clear(screen.getByLabelText('Public base URL'));
    await userEvent.type(screen.getByLabelText('Public base URL'), 'https://console2.example.test');
    await userEvent.click(enabledButton('Save'));

    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/settings/control-plane-tls')).toBe(true));
    const saveCall = calls.find((call) => call.method === 'PUT' && call.path === '/api/v1/settings/control-plane-tls');
    expect(saveCall?.body?.public_base_url).toBe('https://console2.example.test');
    expect(await screen.findByText('Control-plane TLS settings saved.')).toBeInTheDocument();

    await userEvent.click(enabledButton('Apply TLS'));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/settings/control-plane-tls/apply')).toBe(false);
    await userEvent.click(screen.getAllByRole('button', { name: 'Apply' }).at(-1)!);

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/settings/control-plane-tls/apply')).toBe(true));
    expect(await screen.findByText(/TLS apply job/)).toBeInTheDocument();
    expect(await screen.findByText('job-tls-1')).toBeInTheDocument();
  });

  it('maps backend field errors for settings save failures', async () => {
    tlsUpdateStatus = 422;
    renderPage(<SettingsPage />);

    await screen.findByLabelText('Public base URL');
    await userEvent.clear(screen.getByLabelText('Public base URL'));
    await userEvent.type(screen.getByLabelText('Public base URL'), 'http://not-secure.example.test');
    await userEvent.click(enabledButton('Save'));

    expect(await screen.findByText('must use https')).toBeInTheDocument();
    expect((await screen.findAllByText('Validation failed: invalid TLS settings')).length).toBeGreaterThan(0);
  });

  it('saves mail settings with masked write-only secrets and runs real mail test', async () => {
    renderPage(<MailPage />);

    const passwordInput = await screen.findByLabelText('SMTP password') as HTMLInputElement;
    expect(passwordInput.type).toBe('password');
    expect(screen.queryByText(smtpSecretRef)).not.toBeInTheDocument();

    await userEvent.type(passwordInput, smtpSecret);
    await userEvent.clear(screen.getByLabelText('From name'));
    await userEvent.type(screen.getByLabelText('From name'), 'MegaVPN Console');
    await userEvent.click(enabledButton('Save'));

    await waitFor(() => expect(calls.some((call) => call.method === 'PUT' && call.path === '/api/v1/settings/mail')).toBe(true));
    const saveCall = calls.find((call) => call.method === 'PUT' && call.path === '/api/v1/settings/mail');
    expect(saveCall?.body?.smtp_password).toBe(smtpSecret);
    expect(await screen.findByText('Mail settings saved.')).toBeInTheDocument();
    expect(screen.queryByDisplayValue(smtpSecret)).not.toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('Test email'), 'ops@example.test');
    await userEvent.click(enabledButton('Send test'));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/settings/mail/test')).toBe(true));
    const testCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/settings/mail/test');
    expect(testCall?.body?.email).toBe('ops@example.test');

    expect(screen.queryByText(smtpSecret)).not.toBeInTheDocument();
    expect(serializedCalls(consoleLogSpy)).not.toContain(smtpSecret);
    expect(serializedCalls(consoleWarnSpy)).not.toContain(smtpSecret);
    expect(serializedCalls(consoleErrorSpy)).not.toContain(smtpSecret);
    expect(serializedCalls(storageSetSpy)).not.toContain(smtpSecret);
  });

  it('loads platform users, creates invites without rendering invite secrets and keeps invite revoke disabled', async () => {
    renderPage(<AccessPage />);

    expect((await screen.findAllByText('operator')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    expect((await screen.findAllByText('Operator One')).length).toBeGreaterThan(0);
    expect(screen.getByText('local')).toBeInTheDocument();

    await userEvent.click(enabledButton('Create invite'));
    await userEvent.type(await screen.findByLabelText('Username'), 'new-operator');
    await userEvent.type(screen.getByLabelText('Email'), 'new-operator@example.test');
    await userEvent.clear(screen.getByLabelText('Roles'));
    await userEvent.type(screen.getByLabelText('Roles'), 'operator, auditor');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create invite' }).at(-1)!);

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/admin/users/invite')).toBe(true));
    const inviteCall = calls.find((call) => call.method === 'POST' && call.path === '/api/v1/admin/users/invite');
    expect(inviteCall?.body?.role_codes).toEqual(['operator', 'auditor']);
    expect(await screen.findByText('Invite created and delivery requested.')).toBeInTheDocument();
    expect(screen.queryByText(inviteSecret)).not.toBeInTheDocument();

    const disabledRevoke = screen.getAllByRole('button', { name: 'Revoke invite' })[0] as HTMLButtonElement;
    expect(disabledRevoke).toBeDisabled();
    expect(disabledRevoke.title).toContain('backend has no invite revoke endpoint');
  });

  it('requires confirmation before revoking sessions', async () => {
    renderPage(<AccessPage />);

    expect((await screen.findAllByText('203.0.113.10')).length).toBeGreaterThan(0);
    await userEvent.click(enabledButton('Revoke session'));
    expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/admin/sessions/sess-2/revoke')).toBe(false);
    await userEvent.click(screen.getAllByRole('button', { name: 'Apply' }).at(-1)!);

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/admin/sessions/sess-2/revoke')).toBe(true));
    expect(await screen.findByText('Session revoked.')).toBeInTheDocument();
  });

  it('surfaces 403, 409 and 422 errors for access mutations', async () => {
    renderPage(<AccessPage />);

    await userEvent.click(await screen.findByRole('button', { name: 'Create invite' }));
    await userEvent.type(await screen.findByLabelText('Username'), 'blocked-user');
    await userEvent.type(screen.getByLabelText('Email'), 'blocked@example.test');

    inviteCreateStatus = 403;
    await userEvent.click(screen.getAllByRole('button', { name: 'Create invite' }).at(-1)!);
    expect(await screen.findByText('Permission denied: auth.manage permission required')).toBeInTheDocument();

    inviteCreateStatus = 409;
    await userEvent.click(screen.getAllByRole('button', { name: 'Create invite' }).at(-1)!);
    expect(await screen.findByText('Conflict: mail settings are disabled')).toBeInTheDocument();

    inviteCreateStatus = 422;
    await userEvent.click(screen.getAllByRole('button', { name: 'Create invite' }).at(-1)!);
    expect(await screen.findByText('Validation failed: invalid invite payload')).toBeInTheDocument();
  });

  it('does not call /legacy for implemented settings, mail and access workflows', async () => {
    renderPage(<SettingsPage />);
    await screen.findByLabelText('Public base URL');
    await userEvent.click(enabledButton('Apply TLS'));
    await userEvent.click(screen.getAllByRole('button', { name: 'Apply' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.path === '/api/v1/settings/control-plane-tls/apply')).toBe(true));
    cleanup();

    renderPage(<MailPage />);
    await userEvent.type(await screen.findByLabelText('Test email'), 'ops@example.test');
    await userEvent.click(enabledButton('Send test'));
    await waitFor(() => expect(calls.some((call) => call.path === '/api/v1/settings/mail/test')).toBe(true));
    cleanup();

    renderPage(<AccessPage />);
    expect((await screen.findAllByText('203.0.113.10')).length).toBeGreaterThan(0);
    await userEvent.click(enabledButton('Revoke session'));
    await userEvent.click(screen.getAllByRole('button', { name: 'Apply' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.path === '/api/v1/admin/sessions/sess-2/revoke')).toBe(true));

    expect(calls.some((call) => call.path.includes('/legacy'))).toBe(false);
  });
});
