(function (window, document) {
  'use strict';

  function escapeHTML(value) {
    return String(value ?? '').replace(/[&<>'"]/g, (ch) => ({
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      "'": '&#39;',
      '"': '&quot;',
    }[ch]));
  }

  function isTextControl(control) {
    if (!(control instanceof HTMLInputElement || control instanceof HTMLTextAreaElement)) return false;
    if (control instanceof HTMLTextAreaElement) return true;
    const type = String(control.type || 'text').toLowerCase();
    return ['text', 'email', 'url', 'search', 'tel', 'number', 'datetime-local'].includes(type);
  }

  function shouldTrim(control) {
    const name = String(control.name || '').toLowerCase();
    if (!isTextControl(control)) return false;
    if (control instanceof HTMLTextAreaElement) return false;
    if (['password', 'secret', 'token', 'key', 'private', 'psk'].some((marker) => name.includes(marker))) return false;
    return !control.hasAttribute('data-preserve-whitespace');
  }

  function fieldFor(control) {
    return control.closest('.field') || control.parentElement;
  }

  function labelFor(control) {
    const field = fieldFor(control);
    return field?.querySelector('label') || null;
  }

  function ensureRequiredMarker(control) {
    const label = labelFor(control);
    if (!label) return;
    if (!control.required) {
      label.querySelector(':scope > .required-marker')?.remove();
      delete label.dataset.requiredMarked;
      return;
    }
    if (label.dataset.requiredMarked === '1') return;
    label.dataset.requiredMarked = '1';
    label.insertAdjacentHTML('beforeend', ' <span class="required-marker" aria-hidden="true">*</span>');
  }

  function controlLabel(control) {
    return labelFor(control)?.textContent?.replace('*', '').trim()
      || control.getAttribute('aria-label')
      || control.name
      || 'Field';
  }

  function ensureSummary(form) {
    let summary = form.querySelector(':scope > .form-validation-summary');
    if (!summary) {
      summary = document.createElement('div');
      summary.className = 'form-validation-summary';
      summary.setAttribute('role', 'alert');
      summary.hidden = true;
      form.insertBefore(summary, form.firstChild);
    }
    return summary;
  }

  function renderValidationSummary(form) {
    const invalid = Array.from(form.elements).filter((control) => control instanceof HTMLElement && !control.disabled && control.willValidate && !control.validity.valid);
    const summary = ensureSummary(form);
    if (!invalid.length) {
      summary.hidden = true;
      summary.innerHTML = '';
      return true;
    }
    summary.hidden = false;
    summary.innerHTML = `
      <strong>Check the highlighted fields.</strong>
      <ul>${invalid.slice(0, 8).map((control) => `<li>${escapeHTML(controlLabel(control))}: ${escapeHTML(control.validationMessage || 'Invalid value')}</li>`).join('')}</ul>`;
    invalid[0]?.focus?.({ preventScroll: false });
    return false;
  }

  function refreshInvalidState(form) {
    Array.from(form.elements).forEach((control) => {
      if (!(control instanceof HTMLElement) || !control.willValidate) return;
      control.classList.toggle('is-invalid', !control.disabled && control.value !== '' && !control.validity.valid);
    });
  }

  function enhanceForm(form) {
    if (!(form instanceof HTMLFormElement)) return;
    const firstRun = form.dataset.formEnhanced !== '1';
    form.dataset.formEnhanced = '1';
    form.classList.add('enhanced-form');
    Array.from(form.elements).forEach((control) => {
      if (!(control instanceof HTMLElement)) return;
      ensureRequiredMarker(control);
      if (control instanceof HTMLInputElement && control.type === 'number') {
        control.inputMode = control.step && String(control.step).includes('.') ? 'decimal' : 'numeric';
      }
      if (!firstRun) return;
      control.addEventListener('input', () => refreshInvalidState(form));
      control.addEventListener('change', () => refreshInvalidState(form));
    });
    if (!firstRun) return;
    form.addEventListener('submit', (event) => {
      Array.from(form.elements).forEach((control) => {
        if (shouldTrim(control)) control.value = control.value.trim();
      });
      refreshInvalidState(form);
      if (!form.checkValidity()) {
        event.preventDefault();
        event.stopPropagation();
        renderValidationSummary(form);
        form.reportValidity();
      }
    }, true);
  }

  function enhance(root = document) {
    const scope = root instanceof Element || root instanceof Document ? root : document;
    if (scope instanceof HTMLFormElement) enhanceForm(scope);
    scope.querySelectorAll('form').forEach(enhanceForm);
  }

  function start() {
    enhance(document);
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        mutation.addedNodes.forEach((node) => {
          if (!(node instanceof Element)) return;
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

  window.MegaVPNFormEnhancer = { enhance };
})(window, document);
