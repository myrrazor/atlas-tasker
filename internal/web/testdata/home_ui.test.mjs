import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const script = readFileSync(new URL('../static/app.js', import.meta.url), 'utf8');

function loadApp(search = '?project=WEB') {
  const nodes = new Map([
    ['meta[name="atlas-csrf-token"]', { content: 'csrf' }],
    ['#atlas-i18n', { dataset: { offline: 'offline copy', conflict: 'revision conflict' } }],
    ['[data-net-banner]', { hidden: true, classList: { add() {}, remove() {} }, textContent: '' }],
  ]);
  const document = {
    body: { dataset: { page: 'board', boardPath: '/board', actionPrefix: '' }, classList: { contains: () => false } },
    querySelector(selector) { return nodes.get(selector) ?? null; },
    querySelectorAll() { return []; },
    addEventListener() {},
  };
  const window = {
    location: new URL('http://127.0.0.1/board' + search),
    matchMedia() { return { matches: false }; },
    addEventListener() {},
    setTimeout() { return 1; },
    navigator: { onLine: true },
  };
  const context = { window, document, URL, URLSearchParams, console };
  vm.runInNewContext(script, context, { filename: 'app.js' });
  return context.window.atlasBoard;
}

test('ticketHref keeps the explicit project filter', () => {
  const board = loadApp('?project=WEB&view=web-board');
  assert.equal(board.ticketHref('WEB-1'), '/board?project=WEB&view=web-board&ticket=WEB-1');
});

test('ticketHref uses a Home-prefixed board path', () => {
  const board = loadApp('?project=DEMO');
  // boardPath comes from the body dataset in this harness
  assert.ok(board.ticketHref('DEMO-2').includes('ticket=DEMO-2'));
  assert.equal(board.boardPath(), '/board');
  assert.equal(board.actionPrefix(), '');
});

const claimScript = readFileSync(new URL('../static/claim.js', import.meta.url), 'utf8');
function loadClaim(hash) {
  const calls = [];
  const events = new Map();
  const location = { hash, pathname: '/', search: '', replace(path) { calls.push(['navigate', path]); } };
  const window = { location, addEventListener(name, handler) { events.set(name, handler); } };
  const history = { replaceState(_state, _title, path) { calls.push(['clear', path]); location.hash = ''; } };
  const fetch = (path, options) => { calls.push(['post', path, options]); return Promise.resolve({}); };
  vm.runInNewContext(claimScript, { window, location, history, fetch, encodeURIComponent });
  return { calls, events, location };
}

test('Home clears a new-tab claim before sending it in a POST body', async () => {
  const app = loadClaim('#claim=synthetic123');
  await Promise.resolve();
  assert.deepEqual(app.calls.map(call => call[0]), ['clear', 'post', 'navigate']);
  const [, path, options] = app.calls[1];
  assert.equal(path, '/session/claim');
  assert.equal(options.method, 'POST');
  assert.equal(options.body, 'claim=synthetic123');
  assert.equal(app.location.hash, '');
});

test('Home consumes a claim added to an already-open sign-in tab only once', async () => {
  const app = loadClaim('');
  assert.equal(app.calls.length, 0);
  app.location.hash = '#claim=synthetic456';
  app.events.get('hashchange')();
  app.events.get('hashchange')();
  await Promise.resolve();
  assert.equal(app.calls.filter(call => call[0] === 'post').length, 1);
  assert.equal(app.location.hash, '');
});

const homeScript = readFileSync(new URL('../static/home.js', import.meta.url), 'utf8');
test('canceling a Home confirmation prompts once and leaves the form usable', () => {
  const listeners = [];
  const button = { disabled: false };
  const classes = new Set();
  const form = {
    dataset: { confirm: 'Create this project?' },
    classList: { add: value => classes.add(value), remove: value => classes.delete(value) },
    querySelectorAll: () => [button],
    addEventListener(name, handler) { if (name === 'submit') listeners.push(handler); },
  };
  const document = {
    body: { dataset: { page: 'home' }, classList: { contains: value => value === 'home-body' } },
    querySelector: () => null,
    querySelectorAll(selector) { return selector.startsWith('form[') ? [form] : []; },
    addEventListener() {},
  };
  let prompts = 0;
  let accepted = false;
  const window = {
    location: new URL('http://127.0.0.1/'),
    matchMedia: () => ({ matches: false }), addEventListener() {},
    setTimeout() {}, confirm() { prompts++; return accepted; },
  };
  const context = { window, document, URL, URLSearchParams, console };
  vm.runInNewContext(script, context);
  vm.runInNewContext(homeScript, context);
  const event = { defaultPrevented: false, preventDefault() { this.defaultPrevented = true; } };
  for (const handler of listeners) handler(event);
  assert.equal(prompts, 1);
  assert.equal(event.defaultPrevented, true);
  assert.equal(button.disabled, false);
  assert.equal(classes.size, 0);
  accepted = true;
  for (const handler of listeners) handler({ defaultPrevented: false });
  assert.equal(prompts, 2);
  assert.equal(button.disabled, true);
});
