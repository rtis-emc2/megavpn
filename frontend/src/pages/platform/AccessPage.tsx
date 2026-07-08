import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../../shared/api/endpoints';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function AccessPage() {
  const { t } = useTranslation();
  const users = useQuery({ queryKey: ['admin-users'], queryFn: endpoints.users, retry: false, staleTime: 30_000 });
  const sessions = useQuery({ queryKey: ['admin-sessions'], queryFn: endpoints.sessions, retry: false, staleTime: 30_000 });
  return (
    <PageScaffold title={t('nav.access')} subtitle={t('settings.security')} actions={<Button disabled>{t('common.create')}</Button>}>
      <QueryBoundary isLoading={users.isLoading} isError={users.isError} error={users.error} refetch={() => void users.refetch()}>
        <DataTable
          title={t('settings.users')}
          rows={users.data || []}
          columns={[
            { key: 'username', header: t('clients.username'), render: (row) => <strong>{text(row.username || row.id)}</strong> },
            { key: 'email', header: t('common.email'), render: (row) => text(row.email) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'unknown')} /> },
            { key: 'roles', header: t('common.roles'), render: (row) => text(Array.isArray(row.roles) ? row.roles.join(', ') : row.roles) },
          ]}
        />
      </QueryBoundary>
      <QueryBoundary isLoading={sessions.isLoading} isError={sessions.isError} error={sessions.error} refetch={() => void sessions.refetch()}>
        <DataTable
          title={t('settings.sessions')}
          rows={sessions.data || []}
          columns={[
            { key: 'id', header: 'ID', render: (row) => <code>{text(row.id)}</code> },
            { key: 'user', header: t('common.account'), render: (row) => text(row.username || row.user_id) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'active')} /> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
