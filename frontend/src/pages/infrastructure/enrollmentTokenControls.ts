export const ENROLLMENT_TOKEN_TTL_MIN_HOURS = 1;
export const ENROLLMENT_TOKEN_TTL_MAX_HOURS = 720;
export const ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS = 24;

export type EnrollmentTokenTTLValidation = {
  ttlHours: number;
  errorKey?: 'required' | 'integer' | 'range';
};

export function validateEnrollmentTokenTTL(value: string | number): EnrollmentTokenTTLValidation {
  const raw = String(value).trim();
  if (!raw) {
    return { ttlHours: ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS, errorKey: 'required' };
  }
  if (!/^\d+$/.test(raw)) {
    return { ttlHours: ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS, errorKey: 'integer' };
  }
  const parsed = Number(raw);
  if (!Number.isSafeInteger(parsed)) {
    return { ttlHours: ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS, errorKey: 'integer' };
  }
  if (parsed < ENROLLMENT_TOKEN_TTL_MIN_HOURS || parsed > ENROLLMENT_TOKEN_TTL_MAX_HOURS) {
    return { ttlHours: parsed, errorKey: 'range' };
  }
  return { ttlHours: parsed };
}
