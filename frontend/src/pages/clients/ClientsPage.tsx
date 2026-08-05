import { Ban, Download, Eraser, Hammer, KeyRound, LinkIcon, Mail, Plus, RefreshCw, RotateCw, Route, Search, ShieldCheck, Trash2, UserCheck, UserMinus, UserPlus } from 'lucide-react';
import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { APIError } from '../../shared/api/client';
import { getClientArtifactDownload } from '../../shared/api/endpoints';
import type { ClientAccess, ClientAccessDeleteResult, ClientAccessGroup, ClientAccessGroupMembershipResult, ClientAccessRevokeResult, ClientAccessRotationResult, ClientAccount, ClientArtifact, ClientArtifactBuildResult, ClientConfigCleanupResult, ClientDeliveryHistoryItem, ClientDetail, ClientEmailDeliveryResult, ClientRoute, ClientRouteInput, ClientServiceAccess, ClientShareLink, ClientSubscription, ClientUpdateInput, Job, OneTimeSecretDisplay } from '../../shared/api/types';
import {
  useApplySingleClientAccessGroupAssignment,
  useBuildClientArtifact,
  useClientAccessGroups,
  useClientAccessOverview,
  useClientAccesses,
  useClientArtifacts,
  useClientDeliveryHistory,
  useCleanupClientConfigs,
  useClientShareLinks,
  useClientSubscriptions,
  useClientDetail,
  useClientRoutes,
  useClients,
  useCreateClientShareLink,
  useCreateClientSubscription,
  useCreateClient,
  useCreateClientRoute,
  useDeleteClientAccess,
  useDeleteClient,
  useDeleteClientArtifact,
  useDeleteClientRoute,
  useRevokeClientAccess,
  useJobs,
  usePreviewSingleClientAccessGroupAssignment,
  useRevokeClientShareLink,
  useRevokeClientSubscription,
  useRemoveClientAccessGroupMembership,
  useRevokeClient,
  useRotateClientAccess,
  useRotateClientShareLink,
  useRotateClientSubscription,
  useSendClientArtifactEmail,
  useUpdateClient,
  useUpdateClientRoute,
  useUpdateClientStatus,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, Checkbox, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Modal, OneTimeSecretPanel, Select, StatusBadge, TextField, Textarea, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type ClientTab = 'overview' | 'access' | 'routes' | 'maintenance' | 'artifacts' | 'delivery' | 'jobs';
type ConfirmAction =
  | { type: 'suspend' | 'activate' | 'revoke' | 'delete'; client: ClientDetail }
  | { type: 'remove-vless'; client: ClientDetail; group: ClientAccessGroup }
  | { type: 'delete-artifact'; client: ClientDetail; artifact: ClientArtifact };

const statusOptions = ['all', 'active', 'suspended', 'revoked', 'expired'];
const artifactTypes = ['all', 'zip_bundle', 'vless_url', 'ovpn', 'wg_conf', 'mtproto_url', 'http_proxy_bundle', 'ss_url', 'ipsec_bundle'];
const routeActions = ['allow', 'deny'];
const routeDestinationTypes = ['cidr', 'endpoint', 'dns', 'service'];
const routeProtocols = ['any', 'tcp', 'udp', 'icmp'];
const rotationDrivers = [
  { value: 'xray-core', label: 'Xray / VLESS' },
  { value: 'openvpn', label: 'OpenVPN' },
  { value: 'wireguard', label: 'WireGuard' },
  { value: 'mtproto', label: 'MTProto' },
  { value: 'ipsec', label: 'IPsec' },
  { value: 'http_proxy', label: 'HTTP proxy' },
  { value: 'shadowsocks', label: 'Shadowsocks' },
] as const;
type RotationDriver = typeof rotationDrivers[number]['value'];

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

function normalizeDriver(value: unknown): RotationDriver | '' {
  const normalized = String(value || '').trim().toLowerCase().replace(/_/g, '-');
  if (normalized === 'vless' || normalized === 'xray' || normalized === 'xray-core') return 'xray-core';
  if (normalized === 'http-proxy') return 'http_proxy';
  return rotationDrivers.some((driver) => driver.value === normalized) ? normalized as RotationDriver : '';
}

function accessDriverHint(access?: ClientAccess | ClientServiceAccess | null): RotationDriver | '' {
  if (!access) return '';
  return normalizeDriver(
    access.metadata?.service_code ||
    access.metadata?.driver ||
    access.metadata?.runtime_driver ||
    access.policy?.service_code ||
    access.policy?.driver,
  );
}

function redactedAccessIdentity(access: ClientAccess | ClientServiceAccess): string {
  const metadata = access.metadata || {};
  const hasSecretLikeIdentity = Boolean(
    metadata.xray_uuid ||
    metadata.uuid ||
    metadata.password ||
    metadata.secret ||
    metadata.private_key ||
    metadata.token,
  );
  if (hasSecretLikeIdentity) return 'redacted';
  return shortID(access.id);
}

function oneTimeSecretFromResult(result: ClientAccessRotationResult | null): OneTimeSecretDisplay | null {
  const candidate = result?.one_time_secret;
  return candidate?.value ? candidate : null;
}

function queuedJobCount(result?: Pick<ClientAccessDeleteResult, 'instance_apply_jobs_queued' | 'route_policy_jobs_queued'> | null): number {
  return Number(result?.instance_apply_jobs_queued || 0) + Number(result?.route_policy_jobs_queued || 0);
}

function toDateTimeLocal(value?: string | null): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function fromDateTimeLocal(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const date = new Date(trimmed);
  return Number.isNaN(date.getTime()) ? trimmed : date.toISOString();
}

function routeInputFromRoute(route: ClientRoute): ClientRouteInput {
  return {
    service_access_id: route.service_access_id || '',
    instance_id: route.service_access_id ? '' : route.instance_id || '',
    node_id: route.service_access_id || route.instance_id ? '' : route.node_id || '',
    name: route.name || route.destination || route.id,
    status: route.status || 'active',
    action: route.action || 'allow',
    destination_type: route.destination_type || 'cidr',
    destination: route.destination || '',
    protocol: route.protocol || 'any',
    ports: route.ports || '*',
    description: route.description || '',
    policy: route.policy,
    metadata: route.metadata,
  };
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
  const [deliveryArtifactId, setDeliveryArtifactId] = useState('');

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
        deliveryArtifactId={deliveryArtifactId}
        onOpenDelivery={(artifactId) => {
          setDeliveryArtifactId(artifactId);
          setActiveTab('delivery');
        }}
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

function ClientDetailDrawer({
  clientId,
  activeTab,
  deliveryArtifactId,
  onTabChange,
  onOpenDelivery,
  onClose,
}: {
  clientId: string;
  activeTab: ClientTab;
  deliveryArtifactId: string;
  onTabChange: (tab: ClientTab) => void;
  onOpenDelivery: (artifactId: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useClientDetail(clientId);
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);

  return (
    <Drawer title={client.data ? clientLabel(client.data) : t('clients.core.clientDetail')} open={Boolean(clientId)} onClose={onClose}>
      <QueryBoundary isLoading={client.isLoading} isError={client.isError} error={client.error} refetch={() => void client.refetch()}>
        {client.data ? (
          <div className="page-stack">
            <div className="tabs" role="tablist" aria-label={t('clients.core.detailTabs')}>
              {(['overview', 'access', 'routes', 'maintenance', 'artifacts', 'delivery', 'jobs'] as ClientTab[]).map((tab) => (
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
            {activeTab === 'routes' ? <RoutesTab client={client.data} /> : null}
            {activeTab === 'maintenance' ? <MaintenanceTab client={client.data} /> : null}
            {activeTab === 'artifacts' ? <ArtifactsTab client={client.data} onConfirm={setConfirm} onOpenDelivery={onOpenDelivery} /> : null}
            {activeTab === 'delivery' ? <DeliveryTab client={client.data} initialArtifactId={deliveryArtifactId} /> : null}
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
  const [editOpen, setEditOpen] = useState(false);
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
              <Button onClick={() => setEditOpen(true)}>{t('common.edit')}</Button>
              <Button icon={<UserCheck size={16} />} disabled={!canActivate || updateStatus.isPending} onClick={() => void runStatus('active')}>{t('clients.core.activate')}</Button>
              <Button icon={<UserMinus size={16} />} disabled={!canSuspend || updateStatus.isPending} onClick={() => void runStatus('suspended')}>{t('clients.core.suspend')}</Button>
              <Button disabled title={t('clients.core.noDisableEndpoint')}>{t('clients.core.disable')}</Button>
              <Button variant="danger" icon={<ShieldCheck size={16} />} onClick={() => onConfirm({ type: 'revoke', client })}>{t('clients.core.revoke')}</Button>
              <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => onConfirm({ type: 'delete', client })}>{t('common.delete')}</Button>
            </Toolbar>
            {notice ? <div role="status">{notice}</div> : null}
            {updateStatus.error ? <div role="alert" className="error-state-inline">{formatAPIError(updateStatus.error)}</div> : null}
          </div>
        </CardBody>
      </Card>
      <EditClientModal key={`${client.id}:${client.updated_at || ''}`} client={client} open={editOpen} onClose={() => setEditOpen(false)} />
    </div>
  );
}

function EditClientModal({ client, open, onClose }: { client: ClientDetail; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const updateClient = useUpdateClient();
  const [form, setForm] = useState({
    display_name: client.display_name || '',
    email: client.email || '',
    notes: client.notes || '',
    expires_at: toDateTimeLocal(client.expires_at),
  });
  const errors = fieldErrors(updateClient.error);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const input: ClientUpdateInput = {
      display_name: form.display_name.trim(),
      email: form.email.trim(),
      notes: form.notes.trim(),
    };
    const nextExpires = fromDateTimeLocal(form.expires_at);
    if (nextExpires || client.expires_at) {
      input.expires_at = nextExpires;
    }
    try {
      await updateClient.mutateAsync({ clientId: client.id, input });
      onClose();
    } catch {
      // React Query exposes the APIError through mutation state for safe rendering.
    }
  };

  return (
    <Modal title={t('clients.core.editClient')} open={open} onClose={onClose}>
      <form className="page-stack" onSubmit={(event) => void submit(event)}>
        <FormGrid>
          <FormField label={t('clients.displayName')}>
            <TextField value={form.display_name} onChange={(event) => setForm({ ...form, display_name: event.target.value })} />
            {errors.display_name ? <small className="error-state-inline">{errors.display_name}</small> : null}
          </FormField>
          <FormField label={t('common.email')}>
            <TextField type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} />
            {errors.email ? <small className="error-state-inline">{errors.email}</small> : null}
          </FormField>
          <FormField label={t('common.expires')}>
            <TextField type="datetime-local" value={form.expires_at} onChange={(event) => setForm({ ...form, expires_at: event.target.value })} />
            {errors.expires_at ? <small className="error-state-inline">{errors.expires_at}</small> : null}
          </FormField>
          <FormField label={t('common.description')} full>
            <Textarea value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} />
          </FormField>
        </FormGrid>
        {updateClient.error ? <div role="alert" className="error-state-inline">{formatAPIError(updateClient.error)}</div> : null}
        <Toolbar>
          <Button variant="primary" type="submit" disabled={updateClient.isPending}>{t('common.save')}</Button>
          <Button type="button" onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </Modal>
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
        { key: 'identity', header: t('clients.core.identity'), render: (row) => redactedAccessIdentity(row) === 'redacted' ? t('clients.core.redactedIdentity') : redactedAccessIdentity(row) },
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

function RoutesTab({ client }: { client: ClientDetail }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const routes = useClientRoutes(client.id);
  const accesses = useClientAccesses(client.id);
  const createRoute = useCreateClientRoute();
  const updateRoute = useUpdateClientRoute();
  const deleteRoute = useDeleteClientRoute();
  const activeAccesses = useMemo(() => (accesses.data || []).filter((access) => access.status !== 'revoked' && access.status !== 'deleted'), [accesses.data]);
  const [form, setForm] = useState<ClientRouteInput>({
    service_access_id: '',
    name: '',
    status: 'active',
    action: 'allow',
    destination_type: 'cidr',
    destination: '',
    protocol: 'any',
    ports: '*',
    description: '',
  });
  const [editRoute, setEditRoute] = useState<ClientRoute | null>(null);
  const [confirmRoute, setConfirmRoute] = useState<ClientRoute | null>(null);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');

  const effectiveAccessId = form.service_access_id || activeAccesses[0]?.id || '';

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setNotice('');
    setError('');
    try {
      await createRoute.mutateAsync({
        clientId: client.id,
        input: {
          ...form,
          service_access_id: effectiveAccessId,
          name: form.name.trim(),
          destination: form.destination.trim(),
          ports: (form.ports || '*').trim(),
          description: form.description?.trim(),
        },
      });
      setNotice(t('clients.routes.created'));
      setForm({ ...form, name: '', destination: '', description: '' });
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const runDelete = async () => {
    if (!confirmRoute) return;
    setNotice('');
    setError('');
    try {
      await deleteRoute.mutateAsync({ clientId: client.id, routeId: confirmRoute.id });
      setNotice(t('clients.routes.deleted'));
      setConfirmRoute(null);
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const busy = createRoute.isPending || updateRoute.isPending || deleteRoute.isPending;

  return (
    <QueryBoundary
      isLoading={routes.isLoading || accesses.isLoading}
      isError={routes.isError || accesses.isError}
      error={routes.error || accesses.error}
      refetch={() => { void routes.refetch(); void accesses.refetch(); }}
    >
      <div className="page-stack">
        {notice ? <div role="status">{notice}</div> : null}
        {error ? <div role="alert" className="error-state-inline">{error}</div> : null}

        <Card>
          <CardBody>
            <form className="page-stack" onSubmit={(event) => void submit(event)}>
              <h3 className="card-title">{t('clients.routes.createRoute')}</h3>
              <FormGrid>
                <FormField label={t('clients.core.serviceAccess')}>
                  <Select value={effectiveAccessId} onChange={(event) => setForm({ ...form, service_access_id: event.target.value })}>
                    {activeAccesses.length ? null : <option value="">{t('clients.routes.noAccessTarget')}</option>}
                    {activeAccesses.map((access) => (
                      <option key={access.id} value={access.id}>{shortID(access.id)} / {shortID(access.instance_id)}</option>
                    ))}
                  </Select>
                </FormField>
                <FormField label={t('common.name')}>
                  <TextField required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
                </FormField>
                <FormField label={t('clients.routes.action')}>
                  <Select value={form.action} onChange={(event) => setForm({ ...form, action: event.target.value })}>
                    {routeActions.map((action) => <option key={action} value={action}>{action}</option>)}
                  </Select>
                </FormField>
                <FormField label={t('clients.routes.destinationType')}>
                  <Select value={form.destination_type} onChange={(event) => setForm({ ...form, destination_type: event.target.value })}>
                    {routeDestinationTypes.map((kind) => <option key={kind} value={kind}>{kind}</option>)}
                  </Select>
                </FormField>
                <FormField label={t('clients.routes.destination')}>
                  <TextField required value={form.destination} onChange={(event) => setForm({ ...form, destination: event.target.value })} placeholder="10.10.0.0/16" />
                </FormField>
                <FormField label={t('clients.routes.protocol')}>
                  <Select value={form.protocol} onChange={(event) => setForm({ ...form, protocol: event.target.value })}>
                    {routeProtocols.map((protocol) => <option key={protocol} value={protocol}>{protocol}</option>)}
                  </Select>
                </FormField>
                <FormField label={t('clients.routes.ports')}>
                  <TextField value={form.ports} onChange={(event) => setForm({ ...form, ports: event.target.value })} />
                </FormField>
                <FormField label={t('common.description')} full>
                  <Textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button variant="primary" type="submit" icon={<Route size={16} />} disabled={!effectiveAccessId || busy}>{t('clients.routes.createRoute')}</Button>
                <Button type="button" icon={<RefreshCw size={16} />} onClick={() => { void routes.refetch(); void accesses.refetch(); }}>{t('common.refresh')}</Button>
              </Toolbar>
              {!effectiveAccessId ? <p className="muted">{t('clients.routes.createRequiresAccess')}</p> : null}
            </form>
          </CardBody>
        </Card>

        <DataTable
          title={t('clients.routes.routes')}
          rows={routes.data || []}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.name || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'target', header: t('clients.routes.target'), render: (row) => <code>{shortID(row.service_access_id || row.instance_id || row.node_id)}</code> },
            { key: 'destination', header: t('clients.routes.destination'), render: (row) => text(row.destination) },
            { key: 'protocol', header: t('clients.routes.protocol'), render: (row) => `${text(row.protocol)} / ${text(row.ports)}` },
            { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at || row.created_at) },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button disabled={busy} onClick={() => setEditRoute(row)}>{t('common.edit')}</Button>
                  <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy} onClick={() => setConfirmRoute(row)}>{t('common.delete')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />

        <ConfirmDialog title={t('clients.routes.deleteRoute')} open={Boolean(confirmRoute)} onClose={() => setConfirmRoute(null)}>
          <div className="page-stack">
            <p>{t('clients.routes.confirmDelete', { route: shortID(confirmRoute?.id), client: clientLabel(client) })}</p>
            <p className="muted">{t('clients.routes.deleteImpact')}</p>
            <Toolbar>
              <Button variant="danger" disabled={busy} onClick={() => void runDelete()}>{t('clients.core.confirm')}</Button>
              <Button onClick={() => setConfirmRoute(null)}>{t('common.cancel')}</Button>
            </Toolbar>
          </div>
        </ConfirmDialog>
        <RouteEditModal
          key={editRoute?.id || 'route-edit-closed'}
          client={client}
          route={editRoute}
          accesses={activeAccesses}
          busy={busy}
          onClose={() => setEditRoute(null)}
          onSave={async (routeID, input) => {
            setNotice('');
            setError('');
            try {
              await updateRoute.mutateAsync({ clientId: client.id, routeId: routeID, input });
              setNotice(t('clients.routes.updated'));
              setEditRoute(null);
            } catch (err) {
              setError(formatAPIError(err));
            }
          }}
        />
      </div>
    </QueryBoundary>
  );
}

function RouteEditModal({
  client,
  route,
  accesses,
  busy,
  onClose,
  onSave,
}: {
  client: ClientDetail;
  route: ClientRoute | null;
  accesses: ClientAccess[];
  busy: boolean;
  onClose: () => void;
  onSave: (routeID: string, input: ClientRouteInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState<ClientRouteInput>(() => routeInputFromRoute(route || { id: '' }));
  if (!route) return null;
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    await onSave(route.id, {
      ...form,
      service_access_id: form.service_access_id || undefined,
      instance_id: form.service_access_id ? undefined : form.instance_id || undefined,
      node_id: form.service_access_id || form.instance_id ? undefined : form.node_id || undefined,
      name: form.name.trim(),
      destination: form.destination.trim(),
      ports: (form.ports || '*').trim(),
      description: form.description?.trim(),
    });
  };
  return (
    <Modal title={t('clients.routes.editRoute')} open={Boolean(route)} onClose={onClose}>
      <form className="page-stack" onSubmit={(event) => void submit(event)}>
        <p className="muted">{t('clients.routes.editImpact', { route: shortID(route.id), client: clientLabel(client) })}</p>
        <FormGrid>
          <FormField label={t('clients.core.serviceAccess')}>
            <Select value={form.service_access_id || ''} onChange={(event) => setForm({ ...form, service_access_id: event.target.value })}>
              <option value="">{t('clients.routes.keepInstanceOrNodeTarget')}</option>
              {accesses.map((access) => (
                <option key={access.id} value={access.id}>{shortID(access.id)} / {shortID(access.instance_id)}</option>
              ))}
            </Select>
          </FormField>
          <FormField label={t('common.name')}>
            <TextField required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />
          </FormField>
          <FormField label={t('common.status')}>
            <Select value={form.status || 'active'} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              {['pending', 'active', 'disabled'].map((status) => <option key={status} value={status}>{status}</option>)}
            </Select>
          </FormField>
          <FormField label={t('clients.routes.action')}>
            <Select value={form.action} onChange={(event) => setForm({ ...form, action: event.target.value })}>
              {routeActions.map((action) => <option key={action} value={action}>{action}</option>)}
            </Select>
          </FormField>
          <FormField label={t('clients.routes.destinationType')}>
            <Select value={form.destination_type} onChange={(event) => setForm({ ...form, destination_type: event.target.value })}>
              {routeDestinationTypes.map((kind) => <option key={kind} value={kind}>{kind}</option>)}
            </Select>
          </FormField>
          <FormField label={t('clients.routes.destination')}>
            <TextField required value={form.destination} onChange={(event) => setForm({ ...form, destination: event.target.value })} />
          </FormField>
          <FormField label={t('clients.routes.protocol')}>
            <Select value={form.protocol || 'any'} onChange={(event) => setForm({ ...form, protocol: event.target.value })}>
              {routeProtocols.map((protocol) => <option key={protocol} value={protocol}>{protocol}</option>)}
            </Select>
          </FormField>
          <FormField label={t('clients.routes.ports')}>
            <TextField value={form.ports || '*'} onChange={(event) => setForm({ ...form, ports: event.target.value })} />
          </FormField>
          <FormField label={t('common.description')} full>
            <Textarea value={form.description || ''} onChange={(event) => setForm({ ...form, description: event.target.value })} />
          </FormField>
        </FormGrid>
        <Toolbar>
          <Button variant="primary" type="submit" disabled={busy}>{t('common.save')}</Button>
          <Button type="button" onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </Modal>
  );
}

type MaintenanceConfirmAction =
  | { type: 'rotate-access'; access: ClientAccess; driver: RotationDriver }
  | { type: 'revoke-access'; access: ClientAccess }
  | { type: 'delete-access'; access: ClientAccess }
  | { type: 'cleanup-configs' };

function MaintenanceTab({ client }: { client: ClientDetail }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const accesses = useClientAccesses(client.id);
  const rotateAccess = useRotateClientAccess();
  const revokeAccess = useRevokeClientAccess();
  const deleteAccess = useDeleteClientAccess();
  const cleanupConfigs = useCleanupClientConfigs();
  const [selectedAccessId, setSelectedAccessId] = useState('');
  const [driver, setDriver] = useState<RotationDriver | ''>('');
  const [confirm, setConfirm] = useState<MaintenanceConfirmAction | null>(null);
  const [rotationResult, setRotationResult] = useState<ClientAccessRotationResult | null>(null);
  const [revokeResult, setRevokeResult] = useState<ClientAccessRevokeResult | null>(null);
  const [deleteResult, setDeleteResult] = useState<ClientAccessDeleteResult | null>(null);
  const [cleanupResult, setCleanupResult] = useState<ClientConfigCleanupResult | null>(null);
  const [oneTimeSecret, setOneTimeSecret] = useState<OneTimeSecretDisplay | null>(null);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');

  const activeAccesses = useMemo(() => (accesses.data || []).filter((access) => access.status !== 'revoked' && access.status !== 'deleted'), [accesses.data]);
  const selectedAccess = activeAccesses.find((access) => access.id === selectedAccessId) || activeAccesses[0] || null;
  const effectiveAccessId = selectedAccess?.id || '';
  const effectiveDriver = driver || accessDriverHint(selectedAccess);
  const busy = rotateAccess.isPending || revokeAccess.isPending || deleteAccess.isPending || cleanupConfigs.isPending;

  const selectAccess = (accessId: string) => {
    const nextAccess = activeAccesses.find((access) => access.id === accessId) || null;
    setSelectedAccessId(accessId);
    setDriver(accessDriverHint(nextAccess));
  };

  const clearResultState = () => {
    setError('');
    setNotice('');
    setRotationResult(null);
    setRevokeResult(null);
    setDeleteResult(null);
    setCleanupResult(null);
  };

  const runConfirmed = async () => {
    if (!confirm) return;
    clearResultState();
    try {
      if (confirm.type === 'rotate-access') {
        const result = await rotateAccess.mutateAsync({ clientId: client.id, accessId: confirm.access.id, input: { driver: confirm.driver } });
        setRotationResult(result);
        setOneTimeSecret(oneTimeSecretFromResult(result));
        setNotice(t('clients.maintenance.rotationQueued'));
      } else if (confirm.type === 'revoke-access') {
        const result = await revokeAccess.mutateAsync({ clientId: client.id, accessId: confirm.access.id });
        setRevokeResult(result);
        setNotice(t('clients.maintenance.accessRevoked'));
      } else if (confirm.type === 'delete-access') {
        const result = await deleteAccess.mutateAsync({ clientId: client.id, accessId: confirm.access.id });
        setDeleteResult(result);
        setNotice(t('clients.maintenance.accessDeleted'));
      } else {
        const result = await cleanupConfigs.mutateAsync({ clientId: client.id });
        setCleanupResult(result);
        setNotice(t('clients.maintenance.configsCleaned'));
      }
      setConfirm(null);
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const openRotateConfirm = () => {
    if (!selectedAccess || !effectiveDriver) return;
    setConfirm({ type: 'rotate-access', access: selectedAccess, driver: effectiveDriver });
  };

  return (
    <QueryBoundary isLoading={accesses.isLoading} isError={accesses.isError} error={accesses.error} refetch={() => void accesses.refetch()}>
      <div className="page-stack">
        {oneTimeSecret ? (
          <OneTimeSecretPanel
            label={oneTimeSecret.label}
            value={oneTimeSecret.value}
            expiresAt={oneTimeSecret.expires_at ? fmt.date(oneTimeSecret.expires_at) : undefined}
            onClose={() => setOneTimeSecret(null)}
          />
        ) : null}
        {notice ? <div role="status">{notice}</div> : null}
        {error ? <div role="alert" className="error-state-inline">{error}</div> : null}

        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.maintenance.accessMaintenance')}</h3>
              <FormGrid>
                <FormField label={t('clients.core.serviceAccess')}>
                  <Select value={effectiveAccessId} onChange={(event) => selectAccess(event.target.value)}>
                    {activeAccesses.length ? null : <option value="">{t('clients.maintenance.noAccesses')}</option>}
                    {activeAccesses.map((access) => (
                      <option key={access.id} value={access.id}>{shortID(access.id)} / {shortID(access.instance_id)}</option>
                    ))}
                  </Select>
                </FormField>
                <FormField label={t('clients.maintenance.rotationDriver')}>
                  <Select value={effectiveDriver} onChange={(event) => setDriver(normalizeDriver(event.target.value))}>
                    <option value="">{t('clients.maintenance.selectDriver')}</option>
                    {rotationDrivers.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
                  </Select>
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button variant="danger" icon={<KeyRound size={16} />} disabled={!selectedAccess || !effectiveDriver || busy} onClick={openRotateConfirm}>{t('clients.maintenance.rotateAccess')}</Button>
                <Button variant="danger" icon={<Ban size={16} />} disabled={!selectedAccess || busy} onClick={() => selectedAccess && setConfirm({ type: 'revoke-access', access: selectedAccess })}>{t('clients.maintenance.revokeAccess')}</Button>
                <Button variant="danger" icon={<Trash2 size={16} />} disabled={!selectedAccess || busy} onClick={() => selectedAccess && setConfirm({ type: 'delete-access', access: selectedAccess })}>{t('clients.maintenance.deleteAccess')}</Button>
                <Button icon={<RefreshCw size={16} />} onClick={() => void accesses.refetch()}>{t('common.refresh')}</Button>
              </Toolbar>
              <p className="muted">{t('clients.maintenance.previewUnavailable')}</p>
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.maintenance.configCleanup')}</h3>
              <p className="muted">{t('clients.maintenance.configCleanupImpact')}</p>
              <Toolbar>
                <Button variant="danger" icon={<Eraser size={16} />} disabled={busy} onClick={() => setConfirm({ type: 'cleanup-configs' })}>{t('clients.maintenance.cleanupConfigs')}</Button>
              </Toolbar>
            </div>
          </CardBody>
        </Card>

        <DataTable
          title={t('clients.core.serviceAccesses')}
          rows={activeAccesses}
          columns={[
            { key: 'id', header: t('common.id'), render: (row) => <code>{shortID(row.id)}</code> },
            { key: 'instance', header: t('clients.core.instance'), render: (row) => <code>{shortID(row.instance_id)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'driver', header: t('clients.maintenance.rotationDriver'), render: (row) => text(accessDriverHint(row) || t('clients.maintenance.driverUnknown')) },
            { key: 'identity', header: t('clients.core.identity'), render: (row) => redactedAccessIdentity(row) === 'redacted' ? t('clients.core.redactedIdentity') : redactedAccessIdentity(row) },
          ]}
        />

        {rotationResult?.id ? (
          <div className="inline-panel">
            <div className="page-stack">
              <h3 className="card-title">{t('clients.maintenance.rotationJob')}</h3>
              <Toolbar>
                <Link to="/operations/jobs"><code>{rotationResult.id}</code></Link>
              </Toolbar>
              <JobStatusPanel jobID={rotationResult.id} />
            </div>
          </div>
        ) : null}

        {revokeResult ? <AccessRevokeResultPanel result={revokeResult} /> : null}
        {deleteResult ? <AccessDeleteResultPanel result={deleteResult} /> : null}
        {cleanupResult ? <ConfigCleanupResultPanel result={cleanupResult} /> : null}

        <MaintenanceConfirmDialog action={confirm} client={client} busy={busy} onConfirm={() => void runConfirmed()} onClose={() => setConfirm(null)} />
      </div>
    </QueryBoundary>
  );
}

function AccessRevokeResultPanel({ result }: { result: ClientAccessRevokeResult }) {
  const { t } = useTranslation();
  const jobsQueued = Number(result.instance_apply_jobs_queued || 0) + Number(result.route_policy_jobs_queued || 0);
  return (
    <div className="inline-panel">
      <div className="definition-grid">
        <span>{t('clients.maintenance.revoked')}</span><strong>{result.revoked ? t('common.yes') : t('common.no')}</strong>
        <span>{t('clients.maintenance.alreadyRevoked')}</span><strong>{result.already_revoked ? t('common.yes') : t('common.no')}</strong>
        <span>{t('clients.maintenance.routesRevoked')}</span><strong>{result.access_routes_revoked || 0}</strong>
        <span>{t('clients.maintenance.shareLinksRevoked')}</span><strong>{result.share_links_revoked || 0}</strong>
        <span>{t('clients.maintenance.jobsQueued')}</span><strong>{jobsQueued}</strong>
      </div>
      {jobsQueued ? <Link to="/operations/jobs">{t('clients.maintenance.openJobs')}</Link> : null}
      {result.queue_errors?.length ? <div role="alert" className="error-state-inline">{result.queue_errors.join('; ')}</div> : null}
    </div>
  );
}

function AccessDeleteResultPanel({ result }: { result: ClientAccessDeleteResult }) {
  const { t } = useTranslation();
  const jobsQueued = queuedJobCount(result);
  return (
    <div className="inline-panel">
      <div className="definition-grid">
        <span>{t('clients.maintenance.deleted')}</span><strong>{result.deleted ? t('common.yes') : t('common.no')}</strong>
        <span>{t('clients.maintenance.routesDeleted')}</span><strong>{result.access_routes_deleted || 0}</strong>
        <span>{t('clients.maintenance.secretRefsDeleted')}</span><strong>{result.secret_refs_deleted || 0}</strong>
        <span>{t('clients.maintenance.jobsQueued')}</span><strong>{jobsQueued}</strong>
      </div>
      {jobsQueued ? <Link to="/operations/jobs">{t('clients.maintenance.openJobs')}</Link> : null}
      {result.queue_errors?.length ? <div role="alert" className="error-state-inline">{result.queue_errors.join('; ')}</div> : null}
    </div>
  );
}

function ConfigCleanupResultPanel({ result }: { result: ClientConfigCleanupResult }) {
  const { t } = useTranslation();
  return (
    <div className="inline-panel">
      <div className="definition-grid">
        <span>{t('clients.maintenance.artifactsDeleted')}</span><strong>{result.artifacts_deleted || 0}</strong>
        <span>{t('clients.maintenance.shareLinksDeleted')}</span><strong>{result.share_links_deleted || 0}</strong>
        <span>{t('clients.maintenance.subscriptionsDeleted')}</span><strong>{result.subscriptions_deleted || 0}</strong>
        <span>{t('clients.maintenance.filesDeleted')}</span><strong>{result.files_deleted || 0}</strong>
      </div>
      {result.file_errors?.length ? <div role="alert" className="error-state-inline">{result.file_errors.join('; ')}</div> : null}
    </div>
  );
}

function MaintenanceConfirmDialog({ action, client, busy, onConfirm, onClose }: { action: MaintenanceConfirmAction | null; client: ClientDetail; busy: boolean; onConfirm: () => void; onClose: () => void }) {
  const { t } = useTranslation();
  if (!action) return null;
  const title = action.type === 'rotate-access'
    ? t('clients.maintenance.rotateAccess')
    : action.type === 'revoke-access'
      ? t('clients.maintenance.revokeAccess')
    : action.type === 'delete-access'
      ? t('clients.maintenance.deleteAccess')
      : t('clients.maintenance.cleanupConfigs');
  const object = action.type === 'cleanup-configs' ? clientLabel(client) : shortID(action.access.id);
  const impact = action.type === 'rotate-access'
    ? t('clients.maintenance.rotateImpact', { access: object, driver: action.driver })
    : action.type === 'revoke-access'
      ? t('clients.maintenance.revokeImpact', { access: object })
    : action.type === 'delete-access'
      ? t('clients.maintenance.deleteImpact', { access: object })
      : t('clients.maintenance.cleanupConfirmImpact', { client: clientLabel(client) });
  return (
    <ConfirmDialog title={title} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{impact}</p>
        <Toolbar>
          <Button variant="danger" disabled={busy} onClick={onConfirm}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}

function ArtifactsTab({ client, onConfirm, onOpenDelivery }: { client: ClientDetail; onConfirm: (action: ConfirmAction) => void; onOpenDelivery: (artifactId: string) => void }) {
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
                  <Button icon={<LinkIcon size={16} />} disabled={row.status !== 'ready'} onClick={() => onOpenDelivery(row.id)}>{t('clients.delivery.createShareLink')}</Button>
                  <Button icon={<Mail size={16} />} disabled={row.status !== 'ready'} onClick={() => onOpenDelivery(row.id)}>{t('clients.delivery.sendEmail')}</Button>
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

type DeliveryConfirmAction =
  | { type: 'revoke-share' | 'rotate-share'; share: ClientShareLink }
  | { type: 'revoke-subscription' | 'rotate-subscription'; subscription: ClientSubscription };

function DeliveryTab({ client, initialArtifactId }: { client: ClientDetail; initialArtifactId: string }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const artifacts = useClientArtifacts(client.id);
  const shareLinks = useClientShareLinks(client.id);
  const subscriptions = useClientSubscriptions(client.id);
  const deliveryHistory = useClientDeliveryHistory(client.id);
  const createShare = useCreateClientShareLink();
  const revokeShare = useRevokeClientShareLink();
  const rotateShare = useRotateClientShareLink();
  const createSubscription = useCreateClientSubscription();
  const rotateSubscription = useRotateClientSubscription();
  const revokeSubscription = useRevokeClientSubscription();
  const sendEmail = useSendClientArtifactEmail();
  const readyArtifacts = useMemo(() => (artifacts.data || []).filter((artifact) => artifact.status === 'ready'), [artifacts.data]);
  const [artifactId, setArtifactId] = useState(initialArtifactId);
  const [shareTTL, setShareTTL] = useState(72);
  const [subscriptionTTL, setSubscriptionTTL] = useState(720);
  const [emailTTL, setEmailTTL] = useState(72);
  const [emailSubject, setEmailSubject] = useState('RTIS MegaVPN access package');
  const [emailMessage, setEmailMessage] = useState('');
  const [emailCreateShare, setEmailCreateShare] = useState(false);
  const [confirm, setConfirm] = useState<DeliveryConfirmAction | null>(null);
  const [oneTimeSecret, setOneTimeSecret] = useState<OneTimeSecretDisplay | null>(null);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [emailResult, setEmailResult] = useState<ClientEmailDeliveryResult | null>(null);

  const artifactCandidate = artifactId || initialArtifactId;
  const effectiveArtifactId = readyArtifacts.some((artifact) => artifact.id === artifactCandidate)
    ? artifactCandidate
    : readyArtifacts[0]?.id || '';
  const selectedArtifact = readyArtifacts.find((artifact) => artifact.id === effectiveArtifactId);

  const clearRequestState = () => {
    setError('');
    setNotice('');
    setEmailResult(null);
  };

  const showSecret = (secret?: OneTimeSecretDisplay) => {
    if (secret?.value) setOneTimeSecret(secret);
  };

  const runCreateShare = async () => {
    clearRequestState();
    try {
      const result = await createShare.mutateAsync({ clientId: client.id, input: { target_id: effectiveArtifactId, ttl_hours: shareTTL } });
      showSecret(result.one_time_secret);
      createShare.reset();
      setNotice(t('clients.delivery.shareCreated'));
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const runCreateSubscription = async () => {
    clearRequestState();
    try {
      const result = await createSubscription.mutateAsync({ clientId: client.id, input: { ttl_hours: subscriptionTTL } });
      showSecret(result.one_time_secret);
      createSubscription.reset();
      setNotice(t('clients.delivery.subscriptionCreated'));
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const runSendEmail = async () => {
    clearRequestState();
    try {
      const result = await sendEmail.mutateAsync({
        clientId: client.id,
        artifactId: effectiveArtifactId,
        input: {
          subject: emailSubject.trim(),
          message: emailMessage.trim(),
          ttl_hours: emailTTL,
          create_share_link: emailCreateShare,
        },
      });
      setEmailResult(result);
      setNotice(t('clients.delivery.emailAccepted'));
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const runConfirmedDeliveryAction = async () => {
    if (!confirm) return;
    clearRequestState();
    try {
      if (confirm.type === 'revoke-share') {
        await revokeShare.mutateAsync({ clientId: client.id, shareId: confirm.share.id });
        setNotice(t('clients.delivery.shareRevoked'));
      } else if (confirm.type === 'rotate-share') {
        const result = await rotateShare.mutateAsync({ clientId: client.id, shareId: confirm.share.id, ttlHours: shareTTL });
        showSecret(result.one_time_secret);
        rotateShare.reset();
        setNotice(t('clients.delivery.shareRotated'));
      } else if (confirm.type === 'revoke-subscription') {
        await revokeSubscription.mutateAsync({ clientId: client.id, subscriptionId: confirm.subscription.id });
        setNotice(t('clients.delivery.subscriptionRevoked'));
      } else if (confirm.type === 'rotate-subscription') {
        const result = await rotateSubscription.mutateAsync({ clientId: client.id, input: { ttl_hours: subscriptionTTL } });
        showSecret(result.one_time_secret);
        rotateSubscription.reset();
        setNotice(t('clients.delivery.subscriptionRotated'));
      }
      setConfirm(null);
    } catch (err) {
      setError(formatAPIError(err));
    }
  };

  const busy = createShare.isPending || revokeShare.isPending || rotateShare.isPending || createSubscription.isPending || rotateSubscription.isPending || revokeSubscription.isPending || sendEmail.isPending;

  return (
    <QueryBoundary
      isLoading={artifacts.isLoading || shareLinks.isLoading || subscriptions.isLoading || deliveryHistory.isLoading}
      isError={artifacts.isError || shareLinks.isError || subscriptions.isError || deliveryHistory.isError}
      error={artifacts.error || shareLinks.error || subscriptions.error || deliveryHistory.error}
      refetch={() => { void artifacts.refetch(); void shareLinks.refetch(); void subscriptions.refetch(); void deliveryHistory.refetch(); }}
    >
      <div className="page-stack">
        {oneTimeSecret ? (
          <OneTimeSecretPanel
            label={oneTimeSecret.label}
            value={oneTimeSecret.value}
            expiresAt={oneTimeSecret.expires_at ? fmt.date(oneTimeSecret.expires_at) : undefined}
            onClose={() => setOneTimeSecret(null)}
          />
        ) : null}
        {notice ? <div role="status">{notice}</div> : null}
        {error ? <div role="alert" className="error-state-inline">{error}</div> : null}

        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.delivery.shareLinks')}</h3>
              <FormGrid>
                <FormField label={t('clients.delivery.artifact')}>
                  <Select value={effectiveArtifactId} onChange={(event) => setArtifactId(event.target.value)}>
                    <option value="">{t('clients.delivery.selectReadyArtifact')}</option>
                    {readyArtifacts.map((artifact) => (
                      <option key={artifact.id} value={artifact.id}>{artifact.artifact_type || artifact.type || artifact.id}</option>
                    ))}
                  </Select>
                </FormField>
                <FormField label={t('clients.delivery.ttlHours')}>
                  <TextField type="number" min={1} max={8760} value={shareTTL} onChange={(event) => setShareTTL(Number(event.target.value) || 0)} />
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button variant="primary" icon={<LinkIcon size={16} />} disabled={!effectiveArtifactId || busy} onClick={() => void runCreateShare()}>{t('clients.delivery.createShareLink')}</Button>
                <Button icon={<RefreshCw size={16} />} onClick={() => { void shareLinks.refetch(); void artifacts.refetch(); }}>{t('common.refresh')}</Button>
              </Toolbar>
              {selectedArtifact ? <p className="muted">{t('clients.delivery.selectedArtifact', { artifact: selectedArtifact.artifact_type || selectedArtifact.id })}</p> : null}
            </div>
          </CardBody>
        </Card>

        <DataTable
          title={t('clients.delivery.shareLinks')}
          rows={shareLinks.data || []}
          columns={[
            { key: 'target', header: t('clients.delivery.artifact'), render: (row) => <code>{shortID(row.target_id)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'hint', header: t('clients.delivery.tokenHint'), render: (row) => text(row.token_hint) },
            { key: 'downloads', header: t('clients.delivery.downloads'), render: (row) => fmt.number(row.download_count || 0) },
            { key: 'expires', header: t('common.expires'), render: (row) => fmt.date(row.expires_at) },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button icon={<RotateCw size={16} />} disabled={row.status !== 'active' || busy} onClick={() => setConfirm({ type: 'rotate-share', share: row })}>{t('clients.delivery.rotate')}</Button>
                  <Button variant="danger" icon={<Trash2 size={16} />} disabled={row.status === 'revoked' || busy} onClick={() => setConfirm({ type: 'revoke-share', share: row })}>{t('clients.delivery.revoke')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />

        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.delivery.subscriptions')}</h3>
              <FormGrid>
                <FormField label={t('clients.delivery.ttlHours')}>
                  <TextField type="number" min={1} max={8760} value={subscriptionTTL} onChange={(event) => setSubscriptionTTL(Number(event.target.value) || 0)} />
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button variant="primary" icon={<LinkIcon size={16} />} disabled={busy} onClick={() => void runCreateSubscription()}>{t('clients.delivery.createSubscription')}</Button>
                <Button icon={<RefreshCw size={16} />} onClick={() => void subscriptions.refetch()}>{t('common.refresh')}</Button>
              </Toolbar>
              <p className="muted">{t('clients.delivery.vlessOnly')}</p>
            </div>
          </CardBody>
        </Card>

        <DataTable
          title={t('clients.delivery.subscriptions')}
          rows={subscriptions.data || []}
          columns={[
            { key: 'id', header: t('common.id'), render: (row) => <code>{shortID(row.id)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'hint', header: t('clients.delivery.tokenHint'), render: (row) => text(row.token_hint) },
            { key: 'downloads', header: t('clients.delivery.downloads'), render: (row) => fmt.number(row.download_count || 0) },
            { key: 'lastUsed', header: t('clients.delivery.lastUsed'), render: (row) => fmt.date(row.last_used_at) },
            { key: 'expires', header: t('common.expires'), render: (row) => fmt.date(row.expires_at) },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button icon={<RotateCw size={16} />} disabled={busy} onClick={() => setConfirm({ type: 'rotate-subscription', subscription: row })}>{t('clients.delivery.rotate')}</Button>
                  <Button variant="danger" icon={<Trash2 size={16} />} disabled={row.status === 'revoked' || busy} onClick={() => setConfirm({ type: 'revoke-subscription', subscription: row })}>{t('clients.delivery.revoke')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />

        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('clients.delivery.emailDelivery')}</h3>
              <p className="muted">{client.email ? t('clients.delivery.emailTarget', { email: client.email }) : t('clients.delivery.noClientEmail')}</p>
              <FormGrid>
                <FormField label={t('clients.delivery.subject')}>
                  <TextField value={emailSubject} onChange={(event) => setEmailSubject(event.target.value)} />
                </FormField>
                <FormField label={t('clients.delivery.ttlHours')}>
                  <TextField type="number" min={1} max={8760} value={emailTTL} onChange={(event) => setEmailTTL(Number(event.target.value) || 0)} />
                </FormField>
                <FormField label={t('clients.delivery.message')} full>
                  <Textarea value={emailMessage} onChange={(event) => setEmailMessage(event.target.value)} />
                </FormField>
              </FormGrid>
              <Checkbox checked={emailCreateShare} onChange={(event) => setEmailCreateShare(event.target.checked)} label={t('clients.delivery.createShareForEmail')} />
              <p className="muted">{t('clients.delivery.emailBackendScope')}</p>
              <Toolbar>
                <Button variant="primary" icon={<Mail size={16} />} disabled={!client.email || busy} onClick={() => void runSendEmail()}>{t('clients.delivery.sendEmail')}</Button>
                <Button icon={<RefreshCw size={16} />} onClick={() => void deliveryHistory.refetch()}>{t('common.refresh')}</Button>
              </Toolbar>
              {emailResult?.delivery ? (
                <div className="inline-panel">
                  <div className="definition-grid">
                    <span>{t('common.status')}</span><strong>{emailResult.status || emailResult.delivery.status}</strong>
                    <span>{t('common.id')}</span><strong>{emailResult.delivery.id}</strong>
                    <span>{t('clients.delivery.artifacts')}</span><strong>{emailResult.delivery.artifact_ids?.length || 0}</strong>
                    <span>{t('clients.delivery.shareLinks')}</span><strong>{emailResult.delivery.share_link_ids?.length || 0}</strong>
                  </div>
                </div>
              ) : null}
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardBody>
            <h3 className="card-title">{t('clients.delivery.history')}</h3>
            <DataTable
              rows={deliveryHistory.data || []}
              columns={[
                { key: 'status', header: t('common.status'), render: (row: ClientDeliveryHistoryItem) => <StatusBadge status={row.status} /> },
                { key: 'channel', header: t('clients.delivery.channel'), render: (row: ClientDeliveryHistoryItem) => text(row.channel || row.delivery_type) },
                { key: 'destination', header: t('clients.delivery.destination'), render: (row: ClientDeliveryHistoryItem) => text(row.destination_hint) },
                { key: 'artifacts', header: t('clients.delivery.artifacts'), render: (row: ClientDeliveryHistoryItem) => fmt.number(row.artifact_count || row.related_artifact_ids?.length || 0) },
                { key: 'shareLinks', header: t('clients.delivery.shareLinks'), render: (row: ClientDeliveryHistoryItem) => fmt.number(row.share_link_count || row.related_share_link_ids?.length || 0) },
                { key: 'created', header: t('common.created'), render: (row: ClientDeliveryHistoryItem) => fmt.date(row.created_at) },
                { key: 'completed', header: t('clients.delivery.completed'), render: (row: ClientDeliveryHistoryItem) => fmt.date(row.completed_at || row.sent_at || row.failed_at) },
                { key: 'error', header: t('common.error'), render: (row: ClientDeliveryHistoryItem) => text(row.safe_error_summary) },
              ]}
            />
          </CardBody>
        </Card>

        <DeliveryConfirmDialog action={confirm} busy={busy} client={client} onConfirm={() => void runConfirmedDeliveryAction()} onClose={() => setConfirm(null)} />
      </div>
    </QueryBoundary>
  );
}

function DeliveryConfirmDialog({ action, client, busy, onConfirm, onClose }: { action: DeliveryConfirmAction | null; client: ClientDetail; busy: boolean; onConfirm: () => void; onClose: () => void }) {
  const { t } = useTranslation();
  if (!action) return null;
  const objectType = action.type.includes('subscription') ? t('clients.delivery.subscription') : t('clients.delivery.shareLink');
  const operation = action.type.includes('rotate') ? t('clients.delivery.rotate') : t('clients.delivery.revoke');
  const objectId = 'share' in action ? action.share.id : action.subscription.id;
  return (
    <ConfirmDialog title={`${operation} ${objectType}`} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{t('clients.delivery.confirmImpact', { operation, objectType, client: clientLabel(client), object: shortID(objectId) })}</p>
        <Toolbar>
          <Button variant="danger" disabled={busy} onClick={onConfirm}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
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
