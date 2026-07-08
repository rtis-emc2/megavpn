import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../../shared/api/endpoints';
import { Button, Card, CardBody, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function SettingsPage() {
  const { t } = useTranslation();
  const preflight = useQuery({ queryKey: ['runtime-preflight'], queryFn: endpoints.runtimePreflight, retry: false, staleTime: 30_000 });
  const tls = useQuery({ queryKey: ['control-plane-tls'], queryFn: endpoints.controlPlaneTLS, retry: false, staleTime: 30_000 });

  const checks = Object.entries(preflight.data || {}).map(([key, value]) => ({
    key,
    value,
  }));

  return (
    <PageScaffold title={t('settings.title')} subtitle={t('settings.subtitle')} actions={<Button disabled>{t('common.save')}</Button>}>
      <QueryBoundary isLoading={preflight.isLoading} isError={preflight.isError} error={preflight.error} refetch={() => void preflight.refetch()}>
        <DataTable
          title={t('settings.preflight')}
          rows={checks}
          columns={[
            { key: 'key', header: t('common.name'), render: (row) => <code>{row.key}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={typeof row.value === 'object' && row.value && 'status' in row.value ? String(row.value.status) : 'unknown'} /> },
            { key: 'value', header: t('common.description'), render: (row) => <code>{text(JSON.stringify(row.value))}</code> },
          ]}
        />
      </QueryBoundary>
      <Card>
        <CardBody>
          <div className="page-stack">
            <h2 className="card-title">{t('settings.tls')}</h2>
            <StatusBadge status={tls.isError ? 'failed' : tls.isLoading ? 'pending' : 'configured'} />
            <pre className="code-block">{JSON.stringify(tls.data || {}, null, 2)}</pre>
          </div>
        </CardBody>
      </Card>
    </PageScaffold>
  );
}
