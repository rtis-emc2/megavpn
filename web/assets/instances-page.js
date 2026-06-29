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
      domainUI,
      renderActionResponse,
      sendJSON,
      refresh,
      hasPermission,
      openModal,
      closeModal,
      openActionOutcomeModal,
      openCreateInstanceModal,
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
      !domainUI ||
      typeof domainUI.nodeOptions !== 'function' ||
      typeof domainUI.certificateOptions !== 'function' ||
      typeof domainUI.servicePKIProfileOptions !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof openCreateInstanceModal !== 'function' ||
      typeof openInstanceManageModal !== 'function' ||
      typeof queueInstanceAction !== 'function'
    ) {
      throw new Error('MegaVPNInstancesPage requires page dependencies');
    }

    const { certificateOptions, nodeOptions, servicePKIProfileOptions } = domainUI;

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

    function canManageServicePacks() {
      return hasPermission('settings.manage');
    }

    function servicePackCatalogItems() {
      const source = canManageServicePacks()
        ? (state.servicePackCatalog || state.servicePacks || [])
        : (state.servicePacks || []);
      return (Array.isArray(source) ? source : [])
        .filter((pack) => pack && pack.key)
        .filter((pack) => String(pack.status || 'active').toLowerCase() === 'active')
        .slice()
        .sort((left, right) => Number(left.display_order || 1000) - Number(right.display_order || 1000)
          || String(left.label || left.key).localeCompare(String(right.label || right.key), 'en'));
    }

    function servicePackComponentsLabel(pack) {
      const components = Array.isArray(pack?.components) ? pack.components : [];
      if (!components.length) return 'no components';
      const counts = new Map();
      for (const component of components) {
        const label = String(component.service_code || component.label || 'component').trim() || 'component';
        counts.set(label, (counts.get(label) || 0) + 1);
      }
      return Array.from(counts.entries()).map(([label, count]) => count > 1 ? `${label} x${count}` : label).join(', ');
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

    function ensureCreatePackSelection(packs) {
      const items = Array.isArray(packs) ? packs : servicePackCatalogItems();
      if (!items.length) {
        state.instancesCreatePackKey = '';
        return null;
      }
      const selected = items.find((pack) => pack.key === state.instancesCreatePackKey);
      if (selected) return selected;
      state.instancesCreatePackKey = items[0].key;
      return items[0];
    }

    function servicePackComponents(pack) {
      return Array.isArray(pack?.components) ? pack.components : [];
    }

    function packUsesService(pack, serviceCode) {
      const expected = String(serviceCode || '').trim().toLowerCase();
      return servicePackComponents(pack).some((component) => String(component?.service_code || '').trim().toLowerCase() === expected);
    }

    function endpointRequirementLabel(pack) {
      if (!pack) return 'n/a';
      return pack.requires_endpoint_host ? 'required' : 'optional';
    }

    function renderCreatePackCard(pack, selectedKey) {
      const key = String(pack.key || '').trim();
      const selected = key === selectedKey;
      return `
        <button class="pack-choice-btn ${selected ? 'is-selected' : ''}" type="button" data-pack-key="${escapeHTML(key)}">
          <strong>${escapeHTML(pack.label || key)}</strong>
          <span>${escapeHTML(servicePackComponentsLabel(pack))}</span>
          <small>${escapeHTML(pack.endpoint_hint || 'no endpoint hint')}</small>
        </button>`;
    }

    function renderPackSummary(pack) {
      if (!pack) return '';
      const components = servicePackComponents(pack);
      return `
        <div class="pack-create-summary">
          ${renderInstanceFact('Base name', pack.base_name_template || 'edge-service-pack')}
          ${renderInstanceFact('Endpoint host', endpointRequirementLabel(pack))}
          ${renderInstanceFact('Components', components.length)}
        </div>`;
    }

    function renderPackComponent(component, index) {
      const port = Number(component?.endpoint_port || 0);
      const meta = [
        component?.service_code || 'service',
        component?.preset_key || 'default',
        port ? `port ${port}` : '',
      ].filter(Boolean).join(' · ');
      return `
        <article class="pack-component-card">
          <div class="pack-component-index">${escapeHTML(String(index + 1))}</div>
          <div>
            <strong>${escapeHTML(component?.label || component?.service_code || `Component ${index + 1}`)}</strong>
            <span>${escapeHTML(meta)}</span>
            <p>${escapeHTML(component?.description || 'Instance component generated from the selected pack template.')}</p>
          </div>
        </article>`;
    }

    function renderPackDetails(pack) {
      if (!pack) {
        return '<div class="empty pack-create-empty">No service pack templates loaded.</div>';
      }
      const components = servicePackComponents(pack);
      const recommendations = Array.isArray(pack.recommendations) ? pack.recommendations.filter(Boolean) : [];
      const notes = Array.isArray(pack.platform_notes) ? pack.platform_notes.filter(Boolean) : [];
      return `
        <section class="pack-details-panel">
          <div class="pack-details-head">
            <div>
              <div class="instance-panel-label">Selected pack</div>
              <h3>${escapeHTML(pack.label || pack.key)}</h3>
              <p>${escapeHTML(pack.description || 'Template for creating a managed service instance set.')}</p>
            </div>
            <span class="tag">${escapeHTML(String(components.length))} components</span>
          </div>
          ${renderPackSummary(pack)}
          <div class="pack-component-grid">
            ${components.length ? components.map(renderPackComponent).join('') : '<div class="empty">No components in this pack.</div>'}
          </div>
          ${recommendations.length || notes.length ? `
            <div class="pack-notes">
              ${recommendations.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}
              ${notes.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}
            </div>` : ''}
        </section>`;
    }

    function renderCreatePackResult() {
      const result = state.instancesCreateResult;
      if (!result) return '';
      if (result.status === 'running') {
        return '<div class="form-result"><span class="tag warn">creating</span></div>';
      }
      if (result.status === 'succeeded') {
        return `
          <div class="form-result pack-create-result">
            ${renderActionResponse(result.data, 'Service pack creation')}
            <div class="inline-actions">
              <button class="secondary-btn" id="openInstancesAfterPackCreateBtn" type="button">Open instances</button>
              <button class="secondary-btn" id="createAnotherFromPackBtn" type="button">Create another</button>
            </div>
          </div>`;
      }
      return `<div class="form-result"><span class="tag danger">${escapeHTML(result.message || 'create failed')}</span></div>`;
    }

    function openCreateFromPackPage() {
      state.instancesView = 'create-pack';
      state.instancesCreateResult = null;
      ensureCreatePackSelection(servicePackCatalogItems());
      render();
    }

    function openInstancesListPage() {
      state.instancesView = 'list';
      state.instancesCreateResult = null;
      render();
    }

    function renderCreateFromPackPage() {
      setTitle('Create from pack');
      const packs = servicePackCatalogItems();
      const selectedPack = ensureCreatePackSelection(packs);
      const nodeSelect = nodeOptions();
      const certificateSelect = certificateOptions('', true);
      const usesOpenVPN = packUsesService(selectedPack, 'openvpn');
      const baseName = selectedPack?.base_name_template || 'edge-service-pack';
      const endpointHint = selectedPack?.endpoint_hint || 'edge.example.com';
      const endpointRequired = Boolean(selectedPack?.requires_endpoint_host);
      el('content').innerHTML = `
        <section class="table-card instance-create-page">
          <div class="table-head instance-create-head">
            <div>
              <h2>Create from pack</h2>
              <div class="metric-caption">Instances are created from a reusable service pack template.</div>
            </div>
            <div class="table-tools">
              <button class="secondary-btn" id="backToInstancesBtn" type="button">Back to instances</button>
              <button class="secondary-btn" id="createInstanceBtn" type="button">Manual instance</button>
            </div>
          </div>
          <div class="pack-create-layout">
            <aside class="pack-picker-panel">
              <div class="instance-panel-label">Service pack</div>
              <div class="pack-choice-list">
                ${packs.length ? packs.map((pack) => renderCreatePackCard(pack, state.instancesCreatePackKey)).join('') : '<div class="empty">No templates loaded.</div>'}
              </div>
            </aside>
            <div class="pack-create-main">
              ${renderPackDetails(selectedPack)}
              <form id="createServicePackPageForm" class="pack-create-form">
                <input type="hidden" name="service_pack_key" value="${escapeHTML(selectedPack?.key || '')}">
                <div class="pack-create-form-grid">
                  <div class="field">
                    <label>Node</label>
                    <select name="node_id" required${nodeSelect ? '' : ' disabled'}>
                      ${nodeSelect || '<option value="">No active nodes available</option>'}
                    </select>
                  </div>
                  <div class="field">
                    <label>TLS edge certificate</label>
                    <select name="certificate_id">${certificateSelect}</select>
                    <div class="field-hint">Used only by Nginx or Xray TLS components. OpenVPN uses the Service CA profile below.</div>
                  </div>
                  ${usesOpenVPN ? `
                    <div class="field">
                      <label>OpenVPN CA profile</label>
                      <select name="openvpn_pki_profile">${servicePKIProfileOptions('openvpn', 'default')}</select>
                      <div class="field-hint">All OpenVPN instances created with this profile will trust the same service CA root.</div>
                    </div>` : ''}
                  <div class="field">
                    <label>Base name</label>
                    <input name="base_name" value="${escapeHTML(baseName)}" placeholder="${escapeHTML(baseName)}">
                  </div>
                  <div class="field">
                    <label>Endpoint host</label>
                    <input name="endpoint_host" placeholder="${escapeHTML(endpointHint)}"${endpointRequired ? ' required' : ''}>
                  </div>
                </div>
                <div class="pack-create-actions">
                  <button class="primary-btn" type="submit"${selectedPack && nodeSelect ? '' : ' disabled'}>Create instances</button>
                  <button class="secondary-btn" id="resetPackCreateFormBtn" type="button">Reset form</button>
                </div>
              </form>
              <div id="createServicePackPageResult">${renderCreatePackResult()}</div>
            </div>
          </div>
        </section>`;
      bindCreateFromPackPage();
    }

    function bindCreateFromPackPage() {
      document.getElementById('backToInstancesBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createInstanceBtn')?.addEventListener('click', openCreateInstanceModal);
      document.getElementById('resetPackCreateFormBtn')?.addEventListener('click', () => {
        state.instancesCreateResult = null;
        renderCreateFromPackPage();
      });
      document.querySelectorAll('.pack-choice-btn').forEach((button) => {
        button.addEventListener('click', () => {
          state.instancesCreatePackKey = button.dataset.packKey || '';
          state.instancesCreateResult = null;
          renderCreateFromPackPage();
        });
      });
      document.getElementById('createServicePackPageForm')?.addEventListener('submit', submitCreateFromPackPage);
      document.getElementById('openInstancesAfterPackCreateBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createAnotherFromPackBtn')?.addEventListener('click', () => {
        state.instancesCreateResult = null;
        renderCreateFromPackPage();
      });
    }

    async function submitCreateFromPackPage(event) {
      event.preventDefault();
      const form = event.currentTarget;
      const button = form.querySelector('button[type="submit"]');
      const data = new FormData(form);
      const packKey = String(data.get('service_pack_key') || state.instancesCreatePackKey || '').trim();
      const payload = {
        node_id: String(data.get('node_id') || '').trim(),
        base_name: String(data.get('base_name') || '').trim(),
        endpoint_host: String(data.get('endpoint_host') || '').trim(),
        certificate_id: String(data.get('certificate_id') || '').trim(),
        openvpn_pki_profile: String(data.get('openvpn_pki_profile') || '').trim(),
      };
      if (!packKey) {
        state.instancesCreateResult = { status: 'failed', message: 'service pack is required' };
        renderCreateFromPackPage();
        return;
      }
      if (button) {
        button.disabled = true;
        button.textContent = 'Creating...';
      }
      state.instancesCreateResult = { status: 'running' };
      const result = document.getElementById('createServicePackPageResult');
      if (result) result.innerHTML = renderCreatePackResult();
      try {
        const created = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}/instances`, 'POST', payload);
        state.instancesCreateResult = { status: 'succeeded', data: created };
        await refresh();
        renderCreateFromPackPage();
      } catch (err) {
        state.instancesCreateResult = { status: 'failed', message: err.message || 'service pack create failed' };
        renderCreateFromPackPage();
      }
    }

    function bindActions() {
      document.getElementById('createInstanceBtn')?.addEventListener('click', openCreateInstanceModal);
      document.getElementById('createServicePackBtn')?.addEventListener('click', openCreateFromPackPage);
      document.getElementById('addServicePackTemplateBtn')?.addEventListener('click', () => openServicePackEditor(''));
      document.querySelectorAll('.service-pack-edit-btn').forEach((button) => {
        button.addEventListener('click', () => openServicePackEditor(button.dataset.packKey));
      });
      document.querySelectorAll('.service-pack-delete-btn').forEach((button) => {
        button.addEventListener('click', () => deleteServicePackTemplate(button.dataset.packKey));
      });
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
      if (state.instancesView === 'create-pack') {
        renderCreateFromPackPage();
        return;
      }
      state.instancesView = 'list';
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
        </section>
        ${renderServicePackCatalog()}`;
      bindActions();
    }

    function renderServicePackCatalog() {
      const packs = servicePackCatalogItems();
      const manage = canManageServicePacks();
      const rows = packs.length
        ? packs.map((pack) => `
          <tr class="service-pack-row">
            <td>
              <strong>${escapeHTML(pack.label || pack.key)}</strong>
              <div class="metric-caption"><code>${escapeHTML(pack.key)}</code></div>
              <div class="metric-caption">${escapeHTML(pack.description || 'template')}</div>
            </td>
            <td><span class="template-components">${escapeHTML(servicePackComponentsLabel(pack))}</span></td>
            <td><code class="mono-clip">${escapeHTML(pack.endpoint_hint || 'n/a')}</code></td>
            <td>
              <div class="table-actions service-pack-actions">
                ${manage ? `<button class="secondary-btn service-pack-edit-btn" type="button" data-pack-key="${escapeHTML(pack.key)}">Edit</button>` : ''}
                ${manage ? `<button class="danger-btn service-pack-delete-btn" type="button" data-pack-key="${escapeHTML(pack.key)}">Delete</button>` : ''}
              </div>
            </td>
          </tr>`).join('')
        : `<tr><td colspan="4"><div class="empty">No service pack templates loaded.</div></td></tr>`;
      return `
        <section class="table-card">
          <div class="table-head">
            <h2>Service pack catalog</h2>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(packs.length))} templates</span>
              ${manage ? '<button class="secondary-btn" id="addServicePackTemplateBtn" type="button">Add pack</button>' : '<span class="tag">settings.manage required</span>'}
            </div>
          </div>
          <div class="table-wrap">
            <table class="service-pack-table">
              <thead><tr><th>Pack</th><th>Components</th><th>Endpoint hint</th><th>Actions</th></tr></thead>
              <tbody>${rows}</tbody>
            </table>
          </div>
        </section>`;
    }

    function servicePackEditorModel(packKey) {
      const existing = servicePackCatalogItems().find((pack) => pack.key === packKey);
      if (existing) {
        const model = JSON.parse(JSON.stringify(existing));
        delete model.status;
        delete model.source;
        delete model.version;
        return model;
      }
      return {
        key: 'custom_wireguard_pack',
        label: 'Custom WireGuard Pack',
        description: 'Operator-managed service pack template.',
        base_name_template: 'edge-wireguard',
        endpoint_hint: 'wg.example.com',
        requires_endpoint_host: true,
        platform_notes: [],
        recommendations: ['Address pool can be allocated automatically from the Address Pools catalog.'],
        components: [
          {
            label: 'WireGuard Road Warrior',
            description: 'Standalone WireGuard instance with managed peers.',
            service_code: 'wireguard',
            preset_key: 'roadwarrior',
            name_suffix: 'wireguard',
            slug_suffix: 'wireguard',
            endpoint_port: 51820,
            requires_endpoint_host: true,
            spec: {
              service_profile: 'roadwarrior',
              address_pool_mode: 'auto',
              client_allowed_ips: '0.0.0.0/0, ::/0',
              client_dns: '1.1.1.1, 1.0.0.1',
              persistent_keepalive: 25,
              config_mode: '0600',
            },
          },
        ],
        display_order: 500,
      };
    }

    function openServicePackEditor(packKey) {
      if (!canManageServicePacks()) {
        openActionOutcomeModal('Service pack catalog', 'settings.manage required', 'failed', 'Your role cannot manage service pack templates.', []);
        return;
      }
      const model = servicePackEditorModel(packKey);
      openModal(packKey ? `Edit pack: ${model.label || packKey}` : 'Add service pack', 'Service pack catalog template', `
        <form id="servicePackEditorForm" class="form-grid">
          <div class="field full">
            <label>Template JSON</label>
            <textarea class="code-textarea" name="template_json" rows="24" spellcheck="false">${escapeHTML(JSON.stringify(model, null, 2))}</textarea>
          </div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save pack</button>
            <button class="secondary-btn" id="cancelServicePackEditorBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="servicePackEditorResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelServicePackEditorBtn')?.addEventListener('click', closeModal);
      document.getElementById('servicePackEditorForm')?.addEventListener('submit', submitServicePackEditor);
    }

    async function submitServicePackEditor(event) {
      event.preventDefault();
      const result = document.getElementById('servicePackEditorResult');
      const textarea = event.currentTarget.querySelector('textarea[name="template_json"]');
      let payload = null;
      try {
        payload = JSON.parse(String(textarea?.value || '').trim());
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">invalid JSON: ${escapeHTML(err.message)}</span>`;
        return;
      }
      const key = String(payload?.key || '').trim();
      if (!key) {
        if (result) result.innerHTML = '<span class="tag danger">key is required</span>';
        return;
      }
      if (result) result.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const saved = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(key)}`, 'PUT', payload);
        if (textarea) textarea.value = JSON.stringify(saved, null, 2);
        if (result) result.innerHTML = `<span class="tag ok">saved</span> <code>${escapeHTML(saved.key || key)}</code>`;
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function deleteServicePackTemplate(packKey) {
      if (!packKey || !canManageServicePacks()) return;
      if (!window.confirm(`Delete service pack template ${packKey}? Existing instances are not changed.`)) return;
      try {
        await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}`, 'DELETE', null);
        await refresh();
      } catch (err) {
        openActionOutcomeModal('Service pack catalog', 'Template delete failed', 'failed', err.message || 'Service pack template delete failed.', [
          { label: 'Pack', value: packKey },
        ]);
      }
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
