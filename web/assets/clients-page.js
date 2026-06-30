(function (window) {
  'use strict';

  function createClientsPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      tableCard,
      statusTag,
      escapeHTML,
      formatDate,
      requestJSON,
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
      typeof formatDate !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof renderActionResponse !== 'function'
    ) {
      throw new Error('MegaVPNClientsPage requires page dependencies');
    }

    function findClient(clientID) {
      return (state.clients || []).find((item) => item.id === clientID) || null;
    }

    function findInstance(instanceID) {
      return (state.instances || []).find((item) => item.id === instanceID) || null;
    }

    function findNode(nodeID) {
      return (state.nodes || []).find((item) => item.id === nodeID) || null;
    }

    function egressNodeOptions(selectedID = '') {
      const nodes = (state.nodes || [])
        .filter((node) => String(node.role || '').toLowerCase() === 'egress')
        .filter((node) => String(node.status || '').toLowerCase() !== 'retired');
      const options = ['<option value="">Select egress node</option>'];
      nodes.forEach((node) => {
        const selected = node.id === selectedID ? ' selected' : '';
        const address = node.address ? ` · ${node.address}` : '';
        const role = String(node.role || 'node').trim() || 'node';
        options.push(`<option value="${escapeHTML(node.id)}"${selected}>${escapeHTML(node.name || node.id)} · ${escapeHTML(role)}${escapeHTML(address)}</option>`);
      });
      return options.join('');
    }

    function endpointLabel(instance) {
      if (!instance?.endpoint_host) return 'n/a';
      return `${instance.endpoint_host}:${instance.endpoint_port || 0}`;
    }

    function serviceInstanceLabel(instance) {
      const node = findNode(instance.node_id);
      const role = node?.role ? `/${node.role}` : '';
      const endpoint = endpointLabel(instance);
      return `${instance.name || instance.id} - ${instance.service_code || 'service'} - ${node?.name || instance.node_id || 'node'}${role} - ${endpoint}`;
    }

    function artifactTypeLabel(artifactType) {
      switch (String(artifactType || '').trim()) {
        case 'ovpn':
          return '.ovpn';
        case 'vless_url':
          return 'VLESS URL';
        case 'wg_conf':
          return 'WireGuard';
        case 'mtproto_url':
          return 'MTProto URL';
        case 'http_proxy_bundle':
          return 'HTTP proxy';
        case 'ss_url':
          return 'Shadowsocks URL';
        case 'ipsec_bundle':
          return 'IPsec/L2TP';
        case 'zip_bundle':
          return 'ZIP bundle';
        default:
          return artifactType || 'artifact';
      }
    }

    function shareLinkIsUsable(link) {
      const status = String(link?.status || '').toLowerCase();
      if (status !== 'active') return false;
      if (!link?.expires_at) return true;
      const expiresAt = Date.parse(link.expires_at);
      return Number.isNaN(expiresAt) || expiresAt > Date.now();
    }

    function shareLinkDisplayStatus(link) {
      const status = String(link?.status || 'unknown').toLowerCase();
      if (status === 'active' && !shareLinkIsUsable(link)) return 'expired';
      return status;
    }

    function clientSummary(client) {
      const summary = client?.summary && typeof client.summary === 'object' ? client.summary : {};
      const clientID = client?.id || '';
      const artifacts = (state.artifacts || []).filter((artifact) => artifact.client_account_id === clientID);
      const shareLinks = (state.shareLinks || []).filter((link) => link.client_account_id === clientID);
      const readyArtifacts = artifacts.filter((artifact) => String(artifact.status || '').toLowerCase() === 'ready');
      const activeLinks = shareLinks.filter(shareLinkIsUsable);
      return {
        serviceAccessCount: Number(summary.service_access_count || 0),
        activeServiceAccessCount: Number(summary.active_service_access_count || 0),
        pendingServiceAccessCount: Number(summary.pending_service_access_count || 0),
        routeCount: Number(summary.route_count || 0),
        activeRouteCount: Number(summary.active_route_count || 0),
        artifactCount: Number(summary.artifact_count ?? artifacts.length),
        readyArtifactCount: Number(summary.ready_artifact_count ?? readyArtifacts.length),
        shareLinkCount: Number(summary.share_link_count ?? shareLinks.length),
        activeShareLinkCount: Number(summary.active_share_link_count ?? activeLinks.length),
        lastArtifactAt: summary.last_artifact_at || artifacts[0]?.created_at || '',
        nextShareLinkExpiresAt: summary.next_share_link_expires_at || activeLinks[0]?.expires_at || '',
      };
    }

    function clientLifecycleStatus(client) {
      const status = String(client.status || 'unknown').toLowerCase();
      if (status === 'active' && client.expires_at && Date.parse(client.expires_at) < Date.now()) return 'expired';
      return status;
    }

    function clientDisplayName(client) {
      return client.display_name || client.username || client.id;
    }

    function compactServiceLabel(value) {
      const normalized = String(value || '').trim();
      switch (normalized) {
        case 'xray-core':
          return 'Xray';
        case 'http_proxy':
          return 'HTTP proxy';
        case 'wireguard':
          return 'WireGuard';
        case 'openvpn':
          return 'OpenVPN';
        case 'shadowsocks':
          return 'Shadowsocks';
        case 'mtproto':
          return 'MTProto';
        case 'ipsec':
          return 'IPsec';
        default:
          return normalized || 'service';
      }
    }

    function artifactDownloadURL(clientID, artifactID) {
      const path = `/api/v1/clients/${encodeURIComponent(clientID)}/artifacts/${encodeURIComponent(artifactID)}/download`;
      try {
        return new URL(path, state.apiBase || window.location.origin).toString();
      } catch (_) {
        return path;
      }
    }

    function artifactPreviewable(artifactType) {
      return ['ovpn', 'vless_url', 'wg_conf', 'mtproto_url', 'http_proxy_bundle', 'ss_url', 'ipsec_bundle'].includes(String(artifactType || '').trim());
    }

    function provisionableClientInstances() {
      const allowed = new Set(['openvpn', 'xray-core', 'wireguard', 'mtproto', 'ipsec', 'http_proxy', 'shadowsocks']);
      return (state.instances || []).filter((instance) => {
        return allowed.has(String(instance.service_code || '').trim()) && String(instance.status || '').toLowerCase() !== 'deleted';
      }).sort((left, right) => serviceInstanceLabel(left).localeCompare(serviceInstanceLabel(right)));
    }

    function serviceInstanceOptions(instances, selectedIDs = [], emptyText = 'No provisionable instances') {
      const selected = new Set(selectedIDs || []);
      return (instances || []).map((instance) => {
        return `<option value="${escapeHTML(instance.id)}"${selected.has(instance.id) ? ' selected' : ''}>${escapeHTML(serviceInstanceLabel(instance))}</option>`;
      }).join('') || `<option value="" disabled>${escapeHTML(emptyText)}</option>`;
    }

    function clientConfigInstanceOptions(accessList = [], selectedIDs = []) {
      const accessInstanceIDs = new Set((accessList || []).map((access) => access.instance_id).filter(Boolean));
      const instances = provisionableClientInstances().filter((instance) => accessInstanceIDs.has(instance.id));
      return serviceInstanceOptions(instances, selectedIDs, 'No provisioned service access yet');
    }

    function renderActionButtons(client) {
      return `
        <div class="table-actions client-action-grid">
          <button class="secondary-btn client-accesses-btn" type="button" data-client-id="${escapeHTML(client.id)}">Manage</button>
          <button class="secondary-btn client-provision-btn" type="button" data-client-id="${escapeHTML(client.id)}">Provision</button>
          <button class="secondary-btn client-build-btn" type="button" data-client-id="${escapeHTML(client.id)}">Build</button>
          <button class="secondary-btn client-email-btn" type="button" data-client-id="${escapeHTML(client.id)}">Email</button>
        </div>`;
    }

    function bindListActions() {
      document.getElementById('clientCreateBtn')?.addEventListener('click', openCreateClientModal);
      document.querySelectorAll('.client-provision-btn').forEach((button) => {
        button.addEventListener('click', () => queueClientProvision(button.dataset.clientId));
      });
      document.querySelectorAll('.client-accesses-btn').forEach((button) => {
        button.addEventListener('click', () => openClientAccessesModal(button.dataset.clientId));
      });
      document.querySelectorAll('.client-build-btn').forEach((button) => {
        button.addEventListener('click', () => openBuildClientArtifactsForClient(button.dataset.clientId));
      });
      document.querySelectorAll('.client-email-btn').forEach((button) => {
        button.addEventListener('click', () => openClientEmailModal(button.dataset.clientId));
      });
    }

    function render() {
      setTitle('Clients');
      const rows = state.clients || [];
      const activeClients = rows.filter((client) => clientLifecycleStatus(client) === 'active').length;
      const accessTotal = rows.reduce((sum, client) => sum + clientSummary(client).serviceAccessCount, 0);
      const readyArtifacts = rows.reduce((sum, client) => sum + clientSummary(client).readyArtifactCount, 0);
      const activeLinks = rows.reduce((sum, client) => sum + clientSummary(client).activeShareLinkCount, 0);
      el('content').innerHTML = `
        <section class="clients-workspace">
          <div class="client-summary-grid">
            <div class="client-summary-card">
              <span>Clients</span>
              <strong>${escapeHTML(String(rows.length))}</strong>
              <small>${escapeHTML(String(activeClients))} active</small>
            </div>
            <div class="client-summary-card">
              <span>Service access</span>
              <strong>${escapeHTML(String(accessTotal))}</strong>
              <small>bound instances</small>
            </div>
            <div class="client-summary-card">
              <span>Configs</span>
              <strong>${escapeHTML(String(readyArtifacts))}</strong>
              <small>ready artifacts</small>
            </div>
            <div class="client-summary-card">
              <span>Delivery</span>
              <strong>${escapeHTML(String(activeLinks))}</strong>
              <small>active share links</small>
            </div>
          </div>
          <section class="table-card clients-table-card">
            <div class="table-head">
              <div>
                <h2>Client provisioning</h2>
                <p class="table-subtitle">Bind service access, generate client configs and deliver artifacts from one workflow.</p>
              </div>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(rows.length))} loaded</span>
                <button class="secondary-btn" type="button" id="clientCreateBtn">Create client</button>
              </div>
            </div>
            <div class="table-wrap">
              <table class="clients-table">
                <thead>
                  <tr>
                    <th class="clients-col-client">Client</th>
                    <th class="clients-col-contact">Contact</th>
                    <th class="clients-col-provisioning">Provisioning</th>
                    <th class="clients-col-delivery">Delivery</th>
                    <th class="clients-col-state">State</th>
                    <th class="clients-col-actions">Actions</th>
                  </tr>
                </thead>
                <tbody>${renderClientRows(rows)}</tbody>
              </table>
            </div>
          </section>
        </section>`;
      bindListActions();
    }

    function renderClientRows(rows) {
      return rows.map((client) => {
        const summary = clientSummary(client);
        const lifecycle = clientLifecycleStatus(client);
        return `
          <tr>
            <td>
              <strong class="client-primary">${escapeHTML(client.username || client.id)}</strong>
              <div class="timeline-meta">${escapeHTML(client.display_name || client.id)}</div>
            </td>
            <td>
              <div>${escapeHTML(client.email || 'no email')}</div>
              <div class="timeline-meta">expires ${escapeHTML(formatDate(client.expires_at))}</div>
            </td>
            <td>
              <div class="client-status-cluster">
                <span class="tag">${escapeHTML(String(summary.serviceAccessCount))} access</span>
                <span class="tag ${summary.pendingServiceAccessCount > 0 ? 'warn' : 'stub'}">${escapeHTML(String(summary.pendingServiceAccessCount))} pending</span>
                <span class="tag">${escapeHTML(String(summary.routeCount))} routes</span>
              </div>
            </td>
            <td>
              <div class="client-status-cluster">
                <span class="tag ${summary.readyArtifactCount > 0 ? 'ok' : 'stub'}">${escapeHTML(String(summary.readyArtifactCount))} configs</span>
                <span class="tag ${summary.activeShareLinkCount > 0 ? 'ok' : 'stub'}">${escapeHTML(String(summary.activeShareLinkCount))} links</span>
              </div>
              <div class="timeline-meta">last build ${escapeHTML(formatDate(summary.lastArtifactAt))}</div>
            </td>
            <td>${statusTag(lifecycle)}<div class="timeline-meta">updated ${escapeHTML(formatDate(client.updated_at))}</div></td>
            <td>${renderActionButtons(client)}</td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No client accounts yet. Create a client, bind service access and build configs.</div></td></tr>';
    }

    function openCreateClientModal() {
      openModal('Create client', 'POST /api/v1/clients', `
        <form id="createClientForm" class="form-grid">
          <div class="field"><label>Username</label><input name="username" required placeholder="client-01" /></div>
          <div class="field"><label>Display name</label><input name="display_name" placeholder="Client 01" /></div>
          <div class="field"><label>Email</label><input name="email" type="email" placeholder="client@example.com" /></div>
          <div class="field"><label>Expires at</label><input name="expires_at" type="datetime-local" /></div>
          <div class="field full"><label>Notes</label><textarea name="notes" rows="5" placeholder="Optional notes, contract reference or operator comment."></textarea></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Create client</button></div>
        </form>
        <div id="createClientResult" class="form-result"></div>`);
      document.getElementById('createClientForm')?.addEventListener('submit', createClient);
    }

    async function createClient(event) {
      event.preventDefault();
      const target = document.getElementById('createClientResult');
      if (target) target.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const form = new FormData(event.currentTarget);
        const expiresAtRaw = String(form.get('expires_at') || '').trim();
        const payload = {
          username: String(form.get('username') || '').trim(),
          display_name: String(form.get('display_name') || '').trim(),
          email: String(form.get('email') || '').trim(),
          notes: String(form.get('notes') || '').trim(),
        };
        if (expiresAtRaw) payload.expires_at = new Date(expiresAtRaw).toISOString();
        const data = await sendJSON('/api/v1/clients', 'POST', payload);
        if (target) target.innerHTML = renderActionResponse(data, 'Client creation');
        await refresh();
        setTimeout(closeModal, 400);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openClientEmailModal(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Send access email: ${client.username}`, 'Client delivery', `
        <form id="clientEmailForm" class="form-grid">
          <div class="field"><label>Email</label><input value="${escapeHTML(client.email || '')}" disabled /></div>
          <div class="field"><label>Share link TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="72" /></div>
          <div class="field full"><label>Subject</label><input name="subject" value="RTIS MegaVPN access package" /></div>
          <div class="field full"><label>Message</label><textarea name="message" rows="5" placeholder="Optional custom note for the client."></textarea></div>
          <div class="field"><label>Create/refresh share link</label><select name="create_share_link"><option value="true">true</option><option value="false">false</option></select></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Send email</button></div>
        </form>
        <div id="clientEmailResult" class="form-result"></div>`);
      document.getElementById('clientEmailForm')?.addEventListener('submit', (event) => sendClientEmail(event, clientID));
    }

    async function sendClientEmail(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientEmailResult');
      if (target) target.innerHTML = '<span class="tag warn">sending</span>';
      try {
        const form = new FormData(event.currentTarget);
        const data = await sendJSON(`/api/v1/clients/${clientID}/deliver-email`, 'POST', {
          subject: String(form.get('subject') || '').trim(),
          message: String(form.get('message') || '').trim(),
          ttl_hours: Number(form.get('ttl_hours') || 72),
          create_share_link: String(form.get('create_share_link')) === 'true',
        });
        if (target) target.innerHTML = renderActionResponse(data, 'Client email delivery');
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function queueClientProvision(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      const instances = provisionableClientInstances();
      openModal(`Provision client: ${client.username}`, 'Create service access and queue config generation', `
        <form id="clientProvisionForm" class="client-provision-form">
          <div class="client-workflow-strip">
            <div><span>1</span><strong>Access</strong><small>Bind the client to selected instances.</small></div>
            <div><span>2</span><strong>Secrets</strong><small>Driver state generates certificates, keys or passwords.</small></div>
            <div><span>3</span><strong>Apply</strong><small>Changed instances are queued for agent apply.</small></div>
            <div><span>4</span><strong>Configs</strong><small>Client artifacts are stored for preview/download.</small></div>
          </div>
          <div class="field full">
            <label>Service instances</label>
            <div class="client-provision-choice-grid" id="clientProvisionInstances">${renderProvisionInstanceChoices(instances)}</div>
          </div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit"${instances.length ? '' : ' disabled'}>Queue provisioning</button>
            <button class="secondary-btn" id="cancelProvisionBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="clientProvisionResult" class="form-result"></div>`);
      document.getElementById('cancelProvisionBtn')?.addEventListener('click', closeModal);
      document.getElementById('clientProvisionForm')?.addEventListener('submit', (event) => submitClientProvision(event, clientID));
    }

    function renderProvisionInstanceChoices(instances = []) {
      if (!instances.length) {
        return '<div class="empty compact-empty">No provisionable service instances. Create and apply a service instance first.</div>';
      }
      return instances.map((instance) => {
        const node = findNode(instance.node_id);
        const runtime = (state.instanceRuntimeStates || []).find((item) => item.instance_id === instance.id);
        const runtimeStatus = runtime?.runtime_status || instance.status || 'unknown';
        const healthStatus = runtime?.health_status || 'unknown';
        return `
          <label class="client-provision-choice">
            <input type="checkbox" name="instance_ids" value="${escapeHTML(instance.id)}" />
            <span>
              <strong>${escapeHTML(instance.name || instance.slug || instance.id)}</strong>
              <small>${escapeHTML(compactServiceLabel(instance.service_code))} · ${escapeHTML(node?.name || instance.node_id || 'node')} · ${escapeHTML(node?.role || 'role n/a')}</small>
              <em>${escapeHTML(endpointLabel(instance))}</em>
            </span>
            <span class="client-choice-tags">${statusTag(runtimeStatus)}${statusTag(healthStatus)}</span>
          </label>`;
      }).join('');
    }

    async function submitClientProvision(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientProvisionResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing</span>';
      const instanceIDs = Array.from(event.currentTarget.querySelector('#clientProvisionInstances')?.selectedOptions || [])
        .map((option) => option.value)
        .filter(Boolean);
      const checkboxIDs = Array.from(event.currentTarget.querySelectorAll('input[name="instance_ids"]:checked') || [])
        .map((input) => input.value)
        .filter(Boolean);
      const selectedIDs = checkboxIDs.length ? checkboxIDs : instanceIDs;
      if (selectedIDs.length === 0) {
        if (target) target.innerHTML = '<span class="tag danger">Select at least one service instance</span>';
        return;
      }
      try {
        const data = await sendJSON(`/api/v1/clients/${clientID}/provision`, 'POST', { instance_ids: selectedIDs });
        if (target) target.innerHTML = renderActionResponse(data, 'Client provision');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function renderServiceAccessRows(accessList, clientID) {
      return accessList.map((access) => {
        const instance = findInstance(access.instance_id);
        const serviceCode = instance?.service_code || 'unknown';
        return `
          <tr>
            <td>${escapeHTML(instance?.name || access.instance_id)}</td>
            <td><span class="tag">${escapeHTML(serviceCode)}</span></td>
            <td>${escapeHTML(endpointLabel(instance))}</td>
            <td>${statusTag(access.status || 'unknown')}</td>
            <td>${renderRotateButtons(clientID, access.id, serviceCode)}</td>
          </tr>`;
      }).join('') || '<tr><td colspan="5"><div class="empty">No service accesses for this client.</div></td></tr>';
    }

    function renderRotateButtons(clientID, accessID, serviceCode) {
      const actions = [
        ['openvpn', 'openvpn', 'Rotate OpenVPN'],
        ['xray-core', 'xray-core', 'Rotate Xray UUID'],
        ['xray', 'xray-core', 'Rotate Xray UUID'],
        ['wireguard', 'wireguard', 'Rotate WireGuard Keys'],
        ['mtproto', 'mtproto', 'Rotate MTProto Secret'],
        ['ipsec', 'ipsec', 'Rotate L2TP Access'],
        ['http_proxy', 'http_proxy', 'Rotate Proxy Access'],
        ['shadowsocks', 'shadowsocks', 'Rotate SS Access'],
      ].filter(([code]) => code === serviceCode);
      if (!actions.length) return '<span class="tag">no rotation</span>';
      return `
        <div class="inline-actions compact-actions">
          ${actions.map(([, driver, label]) => `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(accessID)}" data-driver="${escapeHTML(driver)}">${escapeHTML(label)}</button>`).join('')}
        </div>`;
    }

    function renderRouteRows(routeList, client, clientID) {
      return routeList.map((route) => {
        const instance = findInstance(route.instance_id);
        const node = findNode(route.node_id);
        const managed = route.metadata?.baseline === true || route.metadata?.baseline === 'true';
        return `
          <tr>
            <td><strong>${escapeHTML(route.name || route.destination)}</strong><div class="timeline-meta">${managed ? 'managed baseline' : 'manual policy'}</div></td>
            <td>${escapeHTML(client.username || client.display_name || client.id)}</td>
            <td>${escapeHTML(instance?.name || route.instance_id || 'global')}</td>
            <td>${escapeHTML(node?.name || route.node_id || 'n/a')}</td>
            <td><span class="tag">${escapeHTML(route.destination_type || 'endpoint')}</span> ${escapeHTML(route.destination || 'n/a')}</td>
            <td>${escapeHTML(route.protocol || 'any')} / ${escapeHTML(route.ports || '*')}</td>
            <td>${renderRouteEgress(route)}</td>
            <td>${statusTag(route.status || 'unknown')}</td>
            <td>
              <div class="inline-actions compact-actions">
                ${managed ? '<span class="tag">managed</span>' : `<button class="danger-btn client-route-delete-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-route-id="${escapeHTML(route.id)}">Revoke</button>`}
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="9"><div class="empty">No access routes yet. Provision the client or add a manual route.</div></td></tr>';
    }

    function renderRouteEgress(route) {
      const policy = route?.policy || {};
      const egress = policy.egress || {};
      const mode = String(egress.mode || policy.egress_mode || policy.mode || 'auto').trim() || 'auto';
      const nodeID = String(egress.node_id || policy.egress_node_id || policy.node_id || '').trim();
      const node = findNode(nodeID);
      const nextHop = String(egress.next_hop || policy.egress_next_hop || '').trim();
      const iface = String(egress.interface || policy.egress_interface || '').trim();
      if (mode === 'egress_node' || mode === 'remote_node' || mode === 'node') {
        return `
          <div><span class="tag">egress</span> ${escapeHTML(node?.name || nodeID || 'not selected')}</div>
          <div class="timeline-meta">${escapeHTML(nextHop || iface || 'backhaul not set')}</div>`;
      }
      if (mode === 'local_breakout' || mode === 'local' || mode === 'direct') {
        return '<span class="tag ok">local breakout</span>';
      }
      return '<span class="tag warn">auto / requires explicit ingress output</span>';
    }

    function renderClientArtifactRows(artifactList, accessList, clientID) {
      const instanceByAccess = new Map();
      (accessList || []).forEach((access) => {
        const instance = findInstance(access.instance_id);
        if (access.id && instance) instanceByAccess.set(access.id, instance);
      });
      return (artifactList || []).map((artifact) => {
        const instance = instanceByAccess.get(artifact.service_access_id || '');
        const canPreview = artifactPreviewable(artifact.artifact_type);
        return `
          <tr>
            <td><span class="tag">${escapeHTML(artifactTypeLabel(artifact.artifact_type))}</span></td>
            <td>${escapeHTML(instance?.name || artifact.service_access_id || 'shared')}</td>
            <td>${escapeHTML(String(artifact.size_bytes || 0))} B</td>
            <td>${statusTag(artifact.status || 'unknown')}</td>
            <td>${escapeHTML(formatDate(artifact.created_at))}</td>
            <td>
              <div class="inline-actions compact-actions">
                ${canPreview ? `<button class="secondary-btn client-artifact-preview-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-artifact-id="${escapeHTML(artifact.id)}">Preview</button>` : ''}
                <button class="secondary-btn client-artifact-download-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-artifact-id="${escapeHTML(artifact.id)}">Download</button>
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No generated configs yet. Build client configs after service access is created.</div></td></tr>';
    }

    function renderClientShareLinkRows(shareLinkList, artifactList, clientID) {
      const artifactByID = new Map((artifactList || []).map((artifact) => [artifact.id, artifact]));
      return (shareLinkList || []).map((link) => {
        const artifact = artifactByID.get(link.target_id || '');
        return `
          <tr>
            <td><span class="tag">${escapeHTML(link.token_hint || 'hidden')}</span></td>
            <td>${escapeHTML(artifact ? artifactTypeLabel(artifact.artifact_type) : link.target_id || 'artifact')}</td>
            <td>${statusTag(shareLinkDisplayStatus(link))}</td>
            <td>${escapeHTML(formatDate(link.expires_at))}</td>
            <td>${escapeHTML(String(link.download_count || 0))}</td>
            <td>
              <div class="inline-actions compact-actions">
                <button class="danger-btn client-share-revoke-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-link-id="${escapeHTML(link.id)}">Revoke</button>
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No delivery links yet. Publish a link after a config artifact is ready.</div></td></tr>';
    }

    function renderClientAccessOverview(client, accessList, routeList, artifactList, shareLinkList) {
      const activeAccesses = accessList.filter((item) => String(item.status || '').toLowerCase() === 'active').length;
      const pendingAccesses = accessList.filter((item) => String(item.status || '').toLowerCase() === 'pending').length;
      const readyArtifacts = artifactList.filter((item) => String(item.status || '').toLowerCase() === 'ready').length;
      const activeLinks = shareLinkList.filter(shareLinkIsUsable).length;
      return `
        <div class="client-access-overview">
          <div>
            <span>Client</span>
            <strong>${escapeHTML(clientDisplayName(client))}</strong>
            <small>${escapeHTML(client.email || 'no email')}</small>
          </div>
          <div>
            <span>Access</span>
            <strong>${escapeHTML(String(accessList.length))}</strong>
            <small>${escapeHTML(String(activeAccesses))} active · ${escapeHTML(String(pendingAccesses))} pending</small>
          </div>
          <div>
            <span>Routes</span>
            <strong>${escapeHTML(String(routeList.length))}</strong>
            <small>policy rows</small>
          </div>
          <div>
            <span>Configs</span>
            <strong>${escapeHTML(String(readyArtifacts))}</strong>
            <small>${escapeHTML(String(artifactList.length))} total artifacts</small>
          </div>
          <div>
            <span>Delivery</span>
            <strong>${escapeHTML(String(activeLinks))}</strong>
            <small>active links</small>
          </div>
        </div>`;
    }

    async function openClientAccessesModal(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Client access: ${client.username}`, 'Provisioned service bindings and route policy', '<div class="empty">Loading service accesses...</div>', { wide: true });
      try {
        const [accesses, routes, artifacts, shareLinks] = await Promise.all([
          requestJSON(`/api/v1/clients/${clientID}/accesses`),
          requestJSON(`/api/v1/clients/${clientID}/routes`),
          requestJSON(`/api/v1/clients/${clientID}/artifacts`),
          requestJSON(`/api/v1/clients/${clientID}/share-links`),
        ]);
        const accessList = Array.isArray(accesses) ? accesses : [];
        const routeList = Array.isArray(routes) ? routes : [];
        const artifactList = Array.isArray(artifacts) ? artifacts : [];
        const shareLinkList = Array.isArray(shareLinks) ? shareLinks : [];
        const accessOptions = accessList.map((access) => {
          const instance = findInstance(access.instance_id);
          return `<option value="${escapeHTML(access.id)}">${escapeHTML(instance?.name || access.instance_id)} - ${escapeHTML(access.status || 'unknown')}</option>`;
        }).join('');
        el('modalBody').innerHTML = `
          <div id="clientAccessRotateResult" class="form-result"></div>
          ${renderClientAccessOverview(client, accessList, routeList, artifactList, shareLinkList)}
          <section class="table-card compact-card">
            <div class="table-head"><h2>Service Accesses</h2><span class="tag">${escapeHTML(String(accessList.length))}</span></div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Instance</th><th>Service</th><th>Endpoint</th><th>Status</th><th>Actions</th></tr></thead>
                <tbody>${renderServiceAccessRows(accessList, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>Access Routes</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(routeList.length))} routes</span>
                <button class="secondary-btn" id="clientRouteAddBtn" type="button">Add route</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Route</th><th>User</th><th>Instance</th><th>Node</th><th>Destination</th><th>Protocol</th><th>Egress</th><th>Status</th><th>Actions</th></tr></thead>
                <tbody>${renderRouteRows(routeList, client, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>Client Configs</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(artifactList.length))} files</span>
                <button class="secondary-btn" id="clientArtifactBuildBtn" type="button">Build configs</button>
                <button class="secondary-btn" id="clientSharePublishBtn" type="button">Publish link</button>
                <button class="secondary-btn" id="clientManageEmailBtn" type="button">Email</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Type</th><th>Instance</th><th>Size</th><th>Status</th><th>Created</th><th>Actions</th></tr></thead>
                <tbody>${renderClientArtifactRows(artifactList, accessList, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>Delivery Links</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(shareLinkList.length))} links</span>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Token</th><th>Artifact</th><th>Status</th><th>Expires</th><th>Downloads</th><th>Actions</th></tr></thead>
                <tbody>${renderClientShareLinkRows(shareLinkList, artifactList, clientID)}</tbody>
              </table>
            </div>
          </section>`;
        bindAccessModalActions(clientID, accessOptions, accessList, artifactList);
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load service accesses: ${escapeHTML(err.message)}</div>`;
      }
    }

    function bindAccessModalActions(clientID, accessOptions, accessList = [], artifactList = []) {
      document.querySelectorAll('.rotate-access-btn').forEach((button) => {
        button.addEventListener('click', () => rotateClientAccess(button.dataset.clientId, button.dataset.accessId, button.dataset.driver));
      });
      document.getElementById('clientRouteAddBtn')?.addEventListener('click', () => openCreateClientRouteModal(clientID, accessOptions));
      document.querySelectorAll('.client-route-delete-btn').forEach((button) => {
        button.addEventListener('click', () => revokeClientAccessRoute(button.dataset.clientId, button.dataset.routeId));
      });
      document.getElementById('clientArtifactBuildBtn')?.addEventListener('click', () => openBuildClientArtifactsModal(clientID, accessList));
      document.getElementById('clientSharePublishBtn')?.addEventListener('click', () => openPublishShareLinkModal(clientID, artifactList));
      document.getElementById('clientManageEmailBtn')?.addEventListener('click', () => openClientEmailModal(clientID));
      document.querySelectorAll('.client-share-revoke-btn').forEach((button) => {
        button.addEventListener('click', () => revokeClientShareLink(button.dataset.clientId, button.dataset.linkId));
      });
      document.querySelectorAll('.client-artifact-preview-btn').forEach((button) => {
        button.addEventListener('click', () => previewClientArtifact(button.dataset.clientId, button.dataset.artifactId));
      });
      document.querySelectorAll('.client-artifact-download-btn').forEach((button) => {
        button.addEventListener('click', () => {
          const url = artifactDownloadURL(button.dataset.clientId, button.dataset.artifactId);
          window.open(url, '_blank', 'noopener,noreferrer');
        });
      });
    }

    function openBuildClientArtifactsModal(clientID, accessList = []) {
      const client = findClient(clientID);
      if (!client) return;
      const defaultInstances = (accessList || []).map((access) => access.instance_id).filter(Boolean);
      openModal(`Build configs: ${client.username}`, 'Generate OVPN, VLESS and other client artifacts', `
        <form id="clientArtifactBuildForm" class="form-grid">
          <div class="field"><label>Artifact type</label><select name="artifact_type">
            <option value="all">all supported</option>
            <option value="zip_bundle">zip_bundle</option>
            <option value="ovpn">ovpn</option>
            <option value="vless_url">vless_url</option>
            <option value="wg_conf">wg_conf</option>
            <option value="mtproto_url">mtproto_url</option>
            <option value="http_proxy_bundle">http_proxy_bundle</option>
            <option value="ipsec_bundle">ipsec_bundle</option>
            <option value="ss_url">ss_url</option>
          </select></div>
          <div class="field full"><label>Provisioned service accesses</label><select name="instance_ids" id="clientArtifactInstances" multiple size="8">${clientConfigInstanceOptions(accessList, defaultInstances)}</select></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Queue build</button><button class="secondary-btn" id="cancelClientArtifactBuildBtn" type="button">Cancel</button></div>
        </form>
        <div id="clientArtifactBuildResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelClientArtifactBuildBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientArtifactBuildForm')?.addEventListener('submit', (event) => submitClientArtifactBuild(event, clientID));
    }

    async function openBuildClientArtifactsForClient(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Build configs: ${client.username}`, 'Loading provisioned service access', '<div class="empty">Loading service accesses...</div>', { wide: true });
      try {
        const accesses = await requestJSON(`/api/v1/clients/${clientID}/accesses`);
        openBuildClientArtifactsModal(clientID, Array.isArray(accesses) ? accesses : []);
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load service accesses: ${escapeHTML(err.message)}</div>`;
      }
    }

    async function submitClientArtifactBuild(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientArtifactBuildResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing config build</span>';
      try {
        const formElement = event.currentTarget;
        const form = new FormData(formElement);
        const instanceIDs = Array.from(formElement.querySelector('#clientArtifactInstances')?.selectedOptions || [])
          .map((option) => option.value)
          .filter(Boolean);
        if (instanceIDs.length === 0) {
          if (target) target.innerHTML = '<span class="tag danger">Provision at least one service access first</span>';
          return;
        }
        const data = await sendJSON(`/api/v1/clients/${clientID}/artifacts`, 'POST', {
          type: String(form.get('artifact_type') || 'all').trim(),
          instance_ids: instanceIDs,
        });
        if (target) target.innerHTML = renderActionResponse(data, 'Client config build');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openPublishShareLinkModal(clientID, artifactList = []) {
      const client = findClient(clientID);
      if (!client) return;
      const readyArtifacts = (artifactList || []).filter((artifact) => String(artifact.status || '').toLowerCase() === 'ready');
      const options = readyArtifacts.map((artifact) => {
        const label = `${artifactTypeLabel(artifact.artifact_type)} · ${artifact.size_bytes || 0} B · ${formatDate(artifact.created_at)}`;
        return `<option value="${escapeHTML(artifact.id)}">${escapeHTML(label)}</option>`;
      }).join('');
      openModal(`Publish delivery link: ${client.username}`, 'Create a temporary download link for one ready artifact', `
        <form id="clientShareLinkForm" class="form-grid">
          <div class="field full"><label>Ready artifact</label><select name="target_id" required>${options || '<option value="" disabled>No ready artifacts</option>'}</select></div>
          <div class="field"><label>TTL hours</label><input name="ttl_hours" type="number" min="1" max="720" value="72" /></div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit"${readyArtifacts.length ? '' : ' disabled'}>Publish link</button>
            <button class="secondary-btn" id="cancelShareLinkBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="clientShareLinkResult" class="form-result"></div>`);
      document.getElementById('cancelShareLinkBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientShareLinkForm')?.addEventListener('submit', (event) => submitClientShareLink(event, clientID));
    }

    async function submitClientShareLink(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientShareLinkResult');
      if (target) target.innerHTML = '<span class="tag warn">publishing link</span>';
      try {
        const form = new FormData(event.currentTarget);
        await sendJSON(`/api/v1/clients/${clientID}/share-links`, 'POST', {
          target_id: String(form.get('target_id') || '').trim(),
          ttl_hours: Number(form.get('ttl_hours') || 72),
        });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokeClientShareLink(clientID, linkID) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">revoking link</span>';
      try {
        await requestJSON(`/api/v1/clients/${clientID}/share-links/${linkID}/revoke`, { method: 'POST' });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function previewClientArtifact(clientID, artifactID) {
      openModal('Client config preview', 'Loading generated artifact', '<div class="empty">Loading artifact...</div>', { wide: true });
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/artifacts/${artifactID}/content`);
        el('modalBody').innerHTML = `
          <div class="card">
            <div class="mini-label">${escapeHTML(data.artifact_type || 'artifact')}</div>
            <h3>${escapeHTML(data.filename || artifactID)}</h3>
            <div class="metric-caption">${escapeHTML(String(data.size_bytes || 0))} bytes</div>
          </div>
          <textarea class="code-textarea" rows="18" readonly>${escapeHTML(data.content || '')}</textarea>
          <div class="inline-actions" style="margin-top:12px">
            <button class="secondary-btn" id="downloadPreviewArtifactBtn" type="button">Download</button>
          </div>`;
        document.getElementById('downloadPreviewArtifactBtn')?.addEventListener('click', () => {
          window.open(artifactDownloadURL(clientID, artifactID), '_blank', 'noopener,noreferrer');
        });
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load artifact: ${escapeHTML(err.message)}</div>`;
      }
    }

    function openCreateClientRouteModal(clientID, accessOptions = '') {
      const client = findClient(clientID);
      if (!client) return;
      const hasAccessOptions = String(accessOptions || '').trim() !== '';
      const bindingOptions = hasAccessOptions
        ? `<option value="">Select service access</option>${accessOptions}`
        : '<option value="">Provision service access first</option>';
      openModal(`Add route: ${client.username}`, 'Client access routing', `
        <form id="clientRouteForm" class="form-grid">
          <div class="field full"><label>Service access binding</label><select name="service_access_id" required>${bindingOptions}</select></div>
          <div class="field"><label>Name</label><input name="name" placeholder="office-lan" /></div>
          <div class="field"><label>Action</label><select name="action"><option value="allow">allow</option><option value="deny">deny</option></select></div>
          <div class="field"><label>Destination type</label><select name="destination_type"><option value="cidr">cidr</option><option value="dns">dns</option><option value="endpoint">endpoint</option><option value="service">service</option></select></div>
          <div class="field"><label>Destination</label><input name="destination" required placeholder="10.10.0.0/16 or app.internal" /></div>
          <div class="field"><label>Protocol</label><select name="protocol"><option value="any">any</option><option value="tcp">tcp</option><option value="udp">udp</option><option value="icmp">icmp</option></select></div>
          <div class="field"><label>Ports</label><input name="ports" value="*" placeholder="443 or 80,443 or 1000-2000" /></div>
          <div class="field"><label>Egress mode</label><select name="egress_mode"><option value="">auto</option><option value="egress_node">egress node</option><option value="local_breakout">local breakout</option></select></div>
          <div class="field"><label>Egress node</label><select name="egress_node_id">${egressNodeOptions()}</select></div>
          <div class="field"><label>Backhaul next-hop</label><input name="egress_next_hop" placeholder="10.255.0.2" /></div>
          <div class="field"><label>Backhaul interface</label><input name="egress_interface" placeholder="wg-backhaul0" /></div>
          <div class="field"><label>Routing table</label><input name="routing_table" placeholder="main" /></div>
          <div class="field full"><label>Description</label><textarea name="description" rows="3" placeholder="Why this route exists and what it permits."></textarea></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit"${hasAccessOptions ? '' : ' disabled'}>Save route</button><button class="secondary-btn" id="cancelClientRouteBtn" type="button">Cancel</button></div>
        </form>
        <div id="clientRouteResult" class="form-result"></div>`);
      document.getElementById('cancelClientRouteBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientRouteForm')?.addEventListener('submit', (event) => submitClientRoute(event, clientID));
    }

    async function submitClientRoute(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientRouteResult');
      if (target) target.innerHTML = '<span class="tag warn">saving route</span>';
      const form = new FormData(event.currentTarget);
      const payload = {
        service_access_id: String(form.get('service_access_id') || '').trim() || null,
        name: String(form.get('name') || '').trim(),
        action: String(form.get('action') || 'allow'),
        destination_type: String(form.get('destination_type') || 'cidr'),
        destination: String(form.get('destination') || '').trim(),
        protocol: String(form.get('protocol') || 'any'),
        ports: String(form.get('ports') || '*').trim() || '*',
        description: String(form.get('description') || '').trim(),
      };
      const egress = {
        mode: String(form.get('egress_mode') || '').trim(),
        node_id: String(form.get('egress_node_id') || '').trim(),
        next_hop: String(form.get('egress_next_hop') || '').trim(),
        interface: String(form.get('egress_interface') || '').trim(),
        table: String(form.get('routing_table') || '').trim(),
      };
      if (egress.mode || egress.node_id || egress.next_hop || egress.interface || egress.table) {
        payload.policy = { egress };
      }
      try {
        await sendJSON(`/api/v1/clients/${clientID}/routes`, 'POST', payload);
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokeClientAccessRoute(clientID, routeID) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">revoking route</span>';
      try {
        await requestJSON(`/api/v1/clients/${clientID}/routes/${routeID}`, { method: 'DELETE' });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function rotateClientAccess(clientID, accessID, driver) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing rotation</span>';
      const suffix = driver === 'openvpn'
        ? 'rotate-openvpn'
        : driver === 'wireguard'
          ? 'rotate-wireguard'
        : driver === 'mtproto'
          ? 'rotate-mtproto'
        : driver === 'ipsec'
          ? 'rotate-ipsec'
        : driver === 'http_proxy'
          ? 'rotate-http-proxy'
        : driver === 'shadowsocks'
          ? 'rotate-shadowsocks'
          : 'rotate-xray';
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/accesses/${accessID}/${suffix}`, { method: 'POST' });
        if (target) target.innerHTML = renderActionResponse(data, 'Client access rotation');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    return {
      render,
      openCreateClientModal,
    };
  }

  window.MegaVPNClientsPage = { create: createClientsPage };
})(window);
