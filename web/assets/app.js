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
    nodes: [],
    instances: [],
    clients: [],
    jobs: [],
    artifacts: [],
    shareLinks: [],
    servicesCatalog: [],
    servicePacks: [],
    serviceInstallers: [],
    serviceCapabilitiesByNode: {},
    serviceInstallEventsByNode: {},
    mailSettings: null,
    platformCertificates: [],
    platformInvites: [],
    platformPKIRoots: [],
    servicesNodeID: localStorage.getItem('megavpn.servicesNodeID') || '',
    revisionsInstanceID: localStorage.getItem('megavpn.revisionsInstanceID') || '',
    lastError: null,
  };

  localStorage.removeItem('megavpn.authToken');

  const navGroups = [
    ['Operations', [
      ['dashboard', 'Dashboard', '●'],
      ['nodes', 'Nodes', '◇'],
      ['instances', 'Instances', '▣'],
      ['clients', 'Clients', '◎'],
      ['jobs', 'Jobs', '↻'],
    ]],
    ['Provisioning', [
      ['artifacts', 'Artifacts', '▤'],
      ['shareLinks', 'Share links', '↗'],
    ]],
    ['Control', [
      ['services', 'Services', '⚙'],
      ['certificates', 'Certificates', '◈'],
      ['revisions', 'Revisions', '≣'],
      ['telemetry', 'Telemetry', '≋'],
      ['audit', 'Audit', '◇'],
      ['settings', 'Settings', '☷'],
    ]],
  ];

  const el = (id) => document.getElementById(id);
  const apiURL = (path) => `${state.apiBase}${path}`;

  function shareLinkURL(token) {
    if (!token) return 'n/a';
    try {
      return new URL(`/share/${token}`, state.apiBase || window.location.origin).toString();
    } catch (_) {
      return `/share/${token}`;
    }
  }

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
  }

  function clsStatus(status) {
    const normalized = String(status || '').toLowerCase();
    if (['ok', 'ready', 'active', 'healthy', 'succeeded', 'online', 'configured', 'enabled', 'sent', 'delivered'].includes(normalized)) return 'ok';
    if (['stub', 'planned', 'draft', 'pending', 'unknown', 'maintenance'].includes(normalized)) return 'stub';
    if (['degraded', 'warning', 'retrying', 'queued', 'running', 'bootstrapping', 'provisioning', 'inactive'].includes(normalized)) return 'warn';
    if (['failed', 'offline', 'error', 'disabled', 'cancelled', 'revoked', 'missing', 'delivery_failed'].includes(normalized)) return 'danger';
    return 'stub';
  }

  function statusTag(value) {
    return `<span class="tag ${clsStatus(value)}"><span class="dot ${clsStatus(value) === 'stub' ? 'unknown' : clsStatus(value)}"></span>${escapeHTML(value)}</span>`;
  }

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

  function nodeHeartbeatStatus(node) {
    const heartbeat = node?.last_heartbeat_at;
    if (!heartbeat) return 'unknown';
    const diffMs = Date.now() - new Date(heartbeat).getTime();
    if (!Number.isFinite(diffMs)) return 'unknown';
    if (diffMs <= 30 * 1000) return 'online';
    if (diffMs <= 5 * 60 * 1000) return 'degraded';
    return 'offline';
  }

  function countItems(value) {
    if (Array.isArray(value)) return value.length;
    if (value && typeof value === 'object') return Object.keys(value).length;
    return 0;
  }

  function shortFingerprint(value) {
    const text = String(value || '').trim();
    if (!text) return 'n/a';
    if (text.length <= 22) return text;
    return `${text.slice(0, 12)}...${text.slice(-8)}`;
  }

  function latestBootstrapSummary(run) {
    if (!run) return 'none';
    const mode = run.bootstrap_mode || 'n/a';
    const when = formatRelativeDate(run.finished_at || run.started_at || run.created_at);
    return `${mode} · ${when}`;
  }

  function inventoryLabel(payload, path, fallback = 'n/a') {
    const parts = path.split('.');
    let current = payload;
    for (const part of parts) {
      if (!current || typeof current !== 'object') return fallback;
      current = current[part];
    }
    if (current == null || current === '') return fallback;
    return String(current);
  }

  function diagnosticsAgentState(diag) {
    return diag?.agent?.status || diag?.node?.agent_status || 'unknown';
  }

  function commMetricLine(label, when, detail) {
    return `<div class="card"><div class="mini-label">${label}</div><div class="metric-caption">${escapeHTML(formatDate(when))}</div><div class="metric-caption">${escapeHTML(detail || 'n/a')}</div></div>`;
  }

  function objectEntries(value) {
    return value && typeof value === 'object' && !Array.isArray(value) ? Object.entries(value) : [];
  }

  function formatNumber(value, fallback = 'n/a') {
    const num = Number(value);
    return Number.isFinite(num) ? String(num) : fallback;
  }

  function formatBytes(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num < 0) return 'n/a';
    if (num < 1024) return `${num} B`;
    if (num < 1024 * 1024) return `${(num / 1024).toFixed(1)} KB`;
    return `${(num / (1024 * 1024)).toFixed(1)} MB`;
  }

  function renderInventoryFact(label, value, detail = '') {
    return `
      <div class="fact-card">
        <div class="mini-label">${escapeHTML(label)}</div>
        <div class="metric-caption strong">${escapeHTML(value || 'n/a')}</div>
        ${detail ? `<div class="metric-caption">${escapeHTML(detail)}</div>` : ''}
      </div>`;
  }

  function renderChipList(items, emptyText) {
    if (!items.length) {
      return `<div class="empty compact-empty">${escapeHTML(emptyText)}</div>`;
    }
    return `<div class="chip-list">${items.map((item) => `<span class="chip">${escapeHTML(item)}</span>`).join('')}</div>`;
  }

  function renderInventoryCollection(label, items, formatter, emptyText) {
    if (!items.length) {
      return `<div class="empty compact-empty">${escapeHTML(emptyText)}</div>`;
    }
    return `
      <div class="inventory-list">
        ${items.map((item) => `
          <div class="inventory-item">
            <div class="mini-label">${escapeHTML(label)}</div>
            ${formatter(item)}
          </div>`).join('')}
      </div>`;
  }

  function renderInventorySnapshotPanel(latestInventory, node) {
    const payload = latestInventory?.payload || {};
    const osName = inventoryLabel(payload, 'os.pretty_name', `${node.os_family || 'linux'} ${node.os_version || ''}`.trim() || 'n/a');
    const kernel = inventoryLabel(payload, 'kernel', 'kernel n/a');
    const architecture = inventoryLabel(payload, 'arch', node.architecture || 'n/a');
    const hostname = inventoryLabel(payload, 'hostname', node.name || 'n/a');
    const interfaces = Array.isArray(payload.interfaces) ? payload.interfaces : [];
    const ports = Array.isArray(payload.ports) ? payload.ports : [];
    const processes = Array.isArray(payload.processes) ? payload.processes : [];
    const binaries = objectEntries(payload.binaries);
    const services = objectEntries(payload.services);
    const configs = objectEntries(payload.config_files);
    const collectedAt = payload.collected_at || latestInventory?.created_at || null;

    const serviceItems = services
      .slice(0, 10)
      .map(([name, info]) => `${name} · ${info?.active_state || 'unknown'} / ${info?.enabled_state || 'unknown'}`);
    const binaryItems = binaries
      .slice(0, 12)
      .map(([name, version]) => `${name} · ${String(version || '').split('\n')[0].slice(0, 56)}`);

    return `
      <section class="section-card">
        <div class="section-head">
          <div>
            <div class="eyebrow">Inventory Snapshot</div>
            <h2>Host Inventory</h2>
          </div>
          <div class="section-meta">
            <span class="tag">${escapeHTML(formatRelativeDate(collectedAt))}</span>
            <span class="tag">${escapeHTML(formatDate(collectedAt))}</span>
          </div>
        </div>
        <div class="section-body">
          <div class="inventory-facts">
            ${renderInventoryFact('OS', osName, kernel)}
            ${renderInventoryFact('Architecture', architecture, hostname)}
            ${renderInventoryFact('Interfaces', formatNumber(interfaces.length), `listening ports ${formatNumber(ports.length, '0')}`)}
            ${renderInventoryFact('Runtime', `services ${formatNumber(services.length, '0')}`, `configs ${formatNumber(configs.length, '0')} · processes ${formatNumber(processes.length, '0')}`)}
          </div>
          <div class="inventory-columns" style="margin-top:16px">
            <section class="inventory-panel">
              <div class="table-head compact-head"><h3>Network Interfaces</h3><span class="tag">${escapeHTML(String(interfaces.length))}</span></div>
              ${renderInventoryCollection('Interface', interfaces.slice(0, 8), (item) => `
                <div class="inventory-item-title">${escapeHTML(item.name || 'unknown')}</div>
                <div class="metric-caption">${escapeHTML((item.addrs || []).join(' · ') || item.flags || 'no addresses')}</div>
                <div class="metric-caption">mtu ${escapeHTML(formatNumber(item.mtu, 'n/a'))} · ${escapeHTML(item.flags || 'flags n/a')}</div>`, 'No network interface data in the latest snapshot.')}
            </section>
            <section class="inventory-panel">
              <div class="table-head compact-head"><h3>Listening Ports</h3><span class="tag">${escapeHTML(String(ports.length))}</span></div>
              ${renderInventoryCollection('Socket', ports.slice(0, 10), (item) => `
                <div class="inventory-item-title">${escapeHTML(item.local_address || 'unknown')}</div>
                <div class="metric-caption">${escapeHTML(item.network || 'n/a')} · ${escapeHTML(item.state || 'n/a')}</div>`, 'No listening socket data in the latest snapshot.')}
            </section>
            <section class="inventory-panel">
              <div class="table-head compact-head"><h3>Detected Services</h3><span class="tag">${escapeHTML(String(services.length))}</span></div>
              ${renderChipList(serviceItems, 'No detected services in the latest snapshot.')}
            </section>
            <section class="inventory-panel">
              <div class="table-head compact-head"><h3>Detected Binaries</h3><span class="tag">${escapeHTML(String(binaries.length))}</span></div>
              ${renderChipList(binaryItems, 'No runtime binaries were reported.')}
            </section>
            <section class="inventory-panel">
              <div class="table-head compact-head"><h3>Config Files</h3><span class="tag">${escapeHTML(String(configs.length))}</span></div>
              ${renderInventoryCollection('Config', configs.slice(0, 8), ([path, info]) => `
                <div class="inventory-item-title">${escapeHTML(path)}</div>
                <div class="metric-caption">${escapeHTML(formatBytes(info?.size_bytes))} · ${escapeHTML(info?.mode || 'mode n/a')}</div>
                <div class="metric-caption">${escapeHTML(formatDate(info?.modified_at))}</div>`, 'No config files were captured in the latest snapshot.')}
            </section>
            <section class="inventory-panel">
              <div class="table-head compact-head"><h3>Interesting Processes</h3><span class="tag">${escapeHTML(String(processes.length))}</span></div>
              ${renderInventoryCollection('Process', processes.slice(0, 8), (item) => `
                <div class="inventory-item-title">${escapeHTML(item.command || item.pid || 'unknown')}</div>
                <div class="metric-caption">pid ${escapeHTML(item.pid || 'n/a')}</div>
                <div class="metric-caption">${escapeHTML(item.args || '')}</div>`, 'No interesting processes were captured in the latest snapshot.')}
            </section>
          </div>
          <details class="details-block">
            <summary>Raw snapshot JSON</summary>
            <div class="code-block">${escapeHTML(JSON.stringify(payload, null, 2))}</div>
          </details>
        </div>
      </section>`;
  }

  function toMillis(value) {
    if (!value) return 0;
    const ms = Date.parse(value);
    return Number.isFinite(ms) ? ms : 0;
  }

  function headers(extra = {}) {
    return { Accept: 'application/json', ...extra };
  }

  function hasPermission(code) {
    return Array.isArray(state.authPermissions) && state.authPermissions.includes(code);
  }

  function hasRole(code) {
    return Array.isArray(state.authRoles) && state.authRoles.includes(code);
  }

  async function request(path, options = {}) {
    const opts = { credentials: 'include', ...options };
    opts.headers = headers(options.headers || {});
    const method = String(opts.method || 'GET').toUpperCase();
    if (!['GET', 'HEAD', 'OPTIONS', 'TRACE'].includes(method)) {
      opts.headers['X-MegaVPN-CSRF'] = '1';
    }
    const res = await fetch(apiURL(path), opts);
    const contentType = res.headers.get('content-type') || '';
    let data = null;
    let text = '';
    if (contentType.includes('application/json')) {
      data = await res.json().catch(() => null);
    } else {
      text = await res.text().catch(() => '');
    }
    if (!res.ok) {
      const msg = data?.error || text || `${path}: HTTP ${res.status}`;
      const err = new Error(msg);
      err.status = res.status;
      err.payload = data;
      throw err;
    }
    return data;
  }

  async function requestJSON(path, options = {}) {
    return request(path, options);
  }

  async function sendJSON(path, method, payload) {
    return requestJSON(path, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: payload == null ? null : JSON.stringify(payload),
    });
  }

  async function fetchJSON(path, fallback = null, options = {}) {
    try {
      return await requestJSON(path, options);
    } catch (err) {
      state.lastError = err;
      return fallback;
    }
  }

  function metric(label, value, caption) {
    return `<div class="card"><div class="mini-label">${label}</div><div class="metric-value">${escapeHTML(value)}</div><div class="metric-caption">${escapeHTML(caption)}</div></div>`;
  }

  function clientByID(clientID) {
    return (state.clients || []).find((client) => client.id === clientID) || null;
  }

  function nodeByID(nodeID) {
    return (state.nodes || []).find((node) => node.id === nodeID) || null;
  }

  function provisionableArtifactServiceCodes() {
    return new Set(['openvpn', 'xray-core', 'wireguard', 'mtproto', 'ipsec', 'http_proxy', 'shadowsocks']);
  }

  function artifactRowsForClient(clientID) {
    return (state.artifacts || [])
      .filter((artifact) => !clientID || artifact.client_account_id === clientID)
      .sort((left, right) => toMillis(right.created_at) - toMillis(left.created_at));
  }

  function provisionableInstancesForExport() {
    const allowed = provisionableArtifactServiceCodes();
    return (state.instances || []).filter((instance) => {
      const serviceCode = normalizeInstanceServiceCode(instance.service_code);
      return allowed.has(serviceCode) && String(instance.status || '').toLowerCase() !== 'deleted';
    });
  }

  function clientOptions(selectedClientID = '') {
    return (state.clients || []).map((client) => `
      <option value="${escapeHTML(client.id)}"${client.id === selectedClientID ? ' selected' : ''}>
        ${escapeHTML(client.username || client.display_name || client.id)} · ${escapeHTML(client.status || 'unknown')}
      </option>`).join('');
  }

  function artifactTypeLabel(artifactType) {
    switch (String(artifactType || '').trim()) {
      case 'ovpn':
        return '.ovpn profile';
      case 'vless_url':
        return 'VLESS URL';
      case 'wg_conf':
        return 'WireGuard config';
      case 'mtproto_url':
        return 'MTProto URL';
      case 'http_proxy_bundle':
        return 'HTTP proxy bundle';
      case 'ss_url':
        return 'Shadowsocks URL';
      case 'zip_bundle':
        return 'ZIP bundle';
      case 'ipsec_bundle':
        return 'IPsec/L2TP bundle';
      default:
        return artifactType || 'artifact';
    }
  }

  function artifactOptions(clientID, selectedArtifactID = '') {
    const artifacts = artifactRowsForClient(clientID);
    if (!artifacts.length) {
      return '<option value="">No generated artifacts for this client</option>';
    }
    return artifacts.map((artifact) => {
      const accessID = artifact.service_access_id || 'shared';
      const createdAt = formatDate(artifact.created_at);
      const label = `${artifactTypeLabel(artifact.artifact_type)} · ${accessID} · ${createdAt}`;
      return `<option value="${escapeHTML(artifact.id)}"${artifact.id === selectedArtifactID ? ' selected' : ''}>${escapeHTML(label)}</option>`;
    }).join('');
  }

  function instanceOptionsForArtifactExport(selectedIDs = []) {
    const selected = new Set(selectedIDs || []);
    return provisionableInstancesForExport().map((instance) => {
      const node = state.nodes.find((item) => item.id === instance.node_id);
      const label = [
        instance.name || instance.id,
        normalizeInstanceServiceCode(instance.service_code),
        node?.name || instance.node_id || 'node',
      ].join(' · ');
      return `<option value="${escapeHTML(instance.id)}"${selected.has(instance.id) ? ' selected' : ''}>${escapeHTML(label)}</option>`;
    }).join('');
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

  function updateReadyPill() {
    const status = state.ready?.status || 'unknown';
    const dotClass = status === 'ready' ? 'ok' : 'danger';
    el('readyPill').innerHTML = `<span class="dot ${dotClass}"></span>${escapeHTML(status)}`;
    el('apiBaseLabel').textContent = state.apiBase || 'same-origin';
  }

  function renderNotice() {
    const notice = el('notice');
    if (!state.authUser) {
      notice.innerHTML = '<strong>Login required.</strong> Панель работает через local auth, HttpOnly session cookie и CSRF header для mutating API.';
      return;
    }
    if (state.lastError) {
      notice.innerHTML = `<strong>Last UI/API error.</strong> ${escapeHTML(state.lastError.message)}`;
      return;
    }
    notice.innerHTML = '<strong>Control plane online.</strong> Nodes, instances, clients, jobs, artifacts, share links, revisions, telemetry and audit работают с backend API напрямую.';
  }

  function renderNav() {
    const nav = el('nav');
    nav.innerHTML = navGroups.map(([group, items]) => `
      <div class="nav-section">${group}</div>
      ${items.map(([key, label, icon]) => `
        <button class="nav-item ${state.page === key ? 'active' : ''}" type="button" data-page="${key}">
          <span class="nav-left"><span class="nav-icon">${icon}</span>${label}</span>
        </button>
      `).join('')}
    `).join('');
    nav.querySelectorAll('[data-page]').forEach((btn) => btn.addEventListener('click', () => setPage(btn.dataset.page)));
  }

  function renderAuthSlot() {
    const slot = el('authSlot');
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
    state.page = page;
    render();
    if (page === 'services' && state.authUser) {
      void loadServicesPageData();
    }
  }

  function setTitle(title) {
    el('pageTitle').textContent = title;
  }

  async function loadSession() {
    try {
      const data = await requestJSON('/api/v1/auth/me');
      state.authUser = data.user || null;
      state.authSession = data.session || null;
      state.authRoles = Array.isArray(data.roles) ? data.roles : [];
      state.authPermissions = Array.isArray(data.permissions) ? data.permissions : [];
      return true;
    } catch (err) {
      if (err.status === 401) {
        state.authUser = null;
        state.authSession = null;
        state.authRoles = [];
        state.authPermissions = [];
        return false;
      }
      throw err;
    }
  }

  async function loadCore() {
    state.ready = await fetchJSON('/api/v1/ready', { status: 'not_ready' });
    if (!state.authUser) {
      state.dashboard = null;
      state.nodes = [];
      state.instances = [];
      state.clients = [];
      state.jobs = [];
      state.artifacts = [];
      state.shareLinks = [];
      state.servicesCatalog = [];
      state.servicePacks = [];
      state.serviceInstallers = [];
      state.serviceCapabilitiesByNode = {};
      state.serviceInstallEventsByNode = {};
      state.mailSettings = null;
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
    const clients = await fetchJSON('/api/v1/clients', []);
    const jobs = await fetchJSON('/api/v1/jobs?limit=50', []);
    const artifacts = await fetchJSON('/api/v1/artifacts', []);
    const shareLinks = await fetchJSON('/api/v1/share-links', []);
    const servicesCatalog = await fetchJSON('/api/v1/services', []);
    const servicePacks = await fetchJSON('/api/v1/service-packs', []);
    const serviceInstallers = await fetchJSON('/api/v1/services/installers', []);
    const platformCertificates = hasPermission('instance.read') ? await fetchJSON('/api/v1/platform/certificates', []) : [];
    const platformPKIRoots = hasPermission('instance.read') ? await fetchJSON('/api/v1/platform/pki-roots', []) : [];
    state.nodes = Array.isArray(nodes) ? nodes : [];
    state.instances = Array.isArray(instances) ? instances : [];
    state.clients = Array.isArray(clients) ? clients : [];
    state.jobs = Array.isArray(jobs) ? jobs : [];
    state.artifacts = Array.isArray(artifacts) ? artifacts : [];
    state.shareLinks = Array.isArray(shareLinks) ? shareLinks : [];
    state.servicesCatalog = Array.isArray(servicesCatalog) ? servicesCatalog : [];
    state.servicePacks = Array.isArray(servicePacks) ? servicePacks : [];
    state.serviceInstallers = Array.isArray(serviceInstallers) ? serviceInstallers : [];
    state.platformCertificates = Array.isArray(platformCertificates) ? platformCertificates : [];
    state.platformPKIRoots = Array.isArray(platformPKIRoots) ? platformPKIRoots : [];
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
        ${metric('Nodes', d.nodes_total ?? state.nodes.length, `${Number(d.nodes_online || 0)} online`)}
        ${metric('Instances', d.instances_total ?? instanceRows.length, `${Number(d.instances_active || 0)} active`)}
        ${metric('Clients', d.clients_total ?? state.clients.length, `${Number(d.clients_active || 0)} active`)}
        ${metric('Jobs', jobsTotal || jobRows.length, `${Number(d.jobs_queued || 0)} queued · ${Number(d.jobs_running || 0)} running · ${Number(d.jobs_failed || 0)} failed`)}
      </div>
      <div class="split">
        ${tableCard('Service Instances', instanceRows.slice(0, 8).map(toInstanceRow), [
          { title: 'Name', key: 'name' },
          { title: 'Service', key: 'service', render: (r) => `<span class="tag">${escapeHTML(r.service)}</span>` },
          { title: 'Node', key: 'node' },
          { title: 'Endpoint', key: 'endpoint' },
          { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        ])}
        <section class="table-card">
          <div class="table-head"><h2>Runtime Notes</h2><span class="tag ok">live foundation</span></div>
          <div class="card-body timeline">
            <div class="timeline-item"><strong>Auth and sessions</strong><div class="timeline-meta">local auth + HttpOnly session cookie + CSRF header</div></div>
            <div class="timeline-item"><strong>Bootstrap pipeline</strong><div class="timeline-meta">secret refs, access methods, bootstrap jobs, bootstrap runs</div></div>
            <div class="timeline-item"><strong>Next backend slice</strong><div class="timeline-meta">real SSH executor and agent trust plane</div></div>
          </div>
        </section>
      </div>`;
  }

  function renderNodes() {
    setTitle('Nodes');
    const rows = Array.isArray(state.nodes) ? state.nodes : [];
    el('content').innerHTML = `
      ${tableCard('Managed Nodes', rows, [
        { title: 'Name', key: 'name' },
        { title: 'Role', key: 'role', render: (r) => `<span class="tag">${escapeHTML(r.role || 'egress')}</span>` },
        { title: 'Kind', key: 'kind', render: (r) => `<span class="tag">${escapeHTML(r.kind || 'local')}</span>` },
        { title: 'Address', key: 'address' },
        { title: 'Execution', key: 'execution_mode' },
        { title: 'Agent', key: 'agent_status', render: (r) => statusTag(r.agent_status || 'unknown') },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status || 'draft') },
        { title: 'Actions', key: 'id', render: (r) => `<div class="inline-actions"><button class="secondary-btn manage-node-btn" type="button" data-node-id="${escapeHTML(r.id)}">Manage</button></div>` },
      ], '<button class="secondary-btn" id="createNodeBtn" type="button">Add node</button>')}
      <div class="grid cols-3">
        <div class="card"><h3>Bootstrap flow</h3><p>Node → SSH access method → enrollment token → bootstrap job → agent enrollment → heartbeat/jobs.</p></div>
        <div class="card"><h3>Access model</h3><p>SSH используется только для onboarding. После регистрации дальнейшее управление идет через agent pull model.</p></div>
        <div class="card"><h3>Current slice</h3><p>UI уже умеет создавать node, настраивать SSH bootstrap access и ставить bootstrap job в очередь.</p></div>
      </div>`;
    const btn = document.getElementById('createNodeBtn');
    if (btn) btn.addEventListener('click', openCreateNodeModal);
    document.querySelectorAll('.manage-node-btn').forEach((button) => {
      button.addEventListener('click', () => openNodeControlModal(button.dataset.nodeId));
    });
  }

  function renderInstances() {
    setTitle('Instances');
    const rows = (state.instances || []).map(toInstanceRow);
    el('content').innerHTML = `
      ${tableCard('Instances', rows, [
        { title: 'Name', key: 'name' },
        { title: 'Node', key: 'node' },
        { title: 'Service', key: 'service', render: (r) => `<span class="tag">${escapeHTML(r.service)}</span>` },
        { title: 'Endpoint', key: 'endpoint' },
        { title: 'Revision', key: 'revision' },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        { title: 'Actions', key: 'id', render: (r) => `
          <div class="inline-actions">
            <button class="secondary-btn instance-manage-btn" type="button" data-instance-id="${escapeHTML(r.id)}">Manage</button>
            <button class="secondary-btn instance-action-btn" type="button" data-action="apply" data-instance-id="${escapeHTML(r.id)}">Apply</button>
            <button class="secondary-btn instance-action-btn" type="button" data-action="restart" data-instance-id="${escapeHTML(r.id)}">Restart</button>
            <button class="secondary-btn instance-action-btn" type="button" data-action="start" data-instance-id="${escapeHTML(r.id)}">Start</button>
            <button class="secondary-btn instance-action-btn" type="button" data-action="stop" data-instance-id="${escapeHTML(r.id)}">Stop</button>
          </div>` },
      ], '<div class="inline-actions"><button class="secondary-btn" id="createServicePackBtn" type="button">Create service pack</button><button class="secondary-btn" id="createInstanceBtn" type="button">Create instance</button></div>')}
      
      <div class="grid cols-3">
        <div class="card"><h3>Service Packs</h3><p>Для production-сценариев используй готовые bundles: IPsec+XL2TPD, Xray Reality, Xray+Nginx gRPC/HTTP, OpenVPN TCP/UDP. Это безопаснее, чем собирать edge руками.</p></div>
        <div class="card"><h3>Xray Runtime</h3><p>Xray теперь разведен на direct Reality и backend edge-профили. xray-core это runtime, а публичная топология зависит от пресета или pack-а.</p></div>
        <div class="card"><h3>CA Center</h3><p>OpenVPN platform PKI живет на control plane. Активные CA roots видны в Settings → Platform CA Center.</p></div>
      </div>`;
    const createBtn = document.getElementById('createInstanceBtn');
    if (createBtn) createBtn.addEventListener('click', openCreateInstanceModal);
    const createPackBtn = document.getElementById('createServicePackBtn');
    if (createPackBtn) createPackBtn.addEventListener('click', openCreateServicePackModal);
    document.querySelectorAll('.instance-manage-btn').forEach((button) => {
      button.addEventListener('click', () => openInstanceManageModal(button.dataset.instanceId));
    });
    document.querySelectorAll('.instance-action-btn').forEach((button) => {
      button.addEventListener('click', () => queueInstanceAction(button.dataset.instanceId, button.dataset.action));
    });
  }

  function renderServices() {
    setTitle('Services');
    const selectedNode = state.nodes.find((node) => node.id === state.servicesNodeID) || state.nodes[0] || null;
    const runtimeServices = groupInstallersByService(state.serviceInstallers || []);
    const capabilities = selectedNode ? (state.serviceCapabilitiesByNode[selectedNode.id] || []) : [];
    const events = selectedNode ? (state.serviceInstallEventsByNode[selectedNode.id] || []) : [];
    const definitions = Array.isArray(state.servicesCatalog) ? state.servicesCatalog : [];
    el('content').innerHTML = `
      <section class="panel-banner">
        <div>
          <div class="eyebrow">Runtime Capabilities</div>
          <h2>Service installers and node distribution</h2>
          <p>Установка runtime идет через capability jobs на agent-managed nodes. Control Plane ставит задачу, агент устанавливает пакет или проверяет уже существующий runtime, затем inventory и capability state обновляются автоматически.</p>
        </div>
        <div class="panel-banner-side">
          <div class="tag ok">${escapeHTML(String(runtimeServices.length))} installable runtimes</div>
          <div class="metric-caption">${selectedNode ? `Target node: ${selectedNode.name}` : 'No nodes available'}</div>
        </div>
      </section>
      <section class="card">
        <div class="inline-actions">
          <div class="field" style="min-width:280px">
            <label>Target node</label>
            <select id="servicesNodeSelect">
              ${state.nodes.map((node) => `<option value="${escapeHTML(node.id)}"${node.id === selectedNode?.id ? ' selected' : ''}>${escapeHTML(node.name)} · ${escapeHTML(node.address)} · ${escapeHTML(node.agent_status || 'unknown')}</option>`).join('')}
            </select>
          </div>
          <button class="secondary-btn" id="refreshServicesBtn" type="button">Refresh service state</button>
        </div>
      </section>
      <div class="services-grid">
        ${runtimeServices.map((item) => renderServiceRuntimeCard(item, selectedNode, capabilities)).join('')}
      </div>
      <section class="table-card">
        <div class="table-head"><h2>Capability Matrix</h2><div class="table-tools"><span class="tag">${escapeHTML(String(state.nodes.length))} nodes</span></div></div>
        <div class="table-wrap">${renderCapabilityMatrix(state.nodes, state.serviceCapabilitiesByNode)}</div>
      </section>
      <section class="split">
        <section class="table-card">
          <div class="table-head"><h2>Service Catalog</h2><div class="table-tools"><span class="tag">${escapeHTML(String(definitions.length))} definitions</span></div></div>
          <div class="table-wrap">${renderServiceDefinitionsTable(definitions)}</div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Recent Capability Events</h2><div class="table-tools"><span class="tag">${escapeHTML(selectedNode?.name || 'node')}</span></div></div>
          <div class="table-wrap">${renderCapabilityEventsTable(events)}</div>
        </section>
      </section>`;
    const nodeSelect = document.getElementById('servicesNodeSelect');
    if (nodeSelect) {
      nodeSelect.addEventListener('change', async (event) => {
        state.servicesNodeID = event.currentTarget.value;
        localStorage.setItem('megavpn.servicesNodeID', state.servicesNodeID);
        renderServices();
        await loadServicesPageData();
      });
    }
    const refreshBtn = document.getElementById('refreshServicesBtn');
    if (refreshBtn) {
      refreshBtn.addEventListener('click', loadServicesPageData);
    }
    document.querySelectorAll('.service-install-btn').forEach((button) => {
      button.addEventListener('click', () => runServiceInstaller(button.dataset.serviceCode, button.dataset.strategy, button.dataset.channel));
    });
    document.querySelectorAll('.service-verify-btn').forEach((button) => {
      button.addEventListener('click', () => verifyServiceCapability(button.dataset.serviceCode));
    });
    if (!state.serviceCapabilitiesByNode[selectedNode?.id || '']) {
      void loadServicesPageData();
    }
  }

  function renderClients() {
    setTitle('Clients');
    const rows = state.clients || [];
    el('content').innerHTML = `
      ${tableCard('Client Accounts', rows, [
        { title: 'Username', key: 'username' },
        { title: 'Display', key: 'display_name', render: (r) => escapeHTML(r.display_name || 'n/a') },
        { title: 'Email', key: 'email', render: (r) => escapeHTML(r.email || 'n/a') },
        { title: 'Expires', key: 'expires_at', render: (r) => formatDate(r.expires_at) },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        { title: 'Actions', key: 'id', render: (r) => `<div class="inline-actions"><button class="secondary-btn client-provision-btn" type="button" data-client-id="${escapeHTML(r.id)}">Provision</button><button class="secondary-btn client-accesses-btn" type="button" data-client-id="${escapeHTML(r.id)}">Accesses</button><button class="secondary-btn client-email-btn" type="button" data-client-id="${escapeHTML(r.id)}">Send access email</button></div>` },
      ], '<button class="secondary-btn" type="button" id="clientCreateBtn">Create client</button>')}
      <section class="card"><h2>Provisioning flow</h2><p>Create ClientAccount → Resolve instances → Create ServiceAccess → Generate <code>ovpn</code> / <code>vless_url</code> / <code>wg_conf</code> / <code>mtproto_url</code> / <code>http_proxy_bundle</code> / <code>ipsec_bundle</code> / <code>ss_url</code> / <code>zip_bundle</code> → Publish ShareLink → Audit. Для текущего backend-среза поддержаны <code>OpenVPN</code>, <code>Xray</code>, <code>WireGuard</code>, <code>MTProto</code>, <code>HTTP Proxy</code>, <code>IPsec/L2TP</code> и <code>Shadowsocks</code>.</p></section>`;
    const btn = document.getElementById('clientCreateBtn');
    if (btn) btn.addEventListener('click', openCreateClientModal);
    document.querySelectorAll('.client-provision-btn').forEach((button) => {
      button.addEventListener('click', () => queueClientProvision(button.dataset.clientId));
    });
    document.querySelectorAll('.client-accesses-btn').forEach((button) => {
      button.addEventListener('click', () => openClientAccessesModal(button.dataset.clientId));
    });
    document.querySelectorAll('.client-email-btn').forEach((button) => {
      button.addEventListener('click', () => openClientEmailModal(button.dataset.clientId));
    });
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
    setTitle('Artifacts');
    const rows = (state.artifacts || []).map((artifact) => ({
      id: artifact.id,
      client_id: artifact.client_account_id || '',
      client_name: clientByID(artifact.client_account_id)?.username || artifact.client_account_id || 'n/a',
      client: artifact.client_account_id || 'n/a',
      access: artifact.service_access_id || 'n/a',
      type: artifact.artifact_type || 'unknown',
      size: artifact.size_bytes || 0,
      path: artifact.storage_path || 'n/a',
      status: artifact.status || 'unknown',
      created: artifact.created_at,
    }));
    el('content').innerHTML = `
      ${tableCard('Artifacts', rows, [
        { title: 'Type', key: 'type', render: (r) => `<span class="tag">${escapeHTML(r.type)}</span>` },
        { title: 'Client', key: 'client_name' },
        { title: 'Service Access', key: 'access' },
        { title: 'Size', key: 'size', render: (r) => escapeHTML(`${r.size} B`) },
        { title: 'Path', key: 'path', render: (r) => `<code>${escapeHTML(r.path)}</code>` },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        { title: 'Created', key: 'created', render: (r) => formatDate(r.created) },
        { title: 'Actions', key: 'id', render: (r) => `<div class="inline-actions compact-actions"><button class="secondary-btn artifact-publish-btn" type="button" data-client-id="${escapeHTML(r.client_id)}" data-artifact-id="${escapeHTML(r.id)}">Publish link</button></div>` },
      ], '<button class="secondary-btn" type="button" id="artifactExportBtn">Queue export</button>')}
      <section class="card"><h2>Generated outputs</h2><p>Export flow ставит в очередь <code>client.provision</code> и собирает реальные client-facing artifacts для <code>OpenVPN</code>, <code>Xray</code>, <code>WireGuard</code>, <code>MTProto</code>, <code>HTTP Proxy</code>, <code>IPsec/L2TP</code> и <code>Shadowsocks</code>: <code>.ovpn</code>, <code>vless_url</code>, <code>wg_conf</code>, <code>mtproto_url</code>, <code>http_proxy_bundle</code>, <code>ipsec_bundle</code>, <code>ss_url</code> и общий <code>zip_bundle</code>.</p></section>`;
    const exportBtn = document.getElementById('artifactExportBtn');
    if (exportBtn) exportBtn.addEventListener('click', openArtifactExportModal);
    document.querySelectorAll('.artifact-publish-btn').forEach((button) => {
      button.addEventListener('click', () => openShareLinkPublishModal(button.dataset.clientId, button.dataset.artifactId));
    });
  }

  function renderShareLinks() {
    setTitle('Share Links');
    const rows = (state.shareLinks || []).map((link) => ({
      id: link.id,
      client: clientByID(link.client_account_id)?.username || link.client_account_id || 'n/a',
      client_id: link.client_account_id || '',
      target: `${link.target_type || 'artifact'}:${link.target_id || 'n/a'}`,
      status: link.status || 'unknown',
      expires: link.expires_at,
      downloads: link.download_count || 0,
      token: link.token || '',
      url: shareLinkURL(link.token || ''),
    }));
    el('content').innerHTML = `
      ${tableCard('Share Links', rows, [
        { title: 'Client', key: 'client' },
        { title: 'Target', key: 'target' },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        { title: 'Downloads', key: 'downloads' },
        { title: 'Expires', key: 'expires', render: (r) => formatDate(r.expires) },
        { title: 'Token', key: 'token', render: (r) => `<code>${escapeHTML(r.token)}</code>` },
        { title: 'URL', key: 'url', render: (r) => r.url === 'n/a' ? 'n/a' : `<code>${escapeHTML(r.url)}</code>` },
        { title: 'Actions', key: 'id', render: (r) => `<div class="inline-actions compact-actions"><button class="secondary-btn sharelink-open-btn" type="button" data-url="${escapeHTML(r.url)}">Open</button><button class="danger-btn sharelink-revoke-btn" type="button" data-client-id="${escapeHTML(r.client_id)}" data-link-id="${escapeHTML(r.id)}">Revoke</button></div>` },
      ], '<button class="secondary-btn" type="button" id="shareLinkCreateBtn">Publish share link</button>')}
      <section class="card"><h2>Publishing</h2><p>Share links используют token-based download path, TTL и audit download counter. Из этого экрана можно публиковать ссылку на конкретный artifact, открыть ее и revoke без ручных API вызовов.</p></section>`;
    const createBtn = document.getElementById('shareLinkCreateBtn');
    if (createBtn) createBtn.addEventListener('click', () => openShareLinkPublishModal());
    document.querySelectorAll('.sharelink-open-btn').forEach((button) => {
      button.addEventListener('click', () => {
        if (button.dataset.url && button.dataset.url !== 'n/a') window.open(button.dataset.url, '_blank', 'noopener,noreferrer');
      });
    });
    document.querySelectorAll('.sharelink-revoke-btn').forEach((button) => {
      button.addEventListener('click', () => revokeShareLinkAction(button.dataset.clientId, button.dataset.linkId));
    });
  }

  async function renderAudit() {
    setTitle('Audit');
    el('content').innerHTML = '<section class="card"><h2>Audit Trail</h2><div class="empty">Loading recent audit events...</div></section>';
    try {
      const events = await requestJSON('/api/v1/audit?limit=200');
      if (state.page !== 'audit') return;
      const rows = (events || []).map((event) => ({
        created: event.created_at,
        actor: event.actor_type || 'system',
        action: event.action || 'unknown',
        resource: `${event.resource_type || 'resource'}:${event.resource_id || 'n/a'}`,
        summary: event.summary || 'n/a',
      }));
      el('content').innerHTML = `
        <div class="grid cols-4">
          ${metric('Events', String(rows.length), 'recent audit records')}
          ${metric('Actors', String(new Set(rows.map((row) => row.actor)).size), 'distinct actor types')}
          ${metric('Resources', String(new Set(rows.map((row) => row.resource)).size), 'resource targets in the feed')}
          ${metric('Latest', rows[0] ? formatRelativeDate(rows[0].created) : 'n/a', 'most recent event')}
        </div>
        ${tableCard('Audit Events', rows, [
          { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
          { title: 'Actor', key: 'actor', render: (row) => `<span class="tag">${escapeHTML(row.actor)}</span>` },
          { title: 'Action', key: 'action', render: (row) => `<code>${escapeHTML(row.action)}</code>` },
          { title: 'Resource', key: 'resource' },
          { title: 'Summary', key: 'summary' },
        ])}
        <section class="card"><h2>Scope</h2><p>Лента строится напрямую из <code>audit_events</code> и уже включает operator actions, job orchestration, share-link publishing и service access rotations.</p></section>`;
    } catch (err) {
      if (state.page !== 'audit') return;
      el('content').innerHTML = `<section class="card"><h2>Audit Trail</h2><div class="empty">Failed to load audit feed: ${escapeHTML(err.message)}</div></section>`;
    }
  }

  function renderTelemetry() {
    setTitle('Telemetry');
    const nodes = state.nodes || [];
    const jobs = state.jobs || [];
    const instances = state.instances || [];
    const onlineNodes = nodes.filter((node) => nodeHeartbeatStatus(node) === 'online');
    const degradedNodes = nodes.filter((node) => nodeHeartbeatStatus(node) === 'degraded');
    const offlineNodes = nodes.filter((node) => nodeHeartbeatStatus(node) === 'offline');
    const runningJobs = jobs.filter((job) => ['queued', 'running', 'retrying'].includes(String(job.status || '').toLowerCase()));
    const failedJobs = jobs.filter((job) => String(job.status || '').toLowerCase() === 'failed');
    const provisioningInstances = instances.filter((instance) => ['draft', 'provisioning', 'apply'].includes(String(instance.status || '').toLowerCase()));
    const nodeRows = nodes.map((node) => ({
      name: node.name || 'n/a',
      address: node.address || 'n/a',
      heartbeat: nodeHeartbeatStatus(node),
      last: node.last_heartbeat_at,
      agent: node.agent_status || 'unknown',
      mode: node.execution_mode || 'n/a',
    }));
    const failedJobRows = failedJobs.slice(0, 15).map((job) => ({
      created: job.created_at,
      type: job.type || 'unknown',
      scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
      status: job.status || 'failed',
    }));
    el('content').innerHTML = `
      <div class="grid cols-4">
        ${metric('Nodes Online', String(onlineNodes.length), `${degradedNodes.length} degraded / ${offlineNodes.length} offline`)}
        ${metric('Running Jobs', String(runningJobs.length), 'queue pressure and active orchestration')}
        ${metric('Failed Jobs', String(failedJobs.length), 'requires operator follow-up')}
        ${metric('Pending Applies', String(provisioningInstances.length), 'instances waiting for convergence')}
      </div>
      ${tableCard('Node Heartbeats', nodeRows, [
        { title: 'Node', key: 'name' },
        { title: 'Address', key: 'address' },
        { title: 'Heartbeat', key: 'heartbeat', render: (row) => statusTag(row.heartbeat) },
        { title: 'Last Seen', key: 'last', render: (row) => formatDate(row.last) },
        { title: 'Agent', key: 'agent', render: (row) => statusTag(row.agent) },
        { title: 'Mode', key: 'mode', render: (row) => `<span class="tag">${escapeHTML(row.mode)}</span>` },
      ])}
      ${tableCard('Recent Failed Jobs', failedJobRows, [
        { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
        { title: 'Type', key: 'type', render: (row) => `<code>${escapeHTML(row.type)}</code>` },
        { title: 'Scope', key: 'scope' },
        { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
      ])}
      <section class="card"><h2>Operational View</h2><p>Telemetry page агрегирует текущие heartbeat signals, instance convergence state и job failures без ручного перехода по diagnostics и job logs.</p></section>`;
  }

  async function renderRevisions() {
    setTitle('Revisions');
    const instances = state.instances || [];
    if (!instances.length) {
      el('content').innerHTML = '<section class="card"><h2>Instance Revisions</h2><div class="empty">No managed instances available yet.</div></section>';
      return;
    }
    const selectedInstanceID = state.revisionsInstanceID && instances.some((instance) => instance.id === state.revisionsInstanceID)
      ? state.revisionsInstanceID
      : instances[0].id;
    state.revisionsInstanceID = selectedInstanceID;
    localStorage.setItem('megavpn.revisionsInstanceID', selectedInstanceID);
    const selectedInstance = instances.find((instance) => instance.id === selectedInstanceID) || instances[0];
    el('content').innerHTML = `
      <section class="card">
        <div class="table-head">
          <h2>Instance Revisions</h2>
          <div class="table-tools">
            <select id="revisionsInstanceSelect">${instances.map((instance) => `<option value="${escapeHTML(instance.id)}"${instance.id === selectedInstanceID ? ' selected' : ''}>${escapeHTML(instance.name)} · ${escapeHTML(instance.service_code || 'unknown')}</option>`).join('')}</select>
          </div>
        </div>
        <div class="grid cols-4" style="margin-top:14px">
          ${metric('Instance', selectedInstance.name || 'n/a', selectedInstance.service_code || 'unknown')}
          ${metric('Status', selectedInstance.status || 'unknown', selectedInstance.systemd_unit || 'no-systemd-unit')}
          ${metric('Endpoint', selectedInstance.endpoint_host || 'n/a', String(selectedInstance.endpoint_port || 0))}
          ${metric('Node', selectedInstance.node_id || 'n/a', selectedInstance.slug || 'no-slug')}
        </div>
        <div class="form-result" id="revisionsResult" style="margin-top:16px"><div class="empty">Loading revision history...</div></div>
      </section>`;
    const select = document.getElementById('revisionsInstanceSelect');
    if (select) {
      select.addEventListener('change', () => {
        state.revisionsInstanceID = select.value;
        localStorage.setItem('megavpn.revisionsInstanceID', state.revisionsInstanceID);
        renderRevisions();
      });
    }
    try {
      const revisions = await requestJSON(`/api/v1/instances/${selectedInstanceID}/revisions?limit=20`);
      if (state.page !== 'revisions' || state.revisionsInstanceID !== selectedInstanceID) return;
      const rows = (revisions || []).map((revision) => ({
        revision_no: revision.revision_no,
        status: revision.status || 'unknown',
        source: revision.source || 'n/a',
        created: revision.created_at,
        applied: revision.applied_at,
        summary: `spec keys ${Object.keys(revision.spec || {}).length}`,
      }));
      const target = document.getElementById('revisionsResult');
      if (!target) return;
      target.innerHTML = tableCard('Revision Timeline', rows, [
        { title: 'Revision', key: 'revision_no', render: (row) => `<strong>#${escapeHTML(row.revision_no)}</strong>` },
        { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
        { title: 'Source', key: 'source', render: (row) => `<code>${escapeHTML(row.source)}</code>` },
        { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
        { title: 'Applied', key: 'applied', render: (row) => formatDate(row.applied) },
        { title: 'Summary', key: 'summary' },
      ]);
    } catch (err) {
      if (state.page !== 'revisions' || state.revisionsInstanceID !== selectedInstanceID) return;
      const target = document.getElementById('revisionsResult');
      if (target) target.innerHTML = `<div class="empty">Failed to load revision history: ${escapeHTML(err.message)}</div>`;
    }
  }

  function renderUnknownPage(key) {
    setTitle('Unknown Page');
    el('content').innerHTML = `
      <section class="card">
        <h2>Unknown route</h2>
        <p>Page "${escapeHTML(key)}" is not registered in the current Control Plane UI.</p>
      </section>`;
  }

  function groupInstallersByService(items) {
    const grouped = new Map();
    for (const item of items || []) {
      const serviceCode = String(item.service_code || '').trim();
      if (!serviceCode) continue;
      if (!grouped.has(serviceCode)) grouped.set(serviceCode, []);
      grouped.get(serviceCode).push(item);
    }
    return Array.from(grouped.entries()).map(([serviceCode, installers]) => ({ serviceCode, installers }));
  }

  function renderServiceRuntimeCard(item, node, capabilities) {
    const capability = (capabilities || []).find((entry) => entry.capability_code === item.serviceCode);
    const definition = (state.servicesCatalog || []).find((entry) => entry.code === item.serviceCode || (item.serviceCode === 'xray-core' && entry.code === 'xray'));
    return `
      <section class="card service-runtime-card">
        <div class="inline-actions" style="justify-content:space-between;align-items:flex-start">
          <div>
            <div class="mini-label">${escapeHTML(definition?.category || 'runtime')}</div>
            <h2>${escapeHTML(definition?.name || item.serviceCode)}</h2>
          </div>
          ${statusTag(capability?.status || 'missing')}
        </div>
        <p>${escapeHTML(definition?.tier ? `Tier ${definition.tier}. ` : '')}${escapeHTML(definition?.supports_install ? 'Installable runtime through agent jobs.' : 'Managed through installer catalog.')}</p>
        <div class="metric-caption">Node capability version: ${escapeHTML(capability?.version || 'n/a')}</div>
        <div class="service-strategy-list">
          ${item.installers.map((installer) => `
            <div class="service-strategy-row">
              <div>
                <div class="inline-actions" style="justify-content:flex-start;gap:10px">
                  <strong>${escapeHTML(installer.strategy)}</strong>
                  ${serviceInstallerStateTag(installer, capability)}
                </div>
                <span>${escapeHTML(installer.description || '')}</span>
              </div>
              <div class="inline-actions">
                <button class="secondary-btn service-verify-btn" type="button" data-service-code="${escapeHTML(item.serviceCode)}">Verify</button>
                <button class="primary-btn service-install-btn" type="button" data-service-code="${escapeHTML(item.serviceCode)}" data-strategy="${escapeHTML(installer.strategy || '')}" data-channel="${escapeHTML(installer.channel || '')}"${node ? '' : ' disabled'}>${escapeHTML(serviceInstallerPrimaryLabel(installer, capability))}</button>
              </div>
            </div>
          `).join('')}
        </div>
      </section>`;
  }

  function serviceInstallerPrimaryLabel(installer, capability) {
    const strategy = String(installer?.strategy || '').trim();
    const status = String(capability?.status || '').trim().toLowerCase();
    if (strategy === 'manual_present') {
      return status === 'available' ? 'Re-verify' : 'Register';
    }
    return status === 'available' ? 'Reinstall' : 'Install';
  }

  function serviceInstallerStateTag(installer, capability) {
    const strategy = String(installer?.strategy || '').trim();
    const status = String(capability?.status || '').trim().toLowerCase();
    if (strategy === 'manual_present') {
      return status === 'available' ? statusTag('detected') : '<span class="tag">manual</span>';
    }
    if (status === 'available') {
      return statusTag('installed');
    }
    if (status === 'failed') {
      return statusTag('failed');
    }
    return '<span class="tag">ready</span>';
  }

  function renderCapabilityMatrix(nodes, capabilityMap) {
    const columns = ['nginx', 'xray-core', 'openvpn', 'wireguard', 'mtproto', 'ipsec', 'http_proxy', 'xl2tpd', 'shadowsocks'];
    const header = columns.map((code) => `<th>${escapeHTML(code)}</th>`).join('');
    const rows = nodes.length
      ? nodes.map((node) => {
        const caps = capabilityMap[node.id] || [];
        return `<tr>
          <td>${escapeHTML(node.name)}</td>
          ${columns.map((code) => {
            const cap = caps.find((entry) => entry.capability_code === code);
            return `<td>${statusTag(cap?.status || 'missing')}</td>`;
          }).join('')}
        </tr>`;
      }).join('')
      : `<tr><td colspan="${columns.length + 1}"><div class="empty">No nodes available.</div></td></tr>`;
    return `<table><thead><tr><th>Node</th>${header}</tr></thead><tbody>${rows}</tbody></table>`;
  }

  function renderServiceDefinitionsTable(definitions) {
    const rows = definitions.length
      ? definitions.map((entry) => `
        <tr>
          <td>${escapeHTML(entry.code)}</td>
          <td>${escapeHTML(entry.name)}</td>
          <td>${escapeHTML(entry.category)}</td>
          <td>${escapeHTML(entry.tier)}</td>
          <td>${statusTag(entry.enabled ? 'enabled' : 'disabled')}</td>
          <td>${entry.supports_install ? statusTag('installable') : statusTag('managed')}</td>
        </tr>`).join('')
      : '<tr><td colspan="6"><div class="empty">No service definitions loaded.</div></td></tr>';
    return `<table><thead><tr><th>Code</th><th>Name</th><th>Category</th><th>Tier</th><th>Status</th><th>Install</th></tr></thead><tbody>${rows}</tbody></table>`;
  }

  function renderCapabilityEventsTable(events) {
    const rows = events.length
      ? events.map((entry) => `
        <tr>
          <td>${escapeHTML(entry.capability_code || 'n/a')}</td>
          <td>${escapeHTML(entry.strategy || 'n/a')}</td>
          <td>${statusTag(entry.status || 'unknown')}</td>
          <td>${escapeHTML(entry.summary || 'n/a')}</td>
          <td>${formatDate(entry.created_at)}</td>
        </tr>`).join('')
      : '<tr><td colspan="5"><div class="empty">No capability events yet.</div></td></tr>';
    return `<table><thead><tr><th>Capability</th><th>Strategy</th><th>Status</th><th>Summary</th><th>Created</th></tr></thead><tbody>${rows}</tbody></table>`;
  }

  async function loadServicesPageData() {
    if (!state.authUser || !state.nodes.length) {
      return;
    }
    const selectedNodeID = state.servicesNodeID || state.nodes[0]?.id || '';
    const pairs = await Promise.all(state.nodes.map(async (node) => {
      const capabilities = await fetchJSON(`/api/v1/nodes/${node.id}/capabilities`, []);
      return [node.id, capabilities || []];
    }));
    state.serviceCapabilitiesByNode = Object.fromEntries(pairs);
    if (selectedNodeID) {
      state.serviceInstallEventsByNode[selectedNodeID] = await fetchJSON(`/api/v1/nodes/${selectedNodeID}/capabilities/install-events`, []);
    }
    if (state.page === 'services') {
      renderServices();
    }
  }

  async function runServiceInstaller(serviceCode, strategy, channel) {
    if (!state.servicesNodeID) {
      openUnavailableAction('No target node', 'Select a node before installing a runtime capability.');
      return;
    }
    const node = nodeByID(state.servicesNodeID);
    openModal(`Install ${serviceCode}`, 'Capability install job', `
      <div class="card">
        <div class="mini-label">Capability operation</div>
        <div class="timeline">
          <div class="timeline-item"><strong>Target node</strong><div class="timeline-meta">${escapeHTML(node?.name || state.servicesNodeID)}${node?.address ? ` · ${escapeHTML(node.address)}` : ''}</div></div>
          <div class="timeline-item"><strong>Service</strong><div class="timeline-meta">${escapeHTML(serviceCode)}</div></div>
          <div class="timeline-item"><strong>Strategy</strong><div class="timeline-meta">${escapeHTML(strategy || 'default')}</div></div>
          <div class="timeline-item"><strong>Channel</strong><div class="timeline-meta">${escapeHTML(channel || 'default')}</div></div>
        </div>
      </div>
      <div class="modal-actions">
        <button class="secondary-btn" id="cancelInstallBtn" type="button">Cancel</button>
        <button class="primary-btn" id="confirmInstallBtn" type="button">Queue install job</button>
      </div>
      <div id="serviceInstallResult" class="form-result"></div>`);
    document.getElementById('cancelInstallBtn').addEventListener('click', closeModal);
    document.getElementById('confirmInstallBtn').addEventListener('click', async () => {
      const target = document.getElementById('serviceInstallResult');
      const confirmBtn = document.getElementById('confirmInstallBtn');
      const cancelBtn = document.getElementById('cancelInstallBtn');
      target.innerHTML = '<span class="tag warn">queueing install job</span>';
      confirmBtn.disabled = true;
      cancelBtn.disabled = true;
      try {
        const result = await sendJSON(`/api/v1/nodes/${state.servicesNodeID}/capabilities/install`, 'POST', {
          service_code: serviceCode,
          strategy,
          channel,
        });
        await watchJob(result.id, target, 'Capability install', {
          attempts: 80,
          intervalMs: 1500,
          context: {
            node: node?.name || state.servicesNodeID,
            service: serviceCode,
            strategy: strategy || 'default',
            channel: channel || 'default',
          },
        });
        await loadServicesPageData();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      } finally {
        cancelBtn.disabled = false;
      }
    });
  }

  async function verifyServiceCapability(serviceCode) {
    if (!state.servicesNodeID) {
      openUnavailableAction('No target node', 'Select a node before verifying a runtime capability.');
      return;
    }
    const node = nodeByID(state.servicesNodeID);
    try {
      openModal(`Verify ${serviceCode}`, 'Capability verification job', `
        <div class="card">
          <div class="mini-label">Capability operation</div>
          <div class="timeline">
            <div class="timeline-item"><strong>Target node</strong><div class="timeline-meta">${escapeHTML(node?.name || state.servicesNodeID)}${node?.address ? ` · ${escapeHTML(node.address)}` : ''}</div></div>
            <div class="timeline-item"><strong>Service</strong><div class="timeline-meta">${escapeHTML(serviceCode)}</div></div>
            <div class="timeline-item"><strong>Mode</strong><div class="timeline-meta">Agent verification without reinstall.</div></div>
          </div>
        </div>
        <div class="modal-actions">
          <button class="secondary-btn" id="cancelVerifyBtn" type="button">Cancel</button>
          <button class="primary-btn" id="confirmVerifyBtn" type="button">Start verification</button>
        </div>
        <div id="serviceVerifyResult" class="form-result"></div>`);
      document.getElementById('cancelVerifyBtn').addEventListener('click', closeModal);
      document.getElementById('confirmVerifyBtn').addEventListener('click', async () => {
        const target = document.getElementById('serviceVerifyResult');
        const confirmBtn = document.getElementById('confirmVerifyBtn');
        const cancelBtn = document.getElementById('cancelVerifyBtn');
        target.innerHTML = '<span class="tag warn">queueing verification job</span>';
        confirmBtn.disabled = true;
        cancelBtn.disabled = true;
        try {
          const job = await sendJSON(`/api/v1/nodes/${state.servicesNodeID}/capabilities/verify`, 'POST', { service_code: serviceCode });
          await watchJob(job.id, target, 'Capability verify', {
            attempts: 60,
            intervalMs: 1500,
            context: {
              node: node?.name || state.servicesNodeID,
              service: serviceCode,
              strategy: 'verify_only',
            },
          });
          await loadServicesPageData();
        } catch (err) {
          target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        } finally {
          cancelBtn.disabled = false;
        }
      });
    } catch (err) {
      openUnavailableAction(`Verify ${serviceCode}`, err.message);
    }
  }

  function renderSettings() {
    setTitle('Settings');
    const canManageAuth = hasPermission('auth.manage');
    const canDeleteUsers = hasRole('superadmin');
    el('content').innerHTML = `
      <section class="panel-banner">
        <div>
          <div class="eyebrow">Control Plane Security</div>
          <h2>Operator access, sessions and runtime policy</h2>
          <p>В этом разделе находится административная поверхность RTIS MegaVPN: локальные операторы, активные сессии, ротация паролей и базовые runtime-настройки панели.</p>
        </div>
        <div class="panel-banner-side">
          <div class="tag ok">${canManageAuth ? 'auth.manage' : 'readonly'}</div>
          <div class="metric-caption">API base: ${escapeHTML(state.apiBase || 'same-origin')}</div>
        </div>
      </section>
      <div class="grid cols-3">
        <div class="card spotlight-card">
          <div class="mini-label">Current Operator</div>
          <div class="metric-value" style="font-size:28px">${escapeHTML(state.authUser?.display_name || state.authUser?.username || 'unknown')}</div>
          <div class="metric-caption">${escapeHTML(state.authUser?.username || 'n/a')}</div>
          <div class="metric-caption">${escapeHTML(state.authUser?.email || 'n/a')}</div>
        </div>
        <div class="card">
          <div class="mini-label">Roles</div>
          <div class="metric-caption">${(state.authRoles || []).length} roles assigned</div>
          <div class="metric-caption">${escapeHTML((state.authRoles || []).join(', ') || 'none')}</div>
        </div>
        <div class="card">
          <div class="mini-label">Admin Access</div>
          <div class="metric-caption">${canManageAuth ? 'auth.manage granted' : 'readonly access'}</div>
          <div class="metric-caption">${canManageAuth ? (canDeleteUsers ? 'Operator creation, invite resend, delete and session revoke enabled.' : 'Operator creation, invite resend and session revoke enabled.') : 'Only profile and runtime info visible.'}</div>
        </div>
      </div>
      <div class="settings-layout">
        <section class="card">
          <h2>Runtime Configuration</h2>
          <p>Same-origin UI, local auth, HttpOnly session cookies and CSRF-protected mutating requests. UI state is rendered directly from the control plane API.</p>
          <div class="inline-actions" style="margin-top:14px">
            <button class="secondary-btn" id="openSettingsInlineBtn" type="button">API Settings</button>
          </div>
        </section>
        <section class="card">
          <h2>Change Password</h2>
          <p>Смена пароля сразу инвалидирует текущие сессии оператора. После сохранения потребуется повторный вход.</p>
          <form id="changePasswordForm" class="form-grid">
            <div class="field full"><label>Current password</label><input name="current_password" type="password" autocomplete="current-password" required /></div>
            <div class="field full"><label>New password</label><input name="new_password" type="password" autocomplete="new-password" required placeholder="minimum 12 chars" /></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Update password</button></div>
          </form>
          <div id="changePasswordResult" class="form-result"></div>
        </section>
      </div>
      <section class="table-card">
        <div class="table-head"><h2>Platform Users</h2><div class="table-tools">${canManageAuth ? '<span class="tag ok">auth.manage</span>' : '<span class="tag stub">read-only</span>'}</div></div>
        <div class="card-body" id="platformUsersMount"><div class="empty">Loading platform users...</div></div>
      </section>
      <section class="table-card">
        <div class="table-head"><h2>Mail Settings</h2><div class="table-tools"><span class="tag">${canManageAuth ? 'smtp' : 'read-only'}</span></div></div>
        <div class="card-body" id="mailSettingsMount"><div class="empty">Loading mail settings...</div></div>
      </section>
      <section class="table-card">
        <div class="table-head"><h2>Platform CA Center</h2><div class="table-tools"><span class="tag">${hasPermission('instance.read') ? 'instance.read' : 'read-only'}</span></div></div>
        <div class="card-body" id="platformPKIRootsMount"><div class="empty">Loading PKI roots...</div></div>
      </section>
      <section class="table-card">
        <div class="table-head"><h2>Operator Invites</h2><div class="table-tools"><span class="tag">${canManageAuth ? 'email delivery' : 'read-only'}</span></div></div>
        <div class="card-body" id="platformInvitesMount"><div class="empty">Loading invites...</div></div>
      </section>
      <section class="table-card">
        <div class="table-head"><h2>Active Sessions</h2><div class="table-tools"><span class="tag">${escapeHTML(state.authUser?.username || 'operator')}</span></div></div>
        <div class="card-body" id="platformSessionsMount"><div class="empty">Loading active sessions...</div></div>
      </section>`;
    document.getElementById('openSettingsInlineBtn').addEventListener('click', openSettings);
    document.getElementById('changePasswordForm').addEventListener('submit', changeOwnPassword);
    void loadAdminSettings(canManageAuth, canDeleteUsers);
  }

  function renderCertificates() {
    setTitle('Certificates');
    const certRows = Array.isArray(state.platformCertificates) ? state.platformCertificates : [];
    const rootRows = Array.isArray(state.platformPKIRoots) ? state.platformPKIRoots : [];
    const activeLeafs = activeLeafCertificates();
    const managedCAs = activeManagedAuthorities();
    el('content').innerHTML = `
      <section class="panel-banner">
        <div>
          <div class="eyebrow">TLS & PKI</div>
          <h2>Certificates</h2>
          <p>Управление leaf-сертификатами, внутренними managed CA и service-specific CA roots. Leaf certificates можно назначать на Nginx edge и Xray TLS instances, а OpenVPN продолжает использовать platform CA center.</p>
        </div>
        <div class="panel-banner-side">
          <div class="tag ok">${escapeHTML(String(activeLeafs.length))} leaf certificates</div>
          <div class="metric-caption">${escapeHTML(String(managedCAs.length))} managed CAs · ${escapeHTML(String(rootRows.length))} service CA roots</div>
        </div>
      </section>
      ${tableCard('Certificate Inventory', certRows.map((item) => ({
        id: item.id,
        name: item.name || item.common_name || item.id,
        kind: item.kind || 'leaf',
        source: item.source || 'unknown',
        commonName: item.common_name || 'n/a',
        dnsNames: Array.isArray(item.sans) && item.sans.length ? item.sans.join(', ') : 'n/a',
        issuer: item.issuer_name || 'self',
        expires: item.not_after ? formatDate(item.not_after) : 'n/a',
        status: item.status || 'unknown',
        defaultTag: item.is_default ? 'default' : '',
      })), [
        { title: 'Name', key: 'name', render: (r) => `${escapeHTML(r.name)}${r.defaultTag ? ' <span class="tag ok">default</span>' : ''}` },
        { title: 'Kind', key: 'kind', render: (r) => `<span class="tag">${escapeHTML(r.kind)}</span>` },
        { title: 'Source', key: 'source', render: (r) => `<span class="tag">${escapeHTML(r.source)}</span>` },
        { title: 'Common Name', key: 'commonName' },
        { title: 'SAN / DNS', key: 'dnsNames' },
        { title: 'Issuer', key: 'issuer' },
        { title: 'Expires', key: 'expires' },
        { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
      ], '<button class="secondary-btn" id="createCertificateBtn" type="button">Add certificate</button>')}
      <div class="grid cols-2">
        <section class="table-card">
          <div class="table-head"><h2>Managed CA Inventory</h2><div class="table-tools"><span class="tag">${escapeHTML(String(managedCAs.length))} generic CAs</span></div></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Name</th><th>Common Name</th><th>Expires</th><th>Status</th></tr></thead>
              <tbody>
                ${managedCAs.length ? managedCAs.map((item) => `
                  <tr>
                    <td>${escapeHTML(item.name || item.id)}</td>
                    <td>${escapeHTML(item.common_name || 'n/a')}</td>
                    <td>${escapeHTML(item.not_after ? formatDate(item.not_after) : 'n/a')}</td>
                    <td>${statusTag(item.status || 'unknown')}</td>
                  </tr>`).join('') : '<tr><td colspan="4"><div class="empty">No managed certificate authorities yet.</div></td></tr>'}
              </tbody>
            </table>
          </div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Service CA Center</h2><div class="table-tools"><span class="tag">${escapeHTML(String(rootRows.length))} roots</span></div></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Service</th><th>Profile</th><th>Common Name</th><th>Status</th><th>Created</th></tr></thead>
              <tbody>
                ${rootRows.length ? rootRows.map((root) => `
                  <tr>
                    <td>${escapeHTML(root.service_code || 'n/a')}</td>
                    <td>${escapeHTML(root.pki_profile || 'default')}</td>
                    <td>${escapeHTML(root.common_name || 'n/a')}</td>
                    <td>${statusTag(root.status || 'unknown')}</td>
                    <td>${formatDate(root.created_at)}</td>
                  </tr>`).join('') : '<tr><td colspan="5"><div class="empty">No service CA roots yet.</div></td></tr>'}
              </tbody>
            </table>
          </div>
        </section>
      </div>`;
    const btn = document.getElementById('createCertificateBtn');
    if (btn) btn.addEventListener('click', openCreateCertificateWizard);
  }

  async function loadAdminSettings(canManageAuth, canDeleteUsers = hasRole('superadmin')) {
    try {
      const userList = canManageAuth ? await requestJSON('/api/v1/admin/users') : [{ ...state.authUser, roles: state.authRoles || [] }];
      const sessions = canManageAuth ? await requestJSON('/api/v1/admin/sessions') : [];
      const mailSettings = canManageAuth ? await requestJSON('/api/v1/settings/mail') : null;
      const pkiRoots = hasPermission('instance.read') ? await requestJSON('/api/v1/platform/pki-roots') : [];
      const invites = canManageAuth ? await requestJSON('/api/v1/admin/user-invites') : [];
      if (state.page !== 'settings') return;
      state.mailSettings = mailSettings;
      state.platformInvites = invites || [];
      state.platformPKIRoots = pkiRoots || [];
      renderPlatformUsers(userList || [], canManageAuth, canDeleteUsers);
      renderMailSettings(mailSettings, canManageAuth);
      renderPlatformPKIRoots(pkiRoots || [], hasPermission('instance.read'));
      renderPlatformInvites(invites || [], canManageAuth);
      renderPlatformSessions(sessions || [], canManageAuth);
    } catch (err) {
      if (state.page !== 'settings') return;
      document.getElementById('platformUsersMount').innerHTML = `<div class="empty">Failed to load admin data: ${escapeHTML(err.message)}</div>`;
      document.getElementById('mailSettingsMount').innerHTML = `<div class="empty">Failed to load mail settings: ${escapeHTML(err.message)}</div>`;
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
          <td>${statusTag(root.status || 'unknown')}</td>
          <td>${escapeHTML(root.common_name || 'n/a')}</td>
          <td><code title="${escapeHTML(root.ca_cert_secret_ref_id || '')}">${escapeHTML(compactID(root.ca_cert_secret_ref_id))}</code></td>
          <td>${formatDate(root.created_at)}</td>
          <td>${formatDate(root.rotated_at)}</td>
        </tr>`)
        .join('')
      : '<tr><td colspan="7"><div class="empty">No platform CA roots yet. The OpenVPN default CA is created on first OpenVPN apply or client provisioning.</div></td></tr>';
    mount.innerHTML = `
      <div class="metric-caption" style="margin-bottom:12px">Control plane хранит platform PKI inventory для сервисов, где CA lifecycle должен быть централизован. Сейчас production path активен для OpenVPN.</div>
      <div class="table-wrap">
        <table>
          <thead><tr><th>Service</th><th>Profile</th><th>Status</th><th>Common Name</th><th>CA Cert Ref</th><th>Created</th><th>Rotated</th></tr></thead>
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
    el('content').innerHTML = `
      <section class="login-shell">
        <div class="login-hero">
          <div class="eyebrow">Distributed VPN & Edge Platform</div>
          <h2>Secure control plane for nodes, artifacts and remote bootstrap</h2>
          <p>RTIS MegaVPN управляет удаленными хостами через единый control plane: локальные операторы, bootstrap через SSH, agent pull model, аудит и централизованный runtime.</p>
          <div class="login-points">
            <div class="login-point"><strong>Auth</strong><span>Local operator auth, HttpOnly session cookies, CSRF-protected writes.</span></div>
            <div class="login-point"><strong>Bootstrap</strong><span>SSH access methods, secret refs, enrollment token flow.</span></div>
            <div class="login-point"><strong>Operations</strong><span>HA API/worker model, audit trail and controlled lifecycle actions.</span></div>
          </div>
        </div>
        <div class="auth-card">
          <div class="eyebrow">Control Plane Access</div>
          <h2>Sign in to RTIS MegaVPN</h2>
          <p>Используй логин оператора. Для same-origin режима API base URL можно не указывать.</p>
          <form id="loginForm" class="form-grid">
            <div class="field full"><label>Login</label><input name="login" type="text" autocomplete="username" required placeholder="operator login" /></div>
            <div class="field full"><label>Password</label><input name="password" type="password" autocomplete="current-password" required placeholder="••••••••" /></div>
            <div class="field full inline-actions">
              <button class="primary-btn" type="submit">Login</button>
              <button class="secondary-btn" type="button" id="loginSettingsBtn">API Settings</button>
            </div>
          </form>
          <div id="loginResult" class="auth-message"></div>
        </div>
      </section>`;
    document.getElementById('loginForm').addEventListener('submit', login);
    document.getElementById('loginSettingsBtn').addEventListener('click', openSettings);
  }

  function renderInviteAcceptScreen() {
    const invite = state.invitePreview || {};
    setTitle('Invitation');
    el('content').innerHTML = `
      <section class="login-shell">
        <div class="login-hero">
          <div class="eyebrow">RTIS MegaVPN Invitation</div>
          <h2>Complete operator onboarding</h2>
          <p>Одноразовая ссылка задает пароль и сразу открывает operator session.</p>
          <div class="login-points">
            <div class="login-point"><strong>User</strong><span>${escapeHTML(invite.username || 'unknown')}</span></div>
            <div class="login-point"><strong>Email</strong><span>${escapeHTML(invite.email || 'n/a')}</span></div>
            <div class="login-point"><strong>Status</strong><span>${escapeHTML(invite.status || 'pending')}</span></div>
          </div>
        </div>
        <div class="auth-card">
          <div class="eyebrow">One-time access</div>
          <h2>Set your password</h2>
          <form id="inviteAcceptForm" class="form-grid">
            <div class="field full"><label>Password</label><input name="password" type="password" autocomplete="new-password" required placeholder="minimum 12 chars" /></div>
            <div class="field full inline-actions">
              <button class="primary-btn" type="submit">Activate account</button>
              <button class="secondary-btn" type="button" id="inviteBackBtn">Back to login</button>
            </div>
          </form>
          <div id="inviteAcceptResult" class="auth-message"></div>
        </div>
      </section>`;
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await loadAdminSettings(true);
      setTimeout(closeModal, 700);
    } catch (err) {
      const details = err?.payload ? `\n${JSON.stringify(err.payload, null, 2)}` : '';
      target.innerHTML = `<div class="code-block">${escapeHTML(`${err.message}${details}`)}</div>`;
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
      const details = err?.payload ? `\n${JSON.stringify(err.payload, null, 2)}` : '';
      target.innerHTML = `<div class="code-block">${escapeHTML(`${err.message}${details}`)}</div>`;
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
    openModal('Add node', 'POST /api/v1/nodes', `
      <form id="createNodeForm" class="form-grid">
        <div class="field"><label>Name</label><input name="name" required placeholder="edge-01" /></div>
        <div class="field"><label>Role</label><select name="role"><option value="egress">egress</option><option value="ingress">ingress</option></select></div>
        <div class="field"><label>Kind</label><select name="kind"><option value="remote">remote</option><option value="local">local</option></select></div>
        <div class="field"><label>Execution mode</label><select name="execution_mode"><option value="agent_managed">agent_managed</option><option value="ssh_bootstrap">ssh_bootstrap</option><option value="manual_bundle">manual_bundle</option><option value="local_managed">local_managed</option></select></div>
        <div class="field full"><label>Address</label><input name="address" required placeholder="203.0.113.10" /></div>
        <div class="field"><label>OS family</label><input name="os_family" value="ubuntu" /></div>
        <div class="field"><label>OS version</label><input name="os_version" value="24.04" /></div>
        <div class="field"><label>Architecture</label><select name="architecture"><option value="amd64">amd64</option><option value="arm64">arm64</option></select></div>
        <div class="field full"><button class="primary-btn" type="submit">Create node</button></div>
      </form>
      <div id="createNodeResult" style="margin-top:14px"></div>`);
    document.getElementById('createNodeForm').addEventListener('submit', createNode);
  }

  async function createNode(event) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const payload = Object.fromEntries(form.entries());
    const target = document.getElementById('createNodeResult');
    target.innerHTML = '<span class="tag warn">sending</span>';
    try {
      const data = await sendJSON('/api/v1/nodes', 'POST', payload);
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function openNodeControlModal(nodeID) {
    const node = state.nodes.find((item) => item.id === nodeID);
    if (!node) return;
    openModal(`Node: ${node.name}`, 'Bootstrap & access management', '<div class="empty">Loading node details...</div>', { wide: true });
    try {
      const [diag, methods, runs, tokens] = await Promise.all([
        requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`),
        requestJSON(`/api/v1/nodes/${nodeID}/access-methods`),
        requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`),
        requestJSON(`/api/v1/nodes/${nodeID}/enrollment-tokens`),
      ]);
      renderNodeControlModal(diag?.node || node, diag, methods, runs, tokens, '');
    } catch (err) {
      el('modalBody').innerHTML = `<div class="empty">Failed to load node details: ${escapeHTML(err.message)}</div>`;
    }
  }

  function renderNodeControlModal(node, diag, methods, runs, tokens, flash) {
    const heartbeatStatus = diag?.heartbeat_state || nodeHeartbeatStatus(node);
    const heartbeatDrift = diag?.heartbeat_drift_seconds;
    const sshMethod = methods.find((item) => item.method === 'ssh') || null;
    const enabledMethods = methods.filter((item) => item.is_enabled);
    const latestInventory = diag?.latest_inventory || null;
    const inventoryPayload = latestInventory?.payload || {};
    const discoverySummary = diag?.discovery_summary || { total: 0, available: 0, imported: 0, ignored: 0, by_service: {} };
    const recentDiscoveries = Array.isArray(diag?.recent_discoveries) ? diag.recent_discoveries : [];
    const agent = diag?.agent || {};
    const methodRows = methods.length
      ? methods.map((item) => `
          <tr>
            <td>${escapeHTML(item.method)}</td>
            <td>${statusTag(item.is_enabled ? 'enabled' : 'disabled')}</td>
            <td>${escapeHTML(item.ssh_host || 'n/a')}</td>
            <td>${escapeHTML(item.ssh_user || 'n/a')}</td>
            <td>${escapeHTML(item.auth_type || 'n/a')}</td>
            <td>${escapeHTML(item.secret_ref_id || 'n/a')}</td>
          </tr>`).join('')
      : '<tr><td colspan="6"><div class="empty">Access methods are not configured yet.</div></td></tr>';
    const runRows = runs.length
      ? runs.map((item) => `
          <tr>
            <td>${escapeHTML(item.bootstrap_mode)}</td>
            <td>${statusTag(item.status)}</td>
            <td>${formatDate(item.started_at || item.created_at)}</td>
            <td>${formatDate(item.finished_at)}</td>
            <td><button class="secondary-btn bootstrap-run-view-btn" type="button" data-run-id="${escapeHTML(item.id)}" data-job-id="${escapeHTML(item.job_id || '')}">View</button></td>
          </tr>`).join('')
      : '<tr><td colspan="5"><div class="empty">No bootstrap runs yet.</div></td></tr>';
    const tokenRows = tokens.length
      ? tokens.map((item) => `
          <tr>
            <td>${escapeHTML(item.token_hint || item.token || 'n/a')}</td>
            <td>${statusTag(item.status || 'active')}</td>
            <td>${formatDate(item.expires_at)}</td>
            <td>${formatDate(item.used_at)}</td>
          </tr>`).join('')
      : '<tr><td colspan="4"><div class="empty">No enrollment tokens created yet.</div></td></tr>';
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

    el('modalBody').innerHTML = `
      <div class="overview-grid">
        <div class="fact-card emphasis-card"><div class="mini-label">Node</div><div class="metric-caption strong">${escapeHTML(node.name)}</div><div class="metric-caption">${escapeHTML(node.address)}</div></div>
        <div class="fact-card"><div class="mini-label">Execution</div><div class="metric-caption strong">${escapeHTML(node.execution_mode || 'unknown')}</div><div class="metric-caption">${escapeHTML(node.kind || 'remote')} · ${escapeHTML(node.role || 'egress')}</div></div>
        <div class="fact-card"><div class="mini-label">Agent channel</div><div class="metric-caption strong">${escapeHTML(diagnosticsAgentState(diag))}</div><div class="metric-caption">${escapeHTML(diag?.communication_state || 'unknown')}</div></div>
        <div class="fact-card"><div class="mini-label">Heartbeat</div><div class="metric-caption strong">${escapeHTML(heartbeatStatus)}</div><div class="metric-caption">${escapeHTML(heartbeatDrift == null ? formatRelativeDate(node.last_heartbeat_at) : formatDurationSeconds(heartbeatDrift))}</div></div>
        <div class="fact-card"><div class="mini-label">Inventory</div><div class="metric-caption strong">${escapeHTML(formatRelativeDate(inventoryCollectedAt))}</div><div class="metric-caption">${escapeHTML(inventoryLabel(inventoryPayload, 'os.pretty_name', `${node.os_family || 'linux'} ${node.os_version || ''}`))}</div></div>
        <div class="fact-card"><div class="mini-label">Discovery</div><div class="metric-caption strong">${escapeHTML(String(discoverySummary.total || 0))} items</div><div class="metric-caption">${escapeHTML(serviceMix)}</div></div>
      </div>
      ${flash ? `<div class="notice subtle-notice">${escapeHTML(flash)}</div>` : ''}
      <div class="node-modal-grid">
        <div class="stack">
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
              <div class="fact-card emphasis-card" style="margin-bottom:16px">
                <div class="mini-label">Communication hint</div>
                <div class="metric-caption strong">${escapeHTML(diag?.communication_hint || 'n/a')}</div>
                <div class="metric-caption">claim job ${escapeHTML(agent.last_job_claim_job_id || 'n/a')} · result ${escapeHTML(agent.last_job_result_status || 'n/a')}</div>
              </div>
              <div class="grid cols-3">
                <div class="card"><div class="mini-label">Communication state</div><div class="metric-caption">${statusTag(diag?.communication_state || 'unknown')}</div><div class="metric-caption">${escapeHTML(diag?.communication_hint || 'n/a')}</div></div>
          ${commMetricLine('Last auth failure', agent.last_auth_failure_at, agent.last_auth_failure_reason || 'none')}
          ${commMetricLine('Last job poll', agent.last_job_poll_at, 'agent/jobs/next')}
          ${commMetricLine('Last job claim', agent.last_job_claim_at, `${agent.last_job_claim_type || 'n/a'} ${agent.last_job_claim_job_id || ''}`.trim())}
          ${commMetricLine('Last job submit', agent.last_job_result_at, `${agent.last_job_result_type || 'n/a'} ${agent.last_job_result_status || ''}`.trim())}
          ${commMetricLine('Last inventory sync', agent.last_inventory_sync_at, 'agent/inventory')}
          ${commMetricLine('Last discovery sync', agent.last_discovery_sync_at, 'node.services.discover')}
              </div>
              <div class="section-divider"></div>
              <div class="section-actions">
                <button class="secondary-btn" id="retryInventorySyncBtn" type="button">Retry inventory sync</button>
                <button class="secondary-btn" id="retryDiscoverySyncBtn" type="button">Retry discovery sync</button>
                <button class="secondary-btn" id="probeNodeChannelBtn" type="button">Channel probe</button>
                <button class="secondary-btn" id="requeueStuckNodeJobBtn" type="button"${canRequeueStuckJob ? '' : ' disabled'}>Requeue stuck job</button>
                <button class="secondary-btn" id="clearStaleRotationBtn" type="button"${canClearStaleRotation ? '' : ' disabled'}>Clear stale pending rotation</button>
              </div>
              <div id="nodeDiagnosticsActionResult" class="form-result"></div>
              <div class="code-block">claim_job_id = ${escapeHTML(agent.last_job_claim_job_id || 'n/a')}
result_job_id = ${escapeHTML(agent.last_job_result_job_id || 'n/a')}
claim_type = ${escapeHTML(agent.last_job_claim_type || 'n/a')}
result_type = ${escapeHTML(agent.last_job_result_type || 'n/a')}
result_status = ${escapeHTML(agent.last_job_result_status || 'n/a')}</div>
            </div>
          </section>
          ${renderInventorySnapshotPanel(latestInventory, node)}
          <section class="table-card compact-card">
            <div class="table-head"><h2>Recent Discovered Services</h2><span class="tag">${escapeHTML(String(recentDiscoveries.length))} visible</span></div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Service</th><th>Name</th><th>Status</th><th>Endpoint</th><th>Detected</th></tr></thead>
                <tbody>${recentDiscoveryRows}</tbody>
              </table>
            </div>
          </section>
        </div>
        <div class="stack">
          <section class="section-card">
            <div class="section-head">
              <div>
                <div class="eyebrow">Runtime Profile</div>
                <h2>Host Diagnostics</h2>
              </div>
              <div class="section-meta">${statusTag(node.status || 'draft')}</div>
            </div>
            <div class="section-body">
              <div class="inventory-facts">
                ${renderInventoryFact('Agent version', agent.agent_version || 'n/a', `protocol ${agent.protocol_version || 'n/a'}`)}
                ${renderInventoryFact('Agent fingerprint', shortFingerprint(agent.fingerprint), formatDate(agent.registered_at))}
                ${renderInventoryFact('Enabled methods', String(enabledMethods.length), enabledMethods.map((item) => item.method).join(', ') || 'none')}
                ${renderInventoryFact('Token rotation', agent.token_rotation_status || 'missing', agent.token_hint || diag?.active_enrollment_token?.token_hint || 'n/a')}
                ${renderInventoryFact('Last successful bootstrap', diag?.last_successful_bootstrap?.status || 'none', latestBootstrapSummary(diag?.last_successful_bootstrap))}
                ${renderInventoryFact('Last failed bootstrap', diag?.last_failed_bootstrap?.status || 'none', latestBootstrapSummary(diag?.last_failed_bootstrap))}
              </div>
              <div class="code-block" style="margin-top:16px">agent.last_seen_at = ${escapeHTML(formatDate(agent.last_seen_at))}
agent.registered_at = ${escapeHTML(formatDate(agent.registered_at))}
agent.revoked_at = ${escapeHTML(formatDate(agent.revoked_at))}
latest_bootstrap = ${escapeHTML(latestBootstrapSummary(diag?.last_bootstrap))}
active_enrollment_token = ${escapeHTML(diag?.active_enrollment_token?.token_hint || 'none')}</div>
            </div>
          </section>
          <section class="card">
            <h2>Runtime Actions</h2>
            <p>Operational actions that do not require touching SSH bootstrap state.</p>
            <div class="inline-actions">
              ${node.status === 'maintenance'
                ? '<button class="secondary-btn" id="nodeMaintenanceToggleBtn" type="button">Disable maintenance</button>'
                : '<button class="secondary-btn" id="nodeMaintenanceToggleBtn" type="button">Enable maintenance</button>'}
              <button class="secondary-btn" id="refreshNodeRuntimeBtn" type="button">Refresh diagnostics</button>
            </div>
            <div id="nodeRuntimeActionResult" class="form-result"></div>
          </section>
          <section class="card">
            <h2>Enrollment Tokens</h2>
            <p>Use an enrollment token for first registration or controlled re-enroll. After that the node should live on the agent channel only.</p>
            <div class="form-grid">
              <div class="field"><label>TTL hours</label><input id="enrollmentTtlHours" type="number" min="1" max="720" value="24" /></div>
              <div class="field inline-actions align-end"><button class="secondary-btn" id="createEnrollmentTokenBtn" type="button">Rotate token</button></div>
            </div>
            <div id="enrollmentTokenResult" class="form-result"></div>
            <div class="table-wrap" style="margin-top:14px">
              <table>
                <thead><tr><th>Token hint</th><th>Status</th><th>Expires</th><th>Used</th></tr></thead>
                <tbody>${tokenRows}</tbody>
              </table>
            </div>
          </section>
        </div>
      </div>
      <section class="table-card compact-card">
        <div class="table-head"><h2>Access Methods</h2><span class="tag">${escapeHTML(String(methods.length))} configured</span></div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Method</th><th>Status</th><th>Host</th><th>User</th><th>Auth</th><th>Secret Ref</th></tr></thead>
            <tbody>${methodRows}</tbody>
          </table>
        </div>
      </section>
      <section class="card">
        <h2>SSH Bootstrap Access</h2>
        <p>SSH используется только для первичной установки и регистрации агента. После этого все lifecycle operations должны идти через agent channel.</p>
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
            <button class="danger-btn" type="button" id="removeSshAccessBtn">Remove SSH access</button>
          </div>
        </form>
        <div id="sshAccessResult" class="form-result"></div>
      </section>
      <section class="grid cols-2">
        <section class="card">
          <h2>Bootstrap Job</h2>
          <p>Queue bootstrap only after SSH access and enrollment token are prepared. Successful bootstrap should transition into real agent registration and heartbeat, not just a finished worker job.</p>
          <div class="form-grid">
            <div class="field"><label>Bootstrap mode</label><select id="bootstrapMode"><option value="ssh_bootstrap">ssh_bootstrap</option><option value="manual_bundle">manual_bundle</option></select></div>
            <div class="field inline-actions align-end"><button class="primary-btn" id="queueBootstrapBtn" type="button">Queue bootstrap</button><button class="secondary-btn" id="reinstallAgentBtn" type="button">Reinstall agent</button><button class="secondary-btn" id="reenrollAgentBtn" type="button">Re-enroll agent</button><button class="secondary-btn" id="refreshNodeRuntimeBtn" type="button">Refresh</button></div>
          </div>
          <div id="bootstrapJobResult" class="form-result"></div>
          <div class="table-wrap" style="margin-top:14px">
            <table>
              <thead><tr><th>Mode</th><th>Status</th><th>Started</th><th>Finished</th><th>Inspect</th></tr></thead>
              <tbody>${runRows}</tbody>
            </table>
          </div>
        </section>
      </section>
      <section class="card">
        <h2>Agent Trust Lifecycle</h2>
        <p>Управление trust plane для удаленного хоста: rotation bearer token, enrollment rotation, revoke identity и controlled re-enroll до нового heartbeat.</p>
        <div class="inline-actions">
          <button class="secondary-btn" id="rotateAgentTokenBtn" type="button">Rotate agent token</button>
          <button class="secondary-btn" id="rotateEnrollmentTokenBtn" type="button">Rotate enrollment token</button>
          <button class="danger-btn" id="revokeAgentIdentityBtn" type="button">Revoke agent identity</button>
        </div>
        <div id="nodeTrustResult" class="form-result"></div>
      </section>
      <section class="card">
        <h2>Channel Notes</h2>
        <div class="code-block">Remote host control path:
1. SSH bootstrap installs megavpn-agent and writes bootstrap/env files.
2. Agent enrolls against ${escapeHTML(state.apiBase || window.location.origin)} and receives persistent agent_token.
3. Control plane waits for real heartbeat and then sends inventory / runtime jobs through the agent pull channel.</div>
      </section>`;

    document.getElementById('sshAccessForm').addEventListener('submit', (event) => saveSSHAccess(event, node, methods));
    document.getElementById('removeSshAccessBtn').addEventListener('click', () => removeSSHAccess(node, methods));
    document.getElementById('retryInventorySyncBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'inventory'));
    document.getElementById('retryDiscoverySyncBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'discover'));
    document.getElementById('probeNodeChannelBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'probe'));
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
    await refresh();
    renderNodeControlModal(diag?.node || node, diag, methods, runs, tokens, flash);
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
      result.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(token, null, 2))}</div>`;
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
      result.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(token, null, 2))}</div>`;
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
      result.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
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
          <div class="code-block">${escapeHTML(JSON.stringify(run.result_payload || {}, null, 2))}</div>
        </div>
        <div class="card">
          <h2>Worker logs</h2>
          <div class="code-block">${escapeHTML(logLines || 'No logs captured for this bootstrap job yet.')}</div>
        </div>`;
    } catch (err) {
      el('modalBody').innerHTML = `<div class="empty">Failed to load bootstrap run details: ${escapeHTML(err.message)}</div>`;
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
        target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await reloadNodeControlModal(node.id, 'Node maintenance state updated.');
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function login(event) {
    event.preventDefault();
    const target = document.getElementById('loginResult');
    target.innerHTML = '<span class="tag warn">authorizing</span>';
    try {
      const form = new FormData(event.currentTarget);
      await sendJSON('/api/v1/auth/login', 'POST', {
        login: String(form.get('login') || '').trim(),
        password: String(form.get('password') || ''),
      });
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
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

  async function sendMailTest() {
    const target = document.getElementById('mailSettingsResult');
    target.innerHTML = '<span class="tag warn">sending test</span>';
    try {
      const email = String(document.getElementById('mailTestEmail').value || '').trim();
      const data = await sendJSON('/api/v1/settings/mail/test', 'POST', { email });
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await loadAdminSettings(true);
    } catch (err) {
      const details = err?.payload ? `\n${JSON.stringify(err.payload, null, 2)}` : '';
      target.innerHTML = `<div class="code-block">${escapeHTML(`${err.message}${details}`)}</div>`;
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

  function openClientEmailModal(clientID) {
    const client = (state.clients || []).find((item) => item.id === clientID);
    if (!client) return;
    openModal(`Send access email: ${client.username}`, 'Client delivery', `
      <form id="clientEmailForm" class="form-grid">
        <div class="field"><label>Email</label><input value="${escapeHTML(client.email || '')}" disabled /></div>
        <div class="field"><label>Share link TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="72" /></div>
        <div class="field full"><label>Subject</label><input name="subject" value="RTIS MegaVPN access package" /></div>
        <div class="field full"><label>Message</label><textarea name="message" rows="6" placeholder="Optional custom note for the client."></textarea></div>
        <div class="field"><label>Create/refresh share link</label><select name="create_share_link"><option value="true">true</option><option value="false">false</option></select></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Send email</button></div>
      </form>
      <div id="clientEmailResult" class="form-result"></div>`);
    document.getElementById('clientEmailForm').addEventListener('submit', (event) => sendClientEmail(event, clientID));
  }

  async function sendClientEmail(event, clientID) {
    event.preventDefault();
    const target = document.getElementById('clientEmailResult');
    target.innerHTML = '<span class="tag warn">sending</span>';
    try {
      const form = new FormData(event.currentTarget);
      const data = await sendJSON(`/api/v1/clients/${clientID}/deliver-email`, 'POST', {
        subject: String(form.get('subject') || '').trim(),
        message: String(form.get('message') || '').trim(),
        ttl_hours: Number(form.get('ttl_hours') || 72),
        create_share_link: String(form.get('create_share_link')) === 'true',
      });
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function refreshShareLinkArtifactOptions(selectClientID, selectArtifactID, artifactInfoID, selectedArtifactID = '') {
    const clientID = String(selectClientID?.value || '').trim();
    const artifactSelect = document.getElementById(selectArtifactID);
    const artifactInfo = document.getElementById(artifactInfoID);
    if (!artifactSelect || !artifactInfo) return;
    artifactSelect.innerHTML = artifactOptions(clientID, selectedArtifactID);
    const artifacts = artifactRowsForClient(clientID);
    const selectedArtifact = artifacts.find((artifact) => artifact.id === artifactSelect.value) || artifacts[0] || null;
    if (selectedArtifact && artifactSelect.value !== selectedArtifact.id) {
      artifactSelect.value = selectedArtifact.id;
    }
    artifactInfo.innerHTML = selectedArtifact
      ? `<div class="code-block">artifact_id = ${escapeHTML(selectedArtifact.id)}
type = ${escapeHTML(selectedArtifact.artifact_type || 'unknown')}
status = ${escapeHTML(selectedArtifact.status || 'unknown')}
path = ${escapeHTML(selectedArtifact.storage_path || 'n/a')}</div>`
      : '<div class="empty compact-empty">No generated artifacts for the selected client. Queue export first.</div>';
  }

  function openArtifactExportModal() {
    const defaultClientID = state.clients[0]?.id || '';
    const defaultInstances = provisionableInstancesForExport().map((instance) => instance.id);
    openModal('Queue artifact export', 'Artifacts are built through client.provision jobs', `
      <form id="artifactExportForm" class="form-grid">
        <div class="field"><label>Client</label><select name="client_id" id="artifactExportClient">${clientOptions(defaultClientID)}</select></div>
        <div class="field"><label>Requested artifact type</label><select name="artifact_type">
          <option value="all">all supported</option>
          <option value="zip_bundle">zip_bundle</option>
          <option value="ovpn">ovpn</option>
          <option value="vless_url">vless_url</option>
          <option value="wg_conf">wg_conf</option>
          <option value="mtproto_url">mtproto_url</option>
          <option value="http_proxy_bundle">http_proxy_bundle</option>
          <option value="ipsec_bundle">ipsec_bundle</option>
          <option value="ss_url">ss_url</option>
        </select></div>
        <div class="field full"><label>Instances</label><select name="instance_ids" id="artifactExportInstances" multiple size="8">${instanceOptionsForArtifactExport(defaultInstances)}</select></div>
        <div class="field full"><div class="metric-caption">Current export path is implemented for <code>OpenVPN</code>, <code>Xray</code>, <code>WireGuard</code>, <code>MTProto</code>, <code>HTTP Proxy</code>, <code>IPsec/L2TP</code> and <code>Shadowsocks</code>. Leave all selected to queue the full supported bundle.</div></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Queue export job</button></div>
      </form>
      <div id="artifactExportResult" class="form-result"></div>`);
    document.getElementById('artifactExportForm').addEventListener('submit', submitArtifactExport);
  }

  async function submitArtifactExport(event) {
    event.preventDefault();
    const target = document.getElementById('artifactExportResult');
    target.innerHTML = '<span class="tag warn">queueing artifact export</span>';
    try {
      const formElement = event.currentTarget;
      const clientID = String(new FormData(formElement).get('client_id') || '').trim();
      const artifactType = String(new FormData(formElement).get('artifact_type') || 'all').trim();
      const instanceIDs = Array.from(formElement.querySelector('#artifactExportInstances')?.selectedOptions || []).map((option) => option.value);
      if (!clientID) {
        throw new Error('client is required');
      }
      const data = await sendJSON(`/api/v1/clients/${clientID}/artifacts`, 'POST', {
        type: artifactType,
        instance_ids: instanceIDs,
      });
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function openShareLinkPublishModal(initialClientID = '', initialArtifactID = '') {
    const artifact = (state.artifacts || []).find((item) => item.id === initialArtifactID) || null;
    const defaultClientID = String(initialClientID || artifact?.client_account_id || state.clients[0]?.id || '').trim();
    const defaultArtifactID = String(initialArtifactID || '').trim();
    openModal('Publish share link', 'Bind a public URL to one generated artifact', `
      <form id="shareLinkPublishForm" class="form-grid">
        <div class="field"><label>Client</label><select name="client_id" id="shareLinkClient">${clientOptions(defaultClientID)}</select></div>
        <div class="field"><label>TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="72" /></div>
        <div class="field full"><label>Artifact</label><select name="artifact_id" id="shareLinkArtifact"></select></div>
        <div class="field full" id="shareLinkArtifactInfo"></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Publish share link</button></div>
      </form>
      <div id="shareLinkPublishResult" class="form-result"></div>`);
    const clientSelect = document.getElementById('shareLinkClient');
    refreshShareLinkArtifactOptions(clientSelect, 'shareLinkArtifact', 'shareLinkArtifactInfo', defaultArtifactID);
    clientSelect.addEventListener('change', () => refreshShareLinkArtifactOptions(clientSelect, 'shareLinkArtifact', 'shareLinkArtifactInfo'));
    document.getElementById('shareLinkPublishForm').addEventListener('submit', submitShareLinkPublish);
  }

  async function submitShareLinkPublish(event) {
    event.preventDefault();
    const target = document.getElementById('shareLinkPublishResult');
    target.innerHTML = '<span class="tag warn">publishing share link</span>';
    try {
      const formElement = event.currentTarget;
      const form = new FormData(formElement);
      const clientID = String(form.get('client_id') || '').trim();
      const artifactID = String(form.get('artifact_id') || '').trim();
      const ttlHours = Number(form.get('ttl_hours') || 72);
      if (!clientID) {
        throw new Error('client is required');
      }
      if (!artifactID) {
        throw new Error('artifact is required');
      }
      const data = await sendJSON(`/api/v1/clients/${clientID}/share-links`, 'POST', {
        target_id: artifactID,
        ttl_hours: ttlHours,
      });
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify({
        ...data,
        url: shareLinkURL(data?.token || ''),
      }, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function revokeShareLinkAction(clientID, linkID) {
    const link = (state.shareLinks || []).find((item) => item.id === linkID) || null;
    openModal('Revoke share link', 'Disable public download access for this token', `
      <div class="code-block">client = ${escapeHTML(clientByID(clientID)?.username || clientID || 'n/a')}
link_id = ${escapeHTML(linkID || 'n/a')}
status = ${escapeHTML(link?.status || 'unknown')}
url = ${escapeHTML(shareLinkURL(link?.token || ''))}</div>
      <div class="modal-actions">
        <button class="secondary-btn" id="cancelRevokeShareLinkBtn" type="button">Cancel</button>
        <button class="danger-btn" id="confirmRevokeShareLinkBtn" type="button">Revoke link</button>
      </div>
      <div id="revokeShareLinkResult" class="form-result"></div>`);
    document.getElementById('cancelRevokeShareLinkBtn').addEventListener('click', closeModal);
    document.getElementById('confirmRevokeShareLinkBtn').addEventListener('click', async () => {
      const target = document.getElementById('revokeShareLinkResult');
      target.innerHTML = '<span class="tag warn">revoking share link</span>';
      try {
        const data = await sendJSON(`/api/v1/clients/${clientID}/share-links/${linkID}/revoke`, 'POST', {});
        target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
        await refresh();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    });
  }

  async function queueClientProvision(clientID) {
    const client = (state.clients || []).find((item) => item.id === clientID);
    if (!client) return;
    openModal(`Provision client: ${client.username}`, 'Generate OpenVPN / Xray / WireGuard / MTProto / HTTP Proxy / IPsec / Shadowsocks artifacts', `
      <div class="code-block">This action will queue client.provision for all active compatible instances bound to the client. Supported in this release: OpenVPN, Xray, WireGuard, MTProto, HTTP Proxy, IPsec/L2TP and Shadowsocks.</div>
      <div class="modal-actions">
        <button class="secondary-btn" id="cancelProvisionBtn" type="button">Cancel</button>
        <button class="primary-btn" id="confirmProvisionBtn" type="button">Queue provision job</button>
      </div>
      <div id="clientProvisionResult" class="form-result"></div>`);
    document.getElementById('cancelProvisionBtn').addEventListener('click', closeModal);
    document.getElementById('confirmProvisionBtn').addEventListener('click', async () => {
      const target = document.getElementById('clientProvisionResult');
      target.innerHTML = '<span class="tag warn">queueing</span>';
      try {
        const data = await sendJSON(`/api/v1/clients/${clientID}/provision`, 'POST', {});
        target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
        await refresh();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    });
  }

  async function openClientAccessesModal(clientID) {
    const client = (state.clients || []).find((item) => item.id === clientID);
    if (!client) return;
    openModal(`Client accesses: ${client.username}`, 'Provisioned service bindings', '<div class="empty">Loading service accesses...</div>');
    try {
      const accesses = await requestJSON(`/api/v1/clients/${clientID}/accesses`);
      const rows = (accesses || []).map((access) => {
        const instance = (state.instances || []).find((item) => item.id === access.instance_id);
        const serviceCode = instance?.service_code || 'unknown';
        const endpoint = instance?.endpoint_host ? `${instance.endpoint_host}:${instance.endpoint_port || 0}` : 'n/a';
        const canRotateOpenVPN = serviceCode === 'openvpn';
        const canRotateXray = serviceCode === 'xray-core' || serviceCode === 'xray';
        const canRotateWireGuard = serviceCode === 'wireguard';
        const canRotateMTProto = serviceCode === 'mtproto';
        const canRotateIPSec = serviceCode === 'ipsec';
        const canRotateHTTPProxy = serviceCode === 'http_proxy';
        const canRotateShadowsocks = serviceCode === 'shadowsocks';
        return `
          <tr>
            <td>${escapeHTML(instance?.name || access.instance_id)}</td>
            <td><span class="tag">${escapeHTML(serviceCode)}</span></td>
            <td>${escapeHTML(endpoint)}</td>
            <td>${statusTag(access.status || 'unknown')}</td>
            <td>
              <div class="inline-actions compact-actions">
                ${canRotateOpenVPN ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="openvpn">Rotate OpenVPN</button>` : ''}
                ${canRotateXray ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="xray-core">Rotate Xray UUID</button>` : ''}
                ${canRotateWireGuard ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="wireguard">Rotate WireGuard Keys</button>` : ''}
                ${canRotateMTProto ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="mtproto">Rotate MTProto Secret</button>` : ''}
                ${canRotateIPSec ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="ipsec">Rotate L2TP Access</button>` : ''}
                ${canRotateHTTPProxy ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="http_proxy">Rotate Proxy Access</button>` : ''}
                ${canRotateShadowsocks ? `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(access.id)}" data-driver="shadowsocks">Rotate SS Access</button>` : ''}
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="5"><div class="empty">No service accesses for this client.</div></td></tr>';
      el('modalBody').innerHTML = `
        <div id="clientAccessRotateResult" class="form-result"></div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Instance</th><th>Service</th><th>Endpoint</th><th>Status</th><th>Actions</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>`;
      document.querySelectorAll('.rotate-access-btn').forEach((button) => {
        button.addEventListener('click', () => rotateClientAccess(button.dataset.clientId, button.dataset.accessId, button.dataset.driver));
      });
    } catch (err) {
      el('modalBody').innerHTML = `<div class="empty">Failed to load service accesses: ${escapeHTML(err.message)}</div>`;
    }
  }

  async function rotateClientAccess(clientID, accessID, driver) {
    const target = document.getElementById('clientAccessRotateResult');
    if (target) target.innerHTML = '<span class="tag warn">queueing rotation</span>';
    const suffix = driver === 'openvpn'
      ? 'rotate-openvpn'
      : driver === 'wireguard'
        ? 'rotate-wireguard'
      : driver === 'mtproto'
        ? 'rotate-mtproto'
      : driver === 'ipsec'
        ? 'rotate-ipsec'
      : driver === 'http_proxy'
        ? 'rotate-http-proxy'
      : driver === 'shadowsocks'
        ? 'rotate-shadowsocks'
        : 'rotate-xray';
    try {
      const data = await requestJSON(`/api/v1/clients/${clientID}/accesses/${accessID}/${suffix}`, { method: 'POST' });
      if (target) target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function toInstanceRow(instance) {
    const node = state.nodes.find((item) => item.id === instance.node_id);
    return {
      id: instance.id,
      name: instance.name,
      node: node?.name || instance.node_id || 'n/a',
      service: instance.service_code || 'unknown',
      endpoint: instance.endpoint_host ? `${instance.endpoint_host}:${instance.endpoint_port || 0}` : 'n/a',
      revision: instance.status === 'active' ? 'applied' : 'pending',
      status: instance.status || 'draft',
    };
  }

  function instanceServiceOptions() {
    return availableInstanceServices()
      .map((service) => `<option value="${escapeHTML(service.code)}">${escapeHTML(service.display_name || service.name || service.code)} · ${escapeHTML(service.code)}</option>`)
      .join('');
  }

  function availableServicePacks() {
    return (state.servicePacks || [])
      .filter((pack) => Array.isArray(pack.components) && pack.components.length)
      .sort((left, right) => String(left.label || left.key).localeCompare(String(right.label || right.key), 'en'));
  }

  function servicePackByKey(packKey) {
    return availableServicePacks().find((pack) => pack.key === packKey) || null;
  }

  function defaultServicePack() {
    return availableServicePacks()[0] || null;
  }

  function servicePackOptions(selectedKey = '') {
    return availableServicePacks()
      .map((pack) => `<option value="${escapeHTML(pack.key)}"${pack.key === selectedKey ? ' selected' : ''}>${escapeHTML(pack.label || pack.key)}</option>`)
      .join('');
  }

  function activeLeafCertificates() {
    return (state.platformCertificates || [])
      .filter((item) => item.kind === 'leaf' && String(item.status || '').toLowerCase() === 'active' && item.key_secret_ref_id)
      .sort((left, right) => {
        if (Boolean(left.is_default) !== Boolean(right.is_default)) return left.is_default ? -1 : 1;
        return String(left.name || left.common_name || left.id).localeCompare(String(right.name || right.common_name || right.id), 'en');
      });
  }

  function activeManagedAuthorities() {
    return (state.platformCertificates || [])
      .filter((item) => item.kind === 'ca' && String(item.status || '').toLowerCase() === 'active')
      .sort((left, right) => String(left.name || left.common_name || left.id).localeCompare(String(right.name || right.common_name || right.id), 'en'));
  }

  function certificateOptions(selectedID = '', includeEmpty = true) {
    const items = activeLeafCertificates();
    const parts = [];
    if (includeEmpty) {
      parts.push('<option value="">No managed certificate</option>');
    }
    for (const item of items) {
      const expires = item.not_after ? formatDate(item.not_after) : 'n/a';
      const label = `${item.is_default ? '[default] ' : ''}${item.name || item.common_name || item.id} · ${item.source || 'certificate'} · ${expires}`;
      parts.push(`<option value="${escapeHTML(item.id)}"${item.id === selectedID ? ' selected' : ''}>${escapeHTML(label)}</option>`);
    }
    return parts.join('');
  }

  function authorityCertificateOptions(selectedID = '') {
    return activeManagedAuthorities()
      .map((item) => `<option value="${escapeHTML(item.id)}"${item.id === selectedID ? ' selected' : ''}>${escapeHTML(item.name || item.common_name || item.id)} · ${escapeHTML(item.common_name || 'CA')}</option>`)
      .join('');
  }

  function nodeOptions() {
    return (state.nodes || [])
      .map((node) => `<option value="${escapeHTML(node.id)}">${escapeHTML(node.name)} · ${escapeHTML(node.address || 'n/a')} · ${escapeHTML(node.agent_status || 'unknown')}</option>`)
      .join('');
  }

  function normalizeInstanceServiceCode(serviceCode) {
    const normalized = String(serviceCode || '').trim().toLowerCase();
    if (normalized === 'xray') return 'xray-core';
    if (normalized === 'wg' || normalized === 'wg-quick') return 'wireguard';
    if (normalized === 'squid' || normalized === 'http-proxy') return 'http_proxy';
    if (normalized === 'shadowsocks-libev' || normalized === 'ss-server') return 'shadowsocks';
    return normalized;
  }

  function cloneJSON(value) {
    if (value == null) return {};
    return JSON.parse(JSON.stringify(value));
  }

  function stringValue(...values) {
    for (const value of values) {
      const text = String(value ?? '').trim();
      if (text) return text;
    }
    return '';
  }

  function numberValue(...values) {
    for (const value of values) {
      const num = Number(value);
      if (Number.isFinite(num) && num !== 0) return num;
    }
    return 0;
  }

  function slugPathPart(value, fallback = 'server') {
    const normalized = String(value || '').trim().toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
    return normalized || fallback;
  }

  const INSTANCE_SERVICE_ORDER = [
    'xray-core',
    'openvpn',
    'wireguard',
    'ipsec',
    'xl2tpd',
    'http_proxy',
    'mtproto',
    'shadowsocks',
    'nginx',
  ];

  const INSTANCE_SERVICE_BLUEPRINTS = {
    'xray-core': {
      label: 'Xray VLESS / Reality',
      runtime: 'xray-core runtime',
      description: 'Основной modern-transport сервис для персональных VPN/anti-censorship профилей. Это продуктовый сервис, работающий поверх runtime xray-core.',
      unitPattern: 'megavpn-xray-<slug>',
      pathPattern: '/usr/local/etc/xray/<slug>.json',
      recommendations: [
        'Для первого production-среза держать порт 443 и валидный SNI/Server Name.',
        'Для прямого client-facing профиля использовать Reality c short-id и chrome fingerprint.',
        'Nginx/gRPC и HTTP edge-сценарии делать отдельным backend-profile без Reality на runtime-порту.',
        'Оставлять manual JSON override только для нестандартных transport-экспериментов.',
      ],
      presets: [
        {
          key: 'reality_tcp',
          label: 'Reality TCP',
          description: 'Рекомендуемый baseline для большинства egress-нод.',
          recommended: true,
          draft: {
            endpoint_port: 443,
            xray_network: 'tcp',
            xray_dest: 'www.cloudflare.com:443',
            xray_fingerprint: 'chrome',
            config_mode: '0640',
          },
        },
        {
          key: 'nginx_grpc_backend',
          label: 'Nginx gRPC Backend',
          description: 'Backend-профиль для связки Xray + Nginx gRPC edge. Публичный TLS терминируется на Nginx.',
          draft: {
            endpoint_port: 7443,
            xray_security: 'none',
            xray_network: 'grpc',
            xray_service_name: 'vless-grpc',
            config_mode: '0640',
          },
        },
        {
          key: 'nginx_ws_backend',
          label: 'Nginx HTTP/WebSocket Backend',
          description: 'Backend-профиль для связки Xray + Nginx HTTP/WebSocket edge.',
          draft: {
            endpoint_port: 7080,
            xray_security: 'none',
            xray_network: 'ws',
            xray_path: '/ws',
            config_mode: '0640',
          },
        },
      ],
    },
    openvpn: {
      label: 'OpenVPN',
      runtime: 'openvpn server runtime',
      description: 'Классический VPN-сервис для широкой клиентской совместимости и управляемого PKI lifecycle.',
      unitPattern: 'openvpn-server@<slug>',
      pathPattern: '/etc/openvpn/server/<slug>.conf',
      recommendations: [
        'Для массовых клиентов безопасный baseline: TCP/11994, platform PKI, AES-GCM.',
        'UDP имеет смысл только там, где сеть стабильна и нет жестких ограничений firewall.',
        'Не смешивать ручной PKI и platform-managed PKI в одном instance.',
      ],
      presets: [
        {
          key: 'tcp_11994',
          label: 'TCP 11994',
          description: 'Рекомендуемый baseline для совместимости и отдельного TCP-порта под OpenVPN.',
          recommended: true,
          draft: {
            endpoint_port: 11994,
            ovpn_proto: 'tcp',
            ovpn_dev: 'tun',
            ovpn_server_network: '10.8.0.0',
            ovpn_server_netmask: '255.255.255.0',
            config_mode: '0644',
            ovpn_pki_profile: 'default',
          },
        },
        {
          key: 'udp_1194',
          label: 'UDP 1194',
          description: 'Более классический профиль там, где throughput важнее camouflage.',
          draft: {
            endpoint_port: 1194,
            ovpn_proto: 'udp',
            ovpn_dev: 'tun',
            ovpn_server_network: '10.8.0.0',
            ovpn_server_netmask: '255.255.255.0',
            config_mode: '0644',
            ovpn_pki_profile: 'default',
          },
        },
      ],
    },
    wireguard: {
      label: 'WireGuard',
      runtime: 'wg-quick / wireguard-tools',
      description: 'Высокопроизводительный VPN для современных клиентов и минимальной конфигурационной поверхности.',
      unitPattern: 'wg-quick@<slug>',
      pathPattern: '/etc/wireguard/<slug>.conf',
      recommendations: [
        'Держать отдельную /24 сеть на каждый instance и не переиспользовать address pool.',
        'Для удаленных клиентов рекомендуемый keepalive 25 секунд.',
        'Endpoint port обычно 51820, если нет требований camouflage.',
      ],
      presets: [
        {
          key: 'roadwarrior',
          label: 'Road Warrior',
          description: 'Рекомендуемый full-tunnel профиль для клиентов.',
          recommended: true,
          draft: {
            endpoint_port: 51820,
            wg_network_cidr: '10.66.0.0/24',
            wg_server_address: '10.66.0.1/24',
            wg_client_allowed_ips: '0.0.0.0/0, ::/0',
            wg_client_dns: '1.1.1.1, 1.0.0.1',
            wg_keepalive: 25,
            config_mode: '0600',
          },
        },
        {
          key: 'split_tunnel',
          label: 'Split Tunnel',
          description: 'Профиль для корпоративных и частичных маршрутов.',
          draft: {
            endpoint_port: 51820,
            wg_network_cidr: '10.66.10.0/24',
            wg_server_address: '10.66.10.1/24',
            wg_client_allowed_ips: '10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16',
            wg_client_dns: '1.1.1.1, 1.0.0.1',
            wg_keepalive: 25,
            config_mode: '0600',
          },
        },
      ],
    },
    ipsec: {
      label: 'IPsec / IKEv2',
      runtime: 'strongSwan / ipsec',
      description: 'Базовый IPsec/IKE слой. Используется как отдельный managed service и как база для L2TP companion flow.',
      unitPattern: 'strongswan-starter',
      pathPattern: '/etc/ipsec.conf',
      recommendations: [
        'Использовать как transport/security layer, а L2TP держать отдельным companion instance.',
        'PSK-профиль оставить только как bootstrap baseline; дальше переводить в более строгие схемы.',
        'Секреты хранить отдельно и не дублировать руками в нескольких инстансах.',
      ],
      presets: [
        {
          key: 'ikev2_psk',
          label: 'IKEv2 PSK',
          description: 'Рекомендуемый baseline, пока в продукте нет сертификатного IKEv2 flow.',
          recommended: true,
          draft: {
            endpoint_port: 1701,
            ipsec_left: '%defaultroute',
            ipsec_right: '%any',
            ipsec_ike: 'aes256-sha1-modp1024',
            ipsec_esp: 'aes256-sha1',
            config_mode: '0644',
            ipsec_secrets_mode: '0600',
          },
        },
      ],
    },
    xl2tpd: {
      label: 'L2TP Access',
      runtime: 'xl2tpd + pppd',
      description: 'Companion-сервис к IPsec для L2TP remote-access профиля. Сам по себе не должен восприниматься как отдельный secure transport.',
      unitPattern: 'xl2tpd',
      pathPattern: '/etc/xl2tpd/xl2tpd.conf',
      recommendations: [
        'Деплоить вместе с companion IPsec instance.',
        'Явно задавать pool, DNS и default credentials bootstrap-пути.',
        'Не использовать без сопутствующего IPsec transport.',
      ],
      presets: [
        {
          key: 'remote_access',
          label: 'Remote Access',
          description: 'Рекомендуемый baseline для L2TP поверх IPsec.',
          recommended: true,
          draft: {
            endpoint_port: 1701,
            xl2tpd_local_ip: '10.20.0.1',
            xl2tpd_ip_range_start: '10.20.0.10',
            xl2tpd_ip_range_end: '10.20.0.200',
            xl2tpd_dns_primary: '1.1.1.1',
            xl2tpd_dns_secondary: '1.0.0.1',
            config_mode: '0644',
          },
        },
      ],
    },
    http_proxy: {
      label: 'HTTP Proxy / Squid',
      runtime: 'squid runtime',
      description: 'Классический authenticated HTTP proxy. Должен быть отдельным изолированным instance на своем config/unit.',
      unitPattern: 'megavpn-http-proxy-<slug>',
      pathPattern: '/etc/squid/<slug>.conf',
      recommendations: [
        'По умолчанию только authenticated profile; open proxy не делать preset-ом.',
        'Для каждого instance держать отдельные config, passwd, pid и log paths.',
        'Visible hostname и auth realm задавать осмысленно, чтобы упростить поддержку.',
      ],
      presets: [
        {
          key: 'authenticated_edge',
          label: 'Authenticated Edge',
          description: 'Рекомендуемый production baseline с обязательной аутентификацией.',
          recommended: true,
          draft: {
            endpoint_port: 3128,
            proxy_auth_realm: 'RTIS MegaVPN HTTP Proxy',
            proxy_auth_helper_path: '/usr/lib/squid/basic_ncsa_auth',
            config_mode: '0644',
          },
        },
        {
          key: 'authenticated_alt_8080',
          label: 'Authenticated 8080',
          description: 'Альтернативный профиль для окружений, где 3128 конфликтует.',
          draft: {
            endpoint_port: 8080,
            proxy_auth_realm: 'RTIS MegaVPN HTTP Proxy',
            proxy_auth_helper_path: '/usr/lib/squid/basic_ncsa_auth',
            config_mode: '0644',
          },
        },
      ],
    },
    mtproto: {
      label: 'MTProto',
      runtime: 'xray-core runtime',
      description: 'Telegram-oriented proxy profile. Это отдельный продуктовый сервис, но его runtime движок тоже xray-core.',
      unitPattern: 'megavpn-mtproto-<slug>',
      pathPattern: '/usr/local/etc/xray/<slug>.json',
      recommendations: [
        'Держать отдельный unit/config на каждый instance, не смешивать с VLESS instance.',
        'Стандартный production baseline: port 443, отдельный secret per access rotation.',
        'Использовать только как специализированный transport для Telegram-кейса.',
      ],
      presets: [
        {
          key: 'telegram_443',
          label: 'Telegram 443',
          description: 'Рекомендуемый baseline для основного MTProto traffic.',
          recommended: true,
          draft: {
            endpoint_port: 443,
            mtproto_listen: '0.0.0.0',
            config_mode: '0640',
          },
        },
        {
          key: 'telegram_8443',
          label: 'Telegram 8443',
          description: 'Альтернативный профиль для нод, где 443 уже занят.',
          draft: {
            endpoint_port: 8443,
            mtproto_listen: '0.0.0.0',
            config_mode: '0640',
          },
        },
      ],
    },
    shadowsocks: {
      label: 'Shadowsocks',
      runtime: 'shadowsocks-libev runtime',
      description: 'Легковесный proxy/VPN-like сервис для клиентских приложений и быстрых персональных доступов.',
      unitPattern: 'megavpn-shadowsocks-<slug>',
      pathPattern: '/etc/shadowsocks-libev/<slug>.json',
      recommendations: [
        'Стартовый baseline: chacha20-ietf-poly1305 и tcp_and_udp.',
        'Держать отдельные server/access secrets и не переиспользовать их между профилями.',
        'Port base планировать так, чтобы access rotation не конфликтовал с соседними сервисами.',
      ],
      presets: [
        {
          key: 'chacha_full',
          label: 'Chacha Full',
          description: 'Рекомендуемый universal baseline.',
          recommended: true,
          draft: {
            endpoint_port: 8388,
            ss_method: 'chacha20-ietf-poly1305',
            ss_mode: 'tcp_and_udp',
            ss_timeout: 300,
            config_mode: '0640',
          },
        },
        {
          key: 'aes_tcp',
          label: 'AES TCP Only',
          description: 'Консервативный профиль для TCP-only клиентов.',
          draft: {
            endpoint_port: 8388,
            ss_method: 'aes-256-gcm',
            ss_mode: 'tcp_only',
            ss_timeout: 300,
            config_mode: '0640',
          },
        },
      ],
    },
    nginx: {
      label: 'Nginx Edge',
      runtime: 'nginx runtime',
      description: 'Edge/service front для reverse-proxy и static publishing. Это не VPN transport, а обслуживающий ingress layer.',
      unitPattern: 'nginx',
      pathPattern: '/etc/nginx/conf.d/megavpn-<slug>.conf',
      recommendations: [
        'Использовать как reverse-proxy front для UI/API или как static edge.',
        'Для Xray gRPC/HTTP edge держать отдельный Nginx ingress и отдельный backend-port Xray.',
        'TLS-сертификаты и upstream path держать явными, без магических defaults.',
        'Отдельный ingress слой не должен смешиваться с транспортными сервисами.',
      ],
      presets: [
        {
          key: 'reverse_proxy',
          label: 'Reverse Proxy',
          description: 'Рекомендуемый edge profile для API/UI.',
          recommended: true,
          draft: {
            endpoint_port: 8080,
            nginx_mode: 'reverse_proxy',
            nginx_index_files: 'index.html index.htm',
            config_mode: '0644',
          },
        },
        {
          key: 'grpc_edge',
          label: 'Xray gRPC Edge',
          description: 'TLS edge для backend Xray gRPC. Требует cert/key и grpc upstream.',
          draft: {
            endpoint_port: 443,
            nginx_mode: 'grpc_proxy',
            nginx_location_path: '/vless-grpc',
            nginx_upstream_url: 'grpc://127.0.0.1:7443',
            nginx_tls_enabled: 'true',
            config_mode: '0644',
          },
        },
        {
          key: 'ws_edge',
          label: 'Xray HTTP/WebSocket Edge',
          description: 'TLS reverse-proxy для backend Xray HTTP/WebSocket transport.',
          draft: {
            endpoint_port: 443,
            nginx_mode: 'reverse_proxy',
            nginx_upstream_url: 'http://127.0.0.1:7080',
            nginx_tls_enabled: 'true',
            config_mode: '0644',
          },
        },
        {
          key: 'static_site',
          label: 'Static Site',
          description: 'Профиль для статической публикации контента.',
          draft: {
            endpoint_port: 8080,
            nginx_mode: 'static',
            nginx_index_files: 'index.html index.htm',
            config_mode: '0644',
          },
        },
      ],
    },
  };

  function instanceServiceBlueprint(serviceCode) {
    const normalized = normalizeInstanceServiceCode(serviceCode);
    const service = (state.servicesCatalog || []).find((entry) => normalizeInstanceServiceCode(entry.code) === normalized);
    const fallback = INSTANCE_SERVICE_BLUEPRINTS[normalized] || null;
    if (!service) return fallback;
    return {
      label: service.label || service.display_name || fallback?.label || service.name || normalized,
      runtimeCode: service.runtime_code || fallback?.runtimeCode || normalized,
      runtime: service.runtime || fallback?.runtime || 'runtime n/a',
      serviceKind: service.service_kind || fallback?.serviceKind || 'service',
      companionTo: Array.isArray(service.companion_to) ? service.companion_to : (fallback?.companionTo || []),
      companionNote: service.companion_note || fallback?.companionNote || '',
      description: service.description || fallback?.description || '',
      unitPattern: service.unit_pattern || fallback?.unitPattern || 'n/a',
      pathPattern: service.path_pattern || fallback?.pathPattern || 'n/a',
      nameTemplate: service.name_template || fallback?.nameTemplate || '',
      slugTemplate: service.slug_template || fallback?.slugTemplate || '',
      endpointHint: service.endpoint_hint || fallback?.endpointHint || '',
      platformNotes: Array.isArray(service.platform_notes) ? service.platform_notes : (fallback?.platformNotes || []),
      recommendations: Array.isArray(service.recommendations) ? service.recommendations : (fallback?.recommendations || []),
      presets: Array.isArray(service.presets) && service.presets.length ? service.presets : (fallback?.presets || []),
    };
  }

  function availableInstanceServices() {
    const ranked = new Map();
    const fallbackOrderBase = INSTANCE_SERVICE_ORDER.length;
    for (const service of (state.servicesCatalog || [])) {
      if (service.supports_instances === false || service.enabled === false) continue;
      const normalized = normalizeInstanceServiceCode(service.code);
      const blueprint = instanceServiceBlueprint(normalized);
      const candidate = {
        ...service,
        code: normalized,
        display_name: blueprint?.label || service.name || normalized,
      };
      const current = ranked.get(normalized);
      const score = service.code === normalized ? 2 : 1;
      if (!current || score > current.score) {
        ranked.set(normalized, { score, service: candidate });
      }
    }
    return Array.from(ranked.values())
      .map((entry) => entry.service)
      .sort((left, right) => {
        const leftIndex = INSTANCE_SERVICE_ORDER.indexOf(left.code);
        const rightIndex = INSTANCE_SERVICE_ORDER.indexOf(right.code);
        const leftOrder = leftIndex === -1 ? fallbackOrderBase : leftIndex;
        const rightOrder = rightIndex === -1 ? fallbackOrderBase : rightIndex;
        if (leftOrder !== rightOrder) return leftOrder - rightOrder;
        return String(left.display_name).localeCompare(String(right.display_name), 'en');
      });
  }

  function defaultInstancePreset(serviceCode) {
    const presets = instanceServiceBlueprint(serviceCode)?.presets || [];
    return presets.find((preset) => preset.recommended) || presets[0] || null;
  }

  function resolveInstancePreset(serviceCode, presetKey) {
    const presets = instanceServiceBlueprint(serviceCode)?.presets || [];
    if (!presets.length) return null;
    return presets.find((preset) => preset.key === presetKey) || defaultInstancePreset(serviceCode);
  }

  function applyInstancePresetDraft(serviceCode, draft, presetKey) {
    const preset = resolveInstancePreset(serviceCode, presetKey);
    if (!preset) return { ...(draft || {}) };
    return {
      ...(draft || {}),
      ...(preset.draft || {}),
      service_profile: preset.key,
    };
  }

  function finalizeInstanceDraft(serviceCode, instance, spec, draft, presetKey = '') {
    const normalized = normalizeInstanceServiceCode(serviceCode);
    const defaultPreset = defaultInstancePreset(normalized);
    const persistedPreset = stringValue(presetKey, draft?.service_profile, spec?.service_profile, defaultPreset?.key);
    let out = { ...(draft || {}), service_profile: persistedPreset };
    if (!instance || presetKey) {
      out = applyInstancePresetDraft(normalized, out, persistedPreset);
    }
    return out;
  }

  function renderInstanceServiceProfilePanel(serviceCode, draft = {}) {
    const blueprint = instanceServiceBlueprint(serviceCode);
    if (!blueprint) return '';
    const preset = resolveInstancePreset(serviceCode, draft.service_profile);
    const presets = blueprint.presets || [];
    const platformNotes = blueprint.platformNotes || [];
    const recommendations = blueprint.recommendations || [];
    return `
      <div class="field full">
        <div class="code-block">
          <div><strong>${escapeHTML(blueprint.label)}</strong> · ${escapeHTML(blueprint.runtime || 'runtime n/a')}</div>
          <div class="metric-caption" style="margin-top:6px">${escapeHTML(blueprint.description || '')}</div>
          <div class="metric-caption" style="margin-top:6px">Service code: <code>${escapeHTML(normalizeInstanceServiceCode(serviceCode))}</code> · Runtime code: <code>${escapeHTML(blueprint.runtimeCode || normalizeInstanceServiceCode(serviceCode))}</code></div>
          <div class="metric-caption" style="margin-top:8px">Default unit: <code>${escapeHTML(blueprint.unitPattern || 'n/a')}</code> · Config path: <code>${escapeHTML(blueprint.pathPattern || 'n/a')}</code></div>
          ${Array.isArray(blueprint.companionTo) && blueprint.companionTo.length ? `<div class="metric-caption" style="margin-top:6px">Companion services: <code>${escapeHTML(blueprint.companionTo.join(', '))}</code></div>` : ''}
          ${blueprint.companionNote ? `<div class="metric-caption" style="margin-top:6px">${escapeHTML(blueprint.companionNote)}</div>` : ''}
          ${presets.length ? `
            <div style="margin-top:12px">
              <label>Preset</label>
              <select name="service_profile">
                ${presets.map((item) => `<option value="${escapeHTML(item.key)}"${item.key === preset?.key ? ' selected' : ''}>${escapeHTML(item.label)}${item.recommended ? ' (recommended)' : ''}</option>`).join('')}
              </select>
              <div class="metric-caption" style="margin-top:6px">${escapeHTML(preset?.description || '')}</div>
            </div>` : ''}
          ${platformNotes.length ? `<div class="metric-caption" style="margin-top:10px">${platformNotes.map((line) => `CA / platform: ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
          ${recommendations.length ? `<div class="metric-caption" style="margin-top:10px">${recommendations.map((line) => `• ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
        </div>
      </div>`;
  }

  function applyAutoFieldValue(input, nextValue, forceDefaults = false) {
    if (!input || !nextValue) return;
    const current = String(input.value || '').trim();
    const previousAuto = String(input.dataset.autoValue || '').trim();
    if (!current || current === previousAuto) {
      input.value = nextValue;
      input.dataset.autoValue = nextValue;
    }
  }

  function applyCreateInstanceDefaults(form, serviceCode, draft, options = {}) {
    if (!form) return;
    const blueprint = instanceServiceBlueprint(serviceCode);
    if (!blueprint) return;
    const nameInput = form.querySelector('input[name="name"]');
    const slugInput = form.querySelector('input[name="slug"]');
    const hostInput = form.querySelector('input[name="endpoint_host"]');
    const unitInput = form.querySelector('input[name="systemd_unit"]');
    if (nameInput) {
      if (blueprint.nameTemplate) {
        nameInput.placeholder = blueprint.nameTemplate;
        applyAutoFieldValue(nameInput, blueprint.nameTemplate, options.forceDefaults);
      }
    }
    if (slugInput) {
      if (blueprint.slugTemplate) {
        slugInput.placeholder = blueprint.slugTemplate;
        applyAutoFieldValue(slugInput, blueprint.slugTemplate, options.forceDefaults);
      }
    }
    if (hostInput) {
      hostInput.placeholder = blueprint.endpointHint || 'vpn.example.com';
    }
    if (unitInput) {
      unitInput.placeholder = blueprint.unitPattern || 'optional override';
    }
    if (serviceCode === 'ipsec' && hostInput && !String(hostInput.value || '').trim() && blueprint.endpointHint) {
      hostInput.placeholder = blueprint.endpointHint;
    }
    if (serviceCode === 'xl2tpd' && hostInput) {
      hostInput.placeholder = blueprint.endpointHint || 'l2tp.example.com';
    }
    if (draft?.service_profile) {
      const note = form.querySelector('.service-profile-inline-note');
      if (note) note.remove();
      if (blueprint.companionNote) {
        const noteBlock = document.createElement('div');
        noteBlock.className = 'field full service-profile-inline-note';
        noteBlock.innerHTML = `<div class="code-block"><div class="metric-caption">${escapeHTML(blueprint.companionNote)}</div></div>`;
        const target = form.querySelector('.inline-actions');
        if (target?.parentElement) {
          target.parentElement.parentElement.insertBefore(noteBlock, target.parentElement);
        }
      }
    }
  }

  function buildInstanceSpecDraft(serviceCode, instance = null, presetKey = '') {
    const spec = instance?.spec || {};
    const normalized = normalizeInstanceServiceCode(serviceCode || instance?.service_code);
    switch (normalized) {
      case 'xray-core':
        const xraySlug = slugPathPart(instance?.slug, 'xray');
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, 443),
          config_path: stringValue(spec.config_path, `/usr/local/etc/xray/${xraySlug}.json`),
          config_mode: stringValue(spec.config_mode, '0640'),
          xray_security: stringValue(spec.security, 'reality'),
          certificate_id: stringValue(spec.certificate_id),
          xray_server_name: stringValue(spec.server_name, spec.sni, instance?.endpoint_host),
          xray_short_id: stringValue(spec.short_id),
          xray_dest: stringValue(spec.dest, 'www.cloudflare.com:443'),
          xray_fingerprint: stringValue(spec.fingerprint, 'chrome'),
          xray_network: stringValue(spec.network, spec.type, spec.transport, 'tcp'),
          xray_path: stringValue(spec.path, '/ws'),
          xray_service_name: stringValue(spec.service_name, 'vless-grpc'),
          xray_flow: stringValue(spec.flow),
          config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
        }, presetKey);
      case 'openvpn':
        const ovpnSlug = slugPathPart(instance?.slug, 'server');
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, 1194),
          config_path: stringValue(spec.config_path, `/etc/openvpn/server/${ovpnSlug}.conf`),
          config_mode: stringValue(spec.config_mode, '0644'),
          ovpn_proto: stringValue(spec.proto, 'tcp'),
          ovpn_dev: stringValue(spec.dev, 'tun'),
          ovpn_server_network: stringValue(spec.server_network, '10.8.0.0'),
          ovpn_server_netmask: stringValue(spec.server_netmask, '255.255.255.0'),
          ovpn_cipher: stringValue(spec.cipher),
          ovpn_auth: stringValue(spec.auth),
          ovpn_runtime_dir: stringValue(spec.runtime_dir),
          ovpn_pki_profile: stringValue(spec.pki_profile, 'default'),
          ovpn_server_extra_lines: Array.isArray(spec.server_extra_lines) ? spec.server_extra_lines.join('\n') : stringValue(spec.server_extra_lines),
          config_body: stringValue(spec.config_content),
        }, presetKey);
      case 'wireguard':
        const wgSlug = slugPathPart(instance?.slug, 'wg0');
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 51820),
          config_path: stringValue(spec.config_path, `/etc/wireguard/${wgSlug}.conf`),
          config_mode: stringValue(spec.config_mode, '0600'),
          wg_network_cidr: stringValue(spec.network_cidr, '10.66.0.0/24'),
          wg_server_address: stringValue(spec.server_address, '10.66.0.1/24'),
          wg_client_address_start: numberValue(spec.client_address_start, 10),
          wg_client_allowed_ips: stringValue(spec.client_allowed_ips, '0.0.0.0/0, ::/0'),
          wg_client_dns: stringValue(spec.client_dns, '1.1.1.1, 1.0.0.1'),
          wg_keepalive: numberValue(spec.persistent_keepalive, 25),
          wg_mtu: numberValue(spec.mtu),
          wg_interface_extra_lines: Array.isArray(spec.interface_extra_lines) ? spec.interface_extra_lines.join('\n') : stringValue(spec.interface_extra_lines),
          config_body: stringValue(spec.config_content),
        }, presetKey);
      case 'mtproto':
        const mtprotoSlug = slugPathPart(instance?.slug, 'mtproto');
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, 443),
          config_path: stringValue(spec.config_path, `/usr/local/etc/xray/${mtprotoSlug}.json`),
          config_mode: stringValue(spec.config_mode, '0640'),
          mtproto_listen: stringValue(spec.listen, '0.0.0.0'),
          config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
        }, presetKey);
      case 'nginx':
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 8080),
          config_path: stringValue(spec.config_path, '/etc/nginx/conf.d/megavpn-edge.conf'),
          config_mode: stringValue(spec.config_mode, '0644'),
          nginx_mode: stringValue(spec.mode, 'reverse_proxy'),
          nginx_location_path: stringValue(spec.location_path, '/'),
          certificate_id: stringValue(spec.certificate_id),
          nginx_server_name: stringValue(spec.server_name, instance?.endpoint_host, '_'),
          nginx_upstream_url: stringValue(spec.upstream_url, spec.proxy_pass),
          nginx_root_dir: stringValue(spec.root_dir),
          nginx_index_files: stringValue(spec.index_files, 'index.html index.htm'),
          nginx_tls_enabled: String(Boolean(spec.tls_enabled)),
          nginx_tls_cert_path: stringValue(spec.tls_cert_path),
          nginx_tls_key_path: stringValue(spec.tls_key_path),
          nginx_client_max_body_size: stringValue(spec.client_max_body_size),
          nginx_access_log: stringValue(spec.access_log),
          nginx_error_log: stringValue(spec.error_log),
          nginx_location_extra_lines: Array.isArray(spec.location_extra_lines) ? spec.location_extra_lines.join('\n') : stringValue(spec.location_extra_lines),
          nginx_server_extra_lines: Array.isArray(spec.server_extra_lines) ? spec.server_extra_lines.join('\n') : stringValue(spec.server_extra_lines),
          config_body: stringValue(spec.config_content),
        }, presetKey);
      case 'ipsec':
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 1701),
          config_path: stringValue(spec.config_path, '/etc/ipsec.conf'),
          config_mode: stringValue(spec.config_mode, '0644'),
          ipsec_secrets_path: stringValue(spec.secrets_path, '/etc/ipsec.secrets'),
          ipsec_secrets_mode: stringValue(spec.secrets_mode, '0600'),
          ipsec_left: stringValue(spec.left, '%defaultroute'),
          ipsec_leftid: stringValue(spec.leftid, spec.server_id, instance?.endpoint_host),
          ipsec_right: stringValue(spec.right, '%any'),
          ipsec_psk: stringValue(spec.psk),
          ipsec_ike: stringValue(spec.ike, 'aes256-sha1-modp1024'),
          ipsec_esp: stringValue(spec.esp, 'aes256-sha1'),
          ipsec_config_extra_lines: Array.isArray(spec.config_extra_lines) ? spec.config_extra_lines.join('\n') : stringValue(spec.config_extra_lines),
          ipsec_secrets_body: stringValue(spec.secrets_content),
          config_body: stringValue(spec.config_content),
        }, presetKey);
      case 'http_proxy':
        const proxySlug = slugPathPart(instance?.slug, 'proxy');
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 3128),
          config_path: stringValue(spec.config_path, `/etc/squid/${proxySlug}.conf`),
          config_mode: stringValue(spec.config_mode, '0644'),
          proxy_auth_realm: stringValue(spec.auth_realm, 'RTIS MegaVPN HTTP Proxy'),
          proxy_visible_hostname: stringValue(spec.visible_hostname, instance?.endpoint_host, instance?.name),
          proxy_auth_helper_path: stringValue(spec.auth_helper_path, '/usr/lib/squid/basic_ncsa_auth'),
          proxy_config_extra_lines: Array.isArray(spec.config_extra_lines) ? spec.config_extra_lines.join('\n') : stringValue(spec.config_extra_lines),
          config_body: stringValue(spec.config_content),
        }, presetKey);
      case 'xl2tpd':
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 1701),
          config_path: stringValue(spec.config_path, '/etc/xl2tpd/xl2tpd.conf'),
          config_mode: stringValue(spec.config_mode, '0644'),
          xl2tpd_options_path: stringValue(spec.options_path, '/etc/ppp/options.xl2tpd'),
          xl2tpd_chap_secrets_path: stringValue(spec.chap_secrets_path, '/etc/ppp/chap-secrets'),
          xl2tpd_local_ip: stringValue(spec.local_ip, '10.20.0.1'),
          xl2tpd_ip_range_start: stringValue(spec.ip_range_start, '10.20.0.10'),
          xl2tpd_ip_range_end: stringValue(spec.ip_range_end, '10.20.0.200'),
          xl2tpd_dns_primary: stringValue(spec.ppp_dns_primary, '1.1.1.1'),
          xl2tpd_dns_secondary: stringValue(spec.ppp_dns_secondary, '1.0.0.1'),
          xl2tpd_default_username: stringValue(spec.default_username),
          xl2tpd_default_password: stringValue(spec.default_password),
          xl2tpd_chap_secrets_entries: stringValue(spec.chap_secrets_entries, spec.chap_secrets_content),
          xl2tpd_options_extra_lines: Array.isArray(spec.options_extra_lines) ? spec.options_extra_lines.join('\n') : stringValue(spec.options_extra_lines),
          xl2tpd_config_extra_lines: Array.isArray(spec.config_extra_lines) ? spec.config_extra_lines.join('\n') : stringValue(spec.config_extra_lines),
          xl2tpd_options_body: stringValue(spec.options_content),
          config_body: stringValue(spec.config_content),
        }, presetKey);
      case 'shadowsocks':
        const ssSlug = slugPathPart(instance?.slug, 'shadowsocks');
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, spec.access_port_base, 8388),
          config_path: stringValue(spec.config_path, `/etc/shadowsocks-libev/${ssSlug}.json`),
          config_mode: stringValue(spec.config_mode, '0640'),
          ss_method: stringValue(spec.method, 'chacha20-ietf-poly1305'),
          ss_mode: stringValue(spec.mode, 'tcp_and_udp'),
          ss_timeout: numberValue(spec.timeout, 300),
          ss_password: stringValue(spec.password, spec.server_password),
          ss_access_port_base: numberValue(spec.access_port_base, spec.server_port, instance?.endpoint_port, 8388),
          config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
        }, presetKey);
      default:
        return finalizeInstanceDraft(normalized, instance, spec, {
          endpoint_port: numberValue(instance?.endpoint_port),
          config_path: stringValue(spec.config_path),
          config_mode: stringValue(spec.config_mode, '0640'),
          config_type: spec.config_json ? 'json' : 'text',
          config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
        }, presetKey);
    }
  }

  function renderInstanceServiceFields(serviceCode, draft = {}) {
    const intro = renderInstanceServiceProfilePanel(serviceCode, draft);
    switch (normalizeInstanceServiceCode(serviceCode)) {
      case 'xray-core':
        return `${intro}
          <div class="field"><label>Security</label><select name="xray_security">
            <option value="reality"${draft.xray_security !== 'none' && draft.xray_security !== 'tls' ? ' selected' : ''}>reality</option>
            <option value="tls"${draft.xray_security === 'tls' ? ' selected' : ''}>tls</option>
            <option value="none"${draft.xray_security === 'none' ? ' selected' : ''}>none (backend)</option>
          </select></div>
          <div class="field"><label>Managed certificate</label><select name="certificate_id">${certificateOptions(draft.certificate_id || '')}</select></div>
          <div class="field"><label>Server name / SNI</label><input name="xray_server_name" value="${escapeHTML(draft.xray_server_name || '')}" placeholder="vpn.example.com" /></div>
          <div class="field"><label>Short ID</label><input name="xray_short_id" value="${escapeHTML(draft.xray_short_id || '')}" placeholder="0123abcd4567ef89" /></div>
          <div class="field"><label>Reality dest</label><input name="xray_dest" value="${escapeHTML(draft.xray_dest || 'www.cloudflare.com:443')}" /></div>
          <div class="field"><label>Fingerprint</label><input name="xray_fingerprint" value="${escapeHTML(draft.xray_fingerprint || 'chrome')}" /></div>
          <div class="field"><label>Network</label><select name="xray_network">
            <option value="tcp"${draft.xray_network === 'tcp' ? ' selected' : ''}>tcp</option>
            <option value="grpc"${draft.xray_network === 'grpc' ? ' selected' : ''}>grpc</option>
            <option value="ws"${draft.xray_network === 'ws' ? ' selected' : ''}>ws</option>
          </select></div>
          <div class="field"><label>HTTP path</label><input name="xray_path" value="${escapeHTML(draft.xray_path || '/ws')}" placeholder="/ws" /></div>
          <div class="field"><label>gRPC service name</label><input name="xray_service_name" value="${escapeHTML(draft.xray_service_name || 'vless-grpc')}" placeholder="vless-grpc" /></div>
          <div class="field"><label>Flow</label><input name="xray_flow" value="${escapeHTML(draft.xray_flow || '')}" placeholder="optional" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/usr/local/etc/xray/xray.json')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
          <div class="field full"><label>Advanced JSON override</label><textarea name="config_body" rows="12" placeholder='{"inbounds":[...],"outbounds":[...]}'>${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'openvpn':
        return `${intro}
          <div class="field"><label>Protocol</label><select name="ovpn_proto">
            <option value="tcp"${draft.ovpn_proto !== 'udp' ? ' selected' : ''}>tcp</option>
            <option value="udp"${draft.ovpn_proto === 'udp' ? ' selected' : ''}>udp</option>
          </select></div>
          <div class="field"><label>Device</label><input name="ovpn_dev" value="${escapeHTML(draft.ovpn_dev || 'tun')}" /></div>
          <div class="field"><label>Server network</label><input name="ovpn_server_network" value="${escapeHTML(draft.ovpn_server_network || '10.8.0.0')}" /></div>
          <div class="field"><label>Server netmask</label><input name="ovpn_server_netmask" value="${escapeHTML(draft.ovpn_server_netmask || '255.255.255.0')}" /></div>
          <div class="field"><label>Cipher</label><input name="ovpn_cipher" value="${escapeHTML(draft.ovpn_cipher || '')}" placeholder="AES-256-GCM" /></div>
          <div class="field"><label>Auth</label><input name="ovpn_auth" value="${escapeHTML(draft.ovpn_auth || '')}" placeholder="SHA256" /></div>
          <div class="field"><label>Runtime dir</label><input name="ovpn_runtime_dir" value="${escapeHTML(draft.ovpn_runtime_dir || '')}" placeholder="/etc/openvpn/server/megavpn-edge" /></div>
          <div class="field"><label>PKI profile</label><input name="ovpn_pki_profile" value="${escapeHTML(draft.ovpn_pki_profile || 'default')}" placeholder="default" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/openvpn/server/server.conf')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
          <div class="field full"><label>Server extra lines</label><textarea name="ovpn_server_extra_lines" rows="5" placeholder="push &quot;redirect-gateway def1&quot;&#10;push &quot;dhcp-option DNS 1.1.1.1&quot;">${escapeHTML(draft.ovpn_server_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Advanced server config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated OpenVPN server config.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'wireguard':
        return `${intro}
          <div class="field"><label>Network CIDR</label><input name="wg_network_cidr" value="${escapeHTML(draft.wg_network_cidr || '10.66.0.0/24')}" placeholder="10.66.0.0/24" /></div>
          <div class="field"><label>Server address</label><input name="wg_server_address" value="${escapeHTML(draft.wg_server_address || '10.66.0.1/24')}" placeholder="10.66.0.1/24" /></div>
          <div class="field"><label>Client address start</label><input name="wg_client_address_start" type="number" min="2" max="250" value="${escapeHTML(draft.wg_client_address_start || 10)}" /></div>
          <div class="field"><label>Client allowed IPs</label><input name="wg_client_allowed_ips" value="${escapeHTML(draft.wg_client_allowed_ips || '0.0.0.0/0, ::/0')}" placeholder="0.0.0.0/0, ::/0" /></div>
          <div class="field"><label>Client DNS</label><input name="wg_client_dns" value="${escapeHTML(draft.wg_client_dns || '1.1.1.1, 1.0.0.1')}" placeholder="1.1.1.1, 1.0.0.1" /></div>
          <div class="field"><label>Persistent keepalive</label><input name="wg_keepalive" type="number" min="0" max="300" value="${escapeHTML(draft.wg_keepalive || 25)}" /></div>
          <div class="field"><label>MTU</label><input name="wg_mtu" type="number" min="0" max="9000" value="${escapeHTML(draft.wg_mtu || '')}" placeholder="optional" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/wireguard/wg0.conf')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0600')}" /></div>
          <div class="field full"><label>Interface extra lines</label><textarea name="wg_interface_extra_lines" rows="5" placeholder="PostUp = nft add rule inet filter input udp dport 51820 accept&#10;PostDown = nft delete rule inet filter input udp dport 51820 accept">${escapeHTML(draft.wg_interface_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated WireGuard config.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'mtproto':
        return `${intro}
          <div class="field"><label>Listen address</label><input name="mtproto_listen" value="${escapeHTML(draft.mtproto_listen || '0.0.0.0')}" placeholder="0.0.0.0" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/usr/local/etc/xray/mtproto.json')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
          <div class="field full"><label>Advanced JSON override</label><textarea name="config_body" rows="12" placeholder='{"inbounds":[...],"outbounds":[...]}'>${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'nginx':
        return `${intro}
          <div class="field"><label>Mode</label><select name="nginx_mode">
            <option value="reverse_proxy"${draft.nginx_mode !== 'static' && draft.nginx_mode !== 'grpc_proxy' ? ' selected' : ''}>reverse_proxy</option>
            <option value="grpc_proxy"${draft.nginx_mode === 'grpc_proxy' ? ' selected' : ''}>grpc_proxy</option>
            <option value="static"${draft.nginx_mode === 'static' ? ' selected' : ''}>static</option>
          </select></div>
          <div class="field"><label>Managed certificate</label><select name="certificate_id">${certificateOptions(draft.certificate_id || '')}</select></div>
          <div class="field"><label>Location path</label><input name="nginx_location_path" value="${escapeHTML(draft.nginx_location_path || '/')}" placeholder="/vless-grpc or /" /></div>
          <div class="field"><label>Server name</label><input name="nginx_server_name" value="${escapeHTML(draft.nginx_server_name || '_')}" placeholder="edge.example.com" /></div>
          <div class="field"><label>Upstream URL</label><input name="nginx_upstream_url" value="${escapeHTML(draft.nginx_upstream_url || '')}" placeholder="http://127.0.0.1:9000 or grpc://127.0.0.1:7443" /></div>
          <div class="field"><label>Static root</label><input name="nginx_root_dir" value="${escapeHTML(draft.nginx_root_dir || '')}" placeholder="/var/www/html" /></div>
          <div class="field"><label>Index files</label><input name="nginx_index_files" value="${escapeHTML(draft.nginx_index_files || 'index.html index.htm')}" /></div>
          <div class="field"><label>TLS</label><select name="nginx_tls_enabled">
            <option value="false"${draft.nginx_tls_enabled !== 'true' ? ' selected' : ''}>disabled</option>
            <option value="true"${draft.nginx_tls_enabled === 'true' ? ' selected' : ''}>enabled</option>
          </select></div>
          <div class="field"><label>TLS cert path</label><input name="nginx_tls_cert_path" value="${escapeHTML(draft.nginx_tls_cert_path || '')}" placeholder="/etc/letsencrypt/live/example/fullchain.pem" /></div>
          <div class="field"><label>TLS key path</label><input name="nginx_tls_key_path" value="${escapeHTML(draft.nginx_tls_key_path || '')}" placeholder="/etc/letsencrypt/live/example/privkey.pem" /></div>
          <div class="field"><label>Body size</label><input name="nginx_client_max_body_size" value="${escapeHTML(draft.nginx_client_max_body_size || '')}" placeholder="20m" /></div>
          <div class="field"><label>Access log</label><input name="nginx_access_log" value="${escapeHTML(draft.nginx_access_log || '')}" placeholder="/var/log/nginx/megavpn-access.log" /></div>
          <div class="field"><label>Error log</label><input name="nginx_error_log" value="${escapeHTML(draft.nginx_error_log || '')}" placeholder="/var/log/nginx/megavpn-error.log warn" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/nginx/conf.d/megavpn-edge.conf')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
          <div class="field full"><label>Location extra lines</label><textarea name="nginx_location_extra_lines" rows="5" placeholder="proxy_read_timeout 60s;&#10;proxy_send_timeout 60s;">${escapeHTML(draft.nginx_location_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Server extra lines</label><textarea name="nginx_server_extra_lines" rows="5" placeholder="add_header X-MegaVPN edge always;">${escapeHTML(draft.nginx_server_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated nginx server block.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'ipsec':
        return `${intro}
          <div class="field"><label>Left</label><input name="ipsec_left" value="${escapeHTML(draft.ipsec_left || '%defaultroute')}" placeholder="%defaultroute" /></div>
          <div class="field"><label>Left ID</label><input name="ipsec_leftid" value="${escapeHTML(draft.ipsec_leftid || '')}" placeholder="vpn.example.com" /></div>
          <div class="field"><label>Right</label><input name="ipsec_right" value="${escapeHTML(draft.ipsec_right || '%any')}" placeholder="%any" /></div>
          <div class="field"><label>Pre-shared key</label><input name="ipsec_psk" value="${escapeHTML(draft.ipsec_psk || '')}" placeholder="shared secret" /></div>
          <div class="field"><label>IKE</label><input name="ipsec_ike" value="${escapeHTML(draft.ipsec_ike || 'aes256-sha1-modp1024')}" /></div>
          <div class="field"><label>ESP</label><input name="ipsec_esp" value="${escapeHTML(draft.ipsec_esp || 'aes256-sha1')}" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/ipsec.conf')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
          <div class="field"><label>Secrets path</label><input name="ipsec_secrets_path" value="${escapeHTML(draft.ipsec_secrets_path || '/etc/ipsec.secrets')}" /></div>
          <div class="field"><label>Secrets mode</label><input name="ipsec_secrets_mode" value="${escapeHTML(draft.ipsec_secrets_mode || '0600')}" /></div>
          <div class="field full"><label>Config extra lines</label><textarea name="ipsec_config_extra_lines" rows="5" placeholder="ikelifetime=8h&#10;keylife=1h">${escapeHTML(draft.ipsec_config_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Secrets override</label><textarea name="ipsec_secrets_body" rows="4" placeholder="%any %any : PSK &quot;shared-secret&quot;">${escapeHTML(draft.ipsec_secrets_body || '')}</textarea></div>
          <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated ipsec.conf.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'http_proxy':
        return `${intro}
          <div class="field"><label>Auth realm</label><input name="proxy_auth_realm" value="${escapeHTML(draft.proxy_auth_realm || 'RTIS MegaVPN HTTP Proxy')}" /></div>
          <div class="field"><label>Visible hostname</label><input name="proxy_visible_hostname" value="${escapeHTML(draft.proxy_visible_hostname || '')}" placeholder="proxy.example.com" /></div>
          <div class="field"><label>Auth helper path</label><input name="proxy_auth_helper_path" value="${escapeHTML(draft.proxy_auth_helper_path || '/usr/lib/squid/basic_ncsa_auth')}" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/squid/proxy.conf')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
          <div class="field full"><label>Config extra lines</label><textarea name="proxy_config_extra_lines" rows="6" placeholder="cache_mem 64 MB&#10;maximum_object_size_in_memory 512 KB">${escapeHTML(draft.proxy_config_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated squid.conf.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'xl2tpd':
        return `${intro}
          <div class="field"><label>Local IP</label><input name="xl2tpd_local_ip" value="${escapeHTML(draft.xl2tpd_local_ip || '10.20.0.1')}" /></div>
          <div class="field"><label>Range start</label><input name="xl2tpd_ip_range_start" value="${escapeHTML(draft.xl2tpd_ip_range_start || '10.20.0.10')}" /></div>
          <div class="field"><label>Range end</label><input name="xl2tpd_ip_range_end" value="${escapeHTML(draft.xl2tpd_ip_range_end || '10.20.0.200')}" /></div>
          <div class="field"><label>DNS primary</label><input name="xl2tpd_dns_primary" value="${escapeHTML(draft.xl2tpd_dns_primary || '1.1.1.1')}" /></div>
          <div class="field"><label>DNS secondary</label><input name="xl2tpd_dns_secondary" value="${escapeHTML(draft.xl2tpd_dns_secondary || '1.0.0.1')}" /></div>
          <div class="field"><label>Default username</label><input name="xl2tpd_default_username" value="${escapeHTML(draft.xl2tpd_default_username || '')}" placeholder="vpnuser" /></div>
          <div class="field"><label>Default password</label><input name="xl2tpd_default_password" value="${escapeHTML(draft.xl2tpd_default_password || '')}" placeholder="shared password" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/xl2tpd/xl2tpd.conf')}" /></div>
          <div class="field"><label>Options path</label><input name="xl2tpd_options_path" value="${escapeHTML(draft.xl2tpd_options_path || '/etc/ppp/options.xl2tpd')}" /></div>
          <div class="field"><label>CHAP secrets path</label><input name="xl2tpd_chap_secrets_path" value="${escapeHTML(draft.xl2tpd_chap_secrets_path || '/etc/ppp/chap-secrets')}" /></div>
          <div class="field full"><label>CHAP secrets entries</label><textarea name="xl2tpd_chap_secrets_entries" rows="5" placeholder="vpnuser l2tpd password *">${escapeHTML(draft.xl2tpd_chap_secrets_entries || '')}</textarea></div>
          <div class="field full"><label>PPP options extra lines</label><textarea name="xl2tpd_options_extra_lines" rows="5" placeholder="idle 1800&#10;debug">${escapeHTML(draft.xl2tpd_options_extra_lines || '')}</textarea></div>
          <div class="field full"><label>XL2TPD config extra lines</label><textarea name="xl2tpd_config_extra_lines" rows="5" placeholder="ppp debug = yes">${escapeHTML(draft.xl2tpd_config_extra_lines || '')}</textarea></div>
          <div class="field full"><label>Options override</label><textarea name="xl2tpd_options_body" rows="8" placeholder="Leave empty to use generated PPP options.">${escapeHTML(draft.xl2tpd_options_body || '')}</textarea></div>
          <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated xl2tpd.conf.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      case 'shadowsocks':
        return `${intro}
          <div class="field"><label>Method</label><input name="ss_method" value="${escapeHTML(draft.ss_method || 'chacha20-ietf-poly1305')}" placeholder="chacha20-ietf-poly1305" /></div>
          <div class="field"><label>Mode</label><select name="ss_mode">
            <option value="tcp_only"${draft.ss_mode === 'tcp_only' ? ' selected' : ''}>tcp_only</option>
            <option value="tcp_and_udp"${draft.ss_mode !== 'tcp_only' ? ' selected' : ''}>tcp_and_udp</option>
            <option value="udp_only"${draft.ss_mode === 'udp_only' ? ' selected' : ''}>udp_only</option>
          </select></div>
          <div class="field"><label>Timeout</label><input name="ss_timeout" type="number" min="30" max="3600" value="${escapeHTML(draft.ss_timeout || 300)}" /></div>
          <div class="field"><label>Bootstrap password</label><input name="ss_password" value="${escapeHTML(draft.ss_password || '')}" placeholder="required before first apply" /></div>
          <div class="field"><label>Access port base</label><input name="ss_access_port_base" type="number" min="1" max="65535" value="${escapeHTML(draft.ss_access_port_base || draft.endpoint_port || 8388)}" /></div>
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/shadowsocks-libev/shadowsocks.json')}" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
          <div class="field full"><label>Advanced JSON override</label><textarea name="config_body" rows="12" placeholder='{"server":"0.0.0.0","method":"chacha20-ietf-poly1305"}'>${escapeHTML(draft.config_body || '')}</textarea></div>`;
      default:
        return `${intro}
          <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '')}" placeholder="/etc/service/config.conf" /></div>
          <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
          <div class="field"><label>Config type</label><select name="config_type">
            <option value="json"${draft.config_type === 'json' ? ' selected' : ''}>json</option>
            <option value="text"${draft.config_type !== 'json' ? ' selected' : ''}>text</option>
          </select></div>
          <div class="field full"><label>Config body</label><textarea name="config_body" rows="12" placeholder="Optional config content">${escapeHTML(draft.config_body || '')}</textarea></div>`;
    }
  }

  function syncInstanceServiceFields(formID, serviceCode, draft = null, options = {}) {
    const form = document.getElementById(formID);
    if (!form) return;
    const resolvedDraft = draft || buildInstanceSpecDraft(serviceCode, null, options.presetKey || '');
    const container = form.querySelector('.service-fields');
    if (container) container.innerHTML = renderInstanceServiceFields(serviceCode, resolvedDraft);
    const portInput = form.querySelector('input[name="endpoint_port"]');
    if (portInput && resolvedDraft.endpoint_port) {
      applyAutoFieldValue(portInput, String(resolvedDraft.endpoint_port), options.forceDefaults);
    }
    if (formID === 'createInstanceForm') {
      applyCreateInstanceDefaults(form, serviceCode, resolvedDraft, options);
    }
    const presetSelect = form.querySelector('select[name="service_profile"]');
    if (presetSelect) {
      presetSelect.addEventListener('change', () => {
        syncInstanceServiceFields(formID, serviceCode, null, { forceDefaults: true, presetKey: presetSelect.value });
        }, { once: true });
    }
  }

  function buildInstanceSpecPayload(serviceCode, form, baseSpec = {}, endpointPort = 0) {
    const normalized = normalizeInstanceServiceCode(serviceCode);
    const spec = cloneJSON(baseSpec || {});
    const configBody = String(form.get('config_body') || '').trim();
    spec.service_profile = String(form.get('service_profile') || '').trim();
    spec.config_path = String(form.get('config_path') || '').trim();
    spec.config_mode = String(form.get('config_mode') || '').trim();
    if (normalized === 'xray-core') {
      const slug = slugPathPart(form.get('slug'), 'xray');
      const expectedConfigPath = `/usr/local/etc/xray/${slug}.json`;
      if (!spec.config_path || spec.config_path === '/usr/local/etc/xray/config.json') {
        spec.config_path = expectedConfigPath;
      }
      spec.security = String(form.get('xray_security') || 'reality').trim() || 'reality';
      spec.certificate_id = String(form.get('certificate_id') || '').trim();
      spec.server_port = Number(form.get('endpoint_port') || endpointPort || 443) || 443;
      spec.server_name = String(form.get('xray_server_name') || '').trim();
      spec.sni = spec.server_name;
      spec.short_id = String(form.get('xray_short_id') || '').trim();
      spec.dest = String(form.get('xray_dest') || '').trim();
      spec.fingerprint = String(form.get('xray_fingerprint') || '').trim();
      spec.network = String(form.get('xray_network') || 'tcp').trim();
      spec.path = String(form.get('xray_path') || '').trim();
      spec.service_name = String(form.get('xray_service_name') || '').trim();
      spec.flow = String(form.get('xray_flow') || '').trim();
      if (configBody) {
        spec.config_json = JSON.parse(configBody);
        delete spec.config_content;
      } else {
        delete spec.config_json;
        delete spec.config_content;
      }
      return spec;
    }
    if (normalized === 'openvpn') {
      const slug = slugPathPart(form.get('slug'), 'server');
      const expectedConfigPath = `/etc/openvpn/server/${slug}.conf`;
      if (!spec.config_path || spec.config_path === '/etc/openvpn/server/server.conf') {
        spec.config_path = expectedConfigPath;
      }
      spec.server_port = Number(form.get('endpoint_port') || endpointPort || 1194) || 1194;
      spec.proto = String(form.get('ovpn_proto') || 'tcp').trim();
      spec.dev = String(form.get('ovpn_dev') || 'tun').trim();
      spec.server_network = String(form.get('ovpn_server_network') || '10.8.0.0').trim();
      spec.server_netmask = String(form.get('ovpn_server_netmask') || '255.255.255.0').trim();
      spec.cipher = String(form.get('ovpn_cipher') || '').trim();
      spec.auth = String(form.get('ovpn_auth') || '').trim();
      spec.runtime_dir = String(form.get('ovpn_runtime_dir') || '').trim();
      spec.pki_scope = 'platform';
      spec.pki_profile = String(form.get('ovpn_pki_profile') || 'default').trim() || 'default';
      spec.server_extra_lines = String(form.get('ovpn_server_extra_lines') || '').trim();
      if (configBody) spec.config_content = configBody;
      else delete spec.config_content;
      delete spec.config_json;
      return spec;
    }
    if (normalized === 'wireguard') {
      const slug = slugPathPart(form.get('slug'), 'wg0');
      const expectedConfigPath = `/etc/wireguard/${slug}.conf`;
      if (!spec.config_path || spec.config_path === '/etc/wireguard/wg0.conf') {
        spec.config_path = expectedConfigPath;
      }
      spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 51820) || 51820;
      spec.server_port = spec.listen_port;
      spec.network_cidr = String(form.get('wg_network_cidr') || '10.66.0.0/24').trim();
      spec.server_address = String(form.get('wg_server_address') || '').trim();
      spec.client_address_start = Number(form.get('wg_client_address_start') || 10) || 10;
      spec.client_allowed_ips = String(form.get('wg_client_allowed_ips') || '0.0.0.0/0, ::/0').trim();
      spec.client_dns = String(form.get('wg_client_dns') || '').trim();
      spec.persistent_keepalive = Number(form.get('wg_keepalive') || 25) || 25;
      spec.mtu = Number(form.get('wg_mtu') || 0) || 0;
      spec.interface_extra_lines = String(form.get('wg_interface_extra_lines') || '').trim();
      if (configBody) {
        spec.config_content = configBody;
      } else {
        delete spec.config_content;
      }
      delete spec.config_json;
      return spec;
    }
    if (normalized === 'mtproto') {
      const slug = slugPathPart(form.get('slug'), 'mtproto');
      const expectedConfigPath = `/usr/local/etc/xray/${slug}.json`;
      if (!spec.config_path || spec.config_path === '/usr/local/etc/xray/config.json') {
        spec.config_path = expectedConfigPath;
      }
      spec.server_port = Number(form.get('endpoint_port') || endpointPort || 443) || 443;
      spec.listen = String(form.get('mtproto_listen') || '0.0.0.0').trim();
      if (configBody) {
        spec.config_json = JSON.parse(configBody);
        delete spec.config_content;
      } else {
        delete spec.config_json;
        delete spec.config_content;
      }
      return spec;
    }
    if (normalized === 'nginx') {
      spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 8080) || 8080;
      spec.server_port = spec.listen_port;
      spec.mode = String(form.get('nginx_mode') || 'reverse_proxy').trim();
      spec.location_path = String(form.get('nginx_location_path') || '/').trim() || '/';
      spec.certificate_id = String(form.get('certificate_id') || '').trim();
      spec.server_name = String(form.get('nginx_server_name') || '').trim();
      spec.upstream_url = String(form.get('nginx_upstream_url') || '').trim();
      spec.root_dir = String(form.get('nginx_root_dir') || '').trim();
      spec.index_files = String(form.get('nginx_index_files') || '').trim();
      spec.tls_enabled = String(form.get('nginx_tls_enabled') || 'false') === 'true';
      spec.tls_cert_path = String(form.get('nginx_tls_cert_path') || '').trim();
      spec.tls_key_path = String(form.get('nginx_tls_key_path') || '').trim();
      spec.client_max_body_size = String(form.get('nginx_client_max_body_size') || '').trim();
      spec.access_log = String(form.get('nginx_access_log') || '').trim();
      spec.error_log = String(form.get('nginx_error_log') || '').trim();
      spec.location_extra_lines = String(form.get('nginx_location_extra_lines') || '').trim();
      spec.server_extra_lines = String(form.get('nginx_server_extra_lines') || '').trim();
      if (configBody) {
        spec.config_content = configBody;
        delete spec.config_json;
      } else {
        delete spec.config_content;
        delete spec.config_json;
      }
      return spec;
    }
    if (normalized === 'ipsec') {
      spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 1701) || 1701;
      spec.server_port = spec.listen_port;
      spec.left = String(form.get('ipsec_left') || '%defaultroute').trim();
      spec.leftid = String(form.get('ipsec_leftid') || '').trim();
      spec.right = String(form.get('ipsec_right') || '%any').trim();
      spec.psk = String(form.get('ipsec_psk') || '').trim();
      spec.ike = String(form.get('ipsec_ike') || 'aes256-sha1-modp1024').trim();
      spec.esp = String(form.get('ipsec_esp') || 'aes256-sha1').trim();
      spec.secrets_path = String(form.get('ipsec_secrets_path') || '').trim();
      spec.secrets_mode = String(form.get('ipsec_secrets_mode') || '').trim();
      spec.config_extra_lines = String(form.get('ipsec_config_extra_lines') || '').trim();
      spec.secrets_content = String(form.get('ipsec_secrets_body') || '').trim();
      if (configBody) {
        spec.config_content = configBody;
      } else {
        delete spec.config_content;
      }
      delete spec.config_json;
      return spec;
    }
    if (normalized === 'http_proxy') {
      const slug = slugPathPart(form.get('slug'), 'proxy');
      const expectedConfigPath = `/etc/squid/${slug}.conf`;
      if (!spec.config_path || spec.config_path === '/etc/squid/squid.conf') {
        spec.config_path = expectedConfigPath;
      }
      spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 3128) || 3128;
      spec.server_port = spec.listen_port;
      spec.auth_realm = String(form.get('proxy_auth_realm') || 'RTIS MegaVPN HTTP Proxy').trim();
      spec.visible_hostname = String(form.get('proxy_visible_hostname') || '').trim();
      spec.auth_helper_path = String(form.get('proxy_auth_helper_path') || '/usr/lib/squid/basic_ncsa_auth').trim();
      spec.config_extra_lines = String(form.get('proxy_config_extra_lines') || '').trim();
      if (configBody) {
        spec.config_content = configBody;
      } else {
        delete spec.config_content;
      }
      delete spec.config_json;
      return spec;
    }
    if (normalized === 'xl2tpd') {
      spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 1701) || 1701;
      spec.server_port = spec.listen_port;
      spec.local_ip = String(form.get('xl2tpd_local_ip') || '').trim();
      spec.ip_range_start = String(form.get('xl2tpd_ip_range_start') || '').trim();
      spec.ip_range_end = String(form.get('xl2tpd_ip_range_end') || '').trim();
      spec.ppp_dns_primary = String(form.get('xl2tpd_dns_primary') || '').trim();
      spec.ppp_dns_secondary = String(form.get('xl2tpd_dns_secondary') || '').trim();
      spec.default_username = String(form.get('xl2tpd_default_username') || '').trim();
      spec.default_password = String(form.get('xl2tpd_default_password') || '').trim();
      spec.options_path = String(form.get('xl2tpd_options_path') || '').trim();
      spec.chap_secrets_path = String(form.get('xl2tpd_chap_secrets_path') || '').trim();
      spec.chap_secrets_entries = String(form.get('xl2tpd_chap_secrets_entries') || '').trim();
      spec.options_extra_lines = String(form.get('xl2tpd_options_extra_lines') || '').trim();
      spec.config_extra_lines = String(form.get('xl2tpd_config_extra_lines') || '').trim();
      spec.options_content = String(form.get('xl2tpd_options_body') || '').trim();
      if (configBody) {
        spec.config_content = configBody;
      } else {
        delete spec.config_content;
      }
      delete spec.config_json;
      return spec;
    }
    if (normalized === 'shadowsocks') {
      const slug = slugPathPart(form.get('slug'), 'shadowsocks');
      const expectedConfigPath = `/etc/shadowsocks-libev/${slug}.json`;
      if (!spec.config_path || spec.config_path === '/etc/shadowsocks-libev/config.json') {
        spec.config_path = expectedConfigPath;
      }
      spec.server_port = Number(form.get('endpoint_port') || endpointPort || 8388) || 8388;
      spec.access_port_base = Number(form.get('ss_access_port_base') || spec.server_port || 8388) || 8388;
      spec.method = String(form.get('ss_method') || 'chacha20-ietf-poly1305').trim();
      spec.mode = String(form.get('ss_mode') || 'tcp_and_udp').trim();
      spec.timeout = Number(form.get('ss_timeout') || 300) || 300;
      spec.password = String(form.get('ss_password') || '').trim();
      if (configBody) {
        spec.config_json = JSON.parse(configBody);
        delete spec.config_content;
      } else {
        delete spec.config_json;
        delete spec.config_content;
      }
      return spec;
    }
    const configType = String(form.get('config_type') || 'json');
    if (configBody) {
      if (configType === 'json') {
        spec.config_json = JSON.parse(configBody);
        delete spec.config_content;
      } else {
        spec.config_content = configBody;
        delete spec.config_json;
      }
    } else {
      delete spec.config_json;
      delete spec.config_content;
    }
    return spec;
  }

  function renderServicePackProfilePanel(packKey) {
    const pack = servicePackByKey(packKey);
    if (!pack) return '<div class="field full"><div class="empty">No service pack definition available.</div></div>';
    const platformNotes = Array.isArray(pack.platform_notes) ? pack.platform_notes : [];
    const recommendations = Array.isArray(pack.recommendations) ? pack.recommendations : [];
    const components = Array.isArray(pack.components) ? pack.components : [];
    return `
      <div class="field full">
        <div class="code-block">
          <div><strong>${escapeHTML(pack.label || pack.key)}</strong> · <code>${escapeHTML(pack.key)}</code></div>
          <div class="metric-caption" style="margin-top:6px">${escapeHTML(pack.description || '')}</div>
          ${platformNotes.length ? `<div class="metric-caption" style="margin-top:10px">${platformNotes.map((line) => `Platform: ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
          ${recommendations.length ? `<div class="metric-caption" style="margin-top:10px">${recommendations.map((line) => `• ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
          <div class="metric-caption" style="margin-top:12px">Components:</div>
          <div class="timeline" style="margin-top:8px">
            ${components.map((component) => `
              <div class="timeline-item">
                <strong>${escapeHTML(component.label || component.service_code || 'component')}</strong>
                <div class="timeline-meta">${escapeHTML(component.description || '')}</div>
                <div class="metric-caption">service <code>${escapeHTML(component.service_code || 'n/a')}</code> · preset <code>${escapeHTML(component.preset_key || 'n/a')}</code> · port ${escapeHTML(String(component.endpoint_port || 0))}</div>
              </div>`).join('')}
          </div>
        </div>
      </div>`;
  }

  function syncCreateServicePackDefaults(form, packKey) {
    if (!form) return;
    const pack = servicePackByKey(packKey);
    const panel = form.querySelector('#servicePackFields');
    if (panel) panel.innerHTML = renderServicePackProfilePanel(packKey);
    if (!pack) return;
    const baseNameInput = form.querySelector('input[name="base_name"]');
    const hostInput = form.querySelector('input[name="endpoint_host"]');
    if (baseNameInput) {
      const template = String(pack.base_name_template || '').trim();
      if (template) {
        baseNameInput.placeholder = template;
        applyAutoFieldValue(baseNameInput, template, true);
      }
    }
    if (hostInput) {
      const hint = String(pack.endpoint_hint || '').trim() || 'edge.example.com';
      hostInput.placeholder = hint;
      hostInput.required = Boolean(pack.requires_endpoint_host);
    }
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
      setTimeout(closeModal, 500);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function openCreateCertificateWizard() {
    openModal('Create certificate', 'Certificate Manager', `
      <form id="certificateWizardForm" class="form-grid">
        <div class="field full">
          <div class="code-block">
            <div><strong>Please choose an action</strong></div>
            <div class="metric-caption" style="margin-top:6px">Это inventory-поток по образцу Synology: импорт, self-signed, managed CA, issue from CA и service CA для OpenVPN.</div>
          </div>
        </div>
        <div class="field full"><label><input type="radio" name="certificate_action" value="import" checked /> Import certificate</label><div class="metric-caption">Import private key, certificate and optional intermediate chain.</div></div>
        <div class="field full"><label><input type="radio" name="certificate_action" value="self_signed" /> Create self-signed certificate</label><div class="metric-caption">Useful for internal edge and test deployments.</div></div>
        <div class="field full"><label><input type="radio" name="certificate_action" value="managed_ca" /> Create managed CA</label><div class="metric-caption">Create an internal certificate authority for future issuance.</div></div>
        <div class="field full"><label><input type="radio" name="certificate_action" value="issue_from_ca" /> Issue certificate from managed CA</label><div class="metric-caption">Issue a server certificate from an existing managed CA.</div></div>
        <div class="field full"><label><input type="radio" name="certificate_action" value="service_ca_root" /> Create service CA root</label><div class="metric-caption">Managed service-specific CA root, currently relevant for OpenVPN platform PKI.</div></div>
        <div class="field full"><label><input type="radio" name="certificate_action" value="letsencrypt" /> Get certificate from Let's Encrypt <span class="tag">paused</span></label><div class="metric-caption">UI slot stays visible, but backend issuance is intentionally paused until the ACME challenge strategy is approved.</div></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Next</button></div>
      </form>`, { wide: true });
    document.getElementById('certificateWizardForm').addEventListener('submit', (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      openCertificateActionForm(String(form.get('certificate_action') || 'import'));
    });
  }

  function openCertificateActionForm(action) {
    switch (action) {
      case 'import':
        openModal('Import certificate', 'Certificates / Import', `
          <form id="importCertificateForm" class="form-grid">
            <div class="field"><label>Name</label><input name="name" required placeholder="edge.example.com" /></div>
            <div class="field"><label>Description</label><input name="description" placeholder="Commercial wildcard certificate" /></div>
            <div class="field full"><label>Certificate PEM</label><textarea name="certificate" rows="8" required placeholder="-----BEGIN CERTIFICATE-----"></textarea></div>
            <div class="field full"><label>Private key PEM</label><textarea name="private_key" rows="8" required placeholder="-----BEGIN EC PRIVATE KEY-----"></textarea></div>
            <div class="field full"><label>Intermediate / chain PEM</label><textarea name="chain" rows="6" placeholder="-----BEGIN CERTIFICATE-----"></textarea></div>
            <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Import certificate</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        document.getElementById('importCertificateForm').addEventListener('submit', importCertificateSubmit);
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
            <div class="field full"><label>Common Name</label><input name="common_name" required placeholder="RTIS Internal Edge CA" /></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Create managed CA</button></div>
          </form>
          <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
        document.getElementById('managedCAForm').addEventListener('submit', createManagedCASubmit);
        return;
      case 'issue_from_ca':
        openModal('Issue certificate from managed CA', 'Certificates / Issue from CA', `
          <form id="issueFromCAForm" class="form-grid">
            <div class="field"><label>Authority</label><select name="authority_certificate_id" required>${authorityCertificateOptions()}</select></div>
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
            <div class="field full"><label>Common Name</label><input name="common_name" required placeholder="RTIS OpenVPN Platform CA" /></div>
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

  async function importCertificateSubmit(event) {
    event.preventDefault();
    const target = document.getElementById('certificateActionResult');
    target.innerHTML = '<span class="tag warn">importing</span>';
    try {
      const form = new FormData(event.currentTarget);
      const payload = {
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        certificate: String(form.get('certificate') || '').trim(),
        private_key: String(form.get('private_key') || '').trim(),
        chain: String(form.get('chain') || '').trim(),
        is_default: String(form.get('is_default') || '') === '1',
      };
      const data = await sendJSON('/api/v1/platform/certificates/import', 'POST', payload);
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function createSelfSignedCertificateSubmit(event) {
    event.preventDefault();
    const target = document.getElementById('certificateActionResult');
    target.innerHTML = '<span class="tag warn">creating</span>';
    try {
      const form = new FormData(event.currentTarget);
      const payload = {
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
        dns_names: parseCSVList(form.get('dns_names')),
        valid_days: Number(form.get('valid_days') || 365),
        is_default: String(form.get('is_default') || '') === '1',
      };
      const data = await sendJSON('/api/v1/platform/certificates/self-signed', 'POST', payload);
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function createManagedCASubmit(event) {
    event.preventDefault();
    const target = document.getElementById('certificateActionResult');
    target.innerHTML = '<span class="tag warn">creating CA</span>';
    try {
      const form = new FormData(event.currentTarget);
      const payload = {
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
      };
      const data = await sendJSON('/api/v1/platform/certificates/authorities', 'POST', payload);
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function issueCertificateFromCASubmit(event) {
    event.preventDefault();
    const target = document.getElementById('certificateActionResult');
    target.innerHTML = '<span class="tag warn">issuing</span>';
    try {
      const form = new FormData(event.currentTarget);
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  async function createServiceCARootSubmit(event) {
    event.preventDefault();
    const target = document.getElementById('certificateActionResult');
    target.innerHTML = '<span class="tag warn">creating service CA</span>';
    try {
      const form = new FormData(event.currentTarget);
      const payload = {
        service_code: String(form.get('service_code') || '').trim(),
        pki_profile: String(form.get('pki_profile') || '').trim(),
        common_name: String(form.get('common_name') || '').trim(),
      };
      const data = await sendJSON('/api/v1/platform/pki-roots', 'POST', payload);
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
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
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
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
          <div class="code-block">${escapeHTML(JSON.stringify(job.result, null, 2))}</div>
        </section>` : ''}
      <section class="card">
        <div class="table-head compact-head"><h3>Execution Log</h3><span class="tag">${escapeHTML(String(logRows.length))} entries</span></div>
        ${logRows.length ? `
          <div class="timeline">
            ${logRows.map((entry) => `
              <div class="timeline-item">
                <strong>${escapeHTML(formatDate(entry.created_at))} · ${escapeHTML(String(entry.level || 'info').toUpperCase())}</strong>
                <div class="timeline-meta">${escapeHTML(entry.message || '')}</div>
                ${entry.payload && Object.keys(entry.payload || {}).length ? `<div class="code-block">${escapeHTML(JSON.stringify(entry.payload, null, 2))}</div>` : ''}
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
      target.innerHTML += `
        <div class="code-block">${escapeHTML(JSON.stringify({
          wait: label,
          attempt: attempt + 1,
          heartbeat_state: diag?.heartbeat_state,
          last_heartbeat_at: diag?.node?.last_heartbeat_at,
          agent_status: diag?.agent?.status,
          token_rotation_status: diag?.agent?.token_rotation_status,
        }, null, 2))}</div>`;
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
    if (revisionTarget) revisionTarget.innerHTML = '<span class="tag warn">saving revision</span>';
    if (jobTarget) jobTarget.innerHTML = '';
    try {
      const form = new FormData(document.getElementById('editInstanceForm'));
      const spec = buildInstanceSpecPayload(instance.service_code, form, instance.spec || {}, Number(form.get('endpoint_port') || instance.endpoint_port || 0));
      const data = await sendJSON(`/api/v1/instances/${instance.id}/spec`, 'PUT', { spec });
      instance.spec = spec;
      if (revisionTarget) revisionTarget.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
      if (applyAfterSave && jobTarget) {
        await runInstanceAction(instance.id, 'apply', jobTarget);
      }
    } catch (err) {
      if (revisionTarget) revisionTarget.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
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
      clients: openCreateClientModal,
    };
    const handler = handlers[state.page];
    btn.disabled = !handler;
    btn.textContent = handler ? 'Create' : 'No create action';
    btn.onclick = handler || openSettings;
  }

  function openCreateClientModal() {
    openModal('Create client', 'POST /api/v1/clients', `
      <form id="createClientForm" class="form-grid">
        <div class="field"><label>Username</label><input name="username" required placeholder="client-01" /></div>
        <div class="field"><label>Display name</label><input name="display_name" placeholder="Client 01" /></div>
        <div class="field"><label>Email</label><input name="email" type="email" placeholder="client@example.com" /></div>
        <div class="field"><label>Expires at</label><input name="expires_at" type="datetime-local" /></div>
        <div class="field full"><label>Notes</label><textarea name="notes" rows="6" placeholder="Optional notes, contract reference or operator comment."></textarea></div>
        <div class="field full inline-actions"><button class="primary-btn" type="submit">Create client</button></div>
      </form>
      <div id="createClientResult" class="form-result"></div>`);
    document.getElementById('createClientForm').addEventListener('submit', createClient);
  }

  async function createClient(event) {
    event.preventDefault();
    const target = document.getElementById('createClientResult');
    target.innerHTML = '<span class="tag warn">creating</span>';
    try {
      const form = new FormData(event.currentTarget);
      const expiresAtRaw = String(form.get('expires_at') || '').trim();
      const payload = {
        username: String(form.get('username') || '').trim(),
        display_name: String(form.get('display_name') || '').trim(),
        email: String(form.get('email') || '').trim(),
        notes: String(form.get('notes') || '').trim(),
      };
      if (expiresAtRaw) payload.expires_at = new Date(expiresAtRaw).toISOString();
      const data = await sendJSON('/api/v1/clients', 'POST', payload);
      target.innerHTML = `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
      await refresh();
      setTimeout(closeModal, 400);
    } catch (err) {
      target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
    }
  }

  function render() {
    renderNav();
    renderAuthSlot();
    renderCreateAction();
    renderNotice();
    if (!state.authUser) {
      if (state.inviteToken) {
        renderInviteAcceptScreen();
        return;
      }
      renderLoginScreen();
      return;
    }
    if (state.page === 'dashboard') renderDashboard();
    else if (state.page === 'nodes') renderNodes();
    else if (state.page === 'services') renderServices();
    else if (state.page === 'instances') renderInstances();
    else if (state.page === 'clients') renderClients();
    else if (state.page === 'jobs') renderJobs();
    else if (state.page === 'artifacts') renderArtifacts();
    else if (state.page === 'shareLinks') renderShareLinks();
    else if (state.page === 'certificates') renderCertificates();
    else if (state.page === 'revisions') renderRevisions();
    else if (state.page === 'telemetry') renderTelemetry();
    else if (state.page === 'audit') renderAudit();
    else if (state.page === 'settings') renderSettings();
    else renderUnknownPage(state.page);
  }

  async function refresh() {
    try {
      if (!state.authUser && state.inviteToken) {
        await loadInvitePreview();
      }
      await loadSession();
      await loadCore();
      state.lastError = null;
    } catch (err) {
      state.lastError = err;
    }
    render();
  }

  function bind() {
    el('refreshBtn').addEventListener('click', refresh);
    el('openSettingsBtn').addEventListener('click', openSettings);
    el('closeModalBtn').addEventListener('click', closeModal);
    el('modalBackdrop').addEventListener('click', (event) => {
      if (event.target === el('modalBackdrop')) closeModal();
    });
    window.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') closeModal();
    });
  }

  bind();
  refresh();
})();
