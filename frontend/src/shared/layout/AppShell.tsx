import { ChevronDown, KeyRound, LogOut, Menu, RefreshCw, UserRound, X } from 'lucide-react';
import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { navSections } from '../config/navigation';
import { useAuth } from '../auth/AuthProvider';
import { hasPermission, hasPermissions } from '../permissions/permissions';
import { setLocale, supportedLocales, type SupportedLocale } from '../i18n';
import { useReady, useVersion } from '../query/hooks';
import { Button, FormField, IconButton, Modal, StatusBadge, TextField } from '../ui';

export function AppShell() {
  const { t, i18n } = useTranslation();
  const auth = useAuth();
  const location = useLocation();
  const ready = useReady();
  const version = useVersion();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const accountMenuRef = useRef<HTMLDivElement | null>(null);
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
    ? t(compactViewport ? 'common.controlPlaneOnlineShort' : 'common.controlPlaneOnline')
    : rawHealthStatus === 'degraded'
      ? t(compactViewport ? 'common.controlPlaneDegradedShort' : 'common.controlPlaneDegraded')
      : rawHealthStatus === 'failed' || rawHealthStatus === 'blocked'
        ? t(compactViewport ? 'common.controlPlaneUnavailableShort' : 'common.controlPlaneUnavailable')
        : t(compactViewport ? 'common.controlPlaneCheckingShort' : 'common.controlPlaneChecking');
  const accountInitial = displayName.trim().charAt(0).toUpperCase() || '?';

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

  useEffect(() => {
    if (!accountMenuOpen) return undefined;
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (!accountMenuRef.current?.contains(event.target as Node)) setAccountMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setAccountMenuOpen(false);
    };
    document.addEventListener('mousedown', closeOnOutsideClick);
    window.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('mousedown', closeOnOutsideClick);
      window.removeEventListener('keydown', closeOnEscape);
    };
  }, [accountMenuOpen]);

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

  const selectLocale = (locale: SupportedLocale) => {
    void setLocale(locale);
  };

  const submitPasswordChange = async (event: FormEvent) => {
    event.preventDefault();
    setPasswordError('');
    if (newPassword !== confirmPassword) {
      setPasswordError(t('auth.passwordMismatch'));
      return;
    }
    try {
      await auth.changePassword(currentPassword, newPassword);
      setProfileOpen(false);
    } catch (error) {
      setPasswordError(error instanceof Error ? error.message : t('errors.network'));
    }
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
                <NavLink className="app-nav-link app-nav-root-link" to={item.path} end key={item.id} onClick={() => { setMobileNavigationOpen(false); setAccountMenuOpen(false); }}>
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
                    return <NavLink className="app-nav-link" to={item.path} end={item.path === '/' || item.path === '/clients'} key={item.id} onClick={() => { setMobileNavigationOpen(false); setAccountMenuOpen(false); }}>{content}</NavLink>;
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
            <Button icon={<RefreshCw size={16} />} onClick={() => { void ready.refetch(); }}>
              {t('common.refresh')}
            </Button>
            <div className="account-menu" ref={accountMenuRef}>
              <Button
                className="account-trigger"
                aria-haspopup="menu"
                aria-expanded={accountMenuOpen}
                onClick={() => setAccountMenuOpen((current) => !current)}
              >
                <span className="account-avatar" aria-hidden="true">{accountInitial}</span>
                <span className="account-trigger-copy">
                  <strong>{displayName}</strong>
                  <small>{auth.session?.user?.email || auth.session?.user?.username || t('common.operator')}</small>
                </span>
                <ChevronDown className="account-trigger-chevron" size={16} aria-hidden="true" />
              </Button>
              {accountMenuOpen ? (
                <div className="account-popover" role="menu">
                  <div className="account-popover-head">
                    <strong>{displayName}</strong>
                    <span>{auth.session?.user?.email || auth.session?.user?.username || t('common.operator')}</span>
                  </div>
                  <div className="account-language" aria-label={t('common.language')}>
                    {supportedLocales.map((locale) => (
                      <button
                        className="language-option"
                        type="button"
                        aria-pressed={i18n.language.slice(0, 2) === locale}
                        onClick={() => selectLocale(locale)}
                        key={locale}
                      >
                        <span aria-hidden="true">{locale === 'ru' ? '🇷🇺' : '🇬🇧'}</span>
                        <span>{locale.toUpperCase()}</span>
                      </button>
                    ))}
                  </div>
                  <Button
                    className="account-menu-action"
                    icon={<UserRound size={16} />}
                    onClick={() => { setAccountMenuOpen(false); setProfileOpen(true); }}
                  >
                    {t('auth.accountSettings')}
                  </Button>
                  <Button
                    className="account-menu-action account-logout"
                    icon={<LogOut size={16} />}
                    onClick={() => { void auth.logout(); }}
                  >
                    {t('auth.logout')}
                  </Button>
                </div>
              ) : null}
            </div>
          </div>
        </header>
        <div className="content-shell">
          <Outlet />
        </div>
      </main>
      <Modal title={t('auth.accountSettings')} open={profileOpen} onClose={() => setProfileOpen(false)}>
        <form className="page-stack" onSubmit={submitPasswordChange}>
          <section className="account-profile-summary">
            <div><span>{t('auth.userName')}</span><strong>{auth.session?.user?.username || t('common.notAvailable')}</strong></div>
            <div><span>{t('common.email')}</span><strong>{auth.session?.user?.email || t('common.notAvailable')}</strong></div>
            <div><span>{t('common.roles')}</span><strong>{auth.roles.join(', ') || t('common.none')}</strong></div>
          </section>
          <section className="form-section">
            <h3 className="form-section-title">{t('common.language')}</h3>
            <div className="account-language account-language-settings">
              {supportedLocales.map((locale) => (
                <button
                  className="language-option"
                  type="button"
                  aria-pressed={i18n.language.slice(0, 2) === locale}
                  onClick={() => selectLocale(locale)}
                  key={locale}
                >
                  <span aria-hidden="true">{locale === 'ru' ? '🇷🇺' : '🇬🇧'}</span>
                  <span>{locale.toUpperCase()}</span>
                </button>
              ))}
            </div>
          </section>
          <section className="form-section">
            <h3 className="form-section-title">{t('auth.changePassword')}</h3>
            <FormField label={t('auth.currentPassword')}>
              <TextField type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.currentTarget.value)} required />
            </FormField>
            <FormField label={t('auth.newPassword')}>
              <TextField type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.currentTarget.value)} required minLength={12} />
            </FormField>
            <FormField label={t('auth.confirmPassword')}>
              <TextField type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.currentTarget.value)} required minLength={12} />
            </FormField>
            {passwordError ? <div className="form-error" role="alert">{passwordError}</div> : null}
          </section>
          <div className="modal-actions">
            <Button type="button" onClick={() => setProfileOpen(false)}>{t('common.cancel')}</Button>
            <Button variant="primary" icon={<KeyRound size={16} />} type="submit">{t('auth.saveNewPassword')}</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
