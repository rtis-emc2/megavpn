(function (window) {
  'use strict';

  function createCertificatesPage(ctx = {}) {
    const {
      state,
      setTitle,
      el,
      statusTag,
      escapeHTML,
      formatDate,
      certificateDisplayStatus,
      certificateExpiryCaption,
      certificatePrimaryLabel,
      certificateUsageCaption,
      openCreateCertificateWizard,
      openCertificateActionForm,
      openManageCertificateModal,
      submitSetDefaultPlatformCertificate,
    } = ctx;
    if (
      !state ||
      typeof setTitle !== 'function' ||
      typeof el !== 'function' ||
      typeof statusTag !== 'function' ||
      typeof escapeHTML !== 'function' ||
      typeof formatDate !== 'function' ||
      typeof certificateDisplayStatus !== 'function' ||
      typeof certificateExpiryCaption !== 'function' ||
      typeof certificatePrimaryLabel !== 'function' ||
      typeof certificateUsageCaption !== 'function' ||
      typeof openCreateCertificateWizard !== 'function' ||
      typeof openCertificateActionForm !== 'function' ||
      typeof openManageCertificateModal !== 'function' ||
      typeof submitSetDefaultPlatformCertificate !== 'function'
    ) {
      throw new Error('MegaVPNCertificatesPage requires page dependencies');
    }

    function render() {
      setTitle('Certificates');
      const allCertificates = Array.isArray(state.platformCertificates)
        ? state.platformCertificates.filter((item) => String(item.status || '').toLowerCase() !== 'deleted')
        : [];
      const certRows = allCertificates.filter((item) => item.kind === 'leaf' || item.kind === 'ca');
      const rootRows = Array.isArray(state.platformPKIRoots) ? state.platformPKIRoots : [];
      el('content').innerHTML = `
        <section class="table-card certificate-manager-card">
          <div class="table-head">
            <div>
              <h2>Certificates</h2>
            </div>
            <div class="table-tools">
              <button class="primary-btn" id="createCertificateBtn" type="button">Add certificate</button>
              <button class="secondary-btn" id="createManagedCABtn" type="button">Create CA</button>
              <button class="secondary-btn" id="createServiceCABtn" type="button">Service CA root</button>
            </div>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Certificate</th><th>Type</th><th>Issuer</th><th>Expires</th><th>Status</th><th>Usage</th><th>Actions</th></tr></thead>
              <tbody>
                ${certRows.length ? certRows.map((item) => {
                  const status = certificateDisplayStatus(item);
                  const isLeaf = item.kind === 'leaf';
                  const canSetDefault = isLeaf && status === 'active' && !item.is_default && item.key_secret_ref_id;
                  return `
                    <tr>
                      <td>
                        <strong>${escapeHTML(certificatePrimaryLabel(item))}</strong>
                        <div class="timeline-meta">${escapeHTML(item.common_name || 'n/a')}${item.is_default ? ' · default' : ''}</div>
                      </td>
                      <td><span class="tag">${escapeHTML(item.kind || 'certificate')}</span><span class="tag">${escapeHTML(item.source || 'unknown')}</span></td>
                      <td>${escapeHTML(item.issuer_name || 'self')}</td>
                      <td>${escapeHTML(certificateExpiryCaption(item))}</td>
                      <td>${statusTag(status)}</td>
                      <td>${escapeHTML(isLeaf ? certificateUsageCaption(item.id) : 'signing authority')}</td>
                      <td>
                        <div class="inline-actions compact-actions">
                          <button class="secondary-btn certificate-manage-btn" type="button" data-certificate-id="${escapeHTML(item.id)}">Manage</button>
                          ${canSetDefault ? `<button class="secondary-btn certificate-default-btn" type="button" data-certificate-id="${escapeHTML(item.id)}">Set default</button>` : ''}
                        </div>
                      </td>
                    </tr>`;
                }).join('') : '<tr><td colspan="7"><div class="empty">No certificates yet. Use Add certificate to import or create the first leaf certificate.</div></td></tr>'}
              </tbody>
            </table>
          </div>
        </section>
        <section class="table-card">
          <div class="table-head"><h2>Service CA Center</h2><div class="table-tools"><span class="tag">${escapeHTML(String(rootRows.length))} roots</span></div></div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>Service</th><th>Profile</th><th>Common Name</th><th>Expires</th><th>Status</th><th>Created</th></tr></thead>
                <tbody>
                  ${rootRows.length ? rootRows.map((root) => `
                    <tr>
                      <td>${escapeHTML(root.service_code || 'n/a')}</td>
                      <td>${escapeHTML(root.pki_profile || 'default')}</td>
                      <td>${escapeHTML(root.common_name || 'n/a')}</td>
                      <td>${escapeHTML(certificateExpiryCaption(root))}</td>
                      <td>${statusTag(certificateDisplayStatus(root))}</td>
                      <td>${formatDate(root.created_at)}</td>
                    </tr>`).join('') : '<tr><td colspan="6"><div class="empty">No service CA roots yet. OpenVPN can auto-create the default platform root during apply, or create one explicitly.</div></td></tr>'}
                </tbody>
              </table>
            </div>
        </section>`;
      document.getElementById('createCertificateBtn')?.addEventListener('click', openCreateCertificateWizard);
      document.getElementById('createManagedCABtn')?.addEventListener('click', () => openCertificateActionForm('managed_ca'));
      document.getElementById('createServiceCABtn')?.addEventListener('click', () => openCertificateActionForm('service_ca_root'));
      document.querySelectorAll('.certificate-manage-btn').forEach((button) => {
        button.addEventListener('click', () => openManageCertificateModal(button.dataset.certificateId));
      });
      document.querySelectorAll('.certificate-default-btn').forEach((button) => {
        button.addEventListener('click', () => submitSetDefaultPlatformCertificate(button.dataset.certificateId, button));
      });
    }

    return { render };
  }

  window.MegaVPNCertificatesPage = { create: createCertificatesPage };
})(window);
