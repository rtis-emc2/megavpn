(function (window) {
  'use strict';

  function createAuthWorkflows(ctx = {}) {
    const {
      state,
      authView,
      setTitle,
      el,
      requestJSON,
      sendJSON,
      refresh,
      render,
      setSubmitBusy,
      openSettings,
      escapeHTML,
    } = ctx;

    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof render !== 'function' ||
      typeof setSubmitBusy !== 'function' ||
      typeof openSettings !== 'function' ||
      typeof escapeHTML !== 'function'
    ) {
      throw new Error('MegaVPNAuthWorkflows requires workflow dependencies');
    }

    function renderAuthContent(html) {
      const mount = el('authGate') || el('content');
      if (mount) mount.innerHTML = html;
    }

    function applyAuthPayload(data) {
      state.authUser = data?.user || null;
      state.authSession = data?.session || null;
      state.authRoles = Array.isArray(data?.roles) ? data.roles : [];
      state.authPermissions = Array.isArray(data?.permissions) ? data.permissions : [];
    }

    function clearAuthPayload() {
      state.authUser = null;
      state.authSession = null;
      state.authRoles = [];
      state.authPermissions = [];
    }

    async function loadSession() {
      try {
        const data = await requestJSON('/api/v1/auth/me');
        applyAuthPayload(data);
        return true;
      } catch (err) {
        if (err.status === 401) {
          clearAuthPayload();
          return false;
        }
        throw err;
      }
    }

    function renderLoginScreen() {
      setTitle('Operator Login');
      renderAuthContent(authView?.renderLogin
        ? authView.renderLogin({ apiBase: state.apiBase })
        : '<section class="auth-card"><h2>Login</h2><form id="loginForm" class="form-grid"><div class="field full"><label>Login</label><input name="login" required /></div><div class="field full"><label>Password</label><input name="password" type="password" required /></div><button class="primary-btn" type="submit">Login</button></form><div id="loginResult" class="auth-message"></div><button class="secondary-btn" type="button" id="loginSettingsBtn">API Settings</button></section>');
      document.getElementById('loginForm').addEventListener('submit', login);
      document.getElementById('loginSettingsBtn').addEventListener('click', openSettings);
    }

    function renderInviteAcceptScreen() {
      const invite = state.invitePreview || {};
      setTitle('Invitation');
      renderAuthContent(authView?.renderInvite
        ? authView.renderInvite({ invite })
        : '<section class="auth-card"><h2>Invitation</h2><form id="inviteAcceptForm" class="form-grid"><div class="field full"><label>Password</label><input name="password" type="password" required /></div><button class="primary-btn" type="submit">Activate account</button></form><button class="secondary-btn" type="button" id="inviteBackBtn">Back to login</button><div id="inviteAcceptResult" class="auth-message"></div></section>');
      document.getElementById('inviteAcceptForm').addEventListener('submit', acceptInvite);
      document.getElementById('inviteBackBtn').addEventListener('click', () => clearInviteToken(true));
    }

    async function login(event) {
      event.preventDefault();
      const target = document.getElementById('loginResult');
      const formEl = event.currentTarget;
      state.refreshSeq += 1;
      target.innerHTML = '<span class="tag warn">authorizing</span>';
      setSubmitBusy(formEl, true, 'Login...');
      try {
        const form = new FormData(event.currentTarget);
        const data = await sendJSON('/api/v1/auth/login', 'POST', {
          login: String(form.get('login') || '').trim(),
          password: String(form.get('password') || ''),
        });
        applyAuthPayload(data);
        await refresh();
        if (!state.authUser) {
          renderLoginScreen();
          const currentTarget = document.getElementById('loginResult');
          if (currentTarget) {
            currentTarget.innerHTML = '<span class="tag danger">Сессия не открылась после входа. Обновите страницу и повторите вход.</span>';
          }
          return;
        }
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      } finally {
        setSubmitBusy(formEl, false);
      }
    }

    async function logout() {
      try {
        await requestJSON('/api/v1/auth/logout', { method: 'POST' });
      } catch (_) {
        // Session may already be gone; local cleanup still matters.
      }
      clearAuthPayload();
      state.invitePreview = null;
      state.lastError = null;
      render();
    }

    async function loadInvitePreview() {
      if (!state.inviteToken) {
        state.invitePreview = null;
        return;
      }
      try {
        state.invitePreview = await requestJSON(`/api/v1/auth/invites/${encodeURIComponent(state.inviteToken)}`);
      } catch (err) {
        state.invitePreview = { status: 'invalid', error: err.message };
        state.lastError = err;
      }
    }

    async function acceptInvite(event) {
      event.preventDefault();
      const target = document.getElementById('inviteAcceptResult');
      target.innerHTML = '<span class="tag warn">activating</span>';
      try {
        const form = new FormData(event.currentTarget);
        await sendJSON(`/api/v1/auth/invites/${encodeURIComponent(state.inviteToken)}/accept`, 'POST', {
          password: String(form.get('password') || ''),
        });
        clearInviteToken(false);
        await refresh();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function clearInviteToken(shouldRender) {
      state.inviteToken = '';
      state.invitePreview = null;
      const url = new URL(window.location.href);
      url.searchParams.delete('invite_token');
      window.history.replaceState({}, '', url.toString());
      if (shouldRender) render();
    }

    return {
      loadSession,
      renderLoginScreen,
      renderInviteAcceptScreen,
      logout,
      loadInvitePreview,
    };
  }

  window.MegaVPNAuthWorkflows = { create: createAuthWorkflows };
})(window);
