import { describe, expect, it } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeDiagnostics, NodeStaleRotationPreview } from '../../shared/api/types';
import {
  deriveNodeLifecycleStatusModel,
  describeStaleRotationReason,
  formatAgeSeconds,
  nodeAgentIdentityExpectedConfirmation,
  nodeAgentIdentityRevokeErrorKey,
  reasonLooksUnsafeForNodeAgentIdentityRevoke,
  staleRotationCandidateKey,
  staleRotationPreviewErrorKey,
  validateNodeAgentIdentityRevokeForm,
} from './nodeLifecycleControls';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'online',
  agent_status: 'online',
  agent_channel_status: 'connected',
};

const diagnostics: NodeDiagnostics = {
  heartbeat_state: 'healthy',
  communication_state: 'connected',
  agent: {
    status: 'active',
    token_rotation_status: 'active',
    last_seen_at: '2026-07-14T08:00:00Z',
  },
};

const preview: NodeStaleRotationPreview = {
  node_id: 'node-1',
  stale_rotation_detected: true,
  token_rotation_status: 'rotating',
  evaluated_at: '2026-07-14T08:10:00Z',
  candidates: [{
    job_id: 'job-stale-1',
    status: 'running',
    created_at: '2026-07-14T07:50:00Z',
    started_at: '2026-07-14T07:51:00Z',
    age_seconds: 1200,
    stale_reason: 'unclaimed_without_agent_progress',
    safe_to_clear: true,
  }],
};

describe('node lifecycle controls model', () => {
  it('derives read-only lifecycle state from safe node, diagnostics and stale rotation fields', () => {
    const model = deriveNodeLifecycleStatusModel({ node, diagnostics, staleRotationPreview: preview });

    expect(model.nodeId).toBe('node-1');
    expect(model.overallSeverity).toBe('warning');
    expect(model.staleRotation).toMatchObject({
      detected: true,
      candidateCount: 1,
      backendSafeCandidateCount: 1,
      unknownReasonCount: 0,
      tokenRotationStatus: 'rotating',
    });
    expect(model.items.map((item) => item.key)).toEqual([
      'node_status',
      'agent_status',
      'heartbeat_state',
      'communication_state',
      'token_rotation_status',
    ]);
  });

  it('fails unknown stale reasons safe without overriding backend safe_to_clear', () => {
    const unknownPreview: NodeStaleRotationPreview = {
      ...preview,
      candidates: [{ ...preview.candidates[0], stale_reason: 'backend_added_reason', safe_to_clear: true }],
    };
    const model = deriveNodeLifecycleStatusModel({ node, diagnostics, staleRotationPreview: unknownPreview });
    const descriptor = describeStaleRotationReason('backend_added_reason');

    expect(descriptor).toMatchObject({
      known: false,
      labelKey: 'nodes.lifecycleControls.reasons.unknown',
      severity: 'blocked',
    });
    expect(model.staleRotation.unknownReasonCount).toBe(1);
    expect(model.staleRotation.backendSafeCandidateCount).toBe(1);
    expect(model.staleRotation.severity).toBe('blocked');
  });

  it('maps preview errors to safe i18n keys instead of raw backend text', () => {
    expect(staleRotationPreviewErrorKey(new APIError('node.read permission required', 403, { error: 'node.read permission required' }))).toBe('nodes.lifecycleControls.errors.forbidden');
    expect(staleRotationPreviewErrorKey(new APIError('secret_ref should not render', 409, { error: 'secret_ref should not render' }))).toBe('nodes.lifecycleControls.errors.conflict');
    expect(staleRotationPreviewErrorKey(new Error('network token leak'))).toBe('nodes.lifecycleControls.errors.generic');
  });

  it('keeps candidate keys and age formatting deterministic', () => {
    expect(staleRotationCandidateKey(preview.candidates[0], 0)).toBe('job-stale-1');
    expect(staleRotationCandidateKey({ ...preview.candidates[0], job_id: '' }, 3)).toBe('running:2026-07-14T07:50:00Z:3');
    expect(formatAgeSeconds(59)).toBe('59s');
    expect(formatAgeSeconds(125)).toBe('2m 5s');
    expect(formatAgeSeconds(7260)).toBe('2h 1m');
  });
});

describe('node agent identity revoke helpers', () => {
  it('validates exact confirmation, trimmed input, reason bounds and UI-only acknowledgement', () => {
    expect(nodeAgentIdentityExpectedConfirmation(node)).toBe('Edge One');
    expect(nodeAgentIdentityExpectedConfirmation({ ...node, name: '' })).toBe('node-1');

    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: '', reason: 'incident response', acknowledged: true }).errors.confirmation).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationRequired');
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'edge one', reason: 'incident response', acknowledged: true }).errors.confirmation).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationMismatch');
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'x'.repeat(513), reason: 'incident response', acknowledged: true }).errors.confirmation).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationTooLong');
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'Edge\nOne', reason: 'incident response', acknowledged: true }).errors.confirmation).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationUnsafe');
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: ' Edge One ', reason: ' incident response ', acknowledged: false }).errors.acknowledgement).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.acknowledgementRequired');

    const valid = validateNodeAgentIdentityRevokeForm(node, { confirmation: ' Edge One ', reason: ' incident response ', acknowledged: true });
    expect(valid.valid).toBe(true);
    expect(valid.input).toEqual({ confirmation: 'Edge One', reason: 'incident response' });
  });

  it('rejects unsafe reasons without exposing request or secret markers', () => {
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'Edge One', reason: '', acknowledged: true }).errors.reason).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.reasonRequired');
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'Edge One', reason: 'bad', acknowledged: true }).errors.reason).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.reasonTooShort');
    expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'Edge One', reason: 'r'.repeat(501), acknowledged: true }).errors.reason).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.reasonTooLong');

    for (const reason of ['has\nnewline', 'Authorization: Bearer abc', 'Bearer token', 'agent_token=abc', 'enrollment_token=abc', 'token_hash=abc', 'private_key=abc', 'secret_ref=abc', '{"request":"body"}']) {
      expect(reasonLooksUnsafeForNodeAgentIdentityRevoke(reason)).toBe(true);
      expect(validateNodeAgentIdentityRevokeForm(node, { confirmation: 'Edge One', reason, acknowledged: true }).errors.reason).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.reasonUnsafe');
    }
  });

  it('maps backend revoke errors using safe status and code allowlists', () => {
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw sql token_hash', 409, { code: 'node_agent_revoke_confirmation_mismatch', error: 'raw sql token_hash' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationMismatchBackend');
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw', 409, { code: 'node_agent_identity_missing' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.identityMissingBackend');
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw', 409, { code: 'node_agent_revoke_conflict' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.conflict');
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw', 404, { code: 'node_agent_revoke_node_not_found' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.nodeNotFound');
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw', 400, { code: 'node_agent_revoke_request_invalid' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.requestInvalid');
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw', 429, { error: 'rate limited' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.rateLimited');
    expect(nodeAgentIdentityRevokeErrorKey(new APIError('raw', 503, { error: 'down' }))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.serviceUnavailable');
    expect(nodeAgentIdentityRevokeErrorKey(new Error('network'))).toBe('nodes.lifecycleControls.agentIdentityRevoke.errors.generic');
  });
});
