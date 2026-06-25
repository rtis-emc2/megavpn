(function (window) {
  'use strict';

  function createInstanceWorkflows(ctx = {}) {
    const {
      state,
      domainUI,
      requestJSON,
      sendJSON,
      refresh,
      openModal,
      closeModal,
      openActionOutcomeModal,
      renderNotice,
      setSubmitBusy,
      watchJob,
      statusTag,
      escapeHTML,
      renderActionResponse,
    } = ctx;
    if (
      !state ||
      !domainUI ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof renderNotice !== 'function' ||
      typeof setSubmitBusy !== 'function' ||
      typeof watchJob !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof renderActionResponse !== 'function'
    ) {
      throw new Error('MegaVPNInstanceWorkflows requires workflow dependencies');
    }

    const {
      instanceServiceOptions,
      defaultServicePack,
      servicePackOptions,
      certificateOptions,
      nodeOptions,
      normalizeInstanceServiceCode,
      stringValue,
      buildInstanceSpecDraft,
      syncInstanceServiceFields,
      buildInstanceSpecPayload,
      syncCreateServicePackDefaults,
    } = domainUI;

    function openCreateServicePackModal() {
      const initialPack = defaultServicePack();
      const hasPack = Boolean(initialPack);
      openModal('Create instance from pack', 'POST /api/v1/service-packs/{key}/instances', `
        <form id="createServicePackForm" class="form-grid">
          <div class="field"><label>Node</label><select name="node_id" required>${nodeOptions()}</select></div>
          <div class="field"><label>Service pack</label><select name="service_pack_key" required${hasPack ? '' : ' disabled'}>${servicePackOptions(initialPack?.key || '')}</select></div>
          <div class="field"><label>Base name</label><input name="base_name" required placeholder="${escapeHTML(initialPack?.base_name_template || 'edge-service-pack')}" /></div>
          <div class="field"><label>Endpoint host</label><input name="endpoint_host" placeholder="${escapeHTML(initialPack?.endpoint_hint || 'edge.example.com')}" /></div>
          <div class="field"><label>Managed certificate</label><select name="certificate_id">${certificateOptions('', true)}</select></div>
          <div id="servicePackFields" class="form-grid full"></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit"${hasPack ? '' : ' disabled title="No active service pack is available"'}>Create from pack</button></div>
        </form>
        <div id="createServicePackResult" class="form-result"></div>`, { wide: true });
      const form = document.getElementById('createServicePackForm');
      const packSelect = form.querySelector('select[name="service_pack_key"]');
      syncCreateServicePackDefaults(form, packSelect.value);
      packSelect.addEventListener('change', () => syncCreateServicePackDefaults(form, packSelect.value));
      form.addEventListener('submit', createServicePack);
    }

    function openCreateInstanceChoiceModal() {
      openModal('Create instance', 'Choose creation mode', `
        <div class="response-grid">
          <div class="card">
            <div class="mini-label">Catalog model</div>
            <h3>Create from pack</h3>
            <p>Use an approved service pack template from the platform catalog.</p>
            <button class="primary-btn" id="createFromPackChoiceBtn" type="button">Create from pack</button>
          </div>
          <div class="card">
            <div class="mini-label">Custom model</div>
            <h3>Manual instance</h3>
            <p>Build one instance directly from service, endpoint, and runtime spec.</p>
            <button class="secondary-btn" id="manualInstanceChoiceBtn" type="button">Manual instance</button>
          </div>
        </div>`, { wide: true });
      document.getElementById('createFromPackChoiceBtn')?.addEventListener('click', () => {
        closeModal();
        window.setTimeout(openCreateServicePackModal, 0);
      });
      document.getElementById('manualInstanceChoiceBtn')?.addEventListener('click', () => {
        closeModal();
        window.setTimeout(openCreateInstanceModal, 0);
      });
    }

    async function createServicePack(event) {
      event.preventDefault();
      const target = document.getElementById('createServicePackResult');
      target.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const form = new FormData(event.currentTarget);
        const packKey = String(form.get('service_pack_key') || '').trim();
        if (!packKey) {
          throw new Error('No active service pack is available. Apply migrations or enable a pack in the catalog.');
        }
        const payload = {
          node_id: String(form.get('node_id') || '').trim(),
          base_name: String(form.get('base_name') || '').trim(),
          endpoint_host: String(form.get('endpoint_host') || '').trim(),
          certificate_id: String(form.get('certificate_id') || '').trim(),
        };
        const data = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}/instances`, 'POST', payload);
        target.innerHTML = renderActionResponse(data, 'Service pack creation');
        await refresh();
        window.setTimeout(closeModal, 500);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openCreateInstanceModal() {
      openModal('Create instance', 'POST /api/v1/instances', `
        <form id="createInstanceForm" class="form-grid">
          <div class="field"><label>Node</label><select name="node_id" required>${nodeOptions()}</select></div>
          <div class="field"><label>Service</label><select name="service_code" required>${instanceServiceOptions()}</select></div>
          <div class="field"><label>Name</label><input name="name" required placeholder="edge-xray-reality" /></div>
          <div class="field"><label>Slug</label><input name="slug" placeholder="optional" /></div>
          <div class="field"><label>Endpoint host</label><input name="endpoint_host" placeholder="vpn.example.com" /></div>
          <div class="field"><label>Endpoint port</label><input name="endpoint_port" type="number" min="0" max="65535" value="0" /></div>
          <div class="field"><label>Systemd unit</label><input name="systemd_unit" placeholder="optional override" /></div>
          <div id="instanceServiceFields" class="form-grid service-fields full"></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Create instance</button></div>
        </form>
        <div id="createInstanceResult" class="form-result"></div>`);
      const form = document.getElementById('createInstanceForm');
      const serviceSelect = form.querySelector('select[name="service_code"]');
      syncInstanceServiceFields('createInstanceForm', serviceSelect.value, null, { forceDefaults: true });
      serviceSelect.addEventListener('change', () => syncInstanceServiceFields('createInstanceForm', serviceSelect.value, null, { forceDefaults: true }));
      form.addEventListener('submit', createInstance);
    }

    async function createInstance(event) {
      event.preventDefault();
      const target = document.getElementById('createInstanceResult');
      target.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const form = new FormData(event.currentTarget);
        const serviceCode = normalizeInstanceServiceCode(form.get('service_code'));
        const payload = {
          node_id: String(form.get('node_id') || '').trim(),
          service_code: serviceCode,
          name: String(form.get('name') || '').trim(),
          slug: String(form.get('slug') || '').trim(),
          systemd_unit: String(form.get('systemd_unit') || '').trim(),
          endpoint_host: String(form.get('endpoint_host') || '').trim(),
          endpoint_port: Number(form.get('endpoint_port') || 0),
          spec: buildInstanceSpecPayload(serviceCode, form, {}, Number(form.get('endpoint_port') || 0)),
        };
        const data = await sendJSON('/api/v1/instances', 'POST', payload);
        target.innerHTML = renderActionResponse(data, 'Instance creation');
        await refresh();
        window.setTimeout(closeModal, 400);
      } catch (err) {
        target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function runInstanceAction(instanceID, action, targetID = null) {
      const target = typeof targetID === 'string' ? document.getElementById(targetID) : targetID;
      if (target) target.innerHTML = `<span class="tag warn">queueing ${escapeHTML(action)}</span>`;
      const job = await requestJSON(`/api/v1/instances/${instanceID}/${action}`, { method: 'POST' });
      if (target) {
        await watchJob(job.id, target, `${action} instance`);
      }
      await refresh();
      return job;
    }

    async function queueInstanceAction(instanceID, action) {
      const actionLabel = `${action} instance`;
      const buttonSelector = `.instance-action-btn[data-instance-id="${CSS.escape(instanceID)}"][data-action="${CSS.escape(action)}"]`;
      const button = document.querySelector(buttonSelector);
      if (button) {
        button.disabled = true;
        button.textContent = `${action}...`;
      }
      try {
        await runInstanceAction(instanceID, action);
      } catch (err) {
        state.lastError = err;
        renderNotice();
        openModal(actionLabel, 'Instance action failed', `<div class="code-block">${escapeHTML(err.message)}</div>`);
      } finally {
        if (button) {
          button.disabled = false;
          button.textContent = action.charAt(0).toUpperCase() + action.slice(1);
        }
      }
    }

    async function openInstanceManageModal(instanceID) {
      openModal('Instance manage', 'Loading current desired state', '<div class="empty">Loading instance spec...</div>');
      try {
        const instance = await requestJSON(`/api/v1/instances/${instanceID}`);
        const draft = buildInstanceSpecDraft(instance.service_code, instance);
        openModal(`Manage instance: ${instance.name}`, 'Desired state, revisions and apply feedback', `
          <div class="grid cols-2">
            <div class="card">
              <div class="mini-label">Runtime summary</div>
              <div class="timeline">
                <div class="timeline-item"><strong>Service</strong><div class="timeline-meta">${escapeHTML(instance.service_code || 'unknown')}</div></div>
                <div class="timeline-item"><strong>Node</strong><div class="timeline-meta">${escapeHTML(instance.node_id || 'n/a')}</div></div>
                <div class="timeline-item"><strong>Endpoint</strong><div class="timeline-meta">${escapeHTML(instance.endpoint_host || 'n/a')}:${escapeHTML(instance.endpoint_port || 0)}</div></div>
                <div class="timeline-item"><strong>Systemd</strong><div class="timeline-meta">${escapeHTML(instance.systemd_unit || 'n/a')}</div></div>
              </div>
            </div>
            <div class="card">
              <div class="mini-label">Current state</div>
              <div class="inline-actions">
                ${statusTag(instance.status || 'unknown')}
                <span class="tag">${escapeHTML(instance.slug || 'no-slug')}</span>
                ${instance.current_revision_id ? `<span class="tag">rev ${escapeHTML(instance.current_revision_id.slice(0, 8))}</span>` : ''}
                ${instance.last_applied_revision_id ? `<span class="tag ok">applied ${escapeHTML(instance.last_applied_revision_id.slice(0, 8))}</span>` : ''}
              </div>
              <p>Сохранение ниже создает новую revision. Apply остается отдельным действием и будет показан с live job feedback и logs.</p>
            </div>
          </div>
          <form id="editInstanceForm" class="form-grid">
            <input type="hidden" name="slug" value="${escapeHTML(instance.slug || '')}" />
            <div class="field"><label>Endpoint port</label><input name="endpoint_port" type="number" min="0" max="65535" value="${escapeHTML(draft.endpoint_port || instance.endpoint_port || 0)}" /></div>
            <div class="field"><label>Service code</label><input value="${escapeHTML(instance.service_code || '')}" disabled /></div>
            <div class="form-grid service-fields full"></div>
            <div class="field full inline-actions">
              <button class="secondary-btn" type="submit">Save revision</button>
              <button class="primary-btn" type="button" id="saveApplyInstanceBtn">Save and apply</button>
              <button class="secondary-btn" type="button" id="restartInstanceBtn">Restart only</button>
            </div>
          </form>
          <div id="instanceManageRevisionResult" class="form-result"></div>
          <div id="instanceManageJobResult" class="form-result"></div>`);
        syncInstanceServiceFields('editInstanceForm', instance.service_code, draft);
        const form = document.getElementById('editInstanceForm');
        form.addEventListener('submit', (event) => saveManagedInstanceSpec(event, instance, false));
        document.getElementById('saveApplyInstanceBtn').addEventListener('click', (event) => saveManagedInstanceSpec(event, instance, true));
        document.getElementById('restartInstanceBtn').addEventListener('click', async () => {
          const jobTarget = document.getElementById('instanceManageJobResult');
          try {
            await runInstanceAction(instance.id, 'restart', jobTarget);
          } catch (err) {
            jobTarget.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
          }
        });
      } catch (err) {
        document.getElementById('modalBody').innerHTML = `<div class="empty">Failed to load instance: ${escapeHTML(err.message)}</div>`;
      }
    }

    async function saveManagedInstanceSpec(event, instance, applyAfterSave) {
      event.preventDefault();
      const revisionTarget = document.getElementById('instanceManageRevisionResult');
      const jobTarget = document.getElementById('instanceManageJobResult');
      const formEl = document.getElementById('editInstanceForm');
      if (revisionTarget) revisionTarget.innerHTML = '<span class="tag warn">saving revision</span>';
      if (jobTarget) jobTarget.innerHTML = '';
      setSubmitBusy(formEl, true, 'Saving...');
      try {
        const form = new FormData(formEl);
        const spec = buildInstanceSpecPayload(instance.service_code, form, instance.spec || {}, Number(form.get('endpoint_port') || instance.endpoint_port || 0));
        const data = await sendJSON(`/api/v1/instances/${instance.id}/spec`, 'PUT', { spec });
        instance.spec = spec;
        const revision = data?.revision || {};
        const issueCount = Array.isArray(revision.validation_errors) ? revision.validation_errors.length : Number(data?.issue_count || 0);
        if (revisionTarget) revisionTarget.innerHTML = `
          <div class="card">
            <div class="inline-actions" style="justify-content:space-between;align-items:flex-start">
              <div>
                <div class="mini-label">Revision saved</div>
                <div class="metric-caption">${escapeHTML(String(data?.message || 'Desired state updated.'))}</div>
              </div>
              ${statusTag(revision.status || 'unknown')}
            </div>
            <div class="grid cols-2" style="margin-top:14px">
              <div class="card"><div class="mini-label">Revision</div><div class="metric-caption">#${escapeHTML(revision.revision_no || 'n/a')}</div></div>
              <div class="card"><div class="mini-label">Can apply</div><div class="metric-caption">${data?.can_apply ? 'yes' : 'no'}</div></div>
              <div class="card"><div class="mini-label">Rendered hash</div><div class="metric-caption">${escapeHTML(revision.rendered_hash || 'n/a')}</div></div>
              <div class="card"><div class="mini-label">Validation issues</div><div class="metric-caption">${escapeHTML(String(issueCount))}</div></div>
            </div>
            ${issueCount ? `<div class="code-block" style="margin-top:14px">${escapeHTML(JSON.stringify(revision.validation_errors || [], null, 2))}</div>` : ''}
          </div>`;
        await refresh();
        if (applyAfterSave && data?.can_apply && jobTarget) {
          await runInstanceAction(instance.id, 'apply', jobTarget);
        } else if (applyAfterSave && jobTarget) {
          jobTarget.innerHTML = '<span class="tag danger">apply blocked: revision is not validated</span>';
        }
      } catch (err) {
        if (revisionTarget) revisionTarget.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      } finally {
        setSubmitBusy(formEl, false);
      }
    }

    return {
      openCreateInstanceModal,
      openCreateInstanceChoiceModal,
      openCreateServicePackModal,
      openInstanceManageModal,
      queueInstanceAction,
      runInstanceAction,
    };
  }

  window.MegaVPNInstanceWorkflows = { create: createInstanceWorkflows };
})(window);
