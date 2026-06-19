(() => {
  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
  }

  function apiCaption(apiBase) {
    const value = String(apiBase || '').trim();
    return value || 'current host';
  }

  function renderLogin({ apiBase = '' } = {}) {
    return `
      <section class="login-shell login-compact" aria-label="RTIS MegaVPN authentication">
        <div class="auth-card">
          <div class="brand brand-auth auth-brand-inline">
            <div class="brand-mark">
              <img src="./assets/rtis-logo.svg" alt="RTIS" class="brand-logo" />
            </div>
            <div>
              <div class="brand-title">RTIS</div>
              <div class="brand-subtitle">MegaVPN Control Plane</div>
            </div>
          </div>
          <h2>Вход</h2>
          <form id="loginForm" class="form-grid">
            <div class="field full"><label>Логин</label><input name="login" type="text" autocomplete="username" required autofocus /></div>
            <div class="field full"><label>Пароль</label><input name="password" type="password" autocomplete="current-password" required /></div>
            <div class="field full inline-actions">
              <button class="primary-btn" type="submit">Войти</button>
              <button class="secondary-btn" type="button" id="loginSettingsBtn">API</button>
            </div>
          </form>
          <div class="auth-api-caption">API: <code>${escapeHTML(apiCaption(apiBase))}</code></div>
          <div id="loginResult" class="auth-message"></div>
        </div>
      </section>`;
  }

  function renderInvite({ invite = {} } = {}) {
    return `
      <section class="login-shell" aria-label="RTIS MegaVPN invitation activation">
        <div class="login-hero">
          <div class="brand brand-auth">
            <div class="brand-mark">
              <img src="./assets/rtis-logo.svg" alt="RTIS" class="brand-logo" />
            </div>
            <div>
              <div class="brand-title">RTIS</div>
              <div class="brand-subtitle">MegaVPN Control Plane</div>
            </div>
          </div>
          <div class="eyebrow">Operator onboarding</div>
          <h2>Activate protected access</h2>
          <p>Одноразовая ссылка задает пароль оператора и открывает полноценную session после успешной активации.</p>
          <div class="login-points">
            <div class="login-point"><strong>User</strong><span>${escapeHTML(invite.username || 'unknown')}</span></div>
            <div class="login-point"><strong>Email</strong><span>${escapeHTML(invite.email || 'n/a')}</span></div>
            <div class="login-point"><strong>Status</strong><span>${escapeHTML(invite.status || 'pending')}</span></div>
          </div>
        </div>
        <div class="auth-card">
          <div class="eyebrow">One-time access</div>
          <h2>Задайте пароль</h2>
          <p>Пароль должен соответствовать политике безопасности backend API.</p>
          <form id="inviteAcceptForm" class="form-grid">
            <div class="field full"><label>Password</label><input name="password" type="password" autocomplete="new-password" required placeholder="minimum 12 chars" /></div>
            <div class="field full inline-actions">
              <button class="primary-btn" type="submit">Activate account</button>
              <button class="secondary-btn" type="button" id="inviteBackBtn">Back to login</button>
            </div>
          </form>
          <div id="inviteAcceptResult" class="auth-message"></div>
        </div>
      </section>`;
  }

  window.MegaVPNAuthView = { renderLogin, renderInvite };
})();
