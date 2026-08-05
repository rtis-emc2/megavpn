import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, CardBody, ErrorState, LoadingSkeleton, PageHeader } from '../shared/ui';

export function PageScaffold({ title, subtitle, actions, children }: { title: string; subtitle?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <div className="page-stack">
      <PageHeader title={title} subtitle={subtitle} actions={actions} />
      {children}
    </div>
  );
}

export function QueryBoundary({ isLoading, isError, error, refetch, children }: {
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  refetch?: () => void;
  children: ReactNode;
}) {
  if (isLoading) return <LoadingSkeleton />;
  if (isError) return <ErrorState body={error?.message} onRetry={refetch} />;
  return <>{children}</>;
}

export function ComingSoonCard({ capability }: { capability?: string }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <h2 className="card-title">{t('common.comingSoon')}</h2>
          <p>{t('common.unsupportedAction')}</p>
          {capability ? <code>{capability}</code> : null}
        </div>
      </CardBody>
    </Card>
  );
}
