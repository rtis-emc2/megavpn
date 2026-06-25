(function (window) {
  'use strict';

  function createWebTerminal(ctx = {}) {
    const { surface, output, maxLines = 2500 } = ctx;
    if (!surface || !output) {
      throw new Error('MegaVPNWebTerminal requires surface and output elements');
    }

    let lines = [''];
    let row = 0;
    let col = 0;
    let savedRow = 0;
    let savedCol = 0;
    let pendingControl = '';
    let renderScheduled = false;

    function ensureRow(targetRow) {
      while (lines.length <= targetRow) lines.push('');
    }

    function padLine(line, width) {
      if (line.length >= width) return line;
      return line + ' '.repeat(width - line.length);
    }

    function setLine(targetRow, value) {
      ensureRow(targetRow);
      lines[targetRow] = value;
    }

    function currentLine() {
      ensureRow(row);
      return lines[row] || '';
    }

    function trimScrollback() {
      if (lines.length <= maxLines) return;
      const removeCount = lines.length - maxLines;
      lines = lines.slice(removeCount);
      row = Math.max(0, row - removeCount);
      savedRow = Math.max(0, savedRow - removeCount);
    }

    function scrollToBottom() {
      surface.scrollTop = surface.scrollHeight;
    }

    function renderNow() {
      renderScheduled = false;
      output.textContent = lines.join('\n');
      window.requestAnimationFrame(scrollToBottom);
    }

    function scheduleRender() {
      if (renderScheduled) return;
      renderScheduled = true;
      window.requestAnimationFrame(renderNow);
    }

    function newline() {
      row += 1;
      col = 0;
      ensureRow(row);
      trimScrollback();
    }

    function writePrintable(ch) {
      const line = padLine(currentLine(), col);
      setLine(row, line.slice(0, col) + ch + line.slice(col + 1));
      col += 1;
    }

    function writeTab() {
      const nextStop = col + (8 - (col % 8));
      while (col < nextStop) writePrintable(' ');
    }

    function eraseLine(mode) {
      const line = currentLine();
      switch (mode) {
      case 1:
        setLine(row, ' '.repeat(Math.min(col, line.length)) + line.slice(col));
        break;
      case 2:
        setLine(row, '');
        col = 0;
        break;
      default:
        setLine(row, line.slice(0, col));
        break;
      }
    }

    function eraseDisplay(mode) {
      if (mode === 2 || mode === 3) {
        lines = [''];
        row = 0;
        col = 0;
        return;
      }
      lines = lines.slice(0, row + 1);
      setLine(row, currentLine().slice(0, col));
    }

    function deleteChars(count) {
      const line = currentLine();
      setLine(row, line.slice(0, col) + line.slice(col + Math.max(1, count)));
    }

    function insertSpaces(count) {
      const line = padLine(currentLine(), col);
      setLine(row, line.slice(0, col) + ' '.repeat(Math.max(1, count)) + line.slice(col));
    }

    function numberParam(params, index, fallback) {
      const raw = params[index];
      if (raw == null || raw === '') return fallback;
      const value = Number(raw.replace(/^\?/, ''));
      return Number.isFinite(value) ? value : fallback;
    }

    function parseParams(body) {
      return String(body || '').split(';');
    }

    function handleCSI(body, finalChar) {
      if (body.startsWith('?')) return;
      const params = parseParams(body);
      const first = numberParam(params, 0, 0);
      switch (finalChar) {
      case 'm':
      case 'h':
      case 'l':
        return;
      case 'A':
        row = Math.max(0, row - Math.max(1, first || 1));
        return;
      case 'B':
        row += Math.max(1, first || 1);
        ensureRow(row);
        return;
      case 'C':
        col += Math.max(1, first || 1);
        return;
      case 'D':
        col = Math.max(0, col - Math.max(1, first || 1));
        return;
      case 'E':
        row += Math.max(1, first || 1);
        col = 0;
        ensureRow(row);
        return;
      case 'F':
        row = Math.max(0, row - Math.max(1, first || 1));
        col = 0;
        return;
      case 'G':
        col = Math.max(0, numberParam(params, 0, 1) - 1);
        return;
      case 'H':
      case 'f':
        row = Math.max(0, numberParam(params, 0, 1) - 1);
        col = Math.max(0, numberParam(params, 1, 1) - 1);
        ensureRow(row);
        return;
      case 'J':
        eraseDisplay(first);
        return;
      case 'K':
        eraseLine(first);
        return;
      case 'P':
        deleteChars(first || 1);
        return;
      case '@':
        insertSpaces(first || 1);
        return;
      case 's':
        savedRow = row;
        savedCol = col;
        return;
      case 'u':
        row = savedRow;
        col = savedCol;
        ensureRow(row);
        return;
      default:
        return;
      }
    }

    function consumeEscape(data, index) {
      const next = data[index + 1];
      if (!next) {
        pendingControl = data.slice(index);
        return data.length;
      }
      if (next === '[') {
        let end = index + 2;
        while (end < data.length) {
          const code = data.charCodeAt(end);
          if (code >= 0x40 && code <= 0x7e) {
            handleCSI(data.slice(index + 2, end), data[end]);
            return end + 1;
          }
          end += 1;
        }
        pendingControl = data.slice(index);
        return data.length;
      }
      if (next === ']') {
        let end = index + 2;
        while (end < data.length) {
          if (data[end] === '\x07') return end + 1;
          if (data[end] === '\x1b' && data[end + 1] === '\\') return end + 2;
          end += 1;
        }
        pendingControl = data.slice(index);
        return data.length;
      }
      if (next === '7') {
        savedRow = row;
        savedCol = col;
      } else if (next === '8') {
        row = savedRow;
        col = savedCol;
        ensureRow(row);
      }
      if (['(', ')', '*', '+'].includes(next)) {
        return Math.min(index + 3, data.length);
      }
      return Math.min(index + 2, data.length);
    }

    function write(value) {
      const data = pendingControl + String(value || '');
      pendingControl = '';
      for (let i = 0; i < data.length;) {
        const ch = data[i];
        switch (ch) {
        case '\x1b':
          i = consumeEscape(data, i);
          continue;
        case '\r':
          col = 0;
          i += 1;
          continue;
        case '\n':
          newline();
          i += 1;
          continue;
        case '\b':
        case '\x7f':
          col = Math.max(0, col - 1);
          i += 1;
          continue;
        case '\t':
          writeTab();
          i += 1;
          continue;
        case '\x00':
        case '\x07':
          i += 1;
          continue;
        default:
          if (ch < ' ') {
            i += 1;
            continue;
          }
          writePrintable(ch);
          i += 1;
          continue;
        }
      }
      trimScrollback();
      scheduleRender();
    }

    function clear() {
      lines = [''];
      row = 0;
      col = 0;
      savedRow = 0;
      savedCol = 0;
      pendingControl = '';
      renderNow();
    }

    function reset(intro = '') {
      clear();
      if (intro) write(intro);
    }

    return {
      write,
      clear,
      reset,
      scrollToBottom,
    };
  }

  window.MegaVPNWebTerminal = { create: createWebTerminal };
})(window);
