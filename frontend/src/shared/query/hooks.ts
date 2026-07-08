import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { endpoints } from '../api/endpoints';
import type {
  AddressPools,
  Artifact,
  BackhaulLink,
  Certificate,
  ClientAccessGroup,
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

export function useClientAccessGroups(options?: QueryOptions<ClientAccessGroup[]>) {
  return useQuery({ queryKey: ['client-access-groups'], queryFn: endpoints.clientAccessGroups, staleTime: stale.normal, ...options });
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
