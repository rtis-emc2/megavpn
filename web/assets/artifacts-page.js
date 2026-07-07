(function (window) {
  'use strict';

  function createArtifactsPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      tableCard,
      statusTag,
      escapeHTML,
      formatDate,
      requestJSON,
      sendJSON,
      refresh,
      openModal,
      closeModal,
      renderActionResponse,
      normalizeInstanceServiceCode,
      toMillis,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof renderActionResponse !== 'function' ||
      typeof normalizeInstanceServiceCode !== 'function' ||
      typeof toMillis !== 'function'
    ) {
      throw new Error('MegaVPNArtifactsPage requires page dependencies');
    }

    function shareLinkURL(token) {
      if (!token) return 'n/a';
      try {
        return new URL(`/share/${token}`, state.apiBase || window.location.origin).toString();
      } catch (_) {
        return `/share/${token}`;
      }
    }

    function clientByID(clientID) {
      return (state.clients || []).find((client) => client.id === clientID) || null;
    }

    function provisionableArtifactServiceCodes() {
      return new Set(['openvpn', 'xray-core', 'wireguard', 'mtproto', 'ipsec', 'http_proxy', 'shadowsocks']);
    }

    function artifactRowsForClient(clientID) {
      return (state.artifacts || [])
        .filter((artifact) => !clientID || artifact.client_account_id === clientID)
        .sort((left, right) => toMillis(right.created_at) - toMillis(left.created_at));
    }

    function artifactDownloadURL(clientID, artifactID) {
      const path = `/api/v1/clients/${encodeURIComponent(clientID)}/artifacts/${encodeURIComponent(artifactID)}/download`;
      try {
        return new URL(path, state.apiBase || window.location.origin).toString();
      } catch (_) {
        return path;
      }
    }

    function artifactPreviewable(artifactType) {
      return ['ovpn', 'vless_url', 'wg_conf', 'mtproto_url', 'http_proxy_bundle', 'ss_url', 'ipsec_bundle'].includes(String(artifactType || '').trim());
    }

    function provisionableInstancesForExport() {
      const allowed = provisionableArtifactServiceCodes();
      return (state.instances || []).filter((instance) => {
        const serviceCode = normalizeInstanceServiceCode(instance.service_code);
        return allowed.has(serviceCode) && String(instance.status || '').toLowerCase() !== 'deleted';
      });
    }

    function clientOptions(selectedClientID = '') {
      return (state.clients || []).map((client) => `
        <option value="${escapeHTML(client.id)}"${client.id === selectedClientID ? ' selected' : ''}>
          ${escapeHTML(client.username || client.display_name || client.id)} - ${escapeHTML(client.status || 'unknown')}
        </option>`).join('');
    }

    function artifactTypeLabel(artifactType) {
      switch (String(artifactType || '').trim()) {
        case 'ovpn':
          return '.ovpn profile';
        case 'vless_url':
          return 'VLESS URL';
        case 'wg_conf':
          return 'WireGuard config';
        case 'mtproto_url':
          return 'MTProto URL';
        case 'http_proxy_bundle':
          return 'HTTP proxy bundle';
        case 'ss_url':
          return 'Shadowsocks URL';
        case 'zip_bundle':
          return 'ZIP bundle';
        case 'ipsec_bundle':
          return 'IPsec/L2TP bundle';
        default:
          return artifactType || 'artifact';
      }
    }

    function artifactOptions(clientID, selectedArtifactID = '') {
      const artifacts = artifactRowsForClient(clientID);
      if (!artifacts.length) {
        return '<option value="">No generated artifacts for this client</option>';
      }
      return artifacts.map((artifact) => {
        const accessID = artifact.service_access_id || 'shared';
        const createdAt = formatDate(artifact.created_at);
        const label = `${artifactTypeLabel(artifact.artifact_type)} - ${accessID} - ${createdAt}`;
        return `<option value="${escapeHTML(artifact.id)}"${artifact.id === selectedArtifactID ? ' selected' : ''}>${escapeHTML(label)}</option>`;
      }).join('');
    }

    function instanceOptionsForArtifactExport(selectedIDs = []) {
      const selected = new Set(selectedIDs || []);
      return provisionableInstancesForExport().map((instance) => {
        const node = (state.nodes || []).find((item) => item.id === instance.node_id);
        const label = [
          instance.name || instance.id,
          normalizeInstanceServiceCode(instance.service_code),
          node?.name || instance.node_id || 'node',
        ].join(' - ');
        return `<option value="${escapeHTML(instance.id)}"${selected.has(instance.id) ? ' selected' : ''}>${escapeHTML(label)}</option>`;
      }).join('');
    }

    function renderArtifacts() {
      setTitle('Artifacts');
      const rows = (state.artifacts || []).map((artifact) => ({
        id: artifact.id,
        client_id: artifact.client_account_id || '',
        client_name: clientByID(artifact.client_account_id)?.username || artifact.client_account_id || 'n/a',
        access: artifact.service_access_id || 'n/a',
        type: artifact.artifact_type || 'unknown',
        size: artifact.size_bytes || 0,
        path: artifact.storage_path || 'n/a',
        status: artifact.status || 'unknown',
        created: artifact.created_at,
      }));
      el('content').innerHTML = tableCard('Artifacts', rows, [
        { title: 'Type', key: 'type', render: (row) => `<span class="tag">${escapeHTML(row.type)}</span>` },
        { title: 'Client', key: 'client_name' },
        { title: 'Service Access', key: 'access' },
        { title: 'Size', key: 'size', render: (row) => escapeHTML(`${row.size} B`) },
        { title: 'Path', key: 'path', render: (row) => `<code>${escapeHTML(row.path)}</code>` },
        { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
        { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
        { title: 'Actions', key: 'id', render: (row) => `
          <div class="inline-actions compact-actions">
            ${artifactPreviewable(row.type) ? `<button class="secondary-btn artifact-preview-btn" type="button" data-client-id="${escapeHTML(row.client_id)}" data-artifact-id="${escapeHTML(row.id)}">Preview</button>` : ''}
            <button class="secondary-btn artifact-download-btn" type="button" data-client-id="${escapeHTML(row.client_id)}" data-artifact-id="${escapeHTML(row.id)}">Download</button>
            <button class="secondary-btn artifact-publish-btn" type="button" data-client-id="${escapeHTML(row.client_id)}" data-artifact-id="${escapeHTML(row.id)}">Publish link</button>
            <button class="danger-btn artifact-delete-btn" type="button" data-client-id="${escapeHTML(row.client_id)}" data-artifact-id="${escapeHTML(row.id)}">Delete</button>
          </div>` },
      ], '<button class="secondary-btn" type="button" id="artifactExportBtn">Queue export</button>');
      document.getElementById('artifactExportBtn')?.addEventListener('click', openArtifactExportModal);
      document.querySelectorAll('.artifact-preview-btn').forEach((button) => {
        button.addEventListener('click', () => previewArtifact(button.dataset.clientId, button.dataset.artifactId));
      });
      document.querySelectorAll('.artifact-download-btn').forEach((button) => {
        button.addEventListener('click', () => {
          window.open(artifactDownloadURL(button.dataset.clientId, button.dataset.artifactId), '_blank', 'noopener,noreferrer');
        });
      });
      document.querySelectorAll('.artifact-publish-btn').forEach((button) => {
        button.addEventListener('click', () => openShareLinkPublishModal(button.dataset.clientId, button.dataset.artifactId));
      });
      document.querySelectorAll('.artifact-delete-btn').forEach((button) => {
        button.addEventListener('click', () => openDeleteArtifactModal(button.dataset.clientId, button.dataset.artifactId));
      });
    }

    function renderShareLinks() {
      setTitle('Share Links');
      const rows = (state.shareLinks || []).map((link) => ({
        id: link.id,
        client: clientByID(link.client_account_id)?.username || link.client_account_id || 'n/a',
        client_id: link.client_account_id || '',
        target: `${link.target_type || 'artifact'}:${link.target_id || 'n/a'}`,
        status: link.status || 'unknown',
        expires: link.expires_at,
        downloads: link.download_count || 0,
        token: link.token || '',
        token_hint: link.token_hint || '',
        url: shareLinkURL(link.token || ''),
      }));
      el('content').innerHTML = tableCard('Share Links', rows, [
        { title: 'Client', key: 'client' },
        { title: 'Target', key: 'target' },
        { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
        { title: 'Downloads', key: 'downloads' },
        { title: 'Expires', key: 'expires', render: (row) => formatDate(row.expires) },
        { title: 'Token hint', key: 'token_hint', render: (row) => `<code>${escapeHTML(row.token_hint || 'n/a')}</code>` },
        { title: 'URL', key: 'url', render: (row) => row.url === 'n/a' ? 'one-time only' : `<code>${escapeHTML(row.url)}</code>` },
        { title: 'Actions', key: 'id', render: (row) => `<div class="inline-actions compact-actions">${row.url === 'n/a' ? '' : `<button class="secondary-btn sharelink-open-btn" type="button" data-url="${escapeHTML(row.url)}">Open</button>`}<button class="danger-btn sharelink-revoke-btn" type="button" data-client-id="${escapeHTML(row.client_id)}" data-link-id="${escapeHTML(row.id)}">Revoke</button></div>` },
      ], '<button class="secondary-btn" type="button" id="shareLinkCreateBtn">Publish share link</button>');
      document.getElementById('shareLinkCreateBtn')?.addEventListener('click', () => openShareLinkPublishModal());
      document.querySelectorAll('.sharelink-open-btn').forEach((button) => {
        button.addEventListener('click', () => {
          if (button.dataset.url && button.dataset.url !== 'n/a') window.open(button.dataset.url, '_blank', 'noopener,noreferrer');
        });
      });
      document.querySelectorAll('.sharelink-revoke-btn').forEach((button) => {
        button.addEventListener('click', () => revokeShareLinkAction(button.dataset.clientId, button.dataset.linkId));
      });
    }

    function refreshShareLinkArtifactOptions(selectClientID, selectArtifactID, artifactInfoID, selectedArtifactID = '') {
      const clientID = String(selectClientID?.value || '').trim();
      const artifactSelect = document.getElementById(selectArtifactID);
      const artifactInfo = document.getElementById(artifactInfoID);
      if (!artifactSelect || !artifactInfo) return;
      artifactSelect.innerHTML = artifactOptions(clientID, selectedArtifactID);
      const artifacts = artifactRowsForClient(clientID);
      const selectedArtifact = artifacts.find((artifact) => artifact.id === artifactSelect.value) || artifacts[0] || null;
      if (selectedArtifact && artifactSelect.value !== selectedArtifact.id) {
        artifactSelect.value = selectedArtifact.id;
      }
      artifactInfo.innerHTML = selectedArtifact
        ? `<div class="code-block">artifact_id = ${escapeHTML(selectedArtifact.id)}
type = ${escapeHTML(selectedArtifact.artifact_type || 'unknown')}
status = ${escapeHTML(selectedArtifact.status || 'unknown')}
path = ${escapeHTML(selectedArtifact.storage_path || 'n/a')}</div>`
        : '<div class="empty compact-empty">No generated artifacts for the selected client. Queue export first.</div>';
    }

    function openArtifactExportModal() {
      const defaultClientID = state.clients[0]?.id || '';
      const defaultInstances = provisionableInstancesForExport().map((instance) => instance.id);
      openModal('Queue artifact export', 'Artifacts are built through artifact.build jobs', `
        <form id="artifactExportForm" class="form-grid">
          <div class="field"><label>Client</label><select name="client_id" id="artifactExportClient">${clientOptions(defaultClientID)}</select></div>
          <div class="field"><label>Requested artifact type</label><select name="artifact_type">
            <option value="all">all supported</option>
            <option value="zip_bundle">zip_bundle</option>
            <option value="ovpn">ovpn</option>
            <option value="vless_url">vless_url</option>
            <option value="wg_conf">wg_conf</option>
            <option value="mtproto_url">mtproto_url</option>
            <option value="http_proxy_bundle">http_proxy_bundle</option>
            <option value="ipsec_bundle">ipsec_bundle</option>
            <option value="ss_url">ss_url</option>
          </select></div>
          <div class="field full"><label>Instances</label><select name="instance_ids" id="artifactExportInstances" multiple size="8">${instanceOptionsForArtifactExport(defaultInstances)}</select></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Queue export job</button></div>
        </form>
        <div id="artifactExportResult" class="form-result"></div>`);
      document.getElementById('artifactExportForm')?.addEventListener('submit', submitArtifactExport);
    }

    async function submitArtifactExport(event) {
      event.preventDefault();
      const target = document.getElementById('artifactExportResult');
      if (target) target.innerHTML = '<span class="tag warn">queueing artifact export</span>';
      try {
        const formElement = event.currentTarget;
        const form = new FormData(formElement);
        const clientID = String(form.get('client_id') || '').trim();
        const artifactType = String(form.get('artifact_type') || 'all').trim();
        const instanceIDs = Array.from(formElement.querySelector('#artifactExportInstances')?.selectedOptions || []).map((option) => option.value);
        if (!clientID) throw new Error('client is required');
        const data = await sendJSON(`/api/v1/clients/${clientID}/artifacts`, 'POST', {
          type: artifactType,
          instance_ids: instanceIDs,
        });
        if (target) target.innerHTML = renderActionResponse(data, 'Artifact export');
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    async function previewArtifact(clientID, artifactID) {
      openModal('Artifact preview', 'Authenticated generated config preview', '<div class="empty">Loading artifact...</div>', { wide: true });
      try {
        const data = await requestJSON(`/api/v1/clients/${clientID}/artifacts/${artifactID}/content`);
        el('modalBody').innerHTML = `
          <div class="card">
            <div class="mini-label">${escapeHTML(data.artifact_type || 'artifact')}</div>
            <h3>${escapeHTML(data.filename || artifactID)}</h3>
            <div class="metric-caption">${escapeHTML(String(data.size_bytes || 0))} bytes</div>
          </div>
          <textarea class="code-textarea" rows="18" readonly>${escapeHTML(data.content || '')}</textarea>
          <div class="inline-actions" style="margin-top:12px">
            <button class="secondary-btn" id="downloadArtifactPreviewBtn" type="button">Download</button>
          </div>`;
        document.getElementById('downloadArtifactPreviewBtn')?.addEventListener('click', () => {
          window.open(artifactDownloadURL(clientID, artifactID), '_blank', 'noopener,noreferrer');
        });
      } catch (err) {
        el('modalBody').innerHTML = `<div class="empty">Failed to load artifact: ${escapeHTML(err.message)}</div>`;
      }
    }

    function openDeleteArtifactModal(clientID, artifactID) {
      const artifact = (state.artifacts || []).find((item) => item.id === artifactID) || null;
      openModal('Delete config artifact', 'Remove one generated client config', `
        <p class="danger-text">This removes the selected generated config and public links that point to it. Client access remains provisioned and configs can be built again.</p>
        <div class="client-danger-summary">
          <div><span>Client</span><strong>${escapeHTML(clientByID(clientID)?.username || clientID || 'n/a')}</strong></div>
          <div><span>Type</span><strong>${escapeHTML(artifactTypeLabel(artifact?.artifact_type || 'artifact'))}</strong></div>
          <div><span>Artifact</span><strong>${escapeHTML(artifactID || 'n/a')}</strong></div>
          <div><span>Status</span><strong>${escapeHTML(artifact?.status || 'unknown')}</strong></div>
        </div>
        <div class="modal-actions">
          <button class="secondary-btn" id="cancelDeleteArtifactBtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmDeleteArtifactBtn" type="button">Delete config</button>
        </div>
        <div id="deleteArtifactResult" class="form-result"></div>`, { variant: 'danger' });
      document.getElementById('cancelDeleteArtifactBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmDeleteArtifactBtn')?.addEventListener('click', async () => {
        const target = document.getElementById('deleteArtifactResult');
        const button = document.getElementById('confirmDeleteArtifactBtn');
        if (button) {
          button.disabled = true;
          button.textContent = 'Deleting';
        }
        if (target) target.innerHTML = '<span class="tag warn">deleting config</span>';
        try {
          const data = await requestJSON(`/api/v1/clients/${clientID}/artifacts/${artifactID}`, { method: 'DELETE' });
          if (target) target.innerHTML = renderActionResponse(data, 'Config artifact deleted');
          await refresh();
          setTimeout(closeModal, 400);
        } catch (err) {
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
          if (button) {
            button.disabled = false;
            button.textContent = 'Delete config';
          }
        }
      });
    }

    function openShareLinkPublishModal(initialClientID = '', initialArtifactID = '') {
      const artifact = (state.artifacts || []).find((item) => item.id === initialArtifactID) || null;
      const defaultClientID = String(initialClientID || artifact?.client_account_id || state.clients[0]?.id || '').trim();
      const defaultArtifactID = String(initialArtifactID || '').trim();
      openModal('Publish share link', 'Bind a public URL to one generated artifact', `
        <form id="shareLinkPublishForm" class="form-grid">
          <div class="field"><label>Client</label><select name="client_id" id="shareLinkClient">${clientOptions(defaultClientID)}</select></div>
          <div class="field"><label>TTL hours</label><input name="ttl_hours" type="number" min="1" max="168" value="72" /></div>
          <div class="field full"><label>Artifact</label><select name="artifact_id" id="shareLinkArtifact"></select></div>
          <div class="field full" id="shareLinkArtifactInfo"></div>
          <div class="field full inline-actions"><button class="primary-btn" type="submit">Publish share link</button></div>
        </form>
        <div id="shareLinkPublishResult" class="form-result"></div>`);
      const clientSelect = document.getElementById('shareLinkClient');
      refreshShareLinkArtifactOptions(clientSelect, 'shareLinkArtifact', 'shareLinkArtifactInfo', defaultArtifactID);
      clientSelect?.addEventListener('change', () => refreshShareLinkArtifactOptions(clientSelect, 'shareLinkArtifact', 'shareLinkArtifactInfo'));
      document.getElementById('shareLinkPublishForm')?.addEventListener('submit', submitShareLinkPublish);
    }

    async function submitShareLinkPublish(event) {
      event.preventDefault();
      const target = document.getElementById('shareLinkPublishResult');
      if (target) target.innerHTML = '<span class="tag warn">publishing share link</span>';
      try {
        const form = new FormData(event.currentTarget);
        const clientID = String(form.get('client_id') || '').trim();
        const artifactID = String(form.get('artifact_id') || '').trim();
        const ttlHours = Number(form.get('ttl_hours') || 72);
        if (!clientID) throw new Error('client is required');
        if (!artifactID) throw new Error('artifact is required');
        const data = await sendJSON(`/api/v1/clients/${clientID}/share-links`, 'POST', {
          target_id: artifactID,
          ttl_hours: ttlHours,
        });
        if (target) {
          target.innerHTML = renderActionResponse({
            ...data,
            url: shareLinkURL(data?.token || ''),
          }, 'Share link published');
        }
        await refresh();
      } catch (err) {
        if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
      }
    }

    function revokeShareLinkAction(clientID, linkID) {
      const link = (state.shareLinks || []).find((item) => item.id === linkID) || null;
      openModal('Revoke share link', 'Disable public download access for this token', `
        <div class="code-block">client = ${escapeHTML(clientByID(clientID)?.username || clientID || 'n/a')}
link_id = ${escapeHTML(linkID || 'n/a')}
status = ${escapeHTML(link?.status || 'unknown')}
token_hint = ${escapeHTML(link?.token_hint || 'n/a')}</div>
        <div class="modal-actions">
          <button class="secondary-btn" id="cancelRevokeShareLinkBtn" type="button">Cancel</button>
          <button class="danger-btn" id="confirmRevokeShareLinkBtn" type="button">Revoke link</button>
        </div>
        <div id="revokeShareLinkResult" class="form-result"></div>`);
      document.getElementById('cancelRevokeShareLinkBtn')?.addEventListener('click', closeModal);
      document.getElementById('confirmRevokeShareLinkBtn')?.addEventListener('click', async () => {
        const target = document.getElementById('revokeShareLinkResult');
        if (target) target.innerHTML = '<span class="tag warn">revoking share link</span>';
        try {
          const data = await sendJSON(`/api/v1/clients/${clientID}/share-links/${linkID}/revoke`, 'POST', {});
          if (target) target.innerHTML = renderActionResponse(data, 'Share link revoked');
          await refresh();
        } catch (err) {
          if (target) target.innerHTML = `<span class="tag danger">${escapeHTML(err.message)}</span>`;
        }
      });
    }

    return {
      renderArtifacts,
      renderShareLinks,
      openArtifactExportModal,
      openShareLinkPublishModal,
    };
  }

  window.MegaVPNArtifactsPage = { create: createArtifactsPage };
})(window);
