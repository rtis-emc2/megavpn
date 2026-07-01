(function (window) {
  'use strict';

  window.MegaVPNAppConfig = {
    nodeMap: {
      staticMapURL: './assets/world-map.svg',
    },
    navGroups: [
      ['Operations', [
        ['dashboard', 'Dashboard', '●'],
        ['nodes', 'Nodes', '◇'],
        ['nodeMap', 'Node map', '⌖'],
        ['instances', 'Instances', '▣'],
        ['clients', 'Clients', '◎'],
        ['jobs', 'Jobs', '↻'],
      ]],
      ['Provisioning', [
        ['backhaul', 'Backhaul', '⇄'],
        ['addressPools', 'Address pools', '▦'],
        ['artifacts', 'Artifacts', '▤'],
        ['shareLinks', 'Share links', '↗'],
      ]],
      ['Security', [
        ['firewall', 'Firewall', '▧'],
        ['certificates', 'Certificates', '◈'],
        ['audit', 'Audit', '◇'],
      ]],
      ['Control', [
        ['services', 'Services', '⚙'],
        ['telemetry', 'Telemetry', '≋'],
        ['revisions', 'Revisions', '≣'],
        ['settings', 'Settings', '☷'],
      ]],
    ],
    nodeExecutionModes: {
      ssh_bootstrap: {
        title: 'SSH bootstrap',
        caption: 'Install agent over SSH',
        description: 'Control plane connects to the node once over SSH, installs megavpn-agent, creates enrollment token and waits for heartbeat.',
        requirements: 'Requires SSH host, SSH user and private key or password.',
      },
      manual_bundle: {
        title: 'Manual install',
        caption: 'Operator installs agent',
        description: 'Create the node and token, then run the agent install/enrollment command manually on the server.',
        requirements: 'No SSH from control plane is required.',
      },
      agent_managed: {
        title: 'Agent self-enrollment',
        caption: 'Manual agent path',
        description: 'Use when megavpn-agent is already installed or will be installed manually. This does not queue an SSH bootstrap job.',
        requirements: 'Requires a running agent, enrollment token and reachable control plane URL.',
      },
      local_managed: {
        title: 'Local managed',
        caption: 'Control-plane host only',
        description: 'Use for local/lab node records where the control plane and runtime are on the same host.',
        requirements: 'Not intended for remote production nodes.',
      },
    },
  };
})(window);
