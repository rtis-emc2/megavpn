import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import {
  applyClientAccessGroupMembers,
  applyClientAccessGroupSync,
  applyNodeFirewall,
  createClientAccessGroup,
  createFirewallAddressGroup,
  createFirewallAddressGroupEntry,
  createFirewallPolicy,
  createFirewallRule,
  deleteFirewallAddressGroup,
  deleteFirewallAddressGroupEntry,
  deleteFirewallPolicy,
  deleteFirewallRule,
  disableNodeFirewall,
  endpoints,
  getAvailableClientsForGroup,
  getClientAccessGroupMembers,
  getClientAccessGroupScope,
  getClientAccessGroupSyncState,
  getFirewallSafetySettings,
  getNodeFirewallState,
  listClientAccessGroups,
  previewClientAccessGroupMembers,
  previewClientAccessGroupSync,
  previewNodeFirewall,
  removeClientAccessGroupMember,
  updateClientAccessGroup,
  updateClientAccessGroupScope,
  updateFirewallAddressGroup,
  updateFirewallAddressGroupEntry,
  updateFirewallPolicy,
  updateFirewallRule,
  updateFirewallSafetySettings,
  type FirewallAddressGroupEntryInput,
  type FirewallAddressGroupInput,
  type FirewallPolicyInput,
  type FirewallRuleInput,
} from '../api/endpoints';
import type {
  AddressPools,
  Artifact,
  BackhaulLink,
  Certificate,
  ClientAccessGroup,
  ClientAccessGroupInput,
  ClientAccessGroupMemberQuery,
  ClientAccessGroupMembersPage,
  ClientAccessGroupMembershipRequest,
  ClientAccessGroupScope,
  ClientAccessGroupSyncState,
  ClientAccessService,
  ClientAccount,
  Dashboard,
  FirewallAddressGroup,
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
  Job,
  NodeEntity,
  ReadyStatus,
  ServiceInstance,
  ShareLink,
  TrafficSummary,
  AvailableClientsForGroupPage,
  AvailableClientsForGroupQuery,
  VersionInfo,
} from '../api/types';

type QueryOptions<T> = Omit<UseQueryOptions<T, Error>, 'queryKey' | 'queryFn'>;

const stale = {
  fast: 5_000,
  normal: 30_000,
  slow: 120_000,
};

export function useReady(options?: QueryOptions<ReadyStatus>) {
  return useQuery({ queryKey: ['ready'], queryFn: endpoints.ready, staleTime: stale.fast, refetchInterval: 10_000, ...options });
}

export function useVersion(options?: QueryOptions<VersionInfo>) {
  return useQuery({ queryKey: ['version'], queryFn: endpoints.version, staleTime: stale.slow, ...options });
}

export function useDashboard(options?: QueryOptions<Dashboard>) {
  return useQuery({ queryKey: ['dashboard'], queryFn: endpoints.dashboard, staleTime: stale.fast, ...options });
}

export function useNodes(options?: QueryOptions<NodeEntity[]>) {
  return useQuery({ queryKey: ['nodes'], queryFn: endpoints.nodes, staleTime: stale.normal, ...options });
}

export function useInstances(options?: QueryOptions<ServiceInstance[]>) {
  return useQuery({ queryKey: ['instances'], queryFn: endpoints.instances, staleTime: stale.normal, ...options });
}

export function useClients(options?: QueryOptions<ClientAccount[]>) {
  return useQuery({ queryKey: ['clients'], queryFn: endpoints.clients, staleTime: stale.normal, ...options });
}

export function useClientAccessServices(options?: QueryOptions<ClientAccessService[]>) {
  return useQuery({ queryKey: ['client-access-services'], queryFn: endpoints.clientAccessServices, staleTime: stale.slow, ...options });
}

export function useClientAccessGroups(serviceCode?: string, options?: QueryOptions<ClientAccessGroup[]>) {
  return useQuery({
    queryKey: ['client-access-groups', serviceCode || 'all'],
    queryFn: () => listClientAccessGroups({ serviceCode }),
    staleTime: stale.normal,
    ...options,
  });
}

function invalidateClientAccessGroupQueries(queryClient: ReturnType<typeof useQueryClient>, groupID?: string) {
  void queryClient.invalidateQueries({ queryKey: ['client-access-groups'] });
  void queryClient.invalidateQueries({ queryKey: ['clients'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  if (groupID) {
    void queryClient.invalidateQueries({ queryKey: ['client-access-group-members', groupID] });
    void queryClient.invalidateQueries({ queryKey: ['client-access-group-available-clients', groupID] });
    void queryClient.invalidateQueries({ queryKey: ['client-access-group-scope', groupID] });
    void queryClient.invalidateQueries({ queryKey: ['client-access-group-sync-state', groupID] });
  }
}

export function useCreateClientAccessGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ClientAccessGroupInput) => createClientAccessGroup(input),
    onSuccess: (group) => invalidateClientAccessGroupQueries(queryClient, group.id),
  });
}

export function useUpdateClientAccessGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, input }: { groupId: string; input: ClientAccessGroupInput }) => updateClientAccessGroup(groupId, input),
    onSuccess: (group) => invalidateClientAccessGroupQueries(queryClient, group.id),
  });
}

export function useClientAccessGroupMembers(groupId: string | undefined, params: ClientAccessGroupMemberQuery, options?: QueryOptions<ClientAccessGroupMembersPage>) {
  return useQuery({
    queryKey: ['client-access-group-members', groupId, params],
    queryFn: () => getClientAccessGroupMembers(groupId || '', params),
    enabled: Boolean(groupId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useAvailableClientsForGroup(groupId: string | undefined, params: AvailableClientsForGroupQuery, options?: QueryOptions<AvailableClientsForGroupPage>) {
  return useQuery({
    queryKey: ['client-access-group-available-clients', groupId, params],
    queryFn: () => getAvailableClientsForGroup({ ...params, group_id: groupId }),
    enabled: Boolean(groupId),
    staleTime: stale.normal,
    ...options,
  });
}

export function usePreviewClientAccessGroupMembers() {
  return useMutation({
    mutationFn: ({ groupId, payload }: { groupId: string; payload: ClientAccessGroupMembershipRequest }) => previewClientAccessGroupMembers(groupId, payload),
  });
}

export function useApplyClientAccessGroupMembers() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, payload }: { groupId: string; payload: ClientAccessGroupMembershipRequest }) => applyClientAccessGroupMembers(groupId, payload),
    onSuccess: (result, input) => invalidateClientAccessGroupQueries(queryClient, result.group_id || input.groupId),
  });
}

export function useRemoveClientAccessGroupMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, clientId }: { groupId: string; clientId: string }) => removeClientAccessGroupMember(groupId, clientId),
    onSuccess: (result, input) => invalidateClientAccessGroupQueries(queryClient, result.group_id || input.groupId),
  });
}

export function useClientAccessGroupScope(groupId: string | undefined, options?: QueryOptions<ClientAccessGroupScope>) {
  return useQuery({
    queryKey: ['client-access-group-scope', groupId],
    queryFn: () => getClientAccessGroupScope(groupId || ''),
    enabled: Boolean(groupId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useUpdateClientAccessGroupScope() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, payload }: { groupId: string; payload: ClientAccessGroupScope }) => updateClientAccessGroupScope(groupId, payload),
    onSuccess: (scope, input) => invalidateClientAccessGroupQueries(queryClient, scope.group_id || input.groupId),
  });
}

export function usePreviewClientAccessGroupSync() {
  return useMutation({
    mutationFn: (groupId: string) => previewClientAccessGroupSync(groupId),
  });
}

export function useApplyClientAccessGroupSync() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (groupId: string) => applyClientAccessGroupSync(groupId),
    onSuccess: (result, groupId) => invalidateClientAccessGroupQueries(queryClient, result.group_id || groupId),
  });
}

export function useClientAccessGroupSyncState(groupId: string | undefined, options?: QueryOptions<ClientAccessGroupSyncState[]>) {
  return useQuery({
    queryKey: ['client-access-group-sync-state', groupId],
    queryFn: () => getClientAccessGroupSyncState(groupId || ''),
    enabled: Boolean(groupId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useFirewallInventory(options?: QueryOptions<FirewallInventory>) {
  return useQuery({ queryKey: ['firewall-inventory'], queryFn: endpoints.firewallInventory, staleTime: stale.normal, ...options });
}

function invalidateFirewallQueries(queryClient: ReturnType<typeof useQueryClient>, nodeID?: string) {
  void queryClient.invalidateQueries({ queryKey: ['firewall-inventory'] });
  void queryClient.invalidateQueries({ queryKey: ['firewall-address-groups'] });
  void queryClient.invalidateQueries({ queryKey: ['firewall-policies'] });
  void queryClient.invalidateQueries({ queryKey: ['firewall-rules'] });
  void queryClient.invalidateQueries({ queryKey: ['firewall-safety-settings'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  if (nodeID) {
    void queryClient.invalidateQueries({ queryKey: ['node-firewall-state', nodeID] });
  }
}

export function useFirewallAddressGroups(options?: QueryOptions<FirewallAddressGroup[]>) {
  return useQuery({
    queryKey: ['firewall-address-groups'],
    queryFn: async () => (await endpoints.firewallInventory()).address_lists,
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateFirewallAddressGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: FirewallAddressGroupInput) => createFirewallAddressGroup(input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useUpdateFirewallAddressGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: FirewallAddressGroupInput }) => updateFirewallAddressGroup(id, input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useDeleteFirewallAddressGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteFirewallAddressGroup(id),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useCreateFirewallAddressGroupEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, input }: { groupId: string; input: FirewallAddressGroupEntryInput }) => createFirewallAddressGroupEntry(groupId, input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useUpdateFirewallAddressGroupEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, entryId, input }: { groupId: string; entryId: string; input: FirewallAddressGroupEntryInput }) => updateFirewallAddressGroupEntry(groupId, entryId, input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useDeleteFirewallAddressGroupEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, entryId }: { groupId: string; entryId: string }) => deleteFirewallAddressGroupEntry(groupId, entryId),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useFirewallPolicies(options?: QueryOptions<FirewallPolicy[]>) {
  return useQuery({
    queryKey: ['firewall-policies'],
    queryFn: async () => (await endpoints.firewallInventory()).policies,
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateFirewallPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: FirewallPolicyInput) => createFirewallPolicy(input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useUpdateFirewallPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: FirewallPolicyInput }) => updateFirewallPolicy(id, input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useDeleteFirewallPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteFirewallPolicy(id),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useFirewallRules(policyId: string | undefined, options?: QueryOptions<FirewallRule[]>) {
  return useQuery({
    queryKey: ['firewall-rules', policyId || 'all'],
    queryFn: async () => {
      const rules = (await endpoints.firewallInventory()).rules;
      return policyId ? rules.filter((rule) => rule.policy_id === policyId) : rules;
    },
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateFirewallRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ policyId, input }: { policyId: string; input: FirewallRuleInput }) => createFirewallRule(policyId, input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useUpdateFirewallRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ ruleId, input }: { ruleId: string; input: FirewallRuleInput }) => updateFirewallRule(ruleId, input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useDeleteFirewallRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ ruleId, policyId }: { ruleId: string; policyId: string }) => deleteFirewallRule(ruleId, policyId),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useNodeFirewallState(nodeId: string | undefined, options?: QueryOptions<FirewallNodeState | null>) {
  return useQuery({
    queryKey: ['node-firewall-state', nodeId],
    queryFn: () => getNodeFirewallState(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    ...options,
  });
}

export function usePreviewNodeFirewall() {
  const queryClient = useQueryClient();
  return useMutation<FirewallPreviewResult, Error, { nodeId: string; policyId: string; input?: FirewallPreviewRequest }>({
    mutationFn: ({ nodeId, policyId, input }) => previewNodeFirewall(nodeId, policyId, input),
    onSuccess: (_job, input) => invalidateFirewallQueries(queryClient, input.nodeId),
  });
}

export function useApplyNodeFirewall() {
  const queryClient = useQueryClient();
  return useMutation<FirewallApplyResult, Error, { nodeId: string; policyId: string; previewHash?: string; input?: FirewallApplyRequest }>({
    mutationFn: ({ nodeId, policyId, previewHash, input }) => applyNodeFirewall(nodeId, policyId, previewHash, input),
    onSuccess: (job, input) => {
      invalidateFirewallQueries(queryClient, input.nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useDisableNodeFirewall() {
  const queryClient = useQueryClient();
  return useMutation<FirewallDisableResult, Error, string>({
    mutationFn: (nodeId: string) => disableNodeFirewall(nodeId),
    onSuccess: (job, nodeId) => {
      invalidateFirewallQueries(queryClient, nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useFirewallSafetySettings(options?: QueryOptions<FirewallManagementSettings>) {
  return useQuery({ queryKey: ['firewall-safety-settings'], queryFn: getFirewallSafetySettings, staleTime: stale.normal, ...options });
}

export function useUpdateFirewallSafetySettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: FirewallManagementSettings) => updateFirewallSafetySettings(input),
    onSuccess: () => invalidateFirewallQueries(queryClient),
  });
}

export function useTrafficAccounting(options?: QueryOptions<TrafficSummary>) {
  return useQuery({ queryKey: ['traffic-accounting'], queryFn: endpoints.trafficAccounting, staleTime: stale.normal, ...options });
}

export function useJobs(options?: QueryOptions<Job[]>) {
  return useQuery({ queryKey: ['jobs'], queryFn: endpoints.jobs, staleTime: stale.fast, refetchInterval: 10_000, ...options });
}

export function useCancelJob() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: endpoints.cancelJob,
    onSuccess: (job) => {
      void queryClient.invalidateQueries({ queryKey: ['jobs'] });
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useCertificates(options?: QueryOptions<Certificate[]>) {
  return useQuery({ queryKey: ['certificates'], queryFn: endpoints.certificates, staleTime: stale.normal, ...options });
}

export function useBackhaulLinks(options?: QueryOptions<BackhaulLink[]>) {
  return useQuery({ queryKey: ['backhaul-links'], queryFn: endpoints.backhaulLinks, staleTime: stale.normal, ...options });
}

export function useArtifacts(options?: QueryOptions<Artifact[]>) {
  return useQuery({ queryKey: ['artifacts'], queryFn: endpoints.artifacts, staleTime: stale.normal, ...options });
}

export function useShareLinks(options?: QueryOptions<ShareLink[]>) {
  return useQuery({ queryKey: ['share-links'], queryFn: endpoints.shareLinks, staleTime: stale.normal, ...options });
}

export function useAddressPools(options?: QueryOptions<AddressPools>) {
  return useQuery({ queryKey: ['address-pools'], queryFn: endpoints.addressPools, staleTime: stale.normal, ...options });
}
