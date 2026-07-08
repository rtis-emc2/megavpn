import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  ClientAccessGroup,
  ClientAccessGroupInput,
  ClientAccessGroupMember,
  ClientAccessGroupMembershipRequest,
  ClientAccessGroupMembershipResult,
  ClientAccessGroupScope,
  ClientAccessService,
  VLESSAccessGroupPolicy,
} from '../../shared/api/types';
import {
  useApplyClientAccessGroupMembers,
  useApplyClientAccessGroupSync,
  useAvailableClientsForGroup,
  useClientAccessGroupMembers,
  useClientAccessGroupScope,
  useClientAccessGroupSyncState,
  useClientAccessGroups,
  useClientAccessServices,
  useCreateClientAccessGroup,
  useInstances,
  usePreviewClientAccessGroupMembers,
  usePreviewClientAccessGroupSync,
  useRemoveClientAccessGroupMember,
  useUpdateClientAccessGroup,
  useUpdateClientAccessGroupScope,
} from '../../shared/query/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  Checkbox,
  ConfirmDialog,
  DataTable,
  Drawer,
  ErrorState,
  FormField,
  FormGrid,
  Select,
  StatusBadge,
  Textarea,
  TextField,
} from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type GroupForm = {
  serviceCode: string;
  groupKey: string;
  displayName: string;
  description: string;
  status: string;
  routeMode: string;
  egressNodeID: string;
  targetInstanceID: string;
  outboundTag: string;
  adBlock: boolean;
  scopeMode: string;
  autoApplyNewInstances: boolean;
};

type MembersFilters = {
  search: string;
  status: string;
  assignment: string;
  pageSize: number;
  offset: number;
};

const defaultForm: GroupForm = {
  serviceCode: 'vless',
  groupKey: '',
  displayName: '',
  description: '',
  status: 'active',
  routeMode: 'instance_default',
  egressNodeID: '',
  targetInstanceID: '',
  outboundTag: 'direct',
  adBlock: false,
  scopeMode: 'all_active_instances',
  autoApplyNewInstances: true,
};

function getPolicy(group?: ClientAccessGroup | null): VLESSAccessGroupPolicy {
  const policy = group?.policy_json;
  return policy && typeof policy === 'object' ? policy as VLESSAccessGroupPolicy : {};
}

function formFromGroup(group?: ClientAccessGroup | null): GroupForm {
  if (!group) return defaultForm;
  const policy = getPolicy(group);
  return {
    serviceCode: group.service_code || 'vless',
    groupKey: group.group_key || '',
    displayName: group.display_name || '',
    description: group.description || '',
    status: group.status || 'active',
    routeMode: policy.access_mode || 'instance_default',
    egressNodeID: policy.egress_node_id || '',
    targetInstanceID: policy.target_instance_id || '',
    outboundTag: policy.outbound_tag || 'direct',
    adBlock: Boolean(policy.ad_block),
    scopeMode: group.scope_mode || 'all_active_instances',
    autoApplyNewInstances: group.auto_apply_new_instances ?? true,
  };
}

function formToInput(form: GroupForm): ClientAccessGroupInput {
  const policy: VLESSAccessGroupPolicy = {
    access_mode: form.routeMode,
    egress_mode: form.routeMode === 'egress_node' ? 'egress_node' : 'default',
    egress_node_id: form.routeMode === 'egress_node' ? form.egressNodeID.trim() : '',
    target_instance_id: form.routeMode === 'instance_only' ? form.targetInstanceID.trim() : '',
    outbound_tag: form.outboundTag.trim() || 'direct',
    ad_block: form.adBlock,
    rules: [],
    extra_rules: [],
  };
  return {
    service_code: form.serviceCode,
    group_key: form.groupKey.trim(),
    display_name: form.displayName.trim(),
    description: form.description.trim(),
    status: form.status,
    policy_json: policy,
    scope_mode: form.scopeMode,
    auto_apply_new_instances: form.autoApplyNewInstances,
  };
}

function refsFromText(value: string): string[] {
  return value
    .split(/[\n,;]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function policySummary(group: ClientAccessGroup) {
  const policy = getPolicy(group);
  const parts = [policy.access_mode || 'instance_default'];
  if (policy.egress_node_id) parts.push(`egress:${policy.egress_node_id}`);
  if (policy.target_instance_id) parts.push(`target:${policy.target_instance_id}`);
  if (policy.ad_block) parts.push('ad-block');
  return parts.join(' · ');
}

function scopeSummary(group: ClientAccessGroup) {
  return `${group.scope_mode || 'all_active_instances'} · ${group.affected_instances ?? 0}`;
}

function syncSummary(group: ClientAccessGroup) {
  const pending = group.pending_sync_count || 0;
  const failed = group.failed_sync_count || 0;
  const applied = group.applied_sync_count || 0;
  if (failed > 0) return `failed ${failed}`;
  if (pending > 0) return `pending ${pending}`;
  if (applied > 0) return `applied ${applied}`;
  return 'not synced';
}

function serviceCapabilityLabel(service: ClientAccessService) {
  if (service.supports_groups && service.supports_membership && service.supports_materialization) return 'active';
  return service.status || 'catalog_only';
}

function isVLESSServiceReady(service?: ClientAccessService) {
  return Boolean(service?.service_code === 'vless' && service.supports_groups && service.supports_membership && service.supports_materialization);
}

function ResultSummary({ result }: { result?: ClientAccessGroupMembershipResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <div className="toolbar">
            <Badge>{result.dry_run ? t('clients.groups.previewResult') : t('clients.groups.applyResult')}</Badge>
            {result.all_filtered ? <Badge>{t('clients.groups.allFiltered')}</Badge> : null}
          </div>
          <div className="metric-grid">
            <div><strong>{result.created_memberships}</strong><br />{t('clients.groups.created')}</div>
            <div><strong>{result.moved_memberships}</strong><br />{t('clients.groups.moved')}</div>
            <div><strong>{result.skipped_existing}</strong><br />{t('clients.groups.skipped')}</div>
            <div><strong>{result.failed?.length || 0}</strong><br />{t('clients.groups.failed')}</div>
          </div>
          <p>{t('clients.groups.materializedSummary', {
            created: result.materialized_created || 0,
            updated: result.materialized_updated || 0,
            disabled: result.materialized_disabled || 0,
            instances: result.affected_instances || 0,
          })}</p>
          {result.conflicts?.length ? (
            <ul>
              {result.conflicts.map((conflict, index) => (
                <li key={index}>{text(conflict.client_id)}: {text(conflict.reason)} {conflict.existing_group ? `(${conflict.existing_group} -> ${conflict.target_group})` : ''}</li>
              ))}
            </ul>
          ) : null}
          {result.failed?.length ? (
            <ul>
              {result.failed.map((failure, index) => (
                <li key={index}>{text(failure.ref || failure.client_id)}: {text(failure.error)}</li>
              ))}
            </ul>
          ) : null}
          {result.warnings?.length ? (
            <ul>
              {result.warnings.map((warning, index) => <li key={index}>{text(warning)}</li>)}
            </ul>
          ) : null}
          {result.apply_job_ids?.length ? (
            <div className="toolbar">
              <Badge>{t('jobs.title')}</Badge>
              {result.apply_job_ids.map((jobID) => <a className="button button-secondary" href="/operations/jobs" key={jobID}>{jobID}</a>)}
            </div>
          ) : null}
        </div>
      </CardBody>
    </Card>
  );
}

export function ClientGroupsPage() {
  const { t } = useTranslation();
  const services = useClientAccessServices();
  const [serviceFilter, setServiceFilter] = useState('vless');
  const groups = useClientAccessGroups(serviceFilter === 'all' ? undefined : serviceFilter);
  const [formGroup, setFormGroup] = useState<ClientAccessGroup | null | undefined>(undefined);
  const [membersGroup, setMembersGroup] = useState<ClientAccessGroup | null>(null);
  const [scopeGroup, setScopeGroup] = useState<ClientAccessGroup | null>(null);
  const [syncGroup, setSyncGroup] = useState<ClientAccessGroup | null>(null);
  const [success, setSuccess] = useState('');

  const vlessService = useMemo(() => (services.data || []).find((service) => service.service_code === 'vless'), [services.data]);

  return (
    <PageScaffold
      title={t('clients.groupsTitle')}
      subtitle={t('clients.groupsSubtitle')}
      actions={<Button disabled={!isVLESSServiceReady(vlessService)} title={isVLESSServiceReady(vlessService) ? undefined : t('common.unsupportedAction')} onClick={() => setFormGroup(null)}>{t('clients.groups.createVless')}</Button>}
    >
      {success ? <div className="notice"><strong>{success}</strong></div> : null}
      <QueryBoundary isLoading={services.isLoading} isError={services.isError} error={services.error} refetch={() => void services.refetch()}>
        <DataTable
          title={t('clients.groups.serviceCatalog')}
          rows={services.data || []}
          columns={[
            { key: 'service', header: t('instances.service'), render: (service) => <strong>{service.display_name || service.service_code}</strong> },
            { key: 'status', header: t('common.status'), render: (service) => <StatusBadge status={serviceCapabilityLabel(service)} /> },
            { key: 'capabilities', header: t('common.scope'), render: (service) => (
              <div className="toolbar">
                <Badge>{service.supports_groups ? t('clients.groups.groupsSupported') : t('common.catalogOnly')}</Badge>
                <Badge>{service.supports_membership ? t('clients.groups.membersSupported') : t('common.disabled')}</Badge>
                <Badge>{service.supports_materialization ? t('clients.groups.materializationSupported') : t('common.planned')}</Badge>
              </div>
            ) },
            { key: 'description', header: t('common.description'), render: (service) => text(service.description) },
          ]}
        />
      </QueryBoundary>

      <div className="toolbar">
        <FormField label={t('clients.serviceFilter')}>
          <Select value={serviceFilter} onChange={(event) => setServiceFilter(event.currentTarget.value)}>
            <option value="all">{t('common.all')}</option>
            {(services.data || []).map((service) => (
              <option value={service.service_code} key={service.service_code}>
                {service.display_name || service.service_code} · {service.status || 'unknown'}
              </option>
            ))}
          </Select>
        </FormField>
        <Badge>{t('clients.previewRequired')}</Badge>
        <Badge>{t('clients.bulkBounded')}</Badge>
      </div>

      <QueryBoundary isLoading={groups.isLoading} isError={groups.isError} error={groups.error} refetch={() => void groups.refetch()}>
        <DataTable
          title={t('clients.groupsTitle')}
          rows={groups.data || []}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.display_name || row.group_key || row.id)}</strong> },
            { key: 'key', header: t('clients.groups.groupKey'), render: (row) => <code>{text(row.group_key)}</code> },
            { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'members', header: t('clients.members'), render: (row) => `${row.active_member_count ?? 0}/${row.member_count ?? 0}` },
            { key: 'scope', header: t('common.scope'), render: (row) => text(scopeSummary(row)) },
            { key: 'policy', header: t('clients.groups.policy'), render: (row) => text(policySummary(row)) },
            { key: 'sync', header: t('common.sync'), render: (row) => <StatusBadge status={syncSummary(row)} /> },
            { key: 'actions', header: t('common.actions'), render: (row) => (
              <div className="toolbar">
                <Button onClick={() => setMembersGroup(row)}>{t('clients.groups.membersAction')}</Button>
                <Button onClick={() => setFormGroup(row)}>{t('clients.groups.policyAction')}</Button>
                <Button onClick={() => setScopeGroup(row)}>{t('clients.groups.scopeAction')}</Button>
                <Button onClick={() => setSyncGroup(row)}>{t('clients.groups.syncAction')}</Button>
              </div>
            ) },
          ]}
        />
      </QueryBoundary>

      <GroupFormDrawer
        group={formGroup}
        services={services.data || []}
        onClose={() => setFormGroup(undefined)}
        onSuccess={(message) => setSuccess(message)}
      />
      <MembersDrawer group={membersGroup} onClose={() => setMembersGroup(null)} onSuccess={(message) => setSuccess(message)} />
      <ScopeDrawer group={scopeGroup} onClose={() => setScopeGroup(null)} onSuccess={(message) => setSuccess(message)} />
      <SyncDrawer group={syncGroup} onClose={() => setSyncGroup(null)} onSuccess={(message) => setSuccess(message)} />
    </PageScaffold>
  );
}

function GroupFormDrawer({ group, services, onClose, onSuccess }: {
  group: ClientAccessGroup | null | undefined;
  services: ClientAccessService[];
  onClose: () => void;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  const open = group !== undefined;
  const isEdit = Boolean(group);

  return (
    <Drawer open={open} onClose={onClose} title={isEdit ? t('clients.groups.editGroup') : t('clients.groups.createVless')}>
      {open ? (
        <GroupFormDrawerContent
          key={group?.id || 'new'}
          group={group || null}
          services={services}
          onClose={onClose}
          onSuccess={onSuccess}
        />
      ) : null}
    </Drawer>
  );
}

function GroupFormDrawerContent({ group, services, onClose, onSuccess }: {
  group: ClientAccessGroup | null;
  services: ClientAccessService[];
  onClose: () => void;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  const createGroup = useCreateClientAccessGroup();
  const updateGroup = useUpdateClientAccessGroup();
  const [form, setForm] = useState<GroupForm>(() => formFromGroup(group));
  const isEdit = Boolean(group);

  const set = <K extends keyof GroupForm>(key: K, value: GroupForm[K]) => setForm((current) => ({ ...current, [key]: value }));

  const submit = async () => {
    const input = formToInput(form);
    try {
      if (isEdit && group) {
        await updateGroup.mutateAsync({ groupId: group.id, input });
        onSuccess(t('clients.groups.groupUpdated'));
      } else {
        await createGroup.mutateAsync(input);
        onSuccess(t('clients.groups.groupCreated'));
      }
      onClose();
    } catch {
      // Mutation state renders the backend error.
    }
  };

  const mutation = isEdit ? updateGroup : createGroup;
  return (
    <div className="page-stack">
      <FormGrid>
        <FormField label={t('instances.service')}>
          <Select value={form.serviceCode} onChange={(event) => set('serviceCode', event.currentTarget.value)} disabled={isEdit}>
            {services.map((service) => (
              <option key={service.service_code} value={service.service_code} disabled={!isVLESSServiceReady(service)}>
                {service.display_name || service.service_code} · {serviceCapabilityLabel(service)}
              </option>
            ))}
          </Select>
        </FormField>
        <FormField label={t('clients.groups.groupKey')}>
          <TextField value={form.groupKey} onChange={(event) => set('groupKey', event.currentTarget.value)} disabled={isEdit} required />
        </FormField>
        <FormField label={t('common.name')}>
          <TextField value={form.displayName} onChange={(event) => set('displayName', event.currentTarget.value)} required />
        </FormField>
        <FormField label={t('common.status')}>
          <Select value={form.status} onChange={(event) => set('status', event.currentTarget.value)}>
            <option value="active">{t('common.enabled')}</option>
            <option value="disabled">{t('common.disabled')}</option>
          </Select>
        </FormField>
        <FormField label={t('clients.groups.routeMode')}>
          <Select value={form.routeMode} onChange={(event) => set('routeMode', event.currentTarget.value)}>
            <option value="instance_default">{t('clients.groups.route.instanceDefault')}</option>
            <option value="local_breakout">{t('clients.groups.route.localBreakout')}</option>
            <option value="egress_node">{t('clients.groups.route.egressNode')}</option>
            <option value="instance_only">{t('clients.groups.route.instanceOnly')}</option>
            <option value="block">{t('clients.groups.route.block')}</option>
          </Select>
        </FormField>
        <FormField label={t('clients.groups.outboundTag')}>
          <TextField value={form.outboundTag} onChange={(event) => set('outboundTag', event.currentTarget.value)} />
        </FormField>
        {form.routeMode === 'egress_node' ? (
          <FormField label={t('clients.groups.egressNodeId')}>
            <TextField value={form.egressNodeID} onChange={(event) => set('egressNodeID', event.currentTarget.value)} required />
          </FormField>
        ) : null}
        {form.routeMode === 'instance_only' ? (
          <FormField label={t('clients.groups.targetInstanceId')}>
            <TextField value={form.targetInstanceID} onChange={(event) => set('targetInstanceID', event.currentTarget.value)} required />
          </FormField>
        ) : null}
        <FormField label={t('common.scope')}>
          <Select value={form.scopeMode} onChange={(event) => set('scopeMode', event.currentTarget.value)}>
            <option value="all_active_instances">{t('clients.groups.scopeAll')}</option>
            <option value="selected_instances">{t('clients.groups.scopeSelected')}</option>
            <option value="all_except_selected">{t('clients.groups.scopeExcept')}</option>
          </Select>
        </FormField>
        <FormField label={t('clients.groups.options')}>
          <div className="page-stack">
            <Checkbox checked={form.adBlock} onChange={(event) => set('adBlock', event.currentTarget.checked)} label={t('clients.groups.adBlock')} />
            <Checkbox checked={form.autoApplyNewInstances} onChange={(event) => set('autoApplyNewInstances', event.currentTarget.checked)} label={t('clients.groups.autoApplyNewInstances')} />
          </div>
        </FormField>
        <FormField full label={t('common.description')}>
          <Textarea value={form.description} onChange={(event) => set('description', event.currentTarget.value)} rows={3} />
        </FormField>
      </FormGrid>
      {mutation.isError ? <ErrorState body={mutation.error.message} /> : null}
      <div className="toolbar">
        <Button onClick={onClose}>{t('common.close')}</Button>
        <Button variant="primary" disabled={mutation.isPending || !form.groupKey.trim() || !form.displayName.trim()} onClick={() => void submit()}>
          {mutation.isPending ? t('common.loading') : t('common.save')}
        </Button>
      </div>
    </div>
  );
}

function MembersDrawer({ group, onClose, onSuccess }: {
  group: ClientAccessGroup | null;
  onClose: () => void;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Drawer open={Boolean(group)} onClose={onClose} title={group ? `${t('clients.groups.membersAction')}: ${group.display_name || group.group_key}` : t('clients.groups.membersAction')}>
      {group ? <MembersDrawerContent key={group.id} group={group} onSuccess={onSuccess} /> : null}
    </Drawer>
  );
}

function MembersDrawerContent({ group, onSuccess }: {
  group: ClientAccessGroup;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  const [filters, setFilters] = useState<MembersFilters>({ search: '', status: '', assignment: 'unassigned', pageSize: 50, offset: 0 });
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set());
  const [allFiltered, setAllFiltered] = useState(false);
  const [refsText, setRefsText] = useState('');
  const [mode, setMode] = useState<'add_only' | 'add_or_move'>('add_only');
  const [previewResult, setPreviewResult] = useState<ClientAccessGroupMembershipResult | null>(null);
  const [previewStale, setPreviewStale] = useState(false);
  const [applyResult, setApplyResult] = useState<ClientAccessGroupMembershipResult | null>(null);
  const [removeTarget, setRemoveTarget] = useState<ClientAccessGroupMember | null>(null);
  const available = useAvailableClientsForGroup(group?.id, {
    service_code: group?.service_code || 'vless',
    assignment: filters.assignment,
    search: filters.search,
    status: filters.status,
    limit: filters.pageSize,
    offset: filters.offset,
  });
  const members = useClientAccessGroupMembers(group?.id, { limit: 25, offset: 0 });
  const preview = usePreviewClientAccessGroupMembers();
  const apply = useApplyClientAccessGroupMembers();
  const remove = useRemoveClientAccessGroupMember();

  const invalidatePreview = () => {
    if (previewResult) setPreviewStale(true);
    setApplyResult(null);
  };
  const updateFilters = (patch: Partial<MembersFilters>) => {
    setFilters((current) => ({ ...current, ...patch }));
    invalidatePreview();
  };
  const toggleSelected = (clientID: string, checked: boolean) => {
    setSelectedIDs((current) => {
      const next = new Set(current);
      if (checked) next.add(clientID);
      else next.delete(clientID);
      return next;
    });
    invalidatePreview();
  };
  const refs = refsFromText(refsText);
  const selectedCount = allFiltered ? (available.data?.total || 0) + refs.length : selectedIDs.size + refs.length;
  const canPreview = selectedIDs.size > 0 || refs.length > 0 || allFiltered;
  const payload = (): ClientAccessGroupMembershipRequest => ({
    client_ids: Array.from(selectedIDs),
    client_refs: refs,
    mode,
    queue_apply: true,
    all_filtered: allFiltered,
    filter_search: filters.search,
    filter_assignment: filters.assignment,
    filter_status: filters.status,
    filter_group_id: group?.id,
  });
  const runPreview = async () => {
    try {
      const result = await preview.mutateAsync({ groupId: group.id, payload: { ...payload(), dry_run: true } });
      setPreviewResult(result);
      setPreviewStale(false);
      setApplyResult(null);
    } catch {
      // ErrorState renders the backend error.
    }
  };
  const runApply = async () => {
    if (!previewResult || previewStale) return;
    try {
      const result = await apply.mutateAsync({ groupId: group.id, payload: { ...payload(), dry_run: false } });
      setApplyResult(result);
      onSuccess(t('clients.groups.membersApplied'));
      void members.refetch();
      void available.refetch();
    } catch {
      // ErrorState renders the backend error.
    }
  };
  const runRemove = async () => {
    if (!removeTarget) return;
    try {
      await remove.mutateAsync({ groupId: group.id, clientId: removeTarget.client_id });
      setRemoveTarget(null);
      onSuccess(t('clients.groups.memberRemoved'));
      void members.refetch();
      void available.refetch();
    } catch {
      // ErrorState renders the backend error.
    }
  };

  return (
    <div className="page-stack">
      <FormGrid>
        <FormField label={t('common.search')}>
          <TextField value={filters.search} onChange={(event) => updateFilters({ search: event.currentTarget.value, offset: 0 })} />
        </FormField>
        <FormField label={t('common.status')}>
          <Select value={filters.status} onChange={(event) => updateFilters({ status: event.currentTarget.value, offset: 0 })}>
            <option value="">{t('common.all')}</option>
            <option value="active">{t('common.enabled')}</option>
            <option value="disabled">{t('common.disabled')}</option>
            <option value="suspended">suspended</option>
          </Select>
        </FormField>
        <FormField label={t('clients.groups.assignment')}>
          <Select value={filters.assignment} onChange={(event) => updateFilters({ assignment: event.currentTarget.value, offset: 0 })}>
            <option value="unassigned">{t('clients.groups.assignmentUnassigned')}</option>
            <option value="assigned_to_group">{t('clients.groups.assignmentThis')}</option>
            <option value="assigned_other">{t('clients.groups.assignmentOther')}</option>
            <option value="all">{t('clients.groups.assignmentAll')}</option>
          </Select>
        </FormField>
        <FormField label={t('clients.groups.pageSize')}>
          <Select value={filters.pageSize} onChange={(event) => updateFilters({ pageSize: Number(event.currentTarget.value), offset: 0 })}>
            {[25, 50, 100, 250].map((size) => <option key={size} value={size}>{size}</option>)}
          </Select>
        </FormField>
        <FormField label={t('clients.groups.assignmentMode')}>
          <Select value={mode} onChange={(event) => { setMode(event.currentTarget.value as 'add_only' | 'add_or_move'); invalidatePreview(); }}>
            <option value="add_only">{t('clients.groups.addOnly')}</option>
            <option value="add_or_move">{t('clients.groups.addOrMove')}</option>
          </Select>
        </FormField>
        <FormField full label={t('clients.groups.pastedRefs')}>
          <Textarea value={refsText} onChange={(event) => { setRefsText(event.currentTarget.value); invalidatePreview(); }} rows={3} />
        </FormField>
      </FormGrid>
      <div className="toolbar">
        <Badge>{t('clients.groups.selectedCount', { count: selectedCount })}</Badge>
        {previewStale ? <Badge>{t('clients.groups.previewStale')}</Badge> : null}
        <Button onClick={() => {
          setSelectedIDs(new Set((available.data?.items || []).map((item) => item.client_id)));
          setAllFiltered(false);
          invalidatePreview();
        }}>{t('clients.groups.selectVisible')}</Button>
        <Button onClick={() => {
          setSelectedIDs(new Set());
          setAllFiltered(true);
          invalidatePreview();
        }}>{t('clients.groups.selectAllFiltered')}</Button>
        <Button onClick={() => {
          setSelectedIDs(new Set());
          setAllFiltered(false);
          setRefsText('');
          setPreviewResult(null);
          setPreviewStale(false);
        }}>{t('clients.groups.clearSelection')}</Button>
      </div>
      <QueryBoundary isLoading={available.isLoading} isError={available.isError} error={available.error} refetch={() => void available.refetch()}>
        <DataTable
          title={t('clients.groups.availableClients')}
          rows={available.data?.items || []}
          columns={[
            { key: 'select', header: '', render: (row) => (
              <input
                aria-label={t('clients.groups.selectClient', { client: row.username || row.client_id })}
                type="checkbox"
                disabled={allFiltered}
                checked={selectedIDs.has(row.client_id) || allFiltered}
                onChange={(event) => toggleSelected(row.client_id, event.currentTarget.checked)}
              />
            ) },
            { key: 'client', header: t('clients.client'), render: (row) => <strong>{text(row.display_name || row.username || row.client_id)}</strong> },
            { key: 'email', header: t('common.email'), render: (row) => text(row.email) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.client_status} /> },
            { key: 'assignment', header: t('clients.groups.assignment'), render: (row) => text(row.group_key || t('common.none')) },
          ]}
        />
      </QueryBoundary>
      <div className="toolbar">
        <Button disabled={!canPreview || preview.isPending} onClick={() => void runPreview()}>{preview.isPending ? t('common.loading') : t('common.preview')}</Button>
        <Button variant="primary" disabled={!previewResult || previewStale || apply.isPending} onClick={() => void runApply()}>
          {apply.isPending ? t('common.loading') : t('common.apply')}
        </Button>
      </div>
      {preview.isError ? <ErrorState body={preview.error.message} /> : null}
      {apply.isError ? <ErrorState body={apply.error.message} /> : null}
      <ResultSummary result={previewResult} />
      <ResultSummary result={applyResult} />
      <QueryBoundary isLoading={members.isLoading} isError={members.isError} error={members.error} refetch={() => void members.refetch()}>
        <DataTable
          title={t('clients.groups.currentMembers')}
          rows={members.data?.items || []}
          columns={[
            { key: 'client', header: t('clients.client'), render: (row) => <strong>{text(row.display_name || row.username || row.client_id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.membership_status || row.client_status} /> },
            { key: 'identity', header: t('clients.core.identity'), render: (row) => row.xray_uuid ? t('clients.core.redactedIdentity') : <code>{shortID(row.service_access_id)}</code> },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button variant="danger" onClick={() => setRemoveTarget(row)}>{t('clients.groups.removeMember')}</Button> },
          ]}
        />
      </QueryBoundary>
      <ConfirmDialog open={Boolean(removeTarget)} onClose={() => setRemoveTarget(null)} title={t('clients.groups.removeMember')}>
        <div className="page-stack">
          <p>{t('clients.groups.removeConfirm', { client: removeTarget?.username || removeTarget?.client_id || '' })}</p>
          {remove.isError ? <ErrorState body={remove.error.message} /> : null}
          <div className="toolbar">
            <Button onClick={() => setRemoveTarget(null)}>{t('common.close')}</Button>
            <Button variant="danger" disabled={remove.isPending} onClick={() => void runRemove()}>{remove.isPending ? t('common.loading') : t('common.delete')}</Button>
          </div>
        </div>
      </ConfirmDialog>
    </div>
  );
}

function ScopeDrawer({ group, onClose, onSuccess }: {
  group: ClientAccessGroup | null;
  onClose: () => void;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Drawer open={Boolean(group)} onClose={onClose} title={group ? `${t('clients.groups.scopeAction')}: ${group.display_name || group.group_key}` : t('clients.groups.scopeAction')}>
      {group ? <ScopeDrawerContent key={group.id} group={group} onClose={onClose} onSuccess={onSuccess} /> : null}
    </Drawer>
  );
}

type ScopeDraft = {
  scopeMode: string;
  autoApply: boolean;
  selectedInstances: Set<string>;
};

function scopeSelection(scope?: ClientAccessGroupScope): Set<string> {
  return new Set([...(scope?.include_instance_ids || []), ...(scope?.exclude_instance_ids || [])]);
}

function ScopeDrawerContent({ group, onClose, onSuccess }: {
  group: ClientAccessGroup;
  onClose: () => void;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  const instances = useInstances();
  const scope = useClientAccessGroupScope(group?.id);
  const updateScope = useUpdateClientAccessGroupScope();
  const [draft, setDraft] = useState<ScopeDraft | null>(null);
  const [lastResult, setLastResult] = useState<ClientAccessGroupScope | null>(null);

  const backendSelection = useMemo(() => scopeSelection(scope.data), [scope.data]);
  const scopeMode = draft?.scopeMode || scope.data?.scope_mode || 'all_active_instances';
  const autoApply = draft?.autoApply ?? scope.data?.auto_apply_new_instances ?? true;
  const selectedInstances = draft?.selectedInstances || backendSelection;

  const updateDraft = (patch: Partial<ScopeDraft>) => {
    setDraft((current) => ({
      scopeMode,
      autoApply,
      selectedInstances,
      ...current,
      ...patch,
    }));
    setLastResult(null);
  };

  const toggleInstance = (id: string, checked: boolean) => {
    setDraft((current) => {
      const next = new Set(current?.selectedInstances || selectedInstances);
      if (checked) next.add(id);
      else next.delete(id);
      return {
        scopeMode: current?.scopeMode || scopeMode,
        autoApply: current?.autoApply ?? autoApply,
        selectedInstances: next,
      };
    });
    setLastResult(null);
  };
  const save = async () => {
    const ids = Array.from(selectedInstances);
    const payload: ClientAccessGroupScope = {
      group_id: group.id,
      scope_mode: scopeMode,
      auto_apply_new_instances: autoApply,
      include_instance_ids: scopeMode === 'selected_instances' ? ids : [],
      exclude_instance_ids: scopeMode === 'all_except_selected' ? ids : [],
    };
    try {
      const result = await updateScope.mutateAsync({ groupId: group.id, payload });
      setLastResult(result);
      onSuccess(t('clients.groups.scopeSaved'));
    } catch {
      // ErrorState renders the backend error.
    }
  };
  const candidates = (instances.data || []).filter((instance) => instance.service_code === 'xray-core' || instance.service_code === 'xray' || instance.service_code === 'vless');

  return (
    <div className="page-stack">
      <QueryBoundary isLoading={scope.isLoading} isError={scope.isError} error={scope.error} refetch={() => void scope.refetch()}>
        <FormGrid>
          <FormField label={t('common.scope')}>
            <Select value={scopeMode} onChange={(event) => updateDraft({ scopeMode: event.currentTarget.value })}>
              <option value="all_active_instances">{t('clients.groups.scopeAll')}</option>
              <option value="selected_instances">{t('clients.groups.scopeSelected')}</option>
              <option value="all_except_selected">{t('clients.groups.scopeExcept')}</option>
            </Select>
          </FormField>
          <FormField label={t('clients.groups.autoApplyNewInstances')}>
            <Checkbox checked={autoApply} onChange={(event) => updateDraft({ autoApply: event.currentTarget.checked })} label={t('clients.groups.autoApplyNewInstances')} />
          </FormField>
        </FormGrid>
        {scopeMode !== 'all_active_instances' ? (
          <DataTable
            title={t('instances.title')}
            rows={candidates}
            columns={[
              { key: 'select', header: '', render: (row) => (
                <input
                  aria-label={t('clients.groups.selectInstance', { instance: row.name || row.slug || row.id })}
                  type="checkbox"
                  checked={selectedInstances.has(row.id)}
                  onChange={(event) => toggleInstance(row.id, event.currentTarget.checked)}
                />
              ) },
              { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.name || row.slug || row.id)}</strong> },
              { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
              { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            ]}
          />
        ) : null}
        <p>{t('clients.groups.affectedInstances', { count: scope.data?.affected_instances || 0 })}</p>
        {updateScope.isError ? <ErrorState body={updateScope.error.message} /> : null}
        {lastResult ? <ResultSummary result={{
          created_memberships: 0,
          moved_memberships: 0,
          skipped_existing: 0,
          failed: [],
          materialized_created: lastResult.materialized_created || 0,
          materialized_updated: lastResult.materialized_updated || 0,
          materialized_disabled: lastResult.materialized_disabled || 0,
          affected_instances: lastResult.affected_instances || 0,
          apply_job_count: lastResult.apply_job_count || 0,
          apply_job_ids: lastResult.apply_job_ids || [],
          warnings: lastResult.warnings || [],
        }} /> : null}
        <div className="toolbar">
          <Button onClick={onClose}>{t('common.close')}</Button>
          <Button variant="primary" disabled={updateScope.isPending} onClick={() => void save()}>{updateScope.isPending ? t('common.loading') : t('common.save')}</Button>
        </div>
      </QueryBoundary>
    </div>
  );
}

function SyncDrawer({ group, onClose, onSuccess }: {
  group: ClientAccessGroup | null;
  onClose: () => void;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Drawer open={Boolean(group)} onClose={onClose} title={group ? `${t('clients.groups.syncAction')}: ${group.display_name || group.group_key}` : t('clients.groups.syncAction')}>
      {group ? <SyncDrawerContent key={group.id} group={group} onSuccess={onSuccess} /> : null}
    </Drawer>
  );
}

function SyncDrawerContent({ group, onSuccess }: {
  group: ClientAccessGroup;
  onSuccess: (message: string) => void;
}) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const syncState = useClientAccessGroupSyncState(group?.id);
  const preview = usePreviewClientAccessGroupSync();
  const apply = useApplyClientAccessGroupSync();
  const [previewStale, setPreviewStale] = useState(false);
  const [applyResult, setApplyResult] = useState<ClientAccessGroupMembershipResult | null>(null);

  const runPreview = async () => {
    try {
      await preview.mutateAsync(group.id);
      setPreviewStale(false);
      setApplyResult(null);
    } catch {
      // ErrorState renders the backend error.
    }
  };
  const runApply = async () => {
    if (!preview.data || previewStale) return;
    try {
      const result = await apply.mutateAsync(group.id);
      setApplyResult(result);
      onSuccess(t('clients.groups.syncApplied'));
      void syncState.refetch();
      setPreviewStale(true);
    } catch {
      // ErrorState renders the backend error.
    }
  };

  return (
    <div className="page-stack">
      <div className="toolbar">
        <Button disabled={preview.isPending} onClick={() => void runPreview()}>{preview.isPending ? t('common.loading') : t('common.preview')}</Button>
        <Button variant="primary" disabled={!preview.data || previewStale || apply.isPending} onClick={() => void runApply()}>{apply.isPending ? t('common.loading') : t('common.apply')}</Button>
        {previewStale ? <Badge>{t('clients.groups.previewStale')}</Badge> : null}
      </div>
      {preview.isError ? <ErrorState body={preview.error.message} /> : null}
      {apply.isError ? <ErrorState body={apply.error.message} /> : null}
      {preview.data ? (
        <Card>
          <CardBody>
            <div className="page-stack">
              <Badge>{t('clients.groups.syncPreview')}</Badge>
              <p>{t('clients.groups.syncPreviewSummary', {
                instances: preview.data.affected_instances,
                members: preview.data.member_count,
                pending: preview.data.pending_instances,
                applied: preview.data.applied_instances,
                failed: preview.data.failed_instances,
              })}</p>
              <code>{preview.data.desired_hash}</code>
              {preview.data.instance_ids?.length ? <p>{preview.data.instance_ids.join(', ')}</p> : null}
            </div>
          </CardBody>
        </Card>
      ) : null}
      <ResultSummary result={applyResult} />
      <QueryBoundary isLoading={syncState.isLoading} isError={syncState.isError} error={syncState.error} refetch={() => void syncState.refetch()}>
        <DataTable
          title={t('clients.groups.syncState')}
          rows={syncState.data || []}
          columns={[
            { key: 'instance', header: t('instances.instance'), render: (row) => <code>{row.instance_id}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'job', header: t('jobs.job'), render: (row) => row.last_job_id ? <a href="/operations/jobs">{row.last_job_id}</a> : text('') },
            { key: 'error', header: t('common.description'), render: (row) => text(row.last_error) },
            { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at) },
          ]}
        />
      </QueryBoundary>
    </div>
  );
}
