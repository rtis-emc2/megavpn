import { useTranslation } from 'react-i18next';
import { PageTabs } from '../../shared/ui';

export function ServicesTabs() {
  const { t } = useTranslation();
  return (
    <PageTabs
      tabs={[
        { label: t('nav.instances'), to: '/services/instances' },
        { label: t('nav.servicePacks'), to: '/services/service-packs' },
        { label: t('nav.runtimeArtifacts'), to: '/services/runtime-artifacts' },
      ]}
    />
  );
}
