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
      clients: [],
      jobs: [],
      artifacts: [],
      shareLinks: [],
      backhaulLinks: [],
      backhaulDrivers: [],
      servicesCatalog: [],
      servicePacks: [],
      servicePackCatalog: [],
      serviceInstallers: [],
      serviceCapabilitiesByNode: {},
      serviceInstallEventsByNode: {},
      runtimePreflight: null,
      mailSettings: null,
      controlPlaneTLSSettings: null,
      platformCertificates: [],
      platformInvites: [],
      platformPKIRoots: [],
      servicesNodeID: localStorage.getItem('megavpn.servicesNodeID') || '',
      revisionsInstanceID: localStorage.getItem('megavpn.revisionsInstanceID') || '',
      nodeManageID: '',
      nodeManageData: null,
      nodeManageActiveTabs: readSessionObject('megavpn.nodeManageActiveTabs'),
      nodeTerminalActive: false,
      refreshSeq: 0,
      refreshInFlight: false,
      refreshInFlightSeq: 0,
      lastError: null,
    };
  }

  window.MegaVPNAppState = { createInitialState };
})(window);
