(function (window) {
  'use strict';

  function createDomainUI(ctx = {}) {
    const { state, escapeHTML, formatDate } = ctx;
    if (!state || typeof escapeHTML !== 'function' || typeof formatDate !== 'function') {
      throw new Error('MegaVPNDomainUI requires state, escapeHTML and formatDate');
    }

    function instanceServiceOptions() {
      return availableInstanceServices()
        .map((service) => `<option value="${escapeHTML(service.code)}">${escapeHTML(service.display_name || service.name || service.code)} · ${escapeHTML(service.code)}</option>`)
        .join('');
    }

    function availableServicePacks() {
      return (state.servicePacks || [])
        .filter((pack) => Array.isArray(pack.components) && pack.components.length)
        .sort((left, right) => String(left.label || left.key).localeCompare(String(right.label || right.key), 'en'));
    }

    function servicePackByKey(packKey) {
      return availableServicePacks().find((pack) => pack.key === packKey) || null;
    }

    function servicePackComponents(pack) {
      return Array.isArray(pack?.components) ? pack.components : [];
    }

    function servicePackComponentUsesTLSEdgeCertificate(component) {
      const serviceCode = normalizeInstanceServiceCode(component?.service_code);
      const spec = component?.spec && typeof component.spec === 'object' ? component.spec : {};
      if (serviceCode === 'nginx') return true;
      if (serviceCode === 'xray-core') {
        return String(spec.security || spec.tls_security || '').trim().toLowerCase() === 'tls';
      }
      return false;
    }

    function servicePackUsesTLSEdgeCertificate(pack) {
      return servicePackComponents(pack).some(servicePackComponentUsesTLSEdgeCertificate);
    }

    function defaultServicePack() {
      return availableServicePacks()[0] || null;
    }

    function servicePackOptions(selectedKey = '') {
      const packs = availableServicePacks();
      if (!packs.length) {
        return '<option value="">No service packs available</option>';
      }
      return packs
        .map((pack) => `<option value="${escapeHTML(pack.key)}"${pack.key === selectedKey ? ' selected' : ''}>${escapeHTML(pack.label || pack.key)}</option>`)
        .join('');
    }

    function activeLeafCertificates() {
      return (state.platformCertificates || [])
        .filter((item) => item.kind === 'leaf' && certificateDisplayStatus(item) === 'active' && item.key_secret_ref_id && String(item.status || '').toLowerCase() !== 'deleted')
        .sort((left, right) => {
          if (Boolean(left.is_default) !== Boolean(right.is_default)) return left.is_default ? -1 : 1;
          return String(left.name || left.common_name || left.id).localeCompare(String(right.name || right.common_name || right.id), 'en');
        });
    }

    function defaultLeafCertificate() {
      return activeLeafCertificates().find((item) => Boolean(item.is_default)) || null;
    }

    function defaultLeafCertificateID() {
      return defaultLeafCertificate()?.id || '';
    }

    function activeManagedAuthorities() {
      return (state.platformCertificates || [])
        .filter((item) => item.kind === 'ca' && certificateDisplayStatus(item) === 'active' && String(item.status || '').toLowerCase() !== 'deleted')
        .sort((left, right) => String(left.name || left.common_name || left.id).localeCompare(String(right.name || right.common_name || right.id), 'en'));
    }

    function certificateIsExpired(item) {
      if (!item?.not_after) return false;
      const expiresAt = new Date(item.not_after);
      if (Number.isNaN(expiresAt.getTime())) return false;
      return expiresAt.getTime() <= Date.now();
    }

    function certificateDisplayStatus(item) {
      const raw = String(item?.status || 'unknown').toLowerCase();
      if (certificateIsExpired(item) && raw !== 'revoked') return 'expired';
      return raw || 'unknown';
    }

    function certificateExpiryCaption(item) {
      if (!item?.not_after) return 'n/a';
      const expiresAt = new Date(item.not_after);
      if (Number.isNaN(expiresAt.getTime())) return formatDate(item.not_after);
      if (expiresAt.getTime() <= Date.now()) return `${formatDate(item.not_after)} · expired`;
      const daysLeft = Math.ceil((expiresAt.getTime() - Date.now()) / (24 * 60 * 60 * 1000));
      if (daysLeft <= 30) return `${formatDate(item.not_after)} · ${daysLeft}d left`;
      return formatDate(item.not_after);
    }

    function certificatePrimaryLabel(item) {
      return item?.name || item?.common_name || item?.id || 'certificate';
    }

    function certificateUsageCaption(certificateID) {
      const idv = String(certificateID || '').trim();
      if (!idv) return 'n/a';
      const usages = [];
      (state.instances || []).forEach((instance) => {
        const spec = instance?.spec || {};
        const certIDs = [
          spec.certificate_id,
          spec.tls_certificate_id,
          spec?.tls?.certificate_id,
        ].filter(Boolean).map(String);
        if (certIDs.includes(idv)) usages.push(instance.name || instance.slug || instance.id);
      });
      if (state.controlPlaneTLSSettings?.certificate_id === idv) usages.push('control-plane TLS');
      if (!usages.length) return 'not assigned';
      if (usages.length <= 2) return usages.join(', ');
      return `${usages.length} bindings`;
    }

    function certificateOptions(selectedID = '', includeEmpty = true) {
      const items = activeLeafCertificates();
      const parts = [];
      if (includeEmpty) {
        parts.push('<option value="">No managed certificate</option>');
      }
      for (const item of items) {
        const expires = certificateExpiryCaption(item);
        const label = `${item.is_default ? '[default] ' : ''}${item.name || item.common_name || item.id} · ${item.source || 'certificate'} · ${expires}`;
        parts.push(`<option value="${escapeHTML(item.id)}"${item.id === selectedID ? ' selected' : ''}>${escapeHTML(label)}</option>`);
      }
      return parts.join('');
    }

    function authorityCertificateOptions(selectedID = '') {
      return activeManagedAuthorities()
        .map((item) => `<option value="${escapeHTML(item.id)}"${item.id === selectedID ? ' selected' : ''}>${escapeHTML(item.name || item.common_name || item.id)} · ${escapeHTML(item.common_name || 'CA')}</option>`)
        .join('');
    }

    function activeServicePKIRoots(serviceCode = '') {
      const expectedService = normalizeInstanceServiceCode(serviceCode);
      return (state.platformPKIRoots || [])
        .filter((root) => String(root.status || 'active').toLowerCase() === 'active')
        .filter((root) => !expectedService || normalizeInstanceServiceCode(root.service_code) === expectedService)
        .sort((left, right) => String(left.service_code || '').localeCompare(String(right.service_code || ''), 'en')
          || String(left.pki_profile || 'default').localeCompare(String(right.pki_profile || 'default'), 'en'));
    }

    function servicePKIProfileOptions(serviceCode = 'openvpn', selectedProfile = 'default') {
      const selected = String(selectedProfile || 'default').trim() || 'default';
      const roots = activeServicePKIRoots(serviceCode);
      const profiles = new Map();
      for (const root of roots) {
        const profile = String(root.pki_profile || 'default').trim() || 'default';
        if (!profiles.has(profile)) profiles.set(profile, root);
      }
      if (!profiles.has('default')) {
        profiles.set('default', {
          pki_profile: 'default',
          common_name: 'default profile, auto-created on first apply',
          service_code: serviceCode || 'openvpn',
        });
      }
      if (selected && !profiles.has(selected)) {
        profiles.set(selected, {
          pki_profile: selected,
          common_name: 'custom profile, create the matching service CA root before rollout',
          service_code: serviceCode || 'openvpn',
        });
      }
      return Array.from(profiles.entries()).map(([profile, root]) => {
        const expires = root.not_after ? ` · ${certificateExpiryCaption(root)}` : '';
        const label = `${profile} · ${root.common_name || 'service CA root'}${expires}`;
        return `<option value="${escapeHTML(profile)}"${profile === selected ? ' selected' : ''}>${escapeHTML(label)}</option>`;
      }).join('');
    }

    function nodeOptions(selectedID = '', options = {}) {
      const selected = String(selectedID || '').trim();
      const roles = Array.isArray(options.roles)
        ? options.roles.map((role) => String(role || '').trim().toLowerCase()).filter(Boolean)
        : [];
      const includeEmpty = options.includeEmpty === true;
      const emptyLabel = options.emptyLabel || 'Select node';
      const parts = includeEmpty ? [`<option value="">${escapeHTML(emptyLabel)}</option>`] : [];
      (state.nodes || [])
        .filter((node) => {
          if (!roles.length) return true;
          return roles.includes(String(node.role || '').trim().toLowerCase());
        })
        .map((node) => {
          const role = String(node.role || 'node').trim() || 'node';
          return `<option value="${escapeHTML(node.id)}"${node.id === selected ? ' selected' : ''}>${escapeHTML(node.name)} · ${escapeHTML(role)} · ${escapeHTML(node.address || 'n/a')} · ${escapeHTML(node.agent_status || 'unknown')}</option>`;
        })
        .forEach((option) => parts.push(option));
      return parts.join('');
    }

    function normalizeInstanceServiceCode(serviceCode) {
      const normalized = String(serviceCode || '').trim().toLowerCase();
      if (normalized === 'xray') return 'xray-core';
      if (normalized === 'wg' || normalized === 'wg-quick') return 'wireguard';
      if (normalized === 'squid' || normalized === 'http-proxy') return 'http_proxy';
      if (normalized === 'shadowsocks-libev' || normalized === 'ss-server') return 'shadowsocks';
      return normalized;
    }

    function serviceInstanceLabel(instance) {
      if (!instance) return 'service instance';
      const service = String(instance.service_label || instance.service_display_name || instance.service_code || 'service').trim();
      const name = String(instance.name || instance.slug || instance.id || 'instance').trim();
      const node = String(instance.node_name || instance.node || '').trim();
      const endpointHost = String(instance.endpoint_host || '').trim();
      const endpointPort = Number(instance.endpoint_port || 0);
      const endpoint = endpointHost ? `${endpointHost}${endpointPort ? `:${endpointPort}` : ''}` : '';
      return [name, service, node, endpoint].filter(Boolean).join(' · ');
    }

    function selectableTargetInstances() {
      return (state.instances || [])
        .filter((instance) => String(instance.status || '').toLowerCase() !== 'deleted')
        .sort((left, right) => serviceInstanceLabel(left).localeCompare(serviceInstanceLabel(right), 'en'));
    }

    function targetInstanceOptions(selectedID = '') {
      const selected = String(selectedID || '').trim();
      const instances = selectableTargetInstances();
      const parts = ['<option value="">Select target instance</option>'];
      for (const instance of instances) {
        parts.push(`<option value="${escapeHTML(instance.id)}"${instance.id === selected ? ' selected' : ''}>${escapeHTML(serviceInstanceLabel(instance))}</option>`);
      }
      return parts.join('');
    }

    function findInstanceByID(instanceID) {
      const id = String(instanceID || '').trim();
      if (!id) return null;
      return (state.instances || []).find((instance) => String(instance.id || '') === id) || null;
    }

    function cloneJSON(value) {
      if (value == null) return {};
      return JSON.parse(JSON.stringify(value));
    }

    function stringValue(...values) {
      for (const value of values) {
        const text = String(value ?? '').trim();
        if (text) return text;
      }
      return '';
    }

    function openVPNFullTunnelServerExtraLines() {
      return [
        'push "redirect-gateway def1 bypass-dhcp"',
        'push "dhcp-option DNS 1.1.1.1"',
        'push "dhcp-option DNS 1.0.0.1"',
      ].join('\n');
    }

    function numberValue(...values) {
      for (const value of values) {
        const num = Number(value);
        if (Number.isFinite(num) && num !== 0) return num;
      }
      return 0;
    }

    function slugPathPart(value, fallback = 'server') {
      const normalized = String(value || '').trim().toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '');
      return normalized || fallback;
    }

    const strongPasswordCharacterSets = [
      'abcdefghijklmnopqrstuvwxyz',
      'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
      '0123456789',
      '!#$%&()*+,-.:=?@_~',
    ];

    function secureRandomInt(max) {
      if (!Number.isFinite(max) || max <= 0) return 0;
      const cryptoSource = window.crypto || window.msCrypto;
      if (!cryptoSource?.getRandomValues) {
        throw new Error('Secure random generator is not available');
      }
      const values = new Uint32Array(1);
      const limit = Math.floor(0x100000000 / max) * max;
      do {
        cryptoSource.getRandomValues(values);
      } while (values[0] >= limit);
      return values[0] % max;
    }

    function generateStrongPassword(length = 32) {
      const size = Math.max(Number(length) || 32, strongPasswordCharacterSets.length);
      const allCharacters = strongPasswordCharacterSets.join('');
      const out = [];
      for (const set of strongPasswordCharacterSets) {
        out.push(set[secureRandomInt(set.length)]);
      }
      while (out.length < size) {
        out.push(allCharacters[secureRandomInt(allCharacters.length)]);
      }
      for (let i = out.length - 1; i > 0; i -= 1) {
        const j = secureRandomInt(i + 1);
        [out[i], out[j]] = [out[j], out[i]];
      }
      return out.join('');
    }

    function generatedSecretValue(existing) {
      const current = String(existing || '').trim();
      if (current) return current;
      try {
        return generateStrongPassword(32);
      } catch (_) {
        return '';
      }
    }

    const instanceServiceCatalog = window.MegaVPNInstanceCatalog || {};
    const INSTANCE_SERVICE_ORDER = Array.isArray(instanceServiceCatalog.serviceOrder)
      ? instanceServiceCatalog.serviceOrder
      : [];
    const INSTANCE_SERVICE_BLUEPRINTS = instanceServiceCatalog.serviceBlueprints || {};

    function instanceServiceBlueprint(serviceCode) {
      const normalized = normalizeInstanceServiceCode(serviceCode);
      const service = (state.servicesCatalog || []).find((entry) => normalizeInstanceServiceCode(entry.code) === normalized);
      const fallback = INSTANCE_SERVICE_BLUEPRINTS[normalized] || null;
      if (!service) return fallback;
      return {
        label: service.label || service.display_name || fallback?.label || service.name || normalized,
        runtimeCode: service.runtime_code || fallback?.runtimeCode || normalized,
        runtime: service.runtime || fallback?.runtime || 'runtime n/a',
        serviceKind: service.service_kind || fallback?.serviceKind || 'service',
        companionTo: Array.isArray(service.companion_to) ? service.companion_to : (fallback?.companionTo || []),
        companionNote: service.companion_note || fallback?.companionNote || '',
        description: service.description || fallback?.description || '',
        unitPattern: service.unit_pattern || fallback?.unitPattern || 'n/a',
        pathPattern: service.path_pattern || fallback?.pathPattern || 'n/a',
        nameTemplate: service.name_template || fallback?.nameTemplate || '',
        slugTemplate: service.slug_template || fallback?.slugTemplate || '',
        endpointHint: service.endpoint_hint || fallback?.endpointHint || '',
        platformNotes: Array.isArray(service.platform_notes) ? service.platform_notes : (fallback?.platformNotes || []),
        recommendations: Array.isArray(service.recommendations) ? service.recommendations : (fallback?.recommendations || []),
        presets: Array.isArray(service.presets) && service.presets.length ? service.presets : (fallback?.presets || []),
      };
    }

    function availableInstanceServices() {
      const ranked = new Map();
      const fallbackOrderBase = INSTANCE_SERVICE_ORDER.length;
      for (const service of (state.servicesCatalog || [])) {
        if (service.supports_instances === false || service.enabled === false) continue;
        const normalized = normalizeInstanceServiceCode(service.code);
        const blueprint = instanceServiceBlueprint(normalized);
        const candidate = {
          ...service,
          code: normalized,
          display_name: blueprint?.label || service.name || normalized,
        };
        const current = ranked.get(normalized);
        const score = service.code === normalized ? 2 : 1;
        if (!current || score > current.score) {
          ranked.set(normalized, { score, service: candidate });
        }
      }
      return Array.from(ranked.values())
        .map((entry) => entry.service)
        .sort((left, right) => {
          const leftIndex = INSTANCE_SERVICE_ORDER.indexOf(left.code);
          const rightIndex = INSTANCE_SERVICE_ORDER.indexOf(right.code);
          const leftOrder = leftIndex === -1 ? fallbackOrderBase : leftIndex;
          const rightOrder = rightIndex === -1 ? fallbackOrderBase : rightIndex;
          if (leftOrder !== rightOrder) return leftOrder - rightOrder;
          return String(left.display_name).localeCompare(String(right.display_name), 'en');
        });
    }

    function defaultInstancePreset(serviceCode) {
      const presets = instanceServiceBlueprint(serviceCode)?.presets || [];
      return presets.find((preset) => preset.recommended) || presets[0] || null;
    }

    function resolveInstancePreset(serviceCode, presetKey) {
      const presets = instanceServiceBlueprint(serviceCode)?.presets || [];
      if (!presets.length) return null;
      return presets.find((preset) => preset.key === presetKey) || defaultInstancePreset(serviceCode);
    }

    function applyInstancePresetDraft(serviceCode, draft, presetKey) {
      const preset = resolveInstancePreset(serviceCode, presetKey);
      if (!preset) return { ...(draft || {}) };
      return {
        ...(draft || {}),
        ...(preset.draft || {}),
        service_profile: preset.key,
      };
    }

    function finalizeInstanceDraft(serviceCode, instance, spec, draft, presetKey = '') {
      const normalized = normalizeInstanceServiceCode(serviceCode);
      const defaultPreset = defaultInstancePreset(normalized);
      const persistedPreset = stringValue(presetKey, draft?.service_profile, spec?.service_profile, defaultPreset?.key);
      let out = { ...(draft || {}), service_profile: persistedPreset };
      if (!instance || presetKey) {
        out = applyInstancePresetDraft(normalized, out, persistedPreset);
      }
      return out;
    }

    function isManagedVLESSAdBlockRule(rule) {
      if (!rule || typeof rule !== 'object') return false;
      const outboundTag = String(rule.outboundTag || rule.outbound_tag || '').trim().toLowerCase();
      if (outboundTag !== 'block') return false;
      const domains = Array.isArray(rule.domain) ? rule.domain : [];
      return domains.some((domain) => String(domain || '').trim().toLowerCase() === 'geosite:category-ads-all');
    }

    function vlessGroupAdBlockEnabled(source) {
      if (!source || typeof source !== 'object') return false;
      if (source.ad_block === true || source.adBlock === true || source.block_ads === true || source.blockAds === true) return true;
      return Array.isArray(source.rules) && source.rules.some(isManagedVLESSAdBlockRule);
    }

    function managedVLESSAdBlockRule() {
      return {
        type: 'field',
        domain: ['geosite:category-ads-all'],
        outbound_tag: 'block',
      };
    }

    function normalizeVLESSGroupForEditor(group, index) {
      const source = group && typeof group === 'object' ? group : {};
      const key = String(source.key || source.name || source.id || (index === 0 ? 'default' : `group-${index + 1}`)).trim();
      const outboundTag = String(source.outbound_tag || source.outboundTag || source.tag || 'direct').trim() || 'direct';
      const modeSource = String(source.access_mode || source.egress_mode || source.mode || '').trim().toLowerCase();
      let accessMode = modeSource || 'instance_default';
      if (['auto', 'default', 'inherit'].includes(accessMode)) accessMode = 'instance_default';
      if (['direct', 'local', 'current_node'].includes(accessMode)) accessMode = 'local_breakout';
      if (['remote_node', 'remote_egress', 'node'].includes(accessMode)) accessMode = 'egress_node';
      if (['deny', 'blocked'].includes(accessMode)) accessMode = 'block';
      if (outboundTag === 'block' && !modeSource && !Array.isArray(source.rules)) accessMode = 'block';
      if (source.target_instance_id && !modeSource) accessMode = 'instance_only';
      const rawRules = Array.isArray(source.rules) ? source.rules : [];
      const rawExtraRules = Array.isArray(source.extra_rules) ? source.extra_rules : [];
      const sourceRules = rawRules.filter((rule) => !isManagedVLESSAdBlockRule(rule));
      const sourceExtraRules = rawExtraRules.filter((rule) => !isManagedVLESSAdBlockRule(rule));
      return {
        key,
        label: String(source.label || source.title || key || `Group ${index + 1}`).trim(),
        description: String(source.description || '').trim(),
        access_mode: accessMode,
        outbound_tag: outboundTag,
        egress_node_id: String(source.egress_node_id || source.node_id || source.egress?.egress_node_id || source.egress?.node_id || '').trim(),
        target_instance_id: String(source.target_instance_id || source.instance_id || '').trim(),
        target_instance_label: String(source.target_instance_label || '').trim(),
        ad_block: vlessGroupAdBlockEnabled(source),
        rules: sourceRules,
        extra_rules: sourceExtraRules,
        display_order: Number(source.display_order || source.displayOrder || 1000),
      };
    }

    function activeVLESSGroupTemplates() {
      const source = Array.isArray(state.vlessGroupTemplates) && state.vlessGroupTemplates.length
        ? state.vlessGroupTemplates
        : (Array.isArray(state.vlessGroupCatalog) ? state.vlessGroupCatalog : []);
      const active = source
        .filter((group) => group && String(group.status || 'active').toLowerCase() === 'active')
        .map((group, index) => normalizeVLESSGroupForEditor(group, index))
        .filter((group) => group.key)
        .sort((left, right) => Number(left.display_order || left.displayOrder || 1000) - Number(right.display_order || right.displayOrder || 1000)
          || String(left.label || left.key).localeCompare(String(right.label || right.key), 'en'));
      if (active.length) return active;
      return [{ key: 'default', label: 'Default access', access_mode: 'instance_default', egress_mode: 'default', outbound_tag: 'direct', rules: [] }];
    }

    function vlessGroupTemplateOptions(selectedKey = '') {
      const selected = String(selectedKey || '').trim() || 'default';
      return activeVLESSGroupTemplates()
        .map((group) => {
          const meta = [];
          if (group.access_mode === 'local_breakout') meta.push('current node');
          if (group.access_mode === 'egress_node') meta.push('egress node');
          if (group.access_mode === 'instance_only') meta.push('instance only');
          if (group.access_mode === 'block' || group.outbound_tag === 'block') meta.push('blocked');
          if (group.ad_block) meta.push('ad block');
          const label = [group.label || group.key, meta.join(', ')].filter(Boolean).join(' · ');
          return `<option value="${escapeHTML(group.key)}"${group.key === selected ? ' selected' : ''}>${escapeHTML(label)}</option>`;
        })
        .join('');
    }

    function vlessGroupTemplateToSpecGroup(template) {
      const source = normalizeVLESSGroupForEditor(template, 0);
      const group = {
        key: source.key || 'default',
        label: source.label || source.key || 'Default access',
      };
      if (source.description) group.description = source.description;
      if (source.access_mode === 'instance_default') {
        group.access_mode = 'instance_default';
        group.egress_mode = 'default';
        group.outbound_tag = 'direct';
      } else if (source.access_mode === 'local_breakout') {
        group.access_mode = 'local_breakout';
        group.egress_mode = 'local_breakout';
        group.outbound_tag = 'direct';
      } else if (source.access_mode === 'egress_node') {
        group.access_mode = 'egress_node';
        group.egress_mode = 'egress_node';
        group.egress_node_id = source.egress_node_id;
        group.outbound_tag = source.outbound_tag || 'direct';
      } else if (source.access_mode === 'instance_only') {
        const targetInstance = findInstanceByID(source.target_instance_id);
        group.access_mode = 'instance_only';
        group.target_instance_id = source.target_instance_id || '';
        group.target_instance_label = targetInstance ? serviceInstanceLabel(targetInstance) : source.target_instance_label || '';
        group.outbound_tag = 'block';
        const targetRule = endpointRuleForInstance(targetInstance);
        if (targetRule) group.rules = [targetRule];
      } else if (source.access_mode === 'block') {
        group.access_mode = 'block';
        group.egress_mode = 'block';
        group.outbound_tag = 'block';
      }
      const rules = []
        .concat(Array.isArray(source.rules) ? source.rules : [])
        .concat(Array.isArray(source.extra_rules) ? source.extra_rules : []);
      if (rules.length) {
        group.rules = Array.isArray(group.rules) ? group.rules.concat(rules) : rules;
      }
      if (source.ad_block && source.access_mode !== 'block' && source.access_mode !== 'instance_only') {
        const managedRule = managedVLESSAdBlockRule();
        const existingRules = Array.isArray(group.rules) ? group.rules : [];
        if (!existingRules.some(isManagedVLESSAdBlockRule)) {
          group.rules = [managedRule].concat(existingRules);
        }
        group.ad_block = true;
      }
      return group;
    }

    function vlessGroupTemplatesAsSpecGroups() {
      return activeVLESSGroupTemplates().map(vlessGroupTemplateToSpecGroup);
    }

    function renderVLESSGroupCatalogSummary(selectedKey = '') {
      const groups = activeVLESSGroupTemplates();
      const selected = String(selectedKey || '').trim() || 'default';
      return `
        <div class="vless-global-summary">
          <div>
            <strong>${escapeHTML(String(groups.length))} global VLESS groups</strong>
            <span>Managed in Instances / VLESS groups. Every saved VLESS instance receives this catalog.</span>
          </div>
          <span class="tag">${escapeHTML(selected)} default</span>
        </div>`;
    }

    function endpointRuleForInstance(instance) {
      if (!instance) return null;
      const spec = instance.spec && typeof instance.spec === 'object' ? instance.spec : {};
      const host = String(instance.endpoint_host || spec.endpoint_host || spec.server_name || spec.sni || '').trim();
      const port = Number(instance.endpoint_port || spec.server_port || spec.listen_port || spec.port || 0);
      if (!host || port <= 0) return null;
      const rule = {
        type: 'field',
        outbound_tag: 'direct',
      };
      if (host) {
        if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host) || host.includes(':')) {
          rule.ip = [host];
        } else {
          rule.domain = [host];
        }
      }
      if (port > 0) {
        rule.port = String(port);
      }
      return rule;
    }

    function renderInstanceServiceProfilePanel(serviceCode, draft = {}) {
      const blueprint = instanceServiceBlueprint(serviceCode);
      if (!blueprint) return '';
      const preset = resolveInstancePreset(serviceCode, draft.service_profile);
      const presets = blueprint.presets || [];
      const platformNotes = blueprint.platformNotes || [];
      const recommendations = blueprint.recommendations || [];
      return `
        <div class="field full">
          <div class="instance-service-profile-card">
            <div class="instance-service-profile-main">
              <div class="instance-panel-label">${escapeHTML(blueprint.serviceKind || 'service')}</div>
              <strong>${escapeHTML(blueprint.label)}</strong>
              <span>${escapeHTML(blueprint.description || '')}</span>
            </div>
            <div class="instance-service-profile-facts">
              <span>Service <code>${escapeHTML(normalizeInstanceServiceCode(serviceCode))}</code></span>
              <span>Runtime <code>${escapeHTML(blueprint.runtimeCode || normalizeInstanceServiceCode(serviceCode))}</code></span>
              <span>Unit <code>${escapeHTML(blueprint.unitPattern || 'n/a')}</code></span>
              <span>Config <code>${escapeHTML(blueprint.pathPattern || 'n/a')}</code></span>
            </div>
            ${Array.isArray(blueprint.companionTo) && blueprint.companionTo.length ? `<div class="metric-caption" style="margin-top:6px">Companion services: <code>${escapeHTML(blueprint.companionTo.join(', '))}</code></div>` : ''}
            ${blueprint.companionNote ? `<div class="metric-caption" style="margin-top:6px">${escapeHTML(blueprint.companionNote)}</div>` : ''}
            ${presets.length ? `
              <div class="instance-service-profile-preset">
                <label>Preset</label>
                <select name="service_profile">
                  ${presets.map((item) => `<option value="${escapeHTML(item.key)}"${item.key === preset?.key ? ' selected' : ''}>${escapeHTML(item.label)}${item.recommended ? ' (recommended)' : ''}</option>`).join('')}
                </select>
                <div class="field-hint">${escapeHTML(preset?.description || '')}</div>
              </div>` : ''}
            ${platformNotes.length ? `<div class="metric-caption" style="margin-top:10px">${platformNotes.map((line) => `CA / platform: ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
            ${recommendations.length ? `<div class="metric-caption" style="margin-top:10px">${recommendations.map((line) => `• ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
          </div>
        </div>`;
    }

    function applyAutoFieldValue(input, nextValue, forceDefaults = false) {
      if (!input || !nextValue) return;
      const current = String(input.value || '').trim();
      const previousAuto = String(input.dataset.autoValue || '').trim();
      if (!current || current === previousAuto) {
        input.value = nextValue;
        input.dataset.autoValue = nextValue;
      }
    }

    function applyCreateInstanceDefaults(form, serviceCode, draft, options = {}) {
      if (!form) return;
      const blueprint = instanceServiceBlueprint(serviceCode);
      if (!blueprint) return;
      const nameInput = form.querySelector('input[name="name"]');
      const slugInput = form.querySelector('input[name="slug"]');
      const hostInput = form.querySelector('input[name="endpoint_host"]');
      const unitInput = form.querySelector('input[name="systemd_unit"]');
      if (nameInput) {
        if (blueprint.nameTemplate) {
          nameInput.placeholder = blueprint.nameTemplate;
          applyAutoFieldValue(nameInput, blueprint.nameTemplate, options.forceDefaults);
        }
      }
      if (slugInput) {
        if (blueprint.slugTemplate) {
          slugInput.placeholder = blueprint.slugTemplate;
          applyAutoFieldValue(slugInput, blueprint.slugTemplate, options.forceDefaults);
        }
      }
      if (hostInput) {
        hostInput.placeholder = blueprint.endpointHint || 'vpn.example.com';
      }
      if (unitInput) {
        unitInput.placeholder = blueprint.unitPattern || 'optional override';
      }
      if (serviceCode === 'ipsec' && hostInput && !String(hostInput.value || '').trim() && blueprint.endpointHint) {
        hostInput.placeholder = blueprint.endpointHint;
      }
      if (serviceCode === 'xl2tpd' && hostInput) {
        hostInput.placeholder = blueprint.endpointHint || 'l2tp.example.com';
      }
      if (draft?.service_profile) {
        const note = form.querySelector('.service-profile-inline-note');
        if (note) note.remove();
        if (blueprint.companionNote) {
          const noteBlock = document.createElement('div');
          noteBlock.className = 'field full service-profile-inline-note';
          noteBlock.innerHTML = `<div class="code-block"><div class="metric-caption">${escapeHTML(blueprint.companionNote)}</div></div>`;
          const target = form.querySelector('.inline-actions');
          if (target?.parentElement) {
            target.parentElement.parentElement.insertBefore(noteBlock, target.parentElement);
          }
        }
      }
    }

    function buildInstanceSpecDraft(serviceCode, instance = null, presetKey = '') {
      const spec = instance?.spec || {};
      const normalized = normalizeInstanceServiceCode(serviceCode || instance?.service_code);
      switch (normalized) {
        case 'xray-core':
          const xraySlug = slugPathPart(instance?.slug, 'xray');
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, 443),
            config_path: stringValue(spec.config_path, `/usr/local/etc/xray/${xraySlug}.json`),
            config_mode: stringValue(spec.config_mode, '0640'),
            xray_security: stringValue(spec.security, 'reality'),
            certificate_id: stringValue(spec.certificate_id),
            xray_server_name: stringValue(spec.server_name, spec.sni, instance?.endpoint_host),
            xray_short_id: stringValue(spec.short_id),
            xray_dest: stringValue(spec.dest, 'www.cloudflare.com:443'),
            xray_fingerprint: stringValue(spec.fingerprint, 'chrome'),
            xray_network: stringValue(spec.network, spec.type, spec.transport, 'tcp'),
            xray_path: stringValue(spec.path, '/ws'),
            xray_service_name: stringValue(spec.service_name, 'vless-grpc'),
            xray_flow: stringValue(spec.flow),
            xray_egress_mode: stringValue(spec.egress_mode, spec.xray_egress_mode, spec.vless_egress_mode, spec.xray_egress?.mode, 'auto'),
            xray_egress_node_id: stringValue(spec.egress_node_id, spec.xray_egress_node_id, spec.vless_egress_node_id, spec.xray_egress?.egress_node_id, spec.xray_egress?.node_id),
            xray_default_vless_group: stringValue(spec.default_vless_group, 'default'),
            config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
          }, presetKey);
        case 'openvpn':
          const ovpnSlug = slugPathPart(instance?.slug, 'server');
          const ovpnDefaultServerExtraLines = instance ? '' : openVPNFullTunnelServerExtraLines();
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, 1194),
            config_path: stringValue(spec.config_path, `/etc/openvpn/server/${ovpnSlug}.conf`),
            config_mode: stringValue(spec.config_mode, '0644'),
            ovpn_proto: stringValue(spec.proto, 'tcp'),
            ovpn_dev: stringValue(spec.dev, 'tun'),
            ovpn_server_network: stringValue(spec.server_network),
            ovpn_server_netmask: stringValue(spec.server_netmask),
            ovpn_cipher: stringValue(spec.cipher),
            ovpn_auth: stringValue(spec.auth),
            ovpn_runtime_dir: stringValue(spec.runtime_dir),
            ovpn_pki_profile: stringValue(spec.pki_profile, 'default'),
            ovpn_client_proxy_mode: stringValue(spec.client_proxy_mode, spec.proxy_mode, 'direct'),
            ovpn_proxy_host: stringValue(spec.socks_proxy_host, spec.http_proxy_host, spec.proxy_host, '127.0.0.1'),
            ovpn_proxy_port: numberValue(spec.socks_proxy_port, spec.http_proxy_port, spec.proxy_port, 1080),
            ovpn_server_extra_lines: Array.isArray(spec.server_extra_lines) ? spec.server_extra_lines.join('\n') : stringValue(spec.server_extra_lines, ovpnDefaultServerExtraLines),
            ovpn_client_extra_lines: Array.isArray(spec.client_extra_lines) ? spec.client_extra_lines.join('\n') : stringValue(spec.client_extra_lines),
            ovpn_client_template: stringValue(spec.ovpn_inline, spec.client_ovpn_inline),
            config_body: stringValue(spec.config_content),
          }, presetKey);
        case 'wireguard':
          const wgSlug = slugPathPart(instance?.slug, 'wg0');
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 51820),
            config_path: stringValue(spec.config_path, `/etc/wireguard/${wgSlug}.conf`),
            config_mode: stringValue(spec.config_mode, '0600'),
            wg_network_cidr: stringValue(spec.network_cidr),
            wg_server_address: stringValue(spec.server_address),
            wg_client_address_start: numberValue(spec.client_address_start, 10),
            wg_client_allowed_ips: stringValue(spec.client_allowed_ips, '0.0.0.0/0, ::/0'),
            wg_client_dns: stringValue(spec.client_dns, '1.1.1.1, 1.0.0.1'),
            wg_keepalive: numberValue(spec.persistent_keepalive, 25),
            wg_mtu: numberValue(spec.mtu),
            wg_interface_extra_lines: Array.isArray(spec.interface_extra_lines) ? spec.interface_extra_lines.join('\n') : stringValue(spec.interface_extra_lines),
            config_body: stringValue(spec.config_content),
          }, presetKey);
        case 'mtproto':
          const mtprotoSlug = slugPathPart(instance?.slug, 'mtproto');
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, 443),
            config_path: stringValue(spec.config_path, `/usr/local/etc/xray/${mtprotoSlug}.json`),
            config_mode: stringValue(spec.config_mode, '0640'),
            mtproto_listen: stringValue(spec.listen, '0.0.0.0'),
            config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
          }, presetKey);
        case 'nginx':
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 8080),
            config_path: stringValue(spec.config_path, '/etc/nginx/conf.d/megavpn-edge.conf'),
            config_mode: stringValue(spec.config_mode, '0644'),
            nginx_mode: stringValue(spec.mode, 'reverse_proxy'),
            nginx_location_path: stringValue(spec.location_path, '/'),
            certificate_id: stringValue(spec.certificate_id),
            nginx_server_name: stringValue(spec.server_name, instance?.endpoint_host, '_'),
            nginx_upstream_url: stringValue(spec.upstream_url, spec.proxy_pass),
            nginx_fallback_upstream_url: stringValue(spec.fallback_upstream_url, spec.fallback_proxy_pass),
            nginx_fallback_host_header: stringValue(spec.fallback_host_header, spec.fallback_host),
            nginx_fallback_sni: stringValue(spec.fallback_sni),
            nginx_root_dir: stringValue(spec.root_dir),
            nginx_index_files: stringValue(spec.index_files, 'index.html index.htm'),
            nginx_tls_enabled: String(Boolean(spec.tls_enabled)),
            nginx_tls_cert_path: stringValue(spec.tls_cert_path),
            nginx_tls_key_path: stringValue(spec.tls_key_path),
            nginx_client_max_body_size: stringValue(spec.client_max_body_size),
            nginx_access_log: stringValue(spec.access_log),
            nginx_error_log: stringValue(spec.error_log),
            nginx_location_extra_lines: Array.isArray(spec.location_extra_lines) ? spec.location_extra_lines.join('\n') : stringValue(spec.location_extra_lines),
            nginx_fallback_location_extra_lines: Array.isArray(spec.fallback_location_extra_lines) ? spec.fallback_location_extra_lines.join('\n') : stringValue(spec.fallback_location_extra_lines),
            nginx_server_extra_lines: Array.isArray(spec.server_extra_lines) ? spec.server_extra_lines.join('\n') : stringValue(spec.server_extra_lines),
            config_body: stringValue(spec.config_content),
          }, presetKey);
        case 'ipsec':
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 1701),
            config_path: stringValue(spec.config_path, '/etc/ipsec.conf'),
            config_mode: stringValue(spec.config_mode, '0644'),
            ipsec_secrets_path: stringValue(spec.secrets_path, '/etc/ipsec.secrets'),
            ipsec_secrets_mode: stringValue(spec.secrets_mode, '0600'),
            ipsec_left: stringValue(spec.left, '%defaultroute'),
            ipsec_leftid: stringValue(spec.leftid, spec.server_id, instance?.endpoint_host),
            ipsec_right: stringValue(spec.right, '%any'),
            ipsec_psk: stringValue(spec.psk),
            ipsec_ike: stringValue(spec.ike, 'aes256-sha1-modp1024'),
            ipsec_esp: stringValue(spec.esp, 'aes256-sha1'),
            ipsec_config_extra_lines: Array.isArray(spec.config_extra_lines) ? spec.config_extra_lines.join('\n') : stringValue(spec.config_extra_lines),
            ipsec_secrets_body: stringValue(spec.secrets_content),
            config_body: stringValue(spec.config_content),
          }, presetKey);
        case 'http_proxy':
          const proxySlug = slugPathPart(instance?.slug, 'proxy');
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 3128),
            config_path: stringValue(spec.config_path, `/etc/squid/${proxySlug}.conf`),
            config_mode: stringValue(spec.config_mode, '0644'),
            proxy_auth_realm: stringValue(spec.auth_realm, 'RTIS MegaVPN HTTP Proxy'),
            proxy_visible_hostname: stringValue(spec.visible_hostname, instance?.endpoint_host, instance?.name),
            proxy_auth_helper_path: stringValue(spec.auth_helper_path, '/usr/lib/squid/basic_ncsa_auth'),
            proxy_config_extra_lines: Array.isArray(spec.config_extra_lines) ? spec.config_extra_lines.join('\n') : stringValue(spec.config_extra_lines),
            config_body: stringValue(spec.config_content),
          }, presetKey);
        case 'xl2tpd':
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.listen_port, spec.server_port, 1701),
            config_path: stringValue(spec.config_path, '/etc/xl2tpd/xl2tpd.conf'),
            config_mode: stringValue(spec.config_mode, '0644'),
            xl2tpd_options_path: stringValue(spec.options_path, '/etc/ppp/options.xl2tpd'),
            xl2tpd_chap_secrets_path: stringValue(spec.chap_secrets_path, '/etc/ppp/chap-secrets'),
            xl2tpd_local_ip: stringValue(spec.local_ip),
            xl2tpd_ip_range_start: stringValue(spec.ip_range_start),
            xl2tpd_ip_range_end: stringValue(spec.ip_range_end),
            xl2tpd_dns_primary: stringValue(spec.ppp_dns_primary, '1.1.1.1'),
            xl2tpd_dns_secondary: stringValue(spec.ppp_dns_secondary, '1.0.0.1'),
            xl2tpd_default_username: stringValue(spec.default_username),
            xl2tpd_default_password: stringValue(spec.default_password),
            xl2tpd_chap_secrets_entries: stringValue(spec.chap_secrets_entries, spec.chap_secrets_content),
            xl2tpd_options_extra_lines: Array.isArray(spec.options_extra_lines) ? spec.options_extra_lines.join('\n') : stringValue(spec.options_extra_lines),
            xl2tpd_config_extra_lines: Array.isArray(spec.config_extra_lines) ? spec.config_extra_lines.join('\n') : stringValue(spec.config_extra_lines),
            xl2tpd_options_body: stringValue(spec.options_content),
            config_body: stringValue(spec.config_content),
          }, presetKey);
        case 'shadowsocks':
          const ssSlug = slugPathPart(instance?.slug, 'shadowsocks');
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port, spec.server_port, spec.access_port_base, 8388),
            config_path: stringValue(spec.config_path, `/etc/shadowsocks-libev/${ssSlug}.json`),
            config_mode: stringValue(spec.config_mode, '0640'),
            ss_method: stringValue(spec.method, 'chacha20-ietf-poly1305'),
            ss_mode: stringValue(spec.mode, 'tcp_and_udp'),
            ss_timeout: numberValue(spec.timeout, 300),
            ss_password: stringValue(spec.password, spec.server_password),
            ss_access_port_base: numberValue(spec.access_port_base, spec.server_port, instance?.endpoint_port, 8388),
            config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
          }, presetKey);
        default:
          return finalizeInstanceDraft(normalized, instance, spec, {
            endpoint_port: numberValue(instance?.endpoint_port),
            config_path: stringValue(spec.config_path),
            config_mode: stringValue(spec.config_mode, '0640'),
            config_type: spec.config_json ? 'json' : 'text',
            config_body: spec.config_json ? JSON.stringify(spec.config_json, null, 2) : stringValue(spec.config_content),
          }, presetKey);
      }
    }

    function renderInstanceServiceFields(serviceCode, draft = {}) {
      const intro = renderInstanceServiceProfilePanel(serviceCode, draft);
      switch (normalizeInstanceServiceCode(serviceCode)) {
        case 'xray-core':
          return `${intro}
            <div class="field"><label>Security</label><select name="xray_security">
              <option value="reality"${draft.xray_security !== 'none' && draft.xray_security !== 'tls' ? ' selected' : ''}>reality</option>
              <option value="tls"${draft.xray_security === 'tls' ? ' selected' : ''}>tls</option>
              <option value="none"${draft.xray_security === 'none' ? ' selected' : ''}>none (backend)</option>
            </select></div>
            <div class="field"><label>Managed certificate</label><select name="certificate_id">${certificateOptions(draft.certificate_id || '')}</select></div>
            <div class="field"><label>Server name / SNI</label><input name="xray_server_name" value="${escapeHTML(draft.xray_server_name || '')}" placeholder="vpn.example.com" /></div>
            <div class="field"><label>Short ID</label><input name="xray_short_id" value="${escapeHTML(draft.xray_short_id || '')}" placeholder="0123abcd4567ef89" /></div>
            <div class="field"><label>Reality dest</label><input name="xray_dest" value="${escapeHTML(draft.xray_dest || 'www.cloudflare.com:443')}" /></div>
            <div class="field"><label>Fingerprint</label><input name="xray_fingerprint" value="${escapeHTML(draft.xray_fingerprint || 'chrome')}" /></div>
            <div class="field"><label>Network</label><select name="xray_network">
              <option value="tcp"${draft.xray_network === 'tcp' ? ' selected' : ''}>tcp</option>
              <option value="grpc"${draft.xray_network === 'grpc' ? ' selected' : ''}>grpc</option>
              <option value="ws"${draft.xray_network === 'ws' ? ' selected' : ''}>ws</option>
            </select></div>
            <div class="field"><label>HTTP path</label><input name="xray_path" value="${escapeHTML(draft.xray_path || '/ws')}" placeholder="/ws" /></div>
            <div class="field"><label>gRPC service name</label><input name="xray_service_name" value="${escapeHTML(draft.xray_service_name || 'vless-grpc')}" placeholder="vless-grpc" /></div>
            <div class="field"><label>Flow</label><input name="xray_flow" value="${escapeHTML(draft.xray_flow || '')}" placeholder="optional" /></div>
            <div class="field"><label>Egress mode</label><select name="xray_egress_mode">
              <option value="auto"${!draft.xray_egress_mode || draft.xray_egress_mode === 'auto' ? ' selected' : ''}>auto</option>
              <option value="egress_node"${draft.xray_egress_mode === 'egress_node' || draft.xray_egress_mode === 'remote_egress' || draft.xray_egress_mode === 'remote_node' ? ' selected' : ''}>egress node</option>
              <option value="local_breakout"${draft.xray_egress_mode === 'local_breakout' || draft.xray_egress_mode === 'direct' || draft.xray_egress_mode === 'local' ? ' selected' : ''}>local breakout</option>
            </select></div>
            <div class="field"><label>Egress node</label><select name="xray_egress_node_id">${nodeOptions(draft.xray_egress_node_id || '', { roles: ['egress'], includeEmpty: true, emptyLabel: 'Auto or select egress node' })}</select></div>
            <div class="field">
              <label>Default VLESS group</label>
              <select name="xray_default_vless_group">${vlessGroupTemplateOptions(draft.xray_default_vless_group || 'default')}</select>
              <div class="field-hint">Groups are managed globally on the VLESS groups tab.</div>
            </div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/usr/local/etc/xray/xray.json')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
            <div class="field full">${renderVLESSGroupCatalogSummary(draft.xray_default_vless_group || 'default')}</div>
            <details class="field full advanced-form-section">
              <summary>Advanced JSON override</summary>
              <textarea name="config_body" rows="8" placeholder='{"inbounds":[...],"outbounds":[...]}'>${escapeHTML(draft.config_body || '')}</textarea>
            </details>`;
        case 'openvpn':
          return `${intro}
            <div class="field"><label>Protocol</label><select name="ovpn_proto">
              <option value="tcp"${draft.ovpn_proto !== 'udp' ? ' selected' : ''}>tcp</option>
              <option value="udp"${draft.ovpn_proto === 'udp' ? ' selected' : ''}>udp</option>
            </select></div>
            <div class="field"><label>Device</label><input name="ovpn_dev" value="${escapeHTML(draft.ovpn_dev || 'tun')}" /></div>
            <div class="field"><label>Server network</label><input name="ovpn_server_network" value="${escapeHTML(draft.ovpn_server_network || '')}" placeholder="auto from Address Pools" /></div>
            <div class="field"><label>Server netmask</label><input name="ovpn_server_netmask" value="${escapeHTML(draft.ovpn_server_netmask || '')}" placeholder="auto" /></div>
            <div class="field"><label>Cipher</label><input name="ovpn_cipher" value="${escapeHTML(draft.ovpn_cipher || '')}" placeholder="AES-256-GCM" /></div>
            <div class="field"><label>Auth</label><input name="ovpn_auth" value="${escapeHTML(draft.ovpn_auth || '')}" placeholder="SHA256" /></div>
            <div class="field"><label>Runtime dir</label><input name="ovpn_runtime_dir" value="${escapeHTML(draft.ovpn_runtime_dir || '')}" placeholder="/etc/openvpn/server/megavpn-edge" /></div>
            <div class="field">
              <label>Service CA profile</label>
              <select name="ovpn_pki_profile">${servicePKIProfileOptions('openvpn', draft.ovpn_pki_profile || 'default')}</select>
              <div class="field-hint">OpenVPN server/client certificates are issued from this platform PKI profile.</div>
            </div>
            <div class="field"><label>Client proxy mode</label><select name="ovpn_client_proxy_mode">
              <option value="direct"${!draft.ovpn_client_proxy_mode || draft.ovpn_client_proxy_mode === 'direct' || draft.ovpn_client_proxy_mode === 'none' ? ' selected' : ''}>direct</option>
              <option value="socks5"${draft.ovpn_client_proxy_mode === 'socks5' || draft.ovpn_client_proxy_mode === 'socks' ? ' selected' : ''}>socks5</option>
              <option value="http-connect"${draft.ovpn_client_proxy_mode === 'http-connect' || draft.ovpn_client_proxy_mode === 'http' ? ' selected' : ''}>http-connect</option>
            </select></div>
            <div class="field"><label>Client proxy host</label><input name="ovpn_proxy_host" value="${escapeHTML(draft.ovpn_proxy_host || '127.0.0.1')}" placeholder="127.0.0.1" /></div>
            <div class="field"><label>Client proxy port</label><input name="ovpn_proxy_port" type="number" min="1" max="65535" value="${escapeHTML(draft.ovpn_proxy_port || 1080)}" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/openvpn/server/server.conf')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
            <div class="field full"><label>Server extra lines</label><textarea name="ovpn_server_extra_lines" rows="5" placeholder="push &quot;redirect-gateway def1 bypass-dhcp&quot;&#10;push &quot;dhcp-option DNS 1.1.1.1&quot;&#10;push &quot;dhcp-option DNS 1.0.0.1&quot;">${escapeHTML(draft.ovpn_server_extra_lines || '')}</textarea></div>
            <div class="field full">
              <label>Client config extra lines</label>
              <textarea name="ovpn_client_extra_lines" rows="6" placeholder="# Operator notes for generated client configs&#10;# socks-proxy 127.0.0.1 1080">${escapeHTML(draft.ovpn_client_extra_lines || '')}</textarea>
              <div class="field-hint">Appended to every generated client .ovpn after certificates and keys. Use this for operator comments, proxy examples and client-side route hints.</div>
            </div>
            <div class="field full">
              <label>Full client config template</label>
              <textarea name="ovpn_client_template" rows="8" placeholder="Optional advanced override. Leave empty to keep generated client configs. Variables: {{CLIENT_NAME}}, {{CLIENT_USERNAME}}, {{INSTANCE_NAME}}, {{INSTANCE_SLUG}}, {{ENDPOINT_HOST}}, {{ENDPOINT_PORT}}.">${escapeHTML(draft.ovpn_client_template || '')}</textarea>
              <div class="field-hint">Advanced mode replaces the generated client .ovpn body. Prefer extra lines unless you intentionally own the complete client config format.</div>
            </div>
            <div class="field full"><label>Advanced server config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated OpenVPN server config.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'wireguard':
          return `${intro}
            <div class="field"><label>Network CIDR</label><input name="wg_network_cidr" value="${escapeHTML(draft.wg_network_cidr || '')}" placeholder="auto from Address Pools" /></div>
            <div class="field"><label>Server address</label><input name="wg_server_address" value="${escapeHTML(draft.wg_server_address || '')}" placeholder="auto first usable address" /></div>
            <div class="field"><label>Client address start</label><input name="wg_client_address_start" type="number" min="2" max="250" value="${escapeHTML(draft.wg_client_address_start || 10)}" /></div>
            <div class="field"><label>Client allowed IPs</label><input name="wg_client_allowed_ips" value="${escapeHTML(draft.wg_client_allowed_ips || '0.0.0.0/0, ::/0')}" placeholder="0.0.0.0/0, ::/0" /></div>
            <div class="field"><label>Client DNS</label><input name="wg_client_dns" value="${escapeHTML(draft.wg_client_dns || '1.1.1.1, 1.0.0.1')}" placeholder="1.1.1.1, 1.0.0.1" /></div>
            <div class="field"><label>Persistent keepalive</label><input name="wg_keepalive" type="number" min="0" max="300" value="${escapeHTML(draft.wg_keepalive || 25)}" /></div>
            <div class="field"><label>MTU</label><input name="wg_mtu" type="number" min="0" max="9000" value="${escapeHTML(draft.wg_mtu || '')}" placeholder="optional" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/wireguard/wg0.conf')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0600')}" /></div>
            <div class="field full"><label>Interface extra lines</label><textarea name="wg_interface_extra_lines" rows="5" placeholder="PostUp = nft add rule inet filter input udp dport 51820 accept&#10;PostDown = nft delete rule inet filter input udp dport 51820 accept">${escapeHTML(draft.wg_interface_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated WireGuard config.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'mtproto':
          return `${intro}
            <div class="field"><label>Listen address</label><input name="mtproto_listen" value="${escapeHTML(draft.mtproto_listen || '0.0.0.0')}" placeholder="0.0.0.0" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/usr/local/etc/xray/mtproto.json')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
            <div class="field full"><label>Advanced JSON override</label><textarea name="config_body" rows="12" placeholder='{"inbounds":[...],"outbounds":[...]}'>${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'nginx':
          return `${intro}
            <div class="field"><label>Mode</label><select name="nginx_mode">
              <option value="reverse_proxy"${draft.nginx_mode !== 'static' && draft.nginx_mode !== 'grpc_proxy' ? ' selected' : ''}>reverse_proxy</option>
              <option value="grpc_proxy"${draft.nginx_mode === 'grpc_proxy' ? ' selected' : ''}>grpc_proxy</option>
              <option value="static"${draft.nginx_mode === 'static' ? ' selected' : ''}>static</option>
            </select></div>
            <div class="field"><label>Managed certificate</label><select name="certificate_id">${certificateOptions(draft.certificate_id || '')}</select></div>
            <div class="field"><label>Location path</label><input name="nginx_location_path" value="${escapeHTML(draft.nginx_location_path || '/')}" placeholder="/vless-grpc or /" /></div>
            <div class="field"><label>Server name</label><input name="nginx_server_name" value="${escapeHTML(draft.nginx_server_name || '_')}" placeholder="edge.example.com" /></div>
            <div class="field"><label>Upstream URL</label><input name="nginx_upstream_url" value="${escapeHTML(draft.nginx_upstream_url || '')}" placeholder="http://127.0.0.1:9000 or grpc://127.0.0.1:7443" /></div>
            <div class="field"><label>Fallback URL</label><input name="nginx_fallback_upstream_url" value="${escapeHTML(draft.nginx_fallback_upstream_url || '')}" placeholder="https://example.com" /></div>
            <div class="field"><label>Fallback Host</label><input name="nginx_fallback_host_header" value="${escapeHTML(draft.nginx_fallback_host_header || '')}" placeholder="example.com" /></div>
            <div class="field"><label>Fallback SNI</label><input name="nginx_fallback_sni" value="${escapeHTML(draft.nginx_fallback_sni || '')}" placeholder="example.com" /></div>
            <div class="field"><label>Static root</label><input name="nginx_root_dir" value="${escapeHTML(draft.nginx_root_dir || '')}" placeholder="/var/www/html" /></div>
            <div class="field"><label>Index files</label><input name="nginx_index_files" value="${escapeHTML(draft.nginx_index_files || 'index.html index.htm')}" /></div>
            <div class="field"><label>TLS</label><select name="nginx_tls_enabled">
              <option value="false"${draft.nginx_tls_enabled !== 'true' ? ' selected' : ''}>disabled</option>
              <option value="true"${draft.nginx_tls_enabled === 'true' ? ' selected' : ''}>enabled</option>
            </select></div>
            <div class="field"><label>TLS cert path</label><input name="nginx_tls_cert_path" value="${escapeHTML(draft.nginx_tls_cert_path || '')}" placeholder="/etc/letsencrypt/live/example/fullchain.pem" /></div>
            <div class="field"><label>TLS key path</label><input name="nginx_tls_key_path" value="${escapeHTML(draft.nginx_tls_key_path || '')}" placeholder="/etc/letsencrypt/live/example/privkey.pem" /></div>
            <div class="field"><label>Body size</label><input name="nginx_client_max_body_size" value="${escapeHTML(draft.nginx_client_max_body_size || '')}" placeholder="20m" /></div>
            <div class="field"><label>Access log</label><input name="nginx_access_log" value="${escapeHTML(draft.nginx_access_log || '')}" placeholder="/var/log/nginx/megavpn-access.log" /></div>
            <div class="field"><label>Error log</label><input name="nginx_error_log" value="${escapeHTML(draft.nginx_error_log || '')}" placeholder="/var/log/nginx/megavpn-error.log warn" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/nginx/conf.d/megavpn-edge.conf')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
            <div class="field full"><label>Location extra lines</label><textarea name="nginx_location_extra_lines" rows="5" placeholder="proxy_read_timeout 60s;&#10;proxy_send_timeout 60s;">${escapeHTML(draft.nginx_location_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Fallback extra lines</label><textarea name="nginx_fallback_location_extra_lines" rows="4" placeholder="proxy_read_timeout 60s;&#10;proxy_send_timeout 60s;">${escapeHTML(draft.nginx_fallback_location_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Server extra lines</label><textarea name="nginx_server_extra_lines" rows="5" placeholder="add_header X-MegaVPN edge always;">${escapeHTML(draft.nginx_server_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated nginx server block.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'ipsec':
          return `${intro}
            <div class="field"><label>Left</label><input name="ipsec_left" value="${escapeHTML(draft.ipsec_left || '%defaultroute')}" placeholder="%defaultroute" /></div>
            <div class="field"><label>Left ID</label><input name="ipsec_leftid" value="${escapeHTML(draft.ipsec_leftid || '')}" placeholder="vpn.example.com" /></div>
            <div class="field"><label>Right</label><input name="ipsec_right" value="${escapeHTML(draft.ipsec_right || '%any')}" placeholder="%any" /></div>
            <div class="field"><label>Pre-shared key</label><input name="ipsec_psk" value="${escapeHTML(draft.ipsec_psk || '')}" placeholder="shared secret" /></div>
            <div class="field"><label>IKE</label><input name="ipsec_ike" value="${escapeHTML(draft.ipsec_ike || 'aes256-sha1-modp1024')}" /></div>
            <div class="field"><label>ESP</label><input name="ipsec_esp" value="${escapeHTML(draft.ipsec_esp || 'aes256-sha1')}" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/ipsec.conf')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
            <div class="field"><label>Secrets path</label><input name="ipsec_secrets_path" value="${escapeHTML(draft.ipsec_secrets_path || '/etc/ipsec.secrets')}" /></div>
            <div class="field"><label>Secrets mode</label><input name="ipsec_secrets_mode" value="${escapeHTML(draft.ipsec_secrets_mode || '0600')}" /></div>
            <div class="field full"><label>Config extra lines</label><textarea name="ipsec_config_extra_lines" rows="5" placeholder="ikelifetime=8h&#10;keylife=1h">${escapeHTML(draft.ipsec_config_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Secrets override</label><textarea name="ipsec_secrets_body" rows="4" placeholder="%any %any : PSK &quot;shared-secret&quot;">${escapeHTML(draft.ipsec_secrets_body || '')}</textarea></div>
            <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated ipsec.conf.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'http_proxy':
          return `${intro}
            <div class="field"><label>Auth realm</label><input name="proxy_auth_realm" value="${escapeHTML(draft.proxy_auth_realm || 'RTIS MegaVPN HTTP Proxy')}" /></div>
            <div class="field"><label>Visible hostname</label><input name="proxy_visible_hostname" value="${escapeHTML(draft.proxy_visible_hostname || '')}" placeholder="proxy.example.com" /></div>
            <div class="field"><label>Auth helper path</label><input name="proxy_auth_helper_path" value="${escapeHTML(draft.proxy_auth_helper_path || '/usr/lib/squid/basic_ncsa_auth')}" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/squid/proxy.conf')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0644')}" /></div>
            <div class="field full"><label>Config extra lines</label><textarea name="proxy_config_extra_lines" rows="6" placeholder="cache_mem 64 MB&#10;maximum_object_size_in_memory 512 KB">${escapeHTML(draft.proxy_config_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated squid.conf.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'xl2tpd':
          return `${intro}
            <div class="field"><label>Local IP</label><input name="xl2tpd_local_ip" value="${escapeHTML(draft.xl2tpd_local_ip || '')}" placeholder="auto from Address Pools" /></div>
            <div class="field"><label>Range start</label><input name="xl2tpd_ip_range_start" value="${escapeHTML(draft.xl2tpd_ip_range_start || '')}" placeholder="auto" /></div>
            <div class="field"><label>Range end</label><input name="xl2tpd_ip_range_end" value="${escapeHTML(draft.xl2tpd_ip_range_end || '')}" placeholder="auto" /></div>
            <div class="field"><label>DNS primary</label><input name="xl2tpd_dns_primary" value="${escapeHTML(draft.xl2tpd_dns_primary || '1.1.1.1')}" /></div>
            <div class="field"><label>DNS secondary</label><input name="xl2tpd_dns_secondary" value="${escapeHTML(draft.xl2tpd_dns_secondary || '1.0.0.1')}" /></div>
            <div class="field"><label>Default username</label><input name="xl2tpd_default_username" value="${escapeHTML(draft.xl2tpd_default_username || '')}" placeholder="vpnuser" /></div>
            <div class="field"><label>Default password</label><input name="xl2tpd_default_password" value="${escapeHTML(draft.xl2tpd_default_password || '')}" placeholder="shared password" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/xl2tpd/xl2tpd.conf')}" /></div>
            <div class="field"><label>Options path</label><input name="xl2tpd_options_path" value="${escapeHTML(draft.xl2tpd_options_path || '/etc/ppp/options.xl2tpd')}" /></div>
            <div class="field"><label>CHAP secrets path</label><input name="xl2tpd_chap_secrets_path" value="${escapeHTML(draft.xl2tpd_chap_secrets_path || '/etc/ppp/chap-secrets')}" /></div>
            <div class="field full"><label>CHAP secrets entries</label><textarea name="xl2tpd_chap_secrets_entries" rows="5" placeholder="vpnuser l2tpd password *">${escapeHTML(draft.xl2tpd_chap_secrets_entries || '')}</textarea></div>
            <div class="field full"><label>PPP options extra lines</label><textarea name="xl2tpd_options_extra_lines" rows="5" placeholder="idle 1800&#10;debug">${escapeHTML(draft.xl2tpd_options_extra_lines || '')}</textarea></div>
            <div class="field full"><label>XL2TPD config extra lines</label><textarea name="xl2tpd_config_extra_lines" rows="5" placeholder="ppp debug = yes">${escapeHTML(draft.xl2tpd_config_extra_lines || '')}</textarea></div>
            <div class="field full"><label>Options override</label><textarea name="xl2tpd_options_body" rows="8" placeholder="Leave empty to use generated PPP options.">${escapeHTML(draft.xl2tpd_options_body || '')}</textarea></div>
            <div class="field full"><label>Advanced config override</label><textarea name="config_body" rows="12" placeholder="Leave empty to use generated xl2tpd.conf.">${escapeHTML(draft.config_body || '')}</textarea></div>`;
        case 'shadowsocks':
          const ssPassword = generatedSecretValue(draft.ss_password);
          return `${intro}
            <div class="field"><label>Method</label><input name="ss_method" value="${escapeHTML(draft.ss_method || 'chacha20-ietf-poly1305')}" placeholder="chacha20-ietf-poly1305" /></div>
            <div class="field"><label>Mode</label><select name="ss_mode">
              <option value="tcp_only"${draft.ss_mode === 'tcp_only' ? ' selected' : ''}>tcp_only</option>
              <option value="tcp_and_udp"${draft.ss_mode !== 'tcp_only' ? ' selected' : ''}>tcp_and_udp</option>
              <option value="udp_only"${draft.ss_mode === 'udp_only' ? ' selected' : ''}>udp_only</option>
            </select></div>
            <div class="field"><label>Timeout</label><input name="ss_timeout" type="number" min="30" max="3600" value="${escapeHTML(draft.ss_timeout || 300)}" /></div>
            <div class="field generated-secret-field">
              <label>Bootstrap password</label>
              <div class="input-with-button">
                <input name="ss_password" value="${escapeHTML(ssPassword)}" autocomplete="new-password" placeholder="generated automatically" />
                <button class="secondary-btn ss-password-generate-btn" type="button">Generate</button>
              </div>
              <div class="field-hint">Generated automatically: 32 characters with lowercase, uppercase, digits and symbols.</div>
            </div>
            <div class="field"><label>Access port base</label><input name="ss_access_port_base" type="number" min="1" max="65535" value="${escapeHTML(draft.ss_access_port_base || draft.endpoint_port || 8388)}" /></div>
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '/etc/shadowsocks-libev/shadowsocks.json')}" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
            <div class="field full"><label>Advanced JSON override</label><textarea name="config_body" rows="12" placeholder='{"server":"0.0.0.0","method":"chacha20-ietf-poly1305"}'>${escapeHTML(draft.config_body || '')}</textarea></div>`;
        default:
          return `${intro}
            <div class="field"><label>Config path</label><input name="config_path" value="${escapeHTML(draft.config_path || '')}" placeholder="/etc/service/config.conf" /></div>
            <div class="field"><label>Config mode</label><input name="config_mode" value="${escapeHTML(draft.config_mode || '0640')}" /></div>
            <div class="field"><label>Config type</label><select name="config_type">
              <option value="json"${draft.config_type === 'json' ? ' selected' : ''}>json</option>
              <option value="text"${draft.config_type !== 'json' ? ' selected' : ''}>text</option>
            </select></div>
            <div class="field full"><label>Config body</label><textarea name="config_body" rows="12" placeholder="Optional config content">${escapeHTML(draft.config_body || '')}</textarea></div>`;
      }
    }

    function syncInstanceServiceFields(formID, serviceCode, draft = null, options = {}) {
      const form = document.getElementById(formID);
      if (!form) return;
      const resolvedDraft = draft || buildInstanceSpecDraft(serviceCode, null, options.presetKey || '');
      const container = form.querySelector('.service-fields');
      if (container) container.innerHTML = renderInstanceServiceFields(serviceCode, resolvedDraft);
      const passwordButton = form.querySelector('.ss-password-generate-btn');
      if (passwordButton) {
        passwordButton.addEventListener('click', () => {
          const input = form.querySelector('input[name="ss_password"]');
          if (!input) return;
          try {
            input.value = generateStrongPassword(32);
            input.setCustomValidity('');
            input.dispatchEvent(new Event('input', { bubbles: true }));
          } catch (err) {
            input.setCustomValidity(err?.message || 'Secure random generator is not available');
            input.reportValidity();
          }
        });
      }
      const portInput = form.querySelector('input[name="endpoint_port"]');
      if (portInput && resolvedDraft.endpoint_port) {
        applyAutoFieldValue(portInput, String(resolvedDraft.endpoint_port), options.forceDefaults);
      }
      if (formID === 'createInstanceForm') {
        applyCreateInstanceDefaults(form, serviceCode, resolvedDraft, options);
      }
      const presetSelect = form.querySelector('select[name="service_profile"]');
      if (presetSelect) {
        presetSelect.addEventListener('change', () => {
          syncInstanceServiceFields(formID, serviceCode, null, { forceDefaults: true, presetKey: presetSelect.value });
          }, { once: true });
      }
    }

    function buildInstanceSpecPayload(serviceCode, form, baseSpec = {}, endpointPort = 0) {
      const normalized = normalizeInstanceServiceCode(serviceCode);
      const spec = cloneJSON(baseSpec || {});
      const configBody = String(form.get('config_body') || '').trim();
      spec.service_profile = String(form.get('service_profile') || '').trim();
      spec.config_path = String(form.get('config_path') || '').trim();
      spec.config_mode = String(form.get('config_mode') || '').trim();
      if (normalized === 'xray-core') {
        const slug = slugPathPart(form.get('slug'), 'xray');
        const expectedConfigPath = `/usr/local/etc/xray/${slug}.json`;
        if (!spec.config_path || spec.config_path === '/usr/local/etc/xray/config.json') {
          spec.config_path = expectedConfigPath;
        }
        spec.security = String(form.get('xray_security') || 'reality').trim() || 'reality';
        spec.certificate_id = String(form.get('certificate_id') || '').trim();
        spec.server_port = Number(form.get('endpoint_port') || endpointPort || 443) || 443;
        spec.server_name = String(form.get('xray_server_name') || '').trim();
        spec.sni = spec.server_name;
        spec.short_id = String(form.get('xray_short_id') || '').trim();
        spec.dest = String(form.get('xray_dest') || '').trim();
        spec.fingerprint = String(form.get('xray_fingerprint') || '').trim();
        spec.network = String(form.get('xray_network') || 'tcp').trim();
        spec.path = String(form.get('xray_path') || '').trim();
        spec.service_name = String(form.get('xray_service_name') || '').trim();
        spec.flow = String(form.get('xray_flow') || '').trim();
        const xrayEgressMode = String(form.get('xray_egress_mode') || 'auto').trim() || 'auto';
        const xrayEgressNodeID = String(form.get('xray_egress_node_id') || '').trim();
        spec.egress_mode = xrayEgressMode;
        if (xrayEgressNodeID) {
          spec.egress_node_id = xrayEgressNodeID;
        } else {
          delete spec.egress_node_id;
        }
        spec.xray_egress = {
          mode: xrayEgressMode,
          ...(xrayEgressNodeID ? { egress_node_id: xrayEgressNodeID } : {}),
        };
        spec.default_vless_group = String(form.get('xray_default_vless_group') || 'default').trim() || 'default';
        spec.vless_groups = vlessGroupTemplatesAsSpecGroups();
        if (configBody) {
          spec.config_json = JSON.parse(configBody);
          delete spec.config_content;
        } else {
          delete spec.config_json;
          delete spec.config_content;
        }
        return spec;
      }
      if (normalized === 'openvpn') {
        const slug = slugPathPart(form.get('slug'), 'server');
        const expectedConfigPath = `/etc/openvpn/server/${slug}.conf`;
        if (!spec.config_path || spec.config_path === '/etc/openvpn/server/server.conf') {
          spec.config_path = expectedConfigPath;
        }
        spec.server_port = Number(form.get('endpoint_port') || endpointPort || 1194) || 1194;
        spec.proto = String(form.get('ovpn_proto') || 'tcp').trim();
        spec.dev = String(form.get('ovpn_dev') || 'tun').trim();
        const ovpnServerNetwork = String(form.get('ovpn_server_network') || '').trim();
        const ovpnServerNetmask = String(form.get('ovpn_server_netmask') || '').trim();
        if (ovpnServerNetwork) {
          spec.server_network = ovpnServerNetwork;
          spec.server_netmask = ovpnServerNetmask || '255.255.255.0';
          spec.address_pool_mode = 'manual';
        } else {
          delete spec.server_network;
          delete spec.server_netmask;
          spec.address_pool_mode = 'auto';
        }
        spec.cipher = String(form.get('ovpn_cipher') || '').trim();
        spec.auth = String(form.get('ovpn_auth') || '').trim();
        spec.runtime_dir = String(form.get('ovpn_runtime_dir') || '').trim();
        spec.pki_scope = 'platform';
        spec.pki_profile = String(form.get('ovpn_pki_profile') || 'default').trim() || 'default';
        const clientProxyMode = String(form.get('ovpn_client_proxy_mode') || 'direct').trim();
        spec.client_proxy_mode = clientProxyMode;
        delete spec.proxy_mode;
        delete spec.proxy_host;
        delete spec.proxy_port;
        delete spec.socks_proxy_host;
        delete spec.socks_proxy_port;
        delete spec.http_proxy_host;
        delete spec.http_proxy_port;
        if (clientProxyMode === 'socks5' || clientProxyMode === 'socks') {
          spec.socks_proxy_host = String(form.get('ovpn_proxy_host') || '127.0.0.1').trim() || '127.0.0.1';
          spec.socks_proxy_port = Number(form.get('ovpn_proxy_port') || 1080) || 1080;
        } else if (clientProxyMode === 'http-connect' || clientProxyMode === 'http') {
          spec.http_proxy_host = String(form.get('ovpn_proxy_host') || '127.0.0.1').trim() || '127.0.0.1';
          spec.http_proxy_port = Number(form.get('ovpn_proxy_port') || 8080) || 8080;
        }
        spec.server_extra_lines = String(form.get('ovpn_server_extra_lines') || '').trim();
        const clientExtraLines = String(form.get('ovpn_client_extra_lines') || '').trim();
        if (clientExtraLines) spec.client_extra_lines = clientExtraLines;
        else delete spec.client_extra_lines;
        const clientTemplate = String(form.get('ovpn_client_template') || '').trim();
        if (clientTemplate) {
          spec.ovpn_inline = clientTemplate;
        } else {
          delete spec.ovpn_inline;
          delete spec.client_ovpn_inline;
        }
        if (configBody) spec.config_content = configBody;
        else delete spec.config_content;
        delete spec.config_json;
        return spec;
      }
      if (normalized === 'wireguard') {
        const slug = slugPathPart(form.get('slug'), 'wg0');
        const expectedConfigPath = `/etc/wireguard/${slug}.conf`;
        if (!spec.config_path || spec.config_path === '/etc/wireguard/wg0.conf') {
          spec.config_path = expectedConfigPath;
        }
        spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 51820) || 51820;
        spec.server_port = spec.listen_port;
        const wgNetworkCIDR = String(form.get('wg_network_cidr') || '').trim();
        const wgServerAddress = String(form.get('wg_server_address') || '').trim();
        if (wgNetworkCIDR) {
          spec.network_cidr = wgNetworkCIDR;
          spec.server_address = wgServerAddress;
          spec.address_pool_mode = 'manual';
        } else {
          delete spec.network_cidr;
          delete spec.server_address;
          spec.address_pool_mode = 'auto';
        }
        spec.client_address_start = Number(form.get('wg_client_address_start') || 10) || 10;
        spec.client_allowed_ips = String(form.get('wg_client_allowed_ips') || '0.0.0.0/0, ::/0').trim();
        spec.client_dns = String(form.get('wg_client_dns') || '').trim();
        spec.persistent_keepalive = Number(form.get('wg_keepalive') || 25) || 25;
        spec.mtu = Number(form.get('wg_mtu') || 0) || 0;
        spec.interface_extra_lines = String(form.get('wg_interface_extra_lines') || '').trim();
        if (configBody) {
          spec.config_content = configBody;
        } else {
          delete spec.config_content;
        }
        delete spec.config_json;
        return spec;
      }
      if (normalized === 'mtproto') {
        const slug = slugPathPart(form.get('slug'), 'mtproto');
        const expectedConfigPath = `/usr/local/etc/xray/${slug}.json`;
        if (!spec.config_path || spec.config_path === '/usr/local/etc/xray/config.json') {
          spec.config_path = expectedConfigPath;
        }
        spec.server_port = Number(form.get('endpoint_port') || endpointPort || 443) || 443;
        spec.listen = String(form.get('mtproto_listen') || '0.0.0.0').trim();
        if (configBody) {
          spec.config_json = JSON.parse(configBody);
          delete spec.config_content;
        } else {
          delete spec.config_json;
          delete spec.config_content;
        }
        return spec;
      }
      if (normalized === 'nginx') {
        spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 8080) || 8080;
        spec.server_port = spec.listen_port;
        spec.mode = String(form.get('nginx_mode') || 'reverse_proxy').trim();
        spec.location_path = String(form.get('nginx_location_path') || '/').trim() || '/';
        spec.certificate_id = String(form.get('certificate_id') || '').trim();
        spec.server_name = String(form.get('nginx_server_name') || '').trim();
        spec.upstream_url = String(form.get('nginx_upstream_url') || '').trim();
        spec.fallback_upstream_url = String(form.get('nginx_fallback_upstream_url') || '').trim();
        spec.fallback_host_header = String(form.get('nginx_fallback_host_header') || '').trim();
        spec.fallback_sni = String(form.get('nginx_fallback_sni') || '').trim();
        spec.root_dir = String(form.get('nginx_root_dir') || '').trim();
        spec.index_files = String(form.get('nginx_index_files') || '').trim();
        spec.tls_enabled = String(form.get('nginx_tls_enabled') || 'false') === 'true';
        spec.tls_cert_path = String(form.get('nginx_tls_cert_path') || '').trim();
        spec.tls_key_path = String(form.get('nginx_tls_key_path') || '').trim();
        spec.client_max_body_size = String(form.get('nginx_client_max_body_size') || '').trim();
        spec.access_log = String(form.get('nginx_access_log') || '').trim();
        spec.error_log = String(form.get('nginx_error_log') || '').trim();
        spec.location_extra_lines = String(form.get('nginx_location_extra_lines') || '').trim();
        spec.fallback_location_extra_lines = String(form.get('nginx_fallback_location_extra_lines') || '').trim();
        spec.server_extra_lines = String(form.get('nginx_server_extra_lines') || '').trim();
        if (configBody) {
          spec.config_content = configBody;
          delete spec.config_json;
        } else {
          delete spec.config_content;
          delete spec.config_json;
        }
        return spec;
      }
      if (normalized === 'ipsec') {
        spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 1701) || 1701;
        spec.server_port = spec.listen_port;
        spec.left = String(form.get('ipsec_left') || '%defaultroute').trim();
        spec.leftid = String(form.get('ipsec_leftid') || '').trim();
        spec.right = String(form.get('ipsec_right') || '%any').trim();
        spec.psk = String(form.get('ipsec_psk') || '').trim();
        spec.ike = String(form.get('ipsec_ike') || 'aes256-sha1-modp1024').trim();
        spec.esp = String(form.get('ipsec_esp') || 'aes256-sha1').trim();
        spec.secrets_path = String(form.get('ipsec_secrets_path') || '').trim();
        spec.secrets_mode = String(form.get('ipsec_secrets_mode') || '').trim();
        spec.config_extra_lines = String(form.get('ipsec_config_extra_lines') || '').trim();
        spec.secrets_content = String(form.get('ipsec_secrets_body') || '').trim();
        if (configBody) {
          spec.config_content = configBody;
        } else {
          delete spec.config_content;
        }
        delete spec.config_json;
        return spec;
      }
      if (normalized === 'http_proxy') {
        const slug = slugPathPart(form.get('slug'), 'proxy');
        const expectedConfigPath = `/etc/squid/${slug}.conf`;
        if (!spec.config_path || spec.config_path === '/etc/squid/squid.conf') {
          spec.config_path = expectedConfigPath;
        }
        spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 3128) || 3128;
        spec.server_port = spec.listen_port;
        spec.auth_realm = String(form.get('proxy_auth_realm') || 'RTIS MegaVPN HTTP Proxy').trim();
        spec.visible_hostname = String(form.get('proxy_visible_hostname') || '').trim();
        spec.auth_helper_path = String(form.get('proxy_auth_helper_path') || '/usr/lib/squid/basic_ncsa_auth').trim();
        spec.config_extra_lines = String(form.get('proxy_config_extra_lines') || '').trim();
        if (configBody) {
          spec.config_content = configBody;
        } else {
          delete spec.config_content;
        }
        delete spec.config_json;
        return spec;
      }
      if (normalized === 'xl2tpd') {
        spec.listen_port = Number(form.get('endpoint_port') || endpointPort || 1701) || 1701;
        spec.server_port = spec.listen_port;
        const l2tpLocalIP = String(form.get('xl2tpd_local_ip') || '').trim();
        const l2tpRangeStart = String(form.get('xl2tpd_ip_range_start') || '').trim();
        const l2tpRangeEnd = String(form.get('xl2tpd_ip_range_end') || '').trim();
        if (l2tpLocalIP || l2tpRangeStart || l2tpRangeEnd) {
          spec.local_ip = l2tpLocalIP;
          spec.ip_range_start = l2tpRangeStart;
          spec.ip_range_end = l2tpRangeEnd;
          spec.address_pool_mode = 'manual';
        } else {
          delete spec.local_ip;
          delete spec.ip_range_start;
          delete spec.ip_range_end;
          spec.address_pool_mode = 'auto';
        }
        spec.ppp_dns_primary = String(form.get('xl2tpd_dns_primary') || '').trim();
        spec.ppp_dns_secondary = String(form.get('xl2tpd_dns_secondary') || '').trim();
        spec.default_username = String(form.get('xl2tpd_default_username') || '').trim();
        spec.default_password = String(form.get('xl2tpd_default_password') || '').trim();
        spec.options_path = String(form.get('xl2tpd_options_path') || '').trim();
        spec.chap_secrets_path = String(form.get('xl2tpd_chap_secrets_path') || '').trim();
        spec.chap_secrets_entries = String(form.get('xl2tpd_chap_secrets_entries') || '').trim();
        spec.options_extra_lines = String(form.get('xl2tpd_options_extra_lines') || '').trim();
        spec.config_extra_lines = String(form.get('xl2tpd_config_extra_lines') || '').trim();
        spec.options_content = String(form.get('xl2tpd_options_body') || '').trim();
        if (configBody) {
          spec.config_content = configBody;
        } else {
          delete spec.config_content;
        }
        delete spec.config_json;
        return spec;
      }
      if (normalized === 'shadowsocks') {
        const slug = slugPathPart(form.get('slug'), 'shadowsocks');
        const expectedConfigPath = `/etc/shadowsocks-libev/${slug}.json`;
        if (!spec.config_path || spec.config_path === '/etc/shadowsocks-libev/config.json') {
          spec.config_path = expectedConfigPath;
        }
        spec.server_port = Number(form.get('endpoint_port') || endpointPort || 8388) || 8388;
        spec.access_port_base = Number(form.get('ss_access_port_base') || spec.server_port || 8388) || 8388;
        spec.method = String(form.get('ss_method') || 'chacha20-ietf-poly1305').trim();
        spec.mode = String(form.get('ss_mode') || 'tcp_and_udp').trim();
        spec.timeout = Number(form.get('ss_timeout') || 300) || 300;
        spec.password = String(form.get('ss_password') || '').trim() || generateStrongPassword(32);
        if (configBody) {
          spec.config_json = JSON.parse(configBody);
          delete spec.config_content;
        } else {
          delete spec.config_json;
          delete spec.config_content;
        }
        return spec;
      }
      const configType = String(form.get('config_type') || 'json');
      if (configBody) {
        if (configType === 'json') {
          spec.config_json = JSON.parse(configBody);
          delete spec.config_content;
        } else {
          spec.config_content = configBody;
          delete spec.config_json;
        }
      } else {
        delete spec.config_json;
        delete spec.config_content;
      }
      return spec;
    }

    function renderServicePackProfilePanel(packKey) {
      const pack = servicePackByKey(packKey);
      if (!pack) return '<div class="field full"><div class="empty">No active service pack definition is available. Refresh after applying migrations, or enable a pack in the service pack catalog.</div></div>';
      const platformNotes = Array.isArray(pack.platform_notes) ? pack.platform_notes : [];
      const recommendations = Array.isArray(pack.recommendations) ? pack.recommendations : [];
      const components = Array.isArray(pack.components) ? pack.components : [];
      return `
        <div class="field full">
          <div class="code-block">
            <div><strong>${escapeHTML(pack.label || pack.key)}</strong> · <code>${escapeHTML(pack.key)}</code></div>
            <div class="metric-caption" style="margin-top:6px">${escapeHTML(pack.description || '')}</div>
            ${platformNotes.length ? `<div class="metric-caption" style="margin-top:10px">${platformNotes.map((line) => `Platform: ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
            ${recommendations.length ? `<div class="metric-caption" style="margin-top:10px">${recommendations.map((line) => `• ${escapeHTML(line)}`).join('<br>')}</div>` : ''}
            <div class="metric-caption" style="margin-top:12px">Components:</div>
            <div class="timeline" style="margin-top:8px">
              ${components.map((component) => `
                <div class="timeline-item">
                  <strong>${escapeHTML(component.label || component.service_code || 'component')}</strong>
                  <div class="timeline-meta">${escapeHTML(component.description || '')}</div>
                  <div class="metric-caption">service <code>${escapeHTML(component.service_code || 'n/a')}</code> · preset <code>${escapeHTML(component.preset_key || 'n/a')}</code> · port ${escapeHTML(String(component.endpoint_port || 0))}</div>
                </div>`).join('')}
            </div>
          </div>
        </div>`;
    }

    function syncCreateServicePackDefaults(form, packKey) {
      if (!form) return;
      const pack = servicePackByKey(packKey);
      const panel = form.querySelector('#servicePackFields');
      if (panel) panel.innerHTML = renderServicePackProfilePanel(packKey);
      if (!pack) return;
      const baseNameInput = form.querySelector('input[name="base_name"]');
      const hostInput = form.querySelector('input[name="endpoint_host"]');
      if (baseNameInput) {
        const template = String(pack.base_name_template || '').trim();
        if (template) {
          baseNameInput.placeholder = template;
          applyAutoFieldValue(baseNameInput, template, true);
        }
      }
      if (hostInput) {
        const hint = String(pack.endpoint_hint || '').trim() || 'edge.example.com';
        hostInput.placeholder = hint;
        hostInput.required = Boolean(pack.requires_endpoint_host);
      }
    }

    return {
      instanceServiceOptions,
      availableServicePacks,
      servicePackByKey,
      servicePackComponents,
      servicePackComponentUsesTLSEdgeCertificate,
      servicePackUsesTLSEdgeCertificate,
      defaultServicePack,
      servicePackOptions,
      activeLeafCertificates,
      defaultLeafCertificate,
      defaultLeafCertificateID,
      activeManagedAuthorities,
      activeServicePKIRoots,
      certificateIsExpired,
      certificateDisplayStatus,
      certificateExpiryCaption,
      certificatePrimaryLabel,
      certificateUsageCaption,
      certificateOptions,
      authorityCertificateOptions,
      servicePKIProfileOptions,
      nodeOptions,
      normalizeInstanceServiceCode,
      cloneJSON,
      stringValue,
      numberValue,
      slugPathPart,
      instanceServiceBlueprint,
      availableInstanceServices,
      defaultInstancePreset,
      resolveInstancePreset,
      applyInstancePresetDraft,
      finalizeInstanceDraft,
      renderInstanceServiceProfilePanel,
      applyAutoFieldValue,
      applyCreateInstanceDefaults,
      buildInstanceSpecDraft,
      renderInstanceServiceFields,
      syncInstanceServiceFields,
      buildInstanceSpecPayload,
      activeVLESSGroupTemplates,
      vlessGroupTemplatesAsSpecGroups,
      renderServicePackProfilePanel,
      syncCreateServicePackDefaults,
    };
  }

  window.MegaVPNDomainUI = { create: createDomainUI };
})(window);
