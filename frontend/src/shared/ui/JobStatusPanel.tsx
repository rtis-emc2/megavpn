import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../api/endpoints';
import { Card, CardBody } from './Card';
import { StatusBadge } from './Badge';
import { LoadingSkeleton } from './State';

export function JobStatusPanel({ jobID }: { jobID: string }) {
  const { t } = useTranslation();
  const job = useQuery({
    queryKey: ['job', jobID],
    queryFn: () => endpoints.job(jobID),
    enabled: Boolean(jobID),
    refetchInterval: 5_000,
  });
  const logs = useQuery({
    queryKey: ['job', jobID, 'logs'],
    queryFn: () => endpoints.jobLogs(jobID),
    enabled: Boolean(jobID),
    refetchInterval: 5_000,
  });

  if (job.isLoading) return <LoadingSkeleton />;

  return (
    <Card>
      <CardBody>
        <div className="page-stack">
          <div className="toolbar">
            <strong>{t('jobs.job')}</strong>
            <code>{jobID}</code>
            <StatusBadge status={job.data?.status} />
          </div>
          <pre className="code-block">{JSON.stringify(job.data?.result || {}, null, 2)}</pre>
          <strong>{t('jobs.logs')}</strong>
          <pre className="code-block">{JSON.stringify(logs.data || [], null, 2)}</pre>
        </div>
      </CardBody>
    </Card>
  );
}
