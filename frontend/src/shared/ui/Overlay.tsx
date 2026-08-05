import { X } from 'lucide-react';
import { useEffect, useId, useRef, type ReactNode, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { IconButton } from './Button';

type OverlayProps = {
  title: ReactNode;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  size?: 'default' | 'wide';
};

const focusableSelector = [
  'button:not([disabled])',
  '[href]',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

let lockedOverlayCount = 0;

function useDialogLifecycle(
  open: boolean,
  onClose: () => void,
  dialogRef: RefObject<HTMLElement | null>,
) {
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (!open) return undefined;

    const previouslyFocused = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    const dialog = dialogRef.current;

    if (lockedOverlayCount === 0) {
      document.body.classList.add('overlay-open');
    }
    lockedOverlayCount += 1;

    const initialFocus = dialog?.querySelector<HTMLElement>(
      `[autofocus], ${focusableSelector}`,
    );
    (initialFocus || dialog)?.focus();

    const listener = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== 'Tab' || !dialog) return;

      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector));
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || !dialog.contains(active))) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && (active === last || !dialog.contains(active))) {
        event.preventDefault();
        first?.focus();
      }
    };

    window.addEventListener('keydown', listener);
    return () => {
      window.removeEventListener('keydown', listener);
      lockedOverlayCount = Math.max(0, lockedOverlayCount - 1);
      if (lockedOverlayCount === 0) {
        document.body.classList.remove('overlay-open');
      }
      previouslyFocused?.focus();
    };
  }, [dialogRef, open]);
}

export function Drawer({ title, open, onClose, children }: OverlayProps) {
  const { t } = useTranslation();
  const titleID = useId();
  const ref = useRef<HTMLDivElement | null>(null);
  useDialogLifecycle(open, onClose, ref);
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

export function Modal({ title, open, onClose, children, size = 'default' }: OverlayProps) {
  const { t } = useTranslation();
  const titleID = useId();
  const ref = useRef<HTMLDivElement | null>(null);
  useDialogLifecycle(open, onClose, ref);
  if (!open) return null;
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className={`modal ${size === 'wide' ? 'modal-wide' : ''}`.trim()} role="dialog" aria-modal="true" aria-labelledby={titleID} tabIndex={-1} ref={ref}>
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
