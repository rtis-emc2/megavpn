(function (window) {
  'use strict';

  function createInstanceWorkflows(ctx = {}) {
    const {
      state,
      domainUI,
      requestJSON,
      fetchJSON,
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
      formatDate,
      renderActionResponse,
    } = ctx;
    if (
      !state ||
      !domainUI ||
      typeof requestJSON !== 'function' ||
      typeof fetchJSON !== 'function' ||
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
      typeof formatDate !== 'function' ||
      typeof renderActionResponse !== 'function'
    ) {
      throw new Error('MegaVPNInstanceWorkflows requires workflow dependencies');
    }

    const {
      instanceServiceOptions,
      defaultServicePack,
      servicePackByKey,
      servicePackOptions,
      certificateOptions,
      defaultLeafCertificateID,
      servicePackUsesTLSEdgeCertificate,
      servicePKIProfileOptions,
      nodeOptions,
      normalizeInstanceServiceCode,
      stringValue,
      buildInstanceSpecDraft,
      syncInstanceServiceFields,
      buildInstanceSpecPayload,
      syncCreateServicePackDefaults,
    } = domainUI;

    function instanceEndpoint(instance) {
      const host = String(instance?.endpoint_host || '').trim();
      const port = Number(instance?.endpoint_port || 0);
      if (!host && !port) return 'n/a';
      if (!host) return String(port);
      if (!port) return host;
      return `${host}:${port}`;
    }

    function nodeForInstance(instance) {
      return (state.nodes || []).find((node) => node.id === instance?.node_id) || null;
    }

    function serviceDefinition(serviceCode) {
      const code = normalizeInstanceServiceCode(serviceCode);
      return (state.servicesCatalog || []).find((item) => item.code === code || (code === 'xray-core' && item.code === 'xray')) || null;
    }

    function serviceDisplayName(serviceCode) {
      const definition = serviceDefinition(serviceCode);
      return definition?.label || definition?.display_name || definition?.name || normalizeInstanceServiceCode(serviceCode) || 'runtime';
    }

    function shortID(value, left = 8, right = 6) {
      const text = String(value || '').trim();
      if (!text) return 'n/a';
      if (text.length <= left + right + 1) return text;
      return `${text.slice(0, left)}…${text.slice(-right)}`;
    }

    function firstDiagnosticText(...values) {
      for (const value of values) {
        if (Array.isArray(value)) {
          const joined = value.map((item) => String(item || '').trim()).filter(Boolean).join(' · ');
          if (joined) return joined;
          continue;
        }
        const text = String(value ?? '').trim();
        if (text) return text;
      }
      return '';
    }

    function diagnosticResult(runtimeState, observations, latestJob) {
      const latestObservation = Array.isArray(observations) && observations.length ? observations[0] : null;
      return latestJob?.result || latestObservation?.result || runtimeState?.result || {};
    }

    function renderOutputBlock(title, value) {
      const text = String(value || '').trim();
      if (!text) return '';
      return `
        <details class="response-raw" open>
          <summary>${escapeHTML(title)}</summary>
          <div class="code-block">${escapeHTML(text)}</div>
        </details>`;
    }

    function renderInstanceEvidence(runtimeState, observations, latestJob, latestJobLogs) {
      const rows = [];
      if (runtimeState) {
        rows.push([
          'Runtime state',
          firstDiagnosticText(
            runtimeState.runtime_status,
            runtimeState.active_state,
            runtimeState.health_status,
            runtimeState.drift_status
          ),
          firstDiagnosticText(runtimeState.error_text, runtimeState.health_reasons, runtimeState.drift_reasons),
          runtimeState.updated_at || runtimeState.checked_at,
        ]);
      }
      (Array.isArray(observations) ? observations : []).slice(0, 5).forEach((item) => {
        rows.push([
          item.source || 'runtime observation',
          firstDiagnosticText(item.runtime_status, item.active_state, item.health_status, item.drift_status),
          firstDiagnosticText(item.error_text, item.health_reasons, item.drift_reasons, item.result?.message, item.result?.error),
          item.observed_at || item.received_at,
        ]);
      });
      const result = diagnosticResult(runtimeState, observations, latestJob);
      const failedCommand = firstDiagnosticText(result.last_failed_command);
      const failedExitCode = firstDiagnosticText(result.last_failed_exit_code);
      const failedOutput = firstDiagnosticText(result.last_failed_output, result.output, result.systemctl_status_output, result.journal_tail);
      const jobID = runtimeState?.last_job_id || (Array.isArray(observations) && observations[0]?.last_job_id) || latestJob?.id || '';
      const logRows = Array.isArray(latestJobLogs) ? latestJobLogs : [];
      return `
        <section class="card instance-diagnostics">
          <div class="table-head compact-head">
            <div>
              <h3>Operational diagnostics</h3>
              <p>Последнее состояние control plane, agent job и node-side evidence по этому instance.</p>
            </div>
            <div class="inline-actions">
              ${jobID ? `<span class="tag">job ${escapeHTML(shortID(jobID))}</span>` : '<span class="tag stub">no job</span>'}
              ${latestJob?.status ? statusTag(latestJob.status) : ''}
              <button class="secondary-btn small-action" type="button" id="collectInstanceDiagnosticsBtn">Collect node diagnostics</button>
            </div>
          </div>
          <div class="grid cols-2">
            <div class="response-fact">
              <span>Last job</span>
              <strong>${escapeHTML(firstDiagnosticText(latestJob?.type, runtimeState?.last_job_type, 'n/a'))}</strong>
            </div>
            <div class="response-fact">
              <span>Last job status</span>
              <strong>${escapeHTML(firstDiagnosticText(latestJob?.status, runtimeState?.last_job_status, 'n/a'))}</strong>
            </div>
            <div class="response-fact">
              <span>Failed command</span>
              <strong>${escapeHTML(failedCommand || 'n/a')}</strong>
            </div>
            <div class="response-fact">
              <span>Exit code</span>
              <strong>${escapeHTML(failedExitCode || 'n/a')}</strong>
            </div>
          </div>
          ${renderOutputBlock('Last failed output / service output', failedOutput)}
          <div class="table-head compact-head" style="margin-top:14px">
            <h3>Runtime timeline</h3>
            <span class="tag">${escapeHTML(String(rows.length))} records</span>
          </div>
          ${rows.length ? `
            <div class="timeline">
              ${rows.map(([source, stateText, reason, observedAt]) => `
                <div class="timeline-item">
                  <strong>${escapeHTML(source)} · ${escapeHTML(stateText || 'unknown')}</strong>
                  <div class="timeline-meta">${escapeHTML(formatDate(observedAt))}</div>
                  ${reason ? `<div class="metric-caption">${escapeHTML(reason)}</div>` : ''}
                </div>`).join('')}
            </div>` : '<div class="empty compact-empty">No runtime observations have been recorded yet.</div>'}
          <div class="table-head compact-head" style="margin-top:14px">
            <h3>Job logs</h3>
            <span class="tag">${escapeHTML(String(logRows.length))} entries</span>
          </div>
          ${logRows.length ? `
            <div class="timeline">
              ${logRows.slice(0, 20).map((entry) => `
                <div class="timeline-item">
                  <strong>${escapeHTML(formatDate(entry.created_at))} · ${escapeHTML(String(entry.level || 'info').toUpperCase())}</strong>
                  <div class="timeline-meta">${escapeHTML(entry.message || '')}</div>
                  ${entry.payload && Object.keys(entry.payload || {}).length ? renderActionResponse(entry.payload, 'Log payload') : ''}
                </div>`).join('')}
            </div>` : '<div class="empty compact-empty">No job log entries are available for the latest job.</div>'}
        </section>`;
    }

    function runtimeInstallSubmitLabel(serviceCode) {
      switch (normalizeInstanceServiceCode(serviceCode)) {
      case 'shadowsocks':
        return 'Install libev / ss-server';
      case 'openvpn':
        return 'Install OpenVPN';
      case 'xray-core':
        return 'Install Xray';
      default:
        return 'Install runtime';
      }
    }

    function installersForService(serviceCode) {
      const code = normalizeInstanceServiceCode(serviceCode);
      return (state.serviceInstallers || [])
        .filter((installer) => normalizeInstanceServiceCode(installer.service_code) === code)
        .sort((left, right) => {
          const leftManual = String(left.strategy || '') === 'manual_present' ? 1 : 0;
          const rightManual = String(right.strategy || '') === 'manual_present' ? 1 : 0;
          return leftManual - rightManual || String(left.strategy || '').localeCompare(String(right.strategy || ''), 'en');
        });
    }

    function installerValue(installer) {
      return `${String(installer?.strategy || '').trim()}|${String(installer?.channel || '').trim()}`;
    }

    function parseInstallerValue(value) {
      const [strategy, channel] = String(value || '').split('|');
      return { strategy: String(strategy || '').trim(), channel: String(channel || '').trim() };
    }

    function preferredInstallerValue(serviceCode, node, installers) {
      const code = normalizeInstanceServiceCode(serviceCode);
      const artifacts = binaryRepositoryArtifactsForNode(code, node);
      const preferredByService = {
        nginx: 'nginx_org_repo',
        'xray-core': artifacts.length ? 'binary_repository' : 'xtls_install_release',
        openvpn: 'ubuntu_repo',
        wireguard: 'ubuntu_repo',
        ipsec: 'ubuntu_repo',
        http_proxy: 'ubuntu_repo',
        xl2tpd: 'ubuntu_repo',
        shadowsocks: artifacts.length ? 'binary_repository' : 'ubuntu_repo',
      };
      const preferred = preferredByService[code] || '';
      const matched = (installers || []).find((installer) => String(installer.strategy || '') === preferred);
      if (matched) return installerValue(matched);
      const nonManual = (installers || []).find((installer) => String(installer.strategy || '') !== 'manual_present' && String(installer.strategy || '') !== 'binary_repository');
      if (nonManual) return installerValue(nonManual);
      return installerValue((installers || [])[0]);
    }

    function installerOptions(serviceCode, node = null) {
      const installers = installersForService(serviceCode);
      const preferred = preferredInstallerValue(serviceCode, node, installers);
      const automatic = installers.length
        ? `<option value="|" selected>Automatic · platform default</option>`
        : '';
      const options = installers.map((installer, index) => `
        <option value="${escapeHTML(installerValue(installer))}"${!automatic && (installerValue(installer) === preferred || (!preferred && index === 0)) ? ' selected' : ''}>${escapeHTML(installer.strategy || 'default')} · ${escapeHTML(installer.channel || 'default')}</option>`).join('');
      return automatic + options;
    }

    function normalizeRuntimeArchitecture(value) {
      const arch = String(value || '').trim().toLowerCase();
      if (arch === 'x86_64') return 'amd64';
      if (arch === 'aarch64') return 'arm64';
      return arch || 'amd64';
    }

    function binaryRepositoryArtifactsForNode(serviceCode, node) {
      const code = normalizeInstanceServiceCode(serviceCode);
      const osFamily = String(node?.os_family || 'linux').trim().toLowerCase() || 'linux';
      const osVersion = String(node?.os_version || '').trim();
      const arch = normalizeRuntimeArchitecture(node?.architecture);
      return (state.binaryArtifacts || []).filter((artifact) => {
        const kind = String(artifact.kind || '').trim().toLowerCase();
        const artifactOSVersion = String(artifact.os_version || '').trim();
        return String(artifact.status || 'active').toLowerCase() === 'active'
          && normalizeInstanceServiceCode(artifact.service_code) === code
          && ['runtime', 'package', 'script', 'bundle'].includes(kind)
          && String(artifact.os_family || 'linux').trim().toLowerCase() === osFamily
          && normalizeRuntimeArchitecture(artifact.architecture) === arch
          && (!artifactOSVersion || artifactOSVersion === osVersion);
      });
    }

    function runtimeInstallCatalogHint(serviceCode, node, installers) {
      const hasBinaryRepositoryInstaller = (installers || []).some((installer) => String(installer.strategy || '') === 'binary_repository');
      if (!hasBinaryRepositoryInstaller || typeof hasPermission === 'function' && !hasPermission('binary_repository.read')) return '';
      const osFamily = String(node?.os_family || 'linux').trim().toLowerCase() || 'linux';
      const osVersion = String(node?.os_version || '').trim();
      const arch = normalizeRuntimeArchitecture(node?.architecture);
      const artifacts = binaryRepositoryArtifactsForNode(serviceCode, node);
      if (artifacts.length) {
        const versions = artifacts.map((artifact) => artifact.version || artifact.name || artifact.id).filter(Boolean).slice(0, 3).join(', ');
        return `<div class="field-hint">Binary repository: ${escapeHTML(String(artifacts.length))} matching artifact${artifacts.length === 1 ? '' : 's'} for ${escapeHTML(osFamily)} / ${escapeHTML(arch)}${versions ? ` · ${escapeHTML(versions)}` : ''}.</div>`;
      }
      return `<div class="notice subtle-notice">No active binary_repository artifact matches ${escapeHTML(normalizeInstanceServiceCode(serviceCode))} for ${escapeHTML(osFamily)}${osVersion ? ` ${escapeHTML(osVersion)}` : ''} / ${escapeHTML(arch)}. Register a runtime artifact in Services -> Runtime artifacts or choose another strategy.</div>`;
    }

    function openCreateServicePackModal() {
      const initialPack = defaultServicePack();
      const hasPack = Boolean(initialPack);
      const usesTLSEdgeCertificate = servicePackUsesTLSEdgeCertificate(initialPack);
      const usesOpenVPN = Array.isArray(initialPack?.components)
        && initialPack.components.some((component) => String(component?.service_code || '').trim().toLowerCase() === 'openvpn');
      openModal('Create instance from pack', 'POST /api/v1/service-packs/{key}/instances', `
        <form id="createServicePackForm" class="form-grid">
          <div class="field"><label>Node</label><select name="node_id" required>${nodeOptions()}</select></div>
          <div class="field"><label>Service pack</label><select name="service_pack_key" required${hasPack ? '' : ' disabled'}>${servicePackOptions(initialPack?.key || '')}</select></div>
          <div class="field"><label>Base name</label><input name="base_name" required placeholder="${escapeHTML(initialPack?.base_name_template || 'edge-service-pack')}" /></div>
          <div class="field"><label>Endpoint host</label><input name="endpoint_host" placeholder="${escapeHTML(initialPack?.endpoint_hint || 'edge.example.com')}" /></div>
          <div class="field" id="servicePackCertificateField"${usesTLSEdgeCertificate ? '' : ' hidden'}>
            <label>TLS edge certificate</label>
            <select name="certificate_id"${usesTLSEdgeCertificate ? '' : ' disabled'}>${certificateOptions(defaultLeafCertificateID(), true)}</select>
            <div class="field-hint">Optional override for TLS-facing Nginx or Xray TLS components. The platform default certificate is selected automatically.</div>
          </div>
          ${usesOpenVPN ? `<div class="field"><label>OpenVPN CA profile</label><select name="openvpn_pki_profile">${servicePKIProfileOptions('openvpn', 'default')}</select><div class="field-hint">OpenVPN server/client certificates are issued from this service CA profile.</div></div>` : ''}
          <div id="servicePackFields" class="form-grid full"></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit"${hasPack ? '' : ' disabled title="No active service pack is available"'}>Create from pack</button></div>
        </form>
        <div id="createServicePackResult" class="form-result"></div>`, { wide: true });
      const form = document.getElementById('createServicePackForm');
      const packSelect = form.querySelector('select[name="service_pack_key"]');
      syncCreateServicePackDefaults(form, packSelect.value);
      syncCreateServicePackCertificateField(form, initialPack);
      packSelect.addEventListener('change', () => {
        syncCreateServicePackDefaults(form, packSelect.value);
        syncCreateServicePackCertificateField(form, servicePackByKey(packSelect.value));
      });
      form.addEventListener('submit', createServicePack);
    }

    function syncCreateServicePackCertificateField(form, pack) {
      const field = form?.querySelector('#servicePackCertificateField');
      const select = field?.querySelector('select[name="certificate_id"]');
      if (!field || !select) return;
      const usesTLSEdgeCertificate = servicePackUsesTLSEdgeCertificate(pack);
      field.hidden = !usesTLSEdgeCertificate;
      select.disabled = !usesTLSEdgeCertificate;
      if (usesTLSEdgeCertificate && !select.value) {
        select.value = defaultLeafCertificateID();
      }
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
          openvpn_pki_profile: String(form.get('openvpn_pki_profile') || '').trim(),
          auto_install_runtime: true,
        };
        const data = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}/instances`, 'POST', payload);
        target.innerHTML = renderActionResponse(data, 'Service pack creation');
        await refresh();
        window.setTimeout(closeModal, 500);
      } catch (err) {
        target.innerHTML = renderActionResponse({ error: err.message, details: err?.payload || null }, 'Service pack create failed');
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

    function openInstanceRuntimeInstallModal(instanceID, issueText = '') {
      const instance = (state.instances || []).find((item) => item.id === instanceID);
      if (!instance) {
        openActionOutcomeModal('Instance remediation', 'Instance not found', 'failed', 'Refresh the page and try again.', []);
        return;
      }
      const node = nodeForInstance(instance);
      const serviceCode = normalizeInstanceServiceCode(instance.service_code);
      const installers = installersForService(serviceCode);
      if (!node?.id) {
        openActionOutcomeModal('Instance remediation', 'Node not found', 'failed', 'This instance is not attached to a loaded active node.', [
          { label: 'Instance', value: instance.name || instance.id },
          { label: 'Node ID', value: instance.node_id || 'n/a' },
        ]);
        return;
      }
      if (!installers.length) {
        openActionOutcomeModal('Instance remediation', 'No installer registered', 'failed', 'The service catalog does not expose a runtime installer for this service.', [
          { label: 'Instance', value: instance.name || instance.id },
          { label: 'Service', value: serviceCode },
        ]);
        return;
      }
      openModal(`Install runtime: ${serviceDisplayName(serviceCode)}`, 'Instance remediation', `
        <section class="card">
          <div class="mini-label">Target</div>
          <div class="timeline">
            <div class="timeline-item"><strong>Instance</strong><div class="timeline-meta">${escapeHTML(instance.name || instance.id)}</div></div>
            <div class="timeline-item"><strong>Node</strong><div class="timeline-meta">${escapeHTML(node.name || node.id)}${node.address ? ` · ${escapeHTML(node.address)}` : ''}</div></div>
            <div class="timeline-item"><strong>Endpoint</strong><div class="timeline-meta">${escapeHTML(instanceEndpoint(instance))}</div></div>
            <div class="timeline-item"><strong>Issue</strong><div class="timeline-meta">${escapeHTML(issueText || 'Runtime capability appears to be missing on the node.')}</div></div>
          </div>
        </section>
        <form id="instanceRuntimeInstallForm" class="form-grid">
          <div class="field full">
            <label>Install strategy</label>
            <select name="installer" required>${installerOptions(serviceCode, node)}</select>
            <div class="field-hint">The job runs on the selected node through the agent capability installer.</div>
            ${runtimeInstallCatalogHint(serviceCode, node, installers)}
          </div>
          <label class="choice-card full">
            <input name="apply_after_install" type="checkbox" checked />
            <span>
              <strong>Apply instance after successful install</strong>
              <small>Queue a new instance.apply only when the runtime install job succeeds.</small>
            </span>
          </label>
          <div class="field full inline-actions">
            <button class="secondary-btn" id="cancelInstanceRuntimeInstallBtn" type="button">Cancel</button>
            <button class="primary-btn" type="submit">${escapeHTML(runtimeInstallSubmitLabel(serviceCode))}</button>
          </div>
        </form>
        <div id="instanceRuntimeInstallResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelInstanceRuntimeInstallBtn')?.addEventListener('click', closeModal);
      document.getElementById('instanceRuntimeInstallForm')?.addEventListener('submit', (event) => runInstanceRuntimeInstall(event, instance, node, serviceCode));
    }

    async function runInstanceRuntimeInstall(event, instance, node, serviceCode) {
      event.preventDefault();
      const form = event.currentTarget;
      const target = document.getElementById('instanceRuntimeInstallResult');
      const selected = parseInstallerValue(new FormData(form).get('installer'));
      const applyAfterInstall = Boolean(form.querySelector('input[name="apply_after_install"]')?.checked);
      setSubmitBusy(form, true, 'Installing...');
      if (target) target.innerHTML = '<span class="tag warn">queueing runtime install</span>';
      try {
        const installJob = await sendJSON(`/api/v1/nodes/${encodeURIComponent(node.id)}/capabilities/install`, 'POST', {
          service_code: serviceCode,
          strategy: selected.strategy,
          channel: selected.channel,
        });
        const finalInstallJob = await watchJob(installJob.id, target, 'Runtime install', {
          attempts: 80,
          intervalMs: 1500,
          context: {
            node: node.name || node.id,
            service: serviceCode,
            strategy: selected.strategy || 'default',
            channel: selected.channel || 'default',
          },
        });
        await refresh();
        if (applyAfterInstall && finalInstallJob && String(finalInstallJob.status || '').toLowerCase() === 'succeeded') {
          if (target) target.innerHTML += '<div class="form-result"><span class="tag warn">runtime installed; queueing instance apply</span></div>';
          await runInstanceAction(instance.id, 'apply', target);
        }
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      } finally {
        setSubmitBusy(form, false);
      }
    }

    async function openInstanceManageModal(instanceID) {
      openModal('Instance manage', 'Loading current desired state', '<div class="empty">Loading instance spec...</div>');
      try {
        const [instance, runtimeState, observations] = await Promise.all([
          requestJSON(`/api/v1/instances/${instanceID}`),
          fetchJSON(`/api/v1/instances/${instanceID}/runtime-state`, null),
          fetchJSON(`/api/v1/instances/${instanceID}/runtime-observations?limit=8`, []),
        ]);
        const latestJobID = runtimeState?.last_job_id || (Array.isArray(observations) && observations[0]?.last_job_id) || '';
        const [latestJob, latestJobLogs] = latestJobID ? await Promise.all([
          fetchJSON(`/api/v1/jobs/${encodeURIComponent(latestJobID)}`, null),
          fetchJSON(`/api/v1/jobs/${encodeURIComponent(latestJobID)}/logs?limit=30`, []),
        ]) : [null, []];
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
                ${runtimeState?.runtime_status ? statusTag(runtimeState.runtime_status) : ''}
                ${runtimeState?.health_status ? statusTag(runtimeState.health_status) : ''}
                ${runtimeState?.drift_status ? statusTag(runtimeState.drift_status) : ''}
                <span class="tag">${escapeHTML(instance.slug || 'no-slug')}</span>
                ${instance.current_revision_id ? `<span class="tag">rev ${escapeHTML(instance.current_revision_id.slice(0, 8))}</span>` : ''}
                ${instance.last_applied_revision_id ? `<span class="tag ok">applied ${escapeHTML(instance.last_applied_revision_id.slice(0, 8))}</span>` : ''}
              </div>
              <p>Сохранение ниже создает новую revision. Apply остается отдельным действием и будет показан с live job feedback и logs.</p>
            </div>
          </div>
          ${renderInstanceEvidence(runtimeState, observations, latestJob, latestJobLogs)}
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
        document.getElementById('collectInstanceDiagnosticsBtn')?.addEventListener('click', async () => {
          const jobTarget = document.getElementById('instanceManageJobResult');
          if (jobTarget) jobTarget.innerHTML = '<span class="tag warn">queueing diagnostics</span>';
          try {
            const job = await sendJSON(`/api/v1/instances/${instance.id}/diagnose`, 'POST', {});
            await watchJob(job.id, jobTarget, 'Instance diagnostics', {
              attempts: 30,
              intervalMs: 1500,
              context: {
                node: nodeForInstance(instance)?.name || instance.node_id,
                service: instance.service_code,
                strategy: 'read-only',
                channel: 'node-side evidence',
              },
            });
          } catch (err) {
            if (jobTarget) jobTarget.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
          }
        });
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
      openInstanceRuntimeInstallModal,
      queueInstanceAction,
      runInstanceAction,
    };
  }

  window.MegaVPNInstanceWorkflows = { create: createInstanceWorkflows };
})(window);
