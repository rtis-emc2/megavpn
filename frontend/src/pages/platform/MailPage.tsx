import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { endpoints } from '../../shared/api/endpoints';
import { Button, Card, CardBody, StatusBadge } from '../../shared/ui';
import { PageScaffold } from '../common';

export function MailPage() {
  const { t } = useTranslation();
  const mail = useQuery({ queryKey: ['mail-settings'], queryFn: endpoints.mailSettings, retry: false, staleTime: 30_000 });
  return (
    <PageScaffold title={t('nav.mail')} subtitle={t('settings.mail')} actions={<Button disabled>{t('settings.mail')}</Button>}>
      <Card>
        <CardBody>
          <div className="page-stack">
            <StatusBadge status={mail.isError ? 'failed' : mail.isLoading ? 'pending' : 'configured'} />
            <pre className="code-block">{JSON.stringify(mail.data || {}, null, 2)}</pre>
          </div>
        </CardBody>
      </Card>
    </PageScaffold>
  );
}
