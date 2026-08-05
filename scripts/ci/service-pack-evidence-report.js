#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');

function usage() {
  console.log(`Usage:
  scripts/service-pack-evidence-report.js [options] <matrix-summary.json>

Options:
  --require-pack key1,key2  Require these pack keys to have OK evidence; can be repeated.
  --require-no-skips        Treat any SKIPPED matrix row as a failure.
  --json                    Print machine-readable report JSON.
  --help                    Show this help.

The script validates service-pack smoke evidence produced by
MEGAVPN_SMOKE_EVIDENCE_DIR or MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE.
`);
}

function parseArgs(argv) {
  const options = {
    summaryFile: '',
    requiredPacks: new Set(),
    requireNoSkips: false,
    json: false,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--help' || arg === '-h') {
      usage();
      process.exit(0);
    }
    if (arg === '--json') {
      options.json = true;
      continue;
    }
    if (arg === '--require-no-skips') {
      options.requireNoSkips = true;
      continue;
    }
    if (arg === '--require-pack') {
      const value = argv[i + 1] || '';
      if (!value || value.startsWith('-')) {
        throw new Error('--require-pack requires a comma-separated pack list');
      }
      for (const item of splitList(value)) {
        options.requiredPacks.add(item);
      }
      i += 1;
      continue;
    }
    if (arg.startsWith('--require-pack=')) {
      for (const item of splitList(arg.slice('--require-pack='.length))) {
        options.requiredPacks.add(item);
      }
      continue;
    }
    if (arg.startsWith('-')) {
      throw new Error(`unknown option: ${arg}`);
    }
    if (options.summaryFile) {
      throw new Error(`unexpected extra argument: ${arg}`);
    }
    options.summaryFile = arg;
  }

  if (!options.summaryFile) {
    throw new Error('matrix summary file is required');
  }
  return options;
}

function splitList(value) {
  return value
    .split(/[,\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asString(value) {
  return typeof value === 'string' ? value : '';
}

function resolveEvidencePath(summaryFile, evidenceFile) {
  if (!evidenceFile) {
    return '';
  }
  if (path.isAbsolute(evidenceFile)) {
    return evidenceFile;
  }

  const candidates = [
    path.resolve(process.cwd(), evidenceFile),
    path.resolve(path.dirname(summaryFile), evidenceFile),
    path.resolve(path.dirname(summaryFile), path.basename(evidenceFile)),
  ];
  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }
  return candidates[0];
}

function latestRuntimeByInstance(runtimeStates) {
  const latest = new Map();
  for (const state of asArray(runtimeStates)) {
    const id = asString(state.instance_id);
    if (!id) {
      continue;
    }
    latest.set(id, state);
  }
  return latest;
}

function readyRuntime(state, requireAgentReport) {
  if (!state) {
    return false;
  }
  if (state.runtime_status !== 'active') {
    return false;
  }
  if (state.health_status !== 'healthy') {
    return false;
  }
  if (state.drift_status !== 'in_sync') {
    return false;
  }
  if (requireAgentReport && !state.agent_reported_at) {
    return false;
  }
  return true;
}

function expectedArtifactTypes(serviceCode) {
  switch (serviceCode) {
    case 'openvpn':
      return ['ovpn'];
    case 'wireguard':
      return ['wg_conf'];
    case 'xray-core':
    case 'xray':
      return ['vless_url'];
    case 'mtproto':
      return ['mtproto_url'];
    case 'http_proxy':
      return ['http_proxy_bundle'];
    case 'shadowsocks':
      return ['ss_url'];
    case 'ipsec':
      return ['ipsec_bundle'];
    default:
      return [];
  }
}

function validateEvidence(summaryFile, row) {
  const issues = [];
  const evidencePath = resolveEvidencePath(summaryFile, row.evidence_file);
  const report = {
    pack_key: row.pack_key || '',
    status: row.status || '',
    base_name: row.base_name || '',
    endpoint_host: row.endpoint_host || '',
    evidence_file: row.evidence_file || null,
    resolved_evidence_file: evidencePath || null,
    instances: { created: 0, applied: 0 },
    runtime: { status: 'not_checked', ready: 0, expected: 0 },
    service_accesses: { status: 'not_checked', active: 0, expected: 0 },
    artifacts: { status: 'not_checked', ready: 0, total: 0 },
    issues,
  };

  if (!evidencePath) {
    issues.push('OK row has no evidence_file');
    return report;
  }
  if (!fs.existsSync(evidencePath)) {
    issues.push(`evidence file does not exist: ${evidencePath}`);
    return report;
  }

  let evidence;
  try {
    evidence = readJSON(evidencePath);
  } catch (err) {
    issues.push(`evidence file is not valid JSON: ${err.message}`);
    return report;
  }

  if (evidence.status !== 'succeeded') {
    issues.push(`evidence status is ${evidence.status || 'missing'}, expected succeeded`);
  }
  const input = evidence.input || {};
  if (input.pack_key && input.pack_key !== row.pack_key) {
    issues.push(`evidence pack_key=${input.pack_key} does not match summary pack_key=${row.pack_key}`);
  }
  if (row.base_name && input.base_name && input.base_name !== row.base_name) {
    issues.push(`evidence base_name=${input.base_name} does not match summary base_name=${row.base_name}`);
  }
  if (row.endpoint_host && input.endpoint_host && input.endpoint_host !== row.endpoint_host) {
    issues.push(`evidence endpoint_host=${input.endpoint_host} does not match summary endpoint_host=${row.endpoint_host}`);
  }

  const createdInstances = asArray(evidence.created_instances);
  const appliedInstances = asArray(evidence.applied_instances);
  const createdIDs = new Set(createdInstances.map((instance) => asString(instance.id)).filter(Boolean));
  const appliedIDs = new Set(appliedInstances.map((instance) => asString(instance.id)).filter(Boolean));
  const appliedByID = new Map(appliedInstances
    .filter((instance) => asString(instance.id))
    .map((instance) => [asString(instance.id), instance]));
  report.instances.created = createdIDs.size;
  report.instances.applied = appliedIDs.size;

  if (createdIDs.size === 0) {
    issues.push('evidence contains no created_instances');
  }
  for (const id of createdIDs) {
    if (!appliedIDs.has(id)) {
      issues.push(`created instance ${id} is missing from applied_instances`);
    }
  }

  const runtimeCheck = input.smoke_runtime_check !== '0';
  if (runtimeCheck) {
    const requireAgentReport = input.smoke_require_agent_report === '1';
    const latest = latestRuntimeByInstance(evidence.runtime_states);
    report.runtime.expected = createdIDs.size;
    for (const id of createdIDs) {
      if (readyRuntime(latest.get(id), requireAgentReport)) {
        report.runtime.ready += 1;
      } else {
        const state = latest.get(id);
        const status = state
          ? `${state.runtime_status || 'unknown'}/${state.health_status || 'unknown'}/${state.drift_status || 'unknown'}`
          : 'missing';
        issues.push(`runtime for instance ${id} is not ready: ${status}`);
      }
    }
    report.runtime.status = report.runtime.ready === report.runtime.expected ? 'ok' : 'failed';
  } else {
    report.runtime.status = 'skipped';
  }

  const smokeProvision = input.smoke_provision !== '0';
  if (smokeProvision && evidence.client) {
    if (evidence.provision_result && evidence.provision_result.status !== 'succeeded') {
      issues.push(`client provision job status is ${evidence.provision_result.status || 'missing'}`);
    }
    const serviceAccesses = asArray(evidence.service_accesses);
    const artifacts = asArray(evidence.artifacts);
    const readyArtifacts = artifacts.filter((artifact) => (
      artifact.status === 'ready'
      && Number(artifact.size_bytes || 0) > 0
      && Boolean(artifact.service_access_id)
    ));
    report.service_accesses.expected = serviceAccesses.length;
    report.service_accesses.active = serviceAccesses.filter((access) => access.status === 'active').length;
    report.service_accesses.status = serviceAccesses.length > 0 && report.service_accesses.active === serviceAccesses.length ? 'ok' : 'failed';
    report.artifacts.total = artifacts.length;
    report.artifacts.ready = 0;

    if (serviceAccesses.length === 0) {
      issues.push('client provisioning evidence contains no service_accesses');
    }
    for (const access of serviceAccesses) {
      const accessID = asString(access.id);
      const instanceID = asString(access.instance_id);
      if (!accessID) {
        issues.push(`service access for instance ${instanceID || 'unknown'} is missing id`);
        continue;
      }
      if (access.status !== 'active') {
        issues.push(`service access ${accessID} for instance ${instanceID || 'unknown'} is ${access.status || 'missing'}, expected active`);
      }
      const accessArtifacts = readyArtifacts.filter((artifact) => artifact.service_access_id === accessID);
      if (accessArtifacts.length === 0) {
        issues.push(`service access ${accessID} has no ready artifact`);
        continue;
      }
      report.artifacts.ready += 1;
      const instance = appliedByID.get(instanceID) || {};
      const expectedTypes = expectedArtifactTypes(instance.service_code);
      for (const expectedType of expectedTypes) {
        if (!accessArtifacts.some((artifact) => artifact.artifact_type === expectedType)) {
          issues.push(`service access ${accessID} expected ${expectedType} artifact for ${instance.service_code}, got ${accessArtifacts.map((artifact) => artifact.artifact_type || 'unknown').join(',')}`);
        }
      }
    }
    report.artifacts.status = serviceAccesses.length > 0 && report.artifacts.ready === serviceAccesses.length ? 'ok' : 'failed';
  } else {
    report.service_accesses.status = 'skipped';
    report.artifacts.status = 'skipped';
  }

  return report;
}

function buildReport(summaryFile, options) {
  const summary = readJSON(summaryFile);
  const rows = asArray(summary.results);
  const reports = [];
  const issues = [];
  const requiredPacks = new Set(options.requiredPacks);
  const seenOK = new Set();

  if (summary.status && summary.status !== 'succeeded') {
    issues.push(`matrix summary status is ${summary.status}`);
  }

  for (const row of rows) {
    const status = row.status || '';
    if (status === 'OK') {
      const item = validateEvidence(summaryFile, row);
      reports.push(item);
      if (item.issues.length > 0) {
        issues.push(`${row.pack_key}: ${item.issues.join('; ')}`);
      } else {
        seenOK.add(row.pack_key);
      }
      continue;
    }

    const item = {
      pack_key: row.pack_key || '',
      status,
      base_name: row.base_name || '',
      endpoint_host: row.endpoint_host || '',
      evidence_file: row.evidence_file || null,
      resolved_evidence_file: null,
      instances: { created: 0, applied: 0 },
      runtime: { status: 'not_checked', ready: 0, expected: 0 },
      service_accesses: { status: 'not_checked', active: 0, expected: 0 },
      artifacts: { status: 'not_checked', ready: 0, total: 0 },
      issues: [],
    };

    if (status === 'FAILED') {
      item.issues.push(row.reason || 'matrix row failed');
      issues.push(`${row.pack_key}: ${item.issues[0]}`);
    } else if (status === 'SKIPPED' && (options.requireNoSkips || requiredPacks.has(row.pack_key))) {
      item.issues.push(row.reason || 'required matrix row was skipped');
      issues.push(`${row.pack_key}: ${item.issues[0]}`);
    }
    reports.push(item);
  }

  for (const packKey of requiredPacks) {
    if (!seenOK.has(packKey)) {
      issues.push(`required pack did not produce valid OK evidence: ${packKey}`);
    }
  }

  return {
    status: issues.length === 0 ? 'succeeded' : 'failed',
    summary_file: summaryFile,
    generated_at: new Date().toISOString(),
    matrix: {
      status: summary.status || 'unknown',
      generated_at: summary.generated_at || null,
      input: summary.input || {},
      totals: summary.totals || {},
    },
    required_packs: Array.from(requiredPacks).sort(),
    require_no_skips: options.requireNoSkips,
    results: reports,
    issues,
  };
}

function escapeCell(value) {
  return String(value ?? '')
    .replace(/\|/g, '\\|')
    .replace(/\n/g, '<br>');
}

function renderMarkdown(report) {
  const totals = report.matrix.totals || {};
  const lines = [];
  lines.push('# Service-Pack Evidence Report');
  lines.push('');
  lines.push(`- Summary file: \`${report.summary_file}\``);
  lines.push(`- Matrix status: \`${report.matrix.status}\``);
  lines.push(`- Report status: \`${report.status}\``);
  lines.push(`- Totals: ok=${totals.ok ?? 0} failed=${totals.failed ?? 0} skipped=${totals.skipped ?? 0} selected=${totals.selected ?? 0}`);
  if (report.required_packs.length > 0) {
    lines.push(`- Required packs: \`${report.required_packs.join(',')}\``);
  }
  lines.push('');
  lines.push('| Status | Pack | Instances | Runtime | Accesses | Artifacts | Evidence | Issue |');
  lines.push('| --- | --- | ---: | --- | --- | --- | --- | --- |');
  for (const item of report.results) {
    const runtime = `${item.runtime.status} ${item.runtime.ready}/${item.runtime.expected}`;
    const accesses = `${item.service_accesses.status} ${item.service_accesses.active}/${item.service_accesses.expected}`;
    const artifacts = `${item.artifacts.status} ${item.artifacts.ready}/${item.artifacts.total}`;
    const issue = item.issues.length > 0 ? item.issues.join('<br>') : 'ok';
    lines.push(`| ${escapeCell(item.status)} | ${escapeCell(item.pack_key)} | ${item.instances.created}/${item.instances.applied} | ${escapeCell(runtime)} | ${escapeCell(accesses)} | ${escapeCell(artifacts)} | ${escapeCell(item.resolved_evidence_file || item.evidence_file || '-')} | ${escapeCell(issue)} |`);
  }
  if (report.issues.length > 0) {
    lines.push('');
    lines.push('## Blocking Issues');
    lines.push('');
    for (const issue of report.issues) {
      lines.push(`- ${issue}`);
    }
  }
  lines.push('');
  return lines.join('\n');
}

function main() {
  let options;
  try {
    options = parseArgs(process.argv.slice(2));
    const summaryFile = path.resolve(options.summaryFile);
    const report = buildReport(summaryFile, options);
    if (options.json) {
      console.log(JSON.stringify(report, null, 2));
    } else {
      process.stdout.write(renderMarkdown(report));
    }
    if (report.status !== 'succeeded') {
      process.exitCode = 1;
    }
  } catch (err) {
    console.error(err.message || String(err));
    process.exitCode = 2;
  }
}

main();
