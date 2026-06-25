(function (window) {
  'use strict';

  function createNodeWorkflows(ctx = {}) {
    const {
      state,
      nodeUI,
      requestJSON,
      sendJSON,
      fetchJSON,
      refresh,
      loadNodeManagePageData,
      loadCore,
      renderNodeManagePage,
      renderNodeControlModal,
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
    } = ctx;
    if (
      !state ||
      !nodeUI ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof fetchJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof loadNodeManagePageData !== 'function' ||
      typeof loadCore !== 'function' ||
      typeof renderNodeManagePage !== 'function' ||
      typeof renderNodeControlModal !== 'function' ||
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
      typeof formatDate !== 'function'
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

    function arrayOrEmpty(value) {
      return Array.isArray(value) ? value : [];
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

    async function reloadNodeControlModal(nodeID, flash) {
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
