import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import {
  applyClientAccessGroupMembers,
  applyClientAccessGroupSync,
  applyBackhaulLink,
  applyInstance,
  applyRoutePolicy,
  applyTlsSettings,
  applySingleClientAccessGroupAssignment,
  acceptNodeHostKey,
  bootstrapNode,
  buildClientArtifact,
  cleanupClientConfigs,
  cleanupRoutePolicy,
  createClient,
  createClientRoute,
  applyNodeFirewall,
  createClientShareLink,
  createClientSubscription,
  createClientAccessGroup,
  createInvite,
  createManagedCertificateAuthority,
  createNode,
  createPkiRoot,
  createSelfSignedCertificate,
  createInstanceFromServicePack,
  createInstanceManual,
  createEnrollmentToken,
  createServicePack,
  createFirewallAddressGroup,
  createFirewallAddressGroupEntry,
  createFirewallPolicy,
  createFirewallRule,
  deleteFirewallAddressGroup,
  deleteFirewallAddressGroupEntry,
  deleteFirewallPolicy,
  deleteFirewallRule,
  deleteClientAccess,
  deleteClient,
  deleteClientArtifact,
  deleteClientRoute,
  deleteCertificate,
  deleteInstance,
  deleteRuntimeArtifact,
  deleteServicePack,
  discoverNodeServices,
  disableNodeFirewall,
  endpoints,
  forceDeleteInstance,
  forceRetireNode,
  getAvailableClientsForGroup,
  getBackhaulLink,
  getClient,
  getClientAccessGroupMembers,
  getClientAccessGroupScope,
  getClientAccessGroupSyncState,
  getInstance,
  getInstanceAccessGroups,
  getInstanceRevisions,
  getInstanceRuntimeObservations,
  getInstanceRuntimeState,
  getRuntimeArtifact,
  getServicePack,
  getClientAccessOverview,
  getCertificate,
  getMailSettings,
  getPlatformSettings,
  getFirewallSafetySettings,
  getRoutePolicy,
  getUser,
  getNodeFirewallState,
  getNode,
  getNodeAgentState,
  getNodeCapabilitiesDrift,
  getNodeInventory,
  getNodeServiceDiscoverySummary,
  listClientArtifacts,
  listClientAccesses,
  listClientDeliveryHistory,
  listClientRoutes,
  listClientShareLinks,
  listClientSubscriptions,
  listClientAccessGroups,
  listEnrollmentTokens,
  listUsers,
  listInvites,
  listNodeAccessMethods,
  listNodeBootstrapRuns,
  listNodeCapabilities,
  listNodeCapabilityInstallEvents,
  listNodeDiagnostics,
  listNodeServiceDiscoveries,
  listRuntimeArtifacts,
  listRuntimeTargets,
  listRoutePolicies,
  listSessions,
  listServiceInstallers,
  listServicePacks,
  listServiceTypeCapabilities,
  launchNodeSshSession,
  importAllNodeServiceDiscoveries,
  importNodeServiceDiscoveryById,
  installNodeCapability,
  previewSingleClientAccessGroupAssignment,
  previewClientAccessGroupMembers,
  previewClientAccessGroupSync,
  previewNodeFirewall,
  previewRoutePolicy,
  probeBackhaulLink,
  promoteBackhaulLink,
  reapplyInstance,
  reinstallOrUpdateNodeAgent,
  replaceInstanceSpec,
  removeClientAccessGroupMember,
  removeClientAccessGroupMembership,
  retireNode,
  revokeEnrollmentToken,
  revokeClientShareLink,
  revokeClientSubscription,
  revokeClientAccess,
  revokeClient,
  revokeInvite,
  revokeSession,
  rollbackInstance,
  rotateEnrollmentToken,
  rotateNodeAgentToken,
  runInstanceDiagnostics,
  runInstanceLifecycleAction,
  retryNodeDiagnostics,
  runNodeDiagnostics,
  runNodeDiagnosticsAction,
  scanNodeHostKey,
  importRuntimeArtifact,
  importCertificate,
  previewCertificateImport,
  rotateClientAccess,
  rotateClientShareLink,
  rotateClientSubscription,
  sendClientArtifactEmail,
  issueCertificate,
  revokeCertificate,
  setNodeMaintenance,
  setDefaultCertificate,
  updateClientStatus,
  updateClientAccessGroup,
  updateClientAccessGroupScope,
  syncNodeInventory,
  updateServicePack,
  setServicePackEnabled,
  validateInstanceSpec,
  validateServicePack,
  verifyNodeCapability,
  saveInstanceDraft,
  updateFirewallAddressGroup,
  updateFirewallAddressGroupEntry,
  updateFirewallPolicy,
  updateFirewallRule,
  updateFirewallSafetySettings,
  updateBackhaulRouteState,
  updateClient,
  updateMailSettings,
  updateNode,
  updatePlatformSettings,
  updateClientRoute,
  testMailSettings,
  type FirewallAddressGroupEntryInput,
  type FirewallAddressGroupInput,
  type FirewallPolicyInput,
  type FirewallRuleInput,
} from '../api/endpoints';
import type {
  AddressPools,
  Artifact,
  BackhaulActionResult,
  BackhaulLink,
  BackhaulRouteStateInput,
  Certificate,
  CertificateActionResult,
  CertificateAuthorityCreateInput,
  CertificateCreateInput,
  CertificateDeleteResult,
  CertificateDetail,
  CertificateImportInput,
  CertificateImportPreview,
  CertificateIssueInput,
  CertificateRevokeResult,
  ClientAccess,
  ClientAccessDeleteResult,
  ClientAccessGroup,
  ClientAccessGroupInput,
  ClientAccessGroupMembershipResult,
  ClientAccessGroupMemberQuery,
  ClientAccessGroupMembersPage,
  ClientAccessGroupMembershipRequest,
  ClientAccessOverview,
  ClientAccessRevokeResult,
  ClientAccessRotationInput,
  ClientAccessRotationResult,
  ClientAccessGroupScope,
  ClientAccessGroupSyncState,
  ClientAccessService,
  ClientAccount,
  ClientArtifact,
  ClientArtifactBuildRequest,
  ClientArtifactBuildResult,
  ClientArtifactDeleteResult,
  ClientEmailDeliveryInput,
  ClientEmailDeliveryResult,
  ClientCreateInput,
  ClientDeleteResult,
  ClientDetail,
  ClientDeliveryHistoryItem,
  ClientRoute,
  ClientRouteInput,
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
  ClientUpdateInput,
  ClientConfigCleanupResult,
  Dashboard,
  BootstrapRequest,
  BootstrapResult,
  EnrollmentToken,
  EnrollmentTokenCreateInput,
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
  InstanceAccessGroupMaterialization,
  InstanceApplyRequest,
  InstanceApplyResult,
  InstanceCreateFromPackInput,
  InstanceCreateResult,
  InstanceDeleteResult,
  InstanceForceDeleteInput,
  InstanceLifecycleAction,
  InstanceLifecycleResult,
  InstanceManualCreateInput,
  InstanceRollbackRequest,
  InstanceRollbackResult,
  InstanceSpecDraft,
  InstanceSpecValidationResult,
  Job,
  Invite,
  InviteCreateInput,
  InviteCreateResult,
  InviteRevokeResult,
  MailSettings,
  MailSettingsInput,
  MailTestInput,
  MailTestResult,
  HostKeyDecisionResult,
  HostKeyScanResult,
  NodeAgentState,
  NodeAccessMethod,
  NodeBootstrapRun,
  NodeCapability,
  NodeCapabilityDrift,
  NodeCapabilityInstallEvent,
  NodeCapabilityInstallInput,
  NodeCapabilityVerifyInput,
  NodeCreateInput,
  NodeDetail,
  NodeDiagnostics,
  NodeDiagnosticsAction,
  NodeDiagnosticResult,
  NodeEntity,
  NodeInventorySnapshot,
  NodeJobEnvelope,
  NodeRuntimeReconcileResult,
  NodeServiceDiscovery,
  NodeServiceDiscoverySummary,
  NodeServiceInstaller,
  NodeRetireResult,
  NodeUpdateInput,
  PkiRoot,
  PkiRootCreateInput,
  PlatformSettings,
  PlatformSettingsInput,
  ReadyStatus,
  RoutePolicy,
  RoutePolicyApplyResult,
  RoutePolicyCleanupResult,
  RoutePolicyPreviewResult,
  RuntimeArtifact,
  RuntimeArtifactDeleteResult,
  RuntimeArtifactImportInput,
  RuntimeArtifactImportResult,
  ServiceInstance,
  ServiceInstanceDetail,
  ServiceInstanceRevision,
  ServiceInstanceRuntimeObservation,
  ServiceInstanceRuntimeState,
  ServicePack,
  ServicePackCreateInput,
  ServicePackDetail,
  ServicePackUpdateInput,
  ServicePackValidationResult,
  ServiceTypeCapability,
  RuntimeTargetNode,
  ShareLink,
  Session,
  SessionRevokeResult,
  SshSessionLaunchResult,
  SettingsUpdateResult,
  TrafficSummary,
  UserAccount,
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

export function useRuntimePreflight(options?: QueryOptions<Record<string, unknown>>) {
  return useQuery({ queryKey: ['runtime-preflight'], queryFn: endpoints.runtimePreflight, staleTime: stale.normal, ...options });
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

function invalidateNodeQueries(queryClient: ReturnType<typeof useQueryClient>, nodeId?: string) {
  void queryClient.invalidateQueries({ queryKey: ['nodes'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  void queryClient.invalidateQueries({ queryKey: ['dashboard'] });
  if (nodeId) {
    void queryClient.invalidateQueries({ queryKey: ['node', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-diagnostics', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-inventory', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-capabilities', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-capability-drift', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-capability-install-events', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-service-discoveries', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-service-discovery-summary', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-access-methods', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-bootstrap-runs', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node-enrollment-tokens', nodeId] });
  }
}

function invalidateJobsFromResult(queryClient: ReturnType<typeof useQueryClient>, result: unknown) {
  if (!result || typeof result !== 'object') return;
  const candidate = result as { id?: unknown; type?: unknown; status?: unknown; job?: unknown; jobs?: unknown };
  if (typeof candidate.id === 'string' && typeof candidate.type === 'string' && typeof candidate.status === 'string') {
    void queryClient.invalidateQueries({ queryKey: ['job', candidate.id] });
  }
  if (candidate.job) {
    invalidateJobsFromResult(queryClient, candidate.job);
  }
  if (Array.isArray(candidate.jobs)) {
    candidate.jobs.forEach((job) => invalidateJobsFromResult(queryClient, job));
  }
}

export function useNodeDetail(nodeId: string | undefined, options?: QueryOptions<NodeDetail>) {
  return useQuery({
    queryKey: ['node', nodeId],
    queryFn: () => getNode(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateNode() {
  const queryClient = useQueryClient();
  return useMutation<NodeDetail, Error, NodeCreateInput>({
    mutationFn: createNode,
    onSuccess: (node) => invalidateNodeQueries(queryClient, node.id),
  });
}

export function useUpdateNode() {
  const queryClient = useQueryClient();
  return useMutation<NodeDetail, Error, { nodeId: string; input: NodeUpdateInput }>({
    mutationFn: ({ nodeId, input }) => updateNode(nodeId, input),
    onSuccess: (node, input) => invalidateNodeQueries(queryClient, node.id || input.nodeId),
  });
}

export function useNodeDiagnostics(nodeId: string | undefined, options?: QueryOptions<NodeDiagnostics>) {
  return useQuery({
    queryKey: ['node-diagnostics', nodeId],
    queryFn: () => listNodeDiagnostics(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.fast,
    refetchInterval: 15_000,
    ...options,
  });
}

export function useNodeAgentState(nodeId: string | undefined, options?: QueryOptions<NodeAgentState>) {
  return useQuery({
    queryKey: ['node-agent-state', nodeId],
    queryFn: () => getNodeAgentState(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.fast,
    refetchInterval: 15_000,
    ...options,
  });
}

export function useNodeInventory(nodeId: string | undefined, options?: QueryOptions<NodeInventorySnapshot>) {
  return useQuery({
    queryKey: ['node-inventory', nodeId],
    queryFn: () => getNodeInventory(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeCapabilities(nodeId: string | undefined, options?: QueryOptions<NodeCapability[]>) {
  return useQuery({
    queryKey: ['node-capabilities', nodeId],
    queryFn: () => listNodeCapabilities(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useNodeCapabilityDrift(nodeId: string | undefined, options?: QueryOptions<NodeCapabilityDrift>) {
  return useQuery({
    queryKey: ['node-capability-drift', nodeId],
    queryFn: () => getNodeCapabilitiesDrift(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeCapabilityInstallEvents(nodeId: string | undefined, options?: QueryOptions<NodeCapabilityInstallEvent[]>) {
  return useQuery({
    queryKey: ['node-capability-install-events', nodeId],
    queryFn: () => listNodeCapabilityInstallEvents(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeServiceInstallers(options?: QueryOptions<NodeServiceInstaller[]>) {
  return useQuery({ queryKey: ['node-service-installers'], queryFn: listServiceInstallers, staleTime: stale.slow, retry: false, ...options });
}

export function useNodeServiceDiscoveries(nodeId: string | undefined, options?: QueryOptions<NodeServiceDiscovery[]>) {
  return useQuery({
    queryKey: ['node-service-discoveries', nodeId],
    queryFn: () => listNodeServiceDiscoveries(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeServiceDiscovery(nodeId: string | undefined, options?: QueryOptions<NodeServiceDiscovery[]>) {
  return useNodeServiceDiscoveries(nodeId, options);
}

export function useNodeServiceDiscoverySummary(nodeId: string | undefined, options?: QueryOptions<NodeServiceDiscoverySummary>) {
  return useQuery({
    queryKey: ['node-service-discovery-summary', nodeId],
    queryFn: () => getNodeServiceDiscoverySummary(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeEnrollmentTokens(nodeId: string | undefined, options?: QueryOptions<EnrollmentToken[]>) {
  return useQuery({
    queryKey: ['node-enrollment-tokens', nodeId],
    queryFn: () => listEnrollmentTokens(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeAccessMethods(nodeId: string | undefined, options?: QueryOptions<NodeAccessMethod[]>) {
  return useQuery({
    queryKey: ['node-access-methods', nodeId],
    queryFn: () => listNodeAccessMethods(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useNodeBootstrapRuns(nodeId: string | undefined, options?: QueryOptions<NodeBootstrapRun[]>) {
  return useQuery({
    queryKey: ['node-bootstrap-runs', nodeId],
    queryFn: () => listNodeBootstrapRuns(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useCreateEnrollmentToken() {
  const queryClient = useQueryClient();
  return useMutation<EnrollmentToken, Error, { nodeId: string; input?: EnrollmentTokenCreateInput }>({
    mutationFn: ({ nodeId, input }) => createEnrollmentToken(nodeId, input || {}),
    onSuccess: (_token, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useRotateEnrollmentToken() {
  const queryClient = useQueryClient();
  return useMutation<EnrollmentToken, Error, { nodeId: string; input?: EnrollmentTokenCreateInput }>({
    mutationFn: ({ nodeId, input }) => rotateEnrollmentToken(nodeId, input || {}),
    onSuccess: (_token, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useRevokeEnrollmentToken() {
  const queryClient = useQueryClient();
  return useMutation<EnrollmentToken, Error, { nodeId: string; tokenId: string }>({
    mutationFn: ({ nodeId, tokenId }) => revokeEnrollmentToken(nodeId, tokenId),
    onSuccess: (_token, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useBootstrapNode() {
  const queryClient = useQueryClient();
  return useMutation<BootstrapResult, Error, { nodeId: string; input: BootstrapRequest }>({
    mutationFn: ({ nodeId, input }) => bootstrapNode(nodeId, input),
    onSuccess: (result, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useReinstallOrUpdateNodeAgent() {
  const queryClient = useQueryClient();
  return useMutation<BootstrapResult, Error, { nodeId: string; input?: BootstrapRequest }>({
    mutationFn: ({ nodeId, input }) => reinstallOrUpdateNodeAgent(nodeId, input || {}),
    onSuccess: (result, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useScanNodeHostKey() {
  const queryClient = useQueryClient();
  return useMutation<HostKeyScanResult, Error, { nodeId: string; input?: { ssh_host?: string; ssh_port?: number } }>({
    mutationFn: ({ nodeId, input }) => scanNodeHostKey(nodeId, input || {}),
    onSuccess: (_result, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useAcceptNodeHostKey() {
  const queryClient = useQueryClient();
  return useMutation<HostKeyDecisionResult, Error, { nodeId: string; methodId: string; fingerprint: string }>({
    mutationFn: ({ nodeId, methodId, fingerprint }) => acceptNodeHostKey(nodeId, { method_id: methodId, fingerprint }),
    onSuccess: (_result, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useRotateNodeAgentToken() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, { nodeId: string }>({
    mutationFn: ({ nodeId }) => rotateNodeAgentToken(nodeId),
    onSuccess: (job, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useLaunchNodeSshSession() {
  const queryClient = useQueryClient();
  return useMutation<SshSessionLaunchResult, Error, { nodeId: string }>({
    mutationFn: ({ nodeId }) => launchNodeSshSession(nodeId),
    onSuccess: (_session, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useRetireNode() {
  const queryClient = useQueryClient();
  return useMutation<NodeRetireResult, Error, { nodeId: string }>({
    mutationFn: ({ nodeId }) => retireNode(nodeId),
    onSuccess: (_result, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useForceRetireNode() {
  const queryClient = useQueryClient();
  return useMutation<NodeRetireResult, Error, { nodeId: string; confirmation: string; reason?: string }>({
    mutationFn: ({ nodeId, confirmation, reason }) => forceRetireNode(nodeId, { confirmation, reason }),
    onSuccess: (_result, input) => invalidateNodeQueries(queryClient, input.nodeId),
  });
}

export function useSetNodeMaintenance() {
  const queryClient = useQueryClient();
  return useMutation<NodeDetail, Error, { nodeId: string; enabled: boolean }>({
    mutationFn: ({ nodeId, enabled }) => setNodeMaintenance(nodeId, enabled),
    onSuccess: (node, input) => invalidateNodeQueries(queryClient, node.id || input.nodeId),
  });
}

export function useSyncNodeInventory() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, { nodeId: string }>({
    mutationFn: ({ nodeId }) => syncNodeInventory(nodeId),
    onSuccess: (job, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useInstallNodeCapability() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, { nodeId: string; input: NodeCapabilityInstallInput }>({
    mutationFn: ({ nodeId, input }) => installNodeCapability(nodeId, input),
    onSuccess: (job, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useInstallNodeCapabilities() {
  return useInstallNodeCapability();
}

export function useVerifyNodeCapability() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, { nodeId: string; input: NodeCapabilityVerifyInput }>({
    mutationFn: ({ nodeId, input }) => verifyNodeCapability(nodeId, input),
    onSuccess: (job, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useVerifyNodeCapabilities() {
  return useVerifyNodeCapability();
}

export function useRunNodeDiagnosticsAction() {
  const queryClient = useQueryClient();
  return useMutation<NodeJobEnvelope | NodeRuntimeReconcileResult, Error, { nodeId: string; action: NodeDiagnosticsAction }>({
    mutationFn: ({ nodeId, action }) => runNodeDiagnosticsAction(nodeId, action),
    onSuccess: (result, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      const jobs = 'jobs' in result && Array.isArray(result.jobs) ? result.jobs : 'job' in result && result.job ? [result.job] : [];
      jobs.forEach((job) => void queryClient.invalidateQueries({ queryKey: ['job', job.id] }));
    },
  });
}

export function useRunNodeDiagnostics() {
  const queryClient = useQueryClient();
  return useMutation<NodeDiagnosticResult, Error, { nodeId: string; action: NodeDiagnosticsAction }>({
    mutationFn: ({ nodeId, action }) => runNodeDiagnostics(nodeId, { action }),
    onSuccess: (result, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      const jobs = 'jobs' in result && Array.isArray(result.jobs) ? result.jobs : 'job' in result && result.job ? [result.job] : [];
      jobs.forEach((job) => void queryClient.invalidateQueries({ queryKey: ['job', job.id] }));
    },
  });
}

export function useRetryNodeDiagnostics() {
  const queryClient = useQueryClient();
  return useMutation<NodeDiagnosticResult, Error, { nodeId: string; diagnosticId: string }>({
    mutationFn: ({ nodeId, diagnosticId }) => retryNodeDiagnostics(nodeId, diagnosticId),
    onSuccess: (result, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      const jobs = 'jobs' in result && Array.isArray(result.jobs) ? result.jobs : 'job' in result && result.job ? [result.job] : [];
      jobs.forEach((job) => void queryClient.invalidateQueries({ queryKey: ['job', job.id] }));
    },
  });
}

export function useDiscoverNodeServices() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, { nodeId: string }>({
    mutationFn: ({ nodeId }) => discoverNodeServices(nodeId),
    onSuccess: (job, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useImportNodeServiceDiscovery() {
  const queryClient = useQueryClient();
  return useMutation<ServiceInstance, Error, { nodeId: string; discoveryId: string }>({
    mutationFn: ({ nodeId, discoveryId }) => importNodeServiceDiscoveryById(nodeId, discoveryId),
    onSuccess: (_instance, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      invalidateServicesWorkspace(queryClient);
    },
  });
}

export function useImportAllNodeServiceDiscoveries() {
  const queryClient = useQueryClient();
  return useMutation<ServiceInstance[], Error, { nodeId: string }>({
    mutationFn: ({ nodeId }) => importAllNodeServiceDiscoveries(nodeId),
    onSuccess: (_instances, input) => {
      invalidateNodeQueries(queryClient, input.nodeId);
      invalidateServicesWorkspace(queryClient);
    },
  });
}

export function useInstances(options?: QueryOptions<ServiceInstance[]>) {
  return useQuery({ queryKey: ['instances'], queryFn: endpoints.instances, staleTime: stale.normal, ...options });
}

function invalidateServicesWorkspace(queryClient: ReturnType<typeof useQueryClient>, instanceId?: string) {
  void queryClient.invalidateQueries({ queryKey: ['instances'] });
  void queryClient.invalidateQueries({ queryKey: ['instance-runtime-states'] });
  void queryClient.invalidateQueries({ queryKey: ['service-packs'] });
  void queryClient.invalidateQueries({ queryKey: ['runtime-artifacts'] });
  void queryClient.invalidateQueries({ queryKey: ['binary-artifacts'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  if (instanceId) {
    void queryClient.invalidateQueries({ queryKey: ['instance', instanceId] });
    void queryClient.invalidateQueries({ queryKey: ['instance-revisions', instanceId] });
    void queryClient.invalidateQueries({ queryKey: ['instance-runtime-state', instanceId] });
  }
}

export function useServiceTypeCapabilities(options?: QueryOptions<ServiceTypeCapability[]>) {
  return useQuery({ queryKey: ['service-type-capabilities'], queryFn: listServiceTypeCapabilities, staleTime: stale.slow, ...options });
}

export function useRuntimeTargets(options?: QueryOptions<RuntimeTargetNode[]>) {
  return useQuery({ queryKey: ['runtime-targets'], queryFn: () => listRuntimeTargets(), staleTime: stale.normal, ...options });
}

export function useServicePacks(options?: QueryOptions<ServicePack[]>) {
  return useQuery({ queryKey: ['service-packs'], queryFn: () => listServicePacks(), staleTime: stale.slow, ...options });
}

export function useServicePackDetail(packId: string | undefined, options?: QueryOptions<ServicePackDetail>) {
  return useQuery({
    queryKey: ['service-pack', packId],
    queryFn: () => getServicePack(packId || ''),
    enabled: Boolean(packId),
    staleTime: stale.slow,
    ...options,
  });
}

export function useCreateServicePack() {
  const queryClient = useQueryClient();
  return useMutation<ServicePackDetail, Error, ServicePackCreateInput>({
    mutationFn: createServicePack,
    onSuccess: () => invalidateServicesWorkspace(queryClient),
  });
}

export function useUpdateServicePack() {
  const queryClient = useQueryClient();
  return useMutation<ServicePackDetail, Error, { packId: string; input: ServicePackUpdateInput }>({
    mutationFn: ({ packId, input }) => updateServicePack(packId, input),
    onSuccess: (pack) => {
      invalidateServicesWorkspace(queryClient);
      void queryClient.invalidateQueries({ queryKey: ['service-pack', pack.key] });
    },
  });
}

export function useDeleteServicePack() {
  const queryClient = useQueryClient();
  return useMutation<ServicePackDetail, Error, string>({
    mutationFn: deleteServicePack,
    onSuccess: (pack) => {
      invalidateServicesWorkspace(queryClient);
      void queryClient.invalidateQueries({ queryKey: ['service-pack', pack.key] });
    },
  });
}

export function useSetServicePackEnabled() {
  const queryClient = useQueryClient();
  return useMutation<ServicePackDetail, Error, { packId: string; enabled: boolean }>({
    mutationFn: ({ packId, enabled }) => setServicePackEnabled(packId, enabled),
    onSuccess: (pack) => {
      invalidateServicesWorkspace(queryClient);
      void queryClient.invalidateQueries({ queryKey: ['service-pack', pack.key] });
    },
  });
}

export function useValidateServicePack() {
  return useMutation<ServicePackValidationResult, Error, ServicePackCreateInput>({
    mutationFn: validateServicePack,
  });
}

export function useCreateInstanceFromServicePack() {
  const queryClient = useQueryClient();
  return useMutation<InstanceCreateResult, Error, { packId: string; input: InstanceCreateFromPackInput }>({
    mutationFn: ({ packId, input }) => createInstanceFromServicePack(packId, input),
    onSuccess: () => invalidateServicesWorkspace(queryClient),
  });
}

export function useCreateInstanceManual() {
  const queryClient = useQueryClient();
  return useMutation<InstanceCreateResult, Error, InstanceManualCreateInput>({
    mutationFn: createInstanceManual,
    onSuccess: (result) => invalidateServicesWorkspace(queryClient, 'id' in result && typeof result.id === 'string' ? result.id : undefined),
  });
}

export function useValidateInstanceSpec() {
  return useMutation<InstanceSpecValidationResult, Error, InstanceSpecDraft>({
    mutationFn: validateInstanceSpec,
  });
}

export function useSaveInstanceDraft() {
  return useMutation<InstanceSpecValidationResult, Error, { instanceId: string; input: InstanceSpecDraft }>({
    mutationFn: ({ instanceId, input }) => saveInstanceDraft(instanceId, input),
  });
}

export function useReplaceInstanceSpec() {
  const queryClient = useQueryClient();
  return useMutation<InstanceSpecValidationResult, Error, { instanceId: string; input: InstanceSpecDraft }>({
    mutationFn: ({ instanceId, input }) => replaceInstanceSpec(instanceId, input),
    onSuccess: (_result, input) => invalidateServicesWorkspace(queryClient, input.instanceId),
  });
}

export function useRuntimeArtifacts(options?: QueryOptions<RuntimeArtifact[]>) {
  return useQuery({ queryKey: ['runtime-artifacts'], queryFn: () => listRuntimeArtifacts(), staleTime: stale.normal, retry: false, ...options });
}

export function useRuntimeArtifactDetail(artifactId: string | undefined, options?: QueryOptions<RuntimeArtifact>) {
  return useQuery({
    queryKey: ['runtime-artifact', artifactId],
    queryFn: () => getRuntimeArtifact(artifactId || ''),
    enabled: Boolean(artifactId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useImportRuntimeArtifact() {
  const queryClient = useQueryClient();
  return useMutation<RuntimeArtifactImportResult, Error, RuntimeArtifactImportInput>({
    mutationFn: importRuntimeArtifact,
    onSuccess: (artifact) => {
      invalidateServicesWorkspace(queryClient);
      void queryClient.invalidateQueries({ queryKey: ['runtime-artifact', artifact.id] });
    },
  });
}

export function useDeleteRuntimeArtifact() {
  const queryClient = useQueryClient();
  return useMutation<RuntimeArtifactDeleteResult, Error, string>({
    mutationFn: deleteRuntimeArtifact,
    onSuccess: () => invalidateServicesWorkspace(queryClient),
  });
}

function invalidateInstanceQueries(queryClient: ReturnType<typeof useQueryClient>, instanceId?: string) {
  invalidateServicesWorkspace(queryClient, instanceId);
  if (instanceId) {
    void queryClient.invalidateQueries({ queryKey: ['instance-runtime-observations', instanceId] });
    void queryClient.invalidateQueries({ queryKey: ['instance-access-groups', instanceId] });
  }
}

export function useInstanceDetail(instanceId: string | undefined, options?: QueryOptions<ServiceInstanceDetail>) {
  return useQuery({
    queryKey: ['instance', instanceId],
    queryFn: () => getInstance(instanceId || ''),
    enabled: Boolean(instanceId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useInstanceRuntimeStates(options?: QueryOptions<ServiceInstanceRuntimeState[]>) {
  return useQuery({ queryKey: ['instance-runtime-states'], queryFn: endpoints.instanceRuntimeStates, staleTime: stale.fast, refetchInterval: 15_000, ...options });
}

export function useInstanceRuntimeState(instanceId: string | undefined, options?: QueryOptions<ServiceInstanceRuntimeState>) {
  return useQuery({
    queryKey: ['instance-runtime-state', instanceId],
    queryFn: () => getInstanceRuntimeState(instanceId || ''),
    enabled: Boolean(instanceId),
    staleTime: stale.fast,
    refetchInterval: 15_000,
    retry: false,
    ...options,
  });
}

export function useInstanceRevisions(instanceId: string | undefined, options?: QueryOptions<ServiceInstanceRevision[]>) {
  return useQuery({
    queryKey: ['instance-revisions', instanceId],
    queryFn: () => getInstanceRevisions(instanceId || ''),
    enabled: Boolean(instanceId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useInstanceDiagnostics(instanceId: string | undefined, options?: QueryOptions<ServiceInstanceRuntimeObservation[]>) {
  return useQuery({
    queryKey: ['instance-runtime-observations', instanceId],
    queryFn: () => getInstanceRuntimeObservations(instanceId || ''),
    enabled: Boolean(instanceId),
    staleTime: stale.fast,
    refetchInterval: 15_000,
    ...options,
  });
}

export function useInstanceAccessGroups(instanceId: string | undefined, options?: QueryOptions<InstanceAccessGroupMaterialization>) {
  return useQuery({
    queryKey: ['instance-access-groups', instanceId],
    queryFn: () => getInstanceAccessGroups(instanceId || ''),
    enabled: Boolean(instanceId),
    staleTime: stale.normal,
    retry: false,
    ...options,
  });
}

export function useApplyInstance() {
  const queryClient = useQueryClient();
  return useMutation<InstanceApplyResult, Error, { instanceId: string; input?: InstanceApplyRequest }>({
    mutationFn: ({ instanceId, input }) => applyInstance(instanceId, input),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useReapplyInstance() {
  const queryClient = useQueryClient();
  return useMutation<InstanceApplyResult, Error, { instanceId: string; input?: InstanceApplyRequest }>({
    mutationFn: ({ instanceId, input }) => reapplyInstance(instanceId, input),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useRollbackInstance() {
  const queryClient = useQueryClient();
  return useMutation<InstanceRollbackResult, Error, { instanceId: string; input: InstanceRollbackRequest }>({
    mutationFn: ({ instanceId, input }) => rollbackInstance(instanceId, input),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useRunInstanceDiagnostics() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, { instanceId: string }>({
    mutationFn: ({ instanceId }) => runInstanceDiagnostics(instanceId),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useInstanceLifecycleAction() {
  const queryClient = useQueryClient();
  return useMutation<InstanceLifecycleResult, Error, { instanceId: string; action: InstanceLifecycleAction }>({
    mutationFn: ({ instanceId, action }) => runInstanceLifecycleAction(instanceId, action),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useDeleteInstance() {
  const queryClient = useQueryClient();
  return useMutation<InstanceDeleteResult, Error, { instanceId: string }>({
    mutationFn: ({ instanceId }) => deleteInstance(instanceId),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useForceDeleteInstance() {
  const queryClient = useQueryClient();
  return useMutation<InstanceDeleteResult, Error, { instanceId: string; input: InstanceForceDeleteInput }>({
    mutationFn: ({ instanceId, input }) => forceDeleteInstance(instanceId, input),
    onSuccess: (_result, input) => invalidateInstanceQueries(queryClient, input.instanceId),
  });
}

export function useClients(options?: QueryOptions<ClientAccount[]>) {
  return useQuery({ queryKey: ['clients'], queryFn: endpoints.clients, staleTime: stale.normal, ...options });
}

export function useClientDetail(clientId: string | undefined, options?: QueryOptions<ClientDetail>) {
  return useQuery({
    queryKey: ['client', clientId],
    queryFn: () => getClient(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

function invalidateClientQueries(queryClient: ReturnType<typeof useQueryClient>, clientId?: string) {
  void queryClient.invalidateQueries({ queryKey: ['clients'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  if (clientId) {
    void queryClient.invalidateQueries({ queryKey: ['client', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-access-overview', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-accesses', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-routes', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-artifacts', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-share-links', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-subscriptions', clientId] });
    void queryClient.invalidateQueries({ queryKey: ['client-delivery-history', clientId] });
  }
}

export function useCreateClient() {
  const queryClient = useQueryClient();
  return useMutation<ClientDetail, Error, ClientCreateInput>({
    mutationFn: createClient,
    onSuccess: (client) => invalidateClientQueries(queryClient, client.id),
  });
}

export function useUpdateClient() {
  const queryClient = useQueryClient();
  return useMutation<ClientDetail, Error, { clientId: string; input: ClientUpdateInput }>({
    mutationFn: ({ clientId, input }) => updateClient(clientId, input),
    onSuccess: (client, input) => invalidateClientQueries(queryClient, client.id || input.clientId),
  });
}

export function useUpdateClientStatus() {
  const queryClient = useQueryClient();
  return useMutation<ClientDetail, Error, { clientId: string; input: ClientStatusUpdateInput }>({
    mutationFn: ({ clientId, input }) => updateClientStatus(clientId, input),
    onSuccess: (client, input) => invalidateClientQueries(queryClient, client.id || input.clientId),
  });
}

export function useDeleteClient() {
  const queryClient = useQueryClient();
  return useMutation<ClientDeleteResult, Error, string>({
    mutationFn: deleteClient,
    onSuccess: (_result, clientId) => invalidateClientQueries(queryClient, clientId),
  });
}

export function useRevokeClient() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, string>({
    mutationFn: revokeClient,
    onSuccess: (job, clientId) => {
      invalidateClientQueries(queryClient, clientId);
      void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
    },
  });
}

export function useClientRoutes(clientId: string | undefined, options?: QueryOptions<ClientRoute[]>) {
  return useQuery({
    queryKey: ['client-routes', clientId],
    queryFn: () => listClientRoutes(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateClientRoute() {
  const queryClient = useQueryClient();
  return useMutation<ClientRoute, Error, { clientId: string; input: ClientRouteInput }>({
    mutationFn: ({ clientId, input }) => createClientRoute(clientId, input),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useUpdateClientRoute() {
  const queryClient = useQueryClient();
  return useMutation<ClientRoute, Error, { clientId: string; routeId: string; input: ClientRouteInput }>({
    mutationFn: ({ clientId, routeId, input }) => updateClientRoute(clientId, routeId, input),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useDeleteClientRoute() {
  const queryClient = useQueryClient();
  return useMutation<ClientRoute, Error, { clientId: string; routeId: string }>({
    mutationFn: ({ clientId, routeId }) => deleteClientRoute(clientId, routeId),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useClientAccesses(clientId: string | undefined, options?: QueryOptions<ClientAccess[]>) {
  return useQuery({
    queryKey: ['client-accesses', clientId],
    queryFn: () => listClientAccesses(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useRotateClientAccess() {
  const queryClient = useQueryClient();
  return useMutation<ClientAccessRotationResult, Error, { clientId: string; accessId: string; input: ClientAccessRotationInput }>({
    mutationFn: ({ clientId, accessId, input }) => rotateClientAccess(clientId, accessId, input),
    onSuccess: (result, input) => {
      invalidateClientQueries(queryClient, input.clientId);
      if (result.id) void queryClient.invalidateQueries({ queryKey: ['job', result.id] });
    },
  });
}

export function useRevokeClientAccess() {
  const queryClient = useQueryClient();
  return useMutation<ClientAccessRevokeResult, Error, { clientId: string; accessId: string }>({
    mutationFn: ({ clientId, accessId }) => revokeClientAccess(clientId, accessId),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useDeleteClientAccess() {
  const queryClient = useQueryClient();
  return useMutation<ClientAccessDeleteResult, Error, { clientId: string; accessId: string }>({
    mutationFn: ({ clientId, accessId }) => deleteClientAccess(clientId, accessId),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useCleanupClientConfigs() {
  const queryClient = useQueryClient();
  return useMutation<ClientConfigCleanupResult, Error, { clientId: string }>({
    mutationFn: ({ clientId }) => cleanupClientConfigs(clientId),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
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
  void queryClient.invalidateQueries({ queryKey: ['client-access-overview'] });
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

export function useClientAccessOverview(clientId: string | undefined, options?: QueryOptions<ClientAccessOverview>) {
  return useQuery({
    queryKey: ['client-access-overview', clientId],
    queryFn: () => getClientAccessOverview(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function usePreviewSingleClientAccessGroupAssignment() {
  return useMutation<ClientAccessGroupMembershipResult, Error, { groupId: string; clientId: string; mode: 'add_only' | 'add_or_move'; buildArtifacts?: boolean }>({
    mutationFn: ({ groupId, clientId, mode, buildArtifacts }) => previewSingleClientAccessGroupAssignment(groupId, clientId, mode, buildArtifacts),
  });
}

export function useApplySingleClientAccessGroupAssignment() {
  const queryClient = useQueryClient();
  return useMutation<ClientAccessGroupMembershipResult, Error, { groupId: string; clientId: string; mode: 'add_only' | 'add_or_move'; buildArtifacts?: boolean }>({
    mutationFn: ({ groupId, clientId, mode, buildArtifacts }) => applySingleClientAccessGroupAssignment(groupId, clientId, mode, buildArtifacts),
    onSuccess: (result, input) => {
      invalidateClientAccessGroupQueries(queryClient, result.group_id || input.groupId);
      invalidateClientQueries(queryClient, input.clientId);
    },
  });
}

export function useRemoveClientAccessGroupMembership() {
  const queryClient = useQueryClient();
  return useMutation<ClientAccessGroupMembershipResult, Error, { groupId: string; clientId: string }>({
    mutationFn: ({ groupId, clientId }) => removeClientAccessGroupMembership(groupId, clientId),
    onSuccess: (result, input) => {
      invalidateClientAccessGroupQueries(queryClient, result.group_id || input.groupId);
      invalidateClientQueries(queryClient, input.clientId);
    },
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

export function usePlatformSettings(options?: QueryOptions<PlatformSettings>) {
  return useQuery({ queryKey: ['platform-settings'], queryFn: getPlatformSettings, staleTime: stale.normal, ...options });
}

export function useUpdatePlatformSettings() {
  const queryClient = useQueryClient();
  return useMutation<SettingsUpdateResult, Error, PlatformSettingsInput>({
    mutationFn: updatePlatformSettings,
    onSuccess: (settings) => {
      queryClient.setQueryData(['platform-settings'], settings);
      queryClient.setQueryData(['control-plane-tls'], settings);
      void queryClient.invalidateQueries({ queryKey: ['platform-settings'] });
      void queryClient.invalidateQueries({ queryKey: ['control-plane-tls'] });
      void queryClient.invalidateQueries({ queryKey: ['runtime-preflight'] });
    },
  });
}

export function useApplyTlsSettings() {
  const queryClient = useQueryClient();
  return useMutation<Job, Error, void>({
    mutationFn: () => applyTlsSettings(),
    onSuccess: (job) => {
      void queryClient.invalidateQueries({ queryKey: ['jobs'] });
      if (job.id) void queryClient.invalidateQueries({ queryKey: ['job', job.id] });
      void queryClient.invalidateQueries({ queryKey: ['platform-settings'] });
      void queryClient.invalidateQueries({ queryKey: ['control-plane-tls'] });
      void queryClient.invalidateQueries({ queryKey: ['runtime-preflight'] });
    },
  });
}

export function useMailSettings(options?: QueryOptions<MailSettings>) {
  return useQuery({ queryKey: ['mail-settings'], queryFn: getMailSettings, staleTime: stale.normal, ...options });
}

export function useUpdateMailSettings() {
  const queryClient = useQueryClient();
  return useMutation<MailSettings, Error, MailSettingsInput>({
    mutationFn: updateMailSettings,
    onSuccess: (settings) => {
      queryClient.setQueryData(['mail-settings'], settings);
      void queryClient.invalidateQueries({ queryKey: ['mail-settings'] });
      void queryClient.invalidateQueries({ queryKey: ['platform-invites'] });
    },
  });
}

export function useTestMailSettings() {
  const queryClient = useQueryClient();
  return useMutation<MailTestResult, Error, MailTestInput>({
    mutationFn: testMailSettings,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['mail-settings'] });
    },
  });
}

export function usePlatformUsers(options?: QueryOptions<UserAccount[]>) {
  return useQuery({ queryKey: ['platform-users'], queryFn: listUsers, staleTime: stale.normal, ...options });
}

export function usePlatformUserDetail(userId: string | undefined, options?: QueryOptions<UserAccount>) {
  return useQuery({
    queryKey: ['platform-user', userId],
    queryFn: () => getUser(userId || ''),
    enabled: Boolean(userId),
    staleTime: stale.normal,
    ...options,
  });
}

export function usePlatformInvites(options?: QueryOptions<Invite[]>) {
  return useQuery({ queryKey: ['platform-invites'], queryFn: listInvites, staleTime: stale.normal, ...options });
}

export function useCreateInvite() {
  const queryClient = useQueryClient();
  return useMutation<InviteCreateResult, Error, InviteCreateInput>({
    mutationFn: createInvite,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['platform-users'] });
      void queryClient.invalidateQueries({ queryKey: ['platform-invites'] });
    },
  });
}

export function useRevokeInvite() {
  const queryClient = useQueryClient();
  return useMutation<InviteRevokeResult, Error, string>({
    mutationFn: revokeInvite,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['platform-invites'] });
    },
  });
}

export function usePlatformSessions(options?: QueryOptions<Session[]>) {
  return useQuery({ queryKey: ['platform-sessions'], queryFn: listSessions, staleTime: stale.normal, ...options });
}

export function useRevokeSession() {
  const queryClient = useQueryClient();
  return useMutation<SessionRevokeResult, Error, string>({
    mutationFn: revokeSession,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['platform-sessions'] });
      void queryClient.invalidateQueries({ queryKey: ['auth', 'session'] });
    },
  });
}

export function useCertificates(options?: QueryOptions<Certificate[]>) {
  return useQuery({ queryKey: ['certificates'], queryFn: endpoints.certificates, staleTime: stale.normal, ...options });
}

export function useCertificateDetail(certificateId: string | undefined, options?: QueryOptions<CertificateDetail>) {
  return useQuery({
    queryKey: ['certificate', certificateId],
    queryFn: () => getCertificate(certificateId || ''),
    enabled: Boolean(certificateId),
    staleTime: stale.normal,
    ...options,
  });
}

export function usePkiRoots(options?: QueryOptions<PkiRoot[]>) {
  return useQuery({ queryKey: ['pki-roots'], queryFn: endpoints.pkiRoots, staleTime: stale.normal, ...options });
}

function invalidateCertificateQueries(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ['certificates'] });
  void queryClient.invalidateQueries({ queryKey: ['pki-roots'] });
  void queryClient.invalidateQueries({ queryKey: ['control-plane-tls'] });
  void queryClient.invalidateQueries({ queryKey: ['runtime-preflight'] });
}

export function useImportCertificate() {
  const queryClient = useQueryClient();
  return useMutation<Certificate, Error, CertificateImportInput>({
    mutationFn: importCertificate,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function usePreviewCertificateImport() {
  return useMutation<CertificateImportPreview, Error, CertificateImportInput>({
    mutationFn: previewCertificateImport,
  });
}

export function useCreateSelfSignedCertificate() {
  const queryClient = useQueryClient();
  return useMutation<Certificate, Error, CertificateCreateInput>({
    mutationFn: createSelfSignedCertificate,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useCreateManagedCertificateAuthority() {
  const queryClient = useQueryClient();
  return useMutation<Certificate, Error, CertificateAuthorityCreateInput>({
    mutationFn: createManagedCertificateAuthority,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useIssueCertificate() {
  const queryClient = useQueryClient();
  return useMutation<Certificate, Error, CertificateIssueInput>({
    mutationFn: issueCertificate,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useSetDefaultCertificate() {
  const queryClient = useQueryClient();
  return useMutation<CertificateActionResult, Error, string>({
    mutationFn: setDefaultCertificate,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useRevokeCertificate() {
  const queryClient = useQueryClient();
  return useMutation<CertificateRevokeResult, Error, string>({
    mutationFn: revokeCertificate,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useDeleteCertificate() {
  const queryClient = useQueryClient();
  return useMutation<CertificateDeleteResult, Error, string>({
    mutationFn: deleteCertificate,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useCreatePkiRoot() {
  const queryClient = useQueryClient();
  return useMutation<PkiRoot, Error, PkiRootCreateInput>({
    mutationFn: createPkiRoot,
    onSuccess: () => invalidateCertificateQueries(queryClient),
  });
}

export function useBackhaulLinks(options?: QueryOptions<BackhaulLink[]>) {
  return useQuery({ queryKey: ['backhaul-links'], queryFn: endpoints.backhaulLinks, staleTime: stale.normal, ...options });
}

export function useBackhaulLink(linkId: string | undefined, options?: QueryOptions<BackhaulLink>) {
  return useQuery({
    queryKey: ['backhaul-link', linkId],
    queryFn: () => getBackhaulLink(linkId || ''),
    enabled: Boolean(linkId),
    staleTime: stale.normal,
    ...options,
  });
}

function invalidateBackhaulQueries(queryClient: ReturnType<typeof useQueryClient>, linkId?: string) {
  void queryClient.invalidateQueries({ queryKey: ['backhaul-links'] });
  void queryClient.invalidateQueries({ queryKey: ['route-policies'] });
  void queryClient.invalidateQueries({ queryKey: ['nodes'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  void queryClient.invalidateQueries({ queryKey: ['dashboard'] });
  if (linkId) void queryClient.invalidateQueries({ queryKey: ['backhaul-link', linkId] });
}

export function useApplyBackhaulLink() {
  const queryClient = useQueryClient();
  return useMutation<BackhaulActionResult, Error, string>({
    mutationFn: applyBackhaulLink,
    onSuccess: (result, linkId) => {
      invalidateBackhaulQueries(queryClient, linkId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useProbeBackhaulLink() {
  const queryClient = useQueryClient();
  return useMutation<BackhaulActionResult, Error, string>({
    mutationFn: probeBackhaulLink,
    onSuccess: (result, linkId) => {
      invalidateBackhaulQueries(queryClient, linkId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function usePromoteBackhaulLink() {
  const queryClient = useQueryClient();
  return useMutation<BackhaulActionResult, Error, { linkId: string; transportId: string }>({
    mutationFn: ({ linkId, transportId }) => promoteBackhaulLink(linkId, { transport_id: transportId }),
    onSuccess: (result, input) => {
      invalidateBackhaulQueries(queryClient, input.linkId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useUpdateBackhaulRouteState() {
  const queryClient = useQueryClient();
  return useMutation<BackhaulActionResult, Error, { linkId: string; input: BackhaulRouteStateInput }>({
    mutationFn: ({ linkId, input }) => updateBackhaulRouteState(linkId, input),
    onSuccess: (result, input) => {
      invalidateBackhaulQueries(queryClient, input.linkId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useRoutePolicies(options?: QueryOptions<RoutePolicy[]>) {
  return useQuery({ queryKey: ['route-policies'], queryFn: listRoutePolicies, staleTime: stale.normal, ...options });
}

export function useRoutePolicy(nodeId: string | undefined, options?: QueryOptions<RoutePolicy>) {
  return useQuery({
    queryKey: ['route-policy', nodeId],
    queryFn: () => getRoutePolicy(nodeId || ''),
    enabled: Boolean(nodeId),
    staleTime: stale.normal,
    ...options,
  });
}

function invalidateRoutePolicyQueries(queryClient: ReturnType<typeof useQueryClient>, nodeId?: string) {
  void queryClient.invalidateQueries({ queryKey: ['route-policies'] });
  void queryClient.invalidateQueries({ queryKey: ['nodes'] });
  void queryClient.invalidateQueries({ queryKey: ['jobs'] });
  if (nodeId) {
    void queryClient.invalidateQueries({ queryKey: ['route-policy', nodeId] });
    void queryClient.invalidateQueries({ queryKey: ['node', nodeId] });
  }
}

export function usePreviewRoutePolicy() {
  return useMutation<RoutePolicyPreviewResult, Error, string>({
    mutationFn: previewRoutePolicy,
  });
}

export function useApplyRoutePolicy() {
  const queryClient = useQueryClient();
  return useMutation<RoutePolicyApplyResult, Error, string>({
    mutationFn: applyRoutePolicy,
    onSuccess: (result, nodeId) => {
      invalidateRoutePolicyQueries(queryClient, nodeId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useCleanupRoutePolicy() {
  const queryClient = useQueryClient();
  return useMutation<RoutePolicyCleanupResult, Error, string>({
    mutationFn: cleanupRoutePolicy,
    onSuccess: (result, nodeId) => {
      invalidateRoutePolicyQueries(queryClient, nodeId);
      invalidateJobsFromResult(queryClient, result);
    },
  });
}

export function useArtifacts(options?: QueryOptions<Artifact[]>) {
  return useQuery({ queryKey: ['artifacts'], queryFn: endpoints.artifacts, staleTime: stale.normal, ...options });
}

export function useClientArtifacts(clientId: string | undefined, options?: QueryOptions<ClientArtifact[]>) {
  return useQuery({
    queryKey: ['client-artifacts', clientId],
    queryFn: () => listClientArtifacts(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useBuildClientArtifact() {
  const queryClient = useQueryClient();
  return useMutation<ClientArtifactBuildResult, Error, { clientId: string; input: ClientArtifactBuildRequest }>({
    mutationFn: ({ clientId, input }) => buildClientArtifact(clientId, input),
    onSuccess: (result, input) => {
      invalidateClientQueries(queryClient, input.clientId);
      void queryClient.invalidateQueries({ queryKey: ['artifacts'] });
      if (result.job?.id) void queryClient.invalidateQueries({ queryKey: ['job', result.job.id] });
    },
  });
}

export function useDeleteClientArtifact() {
  const queryClient = useQueryClient();
  return useMutation<ClientArtifactDeleteResult, Error, { clientId: string; artifactId: string }>({
    mutationFn: ({ clientId, artifactId }) => deleteClientArtifact(clientId, artifactId),
    onSuccess: (_result, input) => {
      invalidateClientQueries(queryClient, input.clientId);
      void queryClient.invalidateQueries({ queryKey: ['artifacts'] });
    },
  });
}

export function useClientShareLinks(clientId: string | undefined, options?: QueryOptions<ClientShareLink[]>) {
  return useQuery({
    queryKey: ['client-share-links', clientId],
    queryFn: () => listClientShareLinks(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateClientShareLink() {
  const queryClient = useQueryClient();
  return useMutation<ClientShareLinkCreateResult, Error, { clientId: string; input: ClientShareLinkCreateInput }>({
    mutationFn: ({ clientId, input }) => createClientShareLink(clientId, input),
    onSuccess: (_result, input) => {
      invalidateClientQueries(queryClient, input.clientId);
      void queryClient.invalidateQueries({ queryKey: ['share-links'] });
    },
  });
}

export function useRevokeClientShareLink() {
  const queryClient = useQueryClient();
  return useMutation<ClientShareLinkRevokeResult, Error, { clientId: string; shareId: string }>({
    mutationFn: ({ clientId, shareId }) => revokeClientShareLink(clientId, shareId),
    onSuccess: (_result, input) => {
      invalidateClientQueries(queryClient, input.clientId);
      void queryClient.invalidateQueries({ queryKey: ['share-links'] });
    },
  });
}

export function useRotateClientShareLink() {
  const queryClient = useQueryClient();
  return useMutation<ClientShareLinkRotateResult, Error, { clientId: string; shareId: string; ttlHours?: number }>({
    mutationFn: ({ clientId, shareId, ttlHours }) => rotateClientShareLink(clientId, shareId, ttlHours),
    onSuccess: (_result, input) => {
      invalidateClientQueries(queryClient, input.clientId);
      void queryClient.invalidateQueries({ queryKey: ['share-links'] });
    },
  });
}

export function useClientSubscriptions(clientId: string | undefined, options?: QueryOptions<ClientSubscription[]>) {
  return useQuery({
    queryKey: ['client-subscriptions', clientId],
    queryFn: () => listClientSubscriptions(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useCreateClientSubscription() {
  const queryClient = useQueryClient();
  return useMutation<ClientSubscriptionCreateResult, Error, { clientId: string; input: ClientSubscriptionCreateInput }>({
    mutationFn: ({ clientId, input }) => createClientSubscription(clientId, input),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useRotateClientSubscription() {
  const queryClient = useQueryClient();
  return useMutation<ClientSubscriptionRotateResult, Error, { clientId: string; input: ClientSubscriptionCreateInput }>({
    mutationFn: ({ clientId, input }) => rotateClientSubscription(clientId, input),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useRevokeClientSubscription() {
  const queryClient = useQueryClient();
  return useMutation<ClientSubscriptionRevokeResult, Error, { clientId: string; subscriptionId: string }>({
    mutationFn: ({ clientId, subscriptionId }) => revokeClientSubscription(clientId, subscriptionId),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useSendClientArtifactEmail() {
  const queryClient = useQueryClient();
  return useMutation<ClientEmailDeliveryResult, Error, { clientId: string; artifactId?: string; input: ClientEmailDeliveryInput }>({
    mutationFn: ({ clientId, artifactId, input }) => sendClientArtifactEmail(clientId, artifactId, input),
    onSuccess: (_result, input) => invalidateClientQueries(queryClient, input.clientId),
  });
}

export function useClientDeliveryHistory(clientId: string | undefined, options?: QueryOptions<ClientDeliveryHistoryItem[]>) {
  return useQuery({
    queryKey: ['client-delivery-history', clientId],
    queryFn: () => listClientDeliveryHistory(clientId || ''),
    enabled: Boolean(clientId),
    staleTime: stale.normal,
    ...options,
  });
}

export function useShareLinks(options?: QueryOptions<ShareLink[]>) {
  return useQuery({ queryKey: ['share-links'], queryFn: endpoints.shareLinks, staleTime: stale.normal, ...options });
}

export function useAddressPools(options?: QueryOptions<AddressPools>) {
  return useQuery({ queryKey: ['address-pools'], queryFn: endpoints.addressPools, staleTime: stale.normal, ...options });
}
