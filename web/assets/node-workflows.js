(function (window) {
  'use strict';

  function createNodeWorkflows(ctx = {}) {
    const {
      state,
      nodeUI,
      requestJSON,
      sendJSON,
      refresh,
      loadNodeManagePageData,
      setPage,
      openModal,
      closeModal,
      openActionOutcomeModal,
      statusTag,
      escapeHTML,
    } = ctx;
    if (
      !state ||
      !nodeUI ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof loadNodeManagePageData !== 'function' ||
      typeof setPage !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function'
    ) {
      throw new Error('MegaVPNNodeWorkflows requires workflow dependencies');
    }

    const {
      bindNodeOnboardingPicker,
      nodeExecutionLabel,
      nodeLifecycleStatus,
      renderNodeExecutionOptions,
    } = nodeUI;
    if (
      typeof bindNodeOnboardingPicker !== 'function' ||
      typeof nodeExecutionLabel !== 'function' ||
      typeof nodeLifecycleStatus !== 'function' ||
      typeof renderNodeExecutionOptions !== 'function'
    ) {
      throw new Error('MegaVPNNodeWorkflows requires node UI helpers');
    }

    function openCreateNodeModal() {
      openModal('Add node', 'Onboarding wizard', `
        <form id="createNodeForm" class="form-grid">
          <div class="field full">
            <label>Agent setup method</label>
            <input type="hidden" name="execution_mode" value="ssh_bootstrap" />
            <div class="choice-grid node-onboarding-grid">${renderNodeExecutionOptions('ssh_bootstrap')}</div>
            <div class="field-hint">Choose how the node will become manageable. SSH bootstrap queues installation over SSH. Manual agent paths only prepare the profile and wait for the agent heartbeat.</div>
          </div>
          <div class="field"><label>Name</label><input name="name" required placeholder="edge-01" /></div>
          <div class="field"><label>Role</label><select name="role"><option value="egress">egress</option><option value="ingress">ingress</option></select></div>
          <div class="field"><label>Kind</label><select name="kind"><option value="remote">remote</option><option value="local">local</option></select></div>
          <div class="field full"><label>Address</label><input name="address" required placeholder="203.0.113.10" /></div>
          <div class="field"><label>OS family</label><input name="os_family" value="ubuntu" /></div>
          <div class="field"><label>OS version</label><input name="os_version" value="24.04" /></div>
          <div class="field"><label>Architecture</label><select name="architecture"><option value="amd64">amd64</option><option value="arm64">arm64</option></select></div>
          <div class="field full"><button class="primary-btn" type="submit">Create node</button></div>
        </form>
        <div id="createNodeResult" style="margin-top:14px"></div>`);
      const form = document.getElementById('createNodeForm');
      bindNodeOnboardingPicker(form);
      form.addEventListener('submit', createNode);
    }

    async function createNode(event) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const payload = Object.fromEntries(form.entries());
      delete payload.node_setup_choice;
      const target = document.getElementById('createNodeResult');
      target.innerHTML = '<span class="tag warn">sending</span>';
      try {
        const data = await sendJSON('/api/v1/nodes', 'POST', payload);
        await refresh();
        renderNodeCreatedStep(data);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openEditNodeModal(nodeID) {
      const node = state.nodes.find((item) => item.id === nodeID);
      if (!node) return;
      const executionMode = node.execution_mode || 'agent_managed';
      openModal(`Edit node: ${node.name}`, 'Node profile', `
        <form id="editNodeForm" class="form-grid">
          <div class="field full">
            <label>Agent setup method</label>
            <input type="hidden" name="execution_mode" value="${escapeHTML(executionMode)}" />
            <div class="choice-grid node-onboarding-grid">${renderNodeExecutionOptions(executionMode)}</div>
            <div class="field-hint">Changing this updates the node profile only. It does not install, revoke, bootstrap or re-enroll the agent by itself.</div>
          </div>
          <div class="field"><label>Name</label><input name="name" required value="${escapeHTML(node.name || '')}" /></div>
          <div class="field"><label>Role</label><select name="role"><option value="egress"${node.role === 'egress' ? ' selected' : ''}>egress</option><option value="ingress"${node.role === 'ingress' ? ' selected' : ''}>ingress</option></select></div>
          <div class="field"><label>Kind</label><select name="kind"><option value="remote"${node.kind !== 'local' ? ' selected' : ''}>remote</option><option value="local"${node.kind === 'local' ? ' selected' : ''}>local</option></select></div>
          <div class="field full"><label>Address</label><input name="address" required value="${escapeHTML(node.address || '')}" /></div>
          <div class="field"><label>OS family</label><input name="os_family" value="${escapeHTML(node.os_family || 'linux')}" /></div>
          <div class="field"><label>OS version</label><input name="os_version" value="${escapeHTML(node.os_version || 'unknown')}" /></div>
          <div class="field"><label>Architecture</label><select name="architecture"><option value="amd64"${node.architecture !== 'arm64' ? ' selected' : ''}>amd64</option><option value="arm64"${node.architecture === 'arm64' ? ' selected' : ''}>arm64</option></select></div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save node</button>
            <button class="secondary-btn" id="cancelEditNodeBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="editNodeResult" class="form-result"></div>`);
      const form = document.getElementById('editNodeForm');
      bindNodeOnboardingPicker(form);
      form.addEventListener('submit', (event) => updateNode(event, node));
      document.getElementById('cancelEditNodeBtn').addEventListener('click', closeModal);
    }

    async function updateNode(event, node) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const payload = Object.fromEntries(form.entries());
      delete payload.node_setup_choice;
      const target = document.getElementById('editNodeResult');
      target.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const previousMode = node?.execution_mode || 'agent_managed';
        const nextMode = payload.execution_mode || previousMode;
        const modeChanged = previousMode !== nextMode;
        const updated = await sendJSON(`/api/v1/nodes/${node.id}`, 'PUT', payload);
        target.innerHTML = '<span class="tag ok">node updated</span>';
        await refresh();
        if (state.page === 'nodeManage' && state.nodeManageID === node.id) {
          await loadNodeManagePageData(node.id, 'Node profile saved.');
        }
        if (modeChanged) {
          closeModal();
          openNodeExecutionModeOutcome(updated, previousMode, nextMode);
        } else {
          window.setTimeout(closeModal, 450);
        }
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openNodeExecutionModeOutcome(node, previousMode, nextMode) {
      const nextLabel = nodeExecutionLabel(nextMode);
      if (nextMode === 'agent_managed') {
        openActionOutcomeModal(
          `Setup method changed: ${node.name}`,
          'Node profile updated',
          'warning',
          'No bootstrap job was queued. The control plane is waiting for an already installed or manually installed megavpn-agent to enroll and send heartbeat.',
          [
            { label: 'Previous method', value: nodeExecutionLabel(previousMode) },
            { label: 'Current method', value: nextLabel },
            { label: 'Node state', value: nodeLifecycleStatus(node) },
            { label: 'Next action', value: 'Install/start megavpn-agent manually, or switch back to SSH bootstrap and queue bootstrap from Manage.' },
          ],
        );
        return;
      }
      if (nextMode === 'ssh_bootstrap') {
        openActionOutcomeModal(
          `Setup method changed: ${node.name}`,
          'Node profile updated',
          'warning',
          'The node is now configured for SSH bootstrap, but installation starts only after SSH access is saved and Queue bootstrap is clicked in Manage.',
          [
            { label: 'Previous method', value: nodeExecutionLabel(previousMode) },
            { label: 'Current method', value: nextLabel },
            { label: 'Next action', value: 'Open Manage -> Bootstrap, save SSH access, rotate enrollment token if needed, then queue bootstrap.' },
          ],
        );
        return;
      }
      openActionOutcomeModal(
        `Setup method changed: ${node.name}`,
        'Node profile updated',
        'succeeded',
        'The node profile was updated. No runtime action was started automatically.',
        [
          { label: 'Previous method', value: nodeExecutionLabel(previousMode) },
          { label: 'Current method', value: nextLabel },
        ],
      );
    }

    function renderNodeCreatedStep(node) {
      openModal('Node created', 'Next step', `
        <section class="card">
          <div class="table-head compact-head">
            <h2>${escapeHTML(node.name || 'Node')}</h2>
            ${statusTag(node.status || 'draft')}
          </div>
          <div class="card-body">
            <div class="grid cols-2">
              <div class="fact-card"><div class="mini-label">Address</div><div class="metric-caption strong">${escapeHTML(node.address || 'n/a')}</div></div>
              <div class="fact-card"><div class="mini-label">Role</div><div class="metric-caption strong">${escapeHTML(node.role || 'egress')}</div></div>
              <div class="fact-card"><div class="mini-label">Setup method</div><div class="metric-caption strong">${escapeHTML(nodeExecutionLabel(node.execution_mode || 'agent_managed'))}</div></div>
              <div class="fact-card"><div class="mini-label">Agent</div><div class="metric-caption strong">${escapeHTML(node.agent_status || 'unknown')}</div></div>
            </div>
            <div class="modal-actions">
              <button class="primary-btn" id="configureCreatedNodeBtn" type="button">Configure agent</button>
              <button class="secondary-btn" id="addAnotherNodeBtn" type="button">Add another node</button>
              <button class="secondary-btn" id="closeCreatedNodeBtn" type="button">Close</button>
            </div>
          </div>
        </section>`);
      document.getElementById('configureCreatedNodeBtn').addEventListener('click', () => openNodeControlModal(node.id));
      document.getElementById('addAnotherNodeBtn').addEventListener('click', openCreateNodeModal);
      document.getElementById('closeCreatedNodeBtn').addEventListener('click', closeModal);
    }

    function openDeleteNodeModal(nodeID, nodeName) {
      const node = state.nodes.find((item) => item.id === nodeID) || { id: nodeID, name: nodeName };
      if (!node?.id) return;
      openModal(`Delete node: ${node.name || 'node'}`, 'Lifecycle action', `
        <section class="card">
          <h2>Retire node</h2>
          <p>This action removes the node from active operation and revokes its agent identity. The API will block deletion while active instances still exist on this node.</p>
          <div class="code-block">node_id = ${escapeHTML(node.id)}
name = ${escapeHTML(node.name || 'n/a')}
address = ${escapeHTML(node.address || 'n/a')}
status = ${escapeHTML(node.status || 'n/a')}</div>
          <div class="modal-actions">
            <button class="danger-btn" id="confirmDeleteNodeBtn" type="button">Delete node</button>
            <button class="secondary-btn" id="cancelDeleteNodeBtn" type="button">Cancel</button>
          </div>
          <div id="deleteNodeResult" class="form-result"></div>
        </section>`);
      document.getElementById('confirmDeleteNodeBtn').addEventListener('click', () => deleteNode(node.id));
      document.getElementById('cancelDeleteNodeBtn').addEventListener('click', closeModal);
    }

    async function deleteNode(nodeID) {
      const target = document.getElementById('deleteNodeResult');
      const button = document.getElementById('confirmDeleteNodeBtn');
      if (!target) return;
      target.innerHTML = '<span class="tag warn">deleting</span>';
      if (button) button.disabled = true;
      try {
        await requestJSON(`/api/v1/nodes/${nodeID}`, { method: 'DELETE' });
        target.innerHTML = '<span class="tag ok">node retired</span>';
        await refresh();
        window.setTimeout(closeModal, 450);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        if (button) button.disabled = false;
      }
    }

    function openNodeControlModal(nodeID) {
      closeModal();
      state.nodeManageID = nodeID;
      state.nodeManageData = null;
      setPage('nodeManage');
    }

    return {
      openCreateNodeModal,
      openEditNodeModal,
      openDeleteNodeModal,
      openNodeControlModal,
    };
  }

  window.MegaVPNNodeWorkflows = { create: createNodeWorkflows };
})(window);
