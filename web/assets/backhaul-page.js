(function (window) {
  'use strict';

  function createBackhaulPage(ctx = {}) {
    const {
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
      renderActionResponse,
      formatDate,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof renderActionResponse !== 'function'
    ) {
      throw new Error('MegaVPNBackhaulPage requires page dependencies');
    }
    const getJSON = typeof requestJSON === 'function'
      ? requestJSON
      : (path) => sendJSON(path, 'GET', null);
    const renderDate = typeof formatDate === 'function'
      ? formatDate
      : (value) => String(value || 'n/a');

    function nodeByID(id) {
      return (state.nodes || []).find((node) => node.id === id) || null;
    }

    function roleNodes(role) {
      return (state.nodes || []).filter((node) => String(node.role || '').toLowerCase() === role && node.status !== 'retired');
    }

    function driverDef(code) {
      return (state.backhaulDrivers || []).find((driver) => driver.code === code) || null;
    }

    function driverLabel(code) {
      return driverDef(code)?.label || code || 'n/a';
    }

    function selectedTransport(link) {
      const selectedID = link.selected_transport_id || '';
      return (link.transports || []).find((transport) => transport.id === selectedID)
        || (link.transports || []).find((transport) => transport.driver === link.desired_driver)
        || (link.transports || [])[0]
        || null;
    }

    function renderTransportTags(link) {
      const transports = Array.isArray(link.transports) ? link.transports : [];
      if (!transports.length) return '<span class="tag">no transports</span>';
      return transports.map((transport) => `
        <span class="tag ${transport.id === link.selected_transport_id ? 'ok' : ''}">
          ${escapeHTML(transport.driver)}
        </span>`).join('');
    }

    function transportHealth(transport) {
      const health = transport?.health || {};
      const ingress = health.ingress || {};
      const egress = health.egress || {};
      const status = [ingress.status, egress.status].filter(Boolean);
      const avg = [ingress.latency_avg_ms, egress.latency_avg_ms]
        .map((value) => Number(value))
        .filter((value) => Number.isFinite(value));
      return {
        ingress,
        egress,
        status: status.length ? status.join(' / ') : health.status || 'unknown',
        avgLatency: avg.length ? (avg.reduce((sum, value) => sum + value, 0) / avg.length) : null,
      };
    }

    function firstText(...values) {
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
      const unit = firstText(result.systemd_unit, health.systemd_unit);
      const state = firstText(result.health_active_state, health.active_state, result.active_state);
      const output = firstUsefulJobOutputLine(
        result.health_unit_status_output
        || health.unit_status_output
        || result.systemd_output
        || result.pre_start_stop_warning
      );
      return [unit ? `unit ${unit}` : '', state ? `state ${state}` : '', output].filter(Boolean).join(' · ');
    }

    function jobDetailedFailureText(job, result = {}, health = {}) {
      const reason = firstText(job?.error, result.health_reason, health.reason, result.error);
      const details = jobSystemdDiagnosticText(result, health);
      if (reason && details && /systemd|unit|activation|not active/i.test(reason)) {
        return `${reason} · ${details}`;
      }
      return '';
    }

    function optionalNumber(value) {
      const number = Number(value);
      return Number.isFinite(number) ? number : null;
    }

    function formatPercent(value) {
      const number = optionalNumber(value);
      if (number == null) return '';
      return `${number % 1 === 0 ? number.toFixed(0) : number.toFixed(1)}% loss`;
    }

    function healthSummary(health = {}) {
      const reason = firstText(health.reason, health.route_warning, health.error);
      const peer = firstText(health.peer);
      const loss = formatPercent(health.packet_loss_percent);
      const avg = optionalNumber(health.latency_avg_ms);
      const active = firstText(health.active_state);
      const iface = firstText(health.interface);
      const metrics = [
        active ? `unit ${active}` : '',
        iface ? `dev ${iface}` : '',
        peer ? `peer ${peer}` : '',
        loss,
        avg == null ? '' : `${avg.toFixed(1)} ms avg`,
      ].filter(Boolean).join(' · ');
      return [reason, metrics].filter(Boolean).join(' · ');
    }

    function endpointText(transport) {
      if (!transport) return 'n/a';
      const host = firstText(transport.endpoint_host, 'n/a');
      const port = firstText(transport.endpoint_port, 'n/a');
      const protocol = firstText(transport.protocol);
      return `${host}:${port}${protocol ? ` ${protocol}` : ''}`;
    }

    function renderBackhaulFact(label, value, className = '') {
      return `
        <div class="backhaul-fact ${className}">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(firstText(value, 'n/a'))}</strong>
        </div>`;
    }

    function renderBackhaulFactHTML(label, html, className = '') {
      return `
        <div class="backhaul-fact ${className}">
          <span>${escapeHTML(label)}</span>
          <strong>${html}</strong>
        </div>`;
    }

    function renderRoleHealthLine(role, health = {}, applied = false) {
      const status = firstText(health.status, applied ? 'unknown' : 'not applied');
      const reason = firstText(health.reason, health.route_warning, health.error, applied ? '' : 'not applied');
      const peer = firstText(health.peer);
      const iface = firstText(health.interface);
      const loss = formatPercent(health.packet_loss_percent);
      const avg = optionalNumber(health.latency_avg_ms);
      const active = firstText(health.active_state);
      const metrics = [
        avg == null ? '' : `${avg.toFixed(1)} ms avg`,
        loss,
        peer ? `peer ${peer}` : '',
        iface ? `dev ${iface}` : '',
        active ? `unit ${active}` : '',
      ].filter(Boolean);
      return `
        <div class="backhaul-health-line">
          <div class="backhaul-health-line-head">
            <span class="backhaul-role">${escapeHTML(role)}</span>
            ${statusTag(status)}
          </div>
          ${reason ? `<div class="backhaul-health-reason">${escapeHTML(reason)}</div>` : ''}
          ${metrics.length ? `<div class="backhaul-health-metrics">${metrics.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}</div>` : ''}
        </div>`;
    }

    function renderBackhaulHealthBlock(transport) {
      if (!transport) return '<div class="backhaul-health-empty">unknown</div>';
      const health = transportHealth(transport);
      return `
        <div class="backhaul-health-block">
          ${renderRoleHealthLine('ingress', health.ingress, roleApplied(transport, 'ingress'))}
          ${renderRoleHealthLine('egress', health.egress, roleApplied(transport, 'egress'))}
        </div>`;
    }

    function roleApplied(transport, role) {
      if (!transport) return false;
      return role === 'egress' ? Boolean(transport.applied_egress_at) : Boolean(transport.applied_ingress_at);
    }

    function renderAppliedCell(transport) {
      if (!transport) return '<span class="tag">n/a</span>';
      const ingressApplied = roleApplied(transport, 'ingress');
      const egressApplied = roleApplied(transport, 'egress');
      if (ingressApplied && egressApplied) {
        return '<span class="tag ok">both sides</span>';
      }
      if (ingressApplied || egressApplied) {
        return `
          <div class="stacked-status">
            <span class="tag warn">partial</span>
            <small>${ingressApplied ? 'ingress applied' : 'ingress missing'}</small>
            <small>${egressApplied ? 'egress applied' : 'egress missing'}</small>
          </div>`;
      }
      return '<span class="tag">not applied</span>';
    }

    function renderBackhaulLinkCard(link) {
      const ingress = nodeByID(link.ingress_node_id);
      const egress = nodeByID(link.egress_node_id);
      const transport = selectedTransport(link);
      const blockReason = probeBlockReason(link, transport);
      const selectedDriver = transport?.driver || link.desired_driver;
      const route = `${ingress?.name || link.ingress_node_id || 'ingress'} -> ${egress?.name || link.egress_node_id || 'egress'}`;
      const protocol = transport ? firstText(transport.protocol, 'n/a') : 'n/a';
      const iface = transport ? firstText(transport.interface_name, 'n/a') : 'n/a';
      return `
        <article class="backhaul-row" data-link-id="${escapeHTML(link.id || '')}">
          <div class="backhaul-row-head">
            <div class="backhaul-title-block">
              <h3>${escapeHTML(link.name || 'backhaul')}</h3>
              <div class="backhaul-route">${escapeHTML(route)}</div>
            </div>
            <div class="backhaul-row-tags">
              ${statusTag(link.status || 'unknown')}
              <span class="tag">${escapeHTML(driverLabel(selectedDriver))}</span>
              ${renderAppliedCell(transport)}
            </div>
          </div>
          <div class="backhaul-row-grid">
            <section class="backhaul-panel">
              <div class="backhaul-panel-label">Transport</div>
              <div class="backhaul-facts">
                ${renderBackhaulFact('Endpoint', endpointText(transport), 'wide')}
                ${renderBackhaulFact('Protocol', protocol)}
                ${renderBackhaulFactHTML('Profiles', renderTransportTags(link))}
              </div>
            </section>
            <section class="backhaul-panel">
              <div class="backhaul-panel-label">Tunnel</div>
              <div class="backhaul-facts">
                ${renderBackhaulFact('CIDR', transport?.tunnel_cidr)}
                ${renderBackhaulFact('Ingress IP', transport?.ingress_address)}
                ${renderBackhaulFact('Egress IP', transport?.egress_address)}
                ${renderBackhaulFact('Interface', iface)}
              </div>
            </section>
            <section class="backhaul-panel backhaul-health-panel">
              <div class="backhaul-panel-label">Health</div>
              ${renderBackhaulHealthBlock(transport)}
            </section>
            <section class="backhaul-panel backhaul-actions-panel">
              <div class="backhaul-panel-label">Actions</div>
              <div class="backhaul-actions">
                <button class="secondary-btn inspect-backhaul-btn" type="button" data-link-id="${escapeHTML(link.id || '')}">Manage</button>
                <button class="primary-btn apply-backhaul-btn" type="button" data-link-id="${escapeHTML(link.id || '')}">Apply</button>
                <button class="secondary-btn probe-backhaul-btn" type="button" data-link-id="${escapeHTML(link.id || '')}" title="${escapeHTML(blockReason || 'Test both directions')}"${blockReason ? ' disabled' : ''}>Test</button>
                <button class="danger-btn delete-backhaul-btn" type="button" data-link-id="${escapeHTML(link.id || '')}" data-link-name="${escapeHTML(link.name || 'backhaul')}">Delete</button>
              </div>
            </section>
          </div>
        </article>`;
    }

    function renderBackhaulList(links) {
      if (!links.length) return '<div class="empty backhaul-empty">Нет данных для отображения</div>';
      return links.map(renderBackhaulLinkCard).join('');
    }

    function renderTransportProfilePanel(transport, link) {
      const driver = driverDef(transport.driver) || {};
      const selected = transport.id === link.selected_transport_id;
      return `
        <article class="backhaul-profile-row">
          <div class="backhaul-profile-head">
            <div>
              <strong>${escapeHTML(driverLabel(transport.driver))}</strong>
              <span>${escapeHTML(transport.interface_name || 'n/a')}</span>
            </div>
            <div class="backhaul-row-tags">
              ${statusTag(transport.status || 'planned')}
              ${selected ? '<span class="tag ok">selected</span>' : ''}
              ${driverModeTag(driver)}
            </div>
          </div>
          <div class="backhaul-row-grid compact">
            <section class="backhaul-panel">
              <div class="backhaul-panel-label">Transport</div>
              <div class="backhaul-facts">
                ${renderBackhaulFact('Endpoint', endpointText(transport), 'wide')}
                ${renderBackhaulFact('Protocol', transport.protocol)}
              </div>
            </section>
            <section class="backhaul-panel">
              <div class="backhaul-panel-label">Tunnel</div>
              <div class="backhaul-facts">
                ${renderBackhaulFact('CIDR', transport.tunnel_cidr)}
                ${renderBackhaulFact('Ingress IP', transport.ingress_address)}
                ${renderBackhaulFact('Egress IP', transport.egress_address)}
              </div>
            </section>
            <section class="backhaul-panel backhaul-health-panel">
              <div class="backhaul-panel-label">Health</div>
              ${renderBackhaulHealthBlock(transport)}
            </section>
            <section class="backhaul-panel">
              <div class="backhaul-panel-label">Applied</div>
              ${renderAppliedCell(transport)}
            </section>
          </div>
        </article>`;
    }

    function cleanupRoleSummary(cleanup, role) {
      if (!cleanup || typeof cleanup !== 'object') return `${role}: n/a`;
      const status = firstText(cleanup.status, 'unknown');
      const removed = Array.isArray(cleanup.removed_paths) ? cleanup.removed_paths.length : 0;
      const skipped = Array.isArray(cleanup.skipped_items) ? cleanup.skipped_items.length : 0;
      const parts = [`${role}: ${status}`];
      if (removed) parts.push(`${removed} removed`);
      if (skipped) parts.push(`${skipped} skipped`);
      return parts.join(' · ');
    }

    function deletedCleanupSummary(link) {
      const transports = Array.isArray(link?.transports) ? link.transports : [];
      if (!transports.length) return 'No transport cleanup records.';
      return transports.map((transport) => {
        const cleanup = transport.health?.cleanup || {};
        return [
          transport.driver || 'transport',
          cleanupRoleSummary(cleanup.ingress, 'ingress'),
          cleanupRoleSummary(cleanup.egress, 'egress'),
        ].join(' · ');
      }).join(' | ');
    }

    function probeBlockReason(link, transport) {
      if (!transport) return 'Selected transport is not available.';
      if (String(link?.status || '').toLowerCase() !== 'active') return 'Apply profiles successfully on both nodes before testing.';
      if (String(transport.status || '').toLowerCase() !== 'active') return 'Selected transport is not active yet.';
      if (!roleApplied(transport, 'ingress') || !roleApplied(transport, 'egress')) return 'Both ingress and egress sides must be applied before testing.';
      return '';
    }

    function renderJobList(jobs) {
      const list = Array.isArray(jobs) ? jobs : [];
      if (!list.length) return '<div class="empty">No node jobs were queued.</div>';
      return `
        <div class="table-wrap">
          <table>
            <thead><tr><th>Job</th><th>Type</th><th>Status</th><th>Node</th><th>Result</th></tr></thead>
            <tbody>
              ${list.map((job) => `
                <tr data-job-id="${escapeHTML(job.id || '')}">
                  <td>${escapeHTML(job.id || 'n/a')}</td>
                  <td>${escapeHTML(job.type || 'n/a')}</td>
                  <td class="job-status-cell">${statusTag(job.status || 'queued')}</td>
                  <td>${escapeHTML(job.node_id || 'n/a')}</td>
                  <td>${escapeHTML(jobResultSummary(job))}</td>
                </tr>`).join('')}
            </tbody>
          </table>
        </div>`;
    }

    function jobResultSummary(job) {
      const result = job?.result || {};
      const health = result.health || {};
      return firstText(
        jobDetailedFailureText(job, result, health),
        job?.error,
        result.health_reason,
        result.health_route_warning,
        health.reason,
        health.route_warning,
        result.health_error,
        health.error,
        healthSummary(health),
        result.error,
        result.active_state,
        result.message
      );
    }

    async function pollJobs(jobIDs, targetID, onDone) {
      const target = document.getElementById(targetID);
      const ids = (jobIDs || []).filter(Boolean);
      if (!target || !ids.length) return;
      const terminal = new Set(['succeeded', 'failed', 'cancelled']);
      for (let attempt = 0; attempt < 20; attempt += 1) {
        await new Promise((resolve) => setTimeout(resolve, attempt < 3 ? 1000 : 2500));
        const jobs = [];
        for (const id of ids) {
          try {
            jobs.push(await getJSON(`/api/v1/jobs/${encodeURIComponent(id)}`));
          } catch (err) {
            jobs.push({ id, status: 'failed', type: 'unknown', error: err.message });
          }
        }
        target.innerHTML = renderJobList(jobs);
        if (jobs.every((job) => terminal.has(String(job.status || '').toLowerCase()))) {
          if (typeof onDone === 'function') await onDone(jobs);
          return;
        }
      }
    }

    function render() {
      setTitle('Backhaul');
      const allLinks = Array.isArray(state.backhaulLinks) ? state.backhaulLinks : [];
      const links = allLinks.filter((link) => link.status !== 'deleted');
      const deletedLinks = allLinks.filter((link) => link.status === 'deleted');
      const deletedRows = deletedLinks.map((link) => {
        const ingress = nodeByID(link.ingress_node_id);
        const egress = nodeByID(link.egress_node_id);
        return {
          name: link.name || 'backhaul',
          ingress: ingress?.name || link.ingress_node_id,
          egress: egress?.name || link.egress_node_id,
          deletedAt: renderDate(link.updated_at),
          profiles: renderTransportTags(link),
          cleanup: deletedCleanupSummary(link),
        };
      });
      el('content').innerHTML = `
        <section class="table-card backhaul-overview">
          <div class="table-head backhaul-overview-head">
            <h2>Ingress to Egress Backhaul</h2>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(links.length))} active</span>
              <button class="secondary-btn" id="createBackhaulBtn" type="button">Create backhaul</button>
            </div>
          </div>
          <div class="backhaul-list">${renderBackhaulList(links)}</div>
        </section>
        ${deletedRows.length ? tableCard('Recently Deleted Backhaul', deletedRows, [
          { title: 'Name', key: 'name' },
          { title: 'Ingress', key: 'ingress' },
          { title: 'Egress', key: 'egress' },
          { title: 'Deleted', key: 'deletedAt' },
          { title: 'Profiles', key: 'profiles', render: (row) => row.profiles },
          { title: 'Cleanup', key: 'cleanup' },
        ]) : ''}`;
      bindPageActions();
    }

    function bindPageActions() {
      document.getElementById('createBackhaulBtn')?.addEventListener('click', openCreateBackhaulModal);
      document.querySelectorAll('.inspect-backhaul-btn').forEach((button) => {
        button.addEventListener('click', () => openBackhaulDetails(button.dataset.linkId));
      });
      document.querySelectorAll('.apply-backhaul-btn').forEach((button) => {
        button.addEventListener('click', () => applyBackhaul(button.dataset.linkId));
      });
      document.querySelectorAll('.probe-backhaul-btn').forEach((button) => {
        if (button.disabled) return;
        button.addEventListener('click', () => probeBackhaul(button.dataset.linkId));
      });
      document.querySelectorAll('.delete-backhaul-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteBackhaulModal(button.dataset.linkId, button.dataset.linkName));
      });
    }

    function nodeOptions(nodes, selectedID = '') {
      return nodes.map((node) => `
        <option value="${escapeHTML(node.id)}"${node.id === selectedID ? ' selected' : ''}>
          ${escapeHTML(node.name || node.id)} · ${escapeHTML(node.role || 'node')}${node.address ? ` · ${escapeHTML(node.address)}` : ''}
        </option>`).join('');
    }

    function driverOptions(selectedCode = '') {
      return (state.backhaulDrivers || []).map((driver) => `
        <option value="${escapeHTML(driver.code)}"${driver.code === selectedCode ? ' selected' : ''}>
          ${escapeHTML(driver.label || driver.code)}
        </option>`).join('');
    }

    function driverModeTag(driver) {
      const autoStart = driver.activation_mode === 'managed_systemd';
      const routeCapable = Boolean(driver.supports_kernel_routes);
      return `
        <span class="tag ${autoStart ? 'ok' : 'stub'}">${autoStart ? 'auto-start service' : 'profile only'}</span>
        <span class="tag ${routeCapable ? 'ok' : 'stub'}">${routeCapable ? 'L3 route capable' : 'no kernel routes'}</span>`;
    }

    function defaultBackhaulDriverChecked(driver) {
      return ['wireguard', 'openvpn_udp'].includes(driver.code);
    }

    function driverCheckboxes() {
      return (state.backhaulDrivers || []).map((driver, index) => `
        <label class="check-row">
          <input type="checkbox" name="drivers" value="${escapeHTML(driver.code)}"${defaultBackhaulDriverChecked(driver) || index === 0 ? ' checked' : ''} />
          <span>
            <strong>${escapeHTML(driver.label || driver.code)}</strong>
            <small>${escapeHTML(driver.layer || 'transport')} · ${escapeHTML(driver.default_protocol || '')}/${escapeHTML(driver.default_port || '')}</small>
            <span class="inline-actions compact-inline profile-mode-tags">${driverModeTag(driver)}</span>
          </span>
        </label>`).join('');
    }

    function openCreateBackhaulModal() {
      const ingressNodes = roleNodes('ingress');
      const egressNodes = roleNodes('egress');
      if (!ingressNodes.length || !egressNodes.length) {
        openModal('Create backhaul', 'Backhaul unavailable', `
          <div class="empty">Create at least one online ingress node and one egress node before adding a backhaul link.</div>`);
        return;
      }
      const defaultIngress = ingressNodes[0];
      const defaultEgress = egressNodes[0];
      openModal('Create backhaul', 'Ingress to egress transport', `
        <form id="createBackhaulForm" class="form-grid">
          <div class="field">
            <label>Name</label>
            <input name="name" value="${escapeHTML(`${defaultIngress.name || 'ingress'}-to-${defaultEgress.name || 'egress'}`)}" required />
          </div>
          <div class="field">
            <label>Preferred driver</label>
            <select name="desired_driver" required>${driverOptions('wireguard')}</select>
          </div>
          <div class="field">
            <label>Ingress node</label>
            <select name="ingress_node_id" required>${nodeOptions(ingressNodes, defaultIngress.id)}</select>
          </div>
          <div class="field">
            <label>Egress node</label>
            <select name="egress_node_id" required>${nodeOptions(egressNodes, defaultEgress.id)}</select>
          </div>
          <div class="field">
            <label>Egress endpoint host</label>
            <input name="endpoint_host" value="${escapeHTML(defaultEgress.address || '')}" placeholder="public IP or DNS name" required />
          </div>
          <div class="field">
            <label>Tunnel CIDR</label>
            <input name="tunnel_cidr" placeholder="auto, or 10.240.10.0/30" />
          </div>
          <div class="field">
            <label>Routing table</label>
            <input name="routing_table" value="auto" />
          </div>
          <div class="field">
            <label>Route metric</label>
            <input name="route_metric" type="number" min="1" max="4096" value="50" />
          </div>
          <div class="field full">
            <label>Ingress-to-egress transport profiles</label>
            <div class="field-hint">Checked profiles are internal backhaul transports, not client configs. Auto-start profiles install and start systemd services on both nodes; profile-only drivers write configs and stay inactive until their safety gate is implemented.</div>
            <div class="choice-list">${driverCheckboxes()}</div>
          </div>
          <div class="field full modal-actions">
            <button class="secondary-btn" type="button" id="cancelBackhaulCreateBtn">Cancel</button>
            <button class="primary-btn" type="submit">Create</button>
          </div>
          <div class="field full form-result" id="createBackhaulResult"></div>
        </form>`, { wide: true });
      document.getElementById('cancelBackhaulCreateBtn')?.addEventListener('click', closeModal);
      document.getElementById('createBackhaulForm')?.addEventListener('submit', submitCreateBackhaul);
    }

    async function submitCreateBackhaul(event) {
      event.preventDefault();
      const formEl = event.currentTarget;
      const target = document.getElementById('createBackhaulResult');
      const form = new FormData(formEl);
      const drivers = form.getAll('drivers').map((item) => String(item || '').trim()).filter(Boolean);
      const desiredDriver = String(form.get('desired_driver') || '').trim();
      if (desiredDriver && !drivers.includes(desiredDriver)) {
        drivers.unshift(desiredDriver);
      }
      target.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const payload = {
          name: String(form.get('name') || '').trim(),
          ingress_node_id: String(form.get('ingress_node_id') || '').trim(),
          egress_node_id: String(form.get('egress_node_id') || '').trim(),
          desired_driver: desiredDriver,
          endpoint_host: String(form.get('endpoint_host') || '').trim(),
          tunnel_cidr: String(form.get('tunnel_cidr') || '').trim(),
          routing_table: String(form.get('routing_table') || '').trim(),
          route_metric: Number(form.get('route_metric') || 50),
          drivers,
        };
        const data = await sendJSON('/api/v1/backhaul-links', 'POST', payload);
        await refresh();
        openModal('Backhaul created', 'Profiles are ready to apply', `
          ${renderActionResponse(data, 'Backhaul profile created')}
          <div class="empty">Selected backhaul profiles were created in the control plane. Use Apply profiles to write configs on both nodes. Auto-start profiles will also start their services and run agent health checks.</div>
          <div class="modal-actions">
            <button class="primary-btn" type="button" id="closeBackhaulCreateResultBtn">Close</button>
          </div>`, { wide: true });
        document.getElementById('closeBackhaulCreateResultBtn')?.addEventListener('click', closeModal);
      } catch (err) {
        target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Backhaul create failed');
      }
    }

    function openBackhaulDetails(linkID) {
      const link = (state.backhaulLinks || []).find((item) => item.id === linkID);
      if (!link) return;
      const ingress = nodeByID(link.ingress_node_id);
      const egress = nodeByID(link.egress_node_id);
      const transports = Array.isArray(link.transports) ? link.transports : [];
      const selected = selectedTransport(link);
      const blockReason = probeBlockReason(link, selected);
      openModal(link.name || 'Backhaul', 'Ingress-to-egress transport profiles', `
        <div class="grid cols-4">
          <div class="card"><div class="mini-label">Ingress</div><div class="metric-caption">${escapeHTML(ingress?.name || link.ingress_node_id)}</div></div>
          <div class="card"><div class="mini-label">Egress</div><div class="metric-caption">${escapeHTML(egress?.name || link.egress_node_id)}</div></div>
          <div class="card"><div class="mini-label">Preferred driver</div><div class="metric-caption">${escapeHTML(driverLabel(link.desired_driver))}</div></div>
          <div class="card"><div class="mini-label">Status</div><div class="metric-caption">${statusTag(link.status || 'unknown')}</div></div>
        </div>
        <div class="backhaul-profile-list">
          ${transports.length ? transports.map((transport) => renderTransportProfilePanel(transport, link)).join('') : '<div class="empty">No transport profiles.</div>'}
        </div>
        <div class="modal-actions">
          <button class="secondary-btn" id="closeBackhaulDetailsBtn" type="button">Close</button>
          <button class="secondary-btn" id="probeBackhaulDetailsBtn" type="button" title="${escapeHTML(blockReason || 'Test both directions')}"${blockReason ? ' disabled' : ''}>Test both directions</button>
          <button class="primary-btn" id="applyBackhaulDetailsBtn" type="button">Apply profiles</button>
        </div>
        <div id="backhaulDetailsResult" class="form-result"></div>`, { wide: true });
      document.getElementById('closeBackhaulDetailsBtn')?.addEventListener('click', closeModal);
      document.getElementById('applyBackhaulDetailsBtn')?.addEventListener('click', () => applyBackhaul(link.id, 'backhaulDetailsResult'));
      if (!blockReason) {
        document.getElementById('probeBackhaulDetailsBtn')?.addEventListener('click', () => probeBackhaul(link.id, 'backhaulDetailsResult'));
      }
    }

    async function applyBackhaul(linkID, targetID = '') {
      const target = targetID ? document.getElementById(targetID) : null;
      if (target) target.innerHTML = '<span class="tag warn">queueing</span>';
      try {
        const data = await sendJSON(`/api/v1/backhaul-links/${linkID}/apply`, 'POST', {});
        const jobIDs = (data.jobs || []).map((job) => job.id).filter(Boolean);
        const body = `
          ${renderActionResponse(data, 'Backhaul apply queued')}
          <div id="backhaulApplyJobs">${renderJobList(data.jobs || [])}</div>
          <div class="modal-actions">
            <button class="primary-btn" type="button" id="closeBackhaulApplyBtn">Close</button>
          </div>`;
        if (target) target.innerHTML = body;
        else openModal('Backhaul apply queued', 'Jobs', body, { wide: true });
        document.getElementById('closeBackhaulApplyBtn')?.addEventListener('click', closeModal);
        await refresh();
        await pollJobs(jobIDs, 'backhaulApplyJobs', async () => {
          await refresh();
        });
      } catch (err) {
        const body = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Backhaul apply failed');
        if (target) target.innerHTML = body;
        else openModal('Backhaul apply failed', 'Error', body, { wide: true });
      }
    }

    async function probeBackhaul(linkID, targetID = '') {
      const target = targetID ? document.getElementById(targetID) : null;
      if (target) target.innerHTML = '<span class="tag warn">testing</span>';
      try {
        const data = await sendJSON(`/api/v1/backhaul-links/${linkID}/probe`, 'POST', {});
        const jobIDs = (data.jobs || []).map((job) => job.id).filter(Boolean);
        const body = `
          ${renderActionResponse(data, 'Backhaul probe queued')}
          <div id="backhaulProbeJobs">${renderJobList(data.jobs || [])}</div>
          <div class="modal-actions">
            <button class="primary-btn" type="button" id="closeBackhaulProbeBtn">Close</button>
          </div>`;
        if (target) {
          target.innerHTML = body;
        } else {
          openModal('Backhaul test', 'Bidirectional probe', body, { wide: true });
        }
        document.getElementById('closeBackhaulProbeBtn')?.addEventListener('click', closeModal);
        await pollJobs(jobIDs, 'backhaulProbeJobs', async () => {
          await refresh();
        });
      } catch (err) {
        const body = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Backhaul probe failed');
        if (target) target.innerHTML = body;
        else openModal('Backhaul probe failed', 'Error', body, { wide: true });
      }
    }

    function openDeleteBackhaulModal(linkID, linkName) {
      openModal('Delete backhaul', 'Confirmation', `
        <div class="card">
          <div class="mini-label">Backhaul</div>
          <h2>${escapeHTML(linkName || linkID)}</h2>
        </div>
        <div class="modal-actions">
          <button class="secondary-btn" id="cancelBackhaulDeleteBtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmBackhaulDeleteBtn" type="button">Delete</button>
        </div>
        <div id="deleteBackhaulResult" class="form-result"></div>`);
      document.getElementById('cancelBackhaulDeleteBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmBackhaulDeleteBtn')?.addEventListener('click', async () => {
        const target = document.getElementById('deleteBackhaulResult');
        target.innerHTML = '<span class="tag warn">deleting</span>';
        try {
          const data = await sendJSON(`/api/v1/backhaul-links/${linkID}`, 'DELETE', null);
          const jobIDs = (data.jobs || []).map((job) => job.id).filter(Boolean);
          openModal('Backhaul cleanup', 'Removing managed files from nodes', `
            ${renderActionResponse(data, jobIDs.length ? 'Backhaul cleanup queued' : 'Backhaul deleted')}
            <div id="backhaulCleanupJobs">${renderJobList(data.jobs || [])}</div>
            <div class="empty">Cleanup jobs remove managed units and files from both nodes. Stale job leases are recovered automatically; offline agents keep jobs queued until they poll again.</div>
            <div class="modal-actions">
              <button class="primary-btn" id="closeBackhaulCleanupBtn" type="button">Close</button>
            </div>`, { wide: true });
          document.getElementById('closeBackhaulCleanupBtn')?.addEventListener('click', closeModal);
          await refresh();
          await pollJobs(jobIDs, 'backhaulCleanupJobs', async (jobs) => {
            await refresh();
            const failed = jobs.some((job) => ['failed', 'cancelled'].includes(String(job.status || '').toLowerCase()));
            const result = document.getElementById('backhaulCleanupJobs');
            if (result && !failed) {
              result.insertAdjacentHTML('afterend', '<div class="form-result"><span class="tag ok">removed from nodes</span></div>');
            }
          });
        } catch (err) {
          target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Backhaul delete failed');
        }
      });
    }

    return { render, openCreateBackhaulModal };
  }

  window.MegaVPNBackhaulPage = { create: createBackhaulPage };
})(window);
