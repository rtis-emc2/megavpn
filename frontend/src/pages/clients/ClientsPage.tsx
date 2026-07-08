import { Download, Hammer, Plus, RefreshCw, Search, ShieldCheck, Trash2, UserCheck, UserMinus, UserPlus } from 'lucide-react';
import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { APIError } from '../../shared/api/client';
import { getClientArtifactDownload } from '../../shared/api/endpoints';
import type { ClientAccessGroup, ClientAccessGroupMembershipResult, ClientAccount, ClientArtifact, ClientArtifactBuildResult, ClientDetail, ClientServiceAccess, Job } from '../../shared/api/types';
import {
  useApplySingleClientAccessGroupAssignment,
  useBuildClientArtifact,
  useClientAccessGroups,
  useClientAccessOverview,
  useClientArtifacts,
  useClientDetail,
  useClients,
  useCreateClient,
  useDeleteClient,
  useDeleteClientArtifact,
  useJobs,
  usePreviewSingleClientAccessGroupAssignment,
  useRemoveClientAccessGroupMembership,
  useRevokeClient,
  useUpdateClientStatus,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Modal, Select, StatusBadge, TextField, Textarea, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type ClientTab = 'overview' | 'access' | 'artifacts' | 'jobs';
type ConfirmAction =
  | { type: 'suspend' | 'activate' | 'revoke' | 'delete'; client: ClientDetail }
  | { type: 'remove-vless'; client: ClientDetail; group: ClientAccessGroup }
  | { type: 'delete-artifact'; client: ClientDetail; artifact: ClientArtifact };

const statusOptions = ['all', 'active', 'suspended', 'revoked', 'expired'];
const artifactTypes = ['all', 'zip_bundle', 'vless_url', 'ovpn', 'wg_conf', 'mtproto_url', 'http_proxy_bundle', 'ss_url', 'ipsec_bundle'];

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) {
    return error instanceof Error ? error.message : 'Unexpected request failure.';
  }
  const prefix = error.status === 401
    ? 'Authentication required'
    : error.status === 403
      ? 'Permission denied'
      : error.status === 409
        ? 'Conflict'
        : error.status === 422 || error.status === 400
          ? 'Validation error'
          : error.status >= 500
            ? 'Backend error'
            : 'Request failed';
  return `${prefix} (${error.status}): ${error.message}`;
}

function fieldErrors(error: unknown): Record<string, string> {
  if (!(error instanceof APIError) || typeof error.payload !== 'object' || error.payload === null) return {};
  const payload = error.payload as { field?: unknown; fields?: unknown; error?: unknown };
  if (payload.fields && typeof payload.fields === 'object') {
    return Object.fromEntries(Object.entries(payload.fields as Record<string, unknown>).map(([key, value]) => [key, String(value)]));
  }
  if (typeof payload.field === 'string') return { [payload.field]: String(payload.error || error.message) };
  return {};
}

function clientLabel(client?: Pick<ClientAccount, 'id' | 'username' | 'display_name' | 'email'> | null): string {
  return text(client?.display_name || client?.username || client?.email || client?.id);
}

function matchClient(client: ClientAccount, search: string, status: string): boolean {
  const normalizedSearch = search.trim().toLowerCase();
  const normalizedStatus = status === 'all' ? '' : status;
  const matchesStatus = !normalizedStatus || client.status === normalizedStatus;
  if (!matchesStatus) return false;
  if (!normalizedSearch) return true;
  return [client.username, client.display_name, client.email, client.id]
    .map((value) => String(value || '').toLowerCase())
    .some((value) => value.includes(normalizedSearch));
}

function resultHasBlockingIssues(result: ClientAccessGroupMembershipResult | null): boolean {
  return Boolean(result?.failed?.length || result?.conflicts?.length);
}

function jobIDsFromMembership(result?: ClientAccessGroupMembershipResult | null): string[] {
  return [
    result?.sync_job_id,
    ...(Array.isArray(result?.apply_job_ids) ? result.apply_job_ids : []),
  ].filter((value): value is string => Boolean(value));
}

function parseInstanceIDs(value: string): string[] {
  return value.split(/[,\s]+/).map((item) => item.trim()).filter(Boolean);
}

export function ClientsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const clients = useClients();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('all');
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedClientID, setSelectedClientID] = useState('');
  const [activeTab, setActiveTab] = useState<ClientTab>('overview');

  const filteredClients = useMemo(() => (clients.data || []).filter((client) => matchClient(client, search, status)), [clients.data, search, status]);

  const columns = [
    { key: 'username', header: t('clients.username'), render: (row: ClientAccount) => <strong>{clientLabel(row)}</strong> },
    { key: 'email', header: t('common.email'), render: (row: ClientAccount) => text(row.email) },
    { key: 'status', header: t('common.status'), render: (row: ClientAccount) => <StatusBadge status={row.status} /> },
    {
      key: 'access',
      header: t('clients.core.accessSummary'),
      render: (row: ClientAccount) => (
        <span>{fmt.number(row.summary?.active_service_access_count || 0)} / {fmt.number(row.summary?.service_access_count || 0)}</span>
      ),
    },
    {
      key: 'artifacts',
      header: t('clients.core.artifactSummary'),
      render: (row: ClientAccount) => (
        <span>{fmt.number(row.summary?.ready_artifact_count || 0)} / {fmt.number(row.summary?.artifact_count || 0)}</span>
      ),
    },
    { key: 'updated', header: t('common.updated'), render: (row: ClientAccount) => fmt.date(row.updated_at || row.created_at) },
    {
      key: 'actions',
      header: t('common.actions'),
      render: (row: ClientAccount) => (
        <Button variant="ghost" onClick={() => { setSelectedClientID(row.id); setActiveTab('overview'); }}>{t('common.open')}</Button>
      ),
    },
  ];

  return (
    <PageScaffold
      title={t('clients.title')}
      subtitle={t('clients.core.workspaceSubtitle')}
      actions={<Button variant="primary" icon={<Plus size={16} />} onClick={() => setCreateOpen(true)}>{t('clients.core.createClient')}</Button>}
    >
      <Card>
        <CardBody>
          <Toolbar>
            <FormField label={t('common.search')}>
              <div className="input-with-icon">
                <Search size={16} aria-hidden="true" />
                <TextField value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('clients.core.searchPlaceholder')} />
              </div>
            </FormField>
            <FormField label={t('common.status')}>
              <Select value={status} onChange={(event) => setStatus(event.target.value)}>
                {statusOptions.map((option) => <option key={option} value={option}>{option === 'all' ? t('common.all') : option}</option>)}
              </Select>
            </FormField>
            <Button icon={<RefreshCw size={16} />} onClick={() => void clients.refetch()}>{t('common.refresh')}</Button>
          </Toolbar>
        </CardBody>
      </Card>

      <QueryBoundary isLoading={clients.isLoading} isError={clients.isError} error={clients.error} refetch={() => void clients.refetch()}>
        <DataTable rows={filteredClients} columns={columns} />
      </QueryBoundary>

      <CreateClientModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(client) => {
          setCreateOpen(false);
          setSelectedClientID(client.id);
          setActiveTab('overview');
        }}
      />

      <ClientDetailDrawer
        clientId={selectedClientID}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        onClose={() => setSelectedClientID('')}
      />
    </PageScaffold>
  );
}

function CreateClientModal({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (client: ClientDetail) => void }) {
  const { t } = useTranslation();
  const createClient = useCreateClient();
  const [form, setForm] = useState({ username: '', display_name: '', email: '', notes: '' });
  const errors = fieldErrors(createClient.error);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    try {
      const created = await createClient.mutateAsync({
        username: form.username.trim(),
        display_name: form.display_name.trim(),
        email: form.email.trim(),
        notes: form.notes.trim(),
        status: 'active',
      });
      setForm({ username: '', display_name: '', email: '', notes: '' });
      onCreated(created);
    } catch {
      // React Query exposes the APIError through mutation state for safe rendering.
    }
  };

  return (
    <Modal title={t('clients.core.createClient')} open={open} onClose={onClose}>
      <form className="page-stack" onSubmit={(event) => void submit(event)}>
        <FormGrid>
          <FormField label={t('clients.username')}>
            <TextField required value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} />
            {errors.username ? <small className="error-state-inline">{errors.username}</small> : null}
          </FormField>
          <FormField label={t('clients.displayName')}>
            <TextField value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} />
          </FormField>
          <FormField label={t('common.email')}>
            <TextField type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} />
            {errors.email ? <small className="error-state-inline">{errors.email}</small> : null}
          </FormField>
          <FormField label={t('common.description')} full>
            <Textarea value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} />
          </FormField>
        </FormGrid>
        {createClient.error ? <div role="alert" className="error-state-inline">{formatAPIError(createClient.error)}</div> : null}
        <Toolbar>
          <Button variant="primary" type="submit" disabled={createClient.isPending}>{t('clients.core.createClient')}</Button>
          <Button type="button" onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </Modal>
  );
}

function ClientDetailDrawer({ clientId, activeTab, onTabChange, onClose }: { clientId: string; activeTab: ClientTab; onTabChange: (tab: ClientTab) => void; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useClientDetail(clientId);
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);

  return (
    <Drawer title={client.data ? clientLabel(client.data) : t('clients.core.clientDetail')} open={Boolean(clientId)} onClose={onClose}>
      <QueryBoundary isLoading={client.isLoading} isError={client.isError} error={client.error} refetch={() => void client.refetch()}>
        {client.data ? (
          <div className="page-stack">
            <div className="tabs" role="tablist" aria-label={t('clients.core.detailTabs')}>
              {(['overview', 'access', 'artifacts', 'jobs'] as ClientTab[]).map((tab) => (
                <button
                  key={tab}
                  className={`tab-link ${activeTab === tab ? 'active' : ''}`.trim()}
                  role="tab"
                  aria-selected={activeTab === tab}
                  type="button"
                  onClick={() => onTabChange(tab)}
                >
                  {t(`clients.core.tabs.${tab}`)}
                </button>
              ))}
            </div>

            {activeTab === 'overview' ? <OverviewTab client={client.data} onConfirm={setConfirm} /> : null}
            {activeTab === 'access' ? <AccessTab client={client.data} onConfirm={setConfirm} /> : null}
            {activeTab === 'artifacts' ? <ArtifactsTab client={client.data} onConfirm={setConfirm} /> : null}
            {activeTab === 'jobs' ? <JobsTab client={client.data} /> : null}

            <ActionConfirmDialog action={confirm} onClose={() => setConfirm(null)} />
          </div>
        ) : null}
      </QueryBoundary>
    </Drawer>
  );
}

function OverviewTab({ client, onConfirm }: { client: ClientDetail; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const updateStatus = useUpdateClientStatus();
  const [notice, setNotice] = useState('');
  const canSuspend = client.status !== 'suspended' && client.status !== 'revoked' && client.status !== 'deleted';
  const canActivate = client.status !== 'active' && client.status !== 'deleted';

  const runStatus = async (status: 'active' | 'suspended') => {
    try {
      const updated = await updateStatus.mutateAsync({ clientId: client.id, input: { status } });
      setNotice(t('clients.core.statusUpdated', { status: updated.status || status }));
    } catch {
      // React Query exposes the APIError through mutation state for safe rendering.
    }
  };

  return (
    <div className="page-stack">
      <div className="metric-grid">
        <Card><CardBody><div className="metric-label">{t('common.status')}</div><div className="metric-value"><StatusBadge status={client.status} /></div></CardBody></Card>
        <Card><CardBody><div className="metric-label">{t('clients.core.accessSummary')}</div><div className="metric-value">{client.summary?.active_service_access_count || 0}/{client.summary?.service_access_count || 0}</div></CardBody></Card>
        <Card><CardBody><div className="metric-label">{t('clients.core.artifactSummary')}</div><div className="metric-value">{client.summary?.ready_artifact_count || 0}/{client.summary?.artifact_count || 0}</div></CardBody></Card>
        <Card><CardBody><div className="metric-label">{t('common.updated')}</div><div className="metric-caption">{fmt.date(client.updated_at || client.created_at)}</div></CardBody></Card>
      </div>
      <Card>
        <CardBody>
          <div className="definition-grid">
            <span>{t('clients.username')}</span><strong>{text(client.username)}</strong>
            <span>{t('clients.displayName')}</span><strong>{text(client.display_name)}</strong>
            <span>{t('common.email')}</span><strong>{text(client.email)}</strong>
            <span>{t('common.created')}</span><strong>{fmt.date(client.created_at)}</strong>
            <span>{t('common.expires')}</span><strong>{fmt.date(client.expires_at)}</strong>
            <span>{t('common.description')}</span><strong>{text(client.notes)}</strong>
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <Button disabled title={t('clients.core.noGenericEditEndpoint')}>{t('common.edit')}</Button>
              <Button icon={<UserCheck size={16} />} disabled={!canActivate || updateStatus.isPending} onClick={() => void runStatus('active')}>{t('clients.core.activate')}</Button>
              <Button icon={<UserMinus size={16} />} disabled={!canSuspend || updateStatus.isPending} onClick={() => void runStatus('suspended')}>{t('clients.core.suspend')}</Button>
              <Button disabled title={t('clients.core.noDisableEndpoint')}>{t('clients.core.disable')}</Button>
              <Button variant="danger" icon={<ShieldCheck size={16} />} onClick={() => onConfirm({ type: 'revoke', client })}>{t('clients.core.revoke')}</Button>
              <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => onConfirm({ type: 'delete', client })}>{t('common.delete')}</Button>
            </Toolbar>
            <p className="muted">{t('clients.core.noGenericEditEndpoint')}</p>
            {notice ? <div role="status">{notice}</div> : null}
            {updateStatus.error ? <div role="alert" className="error-state-inline">{formatAPIError(updateStatus.error)}</div> : null}
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function AccessTab({ client, onConfirm }: { client: ClientDetail; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const overview = useClientAccessOverview(client.id);
  const vlessGroups = useClientAccessGroups('vless');
  const preview = usePreviewSingleClientAccessGroupAssignment();
  const apply = useApplySingleClientAccessGroupAssignment();
  const [targetGroupId, setTargetGroupId] = useState('');
  const [mode, setMode] = useState<'add_only' | 'add_or_move'>('add_only');
  const [previewResult, setPreviewResult] = useState<ClientAccessGroupMembershipResult | null>(null);
  const [previewSignature, setPreviewSignature] = useState('');
  const [applyResult, setApplyResult] = useState<ClientAccessGroupMembershipResult | null>(null);

  const currentSignature = `${client.id}:${targetGroupId}:${mode}`;
  const previewStale = Boolean(previewResult && previewSignature !== currentSignature);
  const applyEnabled = Boolean(previewResult && !previewStale && !resultHasBlockingIssues(previewResult) && targetGroupId);
  const currentVlessGroup = overview.data?.vless_group || null;

  const runPreview = async () => {
    try {
      const result = await preview.mutateAsync({ groupId: targetGroupId, clientId: client.id, mode });
      setPreviewResult(result);
      setPreviewSignature(currentSignature);
      setApplyResult(null);
    } catch {
      // React Query exposes the APIError through mutation state for safe rendering.
    }
  };

  const runApply = async () => {
    try {
      const result = await apply.mutateAsync({ groupId: targetGroupId, clientId: client.id, mode });
      setApplyResult(result);
    } catch {
      // React Query exposes the APIError through mutation state for safe rendering.
    }
  };

  return (
    <QueryBoundary isLoading={overview.isLoading || vlessGroups.isLoading} isError={overview.isError || vlessGroups.isError} error={overview.error || vlessGroups.error} refetch={() => { void overview.refetch(); void vlessGroups.refetch(); }}>
      <div className="page-stack">
        <Toolbar>
          <Badge>{t('clients.core.currentVlessGroup')}</Badge>
          <StatusBadge status={currentVlessGroup?.status || 'unassigned'} />
          <Link to="/clients/groups">{t('clients.openGroups')}</Link>
        </Toolbar>
        <DataTable
          rows={overview.data?.groups || []}
          columns={[
            { key: 'service', header: t('clients.serviceFilter'), render: (row) => <Badge>{text(row.service_code)}</Badge> },
            { key: 'group', header: t('clients.groups.groupKey'), render: (row) => <strong>{text(row.display_name || row.group_key || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'sync', header: t('clients.groups.syncState'), render: (row) => t('clients.core.syncPendingFailed', { pending: row.pending_sync_count || 0, failed: row.failed_sync_count || 0 }) },
            { key: 'mode', header: t('common.scope'), render: (row) => row.service_code === 'vless' ? t('clients.core.fullyConnected') : t('common.readOnly') },
          ]}
        />

        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.core.assignVlessGroup')}</h3>
              <FormGrid>
                <FormField label={t('clients.groups.groupKey')}>
                  <Select value={targetGroupId} onChange={(event) => setTargetGroupId(event.target.value)}>
                    <option value="">{t('clients.core.selectVlessGroup')}</option>
                    {(vlessGroups.data || []).filter((group) => group.status !== 'deleted').map((group) => (
                      <option key={group.id} value={group.id}>{text(group.display_name || group.group_key || group.id)}</option>
                    ))}
                  </Select>
                </FormField>
                <FormField label={t('clients.groups.assignmentMode')}>
                  <Select value={mode} onChange={(event) => setMode(event.target.value as 'add_only' | 'add_or_move')}>
                    <option value="add_only">{t('clients.groups.addOnly')}</option>
                    <option value="add_or_move">{t('clients.groups.addOrMove')}</option>
                  </Select>
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button icon={<Search size={16} />} disabled={!targetGroupId || preview.isPending} onClick={() => void runPreview()}>{t('common.preview')}</Button>
                <Button variant="primary" icon={<UserPlus size={16} />} disabled={!applyEnabled || apply.isPending} onClick={() => void runApply()}>{t('common.apply')}</Button>
                {currentVlessGroup ? (
                  <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => onConfirm({ type: 'remove-vless', client, group: currentVlessGroup })}>{t('clients.core.removeVless')}</Button>
                ) : null}
              </Toolbar>
              <p className="muted">{t('clients.selectionInvalidatesPreview')}</p>
              {previewStale ? <Badge>{t('clients.groups.previewStale')}</Badge> : null}
              {preview.error ? <div role="alert" className="error-state-inline">{formatAPIError(preview.error)}</div> : null}
              {apply.error ? <div role="alert" className="error-state-inline">{formatAPIError(apply.error)}</div> : null}
            </div>
          </CardBody>
        </Card>
        <MembershipResult title={t('clients.groups.previewResult')} result={previewResult} />
        <MembershipResult title={t('clients.groups.applyResult')} result={applyResult} />

        <ServiceAccessList accesses={overview.data?.accesses || []} />
      </div>
    </QueryBoundary>
  );
}

function ServiceAccessList({ accesses }: { accesses: ClientServiceAccess[] }) {
  const { t } = useTranslation();
  return (
    <DataTable
      title={t('clients.core.serviceAccesses')}
      rows={accesses}
      columns={[
        { key: 'id', header: t('common.id'), render: (row) => <code>{shortID(row.id)}</code> },
        { key: 'instance', header: t('clients.core.instance'), render: (row) => <code>{shortID(row.instance_id)}</code> },
        { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
        { key: 'mode', header: t('clients.core.provisionMode'), render: (row) => text(row.provision_mode) },
        { key: 'identity', header: t('clients.core.identity'), render: (row) => text(row.metadata?.xray_uuid || row.metadata?.uuid || shortID(row.id)) },
      ]}
    />
  );
}

function MembershipResult({ title, result }: { title: string; result: ClientAccessGroupMembershipResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  const jobIDs = jobIDsFromMembership(result);
  return (
    <div className="inline-panel">
        <div className="page-stack">
          <h3 className="card-title">{title}</h3>
          <Toolbar>
            <Badge>{t('clients.groups.created')}: {result.created_memberships}</Badge>
            <Badge>{t('clients.groups.moved')}: {result.moved_memberships}</Badge>
            <Badge>{t('clients.groups.skipped')}: {result.skipped_existing}</Badge>
            <Badge>{t('clients.groups.failed')}: {result.failed?.length || 0}</Badge>
          </Toolbar>
          <p>{t('clients.groups.materializedSummary', {
            created: result.materialized_created || 0,
            updated: result.materialized_updated || 0,
            disabled: result.materialized_disabled || 0,
            instances: result.affected_instances || 0,
          })}</p>
          {result.conflicts?.length ? <pre className="code-block">{JSON.stringify(result.conflicts, null, 2)}</pre> : null}
          {result.warnings?.length ? <pre className="code-block">{result.warnings.join('\n')}</pre> : null}
          {jobIDs.length ? (
            <Toolbar>
              {jobIDs.map((jobID) => <Link key={jobID} to="/operations/jobs"><code>{jobID}</code></Link>)}
            </Toolbar>
          ) : null}
        </div>
    </div>
  );
}

function ArtifactsTab({ client, onConfirm }: { client: ClientDetail; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const artifacts = useClientArtifacts(client.id);
  const build = useBuildClientArtifact();
  const [type, setType] = useState('all');
  const [instanceIds, setInstanceIds] = useState('');
  const [buildResult, setBuildResult] = useState<ClientArtifactBuildResult | null>(null);
  const [downloadNotice, setDownloadNotice] = useState('');

  const runBuild = async () => {
    try {
      const result = await build.mutateAsync({ clientId: client.id, input: { type, instance_ids: parseInstanceIDs(instanceIds) } });
      setBuildResult(result);
    } catch {
      // React Query exposes the APIError through mutation state for safe rendering.
    }
  };

  const runDownload = async (artifact: ClientArtifact) => {
    const target = await getClientArtifactDownload(client.id, artifact.id);
    window.open(target.url, '_blank', 'noopener,noreferrer');
    setDownloadNotice(t('clients.core.downloadStarted'));
  };

  return (
    <QueryBoundary isLoading={artifacts.isLoading} isError={artifacts.isError} error={artifacts.error} refetch={() => void artifacts.refetch()}>
      <div className="page-stack">
        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.core.buildArtifact')}</h3>
              <FormGrid>
                <FormField label={t('common.type')}>
                  <Select value={type} onChange={(event) => setType(event.target.value)}>
                    {artifactTypes.map((item) => <option key={item} value={item}>{item}</option>)}
                  </Select>
                </FormField>
                <FormField label={t('clients.core.instanceIds')}>
                  <TextField value={instanceIds} onChange={(event) => setInstanceIds(event.target.value)} placeholder={t('clients.core.instanceIdsPlaceholder')} />
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button variant="primary" icon={<Hammer size={16} />} disabled={build.isPending} onClick={() => void runBuild()}>{t('clients.core.buildArtifact')}</Button>
                <Button icon={<RefreshCw size={16} />} onClick={() => void artifacts.refetch()}>{t('common.refresh')}</Button>
              </Toolbar>
              {build.error ? <div role="alert" className="error-state-inline">{formatAPIError(build.error)}</div> : null}
              {downloadNotice ? <div role="status">{downloadNotice}</div> : null}
            </div>
          </CardBody>
        </Card>
        {buildResult?.job?.id ? <JobStatusPanel jobID={buildResult.job.id} /> : null}

        <DataTable
          rows={artifacts.data || []}
          columns={[
            { key: 'type', header: t('common.type'), render: (row) => text(row.artifact_type || row.type) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'size', header: t('common.size'), render: (row) => fmt.bytes(row.size_bytes) },
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
            { key: 'access', header: t('clients.core.serviceAccess'), render: (row) => <code>{shortID(row.service_access_id)}</code> },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button icon={<Download size={16} />} disabled={row.status !== 'ready'} onClick={() => void runDownload(row)}>{t('common.download')}</Button>
                  <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => onConfirm({ type: 'delete-artifact', client, artifact: row })}>{t('common.delete')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />
      </div>
    </QueryBoundary>
  );
}

function JobsTab({ client }: { client: ClientDetail }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const jobs = useJobs();
  const [watchJobId, setWatchJobId] = useState('');
  const clientJobs = useMemo(() => (jobs.data || []).filter((job) => {
    const payload = job.payload && typeof job.payload === 'object' ? job.payload as Record<string, unknown> : {};
    return job.scope_id === client.id || payload.client_id === client.id;
  }), [client.id, jobs.data]);

  return (
    <QueryBoundary isLoading={jobs.isLoading} isError={jobs.isError} error={jobs.error} refetch={() => void jobs.refetch()}>
      <div className="page-stack">
        <DataTable
          rows={clientJobs}
          columns={[
            { key: 'id', header: t('common.id'), render: (row: Job) => <code>{shortID(row.id)}</code> },
            { key: 'type', header: t('jobs.kind'), render: (row: Job) => text(row.type) },
            { key: 'status', header: t('common.status'), render: (row: Job) => <StatusBadge status={row.status} /> },
            { key: 'updated', header: t('common.updated'), render: (row: Job) => fmt.date(row.updated_at || row.created_at) },
            { key: 'actions', header: t('common.actions'), render: (row: Job) => <Button onClick={() => setWatchJobId(row.id)}>{t('jobs.watch')}</Button> },
          ]}
        />
        {watchJobId ? <JobStatusPanel jobID={watchJobId} /> : null}
      </div>
    </QueryBoundary>
  );
}

function ActionConfirmDialog({ action, onClose }: { action: ConfirmAction | null; onClose: () => void }) {
  const { t } = useTranslation();
  const updateStatus = useUpdateClientStatus();
  const revokeClient = useRevokeClient();
  const deleteClient = useDeleteClient();
  const removeMembership = useRemoveClientAccessGroupMembership();
  const deleteArtifact = useDeleteClientArtifact();
  const [error, setError] = useState('');
  const [jobId, setJobId] = useState('');

  if (!action) return null;

  const run = async () => {
    setError('');
    setJobId('');
    try {
      if (action.type === 'suspend') {
        await updateStatus.mutateAsync({ clientId: action.client.id, input: { status: 'suspended' } });
      } else if (action.type === 'activate') {
        await updateStatus.mutateAsync({ clientId: action.client.id, input: { status: 'active' } });
      } else if (action.type === 'revoke') {
        const job = await revokeClient.mutateAsync(action.client.id);
        setJobId(job.id);
        return;
      } else if (action.type === 'delete') {
        await deleteClient.mutateAsync(action.client.id);
      } else if (action.type === 'remove-vless') {
        await removeMembership.mutateAsync({ groupId: action.group.id, clientId: action.client.id });
      } else if (action.type === 'delete-artifact') {
        await deleteArtifact.mutateAsync({ clientId: action.client.id, artifactId: action.artifact.id });
      }
      onClose();
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const title = action.type === 'delete-artifact'
    ? t('clients.core.deleteArtifact')
    : action.type === 'remove-vless'
      ? t('clients.core.removeVless')
      : action.type === 'delete'
        ? t('common.delete')
        : action.type === 'revoke'
          ? t('clients.core.revoke')
          : action.type === 'activate'
            ? t('clients.core.activate')
            : t('clients.core.suspend');
  const busy = updateStatus.isPending || revokeClient.isPending || deleteClient.isPending || removeMembership.isPending || deleteArtifact.isPending;

  return (
    <ConfirmDialog title={title} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{t('clients.core.confirmAction', { action: title, client: clientLabel(action.client) })}</p>
        {error ? <div role="alert" className="error-state-inline">{error}</div> : null}
        {jobId ? <JobStatusPanel jobID={jobId} /> : null}
        <Toolbar>
          <Button variant="danger" disabled={busy} onClick={() => void run()}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
