import { Activity, GitBranch, Play, Plus, RefreshCw, ShieldCheck, Trash2, Wrench } from 'lucide-react';
import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import type { BackhaulActionResult, BackhaulCreateInput, BackhaulDriverDefinition, BackhaulLink, BackhaulTransport, JobRef, NodeEntity } from '../../shared/api/types';
import { useApplyBackhaulLink, useBackhaulDrivers, useBackhaulLink, useBackhaulLinks, useCreateBackhaulLink, useDeleteBackhaulLink, useNodes, useProbeBackhaulLink, usePromoteBackhaulLink, useUpdateBackhaulRouteState } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, Checkbox, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, JobStatusPanel, Select, StatusBadge, TextField, Toolbar } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type BackhaulConfirmAction =
  | { type: 'apply'; link: BackhaulLink }
  | { type: 'probe'; link: BackhaulLink }
  | { type: 'promote'; link: BackhaulLink; transport: BackhaulTransport }
  | { type: 'route'; link: BackhaulLink; enabled: boolean }
  | { type: 'delete'; link: BackhaulLink };

type BackhaulDraft = {
  name: string;
  ingressNodeID: string;
  egressNodeID: string;
  desiredDriver: string;
  standbyDrivers: string[];
  endpointHost: string;
  tunnelCIDR: string;
  routingTable: string;
  routeMetric: string;
};

const emptyBackhaulDraft: BackhaulDraft = {
  name: '',
  ingressNodeID: '',
  egressNodeID: '',
  desiredDriver: 'wireguard',
  standbyDrivers: [],
  endpointHost: '',
  tunnelCIDR: '',
  routingTable: '',
  routeMetric: '50',
};

function formatAPIError(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

function linkLabel(link?: BackhaulLink | null): string {
  if (!link) return '';
  return link.name || link.id;
}

function selectedTransport(link?: BackhaulLink | null): BackhaulTransport | undefined {
  if (!link?.transports?.length) return undefined;
  return link.transports.find((transport) => transport.id === link.selected_transport_id) || link.transports[0];
}

function transportLabel(transport?: BackhaulTransport | null): string {
  if (!transport) return 'n/a';
  return [transport.driver, transport.interface_name, transport.id].filter(Boolean).join(' / ');
}

function routeProjectionEnabled(link: BackhaulLink): boolean {
  if (typeof link.route_enabled === 'boolean') return link.route_enabled;
  return link.status !== 'disabled';
}

function backhaulJobs(result: BackhaulActionResult | null): JobRef[] {
  return Array.isArray(result?.jobs) ? result.jobs.filter((job): job is JobRef => Boolean(job?.id)) : [];
}

function transportHealth(transport: BackhaulTransport): string {
  const health = transport.health;
  if (!health || typeof health !== 'object') return 'n/a';
  const status = health.status || health.state || health.result;
  return typeof status === 'string' ? status : 'reported';
}

function nodeLabel(node: NodeEntity): string {
  return [node.name || node.id, node.address].filter(Boolean).join(' · ');
}

function usableNodes(nodes: NodeEntity[] | undefined, role: 'ingress' | 'egress'): NodeEntity[] {
  return (nodes || []).filter((node) => node.role === role && !['deleted', 'retired'].includes(String(node.status || '').toLowerCase()));
}

export function BackhaulPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const links = useBackhaulLinks();
  const nodes = useNodes();
  const [selectedId, setSelectedId] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [confirm, setConfirm] = useState<BackhaulConfirmAction | null>(null);
  const [notice, setNotice] = useState('');
  const [noticeError, setNoticeError] = useState(false);
  const [lastResult, setLastResult] = useState<BackhaulActionResult | null>(null);
  const detail = useBackhaulLink(selectedId || undefined);
  const apply = useApplyBackhaulLink();
  const probe = useProbeBackhaulLink();
  const promote = usePromoteBackhaulLink();
  const routeState = useUpdateBackhaulRouteState();
  const remove = useDeleteBackhaulLink();

  const rows = links.data || [];
  const fallbackSelected = rows.find((link) => link.id === selectedId);
  const selected = detail.data || fallbackSelected || null;
  const busy = apply.isPending || probe.isPending || promote.isPending || routeState.isPending || remove.isPending;
  const resultJobs = useMemo(() => backhaulJobs(lastResult), [lastResult]);
  const nodeNames = useMemo(() => new Map((nodes.data || []).map((node) => [node.id, nodeLabel(node)])), [nodes.data]);

  const runConfirmed = async () => {
    if (!confirm) return;
    setNotice('');
    setNoticeError(false);
    try {
      const result = confirm.type === 'apply'
        ? await apply.mutateAsync(confirm.link.id)
        : confirm.type === 'probe'
          ? await probe.mutateAsync(confirm.link.id)
          : confirm.type === 'promote'
            ? await promote.mutateAsync({ linkId: confirm.link.id, transportId: confirm.transport.id })
            : confirm.type === 'route'
              ? await routeState.mutateAsync({ linkId: confirm.link.id, input: { enabled: confirm.enabled } })
              : await remove.mutateAsync(confirm.link.id);
      setLastResult(result);
      setNotice(confirm.type === 'delete'
        ? t('backhaul.deleteQueued', { count: result.job_count ?? backhaulJobs(result).length })
        : t('backhaul.actionQueued', { count: result.job_count ?? backhaulJobs(result).length }));
      if (confirm.type === 'delete') setSelectedId('');
      setConfirm(null);
    } catch (error) {
      setNoticeError(true);
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold
      title={t('backhaul.title')}
      subtitle={t('backhaul.subtitle')}
      actions={<Button variant="primary" icon={<Plus size={16} />} onClick={() => setCreateOpen(true)}>{t('backhaul.create')}</Button>}
    >
      <QueryBoundary isLoading={links.isLoading} isError={links.isError} error={links.error} refetch={() => void links.refetch()}>
        <div className="page-stack">
          {notice ? <div role={noticeError ? 'alert' : 'status'}>{notice}</div> : null}
          <DataTable
            rows={rows}
            empty={(
              <div className="empty-state">
                <strong>{t('backhaul.emptyTitle')}</strong>
                <span>{t('backhaul.emptyBody')}</span>
                <Button variant="primary" icon={<Plus size={16} />} onClick={() => setCreateOpen(true)}>{t('backhaul.create')}</Button>
              </div>
            )}
            columns={[
              { key: 'name', header: t('backhaul.link'), render: (row) => <strong>{text(linkLabel(row))}</strong> },
              { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
              { key: 'ingress', header: t('backhaul.ingress'), render: (row) => text(nodeNames.get(String(row.ingress_node_id)) || row.ingress_node_id) },
              { key: 'egress', header: t('backhaul.egress'), render: (row) => text(nodeNames.get(String(row.egress_node_id)) || row.egress_node_id) },
              { key: 'driver', header: t('backhaul.driver'), render: (row) => text(row.desired_driver || row.driver || selectedTransport(row)?.driver) },
              { key: 'transport', header: t('backhaul.transport'), render: (row) => text(selectedTransport(row)?.id || row.selected_transport_id) },
              { key: 'routing', header: t('backhaul.routeProjection'), render: (row) => <StatusBadge status={routeProjectionEnabled(row) ? 'enabled' : 'disabled'} /> },
              { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at) },
              {
                key: 'actions',
                header: t('common.actions'),
                render: (row) => (
                  <Toolbar>
                    <Button icon={<GitBranch size={16} />} onClick={() => setSelectedId(row.id)}>{t('common.open')}</Button>
                    <Button icon={<Play size={16} />} disabled={busy} onClick={() => setConfirm({ type: 'apply', link: row })}>{t('backhaul.apply')}</Button>
                    <Button icon={<Activity size={16} />} disabled={busy} onClick={() => setConfirm({ type: 'probe', link: row })}>{t('backhaul.probe')}</Button>
                  </Toolbar>
                ),
              },
            ]}
          />

          {resultJobs.length ? (
            <div className="page-stack">
              <Link to="/operations/jobs">{t('jobs.openJobs')}</Link>
              {resultJobs.map((job) => <JobStatusPanel key={job.id} jobID={job.id} />)}
            </div>
          ) : null}
        </div>

        <Drawer title={selected ? linkLabel(selected) : t('backhaul.link')} open={Boolean(selectedId)} onClose={() => setSelectedId('')}>
          {selected ? (
            <BackhaulDetail
              link={selected}
              loading={detail.isLoading}
              busy={busy}
              onConfirm={setConfirm}
            />
          ) : detail.isLoading ? <div>{t('common.loading')}</div> : null}
          {detail.isError ? <div role="alert" className="error-state-inline">{formatAPIError(detail.error)}</div> : null}
        </Drawer>

        {createOpen ? (
          <BackhaulCreateDrawer
            open
            onClose={() => setCreateOpen(false)}
            onCreated={(link) => {
              setCreateOpen(false);
              setSelectedId(link.id);
              setNoticeError(false);
              setNotice(t('backhaul.created', { link: linkLabel(link) }));
            }}
          />
        ) : null}

        <BackhaulConfirmDialog action={confirm} busy={busy} onClose={() => setConfirm(null)} onConfirm={() => void runConfirmed()} />
      </QueryBoundary>
    </PageScaffold>
  );
}

function BackhaulDetail({ link, loading, busy, onConfirm }: {
  link: BackhaulLink;
  loading: boolean;
  busy: boolean;
  onConfirm: (action: BackhaulConfirmAction) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const currentTransport = selectedTransport(link);
  const routeEnabled = routeProjectionEnabled(link);
  return (
    <div className="page-stack">
      {loading ? <Badge>{t('common.loading')}</Badge> : null}
      <Toolbar>
        <Badge>{t('backhaul.backendValidated')}</Badge>
        <StatusBadge status={link.status} />
      </Toolbar>
      <Card>
        <CardBody>
          <div className="page-stack">
            <div>{t('backhaul.ingress')}: <strong>{text(link.ingress_node_id)}</strong></div>
            <div>{t('backhaul.egress')}: <strong>{text(link.egress_node_id)}</strong></div>
            <div>{t('backhaul.driver')}: <strong>{text(link.desired_driver || link.driver || currentTransport?.driver)}</strong></div>
            <div>{t('backhaul.selectedTransport')}: <strong>{text(currentTransport?.id || link.selected_transport_id)}</strong></div>
            <div>{t('backhaul.routingTable')}: <strong>{text(link.routing_table)}</strong></div>
            <div>{t('backhaul.routeMetric')}: <strong>{text(link.route_metric)}</strong></div>
            <div>{t('backhaul.updated')}: <strong>{fmt.date(link.updated_at)}</strong></div>
          </div>
        </CardBody>
      </Card>

      <Toolbar>
        <Button variant="primary" icon={<Play size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'apply', link })}>{t('backhaul.apply')}</Button>
        <Button icon={<Activity size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'probe', link })}>{t('backhaul.probe')}</Button>
        <Button icon={<RefreshCw size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'route', link, enabled: !routeEnabled })}>
          {routeEnabled ? t('backhaul.disableRoute') : t('backhaul.enableRoute')}
        </Button>
        <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy} onClick={() => onConfirm({ type: 'delete', link })}>{t('backhaul.delete')}</Button>
      </Toolbar>

      <DataTable
        title={t('backhaul.transports')}
        rows={link.transports || []}
        columns={[
          { key: 'transport', header: t('backhaul.transport'), render: (transport) => <strong>{text(transportLabel(transport))}</strong> },
          { key: 'status', header: t('common.status'), render: (transport) => <StatusBadge status={transport.status} /> },
          { key: 'endpoint', header: t('backhaul.endpoint'), render: (transport) => text([transport.endpoint_host, transport.endpoint_port].filter(Boolean).join(':')) },
          { key: 'tunnel', header: t('backhaul.tunnel'), render: (transport) => text(transport.tunnel_cidr) },
          { key: 'addresses', header: t('backhaul.addresses'), render: (transport) => text([transport.ingress_address, transport.egress_address].filter(Boolean).join(' -> ')) },
          { key: 'health', header: t('backhaul.health'), render: (transport) => text(transportHealth(transport)) },
          {
            key: 'actions',
            header: t('common.actions'),
            render: (transport) => (
              <Button
                icon={<ShieldCheck size={16} />}
                disabled={busy || transport.id === link.selected_transport_id}
                onClick={() => onConfirm({ type: 'promote', link, transport })}
              >
                {transport.id === link.selected_transport_id ? t('backhaul.selected') : t('backhaul.promote')}
              </Button>
            ),
          },
        ]}
      />
      <Card>
        <CardBody>
          <div className="page-stack">
            <strong>{t('backhaul.secretSafety')}</strong>
            <p>{t('backhaul.secretSafetyBody')}</p>
          </div>
        </CardBody>
      </Card>
    </div>
  );
}

function BackhaulCreateDrawer({ open, onClose, onCreated }: {
  open: boolean;
  onClose: () => void;
  onCreated: (link: BackhaulLink) => void;
}) {
  const { t } = useTranslation();
  const nodes = useNodes();
  const drivers = useBackhaulDrivers();
  const create = useCreateBackhaulLink();
  const [draft, setDraft] = useState<BackhaulDraft>(emptyBackhaulDraft);
  const [error, setError] = useState('');

  const ingressNodes = usableNodes(nodes.data, 'ingress');
  const egressNodes = usableNodes(nodes.data, 'egress');
  const desiredDriver = drivers.data?.some((driver) => driver.code === draft.desiredDriver)
    ? draft.desiredDriver
    : drivers.data?.[0]?.code || '';
  const selectedDriver = drivers.data?.find((driver) => driver.code === desiredDriver);
  const valid = Boolean(draft.ingressNodeID && draft.egressNodeID && draft.ingressNodeID !== draft.egressNodeID && selectedDriver);

  const set = <K extends keyof BackhaulDraft>(key: K, value: BackhaulDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const toggleStandby = (code: string) => {
    setDraft((current) => ({
      ...current,
      standbyDrivers: current.standbyDrivers.includes(code)
        ? current.standbyDrivers.filter((driver) => driver !== code)
        : [...current.standbyDrivers, code],
    }));
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!valid || create.isPending) return;
    setError('');
    const routeMetric = Number.parseInt(draft.routeMetric, 10);
    const input: BackhaulCreateInput = {
      name: draft.name.trim() || undefined,
      ingress_node_id: draft.ingressNodeID,
      egress_node_id: draft.egressNodeID,
      desired_driver: desiredDriver,
      endpoint_host: draft.endpointHost.trim() || undefined,
      tunnel_cidr: draft.tunnelCIDR.trim() || undefined,
      routing_table: draft.routingTable.trim() || undefined,
      route_metric: Number.isFinite(routeMetric) && routeMetric > 0 ? routeMetric : 50,
      drivers: [desiredDriver, ...draft.standbyDrivers.filter((driver) => driver !== desiredDriver)],
    };
    try {
      onCreated(await create.mutateAsync(input));
    } catch (submitError) {
      setError(formatAPIError(submitError));
    }
  };

  return (
    <Drawer title={t('backhaul.createTitle')} open={open} onClose={onClose} size="wide">
      <form className="page-stack" onSubmit={(event) => void submit(event)}>
        <div>
          <h3 className="card-title">{t('backhaul.connection')}</h3>
          <p>{t('backhaul.createSubtitle')}</p>
        </div>
        {error ? <div role="alert">{error}</div> : null}
        {drivers.isError ? <div role="alert">{formatAPIError(drivers.error)}</div> : null}
        {!nodes.isLoading && !ingressNodes.length ? <div role="alert">{t('backhaul.noIngressNodes')}</div> : null}
        {!nodes.isLoading && !egressNodes.length ? <div role="alert">{t('backhaul.noEgressNodes')}</div> : null}
        <FormGrid>
          <FormField label={t('backhaul.ingressNode')} required>
            <Select aria-label={t('backhaul.ingressNode')} value={draft.ingressNodeID} onChange={(event) => set('ingressNodeID', event.currentTarget.value)}>
              <option value="">{t('backhaul.selectIngress')}</option>
              {ingressNodes.map((node) => <option key={node.id} value={node.id}>{nodeLabel(node)}</option>)}
            </Select>
          </FormField>
          <FormField label={t('backhaul.egressNode')} required>
            <Select aria-label={t('backhaul.egressNode')} value={draft.egressNodeID} onChange={(event) => set('egressNodeID', event.currentTarget.value)}>
              <option value="">{t('backhaul.selectEgress')}</option>
              {egressNodes.map((node) => <option key={node.id} value={node.id}>{nodeLabel(node)}</option>)}
            </Select>
          </FormField>
          <FormField label={t('common.name')} hint={t('backhaul.nameHint')}>
            <TextField aria-label={t('common.name')} value={draft.name} placeholder={t('backhaul.namePlaceholder')} onChange={(event) => set('name', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('backhaul.endpointHost')} hint={t('backhaul.endpointHostHint')}>
            <TextField aria-label={t('backhaul.endpointHost')} value={draft.endpointHost} placeholder={t('backhaul.autoFromEgress')} onChange={(event) => set('endpointHost', event.currentTarget.value)} />
          </FormField>
        </FormGrid>

        <div>
          <h3 className="card-title">{t('backhaul.transportSettings')}</h3>
          <p>{t('backhaul.transportSettingsHint')}</p>
        </div>
        <FormGrid>
          <FormField label={t('backhaul.primaryDriver')} required hint={selectedDriver?.warnings?.[0]}>
            <Select aria-label={t('backhaul.primaryDriver')} value={desiredDriver} onChange={(event) => {
              const value = event.currentTarget.value;
              setDraft((current) => ({ ...current, desiredDriver: value, standbyDrivers: current.standbyDrivers.filter((driver) => driver !== value) }));
            }}>
              {(drivers.data || []).map((driver) => <option key={driver.code} value={driver.code}>{driver.label}</option>)}
            </Select>
          </FormField>
          <FormField label={t('backhaul.routeMetric')} hint={t('backhaul.routeMetricHint')}>
            <TextField aria-label={t('backhaul.routeMetric')} type="number" min="1" max="65535" value={draft.routeMetric} onChange={(event) => set('routeMetric', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('backhaul.tunnelCIDR')} hint={t('backhaul.tunnelCIDRHint')}>
            <TextField aria-label={t('backhaul.tunnelCIDR')} value={draft.tunnelCIDR} placeholder={t('backhaul.automatic')} onChange={(event) => set('tunnelCIDR', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('backhaul.routingTable')} hint={t('backhaul.routingTableHint')}>
            <TextField aria-label={t('backhaul.routingTable')} value={draft.routingTable} placeholder={t('backhaul.automatic')} onChange={(event) => set('routingTable', event.currentTarget.value)} />
          </FormField>
        </FormGrid>

        <Card>
          <CardBody>
            <div className="page-stack">
              <strong>{t('backhaul.standbyDrivers')}</strong>
              <span>{t('backhaul.standbyDriversHint')}</span>
              <div className="toolbar">
                {(drivers.data || []).filter((driver) => driver.code !== desiredDriver).map((driver: BackhaulDriverDefinition) => (
                  <Checkbox
                    key={driver.code}
                    label={driver.label}
                    checked={draft.standbyDrivers.includes(driver.code)}
                    onChange={() => toggleStandby(driver.code)}
                  />
                ))}
              </div>
            </div>
          </CardBody>
        </Card>

        <Toolbar>
          <Button variant="primary" type="submit" icon={<Plus size={16} />} disabled={!valid || create.isPending || nodes.isLoading || drivers.isLoading}>{t('backhaul.createAction')}</Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </Drawer>
  );
}

function BackhaulConfirmDialog({ action, busy, onClose, onConfirm }: {
  action: BackhaulConfirmAction | null;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  if (!action) return null;
  const link = action.link;
  const operation = action.type === 'route'
    ? action.enabled ? t('backhaul.enableRoute') : t('backhaul.disableRoute')
    : action.type === 'promote'
      ? t('backhaul.promote')
      : action.type === 'probe'
        ? t('backhaul.probe')
        : action.type === 'delete'
          ? t('backhaul.delete')
          : t('backhaul.apply');
  const body = action.type === 'promote'
    ? t('backhaul.confirm.promote', { link: linkLabel(link), transport: transportLabel(action.transport) })
    : action.type === 'route'
      ? t('backhaul.confirm.route', { link: linkLabel(link), state: action.enabled ? t('common.enabled') : t('common.disabled') })
      : action.type === 'probe'
        ? t('backhaul.confirm.probe', { link: linkLabel(link) })
        : action.type === 'delete'
          ? t('backhaul.confirm.delete', { link: linkLabel(link) })
          : t('backhaul.confirm.apply', { link: linkLabel(link) });
  return (
    <ConfirmDialog title={t('backhaul.confirm.title', { operation })} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{body}</p>
        <div>{t('backhaul.impact')}: <strong>{t('backhaul.jobImpact')}</strong></div>
        <Toolbar>
          <Button variant={action.type === 'delete' || (action.type === 'route' && !action.enabled) ? 'danger' : 'primary'} icon={action.type === 'delete' ? <Trash2 size={16} /> : <Wrench size={16} />} disabled={busy} onClick={onConfirm}>
            {t('clients.core.confirm')}
          </Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
