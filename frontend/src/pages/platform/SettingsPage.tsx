import { Save, ShieldCheck } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { PlatformSettingsInput } from '../../shared/api/types';
import { useAuth } from '../../shared/auth/AuthProvider';
import { hasPermission } from '../../shared/permissions/permissions';
import { useApplyTlsSettings, useCertificates, usePlatformSettings, useRuntimePreflight, useUpdatePlatformSettings } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, FormField, FormGrid, JobStatusPanel, Select, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type TLSForm = {
  enabled: boolean;
  mode: string;
  publicBaseURL: string;
  serverName: string;
  listenPort: string;
  upstreamURL: string;
  certificateID: string;
  selfSignedCommonName: string;
  selfSignedDNSNames: string;
};

const emptyTLSForm: TLSForm = {
  enabled: true,
  mode: 'managed_certificate',
  publicBaseURL: '',
  serverName: '',
  listenPort: '443',
  upstreamURL: 'http://127.0.0.1:8080',
  certificateID: '',
  selfSignedCommonName: '',
  selfSignedDNSNames: '',
};

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : 'Request failed';
  const prefix = error.status === 403
    ? 'Permission denied'
    : error.status === 409
      ? 'Conflict'
      : error.status === 422 || error.status === 400
        ? 'Validation failed'
        : `HTTP ${error.status}`;
  return `${prefix}: ${error.message}`;
}

function fieldErrors(error: unknown): Record<string, string> {
  if (!(error instanceof APIError) || typeof error.payload !== 'object' || error.payload === null) return {};
  const payload = error.payload as { fields?: unknown };
  if (!payload.fields || typeof payload.fields !== 'object') return {};
  return Object.fromEntries(Object.entries(payload.fields as Record<string, unknown>).map(([key, value]) => [key, String(value)]));
}

function formFromSettings(settings: Record<string, unknown> | undefined): TLSForm {
  if (!settings) return emptyTLSForm;
  return {
    enabled: Boolean(settings.enabled ?? true),
    mode: text(settings.mode, 'managed_certificate'),
    publicBaseURL: text(settings.public_base_url, ''),
    serverName: text(settings.server_name, ''),
    listenPort: text(settings.listen_port, '443'),
    upstreamURL: text(settings.upstream_url, 'http://127.0.0.1:8080'),
    certificateID: text(settings.certificate_id, ''),
    selfSignedCommonName: text(settings.self_signed_common_name, ''),
    selfSignedDNSNames: Array.isArray(settings.self_signed_dns_names) ? settings.self_signed_dns_names.join('\n') : '',
  };
}

function splitNames(value: string): string[] {
  return value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean);
}

function toInput(form: TLSForm): PlatformSettingsInput {
  return {
    enabled: form.enabled,
    mode: form.mode,
    public_base_url: form.publicBaseURL,
    server_name: form.serverName,
    listen_port: Number(form.listenPort) || 443,
    upstream_url: form.upstreamURL,
    certificate_id: form.certificateID,
    self_signed_common_name: form.selfSignedCommonName,
    self_signed_dns_names: splitNames(form.selfSignedDNSNames),
  };
}

export function SettingsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const auth = useAuth();
  const canManage = hasPermission(auth.permissions, auth.roles, 'settings.manage');
  const preflight = useRuntimePreflight({ retry: false });
  const settings = usePlatformSettings({ retry: false });
  const certificates = useCertificates({ retry: false });
  const update = useUpdatePlatformSettings();
  const apply = useApplyTlsSettings();
  const serverForm = useMemo(() => formFromSettings(settings.data), [settings.data]);
  const [draft, setDraft] = useState<TLSForm | null>(null);
  const [notice, setNotice] = useState('');
  const [applyOpen, setApplyOpen] = useState(false);
  const [jobID, setJobID] = useState('');
  const errors = fieldErrors(update.error);
  const form = draft || serverForm;
  const dirty = draft !== null && JSON.stringify(draft) !== JSON.stringify(serverForm);

  const certificateOptions = useMemo(() => (certificates.data || []).filter((item) => item.kind === 'leaf' && item.status === 'active' && item.key_secret_ref_id), [certificates.data]);
  const checks = Object.entries(preflight.data || {}).map(([key, value]) => ({ key, value }));

  const patch = (partial: Partial<TLSForm>) => setDraft((current) => ({ ...(current || serverForm), ...partial }));
  const save = async () => {
    setNotice('');
    try {
      await update.mutateAsync(toInput(form));
      setDraft(null);
      setNotice(t('settings.tlsSaved'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  const runApply = async () => {
    setNotice('');
    try {
      const job = await apply.mutateAsync();
      setJobID(job.id);
      setApplyOpen(false);
      setNotice(t('settings.tlsApplyQueued', { job: shortID(job.id) }));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold title={t('settings.title')} subtitle={t('settings.subtitle')} actions={<Button icon={<Save size={16} />} variant="primary" disabled={!canManage || !dirty || update.isPending} onClick={() => void save()}>{t('common.save')}</Button>}>
      <QueryBoundary isLoading={preflight.isLoading} isError={preflight.isError} error={preflight.error} refetch={() => void preflight.refetch()}>
        <DataTable
          title={t('settings.preflight')}
          rows={checks}
          columns={[
            { key: 'key', header: t('common.name'), render: (row) => <code>{row.key}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={typeof row.value === 'object' && row.value && 'status' in row.value ? String(row.value.status) : 'unknown'} /> },
            { key: 'value', header: t('common.description'), render: (row) => <code>{text(JSON.stringify(row.value))}</code> },
          ]}
        />
      </QueryBoundary>
      <QueryBoundary isLoading={settings.isLoading || certificates.isLoading} isError={settings.isError || certificates.isError} error={(settings.error || certificates.error) as Error | null} refetch={() => { void settings.refetch(); void certificates.refetch(); }}>
        <Card>
          <CardBody>
            <div className="page-stack">
              <Toolbar>
                <h2 className="card-title">{t('settings.tls')}</h2>
                <StatusBadge status={settings.data?.enabled ? 'enabled' : 'disabled'} />
                {!canManage ? <Badge>{t('common.permissionRequired', { permission: 'settings.manage' })}</Badge> : null}
              </Toolbar>
              {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
              {update.error ? <div role="alert" className="error-state-inline">{formatAPIError(update.error)}</div> : null}
              <FormGrid>
                <FormField label={t('settings.enabled')}>
                  <label className="toolbar">
                    <input type="checkbox" checked={form.enabled} onChange={(event) => patch({ enabled: event.target.checked })} />
                    <span>{form.enabled ? t('common.enabled') : t('common.disabled')}</span>
                  </label>
                </FormField>
                <FormField label={t('settings.tlsMode')}>
                  <Select value={form.mode} onChange={(event) => patch({ mode: event.target.value })}>
                    <option value="managed_certificate">{t('settings.managedCertificate')}</option>
                    <option value="self_signed_fallback">{t('settings.selfSignedFallback')}</option>
                  </Select>
                </FormField>
                <FormField label={t('settings.publicBaseURL')}>
                  <TextField value={form.publicBaseURL} onChange={(event) => patch({ publicBaseURL: event.target.value })} />
                  {errors.public_base_url ? <span role="alert">{errors.public_base_url}</span> : null}
                </FormField>
                <FormField label={t('settings.serverName')}>
                  <TextField value={form.serverName} onChange={(event) => patch({ serverName: event.target.value })} />
                  {errors.server_name ? <span role="alert">{errors.server_name}</span> : null}
                </FormField>
                <FormField label={t('settings.listenPort')}>
                  <TextField type="number" min={1} max={65535} value={form.listenPort} onChange={(event) => patch({ listenPort: event.target.value })} />
                  {errors.listen_port ? <span role="alert">{errors.listen_port}</span> : null}
                </FormField>
                <FormField label={t('settings.upstreamURL')}>
                  <TextField value={form.upstreamURL} onChange={(event) => patch({ upstreamURL: event.target.value })} />
                  {errors.upstream_url ? <span role="alert">{errors.upstream_url}</span> : null}
                </FormField>
                <FormField label={t('settings.certificate')}>
                  <Select value={form.certificateID} onChange={(event) => patch({ certificateID: event.target.value })}>
                    <option value="">{t('common.none')}</option>
                    {certificateOptions.map((certificate) => <option key={certificate.id} value={certificate.id}>{certificate.name || certificate.common_name || certificate.id}</option>)}
                  </Select>
                  {errors.certificate_id ? <span role="alert">{errors.certificate_id}</span> : null}
                </FormField>
                <FormField label={t('settings.selfSignedCommonName')}>
                  <TextField value={form.selfSignedCommonName} onChange={(event) => patch({ selfSignedCommonName: event.target.value })} />
                </FormField>
                <FormField label={t('settings.selfSignedDNSNames')} full>
                  <Textarea rows={3} value={form.selfSignedDNSNames} onChange={(event) => patch({ selfSignedDNSNames: event.target.value })} />
                </FormField>
              </FormGrid>
              <div className="definition-grid">
                <span>{t('settings.lastApplied')}</span><strong>{fmt.date(settings.data?.last_applied_at)}</strong>
                <span>{t('settings.lastError')}</span><strong>{text(settings.data?.last_error)}</strong>
              </div>
              <Toolbar>
                <Button icon={<Save size={16} />} variant="primary" disabled={!canManage || !dirty || update.isPending} onClick={() => void save()}>{t('common.save')}</Button>
                <Button icon={<ShieldCheck size={16} />} variant="danger" disabled={!canManage || apply.isPending} onClick={() => setApplyOpen(true)}>{t('settings.applyTls')}</Button>
              </Toolbar>
              {jobID ? <JobStatusPanel jobID={jobID} /> : null}
            </div>
          </CardBody>
        </Card>
      </QueryBoundary>
      <ConfirmDialog title={t('settings.applyTlsConfirmTitle')} open={applyOpen} onClose={() => setApplyOpen(false)}>
        <div className="page-stack">
          <p>{t('settings.applyTlsConfirmBody')}</p>
          <Toolbar>
            <Button variant="danger" disabled={apply.isPending} onClick={() => void runApply()}>{t('common.apply')}</Button>
            <Button onClick={() => setApplyOpen(false)}>{t('common.cancel')}</Button>
          </Toolbar>
        </div>
      </ConfirmDialog>
    </PageScaffold>
  );
}
