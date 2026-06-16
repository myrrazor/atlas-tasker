# Web Board Security

The Atlas web board is a local UI, not a network product.

Defaults:

- bind to `127.0.0.1`
- choose a random free port unless `--port` is supplied
- reject non-loopback hosts unless `--unsafe-host` is explicit
- require a server token/session for page routes
- require CSRF tokens for mutations
- reject cross-origin mutations when `Origin` or `Referer` is present
- send CSP, `X-Content-Type-Options`, `Referrer-Policy`, and `Cache-Control` headers
- do not set CORS headers
- escape all untrusted ticket content with `html/template`

Runtime status is written to `.tracker/runtime/web/server.json`, but session tokens and CSRF tokens are not persisted there.

Descriptions and comments render as escaped text with whitespace preserved. v1.10 intentionally does not render arbitrary Markdown to raw HTML.

