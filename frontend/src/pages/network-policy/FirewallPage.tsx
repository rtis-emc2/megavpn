import { useTranslation } from 'react-i18next';
import { useFirewallInventory } from '../../shared/query/hooks';
import { Badge, Button, DataTable, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function FirewallPage() {
  const { t } = useTranslation();
  const firewall = useFirewallInventory();

  return (
    <PageScaffold title={t('firewall.title')} subtitle={t('firewall.subtitle')}>
      <div className="toolbar">
        <Badge>{t('firewall.previewMandatory')}</Badge>
        <Badge>{t('firewall.sshSafety')}</Badge>
        <Badge>{t('firewall.forwardOutputSafety')}</Badge>
        <Button disabled title={t('common.unsupportedAction')}>{t('common.preview')}</Button>
        <Button variant="primary" disabled title={t('common.unsupportedAction')}>{t('common.apply')}</Button>
        <Button variant="danger" disabled title={t('common.unsupportedAction')}>{t('firewall.emergencyDisable')}</Button>
        <a className="button button-secondary" href="/legacy/">{t('common.legacy')}</a>
      </div>
      <QueryBoundary isLoading={firewall.isLoading} isError={firewall.isError} error={firewall.error} refetch={() => void firewall.refetch()}>
        <DataTable
          title={t('firewall.policies')}
          rows={firewall.data?.policies || []}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.label || row.key || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'default', header: t('common.type'), render: (row) => text(row.default_action) },
            { key: 'scope', header: t('common.scope'), render: (row) => text(row.scope) },
          ]}
        />
        <DataTable
          title={t('firewall.rules')}
          rows={firewall.data?.rules || []}
          columns={[
            { key: 'priority', header: '#', render: (row) => text(row.priority) },
            { key: 'chain', header: t('common.scope'), render: (row) => text(row.chain) },
            { key: 'protocol', header: t('common.type'), render: (row) => text(row.protocol) },
            { key: 'action', header: t('common.actions'), render: (row) => <StatusBadge status={row.action} /> },
            { key: 'comment', header: t('common.description'), render: (row) => text(row.comment) },
          ]}
        />
        <DataTable
          title={t('firewall.addressGroups')}
          rows={firewall.data?.address_lists || []}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.label || row.key || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'count', header: t('firewall.semanticGroups'), render: (row) => text(row.entry_count) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
