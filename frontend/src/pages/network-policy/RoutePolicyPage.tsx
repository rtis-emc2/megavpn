import { Ban, Play, RotateCcw, Route, ShieldCheck } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import type { APIRecord, JobRef, RoutePolicy, RoutePolicyApplyResult, RoutePolicyCleanupResult, RoutePolicyPreviewResult } from '../../shared/api/types';
import { useApplyRoutePolicy, useCleanupRoutePolicy, usePreviewRoutePolicy, useRoutePolicies, useRoutePolicy } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, JobStatusPanel, StatusBadge, Toolbar } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type RoutePolicyConfirmAction =
  | { type: 'apply'; policy: RoutePolicy }
  | { type: 'cleanup'; policy: RoutePolicy };

function formatAPIError(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

function policyLabel(policy?: RoutePolicy | null): string {
  if (!policy) return '';
  return policy.node_name || policy.node_id;
}

function recordString(value: unknown): string {
  if (value === null || value === undefined || value === '') return 'n/a';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value);
}

function warningLabel(warning: APIRecord, index: number): string {
  const reason = warning.reason || warning.message || warning.reasons || warning.type;
  return `${index + 1}. ${recordString(reason)}`;
}

function routeRows(preview: RoutePolicyPreviewResult | null): APIRecord[] {
  return Array.isArray(preview?.routes) ? preview.routes : [];
}

function systemRouteRows(preview: RoutePolicyPreviewResult | null): APIRecord[] {
  return Array.isArray(preview?.system_routes) ? preview.system_routes : [];
}

function resultJob(result: RoutePolicyApplyResult | RoutePolicyCleanupResult | null): JobRef | null {
  return result?.job?.id ? result.job : null;
}

export function RoutePolicyPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const policies = useRoutePolicies();
  const [selectedNodeId, setSelectedNodeId] = useState('');
  const [previewNodeId, setPreviewNodeId] = useState('');
  const [previewResult, setPreviewResult] = useState<RoutePolicyPreviewResult | null>(null);
  const [confirm, setConfirm] = useState<RoutePolicyConfirmAction | null>(null);
  const [notice, setNotice] = useState('');
  const [applyResult, setApplyResult] = useState<RoutePolicyApplyResult | null>(null);
  const [cleanupResult, setCleanupResult] = useState<RoutePolicyCleanupResult | null>(null);
  const detail = useRoutePolicy(selectedNodeId || undefined);
  const preview = usePreviewRoutePolicy();
  const apply = useApplyRoutePolicy();
  const cleanup = useCleanupRoutePolicy();

  const rows = policies.data || [];
  const selected = detail.data || rows.find((policy) => policy.node_id === selectedNodeId) || null;
  const previewFresh = Boolean(previewResult && selectedNodeId && previewNodeId === selectedNodeId);
  const previewSuccessful = previewFresh && previewResult?.status !== 'error';
  const previewStale = Boolean(previewResult && selectedNodeId && previewNodeId !== selectedNodeId);
  const busy = preview.isPending || apply.isPending || cleanup.isPending;
  const applyJob = useMemo(() => resultJob(applyResult), [applyResult]);
  const cleanupJob = useMemo(() => resultJob(cleanupResult), [cleanupResult]);

  const runPreview = async (policy: RoutePolicy) => {
    setNotice('');
    try {
      const result = await preview.mutateAsync(policy.node_id);
      setPreviewResult(result);
      setPreviewNodeId(policy.node_id);
      setNotice(t('routePolicy.previewReady'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const runConfirmed = async () => {
    if (!confirm) return;
    setNotice('');
    try {
      if (confirm.type === 'apply') {
        const result = await apply.mutateAsync(confirm.policy.node_id);
        setApplyResult(result);
        setNotice(result.message || t('routePolicy.applyQueued'));
      } else {
        const result = await cleanup.mutateAsync(confirm.policy.node_id);
        setCleanupResult(result);
        setNotice(result.message || t('routePolicy.cleanupQueued'));
      }
      setConfirm(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold title={t('nav.routePolicy')} subtitle={t('routePolicy.subtitle')}>
      <QueryBoundary isLoading={policies.isLoading} isError={policies.isError} error={policies.error} refetch={() => void policies.refetch()}>
        <div className="page-stack">
          {notice ? <div role={notice.includes('failed') || notice.includes('required') || notice.includes('HTTP') ? 'alert' : 'status'}>{notice}</div> : null}
          <DataTable
            rows={rows}
            columns={[
              { key: 'node', header: t('routePolicy.node'), render: (row) => <strong>{text(policyLabel(row))}</strong> },
              { key: 'role', header: t('nodes.role'), render: (row) => text(row.node_role) },
              { key: 'address', header: t('nodes.address'), render: (row) => text(row.node_address) },
              { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.node_status} /> },
              { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at) },
              {
                key: 'actions',
                header: t('common.actions'),
                render: (row) => (
                  <Toolbar>
                    <Button icon={<Route size={16} />} onClick={() => setSelectedNodeId(row.node_id)}>{t('common.open')}</Button>
                    <Button icon={<Play size={16} />} disabled={busy} onClick={() => { setSelectedNodeId(row.node_id); void runPreview(row); }}>{t('common.preview')}</Button>
                  </Toolbar>
                ),
              },
            ]}
          />

          {selected ? (
            <RoutePolicyDetail
              policy={selected}
              previewResult={previewResult}
              previewFresh={previewFresh}
              previewStale={previewStale}
              previewSuccessful={previewSuccessful}
              busy={busy}
              onPreview={() => void runPreview(selected)}
              onApply={() => setConfirm({ type: 'apply', policy: selected })}
              onCleanup={() => setConfirm({ type: 'cleanup', policy: selected })}
            />
          ) : (
            <Card>
              <CardBody>{t('routePolicy.selectNode')}</CardBody>
            </Card>
          )}

          {applyJob ? (
            <div className="page-stack">
              <Link to="/operations/jobs">{t('jobs.openJobs')}</Link>
              <JobStatusPanel jobID={applyJob.id} />
            </div>
          ) : null}
          {cleanupJob ? (
            <div className="page-stack">
              <Link to="/operations/jobs">{t('jobs.openJobs')}</Link>
              <JobStatusPanel jobID={cleanupJob.id} />
            </div>
          ) : null}
        </div>
        <RoutePolicyConfirmDialog action={confirm} preview={previewResult} previewFresh={previewFresh} busy={busy} onClose={() => setConfirm(null)} onConfirm={() => void runConfirmed()} />
      </QueryBoundary>
    </PageScaffold>
  );
}

function RoutePolicyDetail({ policy, previewResult, previewFresh, previewStale, previewSuccessful, busy, onPreview, onApply, onCleanup }: {
  policy: RoutePolicy;
  previewResult: RoutePolicyPreviewResult | null;
  previewFresh: boolean;
  previewStale: boolean;
  previewSuccessful: boolean;
  busy: boolean;
  onPreview: () => void;
  onApply: () => void;
  onCleanup: () => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const warnings = Array.isArray(previewResult?.warnings) ? previewResult.warnings : [];
  const routes = routeRows(previewResult);
  const systemRoutes = systemRouteRows(previewResult);
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <Badge>{t('routePolicy.backendValidated')}</Badge>
              <StatusBadge status={policy.node_status} />
              {previewFresh ? <Badge>{t('routePolicy.previewFresh')}</Badge> : null}
              {previewStale ? <Badge>{t('routePolicy.previewStale')}</Badge> : null}
            </Toolbar>
            <div>{t('routePolicy.node')}: <strong>{text(policyLabel(policy))}</strong></div>
            <div>{t('nodes.role')}: <strong>{text(policy.node_role)}</strong></div>
            <div>{t('nodes.address')}: <strong>{text(policy.node_address)}</strong></div>
            <div>{t('routePolicy.expectedActions')}: <code>node.route_policy.apply</code>, <code>node.route_policy.cleanup</code></div>
            <Toolbar>
              <Button variant="primary" icon={<Play size={16} />} disabled={busy} onClick={onPreview}>{t('common.preview')}</Button>
              <Button variant="danger" icon={<ShieldCheck size={16} />} disabled={busy || !previewSuccessful} onClick={onApply}>{t('routePolicy.apply')}</Button>
              <Button icon={<RotateCcw size={16} />} disabled={busy} onClick={onCleanup}>{t('routePolicy.cleanup')}</Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>

      {previewResult ? (
        <>
          <Card>
            <CardBody>
              <div className="page-stack">
                <h2 className="card-title">{t('routePolicy.preview')}</h2>
                <div>{t('common.status')}: <strong>{text(previewResult.status)}</strong></div>
                <div>{t('routePolicy.generatedAt')}: <strong>{fmt.date(previewResult.generated_at)}</strong></div>
                <div>{t('routePolicy.revision')}: <code>{text(previewResult.revision)}</code></div>
                <div>{t('routePolicy.outputPath')}: <code>{text(previewResult.output_path)}</code></div>
                {warnings.length ? (
                  <div>
                    <strong>{t('routePolicy.warnings')}</strong>
                    <ul>{warnings.map((warning, index) => <li key={index}>{warningLabel(warning, index)}</li>)}</ul>
                  </div>
                ) : <div>{t('routePolicy.noWarnings')}</div>}
                <strong>{t('routePolicy.summary')}</strong>
                <pre className="code-block">{JSON.stringify(previewResult.summary || {}, null, 2)}</pre>
                <strong>{t('routePolicy.kernel')}</strong>
                <pre className="code-block">{JSON.stringify(previewResult.kernel || {}, null, 2)}</pre>
              </div>
            </CardBody>
          </Card>

          <DataTable
            title={t('routePolicy.routes')}
            rows={routes}
            columns={[
              { key: 'destination', header: t('routePolicy.destination'), render: (route) => text(route.destination || route.dst || route.cidr) },
              { key: 'table', header: t('routePolicy.table'), render: (route) => text(route.table || route.routing_table) },
              { key: 'interface', header: t('routePolicy.interface'), render: (route) => text(route.interface || route.interface_name || route.oif) },
              { key: 'source', header: t('routePolicy.source'), render: (route) => text(route.source_identity || route.source || route.client_id) },
              { key: 'reasons', header: t('routePolicy.reasons'), render: (route) => text(recordString(route.reasons || route.reason)) },
            ]}
          />

          <DataTable
            title={t('routePolicy.systemRoutes')}
            rows={systemRoutes}
            columns={[
              { key: 'destination', header: t('routePolicy.destination'), render: (route) => text(route.destination || route.dst || route.cidr) },
              { key: 'table', header: t('routePolicy.table'), render: (route) => text(route.table || route.routing_table) },
              { key: 'interface', header: t('routePolicy.interface'), render: (route) => text(route.interface || route.interface_name || route.oif) },
              { key: 'state', header: t('common.status'), render: (route) => text(route.status || route.state || route.action) },
            ]}
          />
        </>
      ) : null}
    </div>
  );
}

function RoutePolicyConfirmDialog({ action, preview, previewFresh, busy, onClose, onConfirm }: {
  action: RoutePolicyConfirmAction | null;
  preview: RoutePolicyPreviewResult | null;
  previewFresh: boolean;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  if (!action) return null;
  const applyAllowed = action.type !== 'apply' || previewFresh;
  return (
    <ConfirmDialog title={action.type === 'apply' ? t('routePolicy.confirmApplyTitle') : t('routePolicy.confirmCleanupTitle')} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{action.type === 'apply' ? t('routePolicy.confirmApplyBody', { node: policyLabel(action.policy) }) : t('routePolicy.confirmCleanupBody', { node: policyLabel(action.policy) })}</p>
        {action.type === 'apply' ? (
          <>
            <div>{t('routePolicy.previewRequired')}: <strong>{previewFresh ? t('common.yes') : t('common.no')}</strong></div>
            <div>{t('routePolicy.revision')}: <code>{text(preview?.revision)}</code></div>
          </>
        ) : null}
        {!applyAllowed ? <div role="alert">{t('routePolicy.previewStaleBlocksApply')}</div> : null}
        <Toolbar>
          <Button variant={action.type === 'apply' ? 'danger' : 'primary'} icon={action.type === 'apply' ? <ShieldCheck size={16} /> : <Ban size={16} />} disabled={busy || !applyAllowed} onClick={onConfirm}>
            {t('clients.core.confirm')}
          </Button>
          <Button onClick={onClose}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </ConfirmDialog>
  );
}
