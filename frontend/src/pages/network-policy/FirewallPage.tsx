import { AlertTriangle, Ban, Pencil, Play, Plus, Save, ShieldCheck, Trash2 } from 'lucide-react';
import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { APIError } from '../../shared/api/client';
import type { FirewallAddressGroup, FirewallAddressGroupEntry, FirewallApplyResult, FirewallNodeState, FirewallPolicy, FirewallPreviewResult, FirewallRule } from '../../shared/api/types';
import {
  useApplyNodeFirewall,
  useCreateFirewallAddressGroup,
  useCreateFirewallAddressGroupEntry,
  useCreateFirewallPolicy,
  useCreateFirewallRule,
  useDeleteFirewallAddressGroup,
  useDeleteFirewallAddressGroupEntry,
  useDeleteFirewallPolicy,
  useDeleteFirewallRule,
  useDisableNodeFirewall,
  useFirewallInventory,
  useFirewallSafetySettings,
  useNodes,
  usePreviewNodeFirewall,
  useUpdateFirewallAddressGroup,
  useUpdateFirewallPolicy,
  useUpdateFirewallRule,
} from '../../shared/query/hooks';
import { Button, Card, CardBody, DataTable, FormField, FormGrid, IconButton, JobStatusPanel, Modal, Select, StatusBadge, TextField, Textarea, Toolbar } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type FirewallTab = 'address-groups' | 'policy' | 'rules' | 'preview' | 'node-state' | 'safety';

type GroupForm = {
  id: string;
  key: string;
  label: string;
  description: string;
  scope: string;
  status: string;
};

type EntryForm = {
  groupId: string;
  value: string;
  valueType: string;
  label: string;
};

type PolicyForm = {
  id: string;
  key: string;
  label: string;
  description: string;
  scope: string;
  status: string;
  defaultInputPolicy: string;
  defaultForwardPolicy: string;
  defaultOutputPolicy: string;
};

type RuleForm = {
  id: string;
  policyId: string;
  priority: string;
  chain: string;
  action: string;
  protocol: string;
  srcListId: string;
  dstListId: string;
  srcCIDR: string;
  dstCIDR: string;
  srcPorts: string;
  dstPorts: string;
  stateMatch: string;
  comment: string;
  enabled: boolean;
  log: boolean;
  status: string;
};

const emptyGroupForm: GroupForm = { id: '', key: '', label: '', description: '', scope: 'global', status: 'active' };
const emptyGroups: FirewallAddressGroup[] = [];
const emptyEntries: FirewallAddressGroupEntry[] = [];
const emptyPolicies: FirewallPolicy[] = [];
const emptyRules: FirewallRule[] = [];
const emptyNodeStates: FirewallNodeState[] = [];
const emptyPolicyForm: PolicyForm = {
  id: '',
  key: '',
  label: '',
  description: '',
  scope: 'global',
  status: 'active',
  defaultInputPolicy: 'accept',
  defaultForwardPolicy: 'accept',
  defaultOutputPolicy: 'accept',
};
const emptyRuleForm: RuleForm = {
  id: '',
  policyId: '',
  priority: '100',
  chain: 'input',
  action: 'accept',
  protocol: 'any',
  srcListId: '',
  dstListId: '',
  srcCIDR: '',
  dstCIDR: '',
  srcPorts: '',
  dstPorts: '',
  stateMatch: 'new,established,related',
  comment: '',
  enabled: true,
  log: false,
  status: 'active',
};

function isRenderableEntry(entry: FirewallAddressGroupEntry): boolean {
  const type = String(entry.value_type || '').toLowerCase();
  const value = String(entry.value || '').trim();
  return Boolean(value) && ['address', 'cidr', 'range'].includes(type) && entry.status !== 'deleted';
}

function groupEntries(entries: FirewallAddressGroupEntry[], groupID: string): FirewallAddressGroupEntry[] {
  return entries.filter((entry) => entry.list_id === groupID && entry.status !== 'deleted');
}

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) {
    return error instanceof Error ? error.message : 'Unexpected request failure.';
  }
  const prefix = error.status === 401
    ? 'Authentication required'
    : error.status === 403
      ? 'Permission denied'
      : error.status === 409
        ? 'Conflict'
        : error.status === 422 || error.status === 400
          ? 'Validation error'
          : error.status >= 500
            ? 'Backend error'
            : 'Request failed';
  return `${prefix} (${error.status}): ${error.message}`;
}

function policyName(policy?: FirewallPolicy): string {
  return text(policy?.label || policy?.key || policy?.id || '');
}

function nodeName(nodes: { id: string; name?: string; address?: string }[], nodeID: string): string {
  const node = nodes.find((item) => item.id === nodeID);
  return text(node?.name || node?.address || nodeID);
}

function previewHash(job?: FirewallPreviewResult | null): string {
  return String(job?.payload?.firewall_payload_hash || job?.result?.rendered_hash || '');
}

function firewallJobErrors(job?: FirewallPreviewResult | null): string[] {
  const raw = [
    ...(Array.isArray(job?.result?.blocking_errors) ? job.result.blocking_errors : []),
    ...(Array.isArray(job?.payload?.blocking_errors) ? job.payload.blocking_errors as string[] : []),
  ];
  return raw.map(String).filter(Boolean);
}

function firewallJobWarnings(job?: FirewallPreviewResult | FirewallApplyResult | null): string[] {
  const raw = [
    ...(Array.isArray(job?.result?.warnings) ? job.result.warnings : []),
    ...(Array.isArray(job?.payload?.warnings) ? job.payload.warnings as string[] : []),
  ];
  return raw.map(String).filter(Boolean);
}

function firewallRenderedText(job?: FirewallPreviewResult | null): string {
  const rendered = job?.result?.rendered_nftables || job?.result?.rendered_summary;
  if (typeof rendered === 'string' && rendered.trim()) return rendered;
  return JSON.stringify({
    payload_hash: job?.payload?.firewall_payload_hash,
    safety_mode: job?.payload?.safety_mode,
    policy_id: job?.payload?.policy_id,
    policy_key: job?.payload?.policy_key,
    defaults: {
      input: job?.payload?.default_input_policy,
      forward: job?.payload?.default_forward_policy,
      output: job?.payload?.default_output_policy,
    },
    rules: Array.isArray(job?.payload?.rules) ? job?.payload?.rules.length : 0,
    address_lists: Array.isArray(job?.payload?.address_lists) ? job?.payload?.address_lists.length : 0,
    ssh_bootstrap_ports: job?.payload?.ssh_bootstrap_ports,
    node_requires_forward_preservation: job?.payload?.node_requires_forward_preservation,
  }, null, 2);
}

function rulesUsingGroup(rules: FirewallRule[], groupID: string): FirewallRule[] {
  return rules.filter((rule) => rule.status !== 'deleted' && rule.enabled !== false && rule.action === 'accept' && (rule.src_list_id === groupID || rule.dst_list_id === groupID));
}

export function FirewallPage() {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<FirewallTab>('policy');
  const [targetNodeID, setTargetNodeID] = useState('');
  const [selectedPolicyID, setSelectedPolicyID] = useState('');
  const [strictMode, setStrictMode] = useState(true);
  const [groupForm, setGroupForm] = useState<GroupForm>(emptyGroupForm);
  const [entryForm, setEntryForm] = useState<EntryForm>({ groupId: '', value: '', valueType: 'cidr', label: '' });
  const [policyForm, setPolicyForm] = useState<PolicyForm>(emptyPolicyForm);
  const [ruleForm, setRuleForm] = useState<RuleForm>(emptyRuleForm);
  const [previewJob, setPreviewJob] = useState<FirewallPreviewResult | null>(null);
  const [previewSignature, setPreviewSignature] = useState('');
  const [applyJob, setApplyJob] = useState<FirewallApplyResult | null>(null);
  const [disableJobID, setDisableJobID] = useState('');
  const [confirmApplyOpen, setConfirmApplyOpen] = useState(false);
  const [confirmDisableOpen, setConfirmDisableOpen] = useState(false);
  const [groupEditorOpen, setGroupEditorOpen] = useState(false);
  const [entryEditorOpen, setEntryEditorOpen] = useState(false);
  const [policyEditorOpen, setPolicyEditorOpen] = useState(false);
  const [ruleEditorOpen, setRuleEditorOpen] = useState(false);
  const [notice, setNotice] = useState('');

  const firewall = useFirewallInventory();
  const nodes = useNodes();
  const safety = useFirewallSafetySettings({ retry: false });
  const createGroup = useCreateFirewallAddressGroup();
  const updateGroup = useUpdateFirewallAddressGroup();
  const deleteGroup = useDeleteFirewallAddressGroup();
  const createEntry = useCreateFirewallAddressGroupEntry();
  const deleteEntry = useDeleteFirewallAddressGroupEntry();
  const createPolicy = useCreateFirewallPolicy();
  const updatePolicy = useUpdateFirewallPolicy();
  const deletePolicy = useDeleteFirewallPolicy();
  const createRule = useCreateFirewallRule();
  const updateRule = useUpdateFirewallRule();
  const deleteRule = useDeleteFirewallRule();
  const preview = usePreviewNodeFirewall();
  const apply = useApplyNodeFirewall();
  const disable = useDisableNodeFirewall();

  const inventory = firewall.data;
  const policies = inventory?.policies || emptyPolicies;
  const groups = inventory?.address_lists || emptyGroups;
  const entries = inventory?.entries || emptyEntries;
  const rules = inventory?.rules || emptyRules;
  const nodeStates = inventory?.node_states || emptyNodeStates;
  const nodeList = nodes.data || [];
  const selectedPolicy = policies.find((policy) => policy.id === selectedPolicyID);
  const selectedRules = rules.filter((rule) => rule.policy_id === selectedPolicyID && rule.status !== 'deleted').sort((left, right) => Number(left.priority || 0) - Number(right.priority || 0));
  const selectedNodeState = nodeStates.find((state) => state.node_id === targetNodeID);

  const currentSignature = useMemo(() => JSON.stringify({
    targetNodeID,
    selectedPolicyID,
    strictMode,
    policies: policies.map((policy) => [policy.id, policy.updated_at, policy.default_input_policy, policy.default_forward_policy, policy.default_output_policy, policy.status]),
    rules: selectedRules.map((rule) => [rule.id, rule.updated_at, rule.priority, rule.chain, rule.action, rule.status, rule.enabled]),
    groups: groups.map((group) => [group.id, group.updated_at, group.status, group.entry_count]),
    entries: entries.map((entry) => [entry.id, entry.list_id, entry.updated_at, entry.status, entry.value, entry.value_type]),
  }), [entries, groups, policies, selectedPolicyID, selectedRules, strictMode, targetNodeID]);

  const previewStale = Boolean(previewJob && previewSignature && previewSignature !== currentSignature);
  const blockingErrors = firewallJobErrors(previewJob);
  const warnings = firewallJobWarnings(previewJob);
  const canPreview = Boolean(targetNodeID && selectedPolicyID && !preview.isPending);
  const canApply = Boolean(previewJob && !previewStale && !blockingErrors.length && targetNodeID && selectedPolicyID && !apply.isPending);
  const selectedPreviewHash = previewHash(previewJob);

  function clearNotice() {
    setNotice('');
  }

  async function submitGroup(event: FormEvent) {
    event.preventDefault();
    clearNotice();
    try {
      const input = {
        key: groupForm.key,
        label: groupForm.label,
        description: groupForm.description,
        scope: groupForm.scope,
        status: groupForm.status,
      };
      if (groupForm.id) {
        await updateGroup.mutateAsync({ id: groupForm.id, input });
        setNotice('Address group updated.');
      } else {
        const created = await createGroup.mutateAsync(input);
        setEntryForm((current) => ({ ...current, groupId: created.id }));
        setNotice('Address group created.');
      }
      setGroupForm(emptyGroupForm);
      setGroupEditorOpen(false);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  }

  async function submitEntry(event: FormEvent) {
    event.preventDefault();
    clearNotice();
    if (!entryForm.groupId) {
      setNotice('Select an address group before adding entries.');
      return;
    }
    try {
      await createEntry.mutateAsync({
        groupId: entryForm.groupId,
        input: {
          value: entryForm.value,
          value_type: entryForm.valueType,
          label: entryForm.label,
          status: 'active',
        },
      });
      setEntryForm((current) => ({ ...current, value: '', label: '' }));
      setNotice('Address group entry added.');
      setEntryEditorOpen(false);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  }

  async function submitPolicy(event: FormEvent) {
    event.preventDefault();
    clearNotice();
    try {
      const input = {
        key: policyForm.key,
        label: policyForm.label,
        description: policyForm.description,
        scope: policyForm.scope,
        status: policyForm.status,
        default_input_policy: policyForm.defaultInputPolicy,
        default_forward_policy: policyForm.defaultForwardPolicy,
        default_output_policy: policyForm.defaultOutputPolicy,
      };
      const policy = policyForm.id
        ? await updatePolicy.mutateAsync({ id: policyForm.id, input })
        : await createPolicy.mutateAsync(input);
      setSelectedPolicyID(policy.id);
      setPolicyForm(emptyPolicyForm);
      setNotice(policyForm.id ? 'Firewall policy updated.' : 'Firewall policy created.');
      setPolicyEditorOpen(false);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  }

  async function submitRule(event: FormEvent) {
    event.preventDefault();
    clearNotice();
    const policyId = ruleForm.policyId || selectedPolicyID;
    if (!policyId) {
      setNotice('Select a policy before changing rules.');
      return;
    }
    const input = {
      policy_id: policyId,
      priority: Number(ruleForm.priority || 0),
      chain: ruleForm.chain,
      action: ruleForm.action,
      direction: ruleForm.chain === 'forward' ? 'forward' : ruleForm.chain === 'output' ? 'out' : 'in',
      protocol: ruleForm.protocol,
      src_list_id: ruleForm.srcListId || undefined,
      dst_list_id: ruleForm.dstListId || undefined,
      src_cidr: ruleForm.srcCIDR,
      dst_cidr: ruleForm.dstCIDR,
      src_ports: ruleForm.srcPorts,
      dst_ports: ruleForm.dstPorts,
      state_match: ruleForm.stateMatch.split(',').map((item) => item.trim()).filter(Boolean),
      comment: ruleForm.comment,
      enabled: ruleForm.enabled,
      log: ruleForm.log,
      status: ruleForm.status,
    };
    try {
      if (ruleForm.id) {
        await updateRule.mutateAsync({ ruleId: ruleForm.id, input });
        setNotice('Firewall rule updated.');
      } else {
        await createRule.mutateAsync({ policyId, input });
        setNotice('Firewall rule created.');
      }
      setRuleForm({ ...emptyRuleForm, policyId });
      setRuleEditorOpen(false);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  }

  async function runPreview() {
    clearNotice();
    if (!targetNodeID || !selectedPolicyID) return;
    try {
      const job = await preview.mutateAsync({
        nodeId: targetNodeID,
        policyId: selectedPolicyID,
        input: { enforce_default_policy: strictMode },
      });
      setPreviewJob(job);
      setPreviewSignature(currentSignature);
      setApplyJob(null);
      setNotice('Firewall preview request accepted by backend.');
    } catch (error) {
      setPreviewJob(null);
      setPreviewSignature('');
      setNotice(formatAPIError(error));
    }
  }

  async function runApply() {
    clearNotice();
    if (!targetNodeID || !selectedPolicyID || !canApply) return;
    try {
      const job = await apply.mutateAsync({
        nodeId: targetNodeID,
        policyId: selectedPolicyID,
        previewHash: selectedPreviewHash,
        input: { enforce_default_policy: strictMode },
      });
      setApplyJob(job);
      setConfirmApplyOpen(false);
      setNotice('Firewall apply request accepted by backend.');
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  }

  async function runDisable() {
    clearNotice();
    if (!targetNodeID) return;
    try {
      const job = await disable.mutateAsync(targetNodeID);
      setDisableJobID(job.id);
      setConfirmDisableOpen(false);
      setNotice('Firewall disable request accepted by backend.');
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  }

  function editGroup(group: FirewallAddressGroup) {
    setGroupForm({
      id: group.id,
      key: group.key || '',
      label: group.label || '',
      description: group.description || '',
      scope: group.scope || 'global',
      status: group.status || 'active',
    });
    setEntryForm((current) => ({ ...current, groupId: group.id }));
    setActiveTab('address-groups');
    setGroupEditorOpen(true);
  }

  function editPolicy(policy: FirewallPolicy) {
    setPolicyForm({
      id: policy.id,
      key: policy.key || '',
      label: policy.label || '',
      description: policy.description || '',
      scope: policy.scope || 'global',
      status: policy.status || 'active',
      defaultInputPolicy: policy.default_input_policy || 'accept',
      defaultForwardPolicy: policy.default_forward_policy || 'accept',
      defaultOutputPolicy: policy.default_output_policy || 'accept',
    });
    setSelectedPolicyID(policy.id);
    setActiveTab('policy');
    setPolicyEditorOpen(true);
  }

  function editRule(rule: FirewallRule) {
    setRuleForm({
      id: rule.id,
      policyId: rule.policy_id || selectedPolicyID,
      priority: String(rule.priority || 100),
      chain: rule.chain || 'input',
      action: rule.action || 'accept',
      protocol: rule.protocol || 'any',
      srcListId: rule.src_list_id || '',
      dstListId: rule.dst_list_id || '',
      srcCIDR: rule.src_cidr || '',
      dstCIDR: rule.dst_cidr || '',
      srcPorts: rule.src_ports || '',
      dstPorts: rule.dst_ports || '',
      stateMatch: (rule.state_match || []).join(','),
      comment: rule.comment || '',
      enabled: rule.enabled !== false,
      log: Boolean(rule.log),
      status: rule.status || 'active',
    });
    setActiveTab('rules');
    setRuleEditorOpen(true);
  }

  return (
    <PageScaffold title={t('firewall.title')} subtitle={t('firewall.subtitle')}>
      <QueryBoundary isLoading={firewall.isLoading || nodes.isLoading} isError={firewall.isError || nodes.isError} error={(firewall.error || nodes.error) as Error | null} refetch={() => { void firewall.refetch(); void nodes.refetch(); }}>
        {notice ? <Card><CardBody><div role="status">{notice}</div></CardBody></Card> : null}

        <div className="tabs" role="tablist" aria-label="Firewall workflow">
          {[
            ['policy', t('firewall.policies')],
            ['rules', t('firewall.rules')],
            ['address-groups', t('firewall.addressGroups')],
            ['preview', t('firewall.applyTab')],
            ['node-state', t('firewall.nodeState')],
            ['safety', t('firewall.managementAccess')],
          ].map(([id, label]) => (
            <button
              className={`tab-link ${activeTab === id ? 'active' : ''}`.trim()}
              key={id}
              type="button"
              role="tab"
              aria-selected={activeTab === id}
              onClick={() => setActiveTab(id as FirewallTab)}
            >
              {label}
            </button>
          ))}
        </div>

        {activeTab === 'address-groups' ? (
          <div className="page-stack">
            <DataTable
              title={t('firewall.addressGroups')}
              rows={groups}
              tools={(
                <>
                  <Button icon={<Plus size={16} />} onClick={() => { setGroupForm(emptyGroupForm); setGroupEditorOpen(true); }}>{t('firewall.newGroup')}</Button>
                  <Button variant="primary" icon={<Plus size={16} />} onClick={() => { setEntryForm((current) => ({ ...current, groupId: current.groupId || groups[0]?.id || '' })); setEntryEditorOpen(true); }} disabled={!groups.length}>{t('firewall.addAddress')}</Button>
                </>
              )}
              columns={[
                { key: 'name', header: t('common.name'), render: (group) => <strong>{text(group.label || group.key || group.id)}</strong> },
                { key: 'status', header: t('common.status'), render: (group) => <StatusBadge status={group.status} /> },
                { key: 'counts', header: t('firewall.addresses'), render: (group) => {
                  const groupEntryList = groupEntries(entries, group.id);
                  const renderable = groupEntryList.filter(isRenderableEntry).length;
                  const ignored = groupEntryList.length - renderable;
                  const usedByAccept = rulesUsingGroup(rules, group.id).length > 0;
                  return (
                    <div className="table-cell-stack">
                      <span>{t('firewall.addressCount', { count: group.entry_count ?? groupEntryList.length })}</span>
                      {ignored > 0 ? <span className="status-warning"><AlertTriangle size={14} /> {t('firewall.ignoredAddressCount', { count: ignored })}</span> : null}
                      {usedByAccept && renderable === 0 ? <strong className="status-warning">{t('firewall.emptyGroupWarning')}</strong> : null}
                    </div>
                  );
                } },
                { key: 'actions', header: t('common.actions'), render: (group) => (
                  <Toolbar>
                    <IconButton title={t('firewall.editGroup')} onClick={() => editGroup(group)}><Pencil size={16} /></IconButton>
                    <IconButton className="icon-button-danger" title={t('firewall.deleteGroup')} onClick={() => void deleteGroup.mutateAsync(group.id).catch((error) => setNotice(formatAPIError(error)))}><Trash2 size={16} /></IconButton>
                  </Toolbar>
                ) },
              ]}
            />
            <DataTable
              title={t('firewall.addresses')}
              rows={entries.filter((entry) => entry.status !== 'deleted')}
              columns={[
                { key: 'group', header: t('firewall.addressGroup'), render: (entry) => text(groups.find((group) => group.id === entry.list_id)?.label || groups.find((group) => group.id === entry.list_id)?.key) },
                { key: 'value', header: t('firewall.address'), render: (entry) => <code>{text(entry.value)}</code> },
                { key: 'type', header: t('common.type'), render: (entry) => text(entry.value_type) },
                { key: 'status', header: t('common.status'), render: (entry) => <StatusBadge status={entry.status} /> },
                { key: 'actions', header: t('common.actions'), render: (entry) => <IconButton className="icon-button-danger" title={t('firewall.deleteAddress')} onClick={() => void deleteEntry.mutateAsync({ groupId: entry.list_id || entryForm.groupId, entryId: entry.id }).catch((error) => setNotice(formatAPIError(error)))}><Trash2 size={16} /></IconButton> },
              ]}
            />
          </div>
        ) : null}

        {activeTab === 'policy' ? (
          <div className="page-stack">
            <DataTable
              title={t('firewall.policies')}
              rows={policies}
              tools={<Button variant="primary" icon={<Plus size={16} />} onClick={() => { setPolicyForm(emptyPolicyForm); setPolicyEditorOpen(true); }}>{t('firewall.newPolicy')}</Button>}
              columns={[
                { key: 'name', header: t('common.name'), render: (policy) => <strong>{policyName(policy)}</strong> },
                { key: 'status', header: t('common.status'), render: (policy) => <StatusBadge status={policy.status} /> },
                { key: 'defaults', header: t('firewall.policyDefaults'), render: (policy) => (
                  <div className="table-cell-stack">
                    <span>{t('firewall.incomingTraffic')}: {t(`firewall.${policy.default_input_policy || 'accept'}`, { defaultValue: policy.default_input_policy || 'accept' })}</span>
                    <span>{t('firewall.forwardedTraffic')}: {t(`firewall.${policy.default_forward_policy || 'accept'}`, { defaultValue: policy.default_forward_policy || 'accept' })}</span>
                    <span>{t('firewall.outgoingTraffic')}: {t(`firewall.${policy.default_output_policy || 'accept'}`, { defaultValue: policy.default_output_policy || 'accept' })}</span>
                  </div>
                ) },
                { key: 'rules', header: t('firewall.rules'), render: (policy) => text(policy.rule_count) },
                { key: 'actions', header: t('common.actions'), render: (policy) => (
                  <Toolbar>
                    <IconButton title={t('firewall.editPolicy')} onClick={() => editPolicy(policy)}><Pencil size={16} /></IconButton>
                    <IconButton className="icon-button-danger" title={t('firewall.deletePolicy')} onClick={() => void deletePolicy.mutateAsync(policy.id).catch((error) => setNotice(formatAPIError(error)))}><Trash2 size={16} /></IconButton>
                  </Toolbar>
                ) },
              ]}
            />
          </div>
        ) : null}

        {activeTab === 'rules' ? (
          <div className="page-stack">
            <Card><CardBody><div className="firewall-rules-toolbar"><FormField label={t('firewall.policy')}><Select aria-label="Policy" value={selectedPolicyID} onChange={(event) => { setSelectedPolicyID(event.target.value); setRuleForm((current) => ({ ...current, policyId: event.target.value })); }}><option value="">{t('firewall.selectPolicy')}</option>{policies.map((policy) => <option key={policy.id} value={policy.id}>{policyName(policy)}</option>)}</Select></FormField><Button variant="primary" icon={<Plus size={16} />} disabled={!selectedPolicyID} onClick={() => { setRuleForm({ ...emptyRuleForm, policyId: selectedPolicyID }); setRuleEditorOpen(true); }}>{t('firewall.newRule')}</Button></div></CardBody></Card>
            <DataTable
              title={t('firewall.rules')}
              rows={selectedRules}
              columns={[
                { key: 'priority', header: t('firewall.priority'), render: (rule) => text(rule.priority) },
                { key: 'chain', header: t('firewall.trafficDirection'), render: (rule) => <strong>{t(`firewall.chain.${rule.chain || 'input'}`, { defaultValue: rule.chain || 'input' })}</strong> },
                { key: 'action', header: t('firewall.action'), render: (rule) => <StatusBadge status={rule.action} label={t(`firewall.${rule.action || 'accept'}`, { defaultValue: rule.action || 'accept' })} /> },
                { key: 'match', header: t('firewall.conditions'), render: (rule) => (
                  <div className="table-cell-stack">
                    <span>{t('firewall.protocol')}: {rule.protocol || t('firewall.any')}</span>
                    <span>{t('firewall.source')}: {rule.src_list_key || rule.src_cidr || t('firewall.any')}</span>
                    <span>{t('firewall.destination')}: {rule.dst_list_key || rule.dst_cidr || t('firewall.any')}</span>
                    <span>{t('firewall.ports')}: {rule.dst_ports || rule.src_ports || t('firewall.any')}</span>
                    <span>{t('firewall.connectionStates')}: {(rule.state_match || []).join(', ') || t('firewall.any')}</span>
                  </div>
                ) },
                { key: 'comment', header: t('firewall.comment'), render: (rule) => text(rule.comment) },
                { key: 'actions', header: t('common.actions'), render: (rule) => (
                  <Toolbar>
                    <IconButton title={t('firewall.editRule')} onClick={() => editRule(rule)}><Pencil size={16} /></IconButton>
                    <IconButton className="icon-button-danger" title={t('firewall.deleteRule')} onClick={() => void deleteRule.mutateAsync({ ruleId: rule.id, policyId: rule.policy_id || selectedPolicyID }).catch((error) => setNotice(formatAPIError(error)))}><Trash2 size={16} /></IconButton>
                  </Toolbar>
                ) },
              ]}
            />
          </div>
        ) : null}

        {activeTab === 'preview' ? (
          <div className="page-stack">
            <Card>
              <CardBody>
                <div className="page-stack">
                  <FormGrid>
                    <FormField label={t('firewall.targetNode')}>
                      <Select aria-label="Target node" value={targetNodeID} onChange={(event) => setTargetNodeID(event.target.value)}>
                        <option value="">{t('firewall.selectNode')}</option>
                        {nodeList.map((node) => <option key={node.id} value={node.id}>{nodeName(nodeList, node.id)}</option>)}
                      </Select>
                    </FormField>
                    <FormField label={t('firewall.policy')}>
                      <Select aria-label="Policy" value={selectedPolicyID} onChange={(event) => { setSelectedPolicyID(event.target.value); setRuleForm((current) => ({ ...current, policyId: event.target.value })); }}>
                        <option value="">{t('firewall.selectPolicy')}</option>
                        {policies.map((policy) => <option key={policy.id} value={policy.id}>{policyName(policy)}</option>)}
                      </Select>
                    </FormField>
                    <FormField label={t('firewall.policyDefaults')}>
                      <label className="toolbar">
                        <input type="checkbox" checked={strictMode} onChange={(event) => setStrictMode(event.target.checked)} />
                        <span>{t('firewall.enforceDefaults')}</span>
                      </label>
                    </FormField>
                    <FormField label={t('firewall.currentState')}>
                      <StatusBadge status={selectedNodeState?.status || 'unknown'} />
                    </FormField>
                  </FormGrid>
                  <Toolbar>
                    <Button variant="primary" icon={<Play size={16} />} disabled={!canPreview} onClick={() => void runPreview()}>{t('common.preview')}</Button>
                    <Button variant="danger" icon={<ShieldCheck size={16} />} disabled={!canApply} onClick={() => setConfirmApplyOpen(true)}>{t('common.apply')}</Button>
                    {previewStale ? <StatusBadge status="warning" label={t('firewall.previewStale')} /> : null}
                    {blockingErrors.length ? <StatusBadge status="failed" label={t('firewall.previewBlocked')} /> : null}
                  </Toolbar>
                  {warnings.length ? <div className="validation-summary status-warning"><strong>{t('firewall.warnings')}</strong><ul>{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul></div> : null}
                  {blockingErrors.length ? <div className="validation-summary error-state-inline"><strong>{t('firewall.blockingErrors')}</strong><ul>{blockingErrors.map((error) => <li key={error}>{error}</li>)}</ul></div> : null}
                  {previewJob ? <details className="technical-details"><summary>{t('settings.details')}</summary><pre className="code-block">{firewallRenderedText(previewJob)}</pre></details> : null}
                </div>
              </CardBody>
            </Card>
            {previewJob ? <><Link to="/operations/jobs">{t('jobs.openJobs')}</Link><JobStatusPanel jobID={previewJob.id} /></> : null}
            {applyJob ? <><Link to="/operations/jobs">{t('jobs.openJobs')}</Link><JobStatusPanel jobID={applyJob.id} /></> : null}
          </div>
        ) : null}

        {activeTab === 'node-state' ? (
          <div className="page-stack">
            <DataTable
              title={t('firewall.nodeFirewallState')}
              rows={nodeStates}
              columns={[
                { key: 'node', header: t('firewall.node'), render: (state) => text(state.node_name || state.node_id) },
                { key: 'policy', header: t('firewall.policy'), render: (state) => text(state.policy_key || state.policy_id) },
                { key: 'status', header: t('common.status'), render: (state) => <StatusBadge status={state.status} /> },
                { key: 'revision', header: t('firewall.revision'), render: (state) => text(state.revision_id || state.desired_revision_id) },
                { key: 'job', header: t('firewall.job'), render: (state) => state.last_job_id ? <Link to="/operations/jobs">{state.last_job_id}</Link> : t('common.none') },
              ]}
            />
            <Card>
              <CardBody>
                <div className="page-stack">
                  <FormField label={t('firewall.targetNode')}>
                    <Select aria-label="Disable target node" value={targetNodeID} onChange={(event) => setTargetNodeID(event.target.value)}>
                      <option value="">{t('firewall.selectNode')}</option>
                      {nodeList.map((node) => <option key={node.id} value={node.id}>{nodeName(nodeList, node.id)}</option>)}
                    </Select>
                  </FormField>
                  <Toolbar>
                    <Button variant="danger" icon={<Ban size={16} />} disabled={!targetNodeID || disable.isPending} onClick={() => setConfirmDisableOpen(true)}>{t('firewall.emergencyDisable')}</Button>
                  </Toolbar>
                  {disableJobID ? <><Link to="/operations/jobs">{t('jobs.openJobs')}</Link><JobStatusPanel jobID={disableJobID} /></> : null}
                </div>
              </CardBody>
            </Card>
          </div>
        ) : null}

        {activeTab === 'safety' ? (
          <div className="page-stack">
            <Card>
              <CardBody>
                <div className="page-stack">
                  <h2 className="card-title">{t('firewall.managementAccess')}</h2>
                  <div className="definition-grid">
                    <span>{t('firewall.controlPlaneAccess')}</span><StatusBadge status={(safety.data?.control_plane_source_cidrs || []).length ? 'configured' : 'warning'} label={(safety.data?.control_plane_source_cidrs || []).length ? t('firewall.configured') : t('firewall.notConfigured')} />
                    <span>{t('firewall.operatorAccess')}</span><StatusBadge status={(safety.data?.trusted_operator_cidrs || []).length ? 'configured' : 'warning'} label={(safety.data?.trusted_operator_cidrs || []).length ? t('firewall.configured') : t('firewall.notConfigured')} />
                    <span>{t('firewall.sshBootstrapAccess')}</span><StatusBadge status={(safety.data?.ssh_bootstrap_source_cidrs || []).length ? 'configured' : 'warning'} label={(safety.data?.ssh_bootstrap_source_cidrs || []).length ? t('firewall.configured') : t('firewall.notConfigured')} />
                    <span>{t('firewall.vpnForwarding')}</span><StatusBadge status={groups.some((group) => group.key === 'vpn_client_sources' && groupEntries(entries, group.id).some(isRenderableEntry)) ? 'configured' : 'warning'} label={groups.some((group) => group.key === 'vpn_client_sources' && groupEntries(entries, group.id).some(isRenderableEntry)) ? t('firewall.configured') : t('firewall.notConfigured')} />
                    <span>{t('firewall.backhaulForwarding')}</span><StatusBadge status={groups.some((group) => group.key === 'backhaul_sources' && groupEntries(entries, group.id).some(isRenderableEntry)) ? 'configured' : 'warning'} label={groups.some((group) => group.key === 'backhaul_sources' && groupEntries(entries, group.id).some(isRenderableEntry)) ? t('firewall.configured') : t('firewall.notConfigured')} />
                  </div>
                  {safety.isError ? <div>{formatAPIError(safety.error)}</div> : null}
                </div>
              </CardBody>
            </Card>
          </div>
        ) : null}

        <Modal title={groupForm.id ? t('firewall.editGroup') : t('firewall.newGroup')} open={groupEditorOpen} onClose={() => setGroupEditorOpen(false)}>
          <form className="page-stack" onSubmit={(event) => void submitGroup(event)}>
            <FormGrid>
              <FormField label={t('common.name')} full>
                <TextField aria-label="Address group label" value={groupForm.label} onChange={(event) => setGroupForm({ ...groupForm, label: event.target.value })} required autoFocus />
              </FormField>
              <FormField label={t('common.status')}>
                <Select aria-label="Address group status" value={groupForm.status} onChange={(event) => setGroupForm({ ...groupForm, status: event.target.value })}>
                  <option value="active">{t('common.active')}</option>
                  <option value="disabled">{t('common.disabled')}</option>
                </Select>
              </FormField>
              <FormField label={t('common.description')} full>
                <Textarea aria-label="Address group description" value={groupForm.description} onChange={(event) => setGroupForm({ ...groupForm, description: event.target.value })} />
              </FormField>
            </FormGrid>
            <details className="technical-details">
              <summary>{t('common.advanced')}</summary>
              <FormGrid>
                <FormField label={t('firewall.internalKey')}><TextField aria-label="Address group key" value={groupForm.key} onChange={(event) => setGroupForm({ ...groupForm, key: event.target.value })} /></FormField>
                <FormField label={t('firewall.scope')}><TextField aria-label="Address group scope" value={groupForm.scope} onChange={(event) => setGroupForm({ ...groupForm, scope: event.target.value })} /></FormField>
              </FormGrid>
            </details>
            <div className="modal-actions">
              <Button variant="primary" icon={<Save size={16} />} type="submit" disabled={createGroup.isPending || updateGroup.isPending}>{t('common.save')}</Button>
              <Button onClick={() => setGroupEditorOpen(false)}>{t('common.cancel')}</Button>
            </div>
          </form>
        </Modal>

        <Modal title={t('firewall.addAddress')} open={entryEditorOpen} onClose={() => setEntryEditorOpen(false)}>
          <form className="page-stack" onSubmit={(event) => void submitEntry(event)}>
            <FormGrid>
              <FormField label={t('firewall.addressGroup')} full>
                <Select aria-label="Entry address group" value={entryForm.groupId} onChange={(event) => setEntryForm({ ...entryForm, groupId: event.target.value })} required autoFocus>
                  <option value="">{t('firewall.selectGroup')}</option>
                  {groups.map((group) => <option key={group.id} value={group.id}>{text(group.label || group.key || group.id)}</option>)}
                </Select>
              </FormField>
              <FormField label={t('common.type')}>
                <Select aria-label="Entry value type" value={entryForm.valueType} onChange={(event) => setEntryForm({ ...entryForm, valueType: event.target.value })}>
                  <option value="cidr">CIDR</option>
                  <option value="address">{t('firewall.ipAddress')}</option>
                  <option value="range">{t('firewall.ipRange')}</option>
                  <option value="dns">DNS</option>
                </Select>
              </FormField>
              <FormField label={t('firewall.address')}>
                <TextField aria-label="Entry value" value={entryForm.value} onChange={(event) => setEntryForm({ ...entryForm, value: event.target.value })} placeholder={entryForm.valueType === 'dns' ? 'host.example.com' : '10.0.0.0/24'} required />
              </FormField>
              <FormField label={t('firewall.label')} full>
                <TextField aria-label="Entry label" value={entryForm.label} onChange={(event) => setEntryForm({ ...entryForm, label: event.target.value })} />
              </FormField>
            </FormGrid>
            <div className="modal-actions">
              <Button variant="primary" icon={<Plus size={16} />} type="submit" disabled={createEntry.isPending}>{t('firewall.addAddress')}</Button>
              <Button onClick={() => setEntryEditorOpen(false)}>{t('common.cancel')}</Button>
            </div>
          </form>
        </Modal>

        <Modal title={policyForm.id ? t('firewall.editPolicy') : t('firewall.newPolicy')} open={policyEditorOpen} onClose={() => setPolicyEditorOpen(false)} size="wide">
          <form className="page-stack" onSubmit={(event) => void submitPolicy(event)}>
            <FormGrid>
              <FormField label={t('common.name')} full><TextField aria-label="Policy label" value={policyForm.label} onChange={(event) => setPolicyForm({ ...policyForm, label: event.target.value })} required autoFocus /></FormField>
              <FormField label={t('firewall.incomingTraffic')}><Select aria-label="Default input policy" value={policyForm.defaultInputPolicy} onChange={(event) => setPolicyForm({ ...policyForm, defaultInputPolicy: event.target.value })}><option value="accept">{t('firewall.allow')}</option><option value="drop">{t('firewall.drop')}</option><option value="reject">{t('firewall.reject')}</option></Select></FormField>
              <FormField label={t('firewall.forwardedTraffic')}><Select aria-label="Default forward policy" value={policyForm.defaultForwardPolicy} onChange={(event) => setPolicyForm({ ...policyForm, defaultForwardPolicy: event.target.value })}><option value="accept">{t('firewall.allow')}</option><option value="drop">{t('firewall.drop')}</option><option value="reject">{t('firewall.reject')}</option></Select></FormField>
              <FormField label={t('firewall.outgoingTraffic')}><Select aria-label="Default output policy" value={policyForm.defaultOutputPolicy} onChange={(event) => setPolicyForm({ ...policyForm, defaultOutputPolicy: event.target.value })}><option value="accept">{t('firewall.allow')}</option><option value="drop">{t('firewall.drop')}</option><option value="reject">{t('firewall.reject')}</option></Select></FormField>
              <FormField label={t('common.status')}><Select aria-label="Policy status" value={policyForm.status} onChange={(event) => setPolicyForm({ ...policyForm, status: event.target.value })}><option value="active">{t('common.active')}</option><option value="disabled">{t('common.disabled')}</option></Select></FormField>
              <FormField label={t('common.description')} full><Textarea aria-label="Policy description" value={policyForm.description} onChange={(event) => setPolicyForm({ ...policyForm, description: event.target.value })} /></FormField>
            </FormGrid>
            <details className="technical-details">
              <summary>{t('common.advanced')}</summary>
              <FormGrid>
                <FormField label={t('firewall.internalKey')}><TextField aria-label="Policy key" value={policyForm.key} onChange={(event) => setPolicyForm({ ...policyForm, key: event.target.value })} /></FormField>
                <FormField label={t('firewall.scope')}><TextField aria-label="Policy scope" value={policyForm.scope} onChange={(event) => setPolicyForm({ ...policyForm, scope: event.target.value })} /></FormField>
              </FormGrid>
            </details>
            <div className="modal-actions">
              <Button variant="primary" icon={<Save size={16} />} type="submit" disabled={createPolicy.isPending || updatePolicy.isPending}>{t('common.save')}</Button>
              <Button onClick={() => setPolicyEditorOpen(false)}>{t('common.cancel')}</Button>
            </div>
          </form>
        </Modal>

        <Modal title={ruleForm.id ? t('firewall.editRule') : t('firewall.newRule')} open={ruleEditorOpen} onClose={() => setRuleEditorOpen(false)} size="wide">
          <form className="page-stack" onSubmit={(event) => void submitRule(event)}>
            <FormGrid>
              <FormField label={t('firewall.priority')}><TextField aria-label="Rule priority" type="number" min="0" value={ruleForm.priority} onChange={(event) => setRuleForm({ ...ruleForm, priority: event.target.value })} autoFocus /></FormField>
              <FormField label={t('firewall.trafficDirection')}><Select aria-label="Rule chain" value={ruleForm.chain} onChange={(event) => setRuleForm({ ...ruleForm, chain: event.target.value })}><option value="input">{t('firewall.incomingTraffic')}</option><option value="forward">{t('firewall.forwardedTraffic')}</option><option value="output">{t('firewall.outgoingTraffic')}</option></Select></FormField>
              <FormField label={t('firewall.action')}><Select aria-label="Rule action" value={ruleForm.action} onChange={(event) => setRuleForm({ ...ruleForm, action: event.target.value })}><option value="accept">{t('firewall.allow')}</option><option value="drop">{t('firewall.drop')}</option><option value="reject">{t('firewall.reject')}</option></Select></FormField>
              <FormField label={t('firewall.protocol')}><Select aria-label="Rule protocol" value={ruleForm.protocol} onChange={(event) => setRuleForm({ ...ruleForm, protocol: event.target.value })}><option value="any">{t('firewall.any')}</option><option value="tcp">TCP</option><option value="udp">UDP</option><option value="icmp">ICMP</option><option value="icmpv6">ICMPv6</option></Select></FormField>
              <FormField label={t('firewall.sourceGroup')}><Select aria-label="Source group" value={ruleForm.srcListId} onChange={(event) => setRuleForm({ ...ruleForm, srcListId: event.target.value })}><option value="">{t('firewall.any')}</option>{groups.map((group) => <option key={group.id} value={group.id}>{text(group.label || group.key || group.id)}</option>)}</Select></FormField>
              <FormField label={t('firewall.destinationGroup')}><Select aria-label="Destination group" value={ruleForm.dstListId} onChange={(event) => setRuleForm({ ...ruleForm, dstListId: event.target.value })}><option value="">{t('firewall.any')}</option>{groups.map((group) => <option key={group.id} value={group.id}>{text(group.label || group.key || group.id)}</option>)}</Select></FormField>
              <FormField label={t('firewall.sourceAddress')}><TextField aria-label="Source CIDR" value={ruleForm.srcCIDR} onChange={(event) => setRuleForm({ ...ruleForm, srcCIDR: event.target.value })} placeholder="0.0.0.0/0" /></FormField>
              <FormField label={t('firewall.destinationAddress')}><TextField aria-label="Destination CIDR" value={ruleForm.dstCIDR} onChange={(event) => setRuleForm({ ...ruleForm, dstCIDR: event.target.value })} placeholder="0.0.0.0/0" /></FormField>
              <FormField label={t('firewall.sourcePorts')}><TextField aria-label="Source ports" value={ruleForm.srcPorts} onChange={(event) => setRuleForm({ ...ruleForm, srcPorts: event.target.value })} /></FormField>
              <FormField label={t('firewall.destinationPorts')}><TextField aria-label="Destination ports" value={ruleForm.dstPorts} onChange={(event) => setRuleForm({ ...ruleForm, dstPorts: event.target.value })} placeholder="22,80,443" /></FormField>
              <FormField label={t('common.status')}><Select aria-label="Rule status" value={ruleForm.status} onChange={(event) => setRuleForm({ ...ruleForm, status: event.target.value })}><option value="active">{t('common.active')}</option><option value="disabled">{t('common.disabled')}</option></Select></FormField>
              <FormField label={t('firewall.options')}>
                <div className="checkbox-stack">
                  <label><input aria-label="Rule enabled" type="checkbox" checked={ruleForm.enabled} onChange={(event) => setRuleForm({ ...ruleForm, enabled: event.target.checked })} /> {t('firewall.enabled')}</label>
                  <label><input aria-label="Rule log" type="checkbox" checked={ruleForm.log} onChange={(event) => setRuleForm({ ...ruleForm, log: event.target.checked })} /> {t('firewall.logMatches')}</label>
                </div>
              </FormField>
              <FormField label={t('firewall.comment')} full><Textarea aria-label="Rule comment" value={ruleForm.comment} onChange={(event) => setRuleForm({ ...ruleForm, comment: event.target.value })} /></FormField>
            </FormGrid>
            <details className="technical-details">
              <summary>{t('common.advanced')}</summary>
              <FormField label={t('firewall.connectionStates')}><TextField aria-label="State match" value={ruleForm.stateMatch} onChange={(event) => setRuleForm({ ...ruleForm, stateMatch: event.target.value })} /></FormField>
            </details>
            <div className="modal-actions">
              <Button variant="primary" icon={<Save size={16} />} type="submit" disabled={createRule.isPending || updateRule.isPending}>{t('common.save')}</Button>
              <Button onClick={() => setRuleEditorOpen(false)}>{t('common.cancel')}</Button>
            </div>
          </form>
        </Modal>

        <Modal title={t('firewall.confirmApplyTitle')} open={confirmApplyOpen} onClose={() => setConfirmApplyOpen(false)}>
          <div className="page-stack">
            <div className="definition-grid">
              <span>{t('firewall.targetNode')}</span><strong>{targetNodeID ? nodeName(nodeList, targetNodeID) : t('common.none')}</strong>
              <span>{t('firewall.policy')}</span><strong>{selectedPolicy ? policyName(selectedPolicy) : t('common.none')}</strong>
            </div>
            {warnings.length ? <div className="validation-summary status-warning"><ul>{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul></div> : null}
            <div className="modal-actions">
              <Button variant="danger" onClick={() => void runApply()}>{t('firewall.confirmApply')}</Button>
              <Button onClick={() => setConfirmApplyOpen(false)}>{t('common.cancel')}</Button>
            </div>
          </div>
        </Modal>

        <Modal title={t('firewall.confirmDisableTitle')} open={confirmDisableOpen} onClose={() => setConfirmDisableOpen(false)}>
          <div className="page-stack">
            <p>{t('firewall.disableWarning')}</p>
            <div className="definition-grid"><span>{t('firewall.targetNode')}</span><strong>{targetNodeID ? nodeName(nodeList, targetNodeID) : t('common.none')}</strong></div>
            <div className="modal-actions">
              <Button variant="danger" onClick={() => void runDisable()}>{t('firewall.confirmDisable')}</Button>
              <Button onClick={() => setConfirmDisableOpen(false)}>{t('common.cancel')}</Button>
            </div>
          </div>
        </Modal>
      </QueryBoundary>
    </PageScaffold>
  );
}
