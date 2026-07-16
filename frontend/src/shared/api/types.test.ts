import { describe, expectTypeOf, it } from 'vitest';
import type { EnrollmentToken, EnrollmentTokenIssueResult, Job, NodeAgentIdentityRevokeInput, NodeAgentIdentityRevokeResult, NodeEmergencyCleanupInput, NodeEmergencyCleanupPlanSummary, NodeEmergencyCleanupResult, NodeEmergencyCleanupScope, NodeRebootInput, NodeStaleRotationCandidate, NodeStaleRotationClearInput, NodeStaleRotationClearResult, NodeStaleRotationClearedJob, NodeStaleRotationPreview } from './types';

describe('enrollment token API types', () => {
  it('keeps safe list tokens separate from secret-bearing issue responses', () => {
    expectTypeOf<EnrollmentToken>().not.toHaveProperty('token');
    expectTypeOf<EnrollmentToken>().not.toHaveProperty('enrollment_token');
    expectTypeOf<EnrollmentTokenIssueResult>().toHaveProperty('token');
    expectTypeOf<EnrollmentTokenIssueResult>().toHaveProperty('enrollment_token');
  });
});

describe('node stale rotation preview API types', () => {
  it('matches the backend read-only preview shape', () => {
    expectTypeOf<NodeStaleRotationPreview>().toMatchTypeOf<{
      node_id: string;
      stale_rotation_detected: boolean;
      token_rotation_status: string;
      evaluated_at: string;
      candidates: NodeStaleRotationCandidate[];
    }>();
    expectTypeOf<NodeStaleRotationCandidate>().toMatchTypeOf<{
      job_id: string;
      status: string;
      created_at: string;
      started_at?: string;
      last_claim_at?: string;
      last_result_at?: string;
      last_seen_at?: string;
      last_poll_at?: string;
      age_seconds: number;
      stale_reason: string;
      safe_to_clear: boolean;
    }>();
  });
});

describe('node stale rotation clear API types', () => {
  it('keeps the exact request, cleared-job and unwrapped result contracts', () => {
    expectTypeOf<NodeStaleRotationClearInput>().toEqualTypeOf<{
      confirmation: string;
      reason: string;
      acknowledge_cancel_rotation: boolean;
      expected_job_ids: string[];
    }>();
    expectTypeOf<NodeStaleRotationClearInput>().not.toHaveProperty('node_id');
    expectTypeOf<NodeStaleRotationClearInput>().not.toHaveProperty('evaluated_at');
    expectTypeOf<NodeStaleRotationClearInput>().not.toHaveProperty('candidates');

    expectTypeOf<NodeStaleRotationClearedJob>().toEqualTypeOf<{
      job_id: string;
      previous_status: string;
      status: string;
      stale_reason: string;
      finished_at: string;
    }>();
    expectTypeOf<NodeStaleRotationClearResult>().toEqualTypeOf<{
      status: string;
      node_id: string;
      cleared_count: number;
      cleared_jobs: NodeStaleRotationClearedJob[];
      pending_rotation_state_cleared: boolean;
      active_agent_identity_preserved: boolean;
    }>();
    expectTypeOf<NodeStaleRotationClearResult>().not.toHaveProperty('job');
    expectTypeOf<NodeStaleRotationClearResult>().not.toHaveProperty('result');
    expectTypeOf<NodeStaleRotationClearResult>().not.toHaveProperty('token');
    expectTypeOf<NodeStaleRotationClearResult>().not.toHaveProperty('token_hash');
  });
});

describe('node agent identity revoke API types', () => {
  it('keeps the request and response contract exact and redacted', () => {
    expectTypeOf<NodeAgentIdentityRevokeInput>().toEqualTypeOf<{
      confirmation: string;
      reason: string;
    }>();
    expectTypeOf<NodeAgentIdentityRevokeInput>().not.toHaveProperty('node_id');
    expectTypeOf<NodeAgentIdentityRevokeInput>().not.toHaveProperty('acknowledged');
    expectTypeOf<NodeAgentIdentityRevokeInput>().not.toHaveProperty('token');

    expectTypeOf<NodeAgentIdentityRevokeResult>().toEqualTypeOf<{
      status: string;
      node_id: string;
      agent_status: string;
      revoked_at: string;
      already_revoked: boolean;
      revoked_enrollment_tokens: number;
    }>();
    expectTypeOf<NodeAgentIdentityRevokeResult>().not.toHaveProperty('token');
    expectTypeOf<NodeAgentIdentityRevokeResult>().not.toHaveProperty('token_hash');
    expectTypeOf<NodeAgentIdentityRevokeResult>().not.toHaveProperty('token_hint');
    expectTypeOf<NodeAgentIdentityRevokeResult>().not.toHaveProperty('secret_ref');
  });
});

describe('node reboot API types', () => {
  it('keeps the request exact and uses the redacted Job response contract', () => {
    expectTypeOf<NodeRebootInput>().toEqualTypeOf<{
      confirmation: string;
      reason: string;
    }>();
    expectTypeOf<NodeRebootInput>().not.toHaveProperty('node_id');
    expectTypeOf<NodeRebootInput>().not.toHaveProperty('acknowledged');
    expectTypeOf<NodeRebootInput>().not.toHaveProperty('maintenance');
    expectTypeOf<NodeRebootInput>().not.toHaveProperty('command');
    expectTypeOf<NodeRebootInput>().not.toHaveProperty('token');

    expectTypeOf<Job>().toHaveProperty('id');
    expectTypeOf<Job>().toHaveProperty('type');
    expectTypeOf<Job>().toHaveProperty('status');
    expectTypeOf<Job>().toHaveProperty('scope_id');
    expectTypeOf<Job>().toHaveProperty('node_id');
    expectTypeOf<Job>().toHaveProperty('created_at');
  });
});

describe('node Emergency Cleanup API types', () => {
  it('keeps exact scope, request, plan summary and response wrapper contracts', () => {
    expectTypeOf<NodeEmergencyCleanupScope>().toEqualTypeOf<'services_only' | 'full_node'>();
    expectTypeOf<NodeEmergencyCleanupInput>().toEqualTypeOf<{
      cleanup_scope: NodeEmergencyCleanupScope;
      include_agent: boolean;
      confirmation: string;
      reason: string;
      acknowledge_destructive_cleanup: boolean;
      acknowledge_agent_removal: boolean;
    }>();
    expectTypeOf<NodeEmergencyCleanupInput>().not.toHaveProperty('node_id');
    expectTypeOf<NodeEmergencyCleanupInput>().not.toHaveProperty('instances');
    expectTypeOf<NodeEmergencyCleanupInput>().not.toHaveProperty('paths');
    expectTypeOf<NodeEmergencyCleanupInput>().not.toHaveProperty('units');
    expectTypeOf<NodeEmergencyCleanupInput>().not.toHaveProperty('commands');

    expectTypeOf<NodeEmergencyCleanupPlanSummary>().toEqualTypeOf<{
      cleanup_scope: NodeEmergencyCleanupScope;
      include_agent: boolean;
      instance_target_count: number;
      service_counts: Record<string, number>;
      node_runtime_cleanup: boolean;
      agent_removal_requested: boolean;
    }>();
    expectTypeOf<NodeEmergencyCleanupPlanSummary>().not.toHaveProperty('targets');
    expectTypeOf<NodeEmergencyCleanupPlanSummary>().not.toHaveProperty('managed_paths');
    expectTypeOf<NodeEmergencyCleanupPlanSummary>().not.toHaveProperty('systemd_units');

    expectTypeOf<NodeEmergencyCleanupResult>().toEqualTypeOf<{
      status: string;
      message: string;
      job: Job;
      plan_summary: NodeEmergencyCleanupPlanSummary;
    }>();
    expectTypeOf<NodeEmergencyCleanupResult>().not.toHaveProperty('node');
    expectTypeOf<NodeEmergencyCleanupResult>().not.toHaveProperty('diagnostics');
  });
});
