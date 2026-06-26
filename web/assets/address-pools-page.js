(function (window) {
  'use strict';

  function createAddressPoolsPage(ctx = {}) {
    const {
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
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function'
    ) {
      throw new Error('MegaVPNAddressPoolsPage requires page dependencies');
    }

    function utilization(space) {
      const capacity = Number(space.capacity || 0);
      if (!capacity) return 0;
      return Math.max(0, Math.min(100, Math.round((Number(space.used || 0) / capacity) * 100)));
    }

    function renderPoolSpace(space) {
      const usedPct = utilization(space);
      return `
        <article class="pool-space-card">
          <div class="pool-space-head">
            <div>
              <h3>${escapeHTML(space.label || space.key)}</h3>
              <div class="metric-caption"><code>${escapeHTML(space.key || space.id)}</code> · ${escapeHTML(space.base_cidr || 'n/a')} · start ${escapeHTML(space.start_cidr || 'n/a')}</div>
            </div>
            ${statusTag(space.status || 'unknown')}
          </div>
          <div class="pool-meter">
            <div class="pool-meter-bar" style="width:${usedPct}%"></div>
          </div>
          <div class="pool-facts">
            <div><span>Prefix</span><strong>/${escapeHTML(String(space.allocation_prefix || 0))}</strong></div>
            <div><span>Used</span><strong>${escapeHTML(String(space.used || 0))}</strong></div>
            <div><span>Free</span><strong>${escapeHTML(String(space.free || 0))}</strong></div>
            <div><span>Routing</span><strong>${escapeHTML(space.routing_enabled ? 'enabled' : 'disabled')}</strong></div>
          </div>
          <div class="inline-actions">
            <button class="secondary-btn pool-routing-btn" type="button" data-pool-id="${escapeHTML(space.id || space.key)}" data-routing="${space.routing_enabled ? 'false' : 'true'}"${hasPermission('settings.manage') ? '' : ' disabled'}>
              ${space.routing_enabled ? 'Disable routing' : 'Enable routing'}
            </button>
          </div>
        </article>`;
    }

    function allocationRows() {
      const allocations = Array.isArray(state.addressPoolAllocations) ? state.addressPoolAllocations : [];
      if (!allocations.length) {
        return '<tr><td colspan="8"><div class="empty">No allocated subnets yet.</div></td></tr>';
      }
      return allocations.map((allocation) => `
        <tr>
          <td><code>${escapeHTML(allocation.cidr || 'n/a')}</code></td>
          <td>${escapeHTML(allocation.pool_space_label || allocation.pool_space_key || 'pool')}</td>
          <td>${escapeHTML(allocation.node_name || allocation.node_id || 'n/a')}</td>
          <td>${escapeHTML(allocation.instance_name || allocation.instance_id || 'n/a')}</td>
          <td><span class="tag">${escapeHTML(allocation.service_code || 'service')}</span></td>
          <td><span class="tag">${escapeHTML(allocation.purpose || 'remote_access')}</span></td>
          <td>${statusTag(allocation.route_export ? 'enabled' : 'disabled')}</td>
          <td>${escapeHTML(formatDate(allocation.created_at))}</td>
        </tr>`).join('');
    }

    function render() {
      setTitle('Address pools');
      const spaces = Array.isArray(state.addressPoolSpaces) ? state.addressPoolSpaces : [];
      const allocations = Array.isArray(state.addressPoolAllocations) ? state.addressPoolAllocations : [];
      const used = spaces.reduce((sum, space) => sum + Number(space.used || 0), 0);
      const free = spaces.reduce((sum, space) => sum + Number(space.free || 0), 0);
      el('content').innerHTML = `
        <section class="table-card address-pools-page">
          <div class="table-head">
            <div>
              <h2>Address pool spaces</h2>
              <div class="metric-caption">Allocator source for WireGuard, OpenVPN and L2TP service pools.</div>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(spaces.length))} spaces</span>
              <span class="tag">${escapeHTML(String(used))} used</span>
              <span class="tag">${escapeHTML(String(free))} free</span>
              ${hasPermission('settings.manage') ? '<button class="secondary-btn" id="addAddressPoolBtn" type="button">Add pool</button>' : ''}
            </div>
          </div>
          <div class="pool-space-grid">
            ${spaces.length ? spaces.map(renderPoolSpace).join('') : '<div class="empty">No address pool spaces configured.</div>'}
          </div>
        </section>
        <section class="table-card">
          <div class="table-head">
            <h2>Allocated subnets</h2>
            <div class="table-tools"><span class="tag">${escapeHTML(String(allocations.length))} active</span></div>
          </div>
          <div class="table-wrap">
            <table class="address-pool-table">
              <thead><tr><th>CIDR</th><th>Pool</th><th>Node</th><th>Instance</th><th>Service</th><th>Purpose</th><th>Routing</th><th>Created</th></tr></thead>
              <tbody>${allocationRows()}</tbody>
            </table>
          </div>
        </section>`;
      bindActions();
    }

    function bindActions() {
      document.getElementById('addAddressPoolBtn')?.addEventListener('click', openAddPoolModal);
      document.querySelectorAll('.pool-routing-btn').forEach((button) => {
        button.addEventListener('click', () => togglePoolRouting(button.dataset.poolId, button.dataset.routing === 'true'));
      });
    }

    function openAddPoolModal() {
      openModal('Add address pool', 'Address Pools', `
        <form id="addressPoolForm" class="form-grid">
          <div class="field"><label>Name</label><input name="label" required placeholder="Remote Access EU" /></div>
          <div class="field"><label>Key</label><input name="key" placeholder="remote_access_eu" /></div>
          <div class="field"><label>Base CIDR</label><input name="base_cidr" required value="172.16.0.0/12" /></div>
          <div class="field"><label>Start CIDR</label><input name="start_cidr" required value="172.16.112.0/24" /></div>
          <div class="field"><label>Allocation prefix</label><input name="allocation_prefix" type="number" min="16" max="32" value="24" required /></div>
          <div class="field"><label>Scope</label><input name="service_scope" value="remote_access" /></div>
          <div class="field full"><label>Description</label><textarea name="description" rows="3" placeholder="Where this pool should be used."></textarea></div>
          <label class="field checkbox-line full"><input name="routing_enabled" type="checkbox" /> Enable route export between allocated pools</label>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Create pool</button>
            <button class="secondary-btn" id="cancelAddressPoolBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="addressPoolResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelAddressPoolBtn')?.addEventListener('click', closeModal);
      document.getElementById('addressPoolForm')?.addEventListener('submit', submitAddressPool);
    }

    async function submitAddressPool(event) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const payload = {
        key: String(form.get('key') || '').trim(),
        label: String(form.get('label') || '').trim(),
        description: String(form.get('description') || '').trim(),
        family: 'ipv4',
        base_cidr: String(form.get('base_cidr') || '').trim(),
        start_cidr: String(form.get('start_cidr') || '').trim(),
        allocation_prefix: Number(form.get('allocation_prefix') || 24),
        service_scope: String(form.get('service_scope') || 'remote_access').trim() || 'remote_access',
        routing_enabled: Boolean(form.get('routing_enabled')),
        status: 'active',
      };
      const result = document.getElementById('addressPoolResult');
      if (result) result.innerHTML = '<span class="tag warn">creating</span>';
      try {
        await sendJSON('/api/v1/address-pools/spaces', 'POST', payload);
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function togglePoolRouting(poolID, enabled) {
      try {
        await sendJSON(`/api/v1/address-pools/spaces/${encodeURIComponent(poolID)}/routing`, 'POST', { routing_enabled: enabled });
        await refresh();
      } catch (err) {
        openActionOutcomeModal('Address pool routing', 'Routing flag update failed', 'failed', err.message || 'Address pool routing update failed.', [
          { label: 'Pool', value: poolID || 'n/a' },
        ]);
      }
    }

    return { render };
  }

  window.MegaVPNAddressPoolsPage = { create: createAddressPoolsPage };
})(window);
