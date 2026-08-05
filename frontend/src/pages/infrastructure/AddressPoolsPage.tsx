import { useTranslation } from 'react-i18next';
import { useAddressPools } from '../../shared/query/hooks';
import { DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function AddressPoolsPage() {
  const { t } = useTranslation();
  const pools = useAddressPools();
  return (
    <PageScaffold title={t('nav.addressPools')} subtitle={t('instances.subtitle')}>
      <QueryBoundary isLoading={pools.isLoading} isError={pools.isError} error={pools.error} refetch={() => void pools.refetch()}>
        <DataTable
          rows={pools.data?.spaces || []}
          columns={[
            { key: 'key', header: t('common.name'), render: (row) => <strong>{text(row.label || row.key || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'active')} /> },
            { key: 'cidr', header: 'CIDR', render: (row) => <code>{text(row.cidr || row.network)}</code> },
            { key: 'routing', header: t('backhaul.routeProjection'), render: (row) => text(row.routing_enabled) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
