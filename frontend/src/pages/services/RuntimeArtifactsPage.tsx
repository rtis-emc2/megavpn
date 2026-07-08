import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../../shared/api/endpoints';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function RuntimeArtifactsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const artifacts = useQuery({ queryKey: ['binary-artifacts'], queryFn: endpoints.binaryArtifacts, staleTime: 60_000, retry: false });
  return (
    <PageScaffold title={t('instances.runtimeArtifactsTitle')} subtitle={t('instances.runtimeArtifactsTitle')} actions={<Button disabled>{t('common.create')}</Button>}>
      <QueryBoundary isLoading={artifacts.isLoading} isError={artifacts.isError} error={artifacts.error} refetch={() => void artifacts.refetch()}>
        <DataTable
          rows={artifacts.data || []}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.name || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'unknown')} /> },
            { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'size', header: t('artifacts.size'), render: (row) => fmt.bytes(row.size_bytes) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
