(function (window) {
  'use strict';

  function createDashboardPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      metric,
      tableCard,
      statusTag,
      escapeHTML,
      instanceToRow,
      setPage,
    } = ctx;

    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof metric !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof instanceToRow !== 'function' ||
      typeof setPage !== 'function'
    ) {
      throw new Error('MegaVPNDashboardPage requires page dependencies');
    }

    function render() {
      setTitle('Dashboard');
      const d = state.dashboard || {};
      const instanceRows = Array.isArray(state.instances) ? state.instances : [];
      const jobRows = Array.isArray(state.jobs) ? state.jobs : [];
      const jobsTotal = Number(d.jobs_queued || 0) + Number(d.jobs_running || 0) + Number(d.jobs_failed || 0);
      el('content').innerHTML = `
        <div class="grid cols-4">
          ${metric('Nodes', d.nodes_total ?? state.nodes.length, `${Number(d.nodes_online || 0)} online`, 'nodes')}
          ${metric('Instances', d.instances_total ?? instanceRows.length, `${Number(d.instances_active || 0)} active`, 'instances')}
          ${metric('Clients', d.clients_total ?? state.clients.length, `${Number(d.clients_active || 0)} active`, 'clients')}
          ${metric('Jobs', jobsTotal || jobRows.length, `${Number(d.jobs_queued || 0)} queued · ${Number(d.jobs_running || 0)} running · ${Number(d.jobs_failed || 0)} failed`, 'jobs')}
        </div>
        ${tableCard('Service Instances', instanceRows.slice(0, 8).map((instance) => instanceToRow(instance)), [
          { title: 'Name', key: 'name' },
          { title: 'Service', key: 'service', render: (r) => `<span class="tag">${escapeHTML(r.service)}</span>` },
          { title: 'Node', key: 'node' },
          { title: 'Endpoint', key: 'endpoint' },
          { title: 'Status', key: 'status', render: (r) => statusTag(r.status) },
        ], '<button class="secondary-btn" type="button" id="dashboardInstancesBtn">Open instances</button>')}`;
      document.querySelectorAll('.dashboard-nav-tile').forEach((tile) => {
        const go = () => setPage(tile.dataset.pageTarget);
        tile.addEventListener('click', go);
        tile.addEventListener('keydown', (event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            go();
          }
        });
      });
      document.getElementById('dashboardInstancesBtn')?.addEventListener('click', () => setPage('instances'));
    }

    return { render };
  }

  window.MegaVPNDashboardPage = { create: createDashboardPage };
})(window);
