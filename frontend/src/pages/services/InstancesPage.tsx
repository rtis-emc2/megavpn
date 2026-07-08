import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { useInstances, useNodes } from '../../shared/query/hooks';
import { Badge, Button, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function InstancesPage() {
  const { t } = useTranslation();
  const instances = useInstances();
  const nodes = useNodes({ retry: false });
  const nodeName = (id?: string) => nodes.data?.find((node) => node.id === id)?.name || id || 'n/a';

  return (
    <PageScaffold
      title={t('instances.title')}
      subtitle={t('instances.subtitle')}
      actions={<><Button disabled>{t('instances.createFromPack')}</Button><Button disabled>{t('instances.manualInstance')}</Button></>}
    >
      <Badge>{t('instances.applyExplicit')}</Badge>
      <Badge>{t('instances.accessGroupsReadOnly')} <Link to="/clients/groups">{t('clients.openGroups')}</Link></Badge>
      <QueryBoundary isLoading={instances.isLoading} isError={instances.isError} error={instances.error} refetch={() => void instances.refetch()}>
        <DataTable
          rows={instances.data || []}
          columns={[
            { key: 'name', header: t('instances.instance'), render: (row) => <strong>{text(row.name || row.slug || row.id)}</strong> },
            { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'node', header: t('instances.node'), render: (row) => text(nodeName(row.node_id)) },
            { key: 'endpoint', header: t('instances.endpoint'), render: (row) => <code>{[row.endpoint_host, row.endpoint_port].filter(Boolean).join(':') || 'n/a'}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'actions', header: t('common.actions'), render: () => <Button disabled>{t('common.apply')}</Button> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
