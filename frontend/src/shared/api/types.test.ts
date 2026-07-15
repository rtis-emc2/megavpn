import { describe, expectTypeOf, it } from 'vitest';
import type { EnrollmentToken, EnrollmentTokenIssueResult, NodeStaleRotationCandidate, NodeStaleRotationPreview } from './types';

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
