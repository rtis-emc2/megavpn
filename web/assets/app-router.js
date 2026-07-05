(function (window) {
  'use strict';

  function createAppRouter(ctx = {}) {
    const {
      state,
      el,
      setShellMode,
      renderNav,
      renderAuthSlot,
      renderNotice,
      setTitle,
      escapeHTML,
      authWorkflows,
      nodeWorkflows,
      nodeMapPage,
      instanceWorkflows,
      dashboardPage,
      nodesPage,
      instancesPage,
      addressPoolsPage,
      firewallPage,
      servicesPage,
      clientsPage,
      jobWorkflows,
      artifactsPage,
      backhaulPage,
      certificatesPage,
      revisionsPage,
      opsPages,
      settingsPage,
    } = ctx;
    if (
      !state ||
      typeof el !== 'function' ||
      typeof setShellMode !== 'function' ||
      typeof renderNav !== 'function' ||
      typeof renderAuthSlot !== 'function' ||
      typeof renderNotice !== 'function' ||
      typeof setTitle !== 'function' ||
      typeof escapeHTML !== 'function' ||
      !nodeMapPage ||
      typeof nodeMapPage.render !== 'function' ||
      !instanceWorkflows ||
      typeof instanceWorkflows.renderInstanceManagePage !== 'function' ||
      !firewallPage ||
      typeof firewallPage.render !== 'function'
    ) {
      throw new Error('MegaVPNAppRouter requires shell dependencies');
    }

    const autoRefreshPages = new Set([
      'dashboard',
      'nodes',
      'nodeManage',
      'instanceManage',
      'instances',
      'addressPools',
      'clients',
      'backhaul',
    ]);

    const menuPageKeys = new Set(
      (window.MegaVPNAppConfig?.navGroups || [])
        .flatMap(([, items]) => Array.isArray(items) ? items : [])
        .map(([key]) => key)
        .filter(Boolean),
    );
    const registeredPageKeys = new Set([
      'dashboard',
      'nodes',
      'nodeMap',
      'nodeManage',
      'services',
      'instanceManage',
      'instances',
      'addressPools',
      'firewall',
      'clients',
      'jobs',
      'artifacts',
      'shareLinks',
      'backhaul',
      'certificates',
      'revisions',
      'telemetry',
      'audit',
      'settings',
    ]);
    const missingMenuRoutes = [...menuPageKeys].filter((key) => !registeredPageKeys.has(key));
    if (missingMenuRoutes.length) {
      throw new Error(`Navigation routes are not registered: ${missingMenuRoutes.join(', ')}`);
    }

    function renderUnknownPage(key) {
      setTitle('Unknown Page');
      el('content').innerHTML = `
        <section class="card">
          <h2>Unknown route</h2>
          <p>Page "${escapeHTML(key)}" is not registered in the current Control Plane UI.</p>
        </section>`;
    }

    function renderRoute() {
      if (state.page === 'dashboard') return dashboardPage.render();
      if (state.page === 'nodes') return nodesPage.render();
      if (state.page === 'nodeMap') return nodeMapPage.render();
      if (state.page === 'nodeManage') return nodeWorkflows.renderNodeManagePage();
      if (state.page === 'services') return servicesPage.render();
      if (state.page === 'instanceManage') return instanceWorkflows.renderInstanceManagePage();
      if (state.page === 'instances') return instancesPage.render();
      if (state.page === 'addressPools') return addressPoolsPage.render();
      if (state.page === 'firewall') return firewallPage.render();
      if (state.page === 'clients') return clientsPage.render();
      if (state.page === 'jobs') return jobWorkflows.renderJobs();
      if (state.page === 'artifacts') return artifactsPage.renderArtifacts();
      if (state.page === 'shareLinks') return artifactsPage.renderShareLinks();
      if (state.page === 'backhaul') return backhaulPage.render();
      if (state.page === 'certificates') return certificatesPage.render();
      if (state.page === 'revisions') return revisionsPage.render();
      if (state.page === 'telemetry') return opsPages.renderTelemetry();
      if (state.page === 'audit') return opsPages.renderAudit();
      if (state.page === 'settings') return settingsPage.render();
      return renderUnknownPage(state.page);
    }

    function render() {
      const isAuthenticated = Boolean(state.authUser);
      setShellMode(isAuthenticated);
      if (!isAuthenticated) {
        nodeWorkflows.disconnectNodeTerminal?.();
        if (state.inviteToken) {
          authWorkflows.renderInviteAcceptScreen();
          return;
        }
        authWorkflows.renderLoginScreen();
        return;
      }
      renderNav();
      renderAuthSlot();
      renderNotice();
      renderRoute();
    }

    function setPage(page) {
      if (page !== 'nodeManage') {
        nodeWorkflows.disconnectNodeTerminal?.();
      }
      if (page !== 'nodeManage') {
        state.nodeManageID = '';
        state.nodeManageData = null;
      }
      if (page !== 'instanceManage') {
        state.instanceManageID = '';
        state.instanceManageData = null;
        state.instanceManageDirty = false;
      }
      state.page = page;
      render();
      if (page === 'nodeMap' && state.authUser) {
        void nodeMapPage.activate?.();
      }
      if (page === 'services' && state.authUser) {
        void servicesPage.loadData();
      }
    }

    function autoRefreshEnabledForCurrentPage() {
      if (state.page === 'nodeManage' && state.nodeTerminalActive) return false;
      if (state.page === 'instanceManage' && state.instanceManageDirty) return false;
      if (state.page === 'instances' && state.instancesView === 'create-pack') return false;
      return autoRefreshPages.has(state.page);
    }

    return {
      render,
      setPage,
      autoRefreshEnabledForCurrentPage,
    };
  }

  window.MegaVPNAppRouter = { create: createAppRouter };
})(window);
