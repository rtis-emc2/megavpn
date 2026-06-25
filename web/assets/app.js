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
    watchJob,
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
    watchJob,
    waitForNodeDiagnostics,
    statusTag,
    escapeHTML,
    toMillis,
    formatDate,
    formatRelativeDate,
    formatDurationSeconds,
    platformPublicBaseURL,
  });
  if (!nodeWorkflows) throw new Error('MegaVPNNodeWorkflows is not loaded');

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
    watchJob,
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
    openSettings,
    changeOwnPassword,
    loadAdminSettings,
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
    if (btn) btn.addEventListener('click', logout);
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

  function renderAuthContent(html) {
    const mount = el('authGate') || el('content');
    if (mount) mount.innerHTML = html;
  }

  function applyAuthPayload(data) {
    state.authUser = data?.user || null;
    state.authSession = data?.session || null;
    state.authRoles = Array.isArray(data?.roles) ? data.roles : [];
    state.authPermissions = Array.isArray(data?.permissions) ? data.permissions : [];
  }

  function clearAuthPayload() {
    state.authUser = null;
    state.authSession = null;
    state.authRoles = [];
    state.authPermissions = [];
  }

  async function loadSession() {
    try {
      const data = await requestJSON('/api/v1/auth/me');
      applyAuthPayload(data);
      return true;
    } catch (err) {
      if (err.status === 401) {
        clearAuthPayload();
        return false;
      }
      throw err;
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

  function firstJobResultText(...values) {
    for (const value of values) {
      const text = String(value ?? '').trim();
      if (text) return text;
    }
    return '';
  }

  function firstUsefulJobOutputLine(value) {
    const lines = String(value ?? '').split('\n').map((line) => line.trim()).filter(Boolean);
    if (!lines.length) return '';
    const important = lines.find((line) => /options error|error:|failed|cannot|unable|exiting|status=|active:/i.test(line));
    return important || lines[0];
  }

  function jobSystemdDiagnosticText(result = {}, health = {}) {
    const unit = firstJobResultText(result.systemd_unit, health.systemd_unit);
    const state = firstJobResultText(result.health_active_state, health.active_state, result.active_state);
    const output = firstUsefulJobOutputLine(
      result.health_unit_status_output
      || health.unit_status_output
      || result.systemd_output
      || result.pre_start_stop_warning
    );
    return [unit ? `unit ${unit}` : '', state ? `state ${state}` : '', output].filter(Boolean).join(' · ');
  }

  function jobDetailedFailureText(job, result = {}, health = {}) {
    const reason = firstJobResultText(job?.error, result.health_reason, health.reason, result.error);
    const details = jobSystemdDiagnosticText(result, health);
    if (reason && details && /systemd|unit|activation|not active/i.test(reason)) {
      return `${reason} · ${details}`;
    }
    return '';
  }

  function jobHealthResultText(health = {}) {
    const reason = firstJobResultText(health.reason, health.route_warning, health.error);
    const loss = Number(health.packet_loss_percent);
    const avg = Number(health.latency_avg_ms);
    return [
      reason,
      health.active_state ? `unit ${health.active_state}` : '',
      health.interface ? `dev ${health.interface}` : '',
      Number.isFinite(loss) ? `${loss % 1 === 0 ? loss.toFixed(0) : loss.toFixed(1)}% loss` : '',
      Number.isFinite(avg) ? `${avg.toFixed(1)} ms avg` : '',
    ].filter(Boolean).join(' · ');
  }

  function jobResultText(job) {
    const result = job?.result || {};
    const health = result.health || {};
    return firstJobResultText(
      jobDetailedFailureText(job, result, health),
      job?.error,
      result.health_reason,
      result.health_route_warning,
      health.reason,
      health.route_warning,
      result.health_error,
      health.error,
      jobHealthResultText(health),
      jobSystemdDiagnosticText(result, health),
      result.error,
      result.active_state,
      result.message
    );
  }

  function renderJobs() {
    setTitle('Jobs');
    const rows = (state.jobs || []).map((job) => ({
      id: job.id,
      type: job.type,
      scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
      created: formatDate(job.created_at),
      status: job.status || 'queued',
      result: jobResultText(job),
    }));
    el('content').innerHTML = `
      ${tableCard('Job Queue', rows, [
        { title: 'ID', key: 'id' },
        { title: 'Type', key: 'type', render: (r) => `<span class="tag">${escapeHTML(r.type)}</span>` },
        { title: 'Scope', key: 'scope' },
        { title: 'Created', key: 'created' },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        { title: 'Result', key: 'result' },
      ])}
      <section class="card"><h2>Concurrency rules</h2><p>Один mutating job на instance, один bootstrap/install job на node, destructive actions через lock и audit.</p></section>`;
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

  async function loadAdminSettings(canManageAuth, canDeleteUsers = hasRole('superadmin')) {
    try {
      const canManageSettings = hasPermission('settings.manage');
      const userList = canManageAuth ? await requestJSON('/api/v1/admin/users') : [{ ...state.authUser, roles: state.authRoles || [] }];
      const sessions = canManageAuth ? await requestJSON('/api/v1/admin/sessions') : [];
      const mailSettings = canManageAuth ? await requestJSON('/api/v1/settings/mail') : null;
      const controlPlaneTLSSettings = canManageSettings ? await requestJSON('/api/v1/settings/control-plane-tls') : null;
      const runtimePreflight = canManageSettings ? await requestJSON('/api/v1/runtime/preflight') : null;
      const pkiRoots = hasPermission('instance.read') ? await requestJSON('/api/v1/platform/pki-roots') : [];
      const invites = canManageAuth ? await requestJSON('/api/v1/admin/user-invites') : [];
      if (state.page !== 'settings') return;
      state.mailSettings = mailSettings;
      state.controlPlaneTLSSettings = controlPlaneTLSSettings;
      state.runtimePreflight = runtimePreflight;
      state.platformInvites = invites || [];
      state.platformPKIRoots = pkiRoots || [];
      renderRuntimePreflight(runtimePreflight, canManageSettings);
      renderControlPlaneTLSSettings(controlPlaneTLSSettings, canManageSettings);
      renderPlatformUsers(userList || [], canManageAuth, canDeleteUsers);
      renderMailSettings(mailSettings, canManageAuth);
      renderPlatformPKIRoots(pkiRoots || [], hasPermission('instance.read'));
      renderPlatformInvites(invites || [], canManageAuth);
      renderPlatformSessions(sessions || [], canManageAuth);
    } catch (err) {
      if (state.page !== 'settings') return;
      document.getElementById('platformUsersMount').innerHTML = `<div class="empty">Failed to load admin data: ${escapeHTML(err.message)}</div>`;
      document.getElementById('runtimePreflightMount').innerHTML = `<div class="empty">Failed to load runtime preflight: ${escapeHTML(err.message)}</div>`;
      document.getElementById('mailSettingsMount').innerHTML = `<div class="empty">Failed to load mail settings: ${escapeHTML(err.message)}</div>`;
      document.getElementById('controlPlaneTLSMount').innerHTML = `<div class="empty">Failed to load TLS settings: ${escapeHTML(err.message)}</div>`;
      document.getElementById('platformPKIRootsMount').innerHTML = `<div class="empty">Failed to load CA center inventory: ${escapeHTML(err.message)}</div>`;
      document.getElementById('platformInvitesMount').innerHTML = `<div class="empty">Failed to load invites: ${escapeHTML(err.message)}</div>`;
      document.getElementById('platformSessionsMount').innerHTML = `<div class="empty">Failed to load sessions: ${escapeHTML(err.message)}</div>`;
    }
  }

  function compactID(value) {
    const text = String(value || '').trim();
    if (!text) return 'n/a';
    if (text.length <= 16) return text;
    return `${text.slice(0, 8)}...${text.slice(-6)}`;
  }

  function renderPlatformPKIRoots(roots, canRead) {
    const mount = document.getElementById('platformPKIRootsMount');
    if (!mount) return;
    if (!canRead) {
      mount.innerHTML = '<div class="empty">Platform CA Center requires instance.read permission.</div>';
      return;
    }
    const list = Array.isArray(roots) ? roots : [];
    const rows = list.length
      ? list.map((root) => `
        <tr>
          <td>${escapeHTML(root.service_code || 'n/a')}</td>
          <td>${escapeHTML(root.pki_profile || 'default')}</td>
          <td>${statusTag(certificateDisplayStatus(root))}</td>
          <td>${escapeHTML(root.common_name || 'n/a')}</td>
          <td><code title="${escapeHTML(root.ca_cert_secret_ref_id || '')}">${escapeHTML(compactID(root.ca_cert_secret_ref_id))}</code></td>
          <td>${escapeHTML(certificateExpiryCaption(root))}</td>
          <td>${formatDate(root.created_at)}</td>
          <td>${formatDate(root.rotated_at)}</td>
        </tr>`)
        .join('')
      : '<tr><td colspan="8"><div class="empty">No platform CA roots yet. The OpenVPN default CA is created on first OpenVPN apply or client provisioning.</div></td></tr>';
    mount.innerHTML = `
      <div class="metric-caption" style="margin-bottom:12px">Control plane хранит platform PKI inventory для сервисов, где CA lifecycle должен быть централизован. Сейчас production path активен для OpenVPN.</div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Service</th><th>Profile</th><th>Status</th><th>Common Name</th><th>CA Cert Ref</th><th>Expires</th><th>Created</th><th>Rotated</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>`;
  }

  function renderPlatformUsers(users, canManageAuth, canDeleteUsers) {
    const mount = document.getElementById('platformUsersMount');
    const rows = users.length
      ? users.map((user) => {
        const isCurrentUser = user.id === state.authUser?.id;
        const actions = !canManageAuth
          ? statusTag(user.status || 'active')
          : `
            <div class="inline-actions compact-actions">
              ${user.status === 'active'
                ? `<button class="danger-btn user-status-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-status="disabled">Disable</button>`
                : `<button class="secondary-btn user-status-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-status="active">Activate</button>`}
              <button class="secondary-btn resend-invite-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-username="${escapeHTML(user.username || 'operator')}">Resend invite</button>
              <button class="secondary-btn reset-password-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-username="${escapeHTML(user.username || 'operator')}">Reset password</button>
              ${canDeleteUsers && !isCurrentUser
                ? `<button class="danger-btn delete-user-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-username="${escapeHTML(user.username || 'operator')}">Delete user</button>`
                : ''}
            </div>`;
        return `
          <tr>
            <td>${escapeHTML(user.username || 'n/a')}${isCurrentUser ? ' <span class="tag ok">you</span>' : ''}</td>
            <td>${escapeHTML(user.display_name || 'n/a')}</td>
            <td>${escapeHTML(user.email || 'n/a')}</td>
            <td>${statusTag(user.status || 'active')}</td>
            <td>${escapeHTML((user.roles || []).join(', ') || 'none')}</td>
            <td>${formatDate(user.last_login_at)}</td>
            <td>${actions}</td>
          </tr>`;
      }).join('')
      : '<tr><td colspan="7"><div class="empty">No platform users found.</div></td></tr>';
    mount.innerHTML = `
      ${canManageAuth ? `
        <form id="createOperatorForm" class="form-grid operator-form" style="margin-bottom:18px">
          <div class="field"><label>Username</label><input name="username" required placeholder="operator-01" /></div>
          <div class="field"><label>Display name</label><input name="display_name" placeholder="Operator 01" /></div>
          <div class="field"><label>Email</label><input name="email" type="email" placeholder="operator-01@rtis.local" /></div>
          <div class="field"><label>Invite TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="48" /></div>
          <div class="field full"><label>Roles</label><select name="role_codes" multiple size="4"><option value="superadmin">superadmin</option><option value="admin" selected>admin</option><option value="engineer">engineer</option><option value="readonly">readonly</option></select></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Invite operator</button></div>
        </form>
        <div id="createOperatorResult" class="form-result"></div>
      ` : ''}
      <div id="platformUsersResult" class="form-result"></div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Username</th><th>Display</th><th>Email</th><th>Status</th><th>Roles</th><th>Last Login</th><th>Actions</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>`;
    if (canManageAuth) {
      document.getElementById('createOperatorForm').addEventListener('submit', createOperator);
      mount.querySelectorAll('.user-status-btn').forEach((button) => {
        button.addEventListener('click', () => setPlatformUserStatus(button.dataset.userId, button.dataset.status));
      });
      mount.querySelectorAll('.resend-invite-btn').forEach((button) => {
        button.addEventListener('click', () => openResendInviteModal(button.dataset.userId, button.dataset.username));
      });
      mount.querySelectorAll('.reset-password-btn').forEach((button) => {
        button.addEventListener('click', () => openResetPasswordModal(button.dataset.userId, button.dataset.username));
      });
      mount.querySelectorAll('.delete-user-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteUserModal(button.dataset.userId, button.dataset.username));
      });
    }
  }

  function renderPlatformSessions(sessions, canManageAuth) {
    const mount = document.getElementById('platformSessionsMount');
    const rows = sessions.length
      ? sessions.map((session) => `
          <tr>
            <td>${escapeHTML(session.username || 'n/a')}${session.id === state.authSession?.id ? ' <span class="tag ok">current</span>' : ''}</td>
            <td>${escapeHTML(session.ip || 'n/a')}</td>
            <td>${escapeHTML(session.user_agent || 'n/a')}</td>
            <td>${formatDate(session.created_at)}</td>
            <td>${formatDate(session.expires_at)}</td>
            <td>${canManageAuth ? `<button class="danger-btn revoke-session-btn" type="button" data-session-id="${escapeHTML(session.id)}">Revoke</button>` : statusTag('active')}</td>
          </tr>`).join('')
      : '<tr><td colspan="6"><div class="empty">No active sessions.</div></td></tr>';
    mount.innerHTML = `
      <div id="revokeSessionResult" class="form-result"></div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Username</th><th>IP</th><th>User Agent</th><th>Created</th><th>Expires</th><th>Action</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>`;
    if (canManageAuth) {
      document.querySelectorAll('.revoke-session-btn').forEach((button) => {
        button.addEventListener('click', () => revokePlatformSession(button.dataset.sessionId));
      });
    }
  }

  function renderRuntimePreflight(report, canManageSettings) {
    const mount = document.getElementById('runtimePreflightMount');
    if (!mount) return;
    if (!canManageSettings) {
      mount.innerHTML = '<div class="empty">Runtime preflight requires settings.manage permission.</div>';
      return;
    }
    const checks = arrayOrEmpty(report?.checks);
    const status = report?.status || 'unknown';
    const failed = checks.filter((check) => check.status === 'failed').length;
    const warnings = checks.filter((check) => check.status === 'warning').length;
    mount.innerHTML = `
      <div class="inventory-facts" style="margin-bottom:16px">
        <div class="fact-card">
          <div class="mini-label">Overall status</div>
          <div class="metric-caption strong">${statusTag(status)}</div>
          <div class="metric-caption">Production readiness gate</div>
        </div>
        ${renderInventoryFact('Version', report?.version || state.versionInfo?.version || 'unknown', 'API build')}
        ${renderInventoryFact('Generated', formatDate(report?.generated_at), `${failed} failed · ${warnings} warnings`)}
      </div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Check</th><th>Status</th><th>Summary</th><th>Detail</th></tr></thead>
          <tbody>
            ${checks.length ? checks.map((check) => `
              <tr>
                <td><code>${escapeHTML(check.code || 'unknown')}</code></td>
                <td>${statusTag(check.status || 'unknown')}</td>
                <td>${escapeHTML(check.summary || 'n/a')}</td>
                <td>${escapeHTML(check.detail || '')}</td>
              </tr>`).join('') : '<tr><td colspan="4"><div class="empty">No preflight checks returned.</div></td></tr>'}
          </tbody>
        </table>
      </div>`;
  }

  function renderControlPlaneTLSSettings(settings, canManageSettings) {
    const mount = document.getElementById('controlPlaneTLSMount');
    if (!mount) return;
    const current = settings || {};
    const certID = current.certificate_id || '';
    const selectedCert = certID ? (state.platformCertificates || []).find((item) => item.id === certID) : null;
    const mode = current.mode || 'managed_certificate';
    mount.innerHTML = `
      ${canManageSettings ? `
        <form id="controlPlaneTLSForm" class="form-grid operator-form" style="margin-bottom:18px">
          <div class="field"><label>Enabled</label><select name="enabled"><option value="true"${current.enabled !== false ? ' selected' : ''}>true</option><option value="false"${current.enabled === false ? ' selected' : ''}>false</option></select></div>
          <div class="field"><label>Mode</label><select name="mode"><option value="managed_certificate"${mode !== 'self_signed_fallback' ? ' selected' : ''}>managed certificate</option><option value="self_signed_fallback"${mode === 'self_signed_fallback' ? ' selected' : ''}>self-signed fallback</option></select></div>
          <div class="field"><label>Public HTTPS URL</label><input name="public_base_url" value="${escapeHTML(current.public_base_url || platformPublicBaseURL() || 'https://control.example.com:58765')}" placeholder="https://control.example.com:58765" /></div>
          <div class="field"><label>Server name</label><input name="server_name" value="${escapeHTML(current.server_name || publicURLHostname(platformPublicBaseURL()) || '')}" placeholder="control.example.com" /></div>
          <div class="field"><label>Listen port</label><input name="listen_port" type="number" min="1" max="65535" value="${escapeHTML(String(current.listen_port || publicURLPort(platformPublicBaseURL()) || 443))}" /></div>
          <div class="field"><label>Upstream</label><input name="upstream_url" value="${escapeHTML(current.upstream_url || 'http://127.0.0.1:8080')}" /></div>
          <div class="field full"><label>Managed certificate</label><select name="certificate_id">${certificateOptions(certID, true)}</select></div>
          <div class="field"><label>Self-signed CN</label><input name="self_signed_common_name" value="${escapeHTML(current.self_signed_common_name || current.server_name || publicURLHostname(platformPublicBaseURL()) || '')}" /></div>
          <div class="field"><label>Self-signed SAN</label><input name="self_signed_dns_names" value="${escapeHTML(Array.isArray(current.self_signed_dns_names) ? current.self_signed_dns_names.join(', ') : '')}" placeholder="control.example.com" /></div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save TLS profile</button>
            <button class="secondary-btn" id="applyControlPlaneTLSBtn" type="button">Apply edge</button>
            <button class="secondary-btn" id="openCertificateManagerBtn" type="button">Open certificates</button>
          </div>
        </form>
        <div id="controlPlaneTLSResult" class="form-result"></div>
      ` : `<div class="empty">Control Plane TLS settings require settings.manage.</div>`}
      <div class="grid cols-3">
        <div class="card"><div class="mini-label">Public edge</div><div class="metric-caption strong">${escapeHTML(current.public_base_url || platformPublicBaseURL() || 'missing')}</div><div class="metric-caption">Agents use this exact HTTPS URL.</div></div>
        <div class="card"><div class="mini-label">TLS mode</div><div class="metric-caption">${statusTag(current.enabled === false ? 'disabled' : 'enabled')}</div><div class="metric-caption">${escapeHTML(mode)}</div></div>
        <div class="card"><div class="mini-label">Certificate</div><div class="metric-caption strong">${escapeHTML(selectedCert?.name || selectedCert?.common_name || (certID ? certID : 'not selected'))}</div><div class="metric-caption">${escapeHTML(selectedCert ? `${selectedCert.source || 'certificate'} · ${certificateExpiryCaption(selectedCert)}` : 'Use commercial import or self-signed fallback.')}</div></div>
        <div class="card"><div class="mini-label">Upstream boundary</div><div class="metric-caption">${escapeHTML(current.upstream_url || 'http://127.0.0.1:8080')}</div><div class="metric-caption">Loopback only behind TLS edge.</div></div>
        <div class="card"><div class="mini-label">Last apply</div><div class="metric-caption">${formatDate(current.last_applied_at)}</div></div>
        <div class="card"><div class="mini-label">Last error</div><div class="metric-caption">${escapeHTML(current.last_error || 'none')}</div></div>
      </div>`;
    if (canManageSettings) {
      document.getElementById('controlPlaneTLSForm').addEventListener('submit', saveControlPlaneTLSSettings);
      document.getElementById('applyControlPlaneTLSBtn').addEventListener('click', applyControlPlaneTLSSettings);
      document.getElementById('openCertificateManagerBtn').addEventListener('click', () => setPage('certificates'));
    }
  }

  function renderMailSettings(settings, canManageAuth) {
    const mount = document.getElementById('mailSettingsMount');
    if (!mount) return;
    const current = settings || {};
    const passwordState = current.smtp_password_configured ? 'configured' : 'missing';
    const passwordStateDetail = current.smtp_password_configured
      ? `Secret ref is stored${current.smtp_password_secret_ref_id ? ` · ${current.smtp_password_secret_ref_id}` : ''}. Leave the field empty to keep it.`
      : 'No SMTP password is stored yet. Save one to enable authenticated delivery.';
    mount.innerHTML = `
      ${canManageAuth ? `
        <form id="mailSettingsForm" class="form-grid operator-form" style="margin-bottom:18px">
          <div class="field"><label>Enabled</label><select name="enabled"><option value="true"${current.enabled ? ' selected' : ''}>true</option><option value="false"${!current.enabled ? ' selected' : ''}>false</option></select></div>
          <div class="field"><label>SMTP host</label><input name="smtp_host" value="${escapeHTML(current.smtp_host || '')}" placeholder="smtp.example.com" /></div>
          <div class="field"><label>SMTP port</label><input name="smtp_port" type="number" min="1" max="65535" value="${escapeHTML(String(current.smtp_port || 587))}" /></div>
          <div class="field"><label>SMTP username</label><input name="smtp_username" value="${escapeHTML(current.smtp_username || '')}" /></div>
          <div class="field"><label>Auth mode</label><select name="smtp_auth_mode"><option value="plain"${current.smtp_auth_mode === 'plain' ? ' selected' : ''}>plain</option><option value="none"${current.smtp_auth_mode === 'none' ? ' selected' : ''}>none</option></select></div>
          <div class="field"><label>TLS mode</label><select name="smtp_tls_mode"><option value="starttls"${current.smtp_tls_mode === 'starttls' ? ' selected' : ''}>starttls</option><option value="starttls_required"${current.smtp_tls_mode === 'starttls_required' ? ' selected' : ''}>starttls_required</option><option value="none"${current.smtp_tls_mode === 'none' ? ' selected' : ''}>none</option></select></div>
          <div class="field"><label>From email</label><input name="from_email" type="email" value="${escapeHTML(current.from_email || '')}" /></div>
          <div class="field"><label>From name</label><input name="from_name" value="${escapeHTML(current.from_name || '')}" /></div>
          <div class="field"><label>Reply-to</label><input name="reply_to_email" type="email" value="${escapeHTML(current.reply_to_email || '')}" /></div>
          <div class="field"><label>Invite URL base</label><input name="invite_url_base" value="${escapeHTML(current.invite_url_base || '')}" placeholder="${escapeHTML(state.apiBase || window.location.origin)}" /></div>
          <div class="field full"><label>SMTP password</label><textarea name="smtp_password" rows="3" placeholder="${current.smtp_password_secret_ref_id ? 'Leave empty to keep existing secret ref.' : 'Paste SMTP password or app token.'}"></textarea></div>
          <div class="field full">
            <div class="inline-actions">
              ${statusTag(passwordState)}
              <span class="metric-caption">${escapeHTML(passwordStateDetail)}</span>
            </div>
          </div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save mail settings</button>
            <input id="mailTestEmail" type="email" placeholder="test@example.com" style="max-width:280px" />
            <button class="secondary-btn" id="sendMailTestBtn" type="button">Send test email</button>
          </div>
        </form>
        <div id="mailSettingsResult" class="form-result"></div>
      ` : `<div class="empty">Mail settings are visible only with auth.manage.</div>`}
      <div class="grid cols-3">
        <div class="card"><div class="mini-label">Status</div><div class="metric-caption">${statusTag(current.enabled ? 'enabled' : 'disabled')}</div></div>
        <div class="card"><div class="mini-label">SMTP Password</div><div class="metric-caption">${statusTag(passwordState)}</div><div class="metric-caption">${escapeHTML(current.smtp_password_configured ? 'Stored in secret storage.' : 'Not stored.')}</div></div>
        <div class="card"><div class="mini-label">Last test</div><div class="metric-caption">${formatDate(current.last_test_at)}</div></div>
        <div class="card"><div class="mini-label">Last error</div><div class="metric-caption">${escapeHTML(current.last_error || 'none')}</div></div>
      </div>`;
    if (canManageAuth) {
      document.getElementById('mailSettingsForm').addEventListener('submit', saveMailSettings);
      document.getElementById('sendMailTestBtn').addEventListener('click', sendMailTest);
    }
  }

  function renderPlatformInvites(invites) {
    const mount = document.getElementById('platformInvitesMount');
    if (!mount) return;
    const rows = invites.length
      ? invites.map((invite) => `
          <tr>
            <td>${escapeHTML(invite.username || 'n/a')}</td>
            <td>${escapeHTML(invite.email || 'n/a')}</td>
            <td>${statusTag(invite.status || 'pending')}</td>
            <td>${formatDate(invite.expires_at)}</td>
            <td>${formatDate(invite.sent_at)}</td>
            <td>${escapeHTML(invite.delivery_error || 'n/a')}</td>
          </tr>`).join('')
      : '<tr><td colspan="6"><div class="empty">No operator invites yet.</div></td></tr>';
    mount.innerHTML = `
      <div class="table-wrap">
        <table>
          <thead><tr><th>Username</th><th>Email</th><th>Status</th><th>Expires</th><th>Sent</th><th>Delivery</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>`;
  }

  function renderLoginScreen() {
    setTitle('Operator Login');
    renderAuthContent(authView?.renderLogin
      ? authView.renderLogin({ apiBase: state.apiBase })
      : '<section class="auth-card"><h2>Login</h2><form id="loginForm" class="form-grid"><div class="field full"><label>Login</label><input name="login" required /></div><div class="field full"><label>Password</label><input name="password" type="password" required /></div><button class="primary-btn" type="submit">Login</button></form><div id="loginResult" class="auth-message"></div><button class="secondary-btn" type="button" id="loginSettingsBtn">API Settings</button></section>');
    document.getElementById('loginForm').addEventListener('submit', login);
    document.getElementById('loginSettingsBtn').addEventListener('click', openSettings);
  }

  function renderInviteAcceptScreen() {
    const invite = state.invitePreview || {};
    setTitle('Invitation');
    renderAuthContent(authView?.renderInvite
      ? authView.renderInvite({ invite })
      : '<section class="auth-card"><h2>Invitation</h2><form id="inviteAcceptForm" class="form-grid"><div class="field full"><label>Password</label><input name="password" type="password" required /></div><button class="primary-btn" type="submit">Activate account</button></form><button class="secondary-btn" type="button" id="inviteBackBtn">Back to login</button><div id="inviteAcceptResult" class="auth-message"></div></section>');
    document.getElementById('inviteAcceptForm').addEventListener('submit', acceptInvite);
    document.getElementById('inviteBackBtn').addEventListener('click', () => clearInviteToken(true));
  }

  async function createOperator(event) {
    event.preventDefault();
    const target = document.getElementById('createOperatorResult');
    target.innerHTML = '<span class="tag warn">sending invitation</span>';
    try {
      const form = new FormData(event.currentTarget);
      const roleSelect = event.currentTarget.querySelector('[name="role_codes"]');
      const roleCodes = Array.from(roleSelect.selectedOptions).map((option) => option.value);
      const data = await sendJSON('/api/v1/admin/users/invite', 'POST', {
        username: String(form.get('username') || '').trim(),
        display_name: String(form.get('display_name') || '').trim(),
        email: String(form.get('email') || '').trim(),
        role_codes: roleCodes,
        ttl_hours: Number(form.get('ttl_hours') || 48),
      });
      target.innerHTML = renderActionResponse(data, 'Operator invite');
      await loadAdminSettings(true);
      event.currentTarget.reset();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function changeOwnPassword(event) {
    event.preventDefault();
    const target = document.getElementById('changePasswordResult');
    target.innerHTML = '<span class="tag warn">updating</span>';
    try {
      const form = new FormData(event.currentTarget);
      const data = await sendJSON('/api/v1/auth/change-password', 'POST', {
        current_password: String(form.get('current_password') || ''),
        new_password: String(form.get('new_password') || ''),
      });
      target.innerHTML = '<span class="tag ok">password updated, re-login required</span>';
      if (data?.relogin_required) {
        state.authUser = null;
        state.authSession = null;
        state.authRoles = [];
        state.authPermissions = [];
        setTimeout(() => render(), 200);
      }
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function setPlatformUserStatus(userID, status) {
    const target = document.getElementById('platformUsersResult');
    if (target) target.innerHTML = '<span class="tag warn">updating operator status</span>';
    try {
      await sendJSON(`/api/v1/admin/users/${userID}/status`, 'POST', { status });
      if (target) target.innerHTML = '<span class="tag ok">operator status updated</span>';
      await loadAdminSettings(true);
    } catch (err) {
      if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function openResetPasswordModal(userID, username) {
    openModal(`Reset password: ${username}`, 'Platform user password reset', `
      <form id="resetPasswordForm" class="form-grid">
        <div class="field full"><label>New password for ${escapeHTML(username)}</label><input name="password" type="password" required placeholder="minimum 12 chars" /></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Reset password</button></div>
      </form>
      <div id="resetPasswordResult" class="form-result"></div>`);
    document.getElementById('resetPasswordForm').addEventListener('submit', (event) => submitPlatformUserPasswordReset(event, userID));
  }

  function openResendInviteModal(userID, username) {
    openModal(`Resend invite: ${username}`, 'Operator invitation delivery', `
      <form id="resendInviteForm" class="form-grid">
        <div class="field full"><label>Invite TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="48" /></div>
        <div class="field full"><label>Delivery note</label><div class="code-block">A fresh one-time invite link will be generated and sent to the operator email address stored in the platform user record.</div></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Resend invite</button></div>
      </form>
      <div id="resendInviteResult" class="form-result"></div>`);
    document.getElementById('resendInviteForm').addEventListener('submit', (event) => submitResendInvite(event, userID));
  }

  async function submitResendInvite(event, userID) {
    event.preventDefault();
    const target = document.getElementById('resendInviteResult');
    target.innerHTML = '<span class="tag warn">sending invite</span>';
    try {
      const form = new FormData(event.currentTarget);
      const data = await sendJSON(`/api/v1/admin/users/${userID}/resend-invite`, 'POST', {
        ttl_hours: Number(form.get('ttl_hours') || 48),
      });
      target.innerHTML = renderActionResponse(data, 'Operator invite resent');
      await loadAdminSettings(true);
      setTimeout(closeModal, 700);
    } catch (err) {
      target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Operator invite failed');
    }
  }

  function openDeleteUserModal(userID, username) {
    openModal(`Delete user: ${username}`, 'Superadmin action', `
      <div class="form-grid">
        <div class="field full"><label>Confirmation</label><div class="code-block">This will permanently remove the operator account, its active sessions and related invite records. The current operator and the last superadmin cannot be deleted.</div></div>
        <div class="field full inline-actions"><button class="danger-btn" id="confirmDeleteUserBtn" type="button">Delete user</button></div>
      </div>
      <div id="deleteUserResult" class="form-result"></div>`);
    document.getElementById('confirmDeleteUserBtn').addEventListener('click', () => submitDeleteUser(userID));
  }

  async function submitDeleteUser(userID) {
    const target = document.getElementById('deleteUserResult');
    target.innerHTML = '<span class="tag warn">deleting user</span>';
    try {
      await requestJSON(`/api/v1/admin/users/${userID}`, { method: 'DELETE' });
      target.innerHTML = '<span class="tag ok">operator deleted</span>';
      await loadAdminSettings(true);
      setTimeout(closeModal, 500);
    } catch (err) {
      target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Operator delete failed');
    }
  }

  async function submitPlatformUserPasswordReset(event, userID) {
    event.preventDefault();
    const target = document.getElementById('resetPasswordResult');
    target.innerHTML = '<span class="tag warn">resetting</span>';
    try {
      const form = new FormData(event.currentTarget);
      await sendJSON(`/api/v1/admin/users/${userID}/reset-password`, 'POST', {
        password: String(form.get('password') || ''),
      });
      target.innerHTML = '<span class="tag ok">password updated, sessions revoked</span>';
      await loadAdminSettings(true);
      setTimeout(closeModal, 500);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function revokePlatformSession(sessionID) {
    const target = document.getElementById('revokeSessionResult');
    target.innerHTML = '<span class="tag warn">revoking</span>';
    try {
      await requestJSON(`/api/v1/admin/sessions/${sessionID}/revoke`, { method: 'POST' });
      target.innerHTML = '<span class="tag ok">session revoked</span>';
      await loadAdminSettings(true);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
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

  function openSettings() {
    openModal('Interface Settings', 'Local browser settings', `
      <div class="form-grid">
        <div class="field full"><label>API base URL. Empty value means same-origin.</label><input id="apiBaseInput" value="${escapeHTML(state.apiBase)}" placeholder="https://vpn-panel.example.com" /></div>
        <div class="field full"><label>Install note</label><div class="code-block">By default RTIS MegaVPN API can serve web/index.html and /assets/* itself when the web root is available. Set API base URL here only if the UI is hosted elsewhere.</div></div>
      </div>
      <div class="modal-actions"><button class="secondary-btn" id="saveSettingsBtn" type="button">Save</button></div>`);
    document.getElementById('saveSettingsBtn').addEventListener('click', () => {
      state.apiBase = document.getElementById('apiBaseInput').value.trim().replace(/\/$/, '');
      localStorage.setItem('megavpn.apiBase', state.apiBase);
      updateReadyPill();
      closeModal();
      refresh();
    });
  }

  async function login(event) {
    event.preventDefault();
    const target = document.getElementById('loginResult');
    const formEl = event.currentTarget;
    state.refreshSeq += 1;
    target.innerHTML = '<span class="tag warn">authorizing</span>';
    setSubmitBusy(formEl, true, 'Login...');
    try {
      const form = new FormData(event.currentTarget);
      const data = await sendJSON('/api/v1/auth/login', 'POST', {
        login: String(form.get('login') || '').trim(),
        password: String(form.get('password') || ''),
      });
      applyAuthPayload(data);
      await refresh();
      if (!state.authUser) {
        renderLoginScreen();
        const currentTarget = document.getElementById('loginResult');
        if (currentTarget) {
          currentTarget.innerHTML = '<span class="tag danger">Сессия не открылась после входа. Обновите страницу и повторите вход.</span>';
        }
        return;
      }
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    } finally {
      setSubmitBusy(formEl, false);
    }
  }

  async function logout() {
    try {
      await requestJSON('/api/v1/auth/logout', { method: 'POST' });
    } catch (_) {
      // Session may already be gone; local cleanup still matters.
    }
    state.authUser = null;
    state.authSession = null;
    state.authRoles = [];
    state.authPermissions = [];
    state.invitePreview = null;
    state.lastError = null;
    render();
  }

  function openUnavailableAction(title, text) {
    openModal(title, 'Action unavailable', `<p>${escapeHTML(text)}</p>`);
  }

  async function saveMailSettings(event) {
    event.preventDefault();
    const target = document.getElementById('mailSettingsResult');
    target.innerHTML = '<span class="tag warn">saving</span>';
    try {
      const form = new FormData(event.currentTarget);
      const password = String(form.get('smtp_password') || '').trim();
      await sendJSON('/api/v1/settings/mail', 'PUT', {
        enabled: String(form.get('enabled')) === 'true',
        smtp_host: String(form.get('smtp_host') || '').trim(),
        smtp_port: Number(form.get('smtp_port') || 587),
        smtp_username: String(form.get('smtp_username') || '').trim(),
        smtp_password_secret_ref_id: state.mailSettings?.smtp_password_secret_ref_id || null,
        smtp_password: password,
        smtp_auth_mode: String(form.get('smtp_auth_mode') || 'plain'),
        smtp_tls_mode: String(form.get('smtp_tls_mode') || 'starttls'),
        from_email: String(form.get('from_email') || '').trim(),
        from_name: String(form.get('from_name') || '').trim(),
        reply_to_email: String(form.get('reply_to_email') || '').trim(),
        invite_url_base: String(form.get('invite_url_base') || '').trim(),
      });
      target.innerHTML = '<span class="tag ok">mail settings saved</span>';
      await loadAdminSettings(true);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function saveControlPlaneTLSSettings(event) {
    event.preventDefault();
    const target = document.getElementById('controlPlaneTLSResult');
    target.innerHTML = '<span class="tag warn">saving</span>';
    try {
      const form = new FormData(event.currentTarget);
      await sendJSON('/api/v1/settings/control-plane-tls', 'PUT', {
        enabled: String(form.get('enabled')) === 'true',
        mode: String(form.get('mode') || 'managed_certificate'),
        public_base_url: String(form.get('public_base_url') || '').trim(),
        server_name: String(form.get('server_name') || '').trim(),
        listen_port: Number(form.get('listen_port') || 443),
        upstream_url: String(form.get('upstream_url') || 'http://127.0.0.1:8080').trim(),
        certificate_id: String(form.get('certificate_id') || '').trim(),
        self_signed_common_name: String(form.get('self_signed_common_name') || '').trim(),
        self_signed_dns_names: parseCSVList(String(form.get('self_signed_dns_names') || '')),
      });
      target.innerHTML = '<span class="tag ok">TLS profile saved</span>';
      await loadAdminSettings(hasPermission('auth.manage'));
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function applyControlPlaneTLSSettings() {
    const target = document.getElementById('controlPlaneTLSResult');
    target.innerHTML = '<span class="tag warn">queueing apply job</span>';
    try {
      const job = await sendJSON('/api/v1/settings/control-plane-tls/apply', 'POST', {});
      await watchJob(job.id, target, 'Control Plane TLS apply', {
        attempts: 80,
        intervalMs: 1500,
        context: { public_url: state.controlPlaneTLSSettings?.public_base_url || platformPublicBaseURL() || 'n/a' },
      });
      await loadAdminSettings(hasPermission('auth.manage'));
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function sendMailTest() {
    const target = document.getElementById('mailSettingsResult');
    target.innerHTML = '<span class="tag warn">sending test</span>';
    try {
      const email = String(document.getElementById('mailTestEmail').value || '').trim();
      const data = await sendJSON('/api/v1/settings/mail/test', 'POST', { email });
      target.innerHTML = renderActionResponse(data, 'Mail test');
      await loadAdminSettings(true);
    } catch (err) {
      target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Mail test failed');
    }
  }

  async function loadInvitePreview() {
    if (!state.inviteToken) {
      state.invitePreview = null;
      return;
    }
    try {
      state.invitePreview = await requestJSON(`/api/v1/auth/invites/${encodeURIComponent(state.inviteToken)}`);
    } catch (err) {
      state.invitePreview = { status: 'invalid', error: err.message };
      state.lastError = err;
    }
  }

  async function acceptInvite(event) {
    event.preventDefault();
    const target = document.getElementById('inviteAcceptResult');
    target.innerHTML = '<span class="tag warn">activating</span>';
    try {
      const form = new FormData(event.currentTarget);
      await sendJSON(`/api/v1/auth/invites/${encodeURIComponent(state.inviteToken)}/accept`, 'POST', {
        password: String(form.get('password') || ''),
      });
      clearInviteToken(false);
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function clearInviteToken(shouldRender) {
    state.inviteToken = '';
    state.invitePreview = null;
    const url = new URL(window.location.href);
    url.searchParams.delete('invite_token');
    window.history.replaceState({}, '', url.toString());
    if (shouldRender) render();
  }

  function sleep(ms) {
    return new Promise((resolve) => window.setTimeout(resolve, ms));
  }

  function describeJobStatus(job, label) {
    const status = String(job?.status || '').toLowerCase();
    if (status === 'queued') return `${label} queued in control plane. Waiting for agent pickup.`;
    if (status === 'running' || status === 'retrying') return `${label} is running on the target node.`;
    if (status === 'succeeded') return stringValue(job?.result?.message, `${label} completed successfully.`);
    if (status === 'failed') return stringValue(job?.result?.error, job?.result?.message, `${label} failed. Check logs below.`);
    if (status === 'cancelled') return `${label} was cancelled before completion.`;
    return `${label} status: ${status || 'unknown'}.`;
  }

  function renderWatchedJob(job, logs, label, context = {}) {
    const contextRows = [
      ['Target node', context.node],
      ['Service', context.service],
      ['Strategy', context.strategy],
      ['Channel', context.channel],
      ['Job type', job?.type],
      ['Job ID', job?.id],
      ['Created', formatDate(job?.created_at)],
      ['Started', formatDate(job?.started_at)],
      ['Finished', formatDate(job?.finished_at)],
    ].filter(([, value]) => value && value !== 'n/a');
    const logRows = Array.isArray(logs) ? logs : [];
    return `
      <div class="card">
        <div class="inline-actions" style="justify-content:space-between;align-items:flex-start">
          <div>
            <div class="mini-label">${escapeHTML(label)}</div>
            <div class="metric-caption">${escapeHTML(describeJobStatus(job, label))}</div>
          </div>
          ${statusTag(job?.status || 'unknown')}
        </div>
        <div class="grid cols-2" style="margin-top:14px">
          ${contextRows.map(([title, value]) => `
            <div class="card">
              <div class="mini-label">${escapeHTML(title)}</div>
              <div class="metric-caption">${escapeHTML(String(value))}</div>
            </div>`).join('')}
        </div>
      </div>
      ${job?.result ? `
        <section class="card">
          <div class="table-head compact-head"><h3>Final Result</h3>${statusTag(job.status || 'unknown')}</div>
          ${renderActionResponse(job.result, 'Job final result')}
        </section>` : ''}
      <section class="card">
        <div class="table-head compact-head"><h3>Execution Log</h3><span class="tag">${escapeHTML(String(logRows.length))} entries</span></div>
        ${logRows.length ? `
          <div class="timeline">
            ${logRows.map((entry) => `
              <div class="timeline-item">
                <strong>${escapeHTML(formatDate(entry.created_at))} · ${escapeHTML(String(entry.level || 'info').toUpperCase())}</strong>
                <div class="timeline-meta">${escapeHTML(entry.message || '')}</div>
                ${entry.payload && Object.keys(entry.payload || {}).length ? renderActionResponse(entry.payload, 'Log payload') : ''}
              </div>`).join('')}
          </div>` : '<div class="empty">No job log entries yet.</div>'}
      </section>`;
  }

  async function watchJob(jobID, targetID, label = 'Job', options = {}) {
    const target = typeof targetID === 'string' ? document.getElementById(targetID) : targetID;
    if (!target) return null;
    const attempts = Number(options.attempts || 20);
    const intervalMs = Number(options.intervalMs || 1500);
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      const [job, logs] = await Promise.all([
        requestJSON(`/api/v1/jobs/${jobID}`),
        fetchJSON(`/api/v1/jobs/${jobID}/logs?limit=20`, []),
      ]);
      target.innerHTML = renderWatchedJob(job, logs, label, options.context || {});
      if (['succeeded', 'failed', 'cancelled'].includes(String(job.status || '').toLowerCase())) {
        return job;
      }
      await sleep(intervalMs);
    }
    target.innerHTML += '<div class="tag warn">job polling timed out; refresh jobs for the latest status</div>';
    return null;
  }

  async function waitForNodeDiagnostics(nodeID, targetID, label, predicate, attempts = 20) {
    const target = typeof targetID === 'string' ? document.getElementById(targetID) : targetID;
    if (!target) return null;
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      const diag = await requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`);
      target.innerHTML += renderActionResponse({
          wait: label,
          attempt: attempt + 1,
          heartbeat_state: diag?.heartbeat_state,
          last_heartbeat_at: diag?.node?.last_heartbeat_at,
          agent_status: diag?.agent?.status,
          token_rotation_status: diag?.agent?.token_rotation_status,
        }, 'Diagnostics wait');
      if (predicate(diag)) {
        return diag;
      }
      await sleep(2000);
    }
    target.innerHTML += `<div class="tag warn">${escapeHTML(label)} timed out; refresh diagnostics for the latest state</div>`;
    return null;
  }

  function renderCreateAction() {
    const btn = el('createActionBtn');
    if (!state.authUser) {
      btn.disabled = true;
      btn.textContent = 'Login required';
      btn.onclick = openSettings;
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
    btn.onclick = handler || openSettings;
  }

  function render() {
    const isAuthenticated = Boolean(state.authUser);
    setShellMode(isAuthenticated);
    if (!isAuthenticated) {
      if (state.inviteToken) {
        renderInviteAcceptScreen();
        return;
      }
      renderLoginScreen();
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
    else if (state.page === 'jobs') renderJobs();
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
        await loadInvitePreview();
        if (seq !== state.refreshSeq) return;
      }
      await loadSession();
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
    el('openSettingsBtn')?.addEventListener('click', openSettings);
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
