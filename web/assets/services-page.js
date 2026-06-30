(function (window) {
  'use strict';

  function createServicesPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      fetchJSON,
      requestJSON,
      sendJSON,
      watchJob,
      openModal,
      closeModal,
      openUnavailableAction,
      statusTag,
      escapeHTML,
      formatDate,
      hasPermission,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof fetchJSON !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof watchJob !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openUnavailableAction !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof hasPermission !== 'function'
    ) {
      throw new Error('MegaVPNServicesPage requires page dependencies');
    }

    const binaryArtifactServices = ['xray-core', 'shadowsocks', 'openvpn', 'wireguard', 'nginx'];
    const binaryArtifactPresets = [
      {
        key: 'xray_release_zip',
        label: 'Xray release ZIP',
        summary: 'GitHub ZIP/bundle. Agent extracts the xray executable and installs it.',
        source_mode: 'url',
        service_code: 'xray-core',
        kind: 'bundle',
        install_mode: 'zip_binary',
        install_path: '/usr/local/bin/xray',
        archive_binary_path: 'xray',
        name: 'xray-core-release',
        version: '',
        os_family: 'linux',
        architecture: 'amd64',
        url_placeholder: 'https://github.com/XTLS/Xray-core/releases/download/vX.Y.Z/Xray-linux-64.zip',
      },
      {
        key: 'xray_binary',
        label: 'Xray single binary',
        summary: 'Already extracted executable. Agent copies it to the allowlisted xray path.',
        source_mode: 'upload',
        service_code: 'xray-core',
        kind: 'runtime',
        install_mode: 'copy_binary',
        install_path: '/usr/local/bin/xray',
        archive_binary_path: '',
        name: 'xray-core-binary',
        version: '',
        os_family: 'linux',
        architecture: 'amd64',
        url_placeholder: '',
      },
      {
        key: 'shadowsocks_binary',
        label: 'Shadowsocks ss-server binary',
        summary: 'Standalone ss-server executable. Agent copies it to the allowlisted path.',
        source_mode: 'upload',
        service_code: 'shadowsocks',
        kind: 'runtime',
        install_mode: 'copy_binary',
        install_path: '/usr/local/bin/ss-server',
        archive_binary_path: '',
        name: 'shadowsocks-ss-server',
        version: '',
        os_family: 'linux',
        architecture: 'amd64',
        url_placeholder: '',
      },
      {
        key: 'shadowsocks_deb',
        label: 'Shadowsocks Debian package',
        summary: 'Pinned .deb package. Agent installs it through dpkg.',
        source_mode: 'upload',
        service_code: 'shadowsocks',
        kind: 'package',
        install_mode: 'deb_package',
        install_path: '',
        archive_binary_path: '',
        name: 'shadowsocks-package',
        version: '',
        os_family: 'linux',
        architecture: 'amd64',
        url_placeholder: '',
      },
      {
        key: 'custom',
        label: 'Custom artifact',
        summary: 'Manual metadata for a non-standard runtime artifact.',
        source_mode: 'upload',
        service_code: '',
        kind: 'runtime',
        install_mode: '',
        install_path: '',
        archive_binary_path: '',
        name: '',
        version: '',
        os_family: 'linux',
        architecture: 'amd64',
        url_placeholder: '',
      },
    ];

    function selectedNode() {
      return state.nodes.find((node) => node.id === state.servicesNodeID) || state.nodes[0] || null;
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

    function binaryArtifactsForService(serviceCode) {
      const code = String(serviceCode || '').trim();
      return (state.binaryArtifacts || []).filter((artifact) => String(artifact.service_code || '').trim() === code && String(artifact.status || 'active').toLowerCase() === 'active');
    }

    function binaryArtifactSummary(serviceCode) {
      const artifacts = binaryArtifactsForService(serviceCode);
      if (!artifacts.length) return '<span class="tag warn">no artifact</span>';
      const arches = Array.from(new Set(artifacts.map((artifact) => artifact.architecture || 'arch').filter(Boolean)));
      return `<span class="tag ok">${escapeHTML(String(artifacts.length))} artifact${artifacts.length === 1 ? '' : 's'}</span><span class="tag">${escapeHTML(arches.join(', '))}</span>`;
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
                  <strong>${escapeHTML(serviceInstallerDisplayLabel(installer, item.serviceCode))}</strong>
                  ${serviceInstallerStateTag(installer, capability)}
                  ${String(installer.strategy || '') === 'binary_repository' ? binaryArtifactSummary(item.serviceCode) : ''}
                </div>
                <span>${escapeHTML(installer.description || '')}</span>
              </div>
              <div class="inline-actions">
                  <button class="secondary-btn service-verify-btn" type="button" data-service-code="${escapeHTML(item.serviceCode)}">Verify</button>
                  <button class="primary-btn service-install-btn" type="button" data-service-code="${escapeHTML(item.serviceCode)}" data-strategy="${escapeHTML(installer.strategy || '')}" data-channel="${escapeHTML(installer.channel || '')}" title="${escapeHTML(installer.description || '')}"${node ? '' : ' disabled'}>${escapeHTML(serviceInstallerPrimaryLabel(installer, capability, item.serviceCode))}</button>
                </div>
              </div>
            `).join('')}
          </div>
        </section>`;
    }

    function serviceInstallerDisplayLabel(installer, serviceCode = '') {
      const strategy = String(installer?.strategy || '').trim();
      switch (strategy) {
      case 'binary_repository':
        return 'Pinned artifact';
      case 'xtls_install_release':
        return 'XTLS release script';
      case 'nginx_org_repo':
        return 'nginx.org repo';
      case 'ubuntu_repo':
        return String(serviceCode || '').trim() === 'shadowsocks' ? 'Ubuntu package: libev' : 'Ubuntu package';
      case 'manual_present':
        return 'Already installed';
      default:
        return strategy || 'Default';
      }
    }

    function serviceInstallerPrimaryLabel(installer, capability, serviceCode = '') {
      const strategy = String(installer?.strategy || '').trim();
      const status = String(capability?.status || '').trim().toLowerCase();
      switch (strategy) {
      case 'manual_present':
        return status === 'available' ? 'Re-verify' : 'Register';
      case 'binary_repository':
        return status === 'available' ? 'Reinstall artifact' : 'Install artifact';
      case 'xtls_install_release':
        return status === 'available' ? 'Reinstall XTLS' : 'Install XTLS';
      case 'nginx_org_repo':
        return status === 'available' ? 'Reinstall nginx.org' : 'Install nginx.org';
      case 'ubuntu_repo':
        if (String(serviceCode || '').trim() === 'shadowsocks') {
          return status === 'available' ? 'Reinstall libev' : 'Install libev';
        }
        return status === 'available' ? 'Reinstall Ubuntu pkg' : 'Install Ubuntu pkg';
      default:
        return status === 'available' ? 'Reinstall' : 'Install';
      }
    }

    function serviceInstallerStateTag(installer, capability) {
      const strategy = String(installer?.strategy || '').trim();
      const status = String(capability?.status || '').trim().toLowerCase();
      if (strategy === 'manual_present') {
        return status === 'available' ? statusTag('detected') : '<span class="tag">manual</span>';
      }
      if (status === 'available') return statusTag('installed');
      if (status === 'failed') return statusTag('failed');
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

    function renderBinaryArtifactsTable(artifacts) {
      const rows = artifacts.length
        ? artifacts.map((entry) => `
          <tr>
            <td><strong>${escapeHTML(entry.name || 'artifact')}</strong><div class="mono small">${escapeHTML(entry.id || '')}</div></td>
            <td>${escapeHTML(entry.service_code || 'n/a')}</td>
            <td>
              ${escapeHTML(entry.kind || 'n/a')}
              ${entry.metadata?.install_mode ? `<div class="muted small">${escapeHTML(entry.metadata.install_mode)}</div>` : ''}
              ${entry.metadata?.archive_binary_path ? `<div class="muted small">member: ${escapeHTML(entry.metadata.archive_binary_path)}</div>` : ''}
            </td>
            <td>${escapeHTML(entry.version || 'n/a')}</td>
            <td>${escapeHTML(entry.os_family || 'linux')} · ${escapeHTML(entry.os_version || 'any')} · ${escapeHTML(entry.architecture || 'arch')}</td>
            <td>
              <span class="mono small" title="${escapeHTML(entry.sha256 || '')}">${escapeHTML(shortHash(entry.sha256 || ''))}</span>
              <div class="muted small">${escapeHTML(formatBytes(entry.size_bytes || 0))}</div>
            </td>
            <td>${statusTag(entry.status || 'unknown')}</td>
          </tr>`).join('')
        : '<tr><td colspan="7"><div class="empty">No runtime artifacts registered. Add a pinned artifact before using binary_repository installs.</div></td></tr>';
      return `<table><thead><tr><th>Artifact</th><th>Service</th><th>Kind</th><th>Version</th><th>Target</th><th>SHA-256</th><th>Status</th></tr></thead><tbody>${rows}</tbody></table>`;
    }

    function renderRuntimeServicesTable(runtimeServices, node, capabilities) {
      const rows = runtimeServices.length
        ? runtimeServices.map((item) => {
          const capability = (capabilities || []).find((entry) => entry.capability_code === item.serviceCode);
          const definition = (state.servicesCatalog || []).find((entry) => entry.code === item.serviceCode || (item.serviceCode === 'xray-core' && entry.code === 'xray'));
          const hasBinaryRepository = item.installers.some((installer) => String(installer.strategy || '') === 'binary_repository');
          return `
            <tr>
              <td>
                <strong>${escapeHTML(definition?.name || item.serviceCode)}</strong>
                <div class="mono small">${escapeHTML(item.serviceCode)}</div>
              </td>
              <td>
                ${statusTag(capability?.status || 'missing')}
                <div class="muted small">${escapeHTML(capability?.version || 'not registered')}</div>
              </td>
              <td>${hasBinaryRepository ? binaryArtifactSummary(item.serviceCode) : '<span class="tag">not used</span>'}</td>
              <td>
                <div class="runtime-installer-list">
                  ${item.installers.map((installer) => `
                    <span class="tag" title="${escapeHTML(installer.description || '')}">${escapeHTML(serviceInstallerDisplayLabel(installer, item.serviceCode))}</span>
                  `).join('')}
                </div>
              </td>
              <td>
                <div class="runtime-action-grid">
                  <button class="secondary-btn service-verify-btn" type="button" data-service-code="${escapeHTML(item.serviceCode)}">Verify</button>
                  ${item.installers.map((installer) => `
                    <button class="${String(installer.strategy || '') === 'manual_present' ? 'secondary-btn' : 'primary-btn'} service-install-btn" type="button" data-service-code="${escapeHTML(item.serviceCode)}" data-strategy="${escapeHTML(installer.strategy || '')}" data-channel="${escapeHTML(installer.channel || '')}" title="${escapeHTML(installer.description || '')}"${node ? '' : ' disabled'}>${escapeHTML(serviceInstallerPrimaryLabel(installer, capability, item.serviceCode))}</button>
                  `).join('')}
                </div>
              </td>
            </tr>`;
        }).join('')
        : '<tr><td colspan="5"><div class="empty">No runtime installers loaded.</div></td></tr>';
      return `<table class="runtime-services-table"><thead><tr><th>Runtime</th><th>Node status</th><th>Repository</th><th>Install methods</th><th>Actions</th></tr></thead><tbody>${rows}</tbody></table>`;
    }

    function renderRepositoryWorkflow() {
      return `
        <div class="artifact-workflow">
          <div><strong>1. Register</strong><span>Upload from this computer or let the control plane fetch a pinned HTTPS URL.</span></div>
          <div><strong>2. Pin</strong><span>Store SHA-256, target OS/architecture and install metadata once.</span></div>
          <div><strong>3. Install</strong><span>Agents download the signed artifact only for their own install job.</span></div>
        </div>`;
    }

    function renderNodeRuntimeTarget(node) {
      return `
        <section class="card service-node-target-card">
          <div>
            <div class="mini-label">Node runtime status</div>
            <h2>Install and verify target</h2>
            <p>Selecting a node changes only the capability status, verify jobs and install jobs below. Repository artifacts are global.</p>
          </div>
          <div class="inline-actions">
            <div class="field" style="min-width:320px">
              <label>Target node</label>
              <select id="servicesNodeSelect">
                ${state.nodes.map((item) => `<option value="${escapeHTML(item.id)}"${item.id === node?.id ? ' selected' : ''}>${escapeHTML(item.name)} · ${escapeHTML(item.address)} · ${escapeHTML(item.agent_status || 'unknown')}</option>`).join('')}
              </select>
            </div>
            <button class="secondary-btn" id="refreshServicesBtn" type="button">Refresh state</button>
          </div>
        </section>`;
    }

    function shortHash(value) {
      const hash = String(value || '').trim();
      if (hash.length <= 24) return hash || 'n/a';
      return `${hash.slice(0, 12)}...${hash.slice(-8)}`;
    }

    function formatBytes(value) {
      const size = Number(value || 0);
      if (!Number.isFinite(size) || size <= 0) return '0 B';
      if (size < 1024) return `${size} B`;
      if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
      if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MiB`;
      return `${(size / 1024 / 1024 / 1024).toFixed(1)} GiB`;
    }

    function render() {
      setTitle('Services');
      const node = selectedNode();
      const runtimeServices = groupInstallersByService(state.serviceInstallers || []);
      const capabilities = node ? (state.serviceCapabilitiesByNode[node.id] || []) : [];
      const events = node ? (state.serviceInstallEventsByNode[node.id] || []) : [];
      const definitions = Array.isArray(state.servicesCatalog) ? state.servicesCatalog : [];
      const binaryArtifacts = Array.isArray(state.binaryArtifacts) ? state.binaryArtifacts : [];
      const canManageBinaryRepository = hasPermission('binary_repository.manage');
      el('content').innerHTML = `
        <section class="table-card services-repository-card">
          <div class="table-head">
            <div>
              <div class="mini-label">Global runtime repository</div>
              <h2>Runtime Binary Repository</h2>
              <p>Store Xray, Shadowsocks and other pinned runtime artifacts once on the control plane. Nodes receive them through signed job-scoped downloads.</p>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(binaryArtifacts.length))} artifacts</span>
              ${canManageBinaryRepository ? '<button class="secondary-btn" id="addBinaryArtifactBtn" type="button">Add artifact</button>' : ''}
            </div>
          </div>
          ${renderRepositoryWorkflow()}
          <div class="table-wrap">${renderBinaryArtifactsTable(binaryArtifacts)}</div>
        </section>
        ${renderNodeRuntimeTarget(node)}
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Runtime Capabilities</h2>
              <p>Capability status and installer actions for the selected node.</p>
            </div>
            <div class="table-tools">${node ? `<span class="tag">${escapeHTML(node.name)}</span>` : '<span class="tag warn">no node</span>'}</div>
          </div>
          <div class="table-wrap">${renderRuntimeServicesTable(runtimeServices, node, capabilities)}</div>
        </section>
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
            <div class="table-head"><h2>Recent Capability Events</h2><div class="table-tools"><span class="tag">${escapeHTML(node?.name || 'node')}</span></div></div>
            <div class="table-wrap">${renderCapabilityEventsTable(events)}</div>
          </section>
        </section>`;
      bindActions();
      if (!state.serviceCapabilitiesByNode[node?.id || '']) {
        void loadData();
      }
    }

    function bindActions() {
      document.getElementById('servicesNodeSelect')?.addEventListener('change', async (event) => {
        state.servicesNodeID = event.currentTarget.value;
        localStorage.setItem('megavpn.servicesNodeID', state.servicesNodeID);
        render();
        await loadData();
      });
      document.getElementById('refreshServicesBtn')?.addEventListener('click', loadData);
      document.getElementById('addBinaryArtifactBtn')?.addEventListener('click', () => openBinaryArtifactModal());
      document.querySelectorAll('.service-install-btn').forEach((button) => {
        button.addEventListener('click', () => runInstaller(button.dataset.serviceCode, button.dataset.strategy, button.dataset.channel));
      });
      document.querySelectorAll('.service-verify-btn').forEach((button) => {
        button.addEventListener('click', () => verifyCapability(button.dataset.serviceCode));
      });
    }

    async function loadData() {
      if (!state.authUser || !state.nodes.length) return;
      const selectedNodeID = state.servicesNodeID || state.nodes[0]?.id || '';
      const pairs = await Promise.all(state.nodes.map(async (node) => {
        const capabilities = await fetchJSON(`/api/v1/nodes/${node.id}/capabilities`, []);
        return [node.id, capabilities || []];
      }));
      state.serviceCapabilitiesByNode = Object.fromEntries(pairs);
      state.binaryArtifacts = hasPermission('binary_repository.read') ? await fetchJSON('/api/v1/binary-artifacts', []) : [];
      if (selectedNodeID) {
        state.serviceInstallEventsByNode[selectedNodeID] = await fetchJSON(`/api/v1/nodes/${selectedNodeID}/capabilities/install-events`, []);
      }
      if (state.page === 'services') render();
    }

    function openBinaryArtifactModal(prefillServiceCode = '') {
      if (!hasPermission('binary_repository.manage')) {
        openUnavailableAction('Binary repository', 'Your role can read runtime artifacts but cannot register new artifacts.');
        return;
      }
      const initialPreset = initialBinaryArtifactPreset(prefillServiceCode);
      openModal('Add runtime artifact', 'Binary repository', `
        <form id="binaryArtifactForm" class="form-grid">
          <div class="field full">
            <label>Artifact type</label>
            <select name="preset_key" id="binaryArtifactPreset">
              ${binaryArtifactPresets.map((preset) => `<option value="${escapeHTML(preset.key)}"${preset.key === initialPreset.key ? ' selected' : ''}>${escapeHTML(preset.label)}</option>`).join('')}
            </select>
            <div class="field-hint" id="binaryArtifactPresetHint">${escapeHTML(initialPreset.summary || '')}</div>
          </div>
          <div class="field">
            <label>Source</label>
            <select name="source_mode" id="binaryArtifactSourceMode">
              <option value="upload">Upload from this computer</option>
              <option value="url">Fetch by control plane from HTTPS URL</option>
            </select>
          </div>
          <div class="field full" id="binaryArtifactFileField">
            <label>Artifact file</label>
            <input name="file" type="file" required>
            <div class="field-hint">The control plane stores the file under the configured artifact root and calculates SHA-256 before registration.</div>
          </div>
          <div class="field full" id="binaryArtifactURLField" hidden>
            <label>Source URL</label>
            <input name="source_url" type="url" placeholder="${escapeHTML(initialPreset.url_placeholder || 'https://host/path/artifact')}">
            <div class="field-hint">The control plane downloads this URL directly. HTTPS and expected SHA-256 are required; private/loopback targets are rejected.</div>
          </div>
          <div class="field">
            <label>Name</label>
            <input name="name" placeholder="xray-core-release" required>
          </div>
          <div class="field">
            <label>Service</label>
            <select name="service_code" required>
              <option value="">Select service</option>
              ${binaryArtifactServices.map((code) => `<option value="${escapeHTML(code)}"${code === initialPreset.service_code ? ' selected' : ''}>${escapeHTML(code)}</option>`).join('')}
            </select>
          </div>
          <div class="field">
            <label>Kind</label>
            <select name="kind">
              <option value="script">script</option>
              <option value="package">package</option>
              <option value="runtime">runtime</option>
              <option value="bundle">bundle</option>
            </select>
          </div>
          <div class="field">
            <label>Version</label>
            <input name="version" placeholder="1.0.0" required>
          </div>
          <div class="field">
            <label>OS family</label>
            <input name="os_family" value="linux" required>
          </div>
          <div class="field">
            <label>OS version</label>
            <input name="os_version" placeholder="ubuntu-24.04 or empty for any">
          </div>
          <div class="field">
            <label>Architecture</label>
            <select name="architecture">
              <option value="amd64">amd64</option>
              <option value="arm64">arm64</option>
            </select>
          </div>
          <div class="field">
            <label>Install mode</label>
            <select name="install_mode">
              <option value="">auto by kind</option>
              <option value="copy_binary">copy_binary</option>
              <option value="zip_binary">zip_binary</option>
              <option value="xray_install_script">xray_install_script</option>
              <option value="deb_package">deb_package</option>
            </select>
          </div>
          <div class="field">
            <label>Install path</label>
            <input name="install_path" placeholder="/usr/local/bin/xray or /usr/local/bin/ss-server">
            <div class="field-hint">Only service-specific allowlisted paths are accepted by the agent for binary copy modes.</div>
          </div>
          <div class="field" id="binaryArtifactArchivePathField" hidden>
            <label>Binary path inside archive</label>
            <input name="archive_binary_path" placeholder="xray">
            <div class="field-hint">For ZIP/bundle artifacts, this is the executable member extracted by the agent.</div>
          </div>
          <div class="field full">
            <label>Repository path</label>
            <input name="storage_path" placeholder="auto-generated when empty">
            <div class="field-hint">Optional relative path under the control-plane artifact root. Leave empty for generated runtime-repository path.</div>
          </div>
          <div class="field full">
            <label id="binaryArtifactSHA256Label">Expected SHA-256</label>
            <input name="expected_sha256" pattern="[a-fA-F0-9]{64}" placeholder="optional 64 hex characters">
            <div class="field-hint" id="binaryArtifactSHA256Hint">Leave empty for browser uploads unless you already have a known checksum.</div>
          </div>
          <div class="modal-actions field full">
            <button class="secondary-btn" type="button" id="cancelBinaryArtifactBtn">Cancel</button>
            <button class="primary-btn" type="submit">Upload artifact</button>
          </div>
        </form>
        <div id="binaryArtifactResult" class="form-result"></div>`);
      document.getElementById('cancelBinaryArtifactBtn')?.addEventListener('click', closeModal);
      document.getElementById('binaryArtifactPreset')?.addEventListener('change', () => applyBinaryArtifactPreset(true));
      document.getElementById('binaryArtifactSourceMode')?.addEventListener('change', updateBinaryArtifactSourceMode);
      document.querySelector('#binaryArtifactFileField input[name="file"]')?.addEventListener('change', inferBinaryArtifactFromFile);
      document.querySelector('#binaryArtifactURLField input[name="source_url"]')?.addEventListener('input', inferBinaryArtifactFromURL);
      document.querySelector('#binaryArtifactForm select[name="kind"]')?.addEventListener('change', updateBinaryArtifactArchiveField);
      document.querySelector('#binaryArtifactForm select[name="install_mode"]')?.addEventListener('change', updateBinaryArtifactArchiveField);
      applyBinaryArtifactPreset(true);
      updateBinaryArtifactSourceMode();
      document.getElementById('binaryArtifactForm')?.addEventListener('submit', async (event) => {
        event.preventDefault();
        const form = event.currentTarget;
        const result = document.getElementById('binaryArtifactResult');
        const submitButton = form.querySelector('button[type="submit"]');
        const cancelButton = document.getElementById('cancelBinaryArtifactBtn');
        if (form.dataset.submitting === '1' || form.dataset.completed === '1') {
          return;
        }
        const data = new FormData(form);
        const sourceMode = String(data.get('source_mode') || 'upload');
        const idleLabel = sourceMode === 'url' ? 'Fetch artifact' : 'Upload artifact';
        form.dataset.submitting = '1';
        if (submitButton) {
          submitButton.disabled = true;
          submitButton.textContent = sourceMode === 'url' ? 'Fetching artifact' : 'Uploading artifact';
        }
        result.innerHTML = sourceMode === 'url'
          ? '<span class="tag warn">fetching artifact</span>'
          : '<span class="tag warn">uploading artifact</span>';
        try {
          let artifact;
          if (sourceMode === 'url') {
            artifact = await sendJSON('/api/v1/binary-artifacts/import-url', 'POST', binaryArtifactURLPayload(data));
          } else {
            artifact = await requestJSON('/api/v1/binary-artifacts/import', {
              method: 'POST',
              body: data,
            });
          }
          state.binaryArtifacts = await fetchJSON('/api/v1/binary-artifacts', []);
          form.dataset.completed = '1';
          setBinaryArtifactFormCompleted(form);
          if (cancelButton) cancelButton.textContent = 'Close';
          result.innerHTML = renderBinaryArtifactImportSuccess(artifact, sourceMode);
          render();
        } catch (err) {
          result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        } finally {
          delete form.dataset.submitting;
          if (form.dataset.completed !== '1' && submitButton) {
            submitButton.disabled = false;
            submitButton.textContent = idleLabel;
          }
        }
      });
    }

    function setBinaryArtifactFormCompleted(form) {
      form.querySelectorAll('input, select, button').forEach((control) => {
        if (control.id === 'cancelBinaryArtifactBtn') return;
        control.disabled = true;
      });
    }

    function renderBinaryArtifactImportSuccess(artifact, sourceMode) {
      const name = artifact?.name || 'runtime artifact';
      const version = artifact?.version || 'n/a';
      const serviceCode = artifact?.service_code || 'n/a';
      const sha = artifact?.sha256 || '';
      const mode = sourceMode === 'url' ? 'Fetched and registered' : 'Uploaded and registered';
      return `
        <div class="artifact-success-panel">
          <div class="inline-actions artifact-success-head">
            <span class="tag ok">registered</span>
            <span class="tag">${escapeHTML(mode)}</span>
          </div>
          <strong>${escapeHTML(name)}</strong>
          <p>The artifact is now in the global runtime repository. This dialog is locked to prevent duplicate imports.</p>
          <div class="artifact-success-grid">
            <div><span>Service</span><strong>${escapeHTML(serviceCode)}</strong></div>
            <div><span>Version</span><strong>${escapeHTML(version)}</strong></div>
            <div class="full"><span>SHA-256</span><strong class="mono">${escapeHTML(sha || 'calculated and stored')}</strong></div>
          </div>
        </div>`;
    }

    function updateBinaryArtifactSourceMode() {
      const sourceMode = String(document.getElementById('binaryArtifactSourceMode')?.value || 'upload');
      const fileField = document.getElementById('binaryArtifactFileField');
      const urlField = document.getElementById('binaryArtifactURLField');
      const fileInput = document.querySelector('#binaryArtifactFileField input[name="file"]');
      const urlInput = document.querySelector('#binaryArtifactURLField input[name="source_url"]');
      const expectedSHAInput = document.querySelector('#binaryArtifactForm input[name="expected_sha256"]');
      const expectedSHALabel = document.getElementById('binaryArtifactSHA256Label');
      const expectedSHAHint = document.getElementById('binaryArtifactSHA256Hint');
      const submitButton = document.querySelector('#binaryArtifactForm button[type="submit"]');
      if (fileField) fileField.hidden = sourceMode !== 'upload';
      if (urlField) urlField.hidden = sourceMode !== 'url';
      if (fileInput) fileInput.required = sourceMode === 'upload';
      if (urlInput) urlInput.required = sourceMode === 'url';
      if (expectedSHAInput) expectedSHAInput.required = sourceMode === 'url';
      if (expectedSHALabel) expectedSHALabel.textContent = sourceMode === 'url' ? 'Expected SHA-256 (required)' : 'Expected SHA-256 (optional)';
      if (expectedSHAInput) expectedSHAInput.placeholder = sourceMode === 'url' ? 'required 64 hex SHA-256' : 'optional 64 hex characters';
      if (expectedSHAHint) {
        expectedSHAHint.textContent = sourceMode === 'url'
          ? 'Required for URL fetches. Paste the vendor checksum, or download the same file locally, run shasum -a 256 <file>, and paste the 64-character hash.'
          : 'Optional for browser uploads. Leave empty to let the control plane calculate and store SHA-256 after upload.';
      }
      if (submitButton) submitButton.textContent = sourceMode === 'url' ? 'Fetch artifact' : 'Upload artifact';
      updateBinaryArtifactArchiveField();
    }

    function initialBinaryArtifactPreset(serviceCode) {
      const code = String(serviceCode || '').trim();
      if (code === 'xray-core') return binaryArtifactPresets.find((preset) => preset.key === 'xray_release_zip') || binaryArtifactPresets[0];
      if (code === 'shadowsocks') return binaryArtifactPresets.find((preset) => preset.key === 'shadowsocks_binary') || binaryArtifactPresets[0];
      return binaryArtifactPresets[0];
    }

    function selectedBinaryArtifactPreset() {
      const key = String(document.getElementById('binaryArtifactPreset')?.value || '').trim();
      return binaryArtifactPresets.find((preset) => preset.key === key) || binaryArtifactPresets[0];
    }

    function applyBinaryArtifactPreset(overwrite) {
      const form = document.getElementById('binaryArtifactForm');
      if (!form) return;
      const preset = selectedBinaryArtifactPreset();
      const setValue = (name, value) => {
        const input = form.elements[name];
        if (!input) return;
        if (overwrite || String(input.value || '').trim() === '') {
          input.value = value || '';
        }
      };
      setValue('source_mode', preset.source_mode);
      setValue('service_code', preset.service_code);
      setValue('kind', preset.kind);
      setValue('install_mode', preset.install_mode);
      setValue('install_path', preset.install_path);
      setValue('archive_binary_path', preset.archive_binary_path);
      setValue('name', preset.name);
      setValue('version', preset.version);
      setValue('os_family', preset.os_family);
      setValue('architecture', preset.architecture);
      const hint = document.getElementById('binaryArtifactPresetHint');
      if (hint) hint.textContent = preset.summary || '';
      const urlInput = form.elements.source_url;
      if (urlInput) urlInput.placeholder = preset.url_placeholder || 'https://host/path/artifact';
      updateBinaryArtifactSourceMode();
      updateBinaryArtifactArchiveField();
    }

    function updateBinaryArtifactArchiveField() {
      const form = document.getElementById('binaryArtifactForm');
      const field = document.getElementById('binaryArtifactArchivePathField');
      if (!form || !field) return;
      const kind = String(form.elements.kind?.value || '').trim();
      const mode = String(form.elements.install_mode?.value || '').trim();
      const visible = kind === 'bundle' || mode === 'zip_binary';
      field.hidden = !visible;
      const input = form.elements.archive_binary_path;
      if (input) input.required = visible && mode === 'zip_binary';
    }

    function inferBinaryArtifactFromFile(event) {
      const file = event.currentTarget?.files?.[0];
      if (!file) return;
      inferBinaryArtifactFromName(file.name);
    }

    function inferBinaryArtifactFromURL(event) {
      const value = String(event.currentTarget?.value || '').trim();
      if (!value) return;
      try {
        const url = new URL(value);
        const filename = decodeURIComponent(url.pathname.split('/').filter(Boolean).pop() || '');
        inferBinaryArtifactFromName(filename || value);
        const version = inferVersionFromText(value);
        if (version) setBinaryArtifactFieldIfBlank('version', version);
      } catch {
        const version = inferVersionFromText(value);
        if (version) setBinaryArtifactFieldIfBlank('version', version);
      }
    }

    function inferBinaryArtifactFromName(name) {
      const cleanName = String(name || '').trim();
      if (!cleanName) return;
      const form = document.getElementById('binaryArtifactForm');
      if (!form) return;
      const lower = cleanName.toLowerCase();
      if (lower.endsWith('.zip')) {
        setBinaryArtifactFieldIfBlank('kind', 'bundle');
        setBinaryArtifactFieldIfBlank('install_mode', 'zip_binary');
      } else if (lower.endsWith('.deb')) {
        setBinaryArtifactFieldIfBlank('kind', 'package');
        setBinaryArtifactFieldIfBlank('install_mode', 'deb_package');
      } else if (lower.endsWith('.sh')) {
        setBinaryArtifactFieldIfBlank('kind', 'script');
      }
      const version = inferVersionFromText(cleanName);
      if (version) setBinaryArtifactFieldIfBlank('version', version);
      const baseName = cleanName.replace(/\.[^.]+$/, '');
      setBinaryArtifactFieldIfBlank('name', baseName);
      updateBinaryArtifactArchiveField();
    }

    function inferVersionFromText(value) {
      const match = String(value || '').match(/(?:^|[^0-9])v?(\d+\.\d+\.\d+(?:[-+][A-Za-z0-9._-]+)?)(?:[^0-9]|$)/);
      return match ? match[1] : '';
    }

    function setBinaryArtifactFieldIfBlank(name, value) {
      const input = document.getElementById('binaryArtifactForm')?.elements?.[name];
      if (input && String(input.value || '').trim() === '') {
        input.value = value || '';
      }
    }

    function binaryArtifactURLPayload(data) {
      return {
        source_url: String(data.get('source_url') || '').trim(),
        name: String(data.get('name') || '').trim(),
        service_code: String(data.get('service_code') || '').trim(),
        kind: String(data.get('kind') || '').trim(),
        version: String(data.get('version') || '').trim(),
        os_family: String(data.get('os_family') || 'linux').trim(),
        os_version: String(data.get('os_version') || '').trim(),
        architecture: String(data.get('architecture') || '').trim(),
        install_mode: String(data.get('install_mode') || '').trim(),
        install_path: String(data.get('install_path') || '').trim(),
        archive_binary_path: String(data.get('archive_binary_path') || '').trim(),
        storage_path: String(data.get('storage_path') || '').trim(),
        expected_sha256: String(data.get('expected_sha256') || '').trim().toLowerCase(),
        replace_file: false,
      };
    }

    async function runInstaller(serviceCode, strategy, channel) {
      if (!state.servicesNodeID) {
        openUnavailableAction('No target node', 'Select a node before installing a runtime capability.');
        return;
      }
      if (strategy === 'binary_repository' && !binaryArtifactsForService(serviceCode).length) {
        openBinaryArtifactModal(serviceCode);
        return;
      }
      const node = selectedNode();
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
      document.getElementById('cancelInstallBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmInstallBtn')?.addEventListener('click', async () => {
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
          await loadData();
        } catch (err) {
          target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        } finally {
          cancelBtn.disabled = false;
        }
      });
    }

    async function verifyCapability(serviceCode) {
      if (!state.servicesNodeID) {
        openUnavailableAction('No target node', 'Select a node before verifying a runtime capability.');
        return;
      }
      const node = selectedNode();
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
        document.getElementById('cancelVerifyBtn')?.addEventListener('click', closeModal);
        document.getElementById('confirmVerifyBtn')?.addEventListener('click', async () => {
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
            await loadData();
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

    return {
      render,
      loadData,
    };
  }

  window.MegaVPNServicesPage = { create: createServicesPage };
})(window);
