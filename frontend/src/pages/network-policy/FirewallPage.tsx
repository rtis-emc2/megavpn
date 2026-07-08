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
import { Badge, Button, Card, CardBody, DataTable, FormField, FormGrid, JobStatusPanel, Modal, Select, StatusBadge, TextField, Textarea, Toolbar } from '../../shared/ui';
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
  const [activeTab, setActiveTab] = useState<FirewallTab>('address-groups');
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
  }

  const actions = (
    <Toolbar>
      <Badge>{t('firewall.previewMandatory')}</Badge>
      <Badge>{t('firewall.sshSafety')}</Badge>
      <Badge>{t('firewall.forwardOutputSafety')}</Badge>
    </Toolbar>
  );

  return (
    <PageScaffold title={t('firewall.title')} subtitle={t('firewall.subtitle')} actions={actions}>
      <QueryBoundary isLoading={firewall.isLoading || nodes.isLoading} isError={firewall.isError || nodes.isError} error={(firewall.error || nodes.error) as Error | null} refetch={() => { void firewall.refetch(); void nodes.refetch(); }}>
        <Card>
          <CardBody>
            <FormGrid>
              <FormField label="Target node">
                <Select aria-label="Target node" value={targetNodeID} onChange={(event) => setTargetNodeID(event.target.value)}>
                  <option value="">Select node</option>
                  {nodeList.map((node) => <option key={node.id} value={node.id}>{nodeName(nodeList, node.id)}</option>)}
                </Select>
              </FormField>
              <FormField label="Policy">
                <Select aria-label="Policy" value={selectedPolicyID} onChange={(event) => {
                  setSelectedPolicyID(event.target.value);
                  setRuleForm((current) => ({ ...current, policyId: event.target.value }));
                }}>
                  <option value="">Select policy</option>
                  {policies.map((policy) => <option key={policy.id} value={policy.id}>{policyName(policy)}</option>)}
                </Select>
              </FormField>
              <FormField label="Strict defaults">
                <label className="toolbar">
                  <input type="checkbox" checked={strictMode} onChange={(event) => setStrictMode(event.target.checked)} />
                  <span>Enforce default input/forward/output policies</span>
                </label>
              </FormField>
              <FormField label="Node firewall status">
                <div className="toolbar">
                  <StatusBadge status={selectedNodeState?.status || 'unknown'} />
                  {selectedNodeState?.last_job_id ? <Link to="/operations/jobs">Job {selectedNodeState.last_job_id}</Link> : null}
                </div>
              </FormField>
            </FormGrid>
          </CardBody>
        </Card>

        {notice ? <Card><CardBody><div role="status">{notice}</div></CardBody></Card> : null}

        <div className="tabs" role="tablist" aria-label="Firewall workflow">
          {[
            ['address-groups', 'Address groups'],
            ['policy', 'Policy'],
            ['rules', 'Rules'],
            ['preview', 'Preview & Apply'],
            ['node-state', 'Node state'],
            ['safety', 'Safety'],
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
              title="Address groups"
              rows={groups}
              tools={<Button icon={<Plus size={16} />} onClick={() => setGroupForm(emptyGroupForm)}>New group</Button>}
              columns={[
                { key: 'name', header: t('common.name'), render: (group) => <strong>{text(group.label || group.key || group.id)}</strong> },
                { key: 'status', header: t('common.status'), render: (group) => <StatusBadge status={group.status} /> },
                { key: 'counts', header: 'Entries', render: (group) => {
                  const groupEntryList = groupEntries(entries, group.id);
                  const renderable = groupEntryList.filter(isRenderableEntry).length;
                  const ignored = groupEntryList.length - renderable;
                  const usedByAccept = rulesUsingGroup(rules, group.id).length > 0;
                  return (
                    <div>
                      <div>{group.entry_count ?? groupEntryList.length} total, {renderable} renderable, {ignored} ignored DNS-only</div>
                      {ignored > 0 ? <div><AlertTriangle size={14} /> DNS-only entries are not rendered into nftables.</div> : null}
                      {usedByAccept && renderable === 0 ? <strong>Blocking warning: active accept rule references this group without renderable entries.</strong> : null}
                    </div>
                  );
                } },
                { key: 'actions', header: t('common.actions'), render: (group) => (
                  <Toolbar>
                    <Button icon={<Pencil size={16} />} onClick={() => editGroup(group)}>Edit group</Button>
                    <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => void deleteGroup.mutateAsync(group.id).catch((error) => setNotice(formatAPIError(error)))}>Delete group</Button>
                  </Toolbar>
                ) },
              ]}
            />
            <Card>
              <CardBody>
                <form className="page-stack" onSubmit={(event) => void submitGroup(event)}>
                  <h2 className="card-title">{groupForm.id ? 'Update address group' : 'Create address group'}</h2>
                  <FormGrid>
                    <FormField label="Key"><TextField aria-label="Address group key" value={groupForm.key} onChange={(event) => setGroupForm({ ...groupForm, key: event.target.value })} /></FormField>
                    <FormField label="Label"><TextField aria-label="Address group label" value={groupForm.label} onChange={(event) => setGroupForm({ ...groupForm, label: event.target.value })} required /></FormField>
                    <FormField label="Scope"><TextField aria-label="Address group scope" value={groupForm.scope} onChange={(event) => setGroupForm({ ...groupForm, scope: event.target.value })} /></FormField>
                    <FormField label="Status"><Select aria-label="Address group status" value={groupForm.status} onChange={(event) => setGroupForm({ ...groupForm, status: event.target.value })}><option value="active">active</option><option value="disabled">disabled</option></Select></FormField>
                    <FormField label="Description" full><Textarea aria-label="Address group description" value={groupForm.description} onChange={(event) => setGroupForm({ ...groupForm, description: event.target.value })} /></FormField>
                  </FormGrid>
                  <Toolbar>
                    <Button variant="primary" icon={<Save size={16} />} type="submit">{groupForm.id ? 'Update address group' : 'Create address group'}</Button>
                    {groupForm.id ? <Button type="button" onClick={() => setGroupForm(emptyGroupForm)}>Cancel edit</Button> : null}
                  </Toolbar>
                </form>
              </CardBody>
            </Card>
            <Card>
              <CardBody>
                <form className="page-stack" onSubmit={(event) => void submitEntry(event)}>
                  <h2 className="card-title">Address group entries</h2>
                  <FormGrid>
                    <FormField label="Group">
                      <Select aria-label="Entry address group" value={entryForm.groupId} onChange={(event) => setEntryForm({ ...entryForm, groupId: event.target.value })}>
                        <option value="">Select group</option>
                        {groups.map((group) => <option key={group.id} value={group.id}>{text(group.label || group.key || group.id)}</option>)}
                      </Select>
                    </FormField>
                    <FormField label="Type">
                      <Select aria-label="Entry value type" value={entryForm.valueType} onChange={(event) => setEntryForm({ ...entryForm, valueType: event.target.value })}>
                        <option value="cidr">CIDR</option>
                        <option value="address">IP address</option>
                        <option value="range">IP range</option>
                        <option value="dns">DNS-only</option>
                      </Select>
                    </FormField>
                    <FormField label="Value"><TextField aria-label="Entry value" value={entryForm.value} onChange={(event) => setEntryForm({ ...entryForm, value: event.target.value })} placeholder="10.0.0.0/24" /></FormField>
                    <FormField label="Label"><TextField aria-label="Entry label" value={entryForm.label} onChange={(event) => setEntryForm({ ...entryForm, label: event.target.value })} /></FormField>
                  </FormGrid>
                  <Toolbar><Button variant="primary" type="submit" icon={<Plus size={16} />}>Add entry</Button></Toolbar>
                </form>
              </CardBody>
            </Card>
            <DataTable
              title="Selected group entries"
              rows={groupEntries(entries, entryForm.groupId)}
              columns={[
                { key: 'value', header: 'Value', render: (entry) => <code>{text(entry.value)}</code> },
                { key: 'type', header: 'Type', render: (entry) => text(entry.value_type) },
                { key: 'status', header: 'Status', render: (entry) => <StatusBadge status={entry.status} /> },
                { key: 'actions', header: 'Actions', render: (entry) => <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => void deleteEntry.mutateAsync({ groupId: entry.list_id || entryForm.groupId, entryId: entry.id }).catch((error) => setNotice(formatAPIError(error)))}>Delete entry</Button> },
              ]}
            />
          </div>
        ) : null}

        {activeTab === 'policy' ? (
          <div className="page-stack">
            <DataTable
              title="Policies"
              rows={policies}
              columns={[
                { key: 'name', header: t('common.name'), render: (policy) => <strong>{policyName(policy)}</strong> },
                { key: 'status', header: t('common.status'), render: (policy) => <StatusBadge status={policy.status} /> },
                { key: 'defaults', header: 'Default policy', render: (policy) => `${policy.default_input_policy || 'accept'} / ${policy.default_forward_policy || 'accept'} / ${policy.default_output_policy || 'accept'}` },
                { key: 'rules', header: 'Rules', render: (policy) => text(policy.rule_count) },
                { key: 'actions', header: t('common.actions'), render: (policy) => (
                  <Toolbar>
                    <Button icon={<Pencil size={16} />} onClick={() => editPolicy(policy)}>Edit policy</Button>
                    <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => void deletePolicy.mutateAsync(policy.id).catch((error) => setNotice(formatAPIError(error)))}>Delete policy</Button>
                  </Toolbar>
                ) },
              ]}
            />
            <Card>
              <CardBody>
                <form className="page-stack" onSubmit={(event) => void submitPolicy(event)}>
                  <h2 className="card-title">{policyForm.id ? 'Update policy' : 'Create policy'}</h2>
                  <FormGrid>
                    <FormField label="Key"><TextField aria-label="Policy key" value={policyForm.key} onChange={(event) => setPolicyForm({ ...policyForm, key: event.target.value })} /></FormField>
                    <FormField label="Label"><TextField aria-label="Policy label" value={policyForm.label} onChange={(event) => setPolicyForm({ ...policyForm, label: event.target.value })} required /></FormField>
                    <FormField label="Input default"><Select aria-label="Default input policy" value={policyForm.defaultInputPolicy} onChange={(event) => setPolicyForm({ ...policyForm, defaultInputPolicy: event.target.value })}><option value="accept">accept</option><option value="drop">drop</option><option value="reject">reject</option></Select></FormField>
                    <FormField label="Forward default"><Select aria-label="Default forward policy" value={policyForm.defaultForwardPolicy} onChange={(event) => setPolicyForm({ ...policyForm, defaultForwardPolicy: event.target.value })}><option value="accept">accept</option><option value="drop">drop</option><option value="reject">reject</option></Select></FormField>
                    <FormField label="Output default"><Select aria-label="Default output policy" value={policyForm.defaultOutputPolicy} onChange={(event) => setPolicyForm({ ...policyForm, defaultOutputPolicy: event.target.value })}><option value="accept">accept</option><option value="drop">drop</option><option value="reject">reject</option></Select></FormField>
                    <FormField label="Status"><Select aria-label="Policy status" value={policyForm.status} onChange={(event) => setPolicyForm({ ...policyForm, status: event.target.value })}><option value="active">active</option><option value="disabled">disabled</option></Select></FormField>
                    <FormField label="Description" full><Textarea aria-label="Policy description" value={policyForm.description} onChange={(event) => setPolicyForm({ ...policyForm, description: event.target.value })} /></FormField>
                  </FormGrid>
                  <Toolbar><Button variant="primary" icon={<Save size={16} />} type="submit">{policyForm.id ? 'Update policy' : 'Create policy'}</Button></Toolbar>
                </form>
              </CardBody>
            </Card>
          </div>
        ) : null}

        {activeTab === 'rules' ? (
          <div className="page-stack">
            <Card><CardBody><Toolbar><Badge>Effective order</Badge><Badge>Reorder unsupported by backend API</Badge>{selectedPolicy ? <strong>{policyName(selectedPolicy)}</strong> : null}</Toolbar></CardBody></Card>
            <DataTable
              title="Rules"
              rows={selectedRules}
              columns={[
                { key: 'priority', header: 'Priority', render: (rule) => text(rule.priority) },
                { key: 'chain', header: 'Chain', render: (rule) => <strong>{text(rule.chain)}</strong> },
                { key: 'action', header: 'Action', render: (rule) => <StatusBadge status={rule.action} /> },
                { key: 'match', header: 'Match', render: (rule) => `${rule.protocol || 'any'} src=${rule.src_list_key || rule.src_cidr || 'any'} dst=${rule.dst_list_key || rule.dst_cidr || 'any'} ports=${rule.dst_ports || rule.src_ports || 'any'} state=${(rule.state_match || []).join(',') || 'any'}` },
                { key: 'comment', header: 'Comment', render: (rule) => text(rule.comment) },
                { key: 'actions', header: t('common.actions'), render: (rule) => (
                  <Toolbar>
                    <Button icon={<Pencil size={16} />} onClick={() => editRule(rule)}>Edit rule</Button>
                    <Button variant="danger" icon={<Trash2 size={16} />} onClick={() => void deleteRule.mutateAsync({ ruleId: rule.id, policyId: rule.policy_id || selectedPolicyID }).catch((error) => setNotice(formatAPIError(error)))}>Delete rule</Button>
                  </Toolbar>
                ) },
              ]}
            />
            <Card>
              <CardBody>
                <form className="page-stack" onSubmit={(event) => void submitRule(event)}>
                  <h2 className="card-title">{ruleForm.id ? 'Update rule' : 'Create rule'}</h2>
                  <FormGrid>
                    <FormField label="Priority"><TextField aria-label="Rule priority" type="number" value={ruleForm.priority} onChange={(event) => setRuleForm({ ...ruleForm, priority: event.target.value })} /></FormField>
                    <FormField label="Chain"><Select aria-label="Rule chain" value={ruleForm.chain} onChange={(event) => setRuleForm({ ...ruleForm, chain: event.target.value })}><option value="input">input</option><option value="forward">forward</option><option value="output">output</option></Select></FormField>
                    <FormField label="Action"><Select aria-label="Rule action" value={ruleForm.action} onChange={(event) => setRuleForm({ ...ruleForm, action: event.target.value })}><option value="accept">accept</option><option value="drop">drop</option><option value="reject">reject</option></Select></FormField>
                    <FormField label="Protocol"><Select aria-label="Rule protocol" value={ruleForm.protocol} onChange={(event) => setRuleForm({ ...ruleForm, protocol: event.target.value })}><option value="any">any</option><option value="tcp">tcp</option><option value="udp">udp</option><option value="icmp">icmp</option><option value="icmpv6">icmpv6</option></Select></FormField>
                    <FormField label="Source group"><Select aria-label="Source group" value={ruleForm.srcListId} onChange={(event) => setRuleForm({ ...ruleForm, srcListId: event.target.value })}><option value="">any</option>{groups.map((group) => <option key={group.id} value={group.id}>{text(group.label || group.key || group.id)}</option>)}</Select></FormField>
                    <FormField label="Destination group"><Select aria-label="Destination group" value={ruleForm.dstListId} onChange={(event) => setRuleForm({ ...ruleForm, dstListId: event.target.value })}><option value="">any</option>{groups.map((group) => <option key={group.id} value={group.id}>{text(group.label || group.key || group.id)}</option>)}</Select></FormField>
                    <FormField label="Source CIDR"><TextField aria-label="Source CIDR" value={ruleForm.srcCIDR} onChange={(event) => setRuleForm({ ...ruleForm, srcCIDR: event.target.value })} /></FormField>
                    <FormField label="Destination CIDR"><TextField aria-label="Destination CIDR" value={ruleForm.dstCIDR} onChange={(event) => setRuleForm({ ...ruleForm, dstCIDR: event.target.value })} /></FormField>
                    <FormField label="Source ports"><TextField aria-label="Source ports" value={ruleForm.srcPorts} onChange={(event) => setRuleForm({ ...ruleForm, srcPorts: event.target.value })} /></FormField>
                    <FormField label="Destination ports"><TextField aria-label="Destination ports" value={ruleForm.dstPorts} onChange={(event) => setRuleForm({ ...ruleForm, dstPorts: event.target.value })} /></FormField>
                    <FormField label="State"><TextField aria-label="State match" value={ruleForm.stateMatch} onChange={(event) => setRuleForm({ ...ruleForm, stateMatch: event.target.value })} /></FormField>
                    <FormField label="Enabled"><label className="toolbar"><input aria-label="Rule enabled" type="checkbox" checked={ruleForm.enabled} onChange={(event) => setRuleForm({ ...ruleForm, enabled: event.target.checked })} /><span>enabled</span></label></FormField>
                    <FormField label="Comment" full><Textarea aria-label="Rule comment" value={ruleForm.comment} onChange={(event) => setRuleForm({ ...ruleForm, comment: event.target.value })} /></FormField>
                  </FormGrid>
                  <Toolbar>
                    <Button variant="primary" icon={<Save size={16} />} type="submit">{ruleForm.id ? 'Update rule' : 'Create rule'}</Button>
                    {ruleForm.id ? <Button type="button" onClick={() => setRuleForm({ ...emptyRuleForm, policyId: selectedPolicyID })}>Cancel edit</Button> : null}
                  </Toolbar>
                </form>
              </CardBody>
            </Card>
          </div>
        ) : null}

        {activeTab === 'preview' ? (
          <div className="page-stack">
            <Card>
              <CardBody>
                <div className="page-stack">
                  <Toolbar>
                    <Button variant="primary" icon={<Play size={16} />} disabled={!canPreview} onClick={() => void runPreview()}>Preview</Button>
                    <Button variant="danger" icon={<ShieldCheck size={16} />} disabled={!canApply} onClick={() => setConfirmApplyOpen(true)}>Apply</Button>
                    {previewStale ? <Badge>Preview is stale</Badge> : null}
                    {blockingErrors.length ? <Badge>Blocking errors prevent Apply</Badge> : null}
                  </Toolbar>
                  <div>Target node: <strong>{targetNodeID ? nodeName(nodeList, targetNodeID) : t('common.none')}</strong></div>
                  <div>Policy: <strong>{selectedPolicy ? policyName(selectedPolicy) : t('common.none')}</strong></div>
                  <div>Preview hash: <code>{selectedPreviewHash || t('common.none')}</code></div>
                  <div>Expected action: <code>node.firewall.apply</code></div>
                  {warnings.length ? <div><strong>Safety warnings</strong><ul>{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul></div> : null}
                  {blockingErrors.length ? <div><strong>Blocking errors</strong><ul>{blockingErrors.map((error) => <li key={error}>{error}</li>)}</ul></div> : null}
                  {previewJob ? <pre className="code-block">{firewallRenderedText(previewJob)}</pre> : null}
                </div>
              </CardBody>
            </Card>
            {previewJob ? <><Link to="/operations/jobs">Open Jobs page</Link><JobStatusPanel jobID={previewJob.id} /></> : null}
            {applyJob ? <><Link to="/operations/jobs">Open Jobs page</Link><JobStatusPanel jobID={applyJob.id} /></> : null}
          </div>
        ) : null}

        {activeTab === 'node-state' ? (
          <div className="page-stack">
            <DataTable
              title="Node firewall state"
              rows={nodeStates}
              columns={[
                { key: 'node', header: 'Node', render: (state) => text(state.node_name || state.node_id) },
                { key: 'policy', header: 'Policy', render: (state) => text(state.policy_key || state.policy_id) },
                { key: 'status', header: 'Status', render: (state) => <StatusBadge status={state.status} /> },
                { key: 'revision', header: 'Revision', render: (state) => text(state.revision_id || state.desired_revision_id) },
                { key: 'job', header: 'Job', render: (state) => state.last_job_id ? <Link to="/operations/jobs">{state.last_job_id}</Link> : t('common.none') },
              ]}
            />
            <Card>
              <CardBody>
                <div className="page-stack">
                  <Toolbar>
                    <Button variant="danger" icon={<Ban size={16} />} disabled={!targetNodeID || disable.isPending} onClick={() => setConfirmDisableOpen(true)}>Emergency disable</Button>
                  </Toolbar>
                  <p>Disable removes only managed table inet megavpn_firewall and does not remove instances/backhaul/route policy.</p>
                  {disableJobID ? <><Link to="/operations/jobs">Open Jobs page</Link><JobStatusPanel jobID={disableJobID} /></> : null}
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
                  <h2 className="card-title">Safety summary</h2>
                  <Toolbar>
                    <Badge>input default: {selectedPolicy?.default_input_policy || 'accept'}</Badge>
                    <Badge>forward default: {selectedPolicy?.default_forward_policy || 'accept'}</Badge>
                    <Badge>output default: {selectedPolicy?.default_output_policy || 'accept'}</Badge>
                  </Toolbar>
                  <div>trusted_control_plane: {(safety.data?.control_plane_source_cidrs || []).length ? 'present' : 'absent'}</div>
                  <div>trusted_operators: {(safety.data?.trusted_operator_cidrs || []).length ? 'present' : 'absent'}</div>
                  <div>SSH bootstrap CIDRs: {(safety.data?.ssh_bootstrap_source_cidrs || []).length ? 'present' : 'absent'}</div>
                  <div>vpn_client_sources: {groups.some((group) => group.key === 'vpn_client_sources' && groupEntries(entries, group.id).some(isRenderableEntry)) ? 'present' : 'absent'}</div>
                  <div>backhaul_sources: {groups.some((group) => group.key === 'backhaul_sources' && groupEntries(entries, group.id).some(isRenderableEntry)) ? 'present' : 'absent'}</div>
                  <div>SSH/bootstrap safety status: backend validated during strict preview/apply.</div>
                  <div>Forward safety status: backend validates VPN/backhaul preservation for strict forward drops.</div>
                  <div>Output/control-plane egress safety status: backend validates explicit egress allow rules for strict output drops.</div>
                  {safety.isError ? <div>{formatAPIError(safety.error)}</div> : null}
                </div>
              </CardBody>
            </Card>
          </div>
        ) : null}

        <Modal title="Confirm firewall apply" open={confirmApplyOpen} onClose={() => setConfirmApplyOpen(false)}>
          <div className="page-stack">
            <p>Apply managed firewall policy to the selected node only after backend preview.</p>
            <div>Target node: <strong>{targetNodeID ? nodeName(nodeList, targetNodeID) : t('common.none')}</strong></div>
            <div>Policy: <strong>{selectedPolicy ? policyName(selectedPolicy) : t('common.none')}</strong></div>
            <div>Chains affected: input, forward, output</div>
            <div>Preview hash: <code>{selectedPreviewHash || t('common.none')}</code></div>
            {warnings.length ? <ul>{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul> : <div>No backend warnings returned.</div>}
            <p>Possible connectivity impact: strict input/forward/output policies can isolate a node if backend safety validation rejects required management, SSH or forwarding paths.</p>
            <Toolbar>
              <Button variant="danger" onClick={() => void runApply()}>Confirm Apply</Button>
              <Button onClick={() => setConfirmApplyOpen(false)}>{t('common.cancel')}</Button>
            </Toolbar>
          </div>
        </Modal>

        <Modal title="Confirm emergency disable" open={confirmDisableOpen} onClose={() => setConfirmDisableOpen(false)}>
          <div className="page-stack">
            <p>Disable removes only managed table inet megavpn_firewall and does not remove instances/backhaul/route policy.</p>
            <div>Target node: <strong>{targetNodeID ? nodeName(nodeList, targetNodeID) : t('common.none')}</strong></div>
            <Toolbar>
              <Button variant="danger" onClick={() => void runDisable()}>Confirm Disable</Button>
              <Button onClick={() => setConfirmDisableOpen(false)}>{t('common.cancel')}</Button>
            </Toolbar>
          </div>
        </Modal>
      </QueryBoundary>
    </PageScaffold>
  );
}
