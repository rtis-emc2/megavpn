(() => {
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
    lastError: null,
  };

  localStorage.removeItem('megavpn.authToken');

  const appConfig = window.MegaVPNAppConfig || {};
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

  function clsStatus(status) {
    const normalized = String(status || '').toLowerCase();
    if (['ok', 'ready', 'active', 'healthy', 'succeeded', 'online', 'configured', 'enabled', 'sent', 'delivered', 'in_sync'].includes(normalized)) return 'ok';
    if (['stub', 'planned', 'draft', 'pending', 'unknown', 'maintenance', 'skipped', 'stopped'].includes(normalized)) return 'stub';
    if (['degraded', 'warning', 'retrying', 'queued', 'running', 'starting', 'bootstrapping', 'waiting heartbeat', 'awaiting heartbeat', 'provisioning', 'inactive', 'pending_apply'].includes(normalized)) return 'warn';
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
    instanceServiceOptions,
    availableServicePacks,
    servicePackByKey,
    defaultServicePack,
    servicePackOptions,
    activeLeafCertificates,
    activeManagedAuthorities,
    certificateIsExpired,
    certificateDisplayStatus,
    certificateExpiryCaption,
    certificatePrimaryLabel,
    certificateUsageCaption,
    certificateOptions,
    authorityCertificateOptions,
    nodeOptions,
    normalizeInstanceServiceCode,
    cloneJSON,
    stringValue,
    numberValue,
    slugPathPart,
    instanceServiceBlueprint,
    availableInstanceServices,
    defaultInstancePreset,
    resolveInstancePreset,
    applyInstancePresetDraft,
    finalizeInstanceDraft,
    renderInstanceServiceProfilePanel,
    applyAutoFieldValue,
    applyCreateInstanceDefaults,
    buildInstanceSpecDraft,
    renderInstanceServiceFields,
    syncInstanceServiceFields,
    buildInstanceSpecPayload,
    renderServicePackProfilePanel,
    syncCreateServicePackDefaults,
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
    renderNodeExecutionOptions,
    bindNodeOnboardingPicker,
    nodeHeartbeatStatus,
    countItems,
    bootstrapRunReason,
    defaultNodeConsoleTab,
    nodeConsoleTabButton,
    bindNodeConsoleTabs,
    switchNodeConsoleTab,
    inventoryLabel,
    diagnosticsAgentState,
    commMetricLine,
    formatNumber,
    formatBytes,
    renderInventoryFact,
    renderInventorySnapshotPanel,
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
    openModal,
    closeModal,
    openActionOutcomeModal,
    openCreateInstanceModal,
    openCreateServicePackModal,
    openInstanceManageModal,
    queueInstanceAction,
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
    runInstanceAction,
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
    openCreateNodeModal,
    openNodeControlModal,
    openEditNodeModal,
    openDeleteNodeModal,
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
    openCreateCertificateWizard,
    openCertificateActionForm,
    openManageCertificateModal,
    submitSetDefaultPlatformCertificate,
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

  function renderJobs() {
    setTitle('Jobs');
    const rows = (state.jobs || []).map((job) => ({
      id: job.id,
      type: job.type,
      scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
      created: formatDate(job.created_at),
      status: job.status || 'queued',
    }));
    el('content').innerHTML = `
      ${tableCard('Job Queue', rows, [
        { title: 'ID', key: 'id' },
        { title: 'Type', key: 'type', render: (r) => `<span class="tag">${escapeHTML(r.type)}</span>` },
        { title: 'Scope', key: 'scope' },
        { title: 'Created', key: 'created' },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
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

  async function finishCertificateAction(form, data, config) {
    await refresh();
    closeModal();
    openActionOutcomeModal(config.title, config.eyebrow, 'succeeded', config.message(data), config.details(data));
    setSubmitBusy(form, false);
  }

  function failCertificateAction(form, err, config) {
    closeModal();
    openActionOutcomeModal(config.title, config.eyebrow, 'failed', err.message || 'Operation failed', config.errorDetails ? config.errorDetails(err) : []);
    setSubmitBusy(form, false);
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

  function openCreateNodeModal() {
    openModal('Add node', 'Onboarding wizard', `
      <form id="createNodeForm" class="form-grid">
        <div class="field full">
          <label>Agent setup method</label>
          <input type="hidden" name="execution_mode" value="ssh_bootstrap" />
          <div class="choice-grid node-onboarding-grid">${renderNodeExecutionOptions('ssh_bootstrap')}</div>
          <div class="field-hint">Choose how the node will become manageable. SSH bootstrap queues installation over SSH. Manual agent paths only prepare the profile and wait for the agent heartbeat.</div>
        </div>
        <div class="field"><label>Name</label><input name="name" required placeholder="edge-01" /></div>
        <div class="field"><label>Role</label><select name="role"><option value="egress">egress</option><option value="ingress">ingress</option></select></div>
        <div class="field"><label>Kind</label><select name="kind"><option value="remote">remote</option><option value="local">local</option></select></div>
        <div class="field full"><label>Address</label><input name="address" required placeholder="203.0.113.10" /></div>
        <div class="field"><label>OS family</label><input name="os_family" value="ubuntu" /></div>
        <div class="field"><label>OS version</label><input name="os_version" value="24.04" /></div>
        <div class="field"><label>Architecture</label><select name="architecture"><option value="amd64">amd64</option><option value="arm64">arm64</option></select></div>
        <div class="field full"><button class="primary-btn" type="submit">Create node</button></div>
      </form>
      <div id="createNodeResult" style="margin-top:14px"></div>`);
    const form = document.getElementById('createNodeForm');
    bindNodeOnboardingPicker(form);
    form.addEventListener('submit', createNode);
  }

  async function createNode(event) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const payload = Object.fromEntries(form.entries());
    delete payload.node_setup_choice;
    const target = document.getElementById('createNodeResult');
    target.innerHTML = '<span class="tag warn">sending</span>';
    try {
      const data = await sendJSON('/api/v1/nodes', 'POST', payload);
      await refresh();
      renderNodeCreatedStep(data);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function openEditNodeModal(nodeID) {
    const node = state.nodes.find((item) => item.id === nodeID);
    if (!node) return;
    const executionMode = node.execution_mode || 'agent_managed';
    openModal(`Edit node: ${node.name}`, 'Node profile', `
      <form id="editNodeForm" class="form-grid">
        <div class="field full">
          <label>Agent setup method</label>
          <input type="hidden" name="execution_mode" value="${escapeHTML(executionMode)}" />
          <div class="choice-grid node-onboarding-grid">${renderNodeExecutionOptions(executionMode)}</div>
          <div class="field-hint">Changing this updates the node profile only. It does not install, revoke, bootstrap or re-enroll the agent by itself.</div>
        </div>
        <div class="field"><label>Name</label><input name="name" required value="${escapeHTML(node.name || '')}" /></div>
        <div class="field"><label>Role</label><select name="role"><option value="egress"${node.role === 'egress' ? ' selected' : ''}>egress</option><option value="ingress"${node.role === 'ingress' ? ' selected' : ''}>ingress</option></select></div>
        <div class="field"><label>Kind</label><select name="kind"><option value="remote"${node.kind !== 'local' ? ' selected' : ''}>remote</option><option value="local"${node.kind === 'local' ? ' selected' : ''}>local</option></select></div>
        <div class="field full"><label>Address</label><input name="address" required value="${escapeHTML(node.address || '')}" /></div>
        <div class="field"><label>OS family</label><input name="os_family" value="${escapeHTML(node.os_family || 'linux')}" /></div>
        <div class="field"><label>OS version</label><input name="os_version" value="${escapeHTML(node.os_version || 'unknown')}" /></div>
        <div class="field"><label>Architecture</label><select name="architecture"><option value="amd64"${node.architecture !== 'arm64' ? ' selected' : ''}>amd64</option><option value="arm64"${node.architecture === 'arm64' ? ' selected' : ''}>arm64</option></select></div>
        <div class="field full inline-actions">
          <button class="primary-btn" type="submit">Save node</button>
          <button class="secondary-btn" id="cancelEditNodeBtn" type="button">Cancel</button>
        </div>
      </form>
      <div id="editNodeResult" class="form-result"></div>`);
    const form = document.getElementById('editNodeForm');
    bindNodeOnboardingPicker(form);
    form.addEventListener('submit', (event) => updateNode(event, node));
    document.getElementById('cancelEditNodeBtn').addEventListener('click', closeModal);
  }

  async function updateNode(event, node) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const payload = Object.fromEntries(form.entries());
    delete payload.node_setup_choice;
    const target = document.getElementById('editNodeResult');
    target.innerHTML = '<span class="tag warn">saving</span>';
    try {
      const previousMode = node?.execution_mode || 'agent_managed';
      const nextMode = payload.execution_mode || previousMode;
      const modeChanged = previousMode !== nextMode;
      const updated = await sendJSON(`/api/v1/nodes/${node.id}`, 'PUT', payload);
      target.innerHTML = '<span class="tag ok">node updated</span>';
      await refresh();
      if (state.page === 'nodeManage' && state.nodeManageID === node.id) {
        await loadNodeManagePageData(node.id, 'Node profile saved.');
      }
      if (modeChanged) {
        closeModal();
        openNodeExecutionModeOutcome(updated, previousMode, nextMode);
      } else {
        setTimeout(closeModal, 450);
      }
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function openNodeExecutionModeOutcome(node, previousMode, nextMode) {
    const nextLabel = nodeExecutionLabel(nextMode);
    if (nextMode === 'agent_managed') {
      openActionOutcomeModal(
        `Setup method changed: ${node.name}`,
        'Node profile updated',
        'warning',
        'No bootstrap job was queued. The control plane is waiting for an already installed or manually installed megavpn-agent to enroll and send heartbeat.',
        [
          { label: 'Previous method', value: nodeExecutionLabel(previousMode) },
          { label: 'Current method', value: nextLabel },
          { label: 'Node state', value: nodeLifecycleStatus(node) },
          { label: 'Next action', value: 'Install/start megavpn-agent manually, or switch back to SSH bootstrap and queue bootstrap from Manage.' },
        ],
      );
      return;
    }
    if (nextMode === 'ssh_bootstrap') {
      openActionOutcomeModal(
        `Setup method changed: ${node.name}`,
        'Node profile updated',
        'warning',
        'The node is now configured for SSH bootstrap, but installation starts only after SSH access is saved and Queue bootstrap is clicked in Manage.',
        [
          { label: 'Previous method', value: nodeExecutionLabel(previousMode) },
          { label: 'Current method', value: nextLabel },
          { label: 'Next action', value: 'Open Manage -> Bootstrap, save SSH access, rotate enrollment token if needed, then queue bootstrap.' },
        ],
      );
      return;
    }
    openActionOutcomeModal(
      `Setup method changed: ${node.name}`,
      'Node profile updated',
      'succeeded',
      'The node profile was updated. No runtime action was started automatically.',
      [
        { label: 'Previous method', value: nodeExecutionLabel(previousMode) },
        { label: 'Current method', value: nextLabel },
      ],
    );
  }

  function renderNodeCreatedStep(node) {
    openModal('Node created', 'Next step', `
      <section class="card">
        <div class="table-head compact-head">
          <h2>${escapeHTML(node.name || 'Node')}</h2>
          ${statusTag(node.status || 'draft')}
        </div>
        <div class="card-body">
          <div class="grid cols-2">
            <div class="fact-card"><div class="mini-label">Address</div><div class="metric-caption strong">${escapeHTML(node.address || 'n/a')}</div></div>
            <div class="fact-card"><div class="mini-label">Role</div><div class="metric-caption strong">${escapeHTML(node.role || 'egress')}</div></div>
            <div class="fact-card"><div class="mini-label">Setup method</div><div class="metric-caption strong">${escapeHTML(nodeExecutionLabel(node.execution_mode || 'agent_managed'))}</div></div>
            <div class="fact-card"><div class="mini-label">Agent</div><div class="metric-caption strong">${escapeHTML(node.agent_status || 'unknown')}</div></div>
          </div>
          <div class="modal-actions">
            <button class="primary-btn" id="configureCreatedNodeBtn" type="button">Configure agent</button>
            <button class="secondary-btn" id="addAnotherNodeBtn" type="button">Add another node</button>
            <button class="secondary-btn" id="closeCreatedNodeBtn" type="button">Close</button>
          </div>
        </div>
      </section>`);
    document.getElementById('configureCreatedNodeBtn').addEventListener('click', () => openNodeControlModal(node.id));
    document.getElementById('addAnotherNodeBtn').addEventListener('click', openCreateNodeModal);
    document.getElementById('closeCreatedNodeBtn').addEventListener('click', closeModal);
  }

  function openDeleteNodeModal(nodeID, nodeName) {
    const node = state.nodes.find((item) => item.id === nodeID) || { id: nodeID, name: nodeName };
    if (!node?.id) return;
    openModal(`Delete node: ${node.name || 'node'}`, 'Lifecycle action', `
      <section class="card">
        <h2>Retire node</h2>
        <p>This action removes the node from active operation and revokes its agent identity. The API will block deletion while active instances still exist on this node.</p>
        <div class="code-block">node_id = ${escapeHTML(node.id)}
name = ${escapeHTML(node.name || 'n/a')}
address = ${escapeHTML(node.address || 'n/a')}
status = ${escapeHTML(node.status || 'n/a')}</div>
        <div class="modal-actions">
          <button class="danger-btn" id="confirmDeleteNodeBtn" type="button">Delete node</button>
          <button class="secondary-btn" id="cancelDeleteNodeBtn" type="button">Cancel</button>
        </div>
        <div id="deleteNodeResult" class="form-result"></div>
      </section>`);
    document.getElementById('confirmDeleteNodeBtn').addEventListener('click', () => deleteNode(node.id));
    document.getElementById('cancelDeleteNodeBtn').addEventListener('click', closeModal);
  }

  async function deleteNode(nodeID) {
    const target = document.getElementById('deleteNodeResult');
    const button = document.getElementById('confirmDeleteNodeBtn');
    if (!target) return;
    target.innerHTML = '<span class="tag warn">deleting</span>';
    if (button) button.disabled = true;
    try {
      await requestJSON(`/api/v1/nodes/${nodeID}`, { method: 'DELETE' });
      target.innerHTML = '<span class="tag ok">node retired</span>';
      await refresh();
      setTimeout(closeModal, 450);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      if (button) button.disabled = false;
    }
  }

  function openNodeControlModal(nodeID) {
    closeModal();
    state.nodeManageID = nodeID;
    state.nodeManageData = null;
    setPage('nodeManage');
  }

  function renderNodeManagePage() {
    const nodeID = state.nodeManageID;
    const cachedNode = state.nodes.find((item) => item.id === nodeID) || null;
    const data = state.nodeManageData;
    const node = data?.node || cachedNode || null;
    setTitle(node ? `Node: ${node.name || 'node'}` : 'Node Management');
    if (!nodeID) {
      el('content').innerHTML = `
        <section class="card">
          <h2>Node not selected</h2>
          <p>Return to Nodes and choose a managed node.</p>
          <div class="modal-actions"><button class="primary-btn" id="nodeManageBackBtn" type="button">Back to Nodes</button></div>
        </section>`;
      document.getElementById('nodeManageBackBtn')?.addEventListener('click', () => {
        state.nodeManageID = '';
        state.nodeManageData = null;
        setPage('nodes');
      });
      return;
    }

    el('content').innerHTML = `
      <section class="node-workspace">
        <div class="node-workspace-head">
          <div>
            <div class="eyebrow">Node management</div>
            <h2>${escapeHTML(node?.name || 'Loading node...')}</h2>
            <p>${escapeHTML(node?.address || 'Loading runtime state and bootstrap controls.')}</p>
          </div>
          <div class="node-workspace-actions">
            <button class="secondary-btn" id="nodeManageBackBtn" type="button">Back to Nodes</button>
            <button class="secondary-btn" id="nodeManageRefreshBtn" type="button">Refresh</button>
            ${node ? '<button class="primary-btn" id="nodeManageEditBtn" type="button">Edit profile</button>' : ''}
          </div>
        </div>
        <div id="nodeManageBody">
          <section class="card"><div class="empty">Loading node management workspace...</div></section>
        </div>
      </section>`;

    document.getElementById('nodeManageBackBtn')?.addEventListener('click', () => {
      state.nodeManageID = '';
      state.nodeManageData = null;
      setPage('nodes');
    });
    document.getElementById('nodeManageRefreshBtn')?.addEventListener('click', () => {
      void loadNodeManagePageData(nodeID, 'Node state refreshed.');
    });
    document.getElementById('nodeManageEditBtn')?.addEventListener('click', () => openEditNodeModal(nodeID));

    if (!data || data.nodeID !== nodeID) {
      void loadNodeManagePageData(nodeID);
      return;
    }
    renderNodeControlModal(data.node, data.diag, data.methods, data.runs, data.tokens, data.flash, 'nodeManageBody');
  }

  async function loadNodeManagePageData(nodeID, flash = '') {
    const node = state.nodes.find((item) => item.id === nodeID);
    try {
      const [freshNode, diag, methods, runs, tokens] = await Promise.all([
        requestJSON(`/api/v1/nodes/${nodeID}`),
        requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`),
        requestJSON(`/api/v1/nodes/${nodeID}/access-methods`),
        requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`),
        requestJSON(`/api/v1/nodes/${nodeID}/enrollment-tokens`),
      ]);
      state.nodeManageData = {
        nodeID,
        node: diag?.node || freshNode || node,
        diag: diag || {},
        methods: arrayOrEmpty(methods),
        runs: arrayOrEmpty(runs),
        tokens: arrayOrEmpty(tokens),
        flash,
      };
      if (state.page === 'nodeManage' && state.nodeManageID === nodeID) {
        renderNodeManagePage();
      }
    } catch (err) {
      state.nodeManageData = null;
      if (state.page === 'nodeManage') {
        const body = el('nodeManageBody');
        if (body) body.innerHTML = `<section class="card"><div class="empty">Failed to load node details: ${escapeHTML(err.message)}</div></section>`;
      } else {
        el('modalBody').innerHTML = `<div class="empty">Failed to load node details: ${escapeHTML(err.message)}</div>`;
      }
    }
  }

  function renderNodeControlModal(node, diag, methods, runs, tokens, flash, targetID = 'modalBody') {
    methods = arrayOrEmpty(methods);
    runs = arrayOrEmpty(runs);
    tokens = arrayOrEmpty(tokens);
    const heartbeatStatus = diag?.heartbeat_state || nodeHeartbeatStatus(node);
    const heartbeatDrift = diag?.heartbeat_drift_seconds;
    const sshMethod = methods.find((item) => item.method === 'ssh') || null;
    const latestInventory = diag?.latest_inventory || null;
    const inventoryPayload = latestInventory?.payload || {};
    const discoverySummary = diag?.discovery_summary || { total: 0, available: 0, imported: 0, ignored: 0, by_service: {} };
    const recentDiscoveries = Array.isArray(diag?.recent_discoveries) ? diag.recent_discoveries : [];
    const agent = diag?.agent || {};
    const methodRows = methods.length
      ? methods.map((item) => `
          <tr>
            <td>${escapeHTML(item.method || 'unknown')}</td>
            <td>${escapeHTML(item.ssh_host || 'n/a')}${item.ssh_port ? `:${escapeHTML(String(item.ssh_port))}` : ''}</td>
            <td>${escapeHTML(item.ssh_user || 'n/a')}</td>
            <td>${escapeHTML(item.auth_type || 'n/a')}</td>
            <td>${statusTag(item.is_enabled ? 'enabled' : 'disabled')}</td>
          </tr>`).join('')
      : '<tr><td colspan="5"><div class="empty compact-empty">Access methods are not configured yet.</div></td></tr>';
    const runRows = runs.length
      ? runs.slice(0, 8).map((item) => `
          <tr>
            <td>${escapeHTML(item.bootstrap_mode || 'unknown')}</td>
            <td>${statusTag(item.status || 'unknown')}</td>
            <td>${formatDate(item.started_at || item.created_at)}</td>
            <td>${escapeHTML(bootstrapRunReason(item))}</td>
            <td><button class="secondary-btn bootstrap-run-view-btn" type="button" data-run-id="${escapeHTML(item.id)}" data-job-id="${escapeHTML(item.job_id || '')}">Details</button></td>
          </tr>`).join('')
      : '<tr><td colspan="5"><div class="empty compact-empty">No bootstrap runs yet.</div></td></tr>';
    const tokenRows = tokens.length
      ? tokens.slice(0, 8).map((item) => `
          <tr>
            <td><code>${escapeHTML(item.token_hint || item.token || 'n/a')}</code></td>
            <td>${statusTag(item.status || 'active')}</td>
            <td>${formatDate(item.expires_at)}</td>
            <td>${formatDate(item.used_at)}</td>
          </tr>`).join('')
      : '<tr><td colspan="4"><div class="empty compact-empty">No enrollment tokens created yet.</div></td></tr>';
    const recentDiscoveryRows = recentDiscoveries.length
      ? recentDiscoveries.map((item) => `
          <tr>
            <td>${escapeHTML(item.service_code)}</td>
            <td>${escapeHTML(item.name)}</td>
            <td>${statusTag(item.status)}</td>
            <td>${escapeHTML(item.endpoint_host || 'n/a')}${item.endpoint_port ? `:${escapeHTML(String(item.endpoint_port))}` : ''}</td>
            <td>${formatDate(item.detected_at)}</td>
          </tr>`).join('')
      : '<tr><td colspan="5"><div class="empty">Service discovery has not reported anything yet.</div></td></tr>';
    const serviceMix = Object.entries(discoverySummary.by_service || {}).length
      ? Object.entries(discoverySummary.by_service || {}).map(([code, total]) => `${code}: ${total}`).join(' · ')
      : 'none';
    const inventoryCollectedAt = inventoryPayload.collected_at || latestInventory?.created_at || null;
    const canRequeueStuckJob = String(diag?.communication_state || '') === 'job_result_stalled' && agent.last_job_claim_job_id;
    const canClearStaleRotation = String(agent.token_rotation_status || '') === 'rotating';
    const activeTab = defaultNodeConsoleTab(diag);
    const setupLabel = nodeExecutionLabel(node.execution_mode || 'unknown');
    const communicationState = diag?.communication_state || 'unknown';
    const accessStatus = sshMethod?.is_enabled ? 'configured' : 'missing';
    const publicURL = platformPublicBaseURL() || 'not configured';
    const agentNextStepPanel = String(diag?.communication_state || '') === 'awaiting_enrollment'
      ? `<div class="fact-card emphasis-card node-next-step">
          <div class="mini-label">Next step</div>
          <div class="metric-caption strong">Agent enrollment is still pending.</div>
          <p>Install/start <code>megavpn-agent</code> with an active enrollment token, or use SSH bootstrap from the Bootstrap tab.</p>
          <div class="section-actions compact-section-actions">
            <button class="primary-btn" id="openBootstrapFromAgentBtn" type="button">Open Bootstrap</button>
            <button class="secondary-btn" id="editSetupFromAgentBtn" type="button">Edit setup method</button>
          </div>
        </div>`
      : '';

    const target = el(targetID);
    if (!target) return;
    target.innerHTML = `
      <div class="node-console-summary node-manage-summary">
        <div class="fact-card emphasis-card"><div class="mini-label">Node</div><div class="metric-caption strong">${escapeHTML(node.name || 'node')}</div><div class="metric-caption">${escapeHTML(node.address || 'n/a')}</div></div>
        <div class="fact-card"><div class="mini-label">Lifecycle</div><div class="metric-caption strong">${statusTag(node.status || 'draft')}</div><div class="metric-caption">${escapeHTML(node.kind || 'remote')} · ${escapeHTML(node.role || 'egress')}</div></div>
        <div class="fact-card"><div class="mini-label">Agent</div><div class="metric-caption strong">${statusTag(diagnosticsAgentState(diag))}</div><div class="metric-caption">${escapeHTML(communicationState)}</div></div>
        <div class="fact-card"><div class="mini-label">Heartbeat</div><div class="metric-caption strong">${escapeHTML(heartbeatStatus)}</div><div class="metric-caption">${escapeHTML(heartbeatDrift == null ? formatRelativeDate(node.last_heartbeat_at) : formatDurationSeconds(heartbeatDrift))}</div></div>
        <div class="fact-card"><div class="mini-label">Bootstrap</div><div class="metric-caption strong">${statusTag(diag?.last_bootstrap?.status || 'not started')}</div><div class="metric-caption">SSH access ${escapeHTML(accessStatus)}</div></div>
        <div class="fact-card"><div class="mini-label">Inventory</div><div class="metric-caption strong">${escapeHTML(formatRelativeDate(inventoryCollectedAt))}</div><div class="metric-caption">${escapeHTML(inventoryLabel(inventoryPayload, 'os.pretty_name', `${node.os_family || 'linux'} ${node.os_version || ''}`))}</div></div>
      </div>
      ${flash ? `<div class="notice subtle-notice">${escapeHTML(flash)}</div>` : ''}
      <div class="node-console-layout">
        <nav class="node-console-nav" aria-label="Node management sections">
          ${nodeConsoleTabButton('overview', 'Overview', 'profile and actions', activeTab)}
          ${nodeConsoleTabButton('bootstrap', 'Bootstrap', 'SSH, tokens, jobs', activeTab)}
          ${nodeConsoleTabButton('agent', 'Agent channel', 'health and trust', activeTab)}
          ${nodeConsoleTabButton('inventory', 'Inventory', 'host snapshot', activeTab)}
          ${nodeConsoleTabButton('services', 'Services', 'discovery results', activeTab)}
        </nav>
        <div class="node-console-content">
          <section class="node-tab-panel${activeTab === 'overview' ? ' is-active' : ''}" data-node-panel="overview">
            <div class="node-panel-head">
              <div><div class="eyebrow">Runtime Profile</div><h2>Node Overview</h2></div>
              <div class="section-meta">${statusTag(node.status || 'draft')}</div>
            </div>
            <div class="node-manage-grid">
              <section class="section-card node-profile-card">
                <div class="section-head">
                  <div><div class="eyebrow">Profile</div><h2>Core settings</h2></div>
                </div>
                <div class="section-body">
                  <div class="node-detail-list">
                    <div><span>Name</span><strong>${escapeHTML(node.name || 'n/a')}</strong></div>
                    <div><span>Address</span><strong>${escapeHTML(node.address || 'n/a')}</strong></div>
                    <div><span>Role</span><strong>${escapeHTML(node.role || 'egress')}</strong></div>
                    <div><span>Kind</span><strong>${escapeHTML(node.kind || 'remote')}</strong></div>
                    <div><span>Setup method</span><strong>${escapeHTML(setupLabel)}</strong></div>
                    <div><span>Public control URL</span><strong>${escapeHTML(publicURL)}</strong></div>
                  </div>
                </div>
              </section>
              <section class="section-card">
                <div class="section-head">
                  <div><div class="eyebrow">Operations</div><h2>Node actions</h2></div>
                </div>
                <div class="section-body">
                  <div class="operator-action-grid">
                    <button class="operator-action" id="editNodeFromManageBtn" type="button"><strong>Edit profile</strong><span>Name, role, address, setup method.</span></button>
                    <button class="operator-action" id="refreshNodeRuntimeBtn" type="button"><strong>Refresh diagnostics</strong><span>Reload current runtime state.</span></button>
                    <button class="operator-action" id="nodeMaintenanceToggleBtn" type="button"><strong>${node.status === 'maintenance' ? 'Disable maintenance' : 'Enable maintenance'}</strong><span>Control scheduling state for this node.</span></button>
                    <button class="operator-action danger-action" id="deleteNodeFromManageBtn" type="button"><strong>Delete node</strong><span>Retire node after instances are moved or removed.</span></button>
                  </div>
                  <div id="nodeRuntimeActionResult" class="form-result"></div>
                </div>
              </section>
            </div>
          </section>
          <section class="node-tab-panel${activeTab === 'agent' ? ' is-active' : ''}" data-node-panel="agent">
          <section class="section-card">
            <div class="section-head">
              <div>
                <div class="eyebrow">Agent Diagnostics</div>
                <h2>Communication Health</h2>
              </div>
              <div class="section-meta">
                ${statusTag(diag?.communication_state || 'unknown')}
                ${statusTag(agent.token_rotation_status || 'missing')}
              </div>
            </div>
            <div class="section-body">
              ${agentNextStepPanel}
              <div class="grid cols-3">
                <div class="card"><div class="mini-label">Communication</div><div class="metric-caption">${statusTag(communicationState)}</div><div class="metric-caption">${escapeHTML(diag?.communication_hint || 'n/a')}</div></div>
                ${commMetricLine('Last job poll', agent.last_job_poll_at, 'agent/jobs/next')}
                ${commMetricLine('Last inventory sync', agent.last_inventory_sync_at, 'agent/inventory')}
                ${commMetricLine('Last discovery sync', agent.last_discovery_sync_at, 'service discovery')}
                ${commMetricLine('Last auth failure', agent.last_auth_failure_at, agent.last_auth_failure_reason || 'none')}
                ${commMetricLine('Registered', agent.registered_at, `version ${agent.agent_version || 'n/a'}`)}
              </div>
              <div class="section-divider"></div>
              <div class="section-actions">
                <button class="secondary-btn" id="retryInventorySyncBtn" type="button">Retry inventory sync</button>
                <button class="secondary-btn" id="retryDiscoverySyncBtn" type="button">Retry discovery sync</button>
                <button class="secondary-btn" id="probeNodeChannelBtn" type="button">Channel probe</button>
                <button class="secondary-btn" id="syncRoutePolicyBtn" type="button">Sync route policy</button>
                <button class="secondary-btn" id="requeueStuckNodeJobBtn" type="button"${canRequeueStuckJob ? '' : ' disabled'}>Requeue stuck job</button>
                <button class="secondary-btn" id="clearStaleRotationBtn" type="button"${canClearStaleRotation ? '' : ' disabled'}>Clear stale pending rotation</button>
              </div>
              <div id="nodeDiagnosticsActionResult" class="form-result"></div>
              <details class="details-block">
                <summary>Technical job identifiers</summary>
                <div class="code-block">claim_job_id = ${escapeHTML(agent.last_job_claim_job_id || 'n/a')}
result_job_id = ${escapeHTML(agent.last_job_result_job_id || 'n/a')}
claim_type = ${escapeHTML(agent.last_job_claim_type || 'n/a')}
result_type = ${escapeHTML(agent.last_job_result_type || 'n/a')}
result_status = ${escapeHTML(agent.last_job_result_status || 'n/a')}</div>
              </details>
            </div>
          </section>
          <section class="section-card">
            <div class="section-head">
              <div><div class="eyebrow">Trust Plane</div><h2>Agent Trust Lifecycle</h2></div>
            </div>
            <div class="section-body">
              <p>Use these actions only for controlled token rotation, re-enrollment or incident response.</p>
              <div class="section-actions">
                <button class="secondary-btn" id="rotateAgentTokenBtn" type="button">Rotate agent token</button>
                <button class="secondary-btn" id="rotateEnrollmentTokenBtn" type="button">Rotate enrollment token</button>
                <button class="danger-btn" id="revokeAgentIdentityBtn" type="button">Revoke agent identity</button>
              </div>
              <div id="nodeTrustResult" class="form-result"></div>
            </div>
          </section>
          </section>
          <section class="node-tab-panel${activeTab === 'inventory' ? ' is-active' : ''}" data-node-panel="inventory">
          ${renderInventorySnapshotPanel(latestInventory, node)}
          </section>
          <section class="node-tab-panel${activeTab === 'services' ? ' is-active' : ''}" data-node-panel="services">
          <section class="table-card compact-card">
            <div class="table-head"><h2>Discovered Services</h2><div class="table-tools"><span class="tag">${escapeHTML(String(recentDiscoveries.length))} visible</span><span class="tag">${escapeHTML(serviceMix)}</span></div></div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Service</th><th>Name</th><th>Status</th><th>Endpoint</th><th>Detected</th></tr></thead>
                <tbody>${recentDiscoveryRows}</tbody>
              </table>
            </div>
          </section>
          </section>
          <section class="node-tab-panel node-bootstrap-panel${activeTab === 'bootstrap' ? ' is-active' : ''}" data-node-panel="bootstrap">
            <div class="node-panel-head">
              <div><div class="eyebrow">Bootstrap Pipeline</div><h2>SSH Access, Tokens and Jobs</h2></div>
              <div class="section-meta">${statusTag(diag?.last_bootstrap?.status || 'not_started')}</div>
            </div>
            <div class="node-modal-grid compact-node-grid">
              <div class="stack">
                <section class="section-card">
                  <div class="section-head">
                    <div><div class="eyebrow">Access</div><h2>SSH Bootstrap Access</h2></div>
                  </div>
                  <div class="section-body">
                    <form id="sshAccessForm" class="form-grid">
                      <div class="field"><label>SSH host</label><input name="ssh_host" required value="${escapeHTML(sshMethod?.ssh_host || node.address || '')}" /></div>
                      <div class="field"><label>SSH user</label><input name="ssh_user" required value="${escapeHTML(sshMethod?.ssh_user || 'root')}" /></div>
                      <div class="field"><label>SSH port</label><input name="ssh_port" type="number" min="1" max="65535" value="${escapeHTML(String(sshMethod?.ssh_port || 22))}" /></div>
                      <div class="field"><label>Auth type</label><select name="auth_type"><option value="ssh_key"${sshMethod?.auth_type === 'ssh_key' ? ' selected' : ''}>ssh_key</option><option value="password"${sshMethod?.auth_type === 'password' ? ' selected' : ''}>password</option><option value="token"${sshMethod?.auth_type === 'token' ? ' selected' : ''}>token</option></select></div>
                      <div class="field"><label>Secret type</label><select name="secret_type"><option value="ssh_key">ssh_key</option><option value="password">password</option><option value="api_token">api_token</option><option value="opaque">opaque</option></select></div>
                      <div class="field"><label>Enabled</label><select name="is_enabled"><option value="true"${sshMethod?.is_enabled !== false ? ' selected' : ''}>true</option><option value="false"${sshMethod?.is_enabled === false ? ' selected' : ''}>false</option></select></div>
                      <div class="field full"><label>Secret value</label><textarea name="secret_value" rows="5" placeholder="${sshMethod?.secret_ref_id ? 'Leave empty to keep existing secret_ref_id.' : 'Paste SSH private key, password or token.'}"></textarea></div>
                      <div class="field full inline-actions">
                        <button class="primary-btn" type="submit">Save SSH access</button>
                        <button class="secondary-btn" type="button" id="cancelSshAccessBtn">Cancel changes</button>
                        <button class="danger-btn" type="button" id="removeSshAccessBtn">Remove SSH access</button>
                      </div>
                    </form>
                    <div id="sshAccessResult" class="form-result"></div>
                  </div>
                </section>
                <section class="table-card compact-card">
                  <div class="table-head"><h2>Access Methods</h2><span class="tag">${escapeHTML(String(methods.length))} configured</span></div>
                  <div class="table-wrap">
                    <table>
                      <thead><tr><th>Method</th><th>Endpoint</th><th>User</th><th>Auth</th><th>Status</th></tr></thead>
                      <tbody>${methodRows}</tbody>
                    </table>
                  </div>
                </section>
              </div>
              <div class="stack">
                <section class="section-card">
                  <div class="section-head">
                    <div><div class="eyebrow">Run</div><h2>Bootstrap Job</h2></div>
                  </div>
                  <div class="section-body">
                    <div class="form-grid">
                      <div class="field"><label>Bootstrap mode</label><select id="bootstrapMode"><option value="ssh_bootstrap">ssh_bootstrap</option><option value="manual_bundle">manual_bundle</option></select></div>
                      <div class="field inline-actions align-end"><button class="primary-btn" id="queueBootstrapBtn" type="button">Queue bootstrap</button><button class="secondary-btn" id="reinstallAgentBtn" type="button">Reinstall agent</button><button class="secondary-btn" id="reenrollAgentBtn" type="button">Re-enroll agent</button><button class="secondary-btn" id="refreshNodeBootstrapBtn" type="button">Refresh</button></div>
                    </div>
                    <div id="bootstrapJobResult" class="form-result"></div>
                  </div>
                </section>
                <section class="section-card">
                  <div class="section-head">
                    <div><div class="eyebrow">Enrollment</div><h2>Enrollment Tokens</h2></div>
                  </div>
                  <div class="section-body">
                    <div class="form-grid">
                      <div class="field"><label>TTL hours</label><input id="enrollmentTtlHours" type="number" min="1" max="720" value="24" /></div>
                      <div class="field inline-actions align-end"><button class="secondary-btn" id="createEnrollmentTokenBtn" type="button">Create token</button></div>
                    </div>
                    <div id="enrollmentTokenResult" class="form-result"></div>
                  </div>
                </section>
              </div>
            </div>
            <section class="table-card compact-card">
              <div class="table-head"><h2>Enrollment Tokens</h2><span class="tag">${escapeHTML(String(tokens.length))}</span></div>
              <div class="table-wrap"><table><thead><tr><th>Token</th><th>Status</th><th>Expires</th><th>Used</th></tr></thead><tbody>${tokenRows}</tbody></table></div>
            </section>
            <section class="table-card compact-card">
              <div class="table-head"><h2>Bootstrap Runs</h2><span class="tag">${escapeHTML(String(runs.length))}</span></div>
              <div class="table-wrap"><table><thead><tr><th>Mode</th><th>Status</th><th>Started</th><th>Result</th><th>Actions</th></tr></thead><tbody>${runRows}</tbody></table></div>
            </section>
          </section>
        </div>
      </div>`;

    bindNodeConsoleTabs();
    document.getElementById('sshAccessForm').addEventListener('submit', (event) => saveSSHAccess(event, node, methods));
    document.getElementById('cancelSshAccessBtn').addEventListener('click', () => reloadNodeControlModal(node.id, 'Unsaved SSH access changes discarded.'));
    document.getElementById('removeSshAccessBtn').addEventListener('click', () => removeSSHAccess(node, methods));
    document.getElementById('retryInventorySyncBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'inventory'));
    document.getElementById('retryDiscoverySyncBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'discover'));
    document.getElementById('probeNodeChannelBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'probe'));
    document.getElementById('syncRoutePolicyBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'routes'));
    document.getElementById('requeueStuckNodeJobBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'requeue'));
    document.getElementById('clearStaleRotationBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'clear_rotation'));
    document.getElementById('nodeMaintenanceToggleBtn').addEventListener('click', () => toggleNodeMaintenance(node));
    document.getElementById('createEnrollmentTokenBtn').addEventListener('click', () => createEnrollmentToken(node));
    document.getElementById('rotateEnrollmentTokenBtn').addEventListener('click', () => rotateEnrollmentToken(node));
    document.getElementById('queueBootstrapBtn').addEventListener('click', () => queueBootstrap(node));
    document.getElementById('reinstallAgentBtn').addEventListener('click', () => queueBootstrap(node, { reinstall_agent: true }));
    document.getElementById('reenrollAgentBtn').addEventListener('click', () => queueBootstrap(node, { reinstall_agent: true, force_reenroll: true }));
    document.getElementById('rotateAgentTokenBtn').addEventListener('click', () => rotateAgentToken(node));
    document.getElementById('revokeAgentIdentityBtn').addEventListener('click', () => revokeAgentIdentity(node));
    document.getElementById('refreshNodeRuntimeBtn').addEventListener('click', () => reloadNodeControlModal(node.id, 'Node runtime state refreshed.'));
    document.getElementById('refreshNodeBootstrapBtn').addEventListener('click', () => reloadNodeControlModal(node.id, 'Bootstrap state refreshed.'));
    document.getElementById('editNodeFromManageBtn').addEventListener('click', () => openEditNodeModal(node.id));
    document.getElementById('deleteNodeFromManageBtn').addEventListener('click', () => openDeleteNodeModal(node.id, node.name));
    document.getElementById('openBootstrapFromAgentBtn')?.addEventListener('click', () => switchNodeConsoleTab('bootstrap'));
    document.getElementById('editSetupFromAgentBtn')?.addEventListener('click', () => openEditNodeModal(node.id));
    document.querySelectorAll('.bootstrap-run-view-btn').forEach((button) => {
      button.addEventListener('click', () => viewBootstrapRun(node.id, button.dataset.runId, button.dataset.jobId));
    });
  }

  async function reloadNodeControlModal(nodeID, flash) {
    const [node, diag, methods, runs, tokens] = await Promise.all([
      requestJSON(`/api/v1/nodes/${nodeID}`),
      requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`),
      requestJSON(`/api/v1/nodes/${nodeID}/access-methods`),
      requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`),
      requestJSON(`/api/v1/nodes/${nodeID}/enrollment-tokens`),
    ]);
    await loadCore();
    state.nodeManageData = {
      nodeID,
      node: diag?.node || node,
      diag: diag || {},
      methods: arrayOrEmpty(methods),
      runs: arrayOrEmpty(runs),
      tokens: arrayOrEmpty(tokens),
      flash,
    };
    if (state.page === 'nodeManage' && state.nodeManageID === nodeID) {
      renderNodeManagePage();
      return;
    }
    renderNodeControlModal(diag?.node || node, diag || {}, arrayOrEmpty(methods), arrayOrEmpty(runs), arrayOrEmpty(tokens), flash);
  }

  async function saveSSHAccess(event, node, methods) {
    event.preventDefault();
    const result = document.getElementById('sshAccessResult');
    result.innerHTML = '<span class="tag warn">saving</span>';
    try {
      const form = new FormData(event.currentTarget);
      const existingSSH = methods.find((item) => item.method === 'ssh') || null;
      let secretRefID = existingSSH?.secret_ref_id || null;
      const secretValue = String(form.get('secret_value') || '').trim();
      if (secretValue) {
        const secretRef = await sendJSON('/api/v1/secret-refs', 'POST', {
          secret_type: String(form.get('secret_type') || 'ssh_key'),
          value: secretValue,
          meta: {
            node_id: node.id,
            usage: 'node_access_method',
            method: 'ssh',
          },
        });
        secretRefID = secretRef.id;
      }
      if (!secretRefID) {
        throw new Error('secret value is required for the first SSH access save');
      }
      const sshMethod = {
        id: existingSSH?.id || '',
        method: 'ssh',
        is_enabled: String(form.get('is_enabled')) === 'true',
        ssh_host: String(form.get('ssh_host') || '').trim(),
        ssh_port: Number(form.get('ssh_port') || 22),
        ssh_user: String(form.get('ssh_user') || '').trim(),
        auth_type: String(form.get('auth_type') || 'ssh_key'),
        secret_ref_id: secretRefID,
      };
      const items = methods.filter((item) => item.method !== 'ssh').map((item) => ({ ...item }));
      items.push(sshMethod);
      await sendJSON(`/api/v1/nodes/${node.id}/access-methods`, 'PUT', { items });
      await reloadNodeControlModal(node.id, 'SSH access updated.');
    } catch (err) {
      result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function removeSSHAccess(node, methods) {
    const result = document.getElementById('sshAccessResult');
    result.innerHTML = '<span class="tag warn">removing</span>';
    try {
      const items = methods.filter((item) => item.method !== 'ssh').map((item) => ({ ...item }));
      await sendJSON(`/api/v1/nodes/${node.id}/access-methods`, 'PUT', { items });
      await reloadNodeControlModal(node.id, 'SSH access removed.');
    } catch (err) {
      result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function createEnrollmentToken(node) {
    const result = document.getElementById('enrollmentTokenResult');
    result.innerHTML = '<span class="tag warn">creating</span>';
    try {
      const ttlHours = Math.max(1, Math.min(720, Number(document.getElementById('enrollmentTtlHours').value || 24)));
      const token = await requestJSON(`/api/v1/nodes/${node.id}/enrollment-token?ttl_hours=${ttlHours}`, { method: 'POST' });
      result.innerHTML = renderActionResponse(token, 'Enrollment token created');
      await reloadNodeControlModal(node.id, 'Enrollment token created.');
    } catch (err) {
      result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function rotateEnrollmentToken(node) {
    const result = document.getElementById('enrollmentTokenResult');
    result.innerHTML = '<span class="tag warn">rotating</span>';
    try {
      const ttlHours = Math.max(1, Math.min(720, Number(document.getElementById('enrollmentTtlHours').value || 24)));
      const token = await requestJSON(`/api/v1/nodes/${node.id}/enrollment-token/rotate?ttl_hours=${ttlHours}`, { method: 'POST' });
      result.innerHTML = renderActionResponse(token, 'Enrollment token rotated');
      await reloadNodeControlModal(node.id, 'Enrollment token rotated.');
    } catch (err) {
      result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function queueBootstrap(node, options = {}) {
    const result = document.getElementById('bootstrapJobResult');
    result.innerHTML = '<span class="tag warn">queueing</span>';
    try {
      const previousHeartbeat = toMillis(node.last_heartbeat_at);
      const payload = {
        bootstrap_mode: document.getElementById('bootstrapMode').value,
        reinstall_agent: Boolean(options.reinstall_agent),
        force_reenroll: Boolean(options.force_reenroll),
      };
      const data = await sendJSON(`/api/v1/nodes/${node.id}/bootstrap`, 'POST', payload);
      result.innerHTML = renderActionResponse(data, 'Node bootstrap');
      if (data?.job?.id) {
        const finalJob = await watchJob(data.job.id, result, 'node bootstrap');
        if (payload.force_reenroll && finalJob && String(finalJob.status || '').toLowerCase() === 'succeeded') {
          await waitForNodeDiagnostics(node.id, result, 're-enroll heartbeat', (diag) => {
            const heartbeatTs = toMillis(diag?.node?.last_heartbeat_at);
            const tokenState = String(diag?.agent?.token_rotation_status || '');
            return heartbeatTs > previousHeartbeat && ['online', 'degraded'].includes(String(diag?.heartbeat_state || '')) && tokenState === 'active';
          });
        }
      }
      const flash = payload.force_reenroll
        ? 'Re-enroll workflow queued. Agent state will be cleared and enrollment will happen again.'
        : payload.reinstall_agent
          ? 'Reinstall workflow queued. Agent binary and unit will be replaced on the remote host.'
          : 'Bootstrap workflow updated. Check heartbeat and agent status below.';
      await reloadNodeControlModal(node.id, flash);
    } catch (err) {
      result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function rotateAgentToken(node) {
    const target = document.getElementById('nodeTrustResult');
    target.innerHTML = '<span class="tag warn">queueing token rotation</span>';
    try {
      const baseline = Math.max(Date.now(), toMillis(node.last_heartbeat_at));
      const job = await requestJSON(`/api/v1/nodes/${node.id}/agent-token/rotate`, { method: 'POST' });
      const finalJob = await watchJob(job.id, target, 'agent token rotate');
      if (finalJob && String(finalJob.status || '').toLowerCase() === 'succeeded') {
        await waitForNodeDiagnostics(node.id, target, 'post-rotation heartbeat', (diag) => {
          const heartbeatTs = toMillis(diag?.node?.last_heartbeat_at);
          return heartbeatTs > baseline && String(diag?.agent?.token_rotation_status || '') === 'active';
        });
      }
      await reloadNodeControlModal(node.id, 'Agent token rotation finished.');
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function revokeAgentIdentity(node) {
    const target = document.getElementById('nodeTrustResult');
    target.innerHTML = '<span class="tag warn">revoking identity</span>';
    try {
      const data = await requestJSON(`/api/v1/nodes/${node.id}/agent-identity/revoke`, { method: 'POST' });
      target.innerHTML = renderActionResponse(data, 'Agent identity revoked');
      await reloadNodeControlModal(node.id, 'Agent identity revoked. The node now requires a new enrollment/bootstrap path.');
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function viewBootstrapRun(nodeID, runID, jobID) {
    openModal(`Bootstrap run: ${runID}`, 'Node bootstrap result', '<div class="empty">Loading bootstrap run details...</div>');
    try {
      const runs = await requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`);
      const run = (runs || []).find((item) => item.id === runID);
      if (!run) {
        el('modalBody').innerHTML = '<div class="empty">Bootstrap run not found.</div>';
        return;
      }
      let logs = [];
      if (jobID) {
        logs = await fetchJSON(`/api/v1/jobs/${jobID}/logs?limit=50`, []);
      }
      const canRevealManualBundle = run.bootstrap_mode === 'manual_bundle' && run.result_payload?.agent_bootstrapenv_available;
      const logLines = (logs || []).map((entry) => `${formatDate(entry.created_at)} [${String(entry.level || 'info').toUpperCase()}] ${entry.message}`).join('\n');
      el('modalBody').innerHTML = `
        <div class="grid cols-2">
          <div class="card"><div class="mini-label">Mode</div><div class="metric-caption">${escapeHTML(run.bootstrap_mode || 'n/a')}</div></div>
          <div class="card"><div class="mini-label">Status</div><div class="metric-caption">${statusTag(run.status || 'unknown')}</div></div>
        </div>
        <div class="card">
          <h2>Request payload</h2>
          <div class="code-block">${escapeHTML(JSON.stringify(run.request_payload || {}, null, 2))}</div>
        </div>
        <div class="card">
          <h2>Result payload</h2>
          ${renderActionResponse(run.result_payload || {}, 'Bootstrap result')}
        </div>
        ${canRevealManualBundle ? `
        <div class="card">
          <div class="table-head compact-head"><h2>Manual bundle</h2><button class="secondary-btn" type="button" id="manualBundleRevealBtn">Reveal bundle</button></div>
          <div id="manualBundleRevealResult" class="form-result"></div>
        </div>` : ''}
        <div class="card">
          <h2>Worker logs</h2>
          <div class="code-block">${escapeHTML(logLines || 'No logs captured for this bootstrap job yet.')}</div>
        </div>`;
      document.getElementById('manualBundleRevealBtn')?.addEventListener('click', () => revealManualBootstrapBundle(nodeID, runID));
    } catch (err) {
      el('modalBody').innerHTML = `<div class="empty">Failed to load bootstrap run details: ${escapeHTML(err.message)}</div>`;
    }
  }

  async function revealManualBootstrapBundle(nodeID, runID) {
    const target = document.getElementById('manualBundleRevealResult');
    if (!target) return;
    target.innerHTML = '<span class="tag warn">revealing bundle</span>';
    try {
      const bundle = await requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs/${runID}/bundle`);
      target.innerHTML = `
        <h3>agent.env</h3>
        <div class="code-block">${escapeHTML(bundle.agent_env || '')}</div>
        <h3>agent-bootstrap.env</h3>
        <div class="code-block">${escapeHTML(bundle.agent_bootstrapenv || '')}</div>`;
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function queueNodeRuntimeJob(node, action, targetID = 'nodeRuntimeActionResult', customPath = '') {
    const target = document.getElementById(targetID);
    target.innerHTML = `<span class="tag warn">queueing ${escapeHTML(action)}</span>`;
    try {
      const path = customPath || (action === 'discover'
        ? `/api/v1/nodes/${node.id}/services/discover`
        : `/api/v1/nodes/${node.id}/inventory/sync`);
      const data = await requestJSON(path, { method: 'POST' });
      const job = data?.job || data;
      await watchJob(job.id, target, `node ${action}`);
      await reloadNodeControlModal(node.id, `${action} job updated runtime state.`);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function runNodeDiagnosticsAction(node, action) {
    const target = document.getElementById('nodeDiagnosticsActionResult');
    const actions = {
      inventory: {
        label: 'inventory retry',
        path: `/api/v1/nodes/${node.id}/diagnostics/retry-inventory`,
        flash: 'Inventory retry queued from diagnostics.',
      },
      discover: {
        label: 'discovery retry',
        path: `/api/v1/nodes/${node.id}/diagnostics/retry-discovery`,
        flash: 'Discovery retry queued from diagnostics.',
      },
      probe: {
        label: 'channel probe',
        path: `/api/v1/nodes/${node.id}/diagnostics/channel-probe`,
        flash: 'Channel probe finished.',
      },
      routes: {
        label: 'route policy sync',
        path: `/api/v1/nodes/${node.id}/routes/apply`,
        flash: 'Route policy snapshot applied.',
      },
      requeue: {
        label: 'stuck job requeue',
        path: `/api/v1/nodes/${node.id}/diagnostics/requeue-stuck-job`,
        flash: 'Stale claimed job was requeued.',
      },
      clear_rotation: {
        label: 'clear stale rotation',
        path: `/api/v1/nodes/${node.id}/diagnostics/clear-stale-rotation`,
        flash: 'Stale pending rotation was cleared.',
      },
    };
    const cfg = actions[action];
    if (!cfg) return;

    target.innerHTML = `<span class="tag warn">running ${escapeHTML(cfg.label)}</span>`;
    try {
      const data = await requestJSON(cfg.path, { method: 'POST' });
      const job = data?.job || null;
      if (job?.id) {
        await watchJob(job.id, target, cfg.label);
      } else {
        target.innerHTML = renderActionResponse(data, cfg.label);
      }
      await reloadNodeControlModal(node.id, cfg.flash);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function toggleNodeMaintenance(node) {
    const target = document.getElementById('nodeRuntimeActionResult');
    target.innerHTML = '<span class="tag warn">updating maintenance</span>';
    try {
      const path = node.status === 'maintenance'
        ? `/api/v1/nodes/${node.id}/maintenance/disable`
        : `/api/v1/nodes/${node.id}/maintenance/enable`;
      const data = await requestJSON(path, { method: 'POST' });
      target.innerHTML = renderActionResponse(data, 'Node maintenance');
      await reloadNodeControlModal(node.id, 'Node maintenance state updated.');
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
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

  function openCreateServicePackModal() {
    const initialPack = defaultServicePack();
    openModal('Create service pack', 'POST /api/v1/service-packs/{key}/instances', `
      <form id="createServicePackForm" class="form-grid">
        <div class="field"><label>Node</label><select name="node_id" required>${nodeOptions()}</select></div>
        <div class="field"><label>Service pack</label><select name="service_pack_key" required>${servicePackOptions(initialPack?.key || '')}</select></div>
        <div class="field"><label>Base name</label><input name="base_name" required placeholder="${escapeHTML(initialPack?.base_name_template || 'edge-service-pack')}" /></div>
        <div class="field"><label>Endpoint host</label><input name="endpoint_host" placeholder="${escapeHTML(initialPack?.endpoint_hint || 'edge.example.com')}" /></div>
        <div class="field"><label>Managed certificate</label><select name="certificate_id">${certificateOptions('', true)}</select></div>
        <div id="servicePackFields" class="form-grid full"></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Create service pack</button></div>
      </form>
      <div id="createServicePackResult" class="form-result"></div>`, { wide: true });
    const form = document.getElementById('createServicePackForm');
    const packSelect = form.querySelector('select[name="service_pack_key"]');
    syncCreateServicePackDefaults(form, packSelect.value);
    packSelect.addEventListener('change', () => syncCreateServicePackDefaults(form, packSelect.value));
    form.addEventListener('submit', createServicePack);
  }

  async function createServicePack(event) {
    event.preventDefault();
    const target = document.getElementById('createServicePackResult');
    target.innerHTML = '<span class="tag warn">creating</span>';
    try {
      const form = new FormData(event.currentTarget);
      const packKey = String(form.get('service_pack_key') || '').trim();
      const payload = {
        node_id: String(form.get('node_id') || '').trim(),
        base_name: String(form.get('base_name') || '').trim(),
        endpoint_host: String(form.get('endpoint_host') || '').trim(),
        certificate_id: String(form.get('certificate_id') || '').trim(),
      };
      const data = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}/instances`, 'POST', payload);
      target.innerHTML = renderActionResponse(data, 'Service pack creation');
      await refresh();
      setTimeout(closeModal, 500);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function bindCertificateWizardPicker(form) {
    const options = Array.from(form.querySelectorAll('.certificate-action-option'));
    const activate = (action) => {
      options.forEach((option) => {
        const active = option.dataset.action === action;
        option.classList.toggle('is-selected', active);
        const radio = option.querySelector('input[type="radio"]');
        if (radio) radio.checked = active;
      });
    };
    options.forEach((option) => {
      option.addEventListener('click', () => {
        if (option.classList.contains('is-disabled')) return;
        activate(option.dataset.action);
      });
      option.querySelector('input[type="radio"]')?.addEventListener('change', () => activate(option.dataset.action));
    });
    activate(form.querySelector('input[name="certificate_action"]:checked')?.value || 'import');
  }

  function openCreateCertificateWizard() {
    const canIssueFromCA = activeManagedAuthorities().length > 0;
    openModal('Add certificate', 'Certificates / Add', `
      <form id="certificateWizardForm" class="certificate-wizard">
        <div class="certificate-wizard-head">
          <div>
            <div class="eyebrow">Step 1 of 2</div>
            <h2>Add certificate</h2>
          </div>
        </div>
        <div class="choice-grid certificate-choice-grid">
          <label class="choice-card certificate-action-option" data-action="import">
            <input type="radio" name="certificate_action" value="import" checked />
            <span>
              <strong>Import certificate</strong>
              <em>Certificate, private key and optional chain files.</em>
            </span>
          </label>
          <label class="choice-card certificate-action-option" data-action="self_signed">
            <input type="radio" name="certificate_action" value="self_signed" />
            <span>
              <strong>Create self-signed certificate</strong>
              <em>Internal fallback certificate.</em>
            </span>
          </label>
          <label class="choice-card certificate-action-option ${canIssueFromCA ? '' : 'is-disabled'}" data-action="issue_from_ca">
            <input type="radio" name="certificate_action" value="issue_from_ca"${canIssueFromCA ? '' : ' disabled'} />
            <span>
              <strong>Issue from internal CA</strong>
              <em>${canIssueFromCA ? 'Use managed CA as issuer.' : 'Create a managed CA first.'}</em>
            </span>
          </label>
        </div>
        <details class="details-block certificate-advanced-actions">
          <summary>CA operations</summary>
          <div class="certificate-action-row">
            <button class="secondary-btn" type="button" data-certificate-action="managed_ca">Create managed CA</button>
            <button class="secondary-btn" type="button" data-certificate-action="service_ca_root">Create service CA root</button>
            <button class="secondary-btn" type="button" data-certificate-action="letsencrypt">Let's Encrypt status</button>
          </div>
        </details>
        <div class="modal-actions">
          <button class="secondary-btn" type="button" id="cancelCertificateWizardBtn">Cancel</button>
          <button class="primary-btn" type="submit">Next</button>
        </div>
      </form>`, { wide: true });
    const form = document.getElementById('certificateWizardForm');
    bindCertificateWizardPicker(form);
    form.addEventListener('submit', (event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget);
      openCertificateActionForm(String(data.get('certificate_action') || 'import'));
    });
    document.getElementById('cancelCertificateWizardBtn')?.addEventListener('click', closeModal);
    document.querySelectorAll('[data-certificate-action]').forEach((button) => {
      button.addEventListener('click', () => openCertificateActionForm(button.dataset.certificateAction));
    });
  }

  function openCertificateActionForm(action, options = {}) {
    switch (action) {
      case 'import':
        openModal('Import certificate', 'Certificates / Import', `
          <form id="importCertificateForm" class="form-grid certificate-import-form">
            <div class="field"><label>Name</label><input name="name" placeholder="Auto-filled from certificate CN" /></div>
            <div class="field"><label>Description</label><input name="description" placeholder="Optional" /></div>
            <div class="field full file-field">
              <label>Certificate file</label>
              <input name="certificate_file" type="file" accept=".pem,.crt,.cer,.cert,.txt" required />
            </div>
            <div class="field full file-field">
              <label>Private key file</label>
              <input name="private_key_file" type="file" accept=".pem,.key,.txt" required />
            </div>
            <div class="field full file-field">
              <label>CA chain file</label>
              <input name="chain_file" type="file" accept=".pem,.crt,.cer,.chain,.txt" />
            </div>
            <div class="field full">
              <div id="certificateImportPreview" class="certificate-import-preview empty">Select certificate and private key files.</div>
            </div>
            <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Import certificate</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        {
          const form = document.getElementById('importCertificateForm');
          bindCertificateImportFilePreview(form);
          form.addEventListener('submit', importCertificateSubmit);
        }
        return;
      case 'self_signed':
        openModal('Create self-signed certificate', 'Certificates / Self-signed', `
          <form id="selfSignedCertificateForm" class="form-grid">
            <div class="field"><label>Name</label><input name="name" required placeholder="edge-selfsigned" /></div>
            <div class="field"><label>Description</label><input name="description" placeholder="Internal edge certificate" /></div>
            <div class="field"><label>Common Name</label><input name="common_name" required placeholder="edge.example.com" /></div>
            <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="3650" value="365" /></div>
            <div class="field full"><label>DNS names / SAN</label><input name="dns_names" placeholder="edge.example.com, *.example.com" /></div>
            <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Create self-signed certificate</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        document.getElementById('selfSignedCertificateForm').addEventListener('submit', createSelfSignedCertificateSubmit);
        return;
      case 'managed_ca':
        openModal('Create managed CA', 'Certificates / Managed CA', `
          <form id="managedCAForm" class="form-grid">
            <div class="field"><label>Name</label><input name="name" required placeholder="RTIS Internal Edge CA" /></div>
            <div class="field"><label>Description</label><input name="description" placeholder="Managed internal CA for edge certificates" /></div>
            <div class="field"><label>Common Name</label><input name="common_name" required placeholder="RTIS Internal Edge CA" /></div>
            <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="10950" value="3650" /></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Create managed CA</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        document.getElementById('managedCAForm').addEventListener('submit', createManagedCASubmit);
        return;
      case 'issue_from_ca':
        openModal('Issue certificate from managed CA', 'Certificates / Issue from CA', `
          <form id="issueFromCAForm" class="form-grid">
            <div class="field"><label>Authority</label><select name="authority_certificate_id" required>${authorityCertificateOptions(options.authorityCertificateID || '')}</select></div>
            <div class="field"><label>Name</label><input name="name" required placeholder="edge-issued" /></div>
            <div class="field"><label>Description</label><input name="description" placeholder="Issued from managed CA" /></div>
            <div class="field"><label>Common Name</label><input name="common_name" required placeholder="edge.example.com" /></div>
            <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="3650" value="365" /></div>
            <div class="field full"><label>DNS names / SAN</label><input name="dns_names" placeholder="edge.example.com, *.example.com" /></div>
            <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Issue certificate</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        document.getElementById('issueFromCAForm').addEventListener('submit', issueCertificateFromCASubmit);
        return;
      case 'service_ca_root':
        openModal('Create service CA root', 'Certificates / Service CA', `
          <form id="serviceCARootForm" class="form-grid">
            <div class="field"><label>Service code</label><input name="service_code" value="openvpn" required placeholder="openvpn" /></div>
            <div class="field"><label>PKI profile</label><input name="pki_profile" value="default" placeholder="default" /></div>
            <div class="field"><label>Common Name</label><input name="common_name" required placeholder="RTIS OpenVPN Platform CA" /></div>
            <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="10950" value="3650" /></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Create service CA root</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        document.getElementById('serviceCARootForm').addEventListener('submit', createServiceCARootSubmit);
        return;
      case 'letsencrypt':
        openModal('Let\'s Encrypt', 'Certificates / ACME', `
          <div class="card">
            <h3>ACME is paused for this release line</h3>
            <p>The operator flow stays visible, but backend issuance is intentionally blocked until we resume ACME work and approve the canonical challenge strategy for this product: HTTP-01, DNS-01, or delegated external ACME.</p>
          </div>`, { wide: true });
        return;
      default:
        openCreateCertificateWizard();
    }
  }

  function parseCSVList(value) {
    return String(value || '')
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
  }

  function openManageCertificateModal(certificateID) {
    const item = (state.platformCertificates || []).find((cert) => cert.id === certificateID);
    if (!item) return;
    const status = certificateDisplayStatus(item);
    const isLeaf = item.kind === 'leaf';
    const isCA = item.kind === 'ca';
    const canSetDefault = isLeaf && status === 'active' && !item.is_default && item.key_secret_ref_id;
    const canRevoke = isLeaf && status === 'active';
    const canDeleteCA = isCA && status === 'active';
    const canIssue = isCA && status === 'active';
    openModal(`Manage certificate: ${certificatePrimaryLabel(item)}`, 'Certificates / Manage', `
      <div class="certificate-manage-layout">
        <section class="card">
          <div class="mini-label">Certificate</div>
          <h2>${escapeHTML(certificatePrimaryLabel(item))}</h2>
          <p>${escapeHTML(item.description || 'No description provided.')}</p>
          <div class="inline-actions">
            ${statusTag(status)}
            <span class="tag">${escapeHTML(item.kind || 'certificate')}</span>
            <span class="tag">${escapeHTML(item.source || 'unknown')}</span>
            ${item.is_default ? '<span class="tag ok">default</span>' : ''}
          </div>
        </section>
        <section class="card">
          <div class="mini-label">Lifecycle</div>
          <div class="response-grid">
            <div class="response-fact"><span>Common Name</span><strong>${escapeHTML(item.common_name || 'n/a')}</strong></div>
            <div class="response-fact"><span>Issuer</span><strong>${escapeHTML(item.issuer_name || 'self')}</strong></div>
            <div class="response-fact"><span>Expires</span><strong>${escapeHTML(certificateExpiryCaption(item))}</strong></div>
            <div class="response-fact"><span>Usage</span><strong>${escapeHTML(isLeaf ? certificateUsageCaption(item.id) : 'signing authority')}</strong></div>
          </div>
        </section>
        <section class="card">
          <div class="mini-label">SAN / DNS names</div>
          <div class="chip-list">
            ${Array.isArray(item.sans) && item.sans.length ? item.sans.map((name) => `<span class="chip">${escapeHTML(name)}</span>`).join('') : '<span class="metric-caption">No SAN records.</span>'}
          </div>
        </section>
        <section class="card">
          <div class="mini-label">Operational model</div>
          <p>${isLeaf
            ? 'Leaf certificates can be assigned to edge TLS services and Xray/Nginx instances. Revoke only when no production binding depends on it.'
            : 'Managed CA can issue internal leaf certificates. Delete CA is cascade and marks its issued children as deleted.'}</p>
          <div class="code-block">certificate_id = ${escapeHTML(item.id)}
cert_secret_ref = ${escapeHTML(item.cert_secret_ref_id || 'n/a')}
key_secret_ref = ${escapeHTML(item.key_secret_ref_id || 'n/a')}</div>
        </section>
      </div>
      <div class="modal-actions">
        <button class="secondary-btn" id="closeCertificateManageBtn" type="button">Close</button>
        ${canIssue ? '<button class="secondary-btn" id="issueFromSelectedCABtn" type="button">Issue certificate</button>' : ''}
        ${canSetDefault ? '<button class="primary-btn" id="setDefaultCertificateBtn" type="button">Set as default</button>' : ''}
        ${canRevoke ? '<button class="danger-btn" id="revokeManagedCertificateBtn" type="button">Revoke</button>' : ''}
        ${canDeleteCA ? '<button class="danger-btn" id="deleteManagedCABtn" type="button">Delete CA</button>' : ''}
      </div>`, { wide: true });
    document.getElementById('closeCertificateManageBtn')?.addEventListener('click', closeModal);
    document.getElementById('issueFromSelectedCABtn')?.addEventListener('click', () => openCertificateActionForm('issue_from_ca', { authorityCertificateID: item.id }));
    document.getElementById('setDefaultCertificateBtn')?.addEventListener('click', (event) => submitSetDefaultPlatformCertificate(item.id, event.currentTarget));
    document.getElementById('revokeManagedCertificateBtn')?.addEventListener('click', () => openRevokeCertificateModal(item.id, certificatePrimaryLabel(item)));
    document.getElementById('deleteManagedCABtn')?.addEventListener('click', () => openDeleteCAModal(item.id, certificatePrimaryLabel(item)));
  }

  async function submitSetDefaultPlatformCertificate(certificateID, button) {
    if (button) {
      button.disabled = true;
      button.textContent = 'Setting default...';
    }
    const item = (state.platformCertificates || []).find((cert) => cert.id === certificateID);
    try {
      const data = await sendJSON(`/api/v1/platform/certificates/${encodeURIComponent(certificateID)}/default`, 'POST', {});
      await refresh();
      closeModal();
      openActionOutcomeModal('Default certificate updated', 'Certificates / Success', 'succeeded', `Certificate ${certificatePrimaryLabel(item)} is now default.`, [
        { label: 'Certificate', value: certificatePrimaryLabel(item) },
        { label: 'Status', value: data.status || 'default' },
      ]);
    } catch (err) {
      closeModal();
      openActionOutcomeModal('Default certificate failed', 'Certificates / Error', 'failed', err.message || 'Set default certificate failed', [
        { label: 'Certificate', value: certificatePrimaryLabel(item) },
        { label: 'Action', value: 'Set default certificate' },
      ]);
    }
  }

  function openRevokeCertificateModal(certificateID, certificateName) {
    openModal('Revoke certificate', 'Certificates / Leaf revoke', `
      <div class="form-grid">
        <div class="field full">
          <div class="code-block">
            <div><strong>${escapeHTML(certificateName || certificateID || 'certificate')}</strong></div>
            <div class="metric-caption" style="margin-top:6px">После revoke сертификат станет неактивным, исчезнет из выбора и больше не сможет использоваться в новых apply / materialize операциях.</div>
          </div>
        </div>
        <div class="field full inline-actions">
          <button class="secondary-btn" id="cancelRevokeCertificateBtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmRevokeCertificateBtn" type="button">Revoke certificate</button>
        </div>
      </div>`);
    const cancelBtn = document.getElementById('cancelRevokeCertificateBtn');
    const confirmBtn = document.getElementById('confirmRevokeCertificateBtn');
    if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
    if (confirmBtn) confirmBtn.addEventListener('click', () => submitRevokePlatformCertificate(certificateID, certificateName, confirmBtn));
  }

  function openDeleteCAModal(certificateID, certificateName) {
    openModal('Delete managed CA', 'Certificates / CA delete', `
      <div class="form-grid">
        <div class="field full">
          <div class="code-block">
            <div><strong>${escapeHTML(certificateName || certificateID || 'managed CA')}</strong></div>
            <div class="metric-caption" style="margin-top:6px">Удаление CA каскадно пометит как deleted все сертификаты, которые были ею подписаны. После этого такие сертификаты больше нельзя будет использовать.</div>
          </div>
        </div>
        <div class="field full inline-actions">
          <button class="secondary-btn" id="cancelDeleteCABtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmDeleteCABtn" type="button">Delete CA</button>
        </div>
      </div>`);
    const cancelBtn = document.getElementById('cancelDeleteCABtn');
    const confirmBtn = document.getElementById('confirmDeleteCABtn');
    if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
    if (confirmBtn) confirmBtn.addEventListener('click', () => submitDeletePlatformCA(certificateID, certificateName, confirmBtn));
  }

  async function submitRevokePlatformCertificate(certificateID, certificateName, button) {
    if (button) {
      button.disabled = true;
      button.textContent = 'Revoking...';
    }
    try {
      const data = await sendJSON(`/api/v1/platform/certificates/${encodeURIComponent(certificateID)}/revoke`, 'POST', {});
      await refresh();
      closeModal();
      openActionOutcomeModal('Certificate revoked', 'Certificates / Success', 'succeeded', `Certificate ${certificateName || certificateID} was revoked successfully.`, [
        { label: 'Certificate', value: certificateName || certificateID },
        { label: 'Status', value: data.status || 'revoked' },
      ]);
    } catch (err) {
      closeModal();
      openActionOutcomeModal('Certificate revoke failed', 'Certificates / Error', 'failed', err.message || 'Certificate revoke failed', [
        { label: 'Certificate', value: certificateName || certificateID },
        { label: 'Action', value: 'Revoke leaf certificate' },
      ]);
    }
  }

  async function submitDeletePlatformCA(certificateID, certificateName, button) {
    if (button) {
      button.disabled = true;
      button.textContent = 'Deleting...';
    }
    try {
      const data = await requestJSON(`/api/v1/platform/certificates/${encodeURIComponent(certificateID)}`, { method: 'DELETE' });
      await refresh();
      closeModal();
      openActionOutcomeModal('Managed CA deleted', 'Certificates / Success', 'succeeded', `Managed CA ${certificateName || certificateID} was deleted with cascade.`, [
        { label: 'CA', value: certificateName || certificateID },
        { label: 'Status', value: data.status || 'deleted' },
        { label: 'Cascade count', value: data.cascade_count || 0 },
      ]);
    } catch (err) {
      closeModal();
      openActionOutcomeModal('Managed CA delete failed', 'Certificates / Error', 'failed', err.message || 'Managed CA delete failed', [
        { label: 'CA', value: certificateName || certificateID },
        { label: 'Action', value: 'Delete CA with cascade' },
      ]);
    }
  }

  function selectedFile(formEl, name) {
    const input = formEl.querySelector(`input[name="${name}"]`);
    return input?.files?.[0] || null;
  }

  function readFileAsText(file) {
    if (!file) return Promise.resolve('');
    if (typeof file.text === 'function') return file.text();
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = () => reject(reader.error || new Error('file read failed'));
      reader.readAsText(file);
    });
  }

  async function certificateImportPayloadFromForm(formEl, requireFiles = true) {
    const form = new FormData(formEl);
    const certFile = selectedFile(formEl, 'certificate_file');
    const keyFile = selectedFile(formEl, 'private_key_file');
    const chainFile = selectedFile(formEl, 'chain_file');
    if (requireFiles && !certFile) throw new Error('Select certificate file');
    if (requireFiles && !keyFile) throw new Error('Select private key file');
    return {
      name: String(form.get('name') || '').trim(),
      description: String(form.get('description') || '').trim(),
      certificate: String(await readFileAsText(certFile)).trim(),
      private_key: String(await readFileAsText(keyFile)).trim(),
      chain: String(await readFileAsText(chainFile)).trim(),
      is_default: String(form.get('is_default') || '') === '1',
    };
  }

  let certificateImportPreviewSeq = 0;

  function bindCertificateImportFilePreview(formEl) {
    if (!formEl) return;
    const update = () => refreshCertificateImportPreview(formEl);
    formEl.querySelectorAll('input[type="file"]').forEach((input) => input.addEventListener('change', update));
    formEl.querySelector('input[name="name"]')?.addEventListener('input', () => {
      formEl.dataset.nameEdited = '1';
    });
    update();
  }

  async function refreshCertificateImportPreview(formEl) {
    const target = document.getElementById('certificateImportPreview');
    if (!target) return;
    const certFile = selectedFile(formEl, 'certificate_file');
    const keyFile = selectedFile(formEl, 'private_key_file');
    const chainFile = selectedFile(formEl, 'chain_file');
    if (!certFile && !keyFile && !chainFile) {
      target.className = 'certificate-import-preview empty';
      target.textContent = 'Select certificate and private key files.';
      return;
    }
    if (!certFile || !keyFile) {
      target.className = 'certificate-import-preview empty';
      target.textContent = 'Certificate and private key files are required.';
      return;
    }

    const seq = ++certificateImportPreviewSeq;
    target.className = 'certificate-import-preview';
    target.innerHTML = '<span class="tag warn">checking</span>';
    try {
      const payload = await certificateImportPayloadFromForm(formEl, false);
      if (!payload.certificate || !payload.private_key) {
        target.className = 'certificate-import-preview empty';
        target.textContent = 'Certificate and private key files are required.';
        return;
      }
      const preview = await sendJSON('/api/v1/platform/certificates/preview', 'POST', payload);
      if (seq !== certificateImportPreviewSeq) return;
      const nameInput = formEl.querySelector('input[name="name"]');
      if (nameInput && !nameInput.value.trim() && formEl.dataset.nameEdited !== '1' && preview.common_name) {
        nameInput.value = preview.common_name;
      }
      target.className = 'certificate-import-preview';
      target.innerHTML = renderCertificateImportPreview(preview, { certFile, keyFile, chainFile });
    } catch (err) {
      if (seq !== certificateImportPreviewSeq) return;
      target.className = 'certificate-import-preview error';
      target.innerHTML = `
        <div class="inline-actions"><span class="tag danger">invalid</span></div>
        <div class="metric-caption strong">${escapeHTML(err.message || 'Certificate preview failed')}</div>`;
    }
  }

  function renderCertificateImportPreview(preview, files) {
    const sans = Array.isArray(preview.sans) ? preview.sans : [];
    return `
      <div class="inline-actions"><span class="tag ok">valid</span><span class="tag">${escapeHTML(preview.private_key_type || 'key')}</span>${preview.key_pair_valid ? '<span class="tag ok">key matches</span>' : '<span class="tag danger">key mismatch</span>'}</div>
      <div class="response-grid certificate-preview-grid">
        <div class="response-fact"><span>Common Name</span><strong>${escapeHTML(preview.common_name || 'n/a')}</strong></div>
        <div class="response-fact"><span>Issuer</span><strong>${escapeHTML(preview.issuer_name || 'self')}</strong></div>
        <div class="response-fact"><span>Expires</span><strong>${escapeHTML(certificateExpiryCaption({ not_after: preview.not_after }))}</strong></div>
        <div class="response-fact"><span>Chain</span><strong>${escapeHTML(String(preview.chain_certificate_count || 0))} certificates</strong></div>
      </div>
      <div class="chip-list certificate-file-list">
        <span class="chip">${escapeHTML(files.certFile?.name || 'certificate')}</span>
        <span class="chip">${escapeHTML(files.keyFile?.name || 'private key')}</span>
        ${files.chainFile ? `<span class="chip">${escapeHTML(files.chainFile.name)}</span>` : ''}
      </div>
      <div class="chip-list">
        ${sans.length ? sans.map((name) => `<span class="chip">${escapeHTML(name)}</span>`).join('') : '<span class="metric-caption">No SAN records.</span>'}
      </div>`;
  }

  async function importCertificateSubmit(event) {
    event.preventDefault();
    const formEl = event.currentTarget;
    setSubmitBusy(formEl, true, 'Importing...');
    try {
      const payload = await certificateImportPayloadFromForm(formEl, true);
      const data = await sendJSON('/api/v1/platform/certificates/import', 'POST', payload);
      await finishCertificateAction(formEl, data, {
        title: 'Certificate imported',
        eyebrow: 'Certificates / Success',
        message: (item) => `Certificate ${item.name || item.common_name || item.id} was imported successfully.`,
        details: (item) => [
          { label: 'Name', value: item.name || item.common_name || item.id },
          { label: 'Common Name', value: item.common_name || 'n/a' },
          { label: 'Source', value: item.source || 'imported' },
          { label: 'Expires', value: certificateExpiryCaption(item) },
        ],
        errorDetails: () => [{ label: 'Action', value: 'Import certificate' }],
      });
    } catch (err) {
      failCertificateAction(formEl, err, {
        title: 'Certificate import failed',
        eyebrow: 'Certificates / Error',
        errorDetails: () => [{ label: 'Action', value: 'Import certificate' }],
      });
    }
  }

  async function createSelfSignedCertificateSubmit(event) {
    event.preventDefault();
    const formEl = event.currentTarget;
    setSubmitBusy(formEl, true, 'Creating...');
    try {
      const form = new FormData(formEl);
      const payload = {
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
        dns_names: parseCSVList(form.get('dns_names')),
        valid_days: Number(form.get('valid_days') || 365),
        is_default: String(form.get('is_default') || '') === '1',
      };
      const data = await sendJSON('/api/v1/platform/certificates/self-signed', 'POST', payload);
      await finishCertificateAction(formEl, data, {
        title: 'Self-signed certificate created',
        eyebrow: 'Certificates / Success',
        message: (item) => `Certificate ${item.name || item.common_name || item.id} was created successfully.`,
        details: (item) => [
          { label: 'Name', value: item.name || item.common_name || item.id },
          { label: 'Common Name', value: item.common_name || 'n/a' },
          { label: 'Valid until', value: certificateExpiryCaption(item) },
          { label: 'Default', value: item.is_default ? 'yes' : 'no' },
        ],
      });
    } catch (err) {
      failCertificateAction(formEl, err, {
        title: 'Certificate creation failed',
        eyebrow: 'Certificates / Error',
        errorDetails: () => [{ label: 'Action', value: 'Create self-signed certificate' }],
      });
    }
  }

  async function createManagedCASubmit(event) {
    event.preventDefault();
    const formEl = event.currentTarget;
    setSubmitBusy(formEl, true, 'Creating CA...');
    try {
      const form = new FormData(formEl);
      const payload = {
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
        valid_days: Number(form.get('valid_days') || 3650),
      };
      const data = await sendJSON('/api/v1/platform/certificates/authorities', 'POST', payload);
      await finishCertificateAction(formEl, data, {
        title: 'Managed CA created',
        eyebrow: 'Certificates / Success',
        message: (item) => `Managed certificate authority ${item.name || item.common_name || item.id} was created successfully.`,
        details: (item) => [
          { label: 'Name', value: item.name || item.common_name || item.id },
          { label: 'Common Name', value: item.common_name || 'n/a' },
          { label: 'Kind', value: item.kind || 'ca' },
          { label: 'Valid until', value: certificateExpiryCaption(item) },
        ],
      });
    } catch (err) {
      failCertificateAction(formEl, err, {
        title: 'Managed CA creation failed',
        eyebrow: 'Certificates / Error',
        errorDetails: () => [{ label: 'Action', value: 'Create managed CA' }],
      });
    }
  }

  async function issueCertificateFromCASubmit(event) {
    event.preventDefault();
    const formEl = event.currentTarget;
    setSubmitBusy(formEl, true, 'Issuing...');
    try {
      const form = new FormData(formEl);
      const payload = {
        authority_certificate_id: String(form.get('authority_certificate_id') || '').trim(),
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
        dns_names: parseCSVList(form.get('dns_names')),
        valid_days: Number(form.get('valid_days') || 365),
        is_default: String(form.get('is_default') || '') === '1',
      };
      const data = await sendJSON('/api/v1/platform/certificates/issue-from-ca', 'POST', payload);
      await finishCertificateAction(formEl, data, {
        title: 'Certificate issued',
        eyebrow: 'Certificates / Success',
        message: (item) => `Certificate ${item.name || item.common_name || item.id} was issued successfully.`,
        details: (item) => [
          { label: 'Name', value: item.name || item.common_name || item.id },
          { label: 'Common Name', value: item.common_name || 'n/a' },
          { label: 'Issuer', value: item.issuer_name || 'managed CA' },
          { label: 'Valid until', value: certificateExpiryCaption(item) },
        ],
      });
    } catch (err) {
      failCertificateAction(formEl, err, {
        title: 'Certificate issue failed',
        eyebrow: 'Certificates / Error',
        errorDetails: () => [{ label: 'Action', value: 'Issue certificate from managed CA' }],
      });
    }
  }

  async function createServiceCARootSubmit(event) {
    event.preventDefault();
    const formEl = event.currentTarget;
    setSubmitBusy(formEl, true, 'Creating service CA...');
    try {
      const form = new FormData(formEl);
      const payload = {
        service_code: String(form.get('service_code') || '').trim(),
        pki_profile: String(form.get('pki_profile') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
        valid_days: Number(form.get('valid_days') || 3650),
      };
      const data = await sendJSON('/api/v1/platform/pki-roots', 'POST', payload);
      await finishCertificateAction(formEl, data, {
        title: 'Service CA root created',
        eyebrow: 'Certificates / Success',
        message: (item) => `Service CA root for ${item.service_code || 'service'} / ${item.pki_profile || 'default'} was created successfully.`,
        details: (item) => [
          { label: 'Service', value: item.service_code || 'n/a' },
          { label: 'Profile', value: item.pki_profile || 'default' },
          { label: 'Common Name', value: item.common_name || 'n/a' },
          { label: 'Valid until', value: certificateExpiryCaption(item) },
        ],
      });
    } catch (err) {
      failCertificateAction(formEl, err, {
        title: 'Service CA root creation failed',
        eyebrow: 'Certificates / Error',
        errorDetails: () => [{ label: 'Action', value: 'Create service CA root' }],
      });
    }
  }

  function openCreateInstanceModal() {
    openModal('Create instance', 'POST /api/v1/instances', `
      <form id="createInstanceForm" class="form-grid">
        <div class="field"><label>Node</label><select name="node_id" required>${nodeOptions()}</select></div>
        <div class="field"><label>Service</label><select name="service_code" required>${instanceServiceOptions()}</select></div>
        <div class="field"><label>Name</label><input name="name" required placeholder="edge-xray-reality" /></div>
        <div class="field"><label>Slug</label><input name="slug" placeholder="optional" /></div>
        <div class="field"><label>Endpoint host</label><input name="endpoint_host" placeholder="vpn.example.com" /></div>
        <div class="field"><label>Endpoint port</label><input name="endpoint_port" type="number" min="0" max="65535" value="0" /></div>
        <div class="field"><label>Systemd unit</label><input name="systemd_unit" placeholder="optional override" /></div>
        <div id="instanceServiceFields" class="form-grid service-fields full"></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Create instance</button></div>
      </form>
      <div id="createInstanceResult" class="form-result"></div>`);
    const form = document.getElementById('createInstanceForm');
    const serviceSelect = form.querySelector('select[name="service_code"]');
    syncInstanceServiceFields('createInstanceForm', serviceSelect.value, null, { forceDefaults: true });
    serviceSelect.addEventListener('change', () => syncInstanceServiceFields('createInstanceForm', serviceSelect.value, null, { forceDefaults: true }));
    form.addEventListener('submit', createInstance);
  }

  async function createInstance(event) {
    event.preventDefault();
    const target = document.getElementById('createInstanceResult');
    target.innerHTML = '<span class="tag warn">creating</span>';
    try {
      const form = new FormData(event.currentTarget);
      const serviceCode = normalizeInstanceServiceCode(form.get('service_code'));
      const configBody = String(form.get('config_body') || '').trim();
      const payload = {
        node_id: String(form.get('node_id') || '').trim(),
        service_code: serviceCode,
        name: String(form.get('name') || '').trim(),
        slug: String(form.get('slug') || '').trim(),
        systemd_unit: String(form.get('systemd_unit') || '').trim(),
        endpoint_host: String(form.get('endpoint_host') || '').trim(),
        endpoint_port: Number(form.get('endpoint_port') || 0),
        spec: buildInstanceSpecPayload(serviceCode, form, {}, Number(form.get('endpoint_port') || 0)),
      };
      const data = await sendJSON('/api/v1/instances', 'POST', payload);
      target.innerHTML = renderActionResponse(data, 'Instance creation');
      await refresh();
      setTimeout(closeModal, 400);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
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

  async function runInstanceAction(instanceID, action, targetID = null) {
    const target = typeof targetID === 'string' ? document.getElementById(targetID) : targetID;
    if (target) target.innerHTML = `<span class="tag warn">queueing ${escapeHTML(action)}</span>`;
    const job = await requestJSON(`/api/v1/instances/${instanceID}/${action}`, { method: 'POST' });
    if (target) {
      await watchJob(job.id, target, `${action} instance`);
    }
    await refresh();
    return job;
  }

  async function queueInstanceAction(instanceID, action) {
    const actionLabel = `${action} instance`;
    const buttonSelector = `.instance-action-btn[data-instance-id="${CSS.escape(instanceID)}"][data-action="${CSS.escape(action)}"]`;
    const button = document.querySelector(buttonSelector);
    if (button) {
      button.disabled = true;
      button.textContent = `${action}...`;
    }
    try {
      await runInstanceAction(instanceID, action);
    } catch (err) {
      state.lastError = err;
      renderNotice();
      openModal(actionLabel, 'Instance action failed', `<div class="code-block">${escapeHTML(err.message)}</div>`);
    } finally {
      if (button) {
        button.disabled = false;
        button.textContent = action.charAt(0).toUpperCase() + action.slice(1);
      }
    }
  }

  async function openInstanceManageModal(instanceID) {
    openModal('Instance manage', 'Loading current desired state', '<div class="empty">Loading instance spec...</div>');
    try {
      const instance = await requestJSON(`/api/v1/instances/${instanceID}`);
      const draft = buildInstanceSpecDraft(instance.service_code, instance);
      openModal(`Manage instance: ${instance.name}`, 'Desired state, revisions and apply feedback', `
        <div class="grid cols-2">
          <div class="card">
            <div class="mini-label">Runtime summary</div>
            <div class="timeline">
              <div class="timeline-item"><strong>Service</strong><div class="timeline-meta">${escapeHTML(instance.service_code || 'unknown')}</div></div>
              <div class="timeline-item"><strong>Node</strong><div class="timeline-meta">${escapeHTML(instance.node_id || 'n/a')}</div></div>
              <div class="timeline-item"><strong>Endpoint</strong><div class="timeline-meta">${escapeHTML(instance.endpoint_host || 'n/a')}:${escapeHTML(instance.endpoint_port || 0)}</div></div>
              <div class="timeline-item"><strong>Systemd</strong><div class="timeline-meta">${escapeHTML(instance.systemd_unit || 'n/a')}</div></div>
            </div>
          </div>
          <div class="card">
            <div class="mini-label">Current state</div>
            <div class="inline-actions">
              ${statusTag(instance.status || 'unknown')}
              <span class="tag">${escapeHTML(instance.slug || 'no-slug')}</span>
              ${instance.current_revision_id ? `<span class="tag">rev ${escapeHTML(instance.current_revision_id.slice(0, 8))}</span>` : ''}
              ${instance.last_applied_revision_id ? `<span class="tag ok">applied ${escapeHTML(instance.last_applied_revision_id.slice(0, 8))}</span>` : ''}
            </div>
            <p>Сохранение ниже создает новую revision. Apply остается отдельным действием и будет показан с live job feedback и logs.</p>
          </div>
        </div>
        <form id="editInstanceForm" class="form-grid">
          <input type="hidden" name="slug" value="${escapeHTML(instance.slug || '')}" />
          <div class="field"><label>Endpoint port</label><input name="endpoint_port" type="number" min="0" max="65535" value="${escapeHTML(draft.endpoint_port || instance.endpoint_port || 0)}" /></div>
          <div class="field"><label>Service code</label><input value="${escapeHTML(instance.service_code || '')}" disabled /></div>
          <div class="form-grid service-fields full"></div>
          <div class="field full inline-actions">
            <button class="secondary-btn" type="submit">Save revision</button>
            <button class="primary-btn" type="button" id="saveApplyInstanceBtn">Save and apply</button>
            <button class="secondary-btn" type="button" id="restartInstanceBtn">Restart only</button>
          </div>
        </form>
        <div id="instanceManageRevisionResult" class="form-result"></div>
        <div id="instanceManageJobResult" class="form-result"></div>`);
      syncInstanceServiceFields('editInstanceForm', instance.service_code, draft);
      const form = document.getElementById('editInstanceForm');
      form.addEventListener('submit', (event) => saveManagedInstanceSpec(event, instance, false));
      document.getElementById('saveApplyInstanceBtn').addEventListener('click', (event) => saveManagedInstanceSpec(event, instance, true));
      document.getElementById('restartInstanceBtn').addEventListener('click', async () => {
        const jobTarget = document.getElementById('instanceManageJobResult');
        try {
          await runInstanceAction(instance.id, 'restart', jobTarget);
        } catch (err) {
          jobTarget.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        }
      });
    } catch (err) {
      el('modalBody').innerHTML = `<div class="empty">Failed to load instance: ${escapeHTML(err.message)}</div>`;
    }
  }

  async function saveManagedInstanceSpec(event, instance, applyAfterSave) {
    event.preventDefault();
    const revisionTarget = document.getElementById('instanceManageRevisionResult');
    const jobTarget = document.getElementById('instanceManageJobResult');
    const formEl = document.getElementById('editInstanceForm');
    if (revisionTarget) revisionTarget.innerHTML = '<span class="tag warn">saving revision</span>';
    if (jobTarget) jobTarget.innerHTML = '';
    setSubmitBusy(formEl, true, 'Saving...');
    try {
      const form = new FormData(formEl);
      const spec = buildInstanceSpecPayload(instance.service_code, form, instance.spec || {}, Number(form.get('endpoint_port') || instance.endpoint_port || 0));
      const data = await sendJSON(`/api/v1/instances/${instance.id}/spec`, 'PUT', { spec });
      instance.spec = spec;
      const revision = data?.revision || {};
      const issueCount = Array.isArray(revision.validation_errors) ? revision.validation_errors.length : Number(data?.issue_count || 0);
      if (revisionTarget) revisionTarget.innerHTML = `
        <div class="card">
          <div class="inline-actions" style="justify-content:space-between;align-items:flex-start">
            <div>
              <div class="mini-label">Revision saved</div>
              <div class="metric-caption">${escapeHTML(String(data?.message || 'Desired state updated.'))}</div>
            </div>
            ${statusTag(revision.status || 'unknown')}
          </div>
          <div class="grid cols-2" style="margin-top:14px">
            <div class="card"><div class="mini-label">Revision</div><div class="metric-caption">#${escapeHTML(revision.revision_no || 'n/a')}</div></div>
            <div class="card"><div class="mini-label">Can apply</div><div class="metric-caption">${data?.can_apply ? 'yes' : 'no'}</div></div>
            <div class="card"><div class="mini-label">Rendered hash</div><div class="metric-caption">${escapeHTML(revision.rendered_hash || 'n/a')}</div></div>
            <div class="card"><div class="mini-label">Validation issues</div><div class="metric-caption">${escapeHTML(String(issueCount))}</div></div>
          </div>
          ${issueCount ? `<div class="code-block" style="margin-top:14px">${escapeHTML(JSON.stringify(revision.validation_errors || [], null, 2))}</div>` : ''}
        </div>`;
      await refresh();
      if (applyAfterSave && data?.can_apply && jobTarget) {
        await runInstanceAction(instance.id, 'apply', jobTarget);
      } else if (applyAfterSave && jobTarget) {
        jobTarget.innerHTML = '<span class="tag danger">apply blocked: revision is not validated</span>';
      }
    } catch (err) {
      if (revisionTarget) revisionTarget.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    } finally {
      setSubmitBusy(formEl, false);
    }
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
      nodes: openCreateNodeModal,
      instances: openCreateInstanceModal,
      certificates: openCreateCertificateWizard,
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
    else if (state.page === 'nodeManage') renderNodeManagePage();
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

  async function refresh() {
    const seq = ++state.refreshSeq;
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
    }
    if (seq !== state.refreshSeq) return;
    render();
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

  bind();
  render();
  refresh();
})();
