(function (window) {
  'use strict';

  function createCertificateWorkflows(ctx = {}) {
    const {
      state,
      domainUI,
      requestJSON,
      sendJSON,
      refresh,
      openModal,
      closeModal,
      openActionOutcomeModal,
      setSubmitBusy,
      statusTag,
      escapeHTML,
    } = ctx;
    if (
      !state ||
      !domainUI ||
      typeof requestJSON !== 'function' ||
      typeof sendJSON !== 'function' ||
      typeof refresh !== 'function' ||
      typeof openModal !== 'function' ||
      typeof closeModal !== 'function' ||
      typeof openActionOutcomeModal !== 'function' ||
      typeof setSubmitBusy !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function'
    ) {
      throw new Error('MegaVPNCertificateWorkflows requires workflow dependencies');
    }

    const {
      activeManagedAuthorities,
      authorityCertificateOptions,
      certificateDisplayStatus,
      certificateExpiryCaption,
      certificatePrimaryLabel,
      certificateUsageCaption,
    } = domainUI;
    if (
      typeof activeManagedAuthorities !== 'function' ||
      typeof authorityCertificateOptions !== 'function' ||
      typeof certificateDisplayStatus !== 'function' ||
      typeof certificateExpiryCaption !== 'function' ||
      typeof certificatePrimaryLabel !== 'function' ||
      typeof certificateUsageCaption !== 'function'
    ) {
      throw new Error('MegaVPNCertificateWorkflows requires certificate domain helpers');
    }

    let certificateImportPreviewSeq = 0;

    function finishCertificateAction(form, data, config) {
      return refresh().then(() => {
        closeModal();
        openActionOutcomeModal(config.title, config.eyebrow, 'succeeded', config.message(data), config.details(data));
        setSubmitBusy(form, false);
      });
    }

    function failCertificateAction(form, err, config) {
      closeModal();
      openActionOutcomeModal(config.title, config.eyebrow, 'failed', err.message || 'Operation failed', config.errorDetails ? config.errorDetails(err) : []);
      setSubmitBusy(form, false);
    }

    function bindCertificateWizardPicker(form) {
      const options = Array.from(form.querySelectorAll('.certificate-action-option'));
      const activate = (action) => {
        options.forEach((option) => {
          const active = option.dataset.action === action;
          option.classList.toggle('is-selected', active);
          const radio = option.querySelector('input[type="radio"]');
          if (radio) radio.checked = active;
        });
      };
      options.forEach((option) => {
        option.addEventListener('click', () => {
          if (option.classList.contains('is-disabled')) return;
          activate(option.dataset.action);
        });
        option.querySelector('input[type="radio"]')?.addEventListener('change', () => activate(option.dataset.action));
      });
      activate(form.querySelector('input[name="certificate_action"]:checked')?.value || 'import');
    }

    function openCreateCertificateWizard() {
      const canIssueFromCA = activeManagedAuthorities().length > 0;
      openModal('Add certificate', 'Certificates / Add', `
        <form id="certificateWizardForm" class="certificate-wizard">
          <div class="certificate-wizard-head">
            <div>
              <div class="eyebrow">Step 1 of 2</div>
              <h2>Add certificate</h2>
            </div>
          </div>
          <div class="choice-grid certificate-choice-grid">
            <label class="choice-card certificate-action-option" data-action="import">
              <input type="radio" name="certificate_action" value="import" checked />
              <span>
                <strong>Import certificate</strong>
                <em>Certificate, private key and optional chain files.</em>
              </span>
            </label>
            <label class="choice-card certificate-action-option" data-action="self_signed">
              <input type="radio" name="certificate_action" value="self_signed" />
              <span>
                <strong>Create self-signed certificate</strong>
                <em>Internal fallback certificate.</em>
              </span>
            </label>
            <label class="choice-card certificate-action-option ${canIssueFromCA ? '' : 'is-disabled'}" data-action="issue_from_ca">
              <input type="radio" name="certificate_action" value="issue_from_ca"${canIssueFromCA ? '' : ' disabled'} />
              <span>
                <strong>Issue from internal CA</strong>
                <em>${canIssueFromCA ? 'Use managed CA as issuer.' : 'Create a managed CA first.'}</em>
              </span>
            </label>
          </div>
          <details class="details-block certificate-advanced-actions">
            <summary>CA operations</summary>
            <div class="certificate-action-row">
              <button class="secondary-btn" type="button" data-certificate-action="managed_ca">Create TLS CA</button>
              <button class="secondary-btn" type="button" data-certificate-action="service_ca_root">Create service CA root</button>
              <button class="secondary-btn" type="button" data-certificate-action="letsencrypt">Let's Encrypt status</button>
            </div>
          </details>
          <div class="modal-actions">
            <button class="secondary-btn" type="button" id="cancelCertificateWizardBtn">Cancel</button>
            <button class="primary-btn" type="submit">Next</button>
          </div>
        </form>`, { wide: true });
      const form = document.getElementById('certificateWizardForm');
      bindCertificateWizardPicker(form);
      form.addEventListener('submit', (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        openCertificateActionForm(String(data.get('certificate_action') || 'import'));
      });
      document.getElementById('cancelCertificateWizardBtn')?.addEventListener('click', closeModal);
      document.querySelectorAll('[data-certificate-action]').forEach((button) => {
        button.addEventListener('click', () => openCertificateActionForm(button.dataset.certificateAction));
      });
    }

    function openCertificateActionForm(action, options = {}) {
      switch (action) {
        case 'import':
          openModal('Import certificate', 'Certificates / Import', `
            <form id="importCertificateForm" class="form-grid certificate-import-form">
              <div class="field"><label>Name</label><input name="name" placeholder="Auto-filled from certificate CN" /></div>
              <div class="field"><label>Description</label><input name="description" placeholder="Optional" /></div>
              <div class="field full file-field">
                <label>Certificate file</label>
                <input name="certificate_file" type="file" accept=".pem,.crt,.cer,.cert,.txt" required />
              </div>
              <div class="field full file-field">
                <label>Private key file</label>
                <input name="private_key_file" type="file" accept=".pem,.key,.txt" required />
              </div>
              <div class="field full file-field">
                <label>CA chain file</label>
                <input name="chain_file" type="file" accept=".pem,.crt,.cer,.chain,.txt" />
              </div>
              <div class="field full">
                <div id="certificateImportPreview" class="certificate-import-preview empty">Select certificate and private key files.</div>
              </div>
              <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
              <div class="field full inline-actions"><button class="primary-btn" type="submit">Import certificate</button></div>
            </form>
            <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
          {
            const form = document.getElementById('importCertificateForm');
            bindCertificateImportFilePreview(form);
            form.addEventListener('submit', importCertificateSubmit);
          }
          return;
        case 'self_signed':
          openModal('Create self-signed certificate', 'Certificates / Self-signed', `
            <form id="selfSignedCertificateForm" class="form-grid">
              <div class="field"><label>Name</label><input name="name" required placeholder="edge-selfsigned" /></div>
              <div class="field"><label>Description</label><input name="description" placeholder="Internal edge certificate" /></div>
              <div class="field"><label>Common Name</label><input name="common_name" required placeholder="edge.example.com" /></div>
              <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="3650" value="365" /></div>
              <div class="field full"><label>DNS names / SAN</label><input name="dns_names" placeholder="edge.example.com, *.example.com" /></div>
              <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
              <div class="field full inline-actions"><button class="primary-btn" type="submit">Create self-signed certificate</button></div>
            </form>
            <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
          document.getElementById('selfSignedCertificateForm').addEventListener('submit', createSelfSignedCertificateSubmit);
          return;
        case 'managed_ca':
          openModal('Create TLS CA', 'Certificates / TLS authority', `
            <form id="managedCAForm" class="form-grid">
              <div class="field"><label>Name</label><input name="name" required placeholder="Internal Edge TLS CA" /></div>
              <div class="field"><label>Description</label><input name="description" placeholder="Managed internal CA for edge TLS certificates" /></div>
              <div class="field"><label>Common Name</label><input name="common_name" required placeholder="Internal Edge TLS CA" /></div>
              <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="10950" value="3650" /></div>
              <div class="field full"><div class="field-hint">This CA issues TLS leaf certificates. It is not used as the OpenVPN service CA root.</div></div>
              <div class="field full inline-actions"><button class="primary-btn" type="submit">Create TLS CA</button></div>
            </form>
            <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
          document.getElementById('managedCAForm').addEventListener('submit', createManagedCASubmit);
          return;
        case 'issue_from_ca':
          openModal('Issue certificate from managed CA', 'Certificates / Issue from CA', `
            <form id="issueFromCAForm" class="form-grid">
              <div class="field"><label>Authority</label><select name="authority_certificate_id" required>${authorityCertificateOptions(options.authorityCertificateID || '')}</select></div>
              <div class="field"><label>Name</label><input name="name" required placeholder="edge-issued" /></div>
              <div class="field"><label>Description</label><input name="description" placeholder="Issued from managed CA" /></div>
              <div class="field"><label>Common Name</label><input name="common_name" required placeholder="edge.example.com" /></div>
              <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="3650" value="365" /></div>
              <div class="field full"><label>DNS names / SAN</label><input name="dns_names" placeholder="edge.example.com, *.example.com" /></div>
              <div class="field full"><label><input name="is_default" type="checkbox" value="1" /> Set as default certificate</label></div>
              <div class="field full inline-actions"><button class="primary-btn" type="submit">Issue certificate</button></div>
            </form>
            <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
          document.getElementById('issueFromCAForm').addEventListener('submit', issueCertificateFromCASubmit);
          return;
        case 'service_ca_root':
          openModal('Create service CA root', 'Certificates / Service PKI', `
            <form id="serviceCARootForm" class="form-grid">
              <div class="field"><label>Service code</label><input name="service_code" value="openvpn" required placeholder="openvpn" /></div>
              <div class="field"><label>PKI profile</label><input name="pki_profile" value="default" placeholder="default" /></div>
              <div class="field"><label>Common Name</label><input name="common_name" required placeholder="OpenVPN Platform CA" /></div>
              <div class="field"><label>Valid days</label><input name="valid_days" type="number" min="1" max="10950" value="3650" /></div>
              <div class="field full"><div class="field-hint">OpenVPN instances with the same PKI profile will issue server/client certificates from this root.</div></div>
              <div class="field full inline-actions"><button class="primary-btn" type="submit">Create service CA root</button></div>
            </form>
            <div id="certificateActionResult" class="form-result"></div>`, { wide: true });
          document.getElementById('serviceCARootForm').addEventListener('submit', createServiceCARootSubmit);
          return;
        case 'letsencrypt':
          openModal('Let\'s Encrypt', 'Certificates / ACME', `
            <div class="card">
              <h3>ACME is paused for this release line</h3>
              <p>The operator flow stays visible, but backend issuance is intentionally blocked until we resume ACME work and approve the canonical challenge strategy for this product: HTTP-01, DNS-01, or delegated external ACME.</p>
            </div>`, { wide: true });
          return;
        default:
          openCreateCertificateWizard();
      }
    }

    function parseCSVList(value) {
      return String(value || '')
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
    }

    function openManageCertificateModal(certificateID) {
      const item = (state.platformCertificates || []).find((cert) => cert.id === certificateID);
      if (!item) return;
      const status = certificateDisplayStatus(item);
      const isLeaf = item.kind === 'leaf';
      const isCA = item.kind === 'ca';
      const canSetDefault = isLeaf && status === 'active' && !item.is_default && item.key_secret_ref_id;
      const canRevoke = isLeaf && status === 'active';
      const canDeleteCA = isCA && status === 'active';
      const canIssue = isCA && status === 'active';
      openModal(`Manage certificate: ${certificatePrimaryLabel(item)}`, 'Certificates / Manage', `
        <div class="certificate-manage-layout">
          <section class="card">
            <div class="mini-label">Certificate</div>
            <h2>${escapeHTML(certificatePrimaryLabel(item))}</h2>
            <p>${escapeHTML(item.description || 'No description provided.')}</p>
            <div class="inline-actions">
              ${statusTag(status)}
              <span class="tag">${escapeHTML(item.kind || 'certificate')}</span>
              <span class="tag">${escapeHTML(item.source || 'unknown')}</span>
              ${item.is_default ? '<span class="tag ok">default</span>' : ''}
            </div>
          </section>
          <section class="card">
            <div class="mini-label">Lifecycle</div>
            <div class="response-grid">
              <div class="response-fact"><span>Common Name</span><strong>${escapeHTML(item.common_name || 'n/a')}</strong></div>
              <div class="response-fact"><span>Issuer</span><strong>${escapeHTML(item.issuer_name || 'self')}</strong></div>
              <div class="response-fact"><span>Expires</span><strong>${escapeHTML(certificateExpiryCaption(item))}</strong></div>
              <div class="response-fact"><span>Usage</span><strong>${escapeHTML(isLeaf ? certificateUsageCaption(item.id) : 'signing authority')}</strong></div>
            </div>
          </section>
          <section class="card">
            <div class="mini-label">SAN / DNS names</div>
            <div class="chip-list">
              ${Array.isArray(item.sans) && item.sans.length ? item.sans.map((name) => `<span class="chip">${escapeHTML(name)}</span>`).join('') : '<span class="metric-caption">No SAN records.</span>'}
            </div>
          </section>
          <section class="card">
            <div class="mini-label">Operational model</div>
            <p>${isLeaf
              ? 'Leaf TLS certificates can be assigned to edge TLS services and Xray/Nginx instances. Revoke only when no production binding depends on it.'
              : 'TLS CA can issue internal leaf TLS certificates. OpenVPN uses Service PKI roots instead. Delete CA is cascade and marks its issued children as deleted.'}</p>
            <div class="code-block">certificate_id = ${escapeHTML(item.id)}
cert_secret_ref = ${escapeHTML(item.cert_secret_ref_id || 'n/a')}
key_secret_ref = ${escapeHTML(item.key_secret_ref_id || 'n/a')}</div>
          </section>
        </div>
        <div class="modal-actions">
          <button class="secondary-btn" id="closeCertificateManageBtn" type="button">Close</button>
          ${canIssue ? '<button class="secondary-btn" id="issueFromSelectedCABtn" type="button">Issue certificate</button>' : ''}
          ${canSetDefault ? '<button class="primary-btn" id="setDefaultCertificateBtn" type="button">Set as default</button>' : ''}
          ${canRevoke ? '<button class="danger-btn" id="revokeManagedCertificateBtn" type="button">Revoke</button>' : ''}
          ${canDeleteCA ? '<button class="danger-btn" id="deleteManagedCABtn" type="button">Delete CA</button>' : ''}
        </div>`, { wide: true });
      document.getElementById('closeCertificateManageBtn')?.addEventListener('click', closeModal);
      document.getElementById('issueFromSelectedCABtn')?.addEventListener('click', () => openCertificateActionForm('issue_from_ca', { authorityCertificateID: item.id }));
      document.getElementById('setDefaultCertificateBtn')?.addEventListener('click', (event) => submitSetDefaultPlatformCertificate(item.id, event.currentTarget));
      document.getElementById('revokeManagedCertificateBtn')?.addEventListener('click', () => openRevokeCertificateModal(item.id, certificatePrimaryLabel(item)));
      document.getElementById('deleteManagedCABtn')?.addEventListener('click', () => openDeleteCAModal(item.id, certificatePrimaryLabel(item)));
    }

    async function submitSetDefaultPlatformCertificate(certificateID, button) {
      if (button) {
        button.disabled = true;
        button.textContent = 'Setting default...';
      }
      const item = (state.platformCertificates || []).find((cert) => cert.id === certificateID);
      try {
        const data = await sendJSON(`/api/v1/platform/certificates/${encodeURIComponent(certificateID)}/default`, 'POST', {});
        await refresh();
        closeModal();
        openActionOutcomeModal('Default certificate updated', 'Certificates / Success', 'succeeded', `Certificate ${certificatePrimaryLabel(item)} is now default.`, [
          { label: 'Certificate', value: certificatePrimaryLabel(item) },
          { label: 'Status', value: data.status || 'default' },
        ]);
      } catch (err) {
        closeModal();
        openActionOutcomeModal('Default certificate failed', 'Certificates / Error', 'failed', err.message || 'Set default certificate failed', [
          { label: 'Certificate', value: certificatePrimaryLabel(item) },
          { label: 'Action', value: 'Set default certificate' },
        ]);
      }
    }

    function openRevokeCertificateModal(certificateID, certificateName) {
      openModal('Revoke certificate', 'Certificates / Leaf revoke', `
        <div class="form-grid">
          <div class="field full">
            <div class="code-block">
              <div><strong>${escapeHTML(certificateName || certificateID || 'certificate')}</strong></div>
              <div class="metric-caption" style="margin-top:6px">После revoke сертификат станет неактивным, исчезнет из выбора и больше не сможет использоваться в новых apply / materialize операциях.</div>
            </div>
          </div>
          <div class="field full inline-actions">
            <button class="secondary-btn" id="cancelRevokeCertificateBtn" type="button">Cancel</button>
            <button class="danger-btn" id="confirmRevokeCertificateBtn" type="button">Revoke certificate</button>
          </div>
        </div>`);
      const cancelBtn = document.getElementById('cancelRevokeCertificateBtn');
      const confirmBtn = document.getElementById('confirmRevokeCertificateBtn');
      if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
      if (confirmBtn) confirmBtn.addEventListener('click', () => submitRevokePlatformCertificate(certificateID, certificateName, confirmBtn));
    }

    function openDeleteCAModal(certificateID, certificateName) {
      openModal('Delete managed CA', 'Certificates / CA delete', `
        <div class="form-grid">
          <div class="field full">
            <div class="code-block">
              <div><strong>${escapeHTML(certificateName || certificateID || 'managed CA')}</strong></div>
              <div class="metric-caption" style="margin-top:6px">Удаление CA каскадно пометит как deleted все сертификаты, которые были ею подписаны. После этого такие сертификаты больше нельзя будет использовать.</div>
            </div>
          </div>
          <div class="field full inline-actions">
            <button class="secondary-btn" id="cancelDeleteCABtn" type="button">Cancel</button>
            <button class="danger-btn" id="confirmDeleteCABtn" type="button">Delete CA</button>
          </div>
        </div>`);
      const cancelBtn = document.getElementById('cancelDeleteCABtn');
      const confirmBtn = document.getElementById('confirmDeleteCABtn');
      if (cancelBtn) cancelBtn.addEventListener('click', closeModal);
      if (confirmBtn) confirmBtn.addEventListener('click', () => submitDeletePlatformCA(certificateID, certificateName, confirmBtn));
    }

    async function submitRevokePlatformCertificate(certificateID, certificateName, button) {
      if (button) {
        button.disabled = true;
        button.textContent = 'Revoking...';
      }
      try {
        const data = await sendJSON(`/api/v1/platform/certificates/${encodeURIComponent(certificateID)}/revoke`, 'POST', {});
        await refresh();
        closeModal();
        openActionOutcomeModal('Certificate revoked', 'Certificates / Success', 'succeeded', `Certificate ${certificateName || certificateID} was revoked successfully.`, [
          { label: 'Certificate', value: certificateName || certificateID },
          { label: 'Status', value: data.status || 'revoked' },
        ]);
      } catch (err) {
        closeModal();
        openActionOutcomeModal('Certificate revoke failed', 'Certificates / Error', 'failed', err.message || 'Certificate revoke failed', [
          { label: 'Certificate', value: certificateName || certificateID },
          { label: 'Action', value: 'Revoke leaf certificate' },
        ]);
      }
    }

    async function submitDeletePlatformCA(certificateID, certificateName, button) {
      if (button) {
        button.disabled = true;
        button.textContent = 'Deleting...';
      }
      try {
        const data = await requestJSON(`/api/v1/platform/certificates/${encodeURIComponent(certificateID)}`, { method: 'DELETE' });
        await refresh();
        closeModal();
        openActionOutcomeModal('Managed CA deleted', 'Certificates / Success', 'succeeded', `Managed CA ${certificateName || certificateID} was deleted with cascade.`, [
          { label: 'CA', value: certificateName || certificateID },
          { label: 'Status', value: data.status || 'deleted' },
          { label: 'Cascade count', value: data.cascade_count || 0 },
        ]);
      } catch (err) {
        closeModal();
        openActionOutcomeModal('Managed CA delete failed', 'Certificates / Error', 'failed', err.message || 'Managed CA delete failed', [
          { label: 'CA', value: certificateName || certificateID },
          { label: 'Action', value: 'Delete CA with cascade' },
        ]);
      }
    }

    function selectedFile(formEl, name) {
      const input = formEl.querySelector(`input[name="${name}"]`);
      return input?.files?.[0] || null;
    }

    function readFileAsText(file) {
      if (!file) return Promise.resolve('');
      if (typeof file.text === 'function') return file.text();
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result || ''));
        reader.onerror = () => reject(reader.error || new Error('file read failed'));
        reader.readAsText(file);
      });
    }

    async function certificateImportPayloadFromForm(formEl, requireFiles = true) {
      const form = new FormData(formEl);
      const certFile = selectedFile(formEl, 'certificate_file');
      const keyFile = selectedFile(formEl, 'private_key_file');
      const chainFile = selectedFile(formEl, 'chain_file');
      if (requireFiles && !certFile) throw new Error('Select certificate file');
      if (requireFiles && !keyFile) throw new Error('Select private key file');
      return {
        name: String(form.get('name') || '').trim(),
        description: String(form.get('description') || '').trim(),
        certificate: String(await readFileAsText(certFile)).trim(),
        private_key: String(await readFileAsText(keyFile)).trim(),
        chain: String(await readFileAsText(chainFile)).trim(),
        is_default: String(form.get('is_default') || '') === '1',
      };
    }

    function bindCertificateImportFilePreview(formEl) {
      if (!formEl) return;
      const update = () => refreshCertificateImportPreview(formEl);
      formEl.querySelectorAll('input[type="file"]').forEach((input) => input.addEventListener('change', update));
      formEl.querySelector('input[name="name"]')?.addEventListener('input', () => {
        formEl.dataset.nameEdited = '1';
      });
      update();
    }

    async function refreshCertificateImportPreview(formEl) {
      const target = document.getElementById('certificateImportPreview');
      if (!target) return;
      const certFile = selectedFile(formEl, 'certificate_file');
      const keyFile = selectedFile(formEl, 'private_key_file');
      const chainFile = selectedFile(formEl, 'chain_file');
      if (!certFile && !keyFile && !chainFile) {
        target.className = 'certificate-import-preview empty';
        target.textContent = 'Select certificate and private key files.';
        return;
      }
      if (!certFile || !keyFile) {
        target.className = 'certificate-import-preview empty';
        target.textContent = 'Certificate and private key files are required.';
        return;
      }

      const seq = ++certificateImportPreviewSeq;
      target.className = 'certificate-import-preview';
      target.innerHTML = '<span class="tag warn">checking</span>';
      try {
        const payload = await certificateImportPayloadFromForm(formEl, false);
        if (!payload.certificate || !payload.private_key) {
          target.className = 'certificate-import-preview empty';
          target.textContent = 'Certificate and private key files are required.';
          return;
        }
        const preview = await sendJSON('/api/v1/platform/certificates/preview', 'POST', payload);
        if (seq !== certificateImportPreviewSeq) return;
        const nameInput = formEl.querySelector('input[name="name"]');
        if (nameInput && !nameInput.value.trim() && formEl.dataset.nameEdited !== '1' && preview.common_name) {
          nameInput.value = preview.common_name;
        }
        target.className = 'certificate-import-preview';
        target.innerHTML = renderCertificateImportPreview(preview, { certFile, keyFile, chainFile });
      } catch (err) {
        if (seq !== certificateImportPreviewSeq) return;
        target.className = 'certificate-import-preview error';
        target.innerHTML = `
          <div class="inline-actions"><span class="tag danger">invalid</span></div>
          <div class="metric-caption strong">${escapeHTML(err.message || 'Certificate preview failed')}</div>`;
      }
    }

    function renderCertificateImportPreview(preview, files) {
      const sans = Array.isArray(preview.sans) ? preview.sans : [];
      return `
        <div class="inline-actions"><span class="tag ok">valid</span><span class="tag">${escapeHTML(preview.private_key_type || 'key')}</span>${preview.key_pair_valid ? '<span class="tag ok">key matches</span>' : '<span class="tag danger">key mismatch</span>'}</div>
        <div class="response-grid certificate-preview-grid">
          <div class="response-fact"><span>Common Name</span><strong>${escapeHTML(preview.common_name || 'n/a')}</strong></div>
          <div class="response-fact"><span>Issuer</span><strong>${escapeHTML(preview.issuer_name || 'self')}</strong></div>
          <div class="response-fact"><span>Expires</span><strong>${escapeHTML(certificateExpiryCaption({ not_after: preview.not_after }))}</strong></div>
          <div class="response-fact"><span>Chain</span><strong>${escapeHTML(String(preview.chain_certificate_count || 0))} certificates</strong></div>
        </div>
        <div class="chip-list certificate-file-list">
          <span class="chip">${escapeHTML(files.certFile?.name || 'certificate')}</span>
          <span class="chip">${escapeHTML(files.keyFile?.name || 'private key')}</span>
          ${files.chainFile ? `<span class="chip">${escapeHTML(files.chainFile.name)}</span>` : ''}
        </div>
        <div class="chip-list">
          ${sans.length ? sans.map((name) => `<span class="chip">${escapeHTML(name)}</span>`).join('') : '<span class="metric-caption">No SAN records.</span>'}
        </div>`;
    }

    async function importCertificateSubmit(event) {
      event.preventDefault();
      const formEl = event.currentTarget;
      setSubmitBusy(formEl, true, 'Importing...');
      try {
        const payload = await certificateImportPayloadFromForm(formEl, true);
        const data = await sendJSON('/api/v1/platform/certificates/import', 'POST', payload);
        await finishCertificateAction(formEl, data, {
          title: 'Certificate imported',
          eyebrow: 'Certificates / Success',
          message: (item) => `Certificate ${item.name || item.common_name || item.id} was imported successfully.`,
          details: (item) => [
            { label: 'Name', value: item.name || item.common_name || item.id },
            { label: 'Common Name', value: item.common_name || 'n/a' },
            { label: 'Source', value: item.source || 'imported' },
            { label: 'Expires', value: certificateExpiryCaption(item) },
          ],
          errorDetails: () => [{ label: 'Action', value: 'Import certificate' }],
        });
      } catch (err) {
        failCertificateAction(formEl, err, {
          title: 'Certificate import failed',
          eyebrow: 'Certificates / Error',
          errorDetails: () => [{ label: 'Action', value: 'Import certificate' }],
        });
      }
    }

    async function createSelfSignedCertificateSubmit(event) {
      event.preventDefault();
      const formEl = event.currentTarget;
      setSubmitBusy(formEl, true, 'Creating...');
      try {
        const form = new FormData(formEl);
        const payload = {
          name: String(form.get('name') || '').trim(),
          description: String(form.get('description') || '').trim(),
          common_name: String(form.get('common_name') || '').trim(),
          dns_names: parseCSVList(form.get('dns_names')),
          valid_days: Number(form.get('valid_days') || 365),
          is_default: String(form.get('is_default') || '') === '1',
        };
        const data = await sendJSON('/api/v1/platform/certificates/self-signed', 'POST', payload);
        await finishCertificateAction(formEl, data, {
          title: 'Self-signed certificate created',
          eyebrow: 'Certificates / Success',
          message: (item) => `Certificate ${item.name || item.common_name || item.id} was created successfully.`,
          details: (item) => [
            { label: 'Name', value: item.name || item.common_name || item.id },
            { label: 'Common Name', value: item.common_name || 'n/a' },
            { label: 'Valid until', value: certificateExpiryCaption(item) },
            { label: 'Default', value: item.is_default ? 'yes' : 'no' },
          ],
        });
      } catch (err) {
        failCertificateAction(formEl, err, {
          title: 'Certificate creation failed',
          eyebrow: 'Certificates / Error',
          errorDetails: () => [{ label: 'Action', value: 'Create self-signed certificate' }],
        });
      }
    }

    async function createManagedCASubmit(event) {
      event.preventDefault();
      const formEl = event.currentTarget;
      setSubmitBusy(formEl, true, 'Creating CA...');
      try {
        const form = new FormData(formEl);
        const payload = {
          name: String(form.get('name') || '').trim(),
          description: String(form.get('description') || '').trim(),
          common_name: String(form.get('common_name') || '').trim(),
          valid_days: Number(form.get('valid_days') || 3650),
        };
        const data = await sendJSON('/api/v1/platform/certificates/authorities', 'POST', payload);
        await finishCertificateAction(formEl, data, {
          title: 'TLS CA created',
          eyebrow: 'Certificates / Success',
          message: (item) => `TLS certificate authority ${item.name || item.common_name || item.id} was created successfully.`,
          details: (item) => [
            { label: 'Name', value: item.name || item.common_name || item.id },
            { label: 'Common Name', value: item.common_name || 'n/a' },
            { label: 'Valid until', value: certificateExpiryCaption(item) },
            { label: 'Use for', value: 'issuing TLS leaf certificates' },
          ],
        });
      } catch (err) {
        failCertificateAction(formEl, err, {
          title: 'TLS CA creation failed',
          eyebrow: 'Certificates / Error',
          errorDetails: () => [{ label: 'Action', value: 'Create TLS CA' }],
        });
      }
    }

    async function issueCertificateFromCASubmit(event) {
      event.preventDefault();
      const formEl = event.currentTarget;
      setSubmitBusy(formEl, true, 'Issuing...');
      try {
        const form = new FormData(formEl);
        const payload = {
          authority_certificate_id: String(form.get('authority_certificate_id') || '').trim(),
          name: String(form.get('name') || '').trim(),
          description: String(form.get('description') || '').trim(),
          common_name: String(form.get('common_name') || '').trim(),
          dns_names: parseCSVList(form.get('dns_names')),
          valid_days: Number(form.get('valid_days') || 365),
          is_default: String(form.get('is_default') || '') === '1',
        };
        const data = await sendJSON('/api/v1/platform/certificates/issue-from-ca', 'POST', payload);
        await finishCertificateAction(formEl, data, {
          title: 'Certificate issued',
          eyebrow: 'Certificates / Success',
          message: (item) => `Certificate ${item.name || item.common_name || item.id} was issued successfully.`,
          details: (item) => [
            { label: 'Name', value: item.name || item.common_name || item.id },
            { label: 'Common Name', value: item.common_name || 'n/a' },
            { label: 'Issuer', value: item.issuer_name || 'managed CA' },
            { label: 'Valid until', value: certificateExpiryCaption(item) },
          ],
        });
      } catch (err) {
        failCertificateAction(formEl, err, {
          title: 'Certificate issue failed',
          eyebrow: 'Certificates / Error',
          errorDetails: () => [{ label: 'Action', value: 'Issue certificate from managed CA' }],
        });
      }
    }

    async function createServiceCARootSubmit(event) {
      event.preventDefault();
      const formEl = event.currentTarget;
      setSubmitBusy(formEl, true, 'Creating service CA...');
      try {
        const form = new FormData(formEl);
        const payload = {
          service_code: String(form.get('service_code') || '').trim(),
          pki_profile: String(form.get('pki_profile') || '').trim(),
          common_name: String(form.get('common_name') || '').trim(),
          valid_days: Number(form.get('valid_days') || 3650),
        };
        const data = await sendJSON('/api/v1/platform/pki-roots', 'POST', payload);
        await finishCertificateAction(formEl, data, {
          title: 'Service CA root created',
          eyebrow: 'Certificates / Success',
          message: (item) => `Service CA root for ${item.service_code || 'service'} profile ${item.pki_profile || 'default'} was created successfully.`,
          details: (item) => [
            { label: 'Service', value: item.service_code || 'n/a' },
            { label: 'Profile', value: item.pki_profile || 'default' },
            { label: 'Common Name', value: item.common_name || 'n/a' },
            { label: 'Valid until', value: certificateExpiryCaption(item) },
            { label: 'Use for', value: 'service server/client certificates' },
          ],
        });
      } catch (err) {
        failCertificateAction(formEl, err, {
          title: 'Service CA root creation failed',
          eyebrow: 'Certificates / Error',
          errorDetails: () => [{ label: 'Action', value: 'Create service CA root' }],
        });
      }
    }

    return {
      openCreateCertificateWizard,
      openCertificateActionForm,
      openManageCertificateModal,
      submitSetDefaultPlatformCertificate,
    };
  }

  window.MegaVPNCertificateWorkflows = { create: createCertificateWorkflows };
})(window);
