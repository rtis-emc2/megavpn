(function (window) {
  'use strict';

  function createNodesPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      tableCard,
      statusTag,
      escapeHTML,
      nodeExecutionLabel,
      nodeAgentChannelStatus,
      nodeLifecycleStatus,
      sendJSON,
      refresh,
      openModal,
      closeModal,
      renderActionResponse,
      hasPermission,
      openCreateNodeModal,
      openNodeControlModal,
      openEditNodeModal,
      openDeleteNodeModal,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof nodeExecutionLabel !== 'function' ||
      typeof nodeAgentChannelStatus !== 'function' ||
      typeof nodeLifecycleStatus !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof openCreateNodeModal !== 'function' ||
      typeof openNodeControlModal !== 'function' ||
      typeof openEditNodeModal !== 'function' ||
      typeof openDeleteNodeModal !== 'function'
    ) {
      throw new Error('MegaVPNNodesPage requires page dependencies');
    }

    function targetAgentVersion() {
      return String(state.versionInfo?.agent_target_version || state.versionInfo?.version || '').trim();
    }

    function versionParts(value) {
      const match = String(value || '').trim().match(/(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:\.(\d+))?/);
      if (!match) return null;
      return match.slice(1).map((item) => Number(item || 0));
    }

    function compareAgentVersions(current, target) {
      current = String(current || '').trim();
      target = String(target || '').trim();
      if (!target || current === target) return 0;
      if (!current || ['unknown', 'n/a', 'dev', 'alpha'].includes(current.toLowerCase())) return -1;
      const currentParts = versionParts(current);
      const targetParts = versionParts(target);
      if (!currentParts || !targetParts) return current.localeCompare(target);
      for (let index = 0; index < Math.max(currentParts.length, targetParts.length); index += 1) {
        const left = Number(currentParts[index] || 0);
        const right = Number(targetParts[index] || 0);
        if (left < right) return -1;
        if (left > right) return 1;
      }
      return 0;
    }

    function canBootstrapAgents() {
      return typeof hasPermission !== 'function' || hasPermission('node.bootstrap');
    }

    function agentNeedsUpgrade(node) {
      if (!node || node.status === 'retired' || node.kind === 'local') return false;
      if (!canBootstrapAgents()) return false;
      return compareAgentVersions(node.agent_version, targetAgentVersion()) < 0;
    }

    function renderAgentVersion(row) {
      const current = row.agent_version || 'unknown';
      const target = targetAgentVersion();
      const update = agentNeedsUpgrade(row);
      return `
        <div class="stacked-cell">
          ${statusTag(update ? 'update available' : current)}
          <span class="metric-caption">${escapeHTML(current)}${target ? ` -> ${escapeHTML(target)}` : ''}</span>
        </div>`;
    }

    async function queueAgentUpgrade(nodeID) {
      const node = (state.nodes || []).find((item) => item.id === nodeID);
      if (!node) return;
      openModal('Update agent', node.name || 'node', '<div id="agentUpgradeResult"><span class="tag warn">queueing</span></div>');
      const target = document.getElementById('agentUpgradeResult');
      try {
        const data = await sendJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/bootstrap`, 'POST', {
          bootstrap_mode: 'ssh_bootstrap',
          reinstall_agent: true,
        });
        target.innerHTML = `
          ${renderActionResponse(data, 'Agent update queued')}
          <div class="empty">The agent version changes after the node completes bootstrap and sends the next heartbeat.</div>
          <div class="modal-actions"><button class="primary-btn" id="closeAgentUpgradeBtn" type="button">Close</button></div>`;
        document.getElementById('closeAgentUpgradeBtn')?.addEventListener('click', closeModal);
        await refresh();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function queueAllAgentUpgrades(nodes) {
      const targets = nodes.filter(agentNeedsUpgrade);
      openModal('Update all agents', `${targets.length} node(s)`, '<div id="bulkAgentUpgradeResult"><span class="tag warn">queueing</span></div>', { wide: true });
      const target = document.getElementById('bulkAgentUpgradeResult');
      const results = [];
      for (const node of targets) {
        try {
          const data = await sendJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/bootstrap`, 'POST', {
            bootstrap_mode: 'ssh_bootstrap',
            reinstall_agent: true,
          });
          results.push({ node: node.name || node.id, status: 'queued', job_id: data?.job?.id || '' });
        } catch (err) {
          results.push({ node: node.name || node.id, status: 'failed', error: err.message });
        }
        target.innerHTML = renderActionResponse({ target_version: targetAgentVersion(), results }, 'Agent update queue');
      }
      target.innerHTML += '<div class="modal-actions"><button class="primary-btn" id="closeBulkAgentUpgradeBtn" type="button">Close</button></div>';
      document.getElementById('closeBulkAgentUpgradeBtn')?.addEventListener('click', closeModal);
      await refresh();
    }

    function render() {
      setTitle('Nodes');
      const rows = Array.isArray(state.nodes) ? state.nodes.filter((node) => node.status !== 'retired') : [];
      const outdatedNodes = rows.filter(agentNeedsUpgrade);
      const actions = `
        <div class="inline-actions">
          <button class="secondary-btn" id="updateAllAgentsBtn" type="button"${outdatedNodes.length ? '' : ' disabled'}>Update all agents</button>
          <button class="secondary-btn" id="createNodeBtn" type="button">Add node</button>
        </div>`;
      el('content').innerHTML = `
        ${tableCard('Managed Nodes', rows, [
          { title: 'Name', key: 'name' },
          { title: 'Role', key: 'role', render: (row) => `<span class="tag">${escapeHTML(row.role || 'egress')}</span>` },
          { title: 'Kind', key: 'kind', render: (row) => `<span class="tag">${escapeHTML(row.kind || 'local')}</span>` },
          { title: 'Address', key: 'address' },
          { title: 'Execution', key: 'execution_mode', render: (row) => escapeHTML(nodeExecutionLabel(row.execution_mode)) },
          { title: 'Agent channel', key: 'agent_status', render: (row) => statusTag(nodeAgentChannelStatus(row)) },
          { title: 'Agent version', key: 'agent_version', render: renderAgentVersion },
          { title: 'Node state', key: 'status', render: (row) => statusTag(nodeLifecycleStatus(row)) },
          { title: 'Actions', key: 'id', render: (row) => `
            <div class="inline-actions">
              ${agentNeedsUpgrade(row) ? `<button class="primary-btn update-agent-btn" type="button" data-node-id="${escapeHTML(row.id)}">Update</button>` : ''}
              <button class="secondary-btn manage-node-btn" type="button" data-node-id="${escapeHTML(row.id)}">Manage</button>
              <button class="secondary-btn edit-node-btn" type="button" data-node-id="${escapeHTML(row.id)}">Edit</button>
              <button class="danger-btn delete-node-btn" type="button" data-node-id="${escapeHTML(row.id)}" data-node-name="${escapeHTML(row.name || 'node')}">Delete</button>
            </div>` },
        ], actions)}`;
      document.getElementById('createNodeBtn')?.addEventListener('click', openCreateNodeModal);
      document.getElementById('updateAllAgentsBtn')?.addEventListener('click', () => queueAllAgentUpgrades(rows));
      document.querySelectorAll('.update-agent-btn').forEach((button) => {
        button.addEventListener('click', () => queueAgentUpgrade(button.dataset.nodeId));
      });
      document.querySelectorAll('.manage-node-btn').forEach((button) => {
        button.addEventListener('click', () => openNodeControlModal(button.dataset.nodeId));
      });
      document.querySelectorAll('.edit-node-btn').forEach((button) => {
        button.addEventListener('click', () => openEditNodeModal(button.dataset.nodeId));
      });
      document.querySelectorAll('.delete-node-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteNodeModal(button.dataset.nodeId, button.dataset.nodeName));
      });
    }

    return { render };
  }

  window.MegaVPNNodesPage = { create: createNodesPage };
})(window);
