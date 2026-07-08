import { useTranslation } from 'react-i18next';
import { useInstances } from '../../shared/query/hooks';
import { Badge, Button, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function RevisionsPage() {
  const { t } = useTranslation();
  const instances = useInstances();
  return (
    <PageScaffold title={t('instances.revisionsTitle')} subtitle={t('instances.applyExplicit')}>
      <Badge>{t('instances.applyExplicit')}</Badge>
      <QueryBoundary isLoading={instances.isLoading} isError={instances.isError} error={instances.error} refetch={() => void instances.refetch()}>
        <DataTable
          rows={instances.data || []}
          columns={[
            { key: 'name', header: t('instances.instance'), render: (row) => <strong>{text(row.name || row.slug || row.id)}</strong> },
            { key: 'current', header: t('instances.revision'), render: (row) => <code>{text(row.current_revision_id)}</code> },
            { key: 'applied', header: t('common.status'), render: (row) => <StatusBadge status={row.current_revision_id === row.last_applied_revision_id ? 'applied' : 'pending'} /> },
            { key: 'actions', header: t('common.actions'), render: () => <Button disabled>{t('common.preview')}</Button> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
