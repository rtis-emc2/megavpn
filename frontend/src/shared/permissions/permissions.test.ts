import { describe, expect, it } from 'vitest';
import { hasPermissions } from './permissions';

describe('hasPermissions', () => {
  it('requires every permission for non-superadmin users', () => {
    expect(hasPermissions(['node.read', 'access_group.read'], ['operator'], ['node.read', 'access_group.read'])).toBe(true);
    expect(hasPermissions(['node.read'], ['operator'], ['node.read', 'access_group.read'])).toBe(false);
    expect(hasPermissions(['access_group.read'], ['operator'], ['node.read', 'access_group.read'])).toBe(false);
  });

  it('keeps the superadmin override consistent with hasPermission', () => {
    expect(hasPermissions([], ['superadmin'], ['node.read', 'access_group.read'])).toBe(true);
  });
});
