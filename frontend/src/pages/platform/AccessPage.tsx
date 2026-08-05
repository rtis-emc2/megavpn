import { MailPlus, RefreshCw, ShieldOff } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { InviteCreateInput, Session, UserAccount } from '../../shared/api/types';
import { useAuth } from '../../shared/auth/AuthProvider';
import { hasPermission } from '../../shared/permissions/permissions';
import { useCreateInvite, usePlatformInvites, usePlatformSessions, usePlatformUserDetail, usePlatformUsers, useRevokeSession } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, ConfirmDialog, DataTable, Drawer, FormField, FormGrid, Modal, StatusBadge, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type InviteForm = {
  username: string;
  email: string;
  displayName: string;
  roleCodes: string;
  ttlHours: string;
};

const emptyInviteForm: InviteForm = {
  username: '',
  email: '',
  displayName: '',
  roleCodes: 'operator',
  ttlHours: '48',
};

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : 'Request failed';
  const prefix = error.status === 403
    ? 'Permission denied'
    : error.status === 409
      ? 'Conflict'
      : error.status === 422 || error.status === 400
        ? 'Validation failed'
        : `HTTP ${error.status}`;
  return `${prefix}: ${error.message}`;
}

function splitRoles(value: string): string[] {
  return value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean);
}

function sessionStatus(session: Session): string {
  if (session.revoked_at) return 'revoked';
  if (session.expires_at) {
    const expires = new Date(session.expires_at).getTime();
    if (Number.isFinite(expires) && expires < Date.now()) return 'expired';
  }
  return 'active';
}

function userLabel(user?: UserAccount | null): string {
  if (!user) return 'n/a';
  return text(user.display_name || user.username || user.email || user.id);
}

export function AccessPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const auth = useAuth();
  const canManage = hasPermission(auth.permissions, auth.roles, 'auth.manage');
  const users = usePlatformUsers({ retry: false });
  const invites = usePlatformInvites({ retry: false });
  const sessions = usePlatformSessions({ retry: false });
  const revokeSession = useRevokeSession();
  const [selectedUserId, setSelectedUserId] = useState('');
  const selectedUser = usePlatformUserDetail(selectedUserId || undefined);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [sessionTarget, setSessionTarget] = useState<Session | null>(null);
  const [notice, setNotice] = useState('');
  const selected = selectedUser.data || users.data?.find((item) => item.id === selectedUserId) || null;

  const runRevokeSession = async () => {
    if (!sessionTarget) return;
    setNotice('');
    try {
      await revokeSession.mutateAsync(sessionTarget.id);
      setNotice(t('settings.sessionRevoked'));
      setSessionTarget(null);
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold
      title={t('nav.access')}
      subtitle={t('settings.security')}
      actions={(
        <>
          <Button icon={<MailPlus size={16} />} disabled={!canManage} onClick={() => setInviteOpen(true)}>{t('settings.createInvite')}</Button>
          <Button icon={<RefreshCw size={16} />} onClick={() => { void users.refetch(); void invites.refetch(); void sessions.refetch(); }}>{t('common.refresh')}</Button>
        </>
      )}
    >
      {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
      <QueryBoundary isLoading={users.isLoading} isError={users.isError} error={users.error} refetch={() => void users.refetch()}>
        <DataTable
          title={t('settings.users')}
          rows={users.data || []}
          columns={[
            { key: 'username', header: t('clients.username'), render: (row) => <strong>{text(row.username || row.id)}</strong> },
            { key: 'email', header: t('common.email'), render: (row) => text(row.email) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'unknown')} /> },
            { key: 'roles', header: t('common.roles'), render: (row) => text(Array.isArray(row.roles) ? row.roles.join(', ') : row.roles) },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button onClick={() => setSelectedUserId(row.id)}>{t('common.open')}</Button> },
          ]}
        />
      </QueryBoundary>
      <QueryBoundary isLoading={invites.isLoading} isError={invites.isError} error={invites.error} refetch={() => void invites.refetch()}>
        <DataTable
          title={t('settings.invites')}
          rows={invites.data || []}
          columns={[
            { key: 'email', header: t('common.email'), render: (row) => <strong>{text(row.email)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'unknown')} /> },
            { key: 'hint', header: t('settings.tokenHint'), render: (row) => <code>{text(row.token_hint)}</code> },
            { key: 'expires', header: t('common.expires'), render: (row) => fmt.date(row.expires_at) },
            { key: 'delivery', header: t('settings.delivery'), render: (row) => text(row.delivery_error || row.sent_at) },
            { key: 'actions', header: t('common.actions'), render: () => <Button variant="danger" disabled title={t('settings.inviteRevokeUnsupported')}>{t('settings.revokeInvite')}</Button> },
          ]}
        />
      </QueryBoundary>
      <QueryBoundary isLoading={sessions.isLoading} isError={sessions.isError} error={sessions.error} refetch={() => void sessions.refetch()}>
        <DataTable
          title={t('settings.sessions')}
          rows={sessions.data || []}
          columns={[
            { key: 'id', header: 'ID', render: (row) => <code>{shortID(row.id)}</code> },
            { key: 'user', header: t('common.account'), render: (row) => text(row.username || row.user_id) },
            { key: 'ip', header: 'IP', render: (row) => text(row.ip) },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={sessionStatus(row)} /> },
            { key: 'expires', header: t('common.expires'), render: (row) => fmt.date(row.expires_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button icon={<ShieldOff size={16} />} variant="danger" disabled={!canManage || sessionStatus(row) !== 'active'} onClick={() => setSessionTarget(row)}>{t('settings.revokeSession')}</Button> },
          ]}
        />
      </QueryBoundary>
      <UserDetailDrawer user={selected} open={Boolean(selectedUserId)} onClose={() => setSelectedUserId('')} />
      <InviteModal open={inviteOpen} canManage={canManage} onClose={() => setInviteOpen(false)} onDone={() => setNotice(t('settings.inviteCreated'))} />
      <ConfirmDialog title={t('settings.revokeSessionConfirmTitle')} open={Boolean(sessionTarget)} onClose={() => setSessionTarget(null)}>
        {sessionTarget ? (
          <div className="page-stack">
            <p>{t('settings.revokeSessionConfirmBody', { id: shortID(sessionTarget.id), user: sessionTarget.username || sessionTarget.user_id })}</p>
            <Toolbar>
              <Button variant="danger" disabled={revokeSession.isPending} onClick={() => void runRevokeSession()}>{t('common.apply')}</Button>
              <Button onClick={() => setSessionTarget(null)}>{t('common.cancel')}</Button>
            </Toolbar>
          </div>
        ) : null}
      </ConfirmDialog>
    </PageScaffold>
  );
}

function UserDetailDrawer({ user, open, onClose }: { user: UserAccount | null; open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <Drawer title={userLabel(user)} open={open} onClose={onClose}>
      {user ? (
        <div className="page-stack">
          <Toolbar>
            <StatusBadge status={user.status} />
            {Array.isArray(user.roles) ? user.roles.map((role) => <Badge key={role}>{role}</Badge>) : null}
          </Toolbar>
          <Card>
            <CardBody>
              <div className="definition-grid">
                <span>{t('common.id')}</span><strong>{user.id}</strong>
                <span>{t('clients.username')}</span><strong>{text(user.username)}</strong>
                <span>{t('common.email')}</span><strong>{text(user.email)}</strong>
                <span>{t('common.name')}</span><strong>{text(user.display_name)}</strong>
                <span>{t('settings.authSource')}</span><strong>{text(user.auth_source)}</strong>
                <span>{t('settings.lastLogin')}</span><strong>{fmt.date(user.last_login_at)}</strong>
                <span>{t('common.created')}</span><strong>{fmt.date(user.created_at)}</strong>
                <span>{t('common.updated')}</span><strong>{fmt.date(user.updated_at)}</strong>
              </div>
            </CardBody>
          </Card>
        </div>
      ) : null}
    </Drawer>
  );
}

function InviteModal({ open, canManage, onClose, onDone }: { open: boolean; canManage: boolean; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation();
  const createInvite = useCreateInvite();
  const [form, setForm] = useState<InviteForm>(emptyInviteForm);
  const [notice, setNotice] = useState('');
  const reset = () => {
    setForm(emptyInviteForm);
    setNotice('');
  };
  const close = () => {
    reset();
    onClose();
  };
  const submit = async () => {
    setNotice('');
    const input: InviteCreateInput = {
      username: form.username,
      email: form.email,
      display_name: form.displayName,
      role_codes: splitRoles(form.roleCodes),
      ttl_hours: Number(form.ttlHours) || 48,
    };
    try {
      await createInvite.mutateAsync(input);
      reset();
      onClose();
      onDone();
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  return (
    <Modal title={t('settings.createInvite')} open={open} onClose={close}>
      <div className="page-stack">
        {notice ? <div role="alert">{notice}</div> : null}
        {!canManage ? <Badge>{t('common.permissionRequired', { permission: 'auth.manage' })}</Badge> : null}
        <FormGrid>
          <FormField label={t('clients.username')}>
            <TextField value={form.username} onChange={(event) => setForm((current) => ({ ...current, username: event.target.value }))} />
          </FormField>
          <FormField label={t('common.email')}>
            <TextField value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
          </FormField>
          <FormField label={t('common.name')}>
            <TextField value={form.displayName} onChange={(event) => setForm((current) => ({ ...current, displayName: event.target.value }))} />
          </FormField>
          <FormField label={t('common.roles')}>
            <TextField value={form.roleCodes} onChange={(event) => setForm((current) => ({ ...current, roleCodes: event.target.value }))} />
          </FormField>
          <FormField label={t('settings.ttlHours')}>
            <TextField type="number" min={1} value={form.ttlHours} onChange={(event) => setForm((current) => ({ ...current, ttlHours: event.target.value }))} />
          </FormField>
        </FormGrid>
        <Toolbar>
          <Button variant="primary" disabled={!canManage || !form.username || !form.email || createInvite.isPending} onClick={() => void submit()}>{t('settings.createInvite')}</Button>
          <Button onClick={close}>{t('common.cancel')}</Button>
        </Toolbar>
      </div>
    </Modal>
  );
}
