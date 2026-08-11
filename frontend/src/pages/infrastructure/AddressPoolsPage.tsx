import { Pencil, Plus, Route, RouteOff, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { AddressPoolAllocation, AddressPoolSpace, AddressPoolSpaceInput } from '../../shared/api/types';
import { useAuth } from '../../shared/auth/AuthProvider';
import { hasPermission } from '../../shared/permissions/permissions';
import {
  useAddressPools,
  useCreateAddressPoolSpace,
  useDeleteAddressPoolSpace,
  useSetAddressPoolRouting,
  useUpdateAddressPoolSpace,
} from '../../shared/query/hooks';
import {
  Button,
  Checkbox,
  ConfirmDialog,
  DataTable,
  ErrorState,
  FormField,
  FormGrid,
  MetricCard,
  Modal,
  RefreshButton,
  Select,
  StatusBadge,
  Textarea,
  TextField,
  Toolbar,
} from '../../shared/ui';
import { PageScaffold, QueryBoundary } from '../common';

type PoolDraft = {
  key: string;
  label: string;
  description: string;
  baseCIDR: string;
  startCIDR: string;
  allocationPrefix: string;
  serviceScope: 'remote_access' | 'generic' | 'imported';
  routingEnabled: boolean;
  status: 'active' | 'disabled';
  displayOrder: string;
};

const emptyDraft: PoolDraft = {
  key: '',
  label: '',
  description: '',
  baseCIDR: '172.20.0.0/16',
  startCIDR: '172.20.0.0/24',
  allocationPrefix: '24',
  serviceScope: 'remote_access',
  routingEnabled: false,
  status: 'active',
  displayOrder: '100',
};

function draftFromPool(pool: AddressPoolSpace): PoolDraft {
  return {
    key: pool.key,
    label: pool.label,
    description: pool.description || '',
    baseCIDR: pool.base_cidr,
    startCIDR: pool.start_cidr,
    allocationPrefix: String(pool.allocation_prefix),
    serviceScope: pool.service_scope === 'generic' || pool.service_scope === 'imported' ? pool.service_scope : 'remote_access',
    routingEnabled: pool.routing_enabled,
    status: pool.status === 'disabled' ? 'disabled' : 'active',
    displayOrder: String(pool.display_order || 100),
  };
}

function inputFromDraft(draft: PoolDraft): AddressPoolSpaceInput {
  return {
    key: draft.key.trim() || undefined,
    label: draft.label.trim(),
    description: draft.description.trim(),
    family: 'ipv4',
    base_cidr: draft.baseCIDR.trim(),
    start_cidr: draft.startCIDR.trim(),
    allocation_prefix: Number(draft.allocationPrefix),
    service_scope: draft.serviceScope,
    routing_enabled: draft.routingEnabled,
    status: draft.status,
    display_order: Number(draft.displayOrder) || 100,
  };
}

function errorText(error: unknown): string {
  if (error instanceof APIError) return error.message;
  return error instanceof Error ? error.message : String(error || '');
}

export function AddressPoolsPage() {
  const { t } = useTranslation();
  const auth = useAuth();
  const canManage = hasPermission(auth.permissions, auth.roles, 'settings.manage');
  const pools = useAddressPools();
  const create = useCreateAddressPoolSpace();
  const update = useUpdateAddressPoolSpace();
  const remove = useDeleteAddressPoolSpace();
  const routing = useSetAddressPoolRouting();
  const [editor, setEditor] = useState<AddressPoolSpace | 'create' | null>(null);
  const [deleting, setDeleting] = useState<AddressPoolSpace | null>(null);
  const [actionError, setActionError] = useState('');

  const spaces = pools.data?.spaces || [];
  const allocations = pools.data?.allocations || [];
  const totals = spaces.reduce((result, pool) => ({
    capacity: result.capacity + pool.capacity,
    used: result.used + pool.used,
    free: result.free + pool.free,
  }), { capacity: 0, used: 0, free: 0 });
  const pending = create.isPending || update.isPending || remove.isPending || routing.isPending;

  const toggleRouting = async (pool: AddressPoolSpace) => {
    setActionError('');
    try {
      await routing.mutateAsync({ poolId: pool.id, routingEnabled: !pool.routing_enabled });
    } catch (error) {
      setActionError(errorText(error));
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    setActionError('');
    try {
      await remove.mutateAsync(deleting.id);
      setDeleting(null);
    } catch (error) {
      setActionError(errorText(error));
    }
  };

  const spaceColumns = [
    {
      key: 'name',
      header: t('addressPools.pool'),
      priority: 'high' as const,
      render: (row: AddressPoolSpace) => (
        <div className="table-primary-cell">
          <strong>{row.label}</strong>
          <code>{row.key}</code>
          {row.description ? <span className="muted">{row.description}</span> : null}
        </div>
      ),
    },
    {
      key: 'network',
      header: t('addressPools.addressSpace'),
      priority: 'high' as const,
      render: (row: AddressPoolSpace) => (
        <div className="table-primary-cell">
          <code>{row.base_cidr}</code>
          <span className="muted">{t('addressPools.startsAt')}: {row.start_cidr}</span>
          <span className="muted">{t('addressPools.allocatesPrefix', { prefix: row.allocation_prefix })}</span>
        </div>
      ),
    },
    {
      key: 'usage',
      header: t('addressPools.usage'),
      priority: 'high' as const,
      render: (row: AddressPoolSpace) => (
        <div className="table-primary-cell">
          <strong>{row.used} / {row.capacity}</strong>
          <span className="muted">{t('addressPools.freeCount', { count: row.free })}</span>
        </div>
      ),
    },
    {
      key: 'scope',
      header: t('common.scope'),
      priority: 'medium' as const,
      render: (row: AddressPoolSpace) => t(`addressPools.scope.${row.service_scope}`, { defaultValue: row.service_scope }),
    },
    {
      key: 'state',
      header: t('common.status'),
      priority: 'medium' as const,
      render: (row: AddressPoolSpace) => (
        <div className="table-primary-cell">
          <StatusBadge status={row.status} />
          <StatusBadge
            status={row.routing_enabled ? 'enabled' : 'disabled'}
            label={row.routing_enabled ? t('addressPools.routeExportOn') : t('addressPools.routeExportOff')}
          />
        </div>
      ),
    },
    {
      key: 'actions',
      header: t('common.actions'),
      priority: 'high' as const,
      render: (row: AddressPoolSpace) => (
        <div className="table-action-grid">
          <Button icon={<Pencil size={16} />} disabled={!canManage || pending} onClick={() => setEditor(row)}>
            {t('common.edit')}
          </Button>
          <Button
            icon={row.routing_enabled ? <RouteOff size={16} /> : <Route size={16} />}
            disabled={!canManage || pending}
            onClick={() => void toggleRouting(row)}
          >
            {row.routing_enabled ? t('addressPools.disableRouteExport') : t('addressPools.enableRouteExport')}
          </Button>
          <Button variant="danger" icon={<Trash2 size={16} />} disabled={!canManage || pending || row.used > 0} onClick={() => setDeleting(row)}>
            {t('common.delete')}
          </Button>
        </div>
      ),
    },
  ];

  const allocationColumns = [
    { key: 'cidr', header: 'CIDR', priority: 'high' as const, render: (row: AddressPoolAllocation) => <code>{row.cidr}</code> },
    {
      key: 'pool',
      header: t('addressPools.pool'),
      priority: 'medium' as const,
      render: (row: AddressPoolAllocation) => <div className="table-primary-cell"><strong>{row.pool_space_label}</strong><code>{row.pool_space_key}</code></div>,
    },
    {
      key: 'target',
      header: t('addressPools.assignedTo'),
      priority: 'high' as const,
      render: (row: AddressPoolAllocation) => (
        <div className="table-primary-cell">
          <strong>{row.instance_name || t('common.notAvailable')}</strong>
          <span className="muted">{row.node_name || t('common.notAvailable')}</span>
        </div>
      ),
    },
    {
      key: 'service',
      header: t('addressPools.service'),
      priority: 'medium' as const,
      render: (row: AddressPoolAllocation) => <div className="table-primary-cell"><strong>{row.service_code}</strong><span className="muted">{row.purpose}</span></div>,
    },
    { key: 'routing', header: t('addressPools.routeExport'), priority: 'low' as const, render: (row: AddressPoolAllocation) => row.route_export ? t('common.enabled') : t('common.disabled') },
    { key: 'status', header: t('common.status'), priority: 'high' as const, render: (row: AddressPoolAllocation) => <StatusBadge status={row.status} /> },
  ];

  return (
    <PageScaffold
      title={t('nav.addressPools')}
      subtitle={t('addressPools.subtitle')}
      actions={(
        <Toolbar>
          <RefreshButton onRefresh={() => pools.refetch()}>{t('common.refresh')}</RefreshButton>
          <Button variant="primary" icon={<Plus size={16} />} disabled={!canManage} onClick={() => setEditor('create')}>
            {t('addressPools.newPool')}
          </Button>
        </Toolbar>
      )}
    >
      <QueryBoundary isLoading={pools.isLoading} isError={pools.isError} error={pools.error} refetch={() => void pools.refetch()}>
        <div className="metric-grid">
          <MetricCard label={t('addressPools.spaces')} value={spaces.length} caption={t('addressPools.activeSpaces', { count: spaces.filter((pool) => pool.status === 'active').length })} />
          <MetricCard label={t('addressPools.capacity')} value={totals.capacity} caption={t('addressPools.subnets')} />
          <MetricCard label={t('addressPools.allocated')} value={totals.used} caption={t('addressPools.activeAllocations')} />
          <MetricCard label={t('addressPools.available')} value={totals.free} caption={t('addressPools.subnets')} />
        </div>

        {actionError ? <ErrorState body={actionError} /> : null}
        {!canManage ? <div className="notice-panel">{t('common.permissionRequired', { permission: 'settings.manage' })}</div> : null}

        <DataTable
          title={t('addressPools.spaces')}
          rows={spaces}
          columns={spaceColumns}
          responsive="wide"
        />
        <DataTable
          title={t('addressPools.allocations')}
          tools={<span className="muted">{t('addressPools.allocationsHint')}</span>}
          rows={allocations}
          columns={allocationColumns}
          responsive="wide"
        />
      </QueryBoundary>

      <PoolEditor
        key={editor === 'create' ? 'create' : editor?.id || 'closed'}
        pool={editor === 'create' ? undefined : editor || undefined}
        open={editor !== null}
        pending={create.isPending || update.isPending}
        onClose={() => setEditor(null)}
        onSave={async (input) => {
          if (editor && editor !== 'create') await update.mutateAsync({ poolId: editor.id, input });
          else await create.mutateAsync(input);
          setEditor(null);
        }}
      />

      <ConfirmDialog title={t('addressPools.deleteTitle')} open={Boolean(deleting)} onClose={() => setDeleting(null)}>
        <div className="page-stack">
          <p>{t('addressPools.deleteConfirm', { name: deleting?.label || '' })}</p>
          <p className="muted">{t('addressPools.deleteHint')}</p>
          {remove.error ? <ErrorState body={errorText(remove.error)} /> : null}
          <div className="modal-actions">
            <Button disabled={remove.isPending} onClick={() => setDeleting(null)}>{t('common.cancel')}</Button>
            <Button variant="danger" disabled={remove.isPending} onClick={() => void confirmDelete()}>
              {remove.isPending ? t('common.loading') : t('common.delete')}
            </Button>
          </div>
        </div>
      </ConfirmDialog>
    </PageScaffold>
  );
}

function PoolEditor({ pool, open, pending, onClose, onSave }: {
  pool?: AddressPoolSpace;
  open: boolean;
  pending: boolean;
  onClose: () => void;
  onSave: (input: AddressPoolSpaceInput) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<PoolDraft>(() => pool ? draftFromPool(pool) : emptyDraft);
  const [localError, setLocalError] = useState('');
  const structuralFieldsLocked = Boolean(pool && pool.used > 0);

  const set = <K extends keyof PoolDraft>(key: K, value: PoolDraft[K]) => setDraft((current) => ({ ...current, [key]: value }));
  const valid = Boolean(
    draft.label.trim()
    && draft.baseCIDR.trim()
    && draft.startCIDR.trim()
    && Number(draft.allocationPrefix) >= 1
    && Number(draft.allocationPrefix) <= 32,
  );

  const submit = async () => {
    if (!valid) return;
    setLocalError('');
    try {
      await onSave(inputFromDraft(draft));
    } catch (requestError) {
      setLocalError(errorText(requestError));
    }
  };

  return (
    <Modal size="wide" title={pool ? t('addressPools.editPool') : t('addressPools.newPool')} open={open} onClose={onClose}>
      <div className="page-stack">
        <FormGrid>
          <FormField label={t('common.name')}>
            <TextField autoFocus required value={draft.label} onChange={(event) => set('label', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('addressPools.internalKey')}>
            <TextField disabled={Boolean(pool)} placeholder={t('addressPools.keyGenerated')} value={draft.key} onChange={(event) => set('key', event.currentTarget.value)} />
          </FormField>
          <FormField full label={t('common.description')}>
            <Textarea rows={2} value={draft.description} onChange={(event) => set('description', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('addressPools.baseCIDR')}>
            <TextField disabled={structuralFieldsLocked} required placeholder="172.20.0.0/16" value={draft.baseCIDR} onChange={(event) => set('baseCIDR', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('addressPools.firstSubnet')}>
            <TextField disabled={structuralFieldsLocked} required placeholder="172.20.0.0/24" value={draft.startCIDR} onChange={(event) => set('startCIDR', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('addressPools.subnetPrefix')}>
            <TextField disabled={structuralFieldsLocked} required type="number" min={1} max={32} value={draft.allocationPrefix} onChange={(event) => set('allocationPrefix', event.currentTarget.value)} />
          </FormField>
          <FormField label={t('common.scope')}>
            <Select disabled={structuralFieldsLocked} value={draft.serviceScope} onChange={(event) => set('serviceScope', event.currentTarget.value as PoolDraft['serviceScope'])}>
              <option value="remote_access">{t('addressPools.scope.remote_access')}</option>
              <option value="generic">{t('addressPools.scope.generic')}</option>
              {pool?.service_scope === 'imported' ? <option value="imported">{t('addressPools.scope.imported')}</option> : null}
            </Select>
          </FormField>
          <FormField label={t('common.status')}>
            <Select value={draft.status} onChange={(event) => set('status', event.currentTarget.value as PoolDraft['status'])}>
              <option value="active">{t('common.active')}</option>
              <option value="disabled">{t('common.disabled')}</option>
            </Select>
          </FormField>
          <FormField label={t('addressPools.displayOrder')}>
            <TextField type="number" min={1} value={draft.displayOrder} onChange={(event) => set('displayOrder', event.currentTarget.value)} />
          </FormField>
          <div className="form-field form-field-full">
            <span className="form-label">{t('addressPools.routeExport')}</span>
            <Checkbox checked={draft.routingEnabled} label={t('addressPools.routeExportHelp')} onChange={(event) => set('routingEnabled', event.currentTarget.checked)} />
          </div>
        </FormGrid>
        {pool?.used ? <div className="notice-panel">{t('addressPools.structuralEditLocked', { count: pool.used })}</div> : null}
        {localError ? <ErrorState body={localError} /> : null}
        <div className="modal-actions">
          <Button disabled={pending} onClick={onClose}>{t('common.cancel')}</Button>
          <Button variant="primary" disabled={pending || !valid} onClick={() => void submit()}>
            {pending ? t('common.loading') : t('common.save')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
