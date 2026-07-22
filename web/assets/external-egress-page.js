(function (window) {
  'use strict';

  function resolveImportFormat(protocol, raw = '', existingFormat = '') {
    const selectedProtocol = String(protocol || '').trim().toLowerCase();
    const content = String(raw || '').trim();
    if (!content && String(existingFormat || '').trim()) return String(existingFormat).trim().toLowerCase();
    if (selectedProtocol === 'openvpn') return 'ovpn';
    if (selectedProtocol === 'wireguard') return 'conf';
    if (selectedProtocol === 'l2tp_ipsec') return content.startsWith('{') ? 'json' : 'key_value';
    if (selectedProtocol === 'vless' || selectedProtocol === 'shadowsocks') return content.startsWith('{') ? 'json' : 'url';
    return 'structured';
  }

  function createExternalEgressPage(ctx = {}) {
    const {
      state, setTitle, el, statusTag, escapeHTML, formatDate, hasPermission,
      requestJSON, sendJSON, refresh, openModal, closeModal, renderActionResponse, openActionOutcomeModal,
    } = ctx;
    if (!state || typeof setTitle !== 'function' || typeof el !== 'function' || typeof statusTag !== 'function'
      || typeof escapeHTML !== 'function' || typeof requestJSON !== 'function' || typeof sendJSON !== 'function'
      || typeof refresh !== 'function' || typeof openModal !== 'function' || typeof closeModal !== 'function') {
      throw new Error('MegaVPNExternalEgressPage requires page dependencies');
    }

    const profileByID = (id) => (state.externalEgressProfiles || []).find((item) => item.id === id) || null;
    const catalogByCode = (code) => (state.externalEgressCatalog || []).find((item) => item.code === code) || null;
    const canManage = () => hasPermission('node.write') && hasPermission('access_group.policy.write');

    function supportTag(value) {
      const support = String(value || 'planned').toLowerCase();
      return statusTag(support === 'ready' ? 'ready' : support === 'preview' ? 'preview' : 'planned');
    }

    function protocolLabel(code) {
      return catalogByCode(code)?.label || code || 'unknown';
    }

    function endpoint(profile) {
      if (!profile.endpoint_host) return 'not imported';
      return `${profile.endpoint_host}${profile.endpoint_port ? `:${profile.endpoint_port}` : ''}${profile.transport ? ` · ${profile.transport}` : ''}`;
    }

    function deploymentRows(profile) {
      const deployments = Array.isArray(profile.deployments) ? profile.deployments : [];
      if (!deployments.length) return '<div class="metric-caption">Not deployed on runtime nodes.</div>';
      return deployments.map((deployment) => `
        <div class="external-egress-deployment">
          <div>
            <strong>${escapeHTML(deployment.node_name || deployment.node_id)}</strong>
            <small><code>${escapeHTML(deployment.interface_name || 'pending')}</code> · table ${escapeHTML(deployment.routing_table || 'auto')}</small>
            ${deployment.last_error ? `<small class="text-danger">${escapeHTML(deployment.last_error)}</small>` : ''}
          </div>
          ${statusTag(deployment.status || 'unknown')}
          <div class="table-actions compact-actions">
            <button class="secondary-btn external-egress-probe-btn" type="button" data-deployment-id="${escapeHTML(deployment.id)}"${canManage() ? '' : ' disabled'}>Probe</button>
            <button class="primary-btn external-egress-apply-btn" type="button" data-deployment-id="${escapeHTML(deployment.id)}"${canManage() ? '' : ' disabled'}>Apply</button>
            <button class="danger-btn external-egress-cleanup-btn" type="button" data-deployment-id="${escapeHTML(deployment.id)}"${canManage() ? '' : ' disabled'}>Cleanup</button>
          </div>
        </div>`).join('');
    }

    function profileRows() {
      const profiles = Array.isArray(state.externalEgressProfiles) ? state.externalEgressProfiles : [];
      if (!profiles.length) return '<tr><td colspan="7"><div class="empty">No external egress profiles yet.</div></td></tr>';
      return profiles.map((profile) => `
        <tr>
          <td><strong>${escapeHTML(profile.display_name || profile.profile_key)}</strong><div class="timeline-meta"><code>${escapeHTML(profile.profile_key)}</code></div></td>
          <td><strong>${escapeHTML(protocolLabel(profile.protocol))}</strong><div class="timeline-meta">${escapeHTML(profile.transport || 'n/a')}</div></td>
          <td>${statusTag(profile.status || 'unknown')} ${supportTag(profile.runtime_support)}</td>
          <td><code>${escapeHTML(endpoint(profile))}</code></td>
          <td>${(profile.secret_purposes || []).map((purpose) => `<span class="tag">${escapeHTML(purpose)}</span>`).join(' ') || '<span class="tag">none</span>'}</td>
          <td><div class="external-egress-deployments">${deploymentRows(profile)}</div></td>
          <td>
            <div class="table-actions compact-actions">
              <button class="primary-btn external-egress-deploy-btn" type="button" data-profile-id="${escapeHTML(profile.id)}"${canManage() && profile.status === 'active' && profile.runtime_support === 'ready' ? '' : ' disabled'}>Deploy</button>
              <button class="secondary-btn external-egress-edit-btn" type="button" data-profile-id="${escapeHTML(profile.id)}"${canManage() ? '' : ' disabled'}>Edit</button>
              <button class="danger-btn external-egress-delete-btn" type="button" data-profile-id="${escapeHTML(profile.id)}"${canManage() ? '' : ' disabled'}>Delete</button>
            </div>
          </td>
        </tr>`).join('');
    }

    function render() {
      setTitle('External egress');
      const catalog = Array.isArray(state.externalEgressCatalog) ? state.externalEgressCatalog : [];
      const ready = catalog.filter((item) => item.runtime_support === 'ready').length;
      el('content').innerHTML = `
        <section class="section-card external-egress-summary">
          <div class="section-head">
            <div>
              <h2>External provider gateways</h2>
              <p>Import provider client profiles, deploy them on runtime nodes and assign them to client groups. Node traffic and the main routing table are not redirected.</p>
            </div>
            <div class="table-tools">
              <span class="tag ok">${escapeHTML(String(ready))} runtime-ready</span>
              <span class="tag">${escapeHTML(String(catalog.length))} catalogued</span>
              <button class="primary-btn" id="externalEgressCreateBtn" type="button"${canManage() ? '' : ' disabled'}>New profile</button>
            </div>
          </div>
          <div class="external-egress-flow" aria-label="External egress traffic flow">
            <div><span>1</span><strong>Client group</strong><small>global membership</small></div>
            <i>→</i><div><span>2</span><strong>Xray outbound</strong><small>group-specific route</small></div>
            <i>→</i><div><span>3</span><strong>Isolated runtime</strong><small>local proxy or policy table</small></div>
            <i>→</i><div><span>4</span><strong>Provider gateway</strong><small>L2TP/IPsec, VLESS, Shadowsocks, OpenVPN or WireGuard</small></div>
          </div>
        </section>
        <section class="table-card">
          <div class="table-head"><div><h2>Profiles and deployments</h2><p class="table-subtitle">A profile stores encrypted provider credentials. A deployment materializes it on one node. Profile changes mark existing deployments pending until Apply completes.</p></div><span class="tag">${escapeHTML(String((state.externalEgressProfiles || []).length))} profiles</span></div>
          <div class="table-wrap"><table class="external-egress-table"><thead><tr><th>Profile</th><th>Protocol</th><th>Status</th><th>Endpoint</th><th>Secrets</th><th>Node deployments</th><th>Actions</th></tr></thead><tbody>${profileRows()}</tbody></table></div>
        </section>
        <section class="table-card">
          <div class="table-head"><div><h2>Protocol support</h2><p class="table-subtitle">Only runtime-ready protocols can be activated. Preview and planned profiles cannot affect traffic.</p></div></div>
          <div class="external-egress-catalog">${catalog.map((item) => `
            <article><div><strong>${escapeHTML(item.label)}</strong>${supportTag(item.runtime_support)}</div><small>${escapeHTML(item.notes || '')}</small><div class="timeline-meta">${escapeHTML((item.import_formats || []).join(', '))}</div></article>`).join('')}</div>
        </section>`;
      bindActions();
    }

    function bindActions() {
      document.getElementById('externalEgressCreateBtn')?.addEventListener('click', () => openProfileEditor(null));
      document.querySelectorAll('.external-egress-edit-btn').forEach((button) => button.addEventListener('click', () => openProfileEditor(profileByID(button.dataset.profileId))));
      document.querySelectorAll('.external-egress-deploy-btn').forEach((button) => button.addEventListener('click', () => openDeployModal(profileByID(button.dataset.profileId))));
      document.querySelectorAll('.external-egress-apply-btn').forEach((button) => button.addEventListener('click', () => queueDeploymentAction(button.dataset.deploymentId, 'apply')));
      document.querySelectorAll('.external-egress-probe-btn').forEach((button) => button.addEventListener('click', () => queueDeploymentAction(button.dataset.deploymentId, 'probe')));
      document.querySelectorAll('.external-egress-cleanup-btn').forEach((button) => button.addEventListener('click', () => queueDeploymentAction(button.dataset.deploymentId, 'cleanup')));
      document.querySelectorAll('.external-egress-delete-btn').forEach((button) => button.addEventListener('click', () => deleteProfile(profileByID(button.dataset.profileId))));
    }

    function protocolOptions(selected) {
      return (state.externalEgressCatalog || []).map((item) => `<option value="${escapeHTML(item.code)}"${item.code === selected ? ' selected' : ''}>${escapeHTML(item.label)} · ${escapeHTML(item.runtime_support)}</option>`).join('');
    }

    function openProfileEditor(profile) {
      const isEdit = Boolean(profile);
      const protocol = profile?.protocol || 'openvpn';
      openModal(isEdit ? `Edit profile: ${profile.display_name}` : 'New external egress profile', 'Provider client configuration', `
        <form id="externalEgressProfileForm" class="form-grid external-egress-profile-form">
          <div class="field full external-egress-form-heading"><strong>Profile identity</strong><span>Operator-facing identity and lifecycle state.</span></div>
          <div class="field"><label>Profile key</label><input name="profile_key" required pattern="[a-z][a-z0-9_]{2,63}" value="${escapeHTML(profile?.profile_key || '')}" placeholder="provider_us_east"${isEdit ? ' readonly' : ''}></div>
          <div class="field"><label>Name</label><input name="display_name" required value="${escapeHTML(profile?.display_name || '')}" placeholder="Provider USA East"></div>
          <div class="field"><label>Protocol</label><select name="protocol" id="externalEgressProtocol"${isEdit ? ' disabled' : ''}>${protocolOptions(protocol)}</select>${isEdit ? `<input type="hidden" name="protocol_fixed" value="${escapeHTML(protocol)}">` : ''}</div>
          <div class="field"><label>Status</label><select name="status" id="externalEgressStatus"><option value="draft"${profile?.status === 'draft' || !profile ? ' selected' : ''}>draft</option><option value="active"${profile?.status === 'active' ? ' selected' : ''}>active</option><option value="disabled"${profile?.status === 'disabled' ? ' selected' : ''}>disabled</option></select><div class="field-hint" id="externalEgressRuntimeHint"></div></div>
          <div class="field full"><label>Description</label><input name="description" value="${escapeHTML(profile?.description || '')}" placeholder="Provider account, region and operational owner"></div>

          <div class="field full external-egress-form-heading"><strong>Connection</strong><span id="externalEgressConnectionHint">Import the provider configuration to verify its endpoint and transport.</span></div>
          <div class="field"><label>Provider endpoint</label><input id="externalEgressEndpointHost" name="endpoint_host" value="${escapeHTML(profile?.endpoint_host || '')}" placeholder="vpn.provider.example"></div>
          <div class="field"><label>Endpoint port</label><input id="externalEgressEndpointPort" name="endpoint_port" type="number" min="1" max="65535" value="${escapeHTML(String(profile?.endpoint_port || ''))}" placeholder="1194"></div>
          <div class="field full"><label>Transport</label><input id="externalEgressTransport" name="transport" value="${escapeHTML(profile?.transport || '')}" placeholder="udp, tcp, tls or quic"></div>
          <div class="field full" id="externalEgressImportFields">
            <label>Provider client configuration</label>
            <input id="externalEgressConfigFile" type="file" accept=".ovpn,.conf,.txt,.json">
            <textarea id="externalEgressConfigText" rows="8" placeholder="Paste the complete provider client configuration"></textarea>
            <div class="field-hint">Preview validates the file and fills the endpoint fields. The original configuration is encrypted and is never returned by the API.</div>
          </div>

          <div class="field full external-egress-form-heading" id="externalEgressCredentialsHeading"><strong>Credentials</strong><span>Only fields used by the selected protocol are shown. Leave unchanged secrets empty while editing.</span></div>
          <div class="field external-egress-protocol-field" data-protocols="openvpn,socks5,http_connect,l2tp,l2tp_ipsec,ikev2"><label>Username</label><input name="username" autocomplete="off" placeholder="Provider username"></div>
          <div class="field external-egress-protocol-field" data-protocols="openvpn,socks5,http_connect,shadowsocks,l2tp,l2tp_ipsec,ikev2,trojan,hysteria2"><label>Password</label><input name="password" type="password" autocomplete="new-password" placeholder="Provider password"></div>
          <div class="field external-egress-protocol-field" data-protocols="vless"><label>Client UUID</label><input name="uuid" autocomplete="off" placeholder="VLESS UUID"></div>
          <div class="field external-egress-protocol-field" data-protocols="wireguard,l2tp_ipsec,ikev2"><label>Pre-shared key</label><input name="preshared_key" type="password" autocomplete="new-password" placeholder="Provider PSK"></div>
          <details class="field full" id="externalEgressCertificateFields"><summary>Certificate and key material</summary><div class="form-grid external-egress-secret-grid">
            <div class="field external-egress-protocol-field" data-protocols="openvpn,ikev2,trojan,hysteria2"><label>CA certificate</label><textarea name="ca_certificate" rows="4" placeholder="PEM"></textarea></div>
            <div class="field external-egress-protocol-field" data-protocols="openvpn,ikev2,trojan,hysteria2"><label>Client certificate</label><textarea name="certificate" rows="4" placeholder="PEM"></textarea></div>
            <div class="field external-egress-protocol-field" data-protocols="openvpn,wireguard,ikev2,trojan,hysteria2"><label>Private key</label><textarea name="private_key" rows="4" placeholder="PEM or WireGuard private key"></textarea></div>
            <div class="field external-egress-protocol-field" data-protocols="openvpn"><label>TLS auth key</label><textarea name="tls_auth_key" rows="4" placeholder="OpenVPN tls-auth static key"></textarea></div>
            <div class="field external-egress-protocol-field" data-protocols="openvpn"><label>TLS crypt key</label><textarea name="tls_crypt_key" rows="4" placeholder="OpenVPN tls-crypt static key"></textarea></div>
          </div></details>
          <div class="field full modal-actions">
            <button class="secondary-btn" id="externalEgressPreviewBtn" type="button">Validate configuration</button>
            <button class="primary-btn" id="externalEgressSaveBtn" type="submit">${isEdit ? 'Save profile' : 'Create profile'}</button>
            <button class="secondary-btn" id="externalEgressCancelBtn" type="button">Cancel</button>
          </div>
        </form><div id="externalEgressProfileResult" class="form-result"></div>`, { size: 'large' });
      let previewedContent = '';
      const importedProtocols = new Set(['openvpn', 'wireguard', 'l2tp_ipsec', 'vless', 'shadowsocks']);
      const content = async () => {
        const file = document.getElementById('externalEgressConfigFile')?.files?.[0];
        if (file) return file.text();
        return String(document.getElementById('externalEgressConfigText')?.value || '').trim();
      };
      const syncRuntimeSupport = () => {
        const selectedProtocol = String(document.getElementById('externalEgressProtocol')?.value || protocol);
        const definition = catalogByCode(selectedProtocol);
        const imported = importedProtocols.has(selectedProtocol);
        const status = document.getElementById('externalEgressStatus');
        const activeOption = status?.querySelector('option[value="active"]');
        const ready = definition?.runtime_support === 'ready';
        if (activeOption) activeOption.disabled = !ready;
        if (!ready && status?.value === 'active') status.value = 'draft';
        const hint = document.getElementById('externalEgressRuntimeHint');
        if (hint) hint.textContent = ready
          ? 'Active profiles can be deployed on nodes.'
          : `Runtime is ${definition?.runtime_support || 'planned'}; this profile can only be saved as draft.`;
        const importFields = document.getElementById('externalEgressImportFields');
        if (importFields) importFields.hidden = !imported;
        const previewButton = document.getElementById('externalEgressPreviewBtn');
        if (previewButton) previewButton.hidden = !imported;
        const connectionHint = document.getElementById('externalEgressConnectionHint');
        if (connectionHint) {
          const hints = {
            l2tp_ipsec: 'Paste server=<host> and optional remote_id=<identity>, or a JSON object. Add username, password and PSK below.',
            vless: 'Paste a secure vless:// URL or JSON profile. TLS or REALITY is mandatory.',
            shadowsocks: 'Paste an ss:// URL or JSON profile. Legacy ciphers and plugins are rejected.',
          };
          connectionHint.textContent = imported
            ? (hints[selectedProtocol] || 'Import and validate the provider configuration before activation.')
            : 'This protocol is catalogued for structured preview only; runtime deployment remains blocked.';
        }
        ['externalEgressEndpointHost', 'externalEgressEndpointPort', 'externalEgressTransport'].forEach((id) => {
          const field = document.getElementById(id);
          if (field) field.readOnly = imported;
        });
        document.querySelectorAll('.external-egress-protocol-field').forEach((field) => {
          const visible = String(field.dataset.protocols || '').split(',').includes(selectedProtocol);
          field.hidden = !visible;
          field.querySelectorAll('input, textarea, select').forEach((input) => { input.disabled = !visible; });
        });
        const certificateFields = document.getElementById('externalEgressCertificateFields');
        if (certificateFields) certificateFields.hidden = !certificateFields.querySelector('.external-egress-protocol-field:not([hidden])');
      };
      const invalidatePreview = () => {
        previewedContent = '';
        const target = document.getElementById('externalEgressProfileResult');
        if (target) target.innerHTML = '';
      };
      document.getElementById('externalEgressProtocol')?.addEventListener('change', () => { invalidatePreview(); syncRuntimeSupport(); });
      document.getElementById('externalEgressConfigText')?.addEventListener('input', invalidatePreview);
      document.getElementById('externalEgressConfigFile')?.addEventListener('change', invalidatePreview);
      syncRuntimeSupport();
      document.getElementById('externalEgressCancelBtn')?.addEventListener('click', closeModal);
      document.getElementById('externalEgressPreviewBtn')?.addEventListener('click', async () => {
        const target = document.getElementById('externalEgressProfileResult');
        try {
          const raw = await content();
          if (!raw) throw new Error('Select or paste a provider configuration first.');
          const selectedProtocol = String(document.getElementById('externalEgressProtocol')?.value || protocol);
          const data = await sendJSON('/api/v1/external-egress/import:preview', 'POST', { protocol: selectedProtocol, format: resolveImportFormat(selectedProtocol, raw), content: raw });
          previewedContent = raw;
          const host = document.getElementById('externalEgressEndpointHost');
          const port = document.getElementById('externalEgressEndpointPort');
          const transport = document.getElementById('externalEgressTransport');
          if (host) host.value = data.endpoint_host || '';
          if (port) port.value = data.endpoint_port || '';
          if (transport) transport.value = data.transport || selectedProtocol;
          const requirements = Array.isArray(data.required_secrets) && data.required_secrets.length ? data.required_secrets.join(', ') : 'none';
          const warnings = Array.isArray(data.warnings) && data.warnings.length ? `<span>Warnings: ${escapeHTML(data.warnings.join('; '))}</span>` : '';
          if (target) target.innerHTML = `<div class="callout ok"><strong>Configuration validated</strong><span>${escapeHTML(data.endpoint_host || 'endpoint parsed')}:${escapeHTML(String(data.endpoint_port || ''))} · ${escapeHTML(data.transport || selectedProtocol)} · runtime ${escapeHTML(data.runtime_support)}</span><span>Additional secrets: ${escapeHTML(requirements)}</span>${warnings}</div>`;
        } catch (error) {
          previewedContent = '';
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(error.message)}</span>`;
        }
      });
      document.getElementById('externalEgressProfileForm')?.addEventListener('submit', async (event) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const target = document.getElementById('externalEgressProfileResult');
        try {
          const selectedProtocol = String(form.get('protocol_fixed') || form.get('protocol') || protocol);
          const raw = await content();
          const status = String(form.get('status') || 'draft');
          if (status === 'active' && importedProtocols.has(selectedProtocol) && ((!isEdit && !raw) || (raw && raw !== previewedContent))) {
            throw new Error('Preview the current configuration before creating an active profile.');
          }
          const secrets = {};
          if (raw) secrets.config = raw;
          ['username', 'password', 'uuid', 'preshared_key', 'ca_certificate', 'certificate', 'private_key', 'tls_auth_key', 'tls_crypt_key'].forEach((key) => {
            const value = String(form.get(key) || '');
            if (value.trim()) secrets[key] = value;
          });
          const payload = {
            profile_key: String(form.get('profile_key') || '').trim(), display_name: String(form.get('display_name') || '').trim(),
            description: String(form.get('description') || '').trim(), protocol: selectedProtocol, status,
            endpoint_host: String(form.get('endpoint_host') || '').trim(), endpoint_port: Number(form.get('endpoint_port') || 0),
            transport: String(form.get('transport') || '').trim(),
            import_format: resolveImportFormat(selectedProtocol, raw, profile?.import_format),
            config_json: {}, secrets,
          };
          const data = await sendJSON(isEdit ? `/api/v1/external-egress/profiles/${encodeURIComponent(profile.id)}` : '/api/v1/external-egress/profiles', isEdit ? 'PATCH' : 'POST', payload);
          if (target) target.innerHTML = renderActionResponse(data, 'External egress profile');
          await refresh(); closeModal(); render();
        } catch (error) {
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(error.message)}</span>`;
        }
      });
    }

    function openDeployModal(profile) {
      if (!profile) return;
      const existing = new Set((profile.deployments || []).filter((item) => item.status !== 'deleted').map((item) => item.node_id));
      const l2tpReserved = new Set(
        (state.externalEgressProfiles || [])
          .filter((item) => item.protocol === 'l2tp_ipsec')
          .flatMap((item) => (item.deployments || []).filter((deployment) => deployment.status !== 'deleted').map((deployment) => deployment.node_id)),
      );
      const l2tpServerNodes = new Set(
        (state.instances || [])
          .filter((item) => item.service_code === 'xl2tpd' && item.status !== 'deleted' && item.enabled !== false)
          .map((item) => item.node_id),
      );
      const options = (state.nodes || []).filter((node) => node.status !== 'retired').map((node) => {
        const reserved = existing.has(node.id) || (profile.protocol === 'l2tp_ipsec' && (l2tpReserved.has(node.id) || l2tpServerNodes.has(node.id)));
        return `<option value="${escapeHTML(node.id)}"${reserved ? ' disabled' : ''}>${escapeHTML(node.name || node.id)} · ${escapeHTML(node.role || 'node')}${reserved ? ' · UDP/1701 runtime already reserved' : ''}</option>`;
      }).join('');
      openModal(`Deploy: ${profile.display_name}`, 'Materialize provider tunnel on a runtime node', `
        <form id="externalEgressDeployForm" class="form-grid">
          <div class="field full"><label>Runtime node</label><select name="node_id" required><option value="">Select node</option>${options}</select><div class="field-hint">Deploy on every node that hosts a VLESS instance used by a group assigned to this profile.${profile.protocol === 'l2tp_ipsec' ? ' One managed L2TP/IPsec provider runtime is allowed per node.' : ''}</div></div>
          <div class="field"><label>Routing table</label><input name="routing_table" value="auto" readonly></div>
          <div class="field"><label>Route metric</label><input name="route_metric" type="number" min="1" max="32767" value="100"></div>
          <div class="field full modal-actions"><button class="primary-btn" type="submit">Deploy and apply</button><button class="secondary-btn" id="externalEgressDeployCancel" type="button">Cancel</button></div>
        </form><div id="externalEgressDeployResult" class="form-result"></div>`, { size: 'medium' });
      document.getElementById('externalEgressDeployCancel')?.addEventListener('click', closeModal);
      document.getElementById('externalEgressDeployForm')?.addEventListener('submit', async (event) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const target = document.getElementById('externalEgressDeployResult');
        try {
          const deployment = await sendJSON(`/api/v1/external-egress/profiles/${encodeURIComponent(profile.id)}/deployments`, 'POST', {
            node_id: String(form.get('node_id') || ''), routing_table: 'auto', route_metric: Number(form.get('route_metric') || 100), config_json: {},
          });
          const job = await sendJSON(`/api/v1/external-egress/deployments/${encodeURIComponent(deployment.id)}/apply`, 'POST', {});
          if (target) target.innerHTML = renderActionResponse(job, 'External egress apply');
          await refresh(); closeModal(); render();
        } catch (error) {
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(error.message)}</span>`;
        }
      });
    }

    async function queueDeploymentAction(deploymentID, action) {
      try {
        const job = await sendJSON(`/api/v1/external-egress/deployments/${encodeURIComponent(deploymentID)}/${action}`, 'POST', {});
        await refresh(); render();
        openActionOutcomeModal?.('External egress job queued', 'Provider gateway', 'queued', `${action} was queued for the selected node.`, [
          { label: 'Action', value: action },
          { label: 'Job', value: job?.id || 'queued' },
        ]);
      } catch (error) {
        openActionOutcomeModal?.('External egress action failed', 'Provider gateway', 'failed', error.message || 'The node action could not be queued.');
      }
    }

    function deleteProfile(profile) {
      if (!profile) return;
      openModal(`Delete profile: ${profile.display_name}`, 'Destructive provider gateway operation', `
        <div class="callout danger"><strong>Profile deletion is permanent</strong><span>Active deployments or assigned client groups must be removed first. Encrypted profile secrets are deleted with the profile.</span></div>
        <div class="modal-actions">
          <button class="danger-btn" id="externalEgressDeleteConfirm" type="button">Delete profile</button>
          <button class="secondary-btn" id="externalEgressDeleteCancel" type="button">Cancel</button>
        </div>`, { size: 'medium', variant: 'danger' });
      document.getElementById('externalEgressDeleteCancel')?.addEventListener('click', closeModal);
      document.getElementById('externalEgressDeleteConfirm')?.addEventListener('click', async () => {
        try {
          await sendJSON(`/api/v1/external-egress/profiles/${encodeURIComponent(profile.id)}`, 'DELETE', {});
          await refresh(); closeModal(); render();
          openActionOutcomeModal?.('External egress profile deleted', 'Provider gateway', 'succeeded', `${profile.display_name} was deleted.`);
        } catch (error) {
          openActionOutcomeModal?.('External egress delete blocked', 'Provider gateway', 'failed', error.message || 'The profile could not be deleted.');
        }
      });
    }

    return { render };
  }

  window.MegaVPNExternalEgressPage = { create: createExternalEgressPage, resolveImportFormat };
})(window);
