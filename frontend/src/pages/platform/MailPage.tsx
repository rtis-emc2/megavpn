import { MailCheck, Save } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { MailSettingsInput } from '../../shared/api/types';
import { useAuth } from '../../shared/auth/AuthProvider';
import { hasPermission } from '../../shared/permissions/permissions';
import { useMailSettings, useTestMailSettings, useUpdateMailSettings } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, FormField, FormGrid, Select, StatusBadge, TextField, Toolbar } from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type MailForm = {
  enabled: boolean;
  smtpHost: string;
  smtpPort: string;
  smtpUsername: string;
  smtpPassword: string;
  smtpAuthMode: string;
  smtpTLSMode: string;
  fromEmail: string;
  fromName: string;
  replyToEmail: string;
  inviteURLBase: string;
};

const emptyMailForm: MailForm = {
  enabled: false,
  smtpHost: '',
  smtpPort: '587',
  smtpUsername: '',
  smtpPassword: '',
  smtpAuthMode: 'plain',
  smtpTLSMode: 'starttls',
  fromEmail: '',
  fromName: '',
  replyToEmail: '',
  inviteURLBase: '',
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

function fieldErrors(error: unknown): Record<string, string> {
  if (!(error instanceof APIError) || typeof error.payload !== 'object' || error.payload === null) return {};
  const payload = error.payload as { fields?: unknown };
  if (!payload.fields || typeof payload.fields !== 'object') return {};
  return Object.fromEntries(Object.entries(payload.fields as Record<string, unknown>).map(([key, value]) => [key, String(value)]));
}

function formFromMail(settings: Record<string, unknown> | undefined): MailForm {
  if (!settings) return emptyMailForm;
  return {
    enabled: Boolean(settings.enabled),
    smtpHost: text(settings.smtp_host, ''),
    smtpPort: text(settings.smtp_port, '587'),
    smtpUsername: text(settings.smtp_username, ''),
    smtpPassword: '',
    smtpAuthMode: text(settings.smtp_auth_mode, 'plain'),
    smtpTLSMode: text(settings.smtp_tls_mode, 'starttls'),
    fromEmail: text(settings.from_email, ''),
    fromName: text(settings.from_name, ''),
    replyToEmail: text(settings.reply_to_email, ''),
    inviteURLBase: text(settings.invite_url_base, ''),
  };
}

function toInput(form: MailForm, passwordSecretRefID?: string | null): MailSettingsInput {
  return {
    enabled: form.enabled,
    smtp_host: form.smtpHost,
    smtp_port: Number(form.smtpPort) || 587,
    smtp_username: form.smtpUsername,
    smtp_password_secret_ref_id: form.smtpPassword ? undefined : passwordSecretRefID || undefined,
    smtp_password: form.smtpPassword || undefined,
    smtp_auth_mode: form.smtpAuthMode,
    smtp_tls_mode: form.smtpTLSMode,
    from_email: form.fromEmail,
    from_name: form.fromName,
    reply_to_email: form.replyToEmail,
    invite_url_base: form.inviteURLBase,
  };
}

export function MailPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const auth = useAuth();
  const canManage = hasPermission(auth.permissions, auth.roles, 'auth.manage');
  const mail = useMailSettings({ retry: false });
  const update = useUpdateMailSettings();
  const testMail = useTestMailSettings();
  const serverForm = useMemo(() => formFromMail(mail.data), [mail.data]);
  const [draft, setDraft] = useState<MailForm | null>(null);
  const [testEmail, setTestEmail] = useState('');
  const [notice, setNotice] = useState('');
  const errors = fieldErrors(update.error);
  const form = draft || serverForm;
  const dirty = draft !== null && JSON.stringify(draft) !== JSON.stringify(serverForm);

  const patch = (partial: Partial<MailForm>) => setDraft((current) => ({ ...(current || serverForm), ...partial }));
  const save = async () => {
    setNotice('');
    try {
      await update.mutateAsync(toInput(form, mail.data?.smtp_password_secret_ref_id));
      setDraft(null);
      setNotice(t('settings.mailSaved'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };
  const sendTest = async () => {
    setNotice('');
    try {
      const result = await testMail.mutateAsync({ email: testEmail });
      setNotice(result.message || t('settings.mailTestSent'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold title={t('nav.mail')} subtitle={t('settings.mail')} actions={<Button icon={<Save size={16} />} variant="primary" disabled={!canManage || !dirty || update.isPending} onClick={() => void save()}>{t('common.save')}</Button>}>
      <QueryBoundary isLoading={mail.isLoading} isError={mail.isError} error={mail.error} refetch={() => void mail.refetch()}>
        <Card>
          <CardBody>
            <div className="page-stack">
              <Toolbar>
                <StatusBadge status={mail.data?.enabled ? 'enabled' : 'disabled'} />
                {mail.data?.smtp_password_configured ? <Badge>{t('settings.smtpPasswordConfigured')}</Badge> : <Badge>{t('settings.smtpPasswordMissing')}</Badge>}
                {!canManage ? <Badge>{t('common.permissionRequired', { permission: 'auth.manage' })}</Badge> : null}
              </Toolbar>
              {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
              {update.error ? <div role="alert" className="error-state-inline">{formatAPIError(update.error)}</div> : null}
              <FormGrid>
                <FormField label={t('settings.enabled')}>
                  <label className="toolbar">
                    <input type="checkbox" checked={form.enabled} onChange={(event) => patch({ enabled: event.target.checked })} />
                    <span>{form.enabled ? t('common.enabled') : t('common.disabled')}</span>
                  </label>
                </FormField>
                <FormField label={t('settings.smtpHost')}>
                  <TextField value={form.smtpHost} onChange={(event) => patch({ smtpHost: event.target.value })} />
                  {errors.smtp_host ? <span role="alert">{errors.smtp_host}</span> : null}
                </FormField>
                <FormField label={t('settings.smtpPort')}>
                  <TextField type="number" min={1} max={65535} value={form.smtpPort} onChange={(event) => patch({ smtpPort: event.target.value })} />
                </FormField>
                <FormField label={t('settings.smtpUsername')}>
                  <TextField value={form.smtpUsername} onChange={(event) => patch({ smtpUsername: event.target.value })} />
                </FormField>
                <FormField label={t('settings.smtpPassword')}>
                  <TextField type="password" autoComplete="new-password" value={form.smtpPassword} placeholder={mail.data?.smtp_password_configured ? t('settings.keepExistingSecret') : ''} onChange={(event) => patch({ smtpPassword: event.target.value })} />
                  {errors.smtp_password ? <span role="alert">{errors.smtp_password}</span> : null}
                </FormField>
                <FormField label={t('settings.smtpAuthMode')}>
                  <Select value={form.smtpAuthMode} onChange={(event) => patch({ smtpAuthMode: event.target.value })}>
                    <option value="none">none</option>
                    <option value="plain">plain</option>
                    <option value="login">login</option>
                  </Select>
                </FormField>
                <FormField label={t('settings.smtpTLSMode')}>
                  <Select value={form.smtpTLSMode} onChange={(event) => patch({ smtpTLSMode: event.target.value })}>
                    <option value="none">none</option>
                    <option value="starttls">starttls</option>
                    <option value="tls">tls</option>
                  </Select>
                </FormField>
                <FormField label={t('settings.fromEmail')}>
                  <TextField value={form.fromEmail} onChange={(event) => patch({ fromEmail: event.target.value })} />
                  {errors.from_email ? <span role="alert">{errors.from_email}</span> : null}
                </FormField>
                <FormField label={t('settings.fromName')}>
                  <TextField value={form.fromName} onChange={(event) => patch({ fromName: event.target.value })} />
                </FormField>
                <FormField label={t('settings.replyToEmail')}>
                  <TextField value={form.replyToEmail} onChange={(event) => patch({ replyToEmail: event.target.value })} />
                </FormField>
                <FormField label={t('settings.inviteURLBase')}>
                  <TextField value={form.inviteURLBase} onChange={(event) => patch({ inviteURLBase: event.target.value })} />
                </FormField>
              </FormGrid>
              <div className="definition-grid">
                <span>{t('settings.lastTest')}</span><strong>{fmt.date(mail.data?.last_test_at)}</strong>
                <span>{t('settings.lastError')}</span><strong>{text(mail.data?.last_error)}</strong>
              </div>
              <Toolbar>
                <Button icon={<Save size={16} />} variant="primary" disabled={!canManage || !dirty || update.isPending} onClick={() => void save()}>{t('common.save')}</Button>
              </Toolbar>
            </div>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <div className="page-stack">
              <h2 className="card-title">{t('settings.mailTest')}</h2>
              {testMail.error ? <div role="alert" className="error-state-inline">{formatAPIError(testMail.error)}</div> : null}
              <FormGrid>
                <FormField label={t('settings.testEmail')}>
                  <TextField value={testEmail} onChange={(event) => setTestEmail(event.target.value)} />
                </FormField>
              </FormGrid>
              <Toolbar>
                <Button icon={<MailCheck size={16} />} disabled={!canManage || !testEmail || testMail.isPending} onClick={() => void sendTest()}>{t('settings.sendTest')}</Button>
              </Toolbar>
            </div>
          </CardBody>
        </Card>
      </QueryBoundary>
    </PageScaffold>
  );
}
