(() => {
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

  const state = {
    page: 'dashboard',
    apiBase: localStorage.getItem('megavpn.apiBase') || '',
    inviteToken: new URLSearchParams(window.location.search).get('invite_token') || '',
    invitePreview: null,
    authUser: null,
    authSession: null,
    authRoles: [],
    authPermissions: [],
    dashboard: null,
    ready: null,
    versionInfo: null,
    nodes: [],
    instances: [],
    instanceRuntimeStates: [],
    clients: [],
    jobs: [],
    artifacts: [],
    shareLinks: [],
    backhaulLinks: [],
    backhaulDrivers: [],
    servicesCatalog: [],
    servicePacks: [],
    servicePackCatalog: [],
    serviceInstallers: [],
    serviceCapabilitiesByNode: {},
    serviceInstallEventsByNode: {},
    runtimePreflight: null,
    mailSettings: null,
    controlPlaneTLSSettings: null,
    platformCertificates: [],
    platformInvites: [],
    platformPKIRoots: [],
    servicesNodeID: localStorage.getItem('megavpn.servicesNodeID') || '',
    revisionsInstanceID: localStorage.getItem('megavpn.revisionsInstanceID') || '',
    nodeManageID: '',
    nodeManageData: null,
    refreshSeq: 0,
    refreshInFlight: false,
    refreshInFlightSeq: 0,
    lastError: null,
  };

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
  const { requestJSON, sendJSON, fetchJSON } = apiClient;

  function platformPublicBaseURL() {
    return String(state.versionInfo?.public_base_url || '').trim().replace(/\/$/, '');
  }

  function publicURLHostname(value) {
    try {
      return new URL(value || '').hostname;
    } catch (_) {
      return '';
    }
  }

  function publicURLPort(value) {
    try {
      const url = new URL(value || '');
      if (url.port) return Number(url.port);
      return url.protocol === 'https:' ? 443 : 80;
    } catch (_) {
      return 0;
    }
  }

  function agentEndpointURL(path) {
    const base = platformPublicBaseURL();
    if (!base) return 'n/a';
    return `${base}${path}`;
  }

  function isLoopbackURL(value) {
    try {
      const hostname = new URL(value).hostname.toLowerCase();
      return hostname === 'localhost' || hostname === '::1' || hostname === '[::1]' || hostname.startsWith('127.');
    } catch (_) {
      return false;
    }
  }

  function publicBaseURLStatusTag() {
    const value = platformPublicBaseURL();
    if (!value) return statusTag('missing');
    if (isLoopbackURL(value)) return statusTag('loopback-only');
    return statusTag('configured');
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
  }

  function arrayOrEmpty(value) {
    return Array.isArray(value) ? value : [];
  }

  function parseCSVList(value) {
    return String(value || '')
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
  }

  function clsStatus(status) {
    const normalized = String(status || '').toLowerCase();
    if (['ok', 'ready', 'active', 'healthy', 'succeeded', 'online', 'configured', 'enabled', 'sent', 'delivered', 'in_sync'].includes(normalized)) return 'ok';
    if (['stub', 'planned', 'draft', 'pending', 'unknown', 'maintenance', 'skipped', 'stopped', 'materialized'].includes(normalized)) return 'stub';
    if (['degraded', 'warning', 'retrying', 'queued', 'running', 'starting', 'bootstrapping', 'waiting heartbeat', 'awaiting heartbeat', 'provisioning', 'inactive', 'pending_apply', 'update available'].includes(normalized)) return 'warn';
    if (['failed', 'blocked', 'offline', 'error', 'disabled', 'cancelled', 'revoked', 'missing', 'loopback-only', 'delivery_failed', 'expired', 'invalid', 'deleted', 'unhealthy', 'drifted'].includes(normalized)) return 'danger';
    return 'stub';
  }

  function statusTag(value) {
    return `<span class="tag ${clsStatus(value)}"><span class="dot ${clsStatus(value) === 'stub' ? 'unknown' : clsStatus(value)}"></span>${escapeHTML(value)}</span>`;
  }

  function renderActionResponse(data, title = 'Operation result') {
    if (responseView?.render) return responseView.render(data, { title });
    return `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
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

  function formatDate(value) {
    if (!value) return 'n/a';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return escapeHTML(value);
    return escapeHTML(date.toLocaleString('ru-RU'));
  }

  function formatRelativeDate(value) {
    if (!value) return 'n/a';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return escapeHTML(value);
    const diffMs = Date.now() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return `${diffSec}s ago`;
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHours = Math.floor(diffMin / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}d ago`;
  }

  function formatDurationSeconds(value) {
    const seconds = Number(value);
    if (!Number.isFinite(seconds) || seconds < 0) return 'n/a';
    if (seconds < 60) return `${seconds}s`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ${minutes % 60}m`;
    const days = Math.floor(hours / 24);
    return `${days}d ${hours % 24}h`;
  }

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

  function toMillis(value) {
    if (!value) return 0;
    const ms = Date.parse(value);
    return Number.isFinite(ms) ? ms : 0;
  }

  function hasPermission(code) {
    return Array.isArray(state.authPermissions) && state.authPermissions.includes(code);
  }

  function hasRole(code) {
    return Array.isArray(state.authRoles) && state.authRoles.includes(code);
  }

  function metric(label, value, caption, targetPage = '') {
    const attrs = targetPage ? ` role="button" tabindex="0" data-page-target="${escapeHTML(targetPage)}"` : '';
    return `<div class="card metric-card${targetPage ? ' dashboard-nav-tile' : ''}"${attrs}><div class="mini-label">${escapeHTML(label)}</div><div class="metric-value">${escapeHTML(value)}</div><div class="metric-caption">${escapeHTML(caption)}</div></div>`;
  }

  function tableCard(title, rows, columns, tools = '') {
    const body = rows.length
      ? rows.map((row) => `<tr>${columns.map((c) => `<td>${c.render ? c.render(row) : escapeHTML(row[c.key])}</td>`).join('')}</tr>`).join('')
      : `<tr><td colspan="${columns.length}"><div class="empty">Нет данных для отображения</div></td></tr>`;
    return `
      <section class="table-card">
        <div class="table-head"><h2>${title}</h2><div class="table-tools">${tools}</div></div>
        <div class="table-wrap"><table><thead><tr>${columns.map((c) => `<th>${c.title}</th>`).join('')}</tr></thead><tbody>${body}</tbody></table></div>
      </section>`;
  }

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
  });
  if (!jobWorkflows) throw new Error('MegaVPNJobWorkflows is not loaded');

  const instanceWorkflows = window.MegaVPNInstanceWorkflows?.create?.({
    state,
    domainUI,
    requestJSON,
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

  const authWorkflows = window.MegaVPNAuthWorkflows?.create?.({
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
    requestJSON,
    sendJSON,
    refresh,
    hasPermission,
    openModal,
    closeModal,
    openActionOutcomeModal,
    openCreateInstanceModal: instanceWorkflows.openCreateInstanceModal,
    openCreateInstanceChoiceModal: instanceWorkflows.openCreateInstanceChoiceModal,
    openCreateServicePackModal: instanceWorkflows.openCreateServicePackModal,
    openInstanceManageModal: instanceWorkflows.openInstanceManageModal,
    queueInstanceAction: instanceWorkflows.queueInstanceAction,
  });
  if (!instancesPage) throw new Error('MegaVPNInstancesPage is not loaded');

  const servicesPage = window.MegaVPNServicesPage?.create?.({
    state,
    setTitle,
    el,
    fetchJSON,
    sendJSON,
    watchJob: jobWorkflows.watchJob,
    openModal,
    closeModal,
    openUnavailableAction,
    statusTag,
    escapeHTML,
    formatDate,
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
    openModal,
    closeModal,
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

  function updateReadyPill() {
    const status = state.ready?.status || 'unknown';
    const dotClass = status === 'ready' ? 'ok' : 'danger';
    const readyPill = el('readyPill');
    const apiBaseLabel = el('apiBaseLabel');
    if (readyPill) readyPill.innerHTML = `<span class="dot ${dotClass}"></span>${escapeHTML(status)}`;
    if (apiBaseLabel) {
      apiBaseLabel.textContent = state.apiBase || 'current host';
      apiBaseLabel.title = state.apiBase || 'Frontend uses the same origin as the current browser page.';
    }
    const release = state.versionInfo?.version || state.dashboard?.version || 'unknown';
    const releaseLabel = el('releaseLabel');
    if (releaseLabel) releaseLabel.textContent = `release ${release}`;
  }

  function renderNotice() {
    const notice = el('notice');
    if (!notice) return;
    if (!state.authUser) {
      notice.hidden = true;
      return;
    }
    if (state.lastError) {
      notice.hidden = false;
      notice.innerHTML = `<strong>Last UI/API error.</strong> ${escapeHTML(state.lastError.message)}`;
      return;
    }
    notice.hidden = true;
    notice.innerHTML = '';
  }

  function renderNav() {
    const nav = el('nav');
    if (!nav) return;
    const activePage = state.page === 'nodeManage' ? 'nodes' : state.page;
    nav.innerHTML = navGroups.map(([group, items]) => `
      <div class="nav-section">${group}</div>
      ${items.map(([key, label, icon]) => `
        <button class="nav-item ${activePage === key ? 'active' : ''}" type="button" data-page="${key}">
          <span class="nav-left"><span class="nav-icon">${icon}</span>${label}</span>
        </button>
      `).join('')}
    `).join('');
    nav.querySelectorAll('[data-page]').forEach((btn) => btn.addEventListener('click', () => setPage(btn.dataset.page)));
  }

  function renderAuthSlot() {
    const slot = el('authSlot');
    if (!slot) return;
    if (!state.authUser) {
      slot.innerHTML = '<span class="tag warn">auth required</span>';
      return;
    }
    const displayName = state.authUser.display_name || state.authUser.username || state.authUser.email;
    slot.innerHTML = `
      <div class="auth-slot">
        <div class="auth-identity">
          <span class="tag ok">${escapeHTML(displayName)}</span>
          <span class="auth-username">${escapeHTML(state.authUser.username || state.authUser.email || 'operator')}</span>
        </div>
        <button class="secondary-btn" id="logoutBtn" type="button">Logout</button>
      </div>`;
    const btn = document.getElementById('logoutBtn');
    if (btn) btn.addEventListener('click', authWorkflows.logout);
  }

  function setPage(page) {
    if (page !== 'nodeManage') {
      state.nodeManageID = '';
      state.nodeManageData = null;
    }
    state.page = page;
    render();
    if (page === 'services' && state.authUser) {
      void servicesPage.loadData();
    }
  }

  function setTitle(title) {
    const pageTitle = el('pageTitle');
    if (pageTitle) pageTitle.textContent = title;
  }

  function setShellMode(isAuthenticated) {
    document.body.classList.toggle('auth-mode', !isAuthenticated);
    document.body.classList.toggle('app-mode', isAuthenticated);
    const appShell = el('appShell');
    const authGate = el('authGate');
    if (appShell) appShell.hidden = !isAuthenticated;
    if (authGate) {
      authGate.hidden = isAuthenticated;
      if (isAuthenticated) authGate.innerHTML = '';
    }
  }

  async function loadCore() {
    state.ready = await fetchJSON('/api/v1/ready', { status: 'not_ready' });
    state.versionInfo = await fetchJSON('/api/v1/version', null);
    if (!state.authUser) {
      state.dashboard = null;
      state.nodes = [];
      state.instances = [];
      state.instanceRuntimeStates = [];
      state.clients = [];
      state.jobs = [];
      state.artifacts = [];
      state.shareLinks = [];
      state.backhaulLinks = [];
      state.backhaulDrivers = [];
      state.servicesCatalog = [];
      state.servicePacks = [];
      state.servicePackCatalog = [];
      state.serviceInstallers = [];
      state.serviceCapabilitiesByNode = {};
      state.serviceInstallEventsByNode = {};
      state.mailSettings = null;
      state.controlPlaneTLSSettings = null;
      state.platformCertificates = [];
      state.platformInvites = [];
      state.platformPKIRoots = [];
      updateReadyPill();
      renderNotice();
      return;
    }
    state.dashboard = await fetchJSON('/api/v1/dashboard', null);
    const nodes = await fetchJSON('/api/v1/nodes', []);
    const instances = await fetchJSON('/api/v1/instances', []);
    const instanceRuntimeStates = hasPermission('instance.read') ? await fetchJSON('/api/v1/instances/runtime-states', []) : [];
    const clients = await fetchJSON('/api/v1/clients', []);
    const jobs = await fetchJSON('/api/v1/jobs?limit=50', []);
    const artifacts = await fetchJSON('/api/v1/artifacts', []);
    const shareLinks = await fetchJSON('/api/v1/share-links', []);
    const backhaulLinks = hasPermission('node.read') ? await fetchJSON('/api/v1/backhaul-links', []) : [];
    const backhaulDrivers = hasPermission('node.read') ? await fetchJSON('/api/v1/backhaul/drivers', []) : [];
    const servicesCatalog = await fetchJSON('/api/v1/services', []);
    const servicePacks = await fetchJSON('/api/v1/service-packs', []);
    const servicePackCatalog = hasPermission('settings.manage')
      ? await fetchJSON('/api/v1/service-packs?include_inactive=1', servicePacks)
      : servicePacks;
    const serviceInstallers = await fetchJSON('/api/v1/services/installers', []);
    const platformCertificates = (hasPermission('instance.read') || hasPermission('settings.manage')) ? await fetchJSON('/api/v1/platform/certificates', []) : [];
    const platformPKIRoots = hasPermission('instance.read') ? await fetchJSON('/api/v1/platform/pki-roots', []) : [];
    const controlPlaneTLSSettings = hasPermission('settings.manage') ? await fetchJSON('/api/v1/settings/control-plane-tls', null) : state.controlPlaneTLSSettings;
    state.nodes = Array.isArray(nodes) ? nodes.filter((node) => node.status !== 'retired') : [];
    state.instances = Array.isArray(instances) ? instances : [];
    state.instanceRuntimeStates = Array.isArray(instanceRuntimeStates) ? instanceRuntimeStates : [];
    state.clients = Array.isArray(clients) ? clients : [];
    state.jobs = Array.isArray(jobs) ? jobs : [];
    state.artifacts = Array.isArray(artifacts) ? artifacts : [];
    state.shareLinks = Array.isArray(shareLinks) ? shareLinks : [];
    state.backhaulLinks = Array.isArray(backhaulLinks) ? backhaulLinks : [];
    state.backhaulDrivers = Array.isArray(backhaulDrivers) ? backhaulDrivers : [];
    state.servicesCatalog = Array.isArray(servicesCatalog) ? servicesCatalog : [];
    state.servicePacks = Array.isArray(servicePacks) ? servicePacks : [];
    state.servicePackCatalog = Array.isArray(servicePackCatalog) ? servicePackCatalog : state.servicePacks;
    state.serviceInstallers = Array.isArray(serviceInstallers) ? serviceInstallers : [];
    state.platformCertificates = Array.isArray(platformCertificates) ? platformCertificates : [];
    state.platformPKIRoots = Array.isArray(platformPKIRoots) ? platformPKIRoots : [];
    state.controlPlaneTLSSettings = controlPlaneTLSSettings || null;
    if (!state.servicesNodeID || !state.nodes.some((node) => node.id === state.servicesNodeID)) {
      state.servicesNodeID = state.nodes[0]?.id || '';
      if (state.servicesNodeID) {
        localStorage.setItem('megavpn.servicesNodeID', state.servicesNodeID);
      } else {
        localStorage.removeItem('megavpn.servicesNodeID');
      }
    }
    if (!state.revisionsInstanceID || !state.instances.some((instance) => instance.id === state.revisionsInstanceID)) {
      state.revisionsInstanceID = state.instances[0]?.id || '';
      if (state.revisionsInstanceID) {
        localStorage.setItem('megavpn.revisionsInstanceID', state.revisionsInstanceID);
      } else {
        localStorage.removeItem('megavpn.revisionsInstanceID');
      }
    }
    updateReadyPill();
    renderNotice();
  }

  function renderDashboard() {
    setTitle('Dashboard');
    const d = state.dashboard || {};
    const instanceRows = Array.isArray(state.instances) ? state.instances : [];
    const jobRows = Array.isArray(state.jobs) ? state.jobs : [];
    const jobsTotal = Number(d.jobs_queued || 0) + Number(d.jobs_running || 0) + Number(d.jobs_failed || 0);
    el('content').innerHTML = `
      <div class="grid cols-4">
        ${metric('Nodes', d.nodes_total ?? state.nodes.length, `${Number(d.nodes_online || 0)} online`, 'nodes')}
        ${metric('Instances', d.instances_total ?? instanceRows.length, `${Number(d.instances_active || 0)} active`, 'instances')}
        ${metric('Clients', d.clients_total ?? state.clients.length, `${Number(d.clients_active || 0)} active`, 'clients')}
        ${metric('Jobs', jobsTotal || jobRows.length, `${Number(d.jobs_queued || 0)} queued · ${Number(d.jobs_running || 0)} running · ${Number(d.jobs_failed || 0)} failed`, 'jobs')}
      </div>
      ${tableCard('Service Instances', instanceRows.slice(0, 8).map((instance) => instancesPage.toRow(instance)), [
          { title: 'Name', key: 'name' },
          { title: 'Service', key: 'service', render: (r) => `<span class="tag">${escapeHTML(r.service)}</span>` },
          { title: 'Node', key: 'node' },
          { title: 'Endpoint', key: 'endpoint' },
          { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        ], '<button class="secondary-btn" type="button" id="dashboardInstancesBtn">Open instances</button>')}`;
    document.querySelectorAll('.dashboard-nav-tile').forEach((tile) => {
      const go = () => setPage(tile.dataset.pageTarget);
      tile.addEventListener('click', go);
      tile.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          go();
        }
      });
    });
    document.getElementById('dashboardInstancesBtn')?.addEventListener('click', () => setPage('instances'));
  }

  function renderNodes() {
    nodesPage.render();
  }

  function renderInstances() {
    instancesPage.render();
  }

  function renderServices() {
    servicesPage.render();
  }

  function renderClients() {
    clientsPage.render();
  }

  function renderArtifacts() {
    artifactsPage.renderArtifacts();
  }

  function renderShareLinks() {
    artifactsPage.renderShareLinks();
  }

  function renderBackhaul() {
    backhaulPage.render();
  }

  async function renderAudit() {
    return opsPages.renderAudit();
  }

  function renderTelemetry() {
    opsPages.renderTelemetry();
  }

  function renderRevisions() {
    return revisionsPage.render();
  }

  function renderUnknownPage(key) {
    setTitle('Unknown Page');
    el('content').innerHTML = `
      <section class="card">
        <h2>Unknown route</h2>
        <p>Page "${escapeHTML(key)}" is not registered in the current Control Plane UI.</p>
      </section>`;
  }

  function renderSettings() {
    settingsPage.render();
  }

  function renderCertificates() {
    certificatesPage.render();
  }


  function openModal(title, eyebrow, body, options = {}) {
    const modal = document.querySelector('.modal');
    el('modalTitle').textContent = title;
    el('modalEyebrow').textContent = eyebrow;
    el('modalBody').innerHTML = body;
    if (modal) {
      modal.classList.toggle('modal-wide', Boolean(options.wide));
    }
    el('modalBackdrop').hidden = false;
  }

  function closeModal() {
    el('modalBackdrop').hidden = true;
  }

  function setSubmitBusy(form, busy, pendingLabel = 'Working...') {
    if (!form) return;
    const submit = form.querySelector('button[type="submit"]');
    if (!submit) return;
    if (!submit.dataset.originalLabel) {
      submit.dataset.originalLabel = submit.textContent || '';
    }
    submit.disabled = Boolean(busy);
    submit.textContent = busy ? pendingLabel : submit.dataset.originalLabel;
  }

  function openActionOutcomeModal(title, eyebrow, status, message, details = []) {
    const items = Array.isArray(details) ? details.filter((item) => item && item.label) : [];
    openModal(title, eyebrow, `
      <div class="form-grid">
        <div class="field full">
          <div class="code-block">
            <div style="margin-bottom:8px">${statusTag(status)}</div>
            <div><strong>${escapeHTML(message || 'Operation finished')}</strong></div>
          </div>
        </div>
        ${items.map((item) => `
          <div class="field">
            <label>${escapeHTML(item.label)}</label>
            <div class="code-block">${escapeHTML(String(item.value ?? 'n/a'))}</div>
          </div>`).join('')}
        <div class="field full inline-actions"><button class="primary-btn" id="actionOutcomeCloseBtn" type="button">Close</button></div>
      </div>`, { wide: true });
    const btn = document.getElementById('actionOutcomeCloseBtn');
    if (btn) btn.addEventListener('click', closeModal);
  }


  function openUnavailableAction(title, text) {
    openModal(title, 'Action unavailable', `<p>${escapeHTML(text)}</p>`);
  }


  function renderCreateAction() {
    const btn = el('createActionBtn');
    if (!state.authUser) {
      btn.disabled = true;
      btn.textContent = 'Login required';
      btn.onclick = settingsWorkflows.openSettings;
      return;
    }
    const handlers = {
      nodes: nodeWorkflows.openCreateNodeModal,
      instances: instanceWorkflows.openCreateInstanceChoiceModal,
      certificates: certificateWorkflows.openCreateCertificateWizard,
      clients: clientsPage.openCreateClientModal,
      backhaul: backhaulPage.openCreateBackhaulModal,
    };
    const handler = handlers[state.page];
    btn.disabled = !handler;
    btn.textContent = handler ? 'Create' : 'No create action';
    btn.onclick = handler || settingsWorkflows.openSettings;
  }

  function render() {
    const isAuthenticated = Boolean(state.authUser);
    setShellMode(isAuthenticated);
    if (!isAuthenticated) {
      if (state.inviteToken) {
        authWorkflows.renderInviteAcceptScreen();
        return;
      }
      authWorkflows.renderLoginScreen();
      return;
    }
    renderNav();
    renderAuthSlot();
    renderCreateAction();
    renderNotice();
    if (state.page === 'dashboard') renderDashboard();
    else if (state.page === 'nodes') renderNodes();
    else if (state.page === 'nodeManage') nodeWorkflows.renderNodeManagePage();
    else if (state.page === 'services') renderServices();
    else if (state.page === 'instances') renderInstances();
    else if (state.page === 'clients') renderClients();
    else if (state.page === 'jobs') jobWorkflows.renderJobs();
    else if (state.page === 'artifacts') renderArtifacts();
    else if (state.page === 'shareLinks') renderShareLinks();
    else if (state.page === 'backhaul') renderBackhaul();
    else if (state.page === 'certificates') renderCertificates();
    else if (state.page === 'revisions') renderRevisions();
    else if (state.page === 'telemetry') renderTelemetry();
    else if (state.page === 'audit') renderAudit();
    else if (state.page === 'settings') renderSettings();
    else renderUnknownPage(state.page);
  }

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

  function autoRefreshEnabledForCurrentPage() {
    return [
      'dashboard',
      'nodes',
      'nodeManage',
      'instances',
      'jobs',
      'backhaul',
      'services',
      'revisions',
      'telemetry',
      'audit',
    ].includes(state.page);
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
