import { apiRequest, apiURL, isUnauthorized, sendJSON } from './client';
import type {
  AddressPools,
  Artifact,
  AuthPayload,
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
} from './types';

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

export const endpoints = {
  ready: () => apiRequest<ReadyStatus>('/api/v1/ready'),
  version: () => apiRequest<VersionInfo>('/api/v1/version'),
  dashboard: () => apiRequest<Dashboard>('/api/v1/dashboard'),
  nodes: () => apiRequest<NodeEntity[]>('/api/v1/nodes'),
  instances: () => apiRequest<ServiceInstance[]>('/api/v1/instances'),
  instanceRuntimeStates: () => apiRequest<Record<string, unknown>[]>('/api/v1/instances/runtime-states'),
  clients: () => apiRequest<ClientAccount[]>('/api/v1/clients'),
  clientAccessServices: () => apiRequest<ClientAccessService[]>('/api/v1/client-access-services'),
  clientAccessGroups: () => apiRequest<ClientAccessGroup[]>('/api/v1/client-access-groups'),
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
