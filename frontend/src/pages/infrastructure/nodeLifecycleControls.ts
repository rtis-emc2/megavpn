import { APIError } from '../../shared/api/client';
import type { NodeAgentIdentityRevokeInput, NodeDetail, NodeDiagnostics, NodeStaleRotationCandidate, NodeStaleRotationPreview } from '../../shared/api/types';

export type NodeLifecycleSeverity = 'healthy' | 'warning' | 'blocked' | 'neutral';

export type NodeLifecycleStatusItem = {
  key: 'node_status' | 'agent_status' | 'heartbeat_state' | 'communication_state' | 'token_rotation_status';
  labelKey: string;
  value: string;
  severity: NodeLifecycleSeverity;
};

export type NodeLifecycleStatusModel = {
  nodeId: string;
  overallSeverity: NodeLifecycleSeverity;
  overallStatusKey: string;
  items: NodeLifecycleStatusItem[];
  staleRotation: {
    detected: boolean;
    candidateCount: number;
    backendSafeCandidateCount: number;
    unknownReasonCount: number;
    evaluatedAt?: string;
    tokenRotationStatus: string;
    severity: NodeLifecycleSeverity;
  };
};

export type NodeStaleRotationReasonDescriptor = {
  reason: string;
  labelKey: string;
  severity: NodeLifecycleSeverity;
  known: boolean;
};

export type NodeAgentIdentityRevokeForm = {
  confirmation: string;
  reason: string;
  acknowledged: boolean;
};

export type NodeAgentIdentityRevokeValidation = {
  valid: boolean;
  expectedConfirmation: string;
  input?: NodeAgentIdentityRevokeInput;
  errors: {
    confirmation?: string;
    reason?: string;
    acknowledgement?: string;
  };
};

export const NODE_AGENT_IDENTITY_REVOKE_MAX_CONFIRMATION_LENGTH = 512;
export const NODE_AGENT_IDENTITY_REVOKE_MIN_REASON_LENGTH = 5;
export const NODE_AGENT_IDENTITY_REVOKE_MAX_REASON_LENGTH = 500;

const healthyStatuses = new Set(['active', 'available', 'connected', 'healthy', 'ok', 'online', 'ready', 'registered', 'running', 'succeeded']);
const warningStatuses = new Set(['pending', 'queued', 'rotating', 'starting', 'stale', 'unknown', 'warn', 'warning']);
const blockedStatuses = new Set(['blocked', 'deleted', 'disabled', 'failed', 'offline', 'retired', 'revoked', 'unavailable']);
const unsafeReasonMarkers = [
  'authorization:',
  'bearer ',
  'agent_token',
  'enrollment_token',
  'token_hash',
  'private_key',
  'secret_ref',
  'x-megavpn-agent-signature',
  'x-megavpn-agent-nonce',
];

export const NODE_STALE_ROTATION_REASON_LABEL_KEYS: Record<string, string> = {
  unclaimed_without_agent_progress: 'nodes.lifecycleControls.reasons.unclaimedWithoutAgentProgress',
  claimed_without_result_and_agent_inactive: 'nodes.lifecycleControls.reasons.claimedWithoutResultAndAgentInactive',
  fresh_rotation: 'nodes.lifecycleControls.reasons.freshRotation',
  agent_progress_after_creation: 'nodes.lifecycleControls.reasons.agentProgressAfterCreation',
  agent_progress_after_claim: 'nodes.lifecycleControls.reasons.agentProgressAfterClaim',
  claim_or_lease_still_active: 'nodes.lifecycleControls.reasons.claimOrLeaseStillActive',
  result_already_submitted: 'nodes.lifecycleControls.reasons.resultAlreadySubmitted',
  superseded_by_newer_rotation: 'nodes.lifecycleControls.reasons.supersededByNewerRotation',
  claim_evidence_missing: 'nodes.lifecycleControls.reasons.claimEvidenceMissing',
  agent_identity_not_active: 'nodes.lifecycleControls.reasons.agentIdentityNotActive',
  evidence_ambiguous: 'nodes.lifecycleControls.reasons.evidenceAmbiguous',
  pending_state_ambiguous: 'nodes.lifecycleControls.reasons.pendingStateAmbiguous',
};

const safeReasonSet = new Set([
  'unclaimed_without_agent_progress',
  'claimed_without_result_and_agent_inactive',
]);

const blockedReasonSet = new Set([
  'claim_evidence_missing',
  'agent_identity_not_active',
  'evidence_ambiguous',
  'pending_state_ambiguous',
]);

export function normalizeLifecycleStatus(value: unknown): string {
  return String(value || '').trim().toLowerCase();
}

export function nodeAgentIdentityExpectedConfirmation(node: Pick<NodeDetail, 'id' | 'name'>): string {
  return String(node.name || '').trim() || node.id;
}

export function reasonLooksUnsafeForNodeAgentIdentityRevoke(reason: string): boolean {
  if (hasControlCharacter(reason)) return true;
  if (reason.includes('{') || reason.includes('}')) return true;
  const lower = reason.toLowerCase();
  return unsafeReasonMarkers.some((marker) => lower.includes(marker));
}

function hasControlCharacter(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code < 32 || code === 127) return true;
  }
  return false;
}

export function validateNodeAgentIdentityRevokeForm(node: Pick<NodeDetail, 'id' | 'name'>, form: NodeAgentIdentityRevokeForm): NodeAgentIdentityRevokeValidation {
  const expectedConfirmation = nodeAgentIdentityExpectedConfirmation(node);
  const confirmation = form.confirmation.trim();
  const reason = form.reason.trim();
  const errors: NodeAgentIdentityRevokeValidation['errors'] = {};

  if (!confirmation) {
    errors.confirmation = 'nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationRequired';
  } else if (confirmation.length > NODE_AGENT_IDENTITY_REVOKE_MAX_CONFIRMATION_LENGTH) {
    errors.confirmation = 'nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationTooLong';
  } else if (hasControlCharacter(confirmation)) {
    errors.confirmation = 'nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationUnsafe';
  } else if (confirmation !== expectedConfirmation) {
    errors.confirmation = 'nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationMismatch';
  }

  if (!reason) {
    errors.reason = 'nodes.lifecycleControls.agentIdentityRevoke.errors.reasonRequired';
  } else if (reason.length < NODE_AGENT_IDENTITY_REVOKE_MIN_REASON_LENGTH) {
    errors.reason = 'nodes.lifecycleControls.agentIdentityRevoke.errors.reasonTooShort';
  } else if (reason.length > NODE_AGENT_IDENTITY_REVOKE_MAX_REASON_LENGTH) {
    errors.reason = 'nodes.lifecycleControls.agentIdentityRevoke.errors.reasonTooLong';
  } else if (reasonLooksUnsafeForNodeAgentIdentityRevoke(reason)) {
    errors.reason = 'nodes.lifecycleControls.agentIdentityRevoke.errors.reasonUnsafe';
  }

  if (!form.acknowledged) {
    errors.acknowledgement = 'nodes.lifecycleControls.agentIdentityRevoke.errors.acknowledgementRequired';
  }

  const valid = !errors.confirmation && !errors.reason && !errors.acknowledgement;
  return {
    valid,
    expectedConfirmation,
    input: valid ? { confirmation, reason } : undefined,
    errors,
  };
}

export function lifecycleSeverityForStatus(value: unknown): NodeLifecycleSeverity {
  const normalized = normalizeLifecycleStatus(value);
  if (!normalized) return 'neutral';
  if (healthyStatuses.has(normalized)) return 'healthy';
  if (blockedStatuses.has(normalized)) return 'blocked';
  if (warningStatuses.has(normalized)) return 'warning';
  return 'neutral';
}

function combineSeverity(values: NodeLifecycleSeverity[]): NodeLifecycleSeverity {
  if (values.includes('blocked')) return 'blocked';
  if (values.includes('warning')) return 'warning';
  if (values.includes('healthy')) return 'healthy';
  return 'neutral';
}

export function describeStaleRotationReason(reason: string): NodeStaleRotationReasonDescriptor {
  const normalized = normalizeLifecycleStatus(reason);
  const labelKey = NODE_STALE_ROTATION_REASON_LABEL_KEYS[normalized];
  if (!labelKey) {
    return {
      reason: normalized || 'unknown',
      labelKey: 'nodes.lifecycleControls.reasons.unknown',
      severity: 'blocked',
      known: false,
    };
  }
  return {
    reason: normalized,
    labelKey,
    severity: blockedReasonSet.has(normalized) ? 'blocked' : safeReasonSet.has(normalized) ? 'warning' : 'neutral',
    known: true,
  };
}

export function deriveNodeLifecycleStatusModel(input: {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  staleRotationPreview?: NodeStaleRotationPreview;
}): NodeLifecycleStatusModel {
  const { node, diagnostics, staleRotationPreview } = input;
  const agent = diagnostics?.agent;
  const items: NodeLifecycleStatusItem[] = [
    {
      key: 'node_status',
      labelKey: 'nodes.lifecycleControls.nodeStatus',
      value: node.status || 'unknown',
      severity: lifecycleSeverityForStatus(node.status),
    },
    {
      key: 'agent_status',
      labelKey: 'nodes.lifecycleControls.agentStatus',
      value: agent?.status || node.agent_status || 'unknown',
      severity: lifecycleSeverityForStatus(agent?.status || node.agent_status),
    },
    {
      key: 'heartbeat_state',
      labelKey: 'nodes.lifecycleControls.heartbeatState',
      value: diagnostics?.heartbeat_state || node.status || 'unknown',
      severity: lifecycleSeverityForStatus(diagnostics?.heartbeat_state || node.status),
    },
    {
      key: 'communication_state',
      labelKey: 'nodes.lifecycleControls.communicationState',
      value: diagnostics?.communication_state || node.agent_channel_status || 'unknown',
      severity: lifecycleSeverityForStatus(diagnostics?.communication_state || node.agent_channel_status),
    },
    {
      key: 'token_rotation_status',
      labelKey: 'nodes.lifecycleControls.tokenRotationStatus',
      value: staleRotationPreview?.token_rotation_status || agent?.token_rotation_status || 'unknown',
      severity: lifecycleSeverityForStatus(staleRotationPreview?.token_rotation_status || agent?.token_rotation_status),
    },
  ];

  const candidates = staleRotationPreview?.candidates || [];
  const unknownReasonCount = candidates.filter((candidate) => !describeStaleRotationReason(candidate.stale_reason).known).length;
  const backendSafeCandidateCount = candidates.filter((candidate) => candidate.safe_to_clear).length;
  const staleSeverity: NodeLifecycleSeverity = staleRotationPreview?.stale_rotation_detected
    ? unknownReasonCount > 0 || backendSafeCandidateCount === 0
      ? 'blocked'
      : 'warning'
    : 'healthy';

  return {
    nodeId: node.id,
    overallSeverity: combineSeverity([...items.map((item) => item.severity), staleSeverity]),
    overallStatusKey: staleSeverity === 'blocked'
      ? 'nodes.lifecycleControls.overallBlocked'
      : staleSeverity === 'warning'
        ? 'nodes.lifecycleControls.overallWarning'
        : 'nodes.lifecycleControls.overallHealthy',
    items,
    staleRotation: {
      detected: Boolean(staleRotationPreview?.stale_rotation_detected),
      candidateCount: candidates.length,
      backendSafeCandidateCount,
      unknownReasonCount,
      evaluatedAt: staleRotationPreview?.evaluated_at,
      tokenRotationStatus: staleRotationPreview?.token_rotation_status || agent?.token_rotation_status || 'unknown',
      severity: staleSeverity,
    },
  };
}

export function staleRotationCandidateKey(candidate: NodeStaleRotationCandidate, index: number): string {
  return candidate.job_id || `${candidate.status}:${candidate.created_at}:${index}`;
}

export function staleRotationPreviewErrorKey(error: unknown): string {
  if (!(error instanceof APIError)) return 'nodes.lifecycleControls.errors.generic';
  switch (error.status) {
    case 403:
      return 'nodes.lifecycleControls.errors.forbidden';
    case 404:
      return 'nodes.lifecycleControls.errors.notFound';
    case 409:
      return 'nodes.lifecycleControls.errors.conflict';
    case 400:
    case 422:
      return 'nodes.lifecycleControls.errors.validation';
    default:
      return 'nodes.lifecycleControls.errors.generic';
  }
}

function apiErrorCode(error: APIError): string {
  const payload = error.payload;
  if (payload && typeof payload === 'object' && 'code' in payload) {
    return String((payload as { code?: unknown }).code || '');
  }
  return '';
}

export function nodeAgentIdentityRevokeErrorKey(error: unknown): string {
  if (!(error instanceof APIError)) return 'nodes.lifecycleControls.agentIdentityRevoke.errors.generic';
  const code = apiErrorCode(error);
  if (code === 'node_agent_revoke_confirmation_mismatch') return 'nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationMismatchBackend';
  if (code === 'node_agent_identity_missing') return 'nodes.lifecycleControls.agentIdentityRevoke.errors.identityMissingBackend';
  if (code === 'node_agent_revoke_conflict') return 'nodes.lifecycleControls.agentIdentityRevoke.errors.conflict';
  if (code === 'node_agent_revoke_node_not_found') return 'nodes.lifecycleControls.agentIdentityRevoke.errors.nodeNotFound';
  if (code === 'node_agent_revoke_request_invalid') return 'nodes.lifecycleControls.agentIdentityRevoke.errors.requestInvalid';
  if (code === 'node_agent_revoke_internal_error') return 'nodes.lifecycleControls.agentIdentityRevoke.errors.serviceUnavailable';

  switch (error.status) {
    case 400:
    case 422:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.requestInvalid';
    case 403:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.permissionRequired';
    case 404:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.nodeNotFound';
    case 409:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.conflict';
    case 429:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.rateLimited';
    case 500:
    case 503:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.serviceUnavailable';
    default:
      return 'nodes.lifecycleControls.agentIdentityRevoke.errors.generic';
  }
}

export function formatAgeSeconds(value: unknown): string {
  const seconds = Number(value);
  if (!Number.isFinite(seconds) || seconds < 0) return 'n/a';
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${Math.floor(seconds % 60)}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}
