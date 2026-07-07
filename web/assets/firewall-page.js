(function (window) {
  'use strict';

  function createFirewallPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      statusTag,
      escapeHTML,
      formatDate,
      hasPermission,
      sendJSON,
      refresh,
      openModal,
      closeModal,
      openActionOutcomeModal,
      watchJob,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof hasPermission !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function'
    ) {
      throw new Error('MegaVPNFirewallPage requires page dependencies');
    }

    function inventory() {
      const inv = state.firewallInventory || {};
      return {
        lists: Array.isArray(inv.address_lists) ? inv.address_lists : [],
        entries: Array.isArray(inv.entries) ? inv.entries : [],
        policies: Array.isArray(inv.policies) ? inv.policies : [],
        rules: Array.isArray(inv.rules) ? inv.rules : [],
        nodeStates: Array.isArray(inv.node_states) ? inv.node_states : [],
      };
    }

    function policyByID(policyID) {
      return inventory().policies.find((policy) => policy.id === policyID || policy.key === policyID) || null;
    }

    function addressListByID(listID) {
      return inventory().lists.find((list) => list.id === listID || list.key === listID) || null;
    }

    function entryByID(entryID) {
      return inventory().entries.find((entry) => entry.id === entryID) || null;
    }

    function ruleByID(ruleID) {
      return inventory().rules.find((rule) => rule.id === ruleID) || null;
    }

    function nodeStateByID(nodeID) {
      return inventory().nodeStates.find((row) => row.node_id === nodeID) || null;
    }

    function rulesForPolicy(policyID) {
      return inventory().rules.filter((rule) => rule.policy_id === policyID);
    }

    function ensureFirewallFilters() {
      if (!state.firewallFilters || typeof state.firewallFilters !== 'object') {
        state.firewallFilters = {};
      }
      const filters = state.firewallFilters;
      if (!filters.ruleSearch) filters.ruleSearch = '';
      if (!filters.rulePolicy) filters.rulePolicy = 'all';
      if (!filters.ruleChain) filters.ruleChain = 'all';
      if (!filters.ruleAction) filters.ruleAction = 'all';
      if (!filters.addressSearch) filters.addressSearch = '';
      return filters;
    }

    function policyDefaults(policy = {}) {
      return {
        input: String(policy.default_input_policy || 'accept').toLowerCase(),
        forward: String(policy.default_forward_policy || 'accept').toLowerCase(),
        output: String(policy.default_output_policy || 'accept').toLowerCase(),
      };
    }

    function policyHasStrictDefaults(policy) {
      const defaults = policyDefaults(policy);
      return defaults.input !== 'accept' || defaults.forward !== 'accept' || defaults.output !== 'accept';
    }

    function policyPosture(policy, rules = []) {
      const defaults = policyDefaults(policy);
      const enabledRules = rules.filter((rule) => rule.enabled !== false && String(rule.status || 'active').toLowerCase() === 'active');
      if (String(policy.status || 'active').toLowerCase() !== 'active') {
        return { level: 'disabled', label: 'Disabled', detail: 'not selectable for normal rollout' };
      }
      if (!policyHasStrictDefaults(policy)) {
        return { level: 'observe', label: 'Observe apply', detail: 'defaults stay accept unless strict mode is selected' };
      }
      if (!enabledRules.length) {
        return { level: 'warn', label: 'Strict needs rules', detail: 'non-accept defaults with no active allow rules' };
      }
      if (defaults.output !== 'accept') {
        return { level: 'warn', label: 'Output guard needed', detail: 'strict output requires control-plane egress' };
      }
      return { level: 'ready', label: 'Strict ready', detail: 'non-accept defaults with active catalog rules' };
    }

    function defaultPolicyPill(label, action) {
      const normalized = String(action || 'accept').toLowerCase();
      return `
        <div class="firewall-default-pill ${escapeHTML(normalized)}">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(normalized)}</strong>
        </div>`;
    }

    function actionPill(action) {
      const normalized = String(action || 'unknown').toLowerCase();
      return `<span class="firewall-action-pill ${escapeHTML(normalized)}">${escapeHTML(normalized)}</span>`;
    }

    function chainChip(chain) {
      return `<span class="firewall-chain-chip">${escapeHTML(chain || 'input')}</span>`;
    }

    function firewallNodeStatusTag(value) {
      const normalized = String(value || 'unknown').toLowerCase();
      const cls = {
        applied: 'ok',
        disabled: 'stub',
        pending_disable: 'warn',
        pending: 'warn',
        stale: 'warn',
        failed: 'danger',
        unknown: 'stub',
      }[normalized] || 'stub';
      const label = {
        applied: 'applied',
        disabled: 'disabled',
        pending_disable: 'disabling',
      }[normalized] || normalized;
      return `<span class="firewall-node-status ${escapeHTML(cls)}"><span class="dot ${cls === 'stub' ? 'unknown' : cls}"></span>${escapeHTML(label)}</span>`;
    }

    function renderSummary() {
      const inv = inventory();
      const activePolicies = inv.policies.filter((policy) => policy.status === 'active').length;
      const applied = inv.nodeStates.filter((item) => item.status === 'applied').length;
      const failed = inv.nodeStates.filter((item) => item.status === 'failed').length;
      const strictPolicies = inv.policies.filter(policyHasStrictDefaults).length;
      return `
        <div class="firewall-summary-grid">
          <div class="pool-summary-card"><span>Policies</span><strong>${escapeHTML(String(inv.policies.length))}</strong><small>${escapeHTML(String(activePolicies))} active</small></div>
          <div class="pool-summary-card"><span>Rules</span><strong>${escapeHTML(String(inv.rules.length))}</strong><small>ordered by policy priority</small></div>
          <div class="pool-summary-card"><span>Address groups</span><strong>${escapeHTML(String(inv.lists.length))}</strong><small>${escapeHTML(String(inv.entries.length))} entries</small></div>
          <div class="pool-summary-card"><span>Strict policies</span><strong>${escapeHTML(String(strictPolicies))}</strong><small>${escapeHTML(String(applied))} applied · ${escapeHTML(String(failed))} failed</small></div>
        </div>`;
    }

    function renderFirewallModelGuide() {
      const steps = [
        ['Address groups', 'Reusable source and destination groups', 'CIDR · IP · range'],
        ['Rules', 'Priority, chain, protocol, ports and action', 'lower priority first'],
        ['Policy', 'Default input, forward and output posture', 'observe or strict'],
        ['Apply job', 'Signed node job renders nftables payload', 'node-scoped rollout'],
        ['Node apply', 'Observed revision, rule counts and failures', 'operator evidence'],
      ];
      return `
        <section class="table-card firewall-model-card">
          <div class="table-head">
            <div>
              <h2>Firewall model</h2>
              <div class="metric-caption">Build policy in the catalog first, then apply a snapshot to a selected node.</div>
            </div>
            <div class="table-tools"><span class="tag">catalog -> apply -> observe</span></div>
          </div>
          <div class="firewall-flow-diagram" aria-label="Firewall configuration flow">
            ${steps.map((step, index) => `
              <div class="firewall-flow-step">
                <span>${escapeHTML(String(index + 1))}</span>
                <strong>${escapeHTML(step[0])}</strong>
                <small>${escapeHTML(step[1])}</small>
                <code>${escapeHTML(step[2])}</code>
              </div>
              ${index < steps.length - 1 ? '<div class="firewall-flow-arrow" aria-hidden="true">-></div>' : ''}`).join('')}
          </div>
          <div class="firewall-model-notes">
            <div><strong>Rules only</strong><span>Installs explicit catalog rules while base chain policies stay accept.</span></div>
            <div><strong>Strict defaults</strong><span>Enforces input, forward and output defaults after safety rules are rendered.</span></div>
            <div><strong>Managed table</strong><span>Node apply owns <code>inet megavpn_firewall</code>; manual node rules are not persistent product state.</span></div>
          </div>
        </section>`;
    }

    function renderPolicyCard(policy) {
      const rules = rulesForPolicy(policy.id);
      const posture = policyPosture(policy, rules);
      const canManage = hasPermission('firewall.manage');
      const canApply = hasPermission('firewall.apply');
      const protectedPolicy = policy.key === 'control_plane_default' || policy.key === 'node_base';
      return `
        <article class="firewall-policy-card">
          <div class="firewall-policy-head">
            <div>
              <h3>${escapeHTML(policy.label || policy.key)}</h3>
              <div class="metric-caption"><code>${escapeHTML(policy.key || policy.id)}</code> · ${escapeHTML(policy.scope || 'node')}${policy.node_name ? ` · ${escapeHTML(policy.node_name)}` : ''}</div>
            </div>
            ${statusTag(policy.status || 'unknown')}
          </div>
          ${policy.description ? `<p class="pool-description">${escapeHTML(policy.description)}</p>` : ''}
          <div class="firewall-policy-posture ${escapeHTML(posture.level)}">
            <strong>${escapeHTML(posture.label)}</strong>
            <span>${escapeHTML(posture.detail)}</span>
          </div>
          <div class="firewall-policy-facts">
            ${defaultPolicyPill('Input', policy.default_input_policy || 'accept')}
            ${defaultPolicyPill('Forward', policy.default_forward_policy || 'accept')}
            ${defaultPolicyPill('Output', policy.default_output_policy || 'accept')}
            <div><span>Rules</span><strong>${escapeHTML(String(rules.length))}</strong></div>
          </div>
          <div class="firewall-rule-preview">
            ${rules.length ? rules.slice(0, 4).map((rule) => `
              <div>
                <code>${escapeHTML(String(rule.priority || 1000))}</code>
                ${actionPill(rule.action || 'unknown')}
                <span>${escapeHTML(rule.chain || 'input')} · ${escapeHTML(rule.protocol || 'any')} · ${escapeHTML(firstText(rule.src_cidr, 'any'))} -> ${escapeHTML(firstText(rule.dst_cidr, 'any'))}</span>
              </div>`).join('') : '<div class="empty compact">No rules configured. Strict default policies are only enforced when enabled during apply.</div>'}
          </div>
          <div class="pool-actions">
            <button class="secondary-btn firewall-edit-policy-btn" type="button" data-policy-id="${escapeHTML(policy.id)}"${canManage ? '' : ' disabled'}>Edit policy</button>
            <button class="secondary-btn firewall-add-rule-btn" type="button" data-policy-id="${escapeHTML(policy.id)}"${canManage ? '' : ' disabled'}>Add rule</button>
            <button class="secondary-btn firewall-preview-btn" type="button" data-policy-id="${escapeHTML(policy.id)}"${canApply ? '' : ' disabled'}>Preview</button>
            <button class="primary-btn apply-btn firewall-apply-btn" type="button" data-policy-id="${escapeHTML(policy.id)}"${canApply ? '' : ' disabled'}>Apply to node</button>
            <button class="danger-btn firewall-delete-policy-btn" type="button" data-policy-id="${escapeHTML(policy.id)}" data-policy-name="${escapeHTML(policy.label || policy.key)}"${canManage && !protectedPolicy ? '' : ' disabled'}>Delete</button>
          </div>
        </article>`;
    }

    function entrySearchText(entry, list = {}) {
      return [
        entry.value,
        entry.value_type,
        entry.label,
        entry.status,
        entry.list_key,
        list.key,
        list.label,
        list.scope,
      ].map((value) => String(value || '').toLowerCase()).join(' ');
    }

    function listSearchText(list, entries = []) {
      return [
        list.key,
        list.label,
        list.description,
        list.scope,
        list.status,
        ...entries.map((entry) => entrySearchText(entry, list)),
      ].map((value) => String(value || '').toLowerCase()).join(' ');
    }

    function filteredAddressLists() {
      const inv = inventory();
      const query = String(ensureFirewallFilters().addressSearch || '').trim().toLowerCase();
      if (!query) return inv.lists;
      return inv.lists.filter((list) => listSearchText(list, inv.entries.filter((entry) => entry.list_id === list.id)).includes(query));
    }

    function filteredAddressEntries() {
      const inv = inventory();
      const query = String(ensureFirewallFilters().addressSearch || '').trim().toLowerCase();
      if (!query) return inv.entries;
      return inv.entries.filter((entry) => entrySearchText(entry, addressListByID(entry.list_id) || {}).includes(query));
    }

    function renderAddressLists(lists = filteredAddressLists()) {
      if (!lists.length) {
        const filtered = String(ensureFirewallFilters().addressSearch || '').trim() !== '';
        return `<tr><td colspan="5"><div class="empty">${filtered ? 'No address groups match the current filter.' : `No address groups configured.${hasPermission('firewall.manage') ? ' Create a group before adding source or destination matchers.' : ''}`}</div></td></tr>`;
      }
      return lists.map((list) => `
        <tr>
          <td><strong>${escapeHTML(list.label || list.key)}</strong><br><code>${escapeHTML(list.key || list.id)}</code></td>
          <td>${escapeHTML(list.scope || 'global')}</td>
          <td>${escapeHTML(String(list.entry_count || 0))}</td>
          <td>${statusTag(list.status || 'unknown')}</td>
          <td>
            <div class="compact-action-grid">
              <button class="secondary-btn firewall-add-entry-btn" type="button" data-list-id="${escapeHTML(list.id)}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Add entry</button>
              <button class="secondary-btn firewall-edit-list-btn" type="button" data-list-id="${escapeHTML(list.id)}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Edit</button>
              <button class="danger-btn firewall-delete-list-btn" type="button" data-list-id="${escapeHTML(list.id)}" data-list-name="${escapeHTML(list.label || list.key)}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Delete</button>
            </div>
          </td>
        </tr>`).join('');
    }

    function renderEntryRows(entries = filteredAddressEntries()) {
      if (!entries.length) {
        const filtered = String(ensureFirewallFilters().addressSearch || '').trim() !== '';
        return `<tr><td colspan="6"><div class="empty">${filtered ? 'No address entries match the current filter.' : `No address entries configured.${hasPermission('firewall.manage') ? ' Add entries to reusable address groups.' : ''}`}</div></td></tr>`;
      }
      return entries.map((entry) => {
        const list = addressListByID(entry.list_id);
        return `
          <tr>
            <td><strong>${escapeHTML(list?.label || entry.list_key || 'group')}</strong><br><code>${escapeHTML(entry.list_key || entry.list_id || '')}</code></td>
            <td><code>${escapeHTML(entry.value || '')}</code></td>
            <td>${escapeHTML(entry.value_type || 'auto')}</td>
            <td>${escapeHTML(entry.label || '')}</td>
            <td>${statusTag(entry.status || 'unknown')}</td>
            <td>
              <div class="compact-action-grid">
                <button class="secondary-btn firewall-edit-entry-btn" type="button" data-list-id="${escapeHTML(entry.list_id)}" data-entry-id="${escapeHTML(entry.id)}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Edit</button>
                <button class="danger-btn firewall-delete-entry-btn" type="button" data-list-id="${escapeHTML(entry.list_id)}" data-entry-id="${escapeHTML(entry.id)}" data-entry-value="${escapeHTML(entry.value || '')}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Delete</button>
              </div>
            </td>
          </tr>`;
      }).join('');
    }

    function ruleSearchText(rule) {
      const policy = policyByID(rule.policy_id) || {};
      return [
        rule.priority,
        rule.chain,
        rule.action,
        rule.protocol,
        rule.src_list_key,
        rule.dst_list_key,
        rule.src_cidr,
        rule.dst_cidr,
        rule.src_ports,
        rule.dst_ports,
        Array.isArray(rule.state_match) ? rule.state_match.join(',') : '',
        rule.comment,
        rule.status,
        policy.label,
        policy.key,
      ].map((value) => String(value || '').toLowerCase()).join(' ');
    }

    function filteredRules() {
      const inv = inventory();
      const filters = ensureFirewallFilters();
      const query = String(filters.ruleSearch || '').trim().toLowerCase();
      return inv.rules.filter((rule) => {
        if (filters.rulePolicy !== 'all' && rule.policy_id !== filters.rulePolicy) return false;
        if (filters.ruleChain !== 'all' && String(rule.chain || 'input') !== filters.ruleChain) return false;
        if (filters.ruleAction !== 'all' && String(rule.action || 'accept') !== filters.ruleAction) return false;
        if (query && !ruleSearchText(rule).includes(query)) return false;
        return true;
      });
    }

    function renderRuleRows(rows = filteredRules()) {
      if (!rows.length) {
        const filters = ensureFirewallFilters();
        const filtered = String(filters.ruleSearch || '').trim() !== '' || filters.rulePolicy !== 'all' || filters.ruleChain !== 'all' || filters.ruleAction !== 'all';
        return `<tr><td colspan="9"><div class="empty">${filtered ? 'No firewall rules match the current filters.' : `No firewall rules configured.${hasPermission('firewall.manage') ? ' Create a policy rule or keep default accept.' : ''}`}</div></td></tr>`;
      }
      return rows.map((rule) => {
        const policy = policyByID(rule.policy_id);
        const src = firstText(rule.src_list_key ? '@' + rule.src_list_key : '', rule.src_cidr, 'any');
        const dst = firstText(rule.dst_list_key ? '@' + rule.dst_list_key : '', rule.dst_cidr, 'any');
        const ports = firstText(rule.dst_ports, rule.src_ports, 'any');
        const enabled = rule.enabled !== false && String(rule.status || 'active').toLowerCase() === 'active';
        const states = Array.isArray(rule.state_match) && rule.state_match.length ? rule.state_match.join(', ') : 'any state';
        return `
          <tr>
            <td><code>${escapeHTML(String(rule.priority || 1000))}</code></td>
            <td>${escapeHTML(policy?.label || policy?.key || rule.policy_id || 'policy')}</td>
            <td>${chainChip(rule.chain || 'input')}</td>
            <td>${actionPill(rule.action || 'unknown')}</td>
            <td>${escapeHTML(rule.protocol || 'any')}</td>
            <td><code>${escapeHTML(src)}</code></td>
            <td><code>${escapeHTML(dst)}</code><br><span class="metric-caption">ports ${escapeHTML(ports)} · ${escapeHTML(states)}</span></td>
            <td>${escapeHTML(rule.comment || '')}<br><span class="tag ${enabled ? 'ok' : 'stub'}">${enabled ? 'enabled' : 'disabled'}</span></td>
            <td>
              <div class="compact-action-grid">
                <button class="secondary-btn firewall-edit-rule-btn" type="button" data-policy-id="${escapeHTML(rule.policy_id)}" data-rule-id="${escapeHTML(rule.id)}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Edit</button>
                <button class="danger-btn firewall-delete-rule-btn" type="button" data-policy-id="${escapeHTML(rule.policy_id)}" data-rule-id="${escapeHTML(rule.id)}"${hasPermission('firewall.manage') ? '' : ' disabled'}>Delete</button>
              </div>
            </td>
          </tr>`;
      }).join('');
    }

    function renderNodeStates() {
      const inv = inventory();
      const rows = (state.nodes || []).map((node) => {
        const item = inv.nodeStates.find((row) => row.node_id === node.id);
        const status = String(item?.status || 'unknown').toLowerCase();
        const observed = item?.observed || {};
        const enforcement = firstText(observed.default_policy_enforcement, 'not applied');
        const ruleCount = observed.rule_count === undefined ? 'n/a' : String(observed.rule_count);
        const systemRuleCount = observed.system_rule_count === undefined ? 'n/a' : String(observed.system_rule_count);
        return `
          <tr class="firewall-node-row is-${escapeHTML(status)}">
            <td><strong>${escapeHTML(node.name || node.id)}</strong><br><span class="metric-caption">${escapeHTML(node.role || 'node')} · ${escapeHTML(node.address || 'n/a')}</span></td>
            <td>${escapeHTML(item?.policy_key || 'node_base')}<br><span class="metric-caption">${escapeHTML(enforcement)} · rules ${escapeHTML(ruleCount)} · system ${escapeHTML(systemRuleCount)}</span></td>
            <td>${firewallNodeStatusTag(status)}</td>
            <td>${escapeHTML(item?.updated_at ? formatDate(item.updated_at) : 'n/a')}</td>
            <td>
              <div class="compact-action-grid">
                <button class="secondary-btn firewall-node-preview-btn" type="button" data-node-id="${escapeHTML(node.id)}"${hasPermission('firewall.apply') ? '' : ' disabled'}>Preview</button>
                <button class="primary-btn apply-btn firewall-node-apply-btn" type="button" data-node-id="${escapeHTML(node.id)}"${hasPermission('firewall.apply') ? '' : ' disabled'}>Apply</button>
                <button class="danger-btn firewall-node-disable-btn" type="button" data-node-id="${escapeHTML(node.id)}" data-node-name="${escapeHTML(node.name || node.id)}"${hasPermission('firewall.apply') ? '' : ' disabled'}>Disable</button>
              </div>
            </td>
          </tr>`;
      });
      return rows.length ? rows.join('') : '<tr><td colspan="5"><div class="empty">No active nodes registered.</div></td></tr>';
    }

    function selectedTab() {
      const allowed = new Set(['overview', 'policies', 'rules', 'addressLists', 'nodeState']);
      return allowed.has(state.firewallTab) ? state.firewallTab : 'overview';
    }

    function renderTabs(inv) {
      const failed = inv.nodeStates.filter((item) => item.status === 'failed').length;
      const nodeCount = Array.isArray(state.nodes) ? state.nodes.length : 0;
      const tabs = [
        { key: 'overview', label: 'Overview', count: String(inv.policies.length), hint: 'Posture and counters' },
        { key: 'policies', label: 'Policies', count: String(inv.policies.length), hint: 'Node policy sets' },
        { key: 'rules', label: 'Rules', count: String(inv.rules.length), hint: 'Ordered firewall rules' },
        { key: 'addressLists', label: 'Address groups', count: String(inv.lists.length), hint: `${inv.entries.length} reusable entries` },
        { key: 'nodeState', label: 'Node apply', count: String(nodeCount), hint: failed ? `${failed} failed apply` : 'Preview and apply by node' },
      ];
      const active = selectedTab();
      return `
        <nav class="page-tabs" role="tablist" aria-label="Firewall sections">
          ${tabs.map((tab) => `
            <button class="page-tab${tab.key === active ? ' is-active' : ''}" type="button" role="tab" aria-selected="${tab.key === active ? 'true' : 'false'}" data-firewall-tab="${escapeHTML(tab.key)}">
              <span>${escapeHTML(tab.label)} <em>${escapeHTML(tab.count)}</em></span>
              <small>${escapeHTML(tab.hint)}</small>
            </button>`).join('')}
        </nav>`;
    }

    function renderActionToolbar(inv, active) {
      const canManage = hasPermission('firewall.manage');
      const canApply = hasPermission('firewall.apply');
      const firstPolicy = inv.policies.find((policy) => policy.key === 'node_base') || inv.policies[0] || {};
      const firstNode = (state.nodes || [])[0] || {};
      const applyDisabled = !firstPolicy.id || !firstNode.id;
      if (!canManage && !canApply) {
        return `
          <section class="section-card firewall-toolbar readonly">
            <div class="firewall-toolbar-copy">
              <h3>Read-only firewall catalog</h3>
              <p>Your current role can inspect policies, rules and node apply state, but cannot change or apply firewall configuration.</p>
            </div>
            <div class="tag">missing firewall.manage / firewall.apply</div>
          </section>`;
      }
      const tabCopy = {
        overview: ['Overview', 'Review policy posture before changing node firewall state.'],
        policies: ['Policies', 'Create reusable node policy sets and attach ordered rules.'],
        rules: ['Rules', 'Add or edit ordered match/action rows. Lower priority numbers run first.'],
        addressLists: ['Address groups', 'Create named groups first, then add CIDR, IP or range entries to those groups.'],
        nodeState: ['Node apply', 'Preview, apply or disable the managed firewall on a concrete node. Use the row actions below.'],
      }[active] || ['Firewall', 'Manage catalog objects and apply them to nodes.'];
      const actionHTML = [];
      if (canManage && (active === 'overview' || active === 'policies')) {
        actionHTML.push('<button class="secondary-btn" id="addFirewallPolicyBtnTop" type="button">New policy</button>');
      }
      if (canManage && (active === 'overview' || active === 'addressLists')) {
        actionHTML.push('<button class="primary-btn toolbar-primary-action" id="addFirewallListBtnTop" type="button">New group</button>');
        actionHTML.push('<button class="secondary-btn toolbar-secondary-action" id="addFirewallEntryBtnTop" type="button">Add entry</button>');
      }
      if (canManage && (active === 'overview' || active === 'policies' || active === 'rules')) {
        actionHTML.push('<button class="primary-btn" id="addFirewallRuleBtnTop" type="button">New rule</button>');
      }
      if (canApply && (active === 'overview' || active === 'policies')) {
        actionHTML.push(`<button class="secondary-btn firewall-preview-quick-btn" type="button" data-policy-id="${escapeHTML(firstPolicy.id || '')}" data-node-id="${escapeHTML(firstNode.id || '')}"${applyDisabled ? ' disabled' : ''}>Preview</button>`);
        actionHTML.push(`<button class="primary-btn apply-btn firewall-apply-quick-btn" type="button" data-policy-id="${escapeHTML(firstPolicy.id || '')}" data-node-id="${escapeHTML(firstNode.id || '')}"${applyDisabled ? ' disabled' : ''}>Apply to node</button>`);
      }
      return `
        <section class="section-card firewall-toolbar">
          <div class="firewall-toolbar-copy">
            <h3>${escapeHTML(tabCopy[0])}</h3>
            <p>${escapeHTML(tabCopy[1])}</p>
          </div>
          <div class="firewall-toolbar-actions">${actionHTML.join('')}</div>
        </section>`;
    }

    function renderPolicySection(inv) {
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Firewall policies</h2>
              <div class="metric-caption">Node policies rendered into managed nftables chains.</div>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(inv.policies.length))} policies</span>
            </div>
          </div>
          <div class="firewall-policy-grid">
            ${inv.policies.length ? inv.policies.map(renderPolicyCard).join('') : `<div class="empty">No firewall policies configured.${hasPermission('firewall.manage') ? ' Create a policy first, then add rules to it.' : ''}</div>`}
          </div>
        </section>`;
    }

    function renderNodeStateSection(inv) {
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Node firewall state</h2>
              <div class="metric-caption">Last observed policy apply state per managed node.</div>
            </div>
            <div class="table-tools"><span class="tag">${escapeHTML(String(inv.nodeStates.length))} observed</span></div>
          </div>
          <div class="table-wrap">
            <table class="firewall-node-state-table"><thead><tr><th>Node</th><th>Policy</th><th>Status</th><th>Updated</th><th>Actions</th></tr></thead><tbody>${renderNodeStates()}</tbody></table>
          </div>
        </section>`;
    }

    function renderRuleFilters(inv, rows) {
      const filters = ensureFirewallFilters();
      return `
        <div class="firewall-filter-bar">
          <input id="firewallRuleSearchInput" class="compact-search" value="${escapeHTML(filters.ruleSearch || '')}" placeholder="Search policy, CIDR, list, port, comment">
          <select id="firewallRulePolicyFilter">
            <option value="all"${filters.rulePolicy === 'all' ? ' selected' : ''}>All policies</option>
            ${inv.policies.map((policy) => `<option value="${escapeHTML(policy.id)}"${filters.rulePolicy === policy.id ? ' selected' : ''}>${escapeHTML(policy.label || policy.key)}</option>`).join('')}
          </select>
          <select id="firewallRuleChainFilter">
            ${selectOption('all', filters.ruleChain, 'All chains')}
            ${selectOption('input', filters.ruleChain)}
            ${selectOption('forward', filters.ruleChain)}
            ${selectOption('output', filters.ruleChain)}
          </select>
          <select id="firewallRuleActionFilter">
            ${selectOption('all', filters.ruleAction, 'All actions')}
            ${selectOption('accept', filters.ruleAction)}
            ${selectOption('drop', filters.ruleAction)}
            ${selectOption('reject', filters.ruleAction)}
          </select>
          <button class="secondary-btn" id="applyFirewallRuleFiltersBtn" type="button">Apply filters</button>
          <button class="secondary-btn" id="resetFirewallRuleFiltersBtn" type="button">Reset</button>
          <span class="tag">${escapeHTML(String(rows.length))} shown</span>
        </div>`;
    }

    function renderRulesSection(inv) {
      const rows = filteredRules();
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Rules</h2>
              <div class="metric-caption">Rules are evaluated by priority inside their selected policy.</div>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(inv.rules.length))} rules</span>
            </div>
          </div>
          ${renderRuleAnatomyGuide()}
          ${renderRuleFilters(inv, rows)}
          <div class="table-wrap">
            <table class="firewall-rules-table"><thead><tr><th>Priority</th><th>Policy</th><th>Chain</th><th>Action</th><th>Proto</th><th>Source</th><th>Destination</th><th>Comment</th><th>Actions</th></tr></thead><tbody>${renderRuleRows(rows)}</tbody></table>
          </div>
        </section>`;
    }

    function renderRuleAnatomyGuide() {
      const items = [
        ['Priority', 'execution order'],
        ['Chain', 'input / forward / output'],
        ['Match', 'source, destination, protocol, port and state'],
        ['Action', 'accept / drop / reject'],
        ['Apply mode', 'rules only or strict defaults'],
      ];
      return `
        <div class="firewall-rule-anatomy" aria-label="Firewall rule anatomy">
          <strong>Rule reads left to right</strong>
          ${items.map((item, index) => `
            <div>
              <span>${escapeHTML(String(index + 1))}</span>
              <code>${escapeHTML(item[0])}</code>
              <small>${escapeHTML(item[1])}</small>
            </div>`).join('')}
        </div>`;
    }

    function renderAddressFilters(listRows, entryRows) {
      const filters = ensureFirewallFilters();
      return `
        <div class="firewall-filter-bar">
          <input id="firewallAddressSearchInput" class="compact-search" value="${escapeHTML(filters.addressSearch || '')}" placeholder="Search group, CIDR, range, DNS, label">
          <button class="secondary-btn" id="applyFirewallAddressFiltersBtn" type="button">Apply filters</button>
          <button class="secondary-btn" id="resetFirewallAddressFiltersBtn" type="button">Reset</button>
          <span class="tag">${escapeHTML(String(listRows.length))} groups</span>
          <span class="tag">${escapeHTML(String(entryRows.length))} entries</span>
        </div>`;
    }

    function renderAddressListsSection(inv) {
      const listRows = filteredAddressLists();
      const entryRows = filteredAddressEntries();
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Address groups</h2>
              <div class="metric-caption">Create named groups, then use them in rule source or destination matchers.</div>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(inv.lists.length))} groups</span>
              <span class="tag">${escapeHTML(String(inv.entries.length))} entries</span>
            </div>
          </div>
          ${renderAddressFilters(listRows, entryRows)}
          <div class="table-wrap">
            <table class="firewall-address-list-table"><thead><tr><th>Group</th><th>Scope</th><th>Entries</th><th>Status</th><th>Actions</th></tr></thead><tbody>${renderAddressLists(listRows)}</tbody></table>
          </div>
          <div class="firewall-subtable-head">
            <div>
              <h3>Address entries</h3>
              <span>Concrete CIDR, IP, range or catalog DNS values inside the groups above.</span>
            </div>
            <span class="tag">${escapeHTML(String(entryRows.length))} shown</span>
          </div>
          <div class="table-wrap firewall-entry-wrap">
            <table class="firewall-address-entry-table"><thead><tr><th>Group</th><th>Value</th><th>Type</th><th>Label</th><th>Status</th><th>Actions</th></tr></thead><tbody>${renderEntryRows(entryRows)}</tbody></table>
          </div>
        </section>`;
    }

    function renderFirewallPosturePanel(inv) {
      const strictPolicies = inv.policies.filter(policyHasStrictDefaults);
      const outputStrict = strictPolicies.filter((policy) => policyDefaults(policy).output !== 'accept').length;
      const zeroRuleStrict = strictPolicies.filter((policy) => !rulesForPolicy(policy.id).filter((rule) => rule.enabled !== false && String(rule.status || 'active').toLowerCase() === 'active').length).length;
      const activeAccept = inv.policies.filter((policy) => !policyHasStrictDefaults(policy) && String(policy.status || 'active').toLowerCase() === 'active').length;
      return `
        <section class="table-card firewall-posture-card">
          <div class="table-head">
            <div>
              <h2>Enforcement posture</h2>
              <div class="metric-caption">Default policies are opt-in at apply time; rules remain explicit catalog data.</div>
            </div>
            <div class="table-tools">${statusTag(zeroRuleStrict || outputStrict ? 'pending' : 'ready')}</div>
          </div>
          <div class="firewall-posture-grid">
            <div><span>Default accept</span><strong>${escapeHTML(String(activeAccept))}</strong><small>active observe-mode baselines</small></div>
            <div><span>Strict candidates</span><strong>${escapeHTML(String(strictPolicies.length))}</strong><small>policies with drop/reject defaults</small></div>
            <div><span>Output guarded</span><strong>${escapeHTML(String(outputStrict))}</strong><small>requires control-plane egress</small></div>
            <div><span>Needs allow rules</span><strong>${escapeHTML(String(zeroRuleStrict))}</strong><small>strict defaults with no active rules</small></div>
          </div>
        </section>`;
    }

    function renderOverview(inv) {
      const activePolicies = inv.policies.filter((policy) => policy.status === 'active').length;
      const activeRules = inv.rules.filter((rule) => (rule.status || 'active') === 'active' && rule.enabled !== false).length;
      const failed = inv.nodeStates.filter((item) => item.status === 'failed').length;
      const stale = inv.nodeStates.filter((item) => item.status === 'pending' || item.status === 'stale').length;
      return `
        ${renderSummary()}
        ${renderFirewallModelGuide()}
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Policy posture</h2>
              <div class="metric-caption">Firewall catalog status without forcing live refresh loops.</div>
            </div>
            <div class="table-tools">${statusTag(failed ? 'failed' : stale ? 'pending' : 'ready')}</div>
          </div>
          <div class="firewall-overview-grid">
            <div class="pool-summary-card"><span>Active policies</span><strong>${escapeHTML(String(activePolicies))}</strong><small>available for node apply</small></div>
            <div class="pool-summary-card"><span>Enabled rules</span><strong>${escapeHTML(String(activeRules))}</strong><small>${escapeHTML(String(inv.rules.length - activeRules))} disabled or inactive</small></div>
            <div class="pool-summary-card"><span>Apply drift</span><strong>${escapeHTML(String(stale))}</strong><small>pending or stale node reports</small></div>
            <div class="pool-summary-card"><span>Failures</span><strong>${escapeHTML(String(failed))}</strong><small>requires operator action</small></div>
          </div>
        </section>
        ${renderFirewallPosturePanel(inv)}`;
    }

    function render() {
      setTitle('Firewall');
      const inv = inventory();
      const active = selectedTab();
      el('content').innerHTML = `
        <div class="control-page-shell firewall-page-shell">
          ${renderTabs(inv)}
          ${renderActionToolbar(inv, active)}
          <div class="firewall-tab-panel bounded-stack" data-firewall-panel="overview"${active === 'overview' ? '' : ' hidden'}>${renderOverview(inv)}</div>
          <div class="firewall-tab-panel bounded-stack" data-firewall-panel="policies"${active === 'policies' ? '' : ' hidden'}>${renderPolicySection(inv)}</div>
          <div class="firewall-tab-panel bounded-stack" data-firewall-panel="rules"${active === 'rules' ? '' : ' hidden'}>${renderRulesSection(inv)}</div>
          <div class="firewall-tab-panel bounded-stack" data-firewall-panel="addressLists"${active === 'addressLists' ? '' : ' hidden'}>${renderAddressListsSection(inv)}</div>
          <div class="firewall-tab-panel bounded-stack" data-firewall-panel="nodeState"${active === 'nodeState' ? '' : ' hidden'}>${renderNodeStateSection(inv)}</div>
        </div>`;
      bindTabs();
      bindActions();
      bindRuleFilters();
      bindAddressFilters();
    }

    function bindTabs() {
      document.querySelectorAll('[data-firewall-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          const tab = button.dataset.firewallTab || 'overview';
          state.firewallTab = tab;
          localStorage.setItem('megavpn.firewallTab', tab);
          render();
        });
      });
    }

    function bindActions() {
      document.getElementById('addFirewallPolicyBtnTop')?.addEventListener('click', () => openPolicyModal(''));
      document.getElementById('addFirewallListBtnTop')?.addEventListener('click', openAddressListModal);
      document.getElementById('addFirewallEntryBtnTop')?.addEventListener('click', () => openAddressEntryModal(''));
      document.getElementById('addFirewallRuleBtnTop')?.addEventListener('click', () => openRuleModal(''));
      document.querySelectorAll('.firewall-add-rule-btn').forEach((button) => {
        button.addEventListener('click', () => openRuleModal(button.dataset.policyId || ''));
      });
      document.querySelectorAll('.firewall-edit-policy-btn').forEach((button) => {
        button.addEventListener('click', () => openPolicyModal(button.dataset.policyId || ''));
      });
      document.querySelectorAll('.firewall-delete-policy-btn').forEach((button) => {
        button.addEventListener('click', () => openDeletePolicyModal(button.dataset.policyId || '', button.dataset.policyName || 'policy'));
      });
      document.querySelectorAll('.firewall-add-entry-btn').forEach((button) => {
        button.addEventListener('click', () => openAddressEntryModal(button.dataset.listId || ''));
      });
      document.querySelectorAll('.firewall-edit-list-btn').forEach((button) => {
        button.addEventListener('click', () => openAddressListModal(button.dataset.listId || ''));
      });
      document.querySelectorAll('.firewall-delete-list-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteAddressListModal(button.dataset.listId || '', button.dataset.listName || 'address group'));
      });
      document.querySelectorAll('.firewall-edit-entry-btn').forEach((button) => {
        button.addEventListener('click', () => openAddressEntryModal(button.dataset.listId || '', button.dataset.entryId || ''));
      });
      document.querySelectorAll('.firewall-delete-entry-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteAddressEntryModal(button.dataset.listId || '', button.dataset.entryId || '', button.dataset.entryValue || 'entry'));
      });
      document.querySelectorAll('.firewall-edit-rule-btn').forEach((button) => {
        button.addEventListener('click', () => openRuleModal(button.dataset.policyId || '', button.dataset.ruleId || ''));
      });
      document.querySelectorAll('.firewall-delete-rule-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteRuleModal(button.dataset.policyId || '', button.dataset.ruleId || ''));
      });
      document.querySelectorAll('.firewall-apply-btn').forEach((button) => {
        button.addEventListener('click', () => openApplyModal(button.dataset.policyId || '', ''));
      });
      document.querySelectorAll('.firewall-preview-btn').forEach((button) => {
        button.addEventListener('click', () => openPreviewModal(button.dataset.policyId || '', ''));
      });
      document.querySelectorAll('.firewall-node-apply-btn').forEach((button) => {
        button.addEventListener('click', () => openApplyModal('', button.dataset.nodeId || ''));
      });
      document.querySelectorAll('.firewall-node-disable-btn').forEach((button) => {
        button.addEventListener('click', () => openDisableFirewallModal(button.dataset.nodeId || '', button.dataset.nodeName || 'node'));
      });
      document.querySelectorAll('.firewall-node-preview-btn').forEach((button) => {
        button.addEventListener('click', () => openPreviewModal('', button.dataset.nodeId || ''));
      });
      document.querySelectorAll('.firewall-apply-quick-btn').forEach((button) => {
        button.addEventListener('click', () => openApplyModal(button.dataset.policyId || '', button.dataset.nodeId || ''));
      });
      document.querySelectorAll('.firewall-preview-quick-btn').forEach((button) => {
        button.addEventListener('click', () => openPreviewModal(button.dataset.policyId || '', button.dataset.nodeId || ''));
      });
    }

    function bindRuleFilters() {
      const filters = ensureFirewallFilters();
      const search = document.getElementById('firewallRuleSearchInput');
      const policy = document.getElementById('firewallRulePolicyFilter');
      const chain = document.getElementById('firewallRuleChainFilter');
      const action = document.getElementById('firewallRuleActionFilter');
      const rerender = () => {
        filters.ruleSearch = String(search?.value || '');
        filters.rulePolicy = String(policy?.value || 'all');
        filters.ruleChain = String(chain?.value || 'all');
        filters.ruleAction = String(action?.value || 'all');
        render();
      };
      search?.addEventListener('input', () => {
        filters.ruleSearch = String(search.value || '');
      });
      search?.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') rerender();
      });
      policy?.addEventListener('change', rerender);
      chain?.addEventListener('change', rerender);
      action?.addEventListener('change', rerender);
      document.getElementById('applyFirewallRuleFiltersBtn')?.addEventListener('click', rerender);
      document.getElementById('resetFirewallRuleFiltersBtn')?.addEventListener('click', () => {
        filters.ruleSearch = '';
        filters.rulePolicy = 'all';
        filters.ruleChain = 'all';
        filters.ruleAction = 'all';
        render();
      });
    }

    function bindAddressFilters() {
      const filters = ensureFirewallFilters();
      const search = document.getElementById('firewallAddressSearchInput');
      search?.addEventListener('input', () => {
        filters.addressSearch = String(search.value || '');
      });
      search?.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') render();
      });
      document.getElementById('applyFirewallAddressFiltersBtn')?.addEventListener('click', () => render());
      document.getElementById('resetFirewallAddressFiltersBtn')?.addEventListener('click', () => {
        filters.addressSearch = '';
        render();
      });
    }

    function openPolicyModal(policyID = '') {
      const policy = policyByID(policyID) || {};
      const editing = Boolean(policy.id);
      const selectedNodeID = policy.node_id || '';
      openModal(editing ? `Edit policy: ${policy.label || policy.key}` : 'Create firewall policy', 'Firewall policy', `
        <form id="firewallPolicyForm" class="form-grid" data-policy-id="${escapeHTML(policy.id || '')}">
          <div class="field"><label>Name</label><input name="label" required placeholder="Production node baseline" value="${escapeHTML(policy.label || '')}" /></div>
          <div class="field"><label>Scope</label><select name="scope" id="firewallPolicyScope">
            ${selectOption('node', policy.scope || 'node', 'node')}
            ${selectOption('control_plane', policy.scope || 'node', 'control plane')}
            ${selectOption('template', policy.scope || 'node', 'template')}
          </select></div>
          <div class="field"><label>Target node</label><select name="node_id" id="firewallPolicyNodeID">
            <option value="">all nodes / generic policy</option>
            ${(state.nodes || []).map((node) => `<option value="${escapeHTML(node.id)}"${node.id === selectedNodeID ? ' selected' : ''}>${escapeHTML(node.name || node.id)} · ${escapeHTML(node.role || 'node')} · ${escapeHTML(node.address || 'n/a')}</option>`).join('')}
          </select><small>Only used for node-scoped policy.</small></div>
          <div class="field"><label>Default input</label><select name="default_input_policy">${defaultPolicyOptions(policy.default_input_policy || 'accept')}</select></div>
          <div class="field"><label>Default forward</label><select name="default_forward_policy">${defaultPolicyOptions(policy.default_forward_policy || 'accept')}</select></div>
          <div class="field"><label>Default output</label><select name="default_output_policy">${defaultPolicyOptions(policy.default_output_policy || 'accept')}</select></div>
          <div class="field"><label>Status</label><select name="status">${selectOption('active', policy.status || 'active')}${selectOption('disabled', policy.status || 'active')}</select></div>
          <div class="field full"><label>Description</label><textarea name="description" rows="3" placeholder="What this policy protects and where it should be applied">${escapeHTML(policy.description || '')}</textarea></div>
          <div class="notice field full">Default policies are enforced only when strict mode is selected during policy apply. Keep explicit management and protocol allow rules in place before enabling drop or reject defaults.</div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">${editing ? 'Save policy' : 'Create policy'}</button><button class="secondary-btn" type="button" id="cancelFirewallPolicyBtn">Cancel</button></div>
        </form>
        <div id="firewallPolicyResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallPolicyBtn')?.addEventListener('click', closeModal);
      document.getElementById('firewallPolicyForm')?.addEventListener('submit', submitPolicy);
      const syncNodeSelect = () => {
        const scope = String(document.getElementById('firewallPolicyScope')?.value || 'node');
        const nodeSelect = document.getElementById('firewallPolicyNodeID');
        if (nodeSelect) nodeSelect.disabled = scope !== 'node';
      };
      document.getElementById('firewallPolicyScope')?.addEventListener('change', syncNodeSelect);
      syncNodeSelect();
    }

    function openAddressListModal(listID = '') {
      const list = addressListByID(listID) || {};
      const editing = Boolean(list.id);
      openModal(editing ? `Edit address group: ${list.label || list.key}` : 'Add address group', 'Firewall', `
        <form id="firewallAddressListForm" class="form-grid" data-list-id="${escapeHTML(list.id || '')}">
          <div class="field"><label>Name</label><input name="label" required placeholder="Trusted operators" value="${escapeHTML(list.label || '')}" /></div>
          <div class="field"><label>Scope</label><select name="scope">
            ${selectOption('global', list.scope || 'global')}
            ${selectOption('control_plane', list.scope || 'global')}
            ${selectOption('node', list.scope || 'global')}
            ${selectOption('service', list.scope || 'global')}
            ${selectOption('client', list.scope || 'global')}
          </select></div>
          <div class="field"><label>Status</label><select name="status">${selectOption('active', list.status || 'active')}${selectOption('disabled', list.status || 'active')}</select></div>
          <div class="field full"><label>Description</label><textarea name="description" rows="3">${escapeHTML(list.description || '')}</textarea></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">${editing ? 'Save group' : 'Create group'}</button><button class="secondary-btn" type="button" id="cancelFirewallListBtn">Cancel</button></div>
        </form>
        <div id="firewallListResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallListBtn')?.addEventListener('click', closeModal);
      document.getElementById('firewallAddressListForm')?.addEventListener('submit', submitAddressList);
    }

    function openAddressEntryModal(listID, entryID = '') {
      const lists = inventory().lists;
      if (!lists.length) {
        openModal('Create address group first', 'Firewall address group', `
          <p class="notice">Address entries belong to a named address group. Create the group, then add CIDR, IP range, address or DNS entries to it.</p>
          <div class="inline-actions">
            <button class="primary-btn" id="createFirewallListFromEntryBtn" type="button">Create address group</button>
            <button class="secondary-btn" id="cancelFirewallEntryBtn" type="button">Cancel</button>
          </div>`);
        document.getElementById('createFirewallListFromEntryBtn')?.addEventListener('click', () => openAddressListModal(''));
        document.getElementById('cancelFirewallEntryBtn')?.addEventListener('click', closeModal);
        return;
      }
      const entry = entryByID(entryID) || {};
      const editing = Boolean(entry.id);
      const selected = addressListByID(listID) || lists[0] || {};
      openModal(editing ? `Edit address entry: ${entry.value || entry.id}` : 'Add address entry', 'Firewall address group', `
        <form id="firewallAddressEntryForm" class="form-grid" data-entry-id="${escapeHTML(entry.id || '')}">
          <div class="field"><label>Address group</label><select name="list_id"${editing ? ' disabled' : ''} required>${lists.map((list) => `<option value="${escapeHTML(list.id)}"${list.id === selected.id ? ' selected' : ''}>${escapeHTML(list.label || list.key)}</option>`).join('')}</select>${editing ? `<input type="hidden" name="list_id" value="${escapeHTML(selected.id || '')}" />` : ''}</div>
          <div class="field"><label>Type</label><select name="value_type">${selectOption('', entry.value_type || '', 'auto detect')}${selectOption('cidr', entry.value_type || '')}${selectOption('address', entry.value_type || '')}${selectOption('range', entry.value_type || '')}${selectOption('dns', entry.value_type || '', 'dns (stored only)')}</select><small>Use CIDR, address or range for node nftables apply. DNS values are stored for catalog context and are not rendered into rules.</small></div>
          <div class="field"><label>Value</label><input name="value" required placeholder="203.0.113.0/24" value="${escapeHTML(entry.value || '')}" /></div>
          <div class="field"><label>Label</label><input name="label" placeholder="office range" value="${escapeHTML(entry.label || '')}" /></div>
          <div class="field"><label>Status</label><select name="status">${selectOption('active', entry.status || 'active')}${selectOption('disabled', entry.status || 'active')}</select></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">${editing ? 'Save entry' : 'Add entry'}</button><button class="secondary-btn" type="button" id="cancelFirewallEntryBtn">Cancel</button></div>
        </form>
        <div id="firewallEntryResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallEntryBtn')?.addEventListener('click', closeModal);
      document.getElementById('firewallAddressEntryForm')?.addEventListener('submit', submitAddressEntry);
    }

    function renderRulePresetGroups() {
      const groups = [
        { label: 'Management', items: [['ssh', 'SSH admin'], ['https', 'HTTPS control'], ['cp_egress', 'Control-plane egress']] },
        { label: 'VPN', items: [['wireguard', 'WireGuard UDP'], ['openvpn_udp', 'OpenVPN UDP'], ['openvpn_tcp', 'OpenVPN TCP'], ['ipsec_ike', 'IPsec IKE'], ['l2tp', 'L2TP']] },
        { label: 'Proxy / edge', items: [['shadowsocks_tcp', 'Shadowsocks TCP'], ['shadowsocks_udp', 'Shadowsocks UDP'], ['http_proxy', 'HTTP proxy'], ['mtproto', 'MTProto'], ['nginx_edge', 'Nginx edge']] },
        { label: 'Hygiene', items: [['drop_invalid', 'Drop invalid']] },
      ];
      return `
        <div class="firewall-rule-presets">
          ${groups.map((group) => `
            <div class="firewall-preset-group">
              <strong>${escapeHTML(group.label)}</strong>
              <div>
                ${group.items.map(([key, label]) => `<button type="button" data-firewall-rule-preset="${escapeHTML(key)}">${escapeHTML(label)}</button>`).join('')}
              </div>
            </div>`).join('')}
        </div>`;
    }

    function openRuleModal(policyID, ruleID = '') {
      const inv = inventory();
      if (!inv.policies.length) {
        openModal('Create policy first', 'Firewall rule', `
          <p class="notice">Firewall rules must belong to a policy. Create a policy first, then add ordered rules to it.</p>
          <div class="inline-actions">
            <button class="primary-btn" id="createFirewallPolicyFromRuleBtn" type="button">Create policy</button>
            <button class="secondary-btn" id="cancelFirewallRuleBtn" type="button">Cancel</button>
          </div>`);
        document.getElementById('createFirewallPolicyFromRuleBtn')?.addEventListener('click', () => openPolicyModal(''));
        document.getElementById('cancelFirewallRuleBtn')?.addEventListener('click', closeModal);
        return;
      }
      const rule = ruleByID(ruleID) || {};
      const editing = Boolean(rule.id);
      const selected = policyByID(policyID || rule.policy_id) || inv.policies[0] || {};
      openModal(editing ? `Edit firewall rule: ${rule.comment || rule.id}` : 'Add firewall rule', 'Firewall policy', `
        ${renderRulePresetGroups()}
        <form id="firewallRuleForm" class="form-grid" data-rule-id="${escapeHTML(rule.id || '')}" data-policy-id="${escapeHTML(selected.id || '')}">
          <div class="field"><label>Policy</label><select name="policy_id"${editing ? ' disabled' : ''} required>${inv.policies.map((policy) => `<option value="${escapeHTML(policy.id)}"${policy.id === selected.id ? ' selected' : ''}>${escapeHTML(policy.label || policy.key)}</option>`).join('')}</select>${editing ? `<input type="hidden" name="policy_id" value="${escapeHTML(selected.id || '')}" />` : ''}</div>
          <div class="field"><label>Priority</label><input name="priority" type="number" min="1" max="65000" value="${escapeHTML(String(rule.priority || 1000))}" required /></div>
          <div class="field"><label>Chain</label><select name="chain">${selectOption('input', rule.chain || 'input')}${selectOption('forward', rule.chain || 'input')}${selectOption('output', rule.chain || 'input')}</select></div>
          <div class="field"><label>Action</label><select name="action">${selectOption('accept', rule.action || 'accept')}${selectOption('drop', rule.action || 'accept')}${selectOption('reject', rule.action || 'accept')}</select></div>
          <div class="field"><label>Protocol</label><select name="protocol">${selectOption('any', rule.protocol || 'any')}${selectOption('tcp', rule.protocol || 'any')}${selectOption('udp', rule.protocol || 'any')}${selectOption('icmp', rule.protocol || 'any')}${selectOption('icmpv6', rule.protocol || 'any')}</select></div>
          <div class="field"><label>Source list</label><select name="src_list_id">${addressListOptions(rule.src_list_id || '')}</select></div>
          <div class="field"><label>Destination list</label><select name="dst_list_id">${addressListOptions(rule.dst_list_id || '')}</select></div>
          <div class="field"><label>Source CIDR</label><input name="src_cidr" placeholder="any or 10.0.0.0/8" value="${escapeHTML(rule.src_cidr || '')}" /></div>
          <div class="field"><label>Destination CIDR</label><input name="dst_cidr" placeholder="any or 0.0.0.0/0" value="${escapeHTML(rule.dst_cidr || '')}" /></div>
          <div class="field"><label>Source ports</label><input name="src_ports" pattern="^$|^\\*$|^[0-9,-]+$" placeholder="any or 1024-65535" value="${escapeHTML(rule.src_ports || '')}" /></div>
          <div class="field"><label>Destination ports</label><input name="dst_ports" pattern="^$|^\\*$|^[0-9,-]+$" placeholder="443,8443" value="${escapeHTML(rule.dst_ports || '')}" /></div>
          <div class="field"><label>State</label><input name="state_match" placeholder="established,related,new" value="${escapeHTML(Array.isArray(rule.state_match) ? rule.state_match.join(',') : '')}" /></div>
          <div class="field"><label>Status</label><select name="status">${selectOption('active', rule.status || 'active')}${selectOption('disabled', rule.status || 'active')}</select></div>
          <div class="field full"><label>Comment</label><input name="comment" placeholder="allow agent control channel" value="${escapeHTML(rule.comment || '')}" /></div>
          <label class="field checkbox-line"><input name="enabled" type="checkbox"${rule.id ? (rule.enabled ? ' checked' : '') : ' checked'} /> Enabled</label>
          <label class="field checkbox-line"><input name="log" type="checkbox"${rule.log ? ' checked' : ''} /> Log</label>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">${editing ? 'Save rule' : 'Create rule'}</button><button class="secondary-btn" type="button" id="cancelFirewallRuleBtn">Cancel</button></div>
        </form>
        <div id="firewallRuleResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallRuleBtn')?.addEventListener('click', closeModal);
      const form = document.getElementById('firewallRuleForm');
      form?.addEventListener('submit', submitRule);
      bindRulePresets(form);
    }

    function renderApplyPolicyDetails(policy = {}) {
      const defaults = policyDefaults(policy);
      const rules = policy.id ? rulesForPolicy(policy.id) : [];
      const posture = policyPosture(policy, rules);
      const outputGuard = defaults.output === 'accept'
        ? 'Output default accept'
        : 'Strict output requires pinned control-plane egress or an explicit output allow rule';
      return `
        <div class="firewall-apply-summary">
          <div class="firewall-apply-summary-head">
            <div>
              <strong>${escapeHTML(policy.label || policy.key || 'Policy')}</strong>
              <span>${escapeHTML(policy.key || policy.id || 'not selected')} · ${escapeHTML(policy.scope || 'node')}</span>
            </div>
            <span class="firewall-policy-posture ${escapeHTML(posture.level)} compact">${escapeHTML(posture.label)}</span>
          </div>
          <div class="firewall-policy-facts">
            ${defaultPolicyPill('Input', defaults.input)}
            ${defaultPolicyPill('Forward', defaults.forward)}
            ${defaultPolicyPill('Output', defaults.output)}
            <div><span>Rules</span><strong>${escapeHTML(String(rules.length))}</strong></div>
          </div>
          <div class="notice ${defaults.output === 'accept' ? '' : 'warn'}">${escapeHTML(outputGuard)}</div>
        </div>`;
    }

    function openApplyModal(policyID, nodeID, applyMode = 'observe') {
      const inv = inventory();
      const selectedPolicy = policyByID(policyID) || inv.policies.find((policy) => policy.key === 'node_base') || inv.policies[0] || {};
      const selectedNode = (state.nodes || []).find((node) => node.id === nodeID) || (state.nodes || [])[0] || {};
      const strictMode = String(applyMode || 'observe') === 'strict';
      openModal('Apply firewall policy', 'Node firewall', `
        <form id="firewallApplyForm" class="form-grid">
          <div class="field"><label>Node</label><select name="node_id" required>${(state.nodes || []).map((node) => `<option value="${escapeHTML(node.id)}"${node.id === selectedNode.id ? ' selected' : ''}>${escapeHTML(node.name || node.id)} · ${escapeHTML(node.role || 'node')} · ${escapeHTML(node.address || 'n/a')}</option>`).join('')}</select></div>
          <div class="field"><label>Policy</label><select name="policy_id" id="firewallApplyPolicySelect" required>${inv.policies.map((policy) => `<option value="${escapeHTML(policy.id)}"${policy.id === selectedPolicy.id ? ' selected' : ''}>${escapeHTML(policy.label || policy.key)}</option>`).join('')}</select></div>
          <div id="firewallApplyPolicyDetails" class="field full">${renderApplyPolicyDetails(selectedPolicy)}</div>
          <div class="firewall-apply-mode-grid field full">
            <label class="firewall-apply-mode-card">
              <input name="apply_mode" type="radio" value="observe"${strictMode ? '' : ' checked'} />
              <span>Rules only</span>
              <small>Base chains stay accept; explicit catalog rules are installed.</small>
            </label>
            <label class="firewall-apply-mode-card">
              <input name="apply_mode" type="radio" value="strict"${strictMode ? ' checked' : ''} />
              <span>Strict defaults</span>
              <small>Default input, forward and output policies are enforced.</small>
            </label>
          </div>
          <div class="field full inline-actions"><button class="primary-btn apply-btn" type="submit">Queue apply</button><button class="secondary-btn" type="button" id="cancelFirewallApplyBtn">Cancel</button></div>
        </form>
        <div id="firewallApplyResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallApplyBtn')?.addEventListener('click', closeModal);
      document.getElementById('firewallApplyForm')?.addEventListener('submit', submitApply);
      document.getElementById('firewallApplyPolicySelect')?.addEventListener('change', (event) => {
        const details = document.getElementById('firewallApplyPolicyDetails');
        const policy = policyByID(String(event.currentTarget?.value || '')) || {};
        if (details) details.innerHTML = renderApplyPolicyDetails(policy);
      });
    }

    function openPreviewModal(policyID, nodeID) {
      const inv = inventory();
      const selectedPolicy = policyByID(policyID) || inv.policies.find((policy) => policy.key === 'node_base') || inv.policies[0] || {};
      const selectedNode = (state.nodes || []).find((node) => node.id === nodeID) || (state.nodes || [])[0] || {};
      openModal('Preview firewall policy', 'Node firewall', `
        <form id="firewallPreviewForm" class="form-grid">
          <div class="field"><label>Node</label><select name="node_id" required>${(state.nodes || []).map((node) => `<option value="${escapeHTML(node.id)}"${node.id === selectedNode.id ? ' selected' : ''}>${escapeHTML(node.name || node.id)} · ${escapeHTML(node.role || 'node')} · ${escapeHTML(node.address || 'n/a')}</option>`).join('')}</select></div>
          <div class="field"><label>Policy</label><select name="policy_id" id="firewallPreviewPolicySelect" required>${inv.policies.map((policy) => `<option value="${escapeHTML(policy.id)}"${policy.id === selectedPolicy.id ? ' selected' : ''}>${escapeHTML(policy.label || policy.key)}</option>`).join('')}</select></div>
          <div id="firewallPreviewPolicyDetails" class="field full">${renderApplyPolicyDetails(selectedPolicy)}</div>
          <div class="firewall-apply-mode-grid field full">
            <label class="firewall-apply-mode-card">
              <input name="apply_mode" type="radio" value="observe" checked />
              <span>Rules only</span>
              <small>Preview explicit catalog rules with accept base chain policies.</small>
            </label>
            <label class="firewall-apply-mode-card">
              <input name="apply_mode" type="radio" value="strict" />
              <span>Strict defaults</span>
              <small>Preview enforced input, forward and output default policies.</small>
            </label>
          </div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Run preview</button><button class="secondary-btn" type="button" id="cancelFirewallPreviewBtn">Cancel</button></div>
        </form>
        <div id="firewallPreviewResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallPreviewBtn')?.addEventListener('click', closeModal);
      document.getElementById('firewallPreviewForm')?.addEventListener('submit', submitPreview);
      document.getElementById('firewallPreviewPolicySelect')?.addEventListener('change', (event) => {
        const details = document.getElementById('firewallPreviewPolicyDetails');
        const policy = policyByID(String(event.currentTarget?.value || '')) || {};
        if (details) details.innerHTML = renderApplyPolicyDetails(policy);
      });
    }

    function openDeleteAddressListModal(listID, label) {
      openModal(`Delete address group: ${label || listID}`, 'Firewall', `
        <p class="danger-text">Address group deletion is blocked while active rules reference it.</p>
        <div class="inline-actions">
          <button class="danger-btn" id="confirmFirewallListDeleteBtn" type="button">Delete group</button>
          <button class="secondary-btn" id="cancelFirewallListDeleteBtn" type="button">Cancel</button>
        </div>
        <div id="firewallDeleteResult" class="form-result"></div>`);
      document.getElementById('cancelFirewallListDeleteBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmFirewallListDeleteBtn')?.addEventListener('click', () => deleteAddressList(listID));
    }

    function openDisableFirewallModal(nodeID, nodeName) {
      openModal(`Disable firewall: ${nodeName || nodeID}`, 'Node firewall', `
        <div class="client-danger-summary">
          <div>
            <span>Scope</span>
            <strong>Managed firewall table only</strong>
            <small>Deletes <code>inet megavpn_firewall</code> on this node. Instances, route policy and service runtimes are not stopped.</small>
          </div>
          <div>
            <span>Rollback</span>
            <strong>Apply policy again</strong>
            <small>Use Preview, then Apply, to recreate the managed firewall from the catalog.</small>
          </div>
        </div>
        <p class="danger-text">Disable is intended for emergency rollback or staged testing. It removes the active managed firewall from the selected node.</p>
        <div class="inline-actions">
          <button class="danger-btn" id="confirmFirewallDisableBtn" type="button">Disable firewall</button>
          <button class="secondary-btn" id="cancelFirewallDisableBtn" type="button">Cancel</button>
        </div>
        <div id="firewallDisableResult" class="form-result"></div>`, { wide: true });
      document.getElementById('cancelFirewallDisableBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmFirewallDisableBtn')?.addEventListener('click', () => disableFirewall(nodeID));
    }

    function openDeletePolicyModal(policyID, label) {
      openModal(`Delete policy: ${label || policyID}`, 'Firewall', `
        <p class="danger-text">Policy deletion also removes its catalog rules. It is blocked while a node still references this policy state.</p>
        <div class="inline-actions">
          <button class="danger-btn" id="confirmFirewallPolicyDeleteBtn" type="button">Delete policy</button>
          <button class="secondary-btn" id="cancelFirewallPolicyDeleteBtn" type="button">Cancel</button>
        </div>
        <div id="firewallDeleteResult" class="form-result"></div>`);
      document.getElementById('cancelFirewallPolicyDeleteBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmFirewallPolicyDeleteBtn')?.addEventListener('click', () => deletePolicy(policyID));
    }

    function openDeleteAddressEntryModal(listID, entryID, value) {
      openModal(`Delete address entry: ${value || entryID}`, 'Firewall', `
        <p class="danger-text">Rules that use this address group will no longer match this value after the next apply.</p>
        <div class="inline-actions">
          <button class="danger-btn" id="confirmFirewallEntryDeleteBtn" type="button">Delete entry</button>
          <button class="secondary-btn" id="cancelFirewallEntryDeleteBtn" type="button">Cancel</button>
        </div>
        <div id="firewallDeleteResult" class="form-result"></div>`);
      document.getElementById('cancelFirewallEntryDeleteBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmFirewallEntryDeleteBtn')?.addEventListener('click', () => deleteAddressEntry(listID, entryID));
    }

    function openDeleteRuleModal(policyID, ruleID) {
      const rule = ruleByID(ruleID) || {};
      openModal(`Delete firewall rule: ${rule.comment || ruleID}`, 'Firewall', `
        <p class="danger-text">The rule is removed from the catalog. Existing node nftables rules change only after applying the policy to the node.</p>
        <div class="inline-actions">
          <button class="danger-btn" id="confirmFirewallRuleDeleteBtn" type="button">Delete rule</button>
          <button class="secondary-btn" id="cancelFirewallRuleDeleteBtn" type="button">Cancel</button>
        </div>
        <div id="firewallDeleteResult" class="form-result"></div>`);
      document.getElementById('cancelFirewallRuleDeleteBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmFirewallRuleDeleteBtn')?.addEventListener('click', () => deleteRule(policyID, ruleID));
    }

    async function disableFirewall(nodeID) {
      const result = document.getElementById('firewallDisableResult');
      const button = document.getElementById('confirmFirewallDisableBtn');
      if (button) button.disabled = true;
      if (result) result.innerHTML = '<span class="tag warn">queueing disable</span>';
      try {
        const job = await sendJSON(`/api/v1/nodes/${encodeURIComponent(nodeID)}/firewall/disable`, 'POST', {});
        if (result) result.innerHTML = `<span class="tag ok">disable queued</span> <code>${escapeHTML(job.id || '')}</code>`;
        if (typeof watchJob === 'function' && job?.id) {
          await watchJob(job.id, result, 'Firewall disable');
        }
        selectFirewallTab('nodeState');
        closeModal();
        await refresh();
      } catch (err) {
        if (button) button.disabled = false;
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        openActionOutcomeModal('Firewall disable', 'Disable queue failed', 'failed', err.message || 'Firewall disable failed.', [
          { label: 'Node', value: nodeID || 'n/a' },
        ]);
      }
    }

    async function deletePolicy(policyID) {
      const result = document.getElementById('firewallDeleteResult');
      if (result) result.innerHTML = '<span class="tag warn">deleting</span>';
      try {
        await sendJSON(`/api/v1/firewall/policies/${encodeURIComponent(policyID)}`, 'DELETE', null);
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function deleteAddressList(listID) {
      const result = document.getElementById('firewallDeleteResult');
      if (result) result.innerHTML = '<span class="tag warn">deleting</span>';
      try {
        await sendJSON(`/api/v1/firewall/address-lists/${encodeURIComponent(listID)}`, 'DELETE', null);
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function deleteAddressEntry(listID, entryID) {
      const result = document.getElementById('firewallDeleteResult');
      if (result) result.innerHTML = '<span class="tag warn">deleting</span>';
      try {
        await sendJSON(`/api/v1/firewall/address-lists/${encodeURIComponent(listID)}/entries/${encodeURIComponent(entryID)}`, 'DELETE', null);
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function deleteRule(policyID, ruleID) {
      const result = document.getElementById('firewallDeleteResult');
      if (result) result.innerHTML = '<span class="tag warn">deleting</span>';
      try {
        await sendJSON(`/api/v1/firewall/policies/${encodeURIComponent(policyID)}/rules/${encodeURIComponent(ruleID)}`, 'DELETE', null);
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function submitPolicy(event) {
      event.preventDefault();
      const policyID = String(event.currentTarget?.dataset?.policyId || '').trim();
      const form = new FormData(event.currentTarget);
      const result = document.getElementById('firewallPolicyResult');
      if (result) result.innerHTML = `<span class="tag warn">${policyID ? 'saving' : 'creating'}</span>`;
      try {
        const scope = String(form.get('scope') || 'node').trim();
        const nodeID = scope === 'node' ? String(form.get('node_id') || '').trim() : '';
        const payload = {
          label: String(form.get('label') || '').trim(),
          description: String(form.get('description') || '').trim(),
          scope,
          node_id: nodeID || null,
          default_input_policy: String(form.get('default_input_policy') || 'accept').trim(),
          default_forward_policy: String(form.get('default_forward_policy') || 'accept').trim(),
          default_output_policy: String(form.get('default_output_policy') || 'accept').trim(),
          status: String(form.get('status') || 'active').trim(),
        };
        if (policyID) {
          await sendJSON(`/api/v1/firewall/policies/${encodeURIComponent(policyID)}`, 'PUT', payload);
        } else {
          await sendJSON('/api/v1/firewall/policies', 'POST', payload);
        }
        selectFirewallTab('policies');
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function submitAddressList(event) {
      event.preventDefault();
      const listID = String(event.currentTarget?.dataset?.listId || '').trim();
      const form = new FormData(event.currentTarget);
      const result = document.getElementById('firewallListResult');
      if (result) result.innerHTML = `<span class="tag warn">${listID ? 'saving' : 'creating'}</span>`;
      try {
        const payload = {
          label: String(form.get('label') || '').trim(),
          description: String(form.get('description') || '').trim(),
          scope: String(form.get('scope') || 'global').trim(),
          status: String(form.get('status') || 'active').trim(),
        };
        if (listID) {
          await sendJSON(`/api/v1/firewall/address-lists/${encodeURIComponent(listID)}`, 'PUT', payload);
        } else {
          await sendJSON('/api/v1/firewall/address-lists', 'POST', payload);
        }
        selectFirewallTab('addressLists');
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function submitAddressEntry(event) {
      event.preventDefault();
      const entryID = String(event.currentTarget?.dataset?.entryId || '').trim();
      const form = new FormData(event.currentTarget);
      const listID = String(form.get('list_id') || '').trim();
      const result = document.getElementById('firewallEntryResult');
      if (result) result.innerHTML = `<span class="tag warn">${entryID ? 'saving' : 'adding'}</span>`;
      try {
        const payload = {
          value: String(form.get('value') || '').trim(),
          value_type: String(form.get('value_type') || '').trim(),
          label: String(form.get('label') || '').trim(),
          status: String(form.get('status') || 'active').trim(),
        };
        if (entryID) {
          await sendJSON(`/api/v1/firewall/address-lists/${encodeURIComponent(listID)}/entries/${encodeURIComponent(entryID)}`, 'PUT', payload);
        } else {
          await sendJSON(`/api/v1/firewall/address-lists/${encodeURIComponent(listID)}/entries`, 'POST', payload);
        }
        selectFirewallTab('addressLists');
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function submitRule(event) {
      event.preventDefault();
      const ruleID = String(event.currentTarget?.dataset?.ruleId || '').trim();
      const form = new FormData(event.currentTarget);
      const policyID = String(form.get('policy_id') || '').trim();
      const result = document.getElementById('firewallRuleResult');
      if (result) result.innerHTML = `<span class="tag warn">${ruleID ? 'saving' : 'creating'}</span>`;
      try {
        const status = String(form.get('status') || 'active').trim();
        const currentRule = ruleID ? (ruleByID(ruleID) || {}) : {};
        const metadata = currentRule.metadata && typeof currentRule.metadata === 'object' && !Array.isArray(currentRule.metadata)
          ? currentRule.metadata
          : {};
        const payload = {
          priority: Number(form.get('priority') || 1000),
          chain: String(form.get('chain') || 'input').trim(),
          action: String(form.get('action') || 'accept').trim(),
          protocol: String(form.get('protocol') || 'any').trim(),
          src_list_id: String(form.get('src_list_id') || '').trim() || null,
          dst_list_id: String(form.get('dst_list_id') || '').trim() || null,
          src_cidr: String(form.get('src_cidr') || '').trim(),
          dst_cidr: String(form.get('dst_cidr') || '').trim(),
          src_ports: String(form.get('src_ports') || '').trim(),
          dst_ports: String(form.get('dst_ports') || '').trim(),
          state_match: String(form.get('state_match') || '').split(',').map((item) => item.trim()).filter(Boolean),
          comment: String(form.get('comment') || '').trim(),
          enabled: Boolean(form.get('enabled')),
          log: Boolean(form.get('log')),
          status: Boolean(form.get('enabled')) ? status : 'disabled',
          metadata,
        };
        if (ruleID) {
          await sendJSON(`/api/v1/firewall/policies/${encodeURIComponent(policyID)}/rules/${encodeURIComponent(ruleID)}`, 'PUT', payload);
        } else {
          await sendJSON(`/api/v1/firewall/policies/${encodeURIComponent(policyID)}/rules`, 'POST', payload);
        }
        selectFirewallTab('rules');
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function submitApply(event) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const nodeID = String(form.get('node_id') || '').trim();
      const policyID = String(form.get('policy_id') || '').trim();
      const result = document.getElementById('firewallApplyResult');
      if (result) result.innerHTML = '<span class="tag warn">queueing</span>';
      try {
        const job = await sendJSON(`/api/v1/nodes/${encodeURIComponent(nodeID)}/firewall/apply`, 'POST', {
          policy_id: policyID,
          enforce_default_policy: String(form.get('apply_mode') || 'observe') === 'strict',
        });
        if (result) result.innerHTML = `<span class="tag ok">queued</span> <code>${escapeHTML(job.id || '')}</code>`;
        if (typeof watchJob === 'function' && job?.id) {
          await watchJob(job.id, result);
        }
        selectFirewallTab('nodeState');
        closeModal();
        await refresh();
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        openActionOutcomeModal('Firewall apply', 'Apply queue failed', 'failed', err.message || 'Firewall apply failed.', [
          { label: 'Node', value: nodeID || 'n/a' },
          { label: 'Policy', value: policyID || 'n/a' },
        ]);
      }
    }

    function firewallPreviewDiff(job, nodeID) {
      const result = job?.result || {};
      const current = nodeStateByID(nodeID)?.observed || {};
      const previewHash = String(result.rendered_hash || '').trim();
      const currentHash = String(current.rendered_hash || '').trim();
      if (!previewHash) {
        return { status: 'unknown', label: 'No preview hash', detail: 'Agent did not return a rendered policy hash.', className: 'stub', currentHash, previewHash };
      }
      if (currentHash && previewHash === currentHash) {
        return { status: 'same', label: 'No changes', detail: 'Rendered policy matches the last observed node firewall hash.', className: 'ok', currentHash, previewHash };
      }
      if (currentHash) {
        return { status: 'changed', label: 'Changes pending', detail: 'Rendered policy differs from the last observed node firewall hash.', className: 'warn', currentHash, previewHash };
      }
      return { status: 'new', label: 'Not applied yet', detail: 'Node does not have a comparable observed firewall hash.', className: 'warn', currentHash, previewHash };
    }

    function renderFirewallPreviewResult(job, nodeID, policyID, applyMode) {
      const result = job?.result || {};
      const diff = firewallPreviewDiff(job, nodeID);
      const warnings = Array.isArray(result.warnings) ? result.warnings.filter(Boolean) : [];
      const script = String(result.script || '').trim();
      const canApplyPreview = String(job?.status || '').toLowerCase() === 'succeeded' && Boolean(diff.previewHash);
      return `
        <section class="firewall-preview-result">
          <div class="firewall-preview-head">
            <div>
              <h3>Preview result</h3>
              <p>${escapeHTML(diff.detail)}</p>
            </div>
            ${statusTag(diff.className === 'ok' ? 'ok' : diff.className === 'warn' ? 'pending' : 'unknown')}
          </div>
          <div class="firewall-diff-grid">
            <div><span>Diff</span><strong>${escapeHTML(diff.label)}</strong><small>${escapeHTML(diff.status)}</small></div>
            <div><span>Preview hash</span><code>${escapeHTML(diff.previewHash || 'n/a')}</code></div>
            <div><span>Current hash</span><code>${escapeHTML(diff.currentHash || 'n/a')}</code></div>
            <div><span>Rules</span><strong>${escapeHTML(String(result.rule_count ?? 'n/a'))}</strong><small>system ${escapeHTML(String(result.system_rule_count ?? 'n/a'))}</small></div>
            <div><span>Defaults</span><strong>${escapeHTML(String(result.default_policy_enforcement || 'n/a'))}</strong><small>${String(result.applied) === 'true' ? 'applied' : 'preview only'}</small></div>
          </div>
          ${warnings.length ? `<div class="notice warn">${warnings.map((item) => escapeHTML(String(item))).join('<br>')}</div>` : ''}
          ${script ? `<details class="firewall-script-details"><summary>Rendered nftables script</summary><pre class="firewall-script-preview"><code>${escapeHTML(script)}</code></pre></details>` : ''}
          <div class="inline-actions">
            ${canApplyPreview ? `<button class="primary-btn apply-btn" id="applyPreviewedFirewallBtn" type="button" data-apply-mode="${escapeHTML(String(applyMode || 'observe'))}">Apply this policy</button>` : ''}
            <button class="secondary-btn" type="button" id="closeFirewallPreviewBtn">Close</button>
          </div>
        </section>`;
    }

    function bindPreviewResultActions(nodeID, policyID, applyMode) {
      document.getElementById('applyPreviewedFirewallBtn')?.addEventListener('click', () => openApplyModal(policyID, nodeID, applyMode));
      document.getElementById('closeFirewallPreviewBtn')?.addEventListener('click', closeModal);
    }

    async function submitPreview(event) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const nodeID = String(form.get('node_id') || '').trim();
      const policyID = String(form.get('policy_id') || '').trim();
      const applyMode = String(form.get('apply_mode') || 'observe') === 'strict' ? 'strict' : 'observe';
      const result = document.getElementById('firewallPreviewResult');
      if (result) result.innerHTML = '<span class="tag warn">queueing preview</span>';
      try {
        const job = await sendJSON(`/api/v1/nodes/${encodeURIComponent(nodeID)}/firewall/preview`, 'POST', {
          policy_id: policyID,
          enforce_default_policy: applyMode === 'strict',
        });
        if (result) result.innerHTML = `<span class="tag ok">preview queued</span> <code>${escapeHTML(job.id || '')}</code>`;
        let finalJob = job;
        if (typeof watchJob === 'function' && job?.id) {
          finalJob = await watchJob(job.id, result, 'Firewall preview') || job;
        }
        if (result && finalJob?.result) {
          result.innerHTML = renderFirewallPreviewResult(finalJob, nodeID, policyID, applyMode);
          bindPreviewResultActions(nodeID, policyID, applyMode);
        }
      } catch (err) {
        if (result) result.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        openActionOutcomeModal('Firewall preview', 'Preview failed', 'failed', err.message || 'Firewall preview failed.', [
          { label: 'Node', value: nodeID || 'n/a' },
          { label: 'Policy', value: policyID || 'n/a' },
        ]);
      }
    }

    function firstText(...values) {
      for (const value of values) {
        const text = String(value || '').trim();
        if (text) return text;
      }
      return '';
    }

    function selectFirewallTab(tab) {
      state.firewallTab = tab;
      localStorage.setItem('megavpn.firewallTab', tab);
    }

    function bindRulePresets(form) {
      if (!form) return;
      document.querySelectorAll('[data-firewall-rule-preset]').forEach((button) => {
        button.addEventListener('click', () => applyRulePreset(form, button.dataset.firewallRulePreset || ''));
      });
    }

    function setFormValue(form, name, value) {
      const field = form.querySelector(`[name="${name}"]`);
      if (field) field.value = value;
    }

    function setFormChecked(form, name, value) {
      const field = form.querySelector(`[name="${name}"]`);
      if (field) field.checked = Boolean(value);
    }

    function applyRulePreset(form, preset) {
      const presets = {
        ssh: {
          priority: '100',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '22',
          state_match: 'new,established',
          comment: 'allow SSH management from approved sources',
        },
        https: {
          priority: '110',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '443',
          state_match: 'new,established',
          comment: 'allow HTTPS control channel',
        },
        cp_egress: {
          priority: '90',
          chain: 'output',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '443',
          state_match: 'new,established',
          comment: 'allow control-plane API egress',
        },
        wireguard: {
          priority: '300',
          chain: 'input',
          action: 'accept',
          protocol: 'udp',
          dst_ports: '51820',
          state_match: 'new,established',
          comment: 'allow WireGuard listener',
        },
        openvpn_udp: {
          priority: '310',
          chain: 'input',
          action: 'accept',
          protocol: 'udp',
          dst_ports: '1194',
          state_match: 'new,established',
          comment: 'allow OpenVPN UDP listener',
        },
        openvpn_tcp: {
          priority: '311',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '11994',
          state_match: 'new,established',
          comment: 'allow OpenVPN TCP listener',
        },
        ipsec_ike: {
          priority: '320',
          chain: 'input',
          action: 'accept',
          protocol: 'udp',
          dst_ports: '500,4500',
          state_match: 'new,established',
          comment: 'allow IPsec IKE and NAT-T',
        },
        l2tp: {
          priority: '321',
          chain: 'input',
          action: 'accept',
          protocol: 'udp',
          dst_ports: '1701',
          state_match: 'new,established',
          comment: 'allow L2TP control channel',
        },
        shadowsocks_tcp: {
          priority: '330',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '8388',
          state_match: 'new,established',
          comment: 'allow Shadowsocks TCP listener',
        },
        shadowsocks_udp: {
          priority: '331',
          chain: 'input',
          action: 'accept',
          protocol: 'udp',
          dst_ports: '8388',
          state_match: 'new,established',
          comment: 'allow Shadowsocks UDP relay',
        },
        http_proxy: {
          priority: '340',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '3128',
          state_match: 'new,established',
          comment: 'allow HTTP proxy listener',
        },
        mtproto: {
          priority: '350',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '443',
          state_match: 'new,established',
          comment: 'allow MTProto TLS listener',
        },
        nginx_edge: {
          priority: '120',
          chain: 'input',
          action: 'accept',
          protocol: 'tcp',
          dst_ports: '80,443',
          state_match: 'new,established',
          comment: 'allow edge HTTP and HTTPS',
        },
        drop_invalid: {
          priority: '50',
          chain: 'input',
          action: 'drop',
          protocol: 'any',
          dst_ports: '',
          src_ports: '',
          state_match: 'invalid',
          comment: 'drop invalid tracked packets',
        },
      };
      const values = presets[preset];
      if (!values) return;
      Object.entries(values).forEach(([name, value]) => setFormValue(form, name, value));
      setFormValue(form, 'src_ports', values.src_ports || '');
      setFormValue(form, 'status', 'active');
      setFormChecked(form, 'enabled', true);
    }

    function selectOption(value, selected, label = value) {
      return `<option value="${escapeHTML(value)}"${String(value) === String(selected || '') ? ' selected' : ''}>${escapeHTML(label || value || 'auto')}</option>`;
    }

    function addressListOptions(selectedID) {
      const options = ['<option value="">none</option>'];
      for (const list of inventory().lists) {
        options.push(`<option value="${escapeHTML(list.id)}"${list.id === selectedID ? ' selected' : ''}>${escapeHTML(list.label || list.key)} · ${escapeHTML(list.key || list.id)}</option>`);
      }
      return options.join('');
    }

    function defaultPolicyOptions(selected) {
      return [
        selectOption('accept', selected),
        selectOption('drop', selected),
        selectOption('reject', selected),
      ].join('');
    }

    return { render };
  }

  window.MegaVPNFirewallPage = { create: createFirewallPage };
})(window);
