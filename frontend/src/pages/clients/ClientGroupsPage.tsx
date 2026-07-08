import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useClientAccessGroups, useClientAccessServices } from '../../shared/query/hooks';
import { Badge, Button, DataTable, Select, StatusBadge } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function ClientGroupsPage() {
  const { t } = useTranslation();
  const services = useClientAccessServices();
  const groups = useClientAccessGroups();
  const [serviceFilter, setServiceFilter] = useState('all');

  const filtered = useMemo(() => {
    const source = groups.data || [];
    return serviceFilter === 'all' ? source : source.filter((group) => group.service_code === serviceFilter);
  }, [groups.data, serviceFilter]);

  return (
    <PageScaffold
      title={t('clients.groupsTitle')}
      subtitle={t('clients.groupsSubtitle')}
      actions={<Button disabled>{t('common.create')}</Button>}
    >
      <div className="toolbar">
        <label className="form-field">
          <span className="form-label">{t('clients.serviceFilter')}</span>
          <Select value={serviceFilter} onChange={(event) => setServiceFilter(event.currentTarget.value)}>
            <option value="all">{t('common.all')}</option>
            {(services.data || []).map((service) => (
              <option value={service.service_code} key={service.service_code}>
                {service.display_name || service.service_code} · {service.status || 'unknown'}
              </option>
            ))}
          </Select>
        </label>
        <Badge>{t('clients.previewRequired')}</Badge>
        <Badge>{t('clients.bulkBounded')}</Badge>
      </div>
      <QueryBoundary isLoading={groups.isLoading} isError={groups.isError} error={groups.error} refetch={() => void groups.refetch()}>
        <DataTable
          rows={filtered}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.display_name || row.group_key || row.id)}</strong> },
            { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'members', header: t('clients.members'), render: (row) => text(row.member_count || 0) },
            { key: 'scope', header: t('common.scope'), render: (row) => text(row.scope_mode) },
            { key: 'actions', header: t('common.actions'), render: () => (
              <div className="toolbar">
                <Button disabled title={t('common.unsupportedAction')}>{t('common.preview')}</Button>
                <Button variant="primary" disabled title={t('common.unsupportedAction')}>{t('common.apply')}</Button>
                <a className="button button-secondary" href="/legacy/">{t('common.legacy')}</a>
              </div>
            ) },
          ]}
        />
      </QueryBoundary>
    </PageScaffold>
  );
}
