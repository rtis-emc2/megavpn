(function (window) {
  'use strict';

  function createNodeWorkflows(ctx = {}) {
    const {
      state,
      nodeUI,
      setTitle,
      el,
      requestJSON,
      sendJSON,
      fetchJSON,
      refresh,
      loadCore,
      setPage,
      openModal,
      closeModal,
      openActionOutcomeModal,
      renderActionResponse,
      watchJob,
      waitForNodeDiagnostics,
      statusTag,
      escapeHTML,
      toMillis,
      formatDate,
      formatRelativeDate,
      formatDurationSeconds,
      platformPublicBaseURL,
    } = ctx;
    if (
      !state ||
      !nodeUI ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof fetchJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof loadCore !== 'function' ||
      typeof setPage !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof watchJob !== 'function' ||
      typeof waitForNodeDiagnostics !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof toMillis !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof formatRelativeDate !== 'function' ||
      typeof formatDurationSeconds !== 'function' ||
      typeof platformPublicBaseURL !== 'function'
    ) {
      throw new Error('MegaVPNNodeWorkflows requires workflow dependencies');
    }

    const {
      bindNodeOnboardingPicker,
      bootstrapRunReason,
      bindNodeConsoleTabs,
      commMetricLine,
      defaultNodeConsoleTab,
      diagnosticsAgentState,
      inventoryLabel,
      nodeExecutionLabel,
      nodeConsoleTabButton,
      nodeHeartbeatStatus,
      nodeLifecycleStatus,
      renderInventorySnapshotPanel,
      renderNodeExecutionOptions,
      switchNodeConsoleTab,
    } = nodeUI;
    if (
      typeof bindNodeOnboardingPicker !== 'function' ||
      typeof bootstrapRunReason !== 'function' ||
      typeof bindNodeConsoleTabs !== 'function' ||
      typeof commMetricLine !== 'function' ||
      typeof defaultNodeConsoleTab !== 'function' ||
      typeof diagnosticsAgentState !== 'function' ||
      typeof inventoryLabel !== 'function' ||
      typeof nodeExecutionLabel !== 'function' ||
      typeof nodeConsoleTabButton !== 'function' ||
      typeof nodeHeartbeatStatus !== 'function' ||
      typeof nodeLifecycleStatus !== 'function' ||
      typeof renderInventorySnapshotPanel !== 'function' ||
      typeof renderNodeExecutionOptions !== 'function' ||
      typeof switchNodeConsoleTab !== 'function'
    ) {
      throw new Error('MegaVPNNodeWorkflows requires node UI helpers');
    }

    function arrayOrEmpty(value) {
      return Array.isArray(value) ? value : [];
    }

    const nodeManageTabIDs = new Set(['overview', 'instances', 'bootstrap', 'terminal', 'agent', 'inventory', 'services']);

    function managedInstancesForNode(nodeID) {
      const id = String(nodeID || '').trim();
      return arrayOrEmpty(state.instances)
        .filter((item) => String(item.node_id || '').trim() === id)
        .filter((item) => String(item.status || '').toLowerCase() !== 'deleted');
    }

    function runtimeStateForInstance(instanceID) {
      const id = String(instanceID || '').trim();
      return arrayOrEmpty(state.instanceRuntimeStates).find((item) => String(item.instance_id || '').trim() === id) || null;
    }

    function serviceLabel(serviceCode) {
      const code = String(serviceCode || '').trim();
      const service = arrayOrEmpty(state.servicesCatalog).find((item) => String(item.code || '').trim() === code);
      return service?.label || service?.display_name || service?.name || code || 'unknown';
    }

    function instanceEndpoint(instance) {
      const host = String(instance?.endpoint_host || '').trim();
      const port = Number(instance?.endpoint_port || 0);
      if (!host && !port) return 'n/a';
      if (!host) return String(port);
      if (!port) return host;
      return `${host}:${port}`;
    }

    function openInstancesForNode(node) {
      if (!node?.id) return;
      if (state.nodeTerminalActive) disconnectNodeTerminal();
      state.instancesView = 'list';
      state.instancesListView = 'node';
      state.instancesVisibleLimit = 100;
      state.instanceListFilters = {
        search: '',
        status: 'all',
        service: 'all',
        node: node.id,
      };
      setPage('instances');
    }

    function renderManagedNodeInstanceRows(instances) {
      if (!instances.length) {
        return '<tr><td colspan="6"><div class="empty">No managed instances are assigned to this node.</div></td></tr>';
      }
      return instances.map((instance) => {
        const runtime = runtimeStateForInstance(instance.id);
        const revision = instance.current_revision_id && instance.last_applied_revision_id && instance.current_revision_id === instance.last_applied_revision_id
          ? 'applied'
          : instance.current_revision_id
            ? 'pending'
            : 'n/a';
        const healthReason = Array.isArray(runtime?.health_reasons)
          ? runtime.health_reasons.map((item) => String(item || '').trim()).find(Boolean)
          : '';
        return `
          <tr>
            <td>
              <strong class="mono-clip" title="${escapeHTML(instance.name || '')}">${escapeHTML(instance.name || 'instance')}</strong>
              <span class="metric-caption mono-clip" title="${escapeHTML(instance.id || '')}">${escapeHTML(instance.id || 'n/a')}</span>
            </td>
            <td>
              <strong>${escapeHTML(serviceLabel(instance.service_code))}</strong>
              <span class="metric-caption mono-clip">${escapeHTML(instance.service_code || 'unknown')}</span>
            </td>
            <td><code class="mono-clip" title="${escapeHTML(instanceEndpoint(instance))}">${escapeHTML(instanceEndpoint(instance))}</code></td>
            <td>
              <div class="instance-state-cluster">
                ${statusTag(instance.status || 'draft')}
                <span class="tag">${escapeHTML(revision === 'applied' ? 'rev applied' : `rev ${revision}`)}</span>
              </div>
            </td>
            <td>
              <div class="instance-state-cluster">
                ${statusTag(runtime?.runtime_status || 'unknown')}
                ${statusTag(runtime?.health_status || 'unknown')}
                ${statusTag(runtime?.drift_status || 'unknown')}
              </div>
            </td>
            <td><span class="metric-caption" title="${escapeHTML(healthReason || '')}">${escapeHTML(healthReason || 'No recent runtime issue.')}</span></td>
          </tr>`;
      }).join('');
    }

    function persistNodeManageTabs() {
      try {
        sessionStorage.setItem('megavpn.nodeManageActiveTabs', JSON.stringify(state.nodeManageActiveTabs || {}));
      } catch (_) {
        // Session storage is best-effort only; in-memory state still protects auto-refresh.
      }
    }

    function setNodeManageActiveTab(nodeID, tabID) {
      if (!nodeID || !nodeManageTabIDs.has(tabID)) return;
      if (!state.nodeManageActiveTabs || typeof state.nodeManageActiveTabs !== 'object') {
        state.nodeManageActiveTabs = {};
      }
      state.nodeManageActiveTabs[nodeID] = tabID;
      persistNodeManageTabs();
    }

    function resolveNodeManageActiveTab(nodeID, diag) {
      const saved = state.nodeManageActiveTabs?.[nodeID];
      if (nodeManageTabIDs.has(saved)) return saved;
      const fallback = defaultNodeConsoleTab(diag);
      return nodeManageTabIDs.has(fallback) ? fallback : 'overview';
    }

    let activeTerminalSocket = null;
    let activeTerminalView = null;

    function enabledSSHMethod(methods) {
      return arrayOrEmpty(methods).find((item) => item.method === 'ssh' && item.is_enabled === true) || null;
    }

    function terminalEndpointLabel(method) {
      if (!method) return 'not configured';
      return `${method.ssh_user || 'user'}@${method.ssh_host || 'host'}:${method.ssh_port || 22}`;
    }

    function terminalWebSocketURL(nodeID, sessionID) {
      const base = `${state.apiBase || ''}/api/v1/nodes/${encodeURIComponent(nodeID)}/ssh/terminal?session=${encodeURIComponent(sessionID)}`;
      const url = new URL(base, window.location.href);
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
      return url.toString();
    }

    function terminalOutputElement() {
      return document.getElementById('nodeTerminalOutput');
    }

    function terminalSurfaceElement() {
      return document.getElementById('nodeTerminalSurface');
    }

    function setTerminalStatus(value, kind = 'stub') {
      const target = document.getElementById('nodeTerminalStatus');
      if (!target) return;
      target.innerHTML = `<span class="tag ${kind}">${escapeHTML(value)}</span>`;
    }

    function appendTerminalOutput(value) {
      if (activeTerminalView) {
        activeTerminalView.write(value);
        return;
      }
      const output = terminalOutputElement();
      if (!output) return;
      output.textContent += String(value || '');
    }

    function terminalKeyData(event) {
      if (event.ctrlKey && !event.metaKey && !event.altKey && event.key.length === 1) {
        const code = event.key.toUpperCase().charCodeAt(0);
        if (code >= 64 && code <= 95) return String.fromCharCode(code - 64);
      }
      if (event.metaKey || event.altKey) return '';
      switch (event.key) {
      case 'Enter': return '\r';
      case 'Backspace': return '\x7f';
      case 'Tab': return '\t';
      case 'Escape': return '\x1b';
      case 'ArrowUp': return '\x1b[A';
      case 'ArrowDown': return '\x1b[B';
      case 'ArrowRight': return '\x1b[C';
      case 'ArrowLeft': return '\x1b[D';
      case 'Delete': return '\x1b[3~';
      case 'Home': return '\x1b[H';
      case 'End': return '\x1b[F';
      case 'PageUp': return '\x1b[5~';
      case 'PageDown': return '\x1b[6~';
      default:
        return event.key.length === 1 ? event.key : '';
      }
    }

    function sendTerminalInput(data) {
      if (!activeTerminalSocket || activeTerminalSocket.readyState !== WebSocket.OPEN || !data) return;
      activeTerminalSocket.send(JSON.stringify({ type: 'input', data }));
    }

    async function startNodeTerminal(node) {
      if (!node?.id) return;
      disconnectNodeTerminal();
      const startButton = document.getElementById('startNodeTerminalBtn');
      if (startButton) startButton.disabled = true;
      setTerminalStatus('opening', 'warn');
      appendTerminalOutput(`\n[megavpn] opening ssh terminal for ${node.name || node.id}\n`);
      try {
        const ticket = await sendJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/ssh/sessions`, 'POST', {});
        const socket = new WebSocket(terminalWebSocketURL(node.id, ticket.session_id));
        activeTerminalSocket = socket;
        state.nodeTerminalActive = true;
        socket.addEventListener('open', () => {
          setTerminalStatus('connected', 'ok');
          document.getElementById('disconnectNodeTerminalBtn')?.removeAttribute('disabled');
          terminalSurfaceElement()?.focus();
        });
        socket.addEventListener('message', (event) => {
          let msg = null;
          try {
            msg = JSON.parse(event.data);
          } catch (_) {
            appendTerminalOutput(String(event.data || ''));
            return;
          }
          if (msg.type === 'output') appendTerminalOutput(msg.data || '');
          if (msg.type === 'status') setTerminalStatus(msg.message || 'connected', 'ok');
          if (msg.type === 'error') {
            setTerminalStatus('error', 'danger');
            appendTerminalOutput(`\n[megavpn] ${msg.message || 'terminal error'}\n`);
          }
          if (msg.type === 'exit') {
            setTerminalStatus('closed', 'stub');
            appendTerminalOutput(`\n[megavpn] ${msg.message || 'ssh session closed'}\n`);
          }
        });
        socket.addEventListener('close', () => {
          if (activeTerminalSocket === socket) activeTerminalSocket = null;
          state.nodeTerminalActive = false;
          setTerminalStatus('disconnected', 'stub');
          if (startButton) startButton.disabled = false;
          document.getElementById('disconnectNodeTerminalBtn')?.setAttribute('disabled', 'disabled');
        });
        socket.addEventListener('error', () => {
          setTerminalStatus('connection error', 'danger');
        });
      } catch (err) {
        state.nodeTerminalActive = false;
        setTerminalStatus('failed', 'danger');
        appendTerminalOutput(`\n[megavpn] ${err.message}\n`);
        if (startButton) startButton.disabled = false;
      }
    }

    function disconnectNodeTerminal() {
      if (activeTerminalSocket && activeTerminalSocket.readyState === WebSocket.OPEN) {
        activeTerminalSocket.send(JSON.stringify({ type: 'close' }));
      }
      if (activeTerminalSocket) {
        activeTerminalSocket.close();
      }
      activeTerminalSocket = null;
      state.nodeTerminalActive = false;
    }

    function bindNodeTerminal(node, terminalMethod) {
      const surface = terminalSurfaceElement();
      const output = terminalOutputElement();
      if (surface && output && window.MegaVPNWebTerminal?.create) {
        activeTerminalView = window.MegaVPNWebTerminal.create({ surface, output });
        activeTerminalView.reset(`MegaVPN web terminal\r\nnode: ${node.name || node.id}\r\nendpoint: ${terminalEndpointLabel(terminalMethod)}\r\n`);
      }
      document.getElementById('startNodeTerminalBtn')?.addEventListener('click', () => startNodeTerminal(node));
      document.getElementById('disconnectNodeTerminalBtn')?.addEventListener('click', disconnectNodeTerminal);
      document.getElementById('clearNodeTerminalBtn')?.addEventListener('click', () => {
        if (activeTerminalView) {
          activeTerminalView.clear();
          return;
        }
        if (output) output.textContent = '';
      });
      surface?.addEventListener('click', () => surface.focus());
      surface?.addEventListener('keydown', (event) => {
        const data = terminalKeyData(event);
        if (!data) return;
        event.preventDefault();
        sendTerminalInput(data);
      });
      surface?.addEventListener('paste', (event) => {
        const text = event.clipboardData?.getData('text/plain') || '';
        if (!text) return;
        event.preventDefault();
        sendTerminalInput(text);
      });
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

    function openEmergencyNodeCleanupModal(node) {
      if (!node?.id) return;
      const confirmationName = String(node.name || node.id || '').trim();
      openModal(`Emergency cleanup: ${node.name || 'node'}`, 'Destructive node operation', `
        <section class="card">
          <h2>Clean managed runtime from this node</h2>
          <p>This queues an agent job that removes product-managed service instances from the selected host. Backhaul transport and route-policy runtime are preserved unless full node wipe is selected.</p>
          <div class="code-block">node_id = ${escapeHTML(node.id)}
name = ${escapeHTML(node.name || 'n/a')}
address = ${escapeHTML(node.address || 'n/a')}
agent = ${escapeHTML(node.agent_status || 'unknown')}</div>
          <div class="form-grid">
            <label class="choice-card full">
              <input id="fullNodeCleanupToggle" type="checkbox" />
              <span>
                <strong>Full node wipe</strong>
                <small>Also remove managed backhaul state, route-policy runtime and node-level service material. Use only when taking the node out of service.</small>
              </span>
            </label>
            <label class="choice-card full">
              <input id="includeAgentCleanupToggle" type="checkbox" />
              <span>
                <strong>Also remove the agent</strong>
                <small>Forces full node wipe, then schedules delayed self-removal of the agent binary, unit, env and local state after the job result is submitted.</small>
              </span>
            </label>
            <div class="field full">
              <label>Type node name to confirm</label>
              <input id="emergencyCleanupConfirm" autocomplete="off" spellcheck="false" placeholder="${escapeHTML(confirmationName)}" />
              <div class="field-hint">Required value: <code>${escapeHTML(confirmationName)}</code></div>
            </div>
          </div>
          <div class="modal-actions">
            <button class="danger-btn" id="confirmEmergencyCleanupBtn" type="button">Queue emergency cleanup</button>
            <button class="secondary-btn" id="cancelEmergencyCleanupBtn" type="button">Cancel</button>
          </div>
          <div id="emergencyCleanupResult" class="form-result"></div>
        </section>`, { wide: true });
      const includeAgentToggle = document.getElementById('includeAgentCleanupToggle');
      const fullNodeToggle = document.getElementById('fullNodeCleanupToggle');
      includeAgentToggle?.addEventListener('change', () => {
        if (!fullNodeToggle) return;
        if (includeAgentToggle.checked) {
          fullNodeToggle.checked = true;
          fullNodeToggle.disabled = true;
        } else {
          fullNodeToggle.disabled = false;
        }
      });
      document.getElementById('confirmEmergencyCleanupBtn')?.addEventListener('click', () => queueEmergencyNodeCleanup(node));
      document.getElementById('cancelEmergencyCleanupBtn')?.addEventListener('click', closeModal);
    }

    async function queueEmergencyNodeCleanup(node) {
      const target = document.getElementById('emergencyCleanupResult');
      const button = document.getElementById('confirmEmergencyCleanupBtn');
      const confirmation = String(document.getElementById('emergencyCleanupConfirm')?.value || '').trim();
      const expectedConfirmation = String(node.name || node.id || '').trim();
      const includeAgent = Boolean(document.getElementById('includeAgentCleanupToggle')?.checked);
      const fullNodeCleanup = includeAgent || Boolean(document.getElementById('fullNodeCleanupToggle')?.checked);
      if (!target) return;
      if (confirmation !== expectedConfirmation) {
        target.innerHTML = `<span class="tag danger">type ${escapeHTML(expectedConfirmation)} to confirm</span>`;
        return;
      }
      target.innerHTML = '<span class="tag warn">queueing cleanup</span>';
      if (button) button.disabled = true;
      try {
        const job = await sendJSON(`/api/v1/nodes/${node.id}/emergency-cleanup`, 'POST', {
          include_agent: includeAgent,
          cleanup_scope: fullNodeCleanup ? 'full_node' : 'services_only',
          confirmation,
        });
        target.innerHTML = renderActionResponse(job, 'Emergency cleanup queued');
        if (job?.id) {
          await watchJob(job.id, target, 'node emergency cleanup');
        }
        await refresh();
        if (includeAgent) {
          closeModal();
          state.nodeManageData = null;
          setPage('nodes');
          return;
        }
        await reloadNodeControlModal(node.id, 'Emergency cleanup completed. Runtime state was refreshed.');
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

    function renderNodeManagePage() {
      const nodeID = state.nodeManageID;
      const cachedNode = state.nodes.find((item) => item.id === nodeID) || null;
      const data = state.nodeManageData;
      const node = data?.node || cachedNode || null;
      setTitle(node ? `Node: ${node.name || 'node'}` : 'Node Management');
      if (!nodeID) {
        el('content').innerHTML = `
          <section class="card">
            <h2>Node not selected</h2>
            <p>Return to Nodes and choose a managed node.</p>
            <div class="modal-actions"><button class="primary-btn" id="nodeManageBackBtn" type="button">Back to Nodes</button></div>
          </section>`;
        document.getElementById('nodeManageBackBtn')?.addEventListener('click', () => {
          state.nodeManageID = '';
          state.nodeManageData = null;
          setPage('nodes');
        });
        return;
      }

      el('content').innerHTML = `
        <section class="node-workspace">
          <div class="node-workspace-head">
            <div>
              <div class="eyebrow">Node management</div>
              <h2>${escapeHTML(node?.name || 'Loading node...')}</h2>
              <p>${escapeHTML(node?.address || 'Loading runtime state and bootstrap controls.')}</p>
            </div>
            <div class="node-workspace-actions">
              <button class="secondary-btn" id="nodeManageBackBtn" type="button">Back to Nodes</button>
              <button class="secondary-btn" id="nodeManageRefreshBtn" type="button">Refresh</button>
              ${node ? '<button class="primary-btn" id="nodeManageEditBtn" type="button">Edit profile</button>' : ''}
            </div>
          </div>
          <div id="nodeManageBody">
            <section class="card"><div class="empty">Loading node management workspace...</div></section>
          </div>
        </section>`;

      document.getElementById('nodeManageBackBtn')?.addEventListener('click', () => {
        state.nodeManageID = '';
        state.nodeManageData = null;
        setPage('nodes');
      });
      document.getElementById('nodeManageRefreshBtn')?.addEventListener('click', () => {
        void loadNodeManagePageData(nodeID, 'Node state refreshed.');
      });
      document.getElementById('nodeManageEditBtn')?.addEventListener('click', () => openEditNodeModal(nodeID));

      if (!data || data.nodeID !== nodeID) {
        void loadNodeManagePageData(nodeID);
        return;
      }
      renderNodeControlModal(data.node, data.diag, data.methods, data.runs, data.tokens, data.flash, 'nodeManageBody');
    }

    async function loadNodeManagePageData(nodeID, flash = '') {
      const node = state.nodes.find((item) => item.id === nodeID);
      try {
        const [freshNode, diag, methods, runs, tokens] = await Promise.all([
          requestJSON(`/api/v1/nodes/${nodeID}`),
          requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`),
          requestJSON(`/api/v1/nodes/${nodeID}/access-methods`),
          requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`),
          requestJSON(`/api/v1/nodes/${nodeID}/enrollment-tokens`),
        ]);
        state.nodeManageData = {
          nodeID,
          node: diag?.node || freshNode || node,
          diag: diag || {},
          methods: arrayOrEmpty(methods),
          runs: arrayOrEmpty(runs),
          tokens: arrayOrEmpty(tokens),
          flash,
        };
        if (state.page === 'nodeManage' && state.nodeManageID === nodeID) {
          renderNodeManagePage();
        }
      } catch (err) {
        state.nodeManageData = null;
        if (state.page === 'nodeManage') {
          const body = el('nodeManageBody');
          if (body) body.innerHTML = `<section class="card"><div class="empty">Failed to load node details: ${escapeHTML(err.message)}</div></section>`;
        } else {
          el('modalBody').innerHTML = `<div class="empty">Failed to load node details: ${escapeHTML(err.message)}</div>`;
        }
      }
    }

    function renderNodeControlModal(node, diag, methods, runs, tokens, flash, targetID = 'modalBody') {
      methods = arrayOrEmpty(methods);
      runs = arrayOrEmpty(runs);
      tokens = arrayOrEmpty(tokens);
      const heartbeatStatus = diag?.heartbeat_state || nodeHeartbeatStatus(node);
      const heartbeatDrift = diag?.heartbeat_drift_seconds;
      const sshMethod = methods.find((item) => item.method === 'ssh') || null;
      const latestInventory = diag?.latest_inventory || null;
      const inventoryPayload = latestInventory?.payload || {};
      const discoverySummary = diag?.discovery_summary || { total: 0, available: 0, imported: 0, ignored: 0, by_service: {} };
      const recentDiscoveries = Array.isArray(diag?.recent_discoveries) ? diag.recent_discoveries : [];
      const agent = diag?.agent || {};
      const terminalMethod = enabledSSHMethod(methods);
      const methodRows = methods.length
        ? methods.map((item) => `
            <tr>
              <td>${escapeHTML(item.method || 'unknown')}</td>
              <td>${escapeHTML(item.ssh_host || 'n/a')}${item.ssh_port ? `:${escapeHTML(String(item.ssh_port))}` : ''}</td>
              <td>${escapeHTML(item.ssh_user || 'n/a')}</td>
              <td>${escapeHTML(item.auth_type || 'n/a')}</td>
              <td><code>${escapeHTML(item.ssh_host_key_sha256 || 'n/a')}</code></td>
              <td>${statusTag(item.is_enabled ? 'enabled' : 'disabled')}</td>
            </tr>`).join('')
        : '<tr><td colspan="6"><div class="empty compact-empty">Access methods are not configured yet.</div></td></tr>';
      const runRows = runs.length
        ? runs.slice(0, 8).map((item) => `
            <tr>
              <td>${escapeHTML(item.bootstrap_mode || 'unknown')}</td>
              <td>${statusTag(item.status || 'unknown')}</td>
              <td>${formatDate(item.started_at || item.created_at)}</td>
              <td>${escapeHTML(bootstrapRunReason(item))}</td>
              <td><button class="secondary-btn bootstrap-run-view-btn" type="button" data-run-id="${escapeHTML(item.id)}" data-job-id="${escapeHTML(item.job_id || '')}">Details</button></td>
            </tr>`).join('')
        : '<tr><td colspan="5"><div class="empty compact-empty">No bootstrap runs yet.</div></td></tr>';
      const tokenRows = tokens.length
        ? tokens.slice(0, 8).map((item) => `
            <tr>
              <td><code>${escapeHTML(item.token_hint || item.token || 'n/a')}</code></td>
              <td>${statusTag(item.status || 'active')}</td>
              <td>${formatDate(item.expires_at)}</td>
              <td>${formatDate(item.used_at)}</td>
            </tr>`).join('')
        : '<tr><td colspan="4"><div class="empty compact-empty">No enrollment tokens created yet.</div></td></tr>';
      const recentDiscoveryRows = recentDiscoveries.length
        ? recentDiscoveries.map((item) => `
            <tr>
              <td>${escapeHTML(item.service_code)}</td>
              <td>${escapeHTML(item.name)}</td>
              <td>${statusTag(item.status)}</td>
              <td>${escapeHTML(item.endpoint_host || 'n/a')}${item.endpoint_port ? `:${escapeHTML(String(item.endpoint_port))}` : ''}</td>
              <td>${formatDate(item.detected_at)}</td>
            </tr>`).join('')
        : '<tr><td colspan="5"><div class="empty">Service discovery has not reported anything yet.</div></td></tr>';
      const serviceMix = Object.entries(discoverySummary.by_service || {}).length
        ? Object.entries(discoverySummary.by_service || {}).map(([code, total]) => `${code}: ${total}`).join(' · ')
        : 'none';
      const inventoryCollectedAt = inventoryPayload.collected_at || latestInventory?.created_at || null;
      const canRequeueStuckJob = String(diag?.communication_state || '') === 'job_result_stalled' && agent.last_job_claim_job_id;
      const canClearStaleRotation = String(agent.token_rotation_status || '') === 'rotating';
      const activeTab = resolveNodeManageActiveTab(node.id, diag);
      const setupLabel = nodeExecutionLabel(node.execution_mode || 'unknown');
      const communicationState = diag?.communication_state || 'unknown';
      const accessStatus = terminalMethod ? 'configured' : 'missing';
      const publicURL = platformPublicBaseURL() || 'not configured';
      const managedInstances = managedInstancesForNode(node.id);
      const managedInstanceRows = renderManagedNodeInstanceRows(managedInstances);
      const agentNextStepPanel = String(diag?.communication_state || '') === 'awaiting_enrollment'
        ? `<div class="fact-card emphasis-card node-next-step">
            <div class="mini-label">Next step</div>
            <div class="metric-caption strong">Agent enrollment is still pending.</div>
            <p>Install/start <code>megavpn-agent</code> with an active enrollment token, or use SSH bootstrap from the Bootstrap tab.</p>
            <div class="section-actions compact-section-actions">
              <button class="primary-btn" id="openBootstrapFromAgentBtn" type="button">Open Bootstrap</button>
              <button class="secondary-btn" id="editSetupFromAgentBtn" type="button">Edit setup method</button>
            </div>
          </div>`
        : '';

      const target = el(targetID);
      if (!target) return;
      target.innerHTML = `
        <div class="node-console-summary node-manage-summary">
          <div class="fact-card emphasis-card"><div class="mini-label">Node</div><div class="metric-caption strong">${escapeHTML(node.name || 'node')}</div><div class="metric-caption">${escapeHTML(node.address || 'n/a')}</div></div>
          <div class="fact-card"><div class="mini-label">Lifecycle</div><div class="metric-caption strong">${statusTag(node.status || 'draft')}</div><div class="metric-caption">${escapeHTML(node.kind || 'remote')} · ${escapeHTML(node.role || 'egress')}</div></div>
          <div class="fact-card"><div class="mini-label">Agent</div><div class="metric-caption strong">${statusTag(diagnosticsAgentState(diag))}</div><div class="metric-caption">${escapeHTML(communicationState)}</div></div>
          <div class="fact-card"><div class="mini-label">Heartbeat</div><div class="metric-caption strong">${escapeHTML(heartbeatStatus)}</div><div class="metric-caption">${escapeHTML(heartbeatDrift == null ? formatRelativeDate(node.last_heartbeat_at) : formatDurationSeconds(heartbeatDrift))}</div></div>
          <div class="fact-card"><div class="mini-label">Bootstrap</div><div class="metric-caption strong">${statusTag(diag?.last_bootstrap?.status || 'not started')}</div><div class="metric-caption">SSH access ${escapeHTML(accessStatus)}</div></div>
          <div class="fact-card"><div class="mini-label">Instances</div><div class="metric-caption strong">${escapeHTML(String(managedInstances.length))}</div><div class="metric-caption">managed workloads</div></div>
          <div class="fact-card"><div class="mini-label">Inventory</div><div class="metric-caption strong">${escapeHTML(formatRelativeDate(inventoryCollectedAt))}</div><div class="metric-caption">${escapeHTML(inventoryLabel(inventoryPayload, 'os.pretty_name', `${node.os_family || 'linux'} ${node.os_version || ''}`))}</div></div>
        </div>
        ${flash ? `<div class="notice subtle-notice">${escapeHTML(flash)}</div>` : ''}
        <div class="node-console-layout">
          <nav class="node-console-nav" aria-label="Node management sections">
            ${nodeConsoleTabButton('overview', 'Overview', 'profile and actions', activeTab)}
            ${nodeConsoleTabButton('instances', 'Instances', 'managed workloads', activeTab)}
            ${nodeConsoleTabButton('bootstrap', 'Bootstrap', 'SSH, tokens, jobs', activeTab)}
            ${nodeConsoleTabButton('terminal', 'Terminal', 'browser SSH', activeTab)}
            ${nodeConsoleTabButton('agent', 'Agent channel', 'health and trust', activeTab)}
            ${nodeConsoleTabButton('inventory', 'Inventory', 'host snapshot', activeTab)}
            ${nodeConsoleTabButton('services', 'Services', 'discovery results', activeTab)}
          </nav>
          <div class="node-console-content">
            <section class="node-tab-panel${activeTab === 'overview' ? ' is-active' : ''}" data-node-panel="overview">
              <div class="node-panel-head">
                <div><div class="eyebrow">Runtime Profile</div><h2>Node Overview</h2></div>
                <div class="section-meta">${statusTag(node.status || 'draft')}</div>
              </div>
              <div class="node-manage-grid">
                <section class="section-card node-profile-card">
                  <div class="section-head">
                    <div><div class="eyebrow">Profile</div><h2>Core settings</h2></div>
                  </div>
                  <div class="section-body">
                    <div class="node-detail-list">
                      <div><span>Name</span><strong>${escapeHTML(node.name || 'n/a')}</strong></div>
                      <div><span>Address</span><strong>${escapeHTML(node.address || 'n/a')}</strong></div>
                      <div><span>Role</span><strong>${escapeHTML(node.role || 'egress')}</strong></div>
                      <div><span>Kind</span><strong>${escapeHTML(node.kind || 'remote')}</strong></div>
                      <div><span>Setup method</span><strong>${escapeHTML(setupLabel)}</strong></div>
                      <div><span>Public control URL</span><strong>${escapeHTML(publicURL)}</strong></div>
                    </div>
                  </div>
                </section>
                <section class="section-card">
                  <div class="section-head">
                    <div><div class="eyebrow">Operations</div><h2>Node actions</h2></div>
                  </div>
                  <div class="section-body">
                    <div class="operator-action-grid">
                      <button class="operator-action" id="editNodeFromManageBtn" type="button"><strong>Edit profile</strong><span>Name, role, address, setup method.</span></button>
                      <button class="operator-action" id="openNodeInstancesBtn" type="button"><strong>Managed instances</strong><span>${escapeHTML(String(managedInstances.length))} workload(s) assigned to this node.</span></button>
                      <button class="operator-action" id="openNodeTerminalBtn" type="button"><strong>SSH terminal</strong><span>${escapeHTML(terminalEndpointLabel(terminalMethod))}</span></button>
                      <button class="operator-action" id="refreshNodeRuntimeBtn" type="button"><strong>Refresh diagnostics</strong><span>Reload current runtime state.</span></button>
                      <button class="operator-action" id="nodeMaintenanceToggleBtn" type="button"><strong>${node.status === 'maintenance' ? 'Disable maintenance' : 'Enable maintenance'}</strong><span>Control scheduling state for this node.</span></button>
                      <button class="operator-action danger-action" id="emergencyNodeCleanupBtn" type="button"><strong>Emergency cleanup</strong><span>Remove managed services and optionally remove the agent.</span></button>
                      <button class="operator-action danger-action" id="deleteNodeFromManageBtn" type="button"><strong>Delete node</strong><span>Retire node after instances are moved or removed.</span></button>
                    </div>
                    <div id="nodeRuntimeActionResult" class="form-result"></div>
                  </div>
                </section>
              </div>
            </section>
            <section class="node-tab-panel${activeTab === 'instances' ? ' is-active' : ''}" data-node-panel="instances">
              <section class="table-card compact-card">
                <div class="table-head">
                  <div>
                    <h2>Managed Instances</h2>
                    <div class="metric-caption">Desired-state services assigned to this node.</div>
                  </div>
                  <div class="table-tools">
                    <span class="tag">${escapeHTML(String(managedInstances.length))} workloads</span>
                    <button class="secondary-btn" id="openInstancesFilteredBtn" type="button">Open in Instances</button>
                  </div>
                </div>
                <div class="table-wrap node-managed-instances-wrap">
                  <table class="node-managed-instances-table">
                    <thead>
                      <tr>
                        <th>Instance</th>
                        <th>Service</th>
                        <th>Endpoint</th>
                        <th>Desired</th>
                        <th>Runtime</th>
                        <th>Latest issue</th>
                      </tr>
                    </thead>
                    <tbody>${managedInstanceRows}</tbody>
                  </table>
                </div>
              </section>
            </section>
            <section class="node-tab-panel${activeTab === 'terminal' ? ' is-active' : ''}" data-node-panel="terminal">
              <div class="node-panel-head">
                <div><div class="eyebrow">SSH Terminal</div><h2>Browser Console</h2></div>
                <div class="section-meta">
                  ${statusTag(terminalMethod ? 'configured' : 'missing')}
                  ${terminalMethod ? `<span class="tag">${escapeHTML(terminalMethod.auth_type || 'ssh_key')}</span>` : ''}
                </div>
              </div>
              <section class="section-card web-terminal-card">
                <div class="section-head">
                  <div>
                    <div class="mini-label">Endpoint</div>
                    <h2>${escapeHTML(terminalEndpointLabel(terminalMethod))}</h2>
                  </div>
                  <div id="nodeTerminalStatus">${statusTag('disconnected')}</div>
                </div>
                <div class="section-body">
                  <div class="section-actions">
                    <button class="primary-btn" id="startNodeTerminalBtn" type="button"${terminalMethod ? '' : ' disabled'}>Start terminal</button>
                    <button class="secondary-btn" id="disconnectNodeTerminalBtn" type="button" disabled>Disconnect</button>
                    <button class="secondary-btn" id="clearNodeTerminalBtn" type="button">Clear</button>
                    <button class="secondary-btn" id="openTerminalBootstrapBtn" type="button">SSH settings</button>
                  </div>
                  <div class="web-terminal" id="nodeTerminalSurface" tabindex="0" spellcheck="false" aria-label="Node SSH terminal">
                    <pre id="nodeTerminalOutput"></pre>
                  </div>
                </div>
              </section>
            </section>
            <section class="node-tab-panel${activeTab === 'agent' ? ' is-active' : ''}" data-node-panel="agent">
            <section class="section-card">
              <div class="section-head">
                <div>
                  <div class="eyebrow">Agent Diagnostics</div>
                  <h2>Communication Health</h2>
                </div>
                <div class="section-meta">
                  ${statusTag(diag?.communication_state || 'unknown')}
                  ${statusTag(agent.token_rotation_status || 'missing')}
                </div>
              </div>
              <div class="section-body">
                ${agentNextStepPanel}
                <div class="grid cols-3">
                  <div class="card"><div class="mini-label">Communication</div><div class="metric-caption">${statusTag(communicationState)}</div><div class="metric-caption">${escapeHTML(diag?.communication_hint || 'n/a')}</div></div>
                  ${commMetricLine('Last job poll', agent.last_job_poll_at, 'agent/jobs/next')}
                  ${commMetricLine('Last inventory sync', agent.last_inventory_sync_at, 'agent/inventory')}
                  ${commMetricLine('Last discovery sync', agent.last_discovery_sync_at, 'service discovery')}
                  ${commMetricLine('Last auth failure', agent.last_auth_failure_at, agent.last_auth_failure_reason || 'none')}
                  ${commMetricLine('Registered', agent.registered_at, `version ${agent.agent_version || 'n/a'}`)}
                </div>
                <div class="section-divider"></div>
                <div class="section-actions">
                  <button class="secondary-btn" id="retryInventorySyncBtn" type="button">Retry inventory sync</button>
                  <button class="secondary-btn" id="retryDiscoverySyncBtn" type="button">Retry discovery sync</button>
                  <button class="secondary-btn" id="probeNodeChannelBtn" type="button">Channel probe</button>
                  <button class="secondary-btn" id="syncRoutePolicyBtn" type="button">Sync route policy</button>
                  <button class="secondary-btn" id="requeueStuckNodeJobBtn" type="button"${canRequeueStuckJob ? '' : ' disabled'}>Requeue stuck job</button>
                  <button class="secondary-btn" id="clearStaleRotationBtn" type="button"${canClearStaleRotation ? '' : ' disabled'}>Clear stale pending rotation</button>
                </div>
                <div id="nodeDiagnosticsActionResult" class="form-result"></div>
                <details class="details-block">
                  <summary>Technical job identifiers</summary>
                  <div class="code-block">claim_job_id = ${escapeHTML(agent.last_job_claim_job_id || 'n/a')}
result_job_id = ${escapeHTML(agent.last_job_result_job_id || 'n/a')}
claim_type = ${escapeHTML(agent.last_job_claim_type || 'n/a')}
result_type = ${escapeHTML(agent.last_job_result_type || 'n/a')}
result_status = ${escapeHTML(agent.last_job_result_status || 'n/a')}</div>
                </details>
              </div>
            </section>
            <section class="section-card">
              <div class="section-head">
                <div><div class="eyebrow">Trust Plane</div><h2>Agent Trust Lifecycle</h2></div>
              </div>
              <div class="section-body">
                <p>Use these actions only for controlled token rotation, re-enrollment or incident response.</p>
                <div class="section-actions">
                  <button class="secondary-btn" id="rotateAgentTokenBtn" type="button">Rotate agent token</button>
                  <button class="secondary-btn" id="rotateEnrollmentTokenBtn" type="button">Rotate enrollment token</button>
                  <button class="danger-btn" id="revokeAgentIdentityBtn" type="button">Revoke agent identity</button>
                </div>
                <div id="nodeTrustResult" class="form-result"></div>
              </div>
            </section>
            </section>
            <section class="node-tab-panel${activeTab === 'inventory' ? ' is-active' : ''}" data-node-panel="inventory">
            ${renderInventorySnapshotPanel(latestInventory, node)}
            </section>
            <section class="node-tab-panel${activeTab === 'services' ? ' is-active' : ''}" data-node-panel="services">
            <section class="table-card compact-card">
              <div class="table-head"><h2>Discovered Services</h2><div class="table-tools"><span class="tag">${escapeHTML(String(recentDiscoveries.length))} visible</span><span class="tag">${escapeHTML(serviceMix)}</span></div></div>
              <div class="table-wrap">
                <table>
                  <thead><tr><th>Service</th><th>Name</th><th>Status</th><th>Endpoint</th><th>Detected</th></tr></thead>
                  <tbody>${recentDiscoveryRows}</tbody>
                </table>
              </div>
            </section>
            </section>
            <section class="node-tab-panel node-bootstrap-panel${activeTab === 'bootstrap' ? ' is-active' : ''}" data-node-panel="bootstrap">
              <div class="node-panel-head">
                <div><div class="eyebrow">Bootstrap Pipeline</div><h2>SSH Access, Tokens and Jobs</h2></div>
                <div class="section-meta">${statusTag(diag?.last_bootstrap?.status || 'not_started')}</div>
              </div>
              <div class="node-modal-grid compact-node-grid">
                <div class="stack">
                  <section class="section-card">
                    <div class="section-head">
                      <div><div class="eyebrow">Access</div><h2>SSH Bootstrap Access</h2></div>
                    </div>
                    <div class="section-body">
                      <form id="sshAccessForm" class="form-grid">
                        <div class="field"><label>SSH host</label><input name="ssh_host" required value="${escapeHTML(sshMethod?.ssh_host || node.address || '')}" /></div>
                        <div class="field"><label>SSH user</label><input name="ssh_user" required value="${escapeHTML(sshMethod?.ssh_user || 'root')}" /></div>
                        <div class="field"><label>SSH port</label><input name="ssh_port" type="number" min="1" max="65535" value="${escapeHTML(String(sshMethod?.ssh_port || 22))}" /></div>
                        <div class="field"><label>Auth type</label><select name="auth_type"><option value="ssh_key"${sshMethod?.auth_type === 'ssh_key' ? ' selected' : ''}>ssh_key</option><option value="password"${sshMethod?.auth_type === 'password' ? ' selected' : ''}>password</option><option value="token"${sshMethod?.auth_type === 'token' ? ' selected' : ''}>token</option></select></div>
                        <div class="field"><label>Secret type</label><select name="secret_type"><option value="ssh_key">ssh_key</option><option value="password">password</option><option value="api_token">api_token</option><option value="opaque">opaque</option></select></div>
                        <div class="field"><label>Enabled</label><select name="is_enabled"><option value="true"${sshMethod?.is_enabled !== false ? ' selected' : ''}>true</option><option value="false"${sshMethod?.is_enabled === false ? ' selected' : ''}>false</option></select></div>
                        <div class="field full"><label>SSH host key SHA256</label><input name="ssh_host_key_sha256" required value="${escapeHTML(sshMethod?.ssh_host_key_sha256 || '')}" placeholder="SHA256:..." /></div>
                        <div class="field full"><label>Secret value</label><textarea name="secret_value" rows="5" placeholder="${sshMethod?.secret_ref_id ? 'Leave empty to keep existing secret_ref_id.' : 'Paste SSH private key, password or token.'}"></textarea></div>
                        <div class="field full inline-actions">
                          <button class="primary-btn" type="submit">Save SSH access</button>
                          <button class="secondary-btn" type="button" id="cancelSshAccessBtn">Cancel changes</button>
                          <button class="danger-btn" type="button" id="removeSshAccessBtn">Remove SSH access</button>
                        </div>
                      </form>
                      <div id="sshAccessResult" class="form-result"></div>
                    </div>
                  </section>
                  <section class="table-card compact-card">
                    <div class="table-head"><h2>Access Methods</h2><span class="tag">${escapeHTML(String(methods.length))} configured</span></div>
                    <div class="table-wrap">
                      <table>
                        <thead><tr><th>Method</th><th>Endpoint</th><th>User</th><th>Auth</th><th>Host key</th><th>Status</th></tr></thead>
                        <tbody>${methodRows}</tbody>
                      </table>
                    </div>
                  </section>
                </div>
                <div class="stack">
                  <section class="section-card">
                    <div class="section-head">
                      <div><div class="eyebrow">Run</div><h2>Bootstrap Job</h2></div>
                    </div>
                    <div class="section-body">
                      <div class="form-grid">
                        <div class="field"><label>Bootstrap mode</label><select id="bootstrapMode"><option value="ssh_bootstrap">ssh_bootstrap</option><option value="manual_bundle">manual_bundle</option></select></div>
                        <div class="field inline-actions align-end"><button class="primary-btn" id="queueBootstrapBtn" type="button">Queue bootstrap</button><button class="secondary-btn" id="reinstallAgentBtn" type="button">Reinstall agent</button><button class="secondary-btn" id="reenrollAgentBtn" type="button">Re-enroll agent</button><button class="secondary-btn" id="refreshNodeBootstrapBtn" type="button">Refresh</button></div>
                      </div>
                      <div id="bootstrapJobResult" class="form-result"></div>
                    </div>
                  </section>
                  <section class="section-card">
                    <div class="section-head">
                      <div><div class="eyebrow">Enrollment</div><h2>Enrollment Tokens</h2></div>
                    </div>
                    <div class="section-body">
                      <div class="form-grid">
                        <div class="field"><label>TTL hours</label><input id="enrollmentTtlHours" type="number" min="1" max="720" value="24" /></div>
                        <div class="field inline-actions align-end"><button class="secondary-btn" id="createEnrollmentTokenBtn" type="button">Create token</button></div>
                      </div>
                      <div id="enrollmentTokenResult" class="form-result"></div>
                    </div>
                  </section>
                </div>
              </div>
              <section class="table-card compact-card">
                <div class="table-head"><h2>Enrollment Tokens</h2><span class="tag">${escapeHTML(String(tokens.length))}</span></div>
                <div class="table-wrap"><table><thead><tr><th>Token</th><th>Status</th><th>Expires</th><th>Used</th></tr></thead><tbody>${tokenRows}</tbody></table></div>
              </section>
              <section class="table-card compact-card">
                <div class="table-head"><h2>Bootstrap Runs</h2><span class="tag">${escapeHTML(String(runs.length))}</span></div>
                <div class="table-wrap"><table><thead><tr><th>Mode</th><th>Status</th><th>Started</th><th>Result</th><th>Actions</th></tr></thead><tbody>${runRows}</tbody></table></div>
              </section>
            </section>
          </div>
        </div>`;

      bindNodeConsoleTabs((tabID) => setNodeManageActiveTab(node.id, tabID));
      document.getElementById('sshAccessForm').addEventListener('submit', (event) => saveSSHAccess(event, node, methods));
      document.getElementById('cancelSshAccessBtn').addEventListener('click', () => reloadNodeControlModal(node.id, 'Unsaved SSH access changes discarded.'));
      document.getElementById('removeSshAccessBtn').addEventListener('click', () => removeSSHAccess(node, methods));
      document.getElementById('retryInventorySyncBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'inventory'));
      document.getElementById('retryDiscoverySyncBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'discover'));
      document.getElementById('probeNodeChannelBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'probe'));
      document.getElementById('syncRoutePolicyBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'routes'));
      document.getElementById('requeueStuckNodeJobBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'requeue'));
      document.getElementById('clearStaleRotationBtn').addEventListener('click', () => runNodeDiagnosticsAction(node, 'clear_rotation'));
      document.getElementById('nodeMaintenanceToggleBtn').addEventListener('click', () => toggleNodeMaintenance(node));
      document.getElementById('createEnrollmentTokenBtn').addEventListener('click', () => createEnrollmentToken(node));
      document.getElementById('rotateEnrollmentTokenBtn').addEventListener('click', () => rotateEnrollmentToken(node));
      document.getElementById('queueBootstrapBtn').addEventListener('click', () => queueBootstrap(node));
      document.getElementById('reinstallAgentBtn').addEventListener('click', () => queueBootstrap(node, { reinstall_agent: true }));
      document.getElementById('reenrollAgentBtn').addEventListener('click', () => queueBootstrap(node, { reinstall_agent: true, force_reenroll: true }));
      document.getElementById('rotateAgentTokenBtn').addEventListener('click', () => rotateAgentToken(node));
      document.getElementById('revokeAgentIdentityBtn').addEventListener('click', () => revokeAgentIdentity(node));
      document.getElementById('refreshNodeRuntimeBtn').addEventListener('click', () => reloadNodeControlModal(node.id, 'Node runtime state refreshed.'));
      document.getElementById('refreshNodeBootstrapBtn').addEventListener('click', () => reloadNodeControlModal(node.id, 'Bootstrap state refreshed.'));
      document.getElementById('editNodeFromManageBtn').addEventListener('click', () => openEditNodeModal(node.id));
      document.getElementById('openNodeInstancesBtn')?.addEventListener('click', () => switchNodeConsoleTab('instances'));
      document.getElementById('openInstancesFilteredBtn')?.addEventListener('click', () => openInstancesForNode(node));
      document.getElementById('emergencyNodeCleanupBtn').addEventListener('click', () => openEmergencyNodeCleanupModal(node));
      document.getElementById('deleteNodeFromManageBtn').addEventListener('click', () => openDeleteNodeModal(node.id, node.name));
      document.getElementById('openNodeTerminalBtn')?.addEventListener('click', () => switchNodeConsoleTab('terminal'));
      document.getElementById('openTerminalBootstrapBtn')?.addEventListener('click', () => switchNodeConsoleTab('bootstrap'));
      bindNodeTerminal(node, terminalMethod);
      document.getElementById('openBootstrapFromAgentBtn')?.addEventListener('click', () => switchNodeConsoleTab('bootstrap'));
      document.getElementById('editSetupFromAgentBtn')?.addEventListener('click', () => openEditNodeModal(node.id));
      document.querySelectorAll('.bootstrap-run-view-btn').forEach((button) => {
        button.addEventListener('click', () => viewBootstrapRun(node.id, button.dataset.runId, button.dataset.jobId));
      });
    }

    async function reloadNodeControlModal(nodeID, flash) {
      if (state.nodeTerminalActive) {
        disconnectNodeTerminal();
      }
      const [node, diag, methods, runs, tokens] = await Promise.all([
        requestJSON(`/api/v1/nodes/${nodeID}`),
        requestJSON(`/api/v1/nodes/${nodeID}/diagnostics`),
        requestJSON(`/api/v1/nodes/${nodeID}/access-methods`),
        requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`),
        requestJSON(`/api/v1/nodes/${nodeID}/enrollment-tokens`),
      ]);
      await loadCore();
      state.nodeManageData = {
        nodeID,
        node: diag?.node || node,
        diag: diag || {},
        methods: arrayOrEmpty(methods),
        runs: arrayOrEmpty(runs),
        tokens: arrayOrEmpty(tokens),
        flash,
      };
      if (state.page === 'nodeManage' && state.nodeManageID === nodeID) {
        renderNodeManagePage();
        return;
      }
      renderNodeControlModal(diag?.node || node, diag || {}, arrayOrEmpty(methods), arrayOrEmpty(runs), arrayOrEmpty(tokens), flash);
    }

    async function saveSSHAccess(event, node, methods) {
      event.preventDefault();
      const result = document.getElementById('sshAccessResult');
      result.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const form = new FormData(event.currentTarget);
        const existingSSH = methods.find((item) => item.method === 'ssh') || null;
        let secretRefID = existingSSH?.secret_ref_id || null;
        const secretValue = String(form.get('secret_value') || '').trim();
        if (secretValue) {
          const secretRef = await sendJSON('/api/v1/secret-refs', 'POST', {
            secret_type: String(form.get('secret_type') || 'ssh_key'),
            value: secretValue,
            meta: {
              node_id: node.id,
              usage: 'node_access_method',
              method: 'ssh',
            },
          });
          secretRefID = secretRef.id;
        }
        if (!secretRefID) {
          throw new Error('secret value is required for the first SSH access save');
        }
        const sshMethod = {
          id: existingSSH?.id || '',
          method: 'ssh',
          is_enabled: String(form.get('is_enabled')) === 'true',
          ssh_host: String(form.get('ssh_host') || '').trim(),
          ssh_port: Number(form.get('ssh_port') || 22),
          ssh_user: String(form.get('ssh_user') || '').trim(),
          ssh_host_key_sha256: String(form.get('ssh_host_key_sha256') || '').trim(),
          auth_type: String(form.get('auth_type') || 'ssh_key'),
          secret_ref_id: secretRefID,
        };
        const items = methods.filter((item) => item.method !== 'ssh').map((item) => ({ ...item }));
        items.push(sshMethod);
        await sendJSON(`/api/v1/nodes/${node.id}/access-methods`, 'PUT', { items });
        await reloadNodeControlModal(node.id, 'SSH access updated.');
      } catch (err) {
        result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function removeSSHAccess(node, methods) {
      const result = document.getElementById('sshAccessResult');
      result.innerHTML = '<span class="tag warn">removing</span>';
      try {
        const items = methods.filter((item) => item.method !== 'ssh').map((item) => ({ ...item }));
        await sendJSON(`/api/v1/nodes/${node.id}/access-methods`, 'PUT', { items });
        await reloadNodeControlModal(node.id, 'SSH access removed.');
      } catch (err) {
        result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function createEnrollmentToken(node) {
      const result = document.getElementById('enrollmentTokenResult');
      result.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const ttlHours = Math.max(1, Math.min(720, Number(document.getElementById('enrollmentTtlHours').value || 24)));
        const token = await requestJSON(`/api/v1/nodes/${node.id}/enrollment-token?ttl_hours=${ttlHours}`, { method: 'POST' });
        result.innerHTML = renderActionResponse(token, 'Enrollment token created');
        await reloadNodeControlModal(node.id, 'Enrollment token created.');
      } catch (err) {
        result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function rotateEnrollmentToken(node) {
      const result = document.getElementById('enrollmentTokenResult');
      result.innerHTML = '<span class="tag warn">rotating</span>';
      try {
        const ttlHours = Math.max(1, Math.min(720, Number(document.getElementById('enrollmentTtlHours').value || 24)));
        const token = await requestJSON(`/api/v1/nodes/${node.id}/enrollment-token/rotate?ttl_hours=${ttlHours}`, { method: 'POST' });
        result.innerHTML = renderActionResponse(token, 'Enrollment token rotated');
        await reloadNodeControlModal(node.id, 'Enrollment token rotated.');
      } catch (err) {
        result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function queueBootstrap(node, options = {}) {
      const result = document.getElementById('bootstrapJobResult');
      result.innerHTML = '<span class="tag warn">queueing</span>';
      try {
        const previousHeartbeat = toMillis(node.last_heartbeat_at);
        const payload = {
          bootstrap_mode: document.getElementById('bootstrapMode').value,
          reinstall_agent: Boolean(options.reinstall_agent),
          force_reenroll: Boolean(options.force_reenroll),
        };
        const data = await sendJSON(`/api/v1/nodes/${node.id}/bootstrap`, 'POST', payload);
        result.innerHTML = renderActionResponse(data, 'Node bootstrap');
        if (data?.job?.id) {
          const finalJob = await watchJob(data.job.id, result, 'node bootstrap');
          if (payload.force_reenroll && finalJob && String(finalJob.status || '').toLowerCase() === 'succeeded') {
            await waitForNodeDiagnostics(node.id, result, 're-enroll heartbeat', (diag) => {
              const heartbeatTs = toMillis(diag?.node?.last_heartbeat_at);
              const tokenState = String(diag?.agent?.token_rotation_status || '');
              return heartbeatTs > previousHeartbeat && ['online', 'degraded'].includes(String(diag?.heartbeat_state || '')) && tokenState === 'active';
            });
          }
        }
        const flash = payload.force_reenroll
          ? 'Re-enroll workflow queued. Agent state will be cleared and enrollment will happen again.'
          : payload.reinstall_agent
            ? 'Reinstall workflow queued. Agent binary and unit will be replaced on the remote host.'
            : 'Bootstrap workflow updated. Check heartbeat and agent status below.';
        await reloadNodeControlModal(node.id, flash);
      } catch (err) {
        result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function rotateAgentToken(node) {
      const target = document.getElementById('nodeTrustResult');
      target.innerHTML = '<span class="tag warn">queueing token rotation</span>';
      try {
        const baseline = Math.max(Date.now(), toMillis(node.last_heartbeat_at));
        const job = await requestJSON(`/api/v1/nodes/${node.id}/agent-token/rotate`, { method: 'POST' });
        const finalJob = await watchJob(job.id, target, 'agent token rotate');
        if (finalJob && String(finalJob.status || '').toLowerCase() === 'succeeded') {
          await waitForNodeDiagnostics(node.id, target, 'post-rotation heartbeat', (diag) => {
            const heartbeatTs = toMillis(diag?.node?.last_heartbeat_at);
            return heartbeatTs > baseline && String(diag?.agent?.token_rotation_status || '') === 'active';
          });
        }
        await reloadNodeControlModal(node.id, 'Agent token rotation finished.');
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokeAgentIdentity(node) {
      const target = document.getElementById('nodeTrustResult');
      target.innerHTML = '<span class="tag warn">revoking identity</span>';
      try {
        const data = await requestJSON(`/api/v1/nodes/${node.id}/agent-identity/revoke`, { method: 'POST' });
        target.innerHTML = renderActionResponse(data, 'Agent identity revoked');
        await reloadNodeControlModal(node.id, 'Agent identity revoked. The node now requires a new enrollment/bootstrap path.');
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function viewBootstrapRun(nodeID, runID, jobID) {
      openModal(`Bootstrap run: ${runID}`, 'Node bootstrap result', '<div class="empty">Loading bootstrap run details...</div>');
      const modalBody = document.getElementById('modalBody');
      try {
        const runs = await requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs`);
        const run = (runs || []).find((item) => item.id === runID);
        if (!run) {
          modalBody.innerHTML = '<div class="empty">Bootstrap run not found.</div>';
          return;
        }
        let logs = [];
        if (jobID) {
          logs = await fetchJSON(`/api/v1/jobs/${jobID}/logs?limit=50`, []);
        }
        const canRevealManualBundle = run.bootstrap_mode === 'manual_bundle' && run.result_payload?.agent_bootstrapenv_available;
        const logLines = (logs || []).map((entry) => `${formatDate(entry.created_at)} [${String(entry.level || 'info').toUpperCase()}] ${entry.message}`).join('\n');
        modalBody.innerHTML = `
          <div class="grid cols-2">
            <div class="card"><div class="mini-label">Mode</div><div class="metric-caption">${escapeHTML(run.bootstrap_mode || 'n/a')}</div></div>
            <div class="card"><div class="mini-label">Status</div><div class="metric-caption">${statusTag(run.status || 'unknown')}</div></div>
          </div>
          <div class="card">
            <h2>Request payload</h2>
            <div class="code-block">${escapeHTML(JSON.stringify(run.request_payload || {}, null, 2))}</div>
          </div>
          <div class="card">
            <h2>Result payload</h2>
            ${renderActionResponse(run.result_payload || {}, 'Bootstrap result')}
          </div>
          ${canRevealManualBundle ? `
          <div class="card">
            <div class="table-head compact-head"><h2>Manual bundle</h2><button class="secondary-btn" type="button" id="manualBundleRevealBtn">Reveal bundle</button></div>
            <div id="manualBundleRevealResult" class="form-result"></div>
          </div>` : ''}
          <div class="card">
            <h2>Worker logs</h2>
            <div class="code-block">${escapeHTML(logLines || 'No logs captured for this bootstrap job yet.')}</div>
          </div>`;
        document.getElementById('manualBundleRevealBtn')?.addEventListener('click', () => revealManualBootstrapBundle(nodeID, runID));
      } catch (err) {
        modalBody.innerHTML = `<div class="empty">Failed to load bootstrap run details: ${escapeHTML(err.message)}</div>`;
      }
    }

    async function revealManualBootstrapBundle(nodeID, runID) {
      const target = document.getElementById('manualBundleRevealResult');
      if (!target) return;
      target.innerHTML = '<span class="tag warn">revealing bundle</span>';
      try {
        const bundle = await requestJSON(`/api/v1/nodes/${nodeID}/bootstrap-runs/${runID}/bundle`);
        target.innerHTML = `
          <h3>agent.env</h3>
          <div class="code-block">${escapeHTML(bundle.agent_env || '')}</div>
          <h3>agent-bootstrap.env</h3>
          <div class="code-block">${escapeHTML(bundle.agent_bootstrapenv || '')}</div>`;
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function runNodeDiagnosticsAction(node, action) {
      const target = document.getElementById('nodeDiagnosticsActionResult');
      const actions = {
        inventory: {
          label: 'inventory retry',
          path: `/api/v1/nodes/${node.id}/diagnostics/retry-inventory`,
          flash: 'Inventory retry queued from diagnostics.',
        },
        discover: {
          label: 'discovery retry',
          path: `/api/v1/nodes/${node.id}/diagnostics/retry-discovery`,
          flash: 'Discovery retry queued from diagnostics.',
        },
        probe: {
          label: 'channel probe',
          path: `/api/v1/nodes/${node.id}/diagnostics/channel-probe`,
          flash: 'Channel probe finished.',
        },
        routes: {
          label: 'route policy sync',
          path: `/api/v1/nodes/${node.id}/routes/apply`,
          flash: 'Route policy snapshot applied.',
        },
        requeue: {
          label: 'stuck job requeue',
          path: `/api/v1/nodes/${node.id}/diagnostics/requeue-stuck-job`,
          flash: 'Stale claimed job was requeued.',
        },
        clear_rotation: {
          label: 'clear stale rotation',
          path: `/api/v1/nodes/${node.id}/diagnostics/clear-stale-rotation`,
          flash: 'Stale pending rotation was cleared.',
        },
      };
      const cfg = actions[action];
      if (!cfg) return;

      target.innerHTML = `<span class="tag warn">running ${escapeHTML(cfg.label)}</span>`;
      try {
        const data = await requestJSON(cfg.path, { method: 'POST' });
        const job = data?.job || null;
        if (job?.id) {
          await watchJob(job.id, target, cfg.label);
        } else {
          target.innerHTML = renderActionResponse(data, cfg.label);
        }
        await reloadNodeControlModal(node.id, cfg.flash);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function toggleNodeMaintenance(node) {
      const target = document.getElementById('nodeRuntimeActionResult');
      target.innerHTML = '<span class="tag warn">updating maintenance</span>';
      try {
        const path = node.status === 'maintenance'
          ? `/api/v1/nodes/${node.id}/maintenance/disable`
          : `/api/v1/nodes/${node.id}/maintenance/enable`;
        const data = await requestJSON(path, { method: 'POST' });
        target.innerHTML = renderActionResponse(data, 'Node maintenance');
        await reloadNodeControlModal(node.id, 'Node maintenance state updated.');
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    return {
      openCreateNodeModal,
      openEditNodeModal,
      openDeleteNodeModal,
      openNodeControlModal,
      disconnectNodeTerminal,
      renderNodeManagePage,
      reloadNodeControlModal,
      saveSSHAccess,
      removeSSHAccess,
      createEnrollmentToken,
      rotateEnrollmentToken,
      queueBootstrap,
      rotateAgentToken,
      revokeAgentIdentity,
      viewBootstrapRun,
      runNodeDiagnosticsAction,
      toggleNodeMaintenance,
    };
  }

  window.MegaVPNNodeWorkflows = { create: createNodeWorkflows };
})(window);
