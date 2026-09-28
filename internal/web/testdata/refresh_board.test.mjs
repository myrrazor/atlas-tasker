import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('../static/app.js', import.meta.url), 'utf8');

function boardHarness(responses, { extraNodes = [], location = 'http://127.0.0.1/board?project=WEB' } = {}) {
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
  vm.runInNewContext(script, context, { filename: 'app.js' });
  return { requests, swaps, delays, flash, refresh: window.atlasBoard.refresh };
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

function liveDrawerHarness({ dirtyAction = false, conflictingTitle = false } = {}) {
  const editRevision = input('r1', 'hidden');
  const scheduleRevision = input('r1', 'hidden');
  const relationRevision = input('r1', 'hidden');
  const actionRevision = input('r1', 'hidden');
  const title = input('Original title');
  if (conflictingTitle) title.value = 'My draft title';
  const scheduledAt = input('2026-10-01T12:00', 'datetime-local');
  scheduledAt.value = '2026-10-01T15:00';
  const assignee = input('');
  if (dirtyAction) assignee.value = 'human:alice';
  const edit = {
    querySelector(selector) { return selector === '[name="title"]' ? title : null; },
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
  const board = boardHarness([{
    cards: [],
    drawer: { id: 'WEB-1', revision: 'r2', title: 'Remote title', status: 'ready' },
  }], {
    location: 'http://127.0.0.1/board?ticket=WEB-1',
    extraNodes: [['.detail-drawer', drawer]],
  });
  return { ...board, editRevision, scheduleRevision, relationRevision, actionRevision, title, scheduledAt };
}

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
