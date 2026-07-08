import { useTranslation } from 'react-i18next';
import { useBackhaulLinks } from '../../shared/query/hooks';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function BackhaulPage() {
  const { t } = useTranslation();
  const links = useBackhaulLinks();
  return (
    <PageScaffold title={t('backhaul.title')} subtitle={t('backhaul.subtitle')}>
      <QueryBoundary isLoading={links.isLoading} isError={links.isError} error={links.error} refetch={() => void links.refetch()}>
        <DataTable
          rows={links.data || []}
          columns={[
            { key: 'name', header: t('backhaul.link'), render: (row) => <strong>{text(row.name || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'ingress', header: t('backhaul.ingress'), render: (row) => text(row.ingress_node_id) },
            { key: 'egress', header: t('backhaul.egress'), render: (row) => text(row.egress_node_id) },
            { key: 'driver', header: t('backhaul.driver'), render: (row) => text(row.driver) },
            { key: 'actions', header: t('common.actions'), render: () => <Button disabled>{t('backhaul.probe')}</Button> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
