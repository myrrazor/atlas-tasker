import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('../static/app.js', import.meta.url), 'utf8');

function boardHarness(responses, { extraNodes = [], location = 'http://127.0.0.1/board?project=WEB', documentOverrides = {}, windowOverrides = {} } = {}) {
  const requests = [];
  const swaps = [];
  const delays = [];
  const flash = { textContent: 'Moved WEB-1', className: 'flash' };
  const nodes = new Map([
    ['.flash', flash],
    ['#atlas-i18n', { dataset: { sessionExpired: 'Open a new board session' } }],
    ...['.board-grid', '.mobile-columns'].map(selector => [selector, {
      replaceWith(next) { swaps.push(next.selector); },
    }]),
    ...extraNodes,
  ]);
  const document = {
    body: { dataset: { page: 'board' } },
    querySelector(selector) { return nodes.get(selector) ?? null; },
    querySelectorAll() { return []; },
    addEventListener() {},
    ...documentOverrides,
  };
  const window = {
    location: new URL(location),
    matchMedia(query) { return { matches: query.includes('prefers-reduced-motion') }; },
    addEventListener() {},
    setTimeout(callback, delay) {
      delays.push(delay);
      queueMicrotask(callback);
      return delays.length;
    },
    ...windowOverrides,
  };
  const context = {
    window, document, URL, URLSearchParams, console,
    fetch: async url => {
      const response = responses[Math.min(requests.length, responses.length - 1)];
      requests.push(url);
      if (response instanceof Error) throw response;
      if (typeof response === 'object') {
        return {
          status: 200,
          ok: true,
          headers: { get(name) { return name === 'Content-Type' ? 'application/json' : '"r2"'; } },
          json: async () => response,
        };
      }
      return {
        status: response,
        ok: response === 200,
        text: async () => '<main class="board-grid"></main>',
      };
    },
    DOMParser: class {
      parseFromString() {
        return { querySelector(selector) { return { selector }; } };
      }
    },
  };
  const instrumented = script.replace(/\}\)\(\);\s*$/, 'window.testBoard = { applyDrawerLive, moveTail };\n})();');
  vm.runInNewContext(instrumented, context, { filename: 'app.js' });
  return { requests, swaps, delays, flash, refresh: window.atlasBoard.refresh, hooks: window.testBoard };
}

test('a successful refresh preserves the move confirmation without retrying', async () => {
  const board = boardHarness([200]);
  await board.refresh();
  assert.equal(board.requests.length, 1);
  assert.deepEqual(board.swaps, ['.board-grid', '.mobile-columns']);
  assert.deepEqual(board.delays, []);
  assert.deepEqual(board.flash, { textContent: 'Moved WEB-1', className: 'flash' });
});

test('an expired session shows recovery guidance without replacing the board', async () => {
  const board = boardHarness([401]);
  await board.refresh();
  assert.equal(board.requests.length, 1);
  assert.deepEqual(board.swaps, []);
  assert.deepEqual(board.delays, []);
  assert.deepEqual(board.flash, { textContent: 'Open a new board session', className: 'flash error' });
});

test('a transient failure retries and keeps the move confirmation after recovery', async () => {
  const board = boardHarness([new Error('connection lost'), 200]);
  await board.refresh();
  assert.equal(board.requests.length, 2);
  assert.equal(board.delays.length, 1);
  assert.deepEqual(board.swaps, ['.board-grid', '.mobile-columns']);
  assert.deepEqual(board.flash, { textContent: 'Moved WEB-1', className: 'flash' });
});

function input(value, type = 'text') {
  return { tagName: 'INPUT', type, value, defaultValue: value, dataset: {}, removeAttribute() {} };
}

function liveDrawerHarness({ dirtyAction = false, conflictingTitle = false, overview = false, draggable = false } = {}) {
  const editRevision = input('r1', 'hidden');
  const scheduleRevision = input('r1', 'hidden');
  const relationRevision = input('r1', 'hidden');
  const actionRevision = input('r1', 'hidden');
  const title = input('Original title');
  if (conflictingTitle) title.value = 'My draft title';
  const scheduledAt = input('2026-10-01T12:00', 'datetime-local');
  scheduledAt.value = '2026-10-01T15:00';
  const assignee = input('');
  const notesInput = input('');
  const acceptanceInput = input('');
  const notes = { textContent: '' };
  const notesSection = { hidden: true };
  const acceptance = {
    dataset: { emptyLabel: 'No acceptance criteria' }, children: [],
    replaceChildren() { this.children = []; },
    appendChild(node) { this.children.push(node); },
  };
  const acceptanceCount = { textContent: '0', hidden: true };
  if (dirtyAction) assignee.value = 'human:alice';
  const edit = {
    querySelector(selector) {
      if (selector === '[name="title"]') return title;
      if (selector === '[data-remote-changed]') return title.dataset.remoteChanged ? title : null;
      if (overview && selector === '[name="notes"]') return notesInput;
      if (overview && selector === '[name="acceptance"]') return acceptanceInput;
      return null;
    },
    querySelectorAll() { return [editRevision]; },
    contains(node) { return node === editRevision; },
  };
  const actions = {
    contains() { return false; },
    querySelectorAll(selector) {
      return selector === 'input[name="expected_revision"]' ? [actionRevision] : [assignee, actionRevision];
    },
  };
  const drawer = {
    dataset: { motionBound: 'true' },
    querySelector(selector) {
      if (selector === 'form[action$="/edit"]') return edit;
      if (selector === '.drawer-actions') return actions;
      if (overview && selector === '[data-drawer-notes]') return notes;
      if (overview && selector === '[data-drawer-notes-section]') return notesSection;
      if (overview && selector === '[data-drawer-acceptance]') return acceptance;
      if (overview && selector === '[data-drawer-acceptance-count]') return acceptanceCount;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === 'input[name="expected_revision"]') {
        return [editRevision, scheduleRevision, relationRevision, actionRevision];
      }
      if (selector === 'textarea, input:not([type="hidden"])') return [title, scheduledAt, assignee];
      return [];
    },
  };
  const card = {
    dataset: { ticketId: 'WEB-1', revision: 'r2', status: 'ready', storedStatus: 'ready' },
    addEventListener() {}, setAttribute() {},
  };
  let sortable;
  const board = boardHarness([{
    cards: [],
    drawer: { id: 'WEB-1', revision: 'r2', title: 'Remote title', status: 'ready', notes: 'Remote notes', acceptance: 'First criterion\nSecond criterion' },
  }, { payload: { status: 'in_progress', revision: 'r3' } }, { cards: [] }], {
    location: 'http://127.0.0.1/board?ticket=WEB-1',
    extraNodes: [['.detail-drawer', drawer]],
    documentOverrides: {
      createElement() { return {}; },
      querySelectorAll(selector) {
        if (selector === '.ticket-card[data-ticket-id="WEB-1"]' || selector === '.ticket-card' || selector === '.ticket-card[data-ticket-id]') return [card];
        if (draggable && selector === '.ticket-list') return [{}];
        return [];
      },
    },
    windowOverrides: draggable ? { Sortable: { create(list, options) { sortable = options; return { destroy() {} }; } } } : {},
  });
  return { ...board, editRevision, scheduleRevision, relationRevision, actionRevision, title, scheduledAt, notes, notesSection, acceptance, acceptanceCount, notesInput, acceptanceInput,
    async drag() {
      sortable.onAdd({ item: card, to: { dataset: { status: 'in_progress' } } });
      await board.hooks.moveTail.get('WEB-1');
    },
  };
}

test('a successful drag preserves stale schedule/relation guards and a conflicting edit draft', async () => {
  const board = liveDrawerHarness({ conflictingTitle: true, draggable: true });
  await board.refresh();
  await board.drag();
  assert.equal(board.requests.length >= 2, true, 'the drag submits a move');
  assert.equal(board.title.value, 'My draft title');
  assert.equal(board.editRevision.value, 'r1', 'a successful move must not authorize overwriting the remote title');
  assert.equal(board.scheduleRevision.value, 'r1', 'a stale schedule keeps its conflict guard');
  assert.equal(board.relationRevision.value, 'r1');
  assert.equal(board.actionRevision.value, 'r3', 'reconciled actions advance after the move');
});

test('a successful drag advances a reconciled edit but preserves a typed action guard', async () => {
  const board = liveDrawerHarness({ dirtyAction: true, draggable: true });
  await board.refresh();
  await board.drag();
  assert.equal(board.editRevision.value, 'r3');
  assert.equal(board.actionRevision.value, 'r1');
});

test('live overview displays initially absent notes and refreshes acceptance entries/count', async () => {
  const board = liveDrawerHarness({ overview: true });
  await board.refresh();
  assert.equal(board.notesSection.hidden, false);
  assert.equal(board.notes.textContent, 'Remote notes');
  assert.deepEqual(board.acceptance.children.map(item => item.textContent), ['First criterion', 'Second criterion']);
  assert.equal(board.acceptanceCount.textContent, '2');
  assert.equal(board.acceptanceCount.hidden, false);
  board.hooks.applyDrawerLive({ id: 'WEB-1', revision: 'r3', title: 'Remote title', notes: '', acceptance: '' });
  assert.equal(board.notesSection.hidden, true, 'cleared notes no longer display an empty section');
  assert.deepEqual(board.acceptance.children.map(item => item.textContent), ['No acceptance criteria']);
  assert.equal(board.acceptanceCount.hidden, true);
});

function resyncFocusHarness({ removed = false, focusElsewhere = false } = {}) {
  let currentCard;
  let activeElement;
  const focusCalls = [];
  const oldCard = { dataset: { ticketId: 'WEB-1' }, closest() { return this; } };
  const nextCard = {
    dataset: { ticketId: 'WEB-1' }, setAttribute() {}, addEventListener() {},
    focus(options) { focusCalls.push(options); activeElement = this; },
  };
  const unrelatedControl = { closest() { return null; } };
  activeElement = focusElsewhere ? unrelatedControl : oldCard;
  const list = {
    dataset: { status: 'ready' },
    replaceChildren(fragment) {
      currentCard = fragment?.childNodes[0] || null;
      if (activeElement === oldCard) activeElement = null;
    },
    appendChild() {},
  };
  const board = boardHarness([{ resync: true, cards: removed ? [] : [{ status: 'ready', html: '<a class="ticket-card"></a>' }] }], {
    documentOverrides: {
      get activeElement() { return activeElement; },
      querySelector(selector) { return selector === '.ticket-card[data-ticket-id="WEB-1"]' ? currentCard || null : null; },
      querySelectorAll(selector) {
        if (selector === '.ticket-list[data-status]') return [list];
        if (selector === '.ticket-card' || selector === '.ticket-card[data-ticket-id]') return currentCard ? [currentCard] : [];
        return [];
      },
      createElement(tag) {
        if (tag === 'template') return { content: { querySelector() { return nextCard; } } };
        return {};
      },
      createDocumentFragment() { return { childNodes: [], appendChild(node) { this.childNodes.push(node); } }; },
    },
  });
  return { ...board, focusCalls, get active() { return activeElement; }, nextCard, unrelatedControl };
}

test('a live resync restores keyboard focus to the same surviving ticket', async () => {
  const board = resyncFocusHarness();
  await board.refresh();
  assert.equal(board.active, board.nextCard);
  assert.equal(board.focusCalls.length, 1);
  assert.equal(board.focusCalls[0].preventScroll, true);
});

test('a live resync does not steal focus from other controls or a removed ticket', async () => {
  const elsewhere = resyncFocusHarness({ focusElsewhere: true });
  await elsewhere.refresh();
  assert.equal(elsewhere.active, elsewhere.unrelatedControl);
  assert.equal(elsewhere.focusCalls.length, 0);
  const removed = resyncFocusHarness({ removed: true });
  await removed.refresh();
  assert.equal(removed.focusCalls.length, 0);
});

test('live overview keeps dirty notes and acceptance drafts intact', async () => {
  const board = liveDrawerHarness({ overview: true });
  board.notesInput.value = 'My notes';
  board.acceptanceInput.value = 'My criterion';
  await board.refresh();
  assert.equal(board.notesInput.value, 'My notes');
  assert.equal(board.acceptanceInput.value, 'My criterion');
  assert.equal(board.notesSection.hidden, true);
  assert.equal(board.acceptanceCount.hidden, true);
});

test('live drawer updates preserve revisions for schedule and relation forms that were not reconciled', async () => {
  const board = liveDrawerHarness();
  await board.refresh();
  assert.equal(board.scheduleRevision.value, 'r1', 'a schedule draft must still conflict with the remote revision r2');
  assert.equal(board.relationRevision.value, 'r1', 'the displayed relations were not refreshed');
  assert.equal(board.scheduledAt.value, '2026-10-01T15:00', 'keep the local schedule draft');
  assert.equal(board.title.value, 'Remote title');
  assert.equal(board.editRevision.value, 'r2', 'the clean edit fields were reconciled');
  assert.equal(board.actionRevision.value, 'r2', 'clean drawer actions remain usable');
});

test('live drawer updates preserve the revision of a typed action', async () => {
  const board = liveDrawerHarness({ dirtyAction: true });
  await board.refresh();
  assert.equal(board.actionRevision.value, 'r1');
  assert.equal(board.editRevision.value, 'r2');
});

test('conflicting edit fields retain their draft and revision during a live update', async () => {
  const board = liveDrawerHarness({ conflictingTitle: true });
  await board.refresh();
  assert.equal(board.title.value, 'My draft title');
  assert.equal(board.editRevision.value, 'r1');
  assert.equal(board.title.dataset.remoteChanged, '1');
  assert.equal(board.actionRevision.value, 'r2');
});

function rejectedFormHarness(page, { modal = false, networkError = false } = {}) {
  const mounted = [];
  const backgroundFlash = { textContent: 'Earlier notice', className: 'flash' };
  const surface = {
    querySelector(selector) { return selector === '.flash' ? mounted[0] || null : null; },
    prepend(node) { mounted.unshift(node); },
  };
  const listeners = new Map();
  const button = { disabled: false };
  const draft = input('My unsaved value');
  class Form {
    constructor() {
      this.action = 'http://127.0.0.1/actions/' + (page === 'welcome' ? 'projects/create' : 'schedule/set');
      this.classList = { add() {}, remove() {} };
    }
    getAttribute(name) { return name === 'method' ? 'post' : null; }
    closest(selector) { return modal && selector === 'dialog[open]' ? surface : null; }
    querySelectorAll() { return [button]; }
  }
  const form = new Form();
  const document = {
    body: { dataset: { page } },
    querySelector(selector) {
      if (selector === '.flash') return modal ? backgroundFlash : mounted[0] || null;
      if (selector === '.schedule-page' && page === 'schedule') return surface;
      if (selector === '.welcome-main' && page === 'welcome') return surface;
      return null;
    },
    querySelectorAll() { return []; },
    createElement() { return { setAttribute() {} }; },
    addEventListener(name, handler) { listeners.set(name, handler); },
  };
  const window = {
    location: new URL('http://127.0.0.1/' + page),
    matchMedia() { return { matches: false }; },
    addEventListener() {},
  };
  vm.runInNewContext(script, {
    window, document, URL, URLSearchParams, console,
    HTMLFormElement: Form,
    FormData: class extends Array { constructor() { super(['value', draft.value]); } },
    fetch: async () => {
      if (networkError) throw new TypeError('Failed to fetch');
      return { ok: false, status: 400, text: async () => '<div class="flash error">Invalid value</div>' };
    },
    DOMParser: class {
      parseFromString() { return { querySelector() { return { textContent: 'Invalid value' }; } }; }
    },
  }, { filename: 'app.js' });
  return {
    mounted, backgroundFlash, draft, button,
    submit: () => listeners.get('submit')({ target: form, defaultPrevented: false, preventDefault() {} }),
  };
}

test('schedule validation errors are inserted on pages without an existing flash', async () => {
  const page = rejectedFormHarness('schedule');
  await page.submit();
  assert.equal(page.mounted.length, 1);
  assert.equal(page.mounted[0].textContent, 'Invalid value');
  assert.equal(page.mounted[0].className, 'flash error');
  assert.equal(page.draft.value, 'My unsaved value');
  assert.equal(page.button.disabled, false);
});

test('project validation errors stay inside the active dialog', async () => {
  const page = rejectedFormHarness('welcome', { modal: true });
  await page.submit();
  assert.equal(page.mounted.length, 1);
  assert.equal(page.mounted[0].textContent, 'Invalid value');
  assert.equal(page.backgroundFlash.textContent, 'Earlier notice', 'the modal must not hide the error on the page behind it');
  assert.equal(page.button.disabled, false);
});

test('network failures preserve the project form and show recovery inside its dialog', async () => {
  const page = rejectedFormHarness('welcome', { modal: true, networkError: true });
  await page.submit();
  assert.equal(page.mounted.length, 1);
  assert.match(page.mounted[0].textContent, /local server is unreachable/);
  assert.equal(page.draft.value, 'My unsaved value');
  assert.equal(page.button.disabled, false);
});
