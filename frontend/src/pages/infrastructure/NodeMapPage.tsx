import { useTranslation } from 'react-i18next';
import { useBackhaulLinks, useNodes } from '../../shared/query/hooks';
import { Card, CardBody, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function NodeMapPage() {
  const { t } = useTranslation();
  const nodes = useNodes();
  const links = useBackhaulLinks({ retry: false });
  const mapped = (nodes.data || []).filter((node) => Number.isFinite(Number(node.latitude)) && Number.isFinite(Number(node.longitude)));
  const pending = (nodes.data || []).filter((node) => !mapped.includes(node));

  return (
    <PageScaffold title={t('nodes.mapTitle')} subtitle={t('nodes.mapSubtitle')}>
      <QueryBoundary isLoading={nodes.isLoading} isError={nodes.isError} error={nodes.error} refetch={() => void nodes.refetch()}>
        <Card>
          <CardBody>
            <div className="metric-grid">
              <div><strong>{t('nodes.mappedNodes')}</strong><div className="metric-value">{mapped.length}</div></div>
              <div><strong>{t('nodes.geoPending')}</strong><div className="metric-value">{pending.length}</div></div>
              <div><strong>{t('nav.backhaul')}</strong><div className="metric-value">{links.data?.length || 0}</div></div>
            </div>
          </CardBody>
        </Card>
        <DataTable
          title={t('nodes.mappedNodes')}
          rows={mapped}
          columns={[
            { key: 'name', header: t('nodes.node'), render: (row) => text(row.name || row.id) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status || row.agent_status} /> },
            { key: 'lat', header: 'Lat', render: (row) => text(row.latitude) },
            { key: 'lon', header: 'Lon', render: (row) => text(row.longitude) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
