import { RefreshCw } from 'lucide-react';
import { useEffect, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react';

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  icon?: ReactNode;
};

type RefreshButtonProps = Omit<ButtonProps, 'icon' | 'onClick'> & {
  onRefresh: () => void | Promise<unknown>;
};

export function Button({ variant = 'secondary', icon, children, className = '', type = 'button', ...props }: ButtonProps) {
  return (
    <button type={type} className={`button button-${variant} ${className}`.trim()} {...props}>
      {icon}
      {children ? <span className="button-label">{children}</span> : null}
    </button>
  );
}

export function IconButton({ title, children, className = '', type = 'button', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { title: string }) {
  return (
    <button type={type} className={`button button-secondary icon-button ${className}`.trim()} title={title} aria-label={title} {...props}>
      {children}
    </button>
  );
}

export function RefreshButton({ onRefresh, children, className = '', disabled, ...props }: RefreshButtonProps) {
  const [refreshing, setRefreshing] = useState(false);
  const timerRef = useRef<number | undefined>(undefined);

  useEffect(() => () => {
    if (timerRef.current !== undefined) window.clearTimeout(timerRef.current);
  }, []);

  const refresh = () => {
    if (refreshing || disabled) return;
    const startedAt = Date.now();
    setRefreshing(true);
    void Promise.resolve()
      .then(onRefresh)
      .catch(() => undefined)
      .finally(() => {
        const remaining = Math.max(0, 520 - (Date.now() - startedAt));
        timerRef.current = window.setTimeout(() => setRefreshing(false), remaining);
      });
  };

  return (
    <Button
      {...props}
      className={`button-refresh ${refreshing ? 'is-refreshing' : ''} ${className}`.trim()}
      icon={<RefreshCw size={16} aria-hidden="true" />}
      disabled={disabled || refreshing}
      aria-busy={refreshing}
      onClick={refresh}
    >
      {children}
    </Button>
  );
}
