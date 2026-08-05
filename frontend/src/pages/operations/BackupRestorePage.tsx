import { useTranslation } from 'react-i18next';
import { Button, Card, CardBody } from '../../shared/ui';
import { PageScaffold } from '../common';

export function BackupRestorePage() {
  const { t } = useTranslation();
  return (
    <PageScaffold title={t('nav.backupRestore')} subtitle={t('common.unsupportedAction')}>
      <Card>
        <CardBody>
          <Button disabled>{t('common.apply')}</Button>
        </CardBody>
      </Card>
    </PageScaffold>
  );
}
