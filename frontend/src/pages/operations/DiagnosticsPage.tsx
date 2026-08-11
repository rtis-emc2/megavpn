import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { Job, NodeEntity } from '../../shared/api/types';
import { useDashboard, useJobs, useNodes, useReady, useRuntimePreflight } from '../../shared/query/hooks';
import { DataTable, MetricCard, RefreshButton, StatusBadge } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

function normalizedStatus(value: unknown): string {
  return String(value || 'unknown').trim().toLowerCase();
}

function nodeNeedsAttention(node: NodeEntity): boolean {
  return !['online', 'active', 'healthy'].includes(normalizedStatus(node.status))
    || !['online', 'active', 'healthy'].includes(normalizedStatus(node.agent_status || node.agent_channel_status));
}

function jobNeedsAttention(job: Job): boolean {
  return ['failed', 'retrying', 'running', 'queued'].includes(normalizedStatus(job.status));
}

function jobResult(job: Job): string {
  const result = job.result;
  if (!result) return 'n/a';
  for (const key of ['message', 'error', 'summary']) {
    if (typeof result[key] === 'string' && result[key]) return result[key] as string;
  }
  const encoded = JSON.stringify(result);
  return encoded.length > 180 ? `${encoded.slice(0, 177)}...` : encoded;
}

export function DiagnosticsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const ready = useReady({ refetchInterval: false, retry: false });
  const preflight = useRuntimePreflight({ retry: false });
  const dashboard = useDashboard({ refetchInterval: false, retry: false });
  const nodes = useNodes({ retry: false });
  const jobs = useJobs({ refetchInterval: false, retry: false });

  const attentionNodes = useMemo(() => (nodes.data || []).filter(nodeNeedsAttention), [nodes.data]);
  const attentionJobs = useMemo(
    () => (jobs.data || []).filter(jobNeedsAttention).sort((left, right) => String(right.created_at || '').localeCompare(String(left.created_at || ''))).slice(0, 50),
    [jobs.data],
  );
  const checks = preflight.data?.checks || [];
  const passingChecks = checks.filter((check) => ['ok', 'ready', 'healthy', 'passed'].includes(normalizedStatus(check.status))).length;
  const loading = ready.isLoading || preflight.isLoading || dashboard.isLoading || nodes.isLoading || jobs.isLoading;
  const failedQuery = ready.error || preflight.error || dashboard.error || nodes.error || jobs.error;
  const refresh = () => Promise.all([ready.refetch(), preflight.refetch(), dashboard.refetch(), nodes.refetch(), jobs.refetch()]);

  return (
    <PageScaffold
      title={t('diagnosticsPage.title')}
      subtitle={t('diagnosticsPage.subtitle')}
      actions={<RefreshButton onRefresh={refresh}>{t('common.refresh')}</RefreshButton>}
    >
      <QueryBoundary isLoading={loading} isError={Boolean(failedQuery)} error={failedQuery as Error | null} refetch={() => void refresh()}>
        <div className="metric-grid">
          <MetricCard
            label={t('diagnosticsPage.controlPlane')}
            value={<StatusBadge status={ready.data?.status || preflight.data?.status || 'unknown'} />}
            caption={t('diagnosticsPage.releaseValue', { version: ready.data?.version || preflight.data?.version || dashboard.data?.version || 'n/a' })}
          />
          <MetricCard
            label={t('diagnosticsPage.runtimePreflight')}
            value={`${passingChecks}/${checks.length}`}
            caption={t('diagnosticsPage.checksPassing')}
          />
          <MetricCard
            label={t('diagnosticsPage.nodes')}
            value={`${Math.max(0, (nodes.data || []).length - attentionNodes.length)}/${(nodes.data || []).length}`}
            caption={attentionNodes.length ? t('diagnosticsPage.nodesAttention', { count: attentionNodes.length }) : t('diagnosticsPage.nodesHealthy')}
          />
          <MetricCard
            label={t('diagnosticsPage.jobs')}
            value={dashboard.data?.jobs_active ?? attentionJobs.filter((job) => normalizedStatus(job.status) !== 'failed').length}
            caption={t('diagnosticsPage.jobsFailed', { count: dashboard.data?.jobs_failed ?? attentionJobs.filter((job) => normalizedStatus(job.status) === 'failed').length })}
          />
        </div>

        <DataTable
          title={t('diagnosticsPage.preflightChecks')}
          rows={checks}
          columns={[
            { key: 'check', header: t('diagnosticsPage.check'), render: (row) => <strong>{t(`settings.checkNames.${row.code}`, { defaultValue: row.code })}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'summary', header: t('diagnosticsPage.summary'), render: (row) => text(row.summary) },
            { key: 'detail', header: t('diagnosticsPage.detail'), render: (row) => text(row.detail) },
          ]}
        />

        <DataTable
          title={t('diagnosticsPage.nodeHealth')}
          rows={attentionNodes}
          empty={<div className="empty-state"><strong>{t('diagnosticsPage.noNodeAttention')}</strong></div>}
          columns={[
            { key: 'node', header: t('diagnosticsPage.node'), render: (row) => <><strong>{text(row.name || row.id)}</strong><br /><code>{text(row.address)}</code></> },
            { key: 'role', header: t('diagnosticsPage.role'), render: (row) => text(row.role) },
            { key: 'nodeStatus', header: t('diagnosticsPage.nodeStatus'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'agent', header: t('diagnosticsPage.agent'), render: (row) => <StatusBadge status={row.agent_status || row.agent_channel_status} /> },
            { key: 'lastSeen', header: t('diagnosticsPage.lastSeen'), render: (row) => fmt.date(row.agent_last_seen_at || row.last_heartbeat_at) },
          ]}
        />

        <DataTable
          title={t('diagnosticsPage.workAttention')}
          rows={attentionJobs}
          empty={<div className="empty-state"><strong>{t('diagnosticsPage.noWorkAttention')}</strong></div>}
          responsive="wide"
          columns={[
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
            { key: 'type', header: t('diagnosticsPage.type'), render: (row) => <code>{text(row.type)}</code> },
            { key: 'scope', header: t('common.scope'), render: (row) => <code>{text(row.scope_type)}:{shortID(text(row.scope_id))}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'result', header: t('diagnosticsPage.result'), render: (row) => jobResult(row) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
