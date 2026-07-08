import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../../shared/api/endpoints';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function ServicePacksPage() {
  const { t } = useTranslation();
  const packs = useQuery({ queryKey: ['service-packs'], queryFn: endpoints.servicePacks, staleTime: 120_000 });
  return (
    <PageScaffold title={t('instances.servicePacksTitle')} subtitle={t('instances.subtitle')} actions={<Button disabled>{t('common.create')}</Button>}>
      <QueryBoundary isLoading={packs.isLoading} isError={packs.isError} error={packs.error} refetch={() => void packs.refetch()}>
        <DataTable
          rows={packs.data || []}
          columns={[
            { key: 'key', header: t('common.name'), render: (row) => <strong>{text(row.label || row.key)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'active')} /> },
            { key: 'source', header: t('common.type'), render: (row) => text(row.source) },
            { key: 'components', header: t('nav.instances'), render: (row) => String(Array.isArray(row.components) ? row.components.length : 0) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
