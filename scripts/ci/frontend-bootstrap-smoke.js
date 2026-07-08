#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const vm = require('vm');

const rootDir = path.resolve(__dirname, '..', '..');
const defaultLegacyIndex = path.join(rootDir, 'web', 'legacy', 'index.html');
const defaultIndex = fs.existsSync(defaultLegacyIndex) ? defaultLegacyIndex : path.join(rootDir, 'web', 'index.html');
const indexPath = process.env.MEGAVPN_FRONTEND_SMOKE_INDEX
  ? path.resolve(rootDir, process.env.MEGAVPN_FRONTEND_SMOKE_INDEX)
  : defaultIndex;
const indexDir = path.dirname(indexPath);
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
    const filePath = path.resolve(indexDir, src.replace(/^\.\//, ''));
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
  console.log(`frontend bootstrap smoke ok: ${scripts.length} assets from ${path.relative(rootDir, indexPath)}`);
}

main().catch((err) => {
  console.error(err && err.stack ? err.stack : String(err));
  process.exit(1);
});
