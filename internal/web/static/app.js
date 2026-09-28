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
        window.location.href = boardPath() + '?new=1';
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
        if (window.location.search.includes('new_project=') || window.location.search.includes('find=') || window.location.search.includes('init=')) {
          const url = new URL(window.location.href);
          url.searchParams.delete('new_project');
          url.searchParams.delete('find');
          url.searchParams.delete('init');
          window.history.replaceState({}, '', url.pathname + (url.search ? url.search : ''));
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

  function boardPath() {
    return document.body?.dataset?.boardPath || '/board';
  }

  function actionPrefix() {
    return document.body?.dataset?.actionPrefix || '';
  }

  function ticketHref(ticketID) {
    const url = boardURL();
    url.searchParams.delete('new');
    url.searchParams.set('ticket', ticketID);
    const query = url.searchParams.toString();
    return url.pathname + (query ? '?' + query : '');
  }

  function rewriteCardHrefs() {
    document.querySelectorAll('.ticket-card[data-ticket-id]').forEach((card) => {
      card.setAttribute('href', ticketHref(card.dataset.ticketId));
    });
  }

  function boardURL() {
    // after a rejected form post the address bar can sit on a POST-only
    // /actions/... URL — reloading that would land on a 405 text page
    if (window.location.pathname === boardPath()) {
      return new URL(window.location.href);
    }
    return new URL(boardPath(), window.location.origin);
  }

  // Refresh concurrency model: a generation counter makes every new refresh
  // supersede older in-flight/retrying ones (no stale snapshot or stale
  // flash can land late), and swaps wait for any active drag to finish so
  // the grid is never pulled out from under the pointer.
  let refreshSeq = 0;
  let dragsInFlight = 0;
  let cardPointerDown = false;
  let dragQuietUntil = 0;
  const moveTail = new Map();
  const moveGen = new Map();
  let boundSortables = [];
  let previewTimer = 0;
  let previewCard = null;
  let cardPreview = null;
  const previewBoundCards = new WeakSet();

  function sleep(ms) {
    return new Promise((resolve) => window.setTimeout(resolve, ms));
  }

  async function waitForDragEnd() {
    while (dragBlocked()) {
      await sleep(50);
    }
  }

  function dragBlocked() {
    return dragsInFlight > 0 || cardPointerDown || Date.now() < dragQuietUntil;
  }

  let pointerGuard = 0;
  document.addEventListener('pointerdown', (event) => {
    const target = event.target;
    if (!(target && target.closest && target.closest('.ticket-card'))) return;
    cardPointerDown = true;
    window.clearTimeout(pointerGuard);
    // A press that never receives pointerup (released outside the window,
    // or the tab changed) must not freeze live updates until the next click.
    pointerGuard = window.setTimeout(() => {
      if (dragsInFlight === 0) cardPointerDown = false;
    }, 2000);
  }, true);
  function clearStuckPointer() {
    if (dragsInFlight > 0) return;
    cardPointerDown = false;
    window.clearTimeout(pointerGuard);
  }
  // pointerup can beat dragstart. Keep the card frozen across that gap so a
  // live refresh cannot replace it before the drag is real.
  function releaseCardPointer() {
    window.clearTimeout(pointerGuard);
    window.setTimeout(() => {
      if (dragsInFlight === 0) cardPointerDown = false;
    }, 80);
  }
  document.addEventListener('pointerup', releaseCardPointer, true);
  document.addEventListener('pointercancel', releaseCardPointer, true);
  document.addEventListener('dragend', releaseCardPointer, true);
  window.addEventListener('blur', clearStuckPointer);
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) clearStuckPointer();
  });

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
    if (cardCount() > 200) return { cardRects, columnCounts: new Map() };
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

  let boardETag = '';

  function showServerDown() {
    const banner = document.querySelector('[data-net-banner]');
    if (!banner) return;
    banner.hidden = false;
    banner.classList.remove('is-online');
    banner.textContent = message('offline', 'The local server is unreachable. Work stays on disk; retry when the local server is running.');
  }

  function hideServerDown() {
    const banner = document.querySelector('[data-net-banner]');
    if (!banner || banner.classList.contains('is-online')) return;
    banner.hidden = true;
    banner.textContent = '';
  }

  function isNetworkError(err) {
    const text = String(err && err.message ? err.message : err || '');
    return /failed to fetch|networkerror|load failed|network request failed/i.test(text);
  }

  // Sync with the server WITHOUT navigating: fetch the board page and swap
  // in the fresh grid (and drawer, when provably safe). Typed input, filter
  // fields, and the flash survive; unreachable servers are retried with
  // backoff so a committed-but-unacknowledged move still converges.
  function applyBoardDelta(data) {
    let skipped = false;
    (data.cards || []).forEach((patch) => {
      const id = String(patch.id || '').replace(/"/g, '');
      if (!id) return;
      const existing = document.querySelector(`.ticket-card[data-ticket-id="${id}"]`);
      if (existing && (existing.classList.contains('sortable-chosen') || existing.classList.contains('sortable-ghost') || existing.classList.contains('sortable-dragging'))) {
        skipped = true;
        return;
      }
      if (patch.remove) {
        if (!existing) return;
        const list = existing.parentElement;
        existing.remove();
        if (list && !list.querySelector('.ticket-card') && !list.querySelector('.empty-column')) {
          const empty = document.createElement('div');
          empty.className = 'empty-column';
          list.appendChild(empty);
        }
        return;
      }
      if (!patch.html) return;
      const holder = document.createElement('template');
      holder.innerHTML = String(patch.html).trim();
      const next = holder.content.querySelector('.ticket-card');
      if (!next) return;
      next.setAttribute('href', ticketHref(next.dataset.ticketId));
      if (existing && existing.dataset.revision === next.dataset.revision && existing.dataset.status === next.dataset.status && existing.dataset.comments === next.dataset.comments && existing.dataset.title === next.dataset.title) {
        return;
      }
      const status = next.dataset.status || patch.status;
      const list = document.querySelector(`.ticket-list[data-status="${status}"]`);
      if (existing && existing.dataset.status === status) {
        // Keep the same element. Replacing it detaches a card the pointer
        // is about to drag, which drops the drag before dragstart.
        existing.innerHTML = next.innerHTML;
        for (const attr of next.attributes) existing.setAttribute(attr.name, attr.value);
      } else {
        if (existing) existing.remove();
        if (list) {
          const empty = list.querySelector('.empty-column');
          if (empty) empty.remove();
          list.prepend(next);
        }
      }
    });
    syncColumnCounts();
    setupCardPreviews();
    const safe = drawerSafeToSwap();
    if (safe && data.comments_html) {
      const thread = document.querySelector('[data-comment-thread]');
      if (thread) {
        const holder = document.createElement('template');
        holder.innerHTML = String(data.comments_html).trim();
        const next = holder.content.querySelector('[data-comment-thread]');
        if (next) thread.replaceWith(next);
      }
    }
    if (safe && data.history_html) {
      const history = document.querySelector('[data-history-list]');
      if (history) {
        const holder = document.createElement('template');
        holder.innerHTML = String(data.history_html).trim();
        const next = holder.content.querySelector('[data-history-list]');
        if (next) history.replaceWith(next);
      }
    }
    const commentBlocked = (data.comments_html || data.history_html) && !safe;
    // A large patch must still advance the stamp. Holding it for a dirty
    // drawer re-downloads the whole burst on every poll.
    if (commentBlocked && !data.resync && (data.cards || []).length <= 40) skipped = true;
    return !skipped;
  }

  function controlDirty(el) {
    if (!el) return true;
    if (el.tagName === 'SELECT') {
      let initial = Array.from(el.options).findIndex((opt) => opt.defaultSelected);
      if (initial < 0) initial = 0;
      return el.selectedIndex !== initial;
    }
    if (el.type === 'checkbox' || el.type === 'radio') return el.checked !== el.defaultChecked;
    return el.value !== el.defaultValue;
  }

  function setIfClean(el, value) {
    if (!el || controlDirty(el)) return;
    const next = value == null ? '' : String(value);
    if (el.tagName === 'SELECT') {
      el.value = next;
      Array.from(el.options).forEach((opt) => { opt.defaultSelected = opt.value === next; });
      return;
    }
    el.value = next;
    el.defaultValue = next;
  }

  function applyDrawerLive(drawer) {
    if (!drawer || !drawer.id) return;
    if (new URL(window.location.href).searchParams.get('ticket') !== drawer.id) return;
    const root = document.querySelector('.detail-drawer');
    if (!root || root.dataset.formEcho) return;
    const heading = root.querySelector('.drawer-head h1');
    if (heading && drawer.title) heading.textContent = drawer.title;
    const pill = root.querySelector('.drawer-meta .status-pill');
    if (pill && drawer.status) {
      pill.className = 'status-pill st-' + drawer.status;
      if (drawer.status_label) pill.textContent = drawer.status_label;
    }
    const edit = root.querySelector('form[action$="/edit"]');
    if (edit) {
      setIfClean(edit.querySelector('[name="title"]'), drawer.title);
      setIfClean(edit.querySelector('[name="description"]'), drawer.description);
      setIfClean(edit.querySelector('[name="notes"]'), drawer.notes);
      setIfClean(edit.querySelector('[name="acceptance"]'), drawer.acceptance);
      setIfClean(edit.querySelector('[name="priority"]'), drawer.priority);
      setIfClean(edit.querySelector('[name="assignee"]'), drawer.assignee);
      setIfClean(edit.querySelector('[name="reviewer"]'), drawer.reviewer);
      setIfClean(edit.querySelector('[name="labels"]'), drawer.labels);
    }
    root.querySelectorAll('input[name="expected_revision"]').forEach((rev) => {
      setIfClean(rev, drawer.revision);
    });
    const descInput = edit && edit.querySelector('[name="description"]');
    const prose = root.querySelector('[data-drawer-description]');
    if (prose && (!descInput || !controlDirty(descInput))) {
      prose.textContent = drawer.description || '';
    }
    const notesInput = edit && edit.querySelector('[name="notes"]');
    const notes = root.querySelector('[data-drawer-notes]');
    if (notes && (!notesInput || !controlDirty(notesInput))) {
      notes.textContent = drawer.notes || '';
    }
  }

  function applyBoardResync(data) {
    const dragging = document.querySelector('.ticket-card.sortable-chosen, .ticket-card.sortable-ghost, .ticket-card.sortable-dragging');
    if (dragging) return false;
    const byStatus = new Map();
    (data.cards || []).forEach((patch) => {
      const status = String(patch.status || '');
      if (!status) return;
      if (!byStatus.has(status)) byStatus.set(status, []);
      byStatus.get(status).push(patch);
    });
    document.querySelectorAll('.ticket-list[data-status]').forEach((list) => {
      const patches = byStatus.get(list.dataset.status) || [];
      const frag = document.createDocumentFragment();
      patches.forEach((patch) => {
        if (!patch.html) return;
        const holder = document.createElement('template');
        holder.innerHTML = String(patch.html).trim();
        const next = holder.content.querySelector('.ticket-card');
        if (!next) return;
        next.setAttribute('href', ticketHref(next.dataset.ticketId));
        frag.appendChild(next);
      });
      if (!frag.childNodes.length) {
        list.replaceChildren();
        const empty = document.createElement('div');
        empty.className = 'empty-column';
        list.appendChild(empty);
      } else {
        list.replaceChildren(frag);
      }
    });
    syncColumnCounts();
    setupSortable();
    setupCardPreviews();
    rewriteCardHrefs();
    return true;
  }

  async function refreshBoard(opts) {
    const attempts = (opts && opts.attempts) || 6;
    const quiet = !!(opts && opts.quiet);
    const seq = ++refreshSeq;
    if (quiet && dragBlocked()) return;
    for (let attempt = 0; attempt < attempts; attempt++) {
      if (seq !== refreshSeq) return;
      try {
        const headers = { 'Accept': 'text/html' };
        if (boardETag) headers['If-None-Match'] = boardETag;
        headers['X-Atlas-Live'] = '1';
        const response = await fetch(boardURL().toString(), { headers });
        if (response.status === 304) {
          hideServerDown();
          return;
        }
        if (response.status === 401) {
          let text = '';
          try {
            text = (await response.text()).trim();
          } catch (err) {}
          if (!text || text.length > 240 || text.indexOf('<') !== -1) {
            text = message('sessionExpired', 'Session expired. Run tracker in a terminal on this computer to sign in again.');
          }
          showFlash(text, true);
          return;
        }
        if (!response.ok && response.status !== 404) throw new Error(`board refresh got ${response.status}`);
        const etag = response.headers && typeof response.headers.get === 'function' ? response.headers.get('ETag') : '';
        const ctype = response.headers && typeof response.headers.get === 'function' ? (response.headers.get('Content-Type') || '') : '';
        if (ctype.indexOf('application/json') !== -1) {
          const data = await response.json();
          hideServerDown();
          if (dragBlocked() || seq !== refreshSeq) return;
          const applied = data.resync ? applyBoardResync(data) : applyBoardDelta(data);
          applyDrawerLive(data.drawer);
          if (applied && etag) boardETag = etag;
          return;
        }
        const html = await response.text();
        hideServerDown();
        await waitForDragEnd();
        if (dragBlocked() || seq !== refreshSeq) return;
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
          rewriteCardHrefs();
          rememberTicketFocus();
        }
        if (sweptDrawer) {
          setupTabs();
          setupDrawerMotion(false);
        }
        if (etag) boardETag = etag;
        return;
      } catch (err) {
        if (attempt < attempts - 1) {
          await sleep(2000 * (attempt + 1));
          continue;
        }
        if (quiet) throw err;
        if (isNetworkError(err)) showServerDown();
        if (seq === refreshSeq) {
          showFlash(message('stale', 'Board may be out of date — could not reach the server'), true);
        }
      }
    }
  }

  function syncColumnCounts() {
    document.querySelectorAll('.column[data-status]').forEach((column) => {
      const count = column.querySelectorAll('.ticket-list .ticket-card').length;
      const badge = column.querySelector('.col-count');
      if (badge) badge.textContent = String(count);
    });
  }

  function cardCount() {
    return document.querySelectorAll('.ticket-card').length;
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
        animation: cardCount() > 200 ? 0 : 150,
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
          cardPointerDown = false;
          dragQuietUntil = Date.now() + 400;
          settleDroppedCard(event.item);
        },
        onAdd: (event) => {
          const card = event.item;
          const ticketID = card.dataset.ticketId;
          const status = event.to.dataset.status;
          const gen = (moveGen.get(ticketID) || 0) + 1;
          moveGen.set(ticketID, gen);
          const prev = moveTail.get(ticketID) || Promise.resolve();
          const run = prev.catch(() => {}).then(async () => {
            // A newer drop already happened. Send only the latest column,
            // with the revision the previous request stored.
            if (moveGen.get(ticketID) !== gen) return;
            const live = document.querySelector(`.ticket-card[data-ticket-id="${String(ticketID).replace(/"/g, '')}"]`) || card;
            const body = new URLSearchParams();
            body.set('csrf_token', csrf);
            body.set('status', status);
            body.set('reason', 'web drag move');
            if (live.dataset.revision) body.set('expected_revision', live.dataset.revision);
            try {
              const response = await fetch(`${actionPrefix()}/actions/tickets/${encodeURIComponent(ticketID)}/move`, {
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
              const current = document.querySelector(`.ticket-card[data-ticket-id="${String(ticketID).replace(/"/g, '')}"]`) || live;
              if (response.ok) {
                if (data.payload?.revision) current.dataset.revision = data.payload.revision;
                if (data.payload?.status) current.dataset.status = data.payload.status;
              }
              // A later drop owns the card. Keep the revision, skip the revert.
              if (moveGen.get(ticketID) !== gen) return;
              if (!response.ok) {
                revertCard(event);
                const conflict = response.status === 409;
                showFlash(
                  data.error?.message || (conflict
                    ? message('conflict', 'Someone else changed this ticket. Reload and retry with the current revision.')
                    : message('moveFailedStatus', 'Move failed with {status}', { status: response.status })),
                  true
                );
                refreshBoard();
                return;
              }
              showFlash(data.payload?.flash || message('updated', 'updated {id}', { id: ticketID }), false);
              if (data.payload?.revision) live.dataset.revision = data.payload.revision;
              if (data.payload?.status) live.dataset.status = data.payload.status;
              if (cardCount() > 200) {
                syncColumnCounts();
              } else {
                refreshBoard();
              }
            } catch (err) {
              if (moveGen.get(ticketID) !== gen) return;
              revertCard(event);
              if (isNetworkError(err)) {
                showServerDown();
                showFlash(message('offline', 'The local server is unreachable. Work stays on disk; retry when the local server is running.'), true);
                return;
              }
              showFlash(err.message || message('moveFailed', 'Move failed'), true);
              refreshBoard();
            }
          });
          moveTail.set(ticketID, run);
        }
      }));
    });
  }

  function collapseFiltersOnMobile() {
    if (window.location.search.includes('filters=1')) return;
    document.querySelectorAll('.filters-shell').forEach((shell) => {
      shell.open = false;
    });
  }

  function setupNetworkBanner() {
    const banner = document.querySelector('[data-net-banner]');
    if (!banner) return;
    const set = (online) => {
      if (online) {
        banner.hidden = true;
        banner.classList.remove('is-online');
        banner.textContent = '';
        return;
      }
      banner.hidden = false;
      banner.classList.remove('is-online');
      banner.textContent = message('offline', 'The local server is unreachable. Work stays on disk; retry when the local server is running.');
    };
    set(window.navigator.onLine);
    window.addEventListener('offline', () => set(false));
    window.addEventListener('online', () => {
      banner.hidden = false;
      banner.classList.add('is-online');
      banner.textContent = message('online', 'Back online.');
      window.setTimeout(() => set(true), 1600);
    });
  }

  function setupDrawerEscape() {
    document.addEventListener('keydown', (event) => {
      if (event.key !== 'Escape') return;
      const closer = document.querySelector('.detail-drawer .close-button');
      if (!closer || event.target.closest('dialog')) return;
      if (document.body.dataset.page !== 'board') return;
      event.preventDefault();
      closer.click();
    });
  }

  function rememberTicketFocus() {
    document.querySelectorAll('.ticket-card[data-ticket-id]').forEach((card) => {
      card.addEventListener('click', () => {
        try { window.sessionStorage.setItem('atlas-last-ticket', card.dataset.ticketId); } catch (err) {}
      });
    });
  }

  function restoreTicketFocus() {
    const params = new URLSearchParams(window.location.search);
    if (params.get('ticket') || params.get('new')) return;
    let id = '';
    try { id = window.sessionStorage.getItem('atlas-last-ticket') || ''; } catch (err) {}
    if (!id) return;
    const card = document.querySelector(`.ticket-card[data-ticket-id="${id.replace(/"/g, '')}"]`);
    card?.focus({ preventScroll: false });
  }

  function prepareOpenDrawer() {
    const drawer = document.querySelector('.detail-drawer[data-open]');
    if (!drawer) return;
    drawer.scrollTop = 0;
    const heading = drawer.querySelector('.drawer-head h1');
    const closer = drawer.querySelector('.close-button');
    heading?.setAttribute('tabindex', '-1');
    (closer || heading)?.focus({ preventScroll: true });
  }

  function flashFromHTML(html) {
    const doc = new DOMParser().parseFromString(html || '', 'text/html');
    const node = doc.querySelector('.flash.error, .flash[role="alert"]');
    return (node?.textContent || '').trim();
  }

  function setupResilientForms() {
    document.addEventListener('submit', async (event) => {
      const form = event.target;
      if (!(form instanceof HTMLFormElement)) return;
      if ((form.getAttribute('method') || '').toLowerCase() !== 'post') return;
      const page = document.body?.dataset?.page || '';
      if (page !== 'board' && page !== 'welcome' && page !== 'schedule') return;
      if (event.defaultPrevented) return;
      event.preventDefault();
      form.classList.add('is-saving');
      form.querySelectorAll('button[type="submit"], button:not([type])').forEach((button) => {
        button.disabled = true;
      });
      const body = new URLSearchParams(new FormData(form));
      const release = () => {
        form.classList.remove('is-saving');
        form.querySelectorAll('button[type="submit"], button:not([type])').forEach((button) => {
          button.disabled = false;
        });
      };
      try {
        const response = await fetch(form.action, {
          method: 'POST',
          headers: {
            'Accept': 'text/html',
            'Content-Type': 'application/x-www-form-urlencoded',
            'X-Atlas-CSRF': csrf
          },
          body,
          redirect: 'follow'
        });
        if (!response.ok) {
          const text = flashFromHTML(await response.text()) || `Save failed (${response.status})`;
          showFlash(text, true);
          release();
          return;
        }
        window.location.assign(response.url || boardURL().toString());
      } catch (err) {
        showServerDown();
        showFlash(message('offline', 'The local server is unreachable. Work stays on disk; retry when the local server is running.'), true);
        release();
      }
    });
  }

  window.addEventListener('pageshow', (event) => {
    const nav = window.performance && typeof window.performance.getEntriesByType === 'function'
      ? window.performance.getEntriesByType('navigation')[0]
      : null;
    const back = event.persisted || (nav && nav.type === 'back_forward');
    if (!back) return;
    document.querySelectorAll('form[action*="/tickets/create"]').forEach((form) => {
      form.reset();
    });
  });

  function startLiveBoard() {
    if (document.body?.dataset?.page !== 'board') return;
    if (typeof window.setInterval !== 'function') return;
    const stamp = document.querySelector('meta[name="atlas-board-stamp"]');
    if (stamp && stamp.content) boardETag = '"' + stamp.content + '"';
    let misses = 0;
    window.setInterval(() => {
      if (document.hidden || dragBlocked()) return;
      refreshBoard({ attempts: 1, quiet: true }).then(() => {
        misses = 0;
      }).catch(() => {
        misses += 1;
        if (misses >= 2) showServerDown();
      });
    }, 3000);
  }

  function setupFormBusy() {
    document.querySelectorAll('form[method="post"]').forEach((form) => {
      form.addEventListener('submit', (event) => {
        if (event.defaultPrevented) return;
        form.classList.add('is-saving');
        form.querySelectorAll('button[type="submit"], button:not([type])').forEach((button) => {
          button.disabled = true;
        });
      });
    });
  }

  function revealSelectedScheduleDay() {
    if (!window.matchMedia('(max-width: 760px)').matches) return;
    const selected = document.querySelector('.schedule-day[aria-current="date"]');
    selected?.scrollIntoView({ behavior: 'instant', block: 'nearest', inline: 'center' });
  }

  function setupBusyForms() {
    document.querySelectorAll('form[method="post"]').forEach((form) => {
      form.addEventListener('submit', (event) => {
        if (event.defaultPrevented) return;
        form.classList.add('is-busy');
      });
    });
  }

  setupTabs();
  setupKeyboardHints();
  setupDialogs();
  setupBusyForms();
  setupSortable();
  setupCardPreviews();
  rewriteCardHrefs();
  setupNetworkBanner();
  setupDrawerEscape();
  const drawerParams = new URLSearchParams(window.location.search);
  setupDrawerMotion(drawerParams.has('ticket') || drawerParams.has('new'));
  collapseFiltersOnMobile();
  rememberTicketFocus();
  restoreTicketFocus();
  prepareOpenDrawer();
  setupFormBusy();
  setupResilientForms();
  startLiveBoard();
  revealSelectedScheduleDay();

  document.addEventListener('dragstart', dismissCardPreview, true);
  document.addEventListener('scroll', dismissCardPreview, true);
  window.addEventListener('resize', dismissCardPreview);
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') dismissCardPreview();
  });

  // programmatic refresh for QA tooling and agent-driven browsers
  window.atlasBoard = {
    refresh: refreshBoard,
    dismissPreview: dismissCardPreview,
    ticketHref,
    boardPath,
    actionPrefix
  };
})();
