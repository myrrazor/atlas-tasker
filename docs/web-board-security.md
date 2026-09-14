# Web Board Security

The Atlas web board is a local UI, not a network product.

Defaults:

- bind to `127.0.0.1`
- Home uses configured port **7432** and refuses to fall back; `web serve` chooses a random free port unless `--port` is supplied
- reject every non-loopback host; the local board has no remote-serving escape hatch
- require a server token/session for page routes
- Home clears its one-time URL fragment before `POST /session/claim` (HttpOnly cookie, no claim in request URLs or JS storage); `web serve` still uses `?token=`
- scope the session cookie name to the serving port, so two workspaces served on `127.0.0.1` don't clobber each other's session (browsers ignore ports for cookie storage)
- that cookie is essential to the local application only; the marketing website sets no cookies
- require CSRF tokens for mutations
- reject cross-origin mutations when `Origin` or `Referer` is present (`Origin: null` is treated as cross-origin)
- send CSP, `X-Content-Type-Options`, `Referrer-Policy: same-origin`, and `Cache-Control` headers
- do not set CORS headers
- escape all untrusted ticket content with `html/template`

The referrer policy must stay `same-origin`, not `no-referrer`: under `no-referrer` the Fetch spec makes browsers serialize the `Origin` header as `null` on same-origin form POSTs, so the board's own origin check would reject its own forms. `same-origin` keeps referrers private cross-origin while preserving `Origin`/`Referer` on the board's own requests. A regression test pins the header value.

Runtime status is written to `.tracker/runtime/web/server.json`, but session tokens and CSRF tokens are not persisted there.

Descriptions and comments render as escaped text with whitespace preserved. v1.10 intentionally does not render arbitrary Markdown to raw HTML.
