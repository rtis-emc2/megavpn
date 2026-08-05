#!/usr/bin/env node

import { spawnSync } from 'node:child_process';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, extname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const frontendDir = join(rootDir, 'frontend');
const packageJSON = JSON.parse(readFileSync(join(frontendDir, 'package.json'), 'utf8'));
const packageLock = JSON.parse(readFileSync(join(frontendDir, 'package-lock.json'), 'utf8'));

const routerVersion = '7.18.2';
const advisorySource = 1124282;
const advisoryURL = 'https://github.com/advisories/GHSA-qwww-vcr4-c8h2';

function fail(message) {
  process.stderr.write(`frontend dependency audit failed: ${message}\n`);
  process.exit(1);
}

if (packageJSON.dependencies?.['react-router-dom'] !== routerVersion
    || packageLock.packages?.['node_modules/react-router-dom']?.version !== routerVersion
    || packageLock.packages?.['node_modules/react-router']?.version !== routerVersion) {
  fail(`the reviewed React Router exception requires exact version ${routerVersion}`);
}

const forbiddenRouterPatterns = [
  /from\s+['"]react-router(?:-dom)?\/(?:rsc|server)[^'"]*['"]/,
  /\b(?:RSCStaticRouter|ServerRouter|HydratedRouter|createCallServer|createFromReadableStream|createStaticHandler|matchRSCServerRequest|routeRSCServerRequest|decodeAction|decodeFormState|RouterProvider|createBrowserRouter)\b/,
];

function sourceFiles(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return sourceFiles(path);
    }
    return ['.js', '.jsx', '.mjs', '.ts', '.tsx'].includes(extname(entry.name)) ? [path] : [];
  });
}

for (const path of sourceFiles(join(frontendDir, 'src'))) {
  const source = readFileSync(path, 'utf8');
  for (const pattern of forbiddenRouterPatterns) {
    if (pattern.test(source)) {
      fail(`RSC, server-router or data-router API detected in ${path.slice(rootDir.length + 1)}`);
    }
  }
}

const routerEntry = readFileSync(join(frontendDir, 'src/app/router.tsx'), 'utf8');
if (!routerEntry.includes("import { BrowserRouter } from 'react-router-dom'") || !routerEntry.includes('<BrowserRouter>')) {
  fail('the reviewed exception only covers the declarative BrowserRouter SPA architecture');
}

const npmExecPath = process.env.npm_execpath;
if (!npmExecPath) {
  fail('npm_execpath is unavailable; run this guard through npm run audit:ci');
}

const audit = spawnSync(process.execPath, [npmExecPath, 'audit', '--audit-level=high', '--json'], {
  cwd: frontendDir,
  encoding: 'utf8',
  maxBuffer: 16 * 1024 * 1024,
});
if (audit.error) {
  fail(`unable to execute npm audit: ${audit.error.message}`);
}

let report;
try {
  report = JSON.parse(audit.stdout || '{}');
} catch (error) {
  fail(`npm audit returned invalid JSON: ${error.message}`);
}
if (report.error) {
  fail(`npm audit error: ${report.error.summary || report.error.code || 'unknown error'}`);
}

const vulnerabilities = report.vulnerabilities || {};
const unexpected = [];
let reviewedAdvisoryPresent = false;

for (const [name, vulnerability] of Object.entries(vulnerabilities)) {
  if (!['high', 'critical'].includes(vulnerability.severity)) {
    continue;
  }
  if (name === 'react-router') {
    const advisories = Array.isArray(vulnerability.via)
      ? vulnerability.via.filter((entry) => typeof entry === 'object')
      : [];
    const isReviewed = advisories.length === 1
      && advisories[0].source === advisorySource
      && advisories[0].url === advisoryURL
      && advisories[0].severity === 'high';
    if (isReviewed) {
      reviewedAdvisoryPresent = true;
      continue;
    }
  }
  if (name === 'react-router-dom'
      && Array.isArray(vulnerability.via)
      && vulnerability.via.length === 1
      && vulnerability.via[0] === 'react-router') {
    continue;
  }
  unexpected.push(`${name}:${vulnerability.severity}`);
}

if (unexpected.length > 0) {
  fail(`unreviewed high/critical vulnerabilities: ${unexpected.join(', ')}`);
}
if (audit.status !== 0 && !reviewedAdvisoryPresent) {
  fail(`npm audit exited with ${audit.status} without the reviewed advisory`);
}

if (reviewedAdvisoryPresent) {
  process.stdout.write(
    `frontend dependency audit passed with reviewed non-applicable RSC advisory ${advisoryURL}; `
      + 'the console is guarded as a declarative BrowserRouter SPA\n',
  );
} else {
  process.stdout.write('frontend dependency audit passed with no high/critical vulnerabilities\n');
}
