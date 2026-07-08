import { useTranslation } from 'react-i18next';
import { Button, Card, CardBody } from '../../shared/ui';
import { PageScaffold } from '../common';

export function RoutePolicyPage() {
  const { t } = useTranslation();
  return (
    <PageScaffold title={t('nav.routePolicy')} subtitle={t('nodes.routePolicy')}>
      <Card>
        <CardBody>
          <div className="page-stack">
            <p>{t('common.unsupportedAction')}</p>
            <Button disabled>{t('common.preview')}</Button>
          </div>
        </CardBody>
      </Card>
    </PageScaffold>
  );
}
