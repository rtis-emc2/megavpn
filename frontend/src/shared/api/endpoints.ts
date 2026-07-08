import { apiRequest, apiURL, isUnauthorized, sendJSON } from './client';
import type {
  AddressPools,
  APIRecord,
  Artifact,
  AuthPayload,
  BackhaulLink,
  Certificate,
  ClientAccessGroup,
  ClientAccessGroupInput,
  ClientAccessGroupMemberQuery,
  ClientAccessGroupMembersPage,
  ClientAccessGroupMembershipRequest,
  ClientAccessGroupMembershipResult,
  ClientAccessGroupScope,
  ClientAccessGroupSyncPreview,
  ClientAccessGroupSyncState,
  ClientAccessService,
  ClientAccount,
  ClientAccessOverview,
  ClientArtifact,
  ClientArtifactBuildRequest,
  ClientArtifactBuildResult,
  ClientArtifactDeleteResult,
  ClientArtifactDownloadResult,
  ClientEmailDeliveryInput,
  ClientEmailDeliveryResult,
  ClientCreateInput,
  ClientDeleteResult,
  ClientDetail,
  ClientDeliveryHistoryItem,
  ClientShareLink,
  ClientShareLinkCreateInput,
  ClientShareLinkCreateResult,
  ClientShareLinkRevokeResult,
  ClientShareLinkRotateResult,
  ClientStatusUpdateInput,
  ClientSubscription,
  ClientSubscriptionCreateInput,
  ClientSubscriptionCreateResult,
  ClientSubscriptionRevokeResult,
  ClientSubscriptionRotateResult,
  Dashboard,
  FirewallAddressGroup,
  FirewallAddressGroupEntry,
  FirewallApplyRequest,
  FirewallApplyResult,
  FirewallDisableResult,
  FirewallInventory,
  FirewallManagementSettings,
  FirewallNodeState,
  FirewallPolicy,
  FirewallPreviewRequest,
  FirewallPreviewResult,
  FirewallRule,
  InstanceAccessGroupMaterialization,
  InstanceApplyRequest,
  InstanceApplyResult,
  InstanceDeleteResult,
  InstanceForceDeleteInput,
  InstanceLifecycleAction,
  InstanceLifecycleResult,
  InstanceRollbackRequest,
  InstanceRollbackResult,
  Job,
  NodeEntity,
  ReadyStatus,
  ServiceInstance,
  ServiceInstanceDetail,
  ServiceInstanceRevision,
  ServiceInstanceRuntimeObservation,
  ServiceInstanceRuntimeState,
  ShareLink,
  TrafficSummary,
  AvailableClientsForGroupPage,
  AvailableClientsForGroupQuery,
  VersionInfo,
} from './types';

function queryString(params: object): string {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === '' || value === false) return;
    query.set(key, String(value));
  });
  const raw = query.toString();
  return raw ? `?${raw}` : '';
}

export async function getSession(): Promise<AuthPayload | null> {
  try {
    return await apiRequest<AuthPayload>('/api/v1/auth/me');
  } catch (error) {
    if (isUnauthorized(error)) return null;
    throw error;
  }
}

export function login(loginValue: string, password: string): Promise<AuthPayload> {
  return sendJSON<AuthPayload>('/api/v1/auth/login', 'POST', { login: loginValue, password });
}

export function logout(): Promise<unknown> {
  return apiRequest('/api/v1/auth/logout', { method: 'POST' });
}

export function getInvite(token: string): Promise<Record<string, unknown>> {
  return apiRequest(`/api/v1/auth/invites/${encodeURIComponent(token)}`);
}

export function acceptInvite(token: string, password: string): Promise<unknown> {
  return sendJSON(`/api/v1/auth/invites/${encodeURIComponent(token)}/accept`, 'POST', { password });
}

export function listClients(_params: { search?: string; status?: string } = {}): Promise<ClientAccount[]> {
  return apiRequest<ClientAccount[]>('/api/v1/clients');
}

export function getClient(clientId: string): Promise<ClientDetail> {
  return apiRequest<ClientDetail>(`/api/v1/clients/${encodeURIComponent(clientId)}`);
}

export function createClient(input: ClientCreateInput): Promise<ClientDetail> {
  return sendJSON<ClientDetail>('/api/v1/clients', 'POST', input);
}

export function updateClient(_clientId: string, _input: Partial<ClientCreateInput>): Promise<never> {
  return Promise.reject(new Error('Backend has no generic client update endpoint in this release.'));
}

export function updateClientStatus(clientId: string, input: ClientStatusUpdateInput): Promise<ClientDetail> {
  const action = input.status === 'suspended' ? 'suspend' : input.status === 'active' ? 'activate' : '';
  if (!action) {
    return Promise.reject(new Error('Backend supports only activate and suspend status endpoints in this release.'));
  }
  return sendJSON<ClientDetail>(`/api/v1/clients/${encodeURIComponent(clientId)}/${action}`, 'POST', {});
}

export function deleteClient(clientId: string): Promise<ClientDeleteResult> {
  return apiRequest<ClientDeleteResult>(`/api/v1/clients/${encodeURIComponent(clientId)}`, { method: 'DELETE' });
}

export function revokeClient(clientId: string): Promise<Job> {
  return sendJSON<Job>(`/api/v1/clients/${encodeURIComponent(clientId)}/revoke`, 'POST', {});
}

export function listInstances(_params: { search?: string; status?: string; serviceCode?: string; nodeId?: string } = {}): Promise<ServiceInstance[]> {
  return apiRequest<ServiceInstance[]>('/api/v1/instances');
}

export function getInstance(instanceId: string): Promise<ServiceInstanceDetail> {
  return apiRequest<ServiceInstanceDetail>(`/api/v1/instances/${encodeURIComponent(instanceId)}`);
}

export function listInstanceRuntimeStates(): Promise<ServiceInstanceRuntimeState[]> {
  return apiRequest<ServiceInstanceRuntimeState[]>('/api/v1/instances/runtime-states');
}

export function getInstanceRuntimeState(instanceId: string): Promise<ServiceInstanceRuntimeState> {
  return apiRequest<ServiceInstanceRuntimeState>(`/api/v1/instances/${encodeURIComponent(instanceId)}/runtime-state`);
}

export function getInstanceRuntimeObservations(instanceId: string, limit = 25): Promise<ServiceInstanceRuntimeObservation[]> {
  return apiRequest<ServiceInstanceRuntimeObservation[]>(`/api/v1/instances/${encodeURIComponent(instanceId)}/runtime-observations${queryString({ limit })}`);
}

export function getInstanceRevisions(instanceId: string, limit = 20): Promise<ServiceInstanceRevision[]> {
  return apiRequest<ServiceInstanceRevision[]>(`/api/v1/instances/${encodeURIComponent(instanceId)}/revisions${queryString({ limit })}`);
}

export function getInstanceAccessGroups(instanceId: string): Promise<InstanceAccessGroupMaterialization> {
  return apiRequest<InstanceAccessGroupMaterialization>(`/api/v1/instances/${encodeURIComponent(instanceId)}/vless-groups/members`);
}

export function applyInstance(instanceId: string, _input: InstanceApplyRequest = {}): Promise<InstanceApplyResult> {
  return sendJSON<InstanceApplyResult>(`/api/v1/instances/${encodeURIComponent(instanceId)}/apply`, 'POST', {});
}

export function reapplyInstance(instanceId: string, input: InstanceApplyRequest = {}): Promise<InstanceApplyResult> {
  return applyInstance(instanceId, input);
}

export function rollbackInstance(instanceId: string, input: InstanceRollbackRequest): Promise<InstanceRollbackResult> {
  return sendJSON<InstanceRollbackResult>(`/api/v1/instances/${encodeURIComponent(instanceId)}/rollback`, 'POST', input);
}

export function runInstanceDiagnostics(instanceId: string, _input: APIRecord = {}): Promise<Job> {
  return sendJSON<Job>(`/api/v1/instances/${encodeURIComponent(instanceId)}/diagnose`, 'POST', {});
}

export function runInstanceLifecycleAction(instanceId: string, action: InstanceLifecycleAction, _input: APIRecord = {}): Promise<InstanceLifecycleResult> {
  return sendJSON<InstanceLifecycleResult>(`/api/v1/instances/${encodeURIComponent(instanceId)}/${encodeURIComponent(action)}`, 'POST', {});
}

export function deleteInstance(instanceId: string): Promise<InstanceDeleteResult> {
  return apiRequest<InstanceDeleteResult>(`/api/v1/instances/${encodeURIComponent(instanceId)}`, { method: 'DELETE' });
}

export function forceDeleteInstance(instanceId: string, input: InstanceForceDeleteInput): Promise<InstanceDeleteResult> {
  return sendJSON<InstanceDeleteResult>(`/api/v1/instances/${encodeURIComponent(instanceId)}/force-delete`, 'POST', input);
}

export function listClientAccessServices(): Promise<ClientAccessService[]> {
  return apiRequest<ClientAccessService[]>('/api/v1/client-access-services');
}

export function listClientAccessGroups(params: { serviceCode?: string } = {}): Promise<ClientAccessGroup[]> {
  return apiRequest<ClientAccessGroup[]>(`/api/v1/client-access-groups${queryString({ service_code: params.serviceCode })}`);
}

export function getClientAccessGroup(groupId: string): Promise<ClientAccessGroup> {
  return apiRequest<ClientAccessGroup>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}`);
}

export function createClientAccessGroup(input: ClientAccessGroupInput): Promise<ClientAccessGroup> {
  return sendJSON<ClientAccessGroup>('/api/v1/client-access-groups', 'POST', input);
}

export function updateClientAccessGroup(groupId: string, input: ClientAccessGroupInput): Promise<ClientAccessGroup> {
  return sendJSON<ClientAccessGroup>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}`, 'PATCH', input);
}

export function deleteOrDisableClientAccessGroup(groupId: string): Promise<ClientAccessGroup> {
  return apiRequest<ClientAccessGroup>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}`, { method: 'DELETE' });
}

export function getClientAccessGroupMembers(groupId: string, params: ClientAccessGroupMemberQuery = {}): Promise<ClientAccessGroupMembersPage> {
  return apiRequest<ClientAccessGroupMembersPage>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/members${queryString(params)}`);
}

export function getAvailableClientsForGroup(params: AvailableClientsForGroupQuery): Promise<AvailableClientsForGroupPage> {
  return apiRequest<AvailableClientsForGroupPage>(`/api/v1/client-access-groups/available-clients${queryString({
    group_id: params.group_id,
    service_code: params.service_code,
    search: params.search,
    assignment: params.assignment,
    status: params.status,
    limit: params.limit,
    offset: params.offset,
  })}`);
}

export function previewClientAccessGroupMembers(groupId: string, payload: ClientAccessGroupMembershipRequest): Promise<ClientAccessGroupMembershipResult> {
  return sendJSON<ClientAccessGroupMembershipResult>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/members:preview`, 'POST', payload);
}

export function applyClientAccessGroupMembers(groupId: string, payload: ClientAccessGroupMembershipRequest): Promise<ClientAccessGroupMembershipResult> {
  return sendJSON<ClientAccessGroupMembershipResult>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/members:bulk-apply`, 'POST', payload);
}

export function removeClientAccessGroupMember(groupId: string, clientId: string): Promise<ClientAccessGroupMembershipResult> {
  return apiRequest<ClientAccessGroupMembershipResult>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(clientId)}`, { method: 'DELETE' });
}

export async function getClientAccessOverview(clientId: string): Promise<ClientAccessOverview> {
  const [client, accesses, groups] = await Promise.all([
    getClient(clientId),
    apiRequest<ClientAccessOverview['accesses']>(`/api/v1/clients/${encodeURIComponent(clientId)}/accesses`),
    apiRequest<ClientAccessGroup[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/access-groups`),
  ]);
  const vlessGroup = groups.find((group) => group.service_code === 'vless') || null;
  return { client, accesses, groups, vless_group: vlessGroup };
}

function singleClientMembershipPayload(clientId: string, mode: 'add_only' | 'add_or_move', buildArtifacts = false): ClientAccessGroupMembershipRequest {
  return {
    client_ids: [clientId],
    mode,
    queue_apply: true,
    build_artifacts: buildArtifacts,
  };
}

export function previewSingleClientAccessGroupAssignment(groupId: string, clientId: string, mode: 'add_only' | 'add_or_move', buildArtifacts = false): Promise<ClientAccessGroupMembershipResult> {
  return previewClientAccessGroupMembers(groupId, {
    ...singleClientMembershipPayload(clientId, mode, buildArtifacts),
    dry_run: true,
  });
}

export function applySingleClientAccessGroupAssignment(groupId: string, clientId: string, mode: 'add_only' | 'add_or_move', buildArtifacts = false): Promise<ClientAccessGroupMembershipResult> {
  return applyClientAccessGroupMembers(groupId, singleClientMembershipPayload(clientId, mode, buildArtifacts));
}

export function removeClientAccessGroupMembership(groupId: string, clientId: string): Promise<ClientAccessGroupMembershipResult> {
  return removeClientAccessGroupMember(groupId, clientId);
}

export function getClientAccessGroupScope(groupId: string): Promise<ClientAccessGroupScope> {
  return apiRequest<ClientAccessGroupScope>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/scope`);
}

export function updateClientAccessGroupScope(groupId: string, payload: ClientAccessGroupScope): Promise<ClientAccessGroupScope> {
  return sendJSON<ClientAccessGroupScope>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/scope`, 'PATCH', payload);
}

export function previewClientAccessGroupSync(groupId: string): Promise<ClientAccessGroupSyncPreview> {
  return sendJSON<ClientAccessGroupSyncPreview>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/sync:preview`, 'POST', {});
}

export function applyClientAccessGroupSync(groupId: string): Promise<ClientAccessGroupMembershipResult> {
  return sendJSON<ClientAccessGroupMembershipResult>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/sync:apply`, 'POST', {});
}

export function getClientAccessGroupSyncState(groupId: string): Promise<ClientAccessGroupSyncState[]> {
  return apiRequest<ClientAccessGroupSyncState[]>(`/api/v1/client-access-groups/${encodeURIComponent(groupId)}/sync-state`);
}

export type FirewallAddressGroupInput = Partial<Pick<FirewallAddressGroup, 'key' | 'label' | 'description' | 'scope' | 'status'>>;
export type FirewallAddressGroupEntryInput = Partial<Pick<FirewallAddressGroupEntry, 'value' | 'value_type' | 'label' | 'status'>>;
export type FirewallPolicyInput = Partial<Pick<FirewallPolicy, 'key' | 'label' | 'description' | 'scope' | 'node_id' | 'default_input_policy' | 'default_forward_policy' | 'default_output_policy' | 'status'>>;
export type FirewallRuleInput = Partial<Pick<FirewallRule, 'policy_id' | 'priority' | 'chain' | 'action' | 'direction' | 'protocol' | 'src_list_id' | 'dst_list_id' | 'src_cidr' | 'dst_cidr' | 'src_ports' | 'dst_ports' | 'state_match' | 'comment' | 'enabled' | 'log' | 'status' | 'metadata'>>;

export function getFirewallInventory(): Promise<FirewallInventory> {
  return apiRequest<FirewallInventory>('/api/v1/firewall');
}

export async function listFirewallAddressGroups(): Promise<FirewallAddressGroup[]> {
  return (await getFirewallInventory()).address_lists;
}

export function createFirewallAddressGroup(input: FirewallAddressGroupInput): Promise<FirewallAddressGroup> {
  return sendJSON<FirewallAddressGroup>('/api/v1/firewall/address-lists', 'POST', input);
}

export function updateFirewallAddressGroup(id: string, input: FirewallAddressGroupInput): Promise<FirewallAddressGroup> {
  return sendJSON<FirewallAddressGroup>(`/api/v1/firewall/address-lists/${encodeURIComponent(id)}`, 'PUT', input);
}

export function deleteFirewallAddressGroup(id: string): Promise<FirewallAddressGroup> {
  return apiRequest<FirewallAddressGroup>(`/api/v1/firewall/address-lists/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function listFirewallAddressGroupEntries(groupId?: string): Promise<FirewallAddressGroupEntry[]> {
  const entries = (await getFirewallInventory()).entries;
  return groupId ? entries.filter((entry) => entry.list_id === groupId) : entries;
}

export function createFirewallAddressGroupEntry(groupId: string, input: FirewallAddressGroupEntryInput): Promise<FirewallAddressGroupEntry> {
  return sendJSON<FirewallAddressGroupEntry>(`/api/v1/firewall/address-lists/${encodeURIComponent(groupId)}/entries`, 'POST', input);
}

export function updateFirewallAddressGroupEntry(groupId: string, entryId: string, input: FirewallAddressGroupEntryInput): Promise<FirewallAddressGroupEntry> {
  return sendJSON<FirewallAddressGroupEntry>(`/api/v1/firewall/address-lists/${encodeURIComponent(groupId)}/entries/${encodeURIComponent(entryId)}`, 'PUT', input);
}

export function deleteFirewallAddressGroupEntry(groupId: string, entryId: string): Promise<FirewallAddressGroupEntry> {
  return apiRequest<FirewallAddressGroupEntry>(`/api/v1/firewall/address-lists/${encodeURIComponent(groupId)}/entries/${encodeURIComponent(entryId)}`, { method: 'DELETE' });
}

export async function listFirewallPolicies(): Promise<FirewallPolicy[]> {
  return (await getFirewallInventory()).policies;
}

export function createFirewallPolicy(input: FirewallPolicyInput): Promise<FirewallPolicy> {
  return sendJSON<FirewallPolicy>('/api/v1/firewall/policies', 'POST', input);
}

export function updateFirewallPolicy(id: string, input: FirewallPolicyInput): Promise<FirewallPolicy> {
  return sendJSON<FirewallPolicy>(`/api/v1/firewall/policies/${encodeURIComponent(id)}`, 'PUT', input);
}

export function deleteFirewallPolicy(id: string): Promise<FirewallPolicy> {
  return apiRequest<FirewallPolicy>(`/api/v1/firewall/policies/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function listFirewallRules(policyId?: string): Promise<FirewallRule[]> {
  const rules = (await getFirewallInventory()).rules;
  return policyId ? rules.filter((rule) => rule.policy_id === policyId) : rules;
}

export function createFirewallRule(policyId: string, input: FirewallRuleInput): Promise<FirewallRule> {
  return sendJSON<FirewallRule>(`/api/v1/firewall/policies/${encodeURIComponent(policyId)}/rules`, 'POST', input);
}

export function updateFirewallRule(ruleId: string, input: FirewallRuleInput): Promise<FirewallRule> {
  const policyId = String(input.policy_id || '').trim();
  if (!policyId) {
    return Promise.reject(new Error('firewall policy id is required to update a rule'));
  }
  return sendJSON<FirewallRule>(`/api/v1/firewall/policies/${encodeURIComponent(policyId)}/rules/${encodeURIComponent(ruleId)}`, 'PUT', input);
}

export function deleteFirewallRule(ruleId: string, policyId: string): Promise<FirewallRule> {
  return apiRequest<FirewallRule>(`/api/v1/firewall/policies/${encodeURIComponent(policyId)}/rules/${encodeURIComponent(ruleId)}`, { method: 'DELETE' });
}

export function reorderFirewallRules(): Promise<never> {
  return Promise.reject(new Error('firewall rule reorder is not supported by the current backend API'));
}

export async function getNodeFirewallState(nodeId: string): Promise<FirewallNodeState | null> {
  const inventory = await getFirewallInventory();
  return inventory.node_states.find((state) => state.node_id === nodeId) || null;
}

export function previewNodeFirewall(nodeId: string, policyId: string, input: FirewallPreviewRequest = {}): Promise<FirewallPreviewResult> {
  return sendJSON<FirewallPreviewResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/firewall/preview`, 'POST', {
    policy_id: policyId,
    enforce_default_policy: input.enforce_default_policy ?? true,
  });
}

export function applyNodeFirewall(nodeId: string, policyId: string, _previewTokenOrHash?: string, input: FirewallApplyRequest = {}): Promise<FirewallApplyResult> {
  return sendJSON<FirewallApplyResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/firewall/apply`, 'POST', {
    policy_id: policyId,
    enforce_default_policy: input.enforce_default_policy ?? true,
  });
}

export function disableNodeFirewall(nodeId: string): Promise<FirewallDisableResult> {
  return sendJSON<FirewallDisableResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/firewall/disable`, 'POST', {});
}

export function getFirewallSafetySettings(): Promise<FirewallManagementSettings> {
  return apiRequest<FirewallManagementSettings>('/api/v1/firewall/management-settings');
}

export function updateFirewallSafetySettings(input: FirewallManagementSettings): Promise<FirewallManagementSettings> {
  return sendJSON<FirewallManagementSettings>('/api/v1/firewall/management-settings', 'PUT', input);
}

export function listClientArtifacts(clientId: string): Promise<ClientArtifact[]> {
  return apiRequest<ClientArtifact[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/artifacts`);
}

export function buildClientArtifact(clientId: string, input: ClientArtifactBuildRequest): Promise<ClientArtifactBuildResult> {
  return sendJSON<ClientArtifactBuildResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/artifacts`, 'POST', input);
}

export function getClientArtifactDownload(clientId: string, artifactId: string): Promise<ClientArtifactDownloadResult> {
  return Promise.resolve({
    url: apiURL(`/api/v1/clients/${encodeURIComponent(clientId)}/artifacts/${encodeURIComponent(artifactId)}/download`),
    method: 'GET',
  });
}

export function deleteClientArtifact(clientId: string, artifactId: string): Promise<ClientArtifactDeleteResult> {
  return apiRequest<ClientArtifactDeleteResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/artifacts/${encodeURIComponent(artifactId)}`, { method: 'DELETE' });
}

function publicShareURL(link: ClientShareLink): string | undefined {
  return link.token ? apiURL(`/share/${encodeURIComponent(link.token)}`) : undefined;
}

function shareLinkCreateResult(link: ClientShareLink): ClientShareLinkCreateResult {
  const shareURL = publicShareURL(link);
  const safeLink = { ...link };
  delete safeLink.token;
  return {
    share_link: safeLink,
    share_url: shareURL,
    one_time_secret: shareURL ? {
      kind: 'share_link',
      label: 'Share link URL',
      value: shareURL,
      object_id: link.id,
      expires_at: link.expires_at,
    } : undefined,
  };
}

export function listClientShareLinks(clientId: string): Promise<ClientShareLink[]> {
  return apiRequest<ClientShareLink[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/share-links`);
}

export async function createClientShareLink(clientId: string, input: ClientShareLinkCreateInput): Promise<ClientShareLinkCreateResult> {
  const link = await sendJSON<ClientShareLink>(`/api/v1/clients/${encodeURIComponent(clientId)}/share-links`, 'POST', input);
  return shareLinkCreateResult(link);
}

export function revokeClientShareLink(clientId: string, shareId: string): Promise<ClientShareLinkRevokeResult> {
  return sendJSON<ClientShareLinkRevokeResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/share-links/${encodeURIComponent(shareId)}/revoke`, 'POST', {});
}

export async function rotateClientShareLink(clientId: string, shareId: string, ttlHours?: number): Promise<ClientShareLinkRotateResult> {
  const link = await sendJSON<ClientShareLink>(`/api/v1/clients/${encodeURIComponent(clientId)}/share-links/${encodeURIComponent(shareId)}/rotate`, 'POST', { ttl_hours: ttlHours || 0 });
  return { ...shareLinkCreateResult(link), rotated_from: shareId };
}

export function listClientSubscriptions(clientId: string): Promise<ClientSubscription[]> {
  return apiRequest<ClientSubscription[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/subscriptions`);
}

export async function createClientSubscription(clientId: string, input: ClientSubscriptionCreateInput = {}): Promise<ClientSubscriptionCreateResult> {
  return rotateClientSubscription(clientId, input);
}

export async function rotateClientSubscription(clientId: string, input: ClientSubscriptionCreateInput = {}): Promise<ClientSubscriptionRotateResult> {
  const result = await sendJSON<ClientSubscriptionRotateResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/subscriptions/rotate`, 'POST', { ttl_hours: input.ttl_hours || 0 });
  return {
    ...result,
    one_time_secret: result.subscription_url ? {
      kind: 'subscription',
      label: 'VLESS subscription URL',
      value: result.subscription_url,
      object_id: result.subscription?.id,
      expires_at: result.subscription?.expires_at,
    } : undefined,
  };
}

export function revokeClientSubscription(clientId: string, subscriptionId: string): Promise<ClientSubscriptionRevokeResult> {
  return sendJSON<ClientSubscriptionRevokeResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/subscriptions/${encodeURIComponent(subscriptionId)}/revoke`, 'POST', {});
}

export function sendClientArtifactEmail(clientId: string, _artifactId: string | undefined, input: ClientEmailDeliveryInput): Promise<ClientEmailDeliveryResult> {
  return sendJSON<ClientEmailDeliveryResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/deliver-email`, 'POST', input);
}

export function listClientDeliveryHistory(_clientId: string): Promise<ClientDeliveryHistoryItem[]> {
  return Promise.reject(new Error('Backend has no client delivery history list endpoint in this release.'));
}

export const endpoints = {
  ready: () => apiRequest<ReadyStatus>('/api/v1/ready'),
  version: () => apiRequest<VersionInfo>('/api/v1/version'),
  dashboard: () => apiRequest<Dashboard>('/api/v1/dashboard'),
  nodes: () => apiRequest<NodeEntity[]>('/api/v1/nodes'),
  instances: () => listInstances(),
  instanceRuntimeStates: () => listInstanceRuntimeStates(),
  clients: () => listClients(),
  client: getClient,
  clientAccessServices: listClientAccessServices,
  clientAccessGroups: () => listClientAccessGroups(),
  firewallInventory: getFirewallInventory,
  trafficAccounting: () => apiRequest<TrafficSummary>('/api/v1/traffic/accounting?limit=250'),
  jobs: () => apiRequest<Job[]>('/api/v1/jobs?limit=100'),
  job: (id: string) => apiRequest<Job>(`/api/v1/jobs/${encodeURIComponent(id)}`),
  jobLogs: (id: string) => apiRequest<Record<string, unknown>[]>(`/api/v1/jobs/${encodeURIComponent(id)}/logs?limit=50`),
  cancelJob: (id: string) => sendJSON<Job>(`/api/v1/jobs/${encodeURIComponent(id)}/cancel`, 'POST', {}),
  certificates: () => apiRequest<Certificate[]>('/api/v1/platform/certificates'),
  pkiRoots: () => apiRequest<Record<string, unknown>[]>('/api/v1/platform/pki-roots'),
  backhaulLinks: () => apiRequest<BackhaulLink[]>('/api/v1/backhaul-links'),
  backhaulDrivers: () => apiRequest<Record<string, unknown>[]>('/api/v1/backhaul/drivers'),
  artifacts: () => apiRequest<Artifact[]>('/api/v1/artifacts'),
  shareLinks: () => apiRequest<ShareLink[]>('/api/v1/share-links'),
  addressPools: () => apiRequest<AddressPools>('/api/v1/address-pools'),
  servicePacks: () => apiRequest<Record<string, unknown>[]>('/api/v1/service-packs?include_inactive=1'),
  binaryArtifacts: () => apiRequest<Record<string, unknown>[]>('/api/v1/binary-artifacts'),
  audit: () => apiRequest<Record<string, unknown>[]>('/api/v1/audit?limit=200'),
  runtimePreflight: () => apiRequest<Record<string, unknown>>('/api/v1/runtime/preflight'),
  controlPlaneTLS: () => apiRequest<Record<string, unknown>>('/api/v1/settings/control-plane-tls'),
  mailSettings: () => apiRequest<Record<string, unknown>>('/api/v1/settings/mail'),
  users: () => apiRequest<Record<string, unknown>[]>('/api/v1/admin/users'),
  sessions: () => apiRequest<Record<string, unknown>[]>('/api/v1/admin/sessions'),
};

export function trafficAccountingExportURL(limit = 10_000): string {
  return apiURL(`/api/v1/traffic/accounting/export?limit=${encodeURIComponent(String(limit))}`);
}
