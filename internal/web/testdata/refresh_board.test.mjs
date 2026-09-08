import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('../static/app.js', import.meta.url), 'utf8');

function boardHarness(responses) {
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
  ]);
  const document = {
    body: { dataset: { page: 'board' } },
    querySelector(selector) { return nodes.get(selector) ?? null; },
    querySelectorAll() { return []; },
    addEventListener() {},
  };
  const window = {
    location: new URL('http://127.0.0.1/board?project=WEB'),
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
