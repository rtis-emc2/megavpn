import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCancelJob, useJobs } from '../../shared/query/hooks';
import { Button, ConfirmDialog, DataTable, Drawer, ErrorState, JobStatusPanel, StatusBadge } from '../../shared/ui';
import type { Job } from '../../shared/api/types';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

const cancellableStatuses = new Set(['queued', 'running', 'retrying', 'pending']);

function canCancelJob(job: Job): boolean {
  return cancellableStatuses.has(String(job.status || '').toLowerCase());
}

export function JobsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const jobs = useJobs();
  const cancelJob = useCancelJob();
  const [selected, setSelected] = useState<Job | null>(null);
  const [cancelTarget, setCancelTarget] = useState<Job | null>(null);

  const closeCancelDialog = () => {
    setCancelTarget(null);
    cancelJob.reset();
  };

  const confirmCancel = async () => {
    if (!cancelTarget) return;
    try {
      await cancelJob.mutateAsync(cancelTarget.id);
      closeCancelDialog();
    } catch {
      // ErrorState renders the mutation error; keep the dialog open.
    }
  };

  return (
    <PageScaffold title={t('jobs.title')} subtitle={t('jobs.subtitle')}>
      <QueryBoundary isLoading={jobs.isLoading} isError={jobs.isError} error={jobs.error} refetch={() => void jobs.refetch()}>
        <DataTable
          rows={jobs.data || []}
          columns={[
            { key: 'kind', header: t('jobs.kind'), render: (row) => <code>{text(row.type)}</code> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={row.status} /> },
            { key: 'scope', header: t('jobs.scope'), render: (row) => [row.scope_type, row.scope_id].filter(Boolean).join(':') || 'n/a' },
            { key: 'updated', header: t('common.updated'), render: (row) => fmt.date(row.updated_at || row.created_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => (
              <div className="toolbar">
                <Button onClick={() => setSelected(row)}>{t('common.open')}</Button>
                <Button
                  variant="danger"
                  disabled={!canCancelJob(row)}
                  title={canCancelJob(row) ? undefined : t('jobs.cancelUnavailable')}
                  onClick={() => {
                    cancelJob.reset();
                    setCancelTarget(row);
                  }}
                >
                  {t('common.cancel')}
                </Button>
              </div>
            ) },
          ]}
        />
      </QueryBoundary>
      <Drawer open={Boolean(selected)} onClose={() => setSelected(null)} title={t('jobs.job')}>
        {selected ? <JobStatusPanel jobID={selected.id} /> : null}
      </Drawer>
      <ConfirmDialog open={Boolean(cancelTarget)} onClose={closeCancelDialog} title={t('jobs.cancelConfirmTitle')}>
        <div className="page-stack">
          <p>{t('jobs.cancelConfirmBody', { id: cancelTarget?.id || '', type: cancelTarget?.type || t('common.unknown') })}</p>
          {cancelJob.isError ? <ErrorState body={cancelJob.error.message} /> : null}
          <div className="toolbar">
            <Button onClick={closeCancelDialog}>{t('common.close')}</Button>
            <Button variant="danger" disabled={cancelJob.isPending} onClick={() => void confirmCancel()}>
              {cancelJob.isPending ? t('jobs.cancelPending') : t('common.cancel')}
            </Button>
          </div>
        </div>
      </ConfirmDialog>
    </PageScaffold>
  );
}
