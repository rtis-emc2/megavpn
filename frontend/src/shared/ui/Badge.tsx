import type { ReactNode } from 'react';

export function Badge({ children }: { children: ReactNode }) {
  return <span className="badge">{children}</span>;
}

function normalizeStatus(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9_-]+/g, '-');
}

export function StatusBadge({ status }: { status?: string | null }) {
  const value = status || 'unknown';
  return (
    <span className={`badge status-${normalizeStatus(value)}`}>
      <span className="badge-dot" aria-hidden="true" />
      {value}
    </span>
  );
}
