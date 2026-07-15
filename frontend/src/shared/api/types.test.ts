import { describe, expectTypeOf, it } from 'vitest';
import type { EnrollmentToken, EnrollmentTokenIssueResult } from './types';

describe('enrollment token API types', () => {
  it('keeps safe list tokens separate from secret-bearing issue responses', () => {
    expectTypeOf<EnrollmentToken>().not.toHaveProperty('token');
    expectTypeOf<EnrollmentToken>().not.toHaveProperty('enrollment_token');
    expectTypeOf<EnrollmentTokenIssueResult>().toHaveProperty('token');
    expectTypeOf<EnrollmentTokenIssueResult>().toHaveProperty('enrollment_token');
  });
});
