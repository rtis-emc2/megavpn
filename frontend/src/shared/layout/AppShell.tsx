import { Menu, RefreshCw } from 'lucide-react';
import { NavLink, Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { navSections } from '../config/navigation';
import { useAuth } from '../auth/AuthProvider';
import { getAPIBase } from '../api/client';
import { hasPermission } from '../permissions/permissions';
import { setLocale, supportedLocales, type SupportedLocale } from '../i18n';
import { useReady, useVersion } from '../query/hooks';
import { Button, IconButton, StatusBadge } from '../ui';

export function AppShell() {
  const { t, i18n } = useTranslation();
  const auth = useAuth();
  const ready = useReady();
  const version = useVersion();

  const filteredSections = navSections.map((section) => ({
    ...section,
    items: section.items.filter((item) => hasPermission(auth.permissions, auth.roles, item.permission)),
  })).filter((section) => section.items.length);

  const displayName = auth.session?.user?.display_name || auth.session?.user?.username || auth.session?.user?.email || t('common.operator');

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <div className="app-brand">
          <div className="app-brand-mark">{t('common.brandShort')}</div>
          <div className="app-brand-copy">
            <div className="app-brand-title">{t('common.brandProduct')}</div>
            <div className="app-brand-subtitle">{t('common.newConsole')}</div>
          </div>
        </div>

        <nav className="app-nav">
          {filteredSections.map((section) => {
            const SectionIcon = section.icon;
            return (
              <section className="app-nav-section" key={section.id}>
                <div className="app-nav-section-title">
                  <SectionIcon size={14} />
                  <span>{t(section.labelKey)}</span>
                </div>
                {section.items.map((item) => {
                  const ItemIcon = item.icon;
                  const external = item.path.startsWith('/legacy');
                  const content = (
                    <>
                      <ItemIcon size={17} strokeWidth={2.2} />
                      <span>{t(item.labelKey)}</span>
                    </>
                  );
                  return external ? (
                    <a className="app-nav-link" href={item.path} key={item.id}>{content}</a>
                  ) : (
                    <NavLink className="app-nav-link" to={item.path} end={item.path === '/'} key={item.id}>{content}</NavLink>
                  );
                })}
              </section>
            );
          })}
        </nav>

        <div className="app-sidebar-footer">
          <small>{t('common.apiBase')}</small>
          <code>{getAPIBase() || t('common.currentHost')}</code>
          <small>{t('common.currentRelease', { version: version.data?.version || '8.0.0' })}</small>
        </div>
      </aside>

      <main className="app-main">
        <header className="topbar">
          <div className="toolbar">
            <IconButton title={t('nav.infrastructure')}><Menu size={18} /></IconButton>
            <StatusBadge status={ready.data?.status || (ready.isError ? 'failed' : 'unknown')} />
          </div>
          <div className="topbar-actions">
            <select
              className="select"
              aria-label={t('common.language')}
              value={i18n.language.slice(0, 2)}
              onChange={(event) => void setLocale(event.currentTarget.value as SupportedLocale)}
            >
              {supportedLocales.map((locale) => (
                <option value={locale} key={locale}>{locale === 'ru' ? t('common.russian') : t('common.english')}</option>
              ))}
            </select>
            <Button icon={<RefreshCw size={16} />} onClick={() => { void ready.refetch(); }}>
              {t('common.refresh')}
            </Button>
            <div className="badge">
              <span>{displayName}</span>
            </div>
            <Button variant="ghost" onClick={() => { void auth.logout(); }}>{t('auth.logout')}</Button>
          </div>
        </header>
        <div className="content-shell">
          <Outlet />
        </div>
      </main>
    </div>
  );
}
