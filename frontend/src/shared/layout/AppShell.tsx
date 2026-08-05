import { ChevronDown, Menu, RefreshCw, X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { navSections } from '../config/navigation';
import { useAuth } from '../auth/AuthProvider';
import { hasPermission, hasPermissions } from '../permissions/permissions';
import { setLocale, supportedLocales, type SupportedLocale } from '../i18n';
import { useReady, useVersion } from '../query/hooks';
import { Button, IconButton, StatusBadge } from '../ui';

export function AppShell() {
  const { t, i18n } = useTranslation();
  const auth = useAuth();
  const location = useLocation();
  const ready = useReady();
  const version = useVersion();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const [compactViewport, setCompactViewport] = useState(() => (
    typeof window.matchMedia === 'function' && window.matchMedia('(max-width: 1023px)').matches
  ));

  const filteredSections = useMemo(() => navSections.map((section) => ({
    ...section,
    items: section.items.filter((item) => (
      hasPermission(auth.permissions, auth.roles, item.permission)
      && hasPermissions(auth.permissions, auth.roles, item.permissions)
    )),
  })).filter((section) => section.items.length), [auth.permissions, auth.roles]);

  const displayName = auth.session?.user?.display_name || auth.session?.user?.username || auth.session?.user?.email || t('common.operator');
  const activeSectionID = useMemo(() => filteredSections.find((section) => (
    section.items.some((item) => item.path === '/'
      ? location.pathname === '/'
      : location.pathname === item.path || location.pathname.startsWith(`${item.path}/`))
  ))?.id, [filteredSections, location.pathname]);
  const [expandedSections, setExpandedSections] = useState<Set<string>>(() => new Set(activeSectionID ? [activeSectionID] : []));
  const rawHealthStatus = ready.isError ? 'failed' : String(ready.data?.status || 'unknown');
  const healthLabel = rawHealthStatus === 'ready'
    ? t('common.controlPlaneOnline')
    : rawHealthStatus === 'degraded'
      ? t('common.controlPlaneDegraded')
      : rawHealthStatus === 'failed' || rawHealthStatus === 'blocked'
        ? t('common.controlPlaneUnavailable')
        : t('common.controlPlaneChecking');

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined;
    const media = window.matchMedia('(max-width: 1023px)');
    const handleChange = (event: MediaQueryListEvent) => {
      setCompactViewport(event.matches);
      if (!event.matches) setMobileNavigationOpen(false);
    };
    media.addEventListener('change', handleChange);
    return () => media.removeEventListener('change', handleChange);
  }, []);

  const toggleNavigation = () => {
    if (compactViewport) {
      setMobileNavigationOpen((current) => !current);
      return;
    }
    setSidebarCollapsed((current) => !current);
  };

  const toggleSection = (sectionID: string) => {
    setExpandedSections((current) => {
      const next = new Set(current);
      if (next.has(sectionID)) next.delete(sectionID);
      else next.add(sectionID);
      return next;
    });
  };

  return (
    <div className={`app-shell ${sidebarCollapsed ? 'sidebar-collapsed' : ''} ${mobileNavigationOpen ? 'mobile-navigation-open' : ''}`.trim()}>
      <aside className="app-sidebar" id="primary-navigation" aria-label={t('common.navigation')}>
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
            const isRootLink = section.items.length === 1 && section.items[0]?.id === section.id;
            if (isRootLink) {
              const item = section.items[0];
              const ItemIcon = item.icon;
              return (
                <NavLink className="app-nav-link app-nav-root-link" to={item.path} end key={item.id} onClick={() => setMobileNavigationOpen(false)}>
                  <ItemIcon size={18} strokeWidth={2.2} />
                  <span>{t(item.labelKey)}</span>
                </NavLink>
              );
            }
            const expanded = expandedSections.has(section.id);
            return (
              <section className="app-nav-section" key={section.id}>
                <button
                  className="app-nav-section-toggle"
                  type="button"
                  aria-expanded={expanded}
                  onClick={() => toggleSection(section.id)}
                >
                  <SectionIcon size={14} />
                  <span>{t(section.labelKey)}</span>
                  <ChevronDown className="app-nav-chevron" size={14} aria-hidden="true" />
                </button>
                <div className="app-nav-items" hidden={!expanded && !sidebarCollapsed}>
                  {section.items.map((item) => {
                    const ItemIcon = item.icon;
                    const content = (
                      <>
                        <ItemIcon size={17} strokeWidth={2.2} />
                        <span>{t(item.labelKey)}</span>
                      </>
                    );
                    return <NavLink className="app-nav-link" to={item.path} end={item.path === '/'} key={item.id} onClick={() => setMobileNavigationOpen(false)}>{content}</NavLink>;
                  })}
                </div>
              </section>
            );
          })}
        </nav>

        <div className="app-sidebar-footer">
          <small>{t('common.currentRelease', { version: version.data?.version || 'unknown' })}</small>
        </div>
      </aside>
      {mobileNavigationOpen ? (
        <button
          className="app-sidebar-backdrop"
          type="button"
          aria-label={t('common.closeNavigation')}
          onClick={() => setMobileNavigationOpen(false)}
        />
      ) : null}

      <main className="app-main">
        <header className="topbar">
          <div className="toolbar">
            <IconButton
              title={(compactViewport ? mobileNavigationOpen : !sidebarCollapsed) ? t('common.closeNavigation') : t('common.openNavigation')}
              aria-controls="primary-navigation"
              aria-expanded={compactViewport ? mobileNavigationOpen : !sidebarCollapsed}
              onClick={toggleNavigation}
            >
              {mobileNavigationOpen ? <X size={18} /> : <Menu size={18} />}
            </IconButton>
            <StatusBadge status={rawHealthStatus} label={healthLabel} title={t('common.controlPlaneStatus')} />
          </div>
          <div className="topbar-actions">
            <select
              className="select topbar-language"
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
