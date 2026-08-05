#!/usr/bin/env node
'use strict';

const assert = require('assert');
const fs = require('fs');
const { spawn } = require('child_process');
const http = require('http');
const os = require('os');
const path = require('path');

const rootDir = path.resolve(__dirname, '..', '..');
const smokeScript = path.join(rootDir, 'scripts', 'smoke', 'service-pack-smoke.sh');
const evidenceReportScript = path.join(rootDir, 'scripts', 'ci', 'service-pack-evidence-report.js');
const stagedSmokeScript = path.join(rootDir, 'scripts', 'smoke', 'service-pack-staged-smoke.sh');

function readBody(req) {
  return new Promise((resolve) => {
    let body = '';
    req.on('data', (chunk) => {
      body += chunk;
    });
    req.on('end', () => resolve(body));
  });
}

function sendJSON(res, code, data) {
  const body = JSON.stringify(data);
  res.writeHead(code, {
    'content-type': 'application/json',
    'content-length': Buffer.byteLength(body),
  });
  res.end(body);
}

async function withServer(handler, fn) {
  const requests = [];
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, 'http://127.0.0.1');
    const body = await readBody(req);
    requests.push({ method: req.method, path: url.pathname, search: url.search, body });
    try {
      await handler(req, res, url);
    } catch (err) {
      sendJSON(res, 500, { error: err.stack || String(err) });
    }
  });

  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });

  try {
    const { port } = server.address();
    return await fn(`http://127.0.0.1:${port}`, requests);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

function runSmoke(args, env = {}) {
  return new Promise((resolve) => {
    const child = spawn('bash', [smokeScript, ...args], {
      cwd: rootDir,
      env: {
        ...process.env,
        MEGAVPN_WAIT_ATTEMPTS: '2',
        MEGAVPN_WAIT_INTERVAL: '0',
        MEGAVPN_RUNTIME_WAIT_ATTEMPTS: '2',
        MEGAVPN_RUNTIME_WAIT_INTERVAL: '0',
        ...env,
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('close', (code) => {
      resolve({ code, stdout, stderr, output: `${stdout}${stderr}` });
    });
  });
}

function runStagedSmoke(args, env = {}) {
  return new Promise((resolve) => {
    const child = spawn('bash', [stagedSmokeScript, ...args], {
      cwd: rootDir,
      env: {
        ...process.env,
        MEGAVPN_WAIT_ATTEMPTS: '2',
        MEGAVPN_WAIT_INTERVAL: '0',
        MEGAVPN_RUNTIME_WAIT_ATTEMPTS: '2',
        MEGAVPN_RUNTIME_WAIT_INTERVAL: '0',
        ...env,
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('close', (code) => {
      resolve({ code, stdout, stderr, output: `${stdout}${stderr}` });
    });
  });
}

function runEvidenceReport(args) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [evidenceReportScript, ...args], {
      cwd: rootDir,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('close', (code) => {
      resolve({ code, stdout, stderr, output: `${stdout}${stderr}` });
    });
  });
}

function stagedSummaryPath(output) {
  const match = output.match(/^staged_summary:\s*(.+)$/m);
  assert.ok(match, `staged summary path is missing in output:\n${output}`);
  return match[1].trim();
}

const servicePacks = [
  {
    key: 'openvpn_tcp_11994',
    label: 'OpenVPN TCP',
    description: 'OpenVPN TCP baseline',
    components: [{ service_code: 'openvpn', endpoint_port: 11994 }],
  },
  {
    key: 'openvpn_udp_1194',
    label: 'OpenVPN UDP',
    description: 'OpenVPN UDP baseline',
    components: [{ service_code: 'openvpn', endpoint_port: 1194 }],
  },
  {
    key: 'wireguard_roadwarrior',
    label: 'WireGuard',
    description: 'WireGuard road-warrior baseline',
    components: [{ service_code: 'wireguard', endpoint_port: 51820 }],
  },
  {
    key: 'xray_nginx_http_edge',
    label: 'Xray WebSocket Camouflage',
    description: 'VLESS WebSocket behind Nginx',
    components: [
      { service_code: 'nginx', endpoint_port: 443 },
      { service_code: 'xray-core', endpoint_port: 7080 },
    ],
  },
];

function planHandler(req, res, url) {
  if (req.method === 'GET' && url.pathname === '/api/v1/service-packs') {
    sendJSON(res, 200, servicePacks);
    return;
  }
  sendJSON(res, 404, { error: `unexpected request: ${req.method} ${url.pathname}` });
}

function createLifecycleHandler(options = {}) {
  const state = {
    clientDeleted: false,
    instanceDeleted: false,
    createRequests: 0,
    cleanupRequests: 0,
    artifactsReady: options.artifactsReady !== false,
    accessStatus: options.accessStatus || 'active',
  };

  const handler = (req, res, url) => {
    const { pathname } = url;

    if (req.method === 'GET' && pathname === '/api/v1/service-packs') {
      sendJSON(res, 200, servicePacks);
      return;
    }

    if (req.method === 'POST' && pathname === '/api/v1/service-packs/openvpn_tcp_11994/instances') {
      state.createRequests += 1;
      sendJSON(res, 200, {
        created_instances: [{ id: 'inst-1', name: 'smoke-openvpn', service_code: 'openvpn' }],
        runtime_install_jobs: [{ id: 'job-install-openvpn', type: 'node.capability.install' }],
      });
      return;
    }

    if (req.method === 'GET' && pathname === '/api/v1/jobs') {
      const jobs = [
        { id: 'job-apply-inst-1', instance_id: 'inst-1', type: 'instance.apply' },
      ];
      if (state.instanceDeleted) {
        jobs.push({ id: 'job-delete-inst-1', instance_id: 'inst-1', type: 'instance.delete' });
      }
      sendJSON(res, 200, jobs);
      return;
    }

    const succeededJobs = {
      'job-install-openvpn': { id: 'job-install-openvpn', type: 'node.capability.install', status: 'succeeded', result: {} },
      'job-apply-inst-1': { id: 'job-apply-inst-1', type: 'instance.apply', status: 'succeeded', result: {} },
      'job-provision': {
        id: 'job-provision',
        type: 'client.provision',
        status: 'succeeded',
        result: {
          instance_apply_jobs: [{ job_id: 'job-post-apply', instance_id: 'inst-1', service_code: 'openvpn' }],
        },
      },
      'job-post-apply': { id: 'job-post-apply', type: 'instance.apply', status: 'succeeded', result: {} },
      'job-delete-inst-1': { id: 'job-delete-inst-1', type: 'instance.delete', status: 'succeeded', result: {} },
    };
    const jobMatch = pathname.match(/^\/api\/v1\/jobs\/([^/]+)$/);
    if (req.method === 'GET' && jobMatch && succeededJobs[jobMatch[1]]) {
      sendJSON(res, 200, succeededJobs[jobMatch[1]]);
      return;
    }
    if (req.method === 'GET' && /^\/api\/v1\/jobs\/[^/]+\/logs$/.test(pathname)) {
      sendJSON(res, 200, []);
      return;
    }

    if (req.method === 'GET' && pathname === '/api/v1/instances/inst-1') {
      sendJSON(res, 200, {
        id: 'inst-1',
        name: 'smoke-openvpn',
        service_code: 'openvpn',
        status: 'active',
        enabled: true,
        endpoint_host: 'vpn.test',
        endpoint_port: 11994,
        systemd_unit: 'megavpn-openvpn',
      });
      return;
    }
    if (req.method === 'GET' && pathname === '/api/v1/instances/inst-1/runtime-state') {
      sendJSON(res, 200, {
        instance_id: 'inst-1',
        service_code: 'openvpn',
        systemd_unit: 'megavpn-openvpn',
        runtime_status: 'active',
        health_status: 'healthy',
        drift_status: 'in_sync',
        active_state: 'active',
        enabled_state: 'enabled',
        agent_reported_at: '2026-07-07T00:00:00Z',
        checked_at: '2026-07-07T00:00:01Z',
      });
      return;
    }
    if (req.method === 'GET' && pathname === '/api/v1/instances/inst-1/runtime-observations') {
      sendJSON(res, 200, []);
      return;
    }

    if (req.method === 'POST' && pathname === '/api/v1/clients') {
      sendJSON(res, 200, { id: 'client-1' });
      return;
    }
    if (req.method === 'POST' && pathname === '/api/v1/clients/client-1/provision') {
      sendJSON(res, 200, { id: 'job-provision' });
      return;
    }
    if (req.method === 'GET' && pathname === '/api/v1/clients/client-1/accesses') {
      sendJSON(res, 200, [{ id: 'access-1', instance_id: 'inst-1', status: state.accessStatus, provision_mode: 'manual' }]);
      return;
    }
    if (req.method === 'GET' && pathname === '/api/v1/clients/client-1/artifacts') {
      sendJSON(res, 200, state.artifactsReady
        ? [{ id: 'artifact-1', artifact_type: 'ovpn', service_access_id: 'access-1', status: 'ready', size_bytes: 128 }]
        : []);
      return;
    }
    if (req.method === 'DELETE' && pathname === '/api/v1/clients/client-1') {
      state.clientDeleted = true;
      state.cleanupRequests += 1;
      sendJSON(res, 200, {
        client_id: 'client-1',
        username: 'smoke',
        deleted: true,
        service_accesses_deleted: 1,
        access_routes_deleted: 1,
        email_deliveries_deleted: 0,
        secret_refs_deleted: 1,
        config_cleanup: { deleted: state.artifactsReady ? 1 : 0 },
      });
      return;
    }
    if (req.method === 'DELETE' && pathname === '/api/v1/instances/inst-1') {
      state.instanceDeleted = true;
      state.cleanupRequests += 1;
      sendJSON(res, 200, {
        id: 'inst-1',
        name: 'smoke-openvpn',
        service_code: 'openvpn',
        status: 'deleting',
        enabled: false,
        systemd_unit: 'megavpn-openvpn',
      });
      return;
    }

    sendJSON(res, 404, { error: `unexpected request: ${req.method} ${pathname}` });
  };

  return { handler, state };
}

async function testMatrixPlan() {
  await withServer(planHandler, async (baseURL, requests) => {
    const result = await runSmoke(
      ['--matrix', 'node-1', 'smoke.test', '--packs', 'openvpn_tcp_11994,openvpn_udp_1194', '--plan'],
      { MEGAVPN_PUBLIC_BASE_URL: baseURL },
    );
    assert.strictEqual(result.code, 0, result.output);
    assert.match(result.stdout, /RUN\s+openvpn_tcp_11994/);
    assert.match(result.stdout, /RUN\s+openvpn_udp_1194/);
    assert.match(result.stdout, /SKIP\s+xray_nginx_http_edge/);
    assert.ok(!requests.some((request) => request.method !== 'GET'), result.output);
  });
}

async function testUnknownPackFails() {
  await withServer(planHandler, async (baseURL) => {
    const result = await runSmoke(
      ['--matrix', 'node-1', 'smoke.test', '--packs', 'typo_pack', '--plan'],
      { MEGAVPN_PUBLIC_BASE_URL: baseURL },
    );
    assert.notStrictEqual(result.code, 0, result.output);
    assert.match(result.output, /unknown service pack in --packs\/MEGAVPN_SMOKE_PACKS: typo_pack/);
  });
}

async function testStagedBatchPlan() {
  await withServer(planHandler, async (baseURL, requests) => {
    const evidenceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'megavpn-staged-plan.'));
    try {
      const result = await runStagedSmoke(
        ['--plan', '--batches', 'remote_access_l3', '--evidence-root', evidenceRoot, 'node-1', 'smoke.test'],
        { MEGAVPN_PUBLIC_BASE_URL: baseURL },
      );
      assert.strictEqual(result.code, 0, result.output);
      assert.match(result.stdout, /== batch: remote_access_l3 ==/);
      assert.match(result.stdout, /RUN\s+openvpn_tcp_11994/);
      assert.match(result.stdout, /RUN\s+openvpn_udp_1194/);
      assert.match(result.stdout, /RUN\s+wireguard_roadwarrior/);
      assert.ok(!requests.some((request) => request.method !== 'GET'), result.output);

      const summary = JSON.parse(fs.readFileSync(stagedSummaryPath(result.stdout), 'utf8'));
      assert.strictEqual(summary.status, 'planned');
      assert.strictEqual(summary.node_id, 'node-1');
      assert.strictEqual(summary.endpoint_domain, 'smoke.test');
      assert.strictEqual(summary.options.plan_only, true);
      assert.strictEqual(summary.totals.planned, 1);
      assert.strictEqual(summary.totals.failed, 0);
      assert.strictEqual(summary.results.length, 1);
      assert.strictEqual(summary.results[0].batch, 'remote_access_l3');
      assert.strictEqual(summary.results[0].status, 'PLANNED');
      assert.deepStrictEqual(summary.results[0].packs, ['openvpn_tcp_11994', 'openvpn_udp_1194', 'wireguard_roadwarrior']);
    } finally {
      fs.rmSync(evidenceRoot, { recursive: true, force: true });
    }
  });
}

async function testStagedBatchPortOverlapFailsClosed() {
  await withServer(planHandler, async (baseURL, requests) => {
    const result = await runStagedSmoke(
      ['--batches', 'proxy_access,xray_reality', 'node-1', 'smoke.test'],
      { MEGAVPN_PUBLIC_BASE_URL: baseURL },
    );
    assert.notStrictEqual(result.code, 0, result.output);
    assert.match(result.output, /selected staged batches contain endpoint-port overlaps/);
    assert.match(result.output, /443\/tcp/);
    assert.match(result.output, /enable --cleanup/);
    assert.strictEqual(requests.length, 0, result.output);
  });
}

async function testProvisionSuccessCleanup() {
  const { handler, state } = createLifecycleHandler({ artifactsReady: true });
  await withServer(handler, async (baseURL) => {
    const evidenceDir = fs.mkdtempSync(path.join(os.tmpdir(), 'megavpn-smoke-evidence.'));
    const result = await runSmoke(
      ['node-1', 'openvpn_tcp_11994', 'vpn.test', 'smoke-openvpn'],
      {
        MEGAVPN_PUBLIC_BASE_URL: baseURL,
        MEGAVPN_SMOKE_CLEANUP: '1',
        MEGAVPN_SMOKE_EVIDENCE_DIR: evidenceDir,
      },
    );
    try {
      assert.strictEqual(result.code, 0, result.output);
      assert.match(result.output, /\[runtime-install\] job=job-install-openvpn/);
      assert.match(result.output, /\[provision-apply\] instance=inst-1 service=openvpn job=job-post-apply/);
      assert.match(result.output, /service-pack smoke succeeded: openvpn_tcp_11994/);
      assert.strictEqual(state.createRequests, 1);
      assert.strictEqual(state.clientDeleted, true);
      assert.strictEqual(state.instanceDeleted, true);

      const evidencePath = path.join(evidenceDir, 'smoke-openvpn.json');
      const evidence = JSON.parse(fs.readFileSync(evidencePath, 'utf8'));
      assert.strictEqual(evidence.status, 'succeeded');
      assert.strictEqual(evidence.input.pack_key, 'openvpn_tcp_11994');
      assert.strictEqual(evidence.created_instances[0].id, 'inst-1');
      assert.strictEqual(evidence.runtime_install_jobs[0].id, 'job-install-openvpn');
      assert.strictEqual(evidence.applied_instances[0].id, 'inst-1');
      assert.ok(evidence.runtime_states.some((stateRow) => stateRow.instance_id === 'inst-1' && stateRow.runtime_status === 'active'));
      assert.strictEqual(evidence.client.id, 'client-1');
      assert.strictEqual(evidence.provision_result.id, 'job-provision');
      assert.strictEqual(evidence.service_accesses[0].id, 'access-1');
      assert.strictEqual(evidence.service_accesses[0].status, 'active');
      assert.strictEqual(evidence.artifacts[0].id, 'artifact-1');
    } finally {
      fs.rmSync(evidenceDir, { recursive: true, force: true });
    }
  });
}

async function testMatrixRunSummaryEvidence() {
  const { handler, state } = createLifecycleHandler({ artifactsReady: true });
  await withServer(handler, async (baseURL) => {
    const evidenceDir = fs.mkdtempSync(path.join(os.tmpdir(), 'megavpn-smoke-matrix.'));
    const result = await runSmoke(
      ['--matrix', 'node-1', 'smoke.test', '--packs', 'openvpn_tcp_11994'],
      {
        MEGAVPN_PUBLIC_BASE_URL: baseURL,
        MEGAVPN_SMOKE_EVIDENCE_DIR: evidenceDir,
      },
    );
    try {
      assert.strictEqual(result.code, 0, result.output);
      assert.match(result.output, /matrix summary:/);
      assert.match(result.output, /\[evidence\] wrote matrix summary/);
      assert.strictEqual(state.createRequests, 1);

      const summaryPath = path.join(evidenceDir, '_matrix-summary.json');
      const summary = JSON.parse(fs.readFileSync(summaryPath, 'utf8'));
      assert.strictEqual(summary.status, 'succeeded');
      assert.strictEqual(summary.input.node_id, 'node-1');
      assert.strictEqual(summary.input.endpoint_domain, 'smoke.test');
      assert.strictEqual(summary.totals.ok, 1);
      assert.strictEqual(summary.totals.failed, 0);
      assert.strictEqual(summary.results.length, 4);
      const selected = summary.results.find((row) => row.pack_key === 'openvpn_tcp_11994');
      assert.ok(selected, 'selected pack result is missing');
      assert.strictEqual(selected.status, 'OK');
      assert.ok(selected.evidence_file && fs.existsSync(selected.evidence_file));
      const perPackEvidence = JSON.parse(fs.readFileSync(selected.evidence_file, 'utf8'));
      assert.strictEqual(perPackEvidence.input.pack_key, 'openvpn_tcp_11994');

      const reportResult = await runEvidenceReport([
        '--require-pack',
        'openvpn_tcp_11994',
        summaryPath,
      ]);
      assert.strictEqual(reportResult.code, 0, reportResult.output);
      assert.match(reportResult.stdout, /Service-Pack Evidence Report/);
      assert.match(reportResult.stdout, /\| Status \| Pack \| Instances \| Runtime \| Accesses \| Artifacts \| Evidence \| Issue \|/);
      assert.match(reportResult.stdout, /openvpn_tcp_11994/);
      assert.match(reportResult.stdout, /ok 1\/1/);
      assert.match(reportResult.stdout, /Report status: `succeeded`/);

      const missingRequired = await runEvidenceReport([
        '--require-pack',
        'xray_nginx_http_edge',
        summaryPath,
      ]);
      assert.notStrictEqual(missingRequired.code, 0, missingRequired.output);
      assert.match(missingRequired.stdout, /required pack did not produce valid OK evidence: xray_nginx_http_edge/);
    } finally {
      fs.rmSync(evidenceDir, { recursive: true, force: true });
    }
  });
}

async function testProvisionFailureCleanup() {
  const { handler, state } = createLifecycleHandler({ artifactsReady: false });
  await withServer(handler, async (baseURL) => {
    const result = await runSmoke(
      ['node-1', 'openvpn_tcp_11994', 'vpn.test', 'smoke-openvpn'],
      {
        MEGAVPN_PUBLIC_BASE_URL: baseURL,
        MEGAVPN_SMOKE_CLEANUP_ON_FAILURE: '1',
      },
    );
    assert.notStrictEqual(result.code, 0, result.output);
    assert.match(result.output, /client provisioning did not create a ready artifact for every selected service access/);
    assert.match(result.output, /"missing_ready_artifact_access_ids"/);
    assert.match(result.output, /"access-1"/);
    assert.match(result.output, /MEGAVPN_SMOKE_CLEANUP_ON_FAILURE=1/);
    assert.match(result.output, /\[cleanup\] completed/);
    assert.strictEqual(state.clientDeleted, true);
    assert.strictEqual(state.instanceDeleted, true);
  });
}

async function main() {
  const tests = [
    ['matrix plan filters', testMatrixPlan],
    ['unknown pack fail-fast', testUnknownPackFails],
    ['staged batch plan', testStagedBatchPlan],
    ['staged batch port overlap fail-closed', testStagedBatchPortOverlapFailsClosed],
    ['provision success cleanup', testProvisionSuccessCleanup],
    ['matrix run summary evidence', testMatrixRunSummaryEvidence],
    ['provision failure cleanup', testProvisionFailureCleanup],
  ];

  for (const [name, test] of tests) {
    await test();
    console.log(`service-pack smoke regression ok: ${name}`);
  }
}

main().catch((err) => {
  console.error(err.stack || err);
  process.exit(1);
});
