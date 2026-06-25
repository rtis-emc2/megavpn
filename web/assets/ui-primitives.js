(function (window) {
  'use strict';

  function createUIPrimitives(ctx = {}) {
    const { escapeHTML } = ctx;
    if (typeof escapeHTML !== 'function') {
      throw new Error('MegaVPNUIPrimitives requires escapeHTML');
    }

    function metric(label, value, caption, targetPage = '') {
      const attrs = targetPage ? ` role="button" tabindex="0" data-page-target="${escapeHTML(targetPage)}"` : '';
      return `<div class="card metric-card${targetPage ? ' dashboard-nav-tile' : ''}"${attrs}><div class="mini-label">${escapeHTML(label)}</div><div class="metric-value">${escapeHTML(value)}</div><div class="metric-caption">${escapeHTML(caption)}</div></div>`;
    }

    function tableCard(title, rows = [], columns = [], tools = '') {
      const safeRows = Array.isArray(rows) ? rows : [];
      const body = safeRows.length
        ? safeRows.map((row) => `<tr>${columns.map((column) => `<td>${column.render ? column.render(row) : escapeHTML(row[column.key])}</td>`).join('')}</tr>`).join('')
        : `<tr><td colspan="${columns.length}"><div class="empty">Нет данных для отображения</div></td></tr>`;
      return `
        <section class="table-card">
          <div class="table-head"><h2>${escapeHTML(title)}</h2><div class="table-tools">${tools}</div></div>
          <div class="table-wrap"><table><thead><tr>${columns.map((column) => `<th>${escapeHTML(column.title)}</th>`).join('')}</tr></thead><tbody>${body}</tbody></table></div>
        </section>`;
    }

    return { metric, tableCard };
  }

  window.MegaVPNUIPrimitives = { create: createUIPrimitives };
})(window);
