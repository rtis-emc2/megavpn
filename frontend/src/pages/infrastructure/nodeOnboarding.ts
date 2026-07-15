import type {
  EnrollmentToken,
  NodeAccessMethod,
  NodeBootstrapRun,
  NodeDetail,
  NodeDiagnostics,
  NodeInventorySnapshot,
} from '../../shared/api/types';

export type NodeOnboardingTargetTab = 'overview' | 'runtime' | 'inventory' | 'diagnostics' | 'bootstrap' | 'security';

export type NodeOnboardingStepKey =
  | 'profile'
  | 'credential'
  | 'bootstrap'
  | 'registration'
  | 'heartbeat'
  | 'inventory';

export type NodeOnboardingStepStatus =
  | 'complete'
  | 'current'
  | 'pending'
  | 'warning'
  | 'blocked';

export type NodeOnboardingOverallStatus =
  | 'not_started'
  | 'in_progress'
  | 'action_required'
  | 'ready'
  | 'degraded'
  | 'blocked';

export type NodeOnboardingEvidence = {
  key: string;
  value: string;
};

export type NodeOnboardingStep = {
  key: NodeOnboardingStepKey;
  status: NodeOnboardingStepStatus;
  evidenceCode: string;
  evidence: NodeOnboardingEvidence[];
  timestamp?: string;
  targetTab?: NodeOnboardingTargetTab;
};

export type NodeOnboardingModel = {
  overallStatus: NodeOnboardingOverallStatus;
  currentStep: NodeOnboardingStepKey;
  steps: NodeOnboardingStep[];
  communicationState: string;
  heartbeatState: string;
  tokenRotationStatus: string;
  targetTab?: NodeOnboardingTargetTab;
  registered: boolean;
  heartbeatObserved: boolean;
  inventoryObserved: boolean;
};

export type NodeOnboardingInput = {
  node?: NodeDetail;
  diagnostics?: NodeDiagnostics;
  enrollmentTokens?: EnrollmentToken[];
  bootstrapRuns?: NodeBootstrapRun[];
  inventory?: NodeInventorySnapshot;
  accessMethods?: NodeAccessMethod[];
};

const terminalCommunicationStates = new Set(['inventory_ok', 'discovery_ok', 'healthy']);
const strongActionCommunicationStates = new Set(['auth_failure', 'heartbeat_stalled', 'channel_offline', 'job_result_stalled']);
const unhealthyCommunicationStates = new Set(['degraded', 'auth_failure', 'heartbeat_stalled', 'channel_offline', 'job_result_stalled']);
const inProgressBootstrapStatuses = new Set(['queued', 'running', 'retrying', 'pending']);
const successfulBootstrapStatuses = new Set(['succeeded', 'success', 'completed']);
const failedBootstrapStatuses = new Set(['failed', 'cancelled', 'canceled']);

function normalize(value?: string | null): string {
  return String(value || '').trim().toLowerCase();
}

function present(value?: string | null): string | undefined {
  const trimmed = String(value || '').trim();
  return trimmed || undefined;
}

function timestamp(value?: string | null): number {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function latestByCreatedAt<T extends { created_at?: string | null }>(items: T[]): T | undefined {
  return [...items].sort((left, right) => timestamp(right.created_at) - timestamp(left.created_at))[0];
}

function latestBootstrapByStatus(runs: NodeBootstrapRun[], statuses: Set<string>): NodeBootstrapRun | undefined {
  return latestByCreatedAt(runs.filter((run) => statuses.has(normalize(run.status))));
}

function isRetiredNode(node?: NodeDetail): boolean {
  const status = normalize(node?.status);
  return status === 'retired' || status === 'deleted';
}

function isMaintenanceNode(node?: NodeDetail): boolean {
  return normalize(node?.status) === 'maintenance';
}

function safeActiveEnrollmentToken(diagnostics?: NodeDiagnostics, tokens: EnrollmentToken[] = []): EnrollmentToken | undefined {
  const diagnosticToken = diagnostics?.active_enrollment_token;
  if (diagnosticToken && normalize(diagnosticToken.status) === 'active') {
    return diagnosticToken;
  }
  return tokens.find((token) => normalize(token.status) === 'active');
}

function latestBootstrapRun(diagnostics?: NodeDiagnostics, runs: NodeBootstrapRun[] = []): NodeBootstrapRun | undefined {
  return diagnostics?.last_bootstrap || latestByCreatedAt(runs);
}

function latestSuccessfulBootstrapRun(diagnostics?: NodeDiagnostics, runs: NodeBootstrapRun[] = []): NodeBootstrapRun | undefined {
  return diagnostics?.last_successful_bootstrap || latestBootstrapByStatus(runs, successfulBootstrapStatuses);
}

function latestFailedBootstrapRun(diagnostics?: NodeDiagnostics, runs: NodeBootstrapRun[] = []): NodeBootstrapRun | undefined {
  return diagnostics?.last_failed_bootstrap || latestBootstrapByStatus(runs, failedBootstrapStatuses);
}

function bootstrapMode(run?: NodeBootstrapRun): string | undefined {
  return present(run?.bootstrap_mode);
}

function bootstrapTimestamp(run?: NodeBootstrapRun): string | undefined {
  return run?.finished_at || run?.started_at || run?.created_at || undefined;
}

function safeTokenEvidence(token?: EnrollmentToken): NodeOnboardingEvidence[] {
  if (!token) return [];
  return [
    { key: 'token_status', value: token.status || 'unknown' },
    { key: 'token_hint', value: token.token_hint || 'n/a' },
    { key: 'token_expires_at', value: token.expires_at || 'n/a' },
    { key: 'token_used_at', value: token.used_at || 'n/a' },
  ];
}

function inventoryJobFailed(diagnostics?: NodeDiagnostics): boolean {
  const agent = diagnostics?.agent;
  return normalize(agent?.last_job_result_status) === 'failed' && normalize(agent?.last_job_result_type).includes('inventory');
}

function step(
  key: NodeOnboardingStepKey,
  status: NodeOnboardingStepStatus,
  evidenceCode: string,
  evidence: NodeOnboardingEvidence[] = [],
  timestampValue?: string,
  targetTab?: NodeOnboardingTargetTab,
): NodeOnboardingStep {
  return {
    key,
    status,
    evidenceCode,
    evidence,
    timestamp: timestampValue || undefined,
    targetTab,
  };
}

function firstProblemStep(steps: NodeOnboardingStep[]): NodeOnboardingStep {
  return steps.find((item) => item.status === 'blocked')
    || steps.find((item) => item.status === 'warning')
    || steps.find((item) => item.status === 'current')
    || steps.find((item) => item.status === 'pending')
    || steps[steps.length - 1];
}

export function shouldPollNodeOnboarding(model: NodeOnboardingModel): boolean {
  return model.overallStatus !== 'ready' && model.overallStatus !== 'blocked';
}

export function deriveNodeOnboardingModel(input: NodeOnboardingInput): NodeOnboardingModel {
  const node = input.node;
  const diagnostics = input.diagnostics;
  const tokens = input.enrollmentTokens || [];
  const runs = input.bootstrapRuns || [];
  const inventory = input.inventory;
  const agent = diagnostics?.agent;
  const retired = isRetiredNode(node);
  const communicationState = normalize(diagnostics?.communication_state) || 'unknown';
  const heartbeatState = normalize(diagnostics?.heartbeat_state) || normalize(node?.agent_status) || 'unknown';
  const tokenRotationStatus = normalize(agent?.token_rotation_status) || 'missing';
  const activeToken = safeActiveEnrollmentToken(diagnostics, tokens);
  const latestRun = latestBootstrapRun(diagnostics, runs);
  const latestSuccess = latestSuccessfulBootstrapRun(diagnostics, runs);
  const latestFailed = latestFailedBootstrapRun(diagnostics, runs);
  const latestRunStatus = normalize(latestRun?.status);
  const latestFailureIsCurrent = Boolean(latestFailed) && (!latestSuccess || timestamp(latestFailed?.created_at) >= timestamp(latestSuccess?.created_at));
  const latestSuccessIsCurrent = Boolean(latestSuccess) && (!latestRun || timestamp(latestSuccess?.created_at) >= timestamp(latestRun?.created_at));
  const agentStatus = normalize(agent?.status) || normalize(node?.agent_status) || 'unknown';
  const agentRevoked = agentStatus === 'revoked' || Boolean(agent?.revoked_at) || tokenRotationStatus === 'revoked';
  const registered = Boolean(agent?.registered_at) && !agentRevoked;
  const heartbeatTimestamp = node?.last_heartbeat_at || agent?.last_seen_at || node?.agent_last_seen_at || undefined;
  const heartbeatObserved = Boolean(heartbeatTimestamp);
  const inventoryObserved = Boolean(diagnostics?.latest_inventory?.id || inventory?.id || agent?.last_inventory_sync_at);
  const authOrCredentialProblem = communicationState === 'auth_failure' || tokenRotationStatus === 'revoked';
  const bootstrapInProgress = inProgressBootstrapStatuses.has(latestRunStatus);
  const bootstrapSucceeded = latestSuccessIsCurrent;

  const profileStep = retired
    ? step('profile', 'blocked', 'profile_retired', [
      { key: 'node_name', value: node?.name || node?.id || 'n/a' },
      { key: 'node_status', value: node?.status || 'retired' },
    ], node?.updated_at, 'overview')
    : step('profile', node ? (isMaintenanceNode(node) ? 'warning' : 'complete') : 'current', node ? (isMaintenanceNode(node) ? 'profile_maintenance' : 'profile_loaded') : 'profile_missing', [
      { key: 'node_name', value: node?.name || node?.id || 'n/a' },
      { key: 'execution_mode', value: node?.execution_mode || 'n/a' },
    ], node?.updated_at || node?.created_at, 'overview');

  let credentialStep: NodeOnboardingStep;
  if (retired) {
    credentialStep = step('credential', 'blocked', 'credential_node_blocked', [], undefined, 'overview');
  } else if (authOrCredentialProblem) {
    credentialStep = step('credential', 'warning', communicationState === 'auth_failure' ? 'credential_auth_failure' : 'credential_revoked', safeTokenEvidence(activeToken), agent?.last_auth_failure_at || activeToken?.created_at, communicationState === 'auth_failure' ? 'diagnostics' : 'security');
  } else if (registered) {
    credentialStep = step('credential', 'complete', 'credential_registered', [
      { key: 'agent_status', value: agentStatus },
      { key: 'token_rotation_status', value: tokenRotationStatus },
      { key: 'token_hint', value: agent?.token_hint || activeToken?.token_hint || 'n/a' },
    ], agent?.registered_at, 'runtime');
  } else if (activeToken) {
    credentialStep = step('credential', 'complete', 'credential_active_token', safeTokenEvidence(activeToken), activeToken.created_at, 'security');
  } else {
    credentialStep = step('credential', 'current', 'credential_waiting_for_token', [], undefined, 'security');
  }

  let bootstrapStep: NodeOnboardingStep;
  if (retired) {
    bootstrapStep = step('bootstrap', 'blocked', 'bootstrap_node_blocked', [], undefined, 'overview');
  } else if (registered) {
    bootstrapStep = step('bootstrap', 'complete', 'bootstrap_registered_path', [
      { key: 'agent_status', value: agentStatus },
      { key: 'bootstrap_mode', value: bootstrapMode(latestRun) || 'n/a' },
    ], agent?.registered_at, 'runtime');
  } else if (bootstrapSucceeded && !latestFailureIsCurrent) {
    bootstrapStep = step('bootstrap', 'complete', 'bootstrap_successful', [
      { key: 'bootstrap_mode', value: bootstrapMode(latestSuccess) || 'n/a' },
      { key: 'run_status', value: latestSuccess?.status || 'succeeded' },
    ], bootstrapTimestamp(latestSuccess), 'bootstrap');
  } else if (bootstrapInProgress) {
    bootstrapStep = step('bootstrap', 'current', latestRunStatus === 'running' ? 'bootstrap_running' : 'bootstrap_queued', [
      { key: 'bootstrap_mode', value: bootstrapMode(latestRun) || 'n/a' },
      { key: 'run_status', value: latestRun?.status || 'unknown' },
    ], bootstrapTimestamp(latestRun), 'bootstrap');
  } else if (latestFailureIsCurrent) {
    bootstrapStep = step('bootstrap', 'warning', normalize(latestFailed?.status) === 'cancelled' || normalize(latestFailed?.status) === 'canceled' ? 'bootstrap_cancelled' : 'bootstrap_failed', [
      { key: 'bootstrap_mode', value: bootstrapMode(latestFailed) || 'n/a' },
      { key: 'run_status', value: latestFailed?.status || 'failed' },
    ], bootstrapTimestamp(latestFailed), 'bootstrap');
  } else if (latestRun && latestRunStatus && !successfulBootstrapStatuses.has(latestRunStatus)) {
    bootstrapStep = step('bootstrap', 'pending', 'bootstrap_unknown_status', [
      { key: 'bootstrap_mode', value: bootstrapMode(latestRun) || 'n/a' },
      { key: 'run_status', value: latestRun.status || 'unknown' },
    ], bootstrapTimestamp(latestRun), 'bootstrap');
  } else {
    bootstrapStep = step('bootstrap', 'pending', 'bootstrap_waiting', [], undefined, 'bootstrap');
  }

  let registrationStep: NodeOnboardingStep;
  if (retired) {
    registrationStep = step('registration', 'blocked', 'registration_node_blocked', [], undefined, 'overview');
  } else if (agentRevoked || communicationState === 'auth_failure') {
    registrationStep = step('registration', 'warning', agentRevoked ? 'registration_revoked' : 'registration_auth_failure', [
      { key: 'agent_status', value: agentStatus },
      { key: 'token_rotation_status', value: tokenRotationStatus },
    ], agent?.revoked_at || agent?.last_auth_failure_at || agent?.registered_at, 'diagnostics');
  } else if (registered) {
    registrationStep = step('registration', 'complete', 'registration_registered', [
      { key: 'agent_status', value: agentStatus },
      { key: 'agent_version', value: agent?.agent_version || node?.agent_version || 'n/a' },
      { key: 'protocol_version', value: agent?.protocol_version || node?.agent_protocol_version || 'n/a' },
    ], agent?.registered_at, 'runtime');
  } else if (activeToken || bootstrapInProgress || bootstrapSucceeded) {
    registrationStep = step('registration', 'current', 'registration_waiting', [
      { key: 'token_rotation_status', value: tokenRotationStatus },
      { key: 'bootstrap_status', value: latestRun?.status || 'n/a' },
    ], latestRun?.created_at || activeToken?.created_at, 'runtime');
  } else {
    registrationStep = step('registration', 'pending', 'registration_pending', [], undefined, 'runtime');
  }

  let heartbeatStep: NodeOnboardingStep;
  if (retired) {
    heartbeatStep = step('heartbeat', 'blocked', 'heartbeat_node_blocked', [], undefined, 'overview');
  } else if (registered && heartbeatObserved && heartbeatState === 'online') {
    heartbeatStep = step('heartbeat', 'complete', 'heartbeat_online', [
      { key: 'heartbeat_state', value: heartbeatState },
      { key: 'heartbeat_drift_seconds', value: diagnostics?.heartbeat_drift_seconds == null ? 'n/a' : String(diagnostics.heartbeat_drift_seconds) },
    ], heartbeatTimestamp, 'runtime');
  } else if ((heartbeatObserved && (heartbeatState === 'degraded' || heartbeatState === 'offline')) || ['heartbeat_stalled', 'channel_offline', 'auth_failure'].includes(communicationState)) {
    heartbeatStep = step('heartbeat', 'warning', communicationState === 'heartbeat_stalled' ? 'heartbeat_stalled' : communicationState === 'channel_offline' ? 'heartbeat_channel_offline' : communicationState === 'auth_failure' ? 'heartbeat_auth_failure' : heartbeatState === 'offline' ? 'heartbeat_offline' : 'heartbeat_degraded', [
      { key: 'heartbeat_state', value: heartbeatState },
      { key: 'communication_state', value: communicationState },
      { key: 'heartbeat_drift_seconds', value: diagnostics?.heartbeat_drift_seconds == null ? 'n/a' : String(diagnostics.heartbeat_drift_seconds) },
    ], heartbeatTimestamp, communicationState === 'auth_failure' || communicationState === 'channel_offline' ? 'diagnostics' : 'runtime');
  } else if (registered) {
    heartbeatStep = step('heartbeat', 'current', 'heartbeat_waiting', [
      { key: 'heartbeat_state', value: heartbeatState },
    ], agent?.registered_at, 'runtime');
  } else {
    heartbeatStep = step('heartbeat', 'pending', 'heartbeat_pending', [], undefined, 'runtime');
  }

  let inventoryStep: NodeOnboardingStep;
  if (retired) {
    inventoryStep = step('inventory', 'blocked', 'inventory_node_blocked', [], undefined, 'overview');
  } else if (communicationState === 'job_result_stalled' || communicationState === 'auth_failure' || communicationState === 'channel_offline' || inventoryJobFailed(diagnostics)) {
    inventoryStep = step('inventory', 'warning', communicationState === 'job_result_stalled' ? 'inventory_job_result_stalled' : communicationState === 'auth_failure' ? 'inventory_auth_failure' : communicationState === 'channel_offline' ? 'inventory_channel_offline' : 'inventory_job_failed', [
      { key: 'communication_state', value: communicationState },
      { key: 'last_inventory_sync_at', value: agent?.last_inventory_sync_at || 'n/a' },
    ], agent?.last_job_result_at || agent?.last_inventory_sync_at, 'diagnostics');
  } else if (inventoryObserved) {
    inventoryStep = step('inventory', 'complete', 'inventory_synchronized', [
      { key: 'inventory_id', value: diagnostics?.latest_inventory?.id || inventory?.id || 'n/a' },
      { key: 'last_inventory_sync_at', value: agent?.last_inventory_sync_at || diagnostics?.latest_inventory?.created_at || inventory?.created_at || 'n/a' },
      { key: 'communication_state', value: communicationState },
    ], agent?.last_inventory_sync_at || diagnostics?.latest_inventory?.created_at || inventory?.created_at, 'inventory');
  } else if (registered && heartbeatObserved) {
    inventoryStep = step('inventory', 'current', 'inventory_waiting', [
      { key: 'communication_state', value: communicationState },
    ], heartbeatTimestamp, 'inventory');
  } else {
    inventoryStep = step('inventory', 'pending', 'inventory_pending', [], undefined, 'inventory');
  }

  const steps = [profileStep, credentialStep, bootstrapStep, registrationStep, heartbeatStep, inventoryStep];
  const problem = firstProblemStep(steps);
  const hasBootstrapRun = Boolean(latestRun);
  const hasFailedBootstrapAction = latestFailureIsCurrent && !registered;
  let overallStatus: NodeOnboardingOverallStatus;

  if (retired) {
    overallStatus = 'blocked';
  } else if (registered && heartbeatObserved && inventoryObserved && terminalCommunicationStates.has(communicationState)) {
    overallStatus = 'ready';
  } else if (strongActionCommunicationStates.has(communicationState) || agentRevoked || hasFailedBootstrapAction) {
    overallStatus = 'action_required';
  } else if (registered && heartbeatObserved && inventoryObserved && (unhealthyCommunicationStates.has(communicationState) || heartbeatState === 'degraded' || heartbeatState === 'offline')) {
    overallStatus = 'degraded';
  } else if (!activeToken && !hasBootstrapRun && !agent?.registered_at) {
    overallStatus = 'not_started';
  } else {
    overallStatus = 'in_progress';
  }

  const targetTab = overallStatus === 'ready'
    ? 'runtime'
    : strongActionCommunicationStates.has(communicationState)
      ? 'diagnostics'
      : problem.targetTab;

  return {
    overallStatus,
    currentStep: problem.key,
    steps,
    communicationState,
    heartbeatState,
    tokenRotationStatus,
    targetTab,
    registered,
    heartbeatObserved,
    inventoryObserved,
  };
}
