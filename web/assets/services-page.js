(function (window) {
  'use strict';

  function createServicesPage(ctx = {}) {
    const {
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
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof fetchJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof watchJob !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openUnavailableAction !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function'
    ) {
      throw new Error('MegaVPNServicesPage requires page dependencies');
    }

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

    function render() {
      setTitle('Services');
      const node = selectedNode();
      const runtimeServices = groupInstallersByService(state.serviceInstallers || []);
      const capabilities = node ? (state.serviceCapabilitiesByNode[node.id] || []) : [];
      const events = node ? (state.serviceInstallEventsByNode[node.id] || []) : [];
      const definitions = Array.isArray(state.servicesCatalog) ? state.servicesCatalog : [];
      el('content').innerHTML = `
        <section class="card">
          <div class="inline-actions">
            <div class="field" style="min-width:280px">
              <label>Target node</label>
              <select id="servicesNodeSelect">
                ${state.nodes.map((item) => `<option value="${escapeHTML(item.id)}"${item.id === node?.id ? ' selected' : ''}>${escapeHTML(item.name)} · ${escapeHTML(item.address)} · ${escapeHTML(item.agent_status || 'unknown')}</option>`).join('')}
              </select>
            </div>
            <button class="secondary-btn" id="refreshServicesBtn" type="button">Refresh service state</button>
          </div>
        </section>
        <div class="services-grid">
          ${runtimeServices.map((item) => renderServiceRuntimeCard(item, node, capabilities)).join('')}
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
      if (selectedNodeID) {
        state.serviceInstallEventsByNode[selectedNodeID] = await fetchJSON(`/api/v1/nodes/${selectedNodeID}/capabilities/install-events`, []);
      }
      if (state.page === 'services') render();
    }

    async function runInstaller(serviceCode, strategy, channel) {
      if (!state.servicesNodeID) {
        openUnavailableAction('No target node', 'Select a node before installing a runtime capability.');
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
