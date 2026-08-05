import { useTranslation } from 'react-i18next';
import { Button } from './Button';

export function LoadingSkeleton({ label }: { label?: string }) {
  const { t } = useTranslation();
  return <div className="loading-state">{label || t('common.loading')}</div>;
}

export function EmptyState({ title, body }: { title?: string; body?: string }) {
  const { t } = useTranslation();
  return (
    <div className="empty-state">
      <strong>{title || t('common.emptyTitle')}</strong>
      <span>{body || t('common.emptyBody')}</span>
    </div>
  );
}

export function ErrorState({ title, body, onRetry }: { title?: string; body?: string; onRetry?: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="error-state">
      <strong>{title || t('common.errorTitle')}</strong>
      <span>{body || t('common.errorBody')}</span>
      {onRetry ? <div><Button onClick={onRetry}>{t('common.retry')}</Button></div> : null}
    </div>
  );
}
