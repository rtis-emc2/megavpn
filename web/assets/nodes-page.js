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
      typeof openCreateNodeModal !== 'function' ||
      typeof openNodeControlModal !== 'function' ||
      typeof openEditNodeModal !== 'function' ||
      typeof openDeleteNodeModal !== 'function'
    ) {
      throw new Error('MegaVPNNodesPage requires page dependencies');
    }

    function render() {
      setTitle('Nodes');
      const rows = Array.isArray(state.nodes) ? state.nodes.filter((node) => node.status !== 'retired') : [];
      el('content').innerHTML = `
        ${tableCard('Managed Nodes', rows, [
          { title: 'Name', key: 'name' },
          { title: 'Role', key: 'role', render: (row) => `<span class="tag">${escapeHTML(row.role || 'egress')}</span>` },
          { title: 'Kind', key: 'kind', render: (row) => `<span class="tag">${escapeHTML(row.kind || 'local')}</span>` },
          { title: 'Address', key: 'address' },
          { title: 'Execution', key: 'execution_mode', render: (row) => escapeHTML(nodeExecutionLabel(row.execution_mode)) },
          { title: 'Agent channel', key: 'agent_status', render: (row) => statusTag(nodeAgentChannelStatus(row)) },
          { title: 'Node state', key: 'status', render: (row) => statusTag(nodeLifecycleStatus(row)) },
          { title: 'Actions', key: 'id', render: (row) => `
            <div class="inline-actions">
              <button class="secondary-btn manage-node-btn" type="button" data-node-id="${escapeHTML(row.id)}">Manage</button>
              <button class="secondary-btn edit-node-btn" type="button" data-node-id="${escapeHTML(row.id)}">Edit</button>
              <button class="danger-btn delete-node-btn" type="button" data-node-id="${escapeHTML(row.id)}" data-node-name="${escapeHTML(row.name || 'node')}">Delete</button>
            </div>` },
        ], '<button class="secondary-btn" id="createNodeBtn" type="button">Add node</button>')}`;
      document.getElementById('createNodeBtn')?.addEventListener('click', openCreateNodeModal);
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
