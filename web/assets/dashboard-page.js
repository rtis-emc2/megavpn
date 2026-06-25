(function (window) {
  'use strict';

  function createDashboardPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      statusTag,
      escapeHTML,
      instanceToRow,
      setPage,
    } = ctx;

    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof instanceToRow !== 'function' ||
      typeof setPage !== 'function'
    ) {
      throw new Error('MegaVPNDashboardPage requires page dependencies');
    }

    function numberValue(...values) {
      for (const value of values) {
        const num = Number(value);
        if (Number.isFinite(num)) return num;
      }
      return 0;
    }

    function pageButton(page, label, kind = 'secondary-btn') {
      return `<button class="${kind}" type="button" data-dashboard-page="${escapeHTML(page)}">${escapeHTML(label)}</button>`;
    }

    function dashboardStat(label, value, caption, status = '') {
      return `
        <div class="dashboard-stat">
          <div class="dashboard-stat-head">
            <span>${escapeHTML(label)}</span>
            ${status ? statusTag(status) : ''}
          </div>
          <strong>${escapeHTML(String(value))}</strong>
          <small>${escapeHTML(caption)}</small>
        </div>`;
    }

    function dashboardFact(label, value, note = '') {
      return `
        <div class="dashboard-fact">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(String(value))}</strong>
          ${note ? `<small>${escapeHTML(note)}</small>` : ''}
        </div>`;
    }

    function jobSummary(job) {
      if (!job) return 'No failed jobs in the latest loaded window.';
      const result = job.result || {};
      return String(job.error || result.error || result.message || job.type || 'failed job');
    }

    function renderInstancesTable(instances) {
      if (!instances.length) {
        return `
          <div class="dashboard-empty">
            <strong>No service instances yet</strong>
            <span>Instances will appear here after service packs or manual instance creation.</span>
            ${pageButton('instances', 'Open instances')}
          </div>`;
      }
      const rows = instances.slice(0, 8).map((instance) => instanceToRow(instance));
      return `
        <div class="table-wrap">
          <table class="dashboard-table">
            <thead><tr><th>Name</th><th>Service</th><th>Node</th><th>Endpoint</th><th>Status</th></tr></thead>
            <tbody>
              ${rows.map((row) => `
                <tr>
                  <td>${escapeHTML(row.name)}</td>
                  <td><span class="tag">${escapeHTML(row.service)}</span></td>
                  <td>${escapeHTML(row.node)}</td>
                  <td>${escapeHTML(row.endpoint)}</td>
                  <td>${statusTag(row.status)}</td>
                </tr>`).join('')}
            </tbody>
          </table>
        </div>`;
    }

    function renderFailedJobsTable(jobs) {
      const failed = jobs
        .filter((job) => String(job.status || '').toLowerCase() === 'failed')
        .slice(0, 5);
      if (!failed.length) {
        return '<div class="dashboard-empty compact"><strong>No failed jobs</strong><span>The latest loaded job window has no failures.</span></div>';
      }
      return `
        <div class="table-wrap">
          <table class="dashboard-table">
            <thead><tr><th>Type</th><th>Scope</th><th>Result</th></tr></thead>
            <tbody>
              ${failed.map((job) => `
                <tr>
                  <td><span class="tag danger">${escapeHTML(job.type || 'unknown')}</span></td>
                  <td>${escapeHTML(job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a')}</td>
                  <td>${escapeHTML(jobSummary(job))}</td>
                </tr>`).join('')}
            </tbody>
          </table>
        </div>`;
    }

    function render() {
      setTitle('Dashboard');
      const d = state.dashboard || {};
      const nodes = Array.isArray(state.nodes) ? state.nodes : [];
      const instanceRows = Array.isArray(state.instances) ? state.instances : [];
      const jobRows = Array.isArray(state.jobs) ? state.jobs : [];
      const clients = Array.isArray(state.clients) ? state.clients : [];
      const backhaulLinks = Array.isArray(state.backhaulLinks) ? state.backhaulLinks : [];
      const jobsQueued = numberValue(d.jobs_queued);
      const jobsRunning = numberValue(d.jobs_running);
      const jobsFailed = numberValue(d.jobs_failed, jobRows.filter((job) => String(job.status || '').toLowerCase() === 'failed').length);
      const jobsActive = jobRows.filter((job) => ['queued', 'running', 'retrying'].includes(String(job.status || '').toLowerCase())).length;
      const nodesTotal = numberValue(d.nodes_total, nodes.length);
      const nodesOnline = numberValue(d.nodes_online, nodes.filter((node) => String(node.agent_status || '').toLowerCase() === 'online').length);
      const instancesTotal = numberValue(d.instances_total, instanceRows.length);
      const instancesActive = numberValue(d.instances_active, instanceRows.filter((instance) => String(instance.status || '').toLowerCase() === 'active').length);
      const clientsTotal = numberValue(d.clients_total, clients.length);
      const clientsActive = numberValue(d.clients_active, clients.filter((client) => String(client.status || '').toLowerCase() === 'active').length);
      const backhaulActive = backhaulLinks.filter((link) => String(link.status || '').toLowerCase() === 'active').length;
      const readyStatus = state.ready?.status || 'unknown';
      const latestFailed = jobRows.find((job) => String(job.status || '').toLowerCase() === 'failed');
      el('content').innerHTML = `
        <section class="dashboard-summary">
          <div class="dashboard-status-card">
            <div class="dashboard-status-title">
              <div>
                <div class="mini-label">Control Plane</div>
                <h2>Operational overview</h2>
              </div>
              ${statusTag(readyStatus)}
            </div>
            <div class="dashboard-fact-grid">
              ${dashboardFact('Nodes online', `${nodesOnline}/${nodesTotal}`, nodesTotal ? 'agent reachable inventory' : 'no nodes enrolled')}
              ${dashboardFact('Instances active', `${instancesActive}/${instancesTotal}`, instancesTotal ? 'runtime projection' : 'not deployed yet')}
              ${dashboardFact('Active jobs', jobsActive, `${jobsQueued} queued · ${jobsRunning} running`)}
              ${dashboardFact('Backhaul links', backhaulActive, `${backhaulLinks.length} configured`)}
            </div>
          </div>
          <div class="dashboard-attention-card ${jobsFailed ? 'has-risk' : ''}">
            <div class="mini-label">Attention</div>
            <strong>${escapeHTML(String(jobsFailed))}</strong>
            <span>failed jobs in current summary</span>
            <p>${escapeHTML(jobSummary(latestFailed))}</p>
            ${pageButton('jobs', 'Open jobs', jobsFailed ? 'danger-btn' : 'secondary-btn')}
          </div>
        </section>
        <section class="dashboard-stat-grid">
          ${dashboardStat('Nodes', nodesTotal, `${nodesOnline} online`, nodesOnline === nodesTotal && nodesTotal > 0 ? 'healthy' : 'degraded')}
          ${dashboardStat('Instances', instancesTotal, `${instancesActive} active`, instancesActive > 0 ? 'active' : 'planned')}
          ${dashboardStat('Clients', clientsTotal, `${clientsActive} active`, clientsActive > 0 ? 'active' : 'planned')}
          ${dashboardStat('Jobs', jobsQueued + jobsRunning + jobsFailed || jobRows.length, `${jobsQueued} queued · ${jobsRunning} running · ${jobsFailed} failed`, jobsFailed ? 'failed' : 'healthy')}
        </section>
        <section class="dashboard-panels">
          <section class="table-card dashboard-panel">
            <div class="table-head">
              <h2>Service Instances</h2>
              <div class="table-tools">${pageButton('instances', 'Open instances')}</div>
            </div>
            ${renderInstancesTable(instanceRows)}
          </section>
          <section class="table-card dashboard-panel">
            <div class="table-head">
              <h2>Recent Failed Jobs</h2>
              <div class="table-tools">${pageButton('jobs', 'Open jobs')}</div>
            </div>
            ${renderFailedJobsTable(jobRows)}
          </section>
        </section>`;
      document.querySelectorAll('[data-dashboard-page]').forEach((button) => {
        button.addEventListener('click', () => setPage(button.dataset.dashboardPage));
      });
    }

    return { render };
  }

  window.MegaVPNDashboardPage = { create: createDashboardPage };
})(window);
