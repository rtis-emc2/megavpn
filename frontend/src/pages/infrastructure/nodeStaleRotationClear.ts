import { APIError } from '../../shared/api/client';
import type {
  NodeDetail,
  NodeStaleRotationCandidate,
  NodeStaleRotationClearInput,
  NodeStaleRotationClearedJob,
  NodeStaleRotationPreview,
} from '../../shared/api/types';
import { describeStaleRotationReason } from './nodeLifecycleControls';

export const NODE_STALE_ROTATION_MAX_EXPECTED_JOB_IDS = 20;
export const NODE_STALE_ROTATION_MAX_CONFIRMATION_LENGTH = 512;
export const NODE_STALE_ROTATION_MIN_REASON_LENGTH = 5;
export const NODE_STALE_ROTATION_MAX_REASON_LENGTH = 500;

export type NodeStaleRotationClearContext = {
  expectedJobIds: readonly string[];
  fingerprint: string;
  safeCandidates: readonly NodeStaleRotationCandidate[];
  excludedCandidates: readonly NodeStaleRotationCandidate[];
};

export type NodeStaleRotationClearContextResult =
  | { valid: true; context: NodeStaleRotationClearContext }
  | { valid: false; errorKey: string };

export type NodeStaleRotationClearForm = {
  confirmation: string;
  reason: string;
  acknowledged: boolean;
};

export type NodeStaleRotationClearValidation = {
  valid: boolean;
  expectedConfirmation: string;
  input?: NodeStaleRotationClearInput;
  errors: {
    confirmation?: string;
    reason?: string;
    acknowledgement?: string;
    expectedSet?: string;
  };
};

export type SafeNodeStaleRotationClearResult = {
  nodeId: string;
  clearedCount: number;
  clearedJobs: readonly NodeStaleRotationClearedJob[];
  pendingRotationStateCleared: boolean;
  activeAgentIdentityPreserved: true;
};

const jobIDPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const staleReasonPattern = /^[a-z0-9][a-z0-9._-]{0,127}$/;
const activeJobStatuses = new Set(['queued', 'running', 'retrying']);
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

function hasExactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const actual = Object.keys(value).sort((left, right) => left.localeCompare(right));
  const sortedExpected = [...expected].sort((left, right) => left.localeCompare(right));
  return actual.length === sortedExpected.length && actual.every((key, index) => key === sortedExpected[index]);
}

function validTimestamp(value: unknown): value is string {
  return typeof value === 'string'
    && value.length > 0
    && value.length <= 64
    && !hasControlCharacter(value)
    && !Number.isNaN(new Date(value).getTime());
}

function safeJobID(value: unknown): string | null {
  if (typeof value !== 'string') return null;
  const normalized = value.trim();
  return jobIDPattern.test(normalized) ? normalized : null;
}

function safePreviewCandidate(candidate: NodeStaleRotationCandidate): boolean {
  const descriptor = describeStaleRotationReason(candidate.stale_reason);
  return descriptor.known && descriptor.severity === 'warning';
}

export function deriveNodeStaleRotationClearContext(
  nodeId: string,
  preview: NodeStaleRotationPreview | undefined,
): NodeStaleRotationClearContextResult {
  if (!preview || preview.node_id !== nodeId) {
    return { valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.previewWrongNode' };
  }
  if (!validTimestamp(preview.evaluated_at) || !Array.isArray(preview.candidates)) {
    return { valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.previewContract' };
  }

  const safeCandidates = preview.candidates.filter((candidate) => candidate.safe_to_clear === true);
  const excludedCandidates = preview.candidates.filter((candidate) => candidate.safe_to_clear !== true);
  if (!safeCandidates.length) {
    return { valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.noSafeCandidates' };
  }
  if (safeCandidates.length > NODE_STALE_ROTATION_MAX_EXPECTED_JOB_IDS) {
    return { valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.tooManyCandidates' };
  }

  const expectedJobIds: string[] = [];
  const seen = new Set<string>();
  for (const candidate of safeCandidates) {
    const jobID = safeJobID(candidate.job_id);
    if (!jobID || seen.has(jobID)) {
      return { valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.previewContract' };
    }
    if (!safePreviewCandidate(candidate)) {
      return { valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.unknownSafeReason' };
    }
    seen.add(jobID);
    expectedJobIds.push(jobID);
  }
  expectedJobIds.sort((left, right) => left.localeCompare(right));
  const immutableIDs = Object.freeze([...expectedJobIds]);
  const fingerprint = JSON.stringify({
    node_id: preview.node_id,
    evaluated_at: preview.evaluated_at,
    expected_job_ids: immutableIDs,
    safe_candidate_count: safeCandidates.length,
  });
  return {
    valid: true,
    context: Object.freeze({
      expectedJobIds: immutableIDs,
      fingerprint,
      safeCandidates: Object.freeze([...safeCandidates]),
      excludedCandidates: Object.freeze([...excludedCandidates]),
    }),
  };
}

export function nodeStaleRotationExpectedConfirmation(node: Pick<NodeDetail, 'id' | 'name'>): string {
  return String(node.name || '').trim() || node.id;
}

export function reasonLooksUnsafeForNodeStaleRotationClear(reason: string): boolean {
  if (hasControlCharacter(reason) || reason.includes('{') || reason.includes('}')) return true;
  const lower = reason.toLowerCase();
  return unsafeReasonMarkers.some((marker) => lower.includes(marker));
}

export function validateNodeStaleRotationClearForm(
  node: Pick<NodeDetail, 'id' | 'name'>,
  contextResult: NodeStaleRotationClearContextResult,
  form: NodeStaleRotationClearForm,
): NodeStaleRotationClearValidation {
  const expectedConfirmation = nodeStaleRotationExpectedConfirmation(node);
  const confirmation = form.confirmation.trim();
  const reason = form.reason.trim();
  const errors: NodeStaleRotationClearValidation['errors'] = {};

  if (!contextResult.valid) errors.expectedSet = contextResult.errorKey;
  if (!confirmation) {
    errors.confirmation = 'nodes.lifecycleControls.staleRotationClear.errors.confirmationRequired';
  } else if (confirmation.length > NODE_STALE_ROTATION_MAX_CONFIRMATION_LENGTH) {
    errors.confirmation = 'nodes.lifecycleControls.staleRotationClear.errors.confirmationTooLong';
  } else if (hasControlCharacter(confirmation)) {
    errors.confirmation = 'nodes.lifecycleControls.staleRotationClear.errors.confirmationUnsafe';
  } else if (confirmation !== expectedConfirmation) {
    errors.confirmation = 'nodes.lifecycleControls.staleRotationClear.errors.confirmationMismatch';
  }

  if (!reason) {
    errors.reason = 'nodes.lifecycleControls.staleRotationClear.errors.reasonRequired';
  } else if (reason.length < NODE_STALE_ROTATION_MIN_REASON_LENGTH) {
    errors.reason = 'nodes.lifecycleControls.staleRotationClear.errors.reasonTooShort';
  } else if (reason.length > NODE_STALE_ROTATION_MAX_REASON_LENGTH) {
    errors.reason = 'nodes.lifecycleControls.staleRotationClear.errors.reasonTooLong';
  } else if (reasonLooksUnsafeForNodeStaleRotationClear(reason)) {
    errors.reason = 'nodes.lifecycleControls.staleRotationClear.errors.reasonUnsafe';
  }
  if (!form.acknowledged) {
    errors.acknowledgement = 'nodes.lifecycleControls.staleRotationClear.errors.acknowledgementRequired';
  }

  const valid = Object.keys(errors).length === 0;
  return {
    valid,
    expectedConfirmation,
    input: valid && contextResult.valid ? {
      confirmation,
      reason,
      acknowledge_cancel_rotation: true,
      expected_job_ids: [...contextResult.context.expectedJobIds],
    } : undefined,
    errors,
  };
}

export function validateNodeStaleRotationClearResult(
  value: unknown,
  nodeId: string,
  expectedJobIds: readonly string[],
): SafeNodeStaleRotationClearResult | null {
  if (!isPlainRecord(value) || value.status !== 'cleared' || value.node_id !== nodeId) return null;
  if (!hasExactKeys(value, [
    'status',
    'node_id',
    'cleared_count',
    'cleared_jobs',
    'pending_rotation_state_cleared',
    'active_agent_identity_preserved',
  ])) return null;
  if (!Number.isSafeInteger(value.cleared_count) || Number(value.cleared_count) < 1 || !Array.isArray(value.cleared_jobs)) return null;
  if (value.cleared_count !== value.cleared_jobs.length || value.cleared_jobs.length !== expectedJobIds.length) return null;
  if (typeof value.pending_rotation_state_cleared !== 'boolean' || value.active_agent_identity_preserved !== true) return null;

  const expected = [...expectedJobIds].sort((left, right) => left.localeCompare(right));
  const seen = new Set<string>();
  const clearedJobs: NodeStaleRotationClearedJob[] = [];
  for (const rawJob of value.cleared_jobs) {
    if (!isPlainRecord(rawJob)) return null;
    if (!hasExactKeys(rawJob, ['job_id', 'previous_status', 'status', 'stale_reason', 'finished_at'])) return null;
    const jobID = safeJobID(rawJob.job_id);
    if (!jobID || seen.has(jobID) || !expected.includes(jobID)) return null;
    if (typeof rawJob.previous_status !== 'string' || !activeJobStatuses.has(rawJob.previous_status)) return null;
    if (rawJob.status !== 'cancelled') return null;
    if (typeof rawJob.stale_reason !== 'string' || !staleReasonPattern.test(rawJob.stale_reason)) return null;
    if (!validTimestamp(rawJob.finished_at)) return null;
    seen.add(jobID);
    clearedJobs.push({
      job_id: jobID,
      previous_status: rawJob.previous_status,
      status: 'cancelled',
      stale_reason: rawJob.stale_reason,
      finished_at: rawJob.finished_at,
    });
  }
  if (expected.some((jobID) => !seen.has(jobID))) return null;
  clearedJobs.sort((left, right) => left.job_id.localeCompare(right.job_id));
  return {
    nodeId,
    clearedCount: clearedJobs.length,
    clearedJobs: Object.freeze(clearedJobs),
    pendingRotationStateCleared: value.pending_rotation_state_cleared,
    activeAgentIdentityPreserved: true,
  };
}

export function nodeStaleRotationClearErrorCode(error: unknown): string {
  if (!(error instanceof APIError) || !isPlainRecord(error.payload)) return '';
  return typeof error.payload.code === 'string' ? error.payload.code : '';
}

export function nodeStaleRotationClearErrorKey(error: unknown): string {
  if (!(error instanceof APIError)) return 'nodes.lifecycleControls.staleRotationClear.errors.generic';
  if (error.status === 403) return 'nodes.lifecycleControls.staleRotationClear.errors.permissionRequired';
  const codeKeys: Record<string, string> = {
    node_stale_rotation_request_invalid: 'requestInvalid',
    node_stale_rotation_node_not_found: 'nodeNotFound',
    node_stale_rotation_confirmation_mismatch: 'confirmationMismatchBackend',
    node_stale_rotation_not_found: 'noCandidatesBackend',
    node_stale_rotation_preview_changed: 'previewChangedBackend',
    node_stale_rotation_evidence_ambiguous: 'evidenceAmbiguous',
    node_stale_rotation_pending_state_ambiguous: 'pendingStateAmbiguous',
    node_stale_rotation_conflict: 'conflict',
    node_stale_rotation_internal_error: 'serviceUnavailable',
  };
  const code = nodeStaleRotationClearErrorCode(error);
  if (codeKeys[code]) return `nodes.lifecycleControls.staleRotationClear.errors.${codeKeys[code]}`;
  switch (error.status) {
    case 400:
      return 'nodes.lifecycleControls.staleRotationClear.errors.requestInvalid';
    case 404:
      return 'nodes.lifecycleControls.staleRotationClear.errors.nodeNotFound';
    case 409:
      return 'nodes.lifecycleControls.staleRotationClear.errors.conflict';
    case 429:
      return 'nodes.lifecycleControls.staleRotationClear.errors.rateLimited';
    case 500:
    case 503:
      return 'nodes.lifecycleControls.staleRotationClear.errors.serviceUnavailable';
    default:
      return 'nodes.lifecycleControls.staleRotationClear.errors.generic';
  }
}
