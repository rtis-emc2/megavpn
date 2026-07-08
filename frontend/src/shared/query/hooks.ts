import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import {
  applyClientAccessGroupMembers,
  applyClientAccessGroupSync,
  createClientAccessGroup,
  endpoints,
  getAvailableClientsForGroup,
  getClientAccessGroupMembers,
  getClientAccessGroupScope,
  getClientAccessGroupSyncState,
  listClientAccessGroups,
  previewClientAccessGroupMembers,
  previewClientAccessGroupSync,
  removeClientAccessGroupMember,
  updateClientAccessGroup,
  updateClientAccessGroupScope,
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
  FirewallInventory,
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
