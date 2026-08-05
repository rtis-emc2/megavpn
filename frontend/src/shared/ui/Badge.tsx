import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

export function Badge({ children }: { children: ReactNode }) {
  return <span className="badge">{children}</span>;
}

function normalizeStatus(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9_-]+/g, '-');
}

export function StatusBadge({ status, label, title }: { status?: string | null; label?: ReactNode; title?: string }) {
  const { t } = useTranslation();
  const value = status || 'unknown';
  const normalized = normalizeStatus(value);
  const displayLabel = label || t(`statusValues.${normalized}`, { defaultValue: value });
  return (
    <span className={`badge status-badge status-${normalized}`} title={title}>
      <span className="badge-dot" aria-hidden="true" />
      <span className="badge-label">{displayLabel}</span>
    </span>
  );
}
