import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { SITE_ORIGIN } from "./sync-site-origin.mjs";

const siteRoot = new URL("../", import.meta.url);
const pageCanonicals = new Map([
  ["index.html", `${SITE_ORIGIN}/`],
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

test("legal launch markers render without tripping the placeholder gate", () => {
  for (const file of ["privacy.html", "terms.html"]) {
    const html = pages.get(file);
    assert.ok(html.startsWith("<!-- review with counsel before charging money -->"));
    const rendered = textContent(html);
    assert.match(rendered, /TODO\(launch\)/);
    assert.match(rendered, /\{\{LEGAL_ENTITY\}\}/);
    assert.match(rendered, /\{\{JURISDICTION\}\}/);
  }
});
