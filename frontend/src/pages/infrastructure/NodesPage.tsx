import { Activity, Boxes, CheckCircle2, DownloadCloud, FilePenLine, Fingerprint, KeyRound, PackageCheck, PlusCircle, Play, RefreshCw, RotateCcw, Search, ServerCog, ShieldAlert, ShieldCheck, TerminalSquare, Trash2, Wrench } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import { useAuth } from '../../shared/auth/AuthProvider';
import type {
  APIRecord,
  BootstrapRequest,
  EnrollmentToken,
  HostKeyScanResult,
  Job,
  NodeAccessMethod,
  NodeCapability,
  NodeCapabilityInstallInput,
  NodeCreateInput,
  NodeDetail,
  NodeDiagnostics,
  NodeDiagnosticsAction,
  NodeMutationResult,
  NodeServiceDiscovery,
  NodeServiceInstaller,
} from '../../shared/api/types';
import { hasPermission } from '../../shared/permissions/permissions';
import {
  useAcceptNodeHostKey,
  useBootstrapNode,
  useCreateNode,
  useCreateEnrollmentToken,
  useDiscoverNodeServices,
  useForceRetireNode,
  useImportAllNodeServiceDiscoveries,
  useImportNodeServiceDiscovery,
  useInstallNodeCapability,
  useLaunchNodeSshSession,
  useNodeAccessMethods,
  useNodeBootstrapRuns,
  useNodeCapabilities,
  useNodeCapabilityDrift,
  useNodeCapabilityInstallEvents,
  useNodeDetail,
  useNodeDiagnostics,
  useNodeEnrollmentTokens,
  useNodeInventory,
  useNodeServiceDiscoveries,
  useNodeServiceDiscoverySummary,
  useNodeServiceInstallers,
  useNodes,
  useReinstallOrUpdateNodeAgent,
  useRetireNode,
  useRevokeEnrollmentToken,
  useRotateEnrollmentToken,
  useRotateNodeAgentToken,
  useRunNodeDiagnosticsAction,
  useScanNodeHostKey,
  useSetNodeMaintenance,
  useSyncNodeInventory,
  useUpdateNode,
  useVerifyNodeCapability,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, Checkbox, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Modal, OneTimeSecretPanel, Select, StatusBadge, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type NodeTab = 'overview' | 'runtime' | 'inventory' | 'capabilities' | 'diagnostics' | 'discovery' | 'bootstrap' | 'security' | 'terminal' | 'lifecycle' | 'jobs';

type ConfirmAction =
  | { type: 'maintenance-enable'; node: NodeDetail }
  | { type: 'maintenance-disable'; node: NodeDetail }
  | { type: 'inventory-sync'; node: NodeDetail }
  | { type: 'capability-install'; node: NodeDetail; input: NodeCapabilityInstallInput }
  | { type: 'capability-verify'; node: NodeDetail; serviceCode: string }
  | { type: 'diagnostics'; node: NodeDetail; action: NodeDiagnosticsAction }
  | { type: 'service-discover'; node: NodeDetail }
  | { type: 'service-import'; node: NodeDetail; discovery: NodeServiceDiscovery }
  | { type: 'service-import-all'; node: NodeDetail }
  | { type: 'enrollment-create'; node: NodeDetail; ttlHours: number }
  | { type: 'enrollment-rotate'; node: NodeDetail; ttlHours: number }
  | { type: 'enrollment-revoke'; node: NodeDetail; token: EnrollmentToken }
  | { type: 'bootstrap'; node: NodeDetail; input: BootstrapRequest }
  | { type: 'agent-reinstall'; node: NodeDetail; input: BootstrapRequest }
  | { type: 'host-key-pin'; node: NodeDetail; method: NodeAccessMethod; fingerprint: string }
  | { type: 'agent-token-rotate'; node: NodeDetail }
  | { type: 'ssh-session-launch'; node: NodeDetail }
  | { type: 'node-retire'; node: NodeDetail }
  | { type: 'node-force-retire'; node: NodeDetail; confirmation: string; reason: string };

type OneTimePanelState = {
  label: string;
  value: string;
  expiresAt?: string;
};

type NodeProfileFormState = {
  name: string;
  address: string;
  kind: NodeCreateInput['kind'];
  role: NodeCreateInput['role'];
  locationLabel: string;
  osFamily: string;
  osVersion: string;
  architecture: string;
  executionMode: NodeCreateInput['execution_mode'];
};

const defaultNodeProfileForm: NodeProfileFormState = {
  name: '',
  address: '',
  kind: 'remote',
  role: 'egress',
  locationLabel: '',
  osFamily: 'linux',
  osVersion: 'unknown',
  architecture: 'amd64',
  executionMode: 'agent_managed',
};

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) {
    return error instanceof Error ? error.message : 'Request failed';
  }
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

function nodeLabel(node?: NodeDetail | null): string {
  if (!node) return 'n/a';
  return text(node.name || node.address || node.id);
}

function endpoint(node: NodeDetail): string {
  return [node.address, node.location_label].filter(Boolean).join(' / ') || 'n/a';
}

function safeJSON(value: unknown): string {
  if (value == null || value === '') return 'n/a';
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function runtimePlatform(node: NodeDetail): string {
  return [node.os_family, node.os_version, node.architecture].filter(Boolean).join(' / ') || 'n/a';
}

function nodeFormFromNode(node?: NodeDetail | null): NodeProfileFormState {
  if (!node) return defaultNodeProfileForm;
  return {
    name: node.name || '',
    address: node.address || '',
    kind: (node.kind || defaultNodeProfileForm.kind) as NodeProfileFormState['kind'],
    role: (node.role || defaultNodeProfileForm.role) as NodeProfileFormState['role'],
    locationLabel: node.location_label || '',
    osFamily: node.os_family || defaultNodeProfileForm.osFamily,
    osVersion: node.os_version || defaultNodeProfileForm.osVersion,
    architecture: node.architecture || defaultNodeProfileForm.architecture,
    executionMode: (node.execution_mode || defaultNodeProfileForm.executionMode) as NodeProfileFormState['executionMode'],
  };
}

function nodeInputFromForm(form: NodeProfileFormState): NodeCreateInput {
  return {
    name: form.name.trim(),
    address: form.address.trim(),
    kind: form.kind,
    role: form.role,
    location_label: form.locationLabel.trim(),
    os_family: form.osFamily.trim(),
    os_version: form.osVersion.trim(),
    architecture: form.architecture.trim(),
    execution_mode: form.executionMode,
  };
}

function recordJobsFrom(result: unknown): Job[] {
  if (!result || typeof result !== 'object') return [];
  const candidate = result as { id?: unknown; job?: unknown; jobs?: unknown };
  if (typeof candidate.id === 'string' && 'status' in candidate && 'type' in candidate) {
    return [candidate as Job];
  }
  if (candidate.job && typeof candidate.job === 'object') {
    return recordJobsFrom(candidate.job);
  }
  if (Array.isArray(candidate.jobs)) {
    return candidate.jobs.flatMap(recordJobsFrom);
  }
  return [];
}

function compactDiagnostics(diagnostics?: NodeDiagnostics): APIRecord {
  if (!diagnostics) return {};
  return {
    heartbeat_state: diagnostics.heartbeat_state,
    heartbeat_drift_seconds: diagnostics.heartbeat_drift_seconds,
    communication_state: diagnostics.communication_state,
    communication_hint: diagnostics.communication_hint,
    agent: diagnostics.agent,
    latest_inventory: diagnostics.latest_inventory ? {
      id: diagnostics.latest_inventory.id,
      node_id: diagnostics.latest_inventory.node_id,
      created_at: diagnostics.latest_inventory.created_at,
    } : undefined,
    discovery_summary: diagnostics.discovery_summary,
    recent_discoveries: diagnostics.recent_discoveries,
  };
}

export function NodesPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const auth = useAuth();
  const canWriteNodes = hasPermission(auth.permissions, auth.roles, 'node.write');
  const nodes = useNodes();
  const [selectedId, setSelectedId] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [roleFilter, setRoleFilter] = useState('all');
  const rows = nodes.data || [];
  const statuses = Array.from(new Set(rows.map((row) => row.status).filter(Boolean))).sort();
  const roles = Array.from(new Set(rows.map((row) => row.role).filter(Boolean))).sort();
  const filteredRows = rows.filter((row) => {
    const haystack = `${row.id} ${row.name || ''} ${row.role || ''} ${row.status || ''} ${row.agent_status || ''} ${row.address || ''}`.toLowerCase();
    return (!search || haystack.includes(search.toLowerCase()))
      && (statusFilter === 'all' || row.status === statusFilter)
      && (roleFilter === 'all' || row.role === roleFilter);
  });
  const selected = rows.find((row) => row.id === selectedId);

  return (
    <PageScaffold
      title={t('nodes.title')}
      subtitle={t('nodes.subtitle')}
      actions={(
        <Button
          icon={<PlusCircle size={16} />}
          variant="primary"
          disabled={!canWriteNodes}
          title={!canWriteNodes ? t('common.permissionRequired', { permission: 'node.write' }) : undefined}
          onClick={() => setCreateOpen(true)}
        >
          {t('nodes.createNode')}
        </Button>
      )}
    >
      <NodeProfileModal
        mode="create"
        open={createOpen}
        canWrite={canWriteNodes}
        onClose={() => setCreateOpen(false)}
        onDone={(created) => {
          setSelectedId(created.id);
          setCreateOpen(false);
        }}
      />
      <Card>
        <CardBody>
          <Toolbar>
            <FormField label={t('common.search')}>
              <div className="input-with-icon">
                <Search size={16} />
                <TextField value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('nodes.searchPlaceholder')} />
              </div>
            </FormField>
            <FormField label={t('common.status')}>
              <Select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
                <option value="all">{t('common.all')}</option>
                {statuses.map((status) => <option key={status} value={status}>{status}</option>)}
              </Select>
            </FormField>
            <FormField label={t('nodes.role')}>
              <Select value={roleFilter} onChange={(event) => setRoleFilter(event.target.value)}>
                <option value="all">{t('common.all')}</option>
                {roles.map((role) => <option key={role} value={role}>{role}</option>)}
              </Select>
            </FormField>
            <Button icon={<RefreshCw size={16} />} onClick={() => void nodes.refetch()}>{t('common.refresh')}</Button>
          </Toolbar>
        </CardBody>
      </Card>
      <QueryBoundary isLoading={nodes.isLoading} isError={nodes.isError} error={nodes.error} refetch={() => void nodes.refetch()}>
        <DataTable
          rows={filteredRows}
          columns={[
            { key: 'node', header: t('nodes.node'), render: (row) => <strong>{nodeLabel(row)}</strong> },
            { key: 'role', header: t('nodes.role'), render: (row) => text(row.role || row.kind) },
            { key: 'address', header: t('nodes.address'), render: (row) => <code>{text(row.address)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'agent', header: t('nodes.agent'), render: (row) => <StatusBadge status={row.agent_status || 'unknown'} /> },
            { key: 'heartbeat', header: t('nodes.heartbeat'), render: (row) => fmt.date(row.last_heartbeat_at || row.agent_last_seen_at) },
            { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button onClick={() => setSelectedId(row.id)}>{t('common.open')}</Button> },
          ]}
        />
      </QueryBoundary>
      <NodeDrawer
        key={selectedId || 'closed'}
        node={selected}
        nodeId={selectedId}
        open={Boolean(selectedId)}
        onClose={() => setSelectedId('')}
      />
    </PageScaffold>
  );
}

function NodeProfileModal({ mode, node, open, canWrite, onClose, onDone }: {
  mode: 'create' | 'edit';
  node?: NodeDetail;
  open: boolean;
  canWrite: boolean;
  onClose: () => void;
  onDone: (node: NodeDetail) => void;
}) {
  const { t } = useTranslation();
  const createNode = useCreateNode();
  const updateNode = useUpdateNode();
  const [form, setForm] = useState<NodeProfileFormState>(nodeFormFromNode(node));
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({});
  const mutation = mode === 'create' ? createNode : updateNode;
  const mutationErrors = fieldErrors(mutation.error);
  const errors = { ...mutationErrors, ...localErrors };
  const busy = createNode.isPending || updateNode.isPending;

  const patch = (patchValue: Partial<NodeProfileFormState>) => {
    setForm((current) => ({ ...current, ...patchValue }));
    setLocalErrors((current) => {
      const next = { ...current };
      Object.keys(patchValue).forEach((key) => {
        delete next[key];
      });
      return next;
    });
  };

  const validate = () => {
    const next: Record<string, string> = {};
    if (!form.name.trim()) next.name = t('nodes.validation.nameRequired');
    if (!form.address.trim()) next.address = t('nodes.validation.addressRequired');
    setLocalErrors(next);
    return Object.keys(next).length === 0;
  };

  const close = () => {
    createNode.reset();
    updateNode.reset();
    setForm(nodeFormFromNode(mode === 'edit' ? node : null));
    setLocalErrors({});
    onClose();
  };

  const submit = async () => {
    if (!canWrite || !validate()) return;
    const input = nodeInputFromForm(form);
    try {
      const saved = mode === 'create'
        ? await createNode.mutateAsync(input)
        : await updateNode.mutateAsync({ nodeId: node?.id || '', input });
      createNode.reset();
      updateNode.reset();
      setForm(defaultNodeProfileForm);
      setLocalErrors({});
      onDone(saved);
    } catch {
      // The mutation error is rendered below; keep operator input intact for correction/retry.
    }
  };

  return (
    <Modal title={mode === 'create' ? t('nodes.createNode') : t('nodes.editNode')} open={open} onClose={close}>
      <div className="page-stack">
        {!canWrite ? <div role="alert" className="error-state-inline">{t('common.permissionRequired', { permission: 'node.write' })}</div> : null}
        {mutation.error ? <div role="alert" className="error-state-inline">{formatAPIError(mutation.error)}</div> : null}
        <FormGrid>
          <FormField label={t('common.name')}>
            <TextField value={form.name} onChange={(event) => patch({ name: event.target.value })} autoComplete="off" />
            {errors.name ? <span role="alert">{errors.name}</span> : null}
          </FormField>
          <FormField label={t('nodes.address')}>
            <TextField value={form.address} onChange={(event) => patch({ address: event.target.value })} autoComplete="off" />
            {errors.address ? <span role="alert">{errors.address}</span> : null}
          </FormField>
          <FormField label={t('nodes.kind')}>
            <Select value={form.kind} onChange={(event) => patch({ kind: event.target.value as NodeProfileFormState['kind'] })}>
              <option value="local">{t('nodes.kindOptions.local')}</option>
              <option value="remote">{t('nodes.kindOptions.remote')}</option>
            </Select>
            {errors.kind ? <span role="alert">{errors.kind}</span> : null}
          </FormField>
          <FormField label={t('nodes.role')}>
            <Select value={form.role} onChange={(event) => patch({ role: event.target.value as NodeProfileFormState['role'] })}>
              <option value="ingress">{t('nodes.roleOptions.ingress')}</option>
              <option value="egress">{t('nodes.roleOptions.egress')}</option>
            </Select>
            {errors.role ? <span role="alert">{errors.role}</span> : null}
          </FormField>
          <FormField label={t('nodes.locationLabel')}>
            <TextField value={form.locationLabel} onChange={(event) => patch({ locationLabel: event.target.value })} />
            {errors.location_label ? <span role="alert">{errors.location_label}</span> : null}
          </FormField>
          <FormField label={t('nodes.osFamily')}>
            <TextField value={form.osFamily} onChange={(event) => patch({ osFamily: event.target.value })} />
            {errors.os_family ? <span role="alert">{errors.os_family}</span> : null}
          </FormField>
          <FormField label={t('nodes.osVersion')}>
            <TextField value={form.osVersion} onChange={(event) => patch({ osVersion: event.target.value })} />
            {errors.os_version ? <span role="alert">{errors.os_version}</span> : null}
          </FormField>
          <FormField label={t('nodes.architecture')}>
            <TextField value={form.architecture} onChange={(event) => patch({ architecture: event.target.value })} />
            {errors.architecture ? <span role="alert">{errors.architecture}</span> : null}
          </FormField>
          <FormField label={t('nodes.executionMode')}>
            <Select value={form.executionMode} onChange={(event) => patch({ executionMode: event.target.value as NodeProfileFormState['executionMode'] })}>
              <option value="agent_managed">{t('nodes.executionModeOptions.agentManaged')}</option>
              <option value="ssh_bootstrap">{t('nodes.executionModeOptions.sshBootstrap')}</option>
              <option value="manual_bundle">{t('nodes.executionModeOptions.manualBundle')}</option>
              <option value="local_managed">{t('nodes.executionModeOptions.localManaged')}</option>
            </Select>
            {errors.execution_mode ? <span role="alert">{errors.execution_mode}</span> : null}
          </FormField>
        </FormGrid>
        <p className="muted">{t('nodes.profileSafeFieldsNote')}</p>
        <Toolbar>
          <Button variant="primary" disabled={!canWrite || busy} onClick={() => void submit()}>{mode === 'create' ? t('nodes.createNode') : t('common.save')}</Button>
          <Button onClick={close}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </Modal>
  );
}

function NodeDrawer({ node, nodeId, open, onClose }: { node?: NodeDetail; nodeId: string; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const auth = useAuth();
  const canWriteNodes = hasPermission(auth.permissions, auth.roles, 'node.write');
  const [activeTab, setActiveTab] = useState<NodeTab>('overview');
  const [editOpen, setEditOpen] = useState(false);
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);
  const [notice, setNotice] = useState('');
  const [jobIds, setJobIds] = useState<string[]>([]);
  const [oneTimePanel, setOneTimePanel] = useState<OneTimePanelState | null>(null);
  const detail = useNodeDetail(nodeId, { retry: false });
  const diagnostics = useNodeDiagnostics(nodeId, { retry: false });
  const inventory = useNodeInventory(nodeId, { retry: false });
  const capabilities = useNodeCapabilities(nodeId, { retry: false });
  const capabilityDrift = useNodeCapabilityDrift(nodeId, { retry: false });
  const capabilityEvents = useNodeCapabilityInstallEvents(nodeId, { retry: false });
  const installers = useNodeServiceInstallers({ retry: false });
  const discoveries = useNodeServiceDiscoveries(nodeId, { retry: false });
  const discoverySummary = useNodeServiceDiscoverySummary(nodeId, { retry: false });
  const enrollmentTokens = useNodeEnrollmentTokens(nodeId, { retry: false });
  const accessMethods = useNodeAccessMethods(nodeId, { retry: false });
  const bootstrapRuns = useNodeBootstrapRuns(nodeId, { retry: false });
  const maintenance = useSetNodeMaintenance();
  const syncInventory = useSyncNodeInventory();
  const installCapability = useInstallNodeCapability();
  const verifyCapability = useVerifyNodeCapability();
  const runDiagnostics = useRunNodeDiagnosticsAction();
  const discoverServices = useDiscoverNodeServices();
  const importDiscovery = useImportNodeServiceDiscovery();
  const importAllDiscoveries = useImportAllNodeServiceDiscoveries();
  const createEnrollment = useCreateEnrollmentToken();
  const rotateEnrollment = useRotateEnrollmentToken();
  const revokeEnrollment = useRevokeEnrollmentToken();
  const bootstrap = useBootstrapNode();
  const reinstallAgent = useReinstallOrUpdateNodeAgent();
  const scanHostKey = useScanNodeHostKey();
  const acceptHostKey = useAcceptNodeHostKey();
  const rotateAgentToken = useRotateNodeAgentToken();
  const launchSSH = useLaunchNodeSshSession();
  const retire = useRetireNode();
  const forceRetire = useForceRetireNode();
  const current = detail.data || node;
  const busy = maintenance.isPending
    || syncInventory.isPending
    || installCapability.isPending
    || verifyCapability.isPending
    || runDiagnostics.isPending
    || discoverServices.isPending
    || importDiscovery.isPending
    || importAllDiscoveries.isPending
    || createEnrollment.isPending
    || rotateEnrollment.isPending
    || revokeEnrollment.isPending
    || bootstrap.isPending
    || reinstallAgent.isPending
    || scanHostKey.isPending
    || acceptHostKey.isPending
    || rotateAgentToken.isPending
    || launchSSH.isPending
    || retire.isPending
    || forceRetire.isPending;

  const recordJobs = (result: unknown) => {
    const ids = recordJobsFrom(result).map((job) => job.id).filter(Boolean);
    if (ids.length) {
      setJobIds((existing) => Array.from(new Set([...ids, ...existing])).slice(0, 5));
    }
  };

  const runConfirmed = async () => {
    if (!confirm) return;
    setNotice('');
    try {
      let result: NodeMutationResult | undefined;
      if (confirm.type === 'maintenance-enable') {
        result = await maintenance.mutateAsync({ nodeId: confirm.node.id, enabled: true });
        setNotice(t('nodes.maintenanceUpdated'));
        setActiveTab('overview');
      } else if (confirm.type === 'maintenance-disable') {
        result = await maintenance.mutateAsync({ nodeId: confirm.node.id, enabled: false });
        setNotice(t('nodes.maintenanceUpdated'));
        setActiveTab('overview');
      } else if (confirm.type === 'inventory-sync') {
        result = await syncInventory.mutateAsync({ nodeId: confirm.node.id });
        setNotice(t('nodes.actionAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'capability-install') {
        result = await installCapability.mutateAsync({ nodeId: confirm.node.id, input: confirm.input });
        setNotice(t('nodes.actionAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'capability-verify') {
        result = await verifyCapability.mutateAsync({ nodeId: confirm.node.id, input: { service_code: confirm.serviceCode } });
        setNotice(t('nodes.actionAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'diagnostics') {
        result = await runDiagnostics.mutateAsync({ nodeId: confirm.node.id, action: confirm.action });
        setNotice(t('nodes.actionAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'service-discover') {
        result = await discoverServices.mutateAsync({ nodeId: confirm.node.id });
        setNotice(t('nodes.actionAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'service-import') {
        result = await importDiscovery.mutateAsync({ nodeId: confirm.node.id, discoveryId: confirm.discovery.id });
        setNotice(t('nodes.importAccepted'));
        setActiveTab('discovery');
      } else if (confirm.type === 'service-import-all') {
        result = await importAllDiscoveries.mutateAsync({ nodeId: confirm.node.id });
        setNotice(t('nodes.importAccepted'));
        setActiveTab('discovery');
      } else if (confirm.type === 'enrollment-create') {
        const tokenResult = await createEnrollment.mutateAsync({ nodeId: confirm.node.id, input: { ttl_hours: confirm.ttlHours } });
        result = tokenResult;
        setOneTimePanel({
          label: t('nodes.enrollmentOneTimeToken'),
          value: String(tokenResult.token || tokenResult.enrollment_token || ''),
          expiresAt: tokenResult.expires_at,
        });
        setNotice(t('nodes.tokenCreated'));
        setActiveTab('security');
      } else if (confirm.type === 'enrollment-rotate') {
        const tokenResult = await rotateEnrollment.mutateAsync({ nodeId: confirm.node.id, input: { ttl_hours: confirm.ttlHours } });
        result = tokenResult;
        setOneTimePanel({
          label: t('nodes.enrollmentOneTimeToken'),
          value: String(tokenResult.token || tokenResult.enrollment_token || ''),
          expiresAt: tokenResult.expires_at,
        });
        setNotice(t('nodes.tokenRotated'));
        setActiveTab('security');
      } else if (confirm.type === 'enrollment-revoke') {
        result = await revokeEnrollment.mutateAsync({ nodeId: confirm.node.id, tokenId: confirm.token.id });
        setNotice(t('nodes.tokenRevoked'));
        setActiveTab('security');
      } else if (confirm.type === 'bootstrap') {
        result = await bootstrap.mutateAsync({ nodeId: confirm.node.id, input: confirm.input });
        setNotice(t('nodes.bootstrapQueued'));
        setActiveTab('jobs');
      } else if (confirm.type === 'agent-reinstall') {
        result = await reinstallAgent.mutateAsync({ nodeId: confirm.node.id, input: confirm.input });
        setNotice(t('nodes.bootstrapQueued'));
        setActiveTab('jobs');
      } else if (confirm.type === 'host-key-pin') {
        result = await acceptHostKey.mutateAsync({ nodeId: confirm.node.id, methodId: confirm.method.id, fingerprint: confirm.fingerprint });
        setNotice(t('nodes.hostKeyPinned'));
        setActiveTab('security');
      } else if (confirm.type === 'agent-token-rotate') {
        result = await rotateAgentToken.mutateAsync({ nodeId: confirm.node.id });
        setNotice(t('nodes.actionAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'ssh-session-launch') {
        const sessionResult = await launchSSH.mutateAsync({ nodeId: confirm.node.id });
        result = sessionResult;
        if (sessionResult.terminal_url) {
          setOneTimePanel({
            label: t('nodes.sshSessionOneTimeURL'),
            value: sessionResult.terminal_url,
            expiresAt: sessionResult.expires_at,
          });
        }
        setNotice(t('nodes.sshSessionCreated'));
        setActiveTab('terminal');
      } else if (confirm.type === 'node-retire') {
        result = await retire.mutateAsync({ nodeId: confirm.node.id });
        setNotice(t('nodes.nodeRetired'));
        setActiveTab('lifecycle');
      } else if (confirm.type === 'node-force-retire') {
        result = await forceRetire.mutateAsync({ nodeId: confirm.node.id, confirmation: confirm.confirmation, reason: confirm.reason });
        setNotice(t('nodes.nodeRetired'));
        setActiveTab('lifecycle');
      }
      recordJobs(result);
      setConfirm(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const firstAgentJob = diagnostics.data?.agent?.last_job_result_job_id || diagnostics.data?.agent?.last_job_claim_job_id || diagnostics.data?.agent?.last_job_claim_job_id || '';
  const effectiveJobIds = jobIds.length ? jobIds : firstAgentJob ? [firstAgentJob] : [];

  return (
    <Drawer title={nodeLabel(current)} open={open} onClose={onClose}>
      <QueryBoundary isLoading={detail.isLoading} isError={detail.isError} error={detail.error} refetch={() => void detail.refetch()}>
        {current ? (
          <div className="page-stack">
            <div className="tabs" role="tablist" aria-label={t('nodes.detailTabs')}>
              {(['overview', 'runtime', 'inventory', 'capabilities', 'diagnostics', 'discovery', 'bootstrap', 'security', 'terminal', 'lifecycle', 'jobs'] as NodeTab[]).map((tab) => (
                <button
                  key={tab}
                  className={`tab-link ${activeTab === tab ? 'active' : ''}`.trim()}
                  role="tab"
                  aria-selected={activeTab === tab}
                  type="button"
                  onClick={() => setActiveTab(tab)}
                >
                  {t(`nodes.tabs.${tab}`)}
                </button>
              ))}
            </div>
            {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
            {oneTimePanel ? (
              <OneTimeSecretPanel
                label={oneTimePanel.label}
                value={oneTimePanel.value}
                expiresAt={oneTimePanel.expiresAt}
                onClose={() => setOneTimePanel(null)}
              />
            ) : null}
            {activeTab === 'overview' ? <OverviewTab node={current} busy={busy} onConfirm={setConfirm} canWrite={canWriteNodes} onEdit={() => setEditOpen(true)} /> : null}
            {activeTab === 'runtime' ? <RuntimeTab node={current} diagnostics={diagnostics} /> : null}
            {activeTab === 'inventory' ? <InventoryTab node={current} inventory={inventory} busy={busy} onConfirm={setConfirm} /> : null}
            {activeTab === 'capabilities' ? (
              <CapabilitiesTab
                node={current}
                capabilities={capabilities}
                drift={capabilityDrift}
                events={capabilityEvents}
                installers={installers.data || []}
                installersError={installers.isError ? installers.error : null}
                busy={busy}
                onConfirm={setConfirm}
              />
            ) : null}
            {activeTab === 'diagnostics' ? <DiagnosticsTab node={current} diagnostics={diagnostics} busy={busy} onConfirm={setConfirm} /> : null}
            {activeTab === 'discovery' ? <DiscoveryTab node={current} discoveries={discoveries} summary={discoverySummary} busy={busy} onConfirm={setConfirm} /> : null}
            {activeTab === 'bootstrap' ? <BootstrapTab node={current} accessMethods={accessMethods} runs={bootstrapRuns} busy={busy} onConfirm={setConfirm} /> : null}
            {activeTab === 'security' ? (
              <SecurityTab
                node={current}
                diagnostics={diagnostics.data}
                tokens={enrollmentTokens}
                accessMethods={accessMethods}
                scanHostKey={scanHostKey}
                busy={busy}
                onConfirm={setConfirm}
              />
            ) : null}
            {activeTab === 'terminal' ? <TerminalTab node={current} accessMethods={accessMethods} busy={busy} onConfirm={setConfirm} /> : null}
            {activeTab === 'lifecycle' ? <LifecycleTab node={current} busy={busy} onConfirm={setConfirm} /> : null}
            {activeTab === 'jobs' ? <NodeJobsTab jobIds={effectiveJobIds} diagnostics={diagnostics.data} /> : null}
            <NodeProfileModal
              mode="edit"
              node={current}
              open={editOpen}
              canWrite={canWriteNodes}
              onClose={() => setEditOpen(false)}
              onDone={(updated) => {
                setNotice(t('nodes.profileUpdated'));
                setEditOpen(false);
                setActiveTab('overview');
                if (updated.id) void detail.refetch();
              }}
            />
            <NodeConfirmDialog action={confirm} busy={busy} onConfirm={() => void runConfirmed()} onClose={() => setConfirm(null)} />
          </div>
        ) : null}
      </QueryBoundary>
    </Drawer>
  );
}

function OverviewTab({ node, busy, onConfirm, canWrite, onEdit }: { node: NodeDetail; busy: boolean; onConfirm: (action: ConfirmAction) => void; canWrite: boolean; onEdit: () => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const maintenanceEnabled = node.status === 'maintenance';
  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <div className="definition-grid">
            <span>{t('common.id')}</span><strong>{node.id}</strong>
            <span>{t('common.name')}</span><strong>{nodeLabel(node)}</strong>
            <span>{t('nodes.role')}</span><strong>{node.role || node.kind || 'n/a'}</strong>
            <span>{t('common.status')}</span><strong>{node.status || 'n/a'}</strong>
            <span>{t('nodes.address')}</span><strong>{endpoint(node)}</strong>
            <span>{t('nodes.platform')}</span><strong>{runtimePlatform(node)}</strong>
            <span>{t('nodes.executionMode')}</span><strong>{node.execution_mode || 'n/a'}</strong>
            <span>{t('nodes.agent')}</span><strong>{node.agent_status || 'n/a'}</strong>
            <span>{t('nodes.heartbeat')}</span><strong>{fmt.date(node.last_heartbeat_at || node.agent_last_seen_at)}</strong>
            <span>{t('common.created')}</span><strong>{fmt.date(node.created_at)}</strong>
            <span>{t('common.updated')}</span><strong>{fmt.date(node.updated_at)}</strong>
          </div>
          <Toolbar>
            <StatusBadge status={node.status} />
            <Button
              icon={<FilePenLine size={16} />}
              disabled={!canWrite || busy}
              title={!canWrite ? t('common.permissionRequired', { permission: 'node.write' }) : undefined}
              onClick={onEdit}
            >
              {t('nodes.editNode')}
            </Button>
            <Button
              variant={maintenanceEnabled ? 'secondary' : 'danger'}
              icon={<Wrench size={16} />}
              disabled={busy}
              onClick={() => onConfirm({ type: maintenanceEnabled ? 'maintenance-disable' : 'maintenance-enable', node })}
            >
              {maintenanceEnabled ? t('nodes.disableMaintenance') : t('nodes.enableMaintenance')}
            </Button>
          </Toolbar>
        </div>
      </CardBody>
    </Card>
  );
}

function RuntimeTab({ node, diagnostics }: { node: NodeDetail; diagnostics: ReturnType<typeof useNodeDiagnostics> }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const data = diagnostics.data;
  const agent = data?.agent;
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <StatusBadge status={data?.heartbeat_state || node.status} />
            <StatusBadge status={data?.communication_state || node.agent_status} />
            <Button icon={<RefreshCw size={16} />} onClick={() => void diagnostics.refetch()}>{t('common.refresh')}</Button>
          </Toolbar>
          {diagnostics.isError ? <div role="alert" className="error-state-inline">{t('nodes.diagnosticsUnavailable')}: {formatAPIError(diagnostics.error)}</div> : null}
          <div className="definition-grid">
            <span>{t('nodes.heartbeatState')}</span><strong>{data?.heartbeat_state || node.status || 'n/a'}</strong>
            <span>{t('nodes.communicationState')}</span><strong>{data?.communication_state || 'n/a'}</strong>
            <span>{t('nodes.communicationHint')}</span><strong>{data?.communication_hint || 'n/a'}</strong>
            <span>{t('nodes.agentStatus')}</span><strong>{agent?.status || node.agent_status || 'n/a'}</strong>
            <span>{t('nodes.agentVersion')}</span><strong>{agent?.agent_version || node.agent_version || 'n/a'}</strong>
            <span>{t('nodes.protocolVersion')}</span><strong>{agent?.protocol_version || node.agent_protocol_version || 'n/a'}</strong>
            <span>{t('nodes.lastSeen')}</span><strong>{fmt.date(agent?.last_seen_at || node.agent_last_seen_at)}</strong>
            <span>{t('nodes.lastJobPoll')}</span><strong>{fmt.date(agent?.last_job_poll_at)}</strong>
            <span>{t('nodes.lastJobResult')}</span><strong>{fmt.date(agent?.last_job_result_at)}</strong>
            <span>{t('nodes.lastInventory')}</span><strong>{fmt.date(agent?.last_inventory_sync_at || data?.latest_inventory?.created_at)}</strong>
            <span>{t('nodes.lastDiscovery')}</span><strong>{fmt.date(agent?.last_discovery_sync_at)}</strong>
            <span>{t('nodes.lastRuntime')}</span><strong>{fmt.date(agent?.last_runtime_sync_at)}</strong>
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardBody>
          <pre className="code-block">{safeJSON(compactDiagnostics(data))}</pre>
        </CardBody>
      </Card>
    </div>
  );
}

function InventoryTab({ node, inventory, busy, onConfirm }: { node: NodeDetail; inventory: ReturnType<typeof useNodeInventory>; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <Button variant="primary" icon={<DownloadCloud size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'inventory-sync', node })}>{t('nodes.syncInventory')}</Button>
            <Button icon={<RefreshCw size={16} />} onClick={() => void inventory.refetch()}>{t('common.refresh')}</Button>
          </Toolbar>
          {inventory.isError ? <div role="alert" className="error-state-inline">{t('nodes.inventoryUnavailable')}: {formatAPIError(inventory.error)}</div> : null}
          {inventory.data ? (
            <div className="definition-grid">
              <span>{t('common.id')}</span><strong>{inventory.data.id}</strong>
              <span>{t('common.created')}</span><strong>{fmt.date(inventory.data.created_at)}</strong>
            </div>
          ) : null}
        </CardBody>
      </Card>
      <Card>
        <CardBody>
          <pre className="code-block">{safeJSON(inventory.data?.payload || inventory.data || {})}</pre>
        </CardBody>
      </Card>
    </div>
  );
}

function CapabilitiesTab({ node, capabilities, drift, events, installers, installersError, busy, onConfirm }: {
  node: NodeDetail;
  capabilities: ReturnType<typeof useNodeCapabilities>;
  drift: ReturnType<typeof useNodeCapabilityDrift>;
  events: ReturnType<typeof useNodeCapabilityInstallEvents>;
  installers: NodeServiceInstaller[];
  installersError: Error | null;
  busy: boolean;
  onConfirm: (action: ConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const [serviceCode, setServiceCode] = useState('');
  const [installerKey, setInstallerKey] = useState('');
  const serviceCodes = useMemo(() => Array.from(new Set(installers.map((installer) => installer.service_code).filter(Boolean))).sort(), [installers]);
  const matchingInstallers = installers.filter((installer) => !serviceCode || installer.service_code === serviceCode);
  const selectedInstaller = matchingInstallers.find((installer) => `${installer.strategy || ''}|${installer.channel || ''}` === installerKey);
  const installInput: NodeCapabilityInstallInput = {
    service_code: serviceCode,
    strategy: selectedInstaller?.strategy || undefined,
    channel: selectedInstaller?.channel || undefined,
  };

  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <FormGrid>
              <FormField label={t('nodes.capabilityService')}>
                <TextField list="node-capability-services" value={serviceCode} onChange={(event) => { setServiceCode(event.target.value); setInstallerKey(''); }} />
              </FormField>
              <FormField label={t('nodes.capabilityStrategy')}>
                <Select value={installerKey} onChange={(event) => setInstallerKey(event.target.value)}>
                  <option value="">{t('nodes.backendDefault')}</option>
                  {matchingInstallers.map((installer) => (
                    <option key={`${installer.service_code}:${installer.strategy}:${installer.channel}`} value={`${installer.strategy || ''}|${installer.channel || ''}`}>
                      {[installer.service_code, installer.strategy, installer.channel].filter(Boolean).join(' / ')}
                    </option>
                  ))}
                </Select>
              </FormField>
            </FormGrid>
            <datalist id="node-capability-services">
              {serviceCodes.map((code) => <option key={code} value={code} />)}
            </datalist>
            {installersError ? <div role="alert" className="error-state-inline">{t('nodes.installersUnavailable')}: {formatAPIError(installersError)}</div> : null}
            {selectedInstaller?.description ? <p className="muted">{selectedInstaller.description}</p> : null}
            <Toolbar>
              <Button variant="primary" icon={<PackageCheck size={16} />} disabled={busy || !serviceCode} onClick={() => onConfirm({ type: 'capability-install', node, input: installInput })}>{t('nodes.installCapability')}</Button>
              <Button icon={<CheckCircle2 size={16} />} disabled={busy || !serviceCode} onClick={() => onConfirm({ type: 'capability-verify', node, serviceCode })}>{t('nodes.verifyCapability')}</Button>
              <Button icon={<RefreshCw size={16} />} onClick={() => { void capabilities.refetch(); void drift.refetch(); void events.refetch(); }}>{t('common.refresh')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      <QueryBoundary isLoading={capabilities.isLoading} isError={capabilities.isError} error={capabilities.error} refetch={() => void capabilities.refetch()}>
        <DataTable
          rows={capabilities.data || []}
          columns={[
            { key: 'capability', header: t('nodes.capability'), render: (row) => <code>{text(row.capability_code)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'version', header: t('nodes.version'), render: (row) => text(row.version) },
            { key: 'source', header: t('nodes.source'), render: (row) => text(row.source) },
            { key: 'detected', header: t('nodes.detected'), render: (row) => fmt.date(row.detected_at) },
            { key: 'actions', header: t('common.actions'), render: (row: NodeCapability) => (
              <Button icon={<CheckCircle2 size={16} />} disabled={busy || !row.capability_code} onClick={() => onConfirm({ type: 'capability-verify', node, serviceCode: row.capability_code || '' })}>
                {t('nodes.verifyCapability')}
              </Button>
            ) },
          ]}
        />
      </QueryBoundary>
      {drift.isError ? <div role="alert" className="error-state-inline">{t('nodes.driftUnavailable')}: {formatAPIError(drift.error)}</div> : null}
      {drift.data?.drift?.length ? (
        <DataTable
          rows={drift.data.drift}
          title={t('nodes.capabilityDrift')}
          columns={[
            { key: 'capability', header: t('nodes.capability'), render: (row) => <code>{text(row.capability_code)}</code> },
            { key: 'desired', header: t('nodes.desired'), render: (row) => text(row.desired) },
            { key: 'actual', header: t('nodes.actual'), render: (row) => <StatusBadge status={row.actual} /> },
            { key: 'sync', header: t('nodes.inSync'), render: (row) => row.in_sync ? t('common.yes') : t('common.no') },
          ]}
        />
      ) : null}
      {events.isError ? <div role="alert" className="error-state-inline">{t('nodes.eventsUnavailable')}: {formatAPIError(events.error)}</div> : null}
      {events.data?.length ? (
        <DataTable
          rows={events.data}
          title={t('nodes.capabilityEvents')}
          columns={[
            { key: 'job', header: t('nodes.job'), render: (row) => <code>{shortID(row.job_id || undefined)}</code> },
            { key: 'capability', header: t('nodes.capability'), render: (row) => <code>{text(row.capability_code)}</code> },
            { key: 'strategy', header: t('nodes.capabilityStrategy'), render: (row) => text(row.strategy) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'summary', header: t('common.description'), render: (row) => text(row.summary) },
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
          ]}
        />
      ) : null}
    </div>
  );
}

function DiagnosticsTab({ node, diagnostics, busy, onConfirm }: { node: NodeDetail; diagnostics: ReturnType<typeof useNodeDiagnostics>; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <Button variant="primary" icon={<Activity size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'diagnostics', node, action: 'channel-probe' })}>{t('nodes.probeChannel')}</Button>
            <Button icon={<DownloadCloud size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'diagnostics', node, action: 'retry-inventory' })}>{t('nodes.retryInventory')}</Button>
            <Button icon={<RefreshCw size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'diagnostics', node, action: 'retry-discovery' })}>{t('nodes.retryDiscovery')}</Button>
            <Button icon={<ServerCog size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'diagnostics', node, action: 'reconcile-runtime' })}>{t('nodes.reconcileRuntime')}</Button>
            <Button icon={<Play size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'diagnostics', node, action: 'requeue-stuck-job' })}>{t('nodes.requeueStuckJob')}</Button>
            <Button icon={<RefreshCw size={16} />} onClick={() => void diagnostics.refetch()}>{t('common.refresh')}</Button>
          </Toolbar>
        </CardBody>
      </Card>
      <QueryBoundary isLoading={diagnostics.isLoading} isError={diagnostics.isError} error={diagnostics.error} refetch={() => void diagnostics.refetch()}>
        <Card>
          <CardBody>
            <pre className="code-block">{safeJSON(compactDiagnostics(diagnostics.data))}</pre>
          </CardBody>
        </Card>
      </QueryBoundary>
    </div>
  );
}

function DiscoveryTab({ node, discoveries, summary, busy, onConfirm }: {
  node: NodeDetail;
  discoveries: ReturnType<typeof useNodeServiceDiscoveries>;
  summary: ReturnType<typeof useNodeServiceDiscoverySummary>;
  busy: boolean;
  onConfirm: (action: ConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <Button variant="primary" icon={<Boxes size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'service-discover', node })}>{t('nodes.discoverServices')}</Button>
            <Button icon={<ShieldCheck size={16} />} disabled={busy || !(summary.data?.importable_count || 0)} onClick={() => onConfirm({ type: 'service-import-all', node })}>{t('nodes.importAllDiscoveries')}</Button>
            <Button icon={<RefreshCw size={16} />} onClick={() => { void discoveries.refetch(); void summary.refetch(); }}>{t('common.refresh')}</Button>
          </Toolbar>
          {summary.isError ? <div role="alert" className="error-state-inline">{t('nodes.discoverySummaryUnavailable')}: {formatAPIError(summary.error)}</div> : null}
          {summary.data ? (
            <div className="definition-grid">
              <span>{t('nodes.discovered')}</span><strong>{summary.data.discovered ?? summary.data.total ?? 0}</strong>
              <span>{t('nodes.imported')}</span><strong>{summary.data.imported ?? 0}</strong>
              <span>{t('nodes.ignored')}</span><strong>{summary.data.ignored ?? 0}</strong>
              <span>{t('nodes.importable')}</span><strong>{summary.data.importable_count ?? 0}</strong>
            </div>
          ) : null}
        </CardBody>
      </Card>
      <QueryBoundary isLoading={discoveries.isLoading} isError={discoveries.isError} error={discoveries.error} refetch={() => void discoveries.refetch()}>
        <DataTable
          rows={discoveries.data || []}
          columns={[
            { key: 'service', header: t('nodes.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.name || row.systemd_unit || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'endpoint', header: t('nodes.endpoint'), render: (row) => [row.endpoint_host, row.endpoint_port].filter(Boolean).join(':') || 'n/a' },
            { key: 'source', header: t('nodes.source'), render: (row) => text(row.source) },
            { key: 'confidence', header: t('nodes.confidence'), render: (row) => String(row.confidence ?? 'n/a') },
            { key: 'detected', header: t('nodes.detected'), render: (row) => fmt.date(row.detected_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => (
              <Button icon={<ShieldCheck size={16} />} disabled={busy || row.status === 'imported'} onClick={() => onConfirm({ type: 'service-import', node, discovery: row })}>
                {t('nodes.importDiscovery')}
              </Button>
            ) },
          ]}
        />
      </QueryBoundary>
    </div>
  );
}

function sshMethods(methods: NodeAccessMethod[] | undefined): NodeAccessMethod[] {
  return (methods || []).filter((method) => method.method === 'ssh');
}

function defaultSSHMethod(methods: NodeAccessMethod[] | undefined): NodeAccessMethod | undefined {
  return sshMethods(methods).find((method) => method.is_enabled) || sshMethods(methods)[0];
}

function BootstrapTab({ node, accessMethods, runs, busy, onConfirm }: {
  node: NodeDetail;
  accessMethods: ReturnType<typeof useNodeAccessMethods>;
  runs: ReturnType<typeof useNodeBootstrapRuns>;
  busy: boolean;
  onConfirm: (action: ConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const [mode, setMode] = useState<BootstrapRequest['bootstrap_mode']>('ssh_bootstrap');
  const [forceReenroll, setForceReenroll] = useState(false);
  const hasEnabledSSH = Boolean(defaultSSHMethod(accessMethods.data)?.is_enabled);
  const input: BootstrapRequest = { bootstrap_mode: mode, force_reenroll: forceReenroll };
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <FormGrid>
              <FormField label={t('nodes.bootstrapMode')}>
                <Select value={mode} onChange={(event) => setMode(event.target.value as BootstrapRequest['bootstrap_mode'])}>
                  <option value="ssh_bootstrap">{t('nodes.sshBootstrap')}</option>
                  <option value="manual_bundle">{t('nodes.manualBundle')}</option>
                </Select>
              </FormField>
              <FormField label={t('nodes.forceReenroll')}>
                <Checkbox checked={forceReenroll} onChange={(event) => setForceReenroll(event.target.checked)} label={t('nodes.forceReenrollLabel')} />
              </FormField>
            </FormGrid>
            <p className="muted">{t('nodes.bootstrapImpact')}</p>
            {mode === 'ssh_bootstrap' && !hasEnabledSSH ? <div role="alert" className="error-state-inline">{t('nodes.sshAccessMissing')}</div> : null}
            <Toolbar>
              <Button variant="primary" icon={<Play size={16} />} disabled={busy || (mode === 'ssh_bootstrap' && !hasEnabledSSH)} onClick={() => onConfirm({ type: 'bootstrap', node, input })}>
                {t('nodes.queueBootstrap')}
              </Button>
              <Button icon={<RotateCcw size={16} />} disabled={busy || !hasEnabledSSH} onClick={() => onConfirm({ type: 'agent-reinstall', node, input: { ...input, bootstrap_mode: 'ssh_bootstrap', reinstall_agent: true } })}>
                {t('nodes.reinstallAgent')}
              </Button>
              <Button icon={<RefreshCw size={16} />} onClick={() => { void accessMethods.refetch(); void runs.refetch(); }}>{t('common.refresh')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      {accessMethods.isError ? <div role="alert" className="error-state-inline">{t('nodes.accessMethodsUnavailable')}: {formatAPIError(accessMethods.error)}</div> : null}
      <AccessMethodsTable methods={accessMethods.data || []} />
      {runs.isError ? <div role="alert" className="error-state-inline">{t('nodes.bootstrapRunsUnavailable')}: {formatAPIError(runs.error)}</div> : null}
      <DataTable
        rows={runs.data || []}
        title={t('nodes.bootstrapRuns')}
        columns={[
          { key: 'id', header: t('common.id'), render: (row) => <code>{shortID(row.id)}</code> },
          { key: 'job', header: t('nodes.job'), render: (row) => <code>{shortID(row.job_id || undefined)}</code> },
          { key: 'mode', header: t('nodes.bootstrapMode'), render: (row) => text(row.bootstrap_mode) },
          { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
          { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
        ]}
      />
    </div>
  );
}

function SecurityTab({ node, diagnostics, tokens, accessMethods, scanHostKey, busy, onConfirm }: {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  tokens: ReturnType<typeof useNodeEnrollmentTokens>;
  accessMethods: ReturnType<typeof useNodeAccessMethods>;
  scanHostKey: ReturnType<typeof useScanNodeHostKey>;
  busy: boolean;
  onConfirm: (action: ConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const [ttlHours, setTTLHours] = useState(24);
  const [methodId, setMethodId] = useState('');
  const [scanResult, setScanResult] = useState<HostKeyScanResult | null>(null);
  const methods = sshMethods(accessMethods.data);
  const selectedMethod = methods.find((method) => method.id === methodId) || defaultSSHMethod(accessMethods.data);
  const ttl = Number.isFinite(ttlHours) && ttlHours > 0 ? ttlHours : 24;
  const runScan = async () => {
    if (!selectedMethod) return;
    const result = await scanHostKey.mutateAsync({
      nodeId: node.id,
      input: {
        ssh_host: selectedMethod.ssh_host || node.address,
        ssh_port: selectedMethod.ssh_port || 22,
      },
    });
    setScanResult(result);
  };
  const firstFingerprint = scanResult?.fingerprints?.[0]?.fingerprint || '';
  const currentPin = selectedMethod?.ssh_host_key_sha256 || '';
  const changedPin = Boolean(firstFingerprint && currentPin && firstFingerprint !== currentPin);
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <h3 className="card-title">{t('nodes.enrollmentTokens')}</h3>
            <FormGrid>
              <FormField label={t('nodes.tokenTTL')}>
                <TextField type="number" min={1} max={720} value={ttlHours} onChange={(event) => setTTLHours(Number(event.target.value))} />
              </FormField>
            </FormGrid>
            <Toolbar>
              <Button variant="primary" icon={<KeyRound size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'enrollment-create', node, ttlHours: ttl })}>{t('nodes.createEnrollmentToken')}</Button>
              <Button icon={<RotateCcw size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'enrollment-rotate', node, ttlHours: ttl })}>{t('nodes.rotateEnrollmentToken')}</Button>
              <Button icon={<RefreshCw size={16} />} onClick={() => void tokens.refetch()}>{t('common.refresh')}</Button>
            </Toolbar>
            {tokens.isError ? <div role="alert" className="error-state-inline">{t('nodes.enrollmentTokensUnavailable')}: {formatAPIError(tokens.error)}</div> : null}
          </div>
        </CardBody>
      </Card>
      <DataTable
        rows={tokens.data || []}
        columns={[
          { key: 'hint', header: t('nodes.tokenHint'), render: (row) => <code>{text(row.token_hint)}</code> },
          { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
          { key: 'expires', header: t('nodes.expires'), render: (row) => fmt.date(row.expires_at) },
          { key: 'used', header: t('nodes.used'), render: (row) => fmt.date(row.used_at || undefined) },
          { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
          { key: 'actions', header: t('common.actions'), render: (row) => (
            <Button icon={<Trash2 size={16} />} variant="danger" disabled={busy || row.status === 'revoked'} onClick={() => onConfirm({ type: 'enrollment-revoke', node, token: row })}>
              {t('nodes.revokeToken')}
            </Button>
          ) },
        ]}
      />
      <Card>
        <CardBody>
          <div className="page-stack">
            <h3 className="card-title">{t('nodes.hostKeyTrust')}</h3>
            <FormGrid>
              <FormField label={t('nodes.sshAccessMethod')}>
                <Select value={selectedMethod?.id || ''} onChange={(event) => setMethodId(event.target.value)}>
                  {methods.map((method) => (
                    <option key={method.id} value={method.id}>
                      {[method.ssh_user, method.ssh_host || node.address, method.ssh_port || 22].filter(Boolean).join('@')}
                    </option>
                  ))}
                </Select>
              </FormField>
            </FormGrid>
            {!selectedMethod ? <div role="alert" className="error-state-inline">{t('nodes.sshAccessMissing')}</div> : null}
            <Toolbar>
              <Button variant="primary" icon={<Fingerprint size={16} />} disabled={busy || !selectedMethod} onClick={() => void runScan()}>{t('nodes.scanHostKey')}</Button>
              <Button icon={<RefreshCw size={16} />} onClick={() => void accessMethods.refetch()}>{t('common.refresh')}</Button>
            </Toolbar>
            {accessMethods.isError ? <div role="alert" className="error-state-inline">{t('nodes.accessMethodsUnavailable')}: {formatAPIError(accessMethods.error)}</div> : null}
            {scanHostKey.isError ? <div role="alert" className="error-state-inline">{formatAPIError(scanHostKey.error)}</div> : null}
            {changedPin ? <div role="alert" className="error-state-inline">{t('nodes.hostKeyChangedWarning')}</div> : null}
            {scanResult ? (
              <div className="page-stack">
                <div className="definition-grid">
                  <span>{t('nodes.scanHost')}</span><strong>{scanResult.host || selectedMethod?.ssh_host || node.address}</strong>
                  <span>{t('nodes.scanPort')}</span><strong>{scanResult.port || selectedMethod?.ssh_port || 22}</strong>
                  <span>{t('nodes.currentPin')}</span><strong>{currentPin || 'n/a'}</strong>
                </div>
                <DataTable
                  rows={scanResult.fingerprints || []}
                  columns={[
                    { key: 'fingerprint', header: t('nodes.fingerprint'), render: (row) => <code>{text(row.fingerprint)}</code> },
                    { key: 'algorithm', header: t('nodes.algorithm'), render: (row) => text(row.algorithm) },
                    { key: 'bits', header: t('nodes.bits'), render: (row) => String(row.bits ?? 'n/a') },
                    { key: 'actions', header: t('common.actions'), render: (row) => (
                      <Toolbar>
                        <Button icon={<ShieldCheck size={16} />} disabled={busy || !selectedMethod || !row.fingerprint} onClick={() => selectedMethod && row.fingerprint && onConfirm({ type: 'host-key-pin', node, method: selectedMethod, fingerprint: row.fingerprint })}>
                          {t('nodes.pinFingerprint')}
                        </Button>
                        <Button onClick={() => setScanResult(null)}>{t('nodes.rejectScan')}</Button>
                      </Toolbar>
                    ) },
                  ]}
                />
              </div>
            ) : null}
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardBody>
          <div className="page-stack">
            <h3 className="card-title">{t('nodes.agentTrust')}</h3>
            <div className="definition-grid">
              <span>{t('nodes.tokenHint')}</span><strong>{diagnostics?.agent?.token_hint || 'n/a'}</strong>
              <span>{t('nodes.tokenRotationStatus')}</span><strong>{diagnostics?.agent?.token_rotation_status || 'n/a'}</strong>
              <span>{t('nodes.lastAuthFailure')}</span><strong>{fmt.date(diagnostics?.agent?.last_auth_failure_at)}</strong>
            </div>
            <Toolbar>
              <Button variant="danger" icon={<ShieldAlert size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'agent-token-rotate', node })}>{t('nodes.rotateAgentToken')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function AccessMethodsTable({ methods }: { methods: NodeAccessMethod[] }) {
  const { t } = useTranslation();
  return (
    <DataTable
      rows={methods}
      title={t('nodes.accessMethods')}
      columns={[
        { key: 'method', header: t('nodes.method'), render: (row) => <code>{text(row.method)}</code> },
        { key: 'enabled', header: t('nodes.enabled'), render: (row) => row.is_enabled ? t('common.yes') : t('common.no') },
        { key: 'host', header: t('nodes.sshHost'), render: (row) => text(row.ssh_host) },
        { key: 'port', header: t('nodes.sshPort'), render: (row) => String(row.ssh_port || 'n/a') },
        { key: 'user', header: t('nodes.sshUser'), render: (row) => text(row.ssh_user) },
        { key: 'auth', header: t('nodes.authType'), render: (row) => text(row.auth_type) },
        { key: 'pin', header: t('nodes.currentPin'), render: (row) => <code>{text(row.ssh_host_key_sha256)}</code> },
        { key: 'secret', header: t('nodes.secretAvailable'), render: (row) => row.secret_ref_id ? t('common.yes') : t('common.no') },
      ]}
    />
  );
}

function TerminalTab({ node, accessMethods, busy, onConfirm }: {
  node: NodeDetail;
  accessMethods: ReturnType<typeof useNodeAccessMethods>;
  busy: boolean;
  onConfirm: (action: ConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const method = defaultSSHMethod(accessMethods.data);
  const canLaunch = Boolean(method?.is_enabled && method.secret_ref_id && method.ssh_host_key_sha256);
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <p className="muted">{t('nodes.terminalSecurityNote')}</p>
            {!canLaunch ? <div role="alert" className="error-state-inline">{t('nodes.terminalUnavailable')}</div> : null}
            <Toolbar>
              <Button variant="primary" icon={<TerminalSquare size={16} />} disabled={busy || !canLaunch} onClick={() => onConfirm({ type: 'ssh-session-launch', node })}>{t('nodes.launchSshSession')}</Button>
              <Button icon={<RefreshCw size={16} />} onClick={() => void accessMethods.refetch()}>{t('common.refresh')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      {accessMethods.isError ? <div role="alert" className="error-state-inline">{t('nodes.accessMethodsUnavailable')}: {formatAPIError(accessMethods.error)}</div> : null}
      <AccessMethodsTable methods={accessMethods.data || []} />
    </div>
  );
}

function LifecycleTab({ node, busy, onConfirm }: { node: NodeDetail; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const [confirmation, setConfirmation] = useState('');
  const [reason, setReason] = useState('');
  const expected = nodeLabel(node);
  const forceReady = confirmation.trim() === expected;
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <h3 className="card-title">{t('nodes.retireNode')}</h3>
            <p className="muted">{t('nodes.retireImpact')}</p>
            <Toolbar>
              <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy || node.status === 'retired'} onClick={() => onConfirm({ type: 'node-retire', node })}>{t('nodes.retireNode')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardBody>
          <div className="page-stack">
            <h3 className="card-title">{t('nodes.forceRetireNode')}</h3>
            <p className="muted">{t('nodes.forceRetireImpact', { node: expected })}</p>
            <FormGrid>
              <FormField label={t('nodes.forceRetireConfirmation')}>
                <TextField value={confirmation} onChange={(event) => setConfirmation(event.target.value)} placeholder={expected} />
              </FormField>
              <FormField label={t('nodes.forceRetireReason')}>
                <TextField value={reason} onChange={(event) => setReason(event.target.value)} />
              </FormField>
            </FormGrid>
            <Toolbar>
              <Button variant="danger" icon={<ShieldAlert size={16} />} disabled={busy || !forceReady || node.status === 'retired'} onClick={() => onConfirm({ type: 'node-force-retire', node, confirmation, reason })}>{t('nodes.forceRetireNode')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function NodeJobsTab({ jobIds, diagnostics }: { jobIds: string[]; diagnostics?: NodeDiagnostics }) {
  const { t } = useTranslation();
  const agent = diagnostics?.agent;
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <Link to="/operations/jobs">{t('nodes.openJobs')}</Link>
            {agent?.last_job_result_type ? <Badge>{agent.last_job_result_type}</Badge> : null}
            {agent?.last_job_result_status ? <StatusBadge status={agent.last_job_result_status} /> : null}
          </Toolbar>
        </CardBody>
      </Card>
      {jobIds.length ? jobIds.map((jobId) => <JobStatusPanel key={jobId} jobID={jobId} />) : <Card><CardBody>{t('nodes.noJob')}</CardBody></Card>}
    </div>
  );
}

function NodeConfirmDialog({ action, busy, onConfirm, onClose }: { action: ConfirmAction | null; busy: boolean; onConfirm: () => void; onClose: () => void }) {
  const { t } = useTranslation();
  if (!action) return null;
  const operation = t(`nodes.confirmOperations.${action.type}`);
  const service = action.type === 'capability-install'
    ? action.input.service_code
    : action.type === 'capability-verify'
      ? action.serviceCode
      : action.type === 'service-import'
        ? action.discovery.service_code || action.discovery.name || action.discovery.id
        : undefined;
  const dangerous = [
    'maintenance-enable',
    'enrollment-revoke',
    'agent-token-rotate',
    'node-retire',
    'node-force-retire',
  ].includes(action.type);
  return (
    <ConfirmDialog title={t('nodes.confirmTitle', { operation })} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{t('nodes.confirmImpact', {
          operation,
          node: nodeLabel(action.node),
          status: action.node.status || 'n/a',
          service: service || 'n/a',
        })}</p>
        {action.type === 'diagnostics' && action.action === 'reconcile-runtime' ? <p>{t('nodes.reconcileRuntimeImpact')}</p> : null}
        {action.type === 'capability-install' ? <pre className="code-block">{safeJSON(action.input)}</pre> : null}
        {(action.type === 'bootstrap' || action.type === 'agent-reinstall') ? <pre className="code-block">{safeJSON(action.input)}</pre> : null}
        {action.type === 'enrollment-revoke' ? <pre className="code-block">{safeJSON({ token_id: action.token.id, token_hint: action.token.token_hint, status: action.token.status })}</pre> : null}
        {action.type === 'host-key-pin' ? <pre className="code-block">{safeJSON({ method_id: action.method.id, ssh_host: action.method.ssh_host, ssh_port: action.method.ssh_port, fingerprint: action.fingerprint })}</pre> : null}
        {action.type === 'node-force-retire' ? <pre className="code-block">{safeJSON({ confirmation: action.confirmation, reason: action.reason || 'operator force retire' })}</pre> : null}
        {action.type === 'service-import' ? <pre className="code-block">{safeJSON({ id: action.discovery.id, service_code: action.discovery.service_code, name: action.discovery.name, endpoint: [action.discovery.endpoint_host, action.discovery.endpoint_port].filter(Boolean).join(':') })}</pre> : null}
        <Toolbar>
          <Button variant={dangerous ? 'danger' : 'primary'} disabled={busy} onClick={onConfirm}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
