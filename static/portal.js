// SPDX-License-Identifier: AGPL-3.0-only
(() => {
  const dialog = document.querySelector('#setup-dialog');
  let opener;
  let attempts = 0;
  let timer;
  let requests = 0;
  const ready = new WeakSet();
  const delays = [2000, 5000, 15000, 30000, 60000];
  function update() {
    if (dialog && dialog.querySelector('#setup-title') && !dialog.open) dialog.showModal();
    clearTimeout(timer);
    const form = document.querySelector('[data-retry]');
    if (!form) { attempts = 0; return; }
    if (requests) return;
    // Never submit a native navigation while htmx is still initializing a form.
    if (!ready.has(form)) return;
    if (attempts >= delays.length) {
      document.querySelector('#retry-status').textContent = 'Still pending. Automatic retries paused after five attempts. Sign in again later to resume.';
      return;
    }
    timer = setTimeout(() => { attempts++; form.requestSubmit(); }, delays[attempts]);
  }
  // Observing the targets also works with htmx 4's event lifecycle and error swaps.
  new MutationObserver(update).observe(document.querySelector('#content'), {childList:true, subtree:false});
  new MutationObserver(update).observe(document.querySelector('#setup-content'), {childList:true});
  document.addEventListener('submit', event => {
    if (event.target.matches('.setup-form')) opener = event.submitter;
  });
  document.addEventListener('htmx:after:init', event => {
    if (event.target.matches?.('[data-retry]')) { ready.add(event.target); update(); }
  });
  document.addEventListener('input', event => {
    const input = event.target;
    const prefix = input.dataset.ownerPrefix;
    if (prefix && input.value.trimStart().toLowerCase().startsWith(prefix)) {
      input.value = input.value.trimStart().slice(prefix.length);
    }
  });
  // htmx 4 uses colon-separated lifecycle events. Keep enhancement errors local.
  document.addEventListener('htmx:before:request', event => {
    const form = event.detail.ctx.sourceElement;
    requests++;
    clearTimeout(timer);
    form.setAttribute('aria-busy', 'true');
    for (const button of form.querySelectorAll('button')) button.disabled = true;
  });
  document.addEventListener('htmx:finally:request', event => {
    const ctx = event.detail.ctx;
    const form = ctx.sourceElement;
    form.removeAttribute('aria-busy');
    for (const button of form.querySelectorAll('button')) button.disabled = false;
    // htmx 4 awaits the swap before finally:request. Resume only after all
    // foreground requests finish, including failures with no response to swap.
    requests = Math.max(0, requests - 1);
    update();
    if (form.matches('.setup-form') && !ctx.response) {
      const heading = document.createElement('h2');
      heading.id = 'setup-title'; heading.textContent = 'Set up LiveSync';
      const message = document.createElement('p');
      message.setAttribute('role', 'alert');
      message.textContent = 'Could not load setup. Close this dialog and try again.';
      const close = document.createElement('button');
      close.type = 'button'; close.dataset.close = ''; close.textContent = 'Close';
      document.querySelector('#setup-content').replaceChildren(heading, message, close);
    }
  });
  document.addEventListener('click', async event => {
    const button = event.target.closest('button');
    if (!button) return;
    if (button.hasAttribute('data-close')) dialog.close();
    const reveal = button.dataset.reveal;
    if (reveal) {
      const input = document.getElementById(reveal);
      const visible = input.type === 'password';
      input.type = visible ? 'text' : 'password';
      button.textContent = visible ? 'Hide' : 'Reveal';
      button.setAttribute('aria-pressed', String(visible));
    }
    if (button.dataset.copy) {
      const input = document.getElementById(button.dataset.copy);
      const status = document.querySelector('#clipboard-status');
      try {
        if (!navigator.clipboard?.writeText) throw new Error('unavailable');
        await navigator.clipboard.writeText(input.value);
        if (status.isConnected) status.textContent = 'Copied.';
      } catch {
        if (status.isConnected) status.textContent = 'Could not copy. Reveal the value and select it to copy manually.';
      }
    }
  });
  dialog.addEventListener('click', event => { if (event.target === dialog) {
    const box = dialog.getBoundingClientRect();
    if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) dialog.close();
  }});
  dialog.addEventListener('keydown', event => {
    if (event.key !== 'Tab') return;
    const controls = [...dialog.querySelectorAll('button:not(:disabled), input:not(:disabled), a[href]')].filter(el => el.getClientRects().length);
    const first = controls[0], last = controls.at(-1);
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault(); last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault(); first?.focus();
    }
  });
  dialog.addEventListener('close', () => {
    document.querySelector('#setup-content').replaceChildren();
    opener?.focus();
  });
  window.addEventListener('load', () => {
    const form = document.querySelector('[data-retry]');
    if (form) { htmx.process(form); ready.add(form); }
    update();
  });
  window.addEventListener('pagehide', () => {
    clearTimeout(timer);
    if (dialog.open) dialog.close();
    document.querySelector('#setup-content').replaceChildren();
  });
  update();
})();
