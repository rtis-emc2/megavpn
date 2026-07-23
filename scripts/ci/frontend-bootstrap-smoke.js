#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('assert');
const vm = require('vm');

const rootDir = path.resolve(__dirname, '..', '..');
const indexPath = path.join(rootDir, 'web', 'index.html');
const indexHTML = fs.readFileSync(indexPath, 'utf8');

class ClassList {
  constructor() {
    this.values = new Set();
  }

  add(...items) {
    items.filter(Boolean).forEach((item) => this.values.add(String(item)));
  }

  remove(...items) {
    items.filter(Boolean).forEach((item) => this.values.delete(String(item)));
  }

  toggle(item, force) {
    const value = String(item);
    if (force === true) {
      this.values.add(value);
      return true;
    }
    if (force === false) {
      this.values.delete(value);
      return false;
    }
    if (this.values.has(value)) {
      this.values.delete(value);
      return false;
    }
    this.values.add(value);
    return true;
  }

  contains(item) {
    return this.values.has(String(item));
  }
}

const elements = new Map();

class Element {
  constructor(id = '', tagName = 'div') {
    this.id = id;
    this.tagName = String(tagName || 'div').toUpperCase();
    this.children = [];
    this.dataset = {};
    this.style = {};
    this.attributes = {};
    this.classList = new ClassList();
    this.hidden = false;
    this.disabled = false;
    this.value = '';
    this.textContent = '';
    this.scrollTop = 0;
    this._innerHTML = '';
  }

  set innerHTML(value) {
    this._innerHTML = String(value ?? '');
    registerElementIDs(this._innerHTML);
  }

  get innerHTML() {
    return this._innerHTML;
  }

  addEventListener() {}

  removeEventListener() {}

  appendChild(child) {
    this.children.push(child);
    child.parentElement = this;
    return child;
  }

  insertBefore(child) {
    this.children.push(child);
    child.parentElement = this;
    return child;
  }

  removeChild(child) {
    this.children = this.children.filter((item) => item !== child);
    child.parentElement = null;
    return child;
  }

  remove() {}

  focus() {}

  closest() {
    return null;
  }

  matches() {
    return false;
  }

  querySelector(selector) {
    return document.querySelector(selector);
  }

  querySelectorAll() {
    return [];
  }

  setAttribute(name, value) {
    const key = String(name);
    const stringValue = String(value);
    this.attributes[key] = stringValue;
    if (key === 'id') {
      this.id = stringValue;
      elements.set(stringValue, this);
    }
    if (key.startsWith('data-')) {
      const dataKey = key
        .slice(5)
        .replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
      this.dataset[dataKey] = stringValue;
    }
  }

  getAttribute(name) {
    return this.attributes[String(name)] ?? null;
  }

  hasAttribute(name) {
    return Object.prototype.hasOwnProperty.call(this.attributes, String(name));
  }

  insertAdjacentHTML(_position, html) {
    this.innerHTML += String(html ?? '');
  }
}

function elementForID(id) {
  const key = String(id);
  if (!elements.has(key)) {
    elements.set(key, new Element(key));
  }
  return elements.get(key);
}

function registerElementIDs(html) {
  const pattern = /\sid=(["'])([^"']+)\1/g;
  let match;
  while ((match = pattern.exec(html)) !== null) {
    elementForID(match[2]);
  }
}

[
  'authGate',
  'appShell',
  'refreshBtn',
  'closeModalBtn',
  'modalBackdrop',
  'modalTitle',
  'modalEyebrow',
  'modalBody',
  'content',
  'nav',
  'readyPill',
  'apiBaseLabel',
  'releaseLabel',
  'notice',
  'pageTitle',
  'authSlot',
].forEach(elementForID);

class DocumentStub {
  constructor() {
    this.hidden = false;
    this.readyState = 'complete';
    this.body = elementForID('body');
  }

  getElementById(id) {
    return elementForID(id);
  }

  createElement(tagName) {
    return new Element('', tagName);
  }

  querySelector(selector) {
    const value = String(selector || '');
    if (value.startsWith('#')) return elementForID(value.slice(1));
    return elementForID(`selector:${value}`);
  }

  querySelectorAll() {
    return [];
  }

  addEventListener() {}

  removeEventListener() {}
}

const document = new DocumentStub();

function createStorage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(String(key)) ?? null,
    setItem: (key, value) => values.set(String(key), String(value)),
    removeItem: (key) => values.delete(String(key)),
    clear: () => values.clear(),
  };
}

function jsonResponse(status, payload) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: () => 'application/json' },
    json: async () => payload,
    text: async () => JSON.stringify(payload),
  };
}

async function fetchMock(input) {
  const value = String(input || '');
  const url = value.startsWith('http://') || value.startsWith('https://')
    ? new URL(value)
    : new URL(value, 'https://control.example.test');
  if (url.pathname === '/api/v1/auth/me') {
    return jsonResponse(401, { error: 'unauthorized' });
  }
  if (url.pathname === '/api/v1/ready') {
    return jsonResponse(200, { status: 'ready' });
  }
  if (url.pathname === '/api/v1/version') {
    return jsonResponse(200, { version: 'frontend-bootstrap-smoke' });
  }
  return jsonResponse(200, {});
}

const pendingErrors = [];
process.on('unhandledRejection', (err) => pendingErrors.push(err));

const windowObject = {
  document,
  localStorage: createStorage(),
  sessionStorage: createStorage(),
  location: {
    href: 'https://control.example.test/',
    origin: 'https://control.example.test',
    search: '',
    reload: () => {},
  },
  history: { replaceState: () => {} },
  navigator: { userAgent: 'megavpn-frontend-bootstrap-smoke' },
  console,
  URL,
  URLSearchParams,
  AbortController,
  FormData: class FormData {},
  fetch: fetchMock,
  setTimeout,
  clearTimeout,
  setInterval: () => 0,
  clearInterval: () => {},
  requestAnimationFrame: (callback) => setTimeout(callback, 0),
  addEventListener: () => {},
  removeEventListener: () => {},
  open: () => null,
  confirm: () => true,
  Element,
  HTMLElement: Element,
  Document: DocumentStub,
  HTMLTableElement: Element,
  HTMLFormElement: Element,
  HTMLInputElement: Element,
  HTMLTextAreaElement: Element,
  MutationObserver: class MutationObserver {
    observe() {}
    disconnect() {}
  },
  crypto: {
    getRandomValues: (array) => array.fill(1),
  },
};
windowObject.window = windowObject;

const context = vm.createContext({
  window: windowObject,
  document,
  localStorage: windowObject.localStorage,
  sessionStorage: windowObject.sessionStorage,
  location: windowObject.location,
  history: windowObject.history,
  navigator: windowObject.navigator,
  console,
  URL,
  URLSearchParams,
  AbortController,
  FormData: windowObject.FormData,
  fetch: fetchMock,
  setTimeout,
  clearTimeout,
  setInterval: windowObject.setInterval,
  clearInterval: windowObject.clearInterval,
  requestAnimationFrame: windowObject.requestAnimationFrame,
  Element,
  HTMLElement: Element,
  Document: DocumentStub,
  HTMLTableElement: Element,
  HTMLFormElement: Element,
  HTMLInputElement: Element,
  HTMLTextAreaElement: Element,
  MutationObserver: windowObject.MutationObserver,
});

const scripts = [...indexHTML.matchAll(/<script\s+[^>]*src="([^"]+)"/g)]
  .map((match) => match[1].split('?')[0])
  .filter((src) => src.endsWith('.js'));

if (!scripts.length) {
  throw new Error('web/index.html does not reference frontend JavaScript assets');
}

async function main() {
  for (const src of scripts) {
    const filePath = path.join(rootDir, 'web', src.replace(/^\.\//, ''));
    const source = fs.readFileSync(filePath, 'utf8');
    vm.runInContext(source, context, { filename: filePath });
  }

  await new Promise((resolve) => setImmediate(resolve));
  if (pendingErrors.length) {
    const messages = pendingErrors.map((err) => err && err.stack ? err.stack : String(err)).join('\n');
    throw new Error(`frontend bootstrap emitted async errors:\n${messages}`);
  }
  if (!windowObject.__MegaVPNBootReady) {
    throw new Error('frontend bootstrap did not reach __MegaVPNBootReady');
  }

  const routerState = {
    page: 'instances',
    instancesView: 'list',
    authUser: { id: 'operator' },
  };
  const router = windowObject.MegaVPNAppRouter.create({
    state: routerState,
    el: elementForID,
    setShellMode: () => {},
    renderNav: () => {},
    renderAuthSlot: () => {},
    renderNotice: () => {},
    setTitle: () => {},
    escapeHTML: (value) => String(value ?? ''),
    authWorkflows: {},
    nodeWorkflows: {},
    nodeMapPage: { render: () => {} },
    instanceWorkflows: { renderInstanceManagePage: () => {} },
    firewallPage: { render: () => {} },
    trafficPage: { render: () => {} },
  });
  assert.strictEqual(router.autoRefreshEnabledForCurrentPage(), true, 'instance list should auto-refresh');
  routerState.instancesView = 'create-pack';
  assert.strictEqual(router.autoRefreshEnabledForCurrentPage(), false, 'service pack form must not auto-refresh');
  routerState.instancesView = 'manual';
  assert.strictEqual(router.autoRefreshEnabledForCurrentPage(), false, 'manual instance form must not auto-refresh');

  const resolveImportFormat = windowObject.MegaVPNExternalEgressPage?.resolveImportFormat;
  assert.strictEqual(typeof resolveImportFormat, 'function', 'external egress import format helper must be exported');
  assert.strictEqual(resolveImportFormat('vless', '{"address":"vpn.example.com"}'), 'json');
  assert.strictEqual(resolveImportFormat('vless', '', 'json'), 'json', 'profile edit must retain its stored import format');
  assert.strictEqual(resolveImportFormat('l2tp_ipsec', 'server=vpn.example.com'), 'key_value');
  assert.strictEqual(resolveImportFormat('socks5'), 'structured', 'planned structured protocols must not be rewritten as URL imports');

  const executionTargetForJobType = windowObject.MegaVPNJobWorkflows?.executionTargetForJobType;
  assert.strictEqual(typeof executionTargetForJobType, 'function', 'job execution target helper must be exported');
  assert.strictEqual(executionTargetForJobType('node.external_egress.apply'), 'agent');
  assert.strictEqual(executionTargetForJobType('node.external_egress.probe'), 'agent');
  assert.strictEqual(executionTargetForJobType('node.external_egress.cleanup'), 'agent');
  assert.strictEqual(executionTargetForJobType('instance.apply'), 'agent');
  assert.strictEqual(executionTargetForJobType('node.bootstrap'), 'worker');
  assert.strictEqual(executionTargetForJobType('client.provision'), 'worker');
  const isCancellableJobStatus = windowObject.MegaVPNJobWorkflows?.isCancellableJobStatus;
  assert.strictEqual(typeof isCancellableJobStatus, 'function', 'job cancellation status helper must be exported');
  assert.strictEqual(isCancellableJobStatus('queued'), true);
  assert.strictEqual(isCancellableJobStatus('retrying'), true);
  assert.strictEqual(isCancellableJobStatus('running'), false, 'running jobs must not be cancelled from the queue UI');
  assert.strictEqual(isCancellableJobStatus('succeeded'), false);

  const queuedAt = '2026-07-23T11:18:17Z';
  const cancelledJobRequests = [];
  const jobWorkflowState = {
    jobsTab: 'active',
    jobsSearch: '',
    jobsSort: 'newest',
    jobs: [{
      id: 'a000ffe4-f450-486f-b393-93cbf856b3c3',
      type: 'node.external_egress.apply',
      scope_type: 'external_egress',
      scope_id: '38fdafed-754e-4e5e-8bde-f546e05674f3',
      node_id: 'node-3',
      status: 'queued',
      payload: {},
      result: {},
      created_at: queuedAt,
    }],
    nodes: [{
      id: 'node-3',
      name: 'ingress-node3',
      status: 'online',
      agent_status: 'online',
      agent_last_seen_at: '2026-07-23T11:16:55Z',
    }],
  };
  const jobWorkflows = windowObject.MegaVPNJobWorkflows.create({
    state: jobWorkflowState,
    setTitle: () => {},
    el: elementForID,
    requestJSON: async (requestPath, options = {}) => {
      if (/\/api\/v1\/jobs\/[^/]+\/cancel$/.test(requestPath)) {
        cancelledJobRequests.push({ requestPath, options });
        return { status: 'cancelled' };
      }
      if (requestPath === '/api/v1/jobs?limit=50') return jobWorkflowState.jobs;
      return {};
    },
    fetchJSON: async () => ({}),
    statusTag: () => '',
    escapeHTML: (value) => String(value ?? ''),
    formatDate: (value) => String(value ?? ''),
    renderActionResponse: () => '',
    stringValue: (value) => String(value ?? ''),
    hasPermission: () => true,
  });
  jobWorkflows.renderJobs();
  assert.match(
    elementForID('content').innerHTML,
    /waiting for node agent; no agent sync after job was queued/,
    'queued external egress jobs must report the missing agent sync',
  );
  assert.doesNotMatch(
    elementForID('content').innerHTML,
    /waiting for control-plane worker/,
    'external egress jobs must never be attributed to the control-plane worker',
  );
  assert.match(
    elementForID('content').innerHTML,
    /data-job-cancel-id="a000ffe4-f450-486f-b393-93cbf856b3c3"/,
    'queued jobs must expose an explicit cancel action',
  );
  jobWorkflowState.jobs.push({
    id: 'b111ffe4-f450-486f-b393-93cbf856b3c4',
    type: 'node.capability.install',
    scope_type: 'node',
    scope_id: 'node-3',
    node_id: 'node-3',
    status: 'running',
    payload: {},
    result: {},
    created_at: '2026-07-23T11:17:00Z',
  });
  jobWorkflows.renderJobs();
  assert.match(
    elementForID('content').innerHTML,
    /waiting for node agent; node is executing node\.capability\.install/,
    'queued agent jobs must identify an active same-node blocker',
  );
  assert.doesNotMatch(
    elementForID('content').innerHTML,
    /data-job-cancel-id="b111ffe4-f450-486f-b393-93cbf856b3c4"/,
    'running jobs must not expose a cancel action',
  );
  const cancellationSummary = await jobWorkflows.cancelJobsByIDs([
    'a000ffe4-f450-486f-b393-93cbf856b3c3',
    'b111ffe4-f450-486f-b393-93cbf856b3c4',
  ]);
  assert.deepStrictEqual(
    { requested: cancellationSummary.requested, cancelled: cancellationSummary.cancelled, failed: cancellationSummary.failed },
    { requested: 1, cancelled: 1, failed: 0 },
    'bulk cancellation must include queued/retrying jobs only',
  );
  assert.strictEqual(cancelledJobRequests.length, 1, 'running jobs must never reach the cancel API');
  assert.strictEqual(cancelledJobRequests[0].options.method, 'POST');

  const pendingCoreRequests = [];
  const loaderState = {
    authUser: { id: 'operator' },
    trafficExportFilters: {},
    controlPlaneTLSSettings: null,
  };
  let progressRenders = 0;
  const loader = windowObject.MegaVPNCoreLoader.create({
    state: loaderState,
    fetchJSON: (requestPath, fallback, options) => new Promise((resolve) => {
      pendingCoreRequests.push({ requestPath, fallback, options, resolve });
    }),
    hasPermission: () => true,
    updateReadyPill: () => {},
    renderNotice: () => {},
    renderProgress: () => { progressRenders += 1; },
  });
  const coreLoad = loader.loadCore();
  await new Promise((resolve) => setImmediate(resolve));
  assert.strictEqual(pendingCoreRequests.length, 2, 'readiness requests should start together');
  for (const request of pendingCoreRequests.slice(0, 2)) request.resolve(request.fallback);
  await new Promise((resolve) => setImmediate(resolve));
  assert.strictEqual(pendingCoreRequests.length, 7, 'only five critical authenticated requests should start before first render');
  for (const request of pendingCoreRequests.slice(2, 7)) request.resolve(request.fallback);
  await new Promise((resolve) => setImmediate(resolve));
  assert.strictEqual(progressRenders, 1, 'initial dashboard should render after critical requests finish');
  assert.ok(pendingCoreRequests.length >= 25, 'secondary inventory requests should start after first render');
  for (const request of pendingCoreRequests.slice(2)) {
    assert.ok(request.options?.signal, `core request ${request.requestPath} must be abortable`);
    request.resolve(request.fallback);
  }
  await coreLoad;

  console.log(`frontend bootstrap smoke ok: ${scripts.length} assets`);
}

main().catch((err) => {
  console.error(err && err.stack ? err.stack : String(err));
  process.exit(1);
});
