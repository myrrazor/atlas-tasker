import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const cwd = process.cwd();
const html = fs.readFileSync(path.join(cwd, "app.html"), "utf8");
const payload = JSON.parse(fs.readFileSync(path.join(cwd, "board-payload.json"), "utf8"));
let crowded = null;
const crowdedPath = path.join(cwd, "crowded-board.json");
if (fs.existsSync(crowdedPath)) {
  crowded = JSON.parse(fs.readFileSync(crowdedPath, "utf8"));
}
const scriptMatch = html.match(/<script>([\s\S]*?)<\/script>/);
if (!scriptMatch) {
  console.error("app.html has no script");
  process.exit(1);
}

class El {
  constructor(tag) {
    this.tagName = String(tag || "div").toLowerCase();
    this.children = [];
    this.attrs = {};
    this.className = "";
    this.id = "";
    this._text = "";
    this.clientWidth = 0;
    this.scrollWidth = 0;
    this.scrollHeight = 0;
    this.clientHeight = 0;
  }
  set textContent(value) {
    this._text = value == null ? "" : String(value);
    this.children = [];
  }
  get textContent() {
    if (!this.children.length) return this._text;
    return this.children.map((c) => c.textContent).join("");
  }
  appendChild(child) {
    this.children.push(child);
    return child;
  }
  replaceChildren(...kids) {
    this.children = kids;
    this._text = "";
  }
  setAttribute(name, value) {
    this.attrs[String(name)] = String(value);
  }
  getAttribute(name) {
    return this.attrs[String(name)];
  }
  removeAttribute(name) {
    delete this.attrs[String(name)];
  }
}

function collect(el, out) {
  out.push(el);
  for (const child of el.children) collect(child, out);
  return out;
}

const root = new El("div");
root.id = "root";
const htmlEl = new El("html");
htmlEl.clientWidth = 390;
htmlEl.scrollWidth = 390;
htmlEl.scrollHeight = 720;
htmlEl.clientHeight = 640;
const bodyEl = new El("body");
bodyEl.appendChild(root);
const document = {
  documentElement: htmlEl,
  body: bodyEl,
  getElementById(id) {
    if (id === "root") return root;
    return null;
  },
  createElement(tag) {
    return new El(tag);
  },
};

const parentInbox = [];
const parent = {
  postMessage(msg) {
    parentInbox.push(JSON.parse(JSON.stringify(msg)));
  },
};
const listeners = [];
const windowObj = {
  parent,
  addEventListener(type, fn) {
    if (type === "message") listeners.push(fn);
  },
  postMessage() {},
};

const context = vm.createContext({
  window: windowObj,
  document,
  console,
});
vm.runInContext(scriptMatch[1], context);

function dispatch(data, source) {
  const event = { data, source: source || parent };
  for (const fn of listeners) fn(event);
}

function fail(msg) {
  console.error("FAIL " + msg);
  process.exit(1);
}

function lastMethod(name) {
  return [...parentInbox].reverse().find((m) => m && m.method === name);
}

const init = parentInbox.find((m) => m && m.method === "ui/initialize");
if (!init) fail("app did not send ui/initialize");
if (init.id == null) fail("ui/initialize missing id");
if (init.jsonrpc !== "2.0") fail("ui/initialize missing jsonrpc");
if (!init.params || init.params.protocolVersion !== "2026-01-26") fail("missing protocolVersion");
if (!init.params.appInfo || !init.params.appInfo.name) fail("missing appInfo");
if (!init.params.appCapabilities || Object.keys(init.params.appCapabilities).length !== 0) {
  fail("appCapabilities must only declare implemented fields, got " + JSON.stringify(init.params.appCapabilities));
}

dispatch({ jsonrpc: "2.0", id: init.id, result: { protocolVersion: "2026-01-26", hostCapabilities: {} } });
const initialized = parentInbox.find((m) => m && m.method === "ui/notifications/initialized");
if (!initialized) fail("app did not send ui/notifications/initialized after result");
if (!lastMethod("ui/notifications/size-changed")) fail("missing size-changed after init");

dispatch({
  jsonrpc: "2.0",
  id: init.id,
  result: { protocolVersion: "2026-01-26", hostCapabilities: { openLinks: {} } },
});
dispatch({
  jsonrpc: "2.0",
  method: "ui/notifications/tool-result",
  params: { structuredContent: payload },
});
if (root.textContent.includes("Open board")) fail("stale id=1 init result must not enable openLinks later");

if (!/Ready/i.test(root.textContent) && !/APP-1/.test(root.textContent)) {
  fail("rendered board missing column/card: " + root.textContent);
}
if (!/ticket/i.test(root.textContent)) fail("rendered board missing counts: " + root.textContent);
if (!collect(root, []).some((c) => c.tagName === "h1")) fail("missing h1 title");
if (!collect(root, []).some((el) => el.className === "lanes")) fail("missing semantic lanes");
const articles = collect(root, []).filter((el) => el.tagName === "article");
if (!articles.length) fail("missing card articles");
if (!collect(root, []).some((el) => el.tagName === "details" && el.className === "card")) {
  fail("missing expandable card");
}
if (root.textContent.includes("127.0.0.1") || /\/w\/[0-9a-f-]{8}/i.test(root.textContent)) {
  fail("raw board URL leaked into visual content: " + root.textContent);
}
if (root.textContent.includes("managed-mode.json") && !collect(root, []).some((el) => el.tagName === "details" && el.className === "diagnostics")) {
  fail("diagnostic note shown in primary UI");
}
if (root.textContent.includes("Open board")) fail("Open board shown without host openLinks");

const before = root.textContent;
dispatch(
  {
    jsonrpc: "2.0",
    method: "ui/notifications/tool-result",
    params: { structuredContent: { payload: { board: { title: "INJECTED", columns: [{ status: "ready", label: "Ready", total: 1, cards: [{ id: "X", title: "nope" }] }] } } } },
  },
  { foreign: true }
);
if (root.textContent.includes("INJECTED")) fail("foreign window injection was accepted");
if (root.textContent !== before) fail("foreign source mutated the board");

dispatch({ jsonrpc: "2.0", method: "ui/notifications/tool-result", params: { isError: true, content: [{ type: "text", text: "backup target failed" }] } });
if (!root.textContent.includes("backup target failed")) fail("isError content not shown: " + root.textContent);
if (root.textContent.includes("0 tickets")) fail("isError rendered as empty board");

dispatch({ jsonrpc: "2.0", method: "ui/notifications/tool-cancelled" });
if (!root.textContent.includes("Tool cancelled")) fail("tool-cancelled not shown");

dispatch({
  jsonrpc: "2.0",
  method: "ui/notifications/host-context-changed",
  params: { theme: "dark", containerDimensions: { width: 320, height: 640 } },
});
if (htmlEl.getAttribute("data-theme") !== "dark") fail("dark theme not applied");
if (htmlEl.getAttribute("data-container-narrow") !== "1") fail("320px container did not mark narrow layout");

dispatch({
  jsonrpc: "2.0",
  method: "ui/notifications/host-context-changed",
  params: { theme: "light", styles: { css: { fonts: "@import url(https://evil.example/theme.css);" } }, containerDimensions: { width: 390, height: 720 } },
});
if (htmlEl.getAttribute("data-theme") !== "light") fail("light theme not applied");
if (JSON.stringify(parentInbox).includes("evil.example")) fail("remote theme URL was forwarded");

if (crowded) {
  dispatch({ jsonrpc: "2.0", method: "ui/notifications/tool-result", params: { structuredContent: crowded } });
  const crowdedText = root.textContent;
  for (const label of ["Ready", "In Progress", "Blocked", "In Review"]) {
    if (!crowdedText.includes(label)) fail("crowded board missing " + label + ": " + crowdedText);
  }
  if (htmlEl.getAttribute("data-container-narrow") !== "1") fail("390px crowded layout should stay narrow");
  if (!lastMethod("ui/notifications/size-changed")) fail("missing size-changed after crowded render");
}

dispatch({ jsonrpc: "2.0", id: 99, method: "ping" });
if (!parentInbox.find((m) => m && m.id === 99 && m.result)) fail("ping was not answered");

dispatch({ jsonrpc: "2.0", id: 77, method: "ui/resource-teardown", params: {} });
const teardownReply = parentInbox.find((m) => m && m.id === 77 && m.result && !m.method);
if (!teardownReply) fail("resource-teardown was not answered with an empty result");
if (!root.textContent.includes("App closed")) fail("teardown did not close the app");
dispatch({ jsonrpc: "2.0", method: "ui/notifications/tool-result", params: { structuredContent: payload } });
if (root.textContent.includes("Ship the board") && root.textContent.includes("App closed") === false) {
  fail("messages after teardown were still applied");
}

console.log("PASS");
console.log(before);
