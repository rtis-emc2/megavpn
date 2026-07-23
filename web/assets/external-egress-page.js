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
          <td><strong>${escapeHTML(profile.display_name || protocolLabel(profile.protocol))}</strong>${profile.description ? `<div class="timeline-meta">${escapeHTML(profile.description)}</div>` : ''}</td>
          <td><strong>${escapeHTML(protocolLabel(profile.protocol))}</strong><div class="timeline-meta">${escapeHTML(profile.transport || 'n/a')}</div></td>
          <td>${statusTag(profile.status || 'unknown')}</td>
          <td><code>${escapeHTML(endpoint(profile))}</code></td>
          <td>${(profile.secret_purposes || []).map((purpose) => `<span class="tag">${escapeHTML(purpose)}</span>`).join(' ') || '<span class="tag">none</span>'}</td>
          <td><div class="external-egress-deployments">${deploymentRows(profile)}</div></td>
          <td>
            <div class="table-actions compact-actions">
              <button class="primary-btn external-egress-deploy-btn" type="button" data-profile-id="${escapeHTML(profile.id)}"${canManage() && profile.status === 'active' && profile.runtime_support === 'ready' ? '' : ' disabled'}>Deploy</button>
              <button class="secondary-btn external-egress-edit-btn" type="button" data-profile-id="${escapeHTML(profile.id)}"${canManage() && profile.runtime_support === 'ready' ? '' : ' disabled'}>Edit</button>
              <button class="danger-btn external-egress-delete-btn" type="button" data-profile-id="${escapeHTML(profile.id)}"${canManage() ? '' : ' disabled'}>Delete</button>
            </div>
          </td>
        </tr>`).join('');
    }

    function render() {
      setTitle('External egress');
      const catalog = Array.isArray(state.externalEgressCatalog) ? state.externalEgressCatalog : [];
      el('content').innerHTML = `
        <section class="section-card external-egress-summary">
          <div class="section-head">
            <div>
              <h2>External provider gateways</h2>
              <p>Import provider client profiles, deploy them on runtime nodes and assign them to client groups. Node traffic and the main routing table are not redirected.</p>
            </div>
            <div class="table-tools">
              <span class="tag ok">${escapeHTML(String(catalog.length))} available protocols</span>
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
          <div class="table-head"><div><h2>Available protocols</h2><p class="table-subtitle">Every protocol shown here can be configured and deployed on a managed node.</p></div></div>
          <div class="external-egress-catalog">${catalog.map((item) => `
            <article><div><strong>${escapeHTML(item.label)}</strong></div><small>${escapeHTML(item.notes || '')}</small></article>`).join('')}</div>
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
      return (state.externalEgressCatalog || []).map((item) => `<option value="${escapeHTML(item.code)}"${item.code === selected ? ' selected' : ''}>${escapeHTML(item.label)}</option>`).join('');
    }

    function profileConfig(profile) {
      if (!profile?.config_json) return {};
      if (typeof profile.config_json === 'object') return profile.config_json;
      try {
        return JSON.parse(profile.config_json);
      } catch (_) {
        return {};
      }
    }

    function importFields(protocol, profile) {
      const fileTypes = protocol === 'openvpn' ? '.ovpn,.conf,.txt' : protocol === 'wireguard' ? '.conf,.txt' : '.txt,.json';
      const placeholders = {
        openvpn: 'Paste the complete .ovpn client profile',
        wireguard: 'Paste the complete WireGuard client configuration',
        vless: 'Paste a secure vless:// URL or JSON client profile',
        shadowsocks: 'Paste an ss:// URL or JSON client profile',
      };
      const existingHint = profile ? '<div class="field-hint">Leave empty to keep the encrypted provider configuration already stored.</div>' : '';
      let credentials = '';
      if (protocol === 'openvpn') {
        credentials = `
          <div class="field"><label>Username <span class="optional">optional</span></label><input name="username" autocomplete="off" placeholder="Used when the profile requests auth-user-pass"></div>
          <div class="field"><label>Password <span class="optional">optional</span></label><input name="password" type="password" autocomplete="new-password" placeholder="${profile ? 'Leave empty to keep current password' : 'Provider password'}"></div>
          <details class="field full"><summary>Certificates and OpenVPN keys</summary><div class="form-grid external-egress-secret-grid">
            <div class="field"><label>CA certificate</label><textarea name="ca_certificate" rows="4" placeholder="PEM"></textarea></div>
            <div class="field"><label>Client certificate</label><textarea name="certificate" rows="4" placeholder="PEM"></textarea></div>
            <div class="field"><label>Private key</label><textarea name="private_key" rows="4" placeholder="PEM"></textarea></div>
            <div class="field"><label>TLS auth key</label><textarea name="tls_auth_key" rows="4" placeholder="OpenVPN static key"></textarea></div>
            <div class="field"><label>TLS crypt key</label><textarea name="tls_crypt_key" rows="4" placeholder="OpenVPN tls-crypt key"></textarea></div>
          </div></details>`;
      } else if (protocol === 'wireguard') {
        credentials = `<div class="field full"><label>Private key <span class="optional">only when omitted from config</span></label><textarea name="private_key" rows="3" placeholder="${profile ? 'Leave empty to keep the current key' : 'WireGuard private key'}"></textarea></div>`;
      } else if (protocol === 'vless') {
        credentials = `<div class="field full"><label>Client UUID <span class="optional">only when omitted from profile</span></label><input name="uuid" autocomplete="off" placeholder="${profile ? 'Leave empty to keep the current UUID' : 'VLESS UUID'}"></div>`;
      } else if (protocol === 'shadowsocks') {
        credentials = `<div class="field full"><label>Password <span class="optional">only when omitted from profile</span></label><input name="password" type="password" autocomplete="new-password" placeholder="${profile ? 'Leave empty to keep the current password' : 'Shadowsocks password'}"></div>`;
      }
      return `
        <div class="field full external-egress-form-heading"><strong>Provider configuration</strong><span>The endpoint and transport are read from the provider profile.</span></div>
        <div class="field full external-egress-import-control">
          <label>Client configuration${profile ? ' replacement' : ''}</label>
          <input id="externalEgressConfigFile" type="file" accept="${fileTypes}">
          <textarea id="externalEgressConfigText" rows="8" placeholder="${escapeHTML(placeholders[protocol] || 'Paste provider configuration')}"></textarea>
          ${existingHint}
        </div>
        ${credentials}`;
    }

    function l2tpFields(profile) {
      const config = profileConfig(profile);
      const purposes = new Set(profile?.secret_purposes || []);
      const authMethod = String(config.auth_method || (purposes.has('certificate') ? 'certificate' : 'psk'));
      return `
        <div class="field full external-egress-form-heading"><strong>L2TP connection</strong><span>PPP credentials and IPsec authentication are configured separately.</span></div>
        <div class="field"><label>Provider server</label><input name="l2tp_server" required value="${escapeHTML(profile?.endpoint_host || '')}" placeholder="vpn.provider.example or 192.0.2.10/32"><div class="field-hint">DNS name, IP address, or a single-host CIDR (/32 or /128).</div></div>
        <div class="field"><label>Remote IPsec identity <span class="optional">optional</span></label><input name="l2tp_remote_id" value="${escapeHTML(config.remote_id || '')}" placeholder="vpn.provider.example"></div>
        <div class="field"><label>PPP login</label><input name="username" ${profile ? '' : 'required '}autocomplete="off" placeholder="${profile ? 'Leave empty to keep current login' : 'Provider login'}"></div>
        <div class="field"><label>PPP password</label><input name="password" type="password" ${profile ? '' : 'required '}autocomplete="new-password" placeholder="${profile ? 'Leave empty to keep current password' : 'Provider password'}"></div>
        <div class="field full external-egress-auth-selector">
          <label>IPsec authentication</label>
          <select name="l2tp_auth_method" id="externalEgressL2TPAuth">
            <option value="psk"${authMethod === 'psk' ? ' selected' : ''}>Pre-shared key (PSK)</option>
            <option value="certificate"${authMethod === 'certificate' ? ' selected' : ''}>Client certificate</option>
          </select>
        </div>
        <div class="field full form-grid external-egress-auth-material" id="externalEgressL2TPAuthMaterial"></div>
        <details class="field full"><summary>Advanced IPsec proposals</summary><div class="form-grid external-egress-secret-grid">
          <div class="field"><label>IKE proposal</label><input name="l2tp_ike" value="${escapeHTML(config.ike_proposal || 'aes256-sha256-modp2048,aes256-sha1-modp2048')}"></div>
          <div class="field"><label>ESP proposal</label><input name="l2tp_esp" value="${escapeHTML(config.esp_proposal || 'aes256-sha256,aes256-sha1,aes128-sha1')}"></div>
        </div></details>`;
    }

    function openProfileEditor(profile) {
      const isEdit = Boolean(profile);
      const initialProtocol = profile?.protocol || state.externalEgressCatalog?.[0]?.code || 'openvpn';
      const existingL2TPConfig = profileConfig(profile);
      const existingL2TPPurposes = new Set(profile?.secret_purposes || []);
      const existingL2TPAuth = String(existingL2TPConfig.auth_method || (existingL2TPPurposes.has('certificate') ? 'certificate' : 'psk'));
      const defaultL2TPIKE = 'aes256-sha256-modp2048,aes256-sha1-modp2048';
      const defaultL2TPESP = 'aes256-sha256,aes256-sha1,aes128-sha1';
      const originalL2TPConnection = {
        server: String(profile?.endpoint_host || '').trim(),
        remote_id: String(existingL2TPConfig.remote_id || '').trim(),
        auth_method: existingL2TPAuth,
        ike_proposal: String(existingL2TPConfig.ike_proposal || defaultL2TPIKE).trim(),
        esp_proposal: String(existingL2TPConfig.esp_proposal || defaultL2TPESP).trim(),
      };
      openModal(isEdit ? `Edit profile: ${profile.display_name}` : 'New external egress profile', 'Provider gateway', `
        <form id="externalEgressProfileForm" class="form-grid external-egress-profile-form">
          <div class="field full external-egress-form-heading"><strong>Profile</strong><span>The internal profile key is generated and managed by the platform.</span></div>
          <div class="field"><label>Name</label><input name="display_name" required value="${escapeHTML(profile?.display_name || '')}" placeholder="Provider USA East"></div>
          <div class="field"><label>Protocol</label><select name="protocol" id="externalEgressProtocol"${isEdit ? ' disabled' : ''}>${protocolOptions(initialProtocol)}</select>${isEdit ? `<input type="hidden" name="protocol_fixed" value="${escapeHTML(initialProtocol)}">` : ''}</div>
          <div class="field"><label>Profile availability</label><select name="status"><option value="active"${profile?.status === 'active' || !profile ? ' selected' : ''}>Enabled</option><option value="draft"${profile?.status === 'draft' ? ' selected' : ''}>Draft</option><option value="disabled"${profile?.status === 'disabled' ? ' selected' : ''}>Disabled</option></select></div>
          <div class="field"><label>Description <span class="optional">optional</span></label><input name="description" value="${escapeHTML(profile?.description || '')}" placeholder="Provider, region, account owner"></div>
          <div class="field full form-grid external-egress-protocol-panel" id="externalEgressProtocolPanel"></div>
          <div class="field full modal-actions">
            <button class="secondary-btn" id="externalEgressValidateBtn" type="button">Validate settings</button>
            <button class="primary-btn" type="submit">${isEdit ? 'Save changes' : 'Create profile'}</button>
            <button class="secondary-btn" id="externalEgressCancelBtn" type="button">Cancel</button>
          </div>
        </form><div id="externalEgressProfileResult" class="form-result"></div>`, { size: 'large' });

      const l2tpDrafts = { psk: {}, certificate: {} };
      const selectedProtocol = () => String(document.getElementById('externalEgressProtocol')?.value || initialProtocol);
      const renderL2TPAuthMaterial = (capture = false) => {
        const mode = String(document.getElementById('externalEgressL2TPAuth')?.value || 'psk');
        const target = document.getElementById('externalEgressL2TPAuthMaterial');
        if (!target) return;
        if (capture) {
          ['preshared_key', 'ca_certificate', 'certificate', 'private_key'].forEach((name) => {
            const input = target.querySelector(`[name="${name}"]`);
            if (input) l2tpDrafts[mode][name] = input.value;
          });
        }
        const required = !isEdit || mode !== existingL2TPAuth ? 'required ' : '';
        if (mode === 'psk') {
          target.innerHTML = `<div class="field full"><label>IPsec pre-shared key</label><input name="preshared_key" type="password" ${required}autocomplete="new-password" value="${escapeHTML(l2tpDrafts.psk.preshared_key || '')}" placeholder="${isEdit && mode === existingL2TPAuth ? 'Leave empty to keep the current PSK' : 'Provider PSK'}"></div>`;
        } else {
          target.innerHTML = `
            <div class="field"><label>CA certificate</label><textarea name="ca_certificate" rows="5" ${required}placeholder="PEM">${escapeHTML(l2tpDrafts.certificate.ca_certificate || '')}</textarea></div>
            <div class="field"><label>Client certificate</label><textarea name="certificate" rows="5" ${required}placeholder="PEM">${escapeHTML(l2tpDrafts.certificate.certificate || '')}</textarea></div>
            <div class="field full"><label>Private key</label><textarea name="private_key" rows="5" ${required}placeholder="RSA or EC PEM">${escapeHTML(l2tpDrafts.certificate.private_key || '')}</textarea></div>`;
        }
      };
      const bindL2TPAuth = () => {
        let previousMode = String(document.getElementById('externalEgressL2TPAuth')?.value || 'psk');
        renderL2TPAuthMaterial();
        document.getElementById('externalEgressL2TPAuth')?.addEventListener('change', (event) => {
          const nextMode = String(event.currentTarget.value || 'psk');
          event.currentTarget.value = previousMode;
          renderL2TPAuthMaterial(true);
          event.currentTarget.value = nextMode;
          previousMode = nextMode;
          renderL2TPAuthMaterial();
        });
      };
      const renderProtocolPanel = () => {
        const protocol = selectedProtocol();
        const panel = document.getElementById('externalEgressProtocolPanel');
        if (!panel) return;
        panel.innerHTML = protocol === 'l2tp_ipsec' ? l2tpFields(profile) : importFields(protocol, profile);
        if (protocol === 'l2tp_ipsec') bindL2TPAuth();
        const result = document.getElementById('externalEgressProfileResult');
        if (result) result.innerHTML = '';
      };
      const providerContent = async (form, force = false) => {
        if (selectedProtocol() === 'l2tp_ipsec') {
          const connection = {
            server: String(form.get('l2tp_server') || '').trim(),
            remote_id: String(form.get('l2tp_remote_id') || '').trim(),
            auth_method: String(form.get('l2tp_auth_method') || 'psk'),
            ike_proposal: String(form.get('l2tp_ike') || '').trim(),
            esp_proposal: String(form.get('l2tp_esp') || '').trim(),
          };
          if (isEdit && !force && JSON.stringify(connection) === JSON.stringify(originalL2TPConnection)) return '';
          return JSON.stringify(connection);
        }
        const file = document.getElementById('externalEgressConfigFile')?.files?.[0];
        if (file) return file.text();
        return String(document.getElementById('externalEgressConfigText')?.value || '').trim();
      };
      const validateSettings = async (form, requireContent, forceContent = false) => {
        const raw = await providerContent(form, forceContent);
        if (!raw || (selectedProtocol() !== 'l2tp_ipsec' && !raw.trim())) {
          if (requireContent) throw new Error('Select a provider configuration file or paste its contents.');
          return { raw: '', preview: null };
        }
        const preview = await sendJSON('/api/v1/external-egress/import:preview', 'POST', {
          protocol: selectedProtocol(), format: resolveImportFormat(selectedProtocol(), raw, profile?.import_format), content: raw,
        });
        return { raw, preview };
      };

      document.getElementById('externalEgressProtocol')?.addEventListener('change', renderProtocolPanel);
      document.getElementById('externalEgressCancelBtn')?.addEventListener('click', closeModal);
      document.getElementById('externalEgressValidateBtn')?.addEventListener('click', async () => {
        const target = document.getElementById('externalEgressProfileResult');
        try {
          const result = await validateSettings(new FormData(document.getElementById('externalEgressProfileForm')), true, true);
          const warnings = result.preview?.warnings?.length ? `<span>${escapeHTML(result.preview.warnings.join('; '))}</span>` : '';
          if (target) target.innerHTML = `<div class="callout ok"><strong>Settings are valid</strong><span>${escapeHTML(result.preview.endpoint_host)}:${escapeHTML(String(result.preview.endpoint_port || ''))} · ${escapeHTML(result.preview.transport || selectedProtocol())}</span>${warnings}</div>`;
        } catch (error) {
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(error.message)}</span>`;
        }
      });
      document.getElementById('externalEgressProfileForm')?.addEventListener('submit', async (event) => {
        event.preventDefault();
        const form = new FormData(event.currentTarget);
        const target = document.getElementById('externalEgressProfileResult');
        try {
          const protocol = selectedProtocol();
          const { raw, preview } = await validateSettings(form, !isEdit);
          const secrets = {};
          if (raw) secrets.config = raw;
          ['username', 'password', 'uuid', 'preshared_key', 'ca_certificate', 'certificate', 'private_key', 'tls_auth_key', 'tls_crypt_key'].forEach((key) => {
            const value = String(form.get(key) || '');
            if (value.trim()) secrets[key] = value;
          });
          const l2tpConfig = protocol === 'l2tp_ipsec' ? {
            auth_method: String(form.get('l2tp_auth_method') || 'psk'),
            remote_id: String(form.get('l2tp_remote_id') || '').trim(),
            ike_proposal: String(form.get('l2tp_ike') || '').trim(),
            esp_proposal: String(form.get('l2tp_esp') || '').trim(),
          } : {};
          const payload = {
            display_name: String(form.get('display_name') || '').trim(),
            description: String(form.get('description') || '').trim(),
            protocol,
            status: String(form.get('status') || 'active'),
            endpoint_host: preview?.endpoint_host || profile?.endpoint_host || '',
            endpoint_port: Number(preview?.endpoint_port || profile?.endpoint_port || 0),
            transport: preview?.transport || profile?.transport || '',
            import_format: resolveImportFormat(protocol, raw, profile?.import_format),
            config_json: protocol === 'l2tp_ipsec' && !raw ? profileConfig(profile) : l2tpConfig,
            secrets,
          };
          const data = await sendJSON(isEdit ? `/api/v1/external-egress/profiles/${encodeURIComponent(profile.id)}` : '/api/v1/external-egress/profiles', isEdit ? 'PATCH' : 'POST', payload);
          if (target) target.innerHTML = renderActionResponse(data, 'External egress profile');
          await refresh();
          closeModal();
          render();
        } catch (error) {
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(error.message)}</span>`;
        }
      });
      renderProtocolPanel();
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
