import { APIError } from '../../shared/api/client';
import type {
  Job,
  NodeDetail,
  NodeDiagnostics,
  NodeEmergencyCleanupInput,
  NodeEmergencyCleanupPlanSummary,
  NodeEmergencyCleanupScope,
} from '../../shared/api/types';
import { normalizeLifecycleStatus } from './nodeLifecycleControls';

export const NODE_EMERGENCY_CLEANUP_MAX_CONFIRMATION_LENGTH = 512;
export const NODE_EMERGENCY_CLEANUP_MIN_REASON_LENGTH = 5;
export const NODE_EMERGENCY_CLEANUP_MAX_REASON_LENGTH = 500;

export type NodeEmergencyCleanupForm = {
  cleanupScope: NodeEmergencyCleanupScope | '';
  includeAgent: boolean;
  confirmation: string;
  reason: string;
  acknowledgeDestructiveCleanup: boolean;
  acknowledgeAgentRemoval: boolean;
};

export type NodeEmergencyCleanupValidation = {
  valid: boolean;
  expectedConfirmation: string;
  input?: NodeEmergencyCleanupInput;
  errors: {
    cleanupScope?: string;
    includeAgent?: string;
    confirmation?: string;
    reason?: string;
    acknowledgeDestructiveCleanup?: string;
    acknowledgeAgentRemoval?: string;
  };
};

export type NodeEmergencyCleanupActionState = {
  available: boolean;
  blockedKey?: string;
  unknownState: boolean;
};

export type SafeQueuedNodeEmergencyCleanupResult = {
  job: Pick<Job, 'id' | 'type' | 'status' | 'created_at' | 'node_id' | 'scope_id'>;
  planSummary: NodeEmergencyCleanupPlanSummary;
};

const terminalNodeStatuses = new Set(['deleted', 'deleting', 'retired', 'terminated']);
const unavailableCommunicationStates = new Set([
  'auth_failed',
  'authentication_failed',
  'blocked',
  'disconnected',
  'failed',
  'offline',
  'revoked',
  'unavailable',
]);
const availableCommunicationStates = new Set(['active', 'available', 'connected', 'healthy', 'online', 'ready', 'running']);
const missingAgentStatuses = new Set(['deleted', 'missing', 'none', 'not_found']);
const unsafeReasonMarkers = [
  'authorization:',
  'bearer ',
  'agent_token',
  'enrollment_token',
  'token_hash',
  'private_key',
  'secret_ref',
  'credential',
  'x-megavpn-agent-signature',
  'x-megavpn-agent-nonce',
];
const safeServiceCodePattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;

function hasControlCharacter(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code < 32 || code === 127) return true;
  }
  return false;
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function safeOptionalTimestamp(value: unknown): string | undefined | null {
  if (value == null || value === '') return undefined;
  if (typeof value !== 'string' || value.length > 64 || hasControlCharacter(value)) return null;
  return Number.isNaN(new Date(value).getTime()) ? null : value;
}

function apiErrorCode(error: APIError): string {
  if (!isPlainRecord(error.payload)) return '';
  return typeof error.payload.code === 'string' ? error.payload.code : '';
}

export function nodeEmergencyCleanupExpectedConfirmation(node: Pick<NodeDetail, 'id' | 'name'>): string {
  return String(node.name || '').trim() || node.id;
}

export function resetNodeEmergencyCleanupScopeRelationship(
  form: NodeEmergencyCleanupForm,
  cleanupScope: NodeEmergencyCleanupScope | '',
): NodeEmergencyCleanupForm {
  if (cleanupScope === 'full_node') return { ...form, cleanupScope };
  return {
    ...form,
    cleanupScope,
    includeAgent: false,
    acknowledgeAgentRemoval: false,
  };
}

export function reasonLooksUnsafeForNodeEmergencyCleanup(reason: string): boolean {
  if (hasControlCharacter(reason) || reason.includes('{') || reason.includes('}')) return true;
  const lower = reason.toLowerCase();
  return unsafeReasonMarkers.some((marker) => lower.includes(marker));
}

export function validateNodeEmergencyCleanupForm(
  node: Pick<NodeDetail, 'id' | 'name'>,
  form: NodeEmergencyCleanupForm,
): NodeEmergencyCleanupValidation {
  const expectedConfirmation = nodeEmergencyCleanupExpectedConfirmation(node);
  const confirmation = form.confirmation.trim();
  const reason = form.reason.trim();
  const errors: NodeEmergencyCleanupValidation['errors'] = {};

  if (form.cleanupScope !== 'services_only' && form.cleanupScope !== 'full_node') {
    errors.cleanupScope = 'nodes.lifecycleControls.emergencyCleanup.errors.scopeRequired';
  }
  if (form.includeAgent && form.cleanupScope !== 'full_node') {
    errors.includeAgent = 'nodes.lifecycleControls.emergencyCleanup.errors.includeAgentRequiresFullNode';
  }

  if (!confirmation) {
    errors.confirmation = 'nodes.lifecycleControls.emergencyCleanup.errors.confirmationRequired';
  } else if (confirmation.length > NODE_EMERGENCY_CLEANUP_MAX_CONFIRMATION_LENGTH) {
    errors.confirmation = 'nodes.lifecycleControls.emergencyCleanup.errors.confirmationTooLong';
  } else if (hasControlCharacter(confirmation)) {
    errors.confirmation = 'nodes.lifecycleControls.emergencyCleanup.errors.confirmationUnsafe';
  } else if (confirmation !== expectedConfirmation) {
    errors.confirmation = 'nodes.lifecycleControls.emergencyCleanup.errors.confirmationMismatch';
  }

  if (!reason) {
    errors.reason = 'nodes.lifecycleControls.emergencyCleanup.errors.reasonRequired';
  } else if (reason.length < NODE_EMERGENCY_CLEANUP_MIN_REASON_LENGTH) {
    errors.reason = 'nodes.lifecycleControls.emergencyCleanup.errors.reasonTooShort';
  } else if (reason.length > NODE_EMERGENCY_CLEANUP_MAX_REASON_LENGTH) {
    errors.reason = 'nodes.lifecycleControls.emergencyCleanup.errors.reasonTooLong';
  } else if (reasonLooksUnsafeForNodeEmergencyCleanup(reason)) {
    errors.reason = 'nodes.lifecycleControls.emergencyCleanup.errors.reasonUnsafe';
  }

  if (!form.acknowledgeDestructiveCleanup) {
    errors.acknowledgeDestructiveCleanup = 'nodes.lifecycleControls.emergencyCleanup.errors.destructiveAcknowledgementRequired';
  }
  if (form.includeAgent && !form.acknowledgeAgentRemoval) {
    errors.acknowledgeAgentRemoval = 'nodes.lifecycleControls.emergencyCleanup.errors.agentRemovalAcknowledgementRequired';
  }
  if (!form.includeAgent && form.acknowledgeAgentRemoval) {
    errors.acknowledgeAgentRemoval = 'nodes.lifecycleControls.emergencyCleanup.errors.unexpectedAgentRemovalAcknowledgement';
  }

  const valid = Object.keys(errors).length === 0;
  return {
    valid,
    expectedConfirmation,
    input: valid ? {
      cleanup_scope: form.cleanupScope as NodeEmergencyCleanupScope,
      include_agent: form.includeAgent,
      confirmation,
      reason,
      acknowledge_destructive_cleanup: true,
      acknowledge_agent_removal: form.includeAgent,
    } : undefined,
    errors,
  };
}

export function deriveNodeEmergencyCleanupActionState(input: {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  canBootstrapNode: boolean;
  lifecycleDataCurrent: boolean;
}): NodeEmergencyCleanupActionState {
  const { node, diagnostics, canBootstrapNode, lifecycleDataCurrent } = input;
  const nodeStatus = normalizeLifecycleStatus(node.status);
  const agentStatus = normalizeLifecycleStatus(diagnostics?.agent?.status || node.agent_status || '');
  const communicationState = normalizeLifecycleStatus(diagnostics?.communication_state || node.agent_channel_status || '');
  const unknownState = !agentStatus || agentStatus === 'unknown' || !communicationState || communicationState === 'unknown';

  if (!canBootstrapNode) {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.permissionRequired', unknownState };
  }
  if (!lifecycleDataCurrent) {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.lifecycleDataStale', unknownState: true };
  }
  if (terminalNodeStatuses.has(nodeStatus)) {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.terminalNode', unknownState };
  }
  if (nodeStatus !== 'maintenance') {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.maintenanceRequired', unknownState };
  }
  if (diagnostics?.agent?.revoked_at || agentStatus === 'revoked') {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.identityRevoked', unknownState };
  }
  if (missingAgentStatuses.has(agentStatus)) {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.identityMissing', unknownState };
  }
  if (agentStatus !== 'active') {
    return { available: false, blockedKey: unknownState
      ? 'nodes.lifecycleControls.emergencyCleanup.blocked.stateIncomplete'
      : 'nodes.lifecycleControls.emergencyCleanup.blocked.agentUnavailable', unknownState };
  }
  if (unavailableCommunicationStates.has(communicationState)) {
    return { available: false, blockedKey: communicationState.includes('auth')
      ? 'nodes.lifecycleControls.emergencyCleanup.blocked.authenticationFailure'
      : 'nodes.lifecycleControls.emergencyCleanup.blocked.channelUnavailable', unknownState };
  }
  if (!availableCommunicationStates.has(communicationState)) {
    return { available: false, blockedKey: 'nodes.lifecycleControls.emergencyCleanup.blocked.stateIncomplete', unknownState: true };
  }
  return { available: true, unknownState: false };
}

export function validateQueuedNodeEmergencyCleanupResult(
  value: unknown,
  nodeId: string,
  input: NodeEmergencyCleanupInput,
): SafeQueuedNodeEmergencyCleanupResult | null {
  if (!isPlainRecord(value) || value.status !== 'queued' || !isPlainRecord(value.job) || !isPlainRecord(value.plan_summary)) return null;
  const job = value.job;
  const plan = value.plan_summary;
  const jobID = typeof job.id === 'string' ? job.id.trim() : '';
  const createdAt = safeOptionalTimestamp(job.created_at);
  if (!jobID || jobID.length > 256 || hasControlCharacter(jobID)) return null;
  if (createdAt === null) return null;
  if (job.type !== 'node.emergency_cleanup' || job.status !== 'queued') return null;
  if (job.node_id != null && job.node_id !== nodeId) return null;
  if (job.scope_id != null && job.scope_id !== nodeId) return null;
  if (plan.cleanup_scope !== input.cleanup_scope || plan.include_agent !== input.include_agent) return null;
  if (!Number.isSafeInteger(plan.instance_target_count) || Number(plan.instance_target_count) < 0) return null;
  if (!isPlainRecord(plan.service_counts)) return null;
  if (typeof plan.node_runtime_cleanup !== 'boolean' || plan.node_runtime_cleanup !== (input.cleanup_scope === 'full_node')) return null;
  if (typeof plan.agent_removal_requested !== 'boolean' || plan.agent_removal_requested !== input.include_agent) return null;

  const serviceCounts: Record<string, number> = {};
  for (const [serviceCode, count] of Object.entries(plan.service_counts)) {
    if (!safeServiceCodePattern.test(serviceCode) || !Number.isSafeInteger(count) || Number(count) < 0) return null;
    serviceCounts[serviceCode] = Number(count);
  }

  return {
    job: {
      id: jobID,
      type: 'node.emergency_cleanup',
      status: 'queued',
      created_at: createdAt,
      node_id: typeof job.node_id === 'string' ? job.node_id : undefined,
      scope_id: typeof job.scope_id === 'string' ? job.scope_id : undefined,
    },
    planSummary: {
      cleanup_scope: input.cleanup_scope,
      include_agent: input.include_agent,
      instance_target_count: Number(plan.instance_target_count),
      service_counts: serviceCounts,
      node_runtime_cleanup: plan.node_runtime_cleanup,
      agent_removal_requested: plan.agent_removal_requested,
    },
  };
}

export function nodeEmergencyCleanupErrorKey(error: unknown): string {
  if (!(error instanceof APIError)) return 'nodes.lifecycleControls.emergencyCleanup.errors.generic';
  if (error.status === 403) return 'nodes.lifecycleControls.emergencyCleanup.errors.permissionRequired';
  const code = apiErrorCode(error);
  const codeKeys: Record<string, string> = {
    node_emergency_cleanup_request_invalid: 'requestInvalid',
    node_emergency_cleanup_node_not_found: 'nodeNotFound',
    node_emergency_cleanup_confirmation_mismatch: 'confirmationMismatchBackend',
    node_emergency_cleanup_maintenance_required: 'maintenanceRequiredBackend',
    node_emergency_cleanup_agent_missing: 'agentMissing',
    node_emergency_cleanup_agent_unavailable: 'agentUnavailable',
    node_emergency_cleanup_scope_invalid: 'scopeInvalid',
    node_emergency_cleanup_acknowledgement_required: 'acknowledgementInvalid',
    node_emergency_cleanup_plan_invalid: 'planInvalid',
    node_emergency_cleanup_conflict: 'conflict',
    node_emergency_cleanup_internal_error: 'serviceUnavailable',
  };
  if (codeKeys[code]) return `nodes.lifecycleControls.emergencyCleanup.errors.${codeKeys[code]}`;
  switch (error.status) {
    case 400:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.requestInvalid';
    case 404:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.nodeNotFound';
    case 409:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.conflict';
    case 422:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.planInvalid';
    case 429:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.rateLimited';
    case 500:
    case 503:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.serviceUnavailable';
    default:
      return 'nodes.lifecycleControls.emergencyCleanup.errors.generic';
  }
}
