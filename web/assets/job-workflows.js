(function (window) {
  'use strict';

  function createJobWorkflows(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      requestJSON,
      fetchJSON,
      tableCard,
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
      typeof tableCard !== 'function' ||
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

    function renderJobs() {
      setTitle('Jobs');
      const rows = (state.jobs || []).map((job) => ({
        id: job.id,
        type: job.type,
        scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
        created: formatDate(job.created_at),
        status: job.status || 'queued',
        result: jobResultText(job),
      }));
      el('content').innerHTML = `
        ${tableCard('Job Queue', rows, [
          { title: 'ID', key: 'id' },
          { title: 'Type', key: 'type', render: (r) => `<span class="tag">${escapeHTML(r.type)}</span>` },
          { title: 'Scope', key: 'scope' },
          { title: 'Created', key: 'created' },
          { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
          { title: 'Result', key: 'result' },
        ])}
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
