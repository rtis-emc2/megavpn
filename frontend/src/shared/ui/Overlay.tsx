import { X } from 'lucide-react';
import { useEffect, useId, useRef, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconButton } from './Button';

type OverlayProps = {
  title: ReactNode;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
};

function useEscapeClose(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return undefined;
    const listener = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', listener);
    return () => window.removeEventListener('keydown', listener);
  }, [onClose, open]);
}

export function Drawer({ title, open, onClose, children }: OverlayProps) {
  const { t } = useTranslation();
  const titleID = useId();
  const ref = useRef<HTMLDivElement | null>(null);
  useEscapeClose(open, onClose);
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);
  if (!open) return null;
  return (
    <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <aside className="drawer" role="dialog" aria-modal="true" aria-labelledby={titleID} tabIndex={-1} ref={ref}>
        <div className="overlay-head">
          <h2 id={titleID} className="card-title">{title}</h2>
          <IconButton title={t('common.close')} onClick={onClose}><X size={18} /></IconButton>
        </div>
        <div className="overlay-body">{children}</div>
      </aside>
    </div>
  );
}

export function Modal({ title, open, onClose, children }: OverlayProps) {
  const { t } = useTranslation();
  const titleID = useId();
  const ref = useRef<HTMLDivElement | null>(null);
  useEscapeClose(open, onClose);
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);
  if (!open) return null;
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby={titleID} tabIndex={-1} ref={ref}>
        <div className="overlay-head">
          <h2 id={titleID} className="card-title">{title}</h2>
          <IconButton title={t('common.close')} onClick={onClose}><X size={18} /></IconButton>
        </div>
        <div className="overlay-body">{children}</div>
      </section>
    </div>
  );
}

export function ConfirmDialog({ title, open, onClose, children }: OverlayProps) {
  return <Modal title={title} open={open} onClose={onClose}>{children}</Modal>;
}
