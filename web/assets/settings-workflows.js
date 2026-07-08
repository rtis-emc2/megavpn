(function (window) {
  'use strict';

  function createSettingsWorkflows(ctx = {}) {
    const {
      state,
      domainUI,
      requestJSON,
      sendJSON,
      refresh,
      render,
      setPage,
      openModal,
      closeModal,
      renderActionResponse,
      watchJob,
      updateReadyPill,
      statusTag,
      escapeHTML,
      arrayOrEmpty,
      parseCSVList,
      formatDate,
      renderInventoryFact,
      hasPermission,
      hasRole,
      platformPublicBaseURL,
      publicURLHostname,
      publicURLPort,
    } = ctx;

    if (
      !state ||
      !domainUI ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof render !== 'function' ||
      typeof setPage !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof watchJob !== 'function' ||
      typeof updateReadyPill !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof arrayOrEmpty !== 'function' ||
      typeof parseCSVList !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof renderInventoryFact !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof hasRole !== 'function' ||
      typeof platformPublicBaseURL !== 'function' ||
      typeof publicURLHostname !== 'function' ||
      typeof publicURLPort !== 'function'
    ) {
      throw new Error('MegaVPNSettingsWorkflows requires workflow dependencies');
    }

    const {
      certificateDisplayStatus,
      certificateExpiryCaption,
      certificateOptions,
    } = domainUI;

    function compactID(value) {
      const text = String(value || '').trim();
      if (!text) return 'n/a';
      if (text.length <= 16) return text;
      return `${text.slice(0, 8)}...${text.slice(-6)}`;
    }

    async function loadAdminSettings(canManageAuth, canDeleteUsers = hasRole('superadmin')) {
      try {
        const canManageSettings = hasPermission('settings.manage');
        const canReadFirewallSafety = hasPermission('firewall.read') || hasPermission('firewall.manage');
        const canManageFirewallSafety = hasPermission('firewall.manage');
        const userList = canManageAuth ? await requestJSON('/api/v1/admin/users') : [{ ...state.authUser, roles: state.authRoles || [] }];
        const sessions = canManageAuth ? await requestJSON('/api/v1/admin/sessions') : [];
        const mailSettings = canManageAuth ? await requestJSON('/api/v1/settings/mail') : null;
        const controlPlaneTLSSettings = canManageSettings ? await requestJSON('/api/v1/settings/control-plane-tls') : null;
        const firewallManagementSettings = canReadFirewallSafety ? await requestJSON('/api/v1/firewall/management-settings') : null;
        const runtimePreflight = canManageSettings ? await requestJSON('/api/v1/runtime/preflight') : null;
        const pkiRoots = hasPermission('instance.read') ? await requestJSON('/api/v1/platform/pki-roots') : [];
        const invites = canManageAuth ? await requestJSON('/api/v1/admin/user-invites') : [];
        if (state.page !== 'settings') return;
        state.mailSettings = mailSettings;
        state.controlPlaneTLSSettings = controlPlaneTLSSettings;
        state.firewallManagementSettings = firewallManagementSettings;
        state.runtimePreflight = runtimePreflight;
        state.platformInvites = invites || [];
        state.platformPKIRoots = pkiRoots || [];
        renderRuntimePreflight(runtimePreflight, canManageSettings);
        renderControlPlaneTLSSettings(controlPlaneTLSSettings, canManageSettings);
        renderFirewallManagementSettings(firewallManagementSettings, canReadFirewallSafety, canManageFirewallSafety);
        renderPlatformUsers(userList || [], canManageAuth, canDeleteUsers);
        renderMailSettings(mailSettings, canManageAuth);
        renderPlatformPKIRoots(pkiRoots || [], hasPermission('instance.read'));
        renderPlatformInvites(invites || [], canManageAuth);
        renderPlatformSessions(sessions || [], canManageAuth);
      } catch (err) {
        if (state.page !== 'settings') return;
        document.getElementById('platformUsersMount').innerHTML = `<div class="empty">Failed to load admin data: ${escapeHTML(err.message)}</div>`;
        document.getElementById('runtimePreflightMount').innerHTML = `<div class="empty">Failed to load runtime preflight: ${escapeHTML(err.message)}</div>`;
        document.getElementById('mailSettingsMount').innerHTML = `<div class="empty">Failed to load mail settings: ${escapeHTML(err.message)}</div>`;
        document.getElementById('controlPlaneTLSMount').innerHTML = `<div class="empty">Failed to load TLS settings: ${escapeHTML(err.message)}</div>`;
        document.getElementById('firewallManagementSettingsMount').innerHTML = `<div class="empty">Failed to load firewall safety settings: ${escapeHTML(err.message)}</div>`;
        document.getElementById('platformPKIRootsMount').innerHTML = `<div class="empty">Failed to load CA center inventory: ${escapeHTML(err.message)}</div>`;
        document.getElementById('platformInvitesMount').innerHTML = `<div class="empty">Failed to load invites: ${escapeHTML(err.message)}</div>`;
        document.getElementById('platformSessionsMount').innerHTML = `<div class="empty">Failed to load sessions: ${escapeHTML(err.message)}</div>`;
      }
    }

    function renderPlatformPKIRoots(roots, canRead) {
      const mount = document.getElementById('platformPKIRootsMount');
      if (!mount) return;
      if (!canRead) {
        mount.innerHTML = '<div class="empty">Platform CA Center requires instance.read permission.</div>';
        return;
      }
      const list = Array.isArray(roots) ? roots : [];
      const rows = list.length
        ? list.map((root) => `
          <tr>
            <td>${escapeHTML(root.service_code || 'n/a')}</td>
            <td>${escapeHTML(root.pki_profile || 'default')}</td>
            <td>${statusTag(certificateDisplayStatus(root))}</td>
            <td>${escapeHTML(root.common_name || 'n/a')}</td>
            <td><code title="${escapeHTML(root.ca_cert_secret_ref_id || '')}">${escapeHTML(compactID(root.ca_cert_secret_ref_id))}</code></td>
            <td>${escapeHTML(certificateExpiryCaption(root))}</td>
            <td>${formatDate(root.created_at)}</td>
            <td>${formatDate(root.rotated_at)}</td>
          </tr>`)
          .join('')
        : '<tr><td colspan="8"><div class="empty">No platform CA roots yet. The OpenVPN default CA is created on first OpenVPN apply or client provisioning.</div></td></tr>';
      mount.innerHTML = `
        <div class="metric-caption" style="margin-bottom:12px">Control plane хранит platform PKI inventory для сервисов, где CA lifecycle должен быть централизован. Сейчас production path активен для OpenVPN.</div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Service</th><th>Profile</th><th>Status</th><th>Common Name</th><th>CA Cert Ref</th><th>Expires</th><th>Created</th><th>Rotated</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>`;
    }

    function renderPlatformUsers(users, canManageAuth, canDeleteUsers) {
      const mount = document.getElementById('platformUsersMount');
      const rows = users.length
        ? users.map((user) => {
          const isCurrentUser = user.id === state.authUser?.id;
          const actions = !canManageAuth
            ? statusTag(user.status || 'active')
            : `
              <div class="inline-actions compact-actions">
                ${user.status === 'active'
                  ? `<button class="danger-btn user-status-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-status="disabled">Disable</button>`
                  : `<button class="secondary-btn user-status-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-status="active">Activate</button>`}
                <button class="secondary-btn resend-invite-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-username="${escapeHTML(user.username || 'operator')}">Resend invite</button>
                <button class="secondary-btn reset-password-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-username="${escapeHTML(user.username || 'operator')}">Reset password</button>
                ${canDeleteUsers && !isCurrentUser
                  ? `<button class="danger-btn delete-user-btn" type="button" data-user-id="${escapeHTML(user.id)}" data-username="${escapeHTML(user.username || 'operator')}">Delete user</button>`
                  : ''}
              </div>`;
          return `
            <tr>
              <td>${escapeHTML(user.username || 'n/a')}${isCurrentUser ? ' <span class="tag ok">you</span>' : ''}</td>
              <td>${escapeHTML(user.display_name || 'n/a')}</td>
              <td>${escapeHTML(user.email || 'n/a')}</td>
              <td>${statusTag(user.status || 'active')}</td>
              <td>${escapeHTML((user.roles || []).join(', ') || 'none')}</td>
              <td>${formatDate(user.last_login_at)}</td>
              <td>${actions}</td>
            </tr>`;
        }).join('')
        : '<tr><td colspan="7"><div class="empty">No platform users found.</div></td></tr>';
      mount.innerHTML = `
        ${canManageAuth ? `
          <form id="createOperatorForm" class="form-grid operator-form" style="margin-bottom:18px">
            <div class="field"><label>Username</label><input name="username" required placeholder="operator-01" /></div>
            <div class="field"><label>Display name</label><input name="display_name" placeholder="Operator 01" /></div>
            <div class="field"><label>Email</label><input name="email" type="email" placeholder="operator-01@rtis.local" /></div>
            <div class="field"><label>Invite TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="48" /></div>
            <div class="field full"><label>Roles</label><select name="role_codes" multiple size="4"><option value="superadmin">superadmin</option><option value="admin" selected>admin</option><option value="engineer">engineer</option><option value="readonly">readonly</option></select></div>
            <div class="field full inline-actions"><button class="primary-btn" type="submit">Invite operator</button></div>
          </form>
          <div id="createOperatorResult" class="form-result"></div>
        ` : ''}
        <div id="platformUsersResult" class="form-result"></div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Username</th><th>Display</th><th>Email</th><th>Status</th><th>Roles</th><th>Last Login</th><th>Actions</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>`;
      if (canManageAuth) {
        document.getElementById('createOperatorForm').addEventListener('submit', createOperator);
        mount.querySelectorAll('.user-status-btn').forEach((button) => {
          button.addEventListener('click', () => setPlatformUserStatus(button.dataset.userId, button.dataset.status));
        });
        mount.querySelectorAll('.resend-invite-btn').forEach((button) => {
          button.addEventListener('click', () => openResendInviteModal(button.dataset.userId, button.dataset.username));
        });
        mount.querySelectorAll('.reset-password-btn').forEach((button) => {
          button.addEventListener('click', () => openResetPasswordModal(button.dataset.userId, button.dataset.username));
        });
        mount.querySelectorAll('.delete-user-btn').forEach((button) => {
          button.addEventListener('click', () => openDeleteUserModal(button.dataset.userId, button.dataset.username));
        });
      }
    }

    function renderPlatformSessions(sessions, canManageAuth) {
      const mount = document.getElementById('platformSessionsMount');
      const rows = sessions.length
        ? sessions.map((session) => `
            <tr>
              <td>${escapeHTML(session.username || 'n/a')}${session.id === state.authSession?.id ? ' <span class="tag ok">current</span>' : ''}</td>
              <td>${escapeHTML(session.ip || 'n/a')}</td>
              <td>${escapeHTML(session.user_agent || 'n/a')}</td>
              <td>${formatDate(session.created_at)}</td>
              <td>${formatDate(session.expires_at)}</td>
              <td>${canManageAuth ? `<button class="danger-btn revoke-session-btn" type="button" data-session-id="${escapeHTML(session.id)}">Revoke</button>` : statusTag('active')}</td>
            </tr>`).join('')
        : '<tr><td colspan="6"><div class="empty">No active sessions.</div></td></tr>';
      mount.innerHTML = `
        <div id="revokeSessionResult" class="form-result"></div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Username</th><th>IP</th><th>User Agent</th><th>Created</th><th>Expires</th><th>Action</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>`;
      if (canManageAuth) {
        document.querySelectorAll('.revoke-session-btn').forEach((button) => {
          button.addEventListener('click', () => revokePlatformSession(button.dataset.sessionId));
        });
      }
    }

    function renderRuntimePreflight(report, canManageSettings) {
      const mount = document.getElementById('runtimePreflightMount');
      if (!mount) return;
      if (!canManageSettings) {
        mount.innerHTML = '<div class="empty">Runtime preflight requires settings.manage permission.</div>';
        return;
      }
      const checks = arrayOrEmpty(report?.checks);
      const status = report?.status || 'unknown';
      const failed = checks.filter((check) => check.status === 'failed').length;
      const warnings = checks.filter((check) => check.status === 'warning').length;
      mount.innerHTML = `
        <div class="inventory-facts" style="margin-bottom:16px">
          <div class="fact-card">
            <div class="mini-label">Overall status</div>
            <div class="metric-caption strong">${statusTag(status)}</div>
            <div class="metric-caption">Production readiness gate</div>
          </div>
          ${renderInventoryFact('Version', report?.version || state.versionInfo?.version || 'unknown', 'API build')}
          ${renderInventoryFact('Generated', formatDate(report?.generated_at), `${failed} failed · ${warnings} warnings`)}
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>Check</th><th>Status</th><th>Summary</th><th>Detail</th></tr></thead>
            <tbody>
              ${checks.length ? checks.map((check) => `
                <tr>
                  <td><code>${escapeHTML(check.code || 'unknown')}</code></td>
                  <td>${statusTag(check.status || 'unknown')}</td>
                  <td>${escapeHTML(check.summary || 'n/a')}</td>
                  <td>${escapeHTML(check.detail || '')}</td>
                </tr>`).join('') : '<tr><td colspan="4"><div class="empty">No preflight checks returned.</div></td></tr>'}
            </tbody>
          </table>
        </div>`;
    }

    function renderControlPlaneTLSSettings(settings, canManageSettings) {
      const mount = document.getElementById('controlPlaneTLSMount');
      if (!mount) return;
      const current = settings || {};
      const certID = current.certificate_id || '';
      const selectedCert = certID ? (state.platformCertificates || []).find((item) => item.id === certID) : null;
      const mode = current.mode || 'managed_certificate';
      mount.innerHTML = `
        ${canManageSettings ? `
          <form id="controlPlaneTLSForm" class="form-grid operator-form" style="margin-bottom:18px">
            <div class="field"><label>Enabled</label><select name="enabled"><option value="true"${current.enabled !== false ? ' selected' : ''}>true</option><option value="false"${current.enabled === false ? ' selected' : ''}>false</option></select></div>
            <div class="field"><label>Mode</label><select name="mode"><option value="managed_certificate"${mode !== 'self_signed_fallback' ? ' selected' : ''}>managed certificate</option><option value="self_signed_fallback"${mode === 'self_signed_fallback' ? ' selected' : ''}>self-signed fallback</option></select></div>
            <div class="field"><label>Public HTTPS URL</label><input name="public_base_url" value="${escapeHTML(current.public_base_url || platformPublicBaseURL() || 'https://control.example.com:58765')}" placeholder="https://control.example.com:58765" /></div>
            <div class="field"><label>Server name</label><input name="server_name" value="${escapeHTML(current.server_name || publicURLHostname(platformPublicBaseURL()) || '')}" placeholder="control.example.com" /></div>
            <div class="field"><label>Listen port</label><input name="listen_port" type="number" min="1" max="65535" value="${escapeHTML(String(current.listen_port || publicURLPort(platformPublicBaseURL()) || 443))}" /></div>
            <div class="field"><label>Upstream</label><input name="upstream_url" value="${escapeHTML(current.upstream_url || 'http://127.0.0.1:8080')}" /></div>
            <div class="field full"><label>Managed certificate</label><select name="certificate_id">${certificateOptions(certID, true)}</select></div>
            <div class="field"><label>Self-signed CN</label><input name="self_signed_common_name" value="${escapeHTML(current.self_signed_common_name || current.server_name || publicURLHostname(platformPublicBaseURL()) || '')}" /></div>
            <div class="field"><label>Self-signed SAN</label><input name="self_signed_dns_names" value="${escapeHTML(Array.isArray(current.self_signed_dns_names) ? current.self_signed_dns_names.join(', ') : '')}" placeholder="control.example.com" /></div>
            <div class="field full inline-actions">
              <button class="primary-btn" type="submit">Save TLS profile</button>
              <button class="secondary-btn" id="applyControlPlaneTLSBtn" type="button">Apply edge</button>
              <button class="secondary-btn" id="openCertificateManagerBtn" type="button">Open certificates</button>
            </div>
          </form>
          <div id="controlPlaneTLSResult" class="form-result"></div>
        ` : `<div class="empty">Control Plane TLS settings require settings.manage.</div>`}
        <div class="grid cols-3">
          <div class="card"><div class="mini-label">Public edge</div><div class="metric-caption strong">${escapeHTML(current.public_base_url || platformPublicBaseURL() || 'missing')}</div><div class="metric-caption">Agents use this exact HTTPS URL.</div></div>
          <div class="card"><div class="mini-label">TLS mode</div><div class="metric-caption">${statusTag(current.enabled === false ? 'disabled' : 'enabled')}</div><div class="metric-caption">${escapeHTML(mode)}</div></div>
          <div class="card"><div class="mini-label">Certificate</div><div class="metric-caption strong">${escapeHTML(selectedCert?.name || selectedCert?.common_name || (certID ? certID : 'not selected'))}</div><div class="metric-caption">${escapeHTML(selectedCert ? `${selectedCert.source || 'certificate'} · ${certificateExpiryCaption(selectedCert)}` : 'Use commercial import or self-signed fallback.')}</div></div>
          <div class="card"><div class="mini-label">Upstream boundary</div><div class="metric-caption">${escapeHTML(current.upstream_url || 'http://127.0.0.1:8080')}</div><div class="metric-caption">Loopback only behind TLS edge.</div></div>
          <div class="card"><div class="mini-label">Last apply</div><div class="metric-caption">${formatDate(current.last_applied_at)}</div></div>
          <div class="card"><div class="mini-label">Last error</div><div class="metric-caption">${escapeHTML(current.last_error || 'none')}</div></div>
        </div>`;
      if (canManageSettings) {
        document.getElementById('controlPlaneTLSForm').addEventListener('submit', saveControlPlaneTLSSettings);
        document.getElementById('applyControlPlaneTLSBtn').addEventListener('click', applyControlPlaneTLSSettings);
        document.getElementById('openCertificateManagerBtn').addEventListener('click', () => setPage('certificates'));
      }
    }

    function firewallCIDRText(values) {
      return Array.isArray(values) ? values.join('\n') : '';
    }

    function parseCIDRTextarea(value) {
      return String(value || '')
        .split(/[\n,;\t ]+/)
        .map((item) => item.trim())
        .filter(Boolean);
    }

    function renderFirewallManagementSettings(settings, canReadFirewallSafety, canManageFirewallSafety) {
      const mount = document.getElementById('firewallManagementSettingsMount');
      if (!mount) return;
      if (!canReadFirewallSafety) {
        mount.innerHTML = '<div class="empty">Firewall safety settings require firewall.read permission.</div>';
        return;
      }
      const current = settings || {};
      const cpCIDRs = Array.isArray(current.control_plane_source_cidrs) ? current.control_plane_source_cidrs : [];
      const sshCIDRs = Array.isArray(current.ssh_bootstrap_source_cidrs) ? current.ssh_bootstrap_source_cidrs : [];
      const operatorCIDRs = Array.isArray(current.trusted_operator_cidrs) ? current.trusted_operator_cidrs : [];
      mount.innerHTML = `
        ${canManageFirewallSafety ? `
          <form id="firewallManagementSettingsForm" class="form-grid operator-form" style="margin-bottom:18px">
            <div class="field full">
              <label>Control-plane source CIDRs</label>
              <textarea name="control_plane_source_cidrs" rows="4" placeholder="203.0.113.10/32">${escapeHTML(firewallCIDRText(cpCIDRs))}</textarea>
              <div class="field-hint">Sources allowed to preserve agent/control-plane management traffic during strict firewall apply.</div>
            </div>
            <div class="field full">
              <label>SSH bootstrap source CIDRs</label>
              <textarea name="ssh_bootstrap_source_cidrs" rows="4" placeholder="203.0.113.11/32">${escapeHTML(firewallCIDRText(sshCIDRs))}</textarea>
              <div class="field-hint">Additional control-plane-side sources allowed to preserve SSH bootstrap and agent reinstall paths.</div>
            </div>
            <div class="field full">
              <label>Trusted operator CIDRs</label>
              <textarea name="trusted_operator_cidrs" rows="4" placeholder="198.51.100.0/24">${escapeHTML(firewallCIDRText(operatorCIDRs))}</textarea>
              <div class="field-hint">Operator networks for privileged node access. DNS names and 0.0.0.0/0 are rejected for automatic SSH safety.</div>
            </div>
            <div class="field full inline-actions">
              <button class="primary-btn apply-btn" type="submit">Save firewall sources</button>
            </div>
          </form>
          <div id="firewallManagementSettingsResult" class="form-result"></div>
        ` : `<div class="empty">Editing firewall safety settings requires firewall.manage permission.</div>`}
        <div class="inventory-facts">
          ${renderInventoryFact('Control-plane CIDRs', String(cpCIDRs.length), 'trusted_control_plane managed entries')}
          ${renderInventoryFact('SSH bootstrap CIDRs', String(sshCIDRs.length), 'additional trusted_control_plane entries')}
          ${renderInventoryFact('Operator CIDRs', String(operatorCIDRs.length), 'trusted_operators managed entries')}
          <div class="fact-card">
            <div class="mini-label">Strict safety</div>
            <div class="metric-caption strong">${statusTag(cpCIDRs.length + sshCIDRs.length + operatorCIDRs.length > 0 ? 'configured' : 'missing')}</div>
            <div class="metric-caption">Strict input apply is blocked until at least one valid management CIDR exists.</div>
          </div>
        </div>`;
      if (canManageFirewallSafety) {
        document.getElementById('firewallManagementSettingsForm')?.addEventListener('submit', saveFirewallManagementSettings);
      }
    }

    function renderMailSettings(settings, canManageAuth) {
      const mount = document.getElementById('mailSettingsMount');
      if (!mount) return;
      const current = settings || {};
      const passwordState = current.smtp_password_configured ? 'configured' : 'missing';
      const passwordStateDetail = current.smtp_password_configured
        ? `Secret ref is stored${current.smtp_password_secret_ref_id ? ` · ${current.smtp_password_secret_ref_id}` : ''}. Leave the field empty to keep it.`
        : 'No SMTP password is stored yet. Save one to enable authenticated delivery.';
      mount.innerHTML = `
        ${canManageAuth ? `
          <form id="mailSettingsForm" class="form-grid operator-form" style="margin-bottom:18px">
            <div class="field"><label>Enabled</label><select name="enabled"><option value="true"${current.enabled ? ' selected' : ''}>true</option><option value="false"${!current.enabled ? ' selected' : ''}>false</option></select></div>
            <div class="field"><label>SMTP host</label><input name="smtp_host" value="${escapeHTML(current.smtp_host || '')}" placeholder="smtp.example.com" /></div>
            <div class="field"><label>SMTP port</label><input name="smtp_port" type="number" min="1" max="65535" value="${escapeHTML(String(current.smtp_port || 587))}" /></div>
            <div class="field"><label>SMTP username</label><input name="smtp_username" value="${escapeHTML(current.smtp_username || '')}" /></div>
            <div class="field"><label>Auth mode</label><select name="smtp_auth_mode"><option value="plain"${current.smtp_auth_mode === 'plain' ? ' selected' : ''}>plain</option><option value="none"${current.smtp_auth_mode === 'none' ? ' selected' : ''}>none</option></select></div>
            <div class="field"><label>TLS mode</label><select name="smtp_tls_mode"><option value="starttls"${current.smtp_tls_mode === 'starttls' ? ' selected' : ''}>starttls</option><option value="starttls_required"${current.smtp_tls_mode === 'starttls_required' ? ' selected' : ''}>starttls_required</option><option value="none"${current.smtp_tls_mode === 'none' ? ' selected' : ''}>none</option></select></div>
            <div class="field"><label>From email</label><input name="from_email" type="email" value="${escapeHTML(current.from_email || '')}" /></div>
            <div class="field"><label>From name</label><input name="from_name" value="${escapeHTML(current.from_name || '')}" /></div>
            <div class="field"><label>Reply-to</label><input name="reply_to_email" type="email" value="${escapeHTML(current.reply_to_email || '')}" /></div>
            <div class="field"><label>Invite URL base</label><input name="invite_url_base" value="${escapeHTML(current.invite_url_base || '')}" placeholder="${escapeHTML(state.apiBase || window.location.origin)}" /></div>
            <div class="field full"><label>SMTP password</label><textarea name="smtp_password" rows="3" placeholder="${current.smtp_password_secret_ref_id ? 'Leave empty to keep existing secret ref.' : 'Paste SMTP password or app token.'}"></textarea></div>
            <div class="field full">
              <div class="inline-actions">
                ${statusTag(passwordState)}
                <span class="metric-caption">${escapeHTML(passwordStateDetail)}</span>
              </div>
            </div>
            <div class="field full inline-actions">
              <button class="primary-btn" type="submit">Save mail settings</button>
              <input id="mailTestEmail" type="email" placeholder="test@example.com" style="max-width:280px" />
              <button class="secondary-btn" id="sendMailTestBtn" type="button">Send test email</button>
            </div>
          </form>
          <div id="mailSettingsResult" class="form-result"></div>
        ` : `<div class="empty">Mail settings are visible only with auth.manage.</div>`}
        <div class="grid cols-3">
          <div class="card"><div class="mini-label">Status</div><div class="metric-caption">${statusTag(current.enabled ? 'enabled' : 'disabled')}</div></div>
          <div class="card"><div class="mini-label">SMTP Password</div><div class="metric-caption">${statusTag(passwordState)}</div><div class="metric-caption">${escapeHTML(current.smtp_password_configured ? 'Stored in secret storage.' : 'Not stored.')}</div></div>
          <div class="card"><div class="mini-label">Last test</div><div class="metric-caption">${formatDate(current.last_test_at)}</div></div>
          <div class="card"><div class="mini-label">Last error</div><div class="metric-caption">${escapeHTML(current.last_error || 'none')}</div></div>
        </div>`;
      if (canManageAuth) {
        document.getElementById('mailSettingsForm').addEventListener('submit', saveMailSettings);
        document.getElementById('sendMailTestBtn').addEventListener('click', sendMailTest);
      }
    }

    function renderPlatformInvites(invites) {
      const mount = document.getElementById('platformInvitesMount');
      if (!mount) return;
      const rows = invites.length
        ? invites.map((invite) => `
            <tr>
              <td>${escapeHTML(invite.username || 'n/a')}</td>
              <td>${escapeHTML(invite.email || 'n/a')}</td>
              <td>${statusTag(invite.status || 'pending')}</td>
              <td>${formatDate(invite.expires_at)}</td>
              <td>${formatDate(invite.sent_at)}</td>
              <td>${escapeHTML(invite.delivery_error || 'n/a')}</td>
            </tr>`).join('')
        : '<tr><td colspan="6"><div class="empty">No operator invites yet.</div></td></tr>';
      mount.innerHTML = `
        <div class="table-wrap">
          <table>
            <thead><tr><th>Username</th><th>Email</th><th>Status</th><th>Expires</th><th>Sent</th><th>Delivery</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>`;
    }

    async function createOperator(event) {
      event.preventDefault();
      const target = document.getElementById('createOperatorResult');
      target.innerHTML = '<span class="tag warn">sending invitation</span>';
      try {
        const form = new FormData(event.currentTarget);
        const roleSelect = event.currentTarget.querySelector('[name="role_codes"]');
        const roleCodes = Array.from(roleSelect.selectedOptions).map((option) => option.value);
        const data = await sendJSON('/api/v1/admin/users/invite', 'POST', {
          username: String(form.get('username') || '').trim(),
          display_name: String(form.get('display_name') || '').trim(),
          email: String(form.get('email') || '').trim(),
          role_codes: roleCodes,
          ttl_hours: Number(form.get('ttl_hours') || 48),
        });
        target.innerHTML = renderActionResponse(data, 'Operator invite');
        await loadAdminSettings(true);
        event.currentTarget.reset();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function changeOwnPassword(event) {
      event.preventDefault();
      const target = document.getElementById('changePasswordResult');
      target.innerHTML = '<span class="tag warn">updating</span>';
      try {
        const form = new FormData(event.currentTarget);
        const data = await sendJSON('/api/v1/auth/change-password', 'POST', {
          current_password: String(form.get('current_password') || ''),
          new_password: String(form.get('new_password') || ''),
        });
        target.innerHTML = '<span class="tag ok">password updated, re-login required</span>';
        if (data?.relogin_required) {
          state.authUser = null;
          state.authSession = null;
          state.authRoles = [];
          state.authPermissions = [];
          setTimeout(() => render(), 200);
        }
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function setPlatformUserStatus(userID, status) {
      const target = document.getElementById('platformUsersResult');
      if (target) target.innerHTML = '<span class="tag warn">updating operator status</span>';
      try {
        await sendJSON(`/api/v1/admin/users/${userID}/status`, 'POST', { status });
        if (target) target.innerHTML = '<span class="tag ok">operator status updated</span>';
        await loadAdminSettings(true);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openResetPasswordModal(userID, username) {
      openModal(`Reset password: ${username}`, 'Platform user password reset', `
        <form id="resetPasswordForm" class="form-grid">
          <div class="field full"><label>New password for ${escapeHTML(username)}</label><input name="password" type="password" required placeholder="minimum 12 chars" /></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Reset password</button></div>
        </form>
        <div id="resetPasswordResult" class="form-result"></div>`);
      document.getElementById('resetPasswordForm').addEventListener('submit', (event) => submitPlatformUserPasswordReset(event, userID));
    }

    function openResendInviteModal(userID, username) {
      openModal(`Resend invite: ${username}`, 'Operator invitation delivery', `
        <form id="resendInviteForm" class="form-grid">
          <div class="field full"><label>Invite TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="48" /></div>
          <div class="field full"><label>Delivery note</label><div class="code-block">A fresh one-time invite link will be generated and sent to the operator email address stored in the platform user record.</div></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Resend invite</button></div>
        </form>
        <div id="resendInviteResult" class="form-result"></div>`);
      document.getElementById('resendInviteForm').addEventListener('submit', (event) => submitResendInvite(event, userID));
    }

    async function submitResendInvite(event, userID) {
      event.preventDefault();
      const target = document.getElementById('resendInviteResult');
      target.innerHTML = '<span class="tag warn">sending invite</span>';
      try {
        const form = new FormData(event.currentTarget);
        const data = await sendJSON(`/api/v1/admin/users/${userID}/resend-invite`, 'POST', {
          ttl_hours: Number(form.get('ttl_hours') || 48),
        });
        target.innerHTML = renderActionResponse(data, 'Operator invite resent');
        await loadAdminSettings(true);
        setTimeout(closeModal, 700);
      } catch (err) {
        target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Operator invite failed');
      }
    }

    function openDeleteUserModal(userID, username) {
      openModal(`Delete user: ${username}`, 'Superadmin action', `
        <div class="form-grid">
          <div class="field full"><label>Confirmation</label><div class="code-block">This will permanently remove the operator account, its active sessions and related invite records. The current operator and the last superadmin cannot be deleted.</div></div>
          <div class="field full inline-actions"><button class="danger-btn" id="confirmDeleteUserBtn" type="button">Delete user</button></div>
        </div>
        <div id="deleteUserResult" class="form-result"></div>`);
      document.getElementById('confirmDeleteUserBtn').addEventListener('click', () => submitDeleteUser(userID));
    }

    async function submitDeleteUser(userID) {
      const target = document.getElementById('deleteUserResult');
      target.innerHTML = '<span class="tag warn">deleting user</span>';
      try {
        await requestJSON(`/api/v1/admin/users/${userID}`, { method: 'DELETE' });
        target.innerHTML = '<span class="tag ok">operator deleted</span>';
        await loadAdminSettings(true);
        setTimeout(closeModal, 500);
      } catch (err) {
        target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Operator delete failed');
      }
    }

    async function submitPlatformUserPasswordReset(event, userID) {
      event.preventDefault();
      const target = document.getElementById('resetPasswordResult');
      target.innerHTML = '<span class="tag warn">resetting</span>';
      try {
        const form = new FormData(event.currentTarget);
        await sendJSON(`/api/v1/admin/users/${userID}/reset-password`, 'POST', {
          password: String(form.get('password') || ''),
        });
        target.innerHTML = '<span class="tag ok">password updated, sessions revoked</span>';
        await loadAdminSettings(true);
        setTimeout(closeModal, 500);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokePlatformSession(sessionID) {
      const target = document.getElementById('revokeSessionResult');
      target.innerHTML = '<span class="tag warn">revoking</span>';
      try {
        await requestJSON(`/api/v1/admin/sessions/${sessionID}/revoke`, { method: 'POST' });
        target.innerHTML = '<span class="tag ok">session revoked</span>';
        await loadAdminSettings(true);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openSettings() {
      openModal('Interface Settings', 'Local browser settings', `
        <div class="form-grid">
          <div class="field full"><label>API base URL. Empty value means same-origin.</label><input id="apiBaseInput" value="${escapeHTML(state.apiBase)}" placeholder="https://vpn-panel.example.com" /></div>
          <div class="field full"><label>Install note</label><div class="code-block">By default RTIS MegaVPN API can serve web/index.html and /assets/* itself when the web root is available. Set API base URL here only if the UI is hosted elsewhere.</div></div>
        </div>
        <div class="modal-actions"><button class="secondary-btn" id="saveSettingsBtn" type="button">Save</button></div>`);
      document.getElementById('saveSettingsBtn').addEventListener('click', () => {
        state.apiBase = document.getElementById('apiBaseInput').value.trim().replace(/\/$/, '');
        localStorage.setItem('megavpn.apiBase', state.apiBase);
        updateReadyPill();
        closeModal();
        refresh();
      });
    }

    async function saveMailSettings(event) {
      event.preventDefault();
      const target = document.getElementById('mailSettingsResult');
      target.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const form = new FormData(event.currentTarget);
        const password = String(form.get('smtp_password') || '').trim();
        await sendJSON('/api/v1/settings/mail', 'PUT', {
          enabled: String(form.get('enabled')) === 'true',
          smtp_host: String(form.get('smtp_host') || '').trim(),
          smtp_port: Number(form.get('smtp_port') || 587),
          smtp_username: String(form.get('smtp_username') || '').trim(),
          smtp_password_secret_ref_id: state.mailSettings?.smtp_password_secret_ref_id || null,
          smtp_password: password,
          smtp_auth_mode: String(form.get('smtp_auth_mode') || 'plain'),
          smtp_tls_mode: String(form.get('smtp_tls_mode') || 'starttls'),
          from_email: String(form.get('from_email') || '').trim(),
          from_name: String(form.get('from_name') || '').trim(),
          reply_to_email: String(form.get('reply_to_email') || '').trim(),
          invite_url_base: String(form.get('invite_url_base') || '').trim(),
        });
        target.innerHTML = '<span class="tag ok">mail settings saved</span>';
        await loadAdminSettings(true);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function saveControlPlaneTLSSettings(event) {
      event.preventDefault();
      const target = document.getElementById('controlPlaneTLSResult');
      target.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const form = new FormData(event.currentTarget);
        await sendJSON('/api/v1/settings/control-plane-tls', 'PUT', {
          enabled: String(form.get('enabled')) === 'true',
          mode: String(form.get('mode') || 'managed_certificate'),
          public_base_url: String(form.get('public_base_url') || '').trim(),
          server_name: String(form.get('server_name') || '').trim(),
          listen_port: Number(form.get('listen_port') || 443),
          upstream_url: String(form.get('upstream_url') || 'http://127.0.0.1:8080').trim(),
          certificate_id: String(form.get('certificate_id') || '').trim(),
          self_signed_common_name: String(form.get('self_signed_common_name') || '').trim(),
          self_signed_dns_names: parseCSVList(String(form.get('self_signed_dns_names') || '')),
        });
        target.innerHTML = '<span class="tag ok">TLS profile saved</span>';
        await loadAdminSettings(hasPermission('auth.manage'));
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function saveFirewallManagementSettings(event) {
      event.preventDefault();
      const target = document.getElementById('firewallManagementSettingsResult');
      if (target) target.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const form = new FormData(event.currentTarget);
        const updated = await sendJSON('/api/v1/firewall/management-settings', 'PUT', {
          control_plane_source_cidrs: parseCIDRTextarea(form.get('control_plane_source_cidrs')),
          ssh_bootstrap_source_cidrs: parseCIDRTextarea(form.get('ssh_bootstrap_source_cidrs')),
          trusted_operator_cidrs: parseCIDRTextarea(form.get('trusted_operator_cidrs')),
        });
        state.firewallManagementSettings = updated;
        if (target) target.innerHTML = '<span class="tag ok">firewall sources saved</span>';
        renderFirewallManagementSettings(updated, hasPermission('firewall.read') || hasPermission('firewall.manage'), hasPermission('firewall.manage'));
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function applyControlPlaneTLSSettings() {
      const target = document.getElementById('controlPlaneTLSResult');
      target.innerHTML = '<span class="tag warn">queueing apply job</span>';
      try {
        const job = await sendJSON('/api/v1/settings/control-plane-tls/apply', 'POST', {});
        await watchJob(job.id, target, 'Control Plane TLS apply', {
          attempts: 80,
          intervalMs: 1500,
          context: { public_url: state.controlPlaneTLSSettings?.public_base_url || platformPublicBaseURL() || 'n/a' },
        });
        await loadAdminSettings(hasPermission('auth.manage'));
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function sendMailTest() {
      const target = document.getElementById('mailSettingsResult');
      target.innerHTML = '<span class="tag warn">sending test</span>';
      try {
        const email = String(document.getElementById('mailTestEmail').value || '').trim();
        const data = await sendJSON('/api/v1/settings/mail/test', 'POST', { email });
        target.innerHTML = renderActionResponse(data, 'Mail test');
        await loadAdminSettings(true);
      } catch (err) {
        target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Mail test failed');
      }
    }

    return {
      openSettings,
      changeOwnPassword,
      loadAdminSettings,
    };
  }

  window.MegaVPNSettingsWorkflows = { create: createSettingsWorkflows };
})(window);
