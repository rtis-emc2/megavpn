import type { NodeAccessMethod, NodeBootstrapRun, NodeDetail, NodeDiagnostics } from '../../shared/api/types';

export type NodeBootstrapMode = 'ssh_bootstrap' | 'manual_bundle';

export type NodeBootstrapModeAvailability = {
  mode: NodeBootstrapMode;
  available: boolean;
  recommended: boolean;
  reasonCode: string;
  warnings: string[];
  sshTarget?: {
    host?: string;
    port?: number;
    user?: string;
    enabled: boolean;
    hostKeyConfigured: boolean;
    secretConfigured: boolean;
  };
};

export type NodeBootstrapReadiness = {
  modes: NodeBootstrapModeAvailability[];
  defaultMode?: NodeBootstrapMode;
  selectionRequired: boolean;
  hasRunningBootstrap: boolean;
  hasUnknownBootstrap: boolean;
  latestRun?: NodeBootstrapRun;
  latestSuccessfulRun?: NodeBootstrapRun;
  latestFailedRun?: NodeBootstrapRun;
};

export type NodeBootstrapReadinessInput = {
  node?: NodeDetail;
  accessMethods?: NodeAccessMethod[];
  bootstrapRuns?: NodeBootstrapRun[];
  diagnostics?: NodeDiagnostics;
};

export const NODE_BOOTSTRAP_MODES: NodeBootstrapMode[] = ['ssh_bootstrap', 'manual_bundle'];

const runningStatuses = new Set(['queued', 'running', 'retrying', 'pending']);
const successfulStatuses = new Set(['succeeded', 'success', 'completed']);
const failedStatuses = new Set(['failed', 'cancelled', 'canceled']);

function normalize(value?: string | null): string {
  return String(value || '').trim().toLowerCase();
}

function timestamp(value?: string | null): number {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function runTimestamp(run?: NodeBootstrapRun): number {
  return Math.max(timestamp(run?.created_at), timestamp(run?.started_at), timestamp(run?.finished_at));
}

function latestByTime<T extends { id?: string }>(items: T[], timeOf: (item: T) => number): T | undefined {
  return [...items].sort((left, right) => {
    const diff = timeOf(right) - timeOf(left);
    if (diff !== 0) return diff;
    return String(right.id || '').localeCompare(String(left.id || ''));
  })[0];
}

function latestRunByStatus(runs: NodeBootstrapRun[], statuses: Set<string>): NodeBootstrapRun | undefined {
  return latestByTime(runs.filter((run) => statuses.has(normalize(run.status))), runTimestamp);
}

export function isNodeBootstrapTerminalStatus(status?: string | null): boolean {
  const normalized = normalize(status);
  return successfulStatuses.has(normalized) || failedStatuses.has(normalized);
}

export function isNodeBootstrapRunningStatus(status?: string | null): boolean {
  return runningStatuses.has(normalize(status));
}

export function nodeBootstrapRunStatusGroup(status?: string | null): 'running' | 'succeeded' | 'failed' | 'unknown' {
  const normalized = normalize(status);
  if (runningStatuses.has(normalized)) return 'running';
  if (successfulStatuses.has(normalized)) return 'succeeded';
  if (failedStatuses.has(normalized)) return 'failed';
  return 'unknown';
}

export function isNodeRetiredOrDeleted(node?: NodeDetail): boolean {
  const status = normalize(node?.status);
  return status === 'retired' || status === 'deleted';
}

export function nodeAccessMethodHasConfiguredSecret(method: NodeAccessMethod): boolean {
  return method.secret_configured === true || Boolean(method.secret_ref_id);
}

export function safeSSHAccessMethods(methods: NodeAccessMethod[] | undefined): NodeAccessMethod[] {
  return (methods || []).filter((method) => normalize(method.method) === 'ssh');
}

export function defaultSSHAccessMethod(methods: NodeAccessMethod[] | undefined): NodeAccessMethod | undefined {
  const ssh = safeSSHAccessMethods(methods);
  return ssh.find((method) => method.is_enabled) || ssh[0];
}

function sshReason(method?: NodeAccessMethod): string {
  if (!method) return 'ssh_access_missing';
  if (!method.is_enabled) return 'ssh_access_disabled';
  if (!String(method.ssh_host || '').trim()) return 'ssh_host_missing';
  if (!String(method.ssh_user || '').trim()) return 'ssh_user_missing';
  if (!Number.isFinite(Number(method.ssh_port)) || Number(method.ssh_port) <= 0) return 'ssh_port_missing';
  if (!String(method.ssh_host_key_sha256 || '').trim()) return 'ssh_host_key_missing';
  if (!nodeAccessMethodHasConfiguredSecret(method)) return 'ssh_credential_missing';
  return 'available';
}

function safeSSHTarget(method?: NodeAccessMethod): NodeBootstrapModeAvailability['sshTarget'] | undefined {
  if (!method) return undefined;
  return {
    host: String(method.ssh_host || '').trim() || undefined,
    port: Number.isFinite(Number(method.ssh_port)) ? Number(method.ssh_port) : undefined,
    user: String(method.ssh_user || '').trim() || undefined,
    enabled: method.is_enabled === true,
    hostKeyConfigured: Boolean(String(method.ssh_host_key_sha256 || '').trim()),
    secretConfigured: nodeAccessMethodHasConfiguredSecret(method),
  };
}

function recommendationFor(node: NodeDetail | undefined, availableModes: NodeBootstrapMode[], latestFailed?: NodeBootstrapRun): NodeBootstrapMode | undefined {
  if (availableModes.length === 0) return undefined;
  if (availableModes.length === 1) return availableModes[0];
  const failedMode = normalize(latestFailed?.bootstrap_mode);
  if (failedMode === 'ssh_bootstrap' || failedMode === 'manual_bundle') {
    return availableModes.includes(failedMode) ? failedMode : undefined;
  }
  const executionMode = normalize(node?.execution_mode);
  if (executionMode === 'ssh_bootstrap' && availableModes.includes('ssh_bootstrap')) return 'ssh_bootstrap';
  if (executionMode === 'manual_bundle' && availableModes.includes('manual_bundle')) return 'manual_bundle';
  return undefined;
}

export function deriveNodeBootstrapReadiness(input: NodeBootstrapReadinessInput): NodeBootstrapReadiness {
  const node = input.node;
  const runs = input.bootstrapRuns || [];
  const latestRun = input.diagnostics?.last_bootstrap || latestByTime(runs, runTimestamp);
  const latestSuccessfulRun = input.diagnostics?.last_successful_bootstrap || latestRunByStatus(runs, successfulStatuses);
  const latestFailedRun = input.diagnostics?.last_failed_bootstrap || latestRunByStatus(runs, failedStatuses);
  const hasRunningBootstrap = runs.some((run) => isNodeBootstrapRunningStatus(run.status)) || isNodeBootstrapRunningStatus(latestRun?.status);
  const hasUnknownBootstrap = Boolean(latestRun) && nodeBootstrapRunStatusGroup(latestRun?.status) === 'unknown';
  const retiredOrDeleted = isNodeRetiredOrDeleted(node);
  const localManaged = normalize(node?.execution_mode) === 'local_managed';
  const baseBlocked = !node
    ? 'node_missing'
    : retiredOrDeleted
      ? 'node_inactive'
      : localManaged
        ? 'local_managed'
        : hasRunningBootstrap
          ? 'bootstrap_already_active'
          : hasUnknownBootstrap
            ? 'bootstrap_state_unknown'
            : '';

  const sshMethod = defaultSSHAccessMethod(input.accessMethods);
  const sshSpecificReason = sshReason(sshMethod);
  const sshReasonCode = baseBlocked || sshSpecificReason;
  const manualReasonCode = baseBlocked || 'available';
  const availableModes = [
    sshReasonCode === 'available' ? 'ssh_bootstrap' : undefined,
    manualReasonCode === 'available' ? 'manual_bundle' : undefined,
  ].filter(Boolean) as NodeBootstrapMode[];
  const recommendedMode = recommendationFor(node, availableModes, latestFailedRun);
  const modes: NodeBootstrapModeAvailability[] = [
    {
      mode: 'ssh_bootstrap',
      available: sshReasonCode === 'available',
      recommended: recommendedMode === 'ssh_bootstrap',
      reasonCode: sshReasonCode,
      warnings: sshReasonCode === 'available' ? ['server_firewall_validation'] : [],
      sshTarget: safeSSHTarget(sshMethod),
    },
    {
      mode: 'manual_bundle',
      available: manualReasonCode === 'available',
      recommended: recommendedMode === 'manual_bundle',
      reasonCode: manualReasonCode,
      warnings: manualReasonCode === 'available' ? ['manual_execution_required', 'bundle_generation_not_execution'] : [],
    },
  ];

  return {
    modes,
    defaultMode: recommendedMode,
    selectionRequired: availableModes.length > 1 && !recommendedMode,
    hasRunningBootstrap,
    hasUnknownBootstrap,
    latestRun,
    latestSuccessfulRun,
    latestFailedRun,
  };
}
