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

    const agentJobTypes = new Set([
      'node.inventory',
      'node.inventory.sync',
      'node.services.discover',
      'node.capability.install',
      'node.capability.verify',
      'node.channel.probe',
      'node.agent.rotate_token',
      'node.emergency_cleanup',
      'node.reboot',
      'node.backhaul.apply',
      'node.backhaul.probe',
      'node.backhaul.cleanup',
      'node.route_policy.apply',
      'node.route_policy.cleanup',
      'node.firewall.preview',
      'node.firewall.apply',
      'node.firewall.observe',
      'node.firewall.disable',
      'instance.restart',
      'instance.apply',
      'instance.start',
      'instance.stop',
      'instance.enable',
      'instance.disable',
      'instance.diagnose',
      'instance.delete',
    ]);

    function isAgentJob(job) {
      return agentJobTypes.has(String(job?.type || '').trim());
    }

    function instanceForJob(job) {
      const instanceID = String(job?.instance_id || '').trim();
      if (!instanceID) return null;
      return (state.instances || []).find((instance) => instance.id === instanceID) || null;
    }

    function nodeForJob(job) {
      const directNodeID = String(job?.node_id || '').trim();
      const instance = directNodeID ? null : instanceForJob(job);
      const nodeID = directNodeID || String(instance?.node_id || '').trim();
      if (!nodeID) return null;
      return (state.nodes || []).find((node) => node.id === nodeID) || { id: nodeID };
    }

    function nodeJobContextText(job) {
      const node = nodeForJob(job);
      if (!node) return '';
      const parts = [
        node.name || shortToken(node.id, 8, 4),
        node.agent_status ? `agent ${node.agent_status}` : '',
        node.status ? `node ${node.status}` : '',
      ];
      const lastSeen = node.agent_last_seen_at || node.last_heartbeat_at;
      if (lastSeen) parts.push(`last seen ${formatDate(lastSeen)}`);
      return parts.filter(Boolean).join(' · ');
    }

    function activeJobProgressText(job) {
      const status = String(job?.status || '').trim().toLowerCase();
      if (!['queued', 'running', 'retrying'].includes(status)) return '';
      const context = nodeJobContextText(job);
      if (!isAgentJob(job)) {
        return status === 'running'
          ? 'control-plane worker is running this job'
          : 'waiting for control-plane worker';
      }
      if (status === 'running') {
        const lease = job?.locked_until ? ` · lease until ${formatDate(job.locked_until)}` : '';
        const owner = job?.locked_by ? `claimed by ${job.locked_by}` : 'claimed by node agent';
        return `${owner}; waiting for agent result${lease}${context ? ` · ${context}` : ''}`;
      }
      const verb = status === 'retrying' ? 'waiting for node agent retry poll' : 'waiting for node agent poll';
      return `${verb}${context ? ` · ${context}` : ''}`;
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
        result.message,
        activeJobProgressText(job)
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

    const jobTabs = [
      ['active', 'In work', 'queued, running, retrying'],
      ['failed', 'Errors', 'failed jobs'],
      ['completed', 'Completed', 'successful jobs'],
      ['all', 'All', 'full loaded queue'],
    ];

    function jobBucket(status) {
      const value = String(status || '').toLowerCase();
      if (['queued', 'running', 'retrying'].includes(value)) return 'active';
      if (value === 'failed') return 'failed';
      if (['succeeded', 'cancelled'].includes(value)) return 'completed';
      return 'active';
    }

    function selectedJobsTab() {
      const tab = jobTabs.some(([key]) => key === state.jobsTab) ? state.jobsTab : 'active';
      state.jobsTab = tab;
      return tab;
    }

    function jobCounts(rows) {
      const counts = { all: rows.length, active: 0, failed: 0, completed: 0 };
      rows.forEach((row) => {
        const bucket = jobBucket(row.status);
        counts[bucket] = (counts[bucket] || 0) + 1;
      });
      return counts;
    }

    function filterJobRows(rows) {
      const active = selectedJobsTab();
      const query = String(state.jobsSearch || '').trim().toLowerCase();
      const sort = state.jobsSort === 'oldest' ? 'oldest' : state.jobsSort === 'type' ? 'type' : 'newest';
      state.jobsSort = sort;
      const filtered = rows.filter((row) => {
        if (active !== 'all' && jobBucket(row.status) !== active) return false;
        if (!query) return true;
        return [row.id, row.type, row.scope, row.createdFull, row.status, row.result, row.nodeContext, row.lockContext]
          .some((value) => String(value || '').toLowerCase().includes(query));
      });
      filtered.sort((a, b) => {
        if (sort === 'type') return String(a.type || '').localeCompare(String(b.type || ''));
        const left = new Date(a.createdAt || 0).getTime();
        const right = new Date(b.createdAt || 0).getTime();
        return sort === 'oldest' ? left - right : right - left;
      });
      return filtered;
    }

    function renderJobTabs(active, counts) {
      return `
        <div class="page-tabs control-tabs jobs-tabs" role="tablist" aria-label="Job queue filters">
          ${jobTabs.map(([key, label, caption]) => `
            <button class="page-tab ${active === key ? 'is-active' : ''}" type="button" data-jobs-tab="${escapeHTML(key)}" role="tab" aria-selected="${active === key ? 'true' : 'false'}">
              <span>${escapeHTML(label)} <em>${escapeHTML(String(counts[key] || 0))}</em></span>
              <small>${escapeHTML(caption)}</small>
            </button>`).join('')}
        </div>`;
    }

    function renderJobQueue(rows) {
      if (!rows.length) return '<div class="empty">No jobs match current filters.</div>';
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
                  <td>
                    <details class="job-result-details">
                      <summary><span class="job-result-line" title="${escapeHTML(row.result || 'n/a')}">${escapeHTML(row.result || 'n/a')}</span></summary>
                      <div class="job-result-body">
                        <div><span>ID</span><strong>${escapeHTML(row.id || 'n/a')}</strong></div>
                        <div><span>Scope</span><strong>${escapeHTML(row.scope || 'n/a')}</strong></div>
                        <div><span>Node</span><strong>${escapeHTML(row.nodeContext || 'n/a')}</strong></div>
                        <div><span>Created</span><strong>${escapeHTML(row.createdFull || 'n/a')}</strong></div>
                        <div><span>Status</span><strong>${escapeHTML(row.status || 'unknown')}</strong></div>
                        <div><span>Lock</span><strong>${escapeHTML(row.lockContext || 'n/a')}</strong></div>
                        <p>${escapeHTML(row.result || 'No result payload yet.')}</p>
                      </div>
                    </details>
                  </td>
                </tr>`).join('')}
            </tbody>
          </table>
        </div>`;
    }

    function updateJobQueue(rows) {
      const active = selectedJobsTab();
      document.querySelectorAll('[data-jobs-tab]').forEach((button) => {
        const selected = button.dataset.jobsTab === active;
        button.classList.toggle('is-active', selected);
        button.setAttribute('aria-selected', selected ? 'true' : 'false');
      });
      const target = document.getElementById('jobsQueueMount');
      if (target) target.innerHTML = renderJobQueue(filterJobRows(rows));
    }

    function renderJobs() {
      setTitle('Jobs');
      const rows = (state.jobs || []).map((job) => ({
        id: job.id,
        type: job.type,
        scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
        created: compactJobDate(job.created_at),
        createdFull: formatDate(job.created_at),
        createdAt: job.created_at,
        status: job.status || 'queued',
        result: jobResultText(job),
        nodeContext: nodeJobContextText(job),
        lockContext: [job.locked_by || '', job.locked_until ? `until ${formatDate(job.locked_until)}` : ''].filter(Boolean).join(' · '),
      }));
      const activeTab = selectedJobsTab();
      const counts = jobCounts(rows);
      const filteredRows = filterJobRows(rows);
      el('content').innerHTML = `
        <div class="control-page-shell jobs-page-shell">
          <section class="section-card control-page-intro">
            <div>
              <h2>Job queue</h2>
              <p>Operational queue with explicit filters. The page refreshes on navigation or the Refresh button, not every few seconds while you inspect rows.</p>
            </div>
            <div class="control-page-actions">
              <span class="tag">${escapeHTML(String(rows.length))} loaded</span>
              <span class="tag warn">${escapeHTML(String(counts.active || 0))} active</span>
              <span class="tag danger">${escapeHTML(String(counts.failed || 0))} failed</span>
            </div>
          </section>
          ${renderJobTabs(activeTab, counts)}
          <section class="table-card jobs-table-card">
            <div class="table-head">
              <div>
                <h2>Jobs</h2>
                <p class="table-subtitle">Filter by state, search by type/scope/result, and expand a result cell for full context.</p>
              </div>
              <div class="table-tools ops-filter-bar">
                <input class="search-input compact-search" id="jobsSearchInput" type="search" placeholder="Search jobs..." value="${escapeHTML(state.jobsSearch || '')}" />
                <select id="jobsSortSelect" aria-label="Job sort">
                  <option value="newest"${state.jobsSort === 'newest' ? ' selected' : ''}>Newest first</option>
                  <option value="oldest"${state.jobsSort === 'oldest' ? ' selected' : ''}>Oldest first</option>
                  <option value="type"${state.jobsSort === 'type' ? ' selected' : ''}>Type A-Z</option>
                </select>
              </div>
            </div>
            <div id="jobsQueueMount">${renderJobQueue(filteredRows)}</div>
          </section>
          <section class="card control-note-card"><h2>Concurrency rules</h2><p>One mutating job per instance, one bootstrap/install job per node, destructive actions through explicit locks and audit events.</p></section>
        </div>`;
      document.querySelectorAll('[data-jobs-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          state.jobsTab = button.dataset.jobsTab || 'active';
          localStorage.setItem('megavpn.jobsTab', state.jobsTab);
          updateJobQueue(rows);
        });
      });
      document.getElementById('jobsSearchInput')?.addEventListener('input', (event) => {
        state.jobsSearch = event.target.value || '';
        updateJobQueue(rows);
      });
      document.getElementById('jobsSortSelect')?.addEventListener('change', (event) => {
        state.jobsSort = event.target.value || 'newest';
        localStorage.setItem('megavpn.jobsSort', state.jobsSort);
        updateJobQueue(rows);
      });
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
