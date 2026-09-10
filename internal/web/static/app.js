(function () {
  const csrf = document.querySelector('meta[name="atlas-csrf-token"]')?.content || '';
  const messages = document.querySelector('#atlas-i18n')?.dataset || {};

  function message(name, fallback, values) {
    const raw = messages[name] || fallback;
    if (!values) return raw;
    return raw.replace(/\{(\w+)\}/g, (match, key) => values[key] ?? match);
  }

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
      const onBoard = document.body.dataset.page === 'board';
      if (onBoard && event.key === 'n' && !event.metaKey && !event.ctrlKey && event.target === document.body) {
        window.location.href = '/board?new=1';
      }
      if (onBoard && event.key === '/' && event.target === document.body) {
        event.preventDefault();
        document.querySelector('input[type="search"]')?.focus();
      }
    });
  }

  function setupDialogs() {
    document.querySelectorAll('[data-dialog-open]').forEach((opener) => {
      opener.addEventListener('click', (event) => {
        const dialog = document.getElementById(opener.dataset.dialogOpen);
        if (!dialog?.showModal) return;
        event.preventDefault();
        dialog.showModal();
        dialog.querySelector('input:not([type="hidden"])')?.focus();
      });
    });
    document.querySelectorAll('[data-dialog-close]').forEach((closer) => {
      closer.addEventListener('click', (event) => {
        const dialog = closer.closest('dialog');
        if (!dialog) return;
        event.preventDefault();
        dialog.close();
        if (window.location.search.includes('new_project=')) {
          window.history.replaceState({}, '', '/');
        }
      });
    });
    document.querySelectorAll('dialog[open]').forEach((dialog) => {
      if (!dialog.showModal) return;
      dialog.close();
      dialog.showModal();
      dialog.querySelector('input:not([type="hidden"])')?.focus();
    });
    document.querySelectorAll('form[data-confirm]').forEach((form) => {
      form.addEventListener('submit', (event) => {
        if (!window.confirm(form.dataset.confirm)) {
          event.preventDefault();
        }
      });
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
  let previewTimer = 0;
  let previewCard = null;
  let cardPreview = null;
  const previewBoundCards = new WeakSet();

  function sleep(ms) {
    return new Promise((resolve) => window.setTimeout(resolve, ms));
  }

  async function waitForDragEnd() {
    while (dragsInFlight > 0) {
      await sleep(150);
    }
  }

  function prefersReducedMotion() {
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  }

  function restartMotionClass(element, className) {
    if (!element || prefersReducedMotion()) return;
    element.classList.remove(className);
    window.requestAnimationFrame(() => {
      if (!document.contains(element)) return;
      const clear = () => {
        element.classList.remove(className);
        element.removeEventListener('animationend', clear);
        element.removeEventListener('animationcancel', clear);
      };
      element.addEventListener('animationend', clear);
      element.addEventListener('animationcancel', clear);
      element.classList.add(className);
    });
  }

  function captureBoardMotion(grid) {
    const cardRects = new Map();
    const columnCounts = new Map();
    grid?.querySelectorAll('.ticket-card[data-ticket-id]').forEach((card) => {
      const rect = card.getBoundingClientRect();
      cardRects.set(card.dataset.ticketId, { left: rect.left, top: rect.top });
    });
    grid?.querySelectorAll('.column[data-status]').forEach((column) => {
      const count = column.querySelector('.col-count');
      if (count) columnCounts.set(column.dataset.status, count.textContent.trim());
    });
    return { cardRects, columnCounts };
  }

  function playBoardSwapMotion(previous, grid) {
    if (!previous || !grid || dragsInFlight > 0 || prefersReducedMotion()) return;
    const easing = window.getComputedStyle(document.documentElement)
      .getPropertyValue('--ease').trim() || 'ease-out';

    grid.querySelectorAll('.ticket-card[data-ticket-id]').forEach((card) => {
      const first = previous.cardRects.get(card.dataset.ticketId);
      if (!first || typeof card.animate !== 'function') return;
      const last = card.getBoundingClientRect();
      const x = first.left - last.left;
      const y = first.top - last.top;
      if (Math.abs(x) < 1 && Math.abs(y) < 1) return;
      card.animate(
        [
          { transform: `translate(${x}px, ${y}px)` },
          { transform: 'translate(0, 0)' }
        ],
        { duration: 180, easing }
      );
    });

    grid.querySelectorAll('.column[data-status]').forEach((column) => {
      const count = column.querySelector('.col-count');
      const oldCount = previous.columnCounts.get(column.dataset.status);
      if (count && oldCount !== undefined && oldCount !== count.textContent.trim()) {
        restartMotionClass(count, 'is-count-pulsing');
      }
    });
  }

  function settleDroppedCard(card) {
    restartMotionClass(card, 'is-drop-settling');
  }

  function ensureCardPreview() {
    if (cardPreview) return cardPreview;
    cardPreview = document.createElement('aside');
    cardPreview.className = 'card-preview';
    cardPreview.setAttribute('role', 'tooltip');
    cardPreview.setAttribute('aria-hidden', 'true');
    cardPreview.hidden = true;
    document.body.appendChild(cardPreview);
    return cardPreview;
  }

  function dismissCardPreview() {
    window.clearTimeout(previewTimer);
    previewTimer = 0;
    previewCard = null;
    if (!cardPreview) return;
    cardPreview.classList.remove('is-visible');
    cardPreview.hidden = true;
    cardPreview.setAttribute('aria-hidden', 'true');
  }

  function addPreviewRow(list, label, value) {
    const term = document.createElement('dt');
    term.textContent = label;
    const detail = document.createElement('dd');
    detail.textContent = value || message('none', 'None');
    list.append(term, detail);
  }

  function countLabel(value, singularKey, pluralKey, singular, plural) {
    const count = Number.parseInt(value || '0', 10) || 0;
    const label = count === 1
      ? message(singularKey, singular)
      : message(pluralKey, plural);
    return `${count} ${label}`;
  }

  function showCardPreview(card) {
    if (!document.contains(card) || card.classList.contains('sortable-chosen')) return;
    const preview = ensureCardPreview();
    const data = card.dataset;
    preview.textContent = '';

    const title = document.createElement('h2');
    title.textContent = data.title || data.ticketId;
    const status = document.createElement('p');
    status.className = 'card-preview-status';
    status.textContent = data.statusLabel || data.status || message('unknownStatus', 'Unknown status');
    const details = document.createElement('dl');
    addPreviewRow(details, message('assignee', 'Assignee'), data.assignee || message('unassigned', 'Unassigned'));
    addPreviewRow(details, message('reviewer', 'Reviewer'), data.reviewer || message('none', 'None'));
    addPreviewRow(details, message('priority', 'Priority'), data.priorityLabel || data.priority || message('none', 'None'));
    addPreviewRow(details, message('labels', 'Labels'), data.labels || message('none', 'None'));
    const counts = document.createElement('p');
    counts.className = 'card-preview-counts';
    counts.textContent = [
      countLabel(data.blockers, 'blockerOne', 'blockerOther', 'blocker', 'blockers'),
      countLabel(data.gates, 'gateOne', 'gateOther', 'gate', 'gates'),
      countLabel(data.comments, 'commentOne', 'commentOther', 'comment', 'comments')
    ].join(' · ');
    preview.append(title, status, details, counts);

    preview.hidden = false;
    preview.setAttribute('aria-hidden', 'false');
    preview.style.left = '0px';
    preview.style.top = '0px';
    const cardRect = card.getBoundingClientRect();
    const previewRect = preview.getBoundingClientRect();
    const margin = 12;
    const gap = 10;
    let left = cardRect.right + gap;
    if (left + previewRect.width > window.innerWidth - margin) {
      left = cardRect.left - previewRect.width - gap;
    }
    left = Math.min(Math.max(margin, left), window.innerWidth - previewRect.width - margin);
    const top = Math.min(
      Math.max(margin, cardRect.top),
      window.innerHeight - previewRect.height - margin
    );
    preview.style.left = `${Math.round(left)}px`;
    preview.style.top = `${Math.round(Math.max(margin, top))}px`;
    preview.classList.add('is-visible');
  }

  function setupCardPreviews() {
    document.querySelectorAll('.ticket-card').forEach((card) => {
      if (previewBoundCards.has(card)) return;
      previewBoundCards.add(card);
      card.addEventListener('mouseenter', () => {
        dismissCardPreview();
        previewCard = card;
        previewTimer = window.setTimeout(() => {
          if (previewCard === card) showCardPreview(card);
        }, 2000);
      });
      card.addEventListener('mouseleave', dismissCardPreview);
    });
  }

  function setupDrawerMotion(animateIn) {
    const drawer = document.querySelector('.detail-drawer');
    if (!drawer || drawer.dataset.motionBound) return;
    drawer.dataset.motionBound = 'true';
    drawer.classList.add('drawer--motion-ready');
    const reduceMotion = prefersReducedMotion();
    if (!animateIn || reduceMotion) {
      drawer.classList.add('drawer--open');
    } else {
      window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => drawer.classList.add('drawer--open'));
      });
    }

    drawer.querySelectorAll('.close-button').forEach((closer) => {
      closer.addEventListener('click', (event) => {
        if (reduceMotion) return;
        event.preventDefault();
        dismissCardPreview();
        drawer.classList.remove('drawer--open');
        let navigated = false;
        const navigate = () => {
          if (navigated) return;
          navigated = true;
          window.location.assign(closer.href);
        };
        drawer.addEventListener('transitionend', (transitionEvent) => {
          if (transitionEvent.propertyName === 'transform') navigate();
        }, { once: true });
        window.setTimeout(navigate, 260);
      });
    });
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
  async function refreshBoard() {
    const seq = ++refreshSeq;
    for (let attempt = 0; attempt < 6; attempt++) {
      if (seq !== refreshSeq) return;
      try {
        const response = await fetch(boardURL().toString(), { headers: { 'Accept': 'text/html' } });
        if (response.status === 401) {
          showFlash(message('sessionExpired', 'Session expired — run `tracker web serve --open` and use the new session URL'), true);
          return;
        }
        if (!response.ok) throw new Error(`board refresh got ${response.status}`);
        const html = await response.text();
        await waitForDragEnd();
        if (seq !== refreshSeq) return;
        const boardMotion = prefersReducedMotion() ? null : captureBoardMotion(document.querySelector('.board-grid'));
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
        if (sweptGrid) {
          playBoardSwapMotion(boardMotion, document.querySelector('.board-grid'));
          setupSortable();
          setupCardPreviews();
        }
        if (sweptDrawer) {
          setupTabs();
          setupDrawerMotion(false);
        }
        return;
      } catch (err) {
        if (attempt < 5) {
          await sleep(2000 * (attempt + 1));
        }
      }
    }
    if (seq === refreshSeq) {
      showFlash(message('stale', 'Board may be out of date — could not reach the server'), true);
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
        draggable: '.ticket-card',
        animation: 150,
        sort: false,
        ghostClass: 'sortable-ghost',
        chosenClass: 'sortable-chosen',
        dragClass: 'sortable-dragging',
        onStart: () => {
          dismissCardPreview();
          dragsInFlight++;
        },
        onEnd: (event) => {
          dragsInFlight = Math.max(0, dragsInFlight - 1);
          settleDroppedCard(event.item);
        },
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
              showFlash(data.error?.message || message('moveFailedStatus', 'Move failed with {status}', { status: response.status }), true);
              refreshBoard();
              return;
            }
            showFlash(data.payload?.flash || message('updated', 'updated {id}', { id: ticketID }), false);
            refreshBoard();
          } catch (err) {
            revertCard(event);
            showFlash(err.message || message('moveFailed', 'Move failed'), true);
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

  function revealSelectedScheduleDay() {
    if (!window.matchMedia('(max-width: 760px)').matches) return;
    const selected = document.querySelector('.schedule-day[aria-current="date"]');
    selected?.scrollIntoView({ behavior: 'instant', block: 'nearest', inline: 'center' });
  }

  setupTabs();
  setupKeyboardHints();
  setupDialogs();
  setupSortable();
  setupCardPreviews();
  const drawerParams = new URLSearchParams(window.location.search);
  setupDrawerMotion(drawerParams.has('ticket') || drawerParams.has('new'));
  collapseFiltersOnMobile();
  revealDetailOnMobile();
  revealSelectedScheduleDay();

  document.addEventListener('dragstart', dismissCardPreview, true);
  document.addEventListener('scroll', dismissCardPreview, true);
  window.addEventListener('resize', dismissCardPreview);
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') dismissCardPreview();
  });

  // programmatic refresh for QA tooling and agent-driven browsers
  window.atlasBoard = { refresh: refreshBoard, dismissPreview: dismissCardPreview };
})();
