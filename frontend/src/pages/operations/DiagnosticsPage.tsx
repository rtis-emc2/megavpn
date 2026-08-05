import { useTranslation } from 'react-i18next';
import { Card, CardBody, StatusBadge } from '../../shared/ui';
import { useReady } from '../../shared/query/hooks';
import { PageScaffold } from '../common';

export function DiagnosticsPage() {
  const { t } = useTranslation();
  const ready = useReady();
  return (
    <PageScaffold title={t('nav.diagnostics')} subtitle={t('nodes.diagnostics')}>
      <Card>
        <CardBody>
          <div className="toolbar">
            <StatusBadge status={ready.data?.status || 'unknown'} />
            <code>{ready.data?.preflight_status || ready.data?.service || 'n/a'}</code>
          </div>
        </CardBody>
      </Card>
    </PageScaffold>
  );
}
