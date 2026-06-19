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
      sendJSON,
      refresh,
      openModal,
      closeModal,
      renderActionResponse,
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

    function render() {
      setTitle('Backhaul');
      const links = Array.isArray(state.backhaulLinks) ? state.backhaulLinks.filter((link) => link.status !== 'deleted') : [];
      const rows = links.map((link) => {
        const ingress = nodeByID(link.ingress_node_id);
        const egress = nodeByID(link.egress_node_id);
        const transport = selectedTransport(link);
        return {
          id: link.id,
          name: link.name || 'backhaul',
          ingress: ingress?.name || link.ingress_node_id,
          egress: egress?.name || link.egress_node_id,
          driver: driverLabel(link.desired_driver),
          endpoint: transport ? `${transport.endpoint_host || 'n/a'}:${transport.endpoint_port || 'n/a'} ${transport.protocol || ''}` : 'n/a',
          status: link.status || 'unknown',
          transports: renderTransportTags(link),
        };
      });
      el('content').innerHTML = `
        ${tableCard('Ingress to Egress Backhaul', rows, [
          { title: 'Name', key: 'name' },
          { title: 'Ingress', key: 'ingress' },
          { title: 'Egress', key: 'egress' },
          { title: 'Driver', key: 'driver' },
          { title: 'Endpoint', key: 'endpoint' },
          { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
          { title: 'Profiles', key: 'transports', render: (row) => row.transports },
          { title: 'Actions', key: 'id', render: (row) => `
            <div class="inline-actions">
              <button class="secondary-btn inspect-backhaul-btn" type="button" data-link-id="${escapeHTML(row.id)}">Manage</button>
              <button class="primary-btn apply-backhaul-btn" type="button" data-link-id="${escapeHTML(row.id)}">Apply</button>
              <button class="danger-btn delete-backhaul-btn" type="button" data-link-id="${escapeHTML(row.id)}" data-link-name="${escapeHTML(row.name)}">Delete</button>
            </div>` },
        ], '<button class="secondary-btn" id="createBackhaulBtn" type="button">Create backhaul</button>')}`;
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
      document.querySelectorAll('.delete-backhaul-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteBackhaulModal(button.dataset.linkId, button.dataset.linkName));
      });
    }

    function nodeOptions(nodes, selectedID = '') {
      return nodes.map((node) => `
        <option value="${escapeHTML(node.id)}"${node.id === selectedID ? ' selected' : ''}>
          ${escapeHTML(node.name || node.id)}${node.address ? ` · ${escapeHTML(node.address)}` : ''}
        </option>`).join('');
    }

    function driverOptions(selectedCode = '') {
      return (state.backhaulDrivers || []).map((driver) => `
        <option value="${escapeHTML(driver.code)}"${driver.code === selectedCode ? ' selected' : ''}>
          ${escapeHTML(driver.label || driver.code)}
        </option>`).join('');
    }

    function driverCheckboxes() {
      return (state.backhaulDrivers || []).map((driver, index) => `
        <label class="check-row">
          <input type="checkbox" name="drivers" value="${escapeHTML(driver.code)}"${index < 4 ? ' checked' : ''} />
          <span>
            <strong>${escapeHTML(driver.label || driver.code)}</strong>
            <small>${escapeHTML(driver.layer || 'transport')} · ${escapeHTML(driver.default_protocol || '')}/${escapeHTML(driver.default_port || '')}</small>
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
            <label>Transport profiles</label>
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
      target.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const payload = {
          name: String(form.get('name') || '').trim(),
          ingress_node_id: String(form.get('ingress_node_id') || '').trim(),
          egress_node_id: String(form.get('egress_node_id') || '').trim(),
          desired_driver: String(form.get('desired_driver') || '').trim(),
          endpoint_host: String(form.get('endpoint_host') || '').trim(),
          tunnel_cidr: String(form.get('tunnel_cidr') || '').trim(),
          routing_table: String(form.get('routing_table') || '').trim(),
          route_metric: Number(form.get('route_metric') || 50),
          drivers,
        };
        const data = await sendJSON('/api/v1/backhaul-links', 'POST', payload);
        target.innerHTML = renderActionResponse(data, 'Backhaul created');
        await refresh();
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
      openModal(link.name || 'Backhaul', 'Transport profiles', `
        <div class="grid cols-4">
          <div class="card"><div class="mini-label">Ingress</div><div class="metric-caption">${escapeHTML(ingress?.name || link.ingress_node_id)}</div></div>
          <div class="card"><div class="mini-label">Egress</div><div class="metric-caption">${escapeHTML(egress?.name || link.egress_node_id)}</div></div>
          <div class="card"><div class="mini-label">Preferred driver</div><div class="metric-caption">${escapeHTML(driverLabel(link.desired_driver))}</div></div>
          <div class="card"><div class="mini-label">Status</div><div class="metric-caption">${statusTag(link.status || 'unknown')}</div></div>
        </div>
        <div class="table-wrap" style="margin-top:16px">
          <table>
            <thead><tr><th>Driver</th><th>Status</th><th>Endpoint</th><th>Interface</th><th>Tunnel</th><th>Applied</th></tr></thead>
            <tbody>
              ${transports.length ? transports.map((transport) => `
                <tr>
                  <td>${escapeHTML(driverLabel(transport.driver))}</td>
                  <td>${statusTag(transport.status || 'planned')}</td>
                  <td>${escapeHTML(transport.endpoint_host || 'n/a')}:${escapeHTML(transport.endpoint_port || 'n/a')} ${escapeHTML(transport.protocol || '')}</td>
                  <td>${escapeHTML(transport.interface_name || 'n/a')}</td>
                  <td>${escapeHTML(transport.tunnel_cidr || 'n/a')}</td>
                  <td>${escapeHTML(transport.applied_ingress_at ? 'ingress ' : '')}${escapeHTML(transport.applied_egress_at ? 'egress' : '') || 'n/a'}</td>
                </tr>`).join('') : '<tr><td colspan="6"><div class="empty">No transport profiles.</div></td></tr>'}
            </tbody>
          </table>
        </div>
        <div class="modal-actions">
          <button class="secondary-btn" id="closeBackhaulDetailsBtn" type="button">Close</button>
          <button class="primary-btn" id="applyBackhaulDetailsBtn" type="button">Apply selected driver</button>
        </div>
        <div id="backhaulDetailsResult" class="form-result"></div>`, { wide: true });
      document.getElementById('closeBackhaulDetailsBtn')?.addEventListener('click', closeModal);
      document.getElementById('applyBackhaulDetailsBtn')?.addEventListener('click', () => applyBackhaul(link.id, 'backhaulDetailsResult'));
    }

    async function applyBackhaul(linkID, targetID = '') {
      const target = targetID ? document.getElementById(targetID) : null;
      if (target) target.innerHTML = '<span class="tag warn">queueing</span>';
      try {
        const data = await sendJSON(`/api/v1/backhaul-links/${linkID}/apply`, 'POST', {});
        if (target) target.innerHTML = renderActionResponse(data, 'Backhaul apply queued');
        else openModal('Backhaul apply queued', 'Jobs', renderActionResponse(data, 'Backhaul apply queued'), { wide: true });
        await refresh();
      } catch (err) {
        const body = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Backhaul apply failed');
        if (target) target.innerHTML = body;
        else openModal('Backhaul apply failed', 'Error', body, { wide: true });
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
          target.innerHTML = renderActionResponse(data, 'Backhaul deleted');
          await refresh();
        } catch (err) {
          target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Backhaul delete failed');
        }
      });
    }

    return { render, openCreateBackhaulModal };
  }

  window.MegaVPNBackhaulPage = { create: createBackhaulPage };
})(window);
