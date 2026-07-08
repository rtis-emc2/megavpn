import { Activity, Server, Users, Layers3 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useClients, useDashboard, useInstances, useJobs, useNodes, useReady, useTrafficAccounting } from '../../shared/query/hooks';
import { DataTable, MetricCard, StatusBadge } from '../../shared/ui';
import { useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function DashboardPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const ready = useReady();
  const dashboard = useDashboard();
  const nodes = useNodes();
  const instances = useInstances();
  const clients = useClients();
  const jobs = useJobs();
  const traffic = useTrafficAccounting({ retry: false });

  const activeJobs = (jobs.data || []).filter((job) => ['queued', 'running', 'pending'].includes(String(job.status || '').toLowerCase()));
  const failedJobs = (jobs.data || []).filter((job) => ['failed', 'error'].includes(String(job.status || '').toLowerCase()));

  return (
    <PageScaffold title={t('dashboard.title')} subtitle={t('dashboard.subtitle')}>
      <div className="metric-grid">
        <MetricCard label={t('dashboard.ready')} value={<StatusBadge status={ready.data?.status} />} caption={ready.data?.time || dashboard.data?.version || 'n/a'} />
        <MetricCard label={t('dashboard.nodes')} value={fmt.number(nodes.data?.length || dashboard.data?.nodes_total || 0)} caption={<Server size={14} />} />
        <MetricCard label={t('dashboard.instances')} value={fmt.number(instances.data?.length || dashboard.data?.instances_total || 0)} caption={<Layers3 size={14} />} />
        <MetricCard label={t('dashboard.clients')} value={fmt.number(clients.data?.length || dashboard.data?.clients_total || 0)} caption={<Users size={14} />} />
        <MetricCard label={t('dashboard.activeJobs')} value={fmt.number(activeJobs.length)} caption={<Activity size={14} />} />
        <MetricCard label={t('dashboard.failedJobs')} value={fmt.number(failedJobs.length)} caption={failedJobs[0]?.type || 'n/a'} />
        <MetricCard label={t('dashboard.traffic')} value={fmt.bytes((traffic.data?.summary as Record<string, unknown> | undefined)?.bytes_total || 0)} caption={t('traffic.retention')} />
      </div>
      <QueryBoundary isLoading={jobs.isLoading} isError={jobs.isError} error={jobs.error} refetch={() => void jobs.refetch()}>
        <DataTable
          title={t('dashboard.recentFailures')}
          rows={failedJobs.slice(0, 8)}
          columns={[
            { key: 'type', header: t('jobs.kind'), render: (row) => row.type || 'n/a' },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'scope', header: t('jobs.scope'), render: (row) => [row.scope_type, row.scope_id].filter(Boolean).join(':') || 'n/a' },
            { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
