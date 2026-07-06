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
      renderCreateInstanceForm,
      openInstanceManageModal,
      openInstanceManagePage,
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
      typeof renderCreateInstanceForm !== 'function' ||
      typeof openInstanceManageModal !== 'function' ||
      typeof openInstanceManagePage !== 'function' ||
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
    const INSTANCE_TABS = [
      ['list', 'Instances', 'runtime state'],
      ['create-pack', 'Create from pack', 'template rollout'],
      ['manual', 'Manual instance', 'single service'],
      ['service-packs', 'Service pack catalog', 'pack templates'],
      ['vless-groups', 'VLESS groups', 'client routing'],
    ];

    function normalizedInstancesView() {
      const view = String(state.instancesView || 'list');
      return INSTANCE_TABS.some(([key]) => key === view) ? view : 'list';
    }

    function setInstancesView(view) {
      state.instancesView = INSTANCE_TABS.some(([key]) => key === view) ? view : 'list';
      if (state.instancesView !== 'create-pack') {
        state.instancesCreateResult = null;
        state.instancesCreatePackDraft = null;
      }
      render();
    }

    function renderInstancesTabs(activeView = normalizedInstancesView()) {
      return `
        <nav class="page-tabs instances-tabs" aria-label="Instances workspace">
          ${INSTANCE_TABS.map(([key, label, hint]) => `
            <button class="page-tab instances-tab-btn ${key === activeView ? 'is-active' : ''}" type="button" data-instances-view="${escapeHTML(key)}">
              <span>${escapeHTML(label)}</span>
              <small>${escapeHTML(hint)}</small>
            </button>`).join('')}
        </nav>`;
    }

    function bindInstancesTabs() {
      document.querySelectorAll('.instances-tab-btn').forEach((button) => {
        button.addEventListener('click', () => setInstancesView(button.dataset.instancesView || 'list'));
      });
    }

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

    function servicePackChoiceRank(pack) {
      const status = String(pack?.status || 'active').toLowerCase();
      const statusRank = status === 'active' ? 0 : status === 'disabled' ? 1 : 2;
      const sourceRank = String(pack?.source || 'default').toLowerCase() === 'default' ? 1 : 0;
      return { statusRank, sourceRank, version: Number(pack?.version || 0), order: Number(pack?.display_order || 1000) };
    }

    function preferServicePackChoice(next, current) {
      if (!current) return true;
      const left = servicePackChoiceRank(next);
      const right = servicePackChoiceRank(current);
      if (left.statusRank !== right.statusRank) return left.statusRank < right.statusRank;
      if (left.sourceRank !== right.sourceRank) return left.sourceRank < right.sourceRank;
      if (left.version !== right.version) return left.version > right.version;
      if (left.order !== right.order) return left.order < right.order;
      return String(next?.key || '') < String(current?.key || '');
    }

    function servicePackChoiceFingerprint(pack) {
      const components = Array.isArray(pack?.components) ? pack.components : [];
      if (!String(pack?.label || '').trim() || !components.length) return '';
      const clean = (value) => String(value || '').trim().toLowerCase().replace(/\s+/g, ' ');
      const parts = [
        clean(pack.label),
        clean(pack.base_name_template),
        clean(pack.endpoint_hint),
        String(Boolean(pack.requires_endpoint_host)),
      ];
      components.forEach((component) => {
        const spec = component?.spec && typeof component.spec === 'object' ? component.spec : {};
        parts.push(
          clean(component?.service_code),
          clean(component?.preset_key),
          clean(component?.name_suffix),
          clean(component?.slug_suffix),
          String(Number(component?.endpoint_port || 0)),
          String(Boolean(component?.requires_endpoint_host)),
          clean(spec.service_profile),
        );
      });
      return parts.join('\u001f');
    }

    function dedupeServicePackChoices(items) {
      const out = [];
      const byKey = new Map();
      const byFingerprint = new Map();
      (Array.isArray(items) ? items : []).forEach((pack) => {
        if (!pack || !pack.key) return;
        const key = String(pack.key).trim();
        const fingerprint = servicePackChoiceFingerprint(pack);
        const existingIndex = byKey.has(key) ? byKey.get(key) : (fingerprint ? byFingerprint.get(fingerprint) : undefined);
        if (Number.isInteger(existingIndex)) {
          if (preferServicePackChoice(pack, out[existingIndex])) {
            const oldFingerprint = servicePackChoiceFingerprint(out[existingIndex]);
            byKey.delete(String(out[existingIndex]?.key || '').trim());
            if (oldFingerprint) byFingerprint.delete(oldFingerprint);
            out[existingIndex] = pack;
            byKey.set(key, existingIndex);
            if (fingerprint) byFingerprint.set(fingerprint, existingIndex);
          }
          return;
        }
        const index = out.length;
        out.push(pack);
        byKey.set(key, index);
        if (fingerprint) byFingerprint.set(fingerprint, index);
      });
      return out;
    }

    function servicePackCatalogItems() {
      const source = canManageServicePacks()
        ? (state.servicePackCatalog || state.servicePacks || [])
        : (state.servicePacks || []);
      return dedupeServicePackChoices(source)
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

    function technicalKeyFromLabel(value, fallback = 'item') {
      const raw = String(value || '').toLowerCase().trim();
      let out = '';
      let lastSeparator = false;
      for (const ch of raw) {
        const code = ch.charCodeAt(0);
        const alnum = (code >= 97 && code <= 122) || (code >= 48 && code <= 57);
        if (alnum) {
          out += ch;
          lastSeparator = false;
        } else if (!lastSeparator) {
          out += '_';
          lastSeparator = true;
        }
        if (out.length >= 64) break;
      }
      out = out.replace(/^_+|_+$/g, '');
      return out || fallback;
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
      const activeStateTransitioning = ['activating', 'deactivating', 'reloading'].includes(lowerStatus(row.activeState));
      if (hasActiveInstanceJob(row)) return 'pending';
      if (['provisioning', 'transitioning', 'activating', 'deactivating', 'reloading'].includes(runtime)
          || ['provisioning', 'transitioning'].includes(health)
          || (activeStateTransitioning && !(runtime === 'active' && health === 'healthy'))) {
        return 'pending';
      }
      if (row.capabilityRequired && !runtimeCapabilityReady(row) && runtime !== 'active') return 'problem';
      if (status === 'failed' || runtime === 'failed') {
        return 'problem';
      }
      if (revision && revision !== 'applied' && revision !== 'n/a') return 'pending';
      if (['pending_apply', 'pending', 'drifted', 'out_of_sync'].includes(drift)) return 'pending';
      if (['failed', 'degraded', 'unhealthy', 'error'].includes(health)) {
        return 'problem';
      }
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
        reasons.push(runtimeCapabilityIssueText(row));
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

    function runtimeCapabilityIssueText(row) {
      const status = row.capabilityStatus || 'missing';
      if (row.service === 'shadowsocks') {
        return `ss-server runtime is ${status}; Apply will install shadowsocks-libev automatically before the instance is applied.`;
      }
      if (row.service === 'openvpn') {
        return `OpenVPN runtime is ${status}; Apply will install the openvpn package automatically before the instance is applied.`;
      }
      if (row.service === 'xray-core') {
        return `Xray runtime is ${status}; Apply will run the approved runtime installer before the instance is applied.`;
      }
      return `Runtime capability ${row.service} is ${status} on this node.`;
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
      const runtimeControlBlockedAttr = runtimeBlocked ? ' disabled title="Runtime capability must be installed before controlling this instance"' : '';
      const applyAttr = busy ? busyAttr : (runtimeBlocked ? ' title="Queues runtime install first, then applies this instance after the install succeeds"' : '');
      const controlAttr = busy ? busyAttr : runtimeControlBlockedAttr;
      return `
        <div class="instance-actions">
          <button class="secondary-btn instance-manage-btn" type="button" data-instance-id="${escapeHTML(row.id)}">Manage</button>
          ${row.capabilityMissing ? `<button class="secondary-btn instance-runtime-install-btn" type="button" data-instance-id="${escapeHTML(row.id)}" data-issue="${escapeHTML(row.issue?.text || row.latestJobSummary || '')}">${escapeHTML(runtimeInstallActionLabel(row))}</button>` : ''}
          <button class="primary-btn instance-action-btn" type="button" data-action="apply" data-instance-id="${escapeHTML(row.id)}"${applyAttr}>${runtimeBlocked ? 'Install + apply' : 'Apply'}</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="restart" data-instance-id="${escapeHTML(row.id)}"${controlAttr}>Restart</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="start" data-instance-id="${escapeHTML(row.id)}"${controlAttr}>Start</button>
          <button class="secondary-btn instance-action-btn" type="button" data-action="stop" data-instance-id="${escapeHTML(row.id)}"${busyAttr}>Stop</button>
          <button class="danger-btn instance-delete-btn" type="button" data-instance-id="${escapeHTML(row.id)}" data-instance-name="${escapeHTML(row.name || 'instance')}">Delete</button>
        </div>`;
    }

    function runtimeInstallActionLabel(row) {
      return 'Runtime options';
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
      const expected = normalizeInstanceServiceCode(serviceCode);
      return servicePackComponents(pack).some((component) => normalizeInstanceServiceCode(component?.service_code) === expected);
    }

    function servicePackSpecProfile(component) {
      const spec = component?.spec && typeof component.spec === 'object' ? component.spec : {};
      return String(spec.service_profile || '').trim().toLowerCase();
    }

    function servicePackComponentUsesTrafficCamouflage(component) {
      const profile = servicePackSpecProfile(component);
      if (['ws_camouflage_edge', 'nginx_ws_backend', 'grpc_edge', 'nginx_grpc_backend'].includes(profile)) return true;
      const spec = component?.spec && typeof component.spec === 'object' ? component.spec : {};
      return JSON.stringify(spec).includes('{{camouflage_path}}') || JSON.stringify(spec).includes('{{fallback_upstream_url}}');
    }

    function packUsesTrafficCamouflage(pack) {
      return servicePackComponents(pack).some(servicePackComponentUsesTrafficCamouflage);
    }

    function packUsesConfigurableCamouflagePath(pack) {
      return servicePackComponents(pack).some((component) => {
        const spec = component?.spec && typeof component.spec === 'object' ? component.spec : {};
        return JSON.stringify(spec).includes('{{camouflage_path}}');
      });
    }

    function defaultCamouflagePath(pack) {
      for (const component of servicePackComponents(pack)) {
        const spec = component?.spec && typeof component.spec === 'object' ? component.spec : {};
        for (const key of ['location_path', 'public_path', 'path']) {
          const value = String(spec[key] || '').trim();
          if (value && value.startsWith('/') && value !== '/' && !value.includes('{{')) return value;
        }
      }
      return String(pack?.key || '').includes('grpc') ? '/vless-grpc' : '/assets/rtis-sync';
    }

    function normalizedHostForLoopGuard(value) {
      const raw = String(value || '').trim();
      if (!raw) return '';
      try {
        const parsed = new URL(raw.includes('://') ? raw : `https://${raw}`);
        return String(parsed.hostname || '').replace(/\.$/, '').toLowerCase();
      } catch (_) {
        return raw.replace(/^\[/, '').replace(/\]$/, '').replace(/\.$/, '').toLowerCase();
      }
    }

    function camouflageFallbackLoopError(payload) {
      const endpointHost = normalizedHostForLoopGuard(payload?.endpoint_host);
      if (!endpointHost) return '';
      const candidates = [
        ['Fallback website', payload?.fallback_upstream_url],
        ['Fallback Host header', payload?.fallback_host_header],
        ['Fallback SNI', payload?.fallback_sni],
      ];
      for (const [label, value] of candidates) {
        if (normalizedHostForLoopGuard(value) === endpointHost) {
          return `${label} must not point back to the public ingress endpoint. Use a separate fallback website.`;
        }
      }
      return '';
    }

    function servicePackCreateSucceeded() {
      return String(state.instancesCreateResult?.status || '') === 'succeeded';
    }

    function servicePackCreateRunning() {
      return String(state.instancesCreateResult?.status || '') === 'running';
    }

    function createPackDraft() {
      const draft = state.instancesCreatePackDraft;
      return draft && typeof draft === 'object' && !Array.isArray(draft) ? draft : {};
    }

    function createPackDraftComponentIndexes(draft) {
      if (!Array.isArray(draft?.component_indexes)) return null;
      return draft.component_indexes
        .map((value) => Number.parseInt(String(value), 10))
        .filter((value) => Number.isInteger(value) && value >= 0);
    }

    function createPackComponentSelected(draft, index) {
      const indexes = createPackDraftComponentIndexes(draft);
      return indexes === null || indexes.includes(index);
    }

    function createPackComponentDraft(draft, index) {
      const settings = draft?.component_settings && typeof draft.component_settings === 'object' && !Array.isArray(draft.component_settings)
        ? draft.component_settings
        : {};
      const entry = settings[String(index)];
      return entry && typeof entry === 'object' && !Array.isArray(entry) ? entry : {};
    }

    function packDraftString(draft, key, fallback = '') {
      if (!Object.prototype.hasOwnProperty.call(draft || {}, key)) return fallback;
      return String(draft[key] ?? '');
    }

    function createPackDraftFromForm(form) {
      const data = new FormData(form);
      const componentIndexes = data.getAll('pack_component_indexes')
        .map((value) => Number.parseInt(String(value), 10))
        .filter((value) => Number.isInteger(value) && value >= 0);
      const componentSettings = {};
      form.querySelectorAll('.pack-component-toggle').forEach((toggle) => {
        const index = Number.parseInt(String(toggle.dataset.packComponentIndex || toggle.value || ''), 10);
        if (!Number.isInteger(index) || index < 0) return;
        const endpointPort = form.elements[`component_${index}_endpoint_port`]?.value || '';
        const openvpnPKIProfile = form.elements[`component_${index}_openvpn_pki_profile`]?.value || '';
        componentSettings[String(index)] = {
          endpoint_port: String(endpointPort || '').trim(),
          openvpn_pki_profile: String(openvpnPKIProfile || '').trim(),
        };
      });
      return {
        service_pack_key: String(data.get('service_pack_key') || state.instancesCreatePackKey || '').trim(),
        node_id: String(data.get('node_id') || '').trim(),
        base_name: String(data.get('base_name') || '').trim(),
        endpoint_host: String(data.get('endpoint_host') || '').trim(),
        certificate_id: String(data.get('certificate_id') || '').trim(),
        openvpn_pki_profile: String(data.get('openvpn_pki_profile') || '').trim(),
        xray_egress_mode: String(data.get('xray_egress_mode') || '').trim(),
        xray_egress_node_id: String(data.get('xray_egress_node_id') || '').trim(),
        camouflage_path: String(data.get('camouflage_path') || '').trim(),
        fallback_upstream_url: String(data.get('fallback_upstream_url') || '').trim(),
        fallback_host_header: String(data.get('fallback_host_header') || '').trim(),
        fallback_sni: String(data.get('fallback_sni') || '').trim(),
        component_indexes: Array.from(new Set(componentIndexes)),
        component_settings: componentSettings,
      };
    }

    function servicePackCreateNodeLabel(draft) {
      const nodeID = String(draft?.node_id || '').trim();
      if (!nodeID) return '';
      const node = (state.nodes || []).find((item) => String(item.id || '') === nodeID);
      if (!node) return nodeID;
      const role = String(node.role || 'node').trim() || 'node';
      return `${node.name || nodeID} · ${role}`;
    }

    function renderCreatePackCompletionBanner(pack, draft) {
      const result = state.instancesCreateResult;
      if (String(result?.status || '') !== 'succeeded') return '';
      const installJobs = Array.isArray(result.data?.runtime_install_jobs) ? result.data.runtime_install_jobs : [];
      const applyJobs = Array.isArray(result.data?.apply_jobs) ? result.data.apply_jobs : [];
      const createdInstances = Array.isArray(result.data?.created_instances) ? result.data.created_instances : [];
      const nodeLabel = servicePackCreateNodeLabel(draft);
      const packLabel = pack?.label || draft?.service_pack_key || 'Service pack';
      return `
        <section class="pack-create-completion" id="packCreateCompletionBanner">
          <div class="pack-create-completion-copy">
            <div class="mini-label">Service pack created</div>
            <h3>${escapeHTML(packLabel)}</h3>
            <p>${escapeHTML(String(createdInstances.length))} instance${createdInstances.length === 1 ? '' : 's'} created${nodeLabel ? ` on ${escapeHTML(nodeLabel)}` : ''}. ${escapeHTML(String(applyJobs.length))} apply job${applyJobs.length === 1 ? '' : 's'} queued${installJobs.length ? `, ${escapeHTML(String(installJobs.length))} runtime install job${installJobs.length === 1 ? '' : 's'} queued` : ''}.</p>
            <span>The submitted form is locked to prevent duplicate rollout. Use "Create another" only for a new pack creation.</span>
          </div>
          <div class="pack-create-completion-actions">
            <button class="primary-btn" id="openInstancesAfterPackCreateBtn" type="button">Open instances</button>
            <button class="secondary-btn" id="createAnotherFromPackBtn" type="button">Create another</button>
          </div>
        </section>`;
    }

    function renderXrayEgressPackFields(pack, draft = {}) {
      if (!packUsesService(pack, 'xray-core')) return '';
      const mode = ['egress_node', 'local_breakout'].includes(String(draft.xray_egress_mode || '')) ? String(draft.xray_egress_mode) : 'auto';
      const egressNodes = nodeOptions(draft.xray_egress_node_id || '', { roles: ['egress'], includeEmpty: true, emptyLabel: 'Select egress node' });
      return `
        <div class="field full pack-egress-control" data-pack-feature="xray-egress">
          <div class="instance-panel-label">VLESS routing</div>
          <div class="pack-create-form-grid compact">
            <div class="field">
              <label>Egress mode</label>
              <select name="xray_egress_mode" data-pack-egress-mode>
                <option value="auto"${mode === 'auto' ? ' selected' : ''}>Auto through managed backhaul</option>
                <option value="egress_node"${mode === 'egress_node' ? ' selected' : ''}>Use selected egress node</option>
                <option value="local_breakout"${mode === 'local_breakout' ? ' selected' : ''}>Local breakout on ingress node</option>
              </select>
              <div class="field-hint">Auto uses the active ingress-to-egress backhaul when the route is unambiguous.</div>
            </div>
            <div class="field">
              <label>Egress node</label>
              <select name="xray_egress_node_id" data-pack-egress-node${mode === 'egress_node' ? '' : ' disabled'}>
                ${egressNodes || '<option value="">No egress nodes available</option>'}
              </select>
              <div class="field-hint">Required only when a concrete egress node is selected.</div>
            </div>
          </div>
        </div>`;
    }

    function renderTrafficCamouflagePackFields(pack, draft = {}) {
      if (!packUsesTrafficCamouflage(pack)) return '';
      const usesConfigurablePath = packUsesConfigurableCamouflagePath(pack);
      const camouflagePath = packDraftString(draft, 'camouflage_path', defaultCamouflagePath(pack));
      return `
        <div class="field full pack-camouflage-control" data-pack-feature="camouflage">
          <div class="instance-panel-label">Traffic camouflage</div>
          <div class="pack-create-form-grid compact">
            ${usesConfigurablePath ? `
            <div class="field">
              <label>Hidden VLESS path</label>
              <input name="camouflage_path" value="${escapeHTML(camouflagePath)}" placeholder="/assets/site-sync" data-pack-required="true" required>
              <div class="field-hint">Nginx routes only this path to Xray; root traffic goes to the fallback website.</div>
            </div>` : ''}
            <div class="field">
              <label>Fallback website</label>
              <input name="fallback_upstream_url" value="${escapeHTML(packDraftString(draft, 'fallback_upstream_url', ''))}" placeholder="https://target.example.com" data-pack-required="true" required>
              <div class="field-hint">Ordinary browser requests are reverse-proxied to this upstream. It must be a separate website, not this ingress endpoint.</div>
            </div>
            <div class="field">
              <label>Fallback Host header</label>
              <input name="fallback_host_header" value="${escapeHTML(packDraftString(draft, 'fallback_host_header', ''))}" placeholder="auto from fallback URL">
            </div>
            <div class="field">
              <label>Fallback SNI</label>
              <input name="fallback_sni" value="${escapeHTML(packDraftString(draft, 'fallback_sni', ''))}" placeholder="auto for HTTPS upstream">
            </div>
          </div>
        </div>`;
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

    function openVPNComponentPKIProfileOptions(selectedProfile = '') {
      const selected = String(selectedProfile || '').trim();
      const profiles = new Map();
      for (const root of state.platformPKIRoots || []) {
        if (String(root.status || 'active').toLowerCase() !== 'active') continue;
        if (normalizeInstanceServiceCode(root.service_code) !== 'openvpn') continue;
        const profile = String(root.pki_profile || 'default').trim() || 'default';
        if (!profiles.has(profile)) profiles.set(profile, root);
      }
      if (!profiles.has('default')) {
        profiles.set('default', { pki_profile: 'default', common_name: 'default profile' });
      }
      const parts = [`<option value=""${selected ? '' : ' selected'}>Use pack OpenVPN CA profile</option>`];
      Array.from(profiles.entries())
        .sort(([left], [right]) => left.localeCompare(right, 'en'))
        .forEach(([profile, root]) => {
          const label = `${profile} · ${root.common_name || 'service CA root'}`;
          parts.push(`<option value="${escapeHTML(profile)}"${profile === selected ? ' selected' : ''}>${escapeHTML(label)}</option>`);
        });
      return parts.join('');
    }

    function renderPackComponentSettings(component, index, draft = {}) {
      const port = Number(component?.endpoint_port || 0);
      const serviceCode = normalizeInstanceServiceCode(component?.service_code);
      const componentDraft = createPackComponentDraft(draft, index);
      const endpointPort = packDraftString(componentDraft, 'endpoint_port', port ? String(port) : '');
      const openvpnPKIProfile = packDraftString(componentDraft, 'openvpn_pki_profile', '');
      const openVPNSettings = serviceCode === 'openvpn'
        ? `<label class="pack-component-setting">
            <span>OpenVPN CA</span>
            <select name="component_${index}_openvpn_pki_profile" data-pack-component-setting="${index}">
              ${openVPNComponentPKIProfileOptions(openvpnPKIProfile)}
            </select>
          </label>`
        : '';
      return `
        <div class="pack-component-settings">
          <label class="pack-component-setting">
            <span>Listen port</span>
            <input name="component_${index}_endpoint_port" type="number" min="1" max="65535" step="1" value="${escapeHTML(endpointPort)}" data-pack-component-setting="${index}">
          </label>
          ${openVPNSettings}
        </div>`;
    }

    function renderPackComponent(component, index, draft = {}) {
      const port = Number(component?.endpoint_port || 0);
      const serviceCode = normalizeInstanceServiceCode(component?.service_code);
      const usesCamouflage = servicePackComponentUsesTrafficCamouflage(component);
      const usesTLSEdge = servicePackUsesTLSEdgeCertificate({ components: [component] });
      const selected = createPackComponentSelected(draft, index);
      const meta = [
        component?.service_code || 'service',
        component?.preset_key || 'default',
        port ? `port ${port}` : '',
      ].filter(Boolean).join(' · ');
      return `
        <article class="pack-component-card selectable ${selected ? 'is-selected' : ''}" data-pack-component-card="${escapeHTML(String(index))}" data-pack-component-service="${escapeHTML(serviceCode)}" data-pack-component-camouflage="${usesCamouflage ? 'true' : 'false'}" data-pack-component-tls-edge="${usesTLSEdge ? 'true' : 'false'}">
          <label class="pack-component-toggle-row">
            <input class="pack-component-toggle" type="checkbox" name="pack_component_indexes" value="${escapeHTML(String(index))}" data-pack-component-index="${escapeHTML(String(index))}"${selected ? ' checked' : ''}>
            <span class="pack-component-copy">
              <strong>${escapeHTML(component?.label || component?.service_code || `Component ${index + 1}`)}</strong>
              <span>${escapeHTML(meta)}</span>
              <small class="pack-component-description">${escapeHTML(component?.description || 'Instance component generated from the selected pack template.')}</small>
            </span>
          </label>
          ${renderPackComponentSettings(component, index, draft)}
        </article>`;
    }

    function renderPackDetails(pack, draft = {}) {
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
          <div class="pack-component-toolbar">
            <div>
              <strong>Services to create</strong>
              <span id="packComponentCountTag">${escapeHTML(String(components.length))} selected / ${escapeHTML(String(components.length))}</span>
            </div>
            <div class="inline-actions">
              <button class="secondary-btn" id="selectAllPackComponentsBtn" type="button">Select all</button>
              <button class="secondary-btn" id="clearPackComponentsBtn" type="button">Clear</button>
            </div>
          </div>
          <div class="pack-component-grid">
            ${components.length ? components.map((component, index) => renderPackComponent(component, index, draft)).join('') : '<div class="empty">No components in this pack.</div>'}
          </div>
          ${recommendations.length || notes.length ? `
            <div class="pack-notes">
              ${recommendations.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}
              ${notes.map((item) => `<span>${escapeHTML(item)}</span>`).join('')}
            </div>` : ''}
        </section>`;
    }

    function updatePackComponentSelectionState() {
      const form = document.getElementById('createServicePackPageForm');
      if (!form) return;
      const toggles = Array.from(form.querySelectorAll('.pack-component-toggle'));
      let selected = 0;
      const selectedFeatures = {
        camouflage: false,
        openvpn: false,
        tlsEdge: false,
        xray: false,
      };
      toggles.forEach((toggle) => {
        const index = String(toggle.dataset.packComponentIndex || toggle.value || '');
        const selectorIndex = window.CSS && typeof window.CSS.escape === 'function'
          ? window.CSS.escape(index)
          : index.replace(/["\\]/g, '\\$&');
        const checked = Boolean(toggle.checked);
        const card = form.querySelector(`[data-pack-component-card="${selectorIndex}"]`);
        if (checked) selected += 1;
        card?.classList.toggle('is-selected', checked);
        if (checked && card) {
          const serviceCode = normalizeInstanceServiceCode(card.dataset.packComponentService);
          if (serviceCode === 'openvpn') selectedFeatures.openvpn = true;
          if (serviceCode === 'xray-core') selectedFeatures.xray = true;
          if (card.dataset.packComponentCamouflage === 'true') selectedFeatures.camouflage = true;
          if (card.dataset.packComponentTlsEdge === 'true') selectedFeatures.tlsEdge = true;
        }
        form.querySelectorAll(`[data-pack-component-setting="${selectorIndex}"]`).forEach((input) => {
          input.disabled = !checked;
        });
      });
      updatePackFeatureControls(form, 'camouflage', selectedFeatures.camouflage);
      updatePackFeatureControls(form, 'openvpn', selectedFeatures.openvpn);
      updatePackFeatureControls(form, 'tls-edge', selectedFeatures.tlsEdge);
      updatePackFeatureControls(form, 'xray-egress', selectedFeatures.xray);
      syncPackEgressControlState(form);
      const countTag = document.getElementById('packComponentCountTag');
      if (countTag) countTag.textContent = `${selected} selected / ${toggles.length}`;
      const submit = form.querySelector('button[type="submit"]');
      const nodeSelect = form.querySelector('select[name="node_id"]');
      if (submit) submit.disabled = servicePackCreateSucceeded() || servicePackCreateRunning() || !selected || !nodeSelect || nodeSelect.disabled;
    }

    function updatePackFeatureControls(form, feature, enabled) {
      form.querySelectorAll(`[data-pack-feature="${feature}"]`).forEach((section) => {
        section.classList.toggle('is-disabled', !enabled);
        section.querySelectorAll('input, select, textarea').forEach((input) => {
          input.disabled = !enabled;
          if (input.dataset.packRequired === 'true') {
            input.required = enabled;
          }
        });
      });
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
      state.instancesCreatePackDraft = null;
      ensureCreatePackSelection(servicePackCatalogItems());
      render();
    }

    function openInstancesListPage() {
      state.instancesView = 'list';
      state.instancesCreateResult = null;
      state.instancesCreatePackDraft = null;
      render();
    }

    function openManualInstancePage() {
      state.instancesView = 'manual';
      state.instancesCreateResult = null;
      state.instancesCreatePackDraft = null;
      render();
    }

    function renderCreateFromPackPage() {
      setTitle('Create from pack');
      const packs = servicePackCatalogItems();
      const selectedPack = ensureCreatePackSelection(packs);
      const draft = createPackDraft();
      const completed = servicePackCreateSucceeded();
      const running = servicePackCreateRunning();
      const nodeSelect = nodeOptions(draft.node_id || '');
      const usesOpenVPN = packUsesService(selectedPack, 'openvpn');
      const usesXray = packUsesService(selectedPack, 'xray-core');
      const usesTLSEdgeCertificate = servicePackUsesTLSEdgeCertificate(selectedPack);
      const usesTrafficCamouflage = packUsesTrafficCamouflage(selectedPack);
      const certificateSelect = certificateOptions(draft.certificate_id || defaultLeafCertificateID(), true);
      const baseName = selectedPack?.base_name_template || 'edge-service-pack';
      const endpointHint = selectedPack?.endpoint_hint || 'edge.example.com';
      const endpointRequired = Boolean(selectedPack?.requires_endpoint_host);
      const displayedBaseName = packDraftString(draft, 'base_name', baseName);
      const displayedEndpointHost = packDraftString(draft, 'endpoint_host', '');
      el('content').innerHTML = `
        ${renderInstancesTabs('create-pack')}
        <section class="table-card instance-create-page ${completed ? 'is-create-complete' : ''}">
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
              ${renderCreatePackCompletionBanner(selectedPack, draft)}
              <form id="createServicePackPageForm" class="pack-create-form ${completed ? 'is-completed' : ''}"${completed ? ' aria-disabled="true"' : ''}>
                <input type="hidden" name="service_pack_key" value="${escapeHTML(selectedPack?.key || '')}">
                ${renderPackDetails(selectedPack, draft)}
                <div class="pack-create-form-grid stable">
                  <div class="field">
                    <label>Node</label>
                    <select name="node_id" required${nodeSelect ? '' : ' disabled'}>
                      ${nodeSelect || '<option value="">No active nodes available</option>'}
                    </select>
                  </div>
                  <div class="field">
                    <label>Base name</label>
                    <input name="base_name" value="${escapeHTML(displayedBaseName)}" placeholder="${escapeHTML(baseName)}" required>
                  </div>
                  <div class="field full">
                    <label>Endpoint host</label>
                    <input name="endpoint_host" value="${escapeHTML(displayedEndpointHost)}" placeholder="${escapeHTML(endpointHint)}"${endpointRequired ? ' required' : ''}>
                  </div>
                  ${usesTLSEdgeCertificate ? `
                    <div class="field full" data-pack-feature="tls-edge">
                      <label>TLS edge certificate</label>
                      <select name="certificate_id">${certificateSelect}</select>
                      <div class="field-hint">Optional override for TLS-facing Nginx or Xray TLS components. The platform default certificate is selected automatically.</div>
                    </div>` : ''}
                  ${usesOpenVPN ? `
                    <div class="field full" data-pack-feature="openvpn">
                      <label>OpenVPN CA profile</label>
                      <select name="openvpn_pki_profile">${servicePKIProfileOptions('openvpn', draft.openvpn_pki_profile || 'default')}</select>
                      <div class="field-hint">OpenVPN instances created with this profile trust the same service CA root, which is required for a shared endpoint fleet.</div>
                    </div>` : ''}
                  ${usesTrafficCamouflage ? renderTrafficCamouflagePackFields(selectedPack, draft) : ''}
                  ${usesXray ? renderXrayEgressPackFields(selectedPack, draft) : ''}
                </div>
                <div class="pack-create-actions">
                  <button class="primary-btn" type="submit"${selectedPack && nodeSelect && !completed && !running ? '' : ' disabled'}>${completed ? 'Created' : running ? 'Creating...' : 'Create instances'}</button>
                  <button class="secondary-btn" id="resetPackCreateFormBtn" type="button"${completed || running ? ' disabled' : ''}>Reset form</button>
                  ${completed ? '<span class="pack-create-lock-note">This completed rollout is locked against duplicate submission.</span>' : ''}
                </div>
              </form>
              <div id="createServicePackPageResult">${renderCreatePackResult()}</div>
            </div>
          </div>
        </section>`;
      bindCreateFromPackPage();
    }

    function bindCreateFromPackPage() {
      bindInstancesTabs();
      document.getElementById('backToInstancesBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createInstanceBtn')?.addEventListener('click', openManualInstancePage);
      document.getElementById('resetPackCreateFormBtn')?.addEventListener('click', () => {
        state.instancesCreateResult = null;
        state.instancesCreatePackDraft = null;
        renderCreateFromPackPage();
      });
      document.querySelectorAll('.pack-choice-btn').forEach((button) => {
        button.addEventListener('click', () => {
          if (servicePackCreateSucceeded()) return;
          state.instancesCreatePackKey = button.dataset.packKey || '';
          state.instancesCreateResult = null;
          state.instancesCreatePackDraft = null;
          renderCreateFromPackPage();
        });
      });
      document.querySelectorAll('.pack-component-toggle').forEach((input) => {
        input.addEventListener('change', updatePackComponentSelectionState);
      });
      document.getElementById('selectAllPackComponentsBtn')?.addEventListener('click', () => {
        document.querySelectorAll('.pack-component-toggle').forEach((input) => { input.checked = true; });
        updatePackComponentSelectionState();
      });
      document.getElementById('clearPackComponentsBtn')?.addEventListener('click', () => {
        document.querySelectorAll('.pack-component-toggle').forEach((input) => { input.checked = false; });
        updatePackComponentSelectionState();
      });
      document.getElementById('createServicePackPageForm')?.addEventListener('submit', submitCreateFromPackPage);
      bindPackEgressControls();
      updatePackComponentSelectionState();
      if (servicePackCreateSucceeded()) {
        lockCompletedCreatePackPage();
      }
      document.getElementById('openInstancesAfterPackCreateBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createAnotherFromPackBtn')?.addEventListener('click', () => {
        state.instancesCreateResult = null;
        state.instancesCreatePackDraft = null;
        renderCreateFromPackPage();
      });
    }

    function lockCompletedCreatePackPage() {
      const form = document.getElementById('createServicePackPageForm');
      form?.querySelectorAll('input, select, textarea, button').forEach((control) => {
        control.disabled = true;
      });
      document.querySelectorAll('.pack-choice-btn').forEach((button) => {
        button.disabled = true;
      });
    }

    function bindPackEgressControls() {
      const form = document.getElementById('createServicePackPageForm');
      if (!form) return;
      const modeSelect = form.querySelector('[data-pack-egress-mode]');
      if (!modeSelect) return;
      modeSelect.addEventListener('change', () => syncPackEgressControlState(form));
      syncPackEgressControlState(form);
    }

    function syncPackEgressControlState(form) {
      const modeSelect = form?.querySelector('[data-pack-egress-mode]');
      const nodeSelect = form?.querySelector('[data-pack-egress-node]');
      if (!modeSelect || !nodeSelect) return;
      const section = modeSelect.closest('[data-pack-feature="xray-egress"]');
      const featureEnabled = !section || !section.classList.contains('is-disabled');
      const mode = String(modeSelect.value || 'auto');
      const needsNode = featureEnabled && mode === 'egress_node';
      modeSelect.disabled = !featureEnabled;
      nodeSelect.disabled = !needsNode;
      nodeSelect.required = needsNode;
      if (!needsNode) nodeSelect.value = '';
    }

    async function submitCreateFromPackPage(event) {
      event.preventDefault();
      if (servicePackCreateSucceeded() || servicePackCreateRunning()) return;
      const form = event.currentTarget;
      const button = form.querySelector('button[type="submit"]');
      const data = new FormData(form);
      state.instancesCreatePackDraft = createPackDraftFromForm(form);
      const packKey = String(data.get('service_pack_key') || state.instancesCreatePackKey || '').trim();
      const payload = {
        node_id: String(data.get('node_id') || '').trim(),
        base_name: String(data.get('base_name') || '').trim(),
        endpoint_host: String(data.get('endpoint_host') || '').trim(),
        certificate_id: String(data.get('certificate_id') || '').trim(),
        openvpn_pki_profile: String(data.get('openvpn_pki_profile') || '').trim(),
        xray_egress_mode: String(data.get('xray_egress_mode') || '').trim(),
        xray_egress_node_id: String(data.get('xray_egress_node_id') || '').trim(),
        camouflage_path: String(data.get('camouflage_path') || '').trim(),
        fallback_upstream_url: String(data.get('fallback_upstream_url') || '').trim(),
        fallback_host_header: String(data.get('fallback_host_header') || '').trim(),
        fallback_sni: String(data.get('fallback_sni') || '').trim(),
        auto_install_runtime: true,
      };
      const componentIndexes = data.getAll('pack_component_indexes')
        .map((value) => Number.parseInt(String(value), 10))
        .filter((value) => Number.isInteger(value) && value >= 0);
      if (!componentIndexes.length) {
        state.instancesCreateResult = { status: 'failed', message: 'select at least one service to create' };
        renderCreateFromPackPage();
        return;
      }
      payload.components = componentIndexes.map((index) => {
        const component = { index };
        const port = Number.parseInt(String(data.get(`component_${index}_endpoint_port`) || ''), 10);
        const pkiProfile = String(data.get(`component_${index}_openvpn_pki_profile`) || '').trim();
        if (Number.isInteger(port) && port > 0) component.endpoint_port = port;
        if (pkiProfile) component.openvpn_pki_profile = pkiProfile;
        return component;
      });
      if (!packKey) {
        state.instancesCreateResult = { status: 'failed', message: 'service pack is required' };
        renderCreateFromPackPage();
        return;
      }
      const pack = servicePackCatalogItems().find((item) => item.key === packKey);
      if (packUsesTrafficCamouflage(pack)) {
        const loopError = camouflageFallbackLoopError(payload);
        if (loopError) {
          state.instancesCreateResult = { status: 'failed', message: loopError };
          renderCreateFromPackPage();
          return;
        }
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
        window.requestAnimationFrame(() => {
          document.getElementById('packCreateCompletionBanner')?.scrollIntoView({ block: 'start', behavior: 'smooth' });
        });
      } catch (err) {
        state.instancesCreateResult = { status: 'failed', message: err.message || 'service pack create failed', payload: err.payload || null };
        renderCreateFromPackPage();
      }
    }

    function renderManualInstancePage() {
      setTitle('Manual instance');
      el('content').innerHTML = `
        ${renderInstancesTabs('manual')}
        <section class="table-card instance-create-page">
          <div class="table-head instance-create-head">
            <div>
              <h2>Manual instance</h2>
              <div class="metric-caption">Create a single service instance with explicit node, endpoint, and runtime settings.</div>
            </div>
            <div class="table-tools">
              <button class="secondary-btn" id="backToInstancesBtn" type="button">Back to instances</button>
              <button class="secondary-btn" id="createServicePackBtn" type="button">Create from pack</button>
            </div>
          </div>
          <div class="manual-instance-page">
            <section class="control-page-intro">
              <div>
                <div class="eyebrow">Manual workflow</div>
                <h3>Use this path for one-off services</h3>
                <p>For standard multi-service access use a service pack. Manual instance creation is intended for additional OpenVPN listeners, custom Xray/VLESS endpoints, service experiments, and controlled exceptions.</p>
              </div>
              <div class="control-page-intro-grid">
                <div class="fact-card"><div class="mini-label">Scope</div><div class="metric-caption strong">single instance</div><div class="metric-caption">one runtime unit</div></div>
                <div class="fact-card"><div class="mini-label">Apply</div><div class="metric-caption strong">after validation</div><div class="metric-caption">review before rollout</div></div>
              </div>
            </section>
            <section class="section-card">
              <div class="section-head">
                <div>
                  <div class="mini-label">Instance definition</div>
                  <h3>Service and runtime settings</h3>
                </div>
              </div>
              <div id="manualInstanceCreateMount"></div>
            </section>
          </div>
        </section>`;
      bindInstancesTabs();
      document.getElementById('backToInstancesBtn')?.addEventListener('click', openInstancesListPage);
      document.getElementById('createServicePackBtn')?.addEventListener('click', openCreateFromPackPage);
      renderCreateInstanceForm('manualInstanceCreateMount', { submitLabel: 'Create manual instance' });
    }

    function bindActions() {
      document.getElementById('createInstanceBtn')?.addEventListener('click', openManualInstancePage);
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
        button.addEventListener('click', () => openInstanceManagePage(button.dataset.instanceId));
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

    function canManageVLESSGroups() {
      return hasPermission('settings.manage');
    }

    function vlessGroupCatalogItems() {
      const source = canManageVLESSGroups()
        ? (state.vlessGroupCatalog || state.vlessGroupTemplates || [])
        : (state.vlessGroupTemplates || []);
      return (Array.isArray(source) ? source : [])
        .filter((group) => group && group.key)
        .filter((group) => String(group.status || 'active').toLowerCase() !== 'deleted')
        .slice()
        .sort((left, right) => Number(left.display_order || 1000) - Number(right.display_order || 1000)
          || String(left.label || left.key).localeCompare(String(right.label || right.key), 'en'));
    }

    function vlessGroupModeLabel(group) {
      const mode = String(group?.access_mode || group?.egress_mode || 'instance_default').toLowerCase();
      if (mode === 'local_breakout' || mode === 'local' || mode === 'direct') return 'Current node exit';
      if (mode === 'egress_node' || mode === 'remote_egress' || mode === 'remote_node') return 'Selected egress node';
      if (mode === 'instance_only' || mode === 'target_instance') return 'Instance-only access';
      if (mode === 'block' || mode === 'blocked' || group?.outbound_tag === 'block') return 'Blocked';
      return 'Instance default route';
    }

    function nodeLabelByID(nodeID) {
      const node = nodeByID(nodeID);
      if (!node) return nodeID || 'not selected';
      return [node.name, node.role, node.address].filter(Boolean).join(' · ');
    }

    function instanceLabelByID(instanceID) {
      const instance = (state.instances || []).find((item) => String(item.id || '') === String(instanceID || ''));
      if (!instance) return instanceID || 'not selected';
      return [instance.name || instance.slug || instance.id, serviceLabel(instance.service_code), instanceEndpoint(instance)].filter(Boolean).join(' · ');
    }

    function vlessGroupRouteDetails(group) {
      const parts = [vlessGroupModeLabel(group)];
      if (group?.egress_node_id) parts.push(nodeLabelByID(group.egress_node_id));
      if (group?.target_instance_id) parts.push(instanceLabelByID(group.target_instance_id));
      if (group?.ad_block) parts.push('ad blocking');
      const rules = Array.isArray(group?.rules) ? group.rules.length : 0;
      const extraRules = Array.isArray(group?.extra_rules) ? group.extra_rules.length : 0;
      if (rules + extraRules > 0) parts.push(`${rules + extraRules} advanced rules`);
      return parts.join(' · ');
    }

    function renderVLESSGroupCards(groups) {
      if (!groups.length) {
        return '<div class="empty">No VLESS group templates loaded. Run migrations and seed defaults, or add the first group manually.</div>';
      }
      const manage = canManageVLESSGroups();
      return `
        <div class="vless-group-card-grid">
          ${groups.map((group) => {
            const status = String(group.status || 'active').toLowerCase();
            return `
              <article class="vless-template-card">
                <div class="vless-template-card-head">
                  <div>
                    <h3>${escapeHTML(group.label || group.key)}</h3>
                    <code>${escapeHTML(group.key)}</code>
                  </div>
                  ${status !== 'active' ? statusTag(status) : ''}
                </div>
                <p>${escapeHTML(group.description || 'Reusable VLESS client routing group.')}</p>
                <div class="vless-template-route">
                  <span>${escapeHTML(vlessGroupRouteDetails(group))}</span>
                </div>
                <div class="vless-template-actions">
                  ${manage ? `<button class="secondary-btn vless-group-edit-btn" type="button" data-group-key="${escapeHTML(group.key)}">Edit</button>` : ''}
                  ${manage && status === 'active' ? `<button class="secondary-btn vless-group-disable-btn" type="button" data-group-key="${escapeHTML(group.key)}">Disable</button>` : ''}
                  ${manage && status === 'disabled' ? `<button class="secondary-btn vless-group-enable-btn" type="button" data-group-key="${escapeHTML(group.key)}">Enable</button>` : ''}
                  ${manage ? `<button class="danger-btn vless-group-delete-btn" type="button" data-group-key="${escapeHTML(group.key)}">Delete</button>` : ''}
                </div>
              </article>`;
          }).join('')}
        </div>`;
    }

    function renderVLESSGroupsPage() {
      setTitle('VLESS groups');
      const groups = vlessGroupCatalogItems();
      const active = groups.filter((group) => String(group.status || 'active').toLowerCase() === 'active');
      el('content').innerHTML = `
        ${renderInstancesTabs('vless-groups')}
        <section class="table-card vless-groups-page">
          <div class="table-head">
            <div>
              <h2>VLESS groups</h2>
              <div class="metric-caption">Reusable client routing groups applied to every saved VLESS instance.</div>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(active.length))} active</span>
              <span class="tag">${escapeHTML(String(groups.length))} templates</span>
              ${canManageVLESSGroups() ? '<button class="secondary-btn" id="addVLESSGroupBtn" type="button">Add group</button>' : '<span class="tag">settings.manage required</span>'}
            </div>
          </div>
          <div class="vless-groups-intro">
            <div>
              <strong>Routing model</strong>
              <span>Set groups once here, then assign clients to a group during provisioning. VLESS instance forms only choose the default group.</span>
            </div>
            <div>
              <strong>Safe defaults</strong>
              <span>Default, current-node exit, ad-blocked default and blocked groups are seeded automatically.</span>
            </div>
          </div>
          ${renderVLESSGroupCards(groups)}
        </section>`;
      bindInstancesTabs();
      bindVLESSGroupActions();
    }

    function bindVLESSGroupActions() {
      document.getElementById('addVLESSGroupBtn')?.addEventListener('click', () => openVLESSGroupEditor(''));
      document.querySelectorAll('.vless-group-edit-btn').forEach((button) => {
        button.addEventListener('click', () => openVLESSGroupEditor(button.dataset.groupKey || ''));
      });
      document.querySelectorAll('.vless-group-enable-btn').forEach((button) => {
        button.addEventListener('click', () => setVLESSGroupStatus(button.dataset.groupKey || '', 'enable'));
      });
      document.querySelectorAll('.vless-group-disable-btn').forEach((button) => {
        button.addEventListener('click', () => setVLESSGroupStatus(button.dataset.groupKey || '', 'disable'));
      });
      document.querySelectorAll('.vless-group-delete-btn').forEach((button) => {
        button.addEventListener('click', () => deleteVLESSGroupTemplate(button.dataset.groupKey || ''));
      });
    }

    function vlessGroupEditorModel(groupKey) {
      const existing = vlessGroupCatalogItems().find((group) => group.key === groupKey);
      if (existing) return JSON.parse(JSON.stringify(existing));
      return {
        key: 'remote_egress',
        label: 'Remote egress',
        description: 'Route selected clients through a specific egress node.',
        access_mode: 'egress_node',
        egress_mode: 'egress_node',
        outbound_tag: 'direct',
        rules: [],
        extra_rules: [],
        display_order: 50,
        status: 'active',
      };
    }

    function vlessModeOptions(selected) {
      const mode = String(selected || 'instance_default');
      const options = [
        ['instance_default', 'Instance default route'],
        ['local_breakout', 'Current node exit'],
        ['egress_node', 'Selected egress node'],
        ['instance_only', 'Only selected instance'],
        ['block', 'Block all traffic'],
      ];
      return options.map(([value, label]) => `<option value="${escapeHTML(value)}"${value === mode ? ' selected' : ''}>${escapeHTML(label)}</option>`).join('');
    }

    function targetInstanceOptionsForGroup(selectedID = '') {
      const selected = String(selectedID || '').trim();
      const instances = (state.instances || [])
        .filter((instance) => String(instance.status || '').toLowerCase() !== 'deleted')
        .sort((left, right) => instanceLabelByID(left.id).localeCompare(instanceLabelByID(right.id), 'en'));
      const rows = ['<option value="">Select target instance</option>'];
      for (const instance of instances) {
        rows.push(`<option value="${escapeHTML(instance.id)}"${instance.id === selected ? ' selected' : ''}>${escapeHTML(instanceLabelByID(instance.id))}</option>`);
      }
      return rows.join('');
    }

    function openVLESSGroupEditor(groupKey) {
      if (!canManageVLESSGroups()) {
        openActionOutcomeModal('VLESS groups', 'settings.manage required', 'failed', 'Your role cannot manage VLESS group templates.', []);
        return;
      }
      const model = vlessGroupEditorModel(groupKey);
      const rules = Array.isArray(model.extra_rules) && model.extra_rules.length
        ? model.extra_rules
        : (Array.isArray(model.rules) ? model.rules : []);
      openModal(groupKey ? `Edit VLESS group: ${model.label || groupKey}` : 'Add VLESS group', 'Global client routing template', `
        <form id="vlessGroupEditorForm" class="form-grid vless-group-editor-form">
          <div class="field">
            <label>Label</label>
            <input name="label" value="${escapeHTML(model.label || '')}" placeholder="Remote egress" required>
          </div>
          <div class="field full">
            <label>Description</label>
            <input name="description" value="${escapeHTML(model.description || '')}" placeholder="Short operator-facing explanation">
          </div>
          <div class="field">
            <label>Mode</label>
            <select name="access_mode" data-vless-mode>${vlessModeOptions(model.access_mode || model.egress_mode)}</select>
          </div>
          <div class="field" data-vless-egress-field>
            <label>Egress node</label>
            <select name="egress_node_id">${nodeOptions(model.egress_node_id || '', { roles: ['egress'], includeEmpty: true, emptyLabel: 'Select egress node' })}</select>
          </div>
          <div class="field" data-vless-target-field>
            <label>Target instance</label>
            <select name="target_instance_id">${targetInstanceOptionsForGroup(model.target_instance_id || '')}</select>
          </div>
          <div class="field">
            <label>Order</label>
            <input name="display_order" type="number" min="1" max="9999" value="${escapeHTML(model.display_order || 100)}">
          </div>
          <div class="field" data-vless-adblock-field>
            <label>Filtering</label>
            <label class="checkbox-line">
              <input name="ad_block" type="checkbox"${model.ad_block ? ' checked' : ''}>
              <span>Block managed ad domains</span>
            </label>
          </div>
          <details class="field full advanced-form-section">
            <summary>Advanced internal identity</summary>
            <div class="compact-form-grid">
              <div class="field">
                <label>Internal key</label>
                <input name="key" value="${escapeHTML(model.key || '')}" placeholder="auto from label"${groupKey ? ' readonly' : ''}>
                <div class="field-hint">Generated from the label when empty. Keep it stable after clients start using this group.</div>
              </div>
            </div>
          </details>
          <details class="field full advanced-form-section">
            <summary>Advanced route rules JSON</summary>
            <textarea name="rules_json" rows="5" spellcheck="false" placeholder='[{"domain":["example.com"],"outbound_tag":"direct"}]'>${escapeHTML(rules.length ? JSON.stringify(rules, null, 2) : '')}</textarea>
            <div class="field-hint">Optional Xray routing rules. Instance-only mode automatically creates an allow rule for the selected target instance.</div>
          </details>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save group</button>
            <button class="secondary-btn" id="cancelVLESSGroupEditorBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="vlessGroupEditorResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelVLESSGroupEditorBtn')?.addEventListener('click', closeModal);
      bindVLESSGroupEditorMode();
      document.getElementById('vlessGroupEditorForm')?.addEventListener('submit', submitVLESSGroupEditor);
    }

    function bindVLESSGroupEditorMode() {
      const form = document.getElementById('vlessGroupEditorForm');
      if (!form) return;
      const modeSelect = form.querySelector('[data-vless-mode]');
      const egressField = form.querySelector('[data-vless-egress-field]');
      const targetField = form.querySelector('[data-vless-target-field]');
      const adBlockField = form.querySelector('[data-vless-adblock-field]');
      const egressSelect = form.querySelector('select[name="egress_node_id"]');
      const targetSelect = form.querySelector('select[name="target_instance_id"]');
      const sync = () => {
        const mode = String(modeSelect?.value || 'instance_default');
        if (egressField) egressField.hidden = mode !== 'egress_node';
        if (targetField) targetField.hidden = mode !== 'instance_only';
        if (adBlockField) adBlockField.hidden = mode === 'block' || mode === 'instance_only';
        if (egressSelect) {
          egressSelect.disabled = mode !== 'egress_node';
          egressSelect.required = mode === 'egress_node';
          if (mode !== 'egress_node') egressSelect.value = '';
        }
        if (targetSelect) {
          targetSelect.disabled = mode !== 'instance_only';
          targetSelect.required = mode === 'instance_only';
          if (mode !== 'instance_only') targetSelect.value = '';
        }
      };
      modeSelect?.addEventListener('change', sync);
      sync();
    }

    function vlessGroupSyncSummary(response) {
      if (response?.sync_error) return `sync failed: ${response.sync_error}`;
      const sync = response?.sync;
      if (!sync) return 'saved';
      const failed = Array.isArray(sync.failed_instances) ? sync.failed_instances.length : 0;
      const changed = Number(sync.changed_instances || 0);
      const queued = Number(sync.queued_apply_jobs || 0);
      const skipped = Number(sync.skipped_apply_jobs || 0);
      const parts = [];
      if (changed > 0) parts.push(`${changed} instances synced`);
      if (queued > 0) parts.push(`${queued} apply jobs queued`);
      if (skipped > 0) parts.push(`${skipped} apply jobs skipped`);
      if (failed > 0) parts.push(`${failed} sync failures`);
      return parts.length ? parts.join(' · ') : 'catalog already current';
    }

    function showVLESSGroupSyncWarning(response, groupKey) {
      const failed = Array.isArray(response?.sync?.failed_instances) ? response.sync.failed_instances : [];
      if (!response?.sync_error && failed.length === 0) return;
      const firstFailure = failed[0] || {};
      openActionOutcomeModal('VLESS groups', 'Catalog sync warning', 'failed', 'Group was saved, but propagation needs attention.', [
        { label: 'Group', value: groupKey || response?.key || 'n/a' },
        { label: 'Changed instances', value: response?.sync?.changed_instances ?? 0 },
        { label: 'Queued jobs', value: response?.sync?.queued_apply_jobs ?? 0 },
        { label: 'Failed instances', value: failed.length },
        { label: 'First failure', value: response?.sync_error || firstFailure.error || 'n/a' },
      ]);
    }

    async function submitVLESSGroupEditor(event) {
      event.preventDefault();
      const form = event.currentTarget;
      const result = document.getElementById('vlessGroupEditorResult');
      const data = new FormData(form);
      const mode = String(data.get('access_mode') || 'instance_default');
      const rulesBody = String(data.get('rules_json') || '').trim();
      let rules = [];
      if (rulesBody) {
        try {
          rules = JSON.parse(rulesBody);
          if (!Array.isArray(rules)) throw new Error('Rules JSON must be an array');
        } catch (err) {
          if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message || 'Invalid rules JSON')}</span>`;
          return;
        }
      }
      const payload = {
        key: '',
        label: String(data.get('label') || '').trim(),
        description: String(data.get('description') || '').trim(),
        access_mode: mode,
        egress_mode: mode === 'instance_default' ? 'default' : mode,
        outbound_tag: mode === 'block' || mode === 'instance_only' ? 'block' : 'direct',
        egress_node_id: mode === 'egress_node' ? String(data.get('egress_node_id') || '').trim() : '',
        target_instance_id: mode === 'instance_only' ? String(data.get('target_instance_id') || '').trim() : '',
        ad_block: mode !== 'block' && mode !== 'instance_only' && data.get('ad_block') === 'on',
        rules: mode === 'instance_only' ? [] : rules,
        extra_rules: mode === 'instance_only' ? rules : [],
        status: 'active',
        source: 'operator',
        display_order: Number(data.get('display_order') || 100) || 100,
      };
      payload.key = String(data.get('key') || '').trim() || technicalKeyFromLabel(payload.label, 'vless_group');
      if (result) result.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const saved = await sendJSON(`/api/v1/vless-groups/${encodeURIComponent(payload.key)}`, 'PUT', payload);
        if (result) result.innerHTML = `<span class="tag ok">saved</span> <code>${escapeHTML(saved.key || payload.key)}</code> <small>${escapeHTML(vlessGroupSyncSummary(saved))}</small>`;
        await refresh();
        closeModal();
        renderVLESSGroupsPage();
        showVLESSGroupSyncWarning(saved, saved.key || payload.key);
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message || 'save failed')}</span>`;
      }
    }

    async function setVLESSGroupStatus(groupKey, action) {
      if (!groupKey || !canManageVLESSGroups()) return;
      try {
        const response = await sendJSON(`/api/v1/vless-groups/${encodeURIComponent(groupKey)}/${action}`, 'POST', {});
        await refresh();
        renderVLESSGroupsPage();
        showVLESSGroupSyncWarning(response, groupKey);
      } catch (err) {
        openActionOutcomeModal('VLESS groups', 'Status change failed', 'failed', err.message || 'VLESS group status change failed.', [
          { label: 'Group', value: groupKey },
        ]);
      }
    }

    async function deleteVLESSGroupTemplate(groupKey) {
      if (!groupKey || !canManageVLESSGroups()) return;
      if (!window.confirm(`Delete VLESS group ${groupKey}? Active VLESS instances will be synced and applied automatically.`)) return;
      try {
        const response = await sendJSON(`/api/v1/vless-groups/${encodeURIComponent(groupKey)}`, 'DELETE', null);
        await refresh();
        renderVLESSGroupsPage();
        showVLESSGroupSyncWarning(response, groupKey);
      } catch (err) {
        openActionOutcomeModal('VLESS groups', 'Delete failed', 'failed', err.message || 'VLESS group delete failed.', [
          { label: 'Group', value: groupKey },
        ]);
      }
    }

    function render() {
      const view = normalizedInstancesView();
      if (view === 'create-pack') {
        renderCreateFromPackPage();
        return;
      }
      if (view === 'manual') {
        renderManualInstancePage();
        return;
      }
      if (view === 'service-packs') {
        setTitle('Service pack catalog');
        el('content').innerHTML = `
          ${renderInstancesTabs('service-packs')}
          ${renderServicePackCatalog()}`;
        bindInstancesTabs();
        bindServicePackCatalogActions();
        return;
      }
      if (view === 'vless-groups') {
        renderVLESSGroupsPage();
        return;
      }
      state.instancesView = 'list';
      setTitle('Instances');
      const instances = Array.isArray(state.instances) ? state.instances : [];
      const runtimeReports = Array.isArray(state.instanceRuntimeStates) ? state.instanceRuntimeStates.length : 0;
      el('content').innerHTML = `
        ${renderInstancesTabs('list')}
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
        </section>`;
      bindInstancesTabs();
      bindActions();
    }

    function bindServicePackCatalogActions() {
      document.getElementById('addServicePackTemplateBtn')?.addEventListener('click', () => openServicePackEditor(''));
      document.querySelectorAll('.service-pack-edit-btn').forEach((button) => {
        button.addEventListener('click', () => openServicePackEditor(button.dataset.packKey));
      });
      document.querySelectorAll('.service-pack-delete-btn').forEach((button) => {
        button.addEventListener('click', () => deleteServicePackTemplate(button.dataset.packKey));
      });
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
      const effectiveKey = key || technicalKeyFromLabel(payload?.label || payload?.base_name_template, 'service_pack');
      payload.key = effectiveKey;
      if (result) result.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const saved = await sendJSON(`/api/v1/service-packs/${encodeURIComponent(effectiveKey)}`, 'PUT', payload);
        if (textarea) textarea.value = JSON.stringify(saved, null, 2);
        if (result) result.innerHTML = `<span class="tag ok">saved</span> <code>${escapeHTML(saved.key || effectiveKey)}</code>`;
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
          <p>This queues an agent cleanup job on the selected node. After cleanup succeeds, the control plane removes dependent client service access, managed routes, generated config artifacts and service-access secrets for this instance.</p>
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
