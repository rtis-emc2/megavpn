(function (window) {
  'use strict';

  function createRevisionsPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      requestJSON,
      sendJSON,
      refresh,
      runInstanceAction,
      openModal,
      closeModal,
      openActionOutcomeModal,
      tableCard,
      statusTag,
      escapeHTML,
      formatDate,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof runInstanceAction !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof tableCard !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function'
    ) {
      throw new Error('MegaVPNRevisionsPage requires page dependencies');
    }

    function stableSpecValue(value) {
      if (value === null || typeof value !== 'object') return JSON.stringify(value);
      if (Array.isArray(value)) return `[${value.map((item) => stableSpecValue(item)).join(',')}]`;
      return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableSpecValue(value[key])}`).join(',')}}`;
    }

    function collectSpecEntries(value, prefix = '', out = {}) {
      if (value === null || typeof value !== 'object') {
        out[prefix || 'root'] = stableSpecValue(value);
        return out;
      }
      if (Array.isArray(value)) {
        value.forEach((item, index) => collectSpecEntries(item, `${prefix}[${index}]`, out));
        if (!value.length) out[prefix || 'root'] = '[]';
        return out;
      }
      const keys = Object.keys(value).sort();
      if (!keys.length) {
        out[prefix || 'root'] = '{}';
        return out;
      }
      keys.forEach((key) => collectSpecEntries(value[key], prefix ? `${prefix}.${key}` : key, out));
      return out;
    }

    function diffSpecSummary(currentSpec, previousSpec) {
      const currentEntries = collectSpecEntries(currentSpec || {});
      const previousEntries = collectSpecEntries(previousSpec || {});
      const keys = Array.from(new Set([...Object.keys(currentEntries), ...Object.keys(previousEntries)])).sort();
      const changed = keys.filter((key) => currentEntries[key] !== previousEntries[key]);
      return { count: changed.length, keys: changed.slice(0, 12) };
    }

    function shortID(value, size = 8) {
      const raw = String(value || '').trim();
      if (!raw) return 'n/a';
      if (raw.length <= size * 2 + 1) return raw;
      return `${raw.slice(0, size)}...${raw.slice(-size)}`;
    }

    function endpointLabel(instance) {
      const host = String(instance?.endpoint_host || '').trim();
      const port = Number(instance?.endpoint_port || 0);
      if (!host && !port) return 'n/a';
      if (!host) return String(port);
      if (!port) return host;
      return `${host}:${port}`;
    }

    function optionLabel(instance) {
      return [instance.name || instance.id, instance.service_code || 'unknown'].join(' / ');
    }

    function selectedInstance(instances) {
      const selectedID = state.revisionsInstanceID && instances.some((instance) => instance.id === state.revisionsInstanceID)
        ? state.revisionsInstanceID
        : instances[0]?.id || '';
      state.revisionsInstanceID = selectedID;
      if (selectedID) localStorage.setItem('megavpn.revisionsInstanceID', selectedID);
      else localStorage.removeItem('megavpn.revisionsInstanceID');
      return instances.find((instance) => instance.id === selectedID) || instances[0] || null;
    }

    function renderInstanceSummary(instance) {
      return `
        <div class="revision-instance-strip">
          <div class="revision-instance-main">
            <div class="mini-label">Selected instance</div>
            <strong title="${escapeHTML(instance.id || '')}">${escapeHTML(instance.name || instance.id || 'n/a')}</strong>
            <span>${escapeHTML(instance.service_code || 'unknown service')} / ${escapeHTML(instance.systemd_unit || 'no systemd unit')}</span>
          </div>
          <div class="revision-facts">
            <div class="revision-fact"><span>Status</span>${statusTag(instance.status || 'unknown')}</div>
            <div class="revision-fact"><span>Endpoint</span><strong>${escapeHTML(endpointLabel(instance))}</strong></div>
            <div class="revision-fact"><span>Node</span><strong title="${escapeHTML(instance.node_id || '')}">${escapeHTML(shortID(instance.node_id))}</strong></div>
            <div class="revision-fact"><span>Slug</span><strong>${escapeHTML(instance.slug || 'n/a')}</strong></div>
          </div>
        </div>`;
    }

    function renderShell(instances, instance) {
      const content = el('content');
      if (!content) return;
      content.innerHTML = `
        <section class="card revision-page-card">
          <div class="table-head revision-page-head">
            <div>
              <h2>Instance Revisions</h2>
              <p>Track desired-state changes, validation results and rollback points for the selected instance.</p>
            </div>
            <div class="table-tools">
              <select id="revisionsInstanceSelect">${instances.map((item) => `<option value="${escapeHTML(item.id)}"${item.id === instance.id ? ' selected' : ''}>${escapeHTML(optionLabel(item))}</option>`).join('')}</select>
            </div>
          </div>
          ${renderInstanceSummary(instance)}
          <div class="form-result revision-result" id="revisionsResult">
            <div class="empty compact-empty">Loading revision history...</div>
          </div>
        </section>`;
    }

    function bindShell() {
      const select = document.getElementById('revisionsInstanceSelect');
      if (!select) return;
      select.addEventListener('change', () => {
        state.revisionsInstanceID = select.value;
        localStorage.setItem('megavpn.revisionsInstanceID', state.revisionsInstanceID);
        render();
      });
    }

    function revisionRows(revisions) {
      return (revisions || []).map((revision, index, list) => {
        const previousRevision = list[index + 1] || null;
        const diff = diffSpecSummary(revision.spec || {}, previousRevision?.spec || {});
        const issueCount = Array.isArray(revision.validation_errors) ? revision.validation_errors.length : 0;
        const canRollback = !revision.is_current && ['validated', 'applied', 'superseded'].includes(String(revision.status || '').toLowerCase());
        return {
          revision_no: revision.revision_no,
          revision_id: revision.id,
          status: revision.status || 'unknown',
          source: revision.source || 'n/a',
          created: revision.created_at,
          applied: revision.applied_at,
          summary: `${Object.keys(revision.spec || {}).length} spec keys / ${issueCount} issues / ${diff.count} changed paths`,
          is_current: Boolean(revision.is_current),
          is_last_applied: Boolean(revision.is_last_applied),
          can_rollback: canRollback,
          diff_count: diff.count,
          diff_keys: diff.keys,
          compare_revision_no: previousRevision?.revision_no || null,
          raw: revision,
          previous: previousRevision,
        };
      });
    }

    function bindRevisionActions(instance, rows) {
      const target = document.getElementById('revisionsResult');
      if (!target) return;
      target.querySelectorAll('.revision-preview-btn').forEach((button) => {
        button.addEventListener('click', () => {
          const row = rows.find((item) => item.revision_id === button.dataset.revisionId);
          if (row) openRevisionPreview(instance, row);
        });
      });
      target.querySelectorAll('.revision-rollback-btn').forEach((button) => {
        button.addEventListener('click', () => {
          const row = rows.find((item) => item.revision_id === button.dataset.revisionId);
          if (row) openRevisionRollbackModal(instance, row);
        });
      });
    }

    function renderRevisionTable(instance, revisions) {
      const target = document.getElementById('revisionsResult');
      if (!target) return;
      const rows = revisionRows(revisions);
      target.innerHTML = tableCard('Revision Timeline', rows, [
        { title: 'Revision', key: 'revision_no', render: (row) => `<strong>#${escapeHTML(row.revision_no)}</strong>${row.is_current ? ' <span class="tag">current</span>' : ''}${row.is_last_applied ? ' <span class="tag ok">applied</span>' : ''}` },
        { title: 'Status', key: 'status', render: (row) => statusTag(row.status) },
        { title: 'Source', key: 'source', render: (row) => `<code>${escapeHTML(row.source)}</code>` },
        { title: 'Created', key: 'created', render: (row) => formatDate(row.created) },
        { title: 'Applied', key: 'applied', render: (row) => formatDate(row.applied) },
        { title: 'Summary', key: 'summary' },
        { title: 'Actions', key: 'actions', render: (row) => `
          <div class="inline-actions compact-actions">
            <button class="secondary-btn revision-preview-btn" type="button" data-revision-id="${escapeHTML(row.revision_id)}">Preview</button>
            ${row.can_rollback ? `<button class="danger-btn revision-rollback-btn" type="button" data-revision-id="${escapeHTML(row.revision_id)}">Rollback</button>` : ''}
          </div>` },
      ]);
      bindRevisionActions(instance, rows);
    }

    function renderRevisionError(err) {
      const target = document.getElementById('revisionsResult');
      if (!target) return;
      target.innerHTML = `
        <div class="revision-error">
          <div>
            <strong>Revision history unavailable</strong>
            <span>${escapeHTML(err?.message || 'Backend API did not return revision history.')}</span>
          </div>
          <button class="secondary-btn" id="revisionsRetryBtn" type="button">Retry</button>
        </div>`;
      document.getElementById('revisionsRetryBtn')?.addEventListener('click', () => render());
    }

    async function render() {
      setTitle('Revisions');
      const instances = state.instances || [];
      if (!instances.length) {
        el('content').innerHTML = '<section class="card"><h2>Instance Revisions</h2><div class="empty">No managed instances available yet.</div></section>';
        return;
      }
      const instance = selectedInstance(instances);
      renderShell(instances, instance);
      bindShell();

      const selectedInstanceID = instance.id;
      try {
        const revisions = await requestJSON(`/api/v1/instances/${selectedInstanceID}/revisions?limit=20`);
        if (state.page !== 'revisions' || state.revisionsInstanceID !== selectedInstanceID) return;
        renderRevisionTable(instance, revisions || []);
      } catch (err) {
        if (state.page !== 'revisions' || state.revisionsInstanceID !== selectedInstanceID) return;
        renderRevisionError(err);
      }
    }

    function openRevisionPreview(instance, row) {
      const revision = row.raw || {};
      const diffKeys = Array.isArray(row.diff_keys) ? row.diff_keys : [];
      const validationErrors = Array.isArray(revision.validation_errors) ? revision.validation_errors : [];
      openModal(`Revision #${row.revision_no}`, 'Instance revision preview', `
        <div class="grid cols-2">
          <div class="card">
            <div class="mini-label">Revision state</div>
            <div class="inline-actions">
              ${statusTag(revision.status || 'unknown')}
              ${row.is_current ? '<span class="tag">current</span>' : ''}
              ${row.is_last_applied ? '<span class="tag ok">applied</span>' : ''}
            </div>
            <div class="timeline" style="margin-top:14px">
              <div class="timeline-item"><strong>Source</strong><div class="timeline-meta">${escapeHTML(revision.source || 'n/a')}</div></div>
              <div class="timeline-item"><strong>Created</strong><div class="timeline-meta">${escapeHTML(formatDate(revision.created_at))}</div></div>
              <div class="timeline-item"><strong>Applied</strong><div class="timeline-meta">${escapeHTML(formatDate(revision.applied_at))}</div></div>
              <div class="timeline-item"><strong>Rendered hash</strong><div class="timeline-meta">${escapeHTML(revision.rendered_hash || 'n/a')}</div></div>
            </div>
          </div>
          <div class="card">
            <div class="mini-label">Diff summary</div>
            <div class="metric-caption">${escapeHTML(String(row.diff_count || 0))} changed paths</div>
            <div class="metric-caption">${row.compare_revision_no ? `vs revision #${escapeHTML(row.compare_revision_no)}` : 'No older revision to compare'}</div>
            ${diffKeys.length ? `<div class="code-block" style="margin-top:14px">${escapeHTML(diffKeys.slice(0, 20).join('\n'))}</div>` : '<div class="empty" style="margin-top:14px">No changed paths.</div>'}
          </div>
        </div>
        ${validationErrors.length ? `
          <section class="card">
            <div class="table-head compact-head"><h3>Validation Issues</h3><span class="tag danger">${escapeHTML(String(validationErrors.length))}</span></div>
            <div class="code-block">${escapeHTML(JSON.stringify(validationErrors, null, 2))}</div>
          </section>` : ''}
        <section class="card">
          <div class="table-head compact-head"><h3>Rendered Spec</h3><span class="tag">${escapeHTML(String(Object.keys(revision.spec || {}).length))} keys</span></div>
          <div class="code-block">${escapeHTML(JSON.stringify(revision.spec || {}, null, 2))}</div>
        </section>
        <div class="inline-actions" style="margin-top:16px">
          ${row.can_rollback ? `<button class="danger-btn" id="revisionPreviewRollbackBtn" type="button">Rollback revision</button>` : ''}
          ${row.can_rollback ? `<button class="primary-btn" id="revisionPreviewRollbackApplyBtn" type="button">Rollback and apply</button>` : ''}
          <button class="secondary-btn" id="revisionPreviewCloseBtn" type="button">Close</button>
        </div>`, { wide: true });
      document.getElementById('revisionPreviewCloseBtn')?.addEventListener('click', closeModal);
      document.getElementById('revisionPreviewRollbackBtn')?.addEventListener('click', (event) => submitRevisionRollback(instance, row, false, event.currentTarget));
      document.getElementById('revisionPreviewRollbackApplyBtn')?.addEventListener('click', (event) => submitRevisionRollback(instance, row, true, event.currentTarget));
    }

    function openRevisionRollbackModal(instance, row) {
      openModal(`Rollback to revision #${row.revision_no}`, 'Instance rollback', `
        <div class="form-grid">
          <div class="field full">
            <div class="code-block">
              <div style="margin-bottom:8px">${statusTag(row.status || 'unknown')}</div>
              <div><strong>${escapeHTML(instance.name || instance.id)}</strong></div>
              <div style="margin-top:8px">Create a new current revision from revision #${escapeHTML(row.revision_no)}${row.compare_revision_no ? ' and keep apply as a separate step' : ''}.</div>
            </div>
          </div>
          <div class="field"><label>Changed paths</label><div class="code-block">${escapeHTML(String(row.diff_count || 0))}</div></div>
          <div class="field"><label>Compared to</label><div class="code-block">${escapeHTML(row.compare_revision_no ? `#${row.compare_revision_no}` : 'n/a')}</div></div>
          <div class="field full inline-actions">
            <button class="danger-btn" id="confirmRollbackRevisionBtn" type="button">Rollback revision</button>
            <button class="primary-btn" id="confirmRollbackApplyRevisionBtn" type="button">Rollback and apply</button>
          </div>
        </div>`, { wide: true });
      document.getElementById('confirmRollbackRevisionBtn')?.addEventListener('click', (event) => submitRevisionRollback(instance, row, false, event.currentTarget));
      document.getElementById('confirmRollbackApplyRevisionBtn')?.addEventListener('click', (event) => submitRevisionRollback(instance, row, true, event.currentTarget));
    }

    async function submitRevisionRollback(instance, row, applyAfterSave, button) {
      if (button) {
        button.disabled = true;
        button.dataset.originalLabel = button.dataset.originalLabel || button.textContent || '';
        button.textContent = applyAfterSave ? 'Rolling back...' : 'Rollback...';
      }
      try {
        const data = await sendJSON(`/api/v1/instances/${instance.id}/rollback`, 'POST', { revision_id: row.revision_id });
        await refresh();
        if (applyAfterSave && data?.can_apply) {
          openModal(`Rollback + apply: ${instance.name}`, 'Instance rollback apply', '<div id="rollbackApplyJobResult" class="form-result"><span class="tag warn">queueing apply</span></div>', { wide: true });
          await runInstanceAction(instance.id, 'apply', 'rollbackApplyJobResult');
          return;
        }
        closeModal();
        openActionOutcomeModal(
          `Rollback created: ${instance.name}`,
          'Instance revision rollback',
          data?.can_apply ? 'succeeded' : 'warning',
          String(data?.message || 'Rollback revision created.'),
          [
            { label: 'Target revision', value: `#${row.revision_no}` },
            { label: 'New revision', value: data?.revision?.revision_no ? `#${data.revision.revision_no}` : 'n/a' },
            { label: 'Status', value: data?.revision?.status || 'unknown' },
            { label: 'Can apply', value: data?.can_apply ? 'yes' : 'no' },
            { label: 'Validation issues', value: data?.issue_count ?? 0 },
          ],
        );
      } catch (err) {
        openActionOutcomeModal('Rollback failed', 'Instance revision rollback', 'failed', err.message || 'Rollback failed');
      } finally {
        if (button) {
          button.disabled = false;
          button.textContent = button.dataset.originalLabel || (applyAfterSave ? 'Rollback and apply' : 'Rollback revision');
        }
      }
    }

    return { render };
  }

  window.MegaVPNRevisionsPage = { create: createRevisionsPage };
})(window);
