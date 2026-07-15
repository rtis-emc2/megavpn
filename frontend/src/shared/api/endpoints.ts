import { apiBlobRequest, apiRequest, apiURL, isUnauthorized, sendJSON } from './client';
import type {
  AddressPools,
  APIRecord,
  Artifact,
  AuthPayload,
  BackhaulActionResult,
  BackhaulLink,
  BackhaulPromoteInput,
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
  ClientAccess,
  ClientAccessDeleteResult,
  ClientAccount,
  ClientAccessOverview,
  ClientAccessRevokeResult,
  ClientAccessRotationInput,
  ClientAccessRotationResult,
  ClientArtifact,
  ClientArtifactBuildRequest,
  ClientArtifactBuildResult,
  ClientArtifactDeleteResult,
  ClientArtifactDownloadResult,
  ClientConfigCleanupResult,
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
  Dashboard,
  AgentTokenRotateResult,
  BootstrapRequest,
  BootstrapResult,
  NodeBootstrapBundleDownloadResult,
  NodeBootstrapBundleRevealResult,
  EnrollmentToken,
  EnrollmentTokenCreateInput,
  EnrollmentTokenCreateResult,
  EnrollmentTokenIssueResult,
  EnrollmentTokenRevokeResult,
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
  InstanceCreateFromPackInput,
  InstanceCreateResult,
  InstanceManualCreateInput,
  InstanceSpecDraft,
  InstanceSpecValidationResult,
  InstanceRollbackRequest,
  InstanceRollbackResult,
  Job,
  Invite,
  InviteCreateInput,
  InviteCreateResult,
  InviteRevokeResult,
  MailSettings,
  MailSettingsInput,
  MailTestInput,
  MailTestResult,
  OneTimeSecretDisplay,
  PkiRoot,
  PkiRootCreateInput,
  PlatformSettings,
  PlatformSettingsInput,
  HostKeyDecisionResult,
  HostKeyScanResult,
  NodeAgentIdentityRevokeInput,
  NodeAgentIdentityRevokeResult,
  NodeCapability,
  NodeCapabilityDrift,
  NodeCapabilityInstallEvent,
  NodeCapabilityInstallInput,
  NodeCapabilityInstallResult,
  NodeCapabilityVerifyInput,
  NodeCapabilityVerifyResult,
  NodeAgentState,
  NodeCreateInput,
  NodeDetail,
  NodeDiagnosticResult,
  NodeDiagnostics,
  NodeDiagnosticsAction,
  NodeEntity,
  NodeAccessMethod,
  NodeAccessMethodsResult,
  NodeBootstrapRun,
  NodeInventorySnapshot,
  NodeInventorySyncResult,
  NodeJobEnvelope,
  NodeRuntimeReconcileResult,
  NodeServiceDiscovery,
  NodeServiceDiscoveryImportResult,
  NodeServiceDiscoverySummary,
  NodeServiceInstaller,
  NodeRetireResult,
  NodeSSHAccessMethodCreateInput,
  NodeSSHAccessMethodCreateResult,
  NodeStaleRotationPreview,
  NodeUpdateInput,
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
  ServicePackDetail,
  ServicePackCreateInput,
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

export function updateClient(clientId: string, input: ClientUpdateInput): Promise<ClientDetail> {
  return sendJSON<ClientDetail>(`/api/v1/clients/${encodeURIComponent(clientId)}`, 'PATCH', input);
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

export function listClientRoutes(clientId: string): Promise<ClientRoute[]> {
  return apiRequest<ClientRoute[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/routes`);
}

export function createClientRoute(clientId: string, input: ClientRouteInput): Promise<ClientRoute> {
  return sendJSON<ClientRoute>(`/api/v1/clients/${encodeURIComponent(clientId)}/routes`, 'POST', input);
}

export function updateClientRoute(clientId: string, routeId: string, input: ClientRouteInput): Promise<ClientRoute> {
  return sendJSON<ClientRoute>(`/api/v1/clients/${encodeURIComponent(clientId)}/routes/${encodeURIComponent(routeId)}`, 'PATCH', input);
}

export function deleteClientRoute(clientId: string, routeId: string): Promise<ClientRoute> {
  return apiRequest<ClientRoute>(`/api/v1/clients/${encodeURIComponent(clientId)}/routes/${encodeURIComponent(routeId)}`, { method: 'DELETE' });
}

export function listClientAccesses(clientId: string): Promise<ClientAccess[]> {
  return apiRequest<ClientAccess[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/accesses`);
}

const clientAccessRotationSuffix: Record<ClientAccessRotationInput['driver'], string> = {
  openvpn: 'rotate-openvpn',
  'xray-core': 'rotate-xray',
  xray: 'rotate-xray',
  xray_core: 'rotate-xray',
  vless: 'rotate-xray',
  wireguard: 'rotate-wireguard',
  mtproto: 'rotate-mtproto',
  ipsec: 'rotate-ipsec',
  http_proxy: 'rotate-http-proxy',
  'http-proxy': 'rotate-http-proxy',
  shadowsocks: 'rotate-shadowsocks',
};

const clientAccessRotationSecretFields = ['token', 'secret', 'password', 'private_key', 'config', 'config_payload', 'subscription_url', 'share_url', 'uri'];

function sanitizeClientAccessRotationResult(result: ClientAccessRotationResult, accessId: string): ClientAccessRotationResult {
  const safeResult = { ...result };
  let secret = safeResult.one_time_secret;
  for (const field of clientAccessRotationSecretFields) {
    const value = safeResult[field];
    if (!secret?.value && typeof value === 'string' && value.trim()) {
      secret = {
        kind: 'client_access_rotation',
        label: 'Rotated access one-time value',
        value,
        object_id: accessId,
      } satisfies OneTimeSecretDisplay;
    }
    delete safeResult[field];
  }
  if (safeResult.result && typeof safeResult.result === 'object') {
    const nested = { ...safeResult.result };
    for (const field of clientAccessRotationSecretFields) {
      delete nested[field];
    }
    safeResult.result = nested;
  }
  if (secret?.value) safeResult.one_time_secret = secret;
  return safeResult;
}

export async function rotateClientAccess(clientId: string, accessId: string, input: ClientAccessRotationInput): Promise<ClientAccessRotationResult> {
  const suffix = clientAccessRotationSuffix[input.driver];
  if (!suffix) {
    return Promise.reject(new Error('Unsupported client access rotation driver.'));
  }
  const result = await sendJSON<ClientAccessRotationResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/accesses/${encodeURIComponent(accessId)}/${suffix}`, 'POST', {});
  return sanitizeClientAccessRotationResult(result, accessId);
}

export function revokeClientAccess(clientId: string, accessId: string): Promise<ClientAccessRevokeResult> {
  return sendJSON<ClientAccessRevokeResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/accesses/${encodeURIComponent(accessId)}/revoke`, 'POST', {});
}

export function deleteClientAccess(clientId: string, accessId: string): Promise<ClientAccessDeleteResult> {
  return apiRequest<ClientAccessDeleteResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/accesses/${encodeURIComponent(accessId)}`, { method: 'DELETE' });
}

export function cleanupClientConfigs(clientId: string): Promise<ClientConfigCleanupResult> {
  return apiRequest<ClientConfigCleanupResult>(`/api/v1/clients/${encodeURIComponent(clientId)}/configs`, { method: 'DELETE' });
}

export function listInstances(_params: { search?: string; status?: string; serviceCode?: string; nodeId?: string } = {}): Promise<ServiceInstance[]> {
  return apiRequest<ServiceInstance[]>('/api/v1/instances');
}

export function listServiceTypeCapabilities(): Promise<ServiceTypeCapability[]> {
  return apiRequest<ServiceTypeCapability[]>('/api/v1/services');
}

export function listRuntimeTargets(_params: { serviceCode?: string } = {}): Promise<RuntimeTargetNode[]> {
  return apiRequest<RuntimeTargetNode[]>('/api/v1/nodes');
}

export function listNodes(_params: { search?: string; status?: string; role?: string } = {}): Promise<NodeEntity[]> {
  return apiRequest<NodeEntity[]>('/api/v1/nodes');
}

export function getNode(nodeId: string): Promise<NodeDetail> {
  return apiRequest<NodeDetail>(`/api/v1/nodes/${encodeURIComponent(nodeId)}`);
}

export function createNode(input: NodeCreateInput): Promise<NodeDetail> {
  return sendJSON<NodeDetail>('/api/v1/nodes', 'POST', input);
}

export function updateNode(nodeId: string, input: NodeUpdateInput): Promise<NodeDetail> {
  return sendJSON<NodeDetail>(`/api/v1/nodes/${encodeURIComponent(nodeId)}`, 'PUT', input);
}

function routePolicyFromNode(node: NodeEntity): RoutePolicy {
  return {
    node_id: node.id,
    node_name: node.name,
    node_role: node.role || node.kind,
    node_address: node.address,
    node_status: node.status || node.agent_status,
    updated_at: node.updated_at,
  };
}

export async function listRoutePolicies(): Promise<RoutePolicy[]> {
  const nodes = await listNodes();
  return nodes.map(routePolicyFromNode);
}

export async function getRoutePolicy(nodeId: string): Promise<RoutePolicy> {
  const node = await getNode(nodeId);
  return routePolicyFromNode(node);
}

export function previewRoutePolicy(nodeId: string): Promise<RoutePolicyPreviewResult> {
  return apiRequest<RoutePolicyPreviewResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/routes/preview`);
}

export function applyRoutePolicy(nodeId: string): Promise<RoutePolicyApplyResult> {
  return sendJSON<RoutePolicyApplyResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/routes/apply`, 'POST', {});
}

export function cleanupRoutePolicy(nodeId: string): Promise<RoutePolicyCleanupResult> {
  return sendJSON<RoutePolicyCleanupResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/routes/cleanup`, 'POST', {});
}

export function setNodeMaintenance(nodeId: string, enabled: boolean): Promise<NodeDetail> {
  return sendJSON<NodeDetail>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/maintenance/${enabled ? 'enable' : 'disable'}`, 'POST', {});
}

export function getNodeDiagnostics(nodeId: string): Promise<NodeDiagnostics> {
  return apiRequest<NodeDiagnostics>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/diagnostics`);
}

export function getNodeStaleRotationPreview(nodeId: string): Promise<NodeStaleRotationPreview> {
  return apiRequest<NodeStaleRotationPreview>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/diagnostics/stale-rotation`);
}

export function revokeNodeAgentIdentity(nodeId: string, input: NodeAgentIdentityRevokeInput): Promise<NodeAgentIdentityRevokeResult> {
  return sendJSON<NodeAgentIdentityRevokeResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/agent-identity/revoke`, 'POST', {
    confirmation: input.confirmation,
    reason: input.reason,
  });
}

export async function getNodeAgentState(nodeId: string): Promise<NodeAgentState> {
  const diagnostics = await getNodeDiagnostics(nodeId);
  return diagnostics.agent || { node_id: nodeId };
}

export function listNodeDiagnostics(nodeId: string): Promise<NodeDiagnostics> {
  return getNodeDiagnostics(nodeId);
}

export function runNodeDiagnosticsAction(nodeId: string, action: NodeDiagnosticsAction): Promise<NodeJobEnvelope | NodeRuntimeReconcileResult> {
  const suffixByAction: Record<NodeDiagnosticsAction, string> = {
    'retry-inventory': 'retry-inventory',
    'retry-discovery': 'retry-discovery',
    'reconcile-runtime': 'reconcile-runtime',
    'requeue-stuck-job': 'requeue-stuck-job',
    'channel-probe': 'channel-probe',
  };
  return sendJSON<NodeJobEnvelope | NodeRuntimeReconcileResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/diagnostics/${suffixByAction[action]}`, 'POST', {});
}

export function runNodeDiagnostics(nodeId: string, input: { action: NodeDiagnosticsAction }): Promise<NodeDiagnosticResult> {
  return runNodeDiagnosticsAction(nodeId, input.action);
}

export function retryNodeDiagnostics(nodeId: string, diagnosticId: string): Promise<NodeDiagnosticResult> {
  const normalized = diagnosticId.trim().toLowerCase();
  if (normalized === 'inventory' || normalized === 'retry-inventory') {
    return runNodeDiagnosticsAction(nodeId, 'retry-inventory');
  }
  if (normalized === 'discovery' || normalized === 'retry-discovery') {
    return runNodeDiagnosticsAction(nodeId, 'retry-discovery');
  }
  return Promise.reject(new Error('Unsupported node diagnostic retry action.'));
}

export function getNodeInventory(nodeId: string): Promise<NodeInventorySnapshot> {
  return apiRequest<NodeInventorySnapshot>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/inventory`);
}

export function syncNodeInventory(nodeId: string): Promise<NodeInventorySyncResult> {
  return sendJSON<NodeInventorySyncResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/inventory/sync`, 'POST', {});
}

export function listServiceInstallers(): Promise<NodeServiceInstaller[]> {
  return apiRequest<NodeServiceInstaller[]>('/api/v1/services/installers');
}

export function listNodeCapabilities(nodeId: string): Promise<NodeCapability[]> {
  return apiRequest<NodeCapability[]>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/capabilities`);
}

export function getNodeCapabilitiesDrift(nodeId: string): Promise<NodeCapabilityDrift> {
  return apiRequest<NodeCapabilityDrift>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/capabilities/drift`);
}

export function listNodeCapabilityInstallEvents(nodeId: string, limit = 25): Promise<NodeCapabilityInstallEvent[]> {
  return apiRequest<NodeCapabilityInstallEvent[]>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/capabilities/install-events${queryString({ limit })}`);
}

export function installNodeCapability(nodeId: string, input: NodeCapabilityInstallInput): Promise<NodeCapabilityInstallResult> {
  return sendJSON<NodeCapabilityInstallResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/capabilities/install`, 'POST', input);
}

export function installNodeCapabilities(nodeId: string, input: NodeCapabilityInstallInput): Promise<NodeCapabilityInstallResult> {
  return installNodeCapability(nodeId, input);
}

export function verifyNodeCapability(nodeId: string, input: NodeCapabilityVerifyInput): Promise<NodeCapabilityVerifyResult> {
  return sendJSON<NodeCapabilityVerifyResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/capabilities/verify`, 'POST', input);
}

export function verifyNodeCapabilities(nodeId: string, input: NodeCapabilityVerifyInput): Promise<NodeCapabilityVerifyResult> {
  return verifyNodeCapability(nodeId, input);
}

export function listNodeServiceDiscoveries(nodeId: string): Promise<NodeServiceDiscovery[]> {
  return apiRequest<NodeServiceDiscovery[]>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/services/discovered`);
}

export function getNodeServiceDiscoverySummary(nodeId: string): Promise<NodeServiceDiscoverySummary> {
  return apiRequest<NodeServiceDiscoverySummary>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/services/discovery-summary`);
}

export function discoverNodeServices(nodeId: string): Promise<Job> {
  return sendJSON<Job>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/services/discover`, 'POST', {});
}

export function listNodeServiceDiscovery(nodeId: string): Promise<NodeServiceDiscovery[]> {
  return listNodeServiceDiscoveries(nodeId);
}

export function importNodeServiceDiscoveryById(nodeId: string, discoveryId: string): Promise<ServiceInstance> {
  return sendJSON<ServiceInstance>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/services/discovered/${encodeURIComponent(discoveryId)}/import`, 'POST', {});
}

export function importNodeServiceDiscovery(nodeId: string, input: { discovery_id?: string; import_all?: boolean }): Promise<NodeServiceDiscoveryImportResult> {
  if (input.import_all) return importAllNodeServiceDiscoveries(nodeId);
  if (!input.discovery_id) return Promise.reject(new Error('discovery_id is required'));
  return importNodeServiceDiscoveryById(nodeId, input.discovery_id);
}

export function importAllNodeServiceDiscoveries(nodeId: string): Promise<ServiceInstance[]> {
  return sendJSON<ServiceInstance[]>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/services/import-all`, 'POST', {});
}

export function listEnrollmentTokens(nodeId: string): Promise<EnrollmentToken[]> {
  return apiRequest<EnrollmentToken[]>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/enrollment-tokens`);
}

export function createEnrollmentToken(nodeId: string, input: EnrollmentTokenCreateInput = {}): Promise<EnrollmentTokenCreateResult> {
  return sendJSON<EnrollmentTokenCreateResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/enrollment-token${queryString({ ttl_hours: input.ttl_hours })}`, 'POST', {});
}

export function rotateEnrollmentToken(nodeId: string, input: EnrollmentTokenCreateInput = {}): Promise<EnrollmentTokenCreateResult> {
  return sendJSON<EnrollmentTokenCreateResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/enrollment-token/rotate${queryString({ ttl_hours: input.ttl_hours })}`, 'POST', {});
}

export function extractEnrollmentTokenSecret(result: EnrollmentTokenIssueResult): string {
  const value = typeof result.token === 'string'
    ? result.token.trim()
    : typeof result.enrollment_token === 'string'
      ? result.enrollment_token.trim()
      : '';
  if (!value) throw new Error('enrollment token value was not returned');
  return value;
}

export function revokeEnrollmentToken(nodeId: string, tokenId: string): Promise<EnrollmentTokenRevokeResult> {
  return apiRequest<EnrollmentTokenRevokeResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/enrollment-tokens/${encodeURIComponent(tokenId)}`, { method: 'DELETE' });
}

export function listNodeAccessMethods(nodeId: string): Promise<NodeAccessMethodsResult> {
  return apiRequest<NodeAccessMethodsResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/access-methods`);
}

export function replaceNodeAccessMethods(nodeId: string, items: NodeAccessMethod[]): Promise<NodeAccessMethodsResult> {
  return sendJSON<NodeAccessMethodsResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/access-methods`, 'PUT', { items });
}

export function createNodeSSHAccessMethod(nodeId: string, input: NodeSSHAccessMethodCreateInput): Promise<NodeSSHAccessMethodCreateResult> {
  return sendJSON<NodeSSHAccessMethodCreateResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/access-methods/ssh`, 'POST', input);
}

export function scanNodeHostKey(nodeId: string, input: { ssh_host?: string; ssh_port?: number } = {}): Promise<HostKeyScanResult> {
  return sendJSON<HostKeyScanResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/ssh/host-key-scan`, 'POST', input);
}

export async function acceptNodeHostKey(nodeId: string, input: { method_id: string; fingerprint: string }): Promise<HostKeyDecisionResult> {
  const methods = await listNodeAccessMethods(nodeId);
  const updated = methods.map((method) => method.id === input.method_id ? { ...method, ssh_host_key_sha256: input.fingerprint } : method);
  if (!updated.some((method) => method.id === input.method_id)) {
    throw new Error('SSH access method not found for host-key pinning.');
  }
  return replaceNodeAccessMethods(nodeId, updated);
}

export function rejectNodeHostKey(_nodeId: string, _input: { method_id?: string; fingerprint?: string } = {}): Promise<never> {
  return Promise.reject(new Error('Backend has no persistent host-key reject endpoint; discard the scan result instead.'));
}

export function bootstrapNode(nodeId: string, input: BootstrapRequest): Promise<BootstrapResult> {
  return sendJSON<BootstrapResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/bootstrap`, 'POST', input);
}

export function reinstallOrUpdateNodeAgent(nodeId: string, input: BootstrapRequest = {}): Promise<BootstrapResult> {
  return bootstrapNode(nodeId, { ...input, reinstall_agent: true });
}

export function listNodeBootstrapRuns(nodeId: string, limit = 25): Promise<NodeBootstrapRun[]> {
  return apiRequest<NodeBootstrapRun[]>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/bootstrap-runs${queryString({ limit })}`);
}

export function revealNodeBootstrapBundle(nodeId: string, runId: string): Promise<NodeBootstrapBundleRevealResult> {
  return sendJSON<NodeBootstrapBundleRevealResult>(
    `/api/v1/nodes/${encodeURIComponent(nodeId)}/bootstrap-runs/${encodeURIComponent(runId)}/bundle/reveal`,
    'POST',
    {},
  );
}

export async function downloadNodeBootstrapBundle(nodeId: string, runId: string): Promise<NodeBootstrapBundleDownloadResult> {
  const fallback = safeBootstrapBundleFilename(nodeId);
  const result = await apiBlobRequest(
    `/api/v1/nodes/${encodeURIComponent(nodeId)}/bootstrap-runs/${encodeURIComponent(runId)}/bundle/download`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
      cache: 'no-store',
    },
  );
  return {
    blob: result.blob,
    contentType: result.contentType,
    filename: sanitizeDownloadFilename(parseContentDispositionFilename(result.contentDisposition), fallback),
  };
}

export function parseContentDispositionFilename(header: string | undefined): string {
  if (!header) return '';
  const parts = header.split(';').map((part) => part.trim());
  const encoded = parts.find((part) => part.toLowerCase().startsWith('filename*='));
  if (encoded) {
    const raw = encoded.slice(encoded.indexOf('=') + 1).trim();
    const match = /^utf-8''(.+)$/i.exec(raw);
    try {
      return decodeURIComponent(match ? match[1] : raw);
    } catch {
      return match ? match[1] : raw;
    }
  }
  const plain = parts.find((part) => part.toLowerCase().startsWith('filename='));
  if (!plain) return '';
  const raw = plain.slice(plain.indexOf('=') + 1).trim();
  if (raw.startsWith('"') && raw.endsWith('"')) {
    return raw.slice(1, -1).replace(/\\"/g, '"').replace(/\\\\/g, '\\');
  }
  return raw;
}

export function sanitizeDownloadFilename(value: string | undefined, fallback = 'megavpn-agent-bootstrap.env'): string {
  const raw = String(value || '').trim();
  let output = '';
  let lastDash = false;
  for (const char of raw) {
    let next = '';
    if (/[A-Za-z0-9]/.test(char) || char === '.' || char === '_') {
      next = char;
    } else if (char === '-' || char === ' ' || char === '\t') {
      next = '-';
    }
    if (!next) continue;
    if (next === '-') {
      if (lastDash) continue;
      lastDash = true;
    } else {
      lastDash = false;
    }
    output += next;
    if (output.length >= 96) break;
  }
  output = output.replace(/^\.+/, '').replace(/[._-]+$/g, '');
  if (!output || output === '..' || output.includes('..')) {
    output = fallback;
  }
  if (!output.toLowerCase().endsWith('.env')) {
    output += '.env';
  }
  return output;
}

function safeBootstrapBundleFilename(nodeId: string): string {
  return sanitizeDownloadFilename(`megavpn-agent-${nodeId || 'node'}-bootstrap.env`);
}

export function rotateNodeAgentToken(nodeId: string): Promise<AgentTokenRotateResult> {
  return sendJSON<AgentTokenRotateResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/agent-token/rotate`, 'POST', {});
}

export function launchNodeSshSession(nodeId: string): Promise<SshSessionLaunchResult> {
  return sendJSON<Omit<SshSessionLaunchResult, 'terminal_url'>>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/ssh/sessions`, 'POST', {})
    .then((result) => {
      const session = String(result.session_id || '').trim();
      const terminalURL = session
        ? apiURL(`/api/v1/nodes/${encodeURIComponent(nodeId)}/ssh/terminal${queryString({ session })}`)
        : '';
      return { ...result, terminal_url: terminalURL };
    });
}

export function retireNode(nodeId: string): Promise<NodeRetireResult> {
  return apiRequest<NodeRetireResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}`, { method: 'DELETE' });
}

export function forceRetireNode(nodeId: string, input: { confirmation: string; reason?: string }): Promise<NodeRetireResult> {
  return sendJSON<NodeRetireResult>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/force-retire`, 'POST', input);
}

export function listServicePacks(params: { includeInactive?: boolean } = {}): Promise<ServicePack[]> {
  return apiRequest<ServicePack[]>(`/api/v1/service-packs${queryString({ include_inactive: params.includeInactive ? 1 : undefined })}`);
}

export async function getServicePack(packId: string): Promise<ServicePackDetail> {
  const packs = await listServicePacks({ includeInactive: true });
  const pack = packs.find((item) => item.key === packId);
  if (!pack) throw new Error('service pack not found');
  return pack;
}

export function createServicePack(input: ServicePackCreateInput): Promise<ServicePackDetail> {
  if (!input.key) return Promise.reject(new Error('service pack key is required'));
  return sendJSON<ServicePackDetail>(`/api/v1/service-packs/${encodeURIComponent(input.key)}`, 'PUT', input);
}

export function updateServicePack(packId: string, input: ServicePackUpdateInput): Promise<ServicePackDetail> {
  return sendJSON<ServicePackDetail>(`/api/v1/service-packs/${encodeURIComponent(packId)}`, 'PUT', { ...input, key: packId });
}

export function deleteServicePack(packId: string): Promise<ServicePackDetail> {
  return apiRequest<ServicePackDetail>(`/api/v1/service-packs/${encodeURIComponent(packId)}`, { method: 'DELETE' });
}

export function setServicePackEnabled(packId: string, enabled: boolean): Promise<ServicePackDetail> {
  return sendJSON<ServicePackDetail>(`/api/v1/service-packs/${encodeURIComponent(packId)}/${enabled ? 'enable' : 'disable'}`, 'POST', {});
}

export function validateServicePack(_input: ServicePackCreateInput): Promise<ServicePackValidationResult> {
  return Promise.reject(new Error('Backend has no service pack validation endpoint in this release.'));
}

export function createInstanceFromServicePack(packId: string, input: InstanceCreateFromPackInput): Promise<InstanceCreateResult> {
  return sendJSON<InstanceCreateResult>(`/api/v1/service-packs/${encodeURIComponent(packId)}/instances`, 'POST', input);
}

export function createInstanceManual(input: InstanceManualCreateInput): Promise<InstanceCreateResult> {
  return sendJSON<InstanceCreateResult>('/api/v1/instances', 'POST', input);
}

export function validateInstanceSpec(_input: InstanceSpecDraft): Promise<InstanceSpecValidationResult> {
  return Promise.reject(new Error('Backend has no separate instance spec validation endpoint in this release; save spec uses backend validation.'));
}

export function saveInstanceDraft(_instanceId: string, _input: InstanceSpecDraft): Promise<InstanceSpecValidationResult> {
  return Promise.reject(new Error('Backend has no HTTP save-draft endpoint in this release; use spec replace to create a validated revision.'));
}

export function replaceInstanceSpec(instanceId: string, input: InstanceSpecDraft): Promise<InstanceSpecValidationResult> {
  return sendJSON<InstanceSpecValidationResult>(`/api/v1/instances/${encodeURIComponent(instanceId)}/spec`, 'PUT', { spec: input.spec });
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

export function listRuntimeArtifacts(params: { includeInactive?: boolean } = {}): Promise<RuntimeArtifact[]> {
  return apiRequest<RuntimeArtifact[]>(`/api/v1/binary-artifacts${queryString({ include_inactive: params.includeInactive ? 1 : undefined })}`);
}

export async function getRuntimeArtifact(artifactId: string): Promise<RuntimeArtifact> {
  const artifacts = await listRuntimeArtifacts({ includeInactive: true });
  const artifact = artifacts.find((item) => item.id === artifactId);
  if (!artifact) throw new Error('runtime artifact not found');
  return artifact;
}

export function importRuntimeArtifact(input: RuntimeArtifactImportInput): Promise<RuntimeArtifactImportResult> {
  return sendJSON<RuntimeArtifactImportResult>('/api/v1/binary-artifacts/import-url', 'POST', input);
}

export function deleteRuntimeArtifact(_artifactId: string): Promise<RuntimeArtifactDeleteResult> {
  return Promise.reject(new Error('Backend has no binary runtime artifact delete endpoint in this release.'));
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
    listClientAccesses(clientId),
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

export function listClientDeliveryHistory(clientId: string): Promise<ClientDeliveryHistoryItem[]> {
  return apiRequest<ClientDeliveryHistoryItem[]>(`/api/v1/clients/${encodeURIComponent(clientId)}/deliveries?limit=50`);
}

export function listCertificates(): Promise<Certificate[]> {
  return apiRequest<Certificate[]>('/api/v1/platform/certificates');
}

export async function getCertificate(certificateId: string): Promise<CertificateDetail> {
  const items = await listCertificates();
  const certificate = items.find((item) => item.id === certificateId);
  if (!certificate) throw new Error(`certificate ${certificateId} was not found in platform certificate list`);
  return certificate;
}

export function previewCertificateImport(input: CertificateImportInput): Promise<CertificateImportPreview> {
  return sendJSON<CertificateImportPreview>('/api/v1/platform/certificates/preview', 'POST', input);
}

export function importCertificate(input: CertificateImportInput): Promise<Certificate> {
  return sendJSON<Certificate>('/api/v1/platform/certificates/import', 'POST', input);
}

export function createSelfSignedCertificate(input: CertificateCreateInput): Promise<Certificate> {
  return sendJSON<Certificate>('/api/v1/platform/certificates/self-signed', 'POST', input);
}

export function createManagedCertificateAuthority(input: CertificateAuthorityCreateInput): Promise<Certificate> {
  return sendJSON<Certificate>('/api/v1/platform/certificates/authorities', 'POST', input);
}

export function issueCertificate(input: CertificateIssueInput): Promise<Certificate> {
  return sendJSON<Certificate>('/api/v1/platform/certificates/issue-from-ca', 'POST', input);
}

export function setDefaultCertificate(certificateId: string): Promise<CertificateActionResult> {
  return sendJSON<CertificateActionResult>(`/api/v1/platform/certificates/${encodeURIComponent(certificateId)}/default`, 'POST', {});
}

export function revokeCertificate(certificateId: string): Promise<CertificateRevokeResult> {
  return sendJSON<CertificateRevokeResult>(`/api/v1/platform/certificates/${encodeURIComponent(certificateId)}/revoke`, 'POST', {});
}

export function deleteCertificate(certificateId: string): Promise<CertificateDeleteResult> {
  return apiRequest<CertificateDeleteResult>(`/api/v1/platform/certificates/${encodeURIComponent(certificateId)}`, { method: 'DELETE' });
}

export function listPkiRoots(): Promise<PkiRoot[]> {
  return apiRequest<PkiRoot[]>('/api/v1/platform/pki-roots');
}

export function createPkiRoot(input: PkiRootCreateInput): Promise<PkiRoot> {
  return sendJSON<PkiRoot>('/api/v1/platform/pki-roots', 'POST', input);
}

export function importPkiRoot(input: PkiRootCreateInput): Promise<PkiRoot> {
  return createPkiRoot(input);
}

export function getPlatformSettings(): Promise<PlatformSettings> {
  return apiRequest<PlatformSettings>('/api/v1/settings/control-plane-tls');
}

export function updatePlatformSettings(input: PlatformSettingsInput): Promise<SettingsUpdateResult> {
  return sendJSON<SettingsUpdateResult>('/api/v1/settings/control-plane-tls', 'PUT', input);
}

export function applyTlsSettings(): Promise<Job> {
  return sendJSON<Job>('/api/v1/settings/control-plane-tls/apply', 'POST', {});
}

export function getMailSettings(): Promise<MailSettings> {
  return apiRequest<MailSettings>('/api/v1/settings/mail');
}

export function updateMailSettings(input: MailSettingsInput): Promise<MailSettings> {
  return sendJSON<MailSettings>('/api/v1/settings/mail', 'PUT', input);
}

export function testMailSettings(input: MailTestInput): Promise<MailTestResult> {
  return sendJSON<MailTestResult>('/api/v1/settings/mail/test', 'POST', input);
}

export function listUsers(): Promise<UserAccount[]> {
  return apiRequest<UserAccount[]>('/api/v1/admin/users?limit=200');
}

export async function getUser(userId: string): Promise<UserAccount> {
  const users = await listUsers();
  const user = users.find((item) => item.id === userId);
  if (!user) throw new Error(`platform user ${userId} was not found in admin user list`);
  return user;
}

export function listInvites(): Promise<Invite[]> {
  return apiRequest<Invite[]>('/api/v1/admin/user-invites?limit=200');
}

export function createInvite(input: InviteCreateInput): Promise<InviteCreateResult> {
  return sendJSON<InviteCreateResult>('/api/v1/admin/users/invite', 'POST', input);
}

export function revokeInvite(_inviteId: string): Promise<InviteRevokeResult> {
  return Promise.reject(new Error('Backend has no platform invite revoke endpoint in this release.'));
}

export function listSessions(): Promise<Session[]> {
  return apiRequest<Session[]>('/api/v1/admin/sessions?limit=200');
}

export function revokeSession(sessionId: string): Promise<SessionRevokeResult> {
  return sendJSON<SessionRevokeResult>(`/api/v1/admin/sessions/${encodeURIComponent(sessionId)}/revoke`, 'POST', {});
}

export function listBackhaulLinks(): Promise<BackhaulLink[]> {
  return apiRequest<BackhaulLink[]>('/api/v1/backhaul-links');
}

export function getBackhaulLink(linkId: string): Promise<BackhaulLink> {
  return apiRequest<BackhaulLink>(`/api/v1/backhaul-links/${encodeURIComponent(linkId)}`);
}

export function applyBackhaulLink(linkId: string): Promise<BackhaulActionResult> {
  return sendJSON<BackhaulActionResult>(`/api/v1/backhaul-links/${encodeURIComponent(linkId)}/apply`, 'POST', {});
}

export function probeBackhaulLink(linkId: string): Promise<BackhaulActionResult> {
  return sendJSON<BackhaulActionResult>(`/api/v1/backhaul-links/${encodeURIComponent(linkId)}/probe`, 'POST', {});
}

export function promoteBackhaulLink(linkId: string, input: BackhaulPromoteInput): Promise<BackhaulActionResult> {
  return sendJSON<BackhaulActionResult>(`/api/v1/backhaul-links/${encodeURIComponent(linkId)}/promote`, 'POST', input);
}

export function updateBackhaulRouteState(linkId: string, input: BackhaulRouteStateInput): Promise<BackhaulActionResult> {
  return sendJSON<BackhaulActionResult>(`/api/v1/backhaul-links/${encodeURIComponent(linkId)}/route`, 'PATCH', input);
}

export const endpoints = {
  ready: () => apiRequest<ReadyStatus>('/api/v1/ready'),
  version: () => apiRequest<VersionInfo>('/api/v1/version'),
  dashboard: () => apiRequest<Dashboard>('/api/v1/dashboard'),
  nodes: () => listNodes(),
  instances: () => listInstances(),
  instanceRuntimeStates: () => listInstanceRuntimeStates(),
  serviceTypeCapabilities: () => listServiceTypeCapabilities(),
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
  certificates: () => listCertificates(),
  certificate: getCertificate,
  certificateImportPreview: previewCertificateImport,
  importCertificate,
  createSelfSignedCertificate,
  createManagedCertificateAuthority,
  issueCertificate,
  setDefaultCertificate,
  revokeCertificate,
  deleteCertificate,
  pkiRoots: () => listPkiRoots(),
  createPkiRoot,
  importPkiRoot,
  backhaulLinks: () => listBackhaulLinks(),
  backhaulLink: getBackhaulLink,
  applyBackhaulLink,
  probeBackhaulLink,
  promoteBackhaulLink,
  updateBackhaulRouteState,
  backhaulDrivers: () => apiRequest<Record<string, unknown>[]>('/api/v1/backhaul/drivers'),
  routePolicies: () => listRoutePolicies(),
  routePolicy: getRoutePolicy,
  previewRoutePolicy,
  applyRoutePolicy,
  cleanupRoutePolicy,
  artifacts: () => apiRequest<Artifact[]>('/api/v1/artifacts'),
  shareLinks: () => apiRequest<ShareLink[]>('/api/v1/share-links'),
  addressPools: () => apiRequest<AddressPools>('/api/v1/address-pools'),
  servicePacks: () => listServicePacks(),
  binaryArtifacts: () => listRuntimeArtifacts(),
  audit: () => apiRequest<Record<string, unknown>[]>('/api/v1/audit?limit=200'),
  runtimePreflight: () => apiRequest<Record<string, unknown>>('/api/v1/runtime/preflight'),
  controlPlaneTLS: () => getPlatformSettings(),
  updatePlatformSettings,
  applyTlsSettings,
  mailSettings: () => getMailSettings(),
  updateMailSettings,
  testMailSettings,
  users: () => listUsers(),
  user: getUser,
  invites: () => listInvites(),
  createInvite,
  revokeInvite,
  sessions: () => listSessions(),
  revokeSession,
};

export function trafficAccountingExportURL(limit = 10_000): string {
  return apiURL(`/api/v1/traffic/accounting/export?limit=${encodeURIComponent(String(limit))}`);
}
