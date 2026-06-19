(function (window) {
  'use strict';

  function createInstancesPage(ctx = {}) {
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
      openActionOutcomeModal,
      openCreateInstanceModal,
      openCreateServicePackModal,
      openInstanceManageModal,
      queueInstanceAction,
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
      typeof openActionOutcomeModal !== 'function' ||
      typeof openCreateInstanceModal !== 'function' ||
      typeof openCreateServicePackModal !== 'function' ||
      typeof openInstanceManageModal !== 'function' ||
      typeof queueInstanceAction !== 'function'
    ) {
      throw new Error('MegaVPNInstancesPage requires page dependencies');
    }

    function instanceEndpoint(instance) {
      const host = String(instance?.endpoint_host || '').trim();
      const port = Number(instance?.endpoint_port || 0);
      if (!host && !port) return 'n/a';
      if (!host) return String(port);
      if (!port) return host;
      return `${host}:${port}`;
    }

    function revisionState(instance) {
      if (instance.current_revision_id && instance.last_applied_revision_id && instance.current_revision_id === instance.last_applied_revision_id) {
        return 'applied';
      }
      if (instance.current_revision_id) return 'pending';
      return 'n/a';
    }

    function runtimeStateFor(instanceID) {
      return (state.instanceRuntimeStates || []).find((item) => item.instance_id === instanceID) || null;
    }

    function primaryReason(values, fallback) {
      const list = Array.isArray(values) ? values : [];
      const first = list.map((item) => String(item || '').trim()).find(Boolean);
      return first || fallback || '';
    }

    function compactReason(value) {
      const text = String(value || '').trim();
      if (text.length <= 120) return text;
      return `${text.slice(0, 117)}...`;
    }

    function nameCell(row) {
      return `
        <strong>${escapeHTML(row.name)}</strong>
        <div class="metric-caption">desired: ${escapeHTML(row.status)} · revision: ${escapeHTML(row.revision)}</div>`;
    }

    function stateCell(value, reason) {
      const text = compactReason(reason || 'No runtime details yet.');
      return `
        <div>${statusTag(value || 'unknown')}</div>
        <div class="metric-caption" title="${escapeHTML(reason || '')}">${escapeHTML(text)}</div>`;
    }

    function toInstanceRow(instance) {
      const node = state.nodes.find((item) => item.id === instance.node_id);
      const runtime = runtimeStateFor(instance.id);
      return {
        id: instance.id,
        name: instance.name,
        node: node?.name || instance.node_id || 'n/a',
        service: instance.service_code || 'unknown',
        endpoint: instanceEndpoint(instance),
        revision: revisionState(instance),
        status: instance.status || 'draft',
        runtime: runtime?.runtime_status || 'unknown',
        health: runtime?.health_status || 'unknown',
        drift: runtime?.drift_status || 'unknown',
        healthReason: primaryReason(runtime?.health_reasons, 'Waiting for runtime health report.'),
        driftReason: primaryReason(runtime?.drift_reasons, 'Waiting for runtime drift report.'),
      };
    }

    function actionButtons(row) {
      return `
        <div class="inline-actions instance-row-actions">
          <button class="secondary-btn instance-manage-btn" type="button" data-instance-id="${escapeHTML(row.id)}">Manage</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="apply" data-instance-id="${escapeHTML(row.id)}">Apply</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="restart" data-instance-id="${escapeHTML(row.id)}">Restart</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="start" data-instance-id="${escapeHTML(row.id)}">Start</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="stop" data-instance-id="${escapeHTML(row.id)}">Stop</button>
          <button class="danger-btn instance-delete-btn" type="button" data-instance-id="${escapeHTML(row.id)}" data-instance-name="${escapeHTML(row.name || 'instance')}">Delete</button>
        </div>`;
    }

    function bindActions() {
      document.getElementById('createInstanceBtn')?.addEventListener('click', openCreateInstanceModal);
      document.getElementById('createServicePackBtn')?.addEventListener('click', openCreateServicePackModal);
      document.querySelectorAll('.instance-manage-btn').forEach((button) => {
        button.addEventListener('click', () => openInstanceManageModal(button.dataset.instanceId));
      });
      document.querySelectorAll('.instance-action-btn').forEach((button) => {
        button.addEventListener('click', () => queueInstanceAction(button.dataset.instanceId, button.dataset.action));
      });
      document.querySelectorAll('.instance-delete-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteInstanceModal(button.dataset.instanceId, button.dataset.instanceName));
      });
    }

    function render() {
      setTitle('Instances');
      const rows = (state.instances || []).map(toInstanceRow);
      el('content').innerHTML = `
        ${tableCard('Instances', rows, [
          { title: 'Name', key: 'name', render: nameCell },
          { title: 'Node', key: 'node' },
          { title: 'Service', key: 'service', render: (row) => `<span class="tag">${escapeHTML(row.service)}</span>` },
          { title: 'Endpoint', key: 'endpoint' },
          { title: 'Runtime', key: 'runtime', render: (row) => statusTag(row.runtime) },
          { title: 'Health', key: 'health', render: (row) => stateCell(row.health, row.healthReason) },
          { title: 'Drift', key: 'drift', render: (row) => stateCell(row.drift, row.driftReason) },
          { title: 'Actions', key: 'id', render: actionButtons },
        ], '<div class="inline-actions"><button class="secondary-btn" id="createServicePackBtn" type="button">Create service pack</button><button class="secondary-btn" id="createInstanceBtn" type="button">Create instance</button></div>')}`;
      bindActions();
    }

    function openDeleteInstanceModal(instanceID, instanceName) {
      const instance = (state.instances || []).find((item) => item.id === instanceID);
      openModal(`Delete instance: ${instanceName || 'instance'}`, 'Instance lifecycle', `
        <div class="card">
          <div class="mini-label">Soft delete</div>
          <h3>${escapeHTML(instance?.name || instanceName || instanceID)}</h3>
          <p>This marks the instance as deleted and hides it from operational lists. The backend blocks deletion while active service accesses still reference this instance.</p>
          <div class="response-grid">
            <div class="response-fact"><span>Service</span><strong>${escapeHTML(instance?.service_code || 'unknown')}</strong></div>
            <div class="response-fact"><span>Node</span><strong>${escapeHTML(instance?.node_id || 'n/a')}</strong></div>
            <div class="response-fact"><span>Status</span><strong>${escapeHTML(instance?.status || 'unknown')}</strong></div>
            <div class="response-fact"><span>Endpoint</span><strong>${escapeHTML(instanceEndpoint(instance))}</strong></div>
          </div>
        </div>
        <div class="inline-actions" style="margin-top:14px">
          <button class="secondary-btn" id="cancelDeleteInstanceBtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmDeleteInstanceBtn" type="button">Delete instance</button>
        </div>`, { wide: true });
      document.getElementById('cancelDeleteInstanceBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmDeleteInstanceBtn')?.addEventListener('click', (event) => submitDeleteInstance(instanceID, instanceName, event.currentTarget));
    }

    async function submitDeleteInstance(instanceID, instanceName, button) {
      if (button) {
        button.disabled = true;
        button.textContent = 'Deleting...';
      }
      try {
        const deleted = await sendJSON(`/api/v1/instances/${instanceID}`, 'DELETE', null);
        closeModal();
        await refresh();
        openActionOutcomeModal(
          `Instance deleted: ${instanceName || deleted?.name || instanceID}`,
          'Instance lifecycle',
          'succeeded',
          'Instance was soft-deleted and removed from the active operational list.',
          [
            { label: 'Instance', value: deleted?.name || instanceName || instanceID },
            { label: 'Status', value: deleted?.status || 'deleted' },
          ],
        );
      } catch (err) {
        openActionOutcomeModal(
          'Instance delete blocked',
          'Instance lifecycle',
          'failed',
          err.message || 'Instance delete failed.',
          [
            { label: 'Instance', value: instanceName || instanceID },
            { label: 'Expected fix', value: 'Revoke or delete dependent service accesses first, then retry.' },
          ],
        );
      } finally {
        if (button) {
          button.disabled = false;
          button.textContent = 'Delete instance';
        }
      }
    }

    return {
      render,
      toRow: toInstanceRow,
    };
  }

  window.MegaVPNInstancesPage = { create: createInstancesPage };
})(window);
