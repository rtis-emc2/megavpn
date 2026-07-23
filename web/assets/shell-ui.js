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
      const activePage = state.page === 'nodeManage' ? 'nodes' : (state.page === 'instanceManage' ? 'instances' : state.page);
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
      const username = state.authUser.username || state.authUser.email || 'operator';
      const roles = Array.isArray(state.authRoles) && state.authRoles.length ? state.authRoles.join(', ') : 'operator';
      const email = state.authUser.email || '';
      const secondary = email && email !== displayName && email !== username ? email : roles;
      const initials = String(displayName || username || 'OP')
        .trim()
        .split(/\s+/)
        .slice(0, 2)
        .map((part) => part[0] || '')
        .join('')
        .toUpperCase()
        .slice(0, 2) || 'OP';
      slot.innerHTML = `
        <details class="user-menu">
          <summary class="user-menu-trigger">
            <span class="user-avatar">${escapeHTML(initials)}</span>
            <span class="user-menu-text">
              <strong>${escapeHTML(displayName || username)}</strong>
              <small>${escapeHTML(secondary)}</small>
            </span>
          </summary>
          <div class="user-menu-panel">
            <div class="user-menu-row"><span>Account</span><strong>${escapeHTML(username)}</strong></div>
            ${email ? `<div class="user-menu-row"><span>Email</span><strong>${escapeHTML(email)}</strong></div>` : ''}
            <div class="user-menu-row"><span>Roles</span><strong>${escapeHTML(roles)}</strong></div>
            <div class="user-menu-row"><span>Permissions</span><strong>${escapeHTML(String((state.authPermissions || []).length))}</strong></div>
            <button class="secondary-btn user-menu-logout" id="logoutBtn" type="button">Logout</button>
          </div>
        </details>`;
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
      const backdrop = el('modalBackdrop');
      const modalBody = el('modalBody');
      el('modalTitle').textContent = title;
      el('modalEyebrow').textContent = eyebrow;
      modalBody.innerHTML = body;
      modalBody.className = `modal-body${options.bodyClass ? ` ${options.bodyClass}` : ''}`;
      window.MegaVPNFormEnhancer?.enhance?.(modalBody);
      if (modal) {
        modal.classList.remove('modal-wide', 'modal-compact', 'modal-full');
        modal.classList.toggle('modal-wide', Boolean(options.wide) || options.size === 'large');
        modal.classList.toggle('modal-compact', options.size === 'compact');
        modal.classList.toggle('modal-full', options.size === 'full');
        modal.dataset.variant = String(options.variant || '').trim();
        modal.scrollTop = 0;
      }
      modalBody.scrollTop = 0;
      document.body.classList.add('modal-open');
      backdrop.hidden = false;
      window.requestAnimationFrame(() => modal?.focus?.({ preventScroll: true }));
    }

    function closeModal() {
      el('modalBackdrop').hidden = true;
      document.body.classList.remove('modal-open');
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
        <div class="action-outcome">
          <div class="action-outcome-head">
            ${statusTag(status)}
            <div>
              <h3>${escapeHTML(message || 'Operation finished')}</h3>
              <p>${escapeHTML(eyebrow || title || 'Operation result')}</p>
            </div>
          </div>
          ${items.length ? `
            <div class="response-grid action-outcome-grid">
              ${items.map((item) => `
                <div class="response-fact">
                  <span>${escapeHTML(item.label)}</span>
                  <strong>${escapeHTML(String(item.value ?? 'n/a'))}</strong>
                </div>`).join('')}
            </div>` : ''}
          <div class="modal-actions"><button class="primary-btn" id="actionOutcomeCloseBtn" type="button">Close</button></div>
        </div>`, { wide: items.length > 2, variant: status });
      const btn = document.getElementById('actionOutcomeCloseBtn');
      if (btn) btn.addEventListener('click', closeModal);
    }

    function openUnavailableAction(title, text) {
      openModal(title, 'Action unavailable', `<p>${escapeHTML(text)}</p>`, { variant: 'warning' });
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
