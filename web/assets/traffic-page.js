(function (window) {
  'use strict';

  function createTrafficPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      statusTag,
      escapeHTML,
      formatDate,
      apiURL,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof apiURL !== 'function'
    ) {
      throw new Error('MegaVPNTrafficPage requires page dependencies');
    }

    function bytes(value) {
      const num = Number(value || 0);
      if (!Number.isFinite(num) || num <= 0) return '0 B';
      const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
      let next = num;
      let idx = 0;
      while (next >= 1024 && idx < units.length - 1) {
        next /= 1024;
        idx++;
      }
      const precision = idx === 0 || next >= 100 ? 0 : next >= 10 ? 1 : 2;
      return `${next.toFixed(precision)} ${units[idx]}`;
    }

    function intValue(value) {
      const num = Number(value || 0);
      if (!Number.isFinite(num)) return '0';
      return String(Math.trunc(num));
    }

    function summaryCard(label, value, caption) {
      return `
        <div class="pool-summary-card">
          <span>${escapeHTML(label)}</span>
          <strong>${escapeHTML(String(value))}</strong>
          <small>${escapeHTML(caption)}</small>
        </div>`;
    }

    function trafficExportFilters() {
      const source = state.trafficExportFilters && typeof state.trafficExportFilters === 'object'
        ? state.trafficExportFilters
        : {};
      return {
        limit: String(source.limit || '10000'),
        from: String(source.from || ''),
        to: String(source.to || ''),
        protocol: String(source.protocol || ''),
        client_id: String(source.client_id || ''),
        node_id: String(source.node_id || ''),
      };
    }

    function saveTrafficExportFilters(filters) {
      state.trafficExportFilters = trafficExportFiltersFromObject(filters);
      try {
        sessionStorage.setItem('megavpn.trafficExportFilters', JSON.stringify(state.trafficExportFilters));
      } catch (_) {}
    }

    function trafficExportFiltersFromObject(source = {}) {
      const limit = Number(source.limit || 10000);
      return {
        limit: String(Number.isFinite(limit) && limit > 0 ? Math.min(Math.trunc(limit), 50000) : 10000),
        from: String(source.from || '').trim(),
        to: String(source.to || '').trim(),
        protocol: String(source.protocol || '').trim(),
        client_id: String(source.client_id || '').trim(),
        node_id: String(source.node_id || '').trim(),
      };
    }

    function trafficExportFiltersFromForm(form) {
      const data = new FormData(form);
      return trafficExportFiltersFromObject({
        limit: data.get('limit'),
        from: data.get('from'),
        to: data.get('to'),
        protocol: data.get('protocol'),
        client_id: data.get('client_id'),
        node_id: data.get('node_id'),
      });
    }

    function exportCSVURL(filters = trafficExportFilters()) {
      const params = new URLSearchParams();
      params.set('limit', filters.limit || '10000');
      for (const key of ['from', 'to', 'protocol', 'client_id', 'node_id']) {
        const value = String(filters[key] || '').trim();
        if (value) params.set(key, value);
      }
      return apiURL(`/api/v1/traffic/accounting/export?${params.toString()}`);
    }

    function optionList(items, valueKey, labelFn, selected) {
      const seen = new Set();
      return items.map((item) => {
        const value = String(item?.[valueKey] || '').trim();
        if (!value || seen.has(value)) return '';
        seen.add(value);
        const label = labelFn(item);
        return `<option value="${escapeHTML(value)}"${selected === value ? ' selected' : ''}>${escapeHTML(label)}</option>`;
      }).join('');
    }

    function protocolOptions(samples, selected) {
      const base = ['vless', 'xray', 'wireguard', 'openvpn', 'shadowsocks', 'ipsec', 'l2tp'];
      const observed = samples.map((sample) => String(sample.protocol || '').trim()).filter(Boolean);
      return Array.from(new Set([...base, ...observed])).sort()
        .map((value) => `<option value="${escapeHTML(value)}"${selected === value ? ' selected' : ''}>${escapeHTML(value)}</option>`)
        .join('');
    }

    function dateStart(value) {
      if (!value) return 0;
      const ms = Date.parse(`${value}T00:00:00Z`);
      return Number.isFinite(ms) ? ms : 0;
    }

    function dateEnd(value) {
      if (!value) return 0;
      const ms = Date.parse(`${value}T23:59:59Z`);
      return Number.isFinite(ms) ? ms : 0;
    }

    function filterSamples(samples, filters) {
      const from = dateStart(filters.from);
      const to = dateEnd(filters.to);
      return samples.filter((sample) => {
        if (filters.protocol && String(sample.protocol || '').trim() !== filters.protocol) return false;
        if (filters.client_id && String(sample.client_account_id || '').trim() !== filters.client_id) return false;
        if (filters.node_id && String(sample.node_id || '').trim() !== filters.node_id) return false;
        const bucketStart = Date.parse(sample.bucket_start || '');
        const bucketEnd = Date.parse(sample.bucket_end || '');
        if (from && Number.isFinite(bucketEnd) && bucketEnd < from) return false;
        if (to && Number.isFinite(bucketStart) && bucketStart > to) return false;
        return true;
      });
    }

    function renderExportFilters(filters, samples) {
      const nodes = Array.isArray(state.nodes) ? state.nodes : [];
      const clients = Array.isArray(state.clients) ? state.clients : [];
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Audit export filters</h2>
              <p>Use the same filters for recent-row preview and CSV export. The export is still capped and retention-scoped on the server.</p>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(filterSamples(samples, filters).length))} recent matches</span>
            </div>
          </div>
          <form id="trafficExportFilterForm" class="form-grid traffic-export-filter-form">
            <div class="field">
              <label>From</label>
              <input name="from" type="date" value="${escapeHTML(filters.from)}" />
            </div>
            <div class="field">
              <label>To</label>
              <input name="to" type="date" value="${escapeHTML(filters.to)}" />
            </div>
            <div class="field">
              <label>Protocol</label>
              <select name="protocol">
                <option value="">Any protocol</option>
                ${protocolOptions(samples, filters.protocol)}
              </select>
            </div>
            <div class="field">
              <label>Client</label>
              <select name="client_id">
                <option value="">Any client</option>
                ${optionList(clients, 'id', (client) => client.username || client.email || client.id, filters.client_id)}
              </select>
            </div>
            <div class="field">
              <label>Node</label>
              <select name="node_id">
                <option value="">Any node</option>
                ${optionList(nodes, 'id', (node) => node.name || node.id, filters.node_id)}
              </select>
            </div>
            <div class="field">
              <label>Limit</label>
              <input name="limit" type="number" min="1" max="50000" step="1" value="${escapeHTML(filters.limit)}" />
            </div>
            <div class="field full inline-actions align-end">
              <button class="secondary-btn" id="trafficExportResetBtn" type="button">Reset filters</button>
              <button class="primary-btn" id="trafficExportBtn" type="button">Export CSV</button>
            </div>
          </form>
        </section>`;
    }

    function sampleRows(samples) {
      if (!samples.length) {
        return `
          <tr>
            <td colspan="8">
              <div class="nodes-empty-state compact">
                <strong>No traffic samples yet</strong>
                <span>Agents can submit aggregate accounting samples after collectors are enabled on runtime nodes.</span>
              </div>
            </td>
          </tr>`;
      }
      return samples.map((sample) => `
        <tr>
          <td>
            <strong>${escapeHTML(sample.client_username || 'unattributed')}</strong>
            <small>${escapeHTML(sample.client_account_id || 'no client binding')}</small>
          </td>
          <td>
            <strong>${escapeHTML(sample.node_name || sample.node_id || 'node')}</strong>
            <small>${escapeHTML(sample.instance_name || sample.instance_id || 'node aggregate')}</small>
          </td>
          <td>${escapeHTML(sample.protocol || 'unknown')}</td>
          <td>${statusTag(sample.direction || 'unknown')}</td>
          <td><span class="mono">${escapeHTML(bytes(sample.rx_bytes))}</span></td>
          <td><span class="mono">${escapeHTML(bytes(sample.tx_bytes))}</span></td>
          <td>${escapeHTML(intValue(sample.flow_count))}</td>
          <td>
            <strong>${escapeHTML(formatDate(sample.bucket_end))}</strong>
            <small>${escapeHTML(sample.source || 'agent')}</small>
          </td>
        </tr>`).join('');
    }

    function render() {
      setTitle('Traffic Accounting');
      const data = state.trafficAccounting && typeof state.trafficAccounting === 'object'
        ? state.trafficAccounting
        : { summary: {}, samples: [] };
      const summary = data.summary || {};
      const samples = Array.isArray(data.samples) ? data.samples : [];
      const filters = trafficExportFilters();
      const previewSamples = filterSamples(samples, filters);
      const retention = Number(summary.retention_days || 180);
      const pruneBudget = Number(summary.max_prune_per_ingest || 0);
      const pruneBatch = Number(summary.prune_batch_size || 0);
      const pruneBatches = Number(summary.prune_batches_per_ingest || 0);
      const pruneCaption = pruneBatch > 0 && pruneBatches > 0
        ? `${intValue(pruneBatch)} x ${intValue(pruneBatches)} per ingest`
        : 'bounded cleanup per ingest';
      const cutoff = summary.retention_cutoff ? formatDate(summary.retention_cutoff) : 'not set';
      el('content').innerHTML = `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Traffic accounting</h2>
              <p>Aggregate client and node counters. The platform stores bytes, packets and flow counts only; URLs, payloads and request bodies are not collected.</p>
            </div>
            <div class="table-tools">
              ${statusTag(samples.length ? 'active' : 'planned')}
              <span class="tag">${escapeHTML(String(previewSamples.length))} preview rows</span>
            </div>
          </div>
          <div class="pool-summary-grid">
            ${summaryCard('Retention', `${retention} days`, 'automatic prune window')}
            ${summaryCard('Cutoff', cutoff, 'old rows hidden from reads')}
            ${summaryCard('Prune backlog', intValue(summary.expired_sample_count), 'expired rows pending cleanup')}
            ${summaryCard('Prune budget', pruneBudget ? intValue(pruneBudget) : 'bounded', pruneCaption)}
            ${summaryCard('Samples', intValue(summary.sample_count), 'stored aggregate rows')}
            ${summaryCard('Clients', intValue(summary.client_count), 'with attributed samples')}
            ${summaryCard('Nodes', intValue(summary.node_count), 'reporting counters')}
            ${summaryCard('Received', bytes(summary.rx_bytes), 'client uplink / ingress')}
            ${summaryCard('Sent', bytes(summary.tx_bytes), 'client downlink / egress')}
          </div>
        </section>
        ${renderExportFilters(filters, samples)}
        <section class="table-card">
          <div class="table-head">
            <h2>Recent traffic samples</h2>
            <div class="table-tools"><span class="tag">${escapeHTML(String(previewSamples.length))} / ${escapeHTML(String(samples.length))} rows</span></div>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Client</th>
                  <th>Node / instance</th>
                  <th>Protocol</th>
                  <th>Direction</th>
                  <th>Rx</th>
                  <th>Tx</th>
                  <th>Flows</th>
                  <th>Bucket</th>
                </tr>
              </thead>
              <tbody>${sampleRows(previewSamples)}</tbody>
            </table>
          </div>
        </section>
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Collection model</h2>
              <p>Agents submit normalized counters over the signed agent API. The control plane validates node ownership, links samples to known clients when service access is provided, and rejects malformed counters before storage.</p>
            </div>
          </div>
          <div class="firewall-flow-diagram">
            <div class="firewall-flow-step"><strong>1</strong><span>Runtime collector reads local aggregate counters</span><small>bytes, packets, flows</small></div>
            <div class="firewall-flow-arrow" aria-hidden="true">-></div>
            <div class="firewall-flow-step"><strong>2</strong><span>Agent signs accounting samples</span><small>node identity required</small></div>
            <div class="firewall-flow-arrow" aria-hidden="true">-></div>
            <div class="firewall-flow-step"><strong>3</strong><span>Control plane validates bindings</span><small>node, instance, client</small></div>
            <div class="firewall-flow-arrow" aria-hidden="true">-></div>
            <div class="firewall-flow-step"><strong>4</strong><span>PostgreSQL stores aggregate history</span><small>180-day retention</small></div>
          </div>
        </section>`;
      const form = document.getElementById('trafficExportFilterForm');
      form?.addEventListener('change', () => {
        saveTrafficExportFilters(trafficExportFiltersFromForm(form));
        render();
      });
      document.getElementById('trafficExportResetBtn')?.addEventListener('click', () => {
        saveTrafficExportFilters({ limit: '10000' });
        render();
      });
      document.getElementById('trafficExportBtn')?.addEventListener('click', () => {
        const current = form ? trafficExportFiltersFromForm(form) : trafficExportFilters();
        saveTrafficExportFilters(current);
        window.open(exportCSVURL(current), '_blank', 'noopener,noreferrer');
      });
    }

    return { render };
  }

  window.MegaVPNTrafficPage = { create: createTrafficPage };
})(window);
