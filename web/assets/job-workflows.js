(function (window) {
  'use strict';

  function createJobWorkflows(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      requestJSON,
      fetchJSON,
      statusTag,
      escapeHTML,
      formatDate,
      renderActionResponse,
      stringValue,
    } = ctx;

    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof fetchJSON !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof stringValue !== 'function'
    ) {
      throw new Error('MegaVPNJobWorkflows requires workflow dependencies');
    }

    function firstJobResultText(...values) {
      for (const value of values) {
        const text = String(value ?? '').trim();
        if (text) return text;
      }
      return '';
    }

    function firstUsefulJobOutputLine(value) {
      const lines = String(value ?? '').split('\n').map((line) => line.trim()).filter(Boolean);
      if (!lines.length) return '';
      const important = lines.find((line) => /options error|error:|failed|cannot|unable|exiting|status=|active:/i.test(line));
      return important || lines[0];
    }

    function jobSystemdDiagnosticText(result = {}, health = {}) {
      const unit = firstJobResultText(result.systemd_unit, health.systemd_unit);
      const stateText = firstJobResultText(result.health_active_state, health.active_state, result.active_state);
      const output = firstUsefulJobOutputLine(
        result.health_unit_status_output
        || health.unit_status_output
        || result.systemd_output
        || result.pre_start_stop_warning
      );
      return [unit ? `unit ${unit}` : '', stateText ? `state ${stateText}` : '', output].filter(Boolean).join(' · ');
    }

    function jobDetailedFailureText(job, result = {}, health = {}) {
      const reason = firstJobResultText(job?.error, result.health_reason, health.reason, result.error);
      const details = jobSystemdDiagnosticText(result, health);
      if (reason && details && /systemd|unit|activation|not active/i.test(reason)) {
        return `${reason} · ${details}`;
      }
      return '';
    }

    function jobHealthResultText(health = {}) {
      const reason = firstJobResultText(health.reason, health.route_warning, health.error);
      const loss = Number(health.packet_loss_percent);
      const avg = Number(health.latency_avg_ms);
      return [
        reason,
        health.active_state ? `unit ${health.active_state}` : '',
        health.interface ? `dev ${health.interface}` : '',
        Number.isFinite(loss) ? `${loss % 1 === 0 ? loss.toFixed(0) : loss.toFixed(1)}% loss` : '',
        Number.isFinite(avg) ? `${avg.toFixed(1)} ms avg` : '',
      ].filter(Boolean).join(' · ');
    }

    function jobResultText(job) {
      const result = job?.result || {};
      const health = result.health || {};
      return firstJobResultText(
        jobDetailedFailureText(job, result, health),
        job?.error,
        result.health_reason,
        result.health_route_warning,
        health.reason,
        health.route_warning,
        result.health_error,
        health.error,
        jobHealthResultText(health),
        jobSystemdDiagnosticText(result, health),
        result.error,
        result.active_state,
        result.message
      );
    }

    function shortToken(value, left = 8, right = 4) {
      const text = String(value || '').trim();
      if (!text) return 'n/a';
      if (text.length <= left + right + 1) return text;
      return `${text.slice(0, left)}…${text.slice(-right)}`;
    }

    function compactJobType(value) {
      const text = String(value || 'unknown').trim();
      return text
        .replace(/^node\./, 'n.')
        .replace(/^instance\./, 'i.')
        .replace(/^client\./, 'c.')
        .replace(/^artifact\./, 'a.')
        .replace(/^control_plane\./, 'cp.')
        .replace(/\.backhaul\./, '.bh.')
        .replace(/\.bootstrap$/, '.boot')
        .replace(/\.capability\./, '.cap.');
    }

    function compactScope(value) {
      const text = String(value || 'n/a').trim();
      const [kind, id] = text.includes(':') ? text.split(/:(.*)/s).filter(Boolean) : ['', text];
      if (!kind || !id || id === 'n/a') return shortToken(text, 14, 6);
      const shortKind = kind
        .replace('control_plane', 'cp')
        .replace('backhaul', 'bh')
        .replace('instance', 'inst');
      return `${shortKind}:${shortToken(id, 8, 4)}`;
    }

    function compactJobDate(value) {
      if (!value) return 'n/a';
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return String(value);
      const day = date.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' });
      const time = date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
      return `${day} ${time}`;
    }

    function renderJobStatusLight(status) {
      const value = String(status || 'unknown').toLowerCase();
      let cls = 'stub';
      if (['ok', 'ready', 'active', 'healthy', 'succeeded', 'online', 'configured', 'enabled', 'sent', 'delivered', 'in_sync'].includes(value)) cls = 'ok';
      else if (['degraded', 'warning', 'retrying', 'queued', 'running', 'starting', 'installing', 'bootstrapping', 'waiting heartbeat', 'awaiting heartbeat', 'provisioning', 'inactive', 'pending_apply', 'update available'].includes(value)) cls = 'warn';
      else if (['failed', 'blocked', 'offline', 'error', 'disabled', 'cancelled', 'revoked', 'missing', 'loopback-only', 'delivery_failed', 'expired', 'invalid', 'deleted', 'unhealthy', 'drifted'].includes(value)) cls = 'danger';
      return `<span class="status-light ${cls}" title="${escapeHTML(value)}" aria-label="${escapeHTML(value)}"><span class="status-light-dot"></span><span class="status-light-text">${escapeHTML(value)}</span></span>`;
    }

    function renderJobQueue(rows) {
      if (!rows.length) return '<div class="empty">Нет данных для отображения</div>';
      return `
        <div class="table-wrap jobs-table-wrap">
          <table class="jobs-table">
            <colgroup>
              <col class="jobs-col-id">
              <col class="jobs-col-type">
              <col class="jobs-col-scope">
              <col class="jobs-col-created">
              <col class="jobs-col-status">
              <col class="jobs-col-result">
            </colgroup>
            <thead>
              <tr>
                <th>ID</th>
                <th>Type</th>
                <th>Scope</th>
                <th>Created</th>
                <th>Status</th>
                <th>Result</th>
              </tr>
            </thead>
            <tbody>
              ${rows.map((row) => `
                <tr>
                  <td><span class="mono-clip" title="${escapeHTML(row.id)}">${escapeHTML(shortToken(row.id, 8, 6))}</span></td>
                  <td><span class="job-type-chip" title="${escapeHTML(row.type)}">${escapeHTML(compactJobType(row.type))}</span></td>
                  <td><span class="mono-clip" title="${escapeHTML(row.scope)}">${escapeHTML(compactScope(row.scope))}</span></td>
                  <td><span class="job-date" title="${escapeHTML(row.createdFull)}">${escapeHTML(row.created)}</span></td>
                  <td class="job-status-cell">${renderJobStatusLight(row.status)}</td>
                  <td><span class="job-result-line" title="${escapeHTML(row.result || 'n/a')}">${escapeHTML(row.result || 'n/a')}</span></td>
                </tr>`).join('')}
            </tbody>
          </table>
        </div>`;
    }

    function renderJobs() {
      setTitle('Jobs');
      const rows = (state.jobs || []).map((job) => ({
        id: job.id,
        type: job.type,
        scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
        created: compactJobDate(job.created_at),
        createdFull: formatDate(job.created_at),
        status: job.status || 'queued',
        result: jobResultText(job),
      }));
      el('content').innerHTML = `
        <section class="table-card jobs-table-card">
          <div class="table-head">
            <h2>Job Queue</h2>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(rows.length))} loaded</span>
            </div>
          </div>
          ${renderJobQueue(rows)}
        </section>
        <section class="card"><h2>Concurrency rules</h2><p>Один mutating job на instance, один bootstrap/install job на node, destructive actions через lock и audit.</p></section>`;
    }

    function sleep(ms) {
      return new Promise((resolve) => window.setTimeout(resolve, ms));
    }

    function describeJobStatus(job, label) {
      const status = String(job?.status || '').toLowerCase();
      if (status === 'queued') return `${label} queued in control plane. Waiting for agent pickup.`;
      if (status === 'running' || status === 'retrying') return `${label} is running on the target node.`;
      if (status === 'succeeded') return stringValue(job?.result?.message, `${label} completed successfully.`);
      if (status === 'failed') return stringValue(job?.result?.error, job?.result?.message, `${label} failed. Check logs below.`);
      if (status === 'cancelled') return `${label} was cancelled before completion.`;
      return `${label} status: ${status || 'unknown'}.`;
    }

    function renderWatchedJob(job, logs, label, context = {}) {
      const contextRows = [
        ['Target node', context.node],
        ['Service', context.service],
        ['Strategy', context.strategy],
        ['Channel', context.channel],
        ['Job type', job?.type],
        ['Job ID', job?.id],
        ['Created', formatDate(job?.created_at)],
        ['Started', formatDate(job?.started_at)],
        ['Finished', formatDate(job?.finished_at)],
      ].filter(([, value]) => value && value !== 'n/a');
      const logRows = Array.isArray(logs) ? logs : [];
      return `
        <div class="card">
          <div class="inline-actions" style="justify-content:space-between;align-items:flex-start">
            <div>
              <div class="mini-label">${escapeHTML(label)}</div>
              <div class="metric-caption">${escapeHTML(describeJobStatus(job, label))}</div>
            </div>
            ${statusTag(job?.status || 'unknown')}
          </div>
          <div class="grid cols-2" style="margin-top:14px">
            ${contextRows.map(([title, value]) => `
              <div class="card">
                <div class="mini-label">${escapeHTML(title)}</div>
                <div class="metric-caption">${escapeHTML(String(value))}</div>
              </div>`).join('')}
          </div>
        </div>
        ${job?.result ? `
          <section class="card">
            <div class="table-head compact-head"><h3>Final Result</h3>${statusTag(job.status || 'unknown')}</div>
            ${renderActionResponse(job.result, 'Job final result')}
          </section>` : ''}
        <section class="card">
          <div class="table-head compact-head"><h3>Execution Log</h3><span class="tag">${escapeHTML(String(logRows.length))} entries</span></div>
          ${logRows.length ? `
            <div class="timeline">
              ${logRows.map((entry) => `
                <div class="timeline-item">
                  <strong>${escapeHTML(formatDate(entry.created_at))} · ${escapeHTML(String(entry.level || 'info').toUpperCase())}</strong>
                  <div class="timeline-meta">${escapeHTML(entry.message || '')}</div>
                  ${entry.payload && Object.keys(entry.payload || {}).length ? renderActionResponse(entry.payload, 'Log payload') : ''}
                </div>`).join('')}
            </div>` : '<div class="empty">No job log entries yet.</div>'}
        </section>`;
    }

    async function watchJob(jobID, targetID, label = 'Job', options = {}) {
      const target = typeof targetID === 'string' ? document.getElementById(targetID) : targetID;
      if (!target) return null;
      const attempts = Number(options.attempts || 20);
      const intervalMs = Number(options.intervalMs || 1500);
      for (let attempt = 0; attempt < attempts; attempt += 1) {
        const [job, logs] = await Promise.all([
          requestJSON(`/api/v1/jobs/${jobID}`),
          fetchJSON(`/api/v1/jobs/${jobID}/logs?limit=20`, []),
        ]);
        target.innerHTML = renderWatchedJob(job, logs, label, options.context || {});
        if (['succeeded', 'failed', 'cancelled'].includes(String(job.status || '').toLowerCase())) {
          return job;
        }
        await sleep(intervalMs);
      }
      target.innerHTML += '<div class="tag warn">job polling timed out; refresh jobs for the latest status</div>';
      return null;
    }

    async function waitForNodeDiagnostics(nodeID, targetID, label, predicate, attempts = 20) {
      const target = typeof targetID === 'string' ? document.getElementById(targetID) : targetID;
      if (!target) return null;
      for (let attempt = 0; attempt < attempts; attempt += 1) {
        const diag = await requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`);
        target.innerHTML += renderActionResponse({
          wait: label,
          attempt: attempt + 1,
          heartbeat_state: diag?.heartbeat_state,
          last_heartbeat_at: diag?.node?.last_heartbeat_at,
          agent_status: diag?.agent?.status,
          token_rotation_status: diag?.agent?.token_rotation_status,
        }, 'Diagnostics wait');
        if (predicate(diag)) {
          return diag;
        }
        await sleep(2000);
      }
      target.innerHTML += `<div class="tag warn">${escapeHTML(label)} timed out; refresh diagnostics for the latest state</div>`;
      return null;
    }

    return {
      renderJobs,
      watchJob,
      waitForNodeDiagnostics,
    };
  }

  window.MegaVPNJobWorkflows = { create: createJobWorkflows };
})(window);
