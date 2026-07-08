(function (window) {
  'use strict';

  function createClientsPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      tableCard,
      statusTag,
      escapeHTML,
      formatDate,
      requestJSON,
      sendJSON,
      refresh,
      setPage,
      openModal,
      closeModal,
      openActionOutcomeModal,
      renderActionResponse,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof renderActionResponse !== 'function'
    ) {
      throw new Error('MegaVPNClientsPage requires page dependencies');
    }

    function findClient(clientID) {
      return (state.clients || []).find((item) => item.id === clientID) || null;
    }

    function findInstance(instanceID) {
      return (state.instances || []).find((item) => item.id === instanceID) || null;
    }

    function findNode(nodeID) {
      return (state.nodes || []).find((item) => item.id === nodeID) || null;
    }

    function egressNodeOptions(selectedID = '') {
      const nodes = (state.nodes || [])
        .filter((node) => String(node.role || '').toLowerCase() === 'egress')
        .filter((node) => String(node.status || '').toLowerCase() !== 'retired');
      const options = ['<option value="">Select egress node</option>'];
      nodes.forEach((node) => {
        const selected = node.id === selectedID ? ' selected' : '';
        const address = node.address ? ` · ${node.address}` : '';
        const role = String(node.role || 'node').trim() || 'node';
        options.push(`<option value="${escapeHTML(node.id)}"${selected}>${escapeHTML(node.name || node.id)} · ${escapeHTML(role)}${escapeHTML(address)}</option>`);
      });
      return options.join('');
    }

    function textValue(value) {
      return String(value ?? '').trim();
    }

    function firstTextValue(...values) {
      for (const value of values) {
        const text = textValue(value);
        if (text) return text;
      }
      return '';
    }

    function positiveNumber(...values) {
      for (const value of values) {
        const number = Number(value);
        if (Number.isFinite(number) && number > 0) return number;
      }
      return 0;
    }

    function endpointText(host, port) {
      const cleanHost = textValue(host);
      const cleanPort = positiveNumber(port);
      if (!cleanHost) return cleanPort ? `:${cleanPort}` : 'n/a';
      return cleanPort ? `${cleanHost}:${cleanPort}` : cleanHost;
    }

    function endpointLabel(instance) {
      return endpointText(instance?.endpoint_host, instance?.endpoint_port);
    }

    function instanceSpec(instance) {
      return instance?.spec && typeof instance.spec === 'object' ? instance.spec : {};
    }

    function servicePublicEndpointInfo(instance, metadata = {}, inbound = {}) {
      const spec = instanceSpec(instance);
      const serviceCode = firstTextValue(inbound.service_code, instance?.service_code).toLowerCase();
      const backendEndpoint = firstTextValue(inbound.backend_endpoint) || endpointLabel(instance);
      const hasPublicProfile = serviceCode === 'xray-core' && Boolean(
        firstTextValue(
          metadata.public_host,
          spec.public_host,
          metadata.public_security,
          spec.public_security,
          metadata.public_network,
          spec.public_network,
          metadata.public_path,
          spec.public_path,
          metadata.public_host_header,
          spec.public_host_header,
        ) || positiveNumber(metadata.public_port, spec.public_port),
      );

      if (!hasPublicProfile) {
        return {
          endpoint: firstTextValue(inbound.endpoint) || backendEndpoint,
          backendEndpoint: '',
          endpointKind: 'service',
          transportLabel: '',
        };
      }

      const host = firstTextValue(
        inbound.client_endpoint_host,
        metadata.public_host,
        metadata.server_host,
        spec.public_host,
        spec.server_host,
        inbound.endpoint_host,
        instance?.endpoint_host,
        metadata.public_host_header,
        spec.public_host_header,
      );
      const port = positiveNumber(
        inbound.client_endpoint_port,
        metadata.public_port,
        metadata.server_port,
        spec.public_port,
        spec.server_port,
        instance?.endpoint_port,
      );
      const endpoint = firstTextValue(inbound.client_endpoint) || endpointText(host, port);
      const security = firstTextValue(metadata.public_security, spec.public_security, metadata.security, spec.security);
      const network = firstTextValue(metadata.public_network, spec.public_network, metadata.type, spec.type, metadata.network, spec.network);
      const path = firstTextValue(metadata.public_path, spec.public_path, metadata.path, spec.path);
      const transportParts = [security, network].filter(Boolean);
      return {
        endpoint,
        backendEndpoint: backendEndpoint && backendEndpoint !== endpoint ? backendEndpoint : '',
        endpointKind: 'public',
        transportLabel: `${transportParts.join(' / ')}${path ? ` ${path}` : ''}`.trim(),
      };
    }

    function serviceAccessInboundInfo(access) {
      const metadata = access?.metadata && typeof access.metadata === 'object' ? access.metadata : {};
      const inbound = metadata.inbound_service && typeof metadata.inbound_service === 'object' ? metadata.inbound_service : {};
      const instance = findInstance(access?.instance_id);
      const node = findNode(inbound.node_id || instance?.node_id);
      const serviceCode = inbound.service_code || instance?.service_code || 'unknown';
      const endpointInfo = servicePublicEndpointInfo(instance, metadata, inbound);
      return {
        serviceCode,
        serviceLabel: inbound.service_label || compactServiceLabel(serviceCode),
        instanceName: inbound.instance_name || instance?.name || access?.instance_id || 'instance',
        nodeName: inbound.node_name || node?.name || instance?.node_id || 'node',
        nodeRole: inbound.node_role || node?.role || 'role n/a',
        nodeAddress: inbound.node_address || node?.address || '',
        endpoint: endpointInfo.endpoint,
        backendEndpoint: endpointInfo.backendEndpoint,
        endpointKind: endpointInfo.endpointKind,
        transportLabel: endpointInfo.transportLabel,
        outboundGroup: metadata.vless_group || metadata.xray_group || metadata.outbound_group || inbound.vless_group || inbound.xray_group || inbound.outbound_group || '',
        availability: inbound.availability || (metadata.available_inbound === false ? 'disabled' : 'available'),
        available: inbound.available !== false && metadata.available_inbound !== false,
      };
    }

    function serviceInstanceLabel(instance) {
      const node = findNode(instance.node_id);
      const role = node?.role ? `/${node.role}` : '';
      const endpoint = endpointLabel(instance);
      return `${instance.name || instance.id} - ${instance.service_code || 'service'} - ${node?.name || instance.node_id || 'node'}${role} - ${endpoint}`;
    }

    function artifactTypeLabel(artifactType) {
      switch (String(artifactType || '').trim()) {
        case 'ovpn':
          return '.ovpn';
        case 'vless_url':
          return 'VLESS URL';
        case 'wg_conf':
          return 'WireGuard';
        case 'mtproto_url':
          return 'MTProto URL';
        case 'http_proxy_bundle':
          return 'HTTP proxy';
        case 'ss_url':
          return 'Shadowsocks URL';
        case 'ipsec_bundle':
          return 'IPsec/L2TP';
        case 'zip_bundle':
          return 'ZIP bundle';
        default:
          return artifactType || 'artifact';
      }
    }

    function shareLinkIsUsable(link) {
      const status = String(link?.status || '').toLowerCase();
      if (status !== 'active') return false;
      if (!link?.expires_at) return true;
      const expiresAt = Date.parse(link.expires_at);
      return Number.isNaN(expiresAt) || expiresAt > Date.now();
    }

    function shareLinkDisplayStatus(link) {
      const status = String(link?.status || 'unknown').toLowerCase();
      if (status === 'active' && !shareLinkIsUsable(link)) return 'expired';
      return status;
    }

    function subscriptionDisplayStatus(subscription) {
      const status = String(subscription?.status || 'unknown').toLowerCase();
      if (status === 'active' && subscription?.expires_at) {
        const expiresAt = Date.parse(subscription.expires_at);
        if (!Number.isNaN(expiresAt) && expiresAt <= Date.now()) return 'expired';
      }
      return status;
    }

    function clientSummary(client) {
      const summary = client?.summary && typeof client.summary === 'object' ? client.summary : {};
      const clientID = client?.id || '';
      const artifacts = (state.artifacts || []).filter((artifact) => artifact.client_account_id === clientID);
      const shareLinks = (state.shareLinks || []).filter((link) => link.client_account_id === clientID);
      const readyArtifacts = artifacts.filter((artifact) => String(artifact.status || '').toLowerCase() === 'ready');
      const activeLinks = shareLinks.filter(shareLinkIsUsable);
      return {
        serviceAccessCount: Number(summary.service_access_count || 0),
        activeServiceAccessCount: Number(summary.active_service_access_count || 0),
        pendingServiceAccessCount: Number(summary.pending_service_access_count || 0),
        routeCount: Number(summary.route_count || 0),
        activeRouteCount: Number(summary.active_route_count || 0),
        artifactCount: Number(summary.artifact_count ?? artifacts.length),
        readyArtifactCount: Number(summary.ready_artifact_count ?? readyArtifacts.length),
        shareLinkCount: Number(summary.share_link_count ?? shareLinks.length),
        activeShareLinkCount: Number(summary.active_share_link_count ?? activeLinks.length),
        lastArtifactAt: summary.last_artifact_at || artifacts[0]?.created_at || '',
        nextShareLinkExpiresAt: summary.next_share_link_expires_at || activeLinks[0]?.expires_at || '',
      };
    }

    function clientLifecycleStatus(client) {
      const status = String(client.status || 'unknown').toLowerCase();
      if (status === 'active' && client.expires_at && Date.parse(client.expires_at) < Date.now()) return 'expired';
      return status;
    }

    function clientDisplayName(client) {
      return client.display_name || client.username || client.id;
    }

    function compactServiceLabel(value) {
      const normalized = String(value || '').trim();
      const catalog = clientAccessServiceByCode(normalized);
      if (catalog) return catalog.display_name || catalog.service_code || normalized;
      switch (normalized) {
        case 'vless':
          return 'VLESS / Xray';
        case 'xray-core':
          return 'Xray';
        case 'http_proxy':
          return 'HTTP proxy';
        case 'wireguard':
          return 'WireGuard';
        case 'openvpn':
          return 'OpenVPN';
        case 'shadowsocks':
          return 'Shadowsocks';
        case 'mtproto':
          return 'MTProto';
        case 'ipsec':
          return 'IPsec';
        default:
          return normalized || 'service';
      }
    }

    function clientAccessServices() {
      const items = Array.isArray(state.clientAccessServices) ? state.clientAccessServices : [];
      if (items.length) return items;
      return [
        { service_code: 'vless', display_name: 'VLESS / Xray', category: 'vpn', status: 'active', supports_groups: true, supports_membership: true, supports_materialization: true },
        { service_code: 'openvpn', display_name: 'OpenVPN', category: 'vpn', status: 'coming_soon', supports_groups: false },
        { service_code: 'wireguard', display_name: 'WireGuard', category: 'vpn', status: 'coming_soon', supports_groups: false },
        { service_code: 'l2tp', display_name: 'L2TP / IPsec', category: 'vpn', status: 'coming_soon', supports_groups: false },
        { service_code: 'http_proxy', display_name: 'HTTP Proxy', category: 'proxy', status: 'coming_soon', supports_groups: false },
        { service_code: 'shadowsocks', display_name: 'Shadowsocks', category: 'proxy', status: 'coming_soon', supports_groups: false },
        { service_code: 'mtproto', display_name: 'MTProto', category: 'proxy', status: 'coming_soon', supports_groups: false },
        { service_code: 'socks_proxy', display_name: 'SOCKS Proxy', category: 'proxy', status: 'planned', supports_groups: false },
      ];
    }

    function normalizeClientAccessServiceCode(value) {
      const code = String(value || '').trim().toLowerCase();
      if (['xray', 'xray-core', 'xray_core'].includes(code)) return 'vless';
      if (['l2tpd', 'xl2tpd', 'ipsec'].includes(code)) return 'l2tp';
      if (['http', 'http-proxy'].includes(code)) return 'http_proxy';
      if (['ss'].includes(code)) return 'shadowsocks';
      return code;
    }

    function clientAccessServiceByCode(value) {
      const code = normalizeClientAccessServiceCode(value);
      return clientAccessServices().find((service) => normalizeClientAccessServiceCode(service.service_code) === code) || null;
    }

    function serviceSupportsGroupManagement(serviceOrCode) {
      const service = typeof serviceOrCode === 'object' ? serviceOrCode : clientAccessServiceByCode(serviceOrCode);
      return Boolean(service?.supports_groups && service?.supports_membership && service?.supports_materialization);
    }

    function clientAccessServiceStatusLabel(service) {
      const status = String(service?.status || (serviceSupportsGroupManagement(service) ? 'active' : 'coming_soon')).trim();
      if (status === 'coming_soon') return 'coming soon';
      if (status === 'catalog_only') return 'catalog only';
      return status || 'unknown';
    }

    function clientAccessServiceOptions(selectedCode = 'vless', includeAll = false) {
      const selected = normalizeClientAccessServiceCode(selectedCode || 'vless');
      const rows = [];
      if (includeAll) rows.push(`<option value="all"${selected === 'all' ? ' selected' : ''}>All services</option>`);
      for (const service of clientAccessServices()) {
        const code = normalizeClientAccessServiceCode(service.service_code);
        const suffix = serviceSupportsGroupManagement(service) ? '' : ` · ${clientAccessServiceStatusLabel(service)}`;
        rows.push(`<option value="${escapeHTML(code)}"${selected === code ? ' selected' : ''}>${escapeHTML(service.display_name || code)}${escapeHTML(suffix)}</option>`);
      }
      return rows.join('');
    }

    function artifactDownloadURL(clientID, artifactID) {
      const path = `/api/v1/clients/${encodeURIComponent(clientID)}/artifacts/${encodeURIComponent(artifactID)}/download`;
      try {
        return new URL(path, state.apiBase || window.location.origin).toString();
      } catch (_) {
        return path;
      }
    }

    function artifactPreviewable(artifactType) {
      return ['ovpn', 'vless_url', 'wg_conf', 'mtproto_url', 'http_proxy_bundle', 'ss_url', 'ipsec_bundle'].includes(String(artifactType || '').trim());
    }

    function provisionableClientInstances() {
      const allowed = new Set(['openvpn', 'xray-core', 'wireguard', 'mtproto', 'ipsec', 'http_proxy', 'shadowsocks']);
      return (state.instances || []).filter((instance) => {
        return allowed.has(String(instance.service_code || '').trim()) && String(instance.status || '').toLowerCase() !== 'deleted';
      }).sort((left, right) => serviceInstanceLabel(left).localeCompare(serviceInstanceLabel(right)));
    }

    function normalizeVLESSGroupKey(value) {
      return String(value || '').trim().replace(/\s+/g, '_').replace(/[^A-Za-z0-9_.:-]/g, '').slice(0, 64);
    }

    function vlessGroupsForInstance(instance) {
      const spec = instance?.spec && typeof instance.spec === 'object' ? instance.spec : {};
      const instanceGroups = Array.isArray(spec.vless_groups)
        ? spec.vless_groups
        : (Array.isArray(spec.xray_groups) ? spec.xray_groups : (Array.isArray(spec.outbound_groups) ? spec.outbound_groups : []));
      const catalogGroups = Array.isArray(state.vlessGroupTemplates) && state.vlessGroupTemplates.length
        ? state.vlessGroupTemplates
        : (Array.isArray(state.vlessGroupCatalog) ? state.vlessGroupCatalog.filter((group) => String(group.status || 'active').toLowerCase() === 'active') : []);
      const rawGroups = [...catalogGroups, ...instanceGroups];
      const seen = new Set();
      const groups = rawGroups.map((group) => {
        const source = group && typeof group === 'object' ? group : {};
        const key = normalizeVLESSGroupKey(source.key || source.name || source.id);
        if (!key || seen.has(key)) return null;
        seen.add(key);
        return {
          key,
          label: String(source.label || source.title || key).trim() || key,
          accessMode: String(source.access_mode || source.egress_mode || source.mode || '').trim(),
          egressNodeID: String(source.egress_node_id || source.node_id || source.egress?.egress_node_id || source.egress?.node_id || '').trim(),
          targetInstanceLabel: String(source.target_instance_label || '').trim(),
          outboundTag: String(source.outbound_tag || source.outboundTag || source.tag || 'direct').trim() || 'direct',
          adBlock: source.ad_block === true || source.adBlock === true || source.block_ads === true || source.blockAds === true || (Array.isArray(source.rules) && source.rules.some((rule) => {
            const tag = String(rule?.outboundTag || rule?.outbound_tag || '').trim().toLowerCase();
            const domains = Array.isArray(rule?.domain) ? rule.domain : [];
            return tag === 'block' && domains.some((domain) => String(domain || '').trim().toLowerCase() === 'geosite:category-ads-all');
          })),
        };
      }).filter(Boolean);
      if (!groups.length) {
        const key = normalizeVLESSGroupKey(spec.default_vless_group || spec.default_xray_group || spec.default_outbound_group || 'default') || 'default';
        groups.push({ key, label: key === 'default' ? 'Default access' : key, outboundTag: 'direct' });
      }
      return groups;
    }

    function defaultVLESSGroupForInstance(instance, groups) {
      const spec = instance?.spec && typeof instance.spec === 'object' ? instance.spec : {};
      const wanted = normalizeVLESSGroupKey(spec.default_vless_group || spec.default_xray_group || spec.default_outbound_group || '');
      if (wanted && groups.some((group) => group.key === wanted)) return wanted;
      return groups[0]?.key || 'default';
    }

    function renderVLESSGroupSelect(instance) {
      if (String(instance?.service_code || '').trim() !== 'xray-core') return '';
      const groups = vlessGroupsForInstance(instance);
      const selected = defaultVLESSGroupForInstance(instance, groups);
      const options = groups.map((group) => {
        let routeLabel = '';
        const mode = String(group.accessMode || '').toLowerCase();
        if (mode === 'local_breakout' || mode === 'local' || mode === 'direct') routeLabel = ' · current node';
        else if (mode === 'egress_node' || mode === 'remote_egress' || mode === 'remote_node') routeLabel = ' · selected egress';
        else if (mode === 'instance_only') routeLabel = ` · only ${group.targetInstanceLabel || 'selected instance'}`;
        else if (mode === 'block' || group.outboundTag === 'block') routeLabel = ' · blocked';
        else if (group.outboundTag && group.outboundTag !== 'direct') routeLabel = ` · ${group.outboundTag}`;
        if (group.adBlock) routeLabel += ' · ads blocked';
        return `<option value="${escapeHTML(group.key)}"${group.key === selected ? ' selected' : ''}>${escapeHTML(group.label)}${escapeHTML(routeLabel)}</option>`;
      }).join('');
      return `
        <span class="client-choice-option-row">
          <span>Access group</span>
          <select class="client-vless-group-select" data-instance-id="${escapeHTML(instance.id)}">${options}</select>
        </span>`;
    }

    function serviceInstanceOptions(instances, selectedIDs = [], emptyText = 'No provisionable instances') {
      const selected = new Set(selectedIDs || []);
      return (instances || []).map((instance) => {
        return `<option value="${escapeHTML(instance.id)}"${selected.has(instance.id) ? ' selected' : ''}>${escapeHTML(serviceInstanceLabel(instance))}</option>`;
      }).join('') || `<option value="" disabled>${escapeHTML(emptyText)}</option>`;
    }

    function clientConfigInstanceOptions(accessList = [], selectedIDs = []) {
      const accessInstanceIDs = new Set((accessList || []).map((access) => access.instance_id).filter(Boolean));
      const instances = provisionableClientInstances().filter((instance) => accessInstanceIDs.has(instance.id));
      return serviceInstanceOptions(instances, selectedIDs, 'No provisioned service access yet');
    }

    function renderClientConfigInstanceChoices(accessList = [], selectedIDs = []) {
      const selected = new Set(selectedIDs || []);
      const accessByInstanceID = new Map((accessList || []).filter((access) => access.instance_id).map((access) => [access.instance_id, access]));
      const instances = provisionableClientInstances().filter((instance) => accessByInstanceID.has(instance.id));
      if (!instances.length) {
        return '<div class="empty compact-empty">No provisioned service access yet. Provision this client first.</div>';
      }
      return instances.map((instance) => {
        const access = accessByInstanceID.get(instance.id);
        const node = findNode(instance.node_id);
        const inbound = serviceAccessInboundInfo(access);
        const checked = selected.has(instance.id) ? ' checked' : '';
        return `
          <label class="client-config-choice">
            <input type="checkbox" name="instance_ids" value="${escapeHTML(instance.id)}"${checked} />
            <span class="client-choice-check" aria-hidden="true"></span>
            <span class="client-choice-body">
              <strong>${escapeHTML(instance.name || instance.slug || instance.id)}</strong>
              <small>${escapeHTML(compactServiceLabel(instance.service_code))} · ${escapeHTML(node?.name || instance.node_id || 'node')} · ${escapeHTML(access?.status || 'unknown')}</small>
              <em>${escapeHTML(inbound.endpoint || endpointLabel(instance))}</em>
            </span>
            <span class="client-choice-tags">${statusTag(access?.status || 'unknown')}</span>
          </label>`;
      }).join('');
    }

    function renderActionButtons(client) {
      return `
        <div class="table-actions client-action-grid">
          <button class="secondary-btn client-accesses-btn" type="button" data-client-id="${escapeHTML(client.id)}">Manage</button>
          <button class="secondary-btn client-provision-btn" type="button" data-client-id="${escapeHTML(client.id)}">Provision</button>
          <button class="secondary-btn client-build-btn" type="button" data-client-id="${escapeHTML(client.id)}">Build</button>
          <button class="secondary-btn client-email-btn" type="button" data-client-id="${escapeHTML(client.id)}">Email</button>
          <button class="danger-btn client-delete-btn" type="button" data-client-id="${escapeHTML(client.id)}">Delete</button>
        </div>`;
    }

    function bindListActions() {
      document.getElementById('clientCreateBtn')?.addEventListener('click', openCreateClientModal);
      document.querySelectorAll('.client-provision-btn').forEach((button) => {
        button.addEventListener('click', () => queueClientProvision(button.dataset.clientId));
      });
      document.querySelectorAll('.client-accesses-btn').forEach((button) => {
        button.addEventListener('click', () => openClientAccessesModal(button.dataset.clientId));
      });
      document.querySelectorAll('.client-build-btn').forEach((button) => {
        button.addEventListener('click', () => openBuildClientArtifactsForClient(button.dataset.clientId));
      });
      document.querySelectorAll('.client-email-btn').forEach((button) => {
        button.addEventListener('click', () => openClientEmailModal(button.dataset.clientId));
      });
      document.querySelectorAll('.client-delete-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteClientModal(button.dataset.clientId));
      });
    }

    function renderClientWorkspaceTabs(active = state.clientsTab || 'clients') {
      const tabs = [
        ['clients', 'Clients', 'Provisioning and configs', String((state.clients || []).length)],
        ['groups', 'Groups', 'Client access routing', String((state.clientAccessGroups || []).length)],
      ];
      return `
        <nav class="page-tabs control-tabs clients-tabs" role="tablist" aria-label="Client workspace sections">
          ${tabs.map(([key, label, caption, count]) => `
            <button class="page-tab${active === key ? ' is-active' : ''}" type="button" role="tab" aria-selected="${active === key ? 'true' : 'false'}" data-clients-tab="${escapeHTML(key)}">
              <span>${escapeHTML(label)} <em>${escapeHTML(count)}</em></span>
              <small>${escapeHTML(caption)}</small>
            </button>`).join('')}
        </nav>`;
    }

    function bindClientWorkspaceTabs() {
      document.querySelectorAll('[data-clients-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          state.clientsTab = button.dataset.clientsTab || 'clients';
          localStorage.setItem('megavpn.clientsTab', state.clientsTab);
          render();
        });
      });
    }

    function accessGroupPolicy(group) {
      const policy = group?.policy_json || group?.policy || {};
      if (policy && typeof policy === 'object' && !Array.isArray(policy)) return policy;
      try {
        const parsed = JSON.parse(String(policy || '{}'));
        return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
      } catch (_) {
        return {};
      }
    }

    function accessGroupRouteLabel(group) {
      const policy = accessGroupPolicy(group);
      const mode = String(policy.access_mode || policy.egress_mode || '').trim();
      if (mode === 'egress_node') {
        const node = findNode(policy.egress_node_id);
        return node ? `egress: ${node.name}` : 'selected egress';
      }
      if (mode === 'local_breakout') return 'current node exit';
      if (mode === 'instance_only') return 'selected instance only';
      if (mode === 'block') return 'blocked';
      if (policy.ad_block === true) return 'default + ad blocking';
      return 'default route';
    }

    function accessGroupScopeLabel(group) {
      const mode = String(group?.scope_mode || 'all_active_instances');
      if (mode === 'selected_instances') return 'selected instances';
      if (mode === 'all_except_selected') return 'all except selected';
      return 'all active instances';
    }

    function renderClientGroupsPage() {
      setTitle('Clients');
      const serviceFilter = normalizeClientAccessServiceCode(state.clientAccessGroupsServiceFilter || 'all');
      const allGroups = Array.isArray(state.clientAccessGroups) ? state.clientAccessGroups : [];
      const groups = serviceFilter === 'all' ? allGroups : allGroups.filter((group) => normalizeClientAccessServiceCode(group.service_code) === serviceFilter);
      const conflicts = Array.isArray(state.clientAccessGroupMigrationConflicts) ? state.clientAccessGroupMigrationConflicts : [];
      el('content').innerHTML = `
        <section class="clients-workspace">
          ${renderClientWorkspaceTabs('groups')}
          <section class="table-card">
            <div class="table-head">
              <div>
                <h2>Client access groups</h2>
                <p class="table-subtitle">Groups define client access policy. Runtime instances receive materialized service access automatically.</p>
              </div>
              <div class="table-tools">
                <select id="clientAccessGroupServiceFilter" aria-label="Service filter">
                  ${clientAccessServiceOptions(serviceFilter, true)}
                </select>
                <span class="tag ${conflicts.length ? 'warn' : 'stub'}">${escapeHTML(String(conflicts.length))} migration conflicts</span>
                <button class="primary-btn" id="createAccessGroupBtn" type="button">New group</button>
              </div>
            </div>
            <div class="table-wrap">
              <table class="clients-table access-groups-table">
                <thead>
                  <tr>
                    <th>Group name</th>
                    <th>Service</th>
                    <th>Status</th>
                    <th>Members</th>
                    <th>Scope</th>
                    <th>Route / policy</th>
                    <th>Sync</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>${renderAccessGroupRows(groups)}</tbody>
              </table>
            </div>
          </section>
        </section>`;
      bindClientWorkspaceTabs();
      bindClientGroupsActions();
    }

    function renderAccessGroupRows(groups) {
      if (!groups.length) {
        return '<tr><td colspan="8"><div class="empty">No client access groups yet. Create a VLESS group or run migrations.</div></td></tr>';
      }
      return groups.map((group) => {
        const pending = Number(group.pending_sync_count || 0);
        const failed = Number(group.failed_sync_count || 0);
        const applied = Number(group.applied_sync_count || 0);
        const service = clientAccessServiceByCode(group.service_code);
        const syncTag = failed > 0 ? statusTag('failed') : pending > 0 ? statusTag('pending') : statusTag(applied > 0 ? 'applied' : 'unknown');
        const canManageMembers = serviceSupportsGroupManagement(service);
        return `
          <tr>
            <td>
              <strong>${escapeHTML(group.display_name || group.group_key)}</strong>
              <div class="timeline-meta">${escapeHTML(group.group_key || group.id)}</div>
            </td>
            <td>
              <div>${escapeHTML(compactServiceLabel(group.service_code || 'service'))}</div>
              <div class="timeline-meta">${escapeHTML(clientAccessServiceStatusLabel(service))}</div>
            </td>
            <td>${statusTag(group.status || 'unknown')}</td>
            <td>
              <div class="client-status-cluster">
                <span class="tag">${escapeHTML(String(group.member_count || 0))} members</span>
                <span class="tag ok">${escapeHTML(String(group.active_member_count || 0))} active</span>
              </div>
            </td>
            <td>
              <div>${escapeHTML(accessGroupScopeLabel(group))}</div>
              <div class="timeline-meta">${escapeHTML(String(group.affected_instances || 0))} instances</div>
            </td>
            <td>${escapeHTML(accessGroupRouteLabel(group))}</td>
            <td>${syncTag}</td>
            <td>
              <div class="table-actions compact-actions access-group-actions">
                <button class="${canManageMembers ? 'primary-btn' : 'secondary-btn'} access-group-members-btn" type="button" data-group-id="${escapeHTML(group.id)}"${canManageMembers ? '' : ' disabled'}>Members</button>
                <button class="secondary-btn access-group-edit-btn" type="button" data-group-id="${escapeHTML(group.id)}">Policy</button>
                <button class="secondary-btn access-group-scope-btn" type="button" data-group-id="${escapeHTML(group.id)}">Scope</button>
                <button class="secondary-btn access-group-sync-btn" type="button" data-group-id="${escapeHTML(group.id)}">Sync</button>
              </div>
            </td>
          </tr>`;
      }).join('');
    }

    function bindClientGroupsActions() {
      document.getElementById('clientAccessGroupServiceFilter')?.addEventListener('change', (event) => {
        state.clientAccessGroupsServiceFilter = event.currentTarget.value || 'vless';
        localStorage.setItem('megavpn.clientAccessGroupsServiceFilter', state.clientAccessGroupsServiceFilter);
        render();
      });
      document.getElementById('createAccessGroupBtn')?.addEventListener('click', () => openAccessGroupEditor(null));
      document.querySelectorAll('.access-group-members-btn').forEach((button) => {
        button.addEventListener('click', () => openAccessGroupMembersModal(button.dataset.groupId));
      });
      document.querySelectorAll('.access-group-edit-btn').forEach((button) => {
        button.addEventListener('click', () => openAccessGroupEditor(button.dataset.groupId));
      });
      document.querySelectorAll('.access-group-scope-btn').forEach((button) => {
        button.addEventListener('click', () => openAccessGroupScopeModal(button.dataset.groupId));
      });
      document.querySelectorAll('.access-group-sync-btn').forEach((button) => {
        button.addEventListener('click', () => openAccessGroupSyncModal(button.dataset.groupId));
      });
    }

    function findAccessGroup(groupID) {
      return (state.clientAccessGroups || []).find((group) => group.id === groupID) || null;
    }

    function openAccessGroupEditor(groupID) {
      const group = groupID ? findAccessGroup(groupID) : null;
      const policy = accessGroupPolicy(group);
      const isEdit = Boolean(group);
      const serviceCode = normalizeClientAccessServiceCode(group?.service_code || 'vless');
      const service = clientAccessServiceByCode(serviceCode);
      const serviceEnabled = serviceSupportsGroupManagement(serviceCode);
      openModal(isEdit ? `Edit group: ${group.display_name || group.group_key}` : 'Create client access group', 'Client access routing policy', `
        <form id="accessGroupEditorForm" class="form-grid access-group-editor-form">
          <div class="field">
            <label>Service</label>
            ${isEdit ? `<input type="hidden" name="service_code" value="${escapeHTML(serviceCode)}" />` : ''}
            <select id="accessGroupEditorServiceCode" name="${isEdit ? 'service_code_display' : 'service_code'}"${isEdit ? ' disabled' : ''}>
              ${clientAccessServiceOptions(serviceCode, false)}
            </select>
            <div class="field-hint">Only services with production group materialization can be created.</div>
          </div>
          <div class="field"><label>Group key</label><input name="group_key" required ${isEdit ? 'readonly' : ''} value="${escapeHTML(group?.group_key || '')}" placeholder="out_usa_dallas" /></div>
          <div class="field"><label>Name</label><input name="display_name" required value="${escapeHTML(group?.display_name || '')}" placeholder="Outgoing USA Dallas" /></div>
          <div class="field"><label>Status</label><select name="status"><option value="active"${String(group?.status || 'active') === 'active' ? ' selected' : ''}>active</option><option value="disabled"${String(group?.status || '') === 'disabled' ? ' selected' : ''}>disabled</option></select></div>
          <div class="field full"><label>Description</label><input name="description" value="${escapeHTML(group?.description || '')}" placeholder="Operator note" /></div>
          <div class="field full" id="accessGroupServiceNotice">${renderAccessGroupServiceNotice(service)}</div>
          <div class="field"><label>Route mode</label><select name="access_mode">
            ${['instance_default','local_breakout','egress_node','instance_only','block'].map((mode) => `<option value="${escapeHTML(mode)}"${String(policy.access_mode || 'instance_default') === mode ? ' selected' : ''}>${escapeHTML(mode)}</option>`).join('')}
          </select></div>
          <div class="field"><label>Egress node</label><select name="egress_node_id">${egressNodeOptions(String(policy.egress_node_id || ''))}</select></div>
          <div class="field"><label>Outbound tag</label><input name="outbound_tag" value="${escapeHTML(policy.outbound_tag || 'direct')}" /></div>
          <label class="checkbox-field"><input type="checkbox" name="ad_block"${policy.ad_block ? ' checked' : ''} /> Block managed ad domains</label>
          <div class="field full"><p class="field-hint">Runtime scope is managed with the Scope action in the group list.</p></div>
          <div class="field full inline-actions">
            <button class="primary-btn" id="accessGroupEditorSubmitBtn" type="submit"${serviceEnabled ? '' : ' disabled'}>${isEdit ? 'Save group' : 'Create group'}</button>
            <button class="secondary-btn" type="button" id="accessGroupEditorCancelBtn">Cancel</button>
          </div>
        </form>
        <div id="accessGroupEditorResult" class="form-result"></div>`, { size: 'large' });
      document.getElementById('accessGroupEditorCancelBtn')?.addEventListener('click', closeModal);
      bindAccessGroupEditorService();
      document.getElementById('accessGroupEditorForm')?.addEventListener('submit', (event) => submitAccessGroupEditor(event, group));
    }

    function renderAccessGroupServiceNotice(service) {
      const current = service || clientAccessServiceByCode('vless');
      if (serviceSupportsGroupManagement(current)) {
        return `
          <div class="callout ok">
            <strong>${escapeHTML(current?.display_name || 'Service')} groups are active</strong>
            <span>Membership, scope, preview and runtime materialization are available for this service.</span>
          </div>`;
      }
      return `
        <div class="callout warn">
          <strong>${escapeHTML(current?.display_name || 'Selected service')} groups are not active yet</strong>
          <span>${escapeHTML(current?.description || 'This service is visible in the catalog, but client access group materialization is not implemented yet.')}</span>
        </div>`;
    }

    function bindAccessGroupEditorService() {
      const serviceSelect = document.getElementById('accessGroupEditorServiceCode');
      const notice = document.getElementById('accessGroupServiceNotice');
      const submit = document.getElementById('accessGroupEditorSubmitBtn');
      const sync = () => {
        const service = clientAccessServiceByCode(serviceSelect?.value || 'vless');
        if (notice) notice.innerHTML = renderAccessGroupServiceNotice(service);
        if (submit) submit.disabled = !serviceSupportsGroupManagement(service);
      };
      serviceSelect?.addEventListener('change', sync);
      sync();
    }

    async function submitAccessGroupEditor(event, group) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const accessMode = String(form.get('access_mode') || 'instance_default');
      const policy = {
        access_mode: accessMode,
        egress_mode: accessMode === 'block' ? 'block' : (accessMode === 'egress_node' ? 'egress_node' : accessMode === 'local_breakout' ? 'local_breakout' : 'default'),
        egress_node_id: String(form.get('egress_node_id') || '').trim(),
        outbound_tag: String(form.get('outbound_tag') || 'direct').trim() || 'direct',
        ad_block: Boolean(form.get('ad_block')),
        rules: Boolean(form.get('ad_block')) ? [{ type: 'field', domain: ['geosite:category-ads-all'], outbound_tag: 'block' }] : [],
        extra_rules: [],
      };
      const payload = {
        service_code: normalizeClientAccessServiceCode(String(form.get('service_code') || 'vless')),
        group_key: String(form.get('group_key') || '').trim(),
        display_name: String(form.get('display_name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        status: String(form.get('status') || 'active'),
        policy_json: policy,
        scope_mode: group?.scope_mode || 'all_active_instances',
        auto_apply_new_instances: group?.auto_apply_new_instances === false ? false : true,
      };
      const target = document.getElementById('accessGroupEditorResult');
      if (target) target.innerHTML = '<span class="tag warn">saving</span>';
      try {
        const path = group ? `/api/v1/client-access-groups/${encodeURIComponent(group.id)}` : '/api/v1/client-access-groups';
        const method = group ? 'PATCH' : 'POST';
        const data = await sendJSON(path, method, payload);
        if (target) target.innerHTML = renderActionResponse(data, 'Client access group');
        await refresh();
        setTimeout(() => {
          closeModal();
          render();
        }, 350);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function isVLESSRuntimeInstance(instance) {
      const serviceCode = String(instance?.service_code || '').toLowerCase();
      if (!['xray-core', 'xray', 'vless'].includes(serviceCode)) return false;
      if (String(instance?.status || '').toLowerCase() === 'deleted') return false;
      const spec = instanceSpec(instance);
      const role = String(findNode(instance?.node_id)?.role || spec.role || '').toLowerCase();
      const profile = String(spec.profile || spec.driver_profile || spec.type || '').toLowerCase();
      const network = String(spec.network || spec.public_network || '').toLowerCase();
      return role !== 'egress' && (
        profile.includes('vless') ||
        profile.includes('reality') ||
        profile.includes('ws') ||
        network === 'ws' ||
        serviceCode === 'vless' ||
        serviceCode === 'xray-core'
      );
    }

    function accessGroupScopeInstances() {
      return (state.instances || [])
        .filter(isVLESSRuntimeInstance)
        .sort((left, right) => serviceInstanceLabel(left).localeCompare(serviceInstanceLabel(right), 'en'));
    }

    function selectedScopeInstanceIDs(scope) {
      const mode = String(scope?.scope_mode || 'all_active_instances');
      const source = mode === 'all_except_selected' ? scope?.exclude_instance_ids : scope?.include_instance_ids;
      return new Set((Array.isArray(source) ? source : []).map((value) => String(value || '').trim()).filter(Boolean));
    }

    function scopeModeText(mode) {
      switch (String(mode || 'all_active_instances')) {
        case 'selected_instances':
          return 'Selected instances only';
        case 'all_except_selected':
          return 'All active instances except selected';
        default:
          return 'All active instances';
      }
    }

    function openAccessGroupScopeModal(groupID) {
      const group = findAccessGroup(groupID);
      if (!group) return;
      openModal(`Scope: ${group.display_name || group.group_key}`, 'Choose runtime instances for this client group', `
        <section id="accessGroupScopePanel" data-group-id="${escapeHTML(group.id)}">
          <div id="accessGroupScopeBody" class="form-result"><span class="tag warn">loading scope</span></div>
        </section>`, { size: 'large' });
      void loadAccessGroupScope(group);
    }

    async function loadAccessGroupScope(group) {
      const target = document.getElementById('accessGroupScopeBody');
      try {
        const scope = await requestJSON(`/api/v1/client-access-groups/${encodeURIComponent(group.id)}/scope`);
        if (target) target.innerHTML = renderAccessGroupScopeForm(group, scope);
        bindAccessGroupScopeForm(group);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function renderAccessGroupScopeForm(group, scope = {}) {
      const instances = accessGroupScopeInstances();
      const mode = String(scope.scope_mode || group.scope_mode || 'all_active_instances');
      const selected = selectedScopeInstanceIDs(scope);
      const selectedCount = selected.size;
      const rows = instances.map((instance) => {
        const node = findNode(instance.node_id);
        const checked = selected.has(instance.id);
        return `
          <tr>
            <td>
              <label class="checkbox-field">
                <input class="access-group-scope-instance-check" type="checkbox" name="scope_instance_ids" value="${escapeHTML(instance.id)}"${checked ? ' checked' : ''}${mode === 'all_active_instances' ? ' disabled' : ''} />
                <span>
                  <strong>${escapeHTML(instance.name || instance.slug || instance.id)}</strong>
                  <small>${escapeHTML(compactServiceLabel(instance.service_code))} · ${escapeHTML(node?.name || instance.node_id || 'node')} · ${escapeHTML(node?.role || 'role n/a')}</small>
                </span>
              </label>
            </td>
            <td>${escapeHTML(endpointLabel(instance))}</td>
            <td>${statusTag(instance.status || 'unknown')}</td>
          </tr>`;
      }).join('') || '<tr><td colspan="3"><div class="empty compact-empty">No active VLESS ingress instances are loaded.</div></td></tr>';
      return `
        <form id="accessGroupScopeForm" class="form-grid">
          <div class="field">
            <label>Scope mode</label>
            <select id="accessGroupScopeMode" name="scope_mode">
              ${['all_active_instances','selected_instances','all_except_selected'].map((item) => `<option value="${escapeHTML(item)}"${mode === item ? ' selected' : ''}>${escapeHTML(scopeModeText(item))}</option>`).join('')}
            </select>
          </div>
          <label class="checkbox-field">
            <input type="checkbox" name="auto_apply_new_instances"${scope.auto_apply_new_instances === false ? '' : ' checked'} />
            Auto apply new matching instances
          </label>
          <div class="field full">
            <div class="callout">
              <strong>${escapeHTML(scopeModeText(mode))}</strong>
              <span>${escapeHTML(String(scope.affected_instances ?? group.affected_instances ?? 0))} affected now · ${escapeHTML(String(selectedCount))} selected override entries</span>
            </div>
          </div>
          <div class="field full">
            <label>Runtime instances</label>
            <div class="table-wrap">
              <table class="clients-table">
                <thead><tr><th>Instance</th><th>Endpoint</th><th>Status</th></tr></thead>
                <tbody>${rows}</tbody>
              </table>
            </div>
          </div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Save scope</button>
            <button class="secondary-btn" type="button" id="accessGroupScopeCancelBtn">Cancel</button>
          </div>
        </form>
        <div id="accessGroupScopeResult" class="form-result"></div>`;
    }

    function bindAccessGroupScopeForm(group) {
      const modeSelect = document.getElementById('accessGroupScopeMode');
      const syncDisabled = () => {
        const disabled = String(modeSelect?.value || 'all_active_instances') === 'all_active_instances';
        document.querySelectorAll('.access-group-scope-instance-check').forEach((input) => {
          input.disabled = disabled;
        });
      };
      modeSelect?.addEventListener('change', syncDisabled);
      syncDisabled();
      document.getElementById('accessGroupScopeCancelBtn')?.addEventListener('click', closeModal);
      document.getElementById('accessGroupScopeForm')?.addEventListener('submit', (event) => submitAccessGroupScope(event, group));
    }

    async function submitAccessGroupScope(event, group) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const mode = String(form.get('scope_mode') || 'all_active_instances');
      const selected = Array.from(document.querySelectorAll('.access-group-scope-instance-check:checked'))
        .map((input) => input.value)
        .filter(Boolean);
      const payload = {
        scope_mode: mode,
        auto_apply_new_instances: Boolean(form.get('auto_apply_new_instances')),
        include_instance_ids: mode === 'selected_instances' ? selected : [],
        exclude_instance_ids: mode === 'all_except_selected' ? selected : [],
      };
      const target = document.getElementById('accessGroupScopeResult');
      if (target) target.innerHTML = '<span class="tag warn">saving scope</span>';
      try {
        const data = await sendJSON(`/api/v1/client-access-groups/${encodeURIComponent(group.id)}/scope`, 'PATCH', payload);
        if (target) target.innerHTML = renderActionResponse(data, 'Client access group scope');
        await refresh();
        setTimeout(() => {
          closeModal();
          render();
        }, 350);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openAccessGroupMembersModal(groupID) {
      const group = findAccessGroup(groupID);
      if (!group) return;
      const service = clientAccessServiceByCode(group.service_code);
      if (!serviceSupportsGroupManagement(service)) {
        openActionOutcomeModal('Client access groups', 'Membership is not available', 'failed', `${compactServiceLabel(group.service_code)} groups are visible in the catalog, but runtime materialization is not implemented yet.`, [
          { label: 'Group', value: group.display_name || group.group_key },
          { label: 'Service', value: compactServiceLabel(group.service_code) },
          { label: 'Status', value: clientAccessServiceStatusLabel(service) },
        ]);
        return;
      }
      const groupLabel = group.display_name || group.group_key;
      openModal(`Assign clients: ${groupLabel}`, 'Load clients, preview membership changes, then apply them to the access group.', `
        <section id="clientAccessGroupMembersPanel" class="access-group-members-panel" data-group-id="${escapeHTML(group.id)}" data-offset="0" data-total="0" data-preview-ready="false">
          <div class="access-group-workflow-strip" aria-label="Client assignment workflow">
            <div class="access-group-step-card">
              <span>1</span>
              <strong>Find clients</strong>
              <small>Search, filter, or paste usernames, emails and IDs.</small>
            </div>
            <div class="access-group-step-card">
              <span>2</span>
              <strong>Select scope</strong>
              <small>Choose visible rows, all filtered clients, or pasted refs.</small>
            </div>
            <div class="access-group-step-card">
              <span>3</span>
              <strong>Preview and apply</strong>
              <small>Review create, move, skip and apply jobs before changing access.</small>
            </div>
          </div>
          <div class="access-group-members-toolbar">
            <div class="field">
              <label>Client search</label>
              <input id="accessGroupClientSearch" placeholder="username, email, client ID" autocomplete="off" />
            </div>
            <div class="field">
              <label>Assignment filter</label>
              <select id="accessGroupAssignmentFilter">
                <option value="unassigned">Unassigned to this service</option>
                <option value="assigned_to_group">Already in this group</option>
                <option value="assigned_other">Assigned to another group</option>
                <option value="all">All clients</option>
              </select>
            </div>
            <div class="field">
              <label>Status</label>
              <select id="accessGroupMemberStatus">
                <option value="">Any status</option>
                <option value="active">active</option>
                <option value="suspended">suspended</option>
                <option value="disabled">disabled</option>
              </select>
            </div>
            <div class="field">
              <label>Page size</label>
              <select id="accessGroupClientPageSize">
                <option value="25">25 clients</option>
                <option value="50" selected>50 clients</option>
                <option value="100">100 clients</option>
                <option value="250">250 clients</option>
              </select>
            </div>
            <div class="field access-group-load-field">
              <label>Load</label>
              <button class="secondary-btn" type="button" id="loadAccessGroupClientsBtn">Load clients</button>
            </div>
          </div>
          <div class="access-group-paste-panel">
            <div class="field">
              <label>Paste clients</label>
              <textarea id="accessGroupPasteClients" rows="4" placeholder="usernames, emails or client IDs separated by comma or new line"></textarea>
            </div>
            <div class="access-group-selection-card">
              <div>
                <strong>Selection</strong>
                <p id="accessGroupSelectionHint">Select rows, select all filtered clients, or paste client refs to enable preview.</p>
              </div>
              <div class="access-group-selection-actions">
                <label class="selection-toggle"><input type="checkbox" id="accessGroupSelectVisible" /> Select visible rows</label>
                <label class="selection-toggle"><input type="checkbox" id="accessGroupSelectAllFiltered" /> Select all filtered clients</label>
                <select id="accessGroupBulkMode" aria-label="Bulk mode">
                  <option value="add_only">Add only, do not move existing members</option>
                  <option value="add_or_move">Add or move existing members</option>
                </select>
                <button class="secondary-btn" type="button" id="clearAccessGroupSelectionBtn">Clear selection</button>
              </div>
            </div>
          </div>
          <div id="accessGroupMembersLoaded" class="table-wrap"></div>
          <div id="accessGroupMembersPreview" class="form-result"></div>
          <div class="access-group-members-footer">
            <div>
              <div id="accessGroupSelectionSummary" class="access-group-selection-summary">No clients selected</div>
              <div id="accessGroupFooterHint" class="access-group-footer-hint">Preview is required before access changes are applied.</div>
            </div>
            <div class="inline-actions">
              <button class="secondary-btn" type="button" id="previewAccessGroupBulkBtn" disabled>Preview changes</button>
              <button class="primary-btn" type="button" id="applyAccessGroupBulkBtn" disabled>Apply preview</button>
              <button class="secondary-btn" type="button" id="closeAccessGroupMembersBtn">Close</button>
            </div>
          </div>
        </section>`, { size: 'full', bodyClass: 'client-access-groups-modal' });
      bindAccessGroupMembersModal(group);
    }

    function bindAccessGroupMembersModal(group) {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      if (!panel) return;
      panel._selectedClientIDs = new Set();
      panel._loadedClientIDs = [];
      const lock = () => { state.vlessMembersInteractionLockUntil = Date.now() + 30000; };
      panel.addEventListener('input', lock);
      panel.addEventListener('focusin', lock);
      document.getElementById('closeAccessGroupMembersBtn')?.addEventListener('click', closeModal);
      document.getElementById('loadAccessGroupClientsBtn')?.addEventListener('click', () => loadAccessGroupClients(group, 0));
      document.getElementById('accessGroupClientPageSize')?.addEventListener('change', () => loadAccessGroupClients(group, 0));
      document.getElementById('accessGroupAssignmentFilter')?.addEventListener('change', () => {
        markAccessGroupPreviewStale();
        loadAccessGroupClients(group, 0);
      });
      document.getElementById('accessGroupMemberStatus')?.addEventListener('change', () => {
        markAccessGroupPreviewStale();
        loadAccessGroupClients(group, 0);
      });
      document.getElementById('accessGroupClientSearch')?.addEventListener('input', markAccessGroupPreviewStale);
      document.getElementById('accessGroupClientSearch')?.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') {
          event.preventDefault();
          loadAccessGroupClients(group, 0);
        } else {
          markAccessGroupPreviewStale();
        }
      });
      document.getElementById('accessGroupSelectVisible')?.addEventListener('change', (event) => {
        const selected = accessGroupSelectedClientIDs(panel);
        const checked = Boolean(event.currentTarget.checked);
        for (const clientID of panel._loadedClientIDs || []) {
          if (checked) selected.add(clientID);
          else selected.delete(clientID);
        }
        syncAccessGroupClientCheckboxes(panel);
        markAccessGroupPreviewStale();
      });
      document.getElementById('accessGroupSelectAllFiltered')?.addEventListener('change', () => {
        syncAccessGroupClientCheckboxes(panel);
        markAccessGroupPreviewStale();
      });
      document.getElementById('accessGroupPasteClients')?.addEventListener('input', () => {
        markAccessGroupPreviewStale();
        updateAccessGroupBulkControls();
      });
      document.getElementById('accessGroupBulkMode')?.addEventListener('change', markAccessGroupPreviewStale);
      document.getElementById('clearAccessGroupSelectionBtn')?.addEventListener('click', clearAccessGroupSelection);
      document.getElementById('previewAccessGroupBulkBtn')?.addEventListener('click', () => previewAccessGroupBulk(group));
      document.getElementById('applyAccessGroupBulkBtn')?.addEventListener('click', () => applyAccessGroupBulk(group));
      updateAccessGroupBulkControls();
      void loadAccessGroupClients(group, 0);
    }

    function accessGroupSelectedClientIDs(panel = document.getElementById('clientAccessGroupMembersPanel')) {
      if (!panel) return new Set();
      if (!(panel._selectedClientIDs instanceof Set)) panel._selectedClientIDs = new Set();
      return panel._selectedClientIDs;
    }

    function accessGroupPastedRefs() {
      return String(document.getElementById('accessGroupPasteClients')?.value || '')
        .split(/[\n,;\t]+/)
        .map((item) => item.trim())
        .filter(Boolean);
    }

    function accessGroupMembersPayload(group) {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      const selected = Array.from(accessGroupSelectedClientIDs(panel)).filter(Boolean);
      const refs = accessGroupPastedRefs();
      return {
        client_ids: selected,
        client_refs: refs,
        mode: String(document.getElementById('accessGroupBulkMode')?.value || 'add_only'),
        all_filtered: Boolean(document.getElementById('accessGroupSelectAllFiltered')?.checked),
        filter_group_id: group?.id || '',
        filter_search: String(document.getElementById('accessGroupClientSearch')?.value || '').trim(),
        filter_assignment: String(document.getElementById('accessGroupAssignmentFilter')?.value || 'unassigned'),
        filter_status: String(document.getElementById('accessGroupMemberStatus')?.value || '').trim(),
        queue_apply: true,
      };
    }

    function accessGroupHasSelectableInput() {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      return Boolean(
        document.getElementById('accessGroupSelectAllFiltered')?.checked ||
        accessGroupSelectedClientIDs(panel).size > 0 ||
        accessGroupPastedRefs().length > 0
      );
    }

    function updateAccessGroupBulkControls() {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      const selectedCount = accessGroupSelectedClientIDs(panel).size;
      const pastedCount = accessGroupPastedRefs().length;
      const allFiltered = Boolean(document.getElementById('accessGroupSelectAllFiltered')?.checked);
      const total = Number(panel?.dataset.total || 0);
      const preview = document.getElementById('previewAccessGroupBulkBtn');
      const apply = document.getElementById('applyAccessGroupBulkBtn');
      const summary = document.getElementById('accessGroupSelectionSummary');
      const hint = document.getElementById('accessGroupSelectionHint');
      const footerHint = document.getElementById('accessGroupFooterHint');
      const hasInput = accessGroupHasSelectableInput();
      if (preview) preview.disabled = !hasInput;
      if (apply) apply.disabled = panel?.dataset.previewReady !== 'true';
      if (summary) {
        const selectedLabel = allFiltered
          ? `All filtered clients${total > 0 ? ` (${total})` : ''}`
          : `${selectedCount} selected`;
        const pastedLabel = pastedCount > 0 ? ` · ${pastedCount} pasted` : '';
        summary.textContent = hasInput ? `${selectedLabel}${pastedLabel}` : 'No clients selected';
      }
      const message = !hasInput
        ? 'Select rows, select all filtered clients, or paste client refs to enable preview.'
        : panel?.dataset.previewReady === 'true'
          ? 'Preview is ready. Applying will materialize membership and queue bounded instance apply jobs.'
          : 'Selection changed. Run preview before applying access changes.';
      if (hint) hint.textContent = message;
      if (footerHint) {
        footerHint.textContent = panel?.dataset.previewReady === 'true'
          ? 'Review the preview result, then apply the prepared change.'
          : 'Preview is required before access changes are applied.';
      }
    }

    function syncAccessGroupClientCheckboxes(panel = document.getElementById('clientAccessGroupMembersPanel')) {
      if (!panel) return;
      const selected = accessGroupSelectedClientIDs(panel);
      const allFiltered = Boolean(document.getElementById('accessGroupSelectAllFiltered')?.checked);
      panel.querySelectorAll('input[name="access_group_client_ids"]').forEach((input) => {
        input.checked = selected.has(input.value);
        input.disabled = allFiltered;
      });
      const visible = document.getElementById('accessGroupSelectVisible');
      const loaded = panel._loadedClientIDs || [];
      if (visible) {
        visible.checked = loaded.length > 0 && loaded.every((clientID) => selected.has(clientID));
        visible.disabled = allFiltered || loaded.length === 0;
      }
      updateAccessGroupBulkControls();
    }

    function markAccessGroupPreviewStale() {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      if (panel) panel.dataset.previewReady = 'false';
      const apply = document.getElementById('applyAccessGroupBulkBtn');
      if (apply) apply.disabled = true;
      updateAccessGroupBulkControls();
    }

    function clearAccessGroupSelection() {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      accessGroupSelectedClientIDs(panel).clear();
      const visible = document.getElementById('accessGroupSelectVisible');
      const allFiltered = document.getElementById('accessGroupSelectAllFiltered');
      const paste = document.getElementById('accessGroupPasteClients');
      if (visible) visible.checked = false;
      if (allFiltered) allFiltered.checked = false;
      if (paste) paste.value = '';
      markAccessGroupPreviewStale();
      syncAccessGroupClientCheckboxes(panel);
    }

    async function loadAccessGroupClients(group, offset = 0) {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      const target = document.getElementById('accessGroupMembersLoaded');
      if (!panel || !target) return;
      panel.dataset.offset = String(offset);
      target.innerHTML = '<div class="empty compact-empty">Loading clients...</div>';
      markAccessGroupPreviewStale();
      const params = new URLSearchParams();
      params.set('service_code', group.service_code || 'vless');
      params.set('group_id', group.id);
      params.set('search', String(document.getElementById('accessGroupClientSearch')?.value || '').trim());
      params.set('assignment', String(document.getElementById('accessGroupAssignmentFilter')?.value || 'unassigned'));
      params.set('status', String(document.getElementById('accessGroupMemberStatus')?.value || '').trim());
      params.set('limit', String(document.getElementById('accessGroupClientPageSize')?.value || '50'));
      params.set('offset', String(offset));
      try {
        const data = await requestJSON(`/api/v1/client-access-groups/available-clients?${params.toString()}`);
        panel.dataset.total = String(Number(data?.total || 0));
        panel._loadedClientIDs = (Array.isArray(data?.items) ? data.items : []).map((item) => String(item.client_id || '')).filter(Boolean);
        target.innerHTML = renderAccessGroupAvailableClients(group, data);
        target.querySelectorAll('[data-page-offset]').forEach((button) => {
          button.addEventListener('click', () => loadAccessGroupClients(group, Number(button.dataset.pageOffset || 0)));
        });
        target.querySelectorAll('input[name="access_group_client_ids"]').forEach((input) => {
          input.addEventListener('change', () => {
            const selected = accessGroupSelectedClientIDs(panel);
            if (input.checked) selected.add(input.value);
            else selected.delete(input.value);
            markAccessGroupPreviewStale();
            syncAccessGroupClientCheckboxes(panel);
          });
        });
        syncAccessGroupClientCheckboxes(panel);
      } catch (err) {
        target.innerHTML = `<div class="empty compact-empty">${escapeHTML(err.message)}</div>`;
        panel.dataset.total = '0';
        panel._loadedClientIDs = [];
        syncAccessGroupClientCheckboxes(panel);
      }
    }

    function renderAccessGroupAvailableClients(group, data) {
      const items = Array.isArray(data?.items) ? data.items : [];
      const total = Number(data?.total || 0);
      const limit = Number(data?.limit || 50);
      const offset = Number(data?.offset || 0);
      const nextOffset = offset + limit;
      const prevOffset = Math.max(0, offset - limit);
      const selected = accessGroupSelectedClientIDs();
      const allFiltered = Boolean(document.getElementById('accessGroupSelectAllFiltered')?.checked);
      const rows = items.map((client) => `
        <tr>
          <td><label class="checkbox-field access-group-client-name"><input type="checkbox" name="access_group_client_ids" value="${escapeHTML(client.client_id)}"${selected.has(client.client_id) ? ' checked' : ''}${allFiltered ? ' disabled' : ''} /> <span>${escapeHTML(client.username || client.client_id)}</span></label></td>
          <td>${escapeHTML(client.email || '')}</td>
          <td>${statusTag(client.client_status || 'unknown')}</td>
          <td>${escapeHTML(client.group_name || client.group_key || 'unassigned')}</td>
        </tr>`).join('') || '<tr><td colspan="4"><div class="empty compact-empty access-group-empty-state">No clients for this filter.</div></td></tr>';
      const start = total === 0 ? 0 : offset + 1;
      const end = Math.min(offset + items.length, total);
      return `
        <table class="clients-table access-group-client-picker">
          <thead><tr><th>Client</th><th>Email</th><th>Status</th><th>Current group</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
        <div class="pagination-row">
          <span class="tag">${escapeHTML(String(start))}-${escapeHTML(String(end))} / ${escapeHTML(String(total))}</span>
          <button class="secondary-btn" type="button" data-page-offset="${escapeHTML(String(prevOffset))}"${offset <= 0 ? ' disabled' : ''}>Previous</button>
          <button class="secondary-btn" type="button" data-page-offset="${escapeHTML(String(nextOffset))}"${nextOffset >= total ? ' disabled' : ''}>Next</button>
        </div>`;
    }

    async function previewAccessGroupBulk(group) {
      const target = document.getElementById('accessGroupMembersPreview');
      const apply = document.getElementById('applyAccessGroupBulkBtn');
      if (!accessGroupHasSelectableInput()) {
        if (target) target.innerHTML = '<div class="callout warn"><strong>No clients selected</strong><span>Select rows, select all filtered clients, or paste client refs first.</span></div>';
        updateAccessGroupBulkControls();
        return;
      }
      if (target) target.innerHTML = '<span class="tag warn">previewing</span>';
      if (apply) apply.disabled = true;
      try {
        const payload = accessGroupMembersPayload(group);
        const data = await sendJSON(`/api/v1/client-access-groups/${encodeURIComponent(group.id)}/members:preview`, 'POST', payload);
        if (target) target.innerHTML = renderAccessGroupBulkPreview(data);
        const panel = document.getElementById('clientAccessGroupMembersPanel');
        if (panel) panel.dataset.previewReady = 'true';
        if (apply) apply.disabled = false;
        updateAccessGroupBulkControls();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        updateAccessGroupBulkControls();
      }
    }

    async function applyAccessGroupBulk(group) {
      const panel = document.getElementById('clientAccessGroupMembersPanel');
      const target = document.getElementById('accessGroupMembersPreview');
      if (panel?.dataset.previewReady !== 'true') {
        if (target) target.innerHTML = '<span class="tag danger">Preview the operation before applying it.</span>';
        return;
      }
      if (target) target.innerHTML = '<span class="tag warn">applying</span>';
      try {
        const payload = accessGroupMembersPayload(group);
        const data = await sendJSON(`/api/v1/client-access-groups/${encodeURIComponent(group.id)}/members:bulk-apply`, 'POST', payload);
        if (target) target.innerHTML = renderAccessGroupBulkPreview(data, true);
        const changed = Number(data?.created_memberships || 0) + Number(data?.moved_memberships || 0);
        if (changed > 0) {
          const assignment = document.getElementById('accessGroupAssignmentFilter');
          if (assignment) assignment.value = 'assigned_to_group';
        }
        clearAccessGroupSelection();
        await refresh();
        markAccessGroupPreviewStale();
        loadAccessGroupClients(group, 0);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function renderAccessGroupBulkPreview(data, applied = false) {
      const failed = Array.isArray(data?.failed) ? data.failed : [];
      const conflicts = Array.isArray(data?.conflicts) ? data.conflicts : [];
      const created = Number(data?.created_memberships || 0);
      const moved = Number(data?.moved_memberships || 0);
      const skipped = Number(data?.skipped_existing || 0);
      const jobs = Number(data?.apply_job_count || 0);
      return `
        ${applied ? `<div class="callout ok"><strong>Access group updated</strong><span>${escapeHTML(String(created + moved))} client membership changes applied. ${escapeHTML(String(jobs))} bounded instance apply job(s) queued.</span></div>` : ''}
        <div class="response-grid">
          <div><span>${applied ? 'Created' : 'Will create'}</span><strong>${escapeHTML(String(created))}</strong></div>
          <div><span>${applied ? 'Moved' : 'Will move'}</span><strong>${escapeHTML(String(moved))}</strong></div>
          <div><span>${applied ? 'Skipped' : 'Will skip'}</span><strong>${escapeHTML(String(skipped))}</strong></div>
          <div><span>${applied ? 'Failed' : 'Will fail'}</span><strong>${escapeHTML(String(failed.length))}</strong></div>
          <div><span>Instances</span><strong>${escapeHTML(String(data?.affected_instances || 0))}</strong></div>
          <div><span>Apply jobs</span><strong>${escapeHTML(String(jobs))}</strong></div>
        </div>
        ${conflicts.length ? `<div class="callout warn">${escapeHTML(String(conflicts.length))} existing memberships would be moved when bulk move is selected.</div>` : ''}
        ${failed.length ? `<pre class="technical-details">${escapeHTML(JSON.stringify(failed.slice(0, 20), null, 2))}</pre>` : ''}`;
    }

    function openAccessGroupSyncModal(groupID) {
      const group = findAccessGroup(groupID);
      if (!group) return;
      openModal(`Sync group: ${group.display_name || group.group_key}`, 'Materialize memberships to runtime instances', `
        <section class="form-grid">
          <div id="accessGroupSyncPreview" class="field full"><span class="tag warn">loading preview</span></div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="button" id="accessGroupApplySyncBtn">Apply sync</button>
            <button class="secondary-btn" type="button" id="accessGroupSyncCancelBtn">Cancel</button>
          </div>
        </section>`);
      document.getElementById('accessGroupSyncCancelBtn')?.addEventListener('click', closeModal);
      document.getElementById('accessGroupApplySyncBtn')?.addEventListener('click', () => applyAccessGroupSync(group));
      void previewAccessGroupSync(group);
    }

    async function previewAccessGroupSync(group) {
      const target = document.getElementById('accessGroupSyncPreview');
      try {
        const data = await sendJSON(`/api/v1/client-access-groups/${encodeURIComponent(group.id)}/sync:preview`, 'POST', {});
        if (target) target.innerHTML = renderActionResponse(data, 'Access group sync preview');
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function applyAccessGroupSync(group) {
      const target = document.getElementById('accessGroupSyncPreview');
      if (target) target.innerHTML = '<span class="tag warn">queueing sync</span>';
      try {
        const data = await sendJSON(`/api/v1/client-access-groups/${encodeURIComponent(group.id)}/sync:apply`, 'POST', {});
        if (target) target.innerHTML = renderActionResponse(data, 'Access group sync');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function render() {
      setTitle('Clients');
      if (state.clientsTab === 'groups') {
        renderClientGroupsPage();
        return;
      }
      const rows = state.clients || [];
      const activeClients = rows.filter((client) => clientLifecycleStatus(client) === 'active').length;
      const accessTotal = rows.reduce((sum, client) => sum + clientSummary(client).serviceAccessCount, 0);
      const readyArtifacts = rows.reduce((sum, client) => sum + clientSummary(client).readyArtifactCount, 0);
      const activeLinks = rows.reduce((sum, client) => sum + clientSummary(client).activeShareLinkCount, 0);
      el('content').innerHTML = `
        <section class="clients-workspace">
          ${renderClientWorkspaceTabs('clients')}
          <div class="client-summary-grid">
            <div class="client-summary-card">
              <span>Clients</span>
              <strong>${escapeHTML(String(rows.length))}</strong>
              <small>${escapeHTML(String(activeClients))} active</small>
            </div>
            <div class="client-summary-card">
              <span>Service access</span>
              <strong>${escapeHTML(String(accessTotal))}</strong>
              <small>bound instances</small>
            </div>
            <div class="client-summary-card">
              <span>Configs</span>
              <strong>${escapeHTML(String(readyArtifacts))}</strong>
              <small>ready artifacts</small>
            </div>
            <div class="client-summary-card">
              <span>Delivery</span>
              <strong>${escapeHTML(String(activeLinks))}</strong>
              <small>active share links</small>
            </div>
          </div>
          <section class="table-card clients-table-card">
            <div class="table-head">
              <div>
                <h2>Client provisioning</h2>
                <p class="table-subtitle">Bind service access, generate client configs and deliver artifacts from one workflow.</p>
              </div>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(rows.length))} loaded</span>
                <button class="secondary-btn" type="button" id="clientCreateBtn">Create client</button>
              </div>
            </div>
            <div class="table-wrap">
              <table class="clients-table">
                <thead>
                  <tr>
                    <th class="clients-col-client">Client</th>
                    <th class="clients-col-contact">Contact</th>
                    <th class="clients-col-provisioning">Provisioning</th>
                    <th class="clients-col-delivery">Delivery</th>
                    <th class="clients-col-state">State</th>
                    <th class="clients-col-actions">Actions</th>
                  </tr>
                </thead>
                <tbody>${renderClientRows(rows)}</tbody>
              </table>
            </div>
          </section>
        </section>`;
      bindClientWorkspaceTabs();
      bindListActions();
    }

    function renderClientRows(rows) {
      return rows.map((client) => {
        const summary = clientSummary(client);
        const lifecycle = clientLifecycleStatus(client);
        return `
          <tr>
            <td>
              <strong class="client-primary">${escapeHTML(client.username || client.id)}</strong>
              <div class="timeline-meta">${escapeHTML(client.display_name || client.id)}</div>
            </td>
            <td>
              <div>${escapeHTML(client.email || 'no email')}</div>
              <div class="timeline-meta">expires ${escapeHTML(formatDate(client.expires_at))}</div>
            </td>
            <td>
              <div class="client-status-cluster">
                <span class="tag">${escapeHTML(String(summary.serviceAccessCount))} access</span>
                <span class="tag ${summary.pendingServiceAccessCount > 0 ? 'warn' : 'stub'}">${escapeHTML(String(summary.pendingServiceAccessCount))} pending</span>
                <span class="tag">${escapeHTML(String(summary.routeCount))} routes</span>
              </div>
            </td>
            <td>
              <div class="client-status-cluster">
                <span class="tag ${summary.readyArtifactCount > 0 ? 'ok' : 'stub'}">${escapeHTML(String(summary.readyArtifactCount))} configs</span>
                <span class="tag ${summary.activeShareLinkCount > 0 ? 'ok' : 'stub'}">${escapeHTML(String(summary.activeShareLinkCount))} links</span>
              </div>
              <div class="timeline-meta">last build ${escapeHTML(formatDate(summary.lastArtifactAt))}</div>
            </td>
            <td>${statusTag(lifecycle)}<div class="timeline-meta">updated ${escapeHTML(formatDate(client.updated_at))}</div></td>
            <td>${renderActionButtons(client)}</td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No client accounts yet. Create a client, bind service access and build configs.</div></td></tr>';
    }

    function openCreateClientModal() {
      openModal('Create client', 'POST /api/v1/clients', `
        <form id="createClientForm" class="form-grid">
          <div class="field"><label>Username</label><input name="username" required placeholder="client-01" /></div>
          <div class="field"><label>Display name</label><input name="display_name" placeholder="Client 01" /></div>
          <div class="field"><label>Email</label><input name="email" type="email" placeholder="client@example.com" /></div>
          <div class="field"><label>Expires at</label><input name="expires_at" type="datetime-local" /></div>
          <div class="field full"><label>Notes</label><textarea name="notes" rows="5" placeholder="Optional notes, contract reference or operator comment."></textarea></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Create client</button></div>
        </form>
        <div id="createClientResult" class="form-result"></div>`);
      document.getElementById('createClientForm')?.addEventListener('submit', createClient);
    }

    async function createClient(event) {
      event.preventDefault();
      const target = document.getElementById('createClientResult');
      if (target) target.innerHTML = '<span class="tag warn">creating</span>';
      try {
        const form = new FormData(event.currentTarget);
        const expiresAtRaw = String(form.get('expires_at') || '').trim();
        const payload = {
          username: String(form.get('username') || '').trim(),
          display_name: String(form.get('display_name') || '').trim(),
          email: String(form.get('email') || '').trim(),
          notes: String(form.get('notes') || '').trim(),
        };
        if (expiresAtRaw) payload.expires_at = new Date(expiresAtRaw).toISOString();
        const data = await sendJSON('/api/v1/clients', 'POST', payload);
        if (target) target.innerHTML = renderActionResponse(data, 'Client creation');
        await refresh();
        setTimeout(closeModal, 400);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openClientEmailModal(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Send access email: ${client.username}`, 'Client delivery', `
        <form id="clientEmailForm" class="form-grid">
          <div class="field"><label>Email</label><input value="${escapeHTML(client.email || '')}" disabled /></div>
          <div class="field"><label>Share link TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="72" /></div>
          <div class="field full"><label>Subject</label><input name="subject" value="RTIS MegaVPN access package" /></div>
          <div class="field full"><label>Message</label><textarea name="message" rows="5" placeholder="Optional custom note for the client."></textarea></div>
          <div class="field"><label>Create/refresh share link</label><select name="create_share_link"><option value="true">true</option><option value="false">false</option></select></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Send email</button></div>
        </form>
        <div id="clientEmailResult" class="form-result"></div>`);
      document.getElementById('clientEmailForm')?.addEventListener('submit', (event) => sendClientEmail(event, clientID));
    }

    async function sendClientEmail(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientEmailResult');
      if (target) target.innerHTML = '<span class="tag warn">sending</span>';
      try {
        const form = new FormData(event.currentTarget);
        const data = await sendJSON(`/api/v1/clients/${clientID}/deliver-email`, 'POST', {
          subject: String(form.get('subject') || '').trim(),
          message: String(form.get('message') || '').trim(),
          ttl_hours: Number(form.get('ttl_hours') || 72),
          create_share_link: String(form.get('create_share_link')) === 'true',
        });
        if (target) target.innerHTML = renderActionResponse(data, 'Client email delivery');
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function queueClientProvision(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      const instances = provisionableClientInstances();
      openModal(`Provision client: ${client.username}`, 'Create service access and queue config generation', `
        <form id="clientProvisionForm" class="client-provision-form form-grid">
          <div class="client-workflow-strip">
            <div><span>1</span><strong>Access</strong><small>Bind the client to selected instances.</small></div>
            <div><span>2</span><strong>Secrets</strong><small>Driver state generates certificates, keys or passwords.</small></div>
            <div><span>3</span><strong>Apply</strong><small>Changed instances are queued for agent apply.</small></div>
            <div><span>4</span><strong>Configs</strong><small>Client artifacts are stored for preview/download.</small></div>
          </div>
          <div class="field full">
            <label>Service instances</label>
            <div class="client-provision-choice-grid" id="clientProvisionInstances">${renderProvisionInstanceChoices(instances)}</div>
          </div>
          <div class="field full client-provision-actions">
            <button class="primary-btn" type="submit"${instances.length ? '' : ' disabled'}>Queue provisioning</button>
            <button class="secondary-btn" id="cancelProvisionBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="clientProvisionResult" class="form-result"></div>`, { size: 'full', bodyClass: 'client-provision-modal' });
      document.getElementById('cancelProvisionBtn')?.addEventListener('click', closeModal);
      document.getElementById('clientProvisionForm')?.addEventListener('submit', (event) => submitClientProvision(event, clientID));
      document.querySelectorAll('.client-vless-group-select').forEach((select) => {
        ['click', 'mousedown', 'mouseup', 'keydown'].forEach((eventName) => {
          select.addEventListener(eventName, (event) => event.stopPropagation());
        });
      });
    }

    function renderProvisionInstanceChoices(instances = []) {
      if (!instances.length) {
        return '<div class="empty compact-empty">No provisionable service instances. Create and apply a service instance first.</div>';
      }
      return instances.map((instance) => {
        const node = findNode(instance.node_id);
        const runtime = (state.instanceRuntimeStates || []).find((item) => item.instance_id === instance.id);
        const runtimeStatus = runtime?.runtime_status || instance.status || 'unknown';
        const healthStatus = runtime?.health_status || 'unknown';
        return `
          <label class="client-provision-choice">
            <input type="checkbox" name="instance_ids" value="${escapeHTML(instance.id)}" />
            <span class="client-choice-check" aria-hidden="true"></span>
            <span class="client-choice-body">
              <strong>${escapeHTML(instance.name || instance.slug || instance.id)}</strong>
              <small>${escapeHTML(compactServiceLabel(instance.service_code))} · ${escapeHTML(node?.name || instance.node_id || 'node')} · ${escapeHTML(node?.role || 'role n/a')}</small>
              <em>${escapeHTML(endpointLabel(instance))}</em>
              ${renderVLESSGroupSelect(instance)}
              <span class="client-choice-selected">Selected for client</span>
            </span>
            <span class="client-choice-tags">${statusTag(runtimeStatus)}${statusTag(healthStatus)}</span>
          </label>`;
      }).join('');
    }

    async function submitClientProvision(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientProvisionResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing</span>';
      const formEl = event.currentTarget;
      const submitButton = formEl.querySelector('button[type="submit"]');
      if (submitButton) {
        submitButton.disabled = true;
        submitButton.textContent = 'Queueing provisioning';
      }
      const instanceIDs = Array.from(event.currentTarget.querySelector('#clientProvisionInstances')?.selectedOptions || [])
        .map((option) => option.value)
        .filter(Boolean);
      const checkboxIDs = Array.from(formEl.querySelectorAll('input[name="instance_ids"]:checked') || [])
        .map((input) => input.value)
        .filter(Boolean);
      const selectedIDs = checkboxIDs.length ? checkboxIDs : instanceIDs;
      if (selectedIDs.length === 0) {
        if (target) target.innerHTML = '<span class="tag danger">Select at least one service instance</span>';
        if (submitButton) {
          submitButton.disabled = false;
          submitButton.textContent = 'Queue provisioning';
        }
        return;
      }
      try {
        const selectedSet = new Set(selectedIDs);
        const serviceOptions = {};
        formEl.querySelectorAll('.client-vless-group-select').forEach((select) => {
          const instanceID = String(select.dataset.instanceId || '').trim();
          const group = normalizeVLESSGroupKey(select.value);
          if (instanceID && group && selectedSet.has(instanceID)) {
            serviceOptions[instanceID] = { vless_group: group };
          }
        });
        const payload = { instance_ids: selectedIDs };
        if (Object.keys(serviceOptions).length) payload.service_options = serviceOptions;
        const data = await sendJSON(`/api/v1/clients/${clientID}/provision`, 'POST', payload);
        if (target) target.innerHTML = '';
        const client = findClient(clientID);
        const modalBody = el('modalBody');
        if (modalBody) {
          modalBody.className = 'modal-body client-provision-modal';
          modalBody.innerHTML = renderProvisionQueuedState(client, selectedIDs, payload, data);
          window.MegaVPNFormEnhancer?.enhance?.(modalBody);
        }
        document.getElementById('clientProvisionCloseBtn')?.addEventListener('click', closeModal);
        document.getElementById('clientProvisionAccessBtn')?.addEventListener('click', async () => {
          await refresh();
          await openClientAccessesModal(clientID);
        });
        document.getElementById('clientProvisionJobsBtn')?.addEventListener('click', async () => {
          closeModal();
          if (typeof setPage === 'function') {
            setPage('jobs');
          } else {
            state.page = 'jobs';
          }
          await refresh();
        });
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        if (submitButton) {
          submitButton.disabled = false;
          submitButton.textContent = 'Queue provisioning';
        }
      }
    }

    function renderProvisionQueuedState(client, selectedIDs, payload, job) {
      const selected = selectedIDs.map((id) => findInstance(id)).filter(Boolean);
      const serviceRows = selected.map((instance) => {
        const node = findNode(instance.node_id);
        const group = payload?.service_options?.[instance.id]?.vless_group || '';
        const endpointInfo = servicePublicEndpointInfo(instance, payload?.service_options?.[instance.id] || {}, {});
        return `
          <div class="client-provision-result-service">
            <strong>${escapeHTML(instance.name || instance.slug || instance.id)}</strong>
            <span>${escapeHTML(compactServiceLabel(instance.service_code))} · ${escapeHTML(node?.name || instance.node_id || 'node')} · ${escapeHTML(node?.role || 'role n/a')}</span>
            <code>${escapeHTML(endpointInfo.endpoint)}</code>
            ${endpointInfo.backendEndpoint ? `<em>backend: ${escapeHTML(endpointInfo.backendEndpoint)}</em>` : ''}
            ${group ? `<em>VLESS group: ${escapeHTML(group)}</em>` : ''}
          </div>`;
      }).join('');
      const jobID = job?.id || job?.job_id || '';
      return `
        <section class="client-provision-queued">
          <div class="client-provision-queued-head">
            <div>
              <span class="mini-label">Provisioning queued</span>
              <h3>${escapeHTML(clientDisplayName(client || {}))}</h3>
              <p>Service access is stored. A worker job will generate secrets, apply affected instances and publish client artifacts.</p>
            </div>
            ${statusTag(job?.status || 'queued')}
          </div>
          <div class="client-provision-result-grid">
            <div><span>Job</span><strong>${escapeHTML(jobID || 'queued')}</strong></div>
            <div><span>Type</span><strong>${escapeHTML(job?.type || 'client.provision')}</strong></div>
            <div><span>Services</span><strong>${escapeHTML(String(selected.length))}</strong></div>
          </div>
          <div class="client-provision-service-list">${serviceRows || '<div class="empty compact-empty">Selected instances will appear after refresh.</div>'}</div>
          <div class="client-provision-next">
            <div><span>1</span><strong>Worker</strong><small>Job moves from queued to running.</small></div>
            <div><span>2</span><strong>Secrets</strong><small>UUIDs, keys and certificates are generated.</small></div>
            <div><span>3</span><strong>Apply</strong><small>Affected instances are queued for agent apply.</small></div>
            <div><span>4</span><strong>Configs</strong><small>Artifacts appear after the worker succeeds.</small></div>
          </div>
          <div class="field full client-provision-actions client-provision-actions-success">
            <button class="primary-btn" type="button" id="clientProvisionJobsBtn">Jobs</button>
            <button class="secondary-btn" type="button" id="clientProvisionAccessBtn">Access</button>
            <button class="secondary-btn" type="button" id="clientProvisionCloseBtn">Close</button>
          </div>
        </section>`;
    }

    function renderServiceAccessRows(accessList, clientID) {
      return accessList.map((access) => {
        const inbound = serviceAccessInboundInfo(access);
        return `
          <tr>
            <td>
              <strong>${escapeHTML(inbound.serviceLabel)}</strong>
              <div class="timeline-meta">${escapeHTML(inbound.instanceName)}</div>
              ${inbound.outboundGroup ? `<div class="timeline-meta">outbound group: ${escapeHTML(inbound.outboundGroup)}</div>` : ''}
            </td>
            <td><span class="tag">${escapeHTML(compactServiceLabel(inbound.serviceCode))}</span></td>
            <td>
              <strong>${escapeHTML(inbound.nodeName)}</strong>
              <div class="timeline-meta">${escapeHTML(inbound.nodeRole)}${inbound.nodeAddress ? ` · ${escapeHTML(inbound.nodeAddress)}` : ''}</div>
            </td>
            <td>
              <code>${escapeHTML(inbound.endpoint)}</code>
              ${inbound.endpointKind === 'public' ? '<div class="timeline-meta">public client endpoint</div>' : ''}
              ${inbound.backendEndpoint ? `<div class="timeline-meta">backend ${escapeHTML(inbound.backendEndpoint)}</div>` : ''}
              ${inbound.transportLabel ? `<div class="timeline-meta">${escapeHTML(inbound.transportLabel)}</div>` : ''}
            </td>
            <td><div class="client-status-cluster">${statusTag(access.status || 'unknown')}${statusTag(inbound.availability || 'available')}</div></td>
            <td>${renderServiceAccessActions(clientID, access.id, inbound.serviceCode)}</td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No service accesses for this client.</div></td></tr>';
    }

    function renderServiceAccessActions(clientID, accessID, serviceCode) {
      const actions = [
        ['openvpn', 'openvpn', 'Rotate OpenVPN'],
        ['xray-core', 'xray-core', 'Rotate Xray UUID'],
        ['xray', 'xray-core', 'Rotate Xray UUID'],
        ['wireguard', 'wireguard', 'Rotate WireGuard Keys'],
        ['mtproto', 'mtproto', 'Rotate MTProto Secret'],
        ['ipsec', 'ipsec', 'Rotate L2TP Access'],
        ['http_proxy', 'http_proxy', 'Rotate Proxy Access'],
        ['shadowsocks', 'shadowsocks', 'Rotate SS Access'],
      ].filter(([code]) => code === serviceCode);
      return `
        <div class="inline-actions compact-actions">
          ${actions.map(([, driver, label]) => `<button class="secondary-btn rotate-access-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(accessID)}" data-driver="${escapeHTML(driver)}">${escapeHTML(label)}</button>`).join('')}
          <button class="danger-btn client-access-delete-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-access-id="${escapeHTML(accessID)}">Remove access</button>
        </div>`;
    }

    function renderRouteRows(routeList, client, clientID) {
      return routeList.map((route) => {
        const instance = findInstance(route.instance_id);
        const node = findNode(route.node_id);
        const managed = route.metadata?.baseline === true || route.metadata?.baseline === 'true';
        const inbound = route.metadata?.inbound || route.metadata?.inbound_service || {};
        const serviceLabel = inbound.service_label || compactServiceLabel(instance?.service_code || '');
        return `
          <tr>
            <td><strong>${escapeHTML(route.name || route.destination)}</strong><div class="timeline-meta">${managed ? 'managed baseline' : 'manual policy'}</div></td>
            <td>${escapeHTML(client.username || client.display_name || client.id)}</td>
            <td><strong>${escapeHTML(serviceLabel)}</strong><div class="timeline-meta">${escapeHTML(instance?.name || route.instance_id || 'global')}</div></td>
            <td>${escapeHTML(node?.name || route.node_id || 'n/a')}</td>
            <td><span class="tag">${escapeHTML(route.destination_type || 'endpoint')}</span> ${escapeHTML(route.destination || 'n/a')}</td>
            <td>${escapeHTML(route.protocol || 'any')} / ${escapeHTML(route.ports || '*')}</td>
            <td>${renderRouteEgress(route)}</td>
            <td>${statusTag(route.status || 'unknown')}</td>
            <td>
              <div class="inline-actions compact-actions">
                ${managed ? '<span class="tag">managed</span>' : `<button class="danger-btn client-route-delete-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-route-id="${escapeHTML(route.id)}">Revoke</button>`}
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="9"><div class="empty">No access routes yet. Provision the client or add a manual route.</div></td></tr>';
    }

    function renderRouteEgress(route) {
      if (route?.metadata?.baseline === true || route?.metadata?.baseline === 'true') {
        return '<span class="tag">service default</span>';
      }
      const policy = route?.policy || {};
      const egress = policy.egress || {};
      const mode = String(egress.mode || policy.egress_mode || policy.mode || 'auto').trim() || 'auto';
      const nodeID = String(egress.node_id || policy.egress_node_id || policy.node_id || '').trim();
      const node = findNode(nodeID);
      const nextHop = String(egress.next_hop || policy.egress_next_hop || '').trim();
      const iface = String(egress.interface || policy.egress_interface || '').trim();
      if (mode === 'egress_node' || mode === 'remote_node' || mode === 'node') {
        return `
          <div><span class="tag">egress</span> ${escapeHTML(node?.name || nodeID || 'not selected')}</div>
          <div class="timeline-meta">${escapeHTML(nextHop || iface || 'backhaul not set')}</div>`;
      }
      if (mode === 'local_breakout' || mode === 'local' || mode === 'direct') {
        return '<span class="tag ok">local breakout</span>';
      }
      return '<span class="tag warn">egress not selected</span>';
    }

    function renderClientArtifactRows(artifactList, accessList, clientID) {
      const instanceByAccess = new Map();
      (accessList || []).forEach((access) => {
        const instance = findInstance(access.instance_id);
        if (access.id && instance) instanceByAccess.set(access.id, instance);
      });
      return (artifactList || []).map((artifact) => {
        const instance = instanceByAccess.get(artifact.service_access_id || '');
        const canPreview = artifactPreviewable(artifact.artifact_type);
        return `
          <tr>
            <td><span class="tag">${escapeHTML(artifactTypeLabel(artifact.artifact_type))}</span></td>
            <td>${escapeHTML(instance?.name || artifact.service_access_id || 'shared')}</td>
            <td>${escapeHTML(String(artifact.size_bytes || 0))} B</td>
            <td>${statusTag(artifact.status || 'unknown')}</td>
            <td>${escapeHTML(formatDate(artifact.created_at))}</td>
            <td>
              <div class="inline-actions compact-actions">
                ${canPreview ? `<button class="secondary-btn client-artifact-preview-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-artifact-id="${escapeHTML(artifact.id)}">Preview</button>` : ''}
                <button class="secondary-btn client-artifact-download-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-artifact-id="${escapeHTML(artifact.id)}">Download</button>
                <button class="danger-btn client-artifact-delete-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-artifact-id="${escapeHTML(artifact.id)}">Delete</button>
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No generated configs yet. Build client configs after service access is created.</div></td></tr>';
    }

    function renderClientShareLinkRows(shareLinkList, artifactList, clientID) {
      const artifactByID = new Map((artifactList || []).map((artifact) => [artifact.id, artifact]));
      return (shareLinkList || []).map((link) => {
        const artifact = artifactByID.get(link.target_id || '');
        return `
          <tr>
            <td><span class="tag">${escapeHTML(link.token_hint || 'hidden')}</span></td>
            <td>${escapeHTML(artifact ? artifactTypeLabel(artifact.artifact_type) : link.target_id || 'artifact')}</td>
            <td>${statusTag(shareLinkDisplayStatus(link))}</td>
            <td>${escapeHTML(formatDate(link.expires_at))}</td>
            <td>${escapeHTML(String(link.download_count || 0))}</td>
            <td>
              <div class="inline-actions compact-actions">
                <button class="danger-btn client-share-revoke-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-link-id="${escapeHTML(link.id)}">Revoke</button>
              </div>
            </td>
          </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No delivery links yet. Publish a link after a config artifact is ready.</div></td></tr>';
    }

    function renderClientSubscriptionRows(subscriptionList, clientID) {
      return (subscriptionList || []).map((subscription) => {
        const displayStatus = subscriptionDisplayStatus(subscription);
        const revoked = displayStatus === 'revoked';
        return `
        <tr>
          <td><span class="tag">${escapeHTML(subscription.token_hint || 'hidden')}</span></td>
          <td>${statusTag(displayStatus)}</td>
          <td>${escapeHTML(formatDate(subscription.expires_at))}</td>
          <td>${escapeHTML(subscription.last_used_at ? formatDate(subscription.last_used_at) : 'never')}</td>
          <td>${escapeHTML(String(subscription.download_count || 0))}</td>
          <td>
            <div class="inline-actions compact-actions">
              <button class="danger-btn client-subscription-revoke-btn" type="button" data-client-id="${escapeHTML(clientID)}" data-subscription-id="${escapeHTML(subscription.id)}"${revoked ? ' disabled' : ''}>${revoked ? 'Revoked' : 'Revoke'}</button>
            </div>
          </td>
        </tr>`;
      }).join('') || '<tr><td colspan="6"><div class="empty">No VLESS subscription token yet. Rotate a token and copy the generated URL once.</div></td></tr>';
    }

    function renderClientAccessOverview(client, accessList, routeList, artifactList, shareLinkList, subscriptionList = []) {
      const activeAccesses = accessList.filter((item) => String(item.status || '').toLowerCase() === 'active').length;
      const pendingAccesses = accessList.filter((item) => String(item.status || '').toLowerCase() === 'pending').length;
      const readyArtifacts = artifactList.filter((item) => String(item.status || '').toLowerCase() === 'ready').length;
      const activeLinks = shareLinkList.filter(shareLinkIsUsable).length;
      const activeSubscriptions = subscriptionList.filter((item) => subscriptionDisplayStatus(item) === 'active').length;
      return `
        <div class="client-access-overview">
          <div>
            <span>Client</span>
            <strong>${escapeHTML(clientDisplayName(client))}</strong>
            <small>${escapeHTML(client.email || 'no email')}</small>
          </div>
          <div>
            <span>Access</span>
            <strong>${escapeHTML(String(accessList.length))}</strong>
            <small>${escapeHTML(String(activeAccesses))} active · ${escapeHTML(String(pendingAccesses))} pending</small>
          </div>
          <div>
            <span>Routes</span>
            <strong>${escapeHTML(String(routeList.length))}</strong>
            <small>policy rows</small>
          </div>
          <div>
            <span>Configs</span>
            <strong>${escapeHTML(String(readyArtifacts))}</strong>
            <small>${escapeHTML(String(artifactList.length))} total artifacts</small>
          </div>
          <div>
            <span>Delivery</span>
            <strong>${escapeHTML(String(activeLinks + activeSubscriptions))}</strong>
            <small>${escapeHTML(String(activeLinks))} links · ${escapeHTML(String(activeSubscriptions))} subscriptions</small>
          </div>
        </div>`;
    }

    function renderInboundServiceCards(accessList = []) {
      if (!accessList.length) {
        return '<div class="empty compact-empty">No inbound services are available to this client yet. Run provisioning and select service instances.</div>';
      }
      return `
        <div class="client-inbound-grid">
          ${accessList.map((access) => {
            const inbound = serviceAccessInboundInfo(access);
            return `
              <article class="client-inbound-card">
                <div class="client-inbound-card-head">
                  <div>
                    <strong>${escapeHTML(inbound.serviceLabel)}</strong>
                    <small>${escapeHTML(inbound.instanceName)}</small>
                  </div>
                  <span class="client-inbound-included"><i aria-hidden="true"></i>Included</span>
                </div>
                <div class="client-inbound-meta">
                  <span>${escapeHTML(inbound.nodeName)} · ${escapeHTML(inbound.nodeRole)}</span>
                  <code>${escapeHTML(inbound.endpoint)}</code>
                  ${inbound.backendEndpoint ? `<span>backend ${escapeHTML(inbound.backendEndpoint)}</span>` : ''}
                  ${inbound.transportLabel ? `<span>${escapeHTML(inbound.transportLabel)}</span>` : ''}
                </div>
                <div class="client-status-cluster">
                  ${statusTag(access.status || 'unknown')}
                  ${statusTag(inbound.availability || (inbound.available ? 'available' : 'disabled'))}
                </div>
              </article>`;
          }).join('')}
        </div>`;
    }

    async function openClientAccessesModal(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Client access: ${client.username}`, 'Provisioned service bindings and route policy', '<div class="empty">Loading service accesses...</div>', { wide: true });
      try {
        const [accesses, routes, artifacts, shareLinks, subscriptions] = await Promise.all([
          requestJSON(`/api/v1/clients/${clientID}/accesses`),
          requestJSON(`/api/v1/clients/${clientID}/routes`),
          requestJSON(`/api/v1/clients/${clientID}/artifacts`),
          requestJSON(`/api/v1/clients/${clientID}/share-links`),
          requestJSON(`/api/v1/clients/${clientID}/subscriptions`),
        ]);
        const accessList = Array.isArray(accesses) ? accesses : [];
        const routeList = Array.isArray(routes) ? routes : [];
        const artifactList = Array.isArray(artifacts) ? artifacts : [];
        const shareLinkList = Array.isArray(shareLinks) ? shareLinks : [];
        const subscriptionList = Array.isArray(subscriptions) ? subscriptions : [];
        const accessOptions = accessList.map((access) => {
          const inbound = serviceAccessInboundInfo(access);
          return `<option value="${escapeHTML(access.id)}">${escapeHTML(inbound.serviceLabel)} - ${escapeHTML(inbound.endpoint)} - ${escapeHTML(access.status || 'unknown')}</option>`;
        }).join('');
        el('modalBody').innerHTML = `
          <div id="clientAccessRotateResult" class="form-result"></div>
          ${renderClientAccessOverview(client, accessList, routeList, artifactList, shareLinkList, subscriptionList)}
          <section class="table-card compact-card client-inbound-section">
            <div class="table-head">
              <div>
                <h2>Inbound services</h2>
                <p class="table-subtitle">Service entrypoints this client is allowed to use after provisioning.</p>
              </div>
              <span class="tag">${escapeHTML(String(accessList.length))}</span>
            </div>
            ${renderInboundServiceCards(accessList)}
          </section>
          <section class="table-card compact-card">
            <div class="table-head"><h2>Service Accesses</h2><span class="tag">${escapeHTML(String(accessList.length))}</span></div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Inbound</th><th>Service</th><th>Node</th><th>Endpoint</th><th>Status</th><th>Actions</th></tr></thead>
                <tbody>${renderServiceAccessRows(accessList, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>Access Routes</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(routeList.length))} routes</span>
                <button class="secondary-btn" id="clientRouteAddBtn" type="button">Add route</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Route</th><th>User</th><th>Instance</th><th>Node</th><th>Destination</th><th>Protocol</th><th>Egress</th><th>Status</th><th>Actions</th></tr></thead>
                <tbody>${renderRouteRows(routeList, client, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>Client Configs</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(artifactList.length))} files</span>
                <button class="secondary-btn" id="clientArtifactBuildBtn" type="button">Build configs</button>
                <button class="secondary-btn" id="clientSharePublishBtn" type="button">Publish link</button>
                <button class="secondary-btn" id="clientManageEmailBtn" type="button">Email</button>
                <button class="danger-btn" id="clientConfigsClearBtn" type="button"${artifactList.length || shareLinkList.length || subscriptionList.length ? '' : ' disabled'}>Clear configs</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Type</th><th>Instance</th><th>Size</th><th>Status</th><th>Created</th><th>Actions</th></tr></thead>
                <tbody>${renderClientArtifactRows(artifactList, accessList, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>VLESS Subscription</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(subscriptionList.length))} tokens</span>
                <button class="secondary-btn" id="clientSubscriptionRotateBtn" type="button">Rotate subscription</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Token</th><th>Status</th><th>Expires</th><th>Last used</th><th>Downloads</th><th>Actions</th></tr></thead>
                <tbody>${renderClientSubscriptionRows(subscriptionList, clientID)}</tbody>
              </table>
            </div>
          </section>
          <section class="table-card compact-card" style="margin-top:16px">
            <div class="table-head">
              <h2>Delivery Links</h2>
              <div class="table-tools">
                <span class="tag">${escapeHTML(String(shareLinkList.length))} links</span>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Token</th><th>Artifact</th><th>Status</th><th>Expires</th><th>Downloads</th><th>Actions</th></tr></thead>
                <tbody>${renderClientShareLinkRows(shareLinkList, artifactList, clientID)}</tbody>
              </table>
            </div>
          </section>`;
        bindAccessModalActions(clientID, accessOptions, accessList, artifactList, shareLinkList, subscriptionList);
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load service accesses: ${escapeHTML(err.message)}</div>`;
      }
    }

    function bindAccessModalActions(clientID, accessOptions, accessList = [], artifactList = [], shareLinkList = [], subscriptionList = []) {
      document.querySelectorAll('.rotate-access-btn').forEach((button) => {
        button.addEventListener('click', () => rotateClientAccess(button.dataset.clientId, button.dataset.accessId, button.dataset.driver));
      });
      document.querySelectorAll('.client-access-delete-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteServiceAccessModal(button.dataset.clientId, button.dataset.accessId, accessList));
      });
      document.getElementById('clientRouteAddBtn')?.addEventListener('click', () => openCreateClientRouteModal(clientID, accessOptions));
      document.querySelectorAll('.client-route-delete-btn').forEach((button) => {
        button.addEventListener('click', () => revokeClientAccessRoute(button.dataset.clientId, button.dataset.routeId));
      });
      document.getElementById('clientArtifactBuildBtn')?.addEventListener('click', () => openBuildClientArtifactsModal(clientID, accessList));
      document.getElementById('clientSharePublishBtn')?.addEventListener('click', () => openPublishShareLinkModal(clientID, artifactList));
      document.getElementById('clientManageEmailBtn')?.addEventListener('click', () => openClientEmailModal(clientID));
      document.getElementById('clientConfigsClearBtn')?.addEventListener('click', () => openClearClientConfigsModal(clientID, artifactList, shareLinkList, subscriptionList));
      document.getElementById('clientSubscriptionRotateBtn')?.addEventListener('click', () => openRotateClientSubscriptionModal(clientID));
      document.querySelectorAll('.client-share-revoke-btn').forEach((button) => {
        button.addEventListener('click', () => revokeClientShareLink(button.dataset.clientId, button.dataset.linkId));
      });
      document.querySelectorAll('.client-subscription-revoke-btn:not([disabled])').forEach((button) => {
        button.addEventListener('click', () => revokeClientSubscription(button.dataset.clientId, button.dataset.subscriptionId));
      });
      document.querySelectorAll('.client-artifact-preview-btn').forEach((button) => {
        button.addEventListener('click', () => previewClientArtifact(button.dataset.clientId, button.dataset.artifactId));
      });
      document.querySelectorAll('.client-artifact-download-btn').forEach((button) => {
        button.addEventListener('click', () => {
          const url = artifactDownloadURL(button.dataset.clientId, button.dataset.artifactId);
          window.open(url, '_blank', 'noopener,noreferrer');
        });
      });
      document.querySelectorAll('.client-artifact-delete-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteClientArtifactModal(button.dataset.clientId, button.dataset.artifactId, artifactList));
      });
    }

    function openDeleteServiceAccessModal(clientID, accessID, accessList = []) {
      const client = findClient(clientID);
      const access = (accessList || []).find((item) => item.id === accessID);
      if (!client || !access) return;
      const inbound = serviceAccessInboundInfo(access);
      openModal(`Remove access: ${inbound.serviceLabel}`, 'Delete one service binding and its generated configs', `
        <p class="danger-text">This removes this service access, its managed route rows, generated config artifacts, delivery links for those artifacts and service-access secrets. The client account remains active.</p>
        <div class="client-danger-summary">
          <div><span>Client</span><strong>${escapeHTML(clientDisplayName(client))}</strong></div>
          <div><span>Service</span><strong>${escapeHTML(inbound.serviceLabel)}</strong></div>
          <div><span>Instance</span><strong>${escapeHTML(inbound.instanceName)}</strong></div>
          <div><span>Endpoint</span><strong>${escapeHTML(inbound.endpoint)}</strong></div>
        </div>
        <div class="modal-actions">
          <button class="danger-btn" id="confirmDeleteServiceAccessBtn" type="button">Remove access</button>
          <button class="secondary-btn" id="cancelDeleteServiceAccessBtn" type="button">Cancel</button>
        </div>
        <div id="deleteServiceAccessResult" class="form-result"></div>`, { variant: 'danger' });
      document.getElementById('cancelDeleteServiceAccessBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('confirmDeleteServiceAccessBtn')?.addEventListener('click', () => deleteServiceAccess(clientID, accessID));
    }

    async function deleteServiceAccess(clientID, accessID) {
      const target = document.getElementById('deleteServiceAccessResult');
      const button = document.getElementById('confirmDeleteServiceAccessBtn');
      if (button) {
        button.disabled = true;
        button.textContent = 'Removing access';
      }
      if (target) target.innerHTML = '<span class="tag warn">removing access</span>';
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/accesses/${accessID}`, { method: 'DELETE' });
        if (target) {
          target.innerHTML = `
            <div class="notice success">
              <strong>Service access removed</strong>
              <p>${escapeHTML(String(data.service_accesses_deleted || 0))} access, ${escapeHTML(String(data.access_routes_deleted || 0))} routes and ${escapeHTML(String(data.config_cleanup?.artifacts_deleted || 0))} configs removed.</p>
            </div>`;
        }
        await refresh();
        setTimeout(() => openClientAccessesModal(clientID), 500);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        if (button) {
          button.disabled = false;
          button.textContent = 'Remove access';
        }
      }
    }

    function openClearClientConfigsModal(clientID, artifactList = [], shareLinkList = [], subscriptionList = []) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Clear configs: ${client.username}`, 'Remove generated config artifacts and delivery tokens', `
        <p class="danger-text">This removes generated config files, public delivery links and VLESS subscription tokens for this client. Service access remains provisioned, so new configs can be built again.</p>
        <div class="client-danger-summary">
          <div><span>Configs</span><strong>${escapeHTML(String(artifactList.length))}</strong></div>
          <div><span>Share links</span><strong>${escapeHTML(String(shareLinkList.length))}</strong></div>
          <div><span>Subscriptions</span><strong>${escapeHTML(String(subscriptionList.length))}</strong></div>
        </div>
        <div class="modal-actions">
          <button class="danger-btn" id="confirmClearClientConfigsBtn" type="button">Clear configs</button>
          <button class="secondary-btn" id="cancelClearClientConfigsBtn" type="button">Cancel</button>
        </div>
        <div id="clientConfigCleanupResult" class="form-result"></div>`, { variant: 'warning' });
      document.getElementById('cancelClearClientConfigsBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('confirmClearClientConfigsBtn')?.addEventListener('click', () => clearClientConfigs(clientID));
    }

    async function clearClientConfigs(clientID) {
      const target = document.getElementById('clientConfigCleanupResult');
      const button = document.getElementById('confirmClearClientConfigsBtn');
      if (button) {
        button.disabled = true;
        button.textContent = 'Clearing configs';
      }
      if (target) target.innerHTML = '<span class="tag warn">clearing configs</span>';
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/configs`, { method: 'DELETE' });
        if (target) {
          target.innerHTML = `
            <div class="notice success">
              <strong>Client configs cleared</strong>
              <p>${escapeHTML(String(data.artifacts_deleted || 0))} configs, ${escapeHTML(String(data.share_links_deleted || 0))} links and ${escapeHTML(String(data.subscriptions_deleted || 0))} subscriptions removed.</p>
            </div>`;
        }
        await refresh();
        setTimeout(() => openClientAccessesModal(clientID), 500);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        if (button) {
          button.disabled = false;
          button.textContent = 'Clear configs';
        }
      }
    }

    function openDeleteClientModal(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      const summary = clientSummary(client);
      const confirmValue = client.username || client.id;
      openModal(`Delete client: ${client.username}`, 'Permanent client removal', `
        <p class="danger-text">This permanently deletes the client account, service accesses, routes, generated configs, delivery links, subscriptions and service-access secret refs. Audit and job history remain for traceability.</p>
        <div class="client-danger-summary">
          <div><span>Accesses</span><strong>${escapeHTML(String(summary.serviceAccessCount))}</strong></div>
          <div><span>Routes</span><strong>${escapeHTML(String(summary.routeCount))}</strong></div>
          <div><span>Configs</span><strong>${escapeHTML(String(summary.artifactCount))}</strong></div>
          <div><span>Delivery</span><strong>${escapeHTML(String(summary.shareLinkCount))}</strong></div>
        </div>
        <div class="field full">
          <label>Type client username to confirm</label>
          <input id="deleteClientConfirmInput" autocomplete="off" placeholder="${escapeHTML(confirmValue)}" />
        </div>
        <div class="modal-actions">
          <button class="danger-btn" id="confirmDeleteClientBtn" type="button" disabled>Delete client</button>
          <button class="secondary-btn" id="cancelDeleteClientBtn" type="button">Cancel</button>
        </div>
        <div id="deleteClientResult" class="form-result"></div>`, { variant: 'danger' });
      const input = document.getElementById('deleteClientConfirmInput');
      const button = document.getElementById('confirmDeleteClientBtn');
      input?.addEventListener('input', () => {
        if (button) button.disabled = String(input.value || '').trim() !== confirmValue;
      });
      document.getElementById('cancelDeleteClientBtn')?.addEventListener('click', closeModal);
      button?.addEventListener('click', () => deleteClient(clientID));
    }

    async function deleteClient(clientID) {
      const target = document.getElementById('deleteClientResult');
      const button = document.getElementById('confirmDeleteClientBtn');
      if (button) {
        button.disabled = true;
        button.textContent = 'Deleting client';
      }
      if (target) target.innerHTML = '<span class="tag warn">deleting client</span>';
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}`, { method: 'DELETE' });
        if (target) {
          target.innerHTML = `
            <div class="notice success">
              <strong>Client deleted</strong>
              <p>${escapeHTML(String(data.service_accesses_deleted || 0))} accesses and ${escapeHTML(String(data.config_cleanup?.artifacts_deleted || 0))} configs removed. ${escapeHTML(String(data.instance_apply_jobs_queued || 0))} instance apply jobs queued.</p>
            </div>`;
        }
        await refresh();
        setTimeout(closeModal, 700);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        if (button) {
          button.disabled = false;
          button.textContent = 'Delete client';
        }
      }
    }

    function openRotateClientSubscriptionModal(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`VLESS subscription: ${client.username}`, 'Rotate the per-client subscription token. The full URL is shown once after creation.', `
        <form id="clientSubscriptionForm" class="form-grid">
          <div class="field"><label>TTL hours</label><input name="ttl_hours" type="number" min="1" max="8760" value="720" /></div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit">Rotate token</button>
            <button class="secondary-btn" id="cancelSubscriptionRotateBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="clientSubscriptionResult" class="form-result"></div>`);
      document.getElementById('cancelSubscriptionRotateBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientSubscriptionForm')?.addEventListener('submit', (event) => submitClientSubscriptionRotate(event, clientID));
    }

    async function submitClientSubscriptionRotate(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientSubscriptionResult');
      if (target) target.innerHTML = '<span class="tag warn">rotating subscription</span>';
      try {
        const form = new FormData(event.currentTarget);
        const data = await sendJSON(`/api/v1/clients/${clientID}/subscriptions/rotate`, 'POST', {
          ttl_hours: Number(form.get('ttl_hours') || 720),
        });
        const url = String(data.subscription_url || '').trim();
        if (target) {
          target.innerHTML = `
            <div class="notice success">
              <strong>Subscription URL created</strong>
              <p>Copy it now. The plaintext token is not stored and cannot be displayed again.</p>
              <input class="copy-input" id="clientSubscriptionURL" value="${escapeHTML(url || 'public base URL is not configured')}" readonly />
              <div class="inline-actions" style="margin-top:10px">
                <button class="secondary-btn" id="copySubscriptionURLBtn" type="button"${url ? '' : ' disabled'}>Copy URL</button>
                <button class="secondary-btn" id="backToClientAccessBtn" type="button">Back to client access</button>
              </div>
              <div id="clientSubscriptionCopyStatus" class="form-help" style="margin-top:8px"></div>
            </div>`;
          document.getElementById('copySubscriptionURLBtn')?.addEventListener('click', () => copyTextToClipboard(url, 'clientSubscriptionCopyStatus'));
          document.getElementById('backToClientAccessBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
        }
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokeClientSubscription(clientID, subscriptionID) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">revoking subscription</span>';
      try {
        await requestJSON(`/api/v1/clients/${clientID}/subscriptions/${subscriptionID}/revoke`, { method: 'POST' });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function copyTextToClipboard(value, statusID) {
      const status = document.getElementById(statusID);
      try {
        await navigator.clipboard.writeText(value);
        if (status) status.innerHTML = '<span class="tag ok">copied</span>';
      } catch (_) {
        const input = document.getElementById('clientSubscriptionURL');
        input?.select();
        if (status) status.innerHTML = '<span class="tag warn">select and copy manually</span>';
      }
    }

    function openBuildClientArtifactsModal(clientID, accessList = []) {
      const client = findClient(clientID);
      if (!client) return;
      const defaultInstances = (accessList || []).map((access) => access.instance_id).filter(Boolean);
      openModal(`Build configs: ${client.username}`, 'Generate OVPN, VLESS and other client artifacts', `
        <form id="clientArtifactBuildForm" class="client-config-build-form form-grid">
          <div class="field client-config-type-field"><label>Artifact type</label><select name="artifact_type">
            <option value="all">All supported configs</option>
            <option value="zip_bundle">ZIP bundle</option>
            <option value="ovpn">OpenVPN .ovpn</option>
            <option value="vless_url">VLESS URL</option>
            <option value="wg_conf">WireGuard config</option>
            <option value="mtproto_url">MTProto URL</option>
            <option value="http_proxy_bundle">HTTP proxy bundle</option>
            <option value="ipsec_bundle">IPsec/L2TP bundle</option>
            <option value="ss_url">Shadowsocks URL</option>
          </select></div>
          <div class="client-config-build-note">
            <strong>${escapeHTML(String(defaultInstances.length))}</strong>
            <span>provisioned service accesses selected by default</span>
          </div>
          <div class="field full"><label>Provisioned service access</label><div class="client-config-choice-grid" id="clientArtifactInstances">${renderClientConfigInstanceChoices(accessList, defaultInstances)}</div></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Queue build</button><button class="secondary-btn" id="cancelClientArtifactBuildBtn" type="button">Cancel</button></div>
        </form>
        <div id="clientArtifactBuildResult" class="form-result"></div>`, { size: 'full', bodyClass: 'client-config-build-modal' });
      document.getElementById('cancelClientArtifactBuildBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientArtifactBuildForm')?.addEventListener('submit', (event) => submitClientArtifactBuild(event, clientID));
    }

    async function openBuildClientArtifactsForClient(clientID) {
      const client = findClient(clientID);
      if (!client) return;
      openModal(`Build configs: ${client.username}`, 'Loading provisioned service access', '<div class="empty">Loading service accesses...</div>', { wide: true });
      try {
        const accesses = await requestJSON(`/api/v1/clients/${clientID}/accesses`);
        openBuildClientArtifactsModal(clientID, Array.isArray(accesses) ? accesses : []);
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load service accesses: ${escapeHTML(err.message)}</div>`;
      }
    }

    async function submitClientArtifactBuild(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientArtifactBuildResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing config build</span>';
      try {
        const formElement = event.currentTarget;
        const form = new FormData(formElement);
        const instanceIDs = Array.from(formElement.querySelectorAll('input[name="instance_ids"]:checked') || [])
          .map((input) => input.value)
          .filter(Boolean);
        if (instanceIDs.length === 0) {
          if (target) target.innerHTML = '<span class="tag danger">Provision at least one service access first</span>';
          return;
        }
        const data = await sendJSON(`/api/v1/clients/${clientID}/artifacts`, 'POST', {
          type: String(form.get('artifact_type') || 'all').trim(),
          instance_ids: instanceIDs,
        });
        if (target) target.innerHTML = renderActionResponse(data, 'Client config build');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function openPublishShareLinkModal(clientID, artifactList = []) {
      const client = findClient(clientID);
      if (!client) return;
      const readyArtifacts = (artifactList || []).filter((artifact) => String(artifact.status || '').toLowerCase() === 'ready');
      const options = readyArtifacts.map((artifact) => {
        const label = `${artifactTypeLabel(artifact.artifact_type)} · ${artifact.size_bytes || 0} B · ${formatDate(artifact.created_at)}`;
        return `<option value="${escapeHTML(artifact.id)}">${escapeHTML(label)}</option>`;
      }).join('');
      openModal(`Publish delivery link: ${client.username}`, 'Create a temporary download link for one ready artifact', `
        <form id="clientShareLinkForm" class="form-grid">
          <div class="field full"><label>Ready artifact</label><select name="target_id" required>${options || '<option value="" disabled>No ready artifacts</option>'}</select></div>
          <div class="field"><label>TTL hours</label><input name="ttl_hours" type="number" min="1" max="720" value="72" /></div>
          <div class="field full inline-actions">
            <button class="primary-btn" type="submit"${readyArtifacts.length ? '' : ' disabled'}>Publish link</button>
            <button class="secondary-btn" id="cancelShareLinkBtn" type="button">Cancel</button>
          </div>
        </form>
        <div id="clientShareLinkResult" class="form-result"></div>`);
      document.getElementById('cancelShareLinkBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientShareLinkForm')?.addEventListener('submit', (event) => submitClientShareLink(event, clientID));
    }

    async function submitClientShareLink(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientShareLinkResult');
      if (target) target.innerHTML = '<span class="tag warn">publishing link</span>';
      try {
        const form = new FormData(event.currentTarget);
        await sendJSON(`/api/v1/clients/${clientID}/share-links`, 'POST', {
          target_id: String(form.get('target_id') || '').trim(),
          ttl_hours: Number(form.get('ttl_hours') || 72),
        });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokeClientShareLink(clientID, linkID) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">revoking link</span>';
      try {
        await requestJSON(`/api/v1/clients/${clientID}/share-links/${linkID}/revoke`, { method: 'POST' });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function previewClientArtifact(clientID, artifactID) {
      openModal('Client config preview', 'Loading generated artifact', '<div class="empty">Loading artifact...</div>', { wide: true });
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/artifacts/${artifactID}/content`);
        el('modalBody').innerHTML = `
          <div class="card">
            <div class="mini-label">${escapeHTML(data.artifact_type || 'artifact')}</div>
            <h3>${escapeHTML(data.filename || artifactID)}</h3>
            <div class="metric-caption">${escapeHTML(String(data.size_bytes || 0))} bytes</div>
          </div>
          <textarea class="code-textarea" rows="18" readonly>${escapeHTML(data.content || '')}</textarea>
          <div class="inline-actions" style="margin-top:12px">
            <button class="secondary-btn" id="downloadPreviewArtifactBtn" type="button">Download</button>
          </div>`;
        document.getElementById('downloadPreviewArtifactBtn')?.addEventListener('click', () => {
          window.open(artifactDownloadURL(clientID, artifactID), '_blank', 'noopener,noreferrer');
        });
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load artifact: ${escapeHTML(err.message)}</div>`;
      }
    }

    function openDeleteClientArtifactModal(clientID, artifactID, artifactList = []) {
      const client = findClient(clientID);
      const artifact = (artifactList || []).find((item) => item.id === artifactID);
      if (!client || !artifact) return;
      openModal(`Delete config: ${client.username}`, 'Remove one generated client config', `
        <p class="danger-text">This removes only the selected generated config and public links that point to it. Service access remains provisioned, so a fresh config can be built again.</p>
        <div class="client-danger-summary">
          <div><span>Client</span><strong>${escapeHTML(clientDisplayName(client))}</strong></div>
          <div><span>Type</span><strong>${escapeHTML(artifactTypeLabel(artifact.artifact_type))}</strong></div>
          <div><span>Status</span><strong>${escapeHTML(artifact.status || 'unknown')}</strong></div>
          <div><span>Size</span><strong>${escapeHTML(String(artifact.size_bytes || 0))} B</strong></div>
        </div>
        <div class="modal-actions">
          <button class="danger-btn" id="confirmDeleteClientArtifactBtn" type="button">Delete config</button>
          <button class="secondary-btn" id="cancelDeleteClientArtifactBtn" type="button">Cancel</button>
        </div>
        <div id="deleteClientArtifactResult" class="form-result"></div>`, { variant: 'danger' });
      document.getElementById('cancelDeleteClientArtifactBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('confirmDeleteClientArtifactBtn')?.addEventListener('click', () => deleteClientArtifact(clientID, artifactID));
    }

    async function deleteClientArtifact(clientID, artifactID) {
      const target = document.getElementById('deleteClientArtifactResult');
      const button = document.getElementById('confirmDeleteClientArtifactBtn');
      if (button) {
        button.disabled = true;
        button.textContent = 'Deleting config';
      }
      if (target) target.innerHTML = '<span class="tag warn">deleting config</span>';
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/artifacts/${artifactID}`, { method: 'DELETE' });
        if (target) {
          target.innerHTML = `
            <div class="notice success">
              <strong>Config deleted</strong>
              <p>${escapeHTML(String(data.share_links_deleted || 0))} delivery links and ${escapeHTML(String(data.files_deleted || 0))} files removed.</p>
            </div>`;
        }
        await refresh();
        setTimeout(() => openClientAccessesModal(clientID), 500);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        if (button) {
          button.disabled = false;
          button.textContent = 'Delete config';
        }
      }
    }

    function openCreateClientRouteModal(clientID, accessOptions = '') {
      const client = findClient(clientID);
      if (!client) return;
      const hasAccessOptions = String(accessOptions || '').trim() !== '';
      const bindingOptions = hasAccessOptions
        ? `<option value="">Select service access</option>${accessOptions}`
        : '<option value="">Provision service access first</option>';
      openModal(`Add route: ${client.username}`, 'Client access routing', `
        <form id="clientRouteForm" class="form-grid">
          <div class="field full"><label>Service access binding</label><select name="service_access_id" required>${bindingOptions}</select></div>
          <div class="field"><label>Name</label><input name="name" placeholder="office-lan" /></div>
          <div class="field"><label>Action</label><select name="action"><option value="allow">allow</option><option value="deny">deny</option></select></div>
          <div class="field"><label>Destination type</label><select name="destination_type"><option value="cidr">cidr</option><option value="dns">dns</option><option value="endpoint">endpoint</option><option value="service">service</option></select></div>
          <div class="field"><label>Destination</label><input name="destination" required placeholder="10.10.0.0/16 or app.internal" /></div>
          <div class="field"><label>Protocol</label><select name="protocol"><option value="any">any</option><option value="tcp">tcp</option><option value="udp">udp</option><option value="icmp">icmp</option></select></div>
          <div class="field"><label>Ports</label><input name="ports" value="*" placeholder="443 or 80,443 or 1000-2000" /></div>
          <div class="field"><label>Egress mode</label><select name="egress_mode"><option value="">auto</option><option value="egress_node">egress node</option><option value="local_breakout">local breakout</option></select></div>
          <div class="field"><label>Egress node</label><select name="egress_node_id">${egressNodeOptions()}</select></div>
          <div class="field"><label>Backhaul next-hop</label><input name="egress_next_hop" placeholder="10.255.0.2" /></div>
          <div class="field"><label>Backhaul interface</label><input name="egress_interface" placeholder="wg-backhaul0" /></div>
          <div class="field"><label>Routing table</label><input name="routing_table" placeholder="main" /></div>
          <div class="field full"><label>Description</label><textarea name="description" rows="3" placeholder="Why this route exists and what it permits."></textarea></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit"${hasAccessOptions ? '' : ' disabled'}>Save route</button><button class="secondary-btn" id="cancelClientRouteBtn" type="button">Cancel</button></div>
        </form>
        <div id="clientRouteResult" class="form-result"></div>`);
      document.getElementById('cancelClientRouteBtn')?.addEventListener('click', () => openClientAccessesModal(clientID));
      document.getElementById('clientRouteForm')?.addEventListener('submit', (event) => submitClientRoute(event, clientID));
    }

    async function submitClientRoute(event, clientID) {
      event.preventDefault();
      const target = document.getElementById('clientRouteResult');
      if (target) target.innerHTML = '<span class="tag warn">saving route</span>';
      const form = new FormData(event.currentTarget);
      const payload = {
        service_access_id: String(form.get('service_access_id') || '').trim() || null,
        name: String(form.get('name') || '').trim(),
        action: String(form.get('action') || 'allow'),
        destination_type: String(form.get('destination_type') || 'cidr'),
        destination: String(form.get('destination') || '').trim(),
        protocol: String(form.get('protocol') || 'any'),
        ports: String(form.get('ports') || '*').trim() || '*',
        description: String(form.get('description') || '').trim(),
      };
      const egress = {
        mode: String(form.get('egress_mode') || '').trim(),
        node_id: String(form.get('egress_node_id') || '').trim(),
        next_hop: String(form.get('egress_next_hop') || '').trim(),
        interface: String(form.get('egress_interface') || '').trim(),
        table: String(form.get('routing_table') || '').trim(),
      };
      if (egress.mode || egress.node_id || egress.next_hop || egress.interface || egress.table) {
        payload.policy = { egress };
      }
      try {
        await sendJSON(`/api/v1/clients/${clientID}/routes`, 'POST', payload);
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function revokeClientAccessRoute(clientID, routeID) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">revoking route</span>';
      try {
        await requestJSON(`/api/v1/clients/${clientID}/routes/${routeID}`, { method: 'DELETE' });
        await refresh();
        await openClientAccessesModal(clientID);
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function rotateClientAccess(clientID, accessID, driver) {
      const target = document.getElementById('clientAccessRotateResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing rotation</span>';
      const suffix = driver === 'openvpn'
        ? 'rotate-openvpn'
        : driver === 'wireguard'
          ? 'rotate-wireguard'
        : driver === 'mtproto'
          ? 'rotate-mtproto'
        : driver === 'ipsec'
          ? 'rotate-ipsec'
        : driver === 'http_proxy'
          ? 'rotate-http-proxy'
        : driver === 'shadowsocks'
          ? 'rotate-shadowsocks'
          : 'rotate-xray';
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/accesses/${accessID}/${suffix}`, { method: 'POST' });
        if (target) target.innerHTML = renderActionResponse(data, 'Client access rotation');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    return {
      render,
      openCreateClientModal,
    };
  }

  window.MegaVPNClientsPage = { create: createClientsPage };
})(window);
