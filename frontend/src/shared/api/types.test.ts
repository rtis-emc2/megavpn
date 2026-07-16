import { describe, expectTypeOf, it } from 'vitest';
import type { EnrollmentToken, EnrollmentTokenIssueResult, Job, NodeAgentIdentityRevokeInput, NodeAgentIdentityRevokeResult, NodeRebootInput, NodeStaleRotationCandidate, NodeStaleRotationPreview } from './types';

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
