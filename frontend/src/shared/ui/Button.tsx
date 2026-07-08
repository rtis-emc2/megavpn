import type { ButtonHTMLAttributes, ReactNode } from 'react';

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  icon?: ReactNode;
};

export function Button({ variant = 'secondary', icon, children, className = '', ...props }: ButtonProps) {
  return (
    <button className={`button button-${variant} ${className}`.trim()} {...props}>
      {icon}
      {children ? <span>{children}</span> : null}
    </button>
  );
}

export function IconButton({ title, children, className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { title: string }) {
  return (
    <button className={`button button-secondary icon-button ${className}`.trim()} title={title} aria-label={title} {...props}>
      {children}
    </button>
  );
}
