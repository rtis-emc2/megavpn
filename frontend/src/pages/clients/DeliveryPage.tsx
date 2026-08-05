import { useTranslation } from 'react-i18next';
import { useArtifacts, useShareLinks } from '../../shared/query/hooks';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function DeliveryPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const artifacts = useArtifacts();
  const shares = useShareLinks({ retry: false });
  return (
    <PageScaffold title={t('artifacts.title')} subtitle={t('artifacts.subtitle')}>
      <QueryBoundary isLoading={artifacts.isLoading} isError={artifacts.isError} error={artifacts.error} refetch={() => void artifacts.refetch()}>
        <DataTable
          title={t('artifacts.artifact')}
          rows={artifacts.data || []}
          columns={[
            { key: 'type', header: t('common.type'), render: (row) => <code>{text(row.artifact_type || row.type)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'size', header: t('artifacts.size'), render: (row) => fmt.bytes(row.size_bytes) },
            { key: 'path', header: t('artifacts.path'), render: (row) => <code>{text(row.storage_path || row.path)}</code> },
            { key: 'actions', header: t('common.actions'), render: () => <Button disabled>{t('common.download')}</Button> },
          ]}
        />
      </QueryBoundary>
      <QueryBoundary isLoading={shares.isLoading} isError={shares.isError} error={shares.error} refetch={() => void shares.refetch()}>
        <DataTable
          title={t('artifacts.shareLinks')}
          rows={shares.data || []}
          columns={[
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'token', header: t('artifacts.tokenHint'), render: (row) => <code>{text(row.token_hint)}</code> },
            { key: 'expires', header: t('common.expires'), render: (row) => fmt.date(row.expires_at) },
            { key: 'actions', header: t('common.actions'), render: () => <Button disabled>{t('artifacts.revoke')}</Button> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
