(function () {
  const csrf = document.querySelector('meta[name="atlas-csrf-token"]')?.content || '';

  function showFlash(message, isError) {
    let flash = document.querySelector('.flash');
    if (!flash) {
      flash = document.createElement('div');
      flash.className = 'flash';
      flash.setAttribute('role', 'status');
      document.querySelector('.board-shell')?.prepend(flash);
    }
    flash.className = isError ? 'flash error' : 'flash';
    flash.textContent = message;
  }

  function setupTabs() {
    document.querySelectorAll('.tabs button').forEach((button) => {
      button.addEventListener('click', () => {
        const target = button.dataset.tab;
        document.querySelectorAll('.tabs button').forEach((item) => item.classList.toggle('active', item === button));
        document.querySelectorAll('.tab-panel').forEach((panel) => panel.classList.toggle('active', panel.id === `tab-${target}`));
      });
    });
  }

  function setupKeyboardHints() {
    document.addEventListener('keydown', (event) => {
      if (event.key === 'n' && !event.metaKey && !event.ctrlKey && event.target === document.body) {
        window.location.href = '/board?new=1';
      }
      if (event.key === '/' && event.target === document.body) {
        event.preventDefault();
        document.querySelector('input[type="search"]')?.focus();
      }
    });
  }

  function revertCard(event) {
    // put the card back where the drag started; reloading would wipe the flash
    const siblings = event.from.children;
    if (event.oldIndex >= siblings.length) {
      event.from.appendChild(event.item);
    } else {
      event.from.insertBefore(event.item, siblings[event.oldIndex]);
    }
  }

  function boardURL() {
    // after a rejected form post the address bar can sit on a POST-only
    // /actions/... URL — reloading that would land on a 405 text page
    if (window.location.pathname === '/board') {
      return new URL(window.location.href);
    }
    return new URL('/board', window.location.origin);
  }

  // Sync the columns with the server WITHOUT navigating: fetch the board
  // page, swap in the fresh grid, and rebind drag handlers. Nothing outside
  // the grid is touched, so typed input, open drawers, filter fields, and
  // the flash all survive — success, rejection, and network recovery share
  // this one path.
  async function refreshBoard(message, isError, attempt = 0) {
    try {
      const response = await fetch(boardURL().toString(), { headers: { 'Accept': 'text/html' } });
      if (!response.ok) throw new Error(`board refresh got ${response.status}`);
      const doc = new DOMParser().parseFromString(await response.text(), 'text/html');
      let swapped = false;
      ['.board-grid', '.mobile-columns'].forEach((selector) => {
        const next = doc.querySelector(selector);
        const current = document.querySelector(selector);
        if (next && current) {
          current.replaceWith(next);
          swapped = true;
        }
      });
      if (swapped) setupSortable();
      if (message) showFlash(message, isError);
    } catch (err) {
      // server unreachable or mid-restart: keep the board we have, retry a
      // few times so a committed-but-unacknowledged move still converges
      if (attempt < 5) {
        window.setTimeout(() => refreshBoard(message, isError, attempt + 1), 2000 * (attempt + 1));
        return;
      }
      showFlash('Board may be out of date — could not reach the server', true);
    }
  }

  function setupSortable() {
    if (!window.Sortable) return;
    document.querySelectorAll('.ticket-list').forEach((list) => {
      window.Sortable.create(list, {
        group: 'atlas-board',
        handle: '.drag-handle',
        animation: 120,
        sort: false,
        ghostClass: 'sortable-ghost',
        onAdd: async (event) => {
          const card = event.item;
          const ticketID = card.dataset.ticketId;
          const status = event.to.dataset.status;
          const body = new URLSearchParams();
          body.set('csrf_token', csrf);
          body.set('status', status);
          body.set('reason', 'web drag move');
          try {
            const response = await fetch(`/actions/tickets/${encodeURIComponent(ticketID)}/move`, {
              method: 'POST',
              headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/x-www-form-urlencoded',
                'X-Atlas-CSRF': csrf,
                'X-Atlas-Request': 'fetch'
              },
              body
            });
            if (!response.ok) {
              const data = await response.json().catch(() => ({}));
              const message = data.error?.message || `Move failed with ${response.status}`;
              revertCard(event);
              await refreshBoard(message, true);
              return;
            }
            const data = await response.json().catch(() => ({}));
            await refreshBoard(data.payload?.flash || `updated ${ticketID}`, false);
          } catch (err) {
            // the move may or may not have committed — refreshBoard retries
            // until the server answers, so the board converges either way
            revertCard(event);
            showFlash(err.message || 'Move failed', true);
            refreshBoard(err.message || 'Move failed', true);
          }
        }
      });
    });
  }

  function revealDetailOnMobile() {
    // on narrow screens the drawer renders below the board; scroll it into
    // view when a ticket was explicitly selected, otherwise taps look dead
    const params = new URLSearchParams(window.location.search);
    if (!params.has('ticket') && !params.has('new')) return;
    // 1180px = the app-shell breakpoint where the drawer stacks below the board
    if (window.matchMedia('(max-width: 1180px)').matches) {
      document.querySelector('.detail-drawer')?.scrollIntoView({ behavior: 'instant', block: 'start' });
    }
  }

  setupTabs();
  setupKeyboardHints();
  setupSortable();
  revealDetailOnMobile();

  // programmatic refresh for QA tooling and agent-driven browsers
  window.atlasBoard = { refresh: refreshBoard };
})();

