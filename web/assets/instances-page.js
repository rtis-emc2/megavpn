(function (window) {
  'use strict';

  function createInstancesPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      tableCard,
      statusTag,
      escapeHTML,
      domainUI,
      renderActionResponse,
      sendJSON,
      refresh,
      setPage,
      hasPermission,
      openModal,
      closeModal,
      openActionOutcomeModal,
      openCreateInstanceModal,
      openInstanceManageModal,
      openInstanceRuntimeInstallModal,
      queueInstanceAction,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      !domainUI ||
      typeof domainUI.nodeOptions !== 'function' ||
      typeof domainUI.normalizeInstanceServiceCode !== 'function' ||
      typeof domainUI.certificateOptions !== 'function' ||
      typeof domainUI.defaultLeafCertificateID !== 'function' ||
      typeof domainUI.servicePackUsesTLSEdgeCertificate !== 'function' ||
      typeof domainUI.servicePKIProfileOptions !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof setPage !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof openCreateInstanceModal !== 'function' ||
      typeof openInstanceManageModal !== 'function' ||
      typeof openInstanceRuntimeInstallModal !== 'function' ||
      typeof queueInstanceAction !== 'function'
    ) {
      throw new Error('MegaVPNInstancesPage requires page dependencies');
    }

    const {
      certificateOptions,
      defaultLeafCertificateID,
      nodeOptions,
      normalizeInstanceServiceCode,
      servicePackUsesTLSEdgeCertificate,
      servicePKIProfileOptions,
    } = domainUI;
    const INSTANCE_PAGE_SIZE = 100;

    function instanceEndpoint(instance) {
      const host = String(instance?.endpoint_host || '').trim();
      const port = Number(instance?.endpoint_port || 0);
      if (!host && !port) return 'n/a';
      if (!host) return String(port);
      if (!port) return host;
      return `${host}:${port}`;
    }

    function revisionState(instance) {
      if (instance.current_revision_id && instance.last_applied_revision_id && instance.current_revision_id === instance.last_applied_revision_id) {
        return 'applied';
      }
      if (instance.current_revision_id) return 'pending';
      return 'n/a';
    }

    function runtimeStateFor(instanceID) {
      return (state.instanceRuntimeStates || []).find((item) => item.instance_id === instanceID) || null;
    }

    function activeNodes() {
      return (state.nodes || []).filter((node) => node.status !== 'retired');
    }

    function nodeByID(nodeID) {
      const id = String(nodeID || '').trim();
      return (state.nodes || []).find((item) => String(item.id || '').trim() === id) || null;
    }

    function nodeOptionLabel(row) {
      const role = String(row.nodeRole || '').trim();
      const address = String(row.nodeAddress || '').trim();
      return [row.node || row.nodeID || 'node', role, address].filter(Boolean).join(' · ');
    }

    function instanceListViewMode() {
      return String(state.instancesListView || 'node') === 'table' ? 'table' : 'node';
    }

    function instanceCapableServices() {
      const ranked = new Map();
      for (const service of (state.servicesCatalog || [])) {
        if (service.supports_instances === false || service.enabled === false) continue;
        const code = String(service.code || '').trim();
        if (!code) continue;
        const current = ranked.get(code);
        if (!current || service.code === code) ranked.set(code, service);
      }
      return Array.from(ranked.values());
    }

    function serviceLabel(code) {
      const normalized = String(code || '').trim();
      const service = (state.servicesCatalog || []).find((item) => String(item.code || '').trim() === normalized);
      return service?.label || service?.display_name || service?.name || normalized || 'unknown';
    }

    function canManageServicePacks() {
      return hasPermission('settings.manage');
    }

    function servicePackCatalogItems() {
      const source = canManageServicePacks()
        ? (state.servicePackCatalog || state.servicePacks || [])
        : (state.servicePacks || []);
      return (Array.isArray(source) ? source : [])
        .filter((pack) => pack && pack.key)
        .filter((pack) => String(pack.status || 'active').toLowerCase() === 'active')
        .slice()
        .sort((left, right) => Number(left.display_order || 1000) - Number(right.display_order || 1000)
          || String(left.label || left.key).localeCompare(String(right.label || right.key), 'en'));
    }

    function servicePackComponentsLabel(pack) {
      const components = Array.isArray(pack?.components) ? pack.components : [];
      if (!components.length) return 'no components';
      const counts = new Map();
      for (const component of components) {
        const label = String(component.service_code || component.label || 'component').trim() || 'component';
        counts.set(label, (counts.get(label) || 0) + 1);
      }
      return Array.from(counts.entries()).map(([label, count]) => count > 1 ? `${label} x${count}` : label).join(', ');
    }

    function primaryReason(values, fallback) {
      const list = Array.isArray(values) ? values : [];
      const first = list.map((item) => String(item || '').trim()).find(Boolean);
      return first || fallback || '';
    }

    function compactReason(value) {
      const text = String(value || '').trim();
      if (text.length <= 120) return text;
      return `${text.slice(0, 117)}...`;
    }

    function firstText(...values) {
      return values.map((value) => String(value || '').trim()).find(Boolean) || '';
    }

    function latestInstanceJob(instanceID) {
      const id = String(instanceID || '').trim();
      if (!id) return null;
      return (state.jobs || [])
        .filter((job) => jobTargetsInstance(job, id))
        .sort((left, right) => String(right.created_at || '').localeCompare(String(left.created_at || '')))[0] || null;
    }

    function jobTargetsInstance(job, instanceID) {
      const id = String(instanceID || '').trim();
      if (!id || !job) return false;
      if (String(job.instance_id || '').trim() === id
          || String(job.scope_id || '').trim() === id
          || String(job.payload?.instance_id || '').trim() === id) {
        return true;
      }
      const dependentIDs = Array.isArray(job.payload?.dependent_instance_ids) ? job.payload.dependent_instance_ids : [];
      return dependentIDs.some((item) => String(item || '').trim() === id);
    }

    function jobResultSummary(job) {
      const result = job?.result || {};
      const health = result.health || {};
      return firstText(
        result.health_reason,
        result.health_route_warning,
        health.reason,
        health.route_warning,
        result.health_error,
        health.error,
        result.error,
        result.message,
        result.active_state,
      );
    }

    function hasRuntimeInstaller(serviceCode) {
      const normalized = normalizeInstanceServiceCode(serviceCode);
      return (state.serviceInstallers || []).some((installer) => normalizeInstanceServiceCode(installer.service_code) === normalized);
    }

    function nodeCapability(nodeID, serviceCode) {
      const normalized = normalizeInstanceServiceCode(serviceCode);
      return (state.serviceCapabilitiesByNode?.[nodeID] || [])
        .find((item) => normalizeInstanceServiceCode(item.capability_code) === normalized) || null;
    }

    function runtimeCapabilityState(nodeID, serviceCode) {
      const required = hasRuntimeInstaller(serviceCode);
      if (!required) {
        return { required: false, status: 'not_required', version: '', source: '' };
      }
      const capability = nodeCapability(nodeID, serviceCode);
      return {
        required: true,
        status: String(capability?.status || 'missing').trim().toLowerCase() || 'missing',
        version: capability?.version || '',
        source: capability?.source || '',
      };
    }

    function runtimeCapabilityReady(row) {
      if (!row.capabilityRequired) return true;
      return ['available', 'installed', 'detected'].includes(String(row.capabilityStatus || '').toLowerCase());
    }

    function hasActiveInstanceJob(row) {
      return ['queued', 'running', 'retrying'].includes(lowerStatus(row?.latestJob?.status));
    }

    function activeInstanceLifecycle(row) {
      const jobType = String(row?.latestJob?.type || '').trim();
      if (jobType === 'node.capability.install') return 'installing runtime';
      if (jobType.startsWith('instance.')) return 'provisioning';
      return 'pending';
    }

    function isCapabilityMissingText(value) {
      return /capability missing|binary is not installed|not installed or not executable|runtime capability is not available/i.test(String(value || ''));
    }

    function toInstanceRow(instance) {
      const node = nodeByID(instance.node_id);
      const runtime = runtimeStateFor(instance.id);
      const latestJob = latestInstanceJob(instance.id);
      const service = instance.service_code || 'unknown';
      const capability = runtimeCapabilityState(instance.node_id, service);
      const healthReason = primaryReason(runtime?.health_reasons, 'Waiting for runtime health report.');
      const driftReason = primaryReason(runtime?.drift_reasons, 'Waiting for runtime drift report.');
      const latestJobSummary = jobResultSummary(latestJob);
      const row = {
        id: instance.id,
        name: instance.name,
        nodeID: instance.node_id || '',
        node: node?.name || instance.node_id || 'n/a',
        nodeRole: node?.role || '',
        nodeStatus: node?.status || '',
        nodeAddress: node?.address || '',
        service,
        serviceLabel: serviceLabel(service),
        endpoint: instanceEndpoint(instance),
        revision: revisionState(instance),
        status: instance.status || 'draft',
        runtime: runtime?.runtime_status || 'unknown',
        health: runtime?.health_status || 'unknown',
        drift: runtime?.drift_status || 'unknown',
        activeState: runtime?.active_state || 'unknown',
        healthReason,
        driftReason,
        latestJob,
        latestJobSummary,
        capabilityRequired: capability.required,
        capabilityStatus: capability.status,
        capabilityVersion: capability.version,
        capabilitySource: capability.source,
      };
      row.bucket = instanceBucket(row);
      row.issue = instanceIssue(row);
      row.capabilityMissing = !hasActiveInstanceJob(row)
        && ((row.capabilityRequired && !runtimeCapabilityReady(row) && lowerStatus(row.runtime) !== 'active')
        || (hasRuntimeInstaller(row.service) && isCapabilityMissingText([
        row.latestJobSummary,
        row.healthReason,
        row.driftReason,
        row.issue?.text,
      ].join(' '))));
      return row;
    }

    function lowerStatus(value) {
      return String(value || '').trim().toLowerCase();
    }

    function instanceBucket(row) {
      const status = lowerStatus(row.status);
      const runtime = lowerStatus(row.runtime);
      const health = lowerStatus(row.health);
      const drift = lowerStatus(row.drift);
      const revision = lowerStatus(row.revision);
      if (hasActiveInstanceJob(row)) return 'pending';
      if (['provisioning', 'transitioning', 'activating', 'deactivating', 'reloading'].includes(runtime)
          || ['provisioning', 'transitioning'].includes(health)
          || ['activating', 'deactivating', 'reloading'].includes(lowerStatus(row.activeState))) {
        return 'pending';
      }
      if (row.capabilityRequired && !runtimeCapabilityReady(row) && runtime !== 'active') return 'problem';
      if (status === 'failed' || runtime === 'failed' || ['failed', 'degraded', 'unhealthy', 'error'].includes(health)) {
        return 'problem';
      }
      if (revision && revision !== 'applied' && revision !== 'n/a') return 'pending';
      if (['pending_apply', 'pending', 'drifted', 'out_of_sync'].includes(drift)) return 'pending';
      if (status === 'active' && runtime === 'active' && health === 'healthy' && drift === 'in_sync') return 'healthy';
      return 'unknown';
    }

    function instanceIssue(row) {
      const status = lowerStatus(row.status);
      const runtime = lowerStatus(row.runtime);
      const health = lowerStatus(row.health);
      const drift = lowerStatus(row.drift);
      const revision = lowerStatus(row.revision);
      const reasons = [];
      const latestJobStatus = lowerStatus(row.latestJob?.status);
      const activeJob = hasActiveInstanceJob(row);
      if (row.latestJob && ['failed', 'blocked', 'cancelled'].includes(latestJobStatus)) {
        const summary = row.latestJobSummary ? `: ${row.latestJobSummary}` : '';
        reasons.push(`${row.latestJob.type || 'job'} ${latestJobStatus}${summary}`);
      } else if (activeJob) {
        reasons.push(`${row.latestJob.type || 'job'} is ${latestJobStatus}.`);
      }
      if (!activeJob && row.capabilityRequired && !runtimeCapabilityReady(row) && runtime !== 'active') {
        reasons.push(`Runtime capability ${row.service} is ${row.capabilityStatus || 'missing'} on this node.`);
      }
      if (!activeJob && status === 'failed') {
        reasons.push('Lifecycle is failed; check the latest apply job result.');
      }
      if (revision && revision !== 'applied' && revision !== 'n/a') {
        reasons.push(row.driftReason || 'Current desired revision has not been applied to the node yet.');
      } else if (['pending_apply', 'pending', 'drifted', 'out_of_sync'].includes(drift)) {
        reasons.push(row.driftReason || 'Runtime drift is not in sync with desired state.');
      }
      if (!activeJob && ['failed', 'degraded', 'unhealthy', 'error'].includes(health)) {
        reasons.push(row.healthReason || 'Runtime health status is degraded.');
      }
      if (!activeJob && status === 'active' && ['stopped', 'inactive'].includes(runtime)) {
        reasons.push('Unit is not running on the selected node.');
      }
      const bucket = instanceBucket(row);
      if (!reasons.length && bucket === 'healthy') {
        return { status: 'ok', text: 'No active issue.' };
      }
      if (!reasons.length) {
        return { status: 'unknown', text: 'Waiting for agent runtime report.' };
      }
      return {
        status: bucket === 'problem' ? 'failed' : bucket,
        text: compactReason(reasons.filter(Boolean).slice(0, 3).join(' · ')),
      };
    }

    function ensureInstanceListFilters() {
      if (!state.instanceListFilters || typeof state.instanceListFilters !== 'object') {
        state.instanceListFilters = { search: '', status: 'all', service: 'all', node: 'all' };
      }
      if (!state.instanceListFilters.search) state.instanceListFilters.search = '';
      if (!state.instanceListFilters.status) state.instanceListFilters.status = 'all';
      if (!state.instanceListFilters.service) state.instanceListFilters.service = 'all';
      if (!state.instanceListFilters.node) state.instanceListFilters.node = 'all';
      return state.instanceListFilters;
    }

    function instanceFilterOptions(rows, key, labelFn = (value) => value) {
      const values = Array.from(new Set(rows.map((row) => String(row[key] || '').trim()).filter(Boolean)))
        .sort((left, right) => left.localeCompare(right, 'en'));
      return values.map((value) => ({ value, label: labelFn(value) }));
    }

    function instanceNodeFilterOptions(rows) {
      const byNode = new Map();
      for (const row of rows) {
        const value = String(row.nodeID || row.node || '').trim();
        if (!value || byNode.has(value)) continue;
        byNode.set(value, { value, label: nodeOptionLabel(row) });
      }
      return Array.from(byNode.values()).sort((left, right) => left.label.localeCompare(right.label, 'en'));
    }

    function renderInstanceFilterOptions(options, selected) {
      return options.map((option) => `
        <option value="${escapeHTML(option.value)}"${option.value === selected ? ' selected' : ''}>${escapeHTML(option.label)}</option>`).join('');
    }

    function filterInstanceRows(rows, filters) {
      const query = String(filters.search || '').trim().toLowerCase();
      return rows.filter((row) => {
        if (filters.status !== 'all' && row.bucket !== filters.status) return false;
        if (filters.service !== 'all' && row.service !== filters.service) return false;
        if (filters.node !== 'all' && row.nodeID !== filters.node && row.node !== filters.node) return false;
        if (!query) return true;
        const haystack = [
          row.name,
          row.id,
          row.service,
          row.serviceLabel,
          row.node,
          row.nodeAddress,
          row.endpoint,
          row.status,
          row.runtime,
          row.health,
          row.drift,
          row.issue?.text,
        ].join(' ').toLowerCase();
        return haystack.includes(query);
      });
    }

    function renderInstanceListToolbar(rows, filteredRows) {
      const filters = ensureInstanceListFilters();
      const serviceOptions = instanceFilterOptions(rows, 'service', serviceLabel);
      const nodeOptionsList = instanceNodeFilterOptions(rows);
      const viewMode = instanceListViewMode();
      return `
        <div class="instance-list-toolbar">
          <div class="field compact">
            <label>Search</label>
            <input id="instanceSearchInput" value="${escapeHTML(filters.search)}" placeholder="name, node, endpoint, issue">
          </div>
          <div class="field compact">
            <label>Status</label>
            <select id="instanceStatusFilter">
              <option value="all"${filters.status === 'all' ? ' selected' : ''}>All statuses</option>
              <option value="problem"${filters.status === 'problem' ? ' selected' : ''}>Problems</option>
              <option value="pending"${filters.status === 'pending' ? ' selected' : ''}>Pending apply</option>
              <option value="healthy"${filters.status === 'healthy' ? ' selected' : ''}>Healthy</option>
              <option value="unknown"${filters.status === 'unknown' ? ' selected' : ''}>Unknown</option>
            </select>
          </div>
          <div class="field compact">
            <label>Service</label>
            <select id="instanceServiceFilter">
              <option value="all"${filters.service === 'all' ? ' selected' : ''}>All services</option>
              ${renderInstanceFilterOptions(serviceOptions, filters.service)}
            </select>
          </div>
          <div class="field compact">
            <label>Node</label>
            <select id="instanceNodeFilter">
              <option value="all"${filters.node === 'all' ? ' selected' : ''}>All nodes</option>
              ${renderInstanceFilterOptions(nodeOptionsList, filters.node)}
            </select>
          </div>
          <div class="instance-list-toolbar-actions">
            <div class="instance-view-switch" role="group" aria-label="Instance list view">
              <button class="${viewMode === 'node' ? 'is-active' : ''}" id="instancesByNodeViewBtn" type="button">By node</button>
              <button class="${viewMode === 'table' ? 'is-active' : ''}" id="instancesTableViewBtn" type="button">Table</button>
            </div>
            <button class="secondary-btn" id="applyInstanceFiltersBtn" type="button">Apply filters</button>
            <button class="secondary-btn" id="resetInstanceFiltersBtn" type="button">Reset</button>
            <span class="tag">${escapeHTML(String(filteredRows.length))} shown</span>
          </div>
        </div>`;
    }

    function renderInstanceIssue(row) {
      const tagClass = row.issue.status === 'ok'
        ? 'ok'
        : row.issue.status === 'failed' || row.issue.status === 'problem'
          ? 'danger'
          : row.issue.status === 'pending'
            ? 'warn'
            : 'stub';
      return `
        <div class="instance-issue-cell">
          <span class="tag ${tagClass}">${escapeHTML(row.issue.status || 'unknown')}</span>
          <span title="${escapeHTML(row.issue.text || '')}">${escapeHTML(row.issue.text || 'Waiting for runtime report.')}</span>
        </div>`;
    }

    function renderInstanceStateCluster(row) {
      if (hasActiveInstanceJob(row)) {
        const jobStatus = lowerStatus(row.latestJob?.status) || 'queued';
        const jobType = String(row.latestJob?.type || 'job').replace(/^node\.capability\.install$/, 'runtime install');
        return `
        <div class="instance-state-cluster">
          ${statusTag(activeInstanceLifecycle(row))}
          ${statusTag(jobStatus)}
          <span class="tag warn">${escapeHTML(jobType)}</span>
          <span class="tag">${escapeHTML(row.revision === 'applied' ? 'rev applied' : `rev ${row.revision}`)}</span>
        </div>`;
      }
      return `
        <div class="instance-state-cluster">
          ${statusTag(row.status)}
          ${statusTag(row.runtime)}
          ${statusTag(row.health)}
          <span class="tag">${escapeHTML(row.revision === 'applied' ? 'rev applied' : `rev ${row.revision}`)}</span>
        </div>`;
    }

    function renderInstanceTableRow(row) {
      return `
        <tr>
          <td>
            <strong class="mono-clip" title="${escapeHTML(row.name || '')}">${escapeHTML(row.name || 'instance')}</strong>
            <span class="metric-caption mono-clip" title="${escapeHTML(row.id || '')}">${escapeHTML(row.id || 'n/a')}</span>
          </td>
          <td>
            <strong>${escapeHTML(row.serviceLabel)}</strong>
            <span class="metric-caption mono-clip">${escapeHTML(row.service)}</span>
          </td>
          <td>
            <strong class="mono-clip" title="${escapeHTML(row.node)}">${escapeHTML(row.node)}</strong>
            ${row.nodeAddress ? `<span class="metric-caption mono-clip" title="${escapeHTML(row.nodeAddress)}">${escapeHTML(row.nodeAddress)}</span>` : ''}
          </td>
          <td><code class="mono-clip" title="${escapeHTML(row.endpoint)}">${escapeHTML(row.endpoint)}</code></td>
          <td>${renderInstanceStateCluster(row)}</td>
          <td>${renderInstanceIssue(row)}</td>
          <td>${actionButtons(row)}</td>
        </tr>`;
    }

    function renderNodeInstanceTableRow(row) {
      return `
        <tr>
          <td>
            <strong class="mono-clip" title="${escapeHTML(row.name || '')}">${escapeHTML(row.name || 'instance')}</strong>
            <span class="metric-caption mono-clip" title="${escapeHTML(row.id || '')}">${escapeHTML(row.id || 'n/a')}</span>
          </td>
          <td>
            <strong>${escapeHTML(row.serviceLabel)}</strong>
            <span class="metric-caption mono-clip">${escapeHTML(row.service)}</span>
          </td>
          <td><code class="mono-clip" title="${escapeHTML(row.endpoint)}">${escapeHTML(row.endpoint)}</code></td>
          <td>${renderInstanceStateCluster(row)}</td>
          <td>${renderInstanceIssue(row)}</td>
          <td>${actionButtons(row)}</td>
        </tr>`;
    }

    function renderInstanceFact(label, value, className = '') {
      return `
        <div class="instance-fact ${className}">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(String(value ?? '').trim() || 'n/a')}</strong>
        </div>`;
    }

    function actionButtons(row) {
      const busy = hasActiveInstanceJob(row);
      const busyAttr = busy ? ' disabled title="Current instance job is still queued or running"' : '';
      const runtimeBlocked = row.capabilityMissing;
      const runtimeBlockedAttr = runtimeBlocked ? ' disabled title="Install the runtime capability before applying or controlling this instance"' : '';
      return `
        <div class="instance-actions">
          <button class="secondary-btn instance-manage-btn" type="button" data-instance-id="${escapeHTML(row.id)}">Manage</button>
          ${row.capabilityMissing ? `<button class="primary-btn instance-runtime-install-btn" type="button" data-instance-id="${escapeHTML(row.id)}" data-issue="${escapeHTML(row.issue?.text || row.latestJobSummary || '')}">Install runtime</button>` : ''}
          <button class="${row.capabilityMissing ? 'secondary-btn' : 'primary-btn'} instance-action-btn" type="button" data-action="apply" data-instance-id="${escapeHTML(row.id)}"${busyAttr || runtimeBlockedAttr}>Apply</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="restart" data-instance-id="${escapeHTML(row.id)}"${busyAttr || runtimeBlockedAttr}>Restart</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="start" data-instance-id="${escapeHTML(row.id)}"${busyAttr || runtimeBlockedAttr}>Start</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="stop" data-instance-id="${escapeHTML(row.id)}"${busyAttr}>Stop</button>
          <button class="danger-btn instance-delete-btn" type="button" data-instance-id="${escapeHTML(row.id)}" data-instance-name="${escapeHTML(row.name || 'instance')}">Delete</button>
        </div>`;
    }

    function groupRowsByNode(rows) {
      const groups = new Map();
      for (const row of rows) {
        const key = String(row.nodeID || row.node || 'unknown').trim() || 'unknown';
        if (!groups.has(key)) {
          groups.set(key, {
            key,
            nodeID: row.nodeID || '',
            node: row.node || 'n/a',
            role: row.nodeRole || '',
            status: row.nodeStatus || '',
            address: row.nodeAddress || '',
            rows: [],
          });
        }
        groups.get(key).rows.push(row);
      }
      return Array.from(groups.values()).sort((left, right) => left.node.localeCompare(right.node, 'en'));
    }

    function groupBucketCount(group, bucket) {
      return group.rows.filter((row) => row.bucket === bucket).length;
    }

    function renderNodeInstanceGroup(group) {
      const problem = groupBucketCount(group, 'problem');
      const pending = groupBucketCount(group, 'pending');
      const healthy = groupBucketCount(group, 'healthy');
      const unknown = group.rows.length - problem - pending - healthy;
      return `
        <section class="node-instance-group">
          <div class="node-instance-group-head">
            <div class="node-instance-title">
              <div class="instance-panel-label">Node workload</div>
              <h3>${escapeHTML(group.node || 'node')}</h3>
              <p>${escapeHTML([group.role, group.address].filter(Boolean).join(' · ') || 'node metadata unavailable')}</p>
            </div>
            <div class="node-instance-summary">
              ${group.status ? statusTag(group.status) : ''}
              <span class="tag">${escapeHTML(String(group.rows.length))} instances</span>
              ${problem ? `<span class="tag danger">${escapeHTML(String(problem))} problem</span>` : ''}
              ${pending ? `<span class="tag warn">${escapeHTML(String(pending))} pending</span>` : ''}
              ${healthy ? `<span class="tag ok">${escapeHTML(String(healthy))} healthy</span>` : ''}
              ${unknown ? `<span class="tag">${escapeHTML(String(unknown))} unknown</span>` : ''}
              ${group.nodeID ? `<button class="secondary-btn node-instance-open-btn" type="button" data-node-id="${escapeHTML(group.nodeID)}">Open node</button>` : ''}
            </div>
          </div>
          <div class="table-wrap node-instance-table-wrap">
            <table class="instances-table node-instance-table">
              <thead>
                <tr>
                  <th>Instance</th>
                  <th>Service</th>
                  <th>Endpoint</th>
                  <th>State</th>
                  <th>Issue</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>${group.rows.map(renderNodeInstanceTableRow).join('')}</tbody>
            </table>
          </div>
        </section>`;
    }

    function renderInstancesByNode(rows) {
      if (!rows.length) return '<div class="empty instance-filter-empty">No instances match the selected filters.</div>';
      return `<div class="node-instance-groups">${groupRowsByNode(rows).map(renderNodeInstanceGroup).join('')}</div>`;
    }

    function renderInstancesTable(rows) {
      return `
        <div class="table-wrap instances-table-wrap">
          <table class="instances-table">
            <thead>
              <tr>
                <th>Instance</th>
                <th>Service</th>
                <th>Node</th>
                <th>Endpoint</th>
                <th>State</th>
                <th>Issue</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              ${rows.length ? rows.map(renderInstanceTableRow).join('') : '<tr><td colspan="7"><div class="empty">No instances match the selected filters.</div></td></tr>'}
            </tbody>
          </table>
        </div>`;
    }

    function renderInstancesList(instances) {
      if (!instances.length) return renderEmptyInstancesState();
      const rows = instances.map(toInstanceRow);
      const filters = ensureInstanceListFilters();
      const filteredRows = filterInstanceRows(rows, filters);
      const limit = Number(state.instancesVisibleLimit || INSTANCE_PAGE_SIZE);
      const visibleRows = filteredRows.slice(0, limit);
      const hiddenCount = Math.max(0, filteredRows.length - visibleRows.length);
      return `
        ${renderInstanceListToolbar(rows, filteredRows)}
        ${instanceListViewMode() === 'node' ? renderInstancesByNode(visibleRows) : renderInstancesTable(visibleRows)}
        ${hiddenCount ? `
          <div class="instance-load-more">
            <button class="secondary-btn" id="showMoreInstancesBtn" type="button">Show next ${escapeHTML(String(Math.min(INSTANCE_PAGE_SIZE, hiddenCount)))} of ${escapeHTML(String(hiddenCount))}</button>
          </div>` : ''}`;
    }

    function renderEmptyInstancesState() {
      const nodes = activeNodes();
      const services = instanceCapableServices();
      const runtimeReports = Array.isArray(state.instanceRuntimeStates) ? state.instanceRuntimeStates.length : 0;
      return `
        <div class="instances-empty-state">
          <div class="instances-empty-grid">
            <div class="instance-empty-panel">
              <span>Nodes</span>
              <strong>${escapeHTML(String(nodes.length))}</strong>
              <span>${escapeHTML(nodes.length ? 'available for managed instances' : 'no active nodes loaded')}</span>
            </div>
            <div class="instance-empty-panel">
              <span>Service profiles</span>
              <strong>${escapeHTML(String(services.length))}</strong>
              <span>${escapeHTML(services.length ? services.slice(0, 4).map((item) => serviceLabel(item.code)).join(', ') : 'no instance-capable services loaded')}</span>
            </div>
            <div class="instance-empty-panel">
              <span>Runtime reports</span>
              <strong>${escapeHTML(String(runtimeReports))}</strong>
              <span>${escapeHTML(runtimeReports ? 'agent observations available' : 'waiting for first instance report')}</span>
            </div>
          </div>
        </div>`;
    }

    function ensureCreatePackSelection(packs) {
      const items = Array.isArray(packs) ? packs : servicePackCatalogItems();
      if (!items.length) {
        state.instancesCreatePackKey = '';
        return null;
      }
      const selected = items.find((pack) => pack.key === state.instancesCreatePackKey);
      if (selected) return selected;
      state.instancesCreatePackKey = items[0].key;
      return items[0];
    }

    function servicePackComponents(pack) {
      return Array.isArray(pack?.components) ? pack.components : [];
    }

    function packUsesService(pack, serviceCode) {
      const expected = String(serviceCode || '').trim().toLowerCase();
      return servicePackComponents(pack).some((component) => String(component?.service_code || '').trim().toLowerCase() === expected);
    }

    function endpointRequirementLabel(pack) {
      if (!pack) return 'n/a';
      return pack.requires_endpoint_host ? 'required' : 'optional';
    }

    function renderCreatePackCard(pack, selectedKey) {
      const key = String(pack.key || '').trim();
      const selected = key === selectedKey;
      return `
        <button class="pack-choice-btn ${selected ? 'is-selected' : ''}" type="button" data-pack-key="${escapeHTML(key)}">
          <strong>${escapeHTML(pack.label || key)}</strong>
          <span>${escapeHTML(servicePackComponentsLabel(pack))}</span>
          <small>${escapeHTML(pack.endpoint_hint || 'no endpoint hint')}</small>
        </button>`;
    }

    function renderPackSummary(pack) {
      if (!pack) return '';
      const components = servicePackComponents(pack);
      return `
        <div class="pack-create-summary">
          ${renderInstanceFact('Base name', pack.base_name_template || 'edge-service-pack')}
          ${renderInstanceFact('Endpoint host', endpointRequirementLabel(pack))}
          ${renderInstanceFact('Components', components.length)}
        </div>`;
    }

    function renderPackComponent(component, index) {
      const port = Number(component?.endpoint_port || 0);
      const meta = [
        component?.service_code || 'service',
        component?.preset_key || 'default',
        port ? `port ${port}` : '',
      ].filter(Boolean).join(' · ');
      return `
        <article class="pack-component-card">
          <div class="pack-component-index">${escapeHTML(String(index + 1))}</div>
          <div>
            <strong>${escapeHTML(component?.label || component?.service_code || `Component ${index + 1}`)}</strong>
            <span>${escapeHTML(meta)}</span>
            <p>${escapeHTML(component?.description || 'Instance component generated from the selected pack template.')}</p>
          </div>
        </article>`;
    }

    function renderPackDetails(pack) {
      if (!pack) {
        return '<div class="empty pack-create-empty">No service pack templates loaded.</div>';
      }
      const components = servicePackComponents(pack);
      const recommendations = Array.isArray(pack.recommendations) ? pack.recommendations.filter(Boolean) : [];
      const notes = Array.isArray(pack.platform_notes) ? pack.platform_notes.filter(Boolean) : [];
      return `
        <section class="pack-details-panel">
          <div class="pack-details-head">
            <div>
              <div class="instance-panel-label">Selected pack</div>
              <h3>${escapeHTML(pack.label || pack.key)}</h3>
              <p>${escapeHTML(pack.description || 'Template for creating a managed service instance set.')}</p>
            </div>
            <span class="tag">${escapeHTML(String(components.length))} components</span>
          </div>
          ${renderPackSummary(pack)}
          <div class="pack-component-grid">
            ${components.length ? components.map(renderPackComponent).join('') : '<div class="empty">No components in this pack.</div>'}
          </div>
          ${recommendations.length || notes.length ? `
            <div class="pack-notes">
              ${recommendations.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}
              ${notes.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}
            </div>` : ''}
        </section>`;
    }

    function renderCreatePackResult() {
      const result = state.instancesCreateResult;
      if (!result) return '';
      if (result.status === 'running') {
        return '<div class="form-result"><span class="tag warn">creating</span></div>';
      }
      if (result.status === 'succeeded') {
        const installJobs = Array.isArray(result.data?.runtime_install_jobs) ? result.data.runtime_install_jobs : [];
        const applyJobs = Array.isArray(result.data?.apply_jobs) ? result.data.apply_jobs : [];
        const createdInstances = Array.isArray(result.data?.created_instances) ? result.data.created_instances : [];
        return `
          <div class="form-result pack-create-result">
            ${renderActionResponse(result.data, 'Service pack creation')}
            <div class="pack-create-job-summary">
              <span class="tag ${installJobs.length ? 'warn' : 'ok'}">${escapeHTML(String(installJobs.length))} runtime install jobs</span>
              <span class="tag ${applyJobs.length ? 'warn' : 'ok'}">${escapeHTML(String(applyJobs.length))} apply jobs</span>
              <span class="tag">${escapeHTML(String(createdInstances.length))} instances</span>
            </div>
            <div class="inline-actions">
              <button class="secondary-btn" id="openInstancesAfterPackCreateBtn" type="button">Open instances</button>
              <button class="secondary-btn" id="createAnotherFromPackBtn" type="button">Create another</button>
            </div>
          </div>`;
      }
      const details = result.payload && typeof result.payload === 'object' ? result.payload : {};
      const facts = [
        details.component ? renderInstanceFact('Component', details.component) : '',
        details.discarded_count !== undefined ? renderInstanceFact('Discarded drafts', details.discarded_count) : '',
        Array.isArray(details.created_instances) ? renderInstanceFact('Created before failure', details.created_instances.length) : '',
      ].filter(Boolean).join('');
      return `
        <div class="form-result pack-create-result">
          <span class="tag danger">${escapeHTML(result.message || 'create failed')}</span>
          ${facts ? `<div class="response-grid">${facts}</div>` : ''}
          ${Object.keys(details).length ? renderActionResponse(details, 'Service pack create failed') : ''}
        </div>`;
    }

    function openCreateFromPackPage() {
      state.instancesView = 'create-pack';
      state.instancesCreateResult = null;
      ensureCreatePackSelection(servicePackCatalogItems());
      render();
    }

    function openInstancesListPage() {
      state.instancesView = 'list';
      state.instancesCreateResult = null;
      render();
    }

    function renderCreateFromPackPage() {
      setTitle('Create from pack');
      const packs = servicePackCatalogItems();
      const selectedPack = ensureCreatePackSelection(packs);
      const nodeSelect = nodeOptions();
      const usesOpenVPN = packUsesService(selectedPack, 'openvpn');
      const usesTLSEdgeCertificate = servicePackUsesTLSEdgeCertificate(selectedPack);
      const certificateSelect = certificateOptions(defaultLeafCertificateID(), true);
      const baseName = selectedPack?.base_name_template || 'edge-service-pack';
      const endpointHint = selectedPack?.endpoint_hint || 'edge.example.com';
      const endpointRequired = Boolean(selectedPack?.requires_endpoint_host);
      el('content').innerHTML = `
        <section class="table-card instance-create-page">
          <div class="table-head instance-create-head">
            <div>
              <h2>Create from pack</h2>
              <div class="metric-caption">Instances are created from a reusable service pack template.</div>
            </div>
            <div class="table-tools">
              <button class="secondary-btn" id="backToInstancesBtn" type="button">Back to instances</button>
              <button class="secondary-btn" id="createInstanceBtn" type="button">Manual instance</button>
            </div>
          </div>
          <div class="pack-create-layout">
            <aside class="pack-picker-panel">
              <div class="instance-panel-label">Service pack</div>
              <div class="pack-choice-list">
                ${packs.length ? packs.map((pack) => renderCreatePackCard(pack, state.instancesCreatePackKey)).join('') : '<div class="empty">No templates loaded.</div>'}
              </div>
            </aside>
            <div class="pack-create-main">
              ${renderPackDetails(selectedPack)}
              <form id="createServicePackPageForm" class="pack-create-form">
                <input type="hidden" name="service_pack_key" value="${escapeHTML(selectedPack?.key || '')}">
                <div class="pack-create-form-grid stable">
                  <div class="field">
                    <label>Node</label>
                    <select name="node_id" required${nodeSelect ? '' : ' disabled'}>
                      ${nodeSelect || '<option value="">No active nodes available</option>'}
                    </select>
                  </div>
                  <div class="field">
                    <label>Base name</label>
                    <input name="base_name" value="${escapeHTML(baseName)}" placeholder="${escapeHTML(baseName)}">
                  </div>
                  <div class="field full">
                    <label>Endpoint host</label>
                    <input name="endpoint_host" placeholder="${escapeHTML(endpointHint)}"${endpointRequired ? ' required' : ''}>
                  </div>
                  ${usesTLSEdgeCertificate ? `
                    <div class="field full">
                      <label>TLS edge certificate</label>
                      <select name="certificate_id">${certificateSelect}</select>
                      <div class="field-hint">Optional override for TLS-facing Nginx or Xray TLS components. The platform default certificate is selected automatically.</div>
                    </div>` : ''}
                  ${usesOpenVPN ? `
                    <div class="field full">
                      <label>OpenVPN CA profile</label>
                      <select name="openvpn_pki_profile">${servicePKIProfileOptions('openvpn', 'default')}</select>
                      <div class="field-hint">OpenVPN instances created with this profile trust the same service CA root, which is required for a shared endpoint fleet.</div>
                    </div>` : ''}
                </div>
                <div class="pack-create-actions">
                  <button class="primary-btn" type="submit"${selectedPack && nodeSelect ? '' : ' disabled'}>Create instances</button>
                  <button class="secondary-btn" id="resetPackCreateFormBtn" type="button">Reset form</button>
                </div>
              </form>
              <div id="createServicePackPageResult">${renderCreatePackResult()}</div>
            </div>
          </div>
        </section>`;
      bindCreateFromPackPage();
    }

    function bindCreateFromPackPage() {
      document.getElementById('backToInstancesBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createInstanceBtn')?.addEventListener('click', openCreateInstanceModal);
      document.getElementById('resetPackCreateFormBtn')?.addEventListener('click', () => {
        state.instancesCreateResult = null;
        renderCreateFromPackPage();
      });
      document.querySelectorAll('.pack-choice-btn').forEach((button) => {
        button.addEventListener('click', () => {
          state.instancesCreatePackKey = button.dataset.packKey || '';
          state.instancesCreateResult = null;
          renderCreateFromPackPage();
        });
      });
      document.getElementById('createServicePackPageForm')?.addEventListener('submit', submitCreateFromPackPage);
      document.getElementById('openInstancesAfterPackCreateBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createAnotherFromPackBtn')?.addEventListener('click', () => {
        state.instancesCreateResult = null;
        renderCreateFromPackPage();
      });
    }

    async function submitCreateFromPackPage(event) {
      event.preventDefault();
      const form = event.currentTarget;
      const button = form.querySelector('button[type="submit"]');
      const data = new FormData(form);
      const packKey = String(data.get('service_pack_key') || state.instancesCreatePackKey || '').trim();
      const payload = {
        node_id: String(data.get('node_id') || '').trim(),
        base_name: String(data.get('base_name') || '').trim(),
        endpoint_host: String(data.get('endpoint_host') || '').trim(),
        certificate_id: String(data.get('certificate_id') || '').trim(),
        openvpn_pki_profile: String(data.get('openvpn_pki_profile') || '').trim(),
      };
      if (!packKey) {
        state.instancesCreateResult = { status: 'failed', message: 'service pack is required' };
        renderCreateFromPackPage();
        return;
      }
      if (button) {
        button.disabled = true;
        button.textContent = 'Creating...';
      }
      state.instancesCreateResult = { status: 'running' };
      const result = document.getElementById('createServicePackPageResult');
      if (result) result.innerHTML = renderCreatePackResult();
      try {
        const created = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}/instances`, 'POST', payload);
        state.instancesCreateResult = { status: 'succeeded', data: created };
        await refresh();
        renderCreateFromPackPage();
      } catch (err) {
        state.instancesCreateResult = { status: 'failed', message: err.message || 'service pack create failed', payload: err.payload || null };
        renderCreateFromPackPage();
      }
    }

    function bindActions() {
      document.getElementById('createInstanceBtn')?.addEventListener('click', openCreateInstanceModal);
      document.getElementById('createServicePackBtn')?.addEventListener('click', openCreateFromPackPage);
      bindInstanceListControls();
      document.getElementById('addServicePackTemplateBtn')?.addEventListener('click', () => openServicePackEditor(''));
      document.querySelectorAll('.service-pack-edit-btn').forEach((button) => {
        button.addEventListener('click', () => openServicePackEditor(button.dataset.packKey));
      });
      document.querySelectorAll('.service-pack-delete-btn').forEach((button) => {
        button.addEventListener('click', () => deleteServicePackTemplate(button.dataset.packKey));
      });
      document.querySelectorAll('.instance-manage-btn').forEach((button) => {
        button.addEventListener('click', () => openInstanceManageModal(button.dataset.instanceId));
      });
      document.querySelectorAll('.instance-action-btn').forEach((button) => {
        button.addEventListener('click', () => queueInstanceAction(button.dataset.instanceId, button.dataset.action));
      });
      document.querySelectorAll('.instance-runtime-install-btn').forEach((button) => {
        button.addEventListener('click', () => openInstanceRuntimeInstallModal(button.dataset.instanceId, button.dataset.issue || ''));
      });
      document.querySelectorAll('.instance-delete-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteInstanceModal(button.dataset.instanceId, button.dataset.instanceName));
      });
      document.querySelectorAll('.node-instance-open-btn').forEach((button) => {
        button.addEventListener('click', () => openNodeFromInstanceList(button.dataset.nodeId));
      });
    }

    function openNodeFromInstanceList(nodeID) {
      if (!nodeID) return;
      if (!state.nodeManageActiveTabs || typeof state.nodeManageActiveTabs !== 'object') {
        state.nodeManageActiveTabs = {};
      }
      state.nodeManageActiveTabs[nodeID] = 'instances';
      state.nodeManageID = nodeID;
      state.nodeManageData = null;
      setPage('nodeManage');
    }

    function bindInstanceListControls() {
      const filters = ensureInstanceListFilters();
      const searchInput = document.getElementById('instanceSearchInput');
      const statusFilter = document.getElementById('instanceStatusFilter');
      const serviceFilter = document.getElementById('instanceServiceFilter');
      const nodeFilter = document.getElementById('instanceNodeFilter');
      const apply = () => {
        filters.search = String(searchInput?.value || '').trim();
        filters.status = String(statusFilter?.value || 'all');
        filters.service = String(serviceFilter?.value || 'all');
        filters.node = String(nodeFilter?.value || 'all');
        state.instancesVisibleLimit = INSTANCE_PAGE_SIZE;
        render();
      };
      searchInput?.addEventListener('input', () => {
        filters.search = String(searchInput.value || '');
      });
      searchInput?.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') apply();
      });
      statusFilter?.addEventListener('change', apply);
      serviceFilter?.addEventListener('change', apply);
      nodeFilter?.addEventListener('change', apply);
      document.getElementById('applyInstanceFiltersBtn')?.addEventListener('click', apply);
      document.getElementById('resetInstanceFiltersBtn')?.addEventListener('click', () => {
        state.instanceListFilters = { search: '', status: 'all', service: 'all', node: 'all' };
        state.instancesVisibleLimit = INSTANCE_PAGE_SIZE;
        render();
      });
      document.getElementById('instancesByNodeViewBtn')?.addEventListener('click', () => {
        state.instancesListView = 'node';
        state.instancesVisibleLimit = INSTANCE_PAGE_SIZE;
        render();
      });
      document.getElementById('instancesTableViewBtn')?.addEventListener('click', () => {
        state.instancesListView = 'table';
        state.instancesVisibleLimit = INSTANCE_PAGE_SIZE;
        render();
      });
      document.getElementById('showMoreInstancesBtn')?.addEventListener('click', () => {
        state.instancesVisibleLimit = Number(state.instancesVisibleLimit || INSTANCE_PAGE_SIZE) + INSTANCE_PAGE_SIZE;
        render();
      });
    }

    function render() {
      if (state.instancesView === 'create-pack') {
        renderCreateFromPackPage();
        return;
      }
      state.instancesView = 'list';
      setTitle('Instances');
      const instances = Array.isArray(state.instances) ? state.instances : [];
      const runtimeReports = Array.isArray(state.instanceRuntimeStates) ? state.instanceRuntimeStates.length : 0;
      el('content').innerHTML = `
        <section class="table-card instances-overview">
          <div class="table-head">
            <h2>Instances</h2>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(instances.length))} active</span>
              <span class="tag">${escapeHTML(String(runtimeReports))} runtime reports</span>
              <button class="secondary-btn" id="createServicePackBtn" type="button">Create from pack</button>
              <button class="secondary-btn" id="createInstanceBtn" type="button">Manual instance</button>
            </div>
          </div>
          <div class="instances-list">${renderInstancesList(instances)}</div>
        </section>
        ${renderServicePackCatalog()}`;
      bindActions();
    }

    function renderServicePackCatalog() {
      const packs = servicePackCatalogItems();
      const manage = canManageServicePacks();
      const rows = packs.length
        ? packs.map((pack) => `
          <tr class="service-pack-row">
            <td>
              <strong>${escapeHTML(pack.label || pack.key)}</strong>
              <div class="metric-caption"><code>${escapeHTML(pack.key)}</code></div>
              <div class="metric-caption">${escapeHTML(pack.description || 'template')}</div>
            </td>
            <td><span class="template-components">${escapeHTML(servicePackComponentsLabel(pack))}</span></td>
            <td><code class="mono-clip">${escapeHTML(pack.endpoint_hint || 'n/a')}</code></td>
            <td>
              <div class="table-actions service-pack-actions">
                ${manage ? `<button class="secondary-btn service-pack-edit-btn" type="button" data-pack-key="${escapeHTML(pack.key)}">Edit</button>` : ''}
                ${manage ? `<button class="danger-btn service-pack-delete-btn" type="button" data-pack-key="${escapeHTML(pack.key)}">Delete</button>` : ''}
              </div>
            </td>
          </tr>`).join('')
        : `<tr><td colspan="4"><div class="empty">No service pack templates loaded.</div></td></tr>`;
      return `
        <section class="table-card">
          <div class="table-head">
            <h2>Service pack catalog</h2>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(packs.length))} templates</span>
              ${manage ? '<button class="secondary-btn" id="addServicePackTemplateBtn" type="button">Add pack</button>' : '<span class="tag">settings.manage required</span>'}
            </div>
          </div>
          <div class="table-wrap">
            <table class="service-pack-table">
              <thead><tr><th>Pack</th><th>Components</th><th>Endpoint hint</th><th>Actions</th></tr></thead>
              <tbody>${rows}</tbody>
            </table>
          </div>
        </section>`;
    }

    function servicePackEditorModel(packKey) {
      const existing = servicePackCatalogItems().find((pack) => pack.key === packKey);
      if (existing) {
        const model = JSON.parse(JSON.stringify(existing));
        delete model.status;
        delete model.source;
        delete model.version;
        return model;
      }
      return {
        key: 'custom_wireguard_pack',
        label: 'Custom WireGuard Pack',
        description: 'Operator-managed service pack template.',
        base_name_template: 'edge-wireguard',
        endpoint_hint: 'wg.example.com',
        requires_endpoint_host: true,
        platform_notes: [],
        recommendations: ['Address pool can be allocated automatically from the Address Pools catalog.'],
        components: [
          {
            label: 'WireGuard Road Warrior',
            description: 'Standalone WireGuard instance with managed peers.',
            service_code: 'wireguard',
            preset_key: 'roadwarrior',
            name_suffix: 'wireguard',
            slug_suffix: 'wireguard',
            endpoint_port: 51820,
            requires_endpoint_host: true,
            spec: {
              service_profile: 'roadwarrior',
              address_pool_mode: 'auto',
              client_allowed_ips: '0.0.0.0/0, ::/0',
              client_dns: '1.1.1.1, 1.0.0.1',
              persistent_keepalive: 25,
              config_mode: '0600',
            },
          },
        ],
        display_order: 500,
      };
    }

    function openServicePackEditor(packKey) {
      if (!canManageServicePacks()) {
        openActionOutcomeModal('Service pack catalog', 'settings.manage required', 'failed', 'Your role cannot manage service pack templates.', []);
        return;
      }
      const model = servicePackEditorModel(packKey);
      openModal(packKey ? `Edit pack: ${model.label || packKey}` : 'Add service pack', 'Service pack catalog template', `
        <form id="servicePackEditorForm" class="form-grid">
          <div class="field full">
            <label>Template JSON</label>
            <textarea class="code-textarea" name="template_json" rows="24" spellcheck="false">${escapeHTML(JSON.stringify(model, null, 2))}</textarea>
          </div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save pack</button>
            <button class="secondary-btn" id="cancelServicePackEditorBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="servicePackEditorResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelServicePackEditorBtn')?.addEventListener('click', closeModal);
      document.getElementById('servicePackEditorForm')?.addEventListener('submit', submitServicePackEditor);
    }

    async function submitServicePackEditor(event) {
      event.preventDefault();
      const result = document.getElementById('servicePackEditorResult');
      const textarea = event.currentTarget.querySelector('textarea[name="template_json"]');
      let payload = null;
      try {
        payload = JSON.parse(String(textarea?.value || '').trim());
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">invalid JSON: ${escapeHTML(err.message)}</span>`;
        return;
      }
      const key = String(payload?.key || '').trim();
      if (!key) {
        if (result) result.innerHTML = '<span class="tag danger">key is required</span>';
        return;
      }
      if (result) result.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const saved = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(key)}`, 'PUT', payload);
        if (textarea) textarea.value = JSON.stringify(saved, null, 2);
        if (result) result.innerHTML = `<span class="tag ok">saved</span> <code>${escapeHTML(saved.key || key)}</code>`;
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function deleteServicePackTemplate(packKey) {
      if (!packKey || !canManageServicePacks()) return;
      if (!window.confirm(`Delete service pack template ${packKey}? Existing instances are not changed.`)) return;
      try {
        await sendJSON(`/api/v1/service-packs/${encodeURIComponent(packKey)}`, 'DELETE', null);
        await refresh();
      } catch (err) {
        openActionOutcomeModal('Service pack catalog', 'Template delete failed', 'failed', err.message || 'Service pack template delete failed.', [
          { label: 'Pack', value: packKey },
        ]);
      }
    }

    function openDeleteInstanceModal(instanceID, instanceName) {
      const instance = (state.instances || []).find((item) => item.id === instanceID);
      openModal(`Delete instance: ${instanceName || 'instance'}`, 'Instance lifecycle', `
        <div class="card">
          <div class="mini-label">Managed cleanup</div>
          <h3>${escapeHTML(instance?.name || instanceName || instanceID)}</h3>
          <p>This queues an agent cleanup job on the selected node. The agent stops the managed unit, removes managed files and then the control plane hides the instance after cleanup succeeds. Deletion is blocked while active service accesses still reference this instance.</p>
          <div class="response-grid">
            <div class="response-fact"><span>Service</span><strong>${escapeHTML(instance?.service_code || 'unknown')}</strong></div>
            <div class="response-fact"><span>Node</span><strong>${escapeHTML(instance?.node_id || 'n/a')}</strong></div>
            <div class="response-fact"><span>Status</span><strong>${escapeHTML(instance?.status || 'unknown')}</strong></div>
            <div class="response-fact"><span>Endpoint</span><strong>${escapeHTML(instanceEndpoint(instance))}</strong></div>
          </div>
        </div>
        <div class="inline-actions" style="margin-top:14px">
          <button class="secondary-btn" id="cancelDeleteInstanceBtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmDeleteInstanceBtn" type="button">Queue cleanup</button>
        </div>`, { wide: true });
      document.getElementById('cancelDeleteInstanceBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmDeleteInstanceBtn')?.addEventListener('click', (event) => submitDeleteInstance(instanceID, instanceName, event.currentTarget));
    }

    async function submitDeleteInstance(instanceID, instanceName, button) {
      if (button) {
        button.disabled = true;
        button.textContent = 'Deleting...';
      }
      try {
        const deleted = await sendJSON(`/api/v1/instances/${instanceID}`, 'DELETE', null);
        closeModal();
        await refresh();
        openActionOutcomeModal(
          `Instance cleanup queued: ${instanceName || deleted?.name || instanceID}`,
          'Instance lifecycle',
          'succeeded',
          'Cleanup job was queued for the node. The instance remains visible as deleting until the agent confirms cleanup.',
          [
            { label: 'Instance', value: deleted?.name || instanceName || instanceID },
            { label: 'Status', value: deleted?.status || 'deleting' },
          ],
        );
      } catch (err) {
        openActionOutcomeModal(
          'Instance delete blocked',
          'Instance lifecycle',
          'failed',
          err.message || 'Instance delete failed.',
          [
            { label: 'Instance', value: instanceName || instanceID },
            { label: 'Expected fix', value: 'Revoke or delete dependent service accesses first, then retry.' },
          ],
        );
      } finally {
        if (button) {
          button.disabled = false;
          button.textContent = 'Queue cleanup';
        }
      }
    }

    return {
      render,
      toRow: toInstanceRow,
    };
  }

  window.MegaVPNInstancesPage = { create: createInstancesPage };
})(window);
