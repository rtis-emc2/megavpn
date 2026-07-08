import { useTranslation } from 'react-i18next';
import { useCertificates } from '../../shared/query/hooks';
import { Button, DataTable, StatusBadge } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function CertificatesPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const certificates = useCertificates();
  return (
    <PageScaffold
      title={t('certificates.title')}
      subtitle={t('certificates.subtitle')}
      actions={<><Button disabled>{t('certificates.import')}</Button><Button disabled>{t('certificates.selfSigned')}</Button></>}
    >
      <QueryBoundary isLoading={certificates.isLoading} isError={certificates.isError} error={certificates.error} refetch={() => void certificates.refetch()}>
        <DataTable
          rows={certificates.data || []}
          columns={[
            { key: 'cn', header: t('certificates.commonName'), render: (row) => <strong>{text(row.common_name || row.id)}</strong> },
            { key: 'issuer', header: t('certificates.issuer'), render: (row) => text(row.issuer_name) },
            { key: 'kind', header: t('common.type'), render: (row) => text(row.kind || row.source) },
            { key: 'expiry', header: t('certificates.expiry'), render: (row) => fmt.date(row.not_after) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
