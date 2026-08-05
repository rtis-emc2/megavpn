import { Activity, GitBranch, Play, RefreshCw, ShieldCheck, Wrench } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import type { BackhaulActionResult, BackhaulLink, BackhaulTransport, JobRef } from '../../shared/api/types';
import { useApplyBackhaulLink, useBackhaulLink, useBackhaulLinks, useProbeBackhaulLink, usePromoteBackhaulLink, useUpdateBackhaulRouteState } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, Drawer, JobStatusPanel, StatusBadge, Toolbar } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type BackhaulConfirmAction =
  | { type: 'apply'; link: BackhaulLink }
  | { type: 'probe'; link: BackhaulLink }
  | { type: 'promote'; link: BackhaulLink; transport: BackhaulTransport }
  | { type: 'route'; link: BackhaulLink; enabled: boolean };

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

export function BackhaulPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const links = useBackhaulLinks();
  const [selectedId, setSelectedId] = useState('');
  const [confirm, setConfirm] = useState<BackhaulConfirmAction | null>(null);
  const [notice, setNotice] = useState('');
  const [lastResult, setLastResult] = useState<BackhaulActionResult | null>(null);
  const detail = useBackhaulLink(selectedId || undefined);
  const apply = useApplyBackhaulLink();
  const probe = useProbeBackhaulLink();
  const promote = usePromoteBackhaulLink();
  const routeState = useUpdateBackhaulRouteState();

  const rows = links.data || [];
  const fallbackSelected = rows.find((link) => link.id === selectedId);
  const selected = detail.data || fallbackSelected || null;
  const busy = apply.isPending || probe.isPending || promote.isPending || routeState.isPending;
  const resultJobs = useMemo(() => backhaulJobs(lastResult), [lastResult]);

  const runConfirmed = async () => {
    if (!confirm) return;
    setNotice('');
    try {
      const result = confirm.type === 'apply'
        ? await apply.mutateAsync(confirm.link.id)
        : confirm.type === 'probe'
          ? await probe.mutateAsync(confirm.link.id)
          : confirm.type === 'promote'
            ? await promote.mutateAsync({ linkId: confirm.link.id, transportId: confirm.transport.id })
            : await routeState.mutateAsync({ linkId: confirm.link.id, input: { enabled: confirm.enabled } });
      setLastResult(result);
      setNotice(t('backhaul.actionQueued', { count: result.job_count ?? backhaulJobs(result).length }));
      setConfirm(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold title={t('backhaul.title')} subtitle={t('backhaul.subtitle')}>
      <QueryBoundary isLoading={links.isLoading} isError={links.isError} error={links.error} refetch={() => void links.refetch()}>
        <div className="page-stack">
          {notice ? <div role={notice.includes('failed') || notice.includes('required') || notice.includes('HTTP') ? 'alert' : 'status'}>{notice}</div> : null}
          <DataTable
            rows={rows}
            columns={[
              { key: 'name', header: t('backhaul.link'), render: (row) => <strong>{text(linkLabel(row))}</strong> },
              { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
              { key: 'ingress', header: t('backhaul.ingress'), render: (row) => text(row.ingress_node_id) },
              { key: 'egress', header: t('backhaul.egress'), render: (row) => text(row.egress_node_id) },
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
        : t('backhaul.apply');
  const body = action.type === 'promote'
    ? t('backhaul.confirm.promote', { link: linkLabel(link), transport: transportLabel(action.transport) })
    : action.type === 'route'
      ? t('backhaul.confirm.route', { link: linkLabel(link), state: action.enabled ? t('common.enabled') : t('common.disabled') })
      : action.type === 'probe'
        ? t('backhaul.confirm.probe', { link: linkLabel(link) })
        : t('backhaul.confirm.apply', { link: linkLabel(link) });
  return (
    <ConfirmDialog title={t('backhaul.confirm.title', { operation })} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{body}</p>
        <div>{t('backhaul.impact')}: <strong>{t('backhaul.jobImpact')}</strong></div>
        <Toolbar>
          <Button variant={action.type === 'route' && !action.enabled ? 'danger' : 'primary'} icon={<Wrench size={16} />} disabled={busy} onClick={onConfirm}>
            {t('clients.core.confirm')}
          </Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
