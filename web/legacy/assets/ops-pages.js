(function (window) {
  'use strict';

  function createOpsPages(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      requestJSON,
      metric,
      tableCard,
      statusTag,
      escapeHTML,
      formatDate,
      formatRelativeDate,
      nodeHeartbeatStatus,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof metric !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof formatRelativeDate !== 'function' ||
      typeof nodeHeartbeatStatus !== 'function'
    ) {
      throw new Error('MegaVPNOpsPages requires page dependencies');
    }

    const auditTabs = [
      ['all', 'All', 'complete feed'],
      ['security', 'Security', 'auth and policy'],
      ['nodes', 'Nodes', 'agent and node events'],
      ['instances', 'Instances', 'service lifecycle'],
      ['backhaul', 'Backhaul', 'node routing links'],
      ['jobs', 'Jobs', 'queue orchestration'],
    ];

    const telemetryTabs = [
      ['overview', 'Overview', 'fleet posture'],
      ['nodes', 'Nodes', 'heartbeat detail'],
      ['instances', 'Instances', 'runtime convergence'],
      ['jobs', 'Jobs', 'queue pressure'],
    ];

    function tabButton(prefix, active, key, label, caption, count = null) {
      return `
        <button class="page-tab ${active === key ? 'is-active' : ''}" type="button" data-${prefix}-tab="${escapeHTML(key)}" role="tab" aria-selected="${active === key ? 'true' : 'false'}">
          <span>${escapeHTML(label)}${count === null ? '' : ` <em>${escapeHTML(String(count))}</em>`}</span>
          <small>${escapeHTML(caption)}</small>
        </button>`;
    }

    function auditCategory(event) {
      const text = [
        event?.action,
        event?.resource_type,
        event?.summary,
      ].map((value) => String(value || '').toLowerCase()).join(' ');
      if (/auth|login|logout|password|permission|role|firewall|certificate|secret|token|tls|pki/.test(text)) return 'security';
      if (/backhaul|route|routing/.test(text)) return 'backhaul';
      if (/node|agent|bootstrap|capability/.test(text)) return 'nodes';
      if (/instance|revision|service/.test(text)) return 'instances';
      if (/job|queue|lease/.test(text)) return 'jobs';
      return 'all';
    }

    function normalizedAuditRows(events) {
      return (events || []).map((event) => ({
        created: event.created_at,
        actor: event.actor_type || event.actor || 'system',
        action: event.action || 'unknown',
        category: auditCategory(event),
        resource: `${event.resource_type || 'resource'}:${event.resource_id || 'n/a'}`,
        summary: event.summary || 'n/a',
        raw: event,
      }));
    }

    function auditCounts(rows) {
      const counts = { all: rows.length };
      rows.forEach((row) => {
        counts[row.category] = (counts[row.category] || 0) + 1;
      });
      return counts;
    }

    function filterAuditRows(rows) {
      const active = auditTabs.some(([key]) => key === state.auditTab) ? state.auditTab : 'all';
      state.auditTab = active;
      const query = String(state.auditSearch || '').trim().toLowerCase();
      const sort = state.auditSort === 'oldest' ? 'oldest' : state.auditSort === 'action' ? 'action' : 'newest';
      state.auditSort = sort;
      const filtered = rows.filter((row) => {
        if (active !== 'all' && row.category !== active) return false;
        if (!query) return true;
        return [row.created, row.actor, row.action, row.resource, row.summary]
          .some((value) => String(value || '').toLowerCase().includes(query));
      });
      filtered.sort((a, b) => {
        if (sort === 'action') return String(a.action).localeCompare(String(b.action));
        const left = new Date(a.created || 0).getTime();
        const right = new Date(b.created || 0).getTime();
        return sort === 'oldest' ? left - right : right - left;
      });
      return filtered;
    }

    function renderAuditList(rows) {
      if (!rows.length) return '<div class="empty">No audit events match the current filter.</div>';
      return `
        <div class="audit-event-list">
          ${rows.map((row) => `
            <details class="audit-event-row">
              <summary>
                <span class="audit-event-time">${escapeHTML(formatDate(row.created))}</span>
                <span class="audit-event-action">${escapeHTML(row.action)}</span>
                <span class="audit-event-resource">${escapeHTML(row.resource)}</span>
                ${statusTag(row.category)}
              </summary>
              <div class="audit-event-body">
                <div class="response-grid">
                  <div class="response-fact"><span>Actor</span><strong>${escapeHTML(row.actor)}</strong></div>
                  <div class="response-fact"><span>Category</span><strong>${escapeHTML(row.category)}</strong></div>
                  <div class="response-fact"><span>Resource</span><strong>${escapeHTML(row.resource)}</strong></div>
                  <div class="response-fact"><span>Created</span><strong>${escapeHTML(formatDate(row.created))}</strong></div>
                </div>
                <p>${escapeHTML(row.summary)}</p>
                <details class="response-raw">
                  <summary>Raw event payload</summary>
                  <div class="code-block">${escapeHTML(JSON.stringify(row.raw || {}, null, 2))}</div>
                </details>
              </div>
            </details>`).join('')}
        </div>`;
    }

    function updateAuditList(rows) {
      const active = auditTabs.some(([key]) => key === state.auditTab) ? state.auditTab : 'all';
      state.auditTab = active;
      document.querySelectorAll('[data-audit-tab]').forEach((button) => {
        const selected = button.dataset.auditTab === active;
        button.classList.toggle('is-active', selected);
        button.setAttribute('aria-selected', selected ? 'true' : 'false');
      });
      const target = document.getElementById('auditListMount');
      if (target) target.innerHTML = renderAuditList(filterAuditRows(rows));
    }

    function renderAuditContent(events) {
      if (state.page !== 'audit') return;
      const rows = normalizedAuditRows(events);
      const counts = auditCounts(rows);
      const filteredRows = filterAuditRows(rows);
      el('content').innerHTML = `
        <div class="control-page-shell audit-page-shell">
          <section class="section-card control-page-intro">
            <div>
              <h2>Audit trail</h2>
              <p>Operator actions, node orchestration and security-sensitive events. This page refreshes only when opened or when you press Refresh.</p>
            </div>
            <div class="control-page-actions">
              <span class="tag">${escapeHTML(String(rows.length))} loaded</span>
              <span class="tag">${escapeHTML(rows[0] ? formatRelativeDate(rows[0].created) : 'no events')}</span>
            </div>
          </section>
          <div class="page-tabs control-tabs" role="tablist" aria-label="Audit event categories">
            ${auditTabs.map(([key, label, caption]) => tabButton('audit', state.auditTab, key, label, caption, counts[key] || 0)).join('')}
          </div>
          <section class="table-card control-list-card">
            <div class="table-head">
              <div>
                <h2>Events</h2>
                <p class="table-subtitle">Use filters for investigation. Expand a row to inspect actor, resource and raw payload.</p>
              </div>
              <div class="table-tools ops-filter-bar">
                <input class="search-input compact-search" id="auditSearchInput" type="search" placeholder="Search action, actor, resource..." value="${escapeHTML(state.auditSearch || '')}" />
                <select id="auditSortSelect" aria-label="Audit sort">
                  <option value="newest"${state.auditSort === 'newest' ? ' selected' : ''}>Newest first</option>
                  <option value="oldest"${state.auditSort === 'oldest' ? ' selected' : ''}>Oldest first</option>
                  <option value="action"${state.auditSort === 'action' ? ' selected' : ''}>Action A-Z</option>
                </select>
              </div>
            </div>
            <div class="card-body" id="auditListMount">${renderAuditList(filteredRows)}</div>
          </section>
        </div>`;
      document.querySelectorAll('[data-audit-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          state.auditTab = button.dataset.auditTab || 'all';
          localStorage.setItem('megavpn.auditTab', state.auditTab);
          updateAuditList(rows);
        });
      });
      document.getElementById('auditSearchInput')?.addEventListener('input', (event) => {
        state.auditSearch = event.target.value || '';
        updateAuditList(rows);
      });
      document.getElementById('auditSortSelect')?.addEventListener('change', (event) => {
        state.auditSort = event.target.value || 'newest';
        localStorage.setItem('megavpn.auditSort', state.auditSort);
        updateAuditList(rows);
      });
    }

    async function renderAudit() {
      setTitle('Audit');
      el('content').innerHTML = '<section class="card"><h2>Audit Trail</h2><div class="empty">Loading recent audit events...</div></section>';
      try {
        const events = await requestJSON('/api/v1/audit?limit=200');
        if (state.page !== 'audit') return;
        renderAuditContent(events || []);
      } catch (err) {
        if (state.page !== 'audit') return;
        el('content').innerHTML = `<section class="card"><h2>Audit Trail</h2><div class="empty">Failed to load audit feed: ${escapeHTML(err.message)}</div></section>`;
      }
    }

    function telemetryActiveTab() {
      const key = telemetryTabs.some(([tab]) => tab === state.telemetryTab) ? state.telemetryTab : 'overview';
      state.telemetryTab = key;
      return key;
    }

    function bindTelemetryTabs() {
      document.querySelectorAll('[data-telemetry-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          const tab = button.dataset.telemetryTab || 'overview';
          state.telemetryTab = tab;
          localStorage.setItem('megavpn.telemetryTab', state.telemetryTab);
          document.querySelectorAll('[data-telemetry-tab]').forEach((item) => {
            const active = item.dataset.telemetryTab === tab;
            item.classList.toggle('is-active', active);
            item.setAttribute('aria-selected', active ? 'true' : 'false');
          });
          document.querySelectorAll('[data-telemetry-panel]').forEach((panel) => {
            panel.hidden = panel.dataset.telemetryPanel !== tab;
          });
        });
      });
    }

    function renderTelemetry() {
      setTitle('Telemetry');
      const activeTab = telemetryActiveTab();
      const nodes = state.nodes || [];
      const jobs = state.jobs || [];
      const instances = state.instances || [];
      const onlineNodes = nodes.filter((node) => nodeHeartbeatStatus(node) === 'online');
      const degradedNodes = nodes.filter((node) => nodeHeartbeatStatus(node) === 'degraded');
      const offlineNodes = nodes.filter((node) => nodeHeartbeatStatus(node) === 'offline');
      const runningJobs = jobs.filter((job) => ['queued', 'running', 'retrying'].includes(String(job.status || '').toLowerCase()));
      const failedJobs = jobs.filter((job) => String(job.status || '').toLowerCase() === 'failed');
      const provisioningInstances = instances.filter((instance) => ['draft', 'provisioning', 'apply'].includes(String(instance.status || '').toLowerCase()));
      const nodeRows = nodes.map((node) => ({
        name: node.name || 'n/a',
        address: node.address || 'n/a',
        heartbeat: nodeHeartbeatStatus(node),
        last: node.last_heartbeat_at,
        agent: node.agent_status || 'unknown',
        mode: node.execution_mode || 'n/a',
      }));
      const failedJobRows = failedJobs.slice(0, 15).map((job) => ({
        created: job.created_at,
        type: job.type || 'unknown',
        scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
        status: job.status || 'failed',
      }));
      const instanceRows = instances.map((instance) => ({
        name: instance.name || instance.slug || instance.id || 'n/a',
        service: instance.service_code || 'n/a',
        node: instance.node_name || instance.node_id || 'n/a',
        lifecycle: instance.status || 'unknown',
        runtime: instance.runtime_status || instance.observed_status || 'unknown',
        health: instance.health_status || 'unknown',
      }));
      const jobRows = jobs.slice(0, 25).map((job) => ({
        created: job.created_at,
        type: job.type || 'unknown',
        scope: job.scope_type && job.scope_id ? `${job.scope_type}:${job.scope_id}` : job.scope_type || 'n/a',
        status: job.status || 'unknown',
      }));
      el('content').innerHTML = `
        <div class="control-page-shell telemetry-page-shell">
          <section class="section-card control-page-intro">
            <div>
              <h2>Telemetry</h2>
              <p>Operational snapshot for nodes, instances and orchestration. Refresh manually when investigating control state.</p>
            </div>
            <div class="control-page-actions">
              <span class="tag">${escapeHTML(String(nodes.length))} nodes</span>
              <span class="tag">${escapeHTML(String(instances.length))} instances</span>
              <span class="tag">${escapeHTML(String(jobs.length))} jobs loaded</span>
            </div>
          </section>
          <div class="page-tabs control-tabs" role="tablist" aria-label="Telemetry views">
            ${telemetryTabs.map(([key, label, caption]) => tabButton('telemetry', activeTab, key, label, caption)).join('')}
          </div>
          <div class="telemetry-tab-panel" data-telemetry-panel="overview" ${activeTab === 'overview' ? '' : 'hidden'}>
            <div class="grid cols-4">
              ${metric('Nodes Online', String(onlineNodes.length), `${degradedNodes.length} degraded / ${offlineNodes.length} offline`)}
              ${metric('Running Jobs', String(runningJobs.length), 'queue pressure and active orchestration')}
              ${metric('Failed Jobs', String(failedJobs.length), 'requires operator follow-up')}
              ${metric('Pending Applies', String(provisioningInstances.length), 'instances waiting for convergence')}
            </div>
            <section class="card telemetry-note-card">
              <h2>Operational view</h2>
              <p>Telemetry aggregates currently loaded inventory state. It is designed for diagnostics, not as a constantly refreshing monitoring wall.</p>
            </section>
          </div>
          <div class="telemetry-tab-panel" data-telemetry-panel="nodes" ${activeTab === 'nodes' ? '' : 'hidden'}>
            ${tableCard('Node Heartbeats', nodeRows, [
              { title: 'Node', key: 'name' },
              { title: 'Address', key: 'address' },
              { title: 'Heartbeat', key: 'heartbeat', render: (row) => statusTag(row.heartbeat) },
              { title: 'Last Seen', key: 'last', render: (row) => formatDate(row.last) },
              { title: 'Agent', key: 'agent', render: (row) => statusTag(row.agent) },
              { title: 'Mode', key: 'mode', render: (row) => `<span class="tag">${escapeHTML(row.mode)}</span>` },
            ])}
          </div>
          <div class="telemetry-tab-panel" data-telemetry-panel="instances" ${activeTab === 'instances' ? '' : 'hidden'}>
            ${tableCard('Instance Runtime', instanceRows, [
              { title: 'Instance', key: 'name' },
              { title: 'Service', key: 'service', render: (row) => `<code>${escapeHTML(row.service)}</code>` },
              { title: 'Node', key: 'node' },
              { title: 'Lifecycle', key: 'lifecycle', render: (row) => statusTag(row.lifecycle) },
              { title: 'Runtime', key: 'runtime', render: (row) => statusTag(row.runtime) },
              { title: 'Health', key: 'health', render: (row) => statusTag(row.health) },
            ])}
          </div>
          <div class="telemetry-tab-panel" data-telemetry-panel="jobs" ${activeTab === 'jobs' ? '' : 'hidden'}>
            ${tableCard('Recent Failed Jobs', failedJobRows, [
              { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
              { title: 'Type', key: 'type', render: (row) => `<code>${escapeHTML(row.type)}</code>` },
              { title: 'Scope', key: 'scope' },
              { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
            ])}
            ${tableCard('Recent Jobs', jobRows, [
              { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
              { title: 'Type', key: 'type', render: (row) => `<code>${escapeHTML(row.type)}</code>` },
              { title: 'Scope', key: 'scope' },
              { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
            ])}
          </div>
        </div>
        `;
      bindTelemetryTabs();
    }

    return {
      renderAudit,
      renderTelemetry,
    };
  }

  window.MegaVPNOpsPages = { create: createOpsPages };
})(window);
