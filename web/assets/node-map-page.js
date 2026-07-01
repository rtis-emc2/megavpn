(function (window) {
  'use strict';

  function createNodeMapPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      setPage,
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

    const TILE_SIZE = 256;
    const TILE_URL = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
    const VIEW_WIDTH = 1100;
    const VIEW_HEIGHT = 560;

    function finiteNumber(value) {
      const num = Number(value);
      return Number.isFinite(num) ? num : null;
    }

    function safeClassToken(value) {
      return String(value || 'unknown').toLowerCase().replace(/[^a-z0-9_-]/g, '-') || 'unknown';
    }

    function nodeLocation(node) {
      const latitude = finiteNumber(node?.latitude);
      const longitude = finiteNumber(node?.longitude);
      if (latitude == null || longitude == null) return null;
      if (latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180) return null;
      return { latitude, longitude };
    }

    function locatedNodes() {
      return (Array.isArray(state.nodes) ? state.nodes : [])
        .map((node) => ({ node, location: nodeLocation(node) }))
        .filter((item) => item.location);
    }

    function nodesByID() {
      return new Map((Array.isArray(state.nodes) ? state.nodes : [])
        .map((node) => [String(node.id || ''), node])
        .filter(([id]) => id));
    }

    function unresolvedNodes() {
      return (Array.isArray(state.nodes) ? state.nodes : []).filter((node) => !nodeLocation(node));
    }

    function geoStatus(node) {
      return String(node?.geoip_status || (nodeLocation(node) ? 'resolved' : 'pending')).trim() || 'pending';
    }

    function countryLabel(node) {
      return String(node.geoip_country_name || node.geoip_country_code || '').trim() || 'unknown country';
    }

    function locationLabel(node) {
      const parts = [
        node.geoip_city,
        node.geoip_region,
        node.geoip_country_name || node.geoip_country_code,
      ].map((part) => String(part || '').trim()).filter(Boolean);
      return parts.length ? parts.join(', ') : (String(node.location_label || '').trim() || 'location pending');
    }

    function ownerLabel(node) {
      const org = String(node.geoip_org || '').trim();
      const asn = String(node.geoip_asn || '').trim();
      if (org && asn && !org.toLowerCase().includes(asn.toLowerCase())) return `${asn} · ${org}`;
      return org || asn || 'provider pending';
    }

    function geoSourceLabel(node) {
      const provider = String(node.geoip_provider || '').trim();
      const ip = String(node.geoip_ip || '').trim();
      if (provider && ip) return `${provider} · ${ip}`;
      return provider || ip || 'pending';
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
      return String(transport?.status || link?.status || 'unknown').trim().toLowerCase() || 'unknown';
    }

    function backhaulLabel(link) {
      const transport = selectedTransport(link);
      const driver = String(transport?.driver || link?.desired_driver || '').trim();
      const status = backhaulStatus(link);
      const metric = Number(link?.route_metric || 0);
      return [
        String(link?.name || 'backhaul').trim(),
        driver,
        status,
        metric > 0 ? `metric ${metric}` : '',
      ].filter(Boolean).join(' · ');
    }

    function activeBackhaulLinks() {
      return (Array.isArray(state.backhaulLinks) ? state.backhaulLinks : [])
        .filter((link) => String(link.status || '').toLowerCase() !== 'deleted');
    }

    function drawableBackhaulLinks(items) {
      const located = new Map(items.map((item) => [String(item.node.id || ''), item]));
      return activeBackhaulLinks()
        .map((link) => ({
          link,
          ingress: located.get(String(link.ingress_node_id || '')),
          egress: located.get(String(link.egress_node_id || '')),
        }))
        .filter((item) => item.ingress && item.egress);
    }

    function tileURL(z, x, y) {
      return TILE_URL
        .replace('{z}', encodeURIComponent(String(z)))
        .replace('{x}', encodeURIComponent(String(x)))
        .replace('{y}', encodeURIComponent(String(y)));
    }

    function clampLatitude(lat) {
      return Math.max(-85.05112878, Math.min(85.05112878, lat));
    }

    function project(location, zoom) {
      const scale = TILE_SIZE * Math.pow(2, zoom);
      const lat = clampLatitude(location.latitude);
      const sin = Math.sin((lat * Math.PI) / 180);
      return {
        x: ((location.longitude + 180) / 360) * scale,
        y: (0.5 - Math.log((1 + sin) / (1 - sin)) / (4 * Math.PI)) * scale,
      };
    }

    function mapViewport(items) {
      if (!items.length) {
        return {
          zoom: 2,
          center: project({ latitude: 20, longitude: 0 }, 2),
        };
      }
      const lats = items.map((item) => item.location.latitude);
      const lons = items.map((item) => item.location.longitude);
      const latSpan = Math.max(...lats) - Math.min(...lats);
      const lonSpan = Math.max(...lons) - Math.min(...lons);
      let zoom = 2;
      if (items.length === 1 || (latSpan <= 16 && lonSpan <= 16)) zoom = 4;
      else if (latSpan <= 42 && lonSpan <= 70) zoom = 3;
      const centerLocation = {
        latitude: lats.reduce((sum, value) => sum + value, 0) / lats.length,
        longitude: lons.reduce((sum, value) => sum + value, 0) / lons.length,
      };
      return { zoom, center: project(centerLocation, zoom) };
    }

    function renderTiles(viewport) {
      const { zoom, center } = viewport;
      const tilesPerAxis = Math.pow(2, zoom);
      const left = center.x - VIEW_WIDTH / 2;
      const top = center.y - VIEW_HEIGHT / 2;
      const startX = Math.floor(left / TILE_SIZE) - 1;
      const endX = Math.ceil((left + VIEW_WIDTH) / TILE_SIZE) + 1;
      const startY = Math.max(0, Math.floor(top / TILE_SIZE) - 1);
      const endY = Math.min(tilesPerAxis - 1, Math.ceil((top + VIEW_HEIGHT) / TILE_SIZE) + 1);
      const tiles = [];
      for (let y = startY; y <= endY; y += 1) {
        for (let x = startX; x <= endX; x += 1) {
          const wrappedX = ((x % tilesPerAxis) + tilesPerAxis) % tilesPerAxis;
          const tileLeft = ((x * TILE_SIZE - left) / VIEW_WIDTH) * 100;
          const tileTop = ((y * TILE_SIZE - top) / VIEW_HEIGHT) * 100;
          const tileWidth = (TILE_SIZE / VIEW_WIDTH) * 100;
          const tileHeight = (TILE_SIZE / VIEW_HEIGHT) * 100;
          tiles.push(`
            <img class="node-map-tile"
                 src="${escapeHTML(tileURL(zoom, wrappedX, y))}"
                 loading="lazy"
                 alt=""
                 style="left:${tileLeft.toFixed(3)}%;top:${tileTop.toFixed(3)}%;width:${tileWidth.toFixed(3)}%;height:${tileHeight.toFixed(3)}%" />`);
        }
      }
      return tiles.join('');
    }

    function markerPosition(location, viewport) {
      const point = project(location, viewport.zoom);
      const left = viewport.center.x - VIEW_WIDTH / 2;
      const top = viewport.center.y - VIEW_HEIGHT / 2;
      return {
        x: ((point.x - left) / VIEW_WIDTH) * 100,
        y: ((point.y - top) / VIEW_HEIGHT) * 100,
      };
    }

    function renderMarkers(items, viewport) {
      return items.map(({ node, location }) => {
        const point = markerPosition(location, viewport);
        const status = safeClassToken(node.status || geoStatus(node));
        const title = `${node.name || 'node'} · ${locationLabel(node)} · ${ownerLabel(node)}`;
        return `
          <button class="node-map-pin ${status}" type="button" data-node-id="${escapeHTML(node.id || '')}" style="left:${point.x.toFixed(3)}%;top:${point.y.toFixed(3)}%" title="${escapeHTML(title)}">
            <span class="node-map-pin-head"></span>
            <span class="node-map-pin-label">${escapeHTML(node.name || 'node')}</span>
          </button>`;
      }).join('');
    }

    function renderBackhaulOverlay(items, viewport) {
      const lines = drawableBackhaulLinks(items).map(({ link, ingress, egress }) => {
        const a = markerPosition(ingress.location, viewport);
        const b = markerPosition(egress.location, viewport);
        const status = safeClassToken(backhaulStatus(link));
        const title = backhaulLabel(link);
        const midX = (a.x + b.x) / 2;
        const midY = (a.y + b.y) / 2;
        return `
          <g class="node-map-backhaul-link ${status}">
            <line x1="${a.x.toFixed(3)}" y1="${a.y.toFixed(3)}" x2="${b.x.toFixed(3)}" y2="${b.y.toFixed(3)}">
              <title>${escapeHTML(title)}</title>
            </line>
            <circle cx="${midX.toFixed(3)}" cy="${midY.toFixed(3)}" r="1.15">
              <title>${escapeHTML(title)}</title>
            </circle>
          </g>`;
      }).join('');
      if (!lines) return '';
      return `<svg class="node-map-backhaul-layer" viewBox="0 0 100 100" preserveAspectRatio="none" aria-label="Managed backhaul links">${lines}</svg>`;
    }

    function countryCount(items) {
      return new Set(items.map(({ node }) => String(node.geoip_country_code || node.geoip_country_name || '').trim()).filter(Boolean)).size;
    }

    function nodeBackhaulSummary(node, links, nodeLookup) {
      const nodeID = String(node.id || '');
      const related = links.filter((link) => String(link.ingress_node_id || '') === nodeID || String(link.egress_node_id || '') === nodeID);
      if (!related.length) return 'none';
      return related.slice(0, 3).map((link) => {
        const isIngress = String(link.ingress_node_id || '') === nodeID;
        const peerID = isIngress ? link.egress_node_id : link.ingress_node_id;
        const peer = nodeLookup.get(String(peerID || ''));
        const direction = isIngress ? 'to' : 'from';
        const transport = selectedTransport(link);
        const driver = String(transport?.driver || link?.desired_driver || '').trim();
        const metric = Number(link?.route_metric || 0);
        return [
          `${direction} ${peer?.name || 'node'}`,
          driver,
          metric > 0 ? `metric ${metric}` : '',
          backhaulStatus(link),
        ].filter(Boolean).join(' · ');
      }).join('; ');
    }

    function renderBackhaulDirectory(links, nodeLookup) {
      if (!links.length) {
        return `
          <section class="section-card node-map-backhaul-directory">
            <div class="section-head">
              <div>
                <h2>Backhaul topology</h2>
                <p>No managed backhaul links are configured.</p>
              </div>
            </div>
          </section>`;
      }
      const rows = links.map((link) => {
        const transport = selectedTransport(link);
        const ingress = nodeLookup.get(String(link.ingress_node_id || ''));
        const egress = nodeLookup.get(String(link.egress_node_id || ''));
        const status = backhaulStatus(link);
        const driver = String(transport?.driver || link?.desired_driver || 'driver pending').trim();
        const endpoint = String(transport?.endpoint || '').trim();
        const routeMetric = Number(link?.route_metric || 0);
        const drawable = nodeLocation(ingress) && nodeLocation(egress);
        return `
          <div class="node-map-backhaul-row">
            <div class="node-map-backhaul-route">
              <strong>${escapeHTML(link.name || 'backhaul')}</strong>
              <span>${escapeHTML(ingress?.name || 'ingress pending')} -> ${escapeHTML(egress?.name || 'egress pending')}</span>
            </div>
            <div class="node-map-backhaul-meta">
              ${statusTag(driver)}
              ${routeMetric > 0 ? statusTag(`metric ${routeMetric}`) : ''}
              ${statusTag(drawable ? 'on map' : 'pending geoip')}
            </div>
            <div class="node-map-backhaul-state">
              ${statusTag(status)}
              ${endpoint ? `<span class="muted-mono">${escapeHTML(endpoint)}</span>` : ''}
            </div>
          </div>`;
      }).join('');
      return `
        <section class="section-card node-map-backhaul-directory">
          <div class="section-head">
            <div>
              <h2>Backhaul topology</h2>
              <p>Ingress-to-egress managed links shown on the map when both endpoint nodes have GeoIP coordinates.</p>
            </div>
          </div>
          <div class="node-map-backhaul-list">${rows}</div>
        </section>`;
    }

    function renderNodeCards(nodes) {
      if (!nodes.length) {
        return '<div class="node-map-empty-state">No nodes registered yet.</div>';
      }
      const links = activeBackhaulLinks();
      const nodeLookup = nodesByID();
      return nodes.map((node) => {
        const loc = nodeLocation(node);
        const coords = loc ? `${loc.latitude.toFixed(4)}, ${loc.longitude.toFixed(4)}` : 'pending';
        const issue = String(node.geoip_error || '').trim();
        const relatedBackhaul = nodeBackhaulSummary(node, links, nodeLookup);
        return `
          <article class="node-map-node-card">
            <div class="node-map-node-card-main">
              <div>
                <h3>${escapeHTML(node.name || 'node')}</h3>
                <p>${escapeHTML(node.address || 'address pending')}</p>
              </div>
              <div class="node-map-node-tags">
                ${statusTag(node.role || 'node')}
                ${statusTag(geoStatus(node))}
              </div>
            </div>
            <div class="node-map-node-facts">
              <div><span>Location</span><strong>${escapeHTML(locationLabel(node))}</strong></div>
              <div><span>Country</span><strong>${escapeHTML(countryLabel(node))}</strong></div>
              <div><span>Network owner</span><strong>${escapeHTML(ownerLabel(node))}</strong></div>
              <div><span>GeoIP source</span><strong>${escapeHTML(geoSourceLabel(node))}</strong></div>
              <div><span>Coordinates</span><strong>${escapeHTML(coords)}</strong></div>
              <div><span>Backhaul</span><strong>${escapeHTML(relatedBackhaul)}</strong></div>
            </div>
            ${issue ? `<div class="node-map-node-issue">${escapeHTML(issue)}</div>` : ''}
            <div class="node-map-node-actions">
              <button class="secondary-btn node-map-open-btn" type="button" data-node-id="${escapeHTML(node.id || '')}">Open node</button>
            </div>
          </article>`;
      }).join('');
    }

    function openNode(nodeID) {
      if (!nodeID) return;
      state.nodeManageID = nodeID;
      state.nodeManageData = null;
      setPage('nodeManage');
    }

    function render() {
      setTitle('Node Map');
      const nodes = Array.isArray(state.nodes) ? state.nodes : [];
      const located = locatedNodes();
      const unresolved = unresolvedNodes();
      const backhaulLinks = activeBackhaulLinks();
      const drawableLinks = drawableBackhaulLinks(located);
      const nodeLookup = nodesByID();
      const viewport = mapViewport(located);

      el('content').innerHTML = `
        <section class="control-page-shell node-map-page">
          <div class="control-page-intro node-map-intro">
            <div>
              <h2>Global node map</h2>
              <p>Node placement is resolved automatically from the public node address. The map shows approximate country, city and network owner data from GeoIP.</p>
            </div>
            <div class="control-page-intro-grid">
              <div class="fact-card"><div class="mini-label">Nodes</div><div class="metric-caption strong">${escapeHTML(String(nodes.length))}</div><div class="metric-caption">registered</div></div>
              <div class="fact-card"><div class="mini-label">Located</div><div class="metric-caption strong">${escapeHTML(String(located.length))}</div><div class="metric-caption">${escapeHTML(String(unresolved.length))} pending</div></div>
              <div class="fact-card"><div class="mini-label">Countries</div><div class="metric-caption strong">${escapeHTML(String(countryCount(located)))}</div><div class="metric-caption">resolved by IP</div></div>
              <div class="fact-card"><div class="mini-label">Backhaul</div><div class="metric-caption strong">${escapeHTML(String(drawableLinks.length))}</div><div class="metric-caption">${escapeHTML(String(backhaulLinks.length))} configured</div></div>
            </div>
          </div>

          <section class="section-card node-map-card">
            <div class="node-real-map" role="img" aria-label="World map with node locations">
              <div class="node-map-tile-layer">${renderTiles(viewport)}</div>
              <div class="node-map-shade"></div>
              ${renderBackhaulOverlay(located, viewport)}
              <div class="node-map-pin-layer">${renderMarkers(located, viewport)}</div>
              ${located.length ? '' : '<div class="node-map-empty">GeoIP coordinates are not available yet. Public node addresses are resolved automatically by the API.</div>'}
            </div>
          </section>

          ${renderBackhaulDirectory(backhaulLinks, nodeLookup)}

          <section class="section-card node-map-directory">
            <div class="section-head">
              <div>
                <h2>Node locations</h2>
                <p>Compact inventory of resolved and pending GeoIP data.</p>
              </div>
            </div>
            <div class="node-map-node-list">${renderNodeCards(nodes)}</div>
          </section>
        </section>`;

      document.querySelectorAll('.node-map-open-btn, .node-map-pin').forEach((button) => {
        button.addEventListener('click', () => openNode(button.dataset.nodeId));
      });
    }

    return { render };
  }

  window.MegaVPNNodeMapPage = { createNodeMapPage };
})(window);
