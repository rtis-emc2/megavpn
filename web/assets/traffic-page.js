(function (window) {
  'use strict';

  function createTrafficPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      requestJSON,
      statusTag,
      escapeHTML,
      formatDate,
      formatDurationSeconds,
      apiURL,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof formatDurationSeconds !== 'function' ||
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
        <div class="traffic-summary-card">
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

    function trafficAccountingQuery(path, filters = trafficExportFilters(), fallbackLimit = '250') {
      const params = new URLSearchParams();
      params.set('limit', filters.limit || fallbackLimit);
      for (const key of ['from', 'to', 'protocol', 'client_id', 'node_id']) {
        const value = String(filters[key] || '').trim();
        if (value) params.set(key, value);
      }
      return `${path}?${params.toString()}`;
    }

    function exportCSVURL(filters = trafficExportFilters()) {
      return apiURL(trafficAccountingQuery('/api/v1/traffic/accounting/export', filters, '10000'));
    }

    async function reloadTrafficAccounting(filters) {
      const data = await requestJSON(trafficAccountingQuery('/api/v1/traffic/accounting', filters, '250'));
      state.trafficAccounting = data && typeof data === 'object' && !Array.isArray(data)
        ? {
            summary: data.summary && typeof data.summary === 'object' ? data.summary : { retention_days: 180 },
            samples: Array.isArray(data.samples) ? data.samples : [],
            collectors: Array.isArray(data.collectors) ? data.collectors : [],
            clients: Array.isArray(data.clients) ? data.clients : [],
          }
        : { summary: { retention_days: 180 }, samples: [], collectors: [], clients: [] };
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

    function hasActiveFilters(filters) {
      return ['from', 'to', 'protocol', 'client_id', 'node_id'].some((key) => String(filters[key] || '').trim() !== '');
    }

    function renderExportFilters(filters, samples) {
      const nodes = Array.isArray(state.nodes) ? state.nodes : [];
      const clients = Array.isArray(state.clients) ? state.clients : [];
      const rowCount = filterSamples(samples, filters).length;
      return `
        <section class="table-card traffic-filter-card">
          <div class="table-head">
            <div>
              <h2>Report filters</h2>
              <p>Filters apply to the counters below and to CSV download.</p>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(rowCount))} rows selected</span>
            </div>
          </div>
          <form id="trafficExportFilterForm" class="form-grid traffic-export-filter-form">
            <div class="field">
              <label>Start date</label>
              <input name="from" type="date" value="${escapeHTML(filters.from)}" />
            </div>
            <div class="field">
              <label>End date</label>
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
              <label>Max rows</label>
              <input name="limit" type="number" min="1" max="50000" step="1" value="${escapeHTML(filters.limit)}" />
            </div>
            <div class="traffic-filter-actions">
              <button class="secondary-btn" id="trafficExportResetBtn" type="button">Reset</button>
              <button class="primary-btn" id="trafficExportBtn" type="button"${rowCount ? '' : ' disabled'}>Download CSV</button>
            </div>
          </form>
        </section>`;
    }

    function renderTrafficEmptyState(filters, collectors) {
      const filtered = hasActiveFilters(filters);
      const title = filtered ? 'No rows match the selected filters' : 'No traffic data yet';
      const caption = filtered
        ? 'Reset filters or choose a wider date range to see retained counters.'
        : 'The control plane has not received accounting samples from node agents for this dataset.';
      const collectorCaption = collectors.length
        ? `${intValue(collectors.length)} collector streams are known, but no retained rows match the current view.`
        : 'No collector streams are reporting yet.';
      return `
        <section class="table-card traffic-empty-card">
          <div>
            <h2>${escapeHTML(title)}</h2>
            <p>${escapeHTML(caption)}</p>
          </div>
          <div class="traffic-empty-grid">
            <div>
              <strong>Collectors</strong>
              <span>${escapeHTML(collectorCaption)}</span>
            </div>
            <div>
              <strong>Managed services</strong>
              <span>Traffic appears after managed Xray, WireGuard or OpenVPN instances are applied and agents submit counters.</span>
            </div>
            <div>
              <strong>Retention</strong>
              <span>Only aggregate bytes, packets and flow counts are stored. URLs and payload bodies are not collected.</span>
            </div>
          </div>
        </section>`;
    }

    const trafficTabs = [
      ['overview', 'Overview', 'Counters and status'],
      ['clients', 'Clients', 'Per-client usage'],
      ['collectors', 'Collectors', 'Agent streams'],
      ['samples', 'Samples', 'Raw retained rows'],
      ['export', 'Export', 'Filters and CSV'],
    ];

    function trafficAccountingActiveTab() {
      if (!state.trafficAccountingTab) {
        try {
          state.trafficAccountingTab = localStorage.getItem('megavpn.trafficAccountingTab') || 'overview';
        } catch (_) {
          state.trafficAccountingTab = 'overview';
        }
      }
      const current = String(state.trafficAccountingTab || '').trim();
      const active = trafficTabs.some(([key]) => key === current) ? current : 'overview';
      state.trafficAccountingTab = active;
      return active;
    }

    function trafficTabBadge(key, summary, clients, collectors, samples) {
      switch (key) {
      case 'clients':
        return intValue(clients.length || summary.client_count);
      case 'collectors':
        return intValue(collectors.length);
      case 'samples':
        return intValue(samples.length);
      case 'export':
        return 'CSV';
      default:
        return bytes(Number(summary.rx_bytes || 0) + Number(summary.tx_bytes || 0));
      }
    }

    function renderTrafficTabs(active, summary, clients, collectors, samples) {
      return `
        <nav class="page-tabs control-tabs traffic-tabs" role="tablist" aria-label="Traffic accounting sections">
          ${trafficTabs.map(([key, label, caption]) => `
            <button class="page-tab ${active === key ? 'is-active' : ''}" type="button" data-traffic-tab="${escapeHTML(key)}" role="tab" aria-selected="${active === key ? 'true' : 'false'}">
              <span>${escapeHTML(label)} <em>${escapeHTML(trafficTabBadge(key, summary, clients, collectors, samples))}</em></span>
              <small>${escapeHTML(caption)}</small>
            </button>`).join('')}
        </nav>`;
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

    function collectorStatusCounts(collectors) {
      return collectors.reduce((acc, collector) => {
        const status = String(collector?.status || 'unknown').toLowerCase();
        acc[status] = (acc[status] || 0) + 1;
        return acc;
      }, {});
    }

    function collectorRows(collectors) {
      if (!collectors.length) {
        return `
          <tr>
            <td colspan="8">
              <div class="nodes-empty-state compact">
                <strong>No collector streams yet</strong>
                <span>Re-apply managed Xray, WireGuard or OpenVPN instances and wait for agent traffic accounting reports.</span>
              </div>
            </td>
          </tr>`;
      }
      return collectors.map((collector) => {
        const rx = Number(collector.rx_bytes || 0);
        const tx = Number(collector.tx_bytes || 0);
        const age = Number(collector.last_received_age_seconds || 0);
        const lastReceived = collector.last_received_at ? formatDate(collector.last_received_at) : 'n/a';
        const lastBucket = collector.last_bucket_end ? formatDate(collector.last_bucket_end) : 'n/a';
        const ageCaption = collector.last_received_at ? `${formatDurationSeconds(age)} ago` : 'no report yet';
        return `
          <tr>
            <td>
              <strong>${escapeHTML(collector.node_name || collector.node_id || 'node')}</strong>
              <small>${escapeHTML(collector.node_id || 'unknown node')}</small>
            </td>
            <td>
              <strong>${escapeHTML(collector.protocol || 'unknown')}</strong>
              <small>${escapeHTML(collector.source || 'agent')}</small>
            </td>
            <td>${statusTag(collector.status || 'unknown')}</td>
            <td>
              <strong>${escapeHTML(lastReceived)}</strong>
              <small>${escapeHTML(ageCaption)}</small>
            </td>
            <td>${escapeHTML(lastBucket)}</td>
            <td>
              <strong>${escapeHTML(intValue(collector.sample_count))}</strong>
              <small>${escapeHTML(intValue(collector.client_count))} clients</small>
            </td>
            <td>
              <strong>${escapeHTML(intValue(collector.expected_instance_count))} / ${escapeHTML(intValue(collector.observed_instance_count))}</strong>
              <small>${escapeHTML(intValue(collector.missing_instance_count))} missing</small>
            </td>
            <td><span class="mono">${escapeHTML(bytes(rx + tx))}</span></td>
          </tr>`;
      }).join('');
    }

    function clientUsageRows(clients) {
      if (!clients.length) {
        return `
          <tr>
            <td colspan="8">
              <div class="nodes-empty-state compact">
                <strong>No attributed client usage yet</strong>
                <span>Traffic samples are retained, but no retained rows are linked to client accounts for the selected filters.</span>
              </div>
            </td>
          </tr>`;
      }
      return clients.map((client) => {
        const rx = Number(client.rx_bytes || 0);
        const tx = Number(client.tx_bytes || 0);
        const total = rx + tx;
        return `
          <tr>
            <td>
              <strong>${escapeHTML(client.client_username || 'client')}</strong>
              <small>${escapeHTML(client.client_account_id || 'unknown client')}</small>
            </td>
            <td><span class="mono">${escapeHTML(bytes(total))}</span></td>
            <td><span class="mono">${escapeHTML(bytes(rx))}</span></td>
            <td><span class="mono">${escapeHTML(bytes(tx))}</span></td>
            <td>${escapeHTML(intValue(client.flow_count))}</td>
            <td>
              <strong>${escapeHTML(intValue(client.node_count))} nodes</strong>
              <small>${escapeHTML(intValue(client.instance_count))} instances · ${escapeHTML(intValue(client.protocol_count))} protocols</small>
            </td>
            <td>
              <strong>${escapeHTML(formatDate(client.last_bucket_end))}</strong>
              <small>first ${escapeHTML(formatDate(client.first_bucket_start))}</small>
            </td>
            <td>${escapeHTML(intValue(client.sample_count))}</td>
          </tr>`;
      }).join('');
    }

    function renderOverviewSection(summary, samples, previewSamples, collectors, filters) {
      const collectorCounts = collectorStatusCounts(collectors);
      const retention = Number(summary.retention_days || 180);
      const hasTrafficRows = Number(summary.sample_count || 0) > 0 || samples.length > 0 || previewSamples.length > 0;
      const hasVisibleTraffic = hasTrafficRows || collectors.length > 0 || Number(summary.client_count || 0) > 0;
      const collectorProblemCount = Number(collectorCounts.degraded || 0) + Number(collectorCounts.missing || 0) + Number(collectorCounts.inactive || 0);
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Traffic accounting</h2>
              <p>Aggregate traffic counters for clients and nodes. Only bytes, packets and flow counts are stored.</p>
            </div>
            <div class="table-tools">
              ${samples.length ? statusTag('active') : '<span class="tag">no data</span>'}
              <span class="tag">${escapeHTML(String(previewSamples.length))} visible rows</span>
            </div>
          </div>
          <div class="traffic-summary-grid">
            ${summaryCard('Total traffic', bytes(Number(summary.rx_bytes || 0) + Number(summary.tx_bytes || 0)), 'received + sent')}
            ${summaryCard('Received', bytes(summary.rx_bytes), 'client upload')}
            ${summaryCard('Sent', bytes(summary.tx_bytes), 'client download')}
            ${summaryCard('Samples', intValue(summary.sample_count), 'retained rows')}
            ${summaryCard('Clients', intValue(summary.client_count), 'with traffic')}
            ${summaryCard('Nodes', intValue(summary.node_count), 'reporting')}
            ${summaryCard('Collectors', intValue(collectors.length), collectors.length ? `${intValue(collectorCounts.active)} active, ${intValue(collectorProblemCount)} need attention` : 'no streams')}
            ${summaryCard('Retention', `${retention} days`, 'audit history')}
          </div>
        </section>
        ${hasVisibleTraffic ? '' : renderTrafficEmptyState(filters, collectors)}`;
    }

    function renderClientUsageSection(clients) {
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Client usage</h2>
              <p>Aggregate traffic by client for the selected report filters.</p>
            </div>
            <div class="table-tools"><span class="tag">${escapeHTML(String(clients.length))} clients</span></div>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Client</th>
                  <th>Total</th>
                  <th>Rx</th>
                  <th>Tx</th>
                  <th>Flows</th>
                  <th>Coverage</th>
                  <th>Window</th>
                  <th>Samples</th>
                </tr>
              </thead>
              <tbody>${clientUsageRows(clients)}</tbody>
            </table>
          </div>
        </section>`;
    }

    function renderCollectorStatusSection(collectors) {
      const collectorCounts = collectorStatusCounts(collectors);
      return `
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Collector status</h2>
              <p>Agent counter streams by node and protocol.</p>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(collectors.length))} streams</span>
              ${statusTag(collectorCounts.missing || collectorCounts.inactive || collectorCounts.degraded ? 'degraded' : collectors.length ? 'active' : 'planned')}
            </div>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Node</th>
                  <th>Collector</th>
                  <th>Status</th>
                  <th>Last report</th>
                  <th>Last bucket</th>
                  <th>Samples</th>
                  <th>Expected / observed</th>
                  <th>Traffic</th>
                </tr>
              </thead>
              <tbody>${collectorRows(collectors)}</tbody>
            </table>
          </div>
        </section>`;
    }

    function renderSampleSection(samples) {
      return `
        <section class="table-card">
          <div class="table-head">
            <h2>Recent traffic samples</h2>
            <div class="table-tools"><span class="tag">${escapeHTML(String(samples.length))} retained rows</span></div>
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
              <tbody>${sampleRows(samples)}</tbody>
            </table>
          </div>
        </section>`;
    }

    function renderTrafficPanel(activeTab, summary, samples, previewSamples, collectors, clients, filters) {
      switch (activeTab) {
      case 'clients':
        return renderClientUsageSection(clients);
      case 'collectors':
        return renderCollectorStatusSection(collectors);
      case 'samples':
        return renderSampleSection(previewSamples);
      case 'export':
        return renderExportFilters(filters, samples);
      default:
        return renderOverviewSection(summary, samples, previewSamples, collectors, filters);
      }
    }

    function render() {
      setTitle('Traffic Accounting');
      const data = state.trafficAccounting && typeof state.trafficAccounting === 'object'
        ? state.trafficAccounting
        : { summary: {}, samples: [] };
      const summary = data.summary || {};
      const samples = Array.isArray(data.samples) ? data.samples : [];
      const collectors = Array.isArray(data.collectors) ? data.collectors : [];
      const clients = Array.isArray(data.clients) ? data.clients : [];
      const filters = trafficExportFilters();
      const previewSamples = filterSamples(samples, filters);
      const activeTab = trafficAccountingActiveTab();
      el('content').innerHTML = `
        ${renderTrafficTabs(activeTab, summary, clients, collectors, previewSamples)}
        ${renderTrafficPanel(activeTab, summary, samples, previewSamples, collectors, clients, filters)}`;
      document.querySelectorAll('[data-traffic-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          const tab = button.dataset.trafficTab || 'overview';
          state.trafficAccountingTab = trafficTabs.some(([key]) => key === tab) ? tab : 'overview';
          try {
            localStorage.setItem('megavpn.trafficAccountingTab', state.trafficAccountingTab);
          } catch (_) {}
          render();
        });
      });
      const form = document.getElementById('trafficExportFilterForm');
      form?.addEventListener('change', async () => {
        const current = trafficExportFiltersFromForm(form);
        saveTrafficExportFilters(current);
        try {
          await reloadTrafficAccounting(current);
        } catch (err) {
          state.lastError = err;
        }
        render();
      });
      document.getElementById('trafficExportResetBtn')?.addEventListener('click', async () => {
        const resetFilters = trafficExportFiltersFromObject({ limit: '10000' });
        saveTrafficExportFilters(resetFilters);
        try {
          await reloadTrafficAccounting(resetFilters);
        } catch (err) {
          state.lastError = err;
        }
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
