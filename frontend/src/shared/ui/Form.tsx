import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react';

export function FormGrid({ children }: { children: ReactNode }) {
  return <div className="form-grid">{children}</div>;
}

export function FormField({ label, children, full = false }: { label: ReactNode; children: ReactNode; full?: boolean }) {
  return (
    <label className={`form-field ${full ? 'form-field-full' : ''}`.trim()}>
      <span className="form-label">{label}</span>
      {children}
    </label>
  );
}

export function TextField(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input className="input" {...props} />;
}

export function Textarea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className="textarea" {...props} />;
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className="select" {...props} />;
}

export function Checkbox({ label, ...props }: InputHTMLAttributes<HTMLInputElement> & { label: ReactNode }) {
  return (
    <label className="toolbar">
      <input type="checkbox" {...props} />
      <span>{label}</span>
    </label>
  );
}

export function Switch(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input type="checkbox" role="switch" {...props} />;
}
