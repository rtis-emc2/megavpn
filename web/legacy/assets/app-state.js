(function (window) {
  'use strict';

  function createInitialState() {
    const readSessionObject = (key) => {
      try {
        const value = JSON.parse(sessionStorage.getItem(key) || '{}');
        return value && typeof value === 'object' && !Array.isArray(value) ? value : {};
      } catch (_) {
        return {};
      }
    };

    return {
      page: 'dashboard',
      apiBase: localStorage.getItem('megavpn.apiBase') || '',
      inviteToken: new URLSearchParams(window.location.search).get('invite_token') || '',
      invitePreview: null,
      authUser: null,
      authSession: null,
      authRoles: [],
      authPermissions: [],
      dashboard: null,
      ready: null,
      versionInfo: null,
      nodes: [],
      instances: [],
      instanceRuntimeStates: [],
      addressPoolSpaces: [],
      addressPoolAllocations: [],
      firewallInventory: { address_lists: [], entries: [], policies: [], rules: [], node_states: [] },
      trafficAccounting: { summary: { retention_days: 180 }, samples: [], collectors: [], clients: [] },
      trafficExportFilters: readSessionObject('megavpn.trafficExportFilters'),
      clients: [],
      clientAccessServices: [],
      clientAccessGroups: [],
      clientAccessGroupMigrationConflicts: [],
      jobs: [],
      artifacts: [],
      shareLinks: [],
      backhaulLinks: [],
      backhaulDrivers: [],
      servicesCatalog: [],
      servicePacks: [],
      servicePackCatalog: [],
      vlessGroupTemplates: [],
      vlessGroupCatalog: [],
      serviceInstallers: [],
      binaryArtifacts: [],
      serviceCapabilitiesByNode: {},
      serviceInstallEventsByNode: {},
      runtimePreflight: null,
      mailSettings: null,
      controlPlaneTLSSettings: null,
      platformCertificates: [],
      platformInvites: [],
      platformPKIRoots: [],
      servicesNodeID: localStorage.getItem('megavpn.servicesNodeID') || '',
      servicesTab: localStorage.getItem('megavpn.servicesTab') || 'runtime',
      firewallTab: localStorage.getItem('megavpn.firewallTab') || 'overview',
      revisionsInstanceID: localStorage.getItem('megavpn.revisionsInstanceID') || '',
      revisionsTab: localStorage.getItem('megavpn.revisionsTab') || 'timeline',
      certificatesTab: localStorage.getItem('megavpn.certificatesTab') || 'overview',
      settingsTab: localStorage.getItem('megavpn.settingsTab') || 'runtime',
      telemetryTab: localStorage.getItem('megavpn.telemetryTab') || 'overview',
      auditTab: localStorage.getItem('megavpn.auditTab') || 'all',
      auditSearch: '',
      auditSort: localStorage.getItem('megavpn.auditSort') || 'newest',
      jobsTab: localStorage.getItem('megavpn.jobsTab') || 'active',
      jobsSearch: '',
      jobsSort: localStorage.getItem('megavpn.jobsSort') || 'newest',
      clientsTab: localStorage.getItem('megavpn.clientsTab') || 'clients',
      clientAccessGroupsServiceFilter: localStorage.getItem('megavpn.clientAccessGroupsServiceFilter') || 'all',
      nodeManageID: '',
      nodeManageData: null,
      nodeManageActiveTabs: readSessionObject('megavpn.nodeManageActiveTabs'),
      nodeManageDirty: false,
      nodeTerminalActive: false,
      instanceManageID: '',
      instanceManageData: null,
      instanceManageDirty: false,
      instancesView: 'list',
      instancesCreatePackKey: '',
      instancesCreatePackDraft: null,
      instancesCreateResult: null,
      vlessMembersInteractionLockUntil: 0,
      refreshSeq: 0,
      refreshInFlight: false,
      refreshInFlightSeq: 0,
      lastError: null,
    };
  }

  window.MegaVPNAppState = { createInitialState };
})(window);
