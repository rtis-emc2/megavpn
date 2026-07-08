import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNodes } from '../../shared/query/hooks';
import { Button, DataTable, Drawer, StatusBadge } from '../../shared/ui';
import type { NodeEntity } from '../../shared/api/types';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

export function NodesPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const nodes = useNodes();
  const [selected, setSelected] = useState<NodeEntity | null>(null);

  return (
    <PageScaffold
      title={t('nodes.title')}
      subtitle={t('nodes.subtitle')}
      actions={<Button disabled title={t('common.permissionRequired', { permission: 'node.write' })}>{t('nodes.createNode')}</Button>}
    >
      <QueryBoundary isLoading={nodes.isLoading} isError={nodes.isError} error={nodes.error} refetch={() => void nodes.refetch()}>
        <DataTable
          rows={nodes.data || []}
          columns={[
            { key: 'node', header: t('nodes.node'), render: (row) => <strong>{text(row.name || row.id)}</strong> },
            { key: 'role', header: t('nodes.role'), render: (row) => text(row.role) },
            { key: 'address', header: t('nodes.address'), render: (row) => <code>{text(row.address)}</code> },
            { key: 'agent', header: t('nodes.agent'), render: (row) => <StatusBadge status={row.agent_status || row.status} /> },
            { key: 'heartbeat', header: t('nodes.heartbeat'), render: (row) => fmt.date(row.last_heartbeat_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button onClick={() => setSelected(row)}>{t('common.open')}</Button> },
          ]}
        />
      </QueryBoundary>
      <Drawer open={Boolean(selected)} onClose={() => setSelected(null)} title={t('nodes.detailTitle')}>
        {selected ? (
          <div className="page-stack">
            <StatusBadge status={selected.agent_status || selected.status} />
            <pre className="code-block">{JSON.stringify(selected, null, 2)}</pre>
            <Button disabled>{t('nodes.control')}</Button>
            <Button disabled>{t('nodes.bootstrap')}</Button>
            <Button disabled>{t('nodes.routePolicy')}</Button>
          </div>
        ) : null}
      </Drawer>
    </PageScaffold>
  );
}
