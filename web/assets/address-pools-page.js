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

    function poolByID(poolID) {
      return (state.addressPoolSpaces || []).find((space) => space.id === poolID || space.key === poolID) || null;
    }

    function poolFact(label, value) {
      return `<div><span>${escapeHTML(label)}</span><strong>${escapeHTML(String(value ?? '').trim() || 'n/a')}</strong></div>`;
    }

    function renderPoolSpace(space) {
      const usedPct = utilization(space);
      const canManage = hasPermission('settings.manage');
      const deleteDisabled = Number(space.used || 0) > 0;
      const status = space.routing_enabled ? 'routing enabled' : 'routing disabled';
      return `
        <article class="pool-space-card">
          <div class="pool-space-head">
            <div>
              <h3>${escapeHTML(space.label || space.key)}</h3>
              <div class="metric-caption"><code>${escapeHTML(space.key || space.id)}</code> · ${escapeHTML(space.service_scope || 'remote_access')}</div>
            </div>
            ${statusTag(space.status || 'unknown')}
          </div>
          ${space.description ? `<p class="pool-description">${escapeHTML(space.description)}</p>` : ''}
          <div class="pool-range-line">
            <div>
              <span>Supernet</span>
              <strong>${escapeHTML(space.base_cidr || 'n/a')}</strong>
            </div>
            <div>
              <span>Allocator starts at</span>
              <strong>${escapeHTML(space.start_cidr || 'n/a')}</strong>
            </div>
            <div>
              <span>Subnet size</span>
              <strong>/${escapeHTML(String(space.allocation_prefix || 0))}</strong>
            </div>
          </div>
          <div class="pool-meter">
            <div class="pool-meter-bar" style="width:${usedPct}%"></div>
          </div>
          <div class="metric-caption">${escapeHTML(String(space.used || 0))} of ${escapeHTML(String(space.capacity || 0))} subnets allocated · ${escapeHTML(String(space.free || 0))} free</div>
          <div class="pool-facts">
            ${poolFact('Used', space.used || 0)}
            ${poolFact('Free', space.free || 0)}
            ${poolFact('Routing', status)}
            ${poolFact('Order', space.display_order || 1000)}
          </div>
          ${deleteDisabled ? '<div class="pool-lock-note">Delete is locked while this pool has active allocations.</div>' : ''}
          <div class="pool-actions">
            <button class="secondary-btn pool-edit-btn" type="button" data-pool-id="${escapeHTML(space.id || space.key)}"${canManage ? '' : ' disabled'}>Edit</button>
            <button class="secondary-btn pool-routing-btn" type="button" data-pool-id="${escapeHTML(space.id || space.key)}" data-routing="${space.routing_enabled ? 'false' : 'true'}"${hasPermission('settings.manage') ? '' : ' disabled'}>
              ${space.routing_enabled ? 'Routing off' : 'Routing on'}
            </button>
            <button class="danger-btn pool-delete-btn" type="button" data-pool-id="${escapeHTML(space.id || space.key)}" data-pool-label="${escapeHTML(space.label || space.key)}"${canManage && !deleteDisabled ? '' : ' disabled'} title="${deleteDisabled ? 'Delete is blocked while active allocations exist.' : ''}">${deleteDisabled ? 'Locked' : 'Delete'}</button>
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
      const capacity = spaces.reduce((sum, space) => sum + Number(space.capacity || 0), 0);
      const routed = allocations.filter((allocation) => allocation.route_export).length;
      el('content').innerHTML = `
        <div class="pool-summary-grid">
          <div class="pool-summary-card"><span>Spaces</span><strong>${escapeHTML(String(spaces.length))}</strong><small>configured supernets</small></div>
          <div class="pool-summary-card"><span>Capacity</span><strong>${escapeHTML(String(capacity))}</strong><small>allocatable subnets</small></div>
          <div class="pool-summary-card"><span>Used</span><strong>${escapeHTML(String(used))}</strong><small>active allocations</small></div>
          <div class="pool-summary-card"><span>Routing export</span><strong>${escapeHTML(String(routed))}</strong><small>routed allocations</small></div>
        </div>
        <section class="table-card address-pools-page">
          <div class="table-head">
            <div>
              <h2>Pool spaces</h2>
              <div class="metric-caption">Supernets used by the allocator for WireGuard, OpenVPN and L2TP service pools.</div>
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
      document.getElementById('addAddressPoolBtn')?.addEventListener('click', () => openAddressPoolModal(''));
      document.querySelectorAll('.pool-edit-btn').forEach((button) => {
        button.addEventListener('click', () => openAddressPoolModal(button.dataset.poolId));
      });
      document.querySelectorAll('.pool-routing-btn').forEach((button) => {
        button.addEventListener('click', () => togglePoolRouting(button.dataset.poolId, button.dataset.routing === 'true'));
      });
      document.querySelectorAll('.pool-delete-btn').forEach((button) => {
        if (button.disabled) return;
        button.addEventListener('click', () => openDeleteAddressPoolModal(button.dataset.poolId, button.dataset.poolLabel));
      });
    }

    function openAddressPoolModal(poolID) {
      const pool = poolByID(poolID) || {};
      const editing = Boolean(pool.id || pool.key);
      const locked = editing && Number(pool.used || 0) > 0;
      openModal(editing ? `Edit pool: ${pool.label || pool.key}` : 'Add address pool', 'Address Pools', `
        <form id="addressPoolForm" class="form-grid">
          <input type="hidden" name="pool_id" value="${escapeHTML(pool.id || pool.key || '')}">
          <div class="field"><label>Name</label><input name="label" required value="${escapeHTML(pool.label || '')}" placeholder="Remote Access EU" /></div>
          <div class="field"><label>Key</label><input name="key" value="${escapeHTML(pool.key || '')}" placeholder="remote_access_eu"${editing ? ' readonly' : ''} /></div>
          <div class="field"><label>Base CIDR</label><input name="base_cidr" required value="${escapeHTML(pool.base_cidr || '172.16.0.0/12')}"${locked ? ' readonly' : ''} /></div>
          <div class="field"><label>Start CIDR</label><input name="start_cidr" required value="${escapeHTML(pool.start_cidr || '172.16.112.0/24')}"${locked ? ' readonly' : ''} /></div>
          <div class="field"><label>Allocation prefix</label><input name="allocation_prefix" type="number" min="16" max="32" value="${escapeHTML(pool.allocation_prefix || 24)}" required${locked ? ' readonly' : ''} /></div>
          <div class="field"><label>Scope</label><input name="service_scope" value="${escapeHTML(pool.service_scope || 'remote_access')}"${locked ? ' readonly' : ''} /></div>
          <div class="field"><label>Status</label><select name="status">
            <option value="active"${pool.status !== 'disabled' ? ' selected' : ''}>active</option>
            <option value="disabled"${pool.status === 'disabled' ? ' selected' : ''}>disabled</option>
          </select></div>
          <div class="field"><label>Display order</label><input name="display_order" type="number" value="${escapeHTML(pool.display_order || 1000)}" /></div>
          <div class="field full"><label>Description</label><textarea name="description" rows="3" placeholder="Where this pool should be used.">${escapeHTML(pool.description || '')}</textarea></div>
          <label class="field checkbox-line full"><input name="routing_enabled" type="checkbox"${pool.routing_enabled ? ' checked' : ''} /> Enable route export between allocated pools</label>
          ${editing && Number(pool.used || 0) > 0 ? '<div class="field full"><div class="empty">This pool has active allocations. Supernet, start CIDR, prefix and scope are locked until allocations are released.</div></div>' : ''}
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">${editing ? 'Save pool' : 'Create pool'}</button>
            <button class="secondary-btn" id="cancelAddressPoolBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="addressPoolResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelAddressPoolBtn')?.addEventListener('click', closeModal);
      document.getElementById('addressPoolForm')?.addEventListener('submit', submitAddressPool);
    }

    function openDeleteAddressPoolModal(poolID, label) {
      const pool = poolByID(poolID) || {};
      const poolLabel = label || pool.label || pool.key || poolID;
      openModal(`Delete pool: ${poolLabel}`, 'Address Pools', `
        <div class="response-grid">
          <div><span>Pool</span><strong>${escapeHTML(poolLabel || 'n/a')}</strong></div>
          <div><span>Supernet</span><strong>${escapeHTML(pool.base_cidr || 'n/a')}</strong></div>
          <div><span>Allocated</span><strong>${escapeHTML(String(pool.used || 0))}</strong></div>
          <div><span>Routing</span><strong>${escapeHTML(pool.routing_enabled ? 'enabled' : 'disabled')}</strong></div>
        </div>
        <div class="notice">
          Delete only removes an unused pool definition. Existing allocations are never removed implicitly.
        </div>
        <div class="inline-actions">
          <button class="danger-btn" id="confirmAddressPoolDeleteBtn" type="button">Delete pool</button>
          <button class="secondary-btn" id="cancelAddressPoolDeleteBtn" type="button">Cancel</button>
        </div>
        <div id="addressPoolDeleteResult" class="form-result"></div>`, { wide: false });
      document.getElementById('cancelAddressPoolDeleteBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmAddressPoolDeleteBtn')?.addEventListener('click', (event) => deleteAddressPool(poolID, poolLabel, event.currentTarget));
    }

    async function submitAddressPool(event) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const poolID = String(form.get('pool_id') || '').trim();
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
        status: String(form.get('status') || 'active').trim() || 'active',
        display_order: Number(form.get('display_order') || 1000),
      };
      const result = document.getElementById('addressPoolResult');
      if (result) result.innerHTML = `<span class="tag warn">${poolID ? 'saving' : 'creating'}</span>`;
      try {
        if (poolID) {
          await sendJSON(`/api/v1/address-pools/spaces/${encodeURIComponent(poolID)}`, 'PUT', payload);
        } else {
          await sendJSON('/api/v1/address-pools/spaces', 'POST', payload);
        }
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

    async function deleteAddressPool(poolID, label, button) {
      if (!poolID) return;
      if (button) {
        button.disabled = true;
        button.textContent = 'Deleting...';
      }
      const result = document.getElementById('addressPoolDeleteResult');
      if (result) result.innerHTML = '<span class="tag warn">deleting</span>';
      try {
        await sendJSON(`/api/v1/address-pools/spaces/${encodeURIComponent(poolID)}`, 'DELETE', null);
        closeModal();
        await refresh();
      } catch (err) {
        if (button) {
          button.disabled = false;
          button.textContent = 'Delete pool';
        }
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        openActionOutcomeModal('Address pool delete', 'Delete blocked', 'failed', err.message || 'Address pool delete failed.', [
          { label: 'Pool', value: label || poolID },
        ]);
      }
    }

    return { render };
  }

  window.MegaVPNAddressPoolsPage = { create: createAddressPoolsPage };
})(window);
