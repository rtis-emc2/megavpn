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

    function activeNodes() {
      return (state.nodes || []).filter((node) => node.status !== 'retired');
    }

    function instanceCapableServices() {
      const ranked = new Map();
      for (const service of (state.servicesCatalog || [])) {
        if (service.supports_instances === false || service.enabled === false) continue;
        const code = String(service.code || '').trim();
        if (!code) continue;
        const current = ranked.get(code);
        if (!current || service.code === code) ranked.set(code, service);
      }
      return Array.from(ranked.values());
    }

    function serviceLabel(code) {
      const normalized = String(code || '').trim();
      const service = (state.servicesCatalog || []).find((item) => String(item.code || '').trim() === normalized);
      return service?.label || service?.display_name || service?.name || normalized || 'unknown';
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

    function renderInstanceFact(label, value, className = '') {
      return `
        <div class="instance-fact ${className}">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(String(value ?? '').trim() || 'n/a')}</strong>
        </div>`;
    }

    function renderInstanceStateLine(label, status, reason) {
      const text = compactReason(reason || 'No runtime details yet.');
      return `
        <div class="instance-state-line">
          <div class="instance-state-head">
            <span>${escapeHTML(label)}</span>
            ${statusTag(status || 'unknown')}
          </div>
          <small title="${escapeHTML(reason || '')}">${escapeHTML(text)}</small>
        </div>`;
    }

    function actionButtons(row) {
      return `
        <div class="instance-actions">
          <button class="secondary-btn instance-manage-btn" type="button" data-instance-id="${escapeHTML(row.id)}">Manage</button>
          <button class="primary-btn instance-action-btn" type="button" data-action="apply" data-instance-id="${escapeHTML(row.id)}">Apply</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="restart" data-instance-id="${escapeHTML(row.id)}">Restart</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="start" data-instance-id="${escapeHTML(row.id)}">Start</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="stop" data-instance-id="${escapeHTML(row.id)}">Stop</button>
          <button class="danger-btn instance-delete-btn" type="button" data-instance-id="${escapeHTML(row.id)}" data-instance-name="${escapeHTML(row.name || 'instance')}">Delete</button>
        </div>`;
    }

    function renderInstanceCard(instance) {
      const row = toInstanceRow(instance);
      return `
        <article class="instance-row-card" data-instance-id="${escapeHTML(row.id)}">
          <div class="instance-row-head">
            <div class="instance-title-block">
              <h3>${escapeHTML(row.name || 'instance')}</h3>
              <div class="instance-subtitle">${escapeHTML(row.node)} -> ${escapeHTML(row.endpoint)}</div>
            </div>
            <div class="instance-row-tags">
              ${statusTag(row.status)}
              <span class="tag">${escapeHTML(serviceLabel(row.service))}</span>
              <span class="tag">${escapeHTML(row.revision === 'applied' ? 'revision applied' : `revision ${row.revision}`)}</span>
            </div>
          </div>
          <div class="instance-row-grid">
            <section class="instance-panel">
              <div class="instance-panel-label">Service</div>
              <div class="instance-facts">
                ${renderInstanceFact('Code', row.service)}
                ${renderInstanceFact('Node', row.node)}
                ${renderInstanceFact('Endpoint', row.endpoint, 'wide')}
              </div>
            </section>
            <section class="instance-panel">
              <div class="instance-panel-label">Desired state</div>
              <div class="instance-facts">
                ${renderInstanceFact('Lifecycle', row.status)}
                ${renderInstanceFact('Revision', row.revision)}
                ${renderInstanceFact('Instance ID', row.id, 'wide')}
              </div>
            </section>
            <section class="instance-panel">
              <div class="instance-panel-label">Runtime</div>
              <div class="instance-state-stack">
                ${renderInstanceStateLine('runtime', row.runtime, 'Agent runtime observation for this unit.')}
                ${renderInstanceStateLine('health', row.health, row.healthReason)}
                ${renderInstanceStateLine('drift', row.drift, row.driftReason)}
              </div>
            </section>
            <section class="instance-panel">
              <div class="instance-panel-label">Actions</div>
              ${actionButtons(row)}
            </section>
          </div>
        </article>`;
    }

    function renderInstancesList(instances) {
      if (!instances.length) return renderEmptyInstancesState();
      return instances.map(renderInstanceCard).join('');
    }

    function renderEmptyInstancesState() {
      const nodes = activeNodes();
      const services = instanceCapableServices();
      const runtimeReports = Array.isArray(state.instanceRuntimeStates) ? state.instanceRuntimeStates.length : 0;
      return `
        <div class="instances-empty-state">
          <div class="instances-empty-grid">
            <div class="instance-empty-panel">
              <span>Nodes</span>
              <strong>${escapeHTML(String(nodes.length))}</strong>
              <span>${escapeHTML(nodes.length ? 'available for managed instances' : 'no active nodes loaded')}</span>
            </div>
            <div class="instance-empty-panel">
              <span>Service profiles</span>
              <strong>${escapeHTML(String(services.length))}</strong>
              <span>${escapeHTML(services.length ? services.slice(0, 4).map((item) => serviceLabel(item.code)).join(', ') : 'no instance-capable services loaded')}</span>
            </div>
            <div class="instance-empty-panel">
              <span>Runtime reports</span>
              <strong>${escapeHTML(String(runtimeReports))}</strong>
              <span>${escapeHTML(runtimeReports ? 'agent observations available' : 'waiting for first instance report')}</span>
            </div>
          </div>
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
      const instances = Array.isArray(state.instances) ? state.instances : [];
      const runtimeReports = Array.isArray(state.instanceRuntimeStates) ? state.instanceRuntimeStates.length : 0;
      el('content').innerHTML = `
        <section class="table-card instances-overview">
          <div class="table-head">
            <h2>Instances</h2>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(instances.length))} active</span>
              <span class="tag">${escapeHTML(String(runtimeReports))} runtime reports</span>
              <button class="secondary-btn" id="createServicePackBtn" type="button">Create from pack</button>
              <button class="secondary-btn" id="createInstanceBtn" type="button">Manual instance</button>
            </div>
          </div>
          <div class="instances-list">${renderInstancesList(instances)}</div>
        </section>`;
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
