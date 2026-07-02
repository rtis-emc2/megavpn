(function (window) {
  'use strict';

  function createNodeMapPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      setPage,
      sendJSON,
      loadCore,
      statusTag,
      escapeHTML,
    } = ctx;

    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof setPage !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function'
    ) {
      throw new Error('MegaVPNNodeMapPage requires page dependencies');
    }

    const config = window.MegaVPNAppConfig?.nodeMap || {};
    const STATIC_MAP_URL = safeStaticMapURL(config.staticMapURL);
    const routeUpdates = new Set();

    function safeStaticMapURL(value) {
      const fallback = './assets/world-map.svg';
      const raw = String(value || fallback).trim();
      if (!raw || raw.includes('..')) return fallback;
      if (!/^\.\/assets\/[a-z0-9/_-]+(?:\.[a-z0-9]+)?$/i.test(raw)) return fallback;
      return raw;
    }

    function finiteNumber(value) {
      const num = Number(value);
      return Number.isFinite(num) ? num : null;
    }

    function safeClassToken(value) {
      return String(value || 'unknown').toLowerCase().replace(/[^a-z0-9_-]/g, '-') || 'unknown';
    }

    function normalizeStatus(value, fallback = 'unknown') {
      return String(value || fallback).trim().toLowerCase() || fallback;
    }

    function nodeLocation(node) {
      const latitude = finiteNumber(node?.latitude);
      const longitude = finiteNumber(node?.longitude);
      if (latitude == null || longitude == null) return null;
      if (latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180) return null;
      return { latitude, longitude };
    }

    function nodesList() {
      return Array.isArray(state.nodes) ? state.nodes : [];
    }

    function backhaulList() {
      return (Array.isArray(state.backhaulLinks) ? state.backhaulLinks : [])
        .filter((link) => normalizeStatus(link.status, '') !== 'deleted');
    }

    function nodesByID() {
      return new Map(nodesList()
        .map((node) => [String(node.id || ''), node])
        .filter(([id]) => id));
    }

    function locatedNodes() {
      return nodesList()
        .map((node) => ({ node, location: nodeLocation(node) }))
        .filter((item) => item.location)
        .sort((a, b) => String(a.node.name || '').localeCompare(String(b.node.name || '')));
    }

    function unresolvedNodes() {
      return nodesList()
        .filter((node) => !nodeLocation(node))
        .sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')));
    }

    function geoStatus(node) {
      return normalizeStatus(node?.geoip_status, nodeLocation(node) ? 'resolved' : 'pending');
    }

    function nodeStatus(node) {
      const candidates = [node?.status, node?.agent_status, node?.node_state]
        .map((item) => normalizeStatus(item, ''))
        .filter(Boolean);
      return candidates[0] || 'unknown';
    }

    function nodeTone(node) {
      const values = [nodeStatus(node), geoStatus(node)].join(' ');
      if (/(failed|offline|unhealthy|error|missing)/.test(values)) return 'danger';
      if (/(degraded|pending|provisioning|unknown|skipped)/.test(values)) return 'warning';
      return 'healthy';
    }

    function linkTone(status) {
      const value = normalizeStatus(status);
      if (/(failed|deleted|error|unhealthy)/.test(value)) return 'danger';
      if (/(disabled|inactive|off)/.test(value)) return 'disabled';
      if (/(degraded|provisioning|pending|unknown)/.test(value)) return 'warning';
      return 'healthy';
    }

    function countryLabel(node) {
      return String(node?.geoip_country_name || node?.geoip_country_code || '').trim() || 'unknown country';
    }

    function locationLabel(node) {
      const parts = [
        node?.geoip_city,
        node?.geoip_region,
        node?.geoip_country_name || node?.geoip_country_code,
      ].map((part) => String(part || '').trim()).filter(Boolean);
      return parts.length ? parts.join(', ') : (String(node?.location_label || '').trim() || 'location pending');
    }

    function ownerLabel(node) {
      const org = String(node?.geoip_org || '').trim();
      const asn = String(node?.geoip_asn || '').trim();
      if (org && asn && !org.toLowerCase().includes(asn.toLowerCase())) return `${asn} · ${org}`;
      return org || asn || 'network owner pending';
    }

    function geoSourceLabel(node) {
      const provider = String(node?.geoip_provider || '').trim();
      const ip = String(node?.geoip_ip || '').trim();
      if (provider && ip) return `${provider} · ${ip}`;
      return provider || ip || 'pending';
    }

    function coordsLabel(location) {
      return location ? `${location.latitude.toFixed(4)}, ${location.longitude.toFixed(4)}` : 'pending';
    }

    function formatTime(value) {
      if (!value) return 'not refreshed yet';
      const time = new Date(value);
      if (Number.isNaN(time.getTime())) return 'not refreshed yet';
      return new Intl.DateTimeFormat(undefined, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      }).format(time);
    }

    function selectedTransport(link) {
      const transports = Array.isArray(link?.transports) ? link.transports : [];
      const selectedID = String(link?.selected_transport_id || '').trim();
      return transports.find((transport) => String(transport.id || '').trim() === selectedID)
        || transports.find((transport) => String(transport.driver || '').trim() === String(link?.desired_driver || '').trim())
        || transports[0]
        || null;
    }

    function backhaulStatus(link) {
      const transport = selectedTransport(link);
      const linkStatus = normalizeStatus(link?.status, '');
      if (['disabled', 'deleted', 'pending_apply', 'failed', 'planned'].includes(linkStatus)) {
        return linkStatus;
      }
      return normalizeStatus(transport?.status || linkStatus, 'unknown');
    }

    function backhaulDriver(link) {
      const transport = selectedTransport(link);
      return String(transport?.driver || link?.desired_driver || 'driver pending').trim();
    }

    function backhaulEndpoint(link) {
      const transport = selectedTransport(link);
      const explicit = String(transport?.endpoint || '').trim();
      if (explicit) return explicit;
      const host = String(transport?.endpoint_host || '').trim();
      if (!host) return '';
      const port = Number(transport?.endpoint_port || 0);
      const protocol = String(transport?.protocol || '').trim();
      const address = port > 0 ? `${host}:${port}` : host;
      return protocol ? `${address} ${protocol}` : address;
    }

    function backhaulIssue(link) {
      const transport = selectedTransport(link);
      return String(transport?.last_error || link?.last_error || '').trim();
    }

    function jobStatusActive(status) {
      return ['queued', 'running', 'retrying'].includes(normalizeStatus(status, ''));
    }

    function jobTypeLabel(type) {
      return String(type || 'job')
        .replace(/^node\./, '')
        .replace(/_/g, ' ')
        .replace(/\./g, ' ')
        .trim() || 'job';
    }

    function jobTypeSummary(jobs) {
      const counts = new Map();
      (Array.isArray(jobs) ? jobs : []).forEach((job) => {
        const label = jobTypeLabel(job?.type);
        counts.set(label, (counts.get(label) || 0) + 1);
      });
      return Array.from(counts.entries())
        .map(([label, count]) => `${label}${count > 1 ? ` x${count}` : ''}`)
        .join(', ');
    }

    function activeBackhaulJobs(link) {
      const linkID = String(link?.id || '');
      const ingressNodeID = String(link?.ingress_node_id || '');
      return (Array.isArray(state.jobs) ? state.jobs : []).filter((job) => {
        if (!jobStatusActive(job?.status)) return false;
        const scopeType = normalizeStatus(job?.scope_type, '');
        const scopeID = String(job?.scope_id || '').trim();
        const nodeID = String(job?.node_id || '').trim();
        const jobType = String(job?.type || '').trim();
        if (scopeType === 'backhaul' && scopeID === linkID) return true;
        if (jobType === 'node.route_policy.apply' && nodeID === ingressNodeID) return true;
        const payload = job?.payload || {};
        return String(payload.link_id || payload.backhaul_link_id || '').trim() === linkID;
      });
    }

    function linkMetric(link) {
      const metric = Number(link?.route_metric || 0);
      return Number.isFinite(metric) && metric > 0 ? metric : 0;
    }

    function drawableBackhaulLinks(items) {
      const located = new Map(items.map((item) => [String(item.node.id || ''), item]));
      return backhaulList()
        .map((link) => ({
          link,
          ingress: located.get(String(link.ingress_node_id || '')),
          egress: located.get(String(link.egress_node_id || '')),
        }))
        .filter((item) => item.ingress && item.egress);
    }

    function relatedBackhaul(nodeID, links = backhaulList()) {
      const id = String(nodeID || '');
      return links.filter((link) => String(link.ingress_node_id || '') === id || String(link.egress_node_id || '') === id);
    }

    function countryCount(items) {
      return new Set(items
        .map(({ node }) => String(node.geoip_country_code || node.geoip_country_name || '').trim())
        .filter(Boolean)).size;
    }

    function clampLatitude(lat) {
      return Math.max(-85.05112878, Math.min(85.05112878, lat));
    }

    function project(location) {
      const lat = clampLatitude(location.latitude);
      const sin = Math.sin((lat * Math.PI) / 180);
      return {
        x: ((location.longitude + 180) / 360) * 100,
        y: (0.5 - Math.log((1 + sin) / (1 - sin)) / (4 * Math.PI)) * 100,
      };
    }

    function mapViewport() {
      return {};
    }

    function markerPosition(location) {
      return project(location);
    }

    function markerPoints(items, viewport) {
      const groups = new Map();
      const points = items.map((item) => {
        const point = markerPosition(item.location, viewport);
        const key = `${Math.round(point.x)}:${Math.round(point.y)}`;
        if (!groups.has(key)) groups.set(key, []);
        const list = groups.get(key);
        const record = { ...item, point, offsetIndex: list.length };
        list.push(record);
        return record;
      });
      return points.map((record) => {
        const key = `${Math.round(record.point.x)}:${Math.round(record.point.y)}`;
        const group = groups.get(key) || [];
        if (group.length <= 1) return { ...record, offsetX: 0, offsetY: 0, clusterSize: 1 };
        const angle = (record.offsetIndex / group.length) * Math.PI * 2;
        const radius = Math.min(34, 14 + group.length * 3);
        return {
          ...record,
          offsetX: Math.cos(angle) * radius,
          offsetY: Math.sin(angle) * radius,
          clusterSize: group.length,
        };
      });
    }

    function renderMarkers(items, viewport, selectedID) {
      return markerPoints(items, viewport).map(({ node, location, point, offsetX, offsetY, clusterSize }) => {
        const id = String(node.id || '');
        const selected = id && id === selectedID;
        const tone = nodeTone(node);
        const title = `${node.name || 'node'} · ${locationLabel(node)} · ${ownerLabel(node)}`;
        return `
          <button class="node-map-pin ${tone}${selected ? ' selected' : ''}" type="button" data-node-id="${escapeHTML(id)}" style="left:calc(${point.x.toFixed(3)}% + ${offsetX.toFixed(1)}px);top:calc(${point.y.toFixed(3)}% + ${offsetY.toFixed(1)}px)" title="${escapeHTML(title)}">
            <span class="node-map-pin-head"></span>
            <span class="node-map-pin-label">${escapeHTML(node.name || 'node')}</span>
            ${clusterSize > 1 ? `<span class="node-map-pin-cluster">+${escapeHTML(String(clusterSize - 1))}</span>` : ''}
            <span class="sr-only">${escapeHTML(coordsLabel(location))}</span>
          </button>`;
      }).join('');
    }

    function curvedPath(a, b) {
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const distance = Math.sqrt((dx * dx) + (dy * dy));
      if (distance < 0.4) {
        return `M ${a.x.toFixed(3)} ${a.y.toFixed(3)} C ${(a.x + 4).toFixed(3)} ${(a.y - 7).toFixed(3)}, ${(a.x + 8).toFixed(3)} ${(a.y + 7).toFixed(3)}, ${a.x.toFixed(3)} ${a.y.toFixed(3)}`;
      }
      const bend = Math.min(9, Math.max(2.5, distance * 0.09));
      const nx = -dy / distance;
      const ny = dx / distance;
      const cx = ((a.x + b.x) / 2) + nx * bend;
      const cy = ((a.y + b.y) / 2) + ny * bend;
      return `M ${a.x.toFixed(3)} ${a.y.toFixed(3)} Q ${cx.toFixed(3)} ${cy.toFixed(3)} ${b.x.toFixed(3)} ${b.y.toFixed(3)}`;
    }

    function renderBackhaulOverlay(items, viewport) {
      const paths = drawableBackhaulLinks(items).map(({ link, ingress, egress }) => {
        const a = markerPosition(ingress.location, viewport);
        const b = markerPosition(egress.location, viewport);
        const status = backhaulStatus(link);
        const tone = linkTone(status);
        const markerID = `node-map-arrow-${tone}`;
        const metric = linkMetric(link);
        const title = [
          String(link?.name || 'backhaul').trim(),
          `${ingress.node.name || 'ingress'} -> ${egress.node.name || 'egress'}`,
          backhaulDriver(link),
          status,
          metric ? `metric ${metric}` : '',
        ].filter(Boolean).join(' · ');
        const midX = (a.x + b.x) / 2;
        const midY = (a.y + b.y) / 2;
        return `
          <g class="node-map-backhaul-link ${tone}">
            <path d="${curvedPath(a, b)}" marker-end="url(#${escapeHTML(markerID)})">
              <title>${escapeHTML(title)}</title>
            </path>
            ${metric ? `<text x="${midX.toFixed(3)}" y="${midY.toFixed(3)}">${escapeHTML(String(metric))}</text>` : ''}
          </g>`;
      }).join('');
      if (!paths) return '';
      return `
        <svg class="node-map-backhaul-layer" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="Managed backhaul links">
          <defs>
            <marker id="node-map-arrow-healthy" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="strokeWidth"><path d="M 0 0 L 8 4 L 0 8 z" fill="rgba(5,150,105,0.86)"></path></marker>
            <marker id="node-map-arrow-warning" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="strokeWidth"><path d="M 0 0 L 8 4 L 0 8 z" fill="rgba(217,119,6,0.86)"></path></marker>
            <marker id="node-map-arrow-danger" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="strokeWidth"><path d="M 0 0 L 8 4 L 0 8 z" fill="rgba(220,38,38,0.86)"></path></marker>
            <marker id="node-map-arrow-disabled" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto" markerUnits="strokeWidth"><path d="M 0 0 L 8 4 L 0 8 z" fill="rgba(100,116,139,0.72)"></path></marker>
          </defs>
          ${paths}
        </svg>`;
    }

    function selectedLocatedNode(items) {
      const selectedID = String(state.nodeMapSelectedNodeID || '').trim();
      const selected = selectedID ? items.find((item) => String(item.node.id || '') === selectedID) : null;
      return selected || items[0] || null;
    }

    function renderTopologyLegend(located, unresolved, drawableLinks, links) {
      return `
        <div class="node-map-hud node-map-legend">
          <div><span class="node-map-legend-dot healthy"></span><strong>${escapeHTML(String(located.length))}</strong><span>located</span></div>
          <div><span class="node-map-legend-dot warning"></span><strong>${escapeHTML(String(unresolved.length))}</strong><span>pending GeoIP</span></div>
          <div><span class="node-map-legend-line"></span><strong>${escapeHTML(String(drawableLinks.length))}</strong><span>mapped links</span></div>
          <div><span class="node-map-legend-line muted"></span><strong>${escapeHTML(String(Math.max(0, links.length - drawableLinks.length)))}</strong><span>waiting coordinates</span></div>
        </div>`;
    }

    function renderRelatedBackhaulRows(node, nodeLookup) {
      const rows = relatedBackhaul(node?.id).map((link) => {
        const currentID = String(node?.id || '');
        const isIngress = String(link.ingress_node_id || '') === currentID;
        const peerID = isIngress ? link.egress_node_id : link.ingress_node_id;
        const peer = nodeLookup.get(String(peerID || ''));
        const status = backhaulStatus(link);
        const endpoint = backhaulEndpoint(link);
        const metric = linkMetric(link);
        return `
          <div class="node-map-link-chip ${linkTone(status)}">
            <div>
              <strong>${escapeHTML(isIngress ? 'egress route' : 'ingress route')}</strong>
              <span>${escapeHTML(isIngress ? 'to' : 'from')} ${escapeHTML(peer?.name || 'node')}</span>
            </div>
            <div>
              ${statusTag(status)}
              ${statusTag(backhaulDriver(link))}
              ${metric ? statusTag(`metric ${metric}`) : ''}
            </div>
            ${endpoint ? `<span class="muted-mono">${escapeHTML(endpoint)}</span>` : ''}
          </div>`;
      });
      return rows.length ? rows.join('') : '<div class="node-map-muted-box">No managed backhaul is attached to this node.</div>';
    }

    function renderSelectedNodePanel(item, nodeLookup) {
      if (!item) {
        return `
          <aside class="node-map-inspector">
            <div class="node-map-inspector-empty">
              <strong>No mapped nodes yet</strong>
              <span>GeoIP will appear here after the API resolves public node addresses.</span>
            </div>
          </aside>`;
      }
      const { node, location } = item;
      return `
        <aside class="node-map-inspector">
          <div class="node-map-inspector-head">
            <div>
              <span class="mini-label">Selected node</span>
              <h3>${escapeHTML(node.name || 'node')}</h3>
              <p>${escapeHTML(node.address || 'address pending')}</p>
            </div>
            <div class="node-map-node-tags">
              ${statusTag(node.role || 'node')}
              ${statusTag(nodeStatus(node))}
              ${statusTag(geoStatus(node))}
            </div>
          </div>
          <div class="node-map-inspector-facts">
            <div><span>Location</span><strong>${escapeHTML(locationLabel(node))}</strong></div>
            <div><span>Country</span><strong>${escapeHTML(countryLabel(node))}</strong></div>
            <div><span>Network</span><strong>${escapeHTML(ownerLabel(node))}</strong></div>
            <div><span>Coordinates</span><strong>${escapeHTML(coordsLabel(location))}</strong></div>
            <div><span>GeoIP source</span><strong>${escapeHTML(geoSourceLabel(node))}</strong></div>
            <div><span>Resolved</span><strong>${escapeHTML(formatTime(node.geoip_resolved_at))}</strong></div>
          </div>
          ${String(node.geoip_error || '').trim() ? `<div class="node-map-node-issue">${escapeHTML(node.geoip_error)}</div>` : ''}
          <div class="node-map-inspector-links">
            <div class="node-map-panel-title">Backhaul on this node</div>
            ${renderRelatedBackhaulRows(node, nodeLookup)}
          </div>
          <div class="node-map-node-actions">
            <button class="secondary-btn node-map-open-btn" type="button" data-node-id="${escapeHTML(node.id || '')}">Open node</button>
          </div>
        </aside>`;
    }

    function renderUnresolvedNodes(nodes) {
      if (!nodes.length) return '';
      const rows = nodes.slice(0, 8).map((node) => {
        const reason = String(node.geoip_error || '').trim()
          || (geoStatus(node) === 'skipped' ? 'private or internal address skipped' : 'waiting for GeoIP refresh');
        return `
          <button class="node-map-pending-row node-map-open-btn" type="button" data-node-id="${escapeHTML(node.id || '')}">
            <span>
              <strong>${escapeHTML(node.name || 'node')}</strong>
              <small>${escapeHTML(node.address || 'address pending')}</small>
            </span>
            <span>${statusTag(geoStatus(node))}</span>
            <small>${escapeHTML(reason)}</small>
          </button>`;
      }).join('');
      const extra = nodes.length > 8 ? `<div class="node-map-muted-box">${escapeHTML(String(nodes.length - 8))} more pending nodes are hidden.</div>` : '';
      return `
        <section class="section-card node-map-pending-card">
          <div class="section-head compact">
            <div>
              <h2>Nodes waiting for GeoIP</h2>
              <p>Only public addresses are sent to the configured GeoIP endpoint. Private and internal addresses stay unmapped.</p>
            </div>
          </div>
          <div class="node-map-pending-list">${rows}${extra}</div>
        </section>`;
    }

    function renderBackhaulMatrix(links, nodeLookup, locatedLookup) {
      if (!links.length) return '';
      const rows = links.map((link) => {
        const ingress = nodeLookup.get(String(link.ingress_node_id || ''));
        const egress = nodeLookup.get(String(link.egress_node_id || ''));
        const drawable = locatedLookup.has(String(link.ingress_node_id || '')) && locatedLookup.has(String(link.egress_node_id || ''));
        const status = backhaulStatus(link);
        const endpoint = backhaulEndpoint(link);
        const issue = backhaulIssue(link);
        const metric = linkMetric(link);
        const linkID = String(link.id || '');
        const disabled = normalizeStatus(link.status, '') === 'disabled';
        const busy = routeUpdates.has(linkID);
        const routeEnabled = !disabled;
        const activeJobs = activeBackhaulJobs(link);
        const activeJobSummary = jobTypeSummary(activeJobs);
        const routeText = routeEnabled
          ? 'Participates in managed routing and route-policy selection.'
          : 'Removed from managed routing until it is enabled again.';
        const routeHint = busy
          ? 'Route state update is queued...'
          : (activeJobSummary ? `Active jobs: ${activeJobSummary}.` : routeText);
        return `
          <article class="node-map-backhaul-card ${linkTone(status)}${busy ? ' updating' : ''}">
            <div class="node-map-backhaul-head">
              <div>
                <span class="mini-label">Backhaul</span>
                <h3>${escapeHTML(link.name || 'managed link')}</h3>
                <p>${escapeHTML(ingress?.name || 'ingress pending')} -> ${escapeHTML(egress?.name || 'egress pending')}</p>
              </div>
              <div class="node-map-backhaul-tags">
                ${statusTag(status)}
                ${statusTag(backhaulDriver(link))}
                ${metric ? statusTag(`metric ${metric}`) : ''}
                ${statusTag(drawable ? 'mapped' : 'waiting GeoIP')}
              </div>
            </div>
            ${issue ? `<div class="node-map-backhaul-issue">${escapeHTML(issue)}</div>` : ''}
            <div class="node-map-backhaul-route-row">
              <span class="muted-mono">${escapeHTML(endpoint || 'endpoint pending')}</span>
              <label class="node-map-route-switch ${routeEnabled ? 'enabled' : 'disabled'}${busy ? ' busy' : ''}">
                <input class="node-map-route-input" type="checkbox" data-link-id="${escapeHTML(linkID)}" ${routeEnabled ? 'checked' : ''} ${busy || typeof sendJSON !== 'function' ? 'disabled' : ''}>
                <span></span>
                <strong>${escapeHTML(routeEnabled ? 'Route enabled' : 'Route disabled')}</strong>
              </label>
            </div>
            <small class="node-map-route-hint">${escapeHTML(routeHint)}</small>
          </article>`;
      }).join('');
      const notice = renderRouteNotice();
      return `
        <section class="section-card node-map-topology-card">
          <div class="section-head compact">
            <div>
              <h2>Backhaul topology</h2>
              <p>Managed ingress-to-egress routes are drawn as directed lines when both endpoint nodes are mapped. Disabled routes stay visible and are excluded from managed routing.</p>
            </div>
          </div>
          ${notice}
          <div class="node-map-backhaul-grid">${rows}</div>
        </section>`;
    }

    function renderRouteNotice() {
      const notice = state.nodeMapRouteNotice;
      if (!notice) return '';
      const jobs = Array.isArray(notice.jobs) ? notice.jobs : [];
      const details = [
        String(notice.detail || '').trim(),
        jobs.length ? `Jobs: ${jobTypeSummary(jobs)}.` : '',
      ].filter(Boolean);
      return `
        <div class="node-map-route-notice ${escapeHTML(safeClassToken(notice.tone || ''))}">
          <strong>${escapeHTML(notice.text || '')}</strong>
          ${details.map((detail) => `<small>${escapeHTML(detail)}</small>`).join('')}
        </div>`;
    }

    function renderLocationDirectory(items) {
      if (!items.length) return '';
      const rows = items.map(({ node, location }) => `
        <button class="node-map-location-row" type="button" data-node-id="${escapeHTML(node.id || '')}">
          <span class="node-map-location-pin ${nodeTone(node)}"></span>
          <span>
            <strong>${escapeHTML(node.name || 'node')}</strong>
            <small>${escapeHTML(locationLabel(node))}</small>
          </span>
          <span>${escapeHTML(ownerLabel(node))}</span>
          <small>${escapeHTML(coordsLabel(location))}</small>
        </button>`).join('');
      return `
        <section class="section-card node-map-location-card">
          <div class="section-head compact">
            <div>
              <h2>Mapped nodes</h2>
              <p>GeoIP placement, network owner and quick selection without a separate table.</p>
            </div>
          </div>
          <div class="node-map-location-list">${rows}</div>
        </section>`;
    }

    function openNode(nodeID) {
      if (!nodeID) return;
      state.nodeManageID = nodeID;
      state.nodeManageData = null;
      setPage('nodeManage');
    }

    function selectNode(nodeID) {
      if (!nodeID) return;
      state.nodeMapSelectedNodeID = nodeID;
      render();
    }

    function mergeBackhaulLink(link) {
      if (!link || !link.id || !Array.isArray(state.backhaulLinks)) return;
      const id = String(link.id);
      const next = state.backhaulLinks.slice();
      const index = next.findIndex((item) => String(item.id || '') === id);
      if (index >= 0) next[index] = link;
      else next.push(link);
      state.backhaulLinks = next;
    }

    async function setRouteEnabled(linkID, enabled) {
      if (!linkID || typeof sendJSON !== 'function') return;
      routeUpdates.add(linkID);
      state.nodeMapRouteNotice = {
        tone: 'warning',
        text: enabled ? 'Route enable request is being sent.' : 'Route disable request is being sent.',
        detail: 'Waiting for the control plane to queue the required node jobs.',
        jobs: [],
      };
      render();
      try {
        const result = await sendJSON(`/api/v1/backhaul-links/${encodeURIComponent(linkID)}/route`, 'PATCH', { enabled });
        mergeBackhaulLink(result?.link);
        const jobs = Array.isArray(result?.jobs) ? result.jobs : [];
        const summary = jobTypeSummary(jobs);
        state.nodeMapRouteNotice = {
          tone: 'healthy',
          text: enabled ? 'Route enable accepted.' : 'Route disable accepted.',
          detail: summary ? `Queued: ${summary}.` : 'No new job was required; route state was already current.',
          jobs,
        };
        if (typeof loadCore === 'function') {
          await loadCore();
        }
      } catch (err) {
        state.nodeMapRouteNotice = {
          tone: 'danger',
          text: err?.message || 'Route state update failed.',
          detail: 'The UI kept the previous route state. Check jobs and retry after the issue is fixed.',
          jobs: [],
        };
      } finally {
        routeUpdates.delete(linkID);
        render();
      }
    }

    function render() {
      setTitle('Node Map');
      const nodes = nodesList();
      const located = locatedNodes();
      const unresolved = unresolvedNodes();
      const links = backhaulList();
      const drawableLinks = drawableBackhaulLinks(located);
      const nodeLookup = nodesByID();
      const locatedLookup = new Map(located.map((item) => [String(item.node.id || ''), item]));
      const viewport = mapViewport(located);
      const selected = selectedLocatedNode(located);
      const selectedID = String(selected?.node?.id || '');
      if (selectedID) state.nodeMapSelectedNodeID = selectedID;

      el('content').innerHTML = `
        <section class="control-page-shell node-map-page">
          <div class="control-page-intro node-map-intro">
            <div>
              <h2>Global node topology</h2>
              <p>Nodes are placed automatically by public IP GeoIP. Backhaul routes are drawn between mapped ingress and egress nodes with their current transport status.</p>
            </div>
            <div class="control-page-intro-grid node-map-summary-grid">
              <div class="fact-card"><div class="mini-label">Nodes</div><div class="metric-caption strong">${escapeHTML(String(nodes.length))}</div><div class="metric-caption">registered</div></div>
              <div class="fact-card"><div class="mini-label">Located</div><div class="metric-caption strong">${escapeHTML(String(located.length))}</div><div class="metric-caption">${escapeHTML(String(unresolved.length))} pending</div></div>
              <div class="fact-card"><div class="mini-label">Countries</div><div class="metric-caption strong">${escapeHTML(String(countryCount(located)))}</div><div class="metric-caption">by GeoIP</div></div>
              <div class="fact-card"><div class="mini-label">Backhaul</div><div class="metric-caption strong">${escapeHTML(String(drawableLinks.length))}</div><div class="metric-caption">${escapeHTML(String(links.length))} configured</div></div>
            </div>
          </div>

          <section class="section-card node-map-card">
            <div class="node-real-map" role="img" aria-label="World map with node locations and managed backhaul">
              <div class="node-map-static-map" style="background-image:url('${escapeHTML(STATIC_MAP_URL)}')"></div>
              <div class="node-map-shade"></div>
              ${renderBackhaulOverlay(located, viewport)}
              <div class="node-map-pin-layer">${renderMarkers(located, viewport, selectedID)}</div>
              ${renderTopologyLegend(located, unresolved, drawableLinks, links)}
              ${renderSelectedNodePanel(selected, nodeLookup)}
              <div class="node-map-attribution">local static map</div>
              ${located.length ? '' : '<div class="node-map-empty">No public node coordinates are cached yet. Add a node with a public address or check the GeoIP settings.</div>'}
            </div>
          </section>

          <div class="node-map-detail-grid">
            ${renderLocationDirectory(located)}
            ${renderUnresolvedNodes(unresolved)}
          </div>
          ${renderBackhaulMatrix(links, nodeLookup, locatedLookup)}
        </section>`;

      document.querySelectorAll('.node-map-pin, .node-map-location-row').forEach((button) => {
        button.addEventListener('click', () => selectNode(button.dataset.nodeId));
      });
      document.querySelectorAll('.node-map-open-btn').forEach((button) => {
        button.addEventListener('click', () => openNode(button.dataset.nodeId));
      });
      document.querySelectorAll('.node-map-route-input').forEach((input) => {
        input.addEventListener('change', () => setRouteEnabled(input.dataset.linkId, input.checked));
      });
    }

    return { render };
  }

  window.MegaVPNNodeMapPage = { create: createNodeMapPage };
})(window);
