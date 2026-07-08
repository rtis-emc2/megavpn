import { Activity, Boxes, CheckCircle2, DownloadCloud, PackageCheck, Play, RefreshCw, Search, ServerCog, ShieldCheck, Wrench } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type {
  APIRecord,
  Job,
  NodeCapability,
  NodeCapabilityInstallInput,
  NodeDetail,
  NodeDiagnostics,
  NodeDiagnosticsAction,
  NodeMutationResult,
  NodeServiceDiscovery,
  NodeServiceInstaller,
} from '../../shared/api/types';
import {
  useDiscoverNodeServices,
  useImportAllNodeServiceDiscoveries,
  useImportNodeServiceDiscovery,
  useInstallNodeCapability,
  useNodeCapabilities,
  useNodeCapabilityDrift,
  useNodeCapabilityInstallEvents,
  useNodeDetail,
  useNodeDiagnostics,
  useNodeInventory,
  useNodeServiceDiscoveries,
  useNodeServiceDiscoverySummary,
  useNodeServiceInstallers,
  useNodes,
  useRunNodeDiagnosticsAction,
  useSetNodeMaintenance,
  useSyncNodeInventory,
  useVerifyNodeCapability,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Select, StatusBadge, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type NodeTab = 'overview' | 'runtime' | 'inventory' | 'capabilities' | 'diagnostics' | 'discovery' | 'jobs';

type ConfirmAction =
  | { type: 'maintenance-enable'; node: NodeDetail }
  | { type: 'maintenance-disable'; node: NodeDetail }
  | { type: 'inventory-sync'; node: NodeDetail }
  | { type: 'capability-install'; node: NodeDetail; input: NodeCapabilityInstallInput }
  | { type: 'capability-verify'; node: NodeDetail; serviceCode: string }
  | { type: 'diagnostics'; node: NodeDetail; action: NodeDiagnosticsAction }
  | { type: 'service-discover'; node: NodeDetail }
  | { type: 'service-import'; node: NodeDetail; discovery: NodeServiceDiscovery }
  | { type: 'service-import-all'; node: NodeDetail };

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) {
    return error instanceof Error ? error.message : 'Request failed';
  }
  const prefix = error.status === 403
    ? 'Permission denied'
    : error.status === 409
      ? 'Conflict'
      : error.status === 422
        ? 'Validation failed'
        : `HTTP ${error.status}`;
  return `${prefix}: ${error.message}`;
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
  const nodes = useNodes();
  const [selectedId, setSelectedId] = useState('');
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
      actions={<Button disabled title={t('common.permissionRequired', { permission: 'node.write' })}>{t('nodes.createNode')}</Button>}
    >
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

function NodeDrawer({ node, nodeId, open, onClose }: { node?: NodeDetail; nodeId: string; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<NodeTab>('overview');
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);
  const [notice, setNotice] = useState('');
  const [jobIds, setJobIds] = useState<string[]>([]);
  const detail = useNodeDetail(nodeId, { retry: false });
  const diagnostics = useNodeDiagnostics(nodeId, { retry: false });
  const inventory = useNodeInventory(nodeId, { retry: false });
  const capabilities = useNodeCapabilities(nodeId, { retry: false });
  const capabilityDrift = useNodeCapabilityDrift(nodeId, { retry: false });
  const capabilityEvents = useNodeCapabilityInstallEvents(nodeId, { retry: false });
  const installers = useNodeServiceInstallers({ retry: false });
  const discoveries = useNodeServiceDiscoveries(nodeId, { retry: false });
  const discoverySummary = useNodeServiceDiscoverySummary(nodeId, { retry: false });
  const maintenance = useSetNodeMaintenance();
  const syncInventory = useSyncNodeInventory();
  const installCapability = useInstallNodeCapability();
  const verifyCapability = useVerifyNodeCapability();
  const runDiagnostics = useRunNodeDiagnosticsAction();
  const discoverServices = useDiscoverNodeServices();
  const importDiscovery = useImportNodeServiceDiscovery();
  const importAllDiscoveries = useImportAllNodeServiceDiscoveries();
  const current = detail.data || node;
  const busy = maintenance.isPending || syncInventory.isPending || installCapability.isPending || verifyCapability.isPending || runDiagnostics.isPending || discoverServices.isPending || importDiscovery.isPending || importAllDiscoveries.isPending;

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
              {(['overview', 'runtime', 'inventory', 'capabilities', 'diagnostics', 'discovery', 'jobs'] as NodeTab[]).map((tab) => (
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
            {activeTab === 'overview' ? <OverviewTab node={current} busy={busy} onConfirm={setConfirm} /> : null}
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
            {activeTab === 'jobs' ? <NodeJobsTab jobIds={effectiveJobIds} diagnostics={diagnostics.data} /> : null}
            <NodeConfirmDialog action={confirm} busy={busy} onConfirm={() => void runConfirmed()} onClose={() => setConfirm(null)} />
          </div>
        ) : null}
      </QueryBoundary>
    </Drawer>
  );
}

function OverviewTab({ node, busy, onConfirm }: { node: NodeDetail; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
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
        {action.type === 'service-import' ? <pre className="code-block">{safeJSON({ id: action.discovery.id, service_code: action.discovery.service_code, name: action.discovery.name, endpoint: [action.discovery.endpoint_host, action.discovery.endpoint_port].filter(Boolean).join(':') })}</pre> : null}
        <Toolbar>
          <Button variant={action.type === 'maintenance-enable' ? 'danger' : 'primary'} disabled={busy} onClick={onConfirm}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
