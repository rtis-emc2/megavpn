(function (window) {
  'use strict';

  function createAppRuntime(ctx = {}) {
    const { state, responseView = null } = ctx;
    if (!state) {
      throw new Error('MegaVPNAppRuntime requires state');
    }

    function escapeHTML(value) {
      return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
    }

    function arrayOrEmpty(value) {
      return Array.isArray(value) ? value : [];
    }

    function parseCSVList(value) {
      return String(value || '')
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
    }

    function clsStatus(status) {
      const normalized = String(status || '').toLowerCase();
      if (['ok', 'ready', 'active', 'healthy', 'succeeded', 'online', 'configured', 'enabled', 'sent', 'delivered', 'in_sync', 'applied'].includes(normalized)) return 'ok';
      if (['stub', 'planned', 'draft', 'pending', 'unknown', 'maintenance', 'skipped', 'stopped', 'materialized'].includes(normalized)) return 'stub';
      if (['degraded', 'warning', 'retrying', 'queued', 'running', 'starting', 'installing', 'bootstrapping', 'waiting heartbeat', 'awaiting heartbeat', 'provisioning', 'inactive', 'pending_apply', 'update available'].includes(normalized)) return 'warn';
      if (['failed', 'blocked', 'offline', 'error', 'disabled', 'cancelled', 'revoked', 'missing', 'loopback-only', 'delivery_failed', 'expired', 'invalid', 'deleted', 'unhealthy', 'drifted'].includes(normalized)) return 'danger';
      return 'stub';
    }

    function statusTag(value) {
      const statusClass = clsStatus(value);
      return `<span class="tag ${statusClass}"><span class="dot ${statusClass === 'stub' ? 'unknown' : statusClass}"></span>${escapeHTML(value)}</span>`;
    }

    function renderActionResponse(data, title = 'Operation result') {
      if (responseView?.render) return responseView.render(data, { title });
      return `<div class="code-block">${escapeHTML(JSON.stringify(data, null, 2))}</div>`;
    }

    function formatDate(value) {
      if (!value) return 'n/a';
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return escapeHTML(value);
      return escapeHTML(date.toLocaleString('ru-RU'));
    }

    function formatRelativeDate(value) {
      if (!value) return 'n/a';
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return escapeHTML(value);
      const diffMs = Date.now() - date.getTime();
      const diffSec = Math.floor(diffMs / 1000);
      if (diffSec < 60) return `${diffSec}s ago`;
      const diffMin = Math.floor(diffSec / 60);
      if (diffMin < 60) return `${diffMin}m ago`;
      const diffHours = Math.floor(diffMin / 60);
      if (diffHours < 24) return `${diffHours}h ago`;
      const diffDays = Math.floor(diffHours / 24);
      return `${diffDays}d ago`;
    }

    function formatDurationSeconds(value) {
      const seconds = Number(value);
      if (!Number.isFinite(seconds) || seconds < 0) return 'n/a';
      if (seconds < 60) return `${seconds}s`;
      const minutes = Math.floor(seconds / 60);
      if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
      const hours = Math.floor(minutes / 60);
      if (hours < 24) return `${hours}h ${minutes % 60}m`;
      const days = Math.floor(hours / 24);
      return `${days}d ${hours % 24}h`;
    }

    function toMillis(value) {
      if (!value) return 0;
      const ms = Date.parse(value);
      return Number.isFinite(ms) ? ms : 0;
    }

    function hasPermission(code) {
      if (hasRole('superadmin')) return true;
      return Array.isArray(state.authPermissions) && state.authPermissions.includes(code);
    }

    function hasRole(code) {
      const expected = String(code || '').trim().toLowerCase();
      return Boolean(expected) && Array.isArray(state.authRoles) && state.authRoles
        .some((role) => String(role || '').trim().toLowerCase() === expected);
    }

    function platformPublicBaseURL() {
      return String(state.versionInfo?.public_base_url || '').trim().replace(/\/$/, '');
    }

    function publicURLHostname(value) {
      try {
        return new URL(value || '').hostname;
      } catch (_) {
        return '';
      }
    }

    function publicURLPort(value) {
      try {
        const url = new URL(value || '');
        if (url.port) return Number(url.port);
        return url.protocol === 'https:' ? 443 : 80;
      } catch (_) {
        return 0;
      }
    }

    function agentEndpointURL(path) {
      const base = platformPublicBaseURL();
      if (!base) return 'n/a';
      return `${base}${path}`;
    }

    function isLoopbackURL(value) {
      try {
        const hostname = new URL(value).hostname.toLowerCase();
        return hostname === 'localhost' || hostname === '::1' || hostname === '[::1]' || hostname.startsWith('127.');
      } catch (_) {
        return false;
      }
    }

    function publicBaseURLStatusTag() {
      const value = platformPublicBaseURL();
      if (!value) return statusTag('missing');
      if (isLoopbackURL(value)) return statusTag('loopback-only');
      return statusTag('configured');
    }

    return {
      escapeHTML,
      arrayOrEmpty,
      parseCSVList,
      clsStatus,
      statusTag,
      renderActionResponse,
      formatDate,
      formatRelativeDate,
      formatDurationSeconds,
      toMillis,
      hasPermission,
      hasRole,
      platformPublicBaseURL,
      publicURLHostname,
      publicURLPort,
      agentEndpointURL,
      isLoopbackURL,
      publicBaseURLStatusTag,
    };
  }

  window.MegaVPNAppRuntime = { create: createAppRuntime };
})(window);
