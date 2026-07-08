import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../shared/i18n';
import { CertificatesPage } from './CertificatesPage';

type FetchCall = {
  method: string;
  path: string;
  body?: Record<string, unknown>;
};

const secretKey = '-----BEGIN PRIVATE KEY-----\ncert-secret-material\n-----END PRIVATE KEY-----';
const certPem = '-----BEGIN CERTIFICATE-----\npublic-cert-material\n-----END CERTIFICATE-----';

const leafCertificate = {
  id: 'cert-leaf-1',
  name: 'edge.example.test',
  description: 'Default edge certificate',
  source: 'imported',
  kind: 'leaf',
  status: 'active',
  common_name: 'edge.example.test',
  sans: ['edge.example.test', 'vpn.example.test'],
  issuer_name: 'MegaVPN Managed CA',
  parent_certificate_id: 'cert-ca-1',
  cert_secret_ref_id: 'secret-cert-ref',
  key_secret_ref_id: 'secret-key-ref',
  chain_secret_ref_id: null,
  not_before: '2026-07-08T00:00:00Z',
  not_after: '2027-07-08T00:00:00Z',
  is_default: true,
  created_at: '2026-07-08T01:00:00Z',
  updated_at: '2026-07-08T01:00:00Z',
};

const caCertificate = {
  id: 'cert-ca-1',
  name: 'MegaVPN Managed CA',
  description: 'Managed authority',
  source: 'managed_ca',
  kind: 'ca',
  status: 'active',
  common_name: 'MegaVPN Managed CA',
  sans: [],
  issuer_name: 'MegaVPN Managed CA',
  parent_certificate_id: null,
  cert_secret_ref_id: 'secret-ca-cert-ref',
  key_secret_ref_id: 'secret-ca-key-ref',
  chain_secret_ref_id: null,
  not_before: '2026-07-08T00:00:00Z',
  not_after: '2036-07-08T00:00:00Z',
  is_default: false,
  created_at: '2026-07-08T01:00:00Z',
  updated_at: '2026-07-08T01:00:00Z',
};

const pkiRoot = {
  id: 'root-1',
  service_code: 'openvpn',
  pki_profile: 'default',
  status: 'active',
  ca_cert_secret_ref_id: 'secret-root-cert-ref',
  common_name: 'MegaVPN OpenVPN Platform CA',
  not_before: '2026-07-08T00:00:00Z',
  not_after: '2036-07-08T00:00:00Z',
  created_at: '2026-07-08T01:00:00Z',
  rotated_at: null,
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
        <CertificatesPage />
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

describe('CertificatesPage', () => {
  const calls: FetchCall[] = [];
  let certificates: Array<typeof leafCertificate | typeof caCertificate>;
  let roots: typeof pkiRoot[];
  let selfSignedStatus = 201;
  let consoleSpy: ReturnType<typeof vi.spyOn>;
  let storageSetSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(async () => {
    calls.length = 0;
    certificates = [leafCertificate, caCertificate];
    roots = [pkiRoot];
    selfSignedStatus = 201;
    window.localStorage.clear();
    await i18n.changeLanguage('en');
    consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    storageSetSpy = vi.spyOn(Storage.prototype, 'setItem');
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://megavpn.test');
      const method = String(init?.method || 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined;
      calls.push({ method, path: `${url.pathname}${url.search}`, body });

      if (method === 'GET' && url.pathname === '/api/v1/platform/certificates') return json(certificates);
      if (method === 'GET' && url.pathname === '/api/v1/platform/pki-roots') return json(roots);
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/preview') {
        return json({
          common_name: 'imported.example.test',
          issuer_name: 'Imported CA',
          sans: ['imported.example.test'],
          not_before: '2026-07-08T00:00:00Z',
          not_after: '2027-07-08T00:00:00Z',
          is_ca: false,
          private_key_type: 'RSA',
          key_pair_valid: true,
          chain_certificate_count: body?.chain ? 1 : 0,
        });
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/import') {
        const imported = {
          ...leafCertificate,
          id: 'cert-imported',
          name: String(body?.name || 'imported.example.test'),
          common_name: 'imported.example.test',
          issuer_name: 'Imported CA',
          source: 'imported',
          is_default: Boolean(body?.is_default),
        };
        certificates = [imported, ...certificates];
        return json(imported, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/self-signed') {
        if (selfSignedStatus === 403) return json({ error: 'instance.write permission required' }, 403);
        if (selfSignedStatus === 409) return json({ error: 'certificate already exists' }, 409);
        if (selfSignedStatus === 422) return json({ error: 'common name is invalid' }, 422);
        const created = { ...leafCertificate, id: 'cert-self', name: String(body?.name || body?.common_name), common_name: String(body?.common_name), source: 'self_signed', is_default: Boolean(body?.is_default) };
        certificates = [created, ...certificates];
        return json(created, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/authorities') {
        const created = { ...caCertificate, id: 'cert-ca-2', name: String(body?.name || body?.common_name), common_name: String(body?.common_name), source: 'managed_ca' };
        certificates = [created, ...certificates];
        return json(created, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/issue-from-ca') {
        const issued = { ...leafCertificate, id: 'cert-issued', name: String(body?.name || body?.common_name), common_name: String(body?.common_name), source: 'ca_issued', parent_certificate_id: String(body?.authority_certificate_id) };
        certificates = [issued, ...certificates];
        return json(issued, 201);
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/cert-leaf-1/default') {
        return json({ certificate_id: 'cert-leaf-1', action: 'set_default', status: 'default' });
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/certificates/cert-leaf-1/revoke') {
        return json({ certificate_id: 'cert-leaf-1', action: 'revoke', status: 'revoked' });
      }
      if (method === 'DELETE' && url.pathname === '/api/v1/platform/certificates/cert-ca-1') {
        return json({ certificate_id: 'cert-ca-1', action: 'delete', status: 'deleted', cascade_ids: ['cert-ca-1', 'cert-leaf-1'], cascade_count: 2 });
      }
      if (method === 'POST' && url.pathname === '/api/v1/platform/pki-roots') {
        const created = { ...pkiRoot, id: 'root-2', service_code: String(body?.service_code), pki_profile: String(body?.pki_profile || 'default'), common_name: String(body?.common_name || 'Managed service CA') };
        roots = [created, ...roots];
        return json(created, 201);
      }
      return json({ error: `unhandled ${method} ${url.pathname}` }, 404);
    }));
  });

  afterEach(() => {
    consoleSpy.mockRestore();
    storageSetSpy.mockRestore();
    vi.unstubAllGlobals();
    cleanup();
  });

  it('loads certificate and PKI root inventory and opens safe detail', async () => {
    renderPage();
    expect((await screen.findAllByText('edge.example.test')).length).toBeGreaterThan(0);
    expect((await screen.findAllByText('MegaVPN OpenVPN Platform CA')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getAllByRole('button', { name: 'Open' })[0]);
    expect(await screen.findByText('Private key stored')).toBeInTheDocument();
    expect(screen.getByText('edge.example.test, vpn.example.test')).toBeInTheDocument();
    expect(screen.queryByText('secret-key-ref')).not.toBeInTheDocument();
    expect(screen.queryByText(secretKey)).not.toBeInTheDocument();
  });

  it('previews and imports certificates through real endpoints with stale preview protection', async () => {
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: 'Import certificate' }));
    await userEvent.type(await screen.findByLabelText('Name'), 'imported edge');
    await userEvent.type(screen.getByLabelText('Certificate PEM'), certPem);
    await userEvent.type(screen.getByLabelText('Private key'), secretKey);
    expect(screen.getAllByRole('button', { name: 'Import certificate' }).at(-1)).toBeDisabled();

    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    expect((await screen.findAllByText('imported.example.test')).length).toBeGreaterThan(0);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/preview')).toBe(true));

    await userEvent.type(screen.getByLabelText('Private key'), '\nchanged');
    expect(screen.getAllByRole('button', { name: 'Import certificate' }).at(-1)).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    await waitFor(() => expect(calls.filter((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/preview').length).toBeGreaterThanOrEqual(2));
    await userEvent.click(screen.getAllByRole('button', { name: 'Import certificate' }).at(-1)!);

    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/import')).toBe(true));
    expect(screen.queryByDisplayValue(secretKey)).not.toBeInTheDocument();
  });

  it('creates self-signed certificates, issues from CA and creates managed service PKI roots', async () => {
    renderPage();
    expect((await screen.findAllByText('edge.example.test')).length).toBeGreaterThan(0);

    await userEvent.click(screen.getByRole('button', { name: 'Create self-signed' }));
    await userEvent.type(await screen.findByLabelText('Common name'), 'self.example.test');
    await userEvent.type(screen.getByLabelText('DNS names'), 'self.example.test,alt.example.test');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create self-signed' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/self-signed')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Create managed CA' }));
    await userEvent.type(await screen.findByLabelText('Common name'), 'Managed Operators CA');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create managed CA' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/authorities')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Issue certificate' }));
    await userEvent.selectOptions(await screen.findByLabelText('Authority'), 'cert-ca-1');
    await userEvent.type(screen.getByLabelText('Common name'), 'issued.example.test');
    await userEvent.click(screen.getAllByRole('button', { name: 'Issue certificate' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/issue-from-ca')).toBe(true));

    await userEvent.click(screen.getByRole('button', { name: 'Create PKI root' }));
    await userEvent.clear(await screen.findByLabelText('Service code'));
    await userEvent.type(screen.getByLabelText('Service code'), 'openvpn');
    await userEvent.clear(screen.getByLabelText('PKI profile'));
    await userEvent.type(screen.getByLabelText('PKI profile'), 'operators');
    await userEvent.click(screen.getAllByRole('button', { name: 'Create PKI root' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/pki-roots')).toBe(true));
  });

  it('requires confirmation for default, revoke and delete actions', async () => {
    renderPage();
    expect((await screen.findAllByText('edge.example.test')).length).toBeGreaterThan(0);

    await userEvent.click(firstEnabledButton('Set default'));
    expect(calls.some((call) => call.path.endsWith('/default'))).toBe(false);
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/cert-leaf-1/default')).toBe(true));

    await userEvent.click(firstEnabledButton('Revoke'));
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'POST' && call.path === '/api/v1/platform/certificates/cert-leaf-1/revoke')).toBe(true));

    await userEvent.click(firstEnabledButton('Delete'));
    await screen.findByText('Backend deletes the selected CA certificate tree as a cascade.');
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(calls.some((call) => call.method === 'DELETE' && call.path === '/api/v1/platform/certificates/cert-ca-1')).toBe(true));
  });

  it('shows 403, 409 and 422 mutation errors without fake success', async () => {
    for (const [status, message] of [
      [403, /Permission denied: instance.write permission required/],
      [409, /Conflict: certificate already exists/],
      [422, /Validation failed: common name is invalid/],
    ] as const) {
      selfSignedStatus = status;
      renderPage();
      expect((await screen.findAllByText('edge.example.test')).length).toBeGreaterThan(0);
      await userEvent.click(screen.getByRole('button', { name: 'Create self-signed' }));
      await userEvent.type(await screen.findByLabelText('Common name'), 'bad.example.test');
      await userEvent.click(screen.getAllByRole('button', { name: 'Create self-signed' }).at(-1)!);
      await screen.findByText(message);
      cleanup();
      calls.length = 0;
    }
  });

  it('does not log or persist certificate private keys and keeps implemented workflow off legacy', async () => {
    renderPage();
    await userEvent.click(await screen.findByRole('button', { name: 'Import certificate' }));
    await userEvent.type(await screen.findByLabelText('Certificate PEM'), certPem);
    await userEvent.type(screen.getByLabelText('Private key'), secretKey);
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    expect((await screen.findAllByText('imported.example.test')).length).toBeGreaterThan(0);
    await userEvent.click(screen.getAllByRole('button', { name: 'Import certificate' }).at(-1)!);
    await waitFor(() => expect(calls.some((call) => call.path === '/api/v1/platform/certificates/import')).toBe(true));

    expect(consoleSpy.mock.calls.flat().some((value: unknown) => String(value).includes('cert-secret-material'))).toBe(false);
    expect(storageSetSpy.mock.calls.some((call: unknown[]) => String(call[0]).toLowerCase().includes('key') || String(call[1]).includes('cert-secret-material'))).toBe(false);
    expect(calls.some((call) => call.path.startsWith('/legacy'))).toBe(false);
    expect(screen.queryByText('/legacy')).not.toBeInTheDocument();
    expect(String(CertificatesPage)).not.toContain('/legacy');
    expect(String(CertificatesPage)).not.toContain('/api/v1');
    expect(String(CertificatesPage)).not.toMatch(/(^|[^A-Za-z0-9_])fetch\s*\(/);

    const importCall = calls.find((call) => call.path === '/api/v1/platform/certificates/import');
    expect(importCall?.body?.private_key).toContain('cert-secret-material');
    expect(within(document.body).queryByText('cert-secret-material')).not.toBeInTheDocument();
  });
});
