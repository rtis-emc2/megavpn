import { FilePenLine, PackagePlus, Power, RefreshCw, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { APIError } from '../../shared/api/client';
import type { InstanceCreateResult, Job, NodeEntity, ServiceInstance, ServicePack } from '../../shared/api/types';
import {
  useCreateInstanceFromServicePack,
  useCreateServicePack,
  useDeleteServicePack,
  useNodes,
  useServicePacks,
  useSetServicePackEnabled,
  useUpdateServicePack,
} from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, Checkbox, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Select, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';
import { ServicesTabs } from './ServicesTabs';

type PackDrawerMode = 'detail' | 'edit' | 'create' | 'create-instance';
type PackConfirm =
  | { type: 'delete'; pack: ServicePack }
  | { type: 'status'; pack: ServicePack; enabled: boolean }
  | { type: 'create-instance'; pack: ServicePack; input: PackInstanceForm };

type PackInstanceForm = {
  nodeId: string;
  baseName: string;
  endpointHost: string;
  autoInstallRuntime: boolean;
  camouflagePath: string;
  fallbackUpstreamURL: string;
  fallbackHostHeader: string;
  fallbackSNI: string;
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

function packLabel(pack?: ServicePack | null): string {
  return pack?.label || pack?.key || 'n/a';
}

function safeJSON(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return '{}';
  }
}

function resultJobs(result: InstanceCreateResult | null): Job[] {
  if (!result || !('status' in result)) return [];
  const jobs = [
    ...((result.runtime_install_jobs as Job[] | undefined) || []),
    ...((result.apply_jobs as Job[] | undefined) || []),
    ...((result.jobs as Job[] | undefined) || []),
  ];
  if (result.job && typeof result.job === 'object') jobs.push(result.job as Job);
  return jobs.filter((job) => Boolean(job?.id));
}

function resultInstances(result: InstanceCreateResult | null, key: 'created_instances' | 'existing_instances'): ServiceInstance[] {
  if (!result || !('status' in result)) return [];
  const value = (result as Record<string, unknown>)[key];
  return Array.isArray(value) ? value as ServiceInstance[] : [];
}

export function ServicePacksPage() {
  const { t } = useTranslation();
  const packs = useServicePacks();
  const nodes = useNodes({ retry: false });
  const createPack = useCreateServicePack();
  const updatePack = useUpdateServicePack();
  const setStatus = useSetServicePackEnabled();
  const deletePack = useDeleteServicePack();
  const createInstances = useCreateInstanceFromServicePack();
  const [selected, setSelected] = useState<ServicePack | null>(null);
  const [mode, setMode] = useState<PackDrawerMode>('detail');
  const [packJSON, setPackJSON] = useState('');
  const [notice, setNotice] = useState('');
  const [confirm, setConfirm] = useState<PackConfirm | null>(null);
  const [createResult, setCreateResult] = useState<InstanceCreateResult | null>(null);
  const busy = createPack.isPending || updatePack.isPending || setStatus.isPending || deletePack.isPending || createInstances.isPending;

  const openDrawer = (nextMode: PackDrawerMode, pack?: ServicePack) => {
    setSelected(pack || null);
    setMode(nextMode);
    setNotice('');
    setCreateResult(null);
    setPackJSON(nextMode === 'create' ? safeJSON({ key: '', label: '', description: '', components: [] }) : safeJSON(pack || {}));
  };

  const savePack = async () => {
    setNotice('');
    try {
      const parsed = JSON.parse(packJSON || '{}') as ServicePack;
      if (!parsed.key) throw new Error(t('servicePacks.keyRequired'));
      const saved = mode === 'create'
        ? await createPack.mutateAsync(parsed)
        : await updatePack.mutateAsync({ packId: selected?.key || parsed.key, input: parsed });
      setSelected(saved);
      setPackJSON(safeJSON(saved));
      setMode('detail');
      setNotice(t('servicePacks.saved'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const runConfirmed = async () => {
    if (!confirm) return;
    setNotice('');
    setCreateResult(null);
    try {
      if (confirm.type === 'delete') {
        const updated = await deletePack.mutateAsync(confirm.pack.key);
        setSelected(updated);
        setNotice(t('servicePacks.deleted'));
      } else if (confirm.type === 'status') {
        const updated = await setStatus.mutateAsync({ packId: confirm.pack.key, enabled: confirm.enabled });
        setSelected(updated);
        setNotice(confirm.enabled ? t('servicePacks.enabled') : t('servicePacks.disabled'));
      } else if (confirm.type === 'create-instance') {
        const result = await createInstances.mutateAsync({
          packId: confirm.pack.key,
          input: {
            node_id: confirm.input.nodeId,
            base_name: confirm.input.baseName,
            endpoint_host: confirm.input.endpointHost,
            auto_install_runtime: confirm.input.autoInstallRuntime,
            camouflage_path: confirm.input.camouflagePath,
            fallback_upstream_url: confirm.input.fallbackUpstreamURL,
            fallback_host_header: confirm.input.fallbackHostHeader,
            fallback_sni: confirm.input.fallbackSNI,
          },
        });
        setCreateResult(result);
        setNotice(t('servicePacks.createInstanceAccepted'));
      }
      setConfirm(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold title={t('instances.servicePacksTitle')} subtitle={t('servicePacks.subtitle')} actions={<Button icon={<FilePenLine size={16} />} onClick={() => openDrawer('create')}>{t('common.create')}</Button>}>
      <ServicesTabs />
      <Toolbar>
        <Badge>{t('servicePacks.backendManaged')}</Badge>
        <Button icon={<RefreshCw size={16} />} onClick={() => void packs.refetch()}>{t('common.refresh')}</Button>
      </Toolbar>
      <QueryBoundary isLoading={packs.isLoading} isError={packs.isError} error={packs.error} refetch={() => void packs.refetch()}>
        <DataTable
          rows={packs.data || []}
          columns={[
            { key: 'key', header: t('common.name'), render: (row) => <strong>{text(packLabel(row))}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'active')} /> },
            { key: 'version', header: t('servicePacks.version'), render: (row) => String(row.version || 'n/a') },
            { key: 'source', header: t('common.type'), render: (row) => text(row.source || 'catalog') },
            { key: 'components', header: t('nav.instances'), render: (row) => String(row.components?.length || 0) },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button onClick={() => openDrawer('detail', row)}>{t('common.open')}</Button>
                  <Button icon={<PackagePlus size={16} />} onClick={() => openDrawer('create-instance', row)}>{t('instances.createFromPack')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />
      </QueryBoundary>
      <Drawer title={mode === 'create' ? t('servicePacks.createTitle') : packLabel(selected)} open={mode !== 'detail' || Boolean(selected)} onClose={() => { setSelected(null); setMode('detail'); }}>
        <div className="page-stack">
          <ServicesTabs />
          {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
          {mode === 'detail' && selected ? (
            <PackDetail pack={selected} busy={busy} onEdit={() => openDrawer('edit', selected)} onCreate={() => openDrawer('create-instance', selected)} onConfirm={setConfirm} />
          ) : null}
          {(mode === 'edit' || mode === 'create') ? (
            <Card>
              <CardBody>
                <div className="page-stack">
                  <Badge>{t('servicePacks.jsonEditorNote')}</Badge>
                  <FormField label={t('servicePacks.definitionJSON')} full>
                    <Textarea value={packJSON} rows={18} spellCheck={false} onChange={(event) => setPackJSON(event.target.value)} />
                  </FormField>
                  <Toolbar>
                    <Button variant="primary" disabled={busy} onClick={() => void savePack()}>{t('common.save')}</Button>
                    {selected ? <Button onClick={() => openDrawer('detail', selected)}>{t('common.cancel')}</Button> : null}
                  </Toolbar>
                </div>
              </CardBody>
            </Card>
          ) : null}
          {mode === 'create-instance' && selected ? (
            <CreateFromPackForm key={`${selected.key}:${nodes.data?.map((node) => node.id).join(',') || ''}`} pack={selected} nodes={nodes.data || []} busy={busy} result={createResult} onConfirm={setConfirm} />
          ) : null}
        </div>
      </Drawer>
      <PackConfirmDialog action={confirm} busy={busy} onConfirm={() => void runConfirmed()} onClose={() => setConfirm(null)} />
    </PageScaffold>
  );
}

function PackDetail({ pack, busy, onEdit, onCreate, onConfirm }: {
  pack: ServicePack;
  busy: boolean;
  onEdit: () => void;
  onCreate: () => void;
  onConfirm: (action: PackConfirm) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <div className="definition-grid">
              <span>{t('common.id')}</span><strong>{pack.key}</strong>
              <span>{t('common.status')}</span><strong>{pack.status || 'active'}</strong>
              <span>{t('servicePacks.version')}</span><strong>{pack.version || 'n/a'}</strong>
              <span>{t('instances.endpoint')}</span><strong>{pack.endpoint_hint || 'n/a'}</strong>
              <span>{t('servicePacks.baseName')}</span><strong>{pack.base_name_template || 'n/a'}</strong>
            </div>
            <p>{text(pack.description)}</p>
            <Toolbar>
              <Button variant="primary" icon={<PackagePlus size={16} />} disabled={busy} onClick={onCreate}>{t('instances.createFromPack')}</Button>
              <Button icon={<FilePenLine size={16} />} disabled={busy} onClick={onEdit}>{t('common.edit')}</Button>
              <Button icon={<Power size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'status', pack, enabled: pack.status === 'disabled' })}>{pack.status === 'disabled' ? t('common.enabled') : t('common.disabled')}</Button>
              <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'delete', pack })}>{t('common.delete')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      <DataTable
        title={t('servicePacks.components')}
        rows={pack.components || []}
        columns={[
          { key: 'label', header: t('common.name'), render: (row) => <strong>{text(row.label || row.name_suffix)}</strong> },
          { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
          { key: 'preset', header: t('common.type'), render: (row) => text(row.preset_key) },
          { key: 'port', header: t('instances.endpointPort'), render: (row) => String(row.endpoint_port || 'n/a') },
          { key: 'endpoint', header: t('servicePacks.requiresEndpoint'), render: (row) => row.requires_endpoint_host ? t('common.enabled') : t('common.disabled') },
        ]}
      />
      <Card>
        <CardBody>
          <pre className="code-block">{safeJSON({ platform_notes: pack.platform_notes || [], recommendations: pack.recommendations || [] })}</pre>
        </CardBody>
      </Card>
    </div>
  );
}

function CreateFromPackForm({ pack, nodes, busy, result, onConfirm }: {
  pack: ServicePack;
  nodes: NodeEntity[];
  busy: boolean;
  result: InstanceCreateResult | null;
  onConfirm: (action: PackConfirm) => void;
}) {
  const { t } = useTranslation();
  const [nodeId, setNodeId] = useState(() => nodes[0]?.id || '');
  const [baseName, setBaseName] = useState(() => pack.base_name_template || pack.key || '');
  const [endpointHost, setEndpointHost] = useState(() => pack.endpoint_hint || '');
  const [autoInstallRuntime, setAutoInstallRuntime] = useState(true);
  const [camouflagePath, setCamouflagePath] = useState('/access');
  const [fallbackUpstreamURL, setFallbackUpstreamURL] = useState('');
  const [fallbackHostHeader, setFallbackHostHeader] = useState('');
  const [fallbackSNI, setFallbackSNI] = useState('');

  const input: PackInstanceForm = useMemo(() => ({
    nodeId,
    baseName,
    endpointHost,
    autoInstallRuntime,
    camouflagePath,
    fallbackUpstreamURL,
    fallbackHostHeader,
    fallbackSNI,
  }), [autoInstallRuntime, baseName, camouflagePath, endpointHost, fallbackHostHeader, fallbackSNI, fallbackUpstreamURL, nodeId]);
  const jobs = resultJobs(result);
  const created = resultInstances(result, 'created_instances');
  const existing = resultInstances(result, 'existing_instances');
  const canCreate = Boolean(nodeId && (!pack.requires_endpoint_host || endpointHost) && !busy);

  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <Toolbar>
            <Badge>{pack.key}</Badge>
            <Badge>{t('servicePacks.noSeparateValidation')}</Badge>
          </Toolbar>
          <FormGrid>
            <FormField label={t('instances.node')}>
              <Select value={nodeId} onChange={(event) => setNodeId(event.target.value)}>
                <option value="">{t('instances.selectNode')}</option>
                {nodes.map((node) => <option key={node.id} value={node.id}>{node.name || node.id}</option>)}
              </Select>
            </FormField>
            <FormField label={t('servicePacks.baseName')}>
              <TextField value={baseName} onChange={(event) => setBaseName(event.target.value)} />
            </FormField>
            <FormField label={t('instances.endpointHost')}>
              <TextField value={endpointHost} onChange={(event) => setEndpointHost(event.target.value)} />
            </FormField>
            <FormField label={t('servicePacks.camouflagePath')}>
              <TextField value={camouflagePath} onChange={(event) => setCamouflagePath(event.target.value)} />
            </FormField>
            <FormField label={t('servicePacks.fallbackUpstream')}>
              <TextField value={fallbackUpstreamURL} onChange={(event) => setFallbackUpstreamURL(event.target.value)} />
            </FormField>
            <FormField label={t('servicePacks.fallbackHostHeader')}>
              <TextField value={fallbackHostHeader} onChange={(event) => setFallbackHostHeader(event.target.value)} />
            </FormField>
            <FormField label={t('servicePacks.fallbackSNI')}>
              <TextField value={fallbackSNI} onChange={(event) => setFallbackSNI(event.target.value)} />
            </FormField>
          </FormGrid>
          <Checkbox label={t('servicePacks.autoInstallRuntime')} checked={autoInstallRuntime} onChange={(event) => setAutoInstallRuntime(event.target.checked)} />
          <Toolbar>
            <Button variant="primary" icon={<PackagePlus size={16} />} disabled={!canCreate} onClick={() => onConfirm({ type: 'create-instance', pack, input })}>{t('common.create')}</Button>
          </Toolbar>
          {result ? (
            <Card>
              <CardBody>
                <div className="page-stack">
                  <Toolbar>
                    <Badge>{String((result as { status?: string }).status || 'created')}</Badge>
                    <Link to="/services/instances">{t('instances.title')}</Link>
                    <Link to="/operations/jobs">{t('instances.openJobs')}</Link>
                  </Toolbar>
                  <pre className="code-block">{safeJSON({ created_instances: created.map((item) => ({ id: item.id, name: item.name, service_code: item.service_code })), existing_instances: existing.map((item) => ({ id: item.id, name: item.name, service_code: item.service_code })), jobs: jobs.map((job) => ({ id: job.id, type: job.type, status: job.status })) })}</pre>
                  {jobs.map((job) => <JobStatusPanel key={job.id} jobID={job.id} />)}
                </div>
              </CardBody>
            </Card>
          ) : null}
        </div>
      </CardBody>
    </Card>
  );
}

function PackConfirmDialog({ action, busy, onConfirm, onClose }: { action: PackConfirm | null; busy: boolean; onConfirm: () => void; onClose: () => void }) {
  const { t } = useTranslation();
  if (!action) return null;
  const label = action.type === 'create-instance' ? packLabel(action.pack) : action.pack.key;
  return (
    <ConfirmDialog title={t('servicePacks.confirmTitle', { action: action.type })} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{t('servicePacks.confirmImpact', { action: action.type, pack: label })}</p>
        {action.type === 'create-instance' ? <pre className="code-block">{safeJSON(action.input)}</pre> : null}
        <Toolbar>
          <Button variant={action.type === 'delete' ? 'danger' : 'primary'} disabled={busy} onClick={onConfirm}>{t('clients.core.confirm')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
