(function (window) {
  'use strict';

  function createCoreLoader(ctx = {}) {
    const {
      state,
      fetchJSON,
      hasPermission,
      updateReadyPill,
      renderNotice,
    } = ctx;
    if (
      !state ||
      typeof fetchJSON !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof updateReadyPill !== 'function' ||
      typeof renderNotice !== 'function'
    ) {
      throw new Error('MegaVPNCoreLoader requires loader dependencies');
    }

    function resetAuthenticatedState() {
      state.dashboard = null;
      state.nodes = [];
      state.instances = [];
      state.instanceRuntimeStates = [];
      state.addressPoolSpaces = [];
      state.addressPoolAllocations = [];
      state.firewallInventory = { address_lists: [], entries: [], policies: [], rules: [], node_states: [] };
      state.trafficAccounting = { summary: { retention_days: 180 }, samples: [] };
      state.clients = [];
      state.jobs = [];
      state.artifacts = [];
      state.shareLinks = [];
      state.backhaulLinks = [];
      state.backhaulDrivers = [];
      state.servicesCatalog = [];
      state.servicePacks = [];
      state.servicePackCatalog = [];
      state.vlessGroupTemplates = [];
      state.vlessGroupCatalog = [];
      state.serviceInstallers = [];
      state.binaryArtifacts = [];
      state.serviceCapabilitiesByNode = {};
      state.serviceInstallEventsByNode = {};
      state.mailSettings = null;
      state.controlPlaneTLSSettings = null;
      state.platformCertificates = [];
      state.platformInvites = [];
      state.platformPKIRoots = [];
    }

    function persistSelectedIDs() {
      if (!state.servicesNodeID || !state.nodes.some((node) => node.id === state.servicesNodeID)) {
        state.servicesNodeID = state.nodes[0]?.id || '';
        if (state.servicesNodeID) {
          localStorage.setItem('megavpn.servicesNodeID', state.servicesNodeID);
        } else {
          localStorage.removeItem('megavpn.servicesNodeID');
        }
      }
      if (!state.revisionsInstanceID || !state.instances.some((instance) => instance.id === state.revisionsInstanceID)) {
        state.revisionsInstanceID = state.instances[0]?.id || '';
        if (state.revisionsInstanceID) {
          localStorage.setItem('megavpn.revisionsInstanceID', state.revisionsInstanceID);
        } else {
          localStorage.removeItem('megavpn.revisionsInstanceID');
        }
      }
    }

    function servicePackRank(pack) {
      const status = String(pack?.status || 'active').toLowerCase();
      const statusRank = status === 'active' ? 0 : status === 'disabled' ? 1 : 2;
      const sourceRank = String(pack?.source || 'default').toLowerCase() === 'default' ? 1 : 0;
      const version = Number(pack?.version || 0);
      return { statusRank, sourceRank, version };
    }

    function preferServicePack(next, current) {
      if (!current) return true;
      const left = servicePackRank(next);
      const right = servicePackRank(current);
      if (left.statusRank !== right.statusRank) return left.statusRank < right.statusRank;
      if (left.sourceRank !== right.sourceRank) return left.sourceRank < right.sourceRank;
      return left.version > right.version;
    }

    function servicePackFingerprint(pack) {
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

    function normalizeServicePackList(value) {
      const items = Array.isArray(value) ? value : [];
      const byKey = new Map();
      const byFingerprint = new Map();
      for (const pack of items) {
        const key = String(pack?.key || '').trim();
        if (!key) continue;
        const fingerprint = servicePackFingerprint(pack);
        const current = byKey.get(key) || (fingerprint ? byFingerprint.get(fingerprint) : null);
        if (!preferServicePack(pack, current)) continue;
        if (current?.key) {
          byKey.delete(String(current.key).trim());
          const currentFingerprint = servicePackFingerprint(current);
          if (currentFingerprint) byFingerprint.delete(currentFingerprint);
        }
        byKey.set(key, pack);
        if (fingerprint) byFingerprint.set(fingerprint, pack);
      }
      return Array.from(byKey.values());
    }

    function activeServicePacksFromCatalog(catalog) {
      return normalizeServicePackList(catalog).filter((pack) => String(pack.status || 'active').toLowerCase() === 'active');
    }

    function trafficAccountingPath() {
      const source = state.trafficExportFilters && typeof state.trafficExportFilters === 'object'
        ? state.trafficExportFilters
        : {};
      const params = new URLSearchParams();
      const limit = String(source.limit || '250').trim();
      params.set('limit', limit || '250');
      for (const key of ['from', 'to', 'protocol', 'client_id', 'node_id']) {
        const value = String(source[key] || '').trim();
        if (value) params.set(key, value);
      }
      return `/api/v1/traffic/accounting?${params.toString()}`;
    }

    async function loadCore() {
      state.ready = await fetchJSON('/api/v1/ready', { status: 'not_ready' });
      state.versionInfo = await fetchJSON('/api/v1/version', null);
      if (!state.authUser) {
        resetAuthenticatedState();
        updateReadyPill();
        renderNotice();
        return;
      }
      state.dashboard = await fetchJSON('/api/v1/dashboard', null);
      const nodes = await fetchJSON('/api/v1/nodes', []);
      const instances = await fetchJSON('/api/v1/instances', []);
      const instanceRuntimeStates = hasPermission('instance.read') ? await fetchJSON('/api/v1/instances/runtime-states', []) : [];
      const addressPools = hasPermission('instance.read') ? await fetchJSON('/api/v1/address-pools', { spaces: [], allocations: [] }) : { spaces: [], allocations: [] };
      const firewallInventory = hasPermission('firewall.read') ? await fetchJSON('/api/v1/firewall', { address_lists: [], entries: [], policies: [], rules: [], node_states: [] }) : { address_lists: [], entries: [], policies: [], rules: [], node_states: [] };
      const trafficAccounting = hasPermission('traffic.read') ? await fetchJSON(trafficAccountingPath(), { summary: { retention_days: 180 }, samples: [] }) : { summary: { retention_days: 180 }, samples: [] };
      const clients = await fetchJSON('/api/v1/clients', []);
      const jobs = await fetchJSON('/api/v1/jobs?limit=50', []);
      const artifacts = await fetchJSON('/api/v1/artifacts', []);
      const shareLinks = await fetchJSON('/api/v1/share-links', []);
      const backhaulLinks = hasPermission('node.read') ? await fetchJSON('/api/v1/backhaul-links', []) : [];
      const backhaulDrivers = hasPermission('node.read') ? await fetchJSON('/api/v1/backhaul/drivers', []) : [];
      const servicesCatalog = await fetchJSON('/api/v1/services', []);
      const servicePacks = await fetchJSON('/api/v1/service-packs', []);
      const vlessGroupTemplates = await fetchJSON('/api/v1/vless-groups', []);
      const serviceCapabilitiesByNode = hasPermission('node.read') ? await fetchJSON('/api/v1/nodes/capabilities', {}) : {};
      const servicePackCatalog = hasPermission('settings.manage')
        ? await fetchJSON('/api/v1/service-packs?include_inactive=1', servicePacks)
        : servicePacks;
      const vlessGroupCatalog = hasPermission('settings.manage')
        ? await fetchJSON('/api/v1/vless-groups?include_inactive=1', vlessGroupTemplates)
        : vlessGroupTemplates;
      const serviceInstallers = await fetchJSON('/api/v1/services/installers', []);
      const binaryArtifacts = hasPermission('binary_repository.read') ? await fetchJSON('/api/v1/binary-artifacts', []) : [];
      const platformCertificates = (hasPermission('instance.read') || hasPermission('settings.manage')) ? await fetchJSON('/api/v1/platform/certificates', []) : [];
      const platformPKIRoots = hasPermission('instance.read') ? await fetchJSON('/api/v1/platform/pki-roots', []) : [];
      const controlPlaneTLSSettings = hasPermission('settings.manage') ? await fetchJSON('/api/v1/settings/control-plane-tls', null) : state.controlPlaneTLSSettings;
      state.nodes = Array.isArray(nodes) ? nodes.filter((node) => node.status !== 'retired') : [];
      state.instances = Array.isArray(instances) ? instances : [];
      state.instanceRuntimeStates = Array.isArray(instanceRuntimeStates) ? instanceRuntimeStates : [];
      state.addressPoolSpaces = Array.isArray(addressPools?.spaces) ? addressPools.spaces : [];
      state.addressPoolAllocations = Array.isArray(addressPools?.allocations) ? addressPools.allocations : [];
      state.firewallInventory = firewallInventory && typeof firewallInventory === 'object' && !Array.isArray(firewallInventory)
        ? {
            address_lists: Array.isArray(firewallInventory.address_lists) ? firewallInventory.address_lists : [],
            entries: Array.isArray(firewallInventory.entries) ? firewallInventory.entries : [],
            policies: Array.isArray(firewallInventory.policies) ? firewallInventory.policies : [],
            rules: Array.isArray(firewallInventory.rules) ? firewallInventory.rules : [],
            node_states: Array.isArray(firewallInventory.node_states) ? firewallInventory.node_states : [],
          }
        : { address_lists: [], entries: [], policies: [], rules: [], node_states: [] };
      state.trafficAccounting = trafficAccounting && typeof trafficAccounting === 'object' && !Array.isArray(trafficAccounting)
        ? {
            summary: trafficAccounting.summary && typeof trafficAccounting.summary === 'object' ? trafficAccounting.summary : { retention_days: 180 },
            samples: Array.isArray(trafficAccounting.samples) ? trafficAccounting.samples : [],
          }
        : { summary: { retention_days: 180 }, samples: [] };
      state.clients = Array.isArray(clients) ? clients : [];
      state.jobs = Array.isArray(jobs) ? jobs : [];
      state.artifacts = Array.isArray(artifacts) ? artifacts : [];
      state.shareLinks = Array.isArray(shareLinks) ? shareLinks : [];
      state.backhaulLinks = Array.isArray(backhaulLinks) ? backhaulLinks : [];
      state.backhaulDrivers = Array.isArray(backhaulDrivers) ? backhaulDrivers : [];
      state.servicesCatalog = Array.isArray(servicesCatalog) ? servicesCatalog : [];
      state.servicePackCatalog = normalizeServicePackList(servicePackCatalog);
      state.servicePacks = normalizeServicePackList(servicePacks);
      if (!state.servicePacks.length && state.servicePackCatalog.length) {
        state.servicePacks = activeServicePacksFromCatalog(state.servicePackCatalog);
      }
      state.vlessGroupCatalog = Array.isArray(vlessGroupCatalog) ? vlessGroupCatalog : [];
      state.vlessGroupTemplates = Array.isArray(vlessGroupTemplates) ? vlessGroupTemplates : [];
      if (!state.vlessGroupTemplates.length && state.vlessGroupCatalog.length) {
        state.vlessGroupTemplates = state.vlessGroupCatalog.filter((group) => String(group.status || 'active').toLowerCase() === 'active');
      }
      state.serviceInstallers = Array.isArray(serviceInstallers) ? serviceInstallers : [];
      state.binaryArtifacts = Array.isArray(binaryArtifacts) ? binaryArtifacts : [];
      state.serviceCapabilitiesByNode = serviceCapabilitiesByNode && typeof serviceCapabilitiesByNode === 'object' && !Array.isArray(serviceCapabilitiesByNode) ? serviceCapabilitiesByNode : {};
      state.platformCertificates = Array.isArray(platformCertificates) ? platformCertificates : [];
      state.platformPKIRoots = Array.isArray(platformPKIRoots) ? platformPKIRoots : [];
      state.controlPlaneTLSSettings = controlPlaneTLSSettings || null;
      persistSelectedIDs();
      updateReadyPill();
      renderNotice();
    }

    return { loadCore };
  }

  window.MegaVPNCoreLoader = { create: createCoreLoader };
})(window);
