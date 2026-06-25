(function (window) {
  'use strict';

  function createShellUI(ctx = {}) {
    const {
      state,
      navGroups,
      el,
      setPage,
      getLogoutHandler,
      statusTag,
      escapeHTML,
      renderActionResponse,
    } = ctx;

    if (
      !state ||
      !Array.isArray(navGroups) ||
      typeof el !== 'function' ||
      typeof setPage !== 'function' ||
      typeof getLogoutHandler !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof renderActionResponse !== 'function'
    ) {
      throw new Error('MegaVPNShellUI requires shell dependencies');
    }

    function updateReadyPill() {
      const status = state.ready?.status || 'unknown';
      const dotClass = status === 'ready' ? 'ok' : 'danger';
      const readyPill = el('readyPill');
      const apiBaseLabel = el('apiBaseLabel');
      if (readyPill) readyPill.innerHTML = `<span class="dot ${dotClass}"></span>${escapeHTML(status)}`;
      if (apiBaseLabel) {
        apiBaseLabel.textContent = state.apiBase || 'current host';
        apiBaseLabel.title = state.apiBase || 'Frontend uses the same origin as the current browser page.';
      }
      const release = state.versionInfo?.version || state.dashboard?.version || 'unknown';
      const releaseLabel = el('releaseLabel');
      if (releaseLabel) releaseLabel.textContent = `release ${release}`;
    }

    function renderNotice() {
      const notice = el('notice');
      if (!notice) return;
      if (!state.authUser) {
        notice.hidden = true;
        return;
      }
      if (state.lastError) {
        notice.hidden = false;
        notice.innerHTML = `<strong>Last UI/API error.</strong> ${escapeHTML(state.lastError.message)}`;
        return;
      }
      notice.hidden = true;
      notice.innerHTML = '';
    }

    function renderNav() {
      const nav = el('nav');
      if (!nav) return;
      const activePage = state.page === 'nodeManage' ? 'nodes' : state.page;
      nav.innerHTML = navGroups.map(([group, items]) => `
        <div class="nav-section">${group}</div>
        ${items.map(([key, label, icon]) => `
          <button class="nav-item ${activePage === key ? 'active' : ''}" type="button" data-page="${key}">
            <span class="nav-left"><span class="nav-icon">${icon}</span>${label}</span>
          </button>
        `).join('')}
      `).join('');
      nav.querySelectorAll('[data-page]').forEach((btn) => btn.addEventListener('click', () => setPage(btn.dataset.page)));
    }

    function renderAuthSlot() {
      const slot = el('authSlot');
      if (!slot) return;
      if (!state.authUser) {
        slot.innerHTML = '<span class="tag warn">auth required</span>';
        return;
      }
      const displayName = state.authUser.display_name || state.authUser.username || state.authUser.email;
      slot.innerHTML = `
        <div class="auth-slot">
          <div class="auth-identity">
            <span class="tag ok">${escapeHTML(displayName)}</span>
            <span class="auth-username">${escapeHTML(state.authUser.username || state.authUser.email || 'operator')}</span>
          </div>
          <button class="secondary-btn" id="logoutBtn" type="button">Logout</button>
        </div>`;
      const btn = document.getElementById('logoutBtn');
      const logout = getLogoutHandler();
      if (btn && typeof logout === 'function') btn.addEventListener('click', logout);
    }

    function setTitle(title) {
      const pageTitle = el('pageTitle');
      if (pageTitle) pageTitle.textContent = title;
    }

    function setShellMode(isAuthenticated) {
      document.body.classList.toggle('auth-mode', !isAuthenticated);
      document.body.classList.toggle('app-mode', isAuthenticated);
      const appShell = el('appShell');
      const authGate = el('authGate');
      if (appShell) appShell.hidden = !isAuthenticated;
      if (authGate) {
        authGate.hidden = isAuthenticated;
        if (isAuthenticated) authGate.innerHTML = '';
      }
    }

    function openModal(title, eyebrow, body, options = {}) {
      const modal = document.querySelector('.modal');
      el('modalTitle').textContent = title;
      el('modalEyebrow').textContent = eyebrow;
      el('modalBody').innerHTML = body;
      if (modal) {
        modal.classList.toggle('modal-wide', Boolean(options.wide));
      }
      el('modalBackdrop').hidden = false;
    }

    function closeModal() {
      el('modalBackdrop').hidden = true;
    }

    function setSubmitBusy(form, busy, pendingLabel = 'Working...') {
      if (!form) return;
      const submit = form.querySelector('button[type="submit"]');
      if (!submit) return;
      if (!submit.dataset.originalLabel) {
        submit.dataset.originalLabel = submit.textContent || '';
      }
      submit.disabled = Boolean(busy);
      submit.textContent = busy ? pendingLabel : submit.dataset.originalLabel;
    }

    function openActionOutcomeModal(title, eyebrow, status, message, details = []) {
      const items = Array.isArray(details) ? details.filter((item) => item && item.label) : [];
      openModal(title, eyebrow, `
        <div class="form-grid">
          <div class="field full">
            <div class="code-block">
              <div style="margin-bottom:8px">${statusTag(status)}</div>
              <div><strong>${escapeHTML(message || 'Operation finished')}</strong></div>
            </div>
          </div>
          ${items.map((item) => `
            <div class="field">
              <label>${escapeHTML(item.label)}</label>
              <div class="code-block">${escapeHTML(String(item.value ?? 'n/a'))}</div>
            </div>`).join('')}
          <div class="field full inline-actions"><button class="primary-btn" id="actionOutcomeCloseBtn" type="button">Close</button></div>
        </div>`, { wide: true });
      const btn = document.getElementById('actionOutcomeCloseBtn');
      if (btn) btn.addEventListener('click', closeModal);
    }

    function openUnavailableAction(title, text) {
      openModal(title, 'Action unavailable', `<p>${escapeHTML(text)}</p>`);
    }

    return {
      updateReadyPill,
      renderNotice,
      renderNav,
      renderAuthSlot,
      setTitle,
      setShellMode,
      openModal,
      closeModal,
      setSubmitBusy,
      openActionOutcomeModal,
      openUnavailableAction,
    };
  }

  window.MegaVPNShellUI = { create: createShellUI };
})(window);
