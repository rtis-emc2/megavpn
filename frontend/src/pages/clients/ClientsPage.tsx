import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { useClients } from '../../shared/query/hooks';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function ClientsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const clients = useClients();
  return (
    <PageScaffold title={t('clients.title')} subtitle={t('clients.subtitle')} actions={<Button disabled>{t('common.create')}</Button>}>
      <QueryBoundary isLoading={clients.isLoading} isError={clients.isError} error={clients.error} refetch={() => void clients.refetch()}>
        <DataTable
          rows={clients.data || []}
          columns={[
            { key: 'username', header: t('clients.username'), render: (row) => <strong>{text(row.username || row.id)}</strong> },
            { key: 'display', header: t('clients.displayName'), render: (row) => text(row.display_name) },
            { key: 'email', header: t('common.email'), render: (row) => text(row.email) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
            { key: 'actions', header: t('common.actions'), render: () => <Link to="/clients/groups">{t('clients.openGroups')}</Link> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
