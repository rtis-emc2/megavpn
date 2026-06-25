(function (window, document) {
  'use strict';

  const STORAGE_PREFIX = 'megavpn.table.widths.';
  const MIN_COLUMN_WIDTH = 72;

  function tableHeaders(table) {
    return Array.from(table.querySelectorAll('thead th'));
  }

  function textKey(value) {
    return String(value || '')
      .trim()
      .toLowerCase()
      .replace(/\s+/g, ' ')
      .slice(0, 160);
  }

  function nearestTableTitle(table) {
    const section = table.closest('.table-card,.section-card,.card,.modal-body,section');
    const title = section?.querySelector('h1,h2,h3,.table-head h2,.table-head h3');
    return textKey(title?.textContent || 'table');
  }

  function storageKey(table) {
    const headings = tableHeaders(table).map((th) => textKey(th.textContent)).join('|');
    const page = textKey(document.getElementById('pageTitle')?.textContent || document.body.dataset.page || 'page');
    return STORAGE_PREFIX + page + '.' + nearestTableTitle(table) + '.' + headings;
  }

  function readStoredWidths(table) {
    try {
      const raw = window.localStorage.getItem(storageKey(table));
      const parsed = raw ? JSON.parse(raw) : null;
      return Array.isArray(parsed) ? parsed.map((item) => Number(item)).filter((item) => Number.isFinite(item) && item > 0) : [];
    } catch (_) {
      return [];
    }
  }

  function writeStoredWidths(table) {
    const cols = Array.from(table.querySelectorAll('colgroup col'));
    const widths = cols.map((col) => Math.round(Number.parseFloat(col.style.width || '0')) || 0);
    if (!widths.some(Boolean)) return;
    try {
      window.localStorage.setItem(storageKey(table), JSON.stringify(widths));
    } catch (_) {
      // localStorage can be disabled in hardened browsers; resizing still works for the current render.
    }
  }

  function ensureTableWrap(table) {
    if (table.parentElement?.classList.contains('table-wrap')) return;
    const wrap = document.createElement('div');
    wrap.className = 'table-wrap';
    table.parentElement?.insertBefore(wrap, table);
    wrap.appendChild(table);
  }

  function ensureColGroup(table, count) {
    let colgroup = table.querySelector(':scope > colgroup');
    if (!colgroup) {
      colgroup = document.createElement('colgroup');
      table.insertBefore(colgroup, table.firstChild);
    }
    while (colgroup.children.length < count) colgroup.appendChild(document.createElement('col'));
    while (colgroup.children.length > count) colgroup.removeChild(colgroup.lastElementChild);
    return Array.from(colgroup.children);
  }

  function measuredWidths(headers) {
    return headers.map((th) => Math.max(MIN_COLUMN_WIDTH, Math.ceil(th.getBoundingClientRect().width)));
  }

  function applyWidths(table, widths) {
    const headers = tableHeaders(table);
    const cols = ensureColGroup(table, headers.length);
    const safeWidths = widths.length === headers.length ? widths : measuredWidths(headers);
    let total = 0;
    cols.forEach((col, index) => {
      const width = Math.max(MIN_COLUMN_WIDTH, Math.round(safeWidths[index] || MIN_COLUMN_WIDTH));
      col.style.width = `${width}px`;
      total += width;
    });
    table.style.tableLayout = 'fixed';
    table.style.minWidth = `${Math.max(total, table.parentElement?.clientWidth || total)}px`;
  }

  function resetWidths(table) {
    try {
      window.localStorage.removeItem(storageKey(table));
    } catch (_) {
      // Ignore storage failures.
    }
    table.querySelectorAll('colgroup col').forEach((col) => {
      col.style.width = '';
    });
    table.style.tableLayout = '';
    table.style.minWidth = '';
    window.requestAnimationFrame(() => applyWidths(table, measuredWidths(tableHeaders(table))));
  }

  function bindResizeHandle(table, th, index) {
    if (th.querySelector(':scope > .table-resize-handle')) return;
    const handle = document.createElement('span');
    handle.className = 'table-resize-handle';
    handle.title = 'Drag to resize column. Double-click to reset this table.';
    handle.setAttribute('aria-hidden', 'true');
    th.appendChild(handle);

    handle.addEventListener('dblclick', (event) => {
      event.preventDefault();
      event.stopPropagation();
      resetWidths(table);
    });

    handle.addEventListener('pointerdown', (event) => {
      event.preventDefault();
      event.stopPropagation();
      const cols = ensureColGroup(table, tableHeaders(table).length);
      const startX = event.clientX;
      const startWidth = Number.parseFloat(cols[index]?.style.width || '') || th.getBoundingClientRect().width;
      document.body.classList.add('table-resizing');
      handle.setPointerCapture?.(event.pointerId);

      function onMove(moveEvent) {
        const nextWidth = Math.max(MIN_COLUMN_WIDTH, Math.round(startWidth + moveEvent.clientX - startX));
        cols[index].style.width = `${nextWidth}px`;
        const total = Array.from(cols).reduce((sum, col) => sum + (Number.parseFloat(col.style.width || '0') || MIN_COLUMN_WIDTH), 0);
        table.style.minWidth = `${Math.max(total, table.parentElement?.clientWidth || total)}px`;
      }

      function onUp() {
        document.body.classList.remove('table-resizing');
        writeStoredWidths(table);
        window.removeEventListener('pointermove', onMove);
        window.removeEventListener('pointerup', onUp);
        window.removeEventListener('pointercancel', onUp);
      }

      window.addEventListener('pointermove', onMove);
      window.addEventListener('pointerup', onUp, { once: true });
      window.addEventListener('pointercancel', onUp, { once: true });
    });
  }

  function enhanceTable(table) {
    if (!(table instanceof HTMLTableElement)) return;
    if (table.dataset.tableEnhanced === '1') return;
    const headers = tableHeaders(table);
    if (!headers.length) return;
    ensureTableWrap(table);
    table.dataset.tableEnhanced = '1';
    table.classList.add('resizable-table');
    window.requestAnimationFrame(() => {
      applyWidths(table, readStoredWidths(table));
      tableHeaders(table).forEach((th, index) => bindResizeHandle(table, th, index));
    });
  }

  function enhance(root = document) {
    const scope = root instanceof Element || root instanceof Document ? root : document;
    scope.querySelectorAll('table').forEach(enhanceTable);
  }

  function start() {
    enhance(document);
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        mutation.addedNodes.forEach((node) => {
          if (!(node instanceof Element)) return;
          if (node.matches('table')) enhanceTable(node);
          enhance(node);
        });
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start, { once: true });
  } else {
    start();
  }

  window.MegaVPNTableEnhancer = { enhance };
})(window, document);
