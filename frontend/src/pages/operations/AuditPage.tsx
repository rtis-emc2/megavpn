import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../../shared/api/endpoints';
import { DataTable, StatusBadge } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function AuditPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const audit = useQuery({ queryKey: ['audit'], queryFn: endpoints.audit, staleTime: 30_000, retry: false });
  return (
    <PageScaffold title={t('nav.audit')} subtitle={t('jobs.subtitle')}>
      <QueryBoundary isLoading={audit.isLoading} isError={audit.isError} error={audit.error} refetch={() => void audit.refetch()}>
        <DataTable
          rows={audit.data || []}
          columns={[
            { key: 'time', header: t('common.created'), render: (row) => fmt.date(row.created_at || row.time) },
            { key: 'type', header: t('common.type'), render: (row) => <code>{text(row.event_type || row.type)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'recorded')} /> },
            { key: 'scope', header: t('common.scope'), render: (row) => text(row.scope || row.entity_id) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
