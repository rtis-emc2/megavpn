(function (window) {
  'use strict';

  function createSettingsPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      statusTag,
      escapeHTML,
      renderInventoryFact,
      hasPermission,
      hasRole,
      platformPublicBaseURL,
      agentEndpointURL,
      publicBaseURLStatusTag,
      isLoopbackURL,
      openSettings,
      changeOwnPassword,
      loadAdminSettings,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof renderInventoryFact !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof hasRole !== 'function' ||
      typeof platformPublicBaseURL !== 'function' ||
      typeof agentEndpointURL !== 'function' ||
      typeof publicBaseURLStatusTag !== 'function' ||
      typeof isLoopbackURL !== 'function' ||
      typeof openSettings !== 'function' ||
      typeof changeOwnPassword !== 'function' ||
      typeof loadAdminSettings !== 'function'
    ) {
      throw new Error('MegaVPNSettingsPage requires page dependencies');
    }

    function render() {
      setTitle('Settings');
      const canManageAuth = hasPermission('auth.manage');
      const canManageSettings = hasPermission('settings.manage');
      const canDeleteUsers = hasRole('superadmin');
      el('content').innerHTML = `
        <div class="settings-layout">
          <section class="card">
            <h2>Runtime Configuration</h2>
            <div class="inventory-facts" style="margin-top:14px">
              ${renderInventoryFact('Browser API base', state.apiBase || 'current host', 'Local UI setting only')}
              ${renderInventoryFact('Agent public URL', platformPublicBaseURL() || 'missing', 'MEGAVPN_PUBLIC_BASE_URL')}
              ${renderInventoryFact('Agent register endpoint', agentEndpointURL('/agent/register'), 'Used by megavpn-agent first enrollment')}
              <div class="fact-card">
                <div class="mini-label">Public URL status</div>
                <div class="metric-caption strong">${publicBaseURLStatusTag()}</div>
                <div class="metric-caption">${escapeHTML(isLoopbackURL(platformPublicBaseURL()) ? 'Remote nodes cannot enroll through loopback.' : 'Must be reachable from every remote node.')}</div>
              </div>
            </div>
            <div class="inline-actions" style="margin-top:14px">
              <button class="secondary-btn" id="openSettingsInlineBtn" type="button">API Settings</button>
            </div>
          </section>
          <section class="card">
            <h2>Change Password</h2>
            <form id="changePasswordForm" class="form-grid">
              <div class="field full"><label>Current password</label><input name="current_password" type="password" autocomplete="current-password" required /></div>
              <div class="field full"><label>New password</label><input name="new_password" type="password" autocomplete="new-password" required placeholder="minimum 12 chars" /></div>
              <div class="field full inline-actions"><button class="primary-btn" type="submit">Update password</button></div>
            </form>
            <div id="changePasswordResult" class="form-result"></div>
          </section>
        </div>
        <section class="table-card">
          <div class="table-head"><h2>Runtime Preflight</h2><div class="table-tools"><span class="tag">${canManageSettings ? 'settings.manage' : 'read-only'}</span></div></div>
          <div class="card-body" id="runtimePreflightMount"><div class="empty">Loading runtime preflight...</div></div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Control Plane TLS</h2><div class="table-tools"><span class="tag">${canManageSettings ? 'settings.manage' : 'read-only'}</span></div></div>
          <div class="card-body" id="controlPlaneTLSMount"><div class="empty">Loading TLS settings...</div></div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Platform Users</h2><div class="table-tools">${canManageAuth ? '<span class="tag ok">auth.manage</span>' : '<span class="tag stub">read-only</span>'}</div></div>
          <div class="card-body" id="platformUsersMount"><div class="empty">Loading platform users...</div></div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Mail Settings</h2><div class="table-tools"><span class="tag">${canManageAuth ? 'smtp' : 'read-only'}</span></div></div>
          <div class="card-body" id="mailSettingsMount"><div class="empty">Loading mail settings...</div></div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Platform CA Center</h2><div class="table-tools"><span class="tag">${hasPermission('instance.read') ? 'instance.read' : 'read-only'}</span></div></div>
          <div class="card-body" id="platformPKIRootsMount"><div class="empty">Loading PKI roots...</div></div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Operator Invites</h2><div class="table-tools"><span class="tag">${canManageAuth ? 'email delivery' : 'read-only'}</span></div></div>
          <div class="card-body" id="platformInvitesMount"><div class="empty">Loading invites...</div></div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Active Sessions</h2><div class="table-tools"><span class="tag">${escapeHTML(state.authUser?.username || 'operator')}</span></div></div>
          <div class="card-body" id="platformSessionsMount"><div class="empty">Loading active sessions...</div></div>
        </section>`;
      document.getElementById('openSettingsInlineBtn')?.addEventListener('click', openSettings);
      document.getElementById('changePasswordForm')?.addEventListener('submit', changeOwnPassword);
      void loadAdminSettings(canManageAuth, canDeleteUsers);
    }

    return { render };
  }

  window.MegaVPNSettingsPage = { create: createSettingsPage };
})(window);
