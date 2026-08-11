import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import type { AuditEvent } from '../../shared/api/types';
import { endpoints } from '../../shared/api/endpoints';
import {
  Button,
  Card,
  CardBody,
  DataTable,
  Drawer,
  FormField,
  FormGrid,
  RefreshButton,
  Select,
  TextField,
} from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

function readableIdentifier(value: string): string {
  return value
    .split(/[._-]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

function actorLabel(event: AuditEvent, translate: TFunction): string {
  if (event.actor_display_name) return event.actor_display_name;
  if (event.actor_username) return event.actor_username;
  if (event.actor_email) return event.actor_email;
  if (event.actor_user_id) return shortID(event.actor_user_id);
  return translate(`auditLog.actorTypes.${event.actor_type}`, { defaultValue: readableIdentifier(event.actor_type) });
}

function resourceLabel(event: AuditEvent, translate: TFunction): string {
  return translate(`auditLog.resourceTypes.${event.resource_type}`, {
    defaultValue: readableIdentifier(event.resource_type),
  });
}

function matchesSearch(event: AuditEvent, query: string): boolean {
  if (!query) return true;
  return [
    event.summary,
    event.action,
    event.resource_type,
    event.resource_id,
    event.actor_type,
    event.actor_username,
    event.actor_email,
    event.actor_display_name,
    event.actor_user_id,
  ].some((value) => String(value || '').toLowerCase().includes(query));
}

export function AuditPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const audit = useQuery({ queryKey: ['audit'], queryFn: endpoints.audit, staleTime: 30_000, retry: false });
  const [search, setSearch] = useState('');
  const [actorType, setActorType] = useState('');
  const [resourceType, setResourceType] = useState('');
  const [selected, setSelected] = useState<AuditEvent | null>(null);

  const rows = useMemo(() => audit.data || [], [audit.data]);
  const actorTypes = useMemo(() => [...new Set(rows.map((event) => event.actor_type).filter(Boolean))].sort(), [rows]);
  const resourceTypes = useMemo(() => [...new Set(rows.map((event) => event.resource_type).filter(Boolean))].sort(), [rows]);
  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return rows.filter((event) => (
      (!actorType || event.actor_type === actorType)
      && (!resourceType || event.resource_type === resourceType)
      && matchesSearch(event, query)
    ));
  }, [actorType, resourceType, rows, search]);

  const clearFilters = () => {
    setSearch('');
    setActorType('');
    setResourceType('');
  };

  return (
    <PageScaffold
      title={t('auditLog.title')}
      subtitle={t('auditLog.subtitle')}
      actions={<RefreshButton onRefresh={() => audit.refetch()}>{t('common.refresh')}</RefreshButton>}
    >
      <Card>
        <CardBody>
          <FormGrid>
            <FormField label={t('common.search')}>
              <TextField
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t('auditLog.searchPlaceholder')}
              />
            </FormField>
            <FormField label={t('auditLog.actor')}>
              <Select value={actorType} onChange={(event) => setActorType(event.target.value)}>
                <option value="">{t('auditLog.allActors')}</option>
                {actorTypes.map((value) => (
                  <option key={value} value={value}>
                    {t(`auditLog.actorTypes.${value}`, { defaultValue: readableIdentifier(value) })}
                  </option>
                ))}
              </Select>
            </FormField>
            <FormField label={t('auditLog.resource')}>
              <Select value={resourceType} onChange={(event) => setResourceType(event.target.value)}>
                <option value="">{t('auditLog.allResources')}</option>
                {resourceTypes.map((value) => (
                  <option key={value} value={value}>
                    {t(`auditLog.resourceTypes.${value}`, { defaultValue: readableIdentifier(value) })}
                  </option>
                ))}
              </Select>
            </FormField>
            <div className="form-field audit-filter-actions">
              <span className="form-label">{t('auditLog.shown')}</span>
              <div className="toolbar">
                <strong>{t('auditLog.eventCount', { count: filtered.length })}</strong>
                <Button disabled={!search && !actorType && !resourceType} onClick={clearFilters}>{t('auditLog.clearFilters')}</Button>
              </div>
            </div>
          </FormGrid>
        </CardBody>
      </Card>

      <QueryBoundary isLoading={audit.isLoading} isError={audit.isError} error={audit.error} refetch={() => void audit.refetch()}>
        <DataTable
          rows={filtered}
          responsive="wide"
          columns={[
            { key: 'time', header: t('auditLog.time'), priority: 'high', render: (row) => fmt.date(row.created_at) },
            { key: 'event', header: t('auditLog.event'), priority: 'high', render: (row) => (
              <div className="table-primary-cell">
                <strong>{text(row.summary, readableIdentifier(row.action))}</strong>
                <code>{row.action}</code>
              </div>
            ) },
            { key: 'actor', header: t('auditLog.actor'), priority: 'medium', render: (row) => (
              <div className="table-primary-cell">
                <strong>{actorLabel(row, t)}</strong>
                {row.actor_email && row.actor_email !== actorLabel(row, t) ? <span className="muted">{row.actor_email}</span> : null}
                <span className="muted">{t(`auditLog.actorTypes.${row.actor_type}`, { defaultValue: readableIdentifier(row.actor_type) })}</span>
              </div>
            ) },
            { key: 'resource', header: t('auditLog.resource'), priority: 'medium', render: (row) => (
              <div className="table-primary-cell">
                <strong>{resourceLabel(row, t)}</strong>
                {row.resource_id ? <code title={row.resource_id}>{shortID(row.resource_id)}</code> : <span className="muted">{t('auditLog.noSpecificResource')}</span>}
              </div>
            ) },
            { key: 'actions', header: t('common.actions'), priority: 'high', render: (row) => (
              <Button onClick={() => setSelected(row)}>{t('common.open')}</Button>
            ) },
          ]}
        />
      </QueryBoundary>

      <Drawer
        open={Boolean(selected)}
        onClose={() => setSelected(null)}
        title={selected ? text(selected.summary, readableIdentifier(selected.action)) : t('auditLog.eventDetails')}
      >
        {selected ? (
          <div className="page-stack">
            <div className="definition-grid">
              <span>{t('auditLog.time')}</span><strong>{fmt.date(selected.created_at)}</strong>
              <span>{t('auditLog.actor')}</span><strong>{actorLabel(selected, t)}</strong>
              <span>{t('common.email')}</span><strong>{selected.actor_email || t('common.notAvailable')}</strong>
              <span>{t('auditLog.action')}</span><code>{selected.action}</code>
              <span>{t('auditLog.resource')}</span><strong>{resourceLabel(selected, t)}</strong>
              <span>{t('auditLog.resourceID')}</span><code>{selected.resource_id || t('auditLog.noSpecificResource')}</code>
              <span>{t('auditLog.actorID')}</span><code>{selected.actor_user_id || t('common.notAvailable')}</code>
              <span>{t('auditLog.eventID')}</span><code>{selected.id}</code>
            </div>
            <div className="inline-panel">
              <strong>{t('auditLog.description')}</strong>
              <span>{selected.summary}</span>
            </div>
          </div>
        ) : null}
      </Drawer>
    </PageScaffold>
  );
}
