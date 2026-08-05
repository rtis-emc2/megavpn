import { Ban, FilePlus2, KeyRound, RefreshCw, ShieldCheck, ShieldPlus, Star, Trash2, Upload } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { Certificate, CertificateCreateInput, CertificateImportInput, CertificateImportPreview, CertificateIssueInput, PkiRoot, PkiRootCreateInput } from '../../shared/api/types';
import {
  useCertificateDetail,
  useCertificates,
  useCreateManagedCertificateAuthority,
  useCreatePkiRoot,
  useCreateSelfSignedCertificate,
  useDeleteCertificate,
  useImportCertificate,
  useIssueCertificate,
  usePkiRoots,
  usePreviewCertificateImport,
  useRevokeCertificate,
  useSetDefaultCertificate,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, Modal, Select, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type ConfirmAction = {
  type: 'default' | 'revoke' | 'delete';
  certificate: Certificate;
};

const defaultImportForm: CertificateImportInput = {
  name: '',
  description: '',
  certificate: '',
  private_key: '',
  chain: '',
  is_default: false,
};

const defaultCertForm: CertificateCreateInput = {
  name: '',
  description: '',
  common_name: '',
  dns_names: [],
  valid_days: 365,
  is_default: false,
};

const defaultAuthorityForm = {
  name: '',
  description: '',
  common_name: '',
  valid_days: 3650,
};

const defaultPkiRootForm: PkiRootCreateInput = {
  service_code: 'openvpn',
  pki_profile: 'default',
  common_name: '',
  valid_days: 10950,
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

function certificateLabel(certificate?: Certificate | null): string {
  if (!certificate) return 'n/a';
  return text(certificate.name || certificate.common_name || certificate.id);
}

function isExpired(certificate: Certificate): boolean {
  if (!certificate.not_after) return false;
  const parsed = new Date(certificate.not_after);
  return Number.isFinite(parsed.getTime()) && parsed.getTime() < Date.now();
}

function isExpiringSoon(certificate: Certificate): boolean {
  if (!certificate.not_after || isExpired(certificate)) return false;
  const parsed = new Date(certificate.not_after);
  return Number.isFinite(parsed.getTime()) && parsed.getTime() - Date.now() <= 1000 * 60 * 60 * 24 * 30;
}

function expiryStatus(certificate: Certificate): string {
  if (isExpired(certificate)) return 'expired';
  if (isExpiringSoon(certificate)) return 'expiring-soon';
  return 'valid';
}

function hasPrivateKey(certificate: Certificate): boolean {
  return Boolean(certificate.key_secret_ref_id);
}

function canSetDefault(certificate: Certificate): boolean {
  return certificate.kind === 'leaf' && certificate.status === 'active' && hasPrivateKey(certificate) && !isExpired(certificate);
}

function canRevoke(certificate: Certificate): boolean {
  return certificate.kind === 'leaf' && certificate.status !== 'deleted' && certificate.status !== 'revoked';
}

function canDelete(certificate: Certificate): boolean {
  return certificate.kind === 'ca' && certificate.status !== 'deleted';
}

function splitNames(value: string): string[] {
  return value
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeDays(value: unknown, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : fallback;
}

function previewFingerprint(input: CertificateImportInput): string {
  return JSON.stringify({
    certificate: input.certificate,
    private_key: input.private_key,
    chain: input.chain || '',
  });
}

async function readFileText(file: File | undefined): Promise<string> {
  if (!file) return '';
  return file.text();
}

function certificateUsage(certificate: Certificate): string {
  const usage = [];
  if (certificate.is_default) usage.push('default');
  if (certificate.kind === 'ca') usage.push('authority');
  if (certificate.parent_certificate_id) usage.push(`issued by ${shortID(certificate.parent_certificate_id)}`);
  if (certificate.chain_secret_ref_id) usage.push('chain');
  if (certificate.source) usage.push(certificate.source);
  return usage.length ? usage.join(', ') : 'inventory';
}

export function CertificatesPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const certificates = useCertificates();
  const pkiRoots = usePkiRoots();
  const setDefault = useSetDefaultCertificate();
  const revoke = useRevokeCertificate();
  const deleteCertificate = useDeleteCertificate();
  const [selectedId, setSelectedId] = useState('');
  const selectedDetail = useCertificateDetail(selectedId || undefined);
  const [importOpen, setImportOpen] = useState(false);
  const [selfSignedOpen, setSelfSignedOpen] = useState(false);
  const [authorityOpen, setAuthorityOpen] = useState(false);
  const [issueOpen, setIssueOpen] = useState(false);
  const [pkiRootOpen, setPkiRootOpen] = useState(false);
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);
  const [notice, setNotice] = useState('');

  const selected = selectedDetail.data || certificates.data?.find((item) => item.id === selectedId) || null;
  const authorities = useMemo(() => (certificates.data || []).filter((item) => item.kind === 'ca' && item.status === 'active'), [certificates.data]);
  const busy = setDefault.isPending || revoke.isPending || deleteCertificate.isPending;

  const runConfirmed = async () => {
    if (!confirm) return;
    setNotice('');
    try {
      if (confirm.type === 'default') await setDefault.mutateAsync(confirm.certificate.id);
      if (confirm.type === 'revoke') await revoke.mutateAsync(confirm.certificate.id);
      if (confirm.type === 'delete') await deleteCertificate.mutateAsync(confirm.certificate.id);
      setNotice(t(`certificates.confirm.${confirm.type}.done`));
      if (confirm.type === 'delete') setSelectedId('');
      setConfirm(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold
      title={t('certificates.title')}
      subtitle={t('certificates.subtitle')}
      actions={(
        <>
          <Button icon={<Upload size={16} />} onClick={() => setImportOpen(true)}>{t('certificates.import')}</Button>
          <Button icon={<FilePlus2 size={16} />} onClick={() => setSelfSignedOpen(true)}>{t('certificates.selfSigned')}</Button>
          <Button icon={<ShieldPlus size={16} />} onClick={() => setAuthorityOpen(true)}>{t('certificates.managedCa')}</Button>
        </>
      )}
    >
      <Toolbar>
        <Button icon={<KeyRound size={16} />} onClick={() => setIssueOpen(true)}>{t('certificates.issue')}</Button>
        <Button icon={<ShieldCheck size={16} />} onClick={() => setPkiRootOpen(true)}>{t('certificates.createPkiRoot')}</Button>
        <Button icon={<RefreshCw size={16} />} onClick={() => { void certificates.refetch(); void pkiRoots.refetch(); }}>{t('common.refresh')}</Button>
      </Toolbar>
      {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}

      <QueryBoundary
        isLoading={certificates.isLoading || pkiRoots.isLoading}
        isError={certificates.isError || pkiRoots.isError}
        error={(certificates.error || pkiRoots.error) as Error | null}
        refetch={() => { void certificates.refetch(); void pkiRoots.refetch(); }}
      >
        <DataTable
          rows={certificates.data || []}
          columns={[
            { key: 'cn', header: t('certificates.commonName'), render: (row) => <strong>{certificateLabel(row)}</strong> },
            { key: 'issuer', header: t('certificates.issuer'), render: (row) => text(row.issuer_name) },
            { key: 'kind', header: t('common.type'), render: (row) => <Badge>{text(row.kind || row.source)}</Badge> },
            { key: 'usage', header: t('certificates.usage'), render: (row) => certificateUsage(row) },
            { key: 'expiry', header: t('certificates.expiry'), render: (row) => <><StatusBadge status={expiryStatus(row)} /> {fmt.date(row.not_after)}</> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button onClick={() => setSelectedId(row.id)}>{t('common.open')}</Button>
                  <Button icon={<Star size={16} />} disabled={!canSetDefault(row)} onClick={() => setConfirm({ type: 'default', certificate: row })}>{t('certificates.setDefault')}</Button>
                  <Button icon={<Ban size={16} />} disabled={!canRevoke(row)} onClick={() => setConfirm({ type: 'revoke', certificate: row })}>{t('certificates.revoke')}</Button>
                  <Button variant="danger" icon={<Trash2 size={16} />} disabled={!canDelete(row)} onClick={() => setConfirm({ type: 'delete', certificate: row })}>{t('common.delete')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />

        <DataTable
          title={t('certificates.serviceRoots')}
          rows={pkiRoots.data || []}
          columns={[
            { key: 'service', header: t('certificates.serviceCode'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'profile', header: t('certificates.pkiProfile'), render: (row) => <code>{text(row.pki_profile)}</code> },
            { key: 'cn', header: t('certificates.commonName'), render: (row) => <strong>{text(row.common_name)}</strong> },
            { key: 'expiry', header: t('certificates.expiry'), render: (row) => fmt.date(row.not_after) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
          ]}
        />
      </QueryBoundary>

      <CertificateDetailDrawer
        certificate={selected}
        pkiRoots={pkiRoots.data || []}
        loading={selectedDetail.isLoading}
        open={Boolean(selectedId)}
        onClose={() => setSelectedId('')}
      />
      <ImportCertificateModal open={importOpen} onClose={() => setImportOpen(false)} onDone={(certificate) => { setSelectedId(certificate.id); setNotice(t('certificates.importDone')); }} />
      <SelfSignedModal open={selfSignedOpen} onClose={() => setSelfSignedOpen(false)} onDone={(certificate) => { setSelectedId(certificate.id); setNotice(t('certificates.selfSignedDone')); }} />
      <AuthorityModal open={authorityOpen} onClose={() => setAuthorityOpen(false)} onDone={(certificate) => { setSelectedId(certificate.id); setNotice(t('certificates.authorityDone')); }} />
      <IssueCertificateModal open={issueOpen} authorities={authorities} onClose={() => setIssueOpen(false)} onDone={(certificate) => { setSelectedId(certificate.id); setNotice(t('certificates.issueDone')); }} />
      <PkiRootModal open={pkiRootOpen} onClose={() => setPkiRootOpen(false)} onDone={(root) => setNotice(t('certificates.pkiRootDone', { root: `${root.service_code || 'service'}/${root.pki_profile || 'default'}` }))} />
      <CertificateConfirmDialog action={confirm} busy={busy} onClose={() => setConfirm(null)} onConfirm={() => void runConfirmed()} />
    </PageScaffold>
  );
}

function CertificateDetailDrawer({ certificate, pkiRoots, loading, open, onClose }: { certificate: Certificate | null; pkiRoots: PkiRoot[]; loading: boolean; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <Drawer title={certificate ? certificateLabel(certificate) : t('certificates.certificate')} open={open} onClose={onClose}>
      {loading ? <p>{t('common.loading')}</p> : null}
      {certificate ? (
        <div className="page-stack">
          <Toolbar>
            <StatusBadge status={certificate.status} />
            <Badge>{text(certificate.kind)}</Badge>
            {certificate.is_default ? <Badge>{t('certificates.default')}</Badge> : null}
            {hasPrivateKey(certificate) ? <Badge>{t('certificates.privateKeyStored')}</Badge> : <Badge>{t('certificates.noPrivateKey')}</Badge>}
          </Toolbar>
          <Card>
            <CardBody>
              <div className="definition-grid">
                <span>{t('common.id')}</span><strong>{certificate.id}</strong>
                <span>{t('common.name')}</span><strong>{text(certificate.name)}</strong>
                <span>{t('common.description')}</span><strong>{text(certificate.description)}</strong>
                <span>{t('certificates.commonName')}</span><strong>{text(certificate.common_name)}</strong>
                <span>{t('certificates.sans')}</span><strong>{certificate.sans?.length ? certificate.sans.join(', ') : 'n/a'}</strong>
                <span>{t('certificates.issuer')}</span><strong>{text(certificate.issuer_name)}</strong>
                <span>{t('certificates.parent')}</span><strong>{shortID(certificate.parent_certificate_id)}</strong>
                <span>{t('certificates.usage')}</span><strong>{certificateUsage(certificate)}</strong>
                <span>{t('certificates.notBefore')}</span><strong>{fmt.date(certificate.not_before)}</strong>
                <span>{t('certificates.expiry')}</span><strong>{fmt.date(certificate.not_after)}</strong>
                <span>{t('common.created')}</span><strong>{fmt.date(certificate.created_at)}</strong>
                <span>{t('common.updated')}</span><strong>{fmt.date(certificate.updated_at)}</strong>
              </div>
            </CardBody>
          </Card>
          <DataTable
            title={t('certificates.serviceRoots')}
            rows={pkiRoots}
            columns={[
              { key: 'service', header: t('certificates.serviceCode'), render: (row) => <code>{text(row.service_code)}</code> },
              { key: 'profile', header: t('certificates.pkiProfile'), render: (row) => <code>{text(row.pki_profile)}</code> },
              { key: 'cn', header: t('certificates.commonName'), render: (row) => text(row.common_name) },
              { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            ]}
          />
        </div>
      ) : null}
    </Drawer>
  );
}

function ImportCertificateModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (certificate: Certificate) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const preview = usePreviewCertificateImport();
  const importCertificate = useImportCertificate();
  const [form, setForm] = useState<CertificateImportInput>(defaultImportForm);
  const [previewResult, setPreviewResult] = useState<CertificateImportPreview | null>(null);
  const [previewHash, setPreviewHash] = useState('');
  const [notice, setNotice] = useState('');
  const currentHash = previewFingerprint(form);
  const previewCurrent = Boolean(previewResult && previewHash === currentHash);

  const reset = () => {
    setForm(defaultImportForm);
    setPreviewResult(null);
    setPreviewHash('');
    setNotice('');
  };
  const close = () => {
    reset();
    onClose();
  };
  const patchForm = (patch: Partial<CertificateImportInput>) => {
    setForm((current) => ({ ...current, ...patch }));
  };
  const runPreview = async () => {
    setNotice('');
    try {
      const result = await preview.mutateAsync(form);
      setPreviewResult(result);
      setPreviewHash(currentHash);
    } catch (error) {
      setPreviewResult(null);
      setPreviewHash('');
      setNotice(formatAPIError(error));
    }
  };
  const submit = async () => {
    if (!previewCurrent) return;
    setNotice('');
    try {
      const certificate = await importCertificate.mutateAsync(form);
      reset();
      onClose();
      onDone(certificate);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <Modal title={t('certificates.import')} open={open} onClose={close}>
      <div className="page-stack">
        {notice ? <div role="alert">{notice}</div> : null}
        <FormGrid>
          <FormField label={t('common.name')}>
            <TextField value={form.name || ''} onChange={(event) => patchForm({ name: event.target.value })} />
          </FormField>
          <FormField label={t('common.description')}>
            <TextField value={form.description || ''} onChange={(event) => patchForm({ description: event.target.value })} />
          </FormField>
          <FormField label={t('certificates.certificatePemFile')}>
            <TextField type="file" accept=".pem,.crt,.cer,text/plain" onChange={(event) => { void readFileText(event.currentTarget.files?.[0]).then((certificate) => patchForm({ certificate })); }} />
          </FormField>
          <FormField label={t('certificates.privateKeyFile')}>
            <TextField type="file" accept=".pem,.key,text/plain" onChange={(event) => { void readFileText(event.currentTarget.files?.[0]).then((privateKey) => patchForm({ private_key: privateKey })); }} />
          </FormField>
          <FormField label={t('certificates.certificatePem')} full>
            <Textarea value={form.certificate} rows={7} onChange={(event) => patchForm({ certificate: event.target.value })} />
          </FormField>
          <FormField label={t('certificates.privateKey')} full>
            <Textarea value={form.private_key} rows={7} onChange={(event) => patchForm({ private_key: event.target.value })} />
          </FormField>
          <FormField label={t('certificates.chainPemFile')}>
            <TextField type="file" accept=".pem,.crt,.cer,text/plain" onChange={(event) => { void readFileText(event.currentTarget.files?.[0]).then((chain) => patchForm({ chain })); }} />
          </FormField>
          <FormField label={t('certificates.chainPem')} full>
            <Textarea value={form.chain || ''} rows={4} onChange={(event) => patchForm({ chain: event.target.value })} />
          </FormField>
        </FormGrid>
        <label className="toolbar">
          <input type="checkbox" checked={Boolean(form.is_default)} onChange={(event) => patchForm({ is_default: event.target.checked })} />
          <span>{t('certificates.makeDefault')}</span>
        </label>
        {previewResult ? (
          <Card>
            <CardBody>
              <div className="definition-grid">
                <span>{t('certificates.previewState')}</span><strong>{previewCurrent ? t('certificates.previewCurrent') : t('certificates.previewStale')}</strong>
                <span>{t('certificates.commonName')}</span><strong>{text(previewResult.common_name)}</strong>
                <span>{t('certificates.issuer')}</span><strong>{text(previewResult.issuer_name)}</strong>
                <span>{t('certificates.sans')}</span><strong>{previewResult.sans?.length ? previewResult.sans.join(', ') : 'n/a'}</strong>
                <span>{t('certificates.keyPairValid')}</span><strong>{previewResult.key_pair_valid ? t('common.yes') : t('common.no')}</strong>
                <span>{t('certificates.privateKeyType')}</span><strong>{text(previewResult.private_key_type)}</strong>
                <span>{t('certificates.expiry')}</span><strong>{fmt.date(previewResult.not_after)}</strong>
              </div>
            </CardBody>
          </Card>
        ) : null}
        <Toolbar>
          <Button variant="primary" disabled={!form.certificate || !form.private_key || preview.isPending} onClick={() => void runPreview()}>{t('common.preview')}</Button>
          <Button variant="primary" disabled={!previewCurrent || importCertificate.isPending} onClick={() => void submit()}>{t('certificates.import')}</Button>
          <Button onClick={close}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </Modal>
  );
}

function SelfSignedModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (certificate: Certificate) => void }) {
  const { t } = useTranslation();
  const mutation = useCreateSelfSignedCertificate();
  const [form, setForm] = useState({ ...defaultCertForm, dns_names_text: '' });
  const [notice, setNotice] = useState('');
  const reset = () => {
    setForm({ ...defaultCertForm, dns_names_text: '' });
    setNotice('');
  };
  const close = () => {
    reset();
    onClose();
  };
  const submit = async () => {
    setNotice('');
    try {
      const certificate = await mutation.mutateAsync({
        name: form.name,
        description: form.description,
        common_name: form.common_name,
        dns_names: splitNames(form.dns_names_text),
        valid_days: normalizeDays(form.valid_days, 365),
        is_default: form.is_default,
      });
      reset();
      onClose();
      onDone(certificate);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  return (
    <Modal title={t('certificates.selfSigned')} open={open} onClose={close}>
      <CertificateCreateForm form={form} notice={notice} busy={mutation.isPending} submitLabel={t('certificates.selfSigned')} onSubmit={() => void submit()} onCancel={close} onChange={(patch) => setForm((current) => ({ ...current, ...patch }))} />
    </Modal>
  );
}

function AuthorityModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (certificate: Certificate) => void }) {
  const { t } = useTranslation();
  const mutation = useCreateManagedCertificateAuthority();
  const [form, setForm] = useState(defaultAuthorityForm);
  const [notice, setNotice] = useState('');
  const reset = () => {
    setForm(defaultAuthorityForm);
    setNotice('');
  };
  const close = () => {
    reset();
    onClose();
  };
  const submit = async () => {
    setNotice('');
    try {
      const certificate = await mutation.mutateAsync({
        name: form.name,
        description: form.description,
        common_name: form.common_name,
        valid_days: normalizeDays(form.valid_days, 3650),
      });
      reset();
      onClose();
      onDone(certificate);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  return (
    <Modal title={t('certificates.managedCa')} open={open} onClose={close}>
      <div className="page-stack">
        {notice ? <div role="alert">{notice}</div> : null}
        <FormGrid>
          <FormField label={t('common.name')}>
            <TextField value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} />
          </FormField>
          <FormField label={t('common.description')}>
            <TextField value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.commonName')}>
            <TextField value={form.common_name} onChange={(event) => setForm((current) => ({ ...current, common_name: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.validDays')}>
            <TextField type="number" min={1} value={form.valid_days} onChange={(event) => setForm((current) => ({ ...current, valid_days: normalizeDays(event.target.value, 3650) }))} />
          </FormField>
        </FormGrid>
        <Toolbar>
          <Button variant="primary" disabled={!form.common_name || mutation.isPending} onClick={() => void submit()}>{t('certificates.managedCa')}</Button>
          <Button onClick={close}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </Modal>
  );
}

function IssueCertificateModal({ open, authorities, onClose, onDone }: { open: boolean; authorities: Certificate[]; onClose: () => void; onDone: (certificate: Certificate) => void }) {
  const { t } = useTranslation();
  const mutation = useIssueCertificate();
  const [form, setForm] = useState<CertificateIssueInput & { dns_names_text: string }>({
    authority_certificate_id: '',
    name: '',
    description: '',
    common_name: '',
    dns_names: [],
    dns_names_text: '',
    valid_days: 365,
    is_default: false,
  });
  const [notice, setNotice] = useState('');
  const reset = () => {
    setForm({ authority_certificate_id: '', name: '', description: '', common_name: '', dns_names: [], dns_names_text: '', valid_days: 365, is_default: false });
    setNotice('');
  };
  const close = () => {
    reset();
    onClose();
  };
  const submit = async () => {
    setNotice('');
    try {
      const certificate = await mutation.mutateAsync({
        authority_certificate_id: form.authority_certificate_id,
        name: form.name,
        description: form.description,
        common_name: form.common_name,
        dns_names: splitNames(form.dns_names_text),
        valid_days: normalizeDays(form.valid_days, 365),
        is_default: form.is_default,
      });
      reset();
      onClose();
      onDone(certificate);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  return (
    <Modal title={t('certificates.issue')} open={open} onClose={close}>
      <div className="page-stack">
        {!authorities.length ? <div role="status">{t('certificates.noAuthorities')}</div> : null}
        {notice ? <div role="alert">{notice}</div> : null}
        <FormGrid>
          <FormField label={t('certificates.authority')}>
            <Select value={form.authority_certificate_id} onChange={(event) => setForm((current) => ({ ...current, authority_certificate_id: event.target.value }))}>
              <option value="">{t('common.none')}</option>
              {authorities.map((authority) => <option key={authority.id} value={authority.id}>{certificateLabel(authority)}</option>)}
            </Select>
          </FormField>
          <FormField label={t('common.name')}>
            <TextField value={form.name || ''} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} />
          </FormField>
          <FormField label={t('common.description')}>
            <TextField value={form.description || ''} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.commonName')}>
            <TextField value={form.common_name} onChange={(event) => setForm((current) => ({ ...current, common_name: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.dnsNames')} full>
            <Textarea value={form.dns_names_text} rows={3} onChange={(event) => setForm((current) => ({ ...current, dns_names_text: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.validDays')}>
            <TextField type="number" min={1} value={form.valid_days} onChange={(event) => setForm((current) => ({ ...current, valid_days: normalizeDays(event.target.value, 365) }))} />
          </FormField>
        </FormGrid>
        <label className="toolbar">
          <input type="checkbox" checked={Boolean(form.is_default)} onChange={(event) => setForm((current) => ({ ...current, is_default: event.target.checked }))} />
          <span>{t('certificates.makeDefault')}</span>
        </label>
        <Toolbar>
          <Button variant="primary" disabled={!form.authority_certificate_id || !form.common_name || mutation.isPending} onClick={() => void submit()}>{t('certificates.issue')}</Button>
          <Button onClick={close}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </Modal>
  );
}

function CertificateCreateForm({ form, notice, busy, submitLabel, onSubmit, onCancel, onChange }: {
  form: CertificateCreateInput & { dns_names_text: string };
  notice: string;
  busy: boolean;
  submitLabel: string;
  onSubmit: () => void;
  onCancel: () => void;
  onChange: (patch: Partial<CertificateCreateInput & { dns_names_text: string }>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="page-stack">
      {notice ? <div role="alert">{notice}</div> : null}
      <FormGrid>
        <FormField label={t('common.name')}>
          <TextField value={form.name || ''} onChange={(event) => onChange({ name: event.target.value })} />
        </FormField>
        <FormField label={t('common.description')}>
          <TextField value={form.description || ''} onChange={(event) => onChange({ description: event.target.value })} />
        </FormField>
        <FormField label={t('certificates.commonName')}>
          <TextField value={form.common_name} onChange={(event) => onChange({ common_name: event.target.value })} />
        </FormField>
        <FormField label={t('certificates.validDays')}>
          <TextField type="number" min={1} value={form.valid_days} onChange={(event) => onChange({ valid_days: normalizeDays(event.target.value, 365) })} />
        </FormField>
        <FormField label={t('certificates.dnsNames')} full>
          <Textarea value={form.dns_names_text} rows={3} onChange={(event) => onChange({ dns_names_text: event.target.value })} />
        </FormField>
      </FormGrid>
      <label className="toolbar">
        <input type="checkbox" checked={Boolean(form.is_default)} onChange={(event) => onChange({ is_default: event.target.checked })} />
        <span>{t('certificates.makeDefault')}</span>
      </label>
      <Toolbar>
        <Button variant="primary" disabled={!form.common_name || busy} onClick={onSubmit}>{submitLabel}</Button>
        <Button onClick={onCancel}>{t('common.cancel')}</Button>
      </Toolbar>
    </div>
  );
}

function PkiRootModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (root: PkiRoot) => void }) {
  const { t } = useTranslation();
  const mutation = useCreatePkiRoot();
  const [form, setForm] = useState(defaultPkiRootForm);
  const [notice, setNotice] = useState('');
  const reset = () => {
    setForm(defaultPkiRootForm);
    setNotice('');
  };
  const close = () => {
    reset();
    onClose();
  };
  const submit = async () => {
    setNotice('');
    try {
      const root = await mutation.mutateAsync({
        service_code: form.service_code,
        pki_profile: form.pki_profile || 'default',
        common_name: form.common_name,
        valid_days: normalizeDays(form.valid_days, 10950),
      });
      reset();
      onClose();
      onDone(root);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  return (
    <Modal title={t('certificates.createPkiRoot')} open={open} onClose={close}>
      <div className="page-stack">
        {notice ? <div role="alert">{notice}</div> : null}
        <FormGrid>
          <FormField label={t('certificates.serviceCode')}>
            <TextField value={form.service_code} onChange={(event) => setForm((current) => ({ ...current, service_code: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.pkiProfile')}>
            <TextField value={form.pki_profile || ''} onChange={(event) => setForm((current) => ({ ...current, pki_profile: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.commonName')}>
            <TextField value={form.common_name || ''} onChange={(event) => setForm((current) => ({ ...current, common_name: event.target.value }))} />
          </FormField>
          <FormField label={t('certificates.validDays')}>
            <TextField type="number" min={1} value={form.valid_days} onChange={(event) => setForm((current) => ({ ...current, valid_days: normalizeDays(event.target.value, 10950) }))} />
          </FormField>
        </FormGrid>
        <Toolbar>
          <Button variant="primary" disabled={!form.service_code || mutation.isPending} onClick={() => void submit()}>{t('certificates.createPkiRoot')}</Button>
          <Button onClick={close}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </Modal>
  );
}

function CertificateConfirmDialog({ action, busy, onClose, onConfirm }: { action: ConfirmAction | null; busy: boolean; onClose: () => void; onConfirm: () => void }) {
  const { t } = useTranslation();
  return (
    <ConfirmDialog title={action ? t(`certificates.confirm.${action.type}.title`) : t('certificates.confirm.title')} open={Boolean(action)} onClose={onClose}>
      {action ? (
        <div className="page-stack">
          <p>{t(`certificates.confirm.${action.type}.body`, { name: certificateLabel(action.certificate) })}</p>
          {action.type === 'delete' ? <Badge>{t('certificates.confirm.delete.cascade')}</Badge> : null}
          <Toolbar>
            <Button variant={action.type === 'default' ? 'primary' : 'danger'} disabled={busy} onClick={onConfirm}>{t('common.apply')}</Button>
            <Button onClick={onClose}>{t('common.cancel')}</Button>
          </Toolbar>
        </div>
      ) : null}
    </ConfirmDialog>
  );
}
