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

  // Refresh concurrency model: a generation counter makes every new refresh
  // supersede older in-flight/retrying ones (no stale snapshot or stale
  // flash can land late), and swaps wait for any active drag to finish so
  // the grid is never pulled out from under the pointer.
  let refreshSeq = 0;
  let dragsInFlight = 0;
  let boundSortables = [];

  function sleep(ms) {
    return new Promise((resolve) => window.setTimeout(resolve, ms));
  }

  async function waitForDragEnd() {
    while (dragsInFlight > 0) {
      await sleep(150);
    }
  }

  function drawerSafeToSwap() {
    // never touch a drawer that is echoing a rejected form or holding any
    // user state — typed text, a changed select (the Move dropdown!), a
    // toggled checkbox, or an expanded panel; only sync when the URL pins
    // the ticket
    if (!new URL(window.location.href).searchParams.has('ticket')) return false;
    const drawer = document.querySelector('.detail-drawer');
    if (!drawer || drawer.dataset.formEcho) return false;
    const fieldsDirty = Array.from(drawer.querySelectorAll('textarea, input:not([type="hidden"])'))
      .some((el) => (el.type === 'checkbox' || el.type === 'radio')
        ? el.checked !== el.defaultChecked
        : el.value !== el.defaultValue);
    const selectsDirty = Array.from(drawer.querySelectorAll('select')).some((sel) => {
      let initial = Array.from(sel.options).findIndex((opt) => opt.defaultSelected);
      if (initial < 0) initial = 0; // browsers select the first option by default
      return sel.selectedIndex !== initial;
    });
    const panelsOpen = Array.from(drawer.querySelectorAll('details')).some((panel) => panel.open);
    return !fieldsDirty && !selectsDirty && !panelsOpen;
  }

  // Sync with the server WITHOUT navigating: fetch the board page and swap
  // in the fresh grid (and drawer, when provably safe). Typed input, filter
  // fields, and the flash survive; unreachable servers are retried with
  // backoff so a committed-but-unacknowledged move still converges.
  async function refreshBoard(message, isError) {
    const seq = ++refreshSeq;
    for (let attempt = 0; attempt < 6; attempt++) {
      if (seq !== refreshSeq) return;
      try {
        const response = await fetch(boardURL().toString(), { headers: { 'Accept': 'text/html' } });
        if (response.status === 401) {
          showFlash('Session expired — run `tracker web serve --open` and use the new session URL', true);
          return;
        }
        if (!response.ok) throw new Error(`board refresh got ${response.status}`);
        const html = await response.text();
        await waitForDragEnd();
        if (seq !== refreshSeq) return;
        const doc = new DOMParser().parseFromString(html, 'text/html');
        const selectors = ['.board-grid', '.mobile-columns'];
        if (drawerSafeToSwap()) selectors.push('.detail-drawer');
        let sweptGrid = false;
        let sweptDrawer = false;
        selectors.forEach((selector) => {
          const next = doc.querySelector(selector);
          const current = document.querySelector(selector);
          if (next && current) {
            current.replaceWith(next);
            if (selector === '.board-grid') sweptGrid = true;
            if (selector === '.detail-drawer') sweptDrawer = true;
          }
        });
        if (sweptGrid) setupSortable();
        if (sweptDrawer) setupTabs();
        if (message) showFlash(message, isError);
        return;
      } catch (err) {
        if (attempt < 5) {
          await sleep(2000 * (attempt + 1));
        }
      }
    }
    if (seq === refreshSeq) {
      showFlash('Board may be out of date — could not reach the server', true);
    }
  }

  function setupSortable() {
    if (!window.Sortable) return;
    // destroy instances bound to grids that replaceWith detached, or every
    // refresh leaks a full board subtree in long-lived tabs
    boundSortables.forEach((instance) => {
      try {
        instance.destroy();
      } catch (err) {
        console.debug('sortable destroy during rebind:', err);
      }
    });
    boundSortables = [];
    document.querySelectorAll('.ticket-list').forEach((list) => {
      boundSortables.push(window.Sortable.create(list, {
        group: 'atlas-board',
        handle: '.drag-handle',
        animation: 120,
        sort: false,
        ghostClass: 'sortable-ghost',
        onStart: () => { dragsInFlight++; },
        onEnd: () => { dragsInFlight = Math.max(0, dragsInFlight - 1); },
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
            const data = await response.json().catch(() => ({}));
            if (!response.ok) {
              // feedback first — the resync may take a while or fail
              revertCard(event);
              showFlash(data.error?.message || `Move failed with ${response.status}`, true);
              refreshBoard();
              return;
            }
            showFlash(data.payload?.flash || `updated ${ticketID}`, false);
            refreshBoard();
          } catch (err) {
            revertCard(event);
            showFlash(err.message || 'Move failed', true);
            refreshBoard();
          }
        }
      }));
    });
  }

  function collapseFiltersOnMobile() {
    // filters ship expanded (no-JS fallback); on phones they eat the first
    // screen, so start them collapsed behind the summary pill
    const shell = document.querySelector('.filters-shell');
    if (shell && window.matchMedia('(max-width: 760px)').matches) {
      shell.open = false;
    }
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
  collapseFiltersOnMobile();
  revealDetailOnMobile();

  // programmatic refresh for QA tooling and agent-driven browsers
  window.atlasBoard = { refresh: refreshBoard };
})();

