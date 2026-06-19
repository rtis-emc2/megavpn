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

    async function renderAudit() {
      setTitle('Audit');
      el('content').innerHTML = '<section class="card"><h2>Audit Trail</h2><div class="empty">Loading recent audit events...</div></section>';
      try {
        const events = await requestJSON('/api/v1/audit?limit=200');
        if (state.page !== 'audit') return;
        const rows = (events || []).map((event) => ({
          created: event.created_at,
          actor: event.actor_type || 'system',
          action: event.action || 'unknown',
          resource: `${event.resource_type || 'resource'}:${event.resource_id || 'n/a'}`,
          summary: event.summary || 'n/a',
        }));
        el('content').innerHTML = `
          <div class="grid cols-4">
            ${metric('Events', String(rows.length), 'recent audit records')}
            ${metric('Actors', String(new Set(rows.map((row) => row.actor)).size), 'distinct actor types')}
            ${metric('Resources', String(new Set(rows.map((row) => row.resource)).size), 'resource targets in the feed')}
            ${metric('Latest', rows[0] ? formatRelativeDate(rows[0].created) : 'n/a', 'most recent event')}
          </div>
          ${tableCard('Audit Events', rows, [
            { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
            { title: 'Actor', key: 'actor', render: (row) => `<span class="tag">${escapeHTML(row.actor)}</span>` },
            { title: 'Action', key: 'action', render: (row) => `<code>${escapeHTML(row.action)}</code>` },
            { title: 'Resource', key: 'resource' },
            { title: 'Summary', key: 'summary' },
          ])}
          <section class="card"><h2>Scope</h2><p>Лента строится напрямую из <code>audit_events</code> и уже включает operator actions, job orchestration, share-link publishing и service access rotations.</p></section>`;
      } catch (err) {
        if (state.page !== 'audit') return;
        el('content').innerHTML = `<section class="card"><h2>Audit Trail</h2><div class="empty">Failed to load audit feed: ${escapeHTML(err.message)}</div></section>`;
      }
    }

    function renderTelemetry() {
      setTitle('Telemetry');
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
      el('content').innerHTML = `
        <div class="grid cols-4">
          ${metric('Nodes Online', String(onlineNodes.length), `${degradedNodes.length} degraded / ${offlineNodes.length} offline`)}
          ${metric('Running Jobs', String(runningJobs.length), 'queue pressure and active orchestration')}
          ${metric('Failed Jobs', String(failedJobs.length), 'requires operator follow-up')}
          ${metric('Pending Applies', String(provisioningInstances.length), 'instances waiting for convergence')}
        </div>
        ${tableCard('Node Heartbeats', nodeRows, [
          { title: 'Node', key: 'name' },
          { title: 'Address', key: 'address' },
          { title: 'Heartbeat', key: 'heartbeat', render: (row) => statusTag(row.heartbeat) },
          { title: 'Last Seen', key: 'last', render: (row) => formatDate(row.last) },
          { title: 'Agent', key: 'agent', render: (row) => statusTag(row.agent) },
          { title: 'Mode', key: 'mode', render: (row) => `<span class="tag">${escapeHTML(row.mode)}</span>` },
        ])}
        ${tableCard('Recent Failed Jobs', failedJobRows, [
          { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
          { title: 'Type', key: 'type', render: (row) => `<code>${escapeHTML(row.type)}</code>` },
          { title: 'Scope', key: 'scope' },
          { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
        ])}
        <section class="card"><h2>Operational View</h2><p>Telemetry page агрегирует текущие heartbeat signals, instance convergence state и job failures без ручного перехода по diagnostics и job logs.</p></section>`;
    }

    return {
      renderAudit,
      renderTelemetry,
    };
  }

  window.MegaVPNOpsPages = { create: createOpsPages };
})(window);
