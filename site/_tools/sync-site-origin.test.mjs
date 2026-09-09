import assert from "node:assert/strict";
import test from "node:test";

import {
  SITE_FILES,
  SITE_ORIGIN,
  findCurrentOrigin,
  normalizeOrigin,
  replaceOrigin,
} from "./sync-site-origin.mjs";

test("publishes the configured Atlas production domain", () => {
  assert.equal(SITE_ORIGIN, "https://atlastasker.com");
});

test("keeps every public SEO output on the same origin", () => {
  assert.deepEqual(SITE_FILES, [
    "index.html",
    "cli.html",
    "mcp.html",
    "docs/index.html",
    "docs/getting-started.html",
    "docs/tickets-and-workflow.html",
    "docs/views-and-search.html",
    "docs/web-board.html",
    "docs/agents-and-dispatch.html",
    "docs/mcp-setup.html",
    "docs/mcp-tools.html",
    "docs/mcp-security.html",
    "docs/json-and-exit-codes.html",
    "docs/faq.html",
    "changelog.html",
    "guide.html",
    "privacy.html",
    "terms.html",
    "robots.txt",
    "sitemap.xml",
    "llms.txt",
  ]);
});

test("normalizes a canonical origin", () => {
  assert.equal(normalizeOrigin("https://atlastasker.com/"), "https://atlastasker.com");
  assert.throws(() => normalizeOrigin("http://atlastasker.com"), /https origin/);
  assert.throws(() => normalizeOrigin("https://example.com/docs"), /without a path/);
});

test("reads the current origin from the canonical link", () => {
  const html = '<link rel="canonical" href="https://old.example/" />';
  assert.equal(findCurrentOrigin(html), "https://old.example");
});

test("replaces every rendered origin reference", () => {
  const rendered = "https://old.example/ https://old.example/og.png";
  assert.equal(
    replaceOrigin(rendered, "https://old.example", "https://new.example"),
    "https://new.example/ https://new.example/og.png",
  );
});
