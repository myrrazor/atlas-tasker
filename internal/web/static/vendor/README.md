# Vendored Browser Assets

## SortableJS

- Package: `sortablejs`
- Version: `1.15.7`
- License: MIT
- Source: `https://github.com/SortableJS/Sortable`
- Vendored file: `sortable.min.js`

The web board uses SortableJS only for progressive cross-column drag/drop. Forms and buttons remain the canonical fallback.

## Geist Sans

- Package: `geist` (npm)
- Version: `1.3.1` (font (C) Vercel, in collaboration with basement.studio)
- License: SIL Open Font License 1.1 (see `geist/LICENSE`)
- Source: `https://github.com/vercel/geist-font`
- Vendored file: `geist/geist-variable.woff2` (variable weight axis)

The board's CSP blocks external hosts, so the UI font ships with the binary.
The variable axis is what allows the 650/750-weight headings and column titles.

## Geist Mono

- Package: `geist` (npm)
- Version: `1.3.1`
- License: SIL Open Font License 1.1 (see `geist-mono/LICENSE`)
- Source: `https://github.com/vercel/geist-font`
- Vendored file: `geist-mono/geist-mono-variable.woff2` (variable weight axis)

Ticket keys, timestamps, counts, and uppercase kickers render in the mono face
per the atlas.pen spec.

## Phosphor Icons

- Package: `@phosphor-icons/core`
- Version: regular weight, fetched 2026-07
- License: MIT
- Source: `https://github.com/phosphor-icons/core`
- Vendored as: inline SVG path data in `internal/web/templates/icons.html` (warning, target, chat-circle, plus, dots-six-vertical, gear)

## Inter

- Package: `@fontsource-variable/inter`
- Version: `5.2.8`
- License: SIL OFL 1.1 (see `inter/LICENSE`)
- Source: `https://github.com/rsms/inter`
- Vendored file: `inter/inter-latin-wght.woff2` (latin subset, variable weight axis)

Primary UI face for the board. The variable axis is what allows the 510-weight
card titles; Geist stays vendored as the fallback face.
