import { useTranslation } from 'react-i18next';
import { trafficAccountingExportURL } from '../../shared/api/endpoints';
import { useTrafficAccounting } from '../../shared/query/hooks';
import { Button, DataTable, MetricCard, StatusBadge } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function TrafficPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const traffic = useTrafficAccounting();
  const summary = traffic.data?.summary || {};

  return (
    <PageScaffold
      title={t('traffic.title')}
      subtitle={t('traffic.subtitle')}
      actions={<Button onClick={() => window.open(trafficAccountingExportURL(), '_blank', 'noopener,noreferrer')}>{t('traffic.export')}</Button>}
    >
      <div className="metric-grid">
        <MetricCard label={t('traffic.bytesIn')} value={fmt.bytes(summary.bytes_in || 0)} caption={t('traffic.noStoreExport')} />
        <MetricCard label={t('traffic.bytesOut')} value={fmt.bytes(summary.bytes_out || 0)} caption={t('traffic.retention')} />
        <MetricCard label={t('traffic.clients')} value={fmt.number((traffic.data?.clients || []).length)} />
        <MetricCard label={t('traffic.collectors')} value={fmt.number((traffic.data?.collectors || []).length)} />
      </div>
      <QueryBoundary isLoading={traffic.isLoading} isError={traffic.isError} error={traffic.error} refetch={() => void traffic.refetch()}>
        <DataTable
          title={t('traffic.clients')}
          rows={traffic.data?.clients || []}
          columns={[
            { key: 'client', header: t('clients.client'), render: (row) => text(row.client_name || row.client_id) },
            { key: 'in', header: t('traffic.bytesIn'), render: (row) => fmt.bytes(row.bytes_in) },
            { key: 'out', header: t('traffic.bytesOut'), render: (row) => fmt.bytes(row.bytes_out) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'active')} /> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
