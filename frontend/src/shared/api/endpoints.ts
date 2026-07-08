import { apiRequest, apiURL, isUnauthorized, sendJSON } from './client';
import type {
  AddressPools,
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

export const endpoints = {
  ready: () => apiRequest<ReadyStatus>('/api/v1/ready'),
  version: () => apiRequest<VersionInfo>('/api/v1/version'),
  dashboard: () => apiRequest<Dashboard>('/api/v1/dashboard'),
  nodes: () => apiRequest<NodeEntity[]>('/api/v1/nodes'),
  instances: () => apiRequest<ServiceInstance[]>('/api/v1/instances'),
  instanceRuntimeStates: () => apiRequest<Record<string, unknown>[]>('/api/v1/instances/runtime-states'),
  clients: () => apiRequest<ClientAccount[]>('/api/v1/clients'),
  clientAccessServices: listClientAccessServices,
  clientAccessGroups: () => listClientAccessGroups(),
  firewallInventory: () => apiRequest<FirewallInventory>('/api/v1/firewall'),
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
