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

    const tabs = [
      ['overview', 'Overview', 'issued inventory'],
      ['leaf', 'TLS certificates', 'leaf and default'],
      ['authorities', 'TLS authorities', 'managed issuers'],
      ['service', 'Service PKI', 'service roots'],
    ];

    function selectedTab() {
      const key = tabs.some(([tab]) => tab === state.certificatesTab) ? state.certificatesTab : 'overview';
      state.certificatesTab = key;
      return key;
    }

    function renderTabs(active, counts) {
      return `
        <div class="page-tabs control-tabs" role="tablist" aria-label="Certificate sections">
          ${tabs.map(([key, label, caption]) => `
            <button class="page-tab ${active === key ? 'is-active' : ''}" type="button" data-certificate-tab="${escapeHTML(key)}" role="tab" aria-selected="${active === key ? 'true' : 'false'}">
              <span>${escapeHTML(label)} <em>${escapeHTML(String(counts[key] || 0))}</em></span>
              <small>${escapeHTML(caption)}</small>
            </button>`).join('')}
        </div>`;
    }

    function bindTabs() {
      document.querySelectorAll('[data-certificate-tab]').forEach((button) => {
        button.addEventListener('click', () => {
          const tab = button.dataset.certificateTab || 'overview';
          state.certificatesTab = tab;
          localStorage.setItem('megavpn.certificatesTab', tab);
          document.querySelectorAll('[data-certificate-tab]').forEach((item) => {
            const active = item.dataset.certificateTab === tab;
            item.classList.toggle('is-active', active);
            item.setAttribute('aria-selected', active ? 'true' : 'false');
          });
          document.querySelectorAll('[data-certificate-panel]').forEach((panel) => {
            panel.hidden = panel.dataset.certificatePanel !== tab;
          });
        });
      });
    }

    function render() {
      setTitle('Certificates');
      const activeTab = selectedTab();
      const allCertificates = Array.isArray(state.platformCertificates)
        ? state.platformCertificates.filter((item) => String(item.status || '').toLowerCase() !== 'deleted')
        : [];
      const leafRows = allCertificates.filter((item) => item.kind === 'leaf');
      const authorityRows = allCertificates.filter((item) => item.kind === 'ca');
      const rootRows = Array.isArray(state.platformPKIRoots)
        ? state.platformPKIRoots.filter((item) => String(item.status || 'active').toLowerCase() !== 'revoked')
        : [];
      const defaultLeaf = leafRows.find((item) => item.is_default);
      const activeRoots = rootRows.filter((item) => certificateDisplayStatus(item) === 'active').length;
      const summaryRows = [
        ...leafRows.map((item) => ({
          type: 'TLS leaf',
          name: certificatePrimaryLabel(item),
          scope: item.is_default ? 'default TLS endpoint' : certificateUsageCaption(item.id),
          common: item.common_name || 'n/a',
          expires: certificateExpiryCaption(item),
          status: certificateDisplayStatus(item),
          created: item.created_at,
        })),
        ...authorityRows.map((item) => ({
          type: 'TLS CA',
          name: certificatePrimaryLabel(item),
          scope: 'issues TLS leaf certificates',
          common: item.common_name || 'n/a',
          expires: certificateExpiryCaption(item),
          status: certificateDisplayStatus(item),
          created: item.created_at,
        })),
        ...rootRows.map((item) => ({
          type: 'Service CA',
          name: `${item.service_code || 'service'} / ${item.pki_profile || 'default'}`,
          scope: 'service client/server certificates',
          common: item.common_name || 'n/a',
          expires: certificateExpiryCaption(item),
          status: certificateDisplayStatus(item),
          created: item.created_at,
        })),
      ].sort((a, b) => new Date(b.created || 0).getTime() - new Date(a.created || 0).getTime());
      const counts = {
        overview: summaryRows.length,
        leaf: leafRows.length,
        authorities: authorityRows.length,
        service: rootRows.length,
      };
      el('content').innerHTML = `
        <div class="control-page-shell certificates-page-shell">
          <section class="section-card control-page-intro">
            <div>
              <h2>Certificate center</h2>
              <p>Single place for issued TLS certificates, managed TLS authorities and service PKI trust roots.</p>
            </div>
            <div class="control-page-actions">
              <button class="primary-btn" id="createCertificateBtn" type="button">Add TLS certificate</button>
              <button class="secondary-btn" id="createManagedCABtn" type="button">Create TLS CA</button>
              <button class="secondary-btn" id="createServiceCABtn" type="button">Create service CA root</button>
            </div>
          </section>
          ${renderTabs(activeTab, counts)}
          <div class="certificates-tab-panel" data-certificate-panel="overview" ${activeTab === 'overview' ? '' : 'hidden'}>
            <div class="certificate-overview-grid">
              <div class="certificate-overview-card">
                <span>TLS certificates</span>
                <strong>${escapeHTML(String(leafRows.length))}</strong>
                <small>${escapeHTML(defaultLeaf ? `default: ${certificatePrimaryLabel(defaultLeaf)}` : 'no default leaf certificate')}</small>
              </div>
              <div class="certificate-overview-card">
                <span>TLS authorities</span>
                <strong>${escapeHTML(String(authorityRows.length))}</strong>
                <small>issue leaf certificates for edge TLS endpoints</small>
              </div>
              <div class="certificate-overview-card">
                <span>Service PKI roots</span>
                <strong>${escapeHTML(String(rootRows.length))}</strong>
                <small>${escapeHTML(String(activeRoots))} active service trust roots</small>
              </div>
            </div>
            <section class="table-card certificate-summary-card">
              <div class="table-head"><h2>Issued certificates</h2><div class="table-tools"><span class="tag">${escapeHTML(String(summaryRows.length))} items</span></div></div>
              <div class="table-wrap">
                <table>
                  <thead><tr><th>Type</th><th>Name</th><th>Common Name</th><th>Scope</th><th>Expires</th><th>Status</th></tr></thead>
                  <tbody>
                    ${summaryRows.length ? summaryRows.map((item) => `
                      <tr>
                        <td><span class="tag">${escapeHTML(item.type)}</span></td>
                        <td><strong>${escapeHTML(item.name)}</strong></td>
                        <td>${escapeHTML(item.common)}</td>
                        <td>${escapeHTML(item.scope)}</td>
                        <td>${escapeHTML(item.expires)}</td>
                        <td>${statusTag(item.status)}</td>
                      </tr>`).join('') : '<tr><td colspan="6"><div class="empty">No certificates or service trust roots have been issued yet.</div></td></tr>'}
                  </tbody>
                </table>
              </div>
            </section>
          </div>
          <div class="certificates-tab-panel" data-certificate-panel="leaf" ${activeTab === 'leaf' ? '' : 'hidden'}>
        <section class="table-card certificate-manager-card">
          <div class="table-head">
            <div>
              <h2>TLS certificates</h2>
              <div class="metric-caption">Leaf certificates with private keys for Nginx, Xray TLS and control-plane TLS bindings.</div>
            </div>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Certificate</th><th>Issuer</th><th>Expires</th><th>Status</th><th>Usage</th><th>Actions</th></tr></thead>
              <tbody>
                ${leafRows.length ? leafRows.map((item) => {
                  const status = certificateDisplayStatus(item);
                  const canSetDefault = status === 'active' && !item.is_default && item.key_secret_ref_id;
                  return `
                    <tr>
                      <td>
                        <strong>${escapeHTML(certificatePrimaryLabel(item))}</strong>
                        <div class="timeline-meta">${escapeHTML(item.common_name || 'n/a')}${item.is_default ? ' · default' : ''}</div>
                      </td>
                      <td>${escapeHTML(item.issuer_name || 'self')}</td>
                      <td>${escapeHTML(certificateExpiryCaption(item))}</td>
                      <td>${statusTag(status)}</td>
                      <td>${escapeHTML(certificateUsageCaption(item.id))}</td>
                      <td>
                        <div class="inline-actions compact-actions">
                          <button class="secondary-btn certificate-manage-btn" type="button" data-certificate-id="${escapeHTML(item.id)}">Manage</button>
                          ${canSetDefault ? `<button class="secondary-btn certificate-default-btn" type="button" data-certificate-id="${escapeHTML(item.id)}">Set default</button>` : ''}
                        </div>
                      </td>
                    </tr>`;
                }).join('') : '<tr><td colspan="6"><div class="empty">No TLS leaf certificates yet. Import one, create a self-signed fallback, or issue from a managed TLS CA.</div></td></tr>'}
              </tbody>
            </table>
          </div>
        </section>
          </div>
          <div class="certificates-tab-panel" data-certificate-panel="authorities" ${activeTab === 'authorities' ? '' : 'hidden'}>
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>TLS certificate authorities</h2>
              <div class="metric-caption">Managed CA roots for issuing internal TLS leaf certificates. Not used as OpenVPN service CA roots.</div>
            </div>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Authority</th><th>Common Name</th><th>Expires</th><th>Status</th><th>Usage</th><th>Actions</th></tr></thead>
              <tbody>
                ${authorityRows.length ? authorityRows.map((item) => `
                  <tr>
                    <td><strong>${escapeHTML(certificatePrimaryLabel(item))}</strong><div class="timeline-meta">${escapeHTML(item.source || 'managed_ca')}</div></td>
                    <td>${escapeHTML(item.common_name || 'n/a')}</td>
                    <td>${escapeHTML(certificateExpiryCaption(item))}</td>
                    <td>${statusTag(certificateDisplayStatus(item))}</td>
                    <td>${escapeHTML('issues TLS leaf certificates')}</td>
                    <td><button class="secondary-btn certificate-manage-btn" type="button" data-certificate-id="${escapeHTML(item.id)}">Manage</button></td>
                  </tr>`).join('') : '<tr><td colspan="6"><div class="empty">No managed TLS CA yet. Create one if you want the platform to issue internal TLS certificates.</div></td></tr>'}
              </tbody>
            </table>
          </div>
        </section>
          </div>
          <div class="certificates-tab-panel" data-certificate-panel="service" ${activeTab === 'service' ? '' : 'hidden'}>
        <section class="table-card">
          <div class="table-head">
            <div>
              <h2>Service PKI roots</h2>
              <div class="metric-caption">Service trust roots for generated service certificates. OpenVPN instances share trust by selecting the same service/profile pair.</div>
            </div>
            <div class="table-tools">
              <span class="tag">${escapeHTML(String(rootRows.length))} roots</span>
            </div>
          </div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Service</th><th>Profile</th><th>Common Name</th><th>Expires</th><th>Status</th><th>Created</th></tr></thead>
              <tbody>
                ${rootRows.length ? rootRows.map((root) => `
                  <tr>
                    <td>${escapeHTML(root.service_code || 'n/a')}</td>
                    <td><span class="tag">${escapeHTML(root.pki_profile || 'default')}</span></td>
                    <td>${escapeHTML(root.common_name || 'n/a')}</td>
                    <td>${escapeHTML(certificateExpiryCaption(root))}</td>
                    <td>${statusTag(certificateDisplayStatus(root))}</td>
                    <td>${formatDate(root.created_at)}</td>
                  </tr>`).join('') : '<tr><td colspan="6"><div class="empty">No service CA roots yet. OpenVPN can auto-create the default platform root during apply, or create one explicitly.</div></td></tr>'}
              </tbody>
            </table>
          </div>
        </section>
          </div>
        </div>`;
      bindTabs();
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
