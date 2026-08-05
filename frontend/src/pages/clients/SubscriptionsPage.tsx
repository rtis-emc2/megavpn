import { useTranslation } from 'react-i18next';
import { Badge, Card, CardBody } from '../../shared/ui';
import { PageScaffold } from '../common';

export function SubscriptionsPage() {
  const { t } = useTranslation();
  return (
    <PageScaffold title={t('nav.subscriptions')} subtitle={t('clients.subscriptions')}>
      <Card>
        <CardBody>
          <div className="page-stack">
            <Badge>{t('common.catalogOnly')}</Badge>
            <p>{t('common.unsupportedAction')}</p>
          </div>
        </CardBody>
      </Card>
    </PageScaffold>
  );
}
