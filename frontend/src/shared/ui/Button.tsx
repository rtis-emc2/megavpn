import type { ButtonHTMLAttributes, ReactNode } from 'react';

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  icon?: ReactNode;
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
