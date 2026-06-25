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
      requestJSON,
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
      typeof requestJSON !== 'function' ||
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
        <div class="stacked-cell">
          ${statusTag(update ? 'update available' : current)}
          <span class="metric-caption">${escapeHTML(current)}${target ? ` -> ${escapeHTML(target)}` : ''}</span>
        </div>`;
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

    function selectEnabledSSHAccess(methods) {
      return normalizeAccessMethods(methods).find((method) => (
        String(method?.method || '').toLowerCase() === 'ssh' && method?.is_enabled === true
      )) || null;
    }

    function sshTarget(method) {
      const user = String(method?.ssh_user || '').trim();
      const host = String(method?.ssh_host || '').trim();
      if (!user || !host) return '';
      return `${user}@${host}`;
    }

    function shellSingleQuote(value) {
      return `'${String(value).replace(/'/g, `'\\''`)}'`;
    }

    function sshPort(method) {
      const port = Number(method?.ssh_port || 22);
      if (!Number.isInteger(port) || port < 1 || port > 65535) return 22;
      return port;
    }

    function sshCommand(method) {
      const target = sshTarget(method);
      if (!target) return '';
      return `ssh -p ${sshPort(method)} -- ${shellSingleQuote(target)}`;
    }

    function sshURL(method) {
      const user = encodeURIComponent(String(method?.ssh_user || '').trim());
      const host = String(method?.ssh_host || '').trim();
      const port = sshPort(method);
      if (!user || !host) return '';
      const normalizedHost = host.startsWith('[') && host.endsWith(']')
        ? host
        : (host.includes(':') ? `[${host}]` : host);
      return `ssh://${user}@${normalizedHost}${port !== 22 ? `:${port}` : ''}`;
    }

    async function copySSHCommand(command, targetID = 'sshConsoleResult') {
      const target = document.getElementById(targetID);
      try {
        await navigator.clipboard.writeText(command);
        if (target) target.innerHTML = '<span class="tag ok">command copied</span>';
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message || 'copy failed')}</span>`;
      }
    }

    async function openSSHConsoleModal(nodeID) {
      const node = (state.nodes || []).find((item) => item.id === nodeID);
      if (!node) return;
      openModal(`SSH: ${node.name || 'node'}`, 'Node access launcher', '<div id="sshConsoleBody"><span class="tag warn">loading access methods</span></div>', { wide: true });
      const body = document.getElementById('sshConsoleBody');
      try {
        const methods = await requestJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/access-methods`);
        const method = selectEnabledSSHAccess(methods);
        if (!method) {
          body.innerHTML = `
            <div class="ssh-console">
              <div class="ssh-console-screen">
                <div class="ssh-console-line muted">$ ssh ${escapeHTML(node.name || node.address || 'node')}</div>
                <div class="ssh-console-line error">enabled SSH access method is not configured</div>
              </div>
              <div class="ssh-console-side">
                <div class="fact-card">
                  <div class="mini-label">Next step</div>
                  <div class="metric-caption strong">Configure SSH access in node bootstrap settings.</div>
                  <p>SSH secrets are not exposed to the browser. The control plane stores them only for bootstrap jobs.</p>
                </div>
                <div class="inline-actions">
                  <button class="primary-btn" id="sshOpenManageBtn" type="button">Open Manage</button>
                  <button class="secondary-btn" id="sshCloseBtn" type="button">Close</button>
                </div>
              </div>
            </div>`;
          document.getElementById('sshOpenManageBtn')?.addEventListener('click', () => {
            closeModal();
            openNodeControlModal(node.id);
          });
          document.getElementById('sshCloseBtn')?.addEventListener('click', closeModal);
          return;
        }
        const command = sshCommand(method);
        if (!command) {
          throw new Error('Enabled SSH access is missing ssh_user or ssh_host.');
        }
        const url = sshURL(method);
        const fingerprint = method.ssh_host_key_sha256 || 'not pinned';
        body.innerHTML = `
          <div class="ssh-console">
            <div class="ssh-console-screen" role="region" aria-label="SSH command">
              <div class="ssh-console-line muted"># ${escapeHTML(node.name || node.id)} · ${escapeHTML(node.address || 'no node address')}</div>
              <div class="ssh-console-line">$ ${escapeHTML(command)}</div>
              <div class="ssh-console-line muted"># pinned host key fingerprint</div>
              <div class="ssh-console-line">${escapeHTML(fingerprint)}</div>
            </div>
            <div class="ssh-console-side">
              <div class="fact-card">
                <div class="mini-label">Endpoint</div>
                <div class="metric-caption strong">${escapeHTML(method.ssh_host || 'n/a')}:${escapeHTML(String(method.ssh_port || 22))}</div>
                <div class="metric-caption">${escapeHTML(method.ssh_user || 'n/a')} · ${escapeHTML(method.auth_type || 'ssh_key')}</div>
              </div>
              <div class="fact-card">
                <div class="mini-label">Security boundary</div>
                <p>The browser never receives the stored SSH private key or password. Use a local key, agent or approved bastion profile with the command above.</p>
              </div>
              <div class="inline-actions ssh-console-actions">
                <button class="primary-btn" id="copySSHCommandBtn" type="button">Copy command</button>
                ${url ? `<a class="secondary-btn button-link" href="${escapeHTML(url)}">Open SSH app</a>` : ''}
                <button class="secondary-btn" id="sshManageBtn" type="button">Manage access</button>
                <button class="secondary-btn" id="sshCloseBtn" type="button">Close</button>
              </div>
              <div id="sshConsoleResult" class="form-result"></div>
            </div>
          </div>`;
        document.getElementById('copySSHCommandBtn')?.addEventListener('click', () => copySSHCommand(command));
        document.getElementById('sshManageBtn')?.addEventListener('click', () => {
          closeModal();
          openNodeControlModal(node.id);
        });
        document.getElementById('sshCloseBtn')?.addEventListener('click', closeModal);
      } catch (err) {
        body.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
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
          <div class="empty">The agent version changes after the node completes bootstrap and sends the next heartbeat.</div>
          <div class="modal-actions"><button class="primary-btn" id="closeAgentUpgradeBtn" type="button">Close</button></div>`;
        document.getElementById('closeAgentUpgradeBtn')?.addEventListener('click', closeModal);
        await refresh();
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function queueAllAgentUpgrades(nodes) {
      const targets = nodes.filter(agentIsUpdateCandidate);
      openModal('Update all agents', `${targets.length} node(s)`, '<div id="bulkAgentUpgradeResult"><span class="tag warn">queueing</span></div>', { wide: true });
      const target = document.getElementById('bulkAgentUpgradeResult');
      const results = [];
      for (const node of targets) {
        try {
          const blocked = agentUpdateBlockReason(node);
          if (blocked) {
            results.push({ node: node.name || node.id, status: 'skipped', reason: blocked });
            target.innerHTML = renderActionResponse({ target_version: targetAgentVersion(), results }, 'Agent update queue');
            continue;
          }
          await ensureSSHBootstrapReady(node);
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
      const outdatedNodes = rows.filter(agentIsUpdateCandidate);
      const upgradeableNodes = outdatedNodes.filter(agentNeedsUpgrade);
      const updateAllReason = outdatedNodes.length && !upgradeableNodes.length
        ? (outdatedNodes.map(agentUpdateBlockReason).find(Boolean) || 'No agent update can be queued from this session.')
        : '';
      const actions = `
        <div class="inline-actions">
          <button class="secondary-btn" id="updateAllAgentsBtn" type="button"${upgradeableNodes.length ? '' : ' disabled'}${updateAllReason ? ` title="${escapeHTML(updateAllReason)}"` : ''}>Update all agents</button>
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
            <div class="table-actions node-table-actions">
              <button class="secondary-btn ssh-node-btn" type="button" data-node-id="${escapeHTML(row.id)}">SSH</button>
              ${agentIsUpdateCandidate(row) ? `<button class="${agentNeedsUpgrade(row) ? 'primary-btn' : 'secondary-btn'} update-agent-btn" type="button" data-node-id="${escapeHTML(row.id)}"${agentNeedsUpgrade(row) ? '' : ' disabled'}${agentUpdateBlockReason(row) ? ` title="${escapeHTML(agentUpdateBlockReason(row))}"` : ''}>Update</button>` : ''}
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
      document.querySelectorAll('.ssh-node-btn').forEach((button) => {
        button.addEventListener('click', () => openSSHConsoleModal(button.dataset.nodeId));
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
