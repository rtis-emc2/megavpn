import { CheckCircle2, DatabaseBackup, Download, ShieldCheck, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { BackupRecord } from '../../shared/api/types';
import { useAuth } from '../../shared/auth/AuthProvider';
import { useBackups, useCreateBackup, useDeleteBackup, useDownloadBackup, useVerifyBackup } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, RefreshButton, StatusBadge, Toolbar } from '../../shared/ui';
import { useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : 'Request failed';
  return `${error.status}: ${error.message}`;
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return 'n/a';
  if (value < 1024) return `${value} B`;
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MiB`;
  return `${(value / 1024 ** 3).toFixed(1)} GiB`;
}

export function BackupRestorePage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const auth = useAuth();
  const isSuperadmin = auth.roles.includes('superadmin');
  const backups = useBackups({ retry: false, enabled: isSuperadmin });
  const createBackup = useCreateBackup();
  const verifyBackup = useVerifyBackup();
  const downloadBackup = useDownloadBackup();
  const deleteBackup = useDeleteBackup();
  const [deleteTarget, setDeleteTarget] = useState<BackupRecord | null>(null);
  const [notice, setNotice] = useState('');
  const busy = createBackup.isPending || verifyBackup.isPending || downloadBackup.isPending || deleteBackup.isPending;

  const create = async () => {
    setNotice('');
    try {
      await createBackup.mutateAsync();
      setNotice(t('backupRestore.createSuccess'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const verify = async (backup: BackupRecord) => {
    setNotice('');
    try {
      await verifyBackup.mutateAsync(backup.id);
      setNotice(t('backupRestore.verifySuccess', { name: backup.filename }));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const download = async (backup: BackupRecord) => {
    setNotice('');
    try {
      const file = await downloadBackup.mutateAsync(backup.id);
      const url = URL.createObjectURL(file.blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = file.filename;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  const remove = async () => {
    if (!deleteTarget) return;
    setNotice('');
    try {
      await deleteBackup.mutateAsync(deleteTarget.id);
      setNotice(t('backupRestore.deleteSuccess'));
      setDeleteTarget(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold
      title={t('backupRestore.title')}
      subtitle={t('backupRestore.subtitle')}
      actions={(
        <>
          <Button variant="primary" icon={<DatabaseBackup size={16} />} disabled={!isSuperadmin || busy} onClick={() => void create()}>{t('backupRestore.create')}</Button>
          <RefreshButton disabled={!isSuperadmin} onRefresh={() => backups.refetch()}>{t('common.refresh')}</RefreshButton>
        </>
      )}
    >
      {!isSuperadmin ? <Badge>{t('backupRestore.superadminOnly')}</Badge> : null}
      {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
      <Card>
        <CardBody>
          <div className="page-stack">
            <div className="toolbar"><ShieldCheck size={18} /><strong>{t('backupRestore.safetyTitle')}</strong></div>
            <p>{t('backupRestore.safetyBody')}</p>
            <p>{t('backupRestore.restoreBody')}</p>
          </div>
        </CardBody>
      </Card>
      <QueryBoundary isLoading={isSuperadmin && backups.isLoading} isError={Boolean(backups.error)} error={backups.error} refetch={() => void backups.refetch()}>
        <DataTable
          title={t('backupRestore.archives')}
          rows={backups.data || []}
          responsive="wide"
          columns={[
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
            { key: 'archive', header: t('backupRestore.archive'), render: (row) => <strong>{row.filename}</strong> },
            { key: 'size', header: t('common.size'), render: (row) => formatBytes(row.size_bytes) },
            { key: 'integrity', header: t('backupRestore.integrity'), render: (row) => <StatusBadge status={row.verified ? 'verified' : 'unverified'} /> },
            { key: 'checksum', header: 'SHA-256', render: (row) => <code title={row.sha256}>{row.sha256.slice(0, 16)}...</code> },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (row) => (
                <Toolbar>
                  <Button icon={<CheckCircle2 size={16} />} disabled={busy} onClick={() => void verify(row)}>{t('backupRestore.verify')}</Button>
                  <Button icon={<Download size={16} />} disabled={busy} onClick={() => void download(row)}>{t('common.download')}</Button>
                  <Button variant="danger" icon={<Trash2 size={16} />} disabled={busy} onClick={() => setDeleteTarget(row)}>{t('common.delete')}</Button>
                </Toolbar>
              ),
            },
          ]}
        />
      </QueryBoundary>
      <ConfirmDialog title={t('backupRestore.deleteTitle')} open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)}>
        <div className="page-stack">
          <p>{t('backupRestore.deleteBody', { name: deleteTarget?.filename || '' })}</p>
          <Toolbar>
            <Button variant="danger" disabled={deleteBackup.isPending} onClick={() => void remove()}>{t('common.delete')}</Button>
            <Button onClick={() => setDeleteTarget(null)}>{t('common.cancel')}</Button>
          </Toolbar>
        </div>
      </ConfirmDialog>
    </PageScaffold>
  );
}
