import { Activity, FilePenLine, LinkIcon, PackagePlus, Play, Power, RefreshCw, RotateCcw, Search, Square, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { APIError } from '../../shared/api/client';
import type { APIRecord, InstanceLifecycleAction, Job, NodeEntity, ServiceInstance, ServiceInstanceRuntimeState, ServiceTypeCapability } from '../../shared/api/types';
import {
  useApplyInstance,
  useCreateInstanceManual,
  useDeleteInstance,
  useForceDeleteInstance,
  useInstanceAccessGroups,
  useInstanceDetail,
  useInstanceDiagnostics,
  useInstanceLifecycleAction,
  useInstanceRevisions,
  useInstanceRuntimeState,
  useInstanceRuntimeStates,
  useInstances,
  useNodes,
  useReapplyInstance,
  useReplaceInstanceSpec,
  useServiceTypeCapabilities,
  useRollbackInstance,
  useRunInstanceDiagnostics,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, Checkbox, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Select, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';
import { ServicesTabs } from './ServicesTabs';

type InstanceTab = 'overview' | 'runtime' | 'spec' | 'revisions' | 'diagnostics' | 'access-groups' | 'jobs';

type ConfirmAction =
  | { type: 'apply'; instance: ServiceInstance }
  | { type: 'reapply'; instance: ServiceInstance }
  | { type: 'lifecycle'; instance: ServiceInstance; action: InstanceLifecycleAction }
  | { type: 'diagnose'; instance: ServiceInstance }
  | { type: 'replace-spec'; instance: ServiceInstance; spec: APIRecord; applyAfterSave: boolean }
  | { type: 'rollback'; instance: ServiceInstance; revisionId: string }
  | { type: 'delete'; instance: ServiceInstance }
  | { type: 'force-delete'; instance: ServiceInstance; confirmation: string; reason: string };

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

function endpoint(instance: ServiceInstance): string {
  return [instance.endpoint_host, instance.endpoint_port].filter(Boolean).join(':') || 'n/a';
}

function instanceLabel(instance?: ServiceInstance | null): string {
  if (!instance) return 'n/a';
  return text(instance.name || instance.slug || instance.id);
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

function runtimeFor(instanceId: string, states: ServiceInstanceRuntimeState[] | undefined): ServiceInstanceRuntimeState | undefined {
  return (states || []).find((state) => state.instance_id === instanceId);
}

export function InstancesPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const navigate = useNavigate();
  const instances = useInstances();
  const runtimeStates = useInstanceRuntimeStates();
  const nodes = useNodes({ retry: false });
  const serviceTypes = useServiceTypeCapabilities({ retry: false });
  const [selectedId, setSelectedId] = useState('');
  const [manualCreateOpen, setManualCreateOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [serviceFilter, setServiceFilter] = useState('all');
  const [nodeFilter, setNodeFilter] = useState('all');

  const nodeName = (id?: string) => nodes.data?.find((node) => node.id === id)?.name || id || 'n/a';
  const rows = instances.data || [];
  const services = Array.from(new Set(rows.map((row) => row.service_code).filter(Boolean))).sort();
  const statuses = Array.from(new Set(rows.map((row) => row.status).filter(Boolean))).sort();
  const filteredRows = rows.filter((row) => {
    const haystack = `${row.name || ''} ${row.slug || ''} ${row.id} ${row.service_code || ''} ${nodeName(row.node_id)} ${row.status || ''}`.toLowerCase();
    return (!search || haystack.includes(search.toLowerCase()))
      && (statusFilter === 'all' || row.status === statusFilter)
      && (serviceFilter === 'all' || row.service_code === serviceFilter)
      && (nodeFilter === 'all' || row.node_id === nodeFilter);
  });
  const selected = rows.find((row) => row.id === selectedId);

  return (
    <PageScaffold
      title={t('instances.title')}
      subtitle={t('instances.subtitle')}
      actions={<><Button icon={<PackagePlus size={16} />} onClick={() => navigate('/services/service-packs')}>{t('instances.createFromPack')}</Button><Button icon={<FilePenLine size={16} />} onClick={() => setManualCreateOpen(true)}>{t('instances.manualInstance')}</Button></>}
    >
      <ServicesTabs />
      <Toolbar>
        <Badge>{t('instances.applyExplicit')}</Badge>
        <Badge>{t('instances.accessGroupsReadOnly')} <Link to="/clients/groups">{t('clients.openGroups')}</Link></Badge>
      </Toolbar>
      <Card>
        <CardBody>
          <Toolbar>
            <FormField label={t('common.search')}>
              <div className="input-with-icon">
                <Search size={16} />
                <TextField value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t('instances.searchPlaceholder')} />
              </div>
            </FormField>
            <FormField label={t('common.status')}>
              <Select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
                <option value="all">{t('common.all')}</option>
                {statuses.map((status) => <option key={status} value={status}>{status}</option>)}
              </Select>
            </FormField>
            <FormField label={t('instances.service')}>
              <Select value={serviceFilter} onChange={(event) => setServiceFilter(event.target.value)}>
                <option value="all">{t('common.all')}</option>
                {services.map((service) => <option key={service} value={service}>{service}</option>)}
              </Select>
            </FormField>
            <FormField label={t('instances.node')}>
              <Select value={nodeFilter} onChange={(event) => setNodeFilter(event.target.value)}>
                <option value="all">{t('common.all')}</option>
                {(nodes.data || []).map((node) => <option key={node.id} value={node.id}>{node.name || node.id}</option>)}
              </Select>
            </FormField>
            <Button icon={<RefreshCw size={16} />} onClick={() => { void instances.refetch(); void runtimeStates.refetch(); }}>{t('common.refresh')}</Button>
          </Toolbar>
        </CardBody>
      </Card>
      <QueryBoundary
        isLoading={instances.isLoading || runtimeStates.isLoading}
        isError={instances.isError || runtimeStates.isError}
        error={instances.error || runtimeStates.error}
        refetch={() => { void instances.refetch(); void runtimeStates.refetch(); }}
      >
        <DataTable
          rows={filteredRows}
          columns={[
            { key: 'name', header: t('instances.instance'), render: (row) => <strong>{instanceLabel(row)}</strong> },
            { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'node', header: t('instances.node'), render: (row) => text(nodeName(row.node_id)) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'runtime', header: t('instances.runtimeState'), render: (row) => <StatusBadge status={runtimeFor(row.id, runtimeStates.data)?.runtime_status || 'unknown'} /> },
            { key: 'health', header: t('instances.health'), render: (row) => <StatusBadge status={runtimeFor(row.id, runtimeStates.data)?.health_status || 'unknown'} /> },
            { key: 'revision', header: t('instances.revision'), render: (row) => <code>{shortID(row.last_applied_revision_id || row.current_revision_id)}</code> },
            { key: 'job', header: t('instances.lastJob'), render: (row) => <code>{shortID(runtimeFor(row.id, runtimeStates.data)?.last_job_id)}</code> },
            { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button onClick={() => setSelectedId(row.id)}>{t('common.open')}</Button> },
          ]}
        />
      </QueryBoundary>
      <InstanceDrawer
        instance={selected}
        nodeName={nodeName}
        open={Boolean(selectedId)}
        onClose={() => setSelectedId('')}
      />
      <ManualInstanceDrawer
        key={manualCreateOpen ? `open:${nodes.data?.[0]?.id || ''}:${serviceTypes.data?.map((service) => service.code).join(',') || ''}` : 'closed'}
        open={manualCreateOpen}
        onClose={() => setManualCreateOpen(false)}
        nodes={nodes.data || []}
        serviceTypes={serviceTypes.data || []}
      />
    </PageScaffold>
  );
}

function InstanceDrawer({ instance, nodeName, open, onClose }: { instance?: ServiceInstance; nodeName: (id?: string) => string; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<InstanceTab>('overview');
  const [confirm, setConfirm] = useState<ConfirmAction | null>(null);
  const [rollbackRevisionId, setRollbackRevisionId] = useState('');
  const [forceConfirmation, setForceConfirmation] = useState('');
  const [forceReason, setForceReason] = useState('');
  const [notice, setNotice] = useState('');
  const [jobId, setJobId] = useState('');
  const detail = useInstanceDetail(instance?.id, { retry: false });
  const runtime = useInstanceRuntimeState(instance?.id, { retry: false });
  const revisions = useInstanceRevisions(instance?.id, { retry: false });
  const diagnostics = useInstanceDiagnostics(instance?.id, { retry: false });
  const accessGroups = useInstanceAccessGroups(instance?.id, { retry: false });
  const apply = useApplyInstance();
  const reapply = useReapplyInstance();
  const rollback = useRollbackInstance();
  const replaceSpec = useReplaceInstanceSpec();
  const diagnose = useRunInstanceDiagnostics();
  const lifecycle = useInstanceLifecycleAction();
  const deleteMutation = useDeleteInstance();
  const forceDelete = useForceDeleteInstance();
  const current = detail.data || instance;
  const busy = apply.isPending || reapply.isPending || rollback.isPending || replaceSpec.isPending || diagnose.isPending || lifecycle.isPending || deleteMutation.isPending || forceDelete.isPending;
  const lastJobId = jobId || runtime.data?.last_job_id || '';

  const recordJob = (job?: Job) => {
    if (job?.id) setJobId(job.id);
  };

  const runConfirmed = async () => {
    if (!confirm || !current) return;
    setNotice('');
    try {
      if (confirm.type === 'apply') {
        recordJob(await apply.mutateAsync({ instanceId: confirm.instance.id }));
        setNotice(t('instances.actionAccepted'));
      } else if (confirm.type === 'reapply') {
        recordJob(await reapply.mutateAsync({ instanceId: confirm.instance.id }));
        setNotice(t('instances.actionAccepted'));
      } else if (confirm.type === 'lifecycle') {
        recordJob(await lifecycle.mutateAsync({ instanceId: confirm.instance.id, action: confirm.action }));
        setNotice(t('instances.actionAccepted'));
      } else if (confirm.type === 'diagnose') {
        recordJob(await diagnose.mutateAsync({ instanceId: confirm.instance.id }));
        setNotice(t('instances.actionAccepted'));
        setActiveTab('diagnostics');
      } else if (confirm.type === 'replace-spec') {
        const result = await replaceSpec.mutateAsync({ instanceId: confirm.instance.id, input: { spec: confirm.spec, apply_after_save: confirm.applyAfterSave } });
        if (confirm.applyAfterSave && result.can_apply) {
          recordJob(await apply.mutateAsync({ instanceId: confirm.instance.id }));
        }
        setNotice(result.message || t('instances.specReplaceAccepted'));
        setActiveTab(confirm.applyAfterSave ? 'jobs' : 'revisions');
      } else if (confirm.type === 'rollback') {
        const result = await rollback.mutateAsync({ instanceId: confirm.instance.id, input: { revision_id: confirm.revisionId } });
        if (result.can_apply) {
          recordJob(await apply.mutateAsync({ instanceId: confirm.instance.id }));
        }
        setNotice(result.message || t('instances.rollbackAccepted'));
        setActiveTab('jobs');
      } else if (confirm.type === 'delete') {
        await deleteMutation.mutateAsync({ instanceId: confirm.instance.id });
        setNotice(t('instances.deleteAccepted'));
      } else if (confirm.type === 'force-delete') {
        await forceDelete.mutateAsync({ instanceId: confirm.instance.id, input: { confirmation: confirm.confirmation, reason: confirm.reason } });
        setNotice(t('instances.forceDeleteAccepted'));
      }
      setConfirm(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <Drawer title={instanceLabel(current)} open={open} onClose={onClose}>
      <QueryBoundary isLoading={detail.isLoading} isError={detail.isError} error={detail.error} refetch={() => void detail.refetch()}>
        <div className="page-stack">
          <div className="tabs" role="tablist" aria-label={t('instances.detailTabs')}>
            {(['overview', 'runtime', 'spec', 'revisions', 'diagnostics', 'access-groups', 'jobs'] as InstanceTab[]).map((tab) => (
              <button
                key={tab}
                className={`tab-link ${activeTab === tab ? 'active' : ''}`.trim()}
                role="tab"
                aria-selected={activeTab === tab}
                type="button"
                onClick={() => setActiveTab(tab)}
              >
                {t(`instances.tabs.${tab}`)}
              </button>
            ))}
          </div>
          {notice ? <div role="status">{notice}</div> : null}
          {current && activeTab === 'overview' ? (
            <OverviewTab instance={current} nodeName={nodeName} runtime={runtime.data} onConfirm={setConfirm} busy={busy} />
          ) : null}
          {current && activeTab === 'runtime' ? (
            <RuntimeTab instance={current} runtime={runtime} onConfirm={setConfirm} busy={busy} />
          ) : null}
          {current && activeTab === 'spec' ? (
            <SpecTab key={`${current.id}:${current.current_revision_id || ''}:${current.updated_at || ''}`} instance={current} onConfirm={setConfirm} busy={busy} />
          ) : null}
          {current && activeTab === 'revisions' ? (
            <RevisionsTab
              instance={current}
              revisions={revisions}
              selectedRevisionId={rollbackRevisionId}
              setSelectedRevisionId={setRollbackRevisionId}
              onConfirm={setConfirm}
              busy={busy}
            />
          ) : null}
          {current && activeTab === 'diagnostics' ? (
            <DiagnosticsTab instance={current} diagnostics={diagnostics} onConfirm={setConfirm} busy={busy} />
          ) : null}
          {current && activeTab === 'access-groups' ? (
            <AccessGroupsTab accessGroups={accessGroups} />
          ) : null}
          {current && activeTab === 'jobs' ? (
            <JobsTab jobId={lastJobId} runtime={runtime.data} />
          ) : null}
          {current ? (
            <Card>
              <CardBody>
                <div className="page-stack">
                  <h3 className="card-title">{t('instances.dangerZone')}</h3>
                  <FormGrid>
                    <FormField label={t('instances.forceConfirmation')}>
                      <TextField value={forceConfirmation} onChange={(event) => setForceConfirmation(event.target.value)} placeholder={`DELETE ${current.slug || current.name || current.id}`} />
                    </FormField>
                    <FormField label={t('instances.forceReason')}>
                      <TextField value={forceReason} onChange={(event) => setForceReason(event.target.value)} />
                    </FormField>
                  </FormGrid>
                  <Toolbar>
                    <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy} onClick={() => setConfirm({ type: 'delete', instance: current })}>{t('instances.deleteInstance')}</Button>
                    <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy || !forceConfirmation} onClick={() => setConfirm({ type: 'force-delete', instance: current, confirmation: forceConfirmation, reason: forceReason })}>{t('instances.forceDelete')}</Button>
                  </Toolbar>
                </div>
              </CardBody>
            </Card>
          ) : null}
          <InstanceConfirmDialog action={confirm} busy={busy} nodeName={nodeName} onConfirm={() => void runConfirmed()} onClose={() => setConfirm(null)} />
        </div>
      </QueryBoundary>
    </Drawer>
  );
}

function OverviewTab({ instance, runtime, nodeName, busy, onConfirm }: { instance: ServiceInstance; runtime?: ServiceInstanceRuntimeState; nodeName: (id?: string) => string; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <div className="definition-grid">
            <span>{t('common.id')}</span><strong>{instance.id}</strong>
            <span>{t('instances.service')}</span><strong>{instance.service_code || 'n/a'}</strong>
            <span>{t('instances.node')}</span><strong>{nodeName(instance.node_id)}</strong>
            <span>{t('instances.systemdUnit')}</span><strong>{instance.systemd_unit || 'n/a'}</strong>
            <span>{t('instances.endpoint')}</span><strong>{endpoint(instance)}</strong>
            <span>{t('common.status')}</span><strong>{instance.status || 'n/a'}</strong>
            <span>{t('instances.runtimeState')}</span><strong>{runtime?.runtime_status || 'n/a'}</strong>
            <span>{t('instances.health')}</span><strong>{runtime?.health_status || 'n/a'}</strong>
            <span>{t('common.created')}</span><strong>{fmt.date(instance.created_at)}</strong>
            <span>{t('common.updated')}</span><strong>{fmt.date(instance.updated_at)}</strong>
          </div>
          <Toolbar>
            <Button variant="primary" icon={<Play size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'apply', instance })}>{t('common.apply')}</Button>
            <Button icon={<RefreshCw size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'reapply', instance })}>{t('instances.reapply')}</Button>
          </Toolbar>
        </div>
      </CardBody>
    </Card>
  );
}

function RuntimeTab({ instance, runtime, busy, onConfirm }: { instance: ServiceInstance; runtime: ReturnType<typeof useInstanceRuntimeState>; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const state = runtime.data;
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <Button variant="primary" icon={<Play size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'apply', instance })}>{t('common.apply')}</Button>
              <Button icon={<RefreshCw size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'reapply', instance })}>{t('instances.reapply')}</Button>
              <Button icon={<RotateCcw size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'lifecycle', instance, action: 'restart' })}>{t('instances.restart')}</Button>
              <Button icon={<Play size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'lifecycle', instance, action: 'start' })}>{t('instances.start')}</Button>
              <Button icon={<Square size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'lifecycle', instance, action: 'stop' })}>{t('instances.stop')}</Button>
              <Button icon={<Power size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'lifecycle', instance, action: 'enable' })}>{t('instances.enable')}</Button>
              <Button icon={<Power size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'lifecycle', instance, action: 'disable' })}>{t('instances.disable')}</Button>
            </Toolbar>
            {runtime.isError ? <div role="alert" className="error-state-inline">{t('instances.runtimeUnavailable')}: {formatAPIError(runtime.error)}</div> : null}
            {state ? (
              <div className="definition-grid">
                <span>{t('instances.desiredStatus')}</span><strong>{state.desired_status || 'n/a'}</strong>
                <span>{t('instances.runtimeState')}</span><strong>{state.runtime_status || 'n/a'}</strong>
                <span>{t('instances.activeState')}</span><strong>{state.active_state || 'n/a'}</strong>
                <span>{t('instances.enabledState')}</span><strong>{state.enabled_state || 'n/a'}</strong>
                <span>{t('instances.drift')}</span><strong>{state.drift_status || 'n/a'}</strong>
                <span>{t('instances.configHash')}</span><strong>{shortID(state.config_hash)}</strong>
                <span>{t('instances.lastJob')}</span><strong>{shortID(state.last_job_id)}</strong>
                <span>{t('instances.lastJobStatus')}</span><strong>{state.last_job_status || 'n/a'}</strong>
                <span>{t('instances.checkedAt')}</span><strong>{fmt.date(state.checked_at)}</strong>
              </div>
            ) : null}
          </div>
        </CardBody>
      </Card>
      <RuntimeDiagnostics state={state} />
    </div>
  );
}

function RuntimeDiagnostics({ state }: { state?: ServiceInstanceRuntimeState }) {
  const { t } = useTranslation();
  if (!state) return null;
  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <h3 className="card-title">{t('instances.runtimeOutput')}</h3>
          <pre className="code-block">{safeJSON({ health_reasons: state.health_reasons || [], drift_reasons: state.drift_reasons || [], error_text: state.error_text || '', result: state.result || {}, listening_ports: state.listening_ports || [] })}</pre>
          {state.health_checks?.length ? (
            <DataTable
              rows={state.health_checks}
              columns={[
                { key: 'code', header: t('common.id'), render: (row) => <code>{row.code || row.signal || 'n/a'}</code> },
                { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
                { key: 'message', header: t('common.description'), render: (row) => text(row.message) },
              ]}
            />
          ) : null}
        </div>
      </CardBody>
    </Card>
  );
}

function SpecTab({ instance, busy, onConfirm }: { instance: ServiceInstance; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(() => safeJSON(instance.spec || {}));
  const [applyAfterSave, setApplyAfterSave] = useState(false);
  const [error, setError] = useState('');

  const submit = () => {
    setError('');
    let spec: APIRecord;
    try {
      const parsed = JSON.parse(draft || '{}');
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error(t('instances.specMustBeObject'));
      }
      spec = parsed as APIRecord;
    } catch (parseError) {
      setError(parseError instanceof Error ? parseError.message : t('instances.invalidJSON'));
      return;
    }
    onConfirm({ type: 'replace-spec', instance, spec, applyAfterSave });
  };

  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <Toolbar>
            <Badge>{t('instances.backendValidated')}</Badge>
            <Badge>{t('common.readOnly')} {t('instances.noHtmlRendering')}</Badge>
          </Toolbar>
          <FormField label={t('instances.specJSON')} full>
            <Textarea value={draft} rows={14} spellCheck={false} onChange={(event) => setDraft(event.target.value)} />
          </FormField>
          <Checkbox label={t('instances.applyAfterSpecSave')} checked={applyAfterSave} onChange={(event) => setApplyAfterSave(event.target.checked)} />
          {error ? <div role="alert" className="error-state-inline">{error}</div> : null}
          <Toolbar>
            <Button variant="primary" icon={<FilePenLine size={16} />} disabled={busy} onClick={submit}>{t('instances.replaceSpec')}</Button>
          </Toolbar>
        </div>
      </CardBody>
    </Card>
  );
}

function RevisionsTab({ instance, revisions, selectedRevisionId, setSelectedRevisionId, busy, onConfirm }: {
  instance: ServiceInstance;
  revisions: ReturnType<typeof useInstanceRevisions>;
  selectedRevisionId: string;
  setSelectedRevisionId: (value: string) => void;
  busy: boolean;
  onConfirm: (action: ConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const rows = revisions.data || [];
  return (
    <QueryBoundary isLoading={revisions.isLoading} isError={revisions.isError} error={revisions.error} refetch={() => void revisions.refetch()}>
      <div className="page-stack">
        <Card>
          <CardBody>
            <Toolbar>
              <FormField label={t('instances.rollbackTarget')}>
                <Select value={selectedRevisionId} onChange={(event) => setSelectedRevisionId(event.target.value)}>
                  <option value="">{t('instances.selectRevision')}</option>
                  {rows.map((revision) => (
                    <option key={revision.id} value={revision.id}>#{revision.revision_no || '?'} {revision.id}</option>
                  ))}
                </Select>
              </FormField>
              <Button variant="danger" icon={<RotateCcw size={16} />} disabled={busy || !selectedRevisionId} onClick={() => onConfirm({ type: 'rollback', instance, revisionId: selectedRevisionId })}>{t('instances.rollback')}</Button>
            </Toolbar>
          </CardBody>
        </Card>
        <DataTable
          rows={rows}
          columns={[
            { key: 'revision', header: t('instances.revision'), render: (row) => <code>{row.revision_no || shortID(row.id)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'hash', header: t('instances.configHash'), render: (row) => <code>{shortID(row.rendered_hash)}</code> },
            { key: 'current', header: t('instances.current'), render: (row) => row.is_current ? <Badge>{t('instances.current')}</Badge> : null },
            { key: 'applied', header: t('instances.lastApplied'), render: (row) => row.is_last_applied ? <Badge>{t('instances.lastApplied')}</Badge> : fmt.date(row.applied_at) },
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
          ]}
        />
      </div>
    </QueryBoundary>
  );
}

function DiagnosticsTab({ instance, diagnostics, busy, onConfirm }: { instance: ServiceInstance; diagnostics: ReturnType<typeof useInstanceDiagnostics>; busy: boolean; onConfirm: (action: ConfirmAction) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <Button variant="primary" icon={<Activity size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'diagnose', instance })}>{t('instances.runDiagnostics')}</Button>
            <Button icon={<RefreshCw size={16} />} onClick={() => void diagnostics.refetch()}>{t('common.refresh')}</Button>
          </Toolbar>
        </CardBody>
      </Card>
      <QueryBoundary isLoading={diagnostics.isLoading} isError={diagnostics.isError} error={diagnostics.error} refetch={() => void diagnostics.refetch()}>
        <DataTable
          rows={diagnostics.data || []}
          columns={[
            { key: 'source', header: t('common.type'), render: (row) => text(row.source) },
            { key: 'runtime', header: t('instances.runtimeState'), render: (row) => <StatusBadge status={row.runtime_status} /> },
            { key: 'health', header: t('instances.health'), render: (row) => <StatusBadge status={row.health_status} /> },
            { key: 'drift', header: t('instances.drift'), render: (row) => <StatusBadge status={row.drift_status} /> },
            { key: 'observed', header: t('instances.observedAt'), render: (row) => fmt.date(row.observed_at || row.checked_at) },
            { key: 'details', header: t('common.description'), render: (row) => <pre className="code-block">{safeJSON({ health_reasons: row.health_reasons || [], drift_reasons: row.drift_reasons || [], error_text: row.error_text || '', result: row.result || {} })}</pre> },
          ]}
        />
      </QueryBoundary>
    </div>
  );
}

function AccessGroupsTab({ accessGroups }: { accessGroups: ReturnType<typeof useInstanceAccessGroups> }) {
  const { t } = useTranslation();
  const rows = accessGroups.data?.groups || [];
  return (
    <QueryBoundary isLoading={accessGroups.isLoading} isError={accessGroups.isError} error={accessGroups.error} refetch={() => void accessGroups.refetch()}>
      <div className="page-stack">
        <Card>
          <CardBody>
            <Toolbar>
              <Badge>{t('common.readOnly')}</Badge>
              <Button icon={<LinkIcon size={16} />}><Link to="/clients/groups">{t('instances.manageInClientGroups')}</Link></Button>
            </Toolbar>
            <p className="muted">{t('instances.accessGroupsReadonlyNote')}</p>
          </CardBody>
        </Card>
        <DataTable
          rows={rows}
          columns={[
            { key: 'key', header: t('clients.groups.groupKey'), render: (row) => <code>{text(row.key)}</code> },
            { key: 'label', header: t('common.name'), render: (row) => <strong>{text(row.label)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'unknown')} /> },
            { key: 'members', header: t('instances.members'), render: (row) => String(row.member_count || 0) },
            { key: 'pending', header: t('instances.pending'), render: (row) => String(row.pending_count || 0) },
            { key: 'active', header: t('instances.active'), render: (row) => String(row.active_count || 0) },
          ]}
        />
      </div>
    </QueryBoundary>
  );
}

function JobsTab({ jobId, runtime }: { jobId: string; runtime?: ServiceInstanceRuntimeState }) {
  const { t } = useTranslation();
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <Toolbar>
            <Link to="/operations/jobs">{t('instances.openJobs')}</Link>
            {runtime?.last_job_type ? <Badge>{runtime.last_job_type}</Badge> : null}
            {runtime?.last_job_status ? <StatusBadge status={runtime.last_job_status} /> : null}
          </Toolbar>
        </CardBody>
      </Card>
      {jobId ? <JobStatusPanel jobID={jobId} /> : <Card><CardBody>{t('instances.noJob')}</CardBody></Card>}
    </div>
  );
}

function ManualInstanceDrawer({ open, onClose, nodes, serviceTypes }: { open: boolean; onClose: () => void; nodes: NodeEntity[]; serviceTypes: ServiceTypeCapability[] }) {
  const { t } = useTranslation();
  const createManual = useCreateInstanceManual();
  const [nodeId, setNodeId] = useState(() => nodes[0]?.id || '');
  const [serviceCode, setServiceCode] = useState(() => serviceTypes.find((service) => service.supports_instances !== false)?.code || '');
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [endpointHost, setEndpointHost] = useState('');
  const [endpointPort, setEndpointPort] = useState('');
  const [specText, setSpecText] = useState('{}');
  const [notice, setNotice] = useState('');
  const [created, setCreated] = useState<ServiceInstance | null>(null);

  const submit = async () => {
    setNotice('');
    setCreated(null);
    let spec: APIRecord;
    try {
      const parsed = JSON.parse(specText || '{}');
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error(t('instances.specMustBeObject'));
      }
      spec = parsed as APIRecord;
    } catch (error) {
      setNotice(error instanceof Error ? error.message : t('instances.invalidJSON'));
      return;
    }
    try {
      const result = await createManual.mutateAsync({
        node_id: nodeId,
        service_code: serviceCode,
        name,
        slug,
        endpoint_host: endpointHost,
        endpoint_port: endpointPort ? Number(endpointPort) : undefined,
        spec,
      });
      if ('id' in result && typeof result.id === 'string') setCreated(result as ServiceInstance);
      setNotice(t('instances.manualCreateAccepted'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const canSubmit = Boolean(nodeId && serviceCode && name && !createManual.isPending);

  return (
    <Drawer title={t('instances.manualInstance')} open={open} onClose={onClose}>
      <div className="page-stack">
        <ServicesTabs />
        <Badge>{t('instances.manualCreateBackendValidated')}</Badge>
        <FormGrid>
          <FormField label={t('instances.node')}>
            <Select value={nodeId} onChange={(event) => setNodeId(event.target.value)}>
              <option value="">{t('instances.selectNode')}</option>
              {nodes.map((node) => <option key={node.id} value={node.id}>{node.name || node.id}</option>)}
            </Select>
          </FormField>
          <FormField label={t('instances.service')}>
            <Select value={serviceCode} onChange={(event) => setServiceCode(event.target.value)}>
              <option value="">{t('instances.selectService')}</option>
              {serviceTypes.filter((service) => service.supports_instances !== false).map((service) => (
                <option key={service.code} value={service.code}>{service.name || service.code}</option>
              ))}
            </Select>
          </FormField>
          <FormField label={t('common.name')}>
            <TextField value={name} onChange={(event) => setName(event.target.value)} />
          </FormField>
          <FormField label={t('instances.slug')}>
            <TextField value={slug} onChange={(event) => setSlug(event.target.value)} />
          </FormField>
          <FormField label={t('instances.endpointHost')}>
            <TextField value={endpointHost} onChange={(event) => setEndpointHost(event.target.value)} />
          </FormField>
          <FormField label={t('instances.endpointPort')}>
            <TextField type="number" min={1} max={65535} value={endpointPort} onChange={(event) => setEndpointPort(event.target.value)} />
          </FormField>
          <FormField label={t('instances.specJSON')} full>
            <Textarea rows={10} spellCheck={false} value={specText} onChange={(event) => setSpecText(event.target.value)} />
          </FormField>
        </FormGrid>
        {notice ? <div role={notice.includes(':') || notice.includes('JSON') ? 'alert' : 'status'}>{notice}</div> : null}
        {created ? (
          <Card>
            <CardBody>
              <Toolbar>
                <Badge>{t('instances.createdInstance')}</Badge>
                <code>{created.id}</code>
              </Toolbar>
            </CardBody>
          </Card>
        ) : null}
        <Toolbar>
          <Button variant="primary" disabled={!canSubmit} onClick={() => void submit()}>{t('common.create')}</Button>
          <Button onClick={onClose}>{t('common.close')}</Button>
        </Toolbar>
      </div>
    </Drawer>
  );
}

function InstanceConfirmDialog({ action, busy, nodeName, onConfirm, onClose }: {
  action: ConfirmAction | null;
  busy: boolean;
  nodeName: (id?: string) => string;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  if (!action) return null;
  const operation = action.type === 'lifecycle' ? action.action : action.type;
  const expectedConfirmation = `DELETE ${action.instance.slug || action.instance.name || action.instance.id}`;
  const forceValid = action.type !== 'force-delete' || action.confirmation === expectedConfirmation;
  return (
    <ConfirmDialog title={t('instances.confirmTitle', { operation })} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{t('instances.confirmImpact', {
          operation,
          instance: instanceLabel(action.instance),
          node: nodeName(action.instance.node_id),
          service: action.instance.service_code || 'n/a',
          status: action.instance.status || 'n/a',
        })}</p>
        {action.type === 'rollback' ? <p>{t('instances.rollbackRevision', { revision: action.revisionId })}</p> : null}
        {action.type === 'replace-spec' ? <p>{t('instances.replaceSpecImpact', { apply: action.applyAfterSave ? t('common.yes') : t('common.no') })}</p> : null}
        {action.type === 'force-delete' ? <p>{t('instances.forceConfirmExpected', { value: expectedConfirmation })}</p> : null}
        <Toolbar>
          <Button variant={action.type === 'delete' || action.type === 'force-delete' || action.type === 'rollback' ? 'danger' : 'primary'} disabled={busy || !forceValid} onClick={onConfirm}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
