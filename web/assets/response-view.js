(() => {
  const SENSITIVE_KEY = /(password|secret|token|private[_-]?key|certificate|agent_env|bootstrapenv|ciphertext|nonce)/i;
  const BULKY_KEY = /(payload|result|spec|config|logs|items|children|private_key|certificate|chain)/i;

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[ch]));
  }

  function statusClass(status) {
    const normalized = String(status || '').toLowerCase();
    if (['ok', 'ready', 'active', 'healthy', 'succeeded', 'online', 'configured', 'enabled', 'sent', 'delivered', 'created', 'updated', 'in_sync'].includes(normalized)) return 'ok';
    if (['queued', 'running', 'retrying', 'pending', 'provisioning', 'installing', 'bootstrapping', 'waiting', 'accepted', 'degraded', 'warning', 'pending_apply'].includes(normalized)) return 'warn';
    if (['failed', 'blocked', 'offline', 'error', 'disabled', 'cancelled', 'revoked', 'missing', 'expired', 'invalid', 'deleted', 'unhealthy', 'drifted'].includes(normalized)) return 'danger';
    return 'stub';
  }

  function statusTag(status) {
    const text = String(status || 'ok');
    const cls = statusClass(text);
    return `<span class="tag ${cls}"><span class="dot ${cls === 'stub' ? 'unknown' : cls}"></span>${escapeHTML(text)}</span>`;
  }

  function humanizeKey(key) {
    return String(key || '')
      .replace(/[_-]+/g, ' ')
      .replace(/\bid\b/gi, 'ID')
      .replace(/\burl\b/gi, 'URL')
      .replace(/\bttl\b/gi, 'TTL')
      .replace(/\bapi\b/gi, 'API')
      .replace(/\bca\b/gi, 'CA')
      .replace(/\bpki\b/gi, 'PKI')
      .replace(/\b\w/g, (ch) => ch.toUpperCase());
  }

  function isObject(value) {
    return value && typeof value === 'object' && !Array.isArray(value);
  }

  function shortID(value) {
    const text = String(value || '');
    if (text.length <= 16) return text;
    return `${text.slice(0, 8)}…${text.slice(-6)}`;
  }

  function previewValue(key, value) {
    if (value == null || value === '') return 'n/a';
    if (SENSITIVE_KEY.test(String(key || ''))) return 'available, hidden in summary';
    if (typeof value === 'boolean') return value ? 'yes' : 'no';
    if (typeof value === 'number') return String(value);
    if (Array.isArray(value)) return `${value.length} item${value.length === 1 ? '' : 's'}`;
    if (isObject(value)) return `${Object.keys(value).length} field${Object.keys(value).length === 1 ? '' : 's'}`;
    const text = String(value);
    if (/(_id|id)$/i.test(String(key || ''))) return shortID(text);
    return text.length > 160 ? `${text.slice(0, 157)}...` : text;
  }

  function collectFacts(data) {
    if (!isObject(data)) return [];
    const preferred = [
      'message', 'status', 'id', 'job_id', 'type', 'kind', 'name', 'username', 'email',
      'node_id', 'instance_id', 'client_id', 'artifact_id', 'share_link_id', 'url',
      'expires_at', 'ttl_hours', 'created_at', 'updated_at', 'public_url',
      'service_code', 'capability_code', 'systemd_unit', 'endpoint_host', 'endpoint_port',
      'revision_id', 'revision_no', 'can_apply', 'issue_count', 'cascade_count',
    ];
    const keys = Object.keys(data);
    const ordered = [
      ...preferred.filter((key) => Object.prototype.hasOwnProperty.call(data, key)),
      ...keys.filter((key) => !preferred.includes(key) && !BULKY_KEY.test(key)).sort(),
    ];
    return ordered
      .filter((key) => data[key] !== undefined)
      .slice(0, 12)
      .map((key) => [humanizeKey(key), previewValue(key, data[key])]);
  }

  function renderFactGrid(facts) {
    if (!facts.length) return '';
    return `
      <div class="response-grid">
        ${facts.map(([label, value]) => `
          <div class="response-fact">
            <span>${escapeHTML(label)}</span>
            <strong>${escapeHTML(value)}</strong>
          </div>`).join('')}
      </div>`;
  }

  function normalizeJob(data) {
    if (isObject(data?.job)) return data.job;
    if (isObject(data) && data.id && (data.status || data.type || data.kind || data.scope_type)) return data;
    return null;
  }

  function inferStatus(data, job) {
    if (job?.status) return job.status;
    if (data?.status) return data.status;
    if (data?.ok === true || data?.success === true) return 'succeeded';
    if (data?.ok === false || data?.success === false || data?.error) return 'failed';
    return 'accepted';
  }

  function inferMessage(data, job, fallback) {
    if (typeof data?.message === 'string' && data.message.trim()) return data.message.trim();
    if (typeof data?.error === 'string' && data.error.trim()) return data.error.trim();
    if (job?.id) return `${fallback} accepted and queued for execution.`;
    return `${fallback} completed.`;
  }

  function renderRawDetails(data) {
    if (data == null) return '';
    let raw = '';
    try {
      raw = JSON.stringify(data, null, 2);
    } catch (_) {
      raw = String(data);
    }
    if (!raw || raw === '{}') return '';
    return `
      <details class="response-raw">
        <summary>Technical details</summary>
        <div class="code-block">${escapeHTML(raw)}</div>
      </details>`;
  }

  function renderListSummary(value) {
    const rows = Array.isArray(value) ? value : [];
    if (!rows.length) return '<div class="empty compact-empty">No returned items.</div>';
    return `
      <div class="timeline response-list">
        ${rows.slice(0, 6).map((item, index) => `
          <div class="timeline-item">
            <strong>${escapeHTML(isObject(item) ? (item.name || item.id || `Item ${index + 1}`) : `Item ${index + 1}`)}</strong>
            <div class="timeline-meta">${escapeHTML(isObject(item) ? previewValue('status', item.status || item.type || item.kind || 'returned') : previewValue('', item))}</div>
          </div>`).join('')}
        ${rows.length > 6 ? `<div class="timeline-item"><strong>${escapeHTML(String(rows.length - 6))} more item(s)</strong><div class="timeline-meta">Open technical details for the full payload.</div></div>` : ''}
      </div>`;
  }

  function render(data, options = {}) {
    const title = String(options.title || 'Operation result');
    const job = normalizeJob(data);
    const status = inferStatus(data || {}, job);
    const message = inferMessage(data || {}, job, title);
    const facts = collectFacts(job ? { ...data, job_id: job.id, type: job.type || job.kind, status: job.status, scope_type: job.scope_type, scope_id: job.scope_id } : data || {});
    const listBody = Array.isArray(data) ? renderListSummary(data) : '';

    return `
      <section class="response-card">
        <div class="response-head">
          <div>
            <div class="mini-label">${escapeHTML(title)}</div>
            <h3>${escapeHTML(message)}</h3>
          </div>
          ${statusTag(status)}
        </div>
        ${renderFactGrid(facts)}
        ${listBody}
        ${renderRawDetails(data)}
      </section>`;
  }

  window.MegaVPNResponseView = { render };
})();
