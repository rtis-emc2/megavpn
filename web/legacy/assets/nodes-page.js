(function (window) {
  'use strict';

  function createNodesPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      statusTag,
      escapeHTML,
      nodeExecutionLabel,
      nodeAgentChannelStatus,
      nodeLifecycleStatus,
      requestJSON,
      sendJSON,
      refresh,
      openModal,
      closeModal,
      renderActionResponse,
      hasPermission,
      watchJob,
      openCreateNodeModal,
      openNodeControlModal,
      openEditNodeModal,
      openDeleteNodeModal,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof nodeExecutionLabel !== 'function' ||
      typeof nodeAgentChannelStatus !== 'function' ||
      typeof nodeLifecycleStatus !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof watchJob !== 'function' ||
      typeof openCreateNodeModal !== 'function' ||
      typeof openNodeControlModal !== 'function' ||
      typeof openEditNodeModal !== 'function' ||
      typeof openDeleteNodeModal !== 'function'
    ) {
      throw new Error('MegaVPNNodesPage requires page dependencies');
    }

    function sleep(ms) {
      return new Promise((resolve) => setTimeout(resolve, ms));
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

    function agentIsUpdateCandidate(node) {
      if (!node || node.status === 'retired' || node.kind === 'local') return false;
      if (!targetAgentVersion()) return false;
      return compareAgentVersions(node.agent_version, targetAgentVersion()) < 0;
    }

    function agentUpdateBlockReason(node) {
      if (!node) return 'Node is not available.';
      if (node.status === 'retired') return 'Retired nodes cannot be updated.';
      if (node.kind === 'local') return 'Local control-plane nodes are updated by the control-plane release process.';
      if (!targetAgentVersion()) return 'Control Plane did not publish an agent target version.';
      if (!agentIsUpdateCandidate(node)) return '';
      if (!canBootstrapAgents()) return 'Your role is missing the node.bootstrap permission.';
      return '';
    }

    function agentNeedsUpgrade(node) {
      return agentIsUpdateCandidate(node) && !agentUpdateBlockReason(node);
    }

    function renderAgentVersion(row) {
      const current = row.agent_version || 'unknown';
      const target = targetAgentVersion();
      const update = agentIsUpdateCandidate(row);
      return `
        <div class="node-version-block">
          ${statusTag(update ? 'update available' : current)}
          <span>${escapeHTML(current)}${target ? ` -> ${escapeHTML(target)}` : ''}</span>
        </div>`;
    }

    function nodeStatusClass(node) {
      const channel = nodeAgentChannelStatus(node);
      const lifecycle = nodeLifecycleStatus(node);
      const channelState = String(channel).toLowerCase();
      const lifecycleState = String(lifecycle).toLowerCase();
      if (['offline', 'failed', 'retired'].includes(channelState) || ['offline', 'failed', 'retired'].includes(lifecycleState)) return 'danger';
      if (agentIsUpdateCandidate(node)) return 'warning';
      if (channelState === 'online' && lifecycleState === 'online') return 'healthy';
      return 'neutral';
    }

    function nodeSummaryFacts(nodes, outdatedNodes) {
      const online = nodes.filter((node) => String(nodeAgentChannelStatus(node)).toLowerCase() === 'online').length;
      const remote = nodes.filter((node) => String(node.kind || '').toLowerCase() === 'remote').length;
      return `
        <div class="node-fleet-summary">
          <div class="node-fleet-fact"><span>Total</span><strong>${escapeHTML(String(nodes.length))}</strong><small>managed nodes</small></div>
          <div class="node-fleet-fact"><span>Online</span><strong>${escapeHTML(String(online))}</strong><small>agent channel healthy</small></div>
          <div class="node-fleet-fact"><span>Remote</span><strong>${escapeHTML(String(remote))}</strong><small>agent-managed hosts</small></div>
          <div class="node-fleet-fact ${outdatedNodes.length ? 'warn' : ''}"><span>Updates</span><strong>${escapeHTML(String(outdatedNodes.length))}</strong><small>${escapeHTML(targetAgentVersion() || 'no target version')}</small></div>
        </div>`;
    }

    function nodeFact(label, value, detail = '') {
      return `
        <div class="node-card-fact">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(value || 'n/a')}</strong>
          ${detail ? `<small>${escapeHTML(detail)}</small>` : ''}
        </div>`;
    }

    function renderNodeActions(node) {
      const updateCandidate = agentIsUpdateCandidate(node);
      const needsUpgrade = agentNeedsUpgrade(node);
      const reason = agentUpdateBlockReason(node);
      return `
        <div class="node-card-actions">
          <button class="primary-btn manage-node-btn" type="button" data-node-id="${escapeHTML(node.id)}">Manage</button>
          ${updateCandidate ? `<button class="${needsUpgrade ? 'secondary-btn' : 'ghost-btn'} update-agent-btn" type="button" data-node-id="${escapeHTML(node.id)}"${needsUpgrade ? '' : ' disabled'}${reason ? ` title="${escapeHTML(reason)}"` : ''}>Update agent</button>` : ''}
          <button class="secondary-btn edit-node-btn" type="button" data-node-id="${escapeHTML(node.id)}">Edit</button>
          <button class="danger-btn delete-node-btn" type="button" data-node-id="${escapeHTML(node.id)}" data-node-name="${escapeHTML(node.name || 'node')}">Delete</button>
        </div>`;
    }

    function renderNodeCard(node) {
      const role = node.role || 'egress';
      const kind = node.kind || 'local';
      const channel = nodeAgentChannelStatus(node);
      const lifecycle = nodeLifecycleStatus(node);
      return `
        <article class="node-card ${nodeStatusClass(node)}">
          <div class="node-card-main">
            <div class="node-card-head">
              <div class="node-card-title">
                <h3>${escapeHTML(node.name || 'node')}</h3>
                <p>${escapeHTML(node.address || 'address n/a')}</p>
              </div>
              <div class="node-card-tags">
                <span class="tag">${escapeHTML(role)}</span>
                <span class="tag">${escapeHTML(kind)}</span>
              </div>
            </div>
            <div class="node-card-grid">
              ${nodeFact('Execution', nodeExecutionLabel(node.execution_mode), kind)}
              <div class="node-card-fact">
                <span>Agent channel</span>
                <strong>${statusTag(channel)}</strong>
                <small>control plane communication</small>
              </div>
              <div class="node-card-fact">
                <span>Agent version</span>
                ${renderAgentVersion(node)}
                <small>${escapeHTML(agentIsUpdateCandidate(node) ? 'upgrade recommended' : 'matches target')}</small>
              </div>
              <div class="node-card-fact">
                <span>Node state</span>
                <strong>${statusTag(lifecycle)}</strong>
                <small>scheduler lifecycle</small>
              </div>
            </div>
          </div>
          ${renderNodeActions(node)}
        </article>`;
    }

    function renderNodeList(nodes) {
      if (!nodes.length) {
        return `
          <div class="nodes-empty-state">
            <strong>No managed nodes</strong>
            <span>Add a node to start agent enrollment and service placement.</span>
            <button class="primary-btn" id="createNodeEmptyBtn" type="button">Add node</button>
          </div>`;
      }
      return `<div class="node-card-list">${nodes.map(renderNodeCard).join('')}</div>`;
    }

    function normalizeAccessMethods(data) {
      if (Array.isArray(data)) return data;
      if (Array.isArray(data?.items)) return data.items;
      if (Array.isArray(data?.methods)) return data.methods;
      return [];
    }

    function hasEnabledSSHAccess(methods) {
      return normalizeAccessMethods(methods).some((method) => (
        String(method?.method || '').toLowerCase() === 'ssh' && method?.is_enabled === true
      ));
    }

    async function ensureSSHBootstrapReady(node) {
      const methods = await requestJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/access-methods`);
      if (!hasEnabledSSHAccess(methods)) {
        throw new Error('Enabled SSH access method is required. Open Manage -> Bootstrap, save SSH access, then run Update again.');
      }
    }

    async function queueAgentUpgrade(nodeID) {
      const node = (state.nodes || []).find((item) => item.id === nodeID);
      if (!node) return;
      openModal('Update agent', node.name || 'node', '<div id="agentUpgradeResult"><span class="tag warn">checking ssh access</span></div>');
      const target = document.getElementById('agentUpgradeResult');
      try {
        const blocked = agentUpdateBlockReason(node);
        if (blocked) throw new Error(blocked);
        await ensureSSHBootstrapReady(node);
        target.innerHTML = '<span class="tag warn">queueing</span>';
        const data = await sendJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/bootstrap`, 'POST', {
          bootstrap_mode: 'ssh_bootstrap',
          reinstall_agent: true,
        });
        target.innerHTML = `
          ${renderActionResponse(data, 'Agent update queued')}
          <div class="empty">The agent version changes after the node completes bootstrap and sends the next heartbeat.</div>`;
        if (data?.job?.id) {
          await watchJob(data.job.id, target, 'Agent update', {
            attempts: 120,
            intervalMs: 2000,
            context: {
              node: node.name || node.id,
              target_version: targetAgentVersion() || 'n/a',
            },
          });
        }
        target.innerHTML += '<div class="modal-actions"><button class="primary-btn" id="closeAgentUpgradeBtn" type="button">Close</button></div>';
        document.getElementById('closeAgentUpgradeBtn')?.addEventListener('click', closeModal);
        await refresh();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function renderBulkAgentUpgradeResults(results, phase = 'queueing') {
      const counts = results.reduce((acc, item) => {
        const status = String(item.status || 'waiting').toLowerCase();
        acc[status] = (acc[status] || 0) + 1;
        return acc;
      }, {});
      return renderActionResponse({
        phase,
        target_version: targetAgentVersion() || 'n/a',
        waiting: counts.waiting || 0,
        checking: counts.checking || 0,
        queueing: counts.queueing || 0,
        queued: counts.queued || 0,
        running: counts.running || 0,
        retrying: counts.retrying || 0,
        succeeded: counts.succeeded || 0,
        failed: counts.failed || 0,
        skipped: counts.skipped || 0,
        results: results.map((item) => ({
          node: item.node,
          status: item.status,
          job_id: item.job_id || '',
          reason: item.reason || item.error || '',
        })),
      }, 'Agent update queue');
    }

    async function runLimited(items, limit, worker) {
      let cursor = 0;
      const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
        while (cursor < items.length) {
          const index = cursor;
          cursor += 1;
          await worker(items[index], index);
        }
      });
      await Promise.all(workers);
    }

    async function pollBulkAgentUpgradeJobs(results, target) {
      const terminal = new Set(['succeeded', 'failed', 'cancelled']);
      const tracked = () => results.filter((item) => item.job_id && !terminal.has(String(item.status || '').toLowerCase()));
      for (let attempt = 0; attempt < 120 && tracked().length; attempt += 1) {
        await Promise.all(tracked().map(async (item) => {
          try {
            const job = await requestJSON(`/api/v1/jobs/${encodeURIComponent(item.job_id)}`);
            item.status = String(job.status || item.status || 'queued');
            if (job.result?.error || job.result?.message) {
              item.reason = job.result.error || job.result.message;
            }
          } catch (err) {
            item.status = 'failed';
            item.error = err.message;
          }
        }));
        target.innerHTML = renderBulkAgentUpgradeResults(results, 'watching jobs');
        if (!tracked().length) break;
        await sleep(2000);
      }
      if (tracked().length) {
        target.innerHTML += '<div class="tag warn">job polling timed out; refresh Nodes or Jobs for the latest status</div>';
      }
    }

    async function queueAllAgentUpgrades(nodes) {
      const targets = nodes.filter(agentIsUpdateCandidate);
      openModal('Update all agents', `${targets.length} node(s)`, '<div id="bulkAgentUpgradeResult"><span class="tag warn">queueing</span></div>', { wide: true });
      const target = document.getElementById('bulkAgentUpgradeResult');
      const results = targets.map((node) => ({ node: node.name || node.id, node_id: node.id, status: 'waiting' }));
      target.innerHTML = renderBulkAgentUpgradeResults(results, 'checking nodes');
      await runLimited(targets, 3, async (node, index) => {
        const row = results[index];
        try {
          row.status = 'checking';
          target.innerHTML = renderBulkAgentUpgradeResults(results, 'checking ssh access');
          const blocked = agentUpdateBlockReason(node);
          if (blocked) {
            row.status = 'skipped';
            row.reason = blocked;
            target.innerHTML = renderBulkAgentUpgradeResults(results, 'checking ssh access');
            return;
          }
          await ensureSSHBootstrapReady(node);
          row.status = 'queueing';
          target.innerHTML = renderBulkAgentUpgradeResults(results, 'queueing jobs');
          const data = await sendJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/bootstrap`, 'POST', {
            bootstrap_mode: 'ssh_bootstrap',
            reinstall_agent: true,
          });
          row.status = data?.job?.status || 'queued';
          row.job_id = data?.job?.id || '';
        } catch (err) {
          row.status = 'failed';
          row.error = err.message;
        }
        target.innerHTML = renderBulkAgentUpgradeResults(results, 'queueing jobs');
      });
      await pollBulkAgentUpgradeJobs(results, target);
      target.innerHTML += '<div class="modal-actions"><button class="primary-btn" id="closeBulkAgentUpgradeBtn" type="button">Close</button></div>';
      document.getElementById('closeBulkAgentUpgradeBtn')?.addEventListener('click', closeModal);
      await refresh();
    }

    function render() {
      setTitle('Nodes');
      const rows = Array.isArray(state.nodes) ? state.nodes.filter((node) => node.status !== 'retired') : [];
      const outdatedNodes = rows.filter(agentIsUpdateCandidate);
      const upgradeableNodes = outdatedNodes.filter(agentNeedsUpgrade);
      const updateAllReason = outdatedNodes.length && !upgradeableNodes.length
        ? (outdatedNodes.map(agentUpdateBlockReason).find(Boolean) || 'No agent update can be queued from this session.')
        : '';
      const actions = `
        <div class="node-page-actions">
          <button class="secondary-btn" id="updateAllAgentsBtn" type="button"${upgradeableNodes.length ? '' : ' disabled'}${updateAllReason ? ` title="${escapeHTML(updateAllReason)}"` : ''}>Update all agents</button>
          <button class="primary-btn" id="createNodeBtn" type="button">Add node</button>
        </div>`;
      el('content').innerHTML = `
        <section class="nodes-workspace">
          <div class="nodes-workspace-head">
            <div>
              <div class="eyebrow">Node Fleet</div>
              <h2>Managed Nodes</h2>
            </div>
            ${actions}
          </div>
          ${nodeSummaryFacts(rows, outdatedNodes)}
          ${renderNodeList(rows)}
        </section>`;
      document.getElementById('createNodeBtn')?.addEventListener('click', openCreateNodeModal);
      document.getElementById('createNodeEmptyBtn')?.addEventListener('click', openCreateNodeModal);
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
