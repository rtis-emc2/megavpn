(function (window) {
  'use strict';

  const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS', 'TRACE']);

  function createAPIClient(ctx = {}) {
    const { getApiBase, onError } = ctx;
    if (typeof getApiBase !== 'function') {
      throw new Error('MegaVPNAPIClient requires getApiBase');
    }

    function apiURL(path) {
      return `${getApiBase() || ''}${path}`;
    }

    function headers(extra = {}) {
      return { Accept: 'application/json', ...extra };
    }

    async function request(path, options = {}) {
      const opts = { credentials: 'include', ...options };
      opts.headers = headers(options.headers || {});
      const method = String(opts.method || 'GET').toUpperCase();
      if (!SAFE_METHODS.has(method)) {
        opts.headers['X-MegaVPN-CSRF'] = '1';
      }

      const res = await fetch(apiURL(path), opts);
      const contentType = res.headers.get('content-type') || '';
      let data = null;
      let text = '';
      if (contentType.includes('application/json')) {
        data = await res.json().catch(() => null);
      } else {
        text = await res.text().catch(() => '');
      }

      if (!res.ok) {
        const msg = data?.error || text || `${path}: HTTP ${res.status}`;
        const err = new Error(msg);
        err.status = res.status;
        err.payload = data;
        throw err;
      }
      return data;
    }

    async function requestJSON(path, options = {}) {
      return request(path, options);
    }

    async function sendJSON(path, method, payload) {
      return requestJSON(path, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: payload == null ? null : JSON.stringify(payload),
      });
    }

    async function fetchJSON(path, fallback = null, options = {}) {
      try {
        return await requestJSON(path, options);
      } catch (err) {
        if (typeof onError === 'function') onError(err, { path, fallback });
        return fallback;
      }
    }

    return {
      apiURL,
      request,
      requestJSON,
      sendJSON,
      fetchJSON,
    };
  }

  window.MegaVPNAPIClient = { create: createAPIClient };
})(window);
