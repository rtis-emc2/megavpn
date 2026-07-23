(() => {
  // Composition root only: bootstrap state, dependency wiring, lifecycle binding and refresh loop stay here.
  // Page workflows, shell rendering, routing and reusable UI primitives belong in dedicated web/assets modules.
  function escapeBootstrapHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
  }

  function renderBootstrapError(error) {
    if (window.__MegaVPNBootReady) return;
    const message = typeof error === 'string' ? error : (error?.message || 'Frontend bootstrap failed');
    const authGate = document.getElementById('authGate');
    const appShell = document.getElementById('appShell');
    if (appShell) appShell.hidden = true;
    if (!authGate) return;
    authGate.hidden = false;
    authGate.innerHTML = `
      <div class="auth-card">
        <div class="eyebrow">Frontend error</div>
        <h1>Unable to load control plane UI</h1>
        <p class="muted">The browser loaded incompatible or incomplete frontend assets. Refresh the page after deployment finishes.</p>
        <div class="form-result"><span class="tag danger">${escapeBootstrapHTML(message)}</span></div>
        <button class="primary-btn" type="button" data-bootstrap-refresh>Refresh</button>
      </div>`;
    authGate.querySelector('[data-bootstrap-refresh]')?.addEventListener('click', () => window.location.reload());
  }

  window.addEventListener('error', (event) => renderBootstrapError(event.error || event.message));
  window.addEventListener('unhandledrejection', (event) => renderBootstrapError(event.reason || 'Unhandled frontend promise rejection'));

  const state = window.MegaVPNAppState?.createInitialState?.();
  if (!state) throw new Error('MegaVPNAppState is not loaded');

  localStorage.removeItem('megavpn.authToken');

  const appConfig = window.MegaVPNAppConfig || {};
  const AUTO_REFRESH_INTERVAL_MS = Number(appConfig.autoRefreshIntervalMs || 5000);
  const navGroups = Array.isArray(appConfig.navGroups) ? appConfig.navGroups : [];
  const authView = window.MegaVPNAuthView || null;
  const responseView = window.MegaVPNResponseView || null;

  const el = (id) => document.getElementById(id);
  const apiClient = window.MegaVPNAPIClient?.create?.({
    getApiBase: () => state.apiBase,
    onError: (err) => {
      state.lastError = err;
    },
  });
  if (!apiClient) throw new Error('MegaVPNAPIClient is not loaded');
  const { apiURL, requestJSON, sendJSON, fetchJSON } = apiClient;

  const appRuntime = window.MegaVPNAppRuntime?.create?.({ state, responseView });
  if (!appRuntime) throw new Error('MegaVPNAppRuntime is not loaded');
  const {
    escapeHTML,
    arrayOrEmpty,
    parseCSVList,
    statusTag,
    renderActionResponse,
    formatDate,
    formatRelativeDate,
    formatDurationSeconds,
    toMillis,
    hasPermission,
    hasRole,
    platformPublicBaseURL,
    publicURLHostname,
    publicURLPort,
    agentEndpointURL,
    isLoopbackURL,
    publicBaseURLStatusTag,
  } = appRuntime;

  function createUnavailablePage(title, moduleName) {
    return {
      render: () => {
        setTitle(title);
        el('content').innerHTML = `
          <section class="section-card">
            <div class="section-head">
              <div>
                <h2>${escapeHTML(title)}</h2>
                <p>This page module is not available in the currently loaded frontend assets.</p>
              </div>
              ${statusTag('unavailable')}
            </div>
            <div class="section-body">
              <div class="notice compact-notice">
                Refresh the page after deployment finishes. If the page stays unavailable, redeploy web assets and verify <code>${escapeHTML(moduleName)}</code> exists under the configured web root and exports a compatible page module.
              </div>
            </div>
          </section>`;
      },
    };
  }

  const nodeExecutionModes = appConfig.nodeExecutionModes || {};

  const domainUI = window.MegaVPNDomainUI?.create?.({ state, escapeHTML, formatDate });
  if (!domainUI) throw new Error('MegaVPNDomainUI is not loaded');
  const {
    certificateDisplayStatus,
    certificateExpiryCaption,
    certificatePrimaryLabel,
    certificateUsageCaption,
    certificateOptions,
    normalizeInstanceServiceCode,
    stringValue,
  } = domainUI;

  const nodeUI = window.MegaVPNNodeUI?.create?.({
    nodeExecutionModes,
    escapeHTML,
    formatDate,
    formatRelativeDate,
  });
  if (!nodeUI) throw new Error('MegaVPNNodeUI is not loaded');
  const {
    nodeExecutionLabel,
    nodeAgentChannelStatus,
    nodeLifecycleStatus,
    nodeHeartbeatStatus,
    renderInventoryFact,
  } = nodeUI;

  const uiPrimitives = window.MegaVPNUIPrimitives?.create?.({ escapeHTML });
  if (!uiPrimitives) throw new Error('MegaVPNUIPrimitives is not loaded');
  const { metric, tableCard } = uiPrimitives;

  let authWorkflows = null;
  let appRouter = null;
  function setPage(page) {
    if (!appRouter) {
      state.page = page;
      return;
    }
    appRouter.setPage(page);
  }

  function render() {
    if (!appRouter) return;
    appRouter.render();
  }

  function autoRefreshEnabledForCurrentPage() {
    return Boolean(appRouter?.autoRefreshEnabledForCurrentPage?.());
  }

  const shellUI = window.MegaVPNShellUI?.create?.({
    state,
    navGroups,
    el,
    setPage,
    getLogoutHandler: () => authWorkflows?.logout,
    statusTag,
    escapeHTML,
    renderActionResponse,
  });
  if (!shellUI) throw new Error('MegaVPNShellUI is not loaded');
  const {
    updateReadyPill,
    renderNotice,
    renderNav,
    renderAuthSlot,
    setTitle,
    setShellMode,
    openModal,
    closeModal,
    setSubmitBusy,
    openActionOutcomeModal,
    openUnavailableAction,
  } = shellUI;

  const coreLoader = window.MegaVPNCoreLoader?.create?.({
    state,
    fetchJSON,
    hasPermission,
    updateReadyPill,
    renderNotice,
    renderProgress: () => render(),
  });
  if (!coreLoader) throw new Error('MegaVPNCoreLoader is not loaded');
  const { loadCore } = coreLoader;

  const jobWorkflows = window.MegaVPNJobWorkflows?.create?.({
    state,
    setTitle,
    el,
    requestJSON,
    fetchJSON,
    tableCard,
    statusTag,
    escapeHTML,
    formatDate,
    renderActionResponse,
    stringValue,
    hasPermission,
    nodeAgentChannelStatus,
  });
  if (!jobWorkflows) throw new Error('MegaVPNJobWorkflows is not loaded');

  const instanceWorkflows = window.MegaVPNInstanceWorkflows?.create?.({
    state,
    setTitle,
    el,
    setPage,
    domainUI,
    requestJSON,
    fetchJSON,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    openActionOutcomeModal,
    renderNotice,
    setSubmitBusy,
    watchJob: jobWorkflows.watchJob,
    statusTag,
    escapeHTML,
    formatDate,
    renderActionResponse,
  });
  if (!instanceWorkflows) throw new Error('MegaVPNInstanceWorkflows is not loaded');

  const certificateWorkflows = window.MegaVPNCertificateWorkflows?.create?.({
    state,
    domainUI,
    requestJSON,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    openActionOutcomeModal,
    setSubmitBusy,
    statusTag,
    escapeHTML,
  });
  if (!certificateWorkflows) throw new Error('MegaVPNCertificateWorkflows is not loaded');

  const nodeWorkflows = window.MegaVPNNodeWorkflows?.create?.({
    state,
    nodeUI,
    setTitle,
    el,
    requestJSON,
    sendJSON,
    fetchJSON,
    refresh,
    loadCore,
    setPage,
    openModal,
    closeModal,
    openActionOutcomeModal,
    renderActionResponse,
    watchJob: jobWorkflows.watchJob,
    waitForNodeDiagnostics: jobWorkflows.waitForNodeDiagnostics,
    statusTag,
    escapeHTML,
    toMillis,
    formatDate,
    formatRelativeDate,
    formatDurationSeconds,
    platformPublicBaseURL,
  });
  if (!nodeWorkflows) throw new Error('MegaVPNNodeWorkflows is not loaded');

  const settingsWorkflows = window.MegaVPNSettingsWorkflows?.create?.({
    state,
    domainUI,
    requestJSON,
    sendJSON,
    refresh,
    render,
    setPage,
    openModal,
    closeModal,
    renderActionResponse,
    watchJob: jobWorkflows.watchJob,
    updateReadyPill,
    statusTag,
    escapeHTML,
    arrayOrEmpty,
    parseCSVList,
    formatDate,
    renderInventoryFact,
    hasPermission,
    hasRole,
    platformPublicBaseURL,
    publicURLHostname,
    publicURLPort,
  });
  if (!settingsWorkflows) throw new Error('MegaVPNSettingsWorkflows is not loaded');

  authWorkflows = window.MegaVPNAuthWorkflows?.create?.({
    state,
    authView,
    setTitle,
    el,
    requestJSON,
    sendJSON,
    refresh,
    render,
    setSubmitBusy,
    openSettings: settingsWorkflows.openSettings,
    escapeHTML,
  });
  if (!authWorkflows) throw new Error('MegaVPNAuthWorkflows is not loaded');

  const instancesPage = window.MegaVPNInstancesPage?.create?.({
    state,
    setTitle,
    el,
    tableCard,
    statusTag,
    escapeHTML,
    domainUI,
    renderActionResponse,
    requestJSON,
    sendJSON,
    refresh,
    setPage,
    hasPermission,
    openModal,
    closeModal,
    openActionOutcomeModal,
    renderCreateInstanceForm: instanceWorkflows.renderCreateInstanceForm,
    openCreateInstanceChoiceModal: instanceWorkflows.openCreateInstanceChoiceModal,
    openInstanceManageModal: instanceWorkflows.openInstanceManageModal,
    openInstanceManagePage: instanceWorkflows.openInstanceManagePage,
    openInstanceRuntimeInstallModal: instanceWorkflows.openInstanceRuntimeInstallModal,
    queueInstanceAction: instanceWorkflows.queueInstanceAction,
  });
  if (!instancesPage) throw new Error('MegaVPNInstancesPage is not loaded');

  const dashboardPage = window.MegaVPNDashboardPage?.create?.({
    state,
    setTitle,
    el,
    metric,
    tableCard,
    statusTag,
    escapeHTML,
    instanceToRow: instancesPage.toRow,
    setPage,
  });
  if (!dashboardPage) throw new Error('MegaVPNDashboardPage is not loaded');

  const nodeMapPage = window.MegaVPNNodeMapPage?.create?.({
    state,
    setTitle,
    el,
    setPage,
    sendJSON,
    loadCore,
    statusTag,
    escapeHTML,
  }) || createUnavailablePage('Node Map', 'assets/node-map-page.js');

  const servicesPage = window.MegaVPNServicesPage?.create?.({
    state,
    setTitle,
    el,
    fetchJSON,
    requestJSON,
    sendJSON,
    watchJob: jobWorkflows.watchJob,
    openModal,
    closeModal,
    openUnavailableAction,
    statusTag,
    escapeHTML,
    formatDate,
    hasPermission,
  });
  if (!servicesPage) throw new Error('MegaVPNServicesPage is not loaded');

  const opsPages = window.MegaVPNOpsPages?.create?.({
    state,
    setTitle,
    el,
    requestJSON,
    metric,
    tableCard,
    statusTag,
    escapeHTML,
    formatDate,
    formatRelativeDate,
    nodeHeartbeatStatus,
  });
  if (!opsPages) throw new Error('MegaVPNOpsPages is not loaded');

  const addressPoolsPage = window.MegaVPNAddressPoolsPage?.create?.({
    state,
    setTitle,
    el,
    statusTag,
    escapeHTML,
    formatDate,
    hasPermission,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    openActionOutcomeModal,
  });
  if (!addressPoolsPage) throw new Error('MegaVPNAddressPoolsPage is not loaded');

  const firewallPage = window.MegaVPNFirewallPage?.create?.({
    state,
    setTitle,
    el,
    statusTag,
    escapeHTML,
    formatDate,
    hasPermission,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    openActionOutcomeModal,
    watchJob: jobWorkflows.watchJob,
  });
  if (!firewallPage) throw new Error('MegaVPNFirewallPage is not loaded');

  const trafficPage = window.MegaVPNTrafficPage?.create?.({
    state,
    setTitle,
    el,
    requestJSON,
    statusTag,
    escapeHTML,
    formatDate,
    formatDurationSeconds,
    apiURL,
  });
  if (!trafficPage) throw new Error('MegaVPNTrafficPage is not loaded');

  const revisionsPage = window.MegaVPNRevisionsPage?.create?.({
    state,
    setTitle,
    el,
    requestJSON,
    sendJSON,
    refresh,
    runInstanceAction: instanceWorkflows.runInstanceAction,
    openModal,
    closeModal,
    openActionOutcomeModal,
    tableCard,
    statusTag,
    escapeHTML,
    formatDate,
  });
  if (!revisionsPage) throw new Error('MegaVPNRevisionsPage is not loaded');

  const clientsPage = window.MegaVPNClientsPage?.create?.({
    state,
    setTitle,
    el,
    tableCard,
    statusTag,
    escapeHTML,
    formatDate,
    requestJSON,
    sendJSON,
    refresh,
    setPage,
    openModal,
    closeModal,
    openActionOutcomeModal,
    renderActionResponse,
  });
  if (!clientsPage) throw new Error('MegaVPNClientsPage is not loaded');

  const artifactsPage = window.MegaVPNArtifactsPage?.create?.({
    state,
    setTitle,
    el,
    tableCard,
    statusTag,
    escapeHTML,
    formatDate,
    requestJSON,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    renderActionResponse,
    normalizeInstanceServiceCode,
    toMillis,
  });
  if (!artifactsPage) throw new Error('MegaVPNArtifactsPage is not loaded');

  const backhaulPage = window.MegaVPNBackhaulPage?.create?.({
    state,
    setTitle,
    el,
    tableCard,
    statusTag,
    escapeHTML,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    renderActionResponse,
    formatDate,
  });
  if (!backhaulPage) throw new Error('MegaVPNBackhaulPage is not loaded');

  const externalEgressPage = window.MegaVPNExternalEgressPage?.create?.({
    state,
    setTitle,
    el,
    statusTag,
    escapeHTML,
    formatDate,
    hasPermission,
    requestJSON,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    renderActionResponse,
    openActionOutcomeModal,
  });
  if (!externalEgressPage) throw new Error('MegaVPNExternalEgressPage is not loaded');

  const nodesPage = window.MegaVPNNodesPage?.create?.({
    state,
    setTitle,
    el,
    tableCard,
    statusTag,
    escapeHTML,
    nodeExecutionLabel,
    nodeAgentChannelStatus,
    nodeLifecycleStatus,
    requestJSON,
    sendJSON,
    refresh,
    openModal,
    closeModal,
    renderActionResponse,
    hasPermission,
    watchJob: jobWorkflows.watchJob,
    openCreateNodeModal: nodeWorkflows.openCreateNodeModal,
    openNodeControlModal: nodeWorkflows.openNodeControlModal,
    openEditNodeModal: nodeWorkflows.openEditNodeModal,
    openDeleteNodeModal: nodeWorkflows.openDeleteNodeModal,
  });
  if (!nodesPage) throw new Error('MegaVPNNodesPage is not loaded');

  const settingsPage = window.MegaVPNSettingsPage?.create?.({
    state,
    setTitle,
    el,
    statusTag,
    escapeHTML,
    renderInventoryFact,
    hasPermission,
    hasRole,
    platformPublicBaseURL,
    agentEndpointURL,
    publicBaseURLStatusTag,
    isLoopbackURL,
    openSettings: settingsWorkflows.openSettings,
    changeOwnPassword: settingsWorkflows.changeOwnPassword,
    loadAdminSettings: settingsWorkflows.loadAdminSettings,
  });
  if (!settingsPage) throw new Error('MegaVPNSettingsPage is not loaded');

  const certificatesPage = window.MegaVPNCertificatesPage?.create?.({
    state,
    setTitle,
    el,
    statusTag,
    escapeHTML,
    formatDate,
    certificateDisplayStatus,
    certificateExpiryCaption,
    certificatePrimaryLabel,
    certificateUsageCaption,
    openCreateCertificateWizard: certificateWorkflows.openCreateCertificateWizard,
    openCertificateActionForm: certificateWorkflows.openCertificateActionForm,
    openManageCertificateModal: certificateWorkflows.openManageCertificateModal,
    submitSetDefaultPlatformCertificate: certificateWorkflows.submitSetDefaultPlatformCertificate,
  });
  if (!certificatesPage) throw new Error('MegaVPNCertificatesPage is not loaded');

  appRouter = window.MegaVPNAppRouter?.create?.({
    state,
    el,
    setShellMode,
    renderNav,
    renderAuthSlot,
    renderNotice,
    setTitle,
    escapeHTML,
    authWorkflows,
    settingsWorkflows,
    nodeWorkflows,
    nodeMapPage,
    instanceWorkflows,
    certificateWorkflows,
    dashboardPage,
    nodesPage,
    instancesPage,
    addressPoolsPage,
    firewallPage,
    trafficPage,
    servicesPage,
    clientsPage,
    jobWorkflows,
    artifactsPage,
    backhaulPage,
    externalEgressPage,
    certificatesPage,
    revisionsPage,
    opsPages,
    settingsPage,
  });
  if (!appRouter) throw new Error('MegaVPNAppRouter is not loaded');

  async function refresh(options = {}) {
    if (options.auto && state.refreshInFlight) return;
    const seq = ++state.refreshSeq;
    state.refreshInFlight = true;
    state.refreshInFlightSeq = seq;
    try {
      if (!state.authUser && state.inviteToken) {
        await authWorkflows.loadInvitePreview();
        if (seq !== state.refreshSeq) return;
      }
      await authWorkflows.loadSession();
      if (seq !== state.refreshSeq) return;
      await loadCore();
      if (seq !== state.refreshSeq) return;
      state.lastError = null;
    } catch (err) {
      if (seq !== state.refreshSeq) return;
      state.lastError = err;
    } finally {
      if (state.refreshInFlightSeq === seq) {
        state.refreshInFlight = false;
        state.refreshInFlightSeq = 0;
      }
    }
    if (seq !== state.refreshSeq) return;
    if (options.auto && !autoRefreshEnabledForCurrentPage()) return;
    render();
  }

  function startAutoRefresh() {
    if (!AUTO_REFRESH_INTERVAL_MS || AUTO_REFRESH_INTERVAL_MS < 1000) return;
    window.setInterval(() => {
      if (document.hidden || !state.authUser || !autoRefreshEnabledForCurrentPage()) return;
      void refresh({ auto: true });
    }, AUTO_REFRESH_INTERVAL_MS);
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden && state.authUser && autoRefreshEnabledForCurrentPage()) void refresh({ auto: true });
    });
  }

  function bind() {
    el('refreshBtn').addEventListener('click', refresh);
    el('openSettingsBtn')?.addEventListener('click', settingsWorkflows.openSettings);
    el('closeModalBtn').addEventListener('click', closeModal);
    el('modalBackdrop').addEventListener('click', (event) => {
      if (event.target === el('modalBackdrop')) closeModal();
    });
    window.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') closeModal();
    });
  }

  try {
    bind();
    render();
    refresh();
    startAutoRefresh();
    window.__MegaVPNBootReady = true;
  } catch (err) {
    renderBootstrapError(err);
    throw err;
  }
})();
