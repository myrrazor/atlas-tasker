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

  function resyncWithError(message) {
    // the failed drag may mean our board is stale (e.g. moved from another
    // window) — resync after the user has had a beat to read the error, and
    // carry it in the URL so the reload doesn't eat it
    window.setTimeout(() => {
      const target = new URL(window.location.href);
      target.searchParams.set('error_flash', message);
      target.searchParams.delete('flash');
      window.location.assign(target.toString());
    }, 1500);
  }

  function reloadWithFlash(message) {
    const target = new URL(window.location.href);
    target.searchParams.set('flash', message);
    target.searchParams.delete('error_flash');
    window.location.assign(target.toString());
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
              showFlash(message, true);
              resyncWithError(message);
              return;
            }
            const data = await response.json().catch(() => ({}));
            reloadWithFlash(data.payload?.flash || `updated ${ticketID}`);
          } catch (err) {
            revertCard(event);
            showFlash(err.message || 'Move failed', true);
            resyncWithError(err.message || 'Move failed');
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
})();

