import { Search, X } from 'lucide-react';
import {
  type InputHTMLAttributes,
  type ReactNode,
} from 'react';
import { Button, IconButton } from './Button';

export function LoadingSkeletonList({
  rows = 3,
  label = 'Loading',
}: {
  rows?: number;
  label?: string;
}) {
  const safeRows = Math.max(1, Math.min(rows, 12));
  return (
    <div className="skeleton-list" role="status" aria-label={label}>
      <span className="sr-only">{label}</span>
      {Array.from({ length: safeRows }, (_, index) => (
        <span key={index} className="skeleton-row" aria-hidden="true" />
      ))}
    </div>
  );
}

type PaginationProps = {
  page: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  disabled?: boolean;
  previousLabel?: string;
  nextLabel?: string;
  pageLabel?: (page: number, pageCount: number) => string;
};

export function Pagination({
  page,
  pageCount,
  onPageChange,
  disabled = false,
  previousLabel = 'Previous',
  nextLabel = 'Next',
  pageLabel = (current, total) => `Page ${current} of ${total}`,
}: PaginationProps) {
  const safeCount = Math.max(1, pageCount);
  const safePage = Math.min(Math.max(1, page), safeCount);
  return (
    <nav className="pagination" aria-label={pageLabel(safePage, safeCount)}>
      <Button
        type="button"
        disabled={disabled || safePage <= 1}
        onClick={() => onPageChange(safePage - 1)}
      >
        {previousLabel}
      </Button>
      <span className="pagination-label" aria-live="polite">
        {pageLabel(safePage, safeCount)}
      </span>
      <Button
        type="button"
        disabled={disabled || safePage >= safeCount}
        onClick={() => onPageChange(safePage + 1)}
      >
        {nextLabel}
      </Button>
    </nav>
  );
}

export function PreviewPanel({
  title,
  status,
  children,
}: {
  title: ReactNode;
  status?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="preview-panel" aria-live="polite">
      <header className="preview-panel-head">
        <h3>{title}</h3>
        {status}
      </header>
      <div className="preview-panel-body">{children}</div>
    </section>
  );
}

type ApplyFooterProps = {
  primaryLabel: ReactNode;
  onApply: () => void;
  secondaryLabel?: ReactNode;
  onCancel?: () => void;
  disabled?: boolean;
  pending?: boolean;
  pendingLabel?: ReactNode;
  notice?: ReactNode;
};

export function ApplyFooter({
  primaryLabel,
  onApply,
  secondaryLabel,
  onCancel,
  disabled = false,
  pending = false,
  pendingLabel,
  notice,
}: ApplyFooterProps) {
  return (
    <footer className="apply-footer">
      <div className="apply-footer-notice" role={pending ? 'status' : undefined} aria-live="polite">
        {notice}
      </div>
      <div className="toolbar">
        {secondaryLabel && onCancel ? (
          <Button type="button" onClick={onCancel} disabled={pending}>
            {secondaryLabel}
          </Button>
        ) : null}
        <Button type="button" variant="primary" onClick={onApply} disabled={disabled || pending}>
          {pending && pendingLabel ? pendingLabel : primaryLabel}
        </Button>
      </div>
    </footer>
  );
}

export type StepperItem = {
  id: string;
  label: ReactNode;
  description?: ReactNode;
  status?: 'complete' | 'current' | 'upcoming' | 'error';
};

export function Stepper({
  steps,
  ariaLabel = 'Progress',
}: {
  steps: StepperItem[];
  ariaLabel?: string;
}) {
  return (
    <ol className="stepper" aria-label={ariaLabel}>
      {steps.map((step, index) => (
        <li
          key={step.id}
          className={`stepper-item stepper-${step.status || 'upcoming'}`}
          aria-current={step.status === 'current' ? 'step' : undefined}
        >
          <span className="stepper-index" aria-hidden="true">{index + 1}</span>
          <span className="stepper-copy">
            <strong>{step.label}</strong>
            {step.description ? <small>{step.description}</small> : null}
          </span>
        </li>
      ))}
    </ol>
  );
}

export function Wizard({
  steps,
  children,
  ariaLabel,
}: {
  steps: StepperItem[];
  children: ReactNode;
  ariaLabel?: string;
}) {
  return (
    <section className="wizard">
      <Stepper steps={steps} ariaLabel={ariaLabel} />
      <div className="wizard-body">{children}</div>
    </section>
  );
}

type SearchInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & {
  label: string;
  clearLabel?: string;
  onClear?: () => void;
};

export function SearchInput({
  label,
  clearLabel = 'Clear search',
  onClear,
  value,
  onChange,
  className = '',
  ...props
}: SearchInputProps) {
  const hasValue = typeof value === 'string' && value.length > 0;
  return (
    <label className={`search-input ${className}`.trim()}>
      <span className="sr-only">{label}</span>
      <Search size={17} aria-hidden="true" />
      <input
        type="search"
        className="input"
        value={value}
        onChange={onChange}
        aria-label={label}
        {...props}
      />
      {hasValue && onClear ? (
        <IconButton type="button" title={clearLabel} onClick={onClear}>
          <X size={16} />
        </IconButton>
      ) : null}
    </label>
  );
}

export function Toast({
  title,
  children,
  tone = 'info',
  dismissLabel = 'Dismiss',
  onDismiss,
}: {
  title: ReactNode;
  children?: ReactNode;
  tone?: 'info' | 'success' | 'warning' | 'danger';
  dismissLabel?: string;
  onDismiss?: () => void;
}) {
  const alert = tone === 'danger' || tone === 'warning';
  return (
    <section
      className={`toast toast-${tone}`}
      role={alert ? 'alert' : 'status'}
      aria-live={alert ? 'assertive' : 'polite'}
    >
      <div>
        <strong>{title}</strong>
        {children ? <div>{children}</div> : null}
      </div>
      {onDismiss ? (
        <IconButton type="button" title={dismissLabel} onClick={onDismiss}>
          <X size={16} />
        </IconButton>
      ) : null}
    </section>
  );
}
