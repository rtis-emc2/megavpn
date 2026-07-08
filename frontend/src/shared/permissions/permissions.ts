export function hasRole(roles: string[], role: string): boolean {
  const expected = role.trim().toLowerCase();
  return roles.some((item) => item.trim().toLowerCase() === expected);
}

export function hasPermission(permissions: string[], roles: string[], permission?: string): boolean {
  if (!permission) return true;
  if (hasRole(roles, 'superadmin')) return true;
  return permissions.includes(permission);
}
