import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { SITE_ORIGIN } from "./sync-site-origin.mjs";

const siteRoot = new URL("../", import.meta.url);
const pageCanonicals = new Map([
  ["index.html", `${SITE_ORIGIN}/`],
  ["cli.html", `${SITE_ORIGIN}/cli.html`],
  ["mcp.html", `${SITE_ORIGIN}/mcp.html`],
  ["docs/index.html", `${SITE_ORIGIN}/docs/`],
  ["docs/getting-started.html", `${SITE_ORIGIN}/docs/getting-started.html`],
  ["docs/tickets-and-workflow.html", `${SITE_ORIGIN}/docs/tickets-and-workflow.html`],
  ["docs/views-and-search.html", `${SITE_ORIGIN}/docs/views-and-search.html`],
  ["docs/web-board.html", `${SITE_ORIGIN}/docs/web-board.html`],
  ["docs/agents-and-dispatch.html", `${SITE_ORIGIN}/docs/agents-and-dispatch.html`],
  ["docs/mcp-setup.html", `${SITE_ORIGIN}/docs/mcp-setup.html`],
  ["docs/mcp-tools.html", `${SITE_ORIGIN}/docs/mcp-tools.html`],
  ["docs/mcp-security.html", `${SITE_ORIGIN}/docs/mcp-security.html`],
  ["docs/json-and-exit-codes.html", `${SITE_ORIGIN}/docs/json-and-exit-codes.html`],
  ["docs/faq.html", `${SITE_ORIGIN}/docs/faq.html`],
  ["changelog.html", `${SITE_ORIGIN}/changelog.html`],
  ["guide.html", `${SITE_ORIGIN}/guide.html`],
  ["privacy.html", `${SITE_ORIGIN}/privacy.html`],
  ["terms.html", `${SITE_ORIGIN}/terms.html`],
]);

const pages = new Map(
  await Promise.all(
    [...pageCanonicals].map(async ([file]) => [
      file,
      await readFile(new URL(file, siteRoot), "utf8"),
    ]),
  ),
);

const css = await readFile(new URL("styles.css", siteRoot), "utf8");
const sitemap = await readFile(new URL("sitemap.xml", siteRoot), "utf8");
const robots = await readFile(new URL("robots.txt", siteRoot), "utf8");
const llms = await readFile(new URL("llms.txt", siteRoot), "utf8");
const sourceMcpTools = await readFile(new URL("../../docs/mcp-tools.md", import.meta.url), "utf8");
const readme = await readFile(new URL("../../README.md", import.meta.url), "utf8");
const canonicalWordmark = await readFile(
  new URL("../../internal/web/static/brand/atlas-tasker-ascii.svg", import.meta.url),
);
const readmeWordmark = await readFile(
  new URL("../../assets/brand/atlas-tasker-terminal-wordmark.svg", import.meta.url),
);
const siteWordmark = await readFile(
  new URL("../atlas-tasker-terminal-wordmark.svg", import.meta.url),
);

function visibleMarkup(html) {
  return html
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/<(script|style)\b[^>]*>[\s\S]*?<\/\1\s*>/gi, "");
}

function parseAttributes(source) {
  const attributes = {};
  const pattern =
    /([^\s=/>]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g;

  for (const match of source.matchAll(pattern)) {
    attributes[match[1].toLowerCase()] =
      match[2] ?? match[3] ?? match[4] ?? "";
  }

  return attributes;
}

function tags(html, name) {
  const pattern = new RegExp(`<${name}\\b([^>]*)>`, "gi");
  return [...visibleMarkup(html).matchAll(pattern)].map((match) =>
    parseAttributes(match[1]),
  );
}

function elementContents(html, name) {
  const pattern = new RegExp(
    `<${name}\\b[^>]*>([\\s\\S]*?)<\\/${name}\\s*>`,
    "gi",
  );
  return [...visibleMarkup(html).matchAll(pattern)].map((match) => match[1]);
}

function decodeEntities(value) {
  const named = {
    amp: "&",
    apos: "'",
    gt: ">",
    lt: "<",
    nbsp: " ",
    quot: '"',
  };

  return value.replace(
    /&(#x[0-9a-f]+|#[0-9]+|[a-z]+);/gi,
    (original, entity) => {
      if (!entity.startsWith("#")) return named[entity.toLowerCase()] ?? original;

      const radix = entity[1].toLowerCase() === "x" ? 16 : 10;
      const digits = radix === 16 ? entity.slice(2) : entity.slice(1);
      const codePoint = Number.parseInt(digits, radix);
      return Number.isInteger(codePoint) && codePoint <= 0x10ffff
        ? String.fromCodePoint(codePoint)
        : original;
    },
  );
}

function textContent(markup) {
  return decodeEntities(markup.replace(/<[^>]+>/g, " "))
    .replace(/\s+/g, " ")
    .replace(/\s+([.,;:!?])/g, "$1")
    .trim();
}

function metaContent(html, key, value, file) {
  const matches = tags(html, "meta").filter(
    (attributes) => attributes[key]?.toLowerCase() === value.toLowerCase(),
  );
  assert.equal(matches.length, 1, `${file}: expected one ${value} meta tag`);
  assert.ok(matches[0].content, `${file}: ${value} has no content`);
  return textContent(matches[0].content);
}

function canonicalHref(html, file) {
  const links = tags(html, "link").filter((attributes) =>
    (attributes.rel ?? "").toLowerCase().split(/\s+/).includes("canonical"),
  );
  assert.equal(links.length, 1, `${file}: expected one canonical link`);
  return decodeEntities(links[0].href);
}

function jsonLdDocuments(html) {
  const documents = [];
  const pattern = /<script\b([^>]*)>([\s\S]*?)<\/script\s*>/gi;

  for (const match of html.matchAll(pattern)) {
    const attributes = parseAttributes(match[1]);
    if (attributes.type?.toLowerCase() === "application/ld+json") {
      documents.push(JSON.parse(match[2].trim()));
    }
  }

  return documents;
}

function hasType(node, type) {
  const types = Array.isArray(node?.["@type"]) ? node["@type"] : [node?.["@type"]];
  return types.includes(type);
}

function objectTree(root) {
  const nodes = [];
  const pending = [root];
  while (pending.length > 0) {
    const node = pending.pop();
    if (!node || typeof node !== "object") continue;
    nodes.push(node);
    pending.push(...Object.values(node));
  }
  return nodes;
}

function sectionById(html, id) {
  const markup = visibleMarkup(html);
  for (const match of markup.matchAll(/<section\b([^>]*)>/gi)) {
    if (parseAttributes(match[1]).id !== id) continue;
    const tail = markup.slice(match.index + match[0].length);
    const closingTag = /<\/section\s*>/i.exec(tail);
    assert.ok(closingTag, `section #${id} has no closing tag`);
    return tail.slice(0, closingTag.index);
  }
  assert.fail(`missing section #${id}`);
}

for (const [file, expectedCanonical] of pageCanonicals) {
  test(`${file} satisfies the public-page SEO contract`, () => {
    const html = pages.get(file);
    assert.equal(tags(html, "html")[0]?.lang?.toLowerCase(), "en");
    assert.equal(tags(html, "h1").length, 1, `${file}: expected exactly one h1`);

    const titles = elementContents(html, "title").map(textContent);
    assert.equal(titles.length, 1, `${file}: expected exactly one title`);
    assert.ok([...titles[0]].length <= 60, `${file}: title is too long`);

    const description = metaContent(html, "name", "description", file);
    assert.ok(
      [...description].length >= 140 && [...description].length <= 160,
      `${file}: description must be 140-160 characters`,
    );

    assert.equal(canonicalHref(html, file), expectedCanonical);
    assert.equal(metaContent(html, "property", "og:url", file), expectedCanonical);
    assert.equal(metaContent(html, "property", "og:image", file), `${SITE_ORIGIN}/og.png`);
    assert.equal(metaContent(html, "name", "twitter:image", file), `${SITE_ORIGIN}/og.png`);
    assert.equal(metaContent(html, "name", "twitter:card", file), "summary_large_image");

    const schemas = jsonLdDocuments(html);
    assert.ok(schemas.length > 0, `${file}: missing JSON-LD`);

    const headingLevels = [...visibleMarkup(html).matchAll(/<h([1-6])\b/gi)].map(
      (match) => Number(match[1]),
    );
    assert.equal(headingLevels[0], 1, `${file}: heading order must start at h1`);
    for (let index = 1; index < headingLevels.length; index += 1) {
      assert.ok(
        headingLevels[index] <= headingLevels[index - 1] + 1,
        `${file}: heading hierarchy skips a level`,
      );
    }

    for (const image of tags(html, "img")) {
      assert.ok(Object.hasOwn(image, "alt"), `${file}: image is missing alt text`);
      if (image.src?.startsWith("assets/")) {
        assert.equal(image.loading, "lazy", `${file}: below-fold image is not lazy`);
        assert.equal(image.decoding, "async", `${file}: below-fold image is not async`);
      }
    }

    const resourceLinks = tags(html, "link").filter((link) =>
      ["icon", "preload", "stylesheet"].includes(link.rel?.toLowerCase()),
    );
    assert.ok(
      resourceLinks.every((link) => !/^https?:/i.test(link.href)),
      `${file}: external render-blocking resource found`,
    );
    assert.ok(tags(html, "script").every((script) => !script.src));
  });
}

test("page titles are unique", () => {
  const titles = [...pages.values()].map(
    (html) => textContent(elementContents(html, "title")[0]),
  );
  assert.equal(new Set(titles).size, titles.length);
});

test("internal page links and fragments resolve", () => {
  const canonicalToFile = new Map(
    [...pageCanonicals].map(([file, canonical]) => [canonical, file]),
  );

  for (const [file, html] of pages) {
    const sourceUrl = pageCanonicals.get(file);
    for (const anchor of tags(html, "a")) {
      if (!anchor.href || /^(mailto:|tel:|javascript:)/i.test(anchor.href)) continue;
      const target = new URL(decodeEntities(anchor.href), sourceUrl);
      if (target.origin !== SITE_ORIGIN) continue;

      const targetCanonical = `${target.origin}${target.pathname}`;
      const targetFile = canonicalToFile.get(targetCanonical);
      assert.ok(targetFile, `${file}: internal link does not resolve: ${anchor.href}`);

      if (target.hash) {
        const targetHtml = pages.get(targetFile);
        const id = decodeURIComponent(target.hash.slice(1));
        const targetElements = [...visibleMarkup(targetHtml).matchAll(/<[a-z][a-z0-9-]*\b([^>]*)>/gi)]
          .map((match) => parseAttributes(match[1]));
        assert.ok(
          targetElements.some((element) => element.id === id),
          `${file}: fragment does not resolve: ${anchor.href}`,
        );
      }
    }
  }
});

test("agent integration tabs expose all six targets with valid relationships", () => {
  const mcp = pages.get("mcp.html");
  const tabButtons = tags(mcp, "button").filter((button) => button.role === "tab");
  const panels = tags(mcp, "div").filter((panel) => panel.role === "tabpanel");

  assert.deepEqual(
    tabButtons.map((button) => button.id),
    ["tab-claude", "tab-codex", "tab-cursor", "tab-openclaw", "tab-grok", "tab-generic"],
  );
  assert.equal(tabButtons.filter((button) => button["aria-selected"] === "true").length, 1);
  assert.equal(panels.length, tabButtons.length);
  for (const button of tabButtons) {
    const panel = panels.find((item) => item.id === button["aria-controls"]);
    assert.ok(panel, `${button.id}: missing controlled panel`);
    assert.equal(panel["aria-labelledby"], button.id);
  }
  assert.match(mcp, /ArrowRight/);
  assert.match(mcp, /ArrowLeft/);
});

test("MCP tool page covers every source workflow tool", () => {
  const workflowNames = [...sourceMcpTools.matchAll(/^\| `(atlas\.[^`]+)` \| workflow \|/gm)]
    .map((match) => match[1]);
  const rendered = textContent(pages.get("docs/mcp-tools.html"));

  assert.equal(workflowNames.length, 26);
  for (const name of workflowNames) {
    assert.match(rendered, new RegExp(`\\b${name.replaceAll(".", "\\.")}\\b`));
  }
});

test("README and site wordmarks match the canonical application wordmark", () => {
  assert.match(readme, /src="assets\/brand\/atlas-tasker-terminal-wordmark\.svg"/);
  assert.ok(readmeWordmark.equals(canonicalWordmark));
  assert.ok(siteWordmark.equals(canonicalWordmark));
});

test("self-hosted fonts use font-display swap", () => {
  const fontFaces = [...css.matchAll(/@font-face\s*{([\s\S]*?)}/g)].map(
    (match) => match[1],
  );
  assert.ok(fontFaces.length > 0);
  assert.ok(fontFaces.every((face) => /font-display:\s*swap/i.test(face)));
  assert.ok(fontFaces.every((face) => !/url\(["']?https?:/i.test(face)));
});

test("OG image is a 1200 by 630 PNG", async () => {
  const png = await readFile(new URL("og.png", siteRoot));
  assert.equal(png.subarray(0, 8).toString("hex"), "89504e470d0a1a0a");
  assert.equal(png.subarray(12, 16).toString("ascii"), "IHDR");
  assert.equal(png.readUInt32BE(16), 1200);
  assert.equal(png.readUInt32BE(20), 630);
});

test("sitemap, robots, and llms cover every public page", () => {
  const locations = [...sitemap.matchAll(/<loc>\s*([^<]+)\s*<\/loc>/gi)].map(
    (match) => decodeEntities(match[1].trim()),
  );
  const expected = [...pageCanonicals.values()];
  assert.equal(locations.length, new Set(locations).size, "duplicate sitemap URL");
  assert.deepEqual([...locations].sort(), [...expected].sort());
  assert.match(robots, new RegExp(`Sitemap: ${SITE_ORIGIN}/sitemap\\.xml`));
  for (const canonical of expected) assert.ok(llms.includes(canonical));
});

test("visible FAQ questions match FAQPage JSON-LD", () => {
  const index = pages.get("index.html");
  const schemas = jsonLdDocuments(index);
  assert.equal(schemas.filter((schema) => hasType(schema, "SoftwareApplication")).length, 1);
  const faqPages = schemas.filter((schema) => hasType(schema, "FAQPage"));
  assert.equal(faqPages.length, 1);

  const faqMarkup = sectionById(index, "faq");
  const visibleEntries = [...faqMarkup.matchAll(/<details>\s*<summary>([\s\S]*?)<\/summary>\s*<p>([\s\S]*?)<\/p>\s*<\/details>/gi)]
    .map((match) => [textContent(match[1]), textContent(match[2])]);
  const structuredEntries = faqPages[0].mainEntity.map((entity) => {
    assert.ok(hasType(entity, "Question"));
    assert.ok(hasType(entity.acceptedAnswer, "Answer"));
    assert.ok(entity.acceptedAnswer.text?.trim());
    return [entity.name.trim(), entity.acceptedAnswer.text.trim()];
  });
  const sorted = (values) =>
    [...values].sort(([left], [right]) => left.localeCompare(right, "en"));
  assert.deepEqual(sorted(structuredEntries), sorted(visibleEntries));
});

test("website notices explain actual use without unresolved legal templates", () => {
  for (const file of ["privacy.html", "terms.html"]) {
    const html = pages.get(file);
    const rendered = textContent(html);
    assert.doesNotMatch(rendered, /TODO\(launch\)|\{\{LEGAL_ENTITY\}\}|\{\{JURISDICTION\}\}/);
    assert.doesNotMatch(html, /mailto:/);
    assert.match(html, /github\.com\/myrrazor\/atlas-tasker/);
  }
  assert.match(textContent(pages.get("privacy.html")), /hosting provider/);
  assert.match(textContent(pages.get("terms.html")), /MIT License/);
});

test("public site omits personal author and direct contact metadata", () => {
  for (const [file, html] of pages) {
    const metadata = tags(html, "meta");
    assert.ok(
      metadata.every((item) => item.name?.toLowerCase() !== "author"),
      `${file}: personal author meta tag found`,
    );

    for (const link of tags(html, "a")) {
      const relationships = (link.rel ?? "").toLowerCase().split(/\s+/);
      assert.ok(!relationships.includes("author"), `${file}: author link found`);
      assert.ok(!relationships.includes("me"), `${file}: personal profile link found`);
      assert.doesNotMatch(link.href ?? "", /^mailto:/i, `${file}: direct email link found`);
    }

    for (const node of jsonLdDocuments(html).flatMap(objectTree)) {
      assert.ok(!hasType(node, "Person"), `${file}: Person JSON-LD found`);
      assert.ok(!Object.hasOwn(node, "email"), `${file}: email JSON-LD found`);
    }
  }
});
