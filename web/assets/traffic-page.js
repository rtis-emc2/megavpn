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

    function exportCSVURL() {
      return apiURL('/api/v1/traffic/accounting/export?limit=10000');
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
              <button class="secondary-btn" id="trafficExportBtn" type="button">Export CSV</button>
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
        <section class="table-card">
          <div class="table-head">
            <h2>Recent traffic samples</h2>
            <div class="table-tools"><span class="tag">${escapeHTML(String(samples.length))} rows</span></div>
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
      document.getElementById('trafficExportBtn')?.addEventListener('click', () => {
        window.open(exportCSVURL(), '_blank', 'noopener,noreferrer');
      });
    }

    return { render };
  }

  window.MegaVPNTrafficPage = { create: createTrafficPage };
})(window);
