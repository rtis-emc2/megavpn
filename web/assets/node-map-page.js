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

    const MAP_WIDTH = 1000;
    const MAP_HEIGHT = 520;

    function finiteNumber(value) {
      const num = Number(value);
      return Number.isFinite(num) ? num : null;
    }

    function safeClassToken(value) {
      return String(value || 'unknown').toLowerCase().replace(/[^a-z0-9_-]/g, '-') || 'unknown';
    }

    function nodeLocation(node) {
      const latitude = finiteNumber(node?.latitude ?? node?.lat ?? node?.location?.latitude ?? node?.geo?.latitude ?? node?.geo?.lat);
      const longitude = finiteNumber(node?.longitude ?? node?.lon ?? node?.lng ?? node?.location?.longitude ?? node?.geo?.longitude ?? node?.geo?.lon ?? node?.geo?.lng);
      const accuracyRadiusKM = finiteNumber(node?.accuracy_radius_km ?? node?.accuracy_km ?? node?.location?.accuracy_radius_km ?? node?.location?.accuracy_km ?? node?.geo?.accuracy_radius_km ?? node?.geo?.accuracy_km);
      if (latitude == null || longitude == null) return null;
      if (latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180) return null;
      return {
        latitude,
        longitude,
        accuracyRadiusKM: accuracyRadiusKM != null && accuracyRadiusKM >= 0 ? accuracyRadiusKM : null,
      };
    }

    function projectPoint(location) {
      return {
        x: ((location.longitude + 180) / 360) * MAP_WIDTH,
        y: ((90 - location.latitude) / 180) * MAP_HEIGHT,
      };
    }

    function nodeLabel(node) {
      const location = String(node.location_label || node.location?.label || node.geo?.label || '').trim();
      return location || node.address || 'location not set';
    }

    function accuracyLabel(location) {
      const radius = finiteNumber(location?.accuracyRadiusKM);
      if (radius == null) return 'not specified';
      if (radius <= 0) return 'exact point';
      if (radius < 1) return '< 1 km';
      if (radius < 10) return `${radius.toFixed(1)} km`;
      return `${Math.round(radius)} km`;
    }

    function nodeByID(nodeID) {
      const id = String(nodeID || '').trim();
      return (state.nodes || []).find((node) => String(node.id || '').trim() === id) || null;
    }

    function locatedNodes() {
      return (Array.isArray(state.nodes) ? state.nodes : [])
        .map((node) => ({ node, location: nodeLocation(node) }))
        .filter((item) => item.location);
    }

    function missingLocationNodes() {
      return (Array.isArray(state.nodes) ? state.nodes : []).filter((node) => !nodeLocation(node));
    }

    function roleClass(node) {
      return String(node?.role || '').toLowerCase() === 'ingress' ? 'ingress' : 'egress';
    }

    function renderGraticule() {
      const vertical = [];
      for (let lon = -150; lon <= 150; lon += 30) {
        const x = ((lon + 180) / 360) * MAP_WIDTH;
        vertical.push(`<line x1="${x.toFixed(2)}" y1="0" x2="${x.toFixed(2)}" y2="${MAP_HEIGHT}" />`);
      }
      const horizontal = [];
      for (let lat = -60; lat <= 60; lat += 30) {
        const y = ((90 - lat) / 180) * MAP_HEIGHT;
        horizontal.push(`<line x1="0" y1="${y.toFixed(2)}" x2="${MAP_WIDTH}" y2="${y.toFixed(2)}" />`);
      }
      return [...vertical, ...horizontal].join('');
    }

    function renderContinents() {
      return `
        <path d="M95 142 C122 104 178 90 226 109 C267 124 271 171 238 196 C213 215 211 257 176 272 C137 288 83 256 69 214 C59 184 72 162 95 142 Z" />
        <path d="M209 268 C249 259 294 296 291 346 C288 400 238 454 198 438 C168 426 164 385 178 351 C190 323 179 287 209 268 Z" />
        <path d="M418 118 C485 82 567 93 627 131 C679 164 692 226 653 258 C615 289 559 259 514 276 C470 292 409 278 385 238 C361 198 374 142 418 118 Z" />
        <path d="M493 286 C535 272 596 293 617 344 C641 400 603 468 550 461 C501 455 468 399 472 351 C474 324 474 299 493 286 Z" />
        <path d="M651 186 C696 142 762 127 825 151 C872 169 900 212 887 252 C873 294 814 303 776 287 C739 271 708 303 673 282 C640 262 625 211 651 186 Z" />
        <path d="M754 326 C795 305 855 318 884 354 C915 392 888 440 836 437 C789 435 744 394 754 326 Z" />
        <path d="M350 424 C421 409 497 414 566 434 C493 470 416 470 350 424 Z" />`;
    }

    function renderBackhaulLines() {
      const located = new Map(locatedNodes().map((item) => [String(item.node.id), item]));
      return (Array.isArray(state.backhaulLinks) ? state.backhaulLinks : [])
        .filter((link) => String(link.status || '').toLowerCase() !== 'deleted')
        .map((link) => {
          const ingress = located.get(String(link.ingress_node_id || ''));
          const egress = located.get(String(link.egress_node_id || ''));
          if (!ingress || !egress) return '';
          const a = projectPoint(ingress.location);
          const b = projectPoint(egress.location);
          const status = safeClassToken(link.status || 'unknown');
          return `<line class="map-link ${status}" x1="${a.x.toFixed(2)}" y1="${a.y.toFixed(2)}" x2="${b.x.toFixed(2)}" y2="${b.y.toFixed(2)}"><title>${escapeHTML(link.name || 'backhaul')} · ${escapeHTML(status)}</title></line>`;
        })
        .join('');
    }

    function renderAccuracyZones() {
      return locatedNodes().map(({ node, location }) => {
        const radiusKM = finiteNumber(location.accuracyRadiusKM);
        if (radiusKM == null || radiusKM <= 0) return '';
        const point = projectPoint(location);
        const latitudeRadians = location.latitude * Math.PI / 180;
        const latitudeCosine = Math.max(0.15, Math.abs(Math.cos(latitudeRadians)));
        const rx = Math.min(MAP_WIDTH, Math.max(3, (radiusKM / (111.32 * latitudeCosine)) * (MAP_WIDTH / 360)));
        const ry = Math.min(MAP_HEIGHT, Math.max(3, (radiusKM / 111.32) * (MAP_HEIGHT / 180)));
        return `<ellipse class="map-accuracy-zone ${roleClass(node)}" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" rx="${rx.toFixed(2)}" ry="${ry.toFixed(2)}"><title>${escapeHTML(node.name || 'node')} · accuracy radius ${escapeHTML(accuracyLabel(location))}</title></ellipse>`;
      }).join('');
    }

    function renderNodeMarkers() {
      return locatedNodes().map(({ node, location }) => {
        const point = projectPoint(location);
        const status = safeClassToken(node.status || 'unknown');
        return `
          <button class="node-map-marker ${roleClass(node)} ${status}" style="left:${(point.x / MAP_WIDTH * 100).toFixed(3)}%;top:${(point.y / MAP_HEIGHT * 100).toFixed(3)}%" type="button" data-node-id="${escapeHTML(node.id)}" title="${escapeHTML(node.name || 'node')} · ${escapeHTML(accuracyLabel(location))}">
            <span class="node-map-dot"></span>
            <span class="node-map-label">${escapeHTML(node.name || 'node')}</span>
          </button>`;
      }).join('');
    }

    function renderLocatedRows(items) {
      if (!items.length) return '<tr><td colspan="7"><div class="empty compact-empty">No nodes with coordinates yet.</div></td></tr>';
      return items.map(({ node, location }) => `
        <tr>
          <td><strong>${escapeHTML(node.name || 'node')}</strong><div class="metric-caption">${escapeHTML(node.id || '')}</div></td>
          <td>${statusTag(node.role || 'egress')}</td>
          <td>${statusTag(node.status || 'unknown')}</td>
          <td>${escapeHTML(nodeLabel(node))}</td>
          <td><code>${escapeHTML(location.latitude.toFixed(6))}, ${escapeHTML(location.longitude.toFixed(6))}</code></td>
          <td>${statusTag(location.accuracyRadiusKM > 0 ? `± ${accuracyLabel(location)}` : accuracyLabel(location))}</td>
          <td><button class="secondary-btn node-map-open-btn" type="button" data-node-id="${escapeHTML(node.id)}">Open node</button></td>
        </tr>`).join('');
    }

    function renderMissingRows(items) {
      if (!items.length) return '<tr><td colspan="4"><div class="empty compact-empty">All active nodes have map coordinates.</div></td></tr>';
      return items.map((node) => `
        <tr>
          <td><strong>${escapeHTML(node.name || 'node')}</strong><div class="metric-caption">${escapeHTML(node.id || '')}</div></td>
          <td>${escapeHTML(node.address || 'n/a')}</td>
          <td>${statusTag(node.role || 'egress')}</td>
          <td><button class="secondary-btn node-map-open-btn" type="button" data-node-id="${escapeHTML(node.id)}">Set location</button></td>
        </tr>`).join('');
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
      const missing = missingLocationNodes();
      const approximate = located.filter((item) => Number(item.location.accuracyRadiusKM || 0) > 0);
      const linksWithCoordinates = (Array.isArray(state.backhaulLinks) ? state.backhaulLinks : []).filter((link) => {
        const ingress = nodeByID(link.ingress_node_id);
        const egress = nodeByID(link.egress_node_id);
        return nodeLocation(ingress) && nodeLocation(egress) && String(link.status || '').toLowerCase() !== 'deleted';
      });

      el('content').innerHTML = `
        <section class="control-page-shell node-map-page">
          <div class="control-page-intro node-map-intro">
            <div>
              <h2>Global node topology</h2>
              <p>World map view for node placement and backhaul topology. Coordinates are stored in the node profile; no external geolocation service is used.</p>
            </div>
            <div class="control-page-intro-grid">
              <div class="fact-card"><div class="mini-label">Nodes</div><div class="metric-caption strong">${escapeHTML(String(nodes.length))}</div><div class="metric-caption">active records</div></div>
              <div class="fact-card"><div class="mini-label">Mapped</div><div class="metric-caption strong">${escapeHTML(String(located.length))}</div><div class="metric-caption">${escapeHTML(String(missing.length))} missing coordinates</div></div>
              <div class="fact-card"><div class="mini-label">Approximate</div><div class="metric-caption strong">${escapeHTML(String(approximate.length))}</div><div class="metric-caption">with accuracy radius</div></div>
              <div class="fact-card"><div class="mini-label">Backhaul</div><div class="metric-caption strong">${escapeHTML(String(linksWithCoordinates.length))}</div><div class="metric-caption">drawable links</div></div>
            </div>
          </div>

          <section class="section-card node-map-card">
            <div class="node-map-canvas">
              <svg class="node-world-map" viewBox="0 0 ${MAP_WIDTH} ${MAP_HEIGHT}" role="img" aria-label="World map with node locations">
                <rect class="map-ocean" x="0" y="0" width="${MAP_WIDTH}" height="${MAP_HEIGHT}" rx="18" />
                <g class="map-graticule">${renderGraticule()}</g>
                <g class="map-land">${renderContinents()}</g>
                <g class="map-accuracy">${renderAccuracyZones()}</g>
                <g class="map-backhaul">${renderBackhaulLines()}</g>
              </svg>
              <div class="node-map-marker-layer">${renderNodeMarkers()}</div>
              ${located.length ? '' : '<div class="node-map-empty">No node coordinates yet. Open a node profile and fill latitude and longitude.</div>'}
            </div>
            <div class="node-map-legend">
              <span><i class="legend-dot ingress"></i> ingress</span>
              <span><i class="legend-dot egress"></i> egress</span>
              <span><i class="legend-area"></i> accuracy radius</span>
              <span><i class="legend-line"></i> backhaul link</span>
            </div>
          </section>

          <section class="table-card">
            <div class="table-head">
              <h2>Mapped nodes</h2>
              <div class="table-tools"><span class="tag">${escapeHTML(String(located.length))} with coordinates</span></div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Node</th><th>Role</th><th>Status</th><th>Location</th><th>Coordinates</th><th>Accuracy</th><th>Actions</th></tr></thead>
                <tbody>${renderLocatedRows(located)}</tbody>
              </table>
            </div>
          </section>

          <section class="table-card">
            <div class="table-head">
              <h2>Needs location</h2>
              <div class="table-tools"><span class="tag warn">${escapeHTML(String(missing.length))} missing</span></div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Node</th><th>Address</th><th>Role</th><th>Actions</th></tr></thead>
                <tbody>${renderMissingRows(missing)}</tbody>
              </table>
            </div>
          </section>
        </section>`;

      document.querySelectorAll('.node-map-open-btn, .node-map-marker').forEach((button) => {
        button.addEventListener('click', () => openNode(button.dataset.nodeId));
      });
    }

    return { render };
  }

  window.MegaVPNNodeMapPage = { create: createNodeMapPage };
})(window);
