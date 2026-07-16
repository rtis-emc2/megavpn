import { describe, expect, it } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeStaleRotationPreview } from '../../shared/api/types';
import {
  deriveNodeStaleRotationClearContext,
  nodeStaleRotationClearErrorKey,
  nodeStaleRotationExpectedConfirmation,
  reasonLooksUnsafeForNodeStaleRotationClear,
  validateNodeStaleRotationClearForm,
  validateNodeStaleRotationClearResult,
} from './nodeStaleRotationClear';

const node: NodeDetail = { id: 'node-1', name: 'Edge One' };
const preview: NodeStaleRotationPreview = {
  node_id: 'node-1',
  stale_rotation_detected: true,
  token_rotation_status: 'rotating',
  evaluated_at: '2026-07-16T08:00:00Z',
  candidates: [
    {
      job_id: 'job-safe-b',
      status: 'running',
      created_at: '2026-07-16T07:00:00Z',
      age_seconds: 3600,
      stale_reason: 'claimed_without_result_and_agent_inactive',
      safe_to_clear: true,
    },
    {
      job_id: 'job-excluded',
      status: 'running',
      created_at: '2026-07-16T07:30:00Z',
      age_seconds: 1800,
      stale_reason: 'claim_or_lease_still_active',
      safe_to_clear: false,
    },
    {
      job_id: ' job-safe-a ',
      status: 'queued',
      created_at: '2026-07-16T06:30:00Z',
      age_seconds: 5400,
      stale_reason: 'unclaimed_without_agent_progress',
      safe_to_clear: true,
    },
  ],
};

function clearResult(overrides: Record<string, unknown> = {}) {
  return {
    status: 'cleared',
    node_id: 'node-1',
    cleared_count: 2,
    cleared_jobs: [
      {
        job_id: 'job-safe-b',
        previous_status: 'running',
        status: 'cancelled',
        stale_reason: 'claimed_without_result_and_agent_inactive',
        finished_at: '2026-07-16T08:01:00Z',
      },
      {
        job_id: 'job-safe-a',
        previous_status: 'queued',
        status: 'cancelled',
        stale_reason: 'unclaimed_without_agent_progress',
        finished_at: '2026-07-16T08:01:01Z',
      },
    ],
    pending_rotation_state_cleared: true,
    active_agent_identity_preserved: true,
    ...overrides,
  };
}

describe('node stale rotation clear contract helpers', () => {
  it('derives the complete deterministic safe set and immutable fingerprint only from preview', () => {
    const result = deriveNodeStaleRotationClearContext('node-1', preview);
    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.context.expectedJobIds).toEqual(['job-safe-a', 'job-safe-b']);
    expect(result.context.excludedCandidates.map((candidate) => candidate.job_id)).toEqual(['job-excluded']);
    expect(result.context.fingerprint).toBe(JSON.stringify({
      node_id: 'node-1',
      evaluated_at: '2026-07-16T08:00:00Z',
      expected_job_ids: ['job-safe-a', 'job-safe-b'],
      safe_candidate_count: 2,
    }));
    expect(Object.isFrozen(result.context.expectedJobIds)).toBe(true);
    expect(Object.isFrozen(result.context)).toBe(true);
  });

  it('fails closed for wrong ownership, empty safe set, malformed or duplicate IDs', () => {
    expect(deriveNodeStaleRotationClearContext('node-other', preview)).toMatchObject({ valid: false });
    expect(deriveNodeStaleRotationClearContext('node-1', { ...preview, candidates: preview.candidates.map((candidate) => ({ ...candidate, safe_to_clear: false })) }))
      .toEqual({ valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.noSafeCandidates' });
    expect(deriveNodeStaleRotationClearContext('node-1', { ...preview, candidates: [{ ...preview.candidates[0], job_id: 'bad job id' }] }))
      .toEqual({ valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.previewContract' });
    expect(deriveNodeStaleRotationClearContext('node-1', { ...preview, candidates: [preview.candidates[0], { ...preview.candidates[0] }] }))
      .toEqual({ valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.previewContract' });
  });

  it('fails closed when backend marks an unknown reason safe', () => {
    expect(deriveNodeStaleRotationClearContext('node-1', {
      ...preview,
      candidates: [{ ...preview.candidates[0], stale_reason: 'new_backend_reason' }],
    })).toEqual({ valid: false, errorKey: 'nodes.lifecycleControls.staleRotationClear.errors.unknownSafeReason' });
  });

  it('changes fingerprint when evaluated time or exact safe set changes', () => {
    const first = deriveNodeStaleRotationClearContext('node-1', preview);
    const refreshed = deriveNodeStaleRotationClearContext('node-1', {
      ...preview,
      evaluated_at: '2026-07-16T08:02:00Z',
      candidates: preview.candidates.filter((candidate) => candidate.job_id.trim() !== 'job-safe-b'),
    });
    expect(first.valid && refreshed.valid && first.context.fingerprint).not.toBe(refreshed.valid ? refreshed.context.fingerprint : '');
  });

  it('validates exact name, secret-safe reason, acknowledgement and generated expected set', () => {
    const context = deriveNodeStaleRotationClearContext('node-1', preview);
    const result = validateNodeStaleRotationClearForm(node, context, {
      confirmation: ' Edge One ',
      reason: ' clear after operator review ',
      acknowledged: true,
    });
    expect(result.valid).toBe(true);
    expect(result.input).toEqual({
      confirmation: 'Edge One',
      reason: 'clear after operator review',
      acknowledge_cancel_rotation: true,
      expected_job_ids: ['job-safe-a', 'job-safe-b'],
    });
    expect(nodeStaleRotationExpectedConfirmation({ id: 'node-empty', name: '  ' })).toBe('node-empty');
  });

  it('rejects case mismatch, control characters and secret or request-like reasons', () => {
    const context = deriveNodeStaleRotationClearContext('node-1', preview);
    const invalid = validateNodeStaleRotationClearForm(node, context, {
      confirmation: 'edge one',
      reason: 'Authorization: Bearer secret',
      acknowledged: false,
    });
    expect(invalid.valid).toBe(false);
    expect(invalid.errors).toMatchObject({
      confirmation: 'nodes.lifecycleControls.staleRotationClear.errors.confirmationMismatch',
      reason: 'nodes.lifecycleControls.staleRotationClear.errors.reasonUnsafe',
      acknowledgement: 'nodes.lifecycleControls.staleRotationClear.errors.acknowledgementRequired',
    });
    expect(reasonLooksUnsafeForNodeStaleRotationClear('line one\nline two')).toBe(true);
    expect(reasonLooksUnsafeForNodeStaleRotationClear('{"reason":"value"}')).toBe(true);
    expect(reasonLooksUnsafeForNodeStaleRotationClear('reviewed stale rotation')).toBe(false);
  });

  it('accepts only an exact, redacted, ownership-matched clear result', () => {
    const result = validateNodeStaleRotationClearResult(clearResult(), 'node-1', ['job-safe-a', 'job-safe-b']);
    expect(result).toMatchObject({
      nodeId: 'node-1',
      clearedCount: 2,
      pendingRotationStateCleared: true,
      activeAgentIdentityPreserved: true,
    });
    expect(result?.clearedJobs.map((job) => job.job_id)).toEqual(['job-safe-a', 'job-safe-b']);
  });

  it('rejects partial, additional, duplicate, wrong-node and identity-not-preserved results', () => {
    const expected = ['job-safe-a', 'job-safe-b'];
    expect(validateNodeStaleRotationClearResult(clearResult({ node_id: 'node-other' }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({ active_agent_identity_preserved: false }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({ token_hash: 'unexpected' }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({ cleared_count: 1, cleared_jobs: [clearResult().cleared_jobs[0]] }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({ cleared_jobs: [clearResult().cleared_jobs[0], clearResult().cleared_jobs[0]] }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({
      cleared_jobs: [clearResult().cleared_jobs[0], { ...clearResult().cleared_jobs[1], job_id: 'job-extra' }],
    }), 'node-1', expected)).toBeNull();
  });

  it('rejects unsafe status, stale reason and timestamp fields', () => {
    const expected = ['job-safe-a', 'job-safe-b'];
    const jobs = clearResult().cleared_jobs as Array<Record<string, unknown>>;
    expect(validateNodeStaleRotationClearResult(clearResult({ cleared_jobs: [{ ...jobs[0], status: 'succeeded' }, jobs[1]] }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({ cleared_jobs: [{ ...jobs[0], stale_reason: 'Authorization: Bearer' }, jobs[1]] }), 'node-1', expected)).toBeNull();
    expect(validateNodeStaleRotationClearResult(clearResult({ cleared_jobs: [{ ...jobs[0], finished_at: 'not-a-time' }, jobs[1]] }), 'node-1', expected)).toBeNull();
  });

  it('maps exact backend codes without exposing raw backend text', () => {
    const cases = [
      ['node_stale_rotation_request_invalid', 'requestInvalid'],
      ['node_stale_rotation_node_not_found', 'nodeNotFound'],
      ['node_stale_rotation_confirmation_mismatch', 'confirmationMismatchBackend'],
      ['node_stale_rotation_not_found', 'noCandidatesBackend'],
      ['node_stale_rotation_preview_changed', 'previewChangedBackend'],
      ['node_stale_rotation_evidence_ambiguous', 'evidenceAmbiguous'],
      ['node_stale_rotation_pending_state_ambiguous', 'pendingStateAmbiguous'],
      ['node_stale_rotation_conflict', 'conflict'],
      ['node_stale_rotation_internal_error', 'serviceUnavailable'],
    ];
    for (const [code, suffix] of cases) {
      const error = new APIError('token_hash raw backend SQL', 409, { code, error: 'token_hash raw backend SQL' });
      expect(nodeStaleRotationClearErrorKey(error)).toBe(`nodes.lifecycleControls.staleRotationClear.errors.${suffix}`);
    }
    expect(nodeStaleRotationClearErrorKey(new APIError('raw', 429, {}))).toBe('nodes.lifecycleControls.staleRotationClear.errors.rateLimited');
  });
});
