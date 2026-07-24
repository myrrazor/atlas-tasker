import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

export const SITE_ORIGIN = "https://atlas-tasker.vercel.app";

const siteRoot = fileURLToPath(new URL("../", import.meta.url));

/** Static outputs whose absolute site origin must stay in sync. */
export const SITE_FILES = Object.freeze([
  "index.html",
  "changelog.html",
  "guide.html",
  "privacy.html",
  "terms.html",
  "robots.txt",
  "sitemap.xml",
  "llms.txt",
]);

/**
 * Return a validated HTTPS origin with no trailing slash.
 * @param {string} value Origin to check.
 * @returns {string} Normalized origin.
 * @example normalizeOrigin("https://example.com/")
 */
export function normalizeOrigin(value) {
  const url = new URL(value);
  if (url.protocol !== "https:" || url.pathname !== "/" || url.search || url.hash) {
    throw new Error("site origin must be an https origin without a path, query, or hash");
  }
  return url.origin;
}

/**
 * Read the currently rendered origin from the page's canonical link.
 * @param {string} indexHtml Rendered index markup.
 * @returns {string} Normalized canonical origin.
 * @example findCurrentOrigin('<link rel="canonical" href="https://example.com/">')
 */
export function findCurrentOrigin(indexHtml) {
  const match = indexHtml.match(/<link rel="canonical" href="([^"]+)"/);
  if (!match) throw new Error("canonical link is missing from index.html");
  return normalizeOrigin(match[1]);
}

/**
 * Replace rendered origin references without touching other URLs.
 * @param {string} contents Rendered file contents.
 * @param {string} currentOrigin Origin currently in the file.
 * @param {string} nextOrigin Replacement origin.
 * @returns {string} Updated contents.
 * @example replaceOrigin("https://old.test/", "https://old.test", "https://new.test")
 */
export function replaceOrigin(contents, currentOrigin, nextOrigin) {
  return contents.replaceAll(currentOrigin, nextOrigin);
}

/**
 * Sync the configured origin into every static SEO output.
 * @param {string} [root] Site directory, including its trailing slash.
 * @returns {Promise<{currentOrigin: string, nextOrigin: string, files: number}>} Sync summary.
 * @example await syncSiteOrigin()
 */
export async function syncSiteOrigin(root = siteRoot) {
  const indexHtml = await readFile(new URL("index.html", `file://${root}`), "utf8");
  const currentOrigin = findCurrentOrigin(indexHtml);
  const nextOrigin = normalizeOrigin(SITE_ORIGIN);

  for (const file of SITE_FILES) {
    const path = new URL(file, `file://${root}`);
    const contents = await readFile(path, "utf8");
    await writeFile(path, replaceOrigin(contents, currentOrigin, nextOrigin));
  }

  return { currentOrigin, nextOrigin, files: SITE_FILES.length };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const result = await syncSiteOrigin();
  process.stdout.write(`site origin synced to ${result.nextOrigin} in ${result.files} files\n`);
}
