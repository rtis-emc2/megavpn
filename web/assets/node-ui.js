(function (window) {
  'use strict';

  function createNodeUI(ctx = {}) {
    const {
      nodeExecutionModes = {},
      escapeHTML,
      formatDate,
      formatRelativeDate,
    } = ctx;
    if (
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof formatRelativeDate !== 'function'
    ) {
      throw new Error('MegaVPNNodeUI requires formatting helpers');
    }

    function nodeExecutionLabel(value) {
      return nodeExecutionModes[value]?.title || value || 'unknown';
    }

    function nodeAgentChannelStatus(node) {
      const lifecycle = String(node?.status || '').trim().toLowerCase();
      const agent = String(node?.agent_status || '').trim().toLowerCase();
      if (lifecycle === 'bootstrapping' && agent === 'starting') return 'awaiting heartbeat';
      return agent || 'unknown';
    }

    function nodeLifecycleStatus(node) {
      const lifecycle = String(node?.status || '').trim().toLowerCase();
      const agent = String(node?.agent_status || '').trim().toLowerCase();
      if (lifecycle === 'bootstrapping' && agent === 'starting') return 'waiting heartbeat';
      return lifecycle || 'draft';
    }

    function renderNodeExecutionOptions(selectedMode = 'ssh_bootstrap') {
      return Object.entries(nodeExecutionModes).map(([mode, meta]) => `
        <label class="choice-card node-onboarding-option" data-mode="${escapeHTML(mode)}" title="${escapeHTML(`${meta.description} ${meta.requirements}`)}">
          <input type="radio" name="node_setup_choice" value="${escapeHTML(mode)}"${mode === selectedMode ? ' checked' : ''} />
          <span>
            <strong>${escapeHTML(meta.title)}</strong>
            <em>${escapeHTML(meta.caption)}</em>
            <small>${escapeHTML(meta.requirements)}</small>
          </span>
        </label>`).join('');
    }

    function bindNodeOnboardingPicker(form) {
      const hidden = form.querySelector('input[name="execution_mode"]');
      const options = Array.from(form.querySelectorAll('.node-onboarding-option'));
      const activate = (mode) => {
        if (hidden) hidden.value = mode;
        options.forEach((option) => {
          const active = option.dataset.mode === mode;
          option.classList.toggle('is-selected', active);
          const radio = option.querySelector('input[type="radio"]');
          if (radio) radio.checked = active;
        });
      };
      options.forEach((option) => {
        option.addEventListener('click', () => activate(option.dataset.mode));
        option.querySelector('input[type="radio"]')?.addEventListener('change', () => activate(option.dataset.mode));
      });
      activate(hidden?.value || 'ssh_bootstrap');
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

    function bootstrapRunReason(run) {
      const result = run?.result_payload || {};
      const stage = String(result.stage || '').trim();
      const error = String(result.error || result.message || '').trim();
      if (stage && error) return `${stage}: ${error}`;
      if (error) return error;
      if (stage) return stage;
      return run?.status === 'failed' ? 'Open details' : 'n/a';
    }

    function defaultNodeConsoleTab(diag) {
      const communication = String(diag?.communication_state || '');
      const lastFailed = diag?.last_failed_bootstrap;
      if (communication === 'awaiting_enrollment' || lastFailed) return 'bootstrap';
      if (communication && communication !== 'healthy' && communication !== 'unknown') return 'agent';
      return 'overview';
    }

    function nodeConsoleTabButton(id, label, caption, activeTab) {
      const active = id === activeTab;
      return `
        <button class="node-tab-btn${active ? ' is-active' : ''}" type="button" data-node-tab="${escapeHTML(id)}" aria-selected="${active ? 'true' : 'false'}">
          <span>${escapeHTML(label)}</span>
          <small>${escapeHTML(caption)}</small>
        </button>`;
    }

    function bindNodeConsoleTabs() {
      const buttons = Array.from(document.querySelectorAll('.node-tab-btn'));
      const panels = Array.from(document.querySelectorAll('.node-tab-panel'));
      const activate = (tabID) => {
        buttons.forEach((button) => {
          const active = button.dataset.nodeTab === tabID;
          button.classList.toggle('is-active', active);
          button.setAttribute('aria-selected', active ? 'true' : 'false');
        });
        panels.forEach((panel) => {
          panel.classList.toggle('is-active', panel.dataset.nodePanel === tabID);
        });
      };
      buttons.forEach((button) => button.addEventListener('click', () => activate(button.dataset.nodeTab)));
    }

    function switchNodeConsoleTab(tabID) {
      Array.from(document.querySelectorAll('.node-tab-btn')).find((button) => button.dataset.nodeTab === tabID)?.click();
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
      const communication = String(diag?.communication_state || '').trim();
      if (communication === 'awaiting_enrollment') return 'awaiting enrollment';
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

    return {
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
    };
  }

  window.MegaVPNNodeUI = { create: createNodeUI };
})(window);
